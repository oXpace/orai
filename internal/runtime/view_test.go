package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/state"
)

// Contract tests for what a person sees (openspec role-sessions): the start and end of
// a run, and the state of one role.

func TestStartIsThreeLines(t *testing.T) {
	f := newFixture(t)
	name := f.p.Name()
	two, none := 2, 0
	lead, dev := f.p.Config.Roles["lead"], f.p.Config.Roles["dev"]
	dev.Worktree = ".worktrees/dev"
	bare := lead
	bare.Model, bare.Effort = "", ""

	for _, test := range []struct {
		label string
		show  func(view)
		want  string
	}{
		{"resumed", func(v view) { v.started(f.p, dev, sid, &two, nil) },
			"● " + name + " › dev  이어서 ed936671\n  Claude · opus · high\n  .worktrees/dev · 새 메시지 2\n"},
		{"new, defaults", func(v view) { v.started(f.p, bare, "", &none, nil) },
			"● " + name + " › lead  새 대화\n  Codex · 기본 모델\n  프로젝트 폴더 · 새 메시지 0\n"},
		{"mail unreadable", func(v view) { v.started(f.p, lead, "", nil, nil) },
			"● " + name + " › lead  새 대화\n  Codex · gpt-test · medium\n  프로젝트 폴더 · 새 메시지 확인 실패\n"},
		{"mailbox added", func(v view) { v.started(f.p, lead, "", &none, []string{"qa", "ops"}) },
			"● " + name + " › lead  새 대화\n  Codex · gpt-test · medium\n  프로젝트 폴더 · 새 메시지 0\n  메일함 추가: qa, ops\n"},
	} {
		var out bytes.Buffer
		test.show(view{out: &out})
		if out.String() != test.want {
			t.Errorf("%s:\n got %q\nwant %q", test.label, out.String(), test.want)
		}
		out.Reset()
		test.show(view{out: &out, quiet: true})
		if out.Len() != 0 {
			t.Errorf("%s: quiet printed %q", test.label, out.String())
		}
	}
}

func TestEndNamesTheNextStep(t *testing.T) {
	const (
		noResume = "! 이어갈 수 없음 → orai %s --fresh"
		approve  = " 후 hook 승인"
		muted    = "! 이번엔 알림 꺼짐 → 다음 실행에서 hook 승인"
		blocked  = "! 다시 실행할 수 없음 → orai doctor"
	)
	for _, test := range []struct {
		label    string
		role     string
		next     nextRun
		captured bool
		code     int
		warning  string
	}{
		{"codex ok", "lead", runResumes, true, 0, ""},
		{"codex ok, failed exit", "lead", runResumes, true, 1, ""},
		{"codex resumed without capture", "lead", runResumes, false, 0, muted},
		{"codex resumed, provider failed", "lead", runResumes, false, 1, ""},
		{"codex new without capture", "lead", runNeedsFresh, false, 0, noResume + approve},
		{"codex new, provider failed", "lead", runNeedsFresh, false, 1, noResume},
		{"codex new, killed", "lead", runNeedsFresh, false, -1, noResume},
		{"codex cleared, no transcript yet", "lead", runNeedsFresh, true, 0, noResume},
		{"codex blocked", "lead", runBlocked, false, 0, blocked},
		{"codex state gone", "lead", runStarts, false, 0, blocked},
		{"claude ok", "dev", runResumes, true, 0, ""},
		{"claude resumed without capture", "dev", runResumes, false, 0, ""},
		{"claude new without capture", "dev", runNeedsFresh, false, 0, noResume},
		{"claude cleared, no transcript yet", "dev", runNeedsFresh, true, 0, noResume},
		{"claude blocked", "dev", runBlocked, true, 0, blocked},
	} {
		f := newFixture(t)
		header := "○ " + f.p.Name() + " › " + test.role + " 종료"
		if test.next == runResumes {
			header += " · 이어가기: orai " + test.role
		}
		warning := strings.ReplaceAll(test.warning, "%s", test.role)
		if warning != "" {
			warning += "\n"
		}
		for _, quiet := range []bool{false, true} {
			want := header + "\n" + warning
			if quiet {
				want = warning
			}
			var out bytes.Buffer
			view{out: &out, quiet: quiet}.stopped(f.p, f.p.Config.Roles[test.role], test.next, test.captured, test.code)
			if out.String() != want {
				t.Errorf("%s (quiet %v):\n got %q\nwant %q", test.label, quiet, out.String(), want)
			}
		}
	}
}

