package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A fake provider stands in for Codex and Claude. With FAKE_CAPTURE it does what the
// real SessionStart hook does: run `orai _capture` in the role's environment.
const fakeProvider = `#!/bin/sh
if [ -n "$FAKE_CAPTURE" ]; then
  printf '{"hook_event_name":"SessionStart","source":"startup","cwd":"%s","session_id":"%s","transcript_path":"%s"}' \
    "$(pwd -P)" "$FAKE_CAPTURE" "$FAKE_TRANSCRIPT" | "$FAKE_ORAI" --project "$ORAI_PROJECT" _capture >/dev/null || exit 9
fi
exit ${FAKE_EXIT:-0}
`

func TestRunShowsWhatStartedAndHowItEnded(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	bin := t.TempDir()
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "schema = 2\nsession = \"orai\"\n\n[roles.lead]\nprovider = \"codex\"\nworktree = \".\"\n" +
		"\n[roles.dev]\nprovider = \"claude\"\nworktree = \".\"\n"
	if err := os.WriteFile(filepath.Join(project, "orai.toml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "claude"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(fakeProvider), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	transcript := filepath.Join(bin, "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	run := func(env []string, args ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = project
		// No stdin: the child reads the null device, a character device like a terminal.
		cmd.Env = append([]string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + home, "TERM=xterm",
			"FAKE_ORAI=" + binary, "FAKE_TRANSCRIPT=" + transcript}, env...)
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		err := cmd.Run()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return out.String(), errOut.String(), code
	}
	const id = "ed936671-e8a5-4d6d-a38f-c5bcc152ab10"
	const folder = "  Codex · 기본 모델\n  프로젝트 폴더 · 새 메시지 0\n"
	fresh := "● app › lead  새 대화\n" + folder
	resumed := "● app › lead  이어서 ed936671\n" + folder
	ended := "○ app › lead 종료"
	next := ended + " · 이어가기: orai lead\n"

	for _, step := range []struct {
		label  string
		env    []string
		args   []string
		stderr string
		code   int
	}{
		{"new, hook skipped", nil, []string{"lead"},
			fresh + ended + "\n! 이어갈 수 없음 → orai lead --fresh 후 hook 승인\n", 0},
		{"new, captured", []string{"FAKE_CAPTURE=" + id}, []string{"lead", "--fresh"}, fresh + next, 0},
		{"resumed, captured", []string{"FAKE_CAPTURE=" + id}, []string{"run", "lead"}, resumed + next, 0},
		{"resumed, hook skipped", nil, []string{"lead"},
			resumed + next + "! 이번엔 알림 꺼짐 → 다음 실행에서 hook 승인\n", 0},
		{"provider failed", []string{"FAKE_EXIT=7"}, []string{"lead"}, resumed + next, 7},
		{"captured, no transcript", []string{"FAKE_CAPTURE=" + id, "FAKE_TRANSCRIPT=" + filepath.Join(bin, "none.jsonl")},
			[]string{"lead", "--fresh"}, fresh + ended + "\n! 이어갈 수 없음 → orai lead --fresh\n", 0},
		{"captured again", []string{"FAKE_CAPTURE=" + id}, []string{"lead", "--fresh"}, fresh + next, 0},
		{"claude without capture", nil, []string{"dev"},
			"● app › dev  새 대화\n  Claude · 기본 모델\n  프로젝트 폴더 · 새 메시지 0\n" +
				"○ app › dev 종료\n! 이어갈 수 없음 → orai dev --fresh\n", 0},
		{"quiet", []string{"FAKE_CAPTURE=" + id}, []string{"lead", "--quiet"}, "", 0},
		{"quiet keeps warnings", nil, []string{"run", "lead", "-q"}, "! 이번엔 알림 꺼짐 → 다음 실행에서 hook 승인\n", 0},
	} {
		out, errOut, code := run(step.env, step.args...)
		if code != step.code || out != "" || errOut != step.stderr {
			t.Fatalf("%s: code %d\nstdout %q\nstderr %q\n  want %q", step.label, code, out, errOut, step.stderr)
		}
	}

	out, errOut, code := run(nil, "lead", "--dry-run")
	if code != 0 || errOut != "" || !strings.HasPrefix(out, "{") || !strings.Contains(out, `"resume": true`) {
		t.Fatalf("dry run: %d %q %q", code, out, errOut)
	}
	out, errOut, code = run(nil, "status", "lead")
	if code != 0 || errOut != "" || !strings.HasPrefix(out, "app › lead\n  상태    멈춤 · 이어가기: orai lead\n") {
		t.Fatalf("status lead: %d %q %q", code, out, errOut)
	}
	if out, _, code = run(nil, "status"); code != 0 || !strings.HasPrefix(out, "[") {
		t.Fatalf("status: %d %q", code, out)
	}
	if _, errOut, code = run(nil, "status", "senior"); code != 2 || !strings.Contains(errOut, `unknown role "senior"`) {
		t.Fatalf("status senior: %d %q", code, errOut)
	}
	if _, _, code = run(nil, "status", "lead", "extra"); code != 2 {
		t.Fatalf("status with two names: %d", code)
	}

	// Asking for help never starts, or with --fresh replaces, a role.
	saved := filepath.Join(project, ".orai")
	before := tree(t, saved)
	for _, args := range [][]string{{"lead", "-q", "--help"}, {"run", "lead", "--quiet", "--help"}, {"lead", "--fresh", "-q", "-h"}} {
		if out, errOut, code = run(nil, args...); code != 0 || errOut != "" || !strings.HasPrefix(out, "usage: orai run") {
			t.Fatalf("%v: %d %q %q", args, code, out, errOut)
		}
	}
	if after := tree(t, saved); after != before {
		t.Fatalf("help changed state:\n%s\n%s", before, after)
	}
}

func tree(t *testing.T, root string) string {
	t.Helper()
	var all strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		all.WriteString(path + "\n")
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			all.Write(data)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return all.String()
}
