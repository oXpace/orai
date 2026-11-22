// Package openspec connects OpenSpec (Fission-AI/OpenSpec), the project's spec layer.
// OpenSpec owns the spec format and its CLI; the Orai skill carries the workflow. Orai
// declares it in orai.toml, runs `openspec init` without skills once during setup, keeps OpenSpec's
// telemetry off in role sessions unless the project turns it on, and diagnoses its state.
// Orai never edits installed OpenSpec skills or OpenSpec's global files.
package openspec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

// Component is the top-level check name; the spec validation is Component+".specs".
const Component = "openspec"

const (
	// Changes is the folder whose presence says `openspec init` ran here: setup itself
	// creates openspec/config.yaml and openspec/specs before init, but not this one.
	Changes = config.OpenSpecDir + "/changes"
	// MinMajor is the first OpenSpec release whose CLI the skill's workflow was written
	// for (1.x wrote commands and an AGENTS.md block on init).
	MinMajor = 2
)

// InitArgs is `openspec init` without skills: the Orai skill covers every step, and no
// tool means nothing is written to project or global skill folders.
var InitArgs = []string{"init", "--tools", "none", "--no-animation", "--no-copilot-cloud"}

// lookPath resolves a binary on PATH; tests swap it.
var lookPath = exec.LookPath

// run runs argv in dir with env added to this process's environment and returns stdout.
// A non-zero exit becomes an error built from stderr, falling back to stdout, and is
// returned with the stdout it printed.
var run = func(argv []string, dir string, env []string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("timed out")
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 300 {
			msg = msg[len(msg)-300:]
		}
		return stdout.Bytes(), fmt.Errorf("%s", msg)
	}
	return stdout.Bytes(), nil
}

// Env is what a process running OpenSpec for this project gets: telemetry off unless
// the project turned it on. Nil when OpenSpec is not declared or telemetry is on.
func Env(cfg *config.OpenSpec) []string {
	if cfg == nil || cfg.Telemetry {
		return nil
	}
	return []string{"OPENSPEC_TELEMETRY=0"}
}

var versionPattern = regexp.MustCompile(`(\d+)\.\d+\.\d+`)

// version runs `openspec --version` and returns the version and its major number.
func version(binary, root string, env []string) (string, int, error) {
	out, err := run([]string{binary, "--version"}, root, env, 30*time.Second)
	if err != nil {
		return "", 0, err
	}
	m := versionPattern.FindStringSubmatch(string(out))
	if m == nil {
		return strings.TrimSpace(string(out)), 0, fmt.Errorf("unrecognized version output %q", strings.TrimSpace(string(out)))
	}
	major, _ := strconv.Atoi(m[1])
	return m[0], major, nil
}

func initialized(root string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(Changes)))
	return err == nil && info.IsDir()
}

// Diagnose is the read-only OpenSpec diagnosis: not-configured → binary on PATH →
// version → initialized → (deep) the main specs validate.
func Diagnose(p *project.Project, deep bool) []doctor.Check {
	cfg := p.Config.OpenSpec
	if cfg == nil {
		return []doctor.Check{doctor.New(Component, doctor.NotConfigured, "no [integrations.openspec] in orai.toml", "")}
	}
	binary, err := lookPath("openspec")
	if err != nil {
		return []doctor.Check{doctor.New(Component, doctor.Blocked, "openspec is not on PATH", doctor.InstallOpenSpec)}
	}
	env := Env(cfg)
	ver, major, err := version(binary, p.Root, env)
	if err != nil {
		return []doctor.Check{doctor.New(Component, doctor.Blocked, fmt.Sprintf("openspec --version failed: %v", err),
			fmt.Sprintf(doctor.CannotRun, "openspec"))}
	}
	detail := map[string]any{"version": ver, "telemetry": cfg.Telemetry}
	if major < MinMajor {
		c := doctor.New(Component, doctor.Unsupported,
			fmt.Sprintf("OpenSpec %s is too old (needs %d.0 or later)", ver, MinMajor), doctor.InstallOpenSpec)
		return []doctor.Check{c.WithDetail(detail)}
	}
	if !initialized(p.Root) {
		c := doctor.New(Component, doctor.NotReady,
			"OpenSpec is not set up in this project yet (no "+Changes+")", "`orai setup`")
		return []doctor.Check{c.WithDetail(detail)}
	}
	checks := []doctor.Check{doctor.New(Component, doctor.Healthy,
		fmt.Sprintf("OpenSpec %s, workflow in the Orai skill", ver), "").WithDetail(detail)}

	if !deep {
		return append(checks, doctor.New(Component+".specs", doctor.NotChecked, "spec validation runs with --deep", ""))
	}
	out, runErr := run([]string{binary, "validate", "--specs", "--json", "--no-interactive"}, p.Root, env, 120*time.Second)
	// validate exits 1 when a spec is invalid and still prints the report: read that
	// first. A failed run that names no invalid spec is never healthy.
	failedRun := func() []doctor.Check {
		return append(checks, doctor.New(Component+".specs", doctor.Blocked, fmt.Sprintf("openspec validate failed: %v", runErr),
			"Run `openspec validate --specs` in the project folder to see the error"))
	}
	var report struct {
		Items []struct {
			ID    string `json:"id"`
			Valid bool   `json:"valid"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		if runErr != nil {
			return failedRun()
		}
		return append(checks, doctor.New(Component+".specs", doctor.Blocked,
			fmt.Sprintf("openspec validate printed something other than its JSON report: %v", err),
			"Run `openspec validate --specs --json` in the project folder to see the output"))
	}
	var failed []string
	for _, item := range report.Items {
		if !item.Valid {
			failed = append(failed, item.ID)
		}
	}
	if len(failed) > 0 {
		return append(checks, doctor.New(Component+".specs", doctor.Degraded,
			fmt.Sprintf("%d of %d spec(s) fail validation: %s", len(failed), len(report.Items), strings.Join(failed, ", ")),
			"`openspec validate --specs` shows what each one lacks"))
	}
	if runErr != nil {
		return failedRun()
	}
	return append(checks, doctor.New(Component+".specs", doctor.Healthy, fmt.Sprintf("%d spec(s) valid", len(report.Items)), ""))
}

// SetupStep is `orai setup`'s OpenSpec step: `openspec init` without skills once,
// skipping (never failing the whole setup) when the tool is missing or too old.
// OpenSpec keeps an existing openspec/config.yaml, so the starter rules setup wrote
// before this step stay.
func SetupStep(p *project.Project) string {
	cfg := p.Config.OpenSpec
	if cfg == nil {
		return "openspec: not configured"
	}
	binary, err := lookPath("openspec")
	if err != nil {
		return "openspec: skipped (OpenSpec is not installed: `npm install -g @fission-ai/openspec@latest`, then `orai setup`)"
	}
	if initialized(p.Root) {
		return "openspec: already set up"
	}
	env := Env(cfg)
	ver, major, err := version(binary, p.Root, env)
	if err != nil {
		return "openspec: init failed: openspec --version: " + err.Error()
	}
	if major < MinMajor {
		return fmt.Sprintf("openspec: skipped (OpenSpec %s is too old; upgrade with `npm install -g @fission-ai/openspec@latest`, then `orai setup`)", ver)
	}
	if _, err := run(append([]string{binary}, InitArgs...), p.Root, env, 120*time.Second); err != nil {
		return "openspec: init failed: " + err.Error()
	}
	return "openspec: set up (specs in " + config.OpenSpecSpecs + ", workflow in the Orai skill)"
}
