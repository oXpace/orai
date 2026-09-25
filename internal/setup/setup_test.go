// Covers setup.Run's top-level orchestration (plan/apply and reporting) using injected
// step functions, so it does not need internal/cli or a real wiki/codegraph tool. The
// wiki-step/codegraph-step tests themselves belong with the packages that own those
// real steps (internal/wiki and internal/codegraph). Tool steps and diagnosis are
// exercised with fakes.
package setup_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/scaffold"
	"github.com/oXpace/orai/internal/setup"
)

// resolvePath mirrors the symlink+absolute resolution scaffold.Plan applies internally
// (see the identical helper's doc comment in internal/scaffold/scaffold_test.go).
func resolvePath(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// noDiagnose is a diagnose func that reports nothing (no checks, no problems), for
// tests that only care about the file-plan/tool-step behavior.
func noDiagnose(*project.Project) []doctor.Check { return nil }

// noSteps replaces wiki/codegraph in tests that don't exercise a real tool.
var noSteps []func(*project.Project) string

func TestEmptyFolderBecomesATrunkRepositoryWithNoTools(t *testing.T) {
	root := resolvePath(t, t.TempDir())
	var out bytes.Buffer
	rc, _ := setup.Run(root, "minimal", false, scaffold.DefaultBranch, false, &out, noSteps, noDiagnose)
	if rc != 0 {
		t.Fatalf("Run() = %d, want 0; output:\n%s", rc, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "Applied: Initialize a Git repository on branch "+scaffold.DefaultBranch) {
		t.Fatalf("output missing git-init line:\n%s", text)
	}
	if got := strings.TrimSpace(git(t, root, "symbolic-ref", "--short", "HEAD")); got != scaffold.DefaultBranch {
		t.Fatalf("HEAD branch = %q, want %q", got, scaffold.DefaultBranch)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "README.md")); err != nil {
		t.Fatalf("docs/README.md not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "orai.toml")); err != nil {
		t.Fatalf("orai.toml not created: %v", err)
	}
	ignored, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ignored), "/.orai/") || !strings.Contains(string(ignored), "/.codegraph/") {
		t.Fatalf(".gitignore missing expected entries: %s", ignored)
	}
	if !strings.Contains(text, "does not pin Orai") || !strings.Contains(text, scaffold.InstallSpec) {
		t.Fatalf("output missing the Orai-pin note:\n%s", text)
	}
	if !strings.Contains(text, "wiki, codegraph: skipped (--no-tools)") {
		t.Fatalf("output missing the --no-tools line:\n%s", text)
	}
	if !strings.Contains(text, "Diagnosis:") {
		t.Fatalf("output missing Diagnosis: line:\n%s", text)
	}
}

