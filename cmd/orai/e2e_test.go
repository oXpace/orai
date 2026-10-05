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

func TestHelpForACommandChangesNothing(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	_ = os.Mkdir(project, 0o755)
	for _, argv := range [][]string{{"setup", "--help"}, {"setup", "-h"}, {"doctor", "--help"}, {"shelf", "--help"},
		{"msg", "--help"}, {"msg", "send", "--help"}, {"run", "--help"}, {"status", "--help"}} {
		out, errOut, code := orai(t, project, "", "", argv...)
		if code != 0 || !strings.HasPrefix(out, "usage: orai "+argv[0]) {
			t.Fatalf("%v: %d %q %q", argv, code, out, errOut)
		}
	}
	if entries, _ := os.ReadDir(project); len(entries) != 0 {
		t.Fatalf("--help created files: %v", entries)
	}
}

func TestRolesChosenAtSetupAndAddedLater(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	_ = os.Mkdir(project, 0o755)
	out, errOut, code := orai(t, project, "", "", "setup", "--role", "lead=codex", "--role", "dev=claude", "--no-tools")
	if code != 0 {
		t.Fatalf("setup: %d %s %s", code, out, errOut)
	}
	// What is left is spelled out as commands, in order.
	for _, want := range []string{"Diagnosis: ", "Next steps:", "git worktree add .worktrees/dev"} {
		if !strings.Contains(out, want) {
			t.Fatalf("setup output lacks %q:\n%s", want, out)
		}
	}
	out, errOut, code = orai(t, project, "", "", "setup", "--role", "reviewer=claude", "--no-tools")
	if code != 0 || !strings.Contains(out, "Add role(s) reviewer to orai.toml") {
		t.Fatalf("adding a role: %d %s %s", code, out, errOut)
	}
	status, _, _ := orai(t, project, "", "", "status")
	for _, role := range []string{"lead", "dev", "reviewer"} {
		if !strings.Contains(status, `"role": "`+role+`"`) {
			t.Fatalf("status lacks %s: %s", role, status)
		}
	}
	if _, errOut, code := orai(t, project, "", "", "setup", "--role", "dev=gpt"); code != 2 || !strings.Contains(errOut, "NAME=PROVIDER") {
		t.Fatalf("bad --role: %d %s", code, errOut)
	}
	if _, errOut, code := orai(t, project, "", "", "setup", "--role", "dev=codex", "--no-tools"); code != 2 || !strings.Contains(errOut, "already declared") {
		t.Fatalf("conflicting --role: %d %s", code, errOut)
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
	out, _, code := orai(t, project, "", "", "doctor", "--json")
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
	// The default output is for people: marks, a status line and numbered next steps.
	text, _, textCode := orai(t, project, "", "", "doctor")
	if textCode != code {
		t.Fatalf("exit code differs between formats: %d vs %d", textCode, code)
	}
	for _, want := range []string{"Orai project: ", "✓ project", "Status: " + report.Status, "Next steps:\n  1. "} {
		if !strings.Contains(text, want) {
			t.Fatalf("doctor output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "docs/compatibility.md)") || strings.Contains(text, "(docs/operations.md)") {
		t.Fatalf("doctor points at a document this project does not have:\n%s", text)
	}
	empty := t.TempDir()
	out, _, code = orai(t, empty, "", "", "doctor")
	if code != 1 || !strings.Contains(out, "No Orai project here") || !strings.Contains(out, "orai setup") {
		t.Fatalf("doctor outside a project: %d %s", code, out)
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

// A project set up by 0.3 keeps working after the binary is upgraded: its schema 1
// orai.toml is read as written, doctor names the two-line edit, and the old command
// name points at the new one.
func TestProjectFromZeroThreeKeepsWorkingAndIsToldWhatToEdit(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	_ = os.Mkdir(project, 0o755)
	orai(t, project, "", "", "setup", "--role", "lead=codex", "--no-tools")
	file := filepath.Join(project, "orai.toml")
	data, _ := os.ReadFile(file)
	old := strings.NewReplacer("schema = 2", "schema = 1", "[integrations.shelf]", "[integrations.wiki]").Replace(string(data))
	if old == string(data) || !strings.Contains(old, "[integrations.wiki]") {
		t.Fatalf("fixture was not rewritten:\n%s", data)
	}
	if err := os.WriteFile(file, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, _ := orai(t, project, "", "", "doctor")
	for _, want := range []string{"! project", "uses schema 1", "set `schema = 2` and rename `[integrations.wiki]` to `[integrations.shelf]`", "shelf"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output lacks %q:\n%s%s", want, out, errOut)
		}
	}
	if status, _, code := orai(t, project, "", "", "status"); code != 0 || !strings.Contains(status, `"role": "lead"`) {
		t.Fatalf("status with a schema 1 file: %d %s", code, status)
	}
	if _, errOut, code := orai(t, project, "", "", "wiki", "refresh"); code != 2 || !strings.Contains(errOut, "`wiki` moved; use `orai shelf`") {
		t.Fatalf("old command name: %d %s", code, errOut)
	}
	after, _ := os.ReadFile(file)
	if string(after) != old {
		t.Fatal("orai.toml was rewritten")
	}
}

// A project keeps its shared settings in orai.toml and each computer its own in
// orai.local.toml. From the binary: a local role is added without touching orai.toml, a
// local model is what a role would be started with, doctor and status say where the
// settings came from, and a broken local file is a usage error that names the file.
func TestLocalSettingsFromTheBinary(t *testing.T) {
	project := filepath.Join(t.TempDir(), "app")
	_ = os.Mkdir(project, 0o755)
	orai(t, project, "", "", "setup", "--role", "lead=codex", "--no-tools")
	shared, _ := os.ReadFile(filepath.Join(project, "orai.toml"))

	if _, errOut, code := orai(t, project, "", "", "setup", "--local", "--no-tools"); code != 2 || !strings.Contains(errOut, "--local goes with --role") {
		t.Fatalf("--local alone: %d %s", code, errOut)
	}
	out, errOut, code := orai(t, project, "", "", "setup", "--role", "scratch=claude:.", "--local", "--no-tools")
	if code != 0 || !strings.Contains(out, "Create orai.local.toml with role(s) scratch (this computer only)") {
		t.Fatalf("local role: %d %s %s", code, out, errOut)
	}
	if after, _ := os.ReadFile(filepath.Join(project, "orai.toml")); string(after) != string(shared) {
		t.Fatalf("orai.toml changed:\n%s", after)
	}
	local := filepath.Join(project, "orai.local.toml")
	text, _ := os.ReadFile(local)
	if err := os.WriteFile(local, append(text, []byte("\n[roles.lead]\nmodel = \"local-model\"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	out, errOut, code = orai(t, project, "", "", "lead", "--dry-run")
	if code != 0 || !strings.Contains(out, "local-model") {
		t.Fatalf("dry run does not use the local model: %d %s %s", code, out, errOut)
	}
	status, _, _ := orai(t, project, "", "", "status")
	for _, want := range []string{`"model": "local-model"`, `"role": "scratch"`, `"local": [`} {
		if !strings.Contains(status, want) {
			t.Fatalf("status lacks %s:\n%s", want, status)
		}
	}
	report, _, _ := orai(t, project, "", "", "doctor")
	if !strings.Contains(report, "orai.local.toml is applied over it and sets roles.lead.model, roles.scratch") {
		t.Fatalf("doctor does not name the local settings:\n%s", report)
	}
	if ignored, _ := os.ReadFile(filepath.Join(project, ".gitignore")); !strings.Contains(string(ignored), "/orai.local.toml") {
		t.Fatalf(".gitignore:\n%s", ignored)
	}

	if err := os.WriteFile(local, []byte("[roles.lead]\nnickname = \"boss\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := orai(t, project, "", "", "status"); code != 2 || !strings.Contains(errOut, "orai.local.toml, applied over orai.toml: roles.lead: unknown key(s) nickname") {
		t.Fatalf("broken local file: %d %s", code, errOut)
	}
	_ = os.Remove(local)
	if status, _, code := orai(t, project, "", "", "status"); code != 0 || strings.Contains(status, "scratch") || strings.Contains(status, "local-model") {
		t.Fatalf("without the local file: %d %s", code, status)
	}
}
