package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
)

const twoRoles = `schema = 1
session = "orai"

[roles.lead]
provider = "codex"

[roles.dev]
provider = "claude"
`

func clearEnv(t *testing.T) {
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "ORAI_") || strings.HasPrefix(key, "AM_") {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
}

func newProject(t *testing.T, dir string) *project.Project {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orai.toml"), []byte(twoRoles), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := mail.Init(mail.Root(p.MailRoot()), p.Config.Handles()); err != nil {
		t.Fatal(err)
	}
	return p
}

func asRole(t *testing.T, p *project.Project, role string) {
	t.Setenv("ORAI_ROLE", role)
	t.Setenv("ORAI_PROJECT", p.Root)
	t.Setenv("ORAI_SESSION", p.Config.Session)
	t.Setenv("AM_ME", role)
}

type result struct {
	code        int
	out, errOut string
}

func run(stdin string, argv ...string) result {
	var out, errOut bytes.Buffer
	code := Main(argv, strings.NewReader(stdin), &out, &errOut)
	return result{code, out.String(), errOut.String()}
}

func TestRoleIdentityIsRequiredAndCannotBeOverridden(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	t.Chdir(p.Root)
	if r := run("", "msg", "inbox"); r.code != 1 || !strings.Contains(r.errOut, "--as user") {
		t.Fatalf("outside a role without --as: %+v", r)
	}
	asRole(t, p, "lead")
	if r := run("", "msg", "inbox", "--as", "user"); r.code != 1 || !strings.Contains(r.errOut, "cannot override") {
		t.Fatalf("override: %+v", r)
	}
	t.Setenv("AM_ME", "dev")
	if r := run("", "msg", "inbox"); r.code != 1 || !strings.Contains(r.errOut, "incomplete") {
		t.Fatalf("mismatched identity: %+v", r)
	}
	t.Setenv("AM_ME", "lead")
	t.Setenv("ORAI_ROLE", "Bad Role")
	if r := run("", "msg", "inbox"); r.code != 1 {
		t.Fatalf("invalid role: %+v", r)
	}
}

func TestSendPeekReceiveReplyFlow(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	asRole(t, p, "lead")
	body := "@literal $(do-not-run) `literal`\n두 번째 줄"
	if r := run("", "msg", "send", "dev", "--kind", "question", "--body", body); r.code != 0 {
		t.Fatalf("send %+v", r)
	}
	asRole(t, p, "dev")
	peek := run("", "msg", "inbox", "--peek")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(peek.out), &listed); err != nil || len(listed) != 1 {
		t.Fatalf("peek %+v", peek)
	}
	id := listed[0]["id"].(string)
	received := run("", "msg", "inbox")
	var drained struct {
		Drained []struct{ ID, Body, Thread string }
		Count   int
	}
	_ = json.Unmarshal([]byte(received.out), &drained)
	if drained.Count != 1 || strings.TrimRight(drained.Drained[0].Body, "\n") != body || drained.Drained[0].ID != id {
		t.Fatalf("drain %+v", received)
	}
	if r := run("answer\n근거", "msg", "reply", id); r.code != 0 {
		t.Fatalf("reply via stdin %+v", r)
	}
	asRole(t, p, "lead")
	reply := run("", "msg", "inbox")
	if !strings.Contains(reply.out, `"kind": "answer"`) || !strings.Contains(reply.out, drained.Drained[0].Thread) {
		t.Fatalf("reply %s", reply.out)
	}
	if r := run("", "msg", "send", "dev", "--body", " "); r.code != 1 {
		t.Fatalf("empty body %+v", r)
	}
	asRole(t, p, "dev")
	if r := run("", "msg", "reply", "missing", "--body", "x"); r.code == 0 {
		t.Fatal("reply to a missing id succeeded")
	}
	file := filepath.Join(t.TempDir(), "body.md")
	_ = os.WriteFile(file, []byte("파일 본문"), 0o600)
	asRole(t, p, "lead")
	run("", "msg", "send", "dev", "--file", file)
	run("", "msg", "send", "dev", "--body", "second")
	asRole(t, p, "dev")
	var pending []map[string]any
	_ = json.Unmarshal([]byte(run("", "msg", "inbox", "--peek").out), &pending)
	target := pending[0]["id"].(string)
	for _, extra := range [][]string{{"--peek"}, {"--limit", "1"}} {
		if r := run("", append([]string{"msg", "inbox", target}, extra...)...); r.code != 1 {
			t.Fatalf("id with %v: %+v", extra, r)
		}
	}
	direct := run("", "msg", "inbox", target)
	if direct.code != 0 || !strings.Contains(direct.out, "파일 본문") {
		t.Fatalf("direct %+v", direct)
	}
	var remaining []map[string]any
	_ = json.Unmarshal([]byte(run("", "msg", "inbox", "--peek").out), &remaining)
	if len(remaining) != 1 || remaining[0]["id"] == target {
		t.Fatalf("remaining %v", remaining)
	}
}