func tree(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func sameTree(t *testing.T, a, b []string) bool {
	t.Helper()
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSetupIsIdempotentAndDryRunWritesNothing(t *testing.T) {
	root := resolvePath(t, t.TempDir())
	before := tree(t, root)

	var out1 bytes.Buffer
	if rc, err := setup.Run(root, "minimal", true, scaffold.DefaultBranch, false, &out1, noSteps, noDiagnose); rc != 0 || err != nil {
		t.Fatalf("dry-run Run() = %d, want 0; output:\n%s", rc, out1.String())
	}
	if after := tree(t, root); !sameTree(t, before, after) {
		t.Fatalf("dry-run wrote files: before %v, after %v", before, after)
	}
	if !strings.Contains(out1.String(), "Would: Initialize a Git repository") {
		t.Fatalf("dry-run output missing Would: line:\n%s", out1.String())
	}

	var out2 bytes.Buffer
	if rc, err := setup.Run(root, "minimal", false, scaffold.DefaultBranch, false, &out2, noSteps, noDiagnose); rc != 0 || err != nil {
		t.Fatalf("apply Run() = %d, want 0; output:\n%s", rc, out2.String())
	}

	var out3 bytes.Buffer
	if rc, err := setup.Run(root, "minimal", true, scaffold.DefaultBranch, false, &out3, noSteps, noDiagnose); rc != 0 || err != nil {
		t.Fatalf("second dry-run Run() = %d, want 0; output:\n%s", rc, out3.String())
	}
	if !strings.Contains(out3.String(), "Already current.") {
		t.Fatalf("second dry-run output missing 'Already current.':\n%s", out3.String())
	}
}

// TestCliInitTwicePrintsAlreadyCurrent ports test_cli_init_twice_prints_already_current
// (originally a cli.main() test) against setup.Run directly, since internal/cli is
// owned elsewhere and setup.Run carries the same "Files already current." contract.
func TestCliInitTwicePrintsAlreadyCurrent(t *testing.T) {
	root := resolvePath(t, t.TempDir())

	var out1 bytes.Buffer
	rc1, _ := setup.Run(root, "minimal", false, scaffold.DefaultBranch, false, &out1, noSteps, noDiagnose)
	if rc1 != 0 {
		t.Fatalf("first Run() = %d, want 0", rc1)
	}
	if strings.Contains(out1.String(), "Files already current.") {
		t.Fatalf("first run should not report 'Files already current.':\n%s", out1.String())
	}

	var out2 bytes.Buffer
	rc2, _ := setup.Run(root, "minimal", false, scaffold.DefaultBranch, false, &out2, noSteps, noDiagnose)
	if rc2 != 0 {
		t.Fatalf("second Run() = %d, want 0", rc2)
	}
	if !strings.Contains(out2.String(), "Files already current.") {
		t.Fatalf("second run should report 'Files already current.':\n%s", out2.String())
	}
}

// TestAFailedToolStepMakesSetupExitNonzero ports test_a_failed_tool_step_makes_setup_exit_nonzero
// using fake steps, exactly matching what setup.Run's injected-steps design is for.
func TestAFailedToolStepMakesSetupExitNonzero(t *testing.T) {
	root := resolvePath(t, t.TempDir())
	steps := []func(*project.Project) string{
		func(*project.Project) string { return "wiki: init failed: boom" },
		func(*project.Project) string { return "codegraph: indexed" },
	}
	var out bytes.Buffer
	rc, _ := setup.Run(root, "minimal", false, scaffold.DefaultBranch, true, &out, steps, noDiagnose)
	if rc != 1 {
		t.Fatalf("Run() = %d, want 1; output:\n%s", rc, out.String())
	}
	if !strings.Contains(out.String(), "wiki: init failed: boom") {
		t.Fatalf("output missing the failed step line:\n%s", out.String())
	}
}

// TestToolsDisabledSkipsStepsEntirely confirms steps are never called when tools=false,
// and that the exact skip line is printed (regardless of whether a step would fail).
func TestToolsDisabledSkipsStepsEntirely(t *testing.T) {
	root := resolvePath(t, t.TempDir())
	called := false
	steps := []func(*project.Project) string{
		func(*project.Project) string { called = true; return "wiki: init failed: boom" },
	}
	var out bytes.Buffer
	rc, _ := setup.Run(root, "minimal", false, scaffold.DefaultBranch, false, &out, steps, noDiagnose)
	if rc != 0 {
		t.Fatalf("Run() = %d, want 0 (a step that would fail must not run with --no-tools)", rc)
	}
	if called {
		t.Fatal("step was called despite tools=false")
	}
	if !strings.Contains(out.String(), "wiki, codegraph: skipped (--no-tools)") {
		t.Fatalf("output missing the skip line:\n%s", out.String())
	}
}

// TestDiagnosisReportsOnlyProblems confirms the problem-line filter: healthy,
// not-configured and not-checked checks are silent; anything else prints with the
// right-aligned status, component, reason and an optional next-action hint.
func TestDiagnosisReportsOnlyProblems(t *testing.T) {
	root := resolvePath(t, t.TempDir())
	checks := []doctor.Check{
		doctor.New("ok", doctor.Healthy, "fine", ""),
		doctor.New("skip", doctor.NotConfigured, "not set up", ""),
		doctor.New("unchecked", doctor.NotChecked, "not run this time", ""),
		doctor.New("broken", doctor.Blocked, "missing binary", "install it"),
	}
	diagnose := func(*project.Project) []doctor.Check { return checks }
	var out bytes.Buffer
	rc, _ := setup.Run(root, "minimal", false, scaffold.DefaultBranch, false, &out, noSteps, diagnose)
	if rc != 0 {
		t.Fatalf("Run() = %d, want 0 (a diagnosis problem alone must not fail setup)", rc)
	}
	text := out.String()
	for _, unwanted := range []string{"ok:", "skip:", "unchecked:"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("output unexpectedly reports a non-problem check (%q):\n%s", unwanted, text)
		}
	}
	if !strings.Contains(text, "broken: missing binary") || !strings.Contains(text, "install it") {
		t.Fatalf("output missing the problem line for 'broken':\n%s", text)
	}
	status, _ := doctor.Overall(checks)
	if !strings.Contains(text, "Diagnosis: "+status) {
		t.Fatalf("output missing 'Diagnosis: %s':\n%s", status, text)
	}
}
