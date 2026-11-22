// Package openspec connects OpenSpec (Fission-AI/OpenSpec), the project's spec layer.
// OpenSpec owns the spec format, its CLI and its agent skills; Orai declares it in
// orai.toml, runs `openspec init` once at project scope during setup, keeps OpenSpec's
// telemetry off in role sessions unless the project turns it on, and diagnoses its state.
// Orai never edits OpenSpec's skills or its global files.
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
	// Skill is the one OpenSpec skill whose presence says `openspec init` ran here at
	// project scope (it writes the skills to .agents/skills and links them for Claude).
	Skill = ".agents/skills/openspec-propose/SKILL.md"
	// Tools are the OpenSpec tool ids for the providers Orai runs. Both are always set
	// up so every computer gets the same committed files whichever roles it runs.
	Tools = "claude,codex"
	// MinMajor is the first OpenSpec release that delivers skills (1.x wrote commands
	// and an AGENTS.md block instead).
	MinMajor = 2
)

// InitArgs is `openspec init` at project scope: OpenSpec's default scope is global
// (~/.agents/skills), which Orai never writes.
var InitArgs = []string{"init", "--tools", Tools, "--scope", "project", "--no-animation", "--no-copilot-cloud"}

// lookPath resolves a binary on PATH; tests swap it.
var lookPath = exec.LookPath

// run runs argv in dir with env added to this process's environment and returns stdout.
// A non-zero exit becomes an error built from stderr, falling back to stdout.
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
		return nil, fmt.Errorf("%s", msg)
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
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(Skill)))
	return err == nil
}

// Diagnose is the read-only OpenSpec diagnosis: not-configured → binary on PATH →
// version → initialized at project scope → (deep) the main specs validate.
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
			fmt.Sprintf("OpenSpec %s predates skills (needs %d.0 or later)", ver, MinMajor), doctor.InstallOpenSpec)
		return []doctor.Check{c.WithDetail(detail)}
	}
	if !initialized(p.Root) {
		c := doctor.New(Component, doctor.NotReady,
			"OpenSpec is not set up in this project yet (no "+Skill+")", "`orai setup`")
		return []doctor.Check{c.WithDetail(detail)}
	}
	checks := []doctor.Check{doctor.New(Component, doctor.Healthy,
		fmt.Sprintf("OpenSpec %s, skills in .agents/skills", ver), "").WithDetail(detail)}

	if !deep {
		return append(checks, doctor.New(Component+".specs", doctor.NotChecked, "spec validation runs with --deep", ""))
	}
	out, err := run([]string{binary, "validate", "--specs", "--json", "--no-interactive"}, p.Root, env, 120*time.Second)
	if err != nil {
		return append(checks, doctor.New(Component+".specs", doctor.Blocked, fmt.Sprintf("openspec validate failed: %v", err),
			"Run `openspec validate --specs` in the project folder to see the error"))
	}
	var report struct {
		Items []struct {
			ID    string `json:"id"`
			Valid bool   `json:"valid"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
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
	return append(checks, doctor.New(Component+".specs", doctor.Healthy, fmt.Sprintf("%d spec(s) valid", len(report.Items)), ""))
}

// SetupStep is `orai setup`'s OpenSpec step: `openspec init` at project scope once,
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
		return fmt.Sprintf("openspec: skipped (OpenSpec %s predates skills; upgrade with `npm install -g @fission-ai/openspec@latest`, then `orai setup`)", ver)
	}
	if _, err := run(append([]string{binary}, InitArgs...), p.Root, env, 120*time.Second); err != nil {
		return "openspec: init failed: " + err.Error()
	}
	return "openspec: set up (skills in .agents/skills, specs in " + config.OpenSpecSpecs + ")"
}
