package openspec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/openspec"
	"github.com/oXpace/orai/internal/project"
)

const declared = "schema = 2\nsession = \"orai\"\n\n[integrations.openspec]\n"

func writeProject(t *testing.T, text string) *project.Project {
	t.Helper()
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	if err := os.WriteFile(filepath.Join(root, project.ConfigName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeOpenSpec puts a stand-in `openspec` first on PATH. It prints version for
// --version, records its arguments and OPENSPEC_TELEMETRY to calls.log, writes the
// propose skill on init, and prints report on validate.
func fakeOpenSpec(t *testing.T, version, report string) string {
	t.Helper()
	bin := t.TempDir()
	script := `#!/bin/sh
echo "$* telemetry=${OPENSPEC_TELEMETRY:-unset}" >> "` + bin + `/calls.log"
case "$1" in
--version) echo "` + version + `" ;;
init) mkdir -p .agents/skills/openspec-propose openspec/specs && echo skill > .agents/skills/openspec-propose/SKILL.md ;;
validate) cat <<'JSON'
` + report + `
JSON
;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "openspec"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("OPENSPEC_TELEMETRY", "")
	return filepath.Join(bin, "calls.log")
}

func calls(t *testing.T, log string) string {
	t.Helper()
	data, _ := os.ReadFile(log)
	return string(data)
}

func TestNotDeclaredIsNotConfigured(t *testing.T) {
	p := writeProject(t, "schema = 2\nsession = \"orai\"\n")
	if checks := openspec.Diagnose(p, true); len(checks) != 1 || checks[0].Status != doctor.NotConfigured {
		t.Fatalf("%+v", checks)
	}
	if line := openspec.SetupStep(p); line != "openspec: not configured" {
		t.Fatal(line)
	}
}

func TestMissingBinaryBlocksDoctorAndSetupOnlySkips(t *testing.T) {
	p := writeProject(t, declared)
	t.Setenv("PATH", t.TempDir())
	checks := openspec.Diagnose(p, false)
	if len(checks) != 1 || checks[0].Status != doctor.Blocked || checks[0].NextAction != doctor.InstallOpenSpec {
		t.Fatalf("%+v", checks)
	}
	if line := openspec.SetupStep(p); !strings.HasPrefix(line, "openspec: skipped") || strings.Contains(line, "failed") {
		t.Fatal(line)
	}
}

// Setup runs init once, at project scope, with telemetry off; the next setup and the
// diagnosis see it as set up.
func TestSetupInitializesAtProjectScopeOnce(t *testing.T) {
	p := writeProject(t, declared)
	log := fakeOpenSpec(t, "2.1.1", `{"items": []}`)

	if line := openspec.SetupStep(p); !strings.HasPrefix(line, "openspec: set up") {
		t.Fatal(line)
	}
	got := calls(t, log)
	if !strings.Contains(got, "init --tools claude,codex --scope project") || !strings.Contains(got, "telemetry=0") {
		t.Fatalf("init was not run at project scope with telemetry off:\n%s", got)
	}
	if line := openspec.SetupStep(p); line != "openspec: already set up" {
		t.Fatal(line)
	}
	if n := strings.Count(calls(t, log), "init "); n != 1 {
		t.Fatalf("init ran %d times", n)
	}
	checks := openspec.Diagnose(p, false)
	if len(checks) != 2 || checks[0].Status != doctor.Healthy || checks[1].Status != doctor.NotChecked {
		t.Fatalf("%+v", checks)
	}
}

func TestTelemetryStaysWhenTheProjectTurnsItOn(t *testing.T) {
	p := writeProject(t, declared+"telemetry = true\n")
	log := fakeOpenSpec(t, "2.1.1", `{"items": []}`)
	openspec.SetupStep(p)
	if !strings.Contains(calls(t, log), "telemetry=unset") {
		t.Fatalf("telemetry was changed:\n%s", calls(t, log))
	}
}

func TestNotInitializedIsNotReady(t *testing.T) {
	p := writeProject(t, declared)
	fakeOpenSpec(t, "2.1.1", `{"items": []}`)
	checks := openspec.Diagnose(p, false)
	if len(checks) != 1 || checks[0].Status != doctor.NotReady || checks[0].NextAction != "`orai setup`" {
		t.Fatalf("%+v", checks)
	}
}

// OpenSpec 1.x wrote commands and an AGENTS.md block; setup must not run it.
func TestOldOpenSpecIsUnsupportedAndNotRun(t *testing.T) {
	p := writeProject(t, declared)
	log := fakeOpenSpec(t, "1.13.2", `{"items": []}`)
	if checks := openspec.Diagnose(p, false); checks[0].Status != doctor.Unsupported {
		t.Fatalf("%+v", checks)
	}
	if line := openspec.SetupStep(p); !strings.HasPrefix(line, "openspec: skipped") {
		t.Fatal(line)
	}
	if strings.Contains(calls(t, log), "init") {
		t.Fatal("init ran on OpenSpec 1.x")
	}
}

func TestDeepValidationNamesFailingSpecs(t *testing.T) {
	p := writeProject(t, declared)
	fakeOpenSpec(t, "2.1.1", `{"items": [{"id": "auth", "valid": false}, {"id": "billing", "valid": true}]}`)
	openspec.SetupStep(p)
	checks := openspec.Diagnose(p, true)
	if len(checks) != 2 || checks[1].Component != "openspec.specs" || checks[1].Status != doctor.Degraded ||
		!strings.Contains(checks[1].Reason, "1 of 2") || !strings.Contains(checks[1].Reason, "auth") {
		t.Fatalf("%+v", checks)
	}
}

func TestDeepValidationOfValidSpecsIsHealthy(t *testing.T) {
	p := writeProject(t, declared)
	fakeOpenSpec(t, "2.1.1", `{"items": [{"id": "auth", "valid": true}]}`)
	openspec.SetupStep(p)
	if checks := openspec.Diagnose(p, true); checks[1].Status != doctor.Healthy {
		t.Fatalf("%+v", checks)
	}
}

func TestUnreadableValidationOutputIsNotHealthy(t *testing.T) {
	p := writeProject(t, declared)
	fakeOpenSpec(t, "2.1.1", "not json")
	openspec.SetupStep(p)
	if checks := openspec.Diagnose(p, true); checks[1].Status != doctor.Blocked {
		t.Fatalf("%+v", checks)
	}
}
