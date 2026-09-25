// Covers codegraph.Diagnose and SetupStep's index-once behavior. `codegraph` is always
// a real (fake) executable resolved through the real PATH, since this external test
// package cannot swap codegraph's unexported lookPath/runJSON vars directly. No test
// touches a real CodeGraph index.
package codegraph_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/codegraph"
	"github.com/oXpace/orai/internal/doctor"
)

func TestCodegraphNotConfigured(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 1\nsession = \"orai\"\n")
	checks := codegraph.Diagnose(p, false)
	if len(checks) != 1 {
		t.Fatalf("checks = %+v, want exactly one", checks)
	}
	if checks[0].Component != "codegraph" || checks[0].Status != doctor.NotConfigured {
		t.Fatalf("checks[0] = %+v, want codegraph not-configured", checks[0])
	}
}

func TestCodegraphBinaryMissingIsBlocked(t *testing.T) {
	tmp := t.TempDir()
	emptyBin := filepath.Join(tmp, "empty-bin")
	if err := os.MkdirAll(emptyBin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyBin)
	p := writeProject(t, filepath.Join(tmp, "proj"), codegraphTOML)

	checks := codegraph.Diagnose(p, false)
	if checks[0].Status != doctor.Blocked {
		t.Fatalf("status = %q, want blocked", checks[0].Status)
	}
	if !strings.Contains(checks[0].Reason, "not on PATH") {
		t.Fatalf("reason = %q, want mention of PATH", checks[0].Reason)
	}
}

func TestCodegraphNotInitialized(t *testing.T) {
	statusData, err := json.Marshal(map[string]any{"initialized": false})
	if err != nil {
		t.Fatal(err)
	}
	p := codegraphProject(t, string(statusData), "[]")

	checks := codegraph.Diagnose(p, false)
	if checks[0].Status != doctor.NotReady {
		t.Fatalf("status = %q, want not-ready", checks[0].Status)
	}
	want := "codegraph init " + p.Root
	if checks[0].NextAction != want {
		t.Fatalf("next action = %q, want %q", checks[0].NextAction, want)
	}
}

func TestCodegraphCompleteIndexIsHealthy(t *testing.T) {
	statusData, err := json.Marshal(completeStatus())
	if err != nil {
		t.Fatal(err)
	}
	p := codegraphProject(t, string(statusData), "[]")

	checks := codegraph.Diagnose(p, false)
	if len(checks) != 1 {
		t.Fatalf("checks = %+v, want exactly one", checks)
	}
	if checks[0].Component != "codegraph.index" || checks[0].Status != doctor.Healthy {
		t.Fatalf("checks[0] = %+v, want codegraph.index healthy", checks[0])
	}
}

func TestCodegraphPendingChangesIsDegraded(t *testing.T) {
	status := completeStatus()
	status["pendingChanges"] = map[string]any{"added": 1, "modified": 0, "deleted": 0}
	statusData, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	p := codegraphProject(t, string(statusData), "[]")

	checks := codegraph.Diagnose(p, false)
	if checks[0].Component != "codegraph.index" || checks[0].Status != doctor.Degraded {
		t.Fatalf("checks[0] = %+v, want codegraph.index degraded", checks[0])
	}
	if !strings.Contains(checks[0].Reason, "unsynced changes") {
		t.Fatalf("reason = %q, want mention of unsynced changes", checks[0].Reason)
	}
}

