// Package doctor is the read-only, per-component diagnosis model: explicit states and
// safe next actions, reported as JSON.
package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	Healthy       = "healthy"
	Degraded      = "degraded"
	NotReady      = "not-ready"
	Blocked       = "blocked"
	NotConfigured = "not-configured"
	Unsupported   = "unsupported"
	// NotChecked: deliberately not exercised in this mode (e.g. model-loading search
	// without --deep). Visible, never counted as verified, and not a problem either.
	NotChecked = "not-checked"
)

const (
	ExitOK       = 0
	ExitDegraded = 1
	ExitBlocked  = 3
)

type Check struct {
	Component  string         `json:"component"`
	Status     string         `json:"status"`
	Reason     string         `json:"reason"`
	NextAction string         `json:"next_action,omitempty"`
	Core       bool           `json:"core"`
	Detail     map[string]any `json:"detail,omitempty"`
	CheckedAt  string         `json:"checked_at"`
}

func Now() string { return time.Now().UTC().Format("2006-01-02T15:04:05+00:00") }

// New builds a check stamped with the current time.
func New(component, status, reason, next string) Check {
	return Check{Component: component, Status: status, Reason: reason, NextAction: next, CheckedAt: Now()}
}

func (c Check) WithCore() Check                        { c.Core = true; return c }
func (c Check) WithDetail(detail map[string]any) Check { c.Detail = detail; return c }

// Overall folds component states into one status and exit code.
func Overall(checks []Check) (string, int) {
	for _, c := range checks {
		if c.Core && (c.Status == Blocked || c.Status == Unsupported) {
			return Blocked, ExitBlocked
		}
	}
	for _, c := range checks {
		if c.Core && c.Status == NotConfigured {
			return NotConfigured, ExitDegraded // nothing project-specific was verified
		}
	}
	for _, c := range checks {
		switch c.Status {
		case Blocked, Degraded, NotReady, Unsupported:
			return Degraded, ExitDegraded
		}
	}
	return Healthy, ExitOK
}

// Report writes the JSON report and returns the exit code.
func Report(w io.Writer, checks []Check, context map[string]any) int {
	status, code := Overall(checks)
	out := map[string]any{"status": status, "checked_at": Now(), "checks": checks}
	for key, value := range context {
		out[key] = value
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(w, string(data))
	return code
}

// Output runs argv and returns combined output; err is set when it cannot run.
func Output(argv ...string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	done := make(chan struct{})
	var out []byte
	var err error
	go func() { out, err = cmd.CombinedOutput(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		return string(out), fmt.Errorf("timed out")
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		err = nil // help text on a non-zero exit is still help text
	}
	return string(out), err
}

var semver = regexp.MustCompile(`\d+\.\d+\.\d+`)

func VersionOf(binary string) string {
	out, _ := Output(binary, "--version")
	return semver.FindString(out)
}

// Requirement is a help invocation and the flags Orai relies on from it.
type Requirement struct {
	Argv    []string
	Needles []string
}

// Tool checks presence, version and required flags from the installed binary's help.
func Tool(component, binary string, core bool, reqs []Requirement, installHint, note string) Check {
	path, err := exec.LookPath(binary)
	if err != nil {
		c := New(component, Blocked, binary+" is not on PATH", installHint)
		c.Core = core
		return c
	}
	detail := map[string]any{"path": path, "version": VersionOf(binary)}
	var missing []string
	for _, req := range reqs {
		text, err := Output(req.Argv...)
		if err != nil {
			c := New(component, Blocked, fmt.Sprintf("`%s` failed: %v", strings.Join(req.Argv, " "), err), "")
			c.Core, c.Detail = core, detail
			return c
		}
		label := strings.Join(req.Argv[1:len(req.Argv)-1], " ")
		if label == "" {
			label = binary
		}
		for _, needle := range req.Needles {
			if !strings.Contains(text, needle) {
				missing = append(missing, label+" "+needle)
			}
		}
	}
	if len(missing) > 0 {
		c := New(component, Unsupported, "installed version lacks: "+strings.Join(missing, ", "),
			"Install a version listed in docs/compatibility.md")
		c.Core, c.Detail = core, detail
		return c
	}
	if note != "" {
		detail["note"] = note
	}
	c := New(component, Healthy, "required capabilities present", "")
	c.Core, c.Detail = core, detail
	return c
}

// Account diagnoses login separately from installation: a working binary may still be
// signed out.
func Account(component string, argv []string, signedIn func(string) bool, hint string) Check {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return New(component, NotReady, argv[0]+" is not installed; login not checked", "").WithCore()
	}
	text, err := Output(argv...)
	if err != nil {
		return New(component, Blocked, fmt.Sprintf("`%s` failed: %v", strings.Join(argv, " "), err), hint).WithCore()
	}
	if signedIn(text) {
		return New(component, Healthy, "signed in", "").WithCore()
	}
	return New(component, Blocked, "not signed in", hint).WithCore()
}

func claudeSignedIn(text string) bool {
	var value map[string]any
	if json.Unmarshal([]byte(text), &value) != nil {
		return false
	}
	return value["loggedIn"] == true
}

// ToolChecks covers the provider CLIs the project's roles use.
func ToolChecks(providers map[string]bool) []Check {
	var checks []Check
	if providers["codex"] {
		checks = append(checks, Tool("tool.codex", "codex", true, []Requirement{
			{[]string{"codex", "queue", "--help"}, []string{"--thread", "--message"}},
			{[]string{"codex", "resume", "--help"}, []string{"SESSION_ID"}},
			{[]string{"codex", "--help"}, []string{"--add-dir", "--config"}},
		}, "Install Codex CLI (docs/compatibility.md)", "SessionStart hook trust is confirmed by Codex at launch (/hooks)."))
		checks = append(checks, Account("account.codex", []string{"codex", "login", "status"},
			func(t string) bool { return strings.Contains(t, "Logged in") }, "codex login"))
	}
	if providers["claude"] {
		checks = append(checks, Tool("tool.claude", "claude", true, []Requirement{
			{[]string{"claude", "--help"}, []string{"--session-id", "--resume", "--settings", "--mcp-config", "--name", "--effort", "--add-dir"}},
		}, "Install Claude Code (docs/compatibility.md)", "Development channel consent is hidden from --help; Claude asks at launch."))
		checks = append(checks, Account("account.claude", []string{"claude", "auth", "status"}, claudeSignedIn, "claude auth login"))
	}
	return checks
}
