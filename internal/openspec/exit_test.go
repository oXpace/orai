package openspec_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/openspec"
)

// OpenSpec 2.1.1 prints the report and exits 1 when a spec is invalid.
func TestDeepValidationReadsTheReportOfAFailedRun(t *testing.T) {
	p := writeProject(t, declared)
	fakeOpenSpec(t, "2.1.1", `{"items": [{"id": "auth", "valid": false}]}`)
	openspec.SetupStep(p)
	bin, err := exec.LookPath("openspec")
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	failing := strings.Replace(string(script), "JSON\n;;", "JSON\nexit 1 ;;", 1)
	if err := os.WriteFile(bin, []byte(failing), 0o755); err != nil {
		t.Fatal(err)
	}
	checks := openspec.Diagnose(p, true)
	if checks[1].Status != doctor.Degraded || !strings.Contains(checks[1].Reason, "auth") {
		t.Fatalf("%+v", checks)
	}
}
