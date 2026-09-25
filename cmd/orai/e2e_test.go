package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests build the real binary and run it from outside the checkout with a
// minimal PATH, so nothing can accidentally come from the source tree or the host.

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "orai-e2e-")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "orai")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func orai(t *testing.T, dir string, stdin string, extraPath string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	gitDir := ""
	if git, err := exec.LookPath("git"); err == nil {
		gitDir = filepath.Dir(git)
	}
	path := strings.Join([]string{extraPath, gitDir, "/usr/bin", "/bin"}, string(os.PathListSeparator))
	cmd.Env = []string{"PATH=" + path, "HOME=" + t.TempDir()}
	cmd.Stdin = strings.NewReader(stdin)
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

func TestSetupTwiceIsANoOpWithEmbeddedTemplates(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := orai(t, project, "", "", "setup", "--preset", "pm-staff", "--no-tools")
	if code != 0 && code != 1 {
		t.Fatalf("setup: %d %s %s", code, out, errOut)
	}
	for _, want := range []string{"Initialize a Git repository on branch trunk", "Initialize mailboxes"} {
		if !strings.Contains(out, want) {
			t.Fatalf("setup output lacks %q:\n%s", want, out)
		}
	}
	skill, _ := os.ReadFile(filepath.Join(project, ".agents/skills/orai/SKILL.md"))
	source, _ := os.ReadFile(filepath.Join(repoRoot(), "internal/scaffold/templates/skill.md"))
	if len(skill) == 0 || !bytes.Equal(skill, source) {
		t.Fatal("installed skill differs from the embedded template")
	}
	out, _, _ = orai(t, project, "", "", "setup", "--no-tools")
	if !strings.Contains(out, "Files already current.") {
		t.Fatalf("second setup changed files:\n%s", out)
	}
}

func TestDryRunHookPointsAtThisBinaryAndNotTheCheckout(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	_ = os.Mkdir(project, 0o755)
	orai(t, project, "", "", "setup", "--preset", "pm-staff", "--no-tools")
	_ = os.MkdirAll(filepath.Join(project, ".worktrees/staff"), 0o755)
	out, errOut, code := orai(t, project, "", "", "pm", "--dry-run")
	if code != 0 {
		t.Fatalf("dry run: %d %s", code, errOut)
	}
	var plan struct{ Command []string }
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Command, " ")
	if strings.Contains(joined, repoRoot()) {
		t.Fatal("launch plan refers to the source checkout")
	}
	resolved, _ := filepath.EvalSymlinks(binary)
	if !strings.Contains(joined, resolved+" --project ") {
		t.Fatalf("hook does not run this binary: %s", joined)
	}
}

func TestMessagesAndDoctorFromTheBinary(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	_ = os.Mkdir(project, 0o755)
	orai(t, project, "", "", "setup", "--preset", "pm-staff", "--no-tools")
	if _, errOut, code := orai(t, project, "hello from desktop", "", "msg", "send", "pm", "--as", "user"); code != 0 {
		t.Fatalf("send: %s", errOut)
	}
	out, _, _ := orai(t, project, "", "", "status")
	if !strings.Contains(out, `"pending": 1`) {
		t.Fatalf("status %s", out)
	}
	out, _, code := orai(t, project, "", "", "doctor")
	var report struct {
		Status string
		Checks []struct{ Component string }
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("doctor output: %v\n%s", err, out)
	}
	components := map[string]bool{}
	for _, c := range report.Checks {
		components[c.Component] = true
	}
	for _, want := range []string{"project", "mail", "role.pm", "role.staff", "tool.codex", "tool.claude"} {
		if !components[want] {
			t.Fatalf("doctor lacks %s: %v", want, components)
		}
	}
	if code == 0 {
		t.Fatal("providers are not on PATH here, so doctor must not report success")
	}
}

func TestChannelStartsAndStopsOnEOF(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(binary, "_channel")
	cmd.Env = []string{"ORAI_RUN_NONCE=n", "ORAI_ROLE=staff", "ORAI_STATE_DIR=" + dir, "ORAI_MAIL_ROOT=" + filepath.Join(dir, "none")}
	cmd.Stdin = strings.NewReader("")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "staff.channel.json"))
	if !strings.Contains(string(data), `"ready": false`) {
		t.Fatalf("state %s", data)
	}
}