func TestNextRunIsJudgedLikeLaunch(t *testing.T) {
	f := newFixture(t)
	role := f.p.Config.Roles["lead"]
	if got := judgeNextRun(f.p, role); got != runStarts {
		t.Fatalf("no state: %v", got)
	}
	f.state(t, map[string]any{"session_id": nil})
	if got := judgeNextRun(f.p, role); got != runNeedsFresh {
		t.Fatalf("no identity: %v", got)
	}
	transcript := filepath.Join(f.p.Root, "transcript.jsonl")
	f.state(t, map[string]any{"transcript": transcript})
	if got := judgeNextRun(f.p, role); got != runNeedsFresh {
		t.Fatalf("no transcript: %v", got)
	}
	if err := os.WriteFile(transcript, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := judgeNextRun(f.p, role); got != runResumes {
		t.Fatalf("resumable: %v", got)
	}

	gone := role
	gone.Worktree = ".worktrees/gone"
	if got := judgeNextRun(f.p, gone); got != runBlocked {
		t.Fatalf("missing folder: %v", got)
	}
	branched := role
	branched.Branch = "release" // the project folder is not on it
	if got := judgeNextRun(f.p, branched); got != runBlocked {
		t.Fatalf("wrong branch: %v", got)
	}
	if err := os.WriteFile(f.files.State(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := judgeNextRun(f.p, role); got != runBlocked {
		t.Fatalf("unreadable state: %v", got)
	}
}

func TestColorFollowsTheStream(t *testing.T) {
	original := terminal
	t.Cleanup(func() { terminal = original })
	for _, test := range []struct {
		label, noColor, term string
		tty, color           bool
	}{
		{"terminal", "", "xterm-256color", true, true},
		{"not a terminal", "", "xterm-256color", false, false},
		{"NO_COLOR", "1", "xterm-256color", true, false},
		{"dumb", "", "dumb", true, false},
	} {
		t.Setenv("NO_COLOR", test.noColor)
		t.Setenv("TERM", test.term)
		terminal = func(io.Writer) bool { return test.tty }
		f := newFixture(t)
		var out bytes.Buffer
		v := newView(&out, false)
		v.started(f.p, f.p.Config.Roles["lead"], sid, nil, nil)
		v.stopped(f.p, f.p.Config.Roles["lead"], runNeedsFresh, false, 0)
		if got := strings.Contains(out.String(), "\x1b["); got != test.color {
			t.Errorf("%s: color %v in %q", test.label, got, out.String())
		}
	}
}

func TestRoleTableSaysWhatStatusJSONSays(t *testing.T) {
	f := newFixture(t)
	root := mail.Root(f.p.MailRoot())
	if err := mail.Init(root, f.p.Config.Handles()); err != nil {
		t.Fatal(err)
	}
	table := func() string {
		t.Helper()
		var out bytes.Buffer
		if err := StatusRole(f.p, "lead", &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	expect := func(run, talk, notify string) {
		t.Helper()
		want := f.p.Name() + " › lead\n" +
			"  상태    " + run + "\n" +
			"  실행    Codex · gpt-test · medium\n" +
			"  대화    " + talk + "\n" +
			"  폴더    프로젝트 폴더\n" +
			"  알림    " + notify + "\n" +
			"  메일    새 메시지 0\n"
		if got := table(); got != want {
			t.Fatalf("got\n%s\nwant\n%s", got, want)
		}
	}

	expect("멈춤 · 시작: orai lead", "-", "-")

	f.state(t, map[string]any{"status": "stopped", "session_id": nil})
	expect("멈춤 · 이어갈 수 없음 → orai lead --fresh", "-", "-")

	transcript := filepath.Join(f.p.Root, "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.state(t, map[string]any{"status": "stopped", "transcript": transcript})
	expect("멈춤 · 이어가기: orai lead", "ed936671", "-")

	lock, err := state.RoleLock(f.files)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	f.state(t, map[string]any{"status": "running", "captured_at": nil, "transcript": transcript})
	expect("실행 중", "ed936671", "준비 안 됨")

	f.state(t, map[string]any{"status": "running", "captured_at": 1.0, "transcript": transcript})
	deliver := func(ready bool, failure any) {
		t.Helper()
		if err := state.WriteJSON(f.files.CodexDelivery(),
			map[string]any{"nonce": "run-one", "ready": ready, "last_error": failure}); err != nil {
			t.Fatal(err)
		}
	}
	deliver(true, nil)
	expect("실행 중", "ed936671", "준비됨")

	deliver(false, "codex queue: exit status 1: no thread\nsecond line")
	expect("실행 중", "ed936671", "오류: codex queue: exit status 1: no thread")

	var out bytes.Buffer
	if err := Status(f.p, &out); err != nil {
		t.Fatal(err)
	}
	var all []RoleStatus
	if err := json.Unmarshal(out.Bytes(), &all); err != nil || len(all) != 2 {
		t.Fatalf("status JSON %v: %s", err, out.String())
	}
}

func TestRoleTableRefusesUnknownRole(t *testing.T) {
	f := newFixture(t)
	err := StatusRole(f.p, "senior", &bytes.Buffer{})
	if _, usage := err.(*UsageError); !usage || !strings.Contains(err.Error(), `unknown role "senior" (configured: dev, lead)`) {
		t.Fatal(err)
	}
}