// blockingReader fails the test if the CLI tries to read an interactive terminal.
type blockingReader struct{ t *testing.T }

func (b blockingReader) Read([]byte) (int, error) {
	b.t.Fatal("interactive stdin must not be read")
	return 0, nil
}

func TestInteractiveStdinWithoutBodyIsRejectedBeforeReading(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	asRole(t, p, "lead")
	saved := isTerminal
	defer func() { isTerminal = saved }()
	isTerminal = func(io.Reader) bool { return true }
	for _, argv := range [][]string{{"msg", "send", "dev"}, {"msg", "reply", "message-id"}} {
		var out, errOut bytes.Buffer
		if code := Main(argv, blockingReader{t}, &out, &errOut); code != 1 ||
			!strings.Contains(errOut.String(), "--body, --file, or piped stdin") {
			t.Fatalf("%v: code %d %s", argv, code, errOut.String())
		}
	}
	if pending, _ := mail.Root(p.MailRoot()).Pending("dev"); len(pending) != 0 {
		t.Fatal("a message was sent")
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	other := newProject(t, t.TempDir())
	asRole(t, p, "lead")
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"msg", "send", "senior", "--body", "x"}, "dev, lead, user"},
		{[]string{"msg", "inbox", "--project", other.Root}, "another project"},
		{[]string{"msg", "inbox", "--limit", "-1"}, "non-negative"},
		{[]string{"msg", "send", "dev", "--kind", "chat", "--body", "x"}, "--kind"},
		{[]string{"inbox"}, "orai msg inbox"},
		{[]string{"qmd", "check"}, "orai wiki"},
		{[]string{"init"}, "orai setup"},
	} {
		if r := run("", tc.argv...); r.code != 2 || !strings.Contains(r.errOut, tc.want) {
			t.Fatalf("%v: %+v", tc.argv, r)
		}
	}
}

func TestDesktopFromRoleWorktreeUsesMainCheckout(t *testing.T) {
	clearEnv(t)
	main := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", main, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "trunk")
	p := newProject(t, main)
	git("add", "orai.toml")
	git("commit", "-qm", "init")
	worktree := filepath.Join(main, ".worktrees", "dev")
	git("worktree", "add", "-q", "-b", "dev", worktree)
	t.Chdir(worktree)
	if r := run("", "msg", "send", "lead", "--as", "user", "--body", "from desktop"); r.code != 0 {
		t.Fatalf("send %+v", r)
	}
	pending, _ := mail.Root(p.MailRoot()).Pending("lead")
	if len(pending) != 1 {
		t.Fatalf("message did not reach the main checkout's mailbox: %v", pending)
	}
}

func TestRoleAliasAndVersion(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	if r := run("", "--project", p.Root, "senior"); r.code != 2 || !strings.Contains(r.errOut, `unknown role "senior"`) {
		t.Fatalf("alias %+v", r)
	}
	if r := run("", "--version"); r.code != 0 || !strings.HasPrefix(r.out, "orai ") {
		t.Fatalf("version %+v", r)
	}
	if r := run("", "status", "--project", p.Root); r.code != 0 || !strings.Contains(r.out, `"role": "dev"`) {
		t.Fatalf("status %+v", r)
	}
}