func TestCodegraphDeepSmokeSymbolFoundIsHealthy(t *testing.T) {
	statusData, err := json.Marshal(completeStatus())
	if err != nil {
		t.Fatal(err)
	}
	queryData, err := json.Marshal([]map[string]any{
		{"node": map[string]any{"name": "kickoff", "filePath": "internal/runtime/runtime.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p := codegraphProject(t, string(statusData), string(queryData))

	checks := codegraph.Diagnose(p, true)
	q := findCheck(t, checks, "codegraph.query")
	if q.Status != doctor.Healthy {
		t.Fatalf("status = %q, want healthy", q.Status)
	}
	if !strings.Contains(q.Reason, "kickoff found in internal/runtime/runtime.go") {
		t.Fatalf("reason = %q, want mention of the file it was found in", q.Reason)
	}
}

func TestCodegraphDeepSmokeSymbolNotFoundIsDegraded(t *testing.T) {
	statusData, err := json.Marshal(completeStatus())
	if err != nil {
		t.Fatal(err)
	}
	p := codegraphProject(t, string(statusData), "[]")

	checks := codegraph.Diagnose(p, true)
	q := findCheck(t, checks, "codegraph.query")
	if q.Status != doctor.Degraded {
		t.Fatalf("status = %q, want degraded", q.Status)
	}
	if !strings.Contains(q.Reason, "not returned") {
		t.Fatalf("reason = %q, want mention of not returned", q.Reason)
	}
}

func TestCodegraphStatusCommandFailureIsBlocked(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBinary(t, binDir, "codegraph", "echo boom >&2\nexit 1\n")
	t.Setenv("PATH", pathWith(binDir))
	p := writeProject(t, filepath.Join(tmp, "proj"), codegraphTOML)

	checks := codegraph.Diagnose(p, false)
	if len(checks) != 1 || checks[0].Status != doctor.Blocked {
		t.Fatalf("checks = %+v, want a single blocked check", checks)
	}
	if !strings.Contains(checks[0].Reason, "boom") {
		t.Fatalf("reason = %q, want mention of the fake binary's stderr", checks[0].Reason)
	}
}

func findCheck(t *testing.T, checks []doctor.Check, component string) doctor.Check {
	t.Helper()
	for _, c := range checks {
		if c.Component == component {
			return c
		}
	}
	t.Fatalf("no %s check among %+v", component, checks)
	return doctor.Check{}
}

// ---- SetupStep -----------------------------------------------------------------------

func TestCodegraphStepNotConfigured(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 1\nsession = \"orai\"\n")
	if got := codegraph.SetupStep(p); got != "codegraph: not configured" {
		t.Fatalf("SetupStep() = %q, want not configured", got)
	}
}

func TestCodegraphStepSkippedWhenNotInstalled(t *testing.T) {
	tmp := t.TempDir()
	empty := filepath.Join(tmp, "empty-bin")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", empty)
	p := writeProject(t, filepath.Join(tmp, "proj"), "schema = 1\nsession = \"orai\"\n\n[integrations.codegraph]\n")
	if got := codegraph.SetupStep(p); !strings.Contains(got, "not installed") {
		t.Fatalf("SetupStep() = %q, want mention of not installed", got)
	}
}

func TestCodegraphStepIndexesOnce(t *testing.T) {
	tmp := t.TempDir()
	p := writeProject(t, filepath.Join(tmp, "p"), "schema = 1\nsession = \"orai\"\n\n[integrations.codegraph]\n")
	fakes := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(fakes, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBinary(t, fakes, "codegraph", `
if [ "$1" != "init" ]; then
  echo "unexpected args: $@" >&2
  exit 1
fi
mkdir -p "$2/.codegraph"
`)
	t.Setenv("PATH", pathWith(fakes))

	if got := codegraph.SetupStep(p); got != "codegraph: indexed" {
		t.Fatalf("SetupStep() = %q, want indexed", got)
	}
	if got := codegraph.SetupStep(p); got != "codegraph: already indexed" {
		t.Fatalf("SetupStep() = %q, want already indexed (no second `codegraph init`)", got)
	}
}

func TestCodegraphStepInitFailureReportsStderrTail(t *testing.T) {
	tmp := t.TempDir()
	p := writeProject(t, filepath.Join(tmp, "p"), "schema = 1\nsession = \"orai\"\n\n[integrations.codegraph]\n")
	fakes := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(fakes, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBinary(t, fakes, "codegraph", "echo 'disk full' >&2\nexit 1\n")
	t.Setenv("PATH", pathWith(fakes))

	got := codegraph.SetupStep(p)
	if !strings.HasPrefix(got, "codegraph: init failed: ") || !strings.Contains(got, "disk full") {
		t.Fatalf("SetupStep() = %q, want a failure mentioning disk full", got)
	}
}
