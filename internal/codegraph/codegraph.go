// Package codegraph diagnoses CodeGraph (colbymchenry/codegraph). Installing,
// registering the MCP server and indexing are separate user steps; Orai only reports
// their state and never edits global config.
package codegraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

// Component is the top-level check name; sub-checks are Component+".index" etc.
const Component = "codegraph"

// lookPath resolves a binary on PATH; tests swap it to simulate a missing "codegraph"
// without touching the real PATH.
var lookPath = exec.LookPath

// runJSON runs argv in cwd with a 60s timeout and decodes its stdout as JSON (an
// object for `status`, an array for `query`). A non-zero exit becomes an error built
// from stderr, falling back to stdout, then the bare exit code.
var runJSON = func(argv []string, cwd string) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cwd
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
			if exitErr, ok := err.(*exec.ExitError); ok {
				msg = fmt.Sprintf("exit %d", exitErr.ExitCode())
			} else {
				msg = err.Error()
			}
		}
		return nil, fmt.Errorf("%s", msg)
	}
	var value any
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return true
	}
}

func anyTruthy(m map[string]any) bool {
	for _, v := range m {
		if truthy(v) {
			return true
		}
	}
	return false
}

// quoted formats a value for a diagnostic detail string: single-quoted strings,
// "None" for a missing key, and the value's default formatting otherwise.
func quoted(v any) string {
	if v == nil {
		return "None"
	}
	if s, ok := v.(string); ok {
		return "'" + s + "'"
	}
	return fmt.Sprintf("%v", v)
}

// plain formats a value for a diagnostic detail string: "True"/"False" for booleans,
// "None" for a missing key, and the value's default formatting otherwise.
func plain(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Diagnose is the read-only CodeGraph diagnosis: not-configured → binary on PATH →
// `codegraph status` → index completeness/sync → (deep) a configured smoke symbol.
func Diagnose(p *project.Project, deep bool) []doctor.Check {
	cfg := p.Config.Codegraph
	if cfg == nil {
		return []doctor.Check{doctor.New(Component, doctor.NotConfigured, "no [integrations.codegraph] in orai.toml", "")}
	}
	binary, err := lookPath("codegraph")
	if err != nil {
		return []doctor.Check{doctor.New(Component, doctor.Blocked, "codegraph is not on PATH",
			"Install colbymchenry/codegraph (npm @colbymchenry/codegraph); see docs/compatibility.md")}
	}
	root := p.Root
	raw, err := runJSON([]string{binary, "status", "--json", root}, root)
	if err != nil {
		return []doctor.Check{doctor.New(Component, doctor.Blocked, fmt.Sprintf("codegraph status failed: %v", err), "")}
	}
	status, _ := raw.(map[string]any)
	detail := map[string]any{
		"version":     status["version"],
		"lastIndexed": status["lastIndexed"],
		"fileCount":   status["fileCount"],
		"nodeCount":   status["nodeCount"],
	}
	if !truthy(status["initialized"]) {
		c := doctor.New(Component, doctor.NotReady, "project is not indexed", "codegraph init "+root)
		return []doctor.Check{c.WithDetail(detail)}
	}
	index, _ := status["index"].(map[string]any)
	pending, _ := status["pendingChanges"].(map[string]any)
	state, _ := index["state"].(string)
	if state != "complete" || truthy(index["reindexRecommended"]) {
		c := doctor.New(Component, doctor.Degraded,
			fmt.Sprintf("index state %s, reindex recommended=%s", quoted(index["state"]), plain(index["reindexRecommended"])),
			"codegraph index "+root)
		return []doctor.Check{c.WithDetail(detail)}
	}

	var checks []doctor.Check
	if anyTruthy(pending) {
		c := doctor.New(Component+".index", doctor.Degraded,
			fmt.Sprintf("unsynced changes: added %v, modified %v, removed %v",
				orZero(pending["added"]), orZero(pending["modified"]), orZero(pending["removed"])), "codegraph sync "+root)
		checks = append(checks, c.WithDetail(detail))
	} else {
		c := doctor.New(Component+".index", doctor.Healthy, "index complete and in sync", "")
		checks = append(checks, c.WithDetail(detail))
	}

	if deep && cfg.SmokeSymbol != "" {
		rawRows, err := runJSON([]string{binary, "query", cfg.SmokeSymbol, "--json", "--limit", "5", "--path", root}, root)
		if err != nil {
			checks = append(checks, doctor.New(Component+".query", doctor.Blocked, fmt.Sprintf("symbol query failed: %v", err), ""))
			return checks
		}
		rows, _ := rawRows.([]any)
		var found string
		for _, r := range rows {
			row, _ := r.(map[string]any)
			node, _ := row["node"].(map[string]any)
			if name, _ := node["name"].(string); name == cfg.SmokeSymbol {
				found, _ = node["filePath"].(string)
				break
			}
		}
		// An empty result is a lookup failure, never evidence that the symbol is absent.
		if found != "" {
			checks = append(checks, doctor.New(Component+".query", doctor.Healthy,
				fmt.Sprintf("%s found in %s", cfg.SmokeSymbol, found), ""))
		} else {
			checks = append(checks, doctor.New(Component+".query", doctor.Degraded,
				fmt.Sprintf("%s not returned; the index may be stale", cfg.SmokeSymbol), "codegraph sync "+root))
		}
	}
	return checks
}

// SetupStep is `orai setup`'s codegraph step: index the project once, skipping (never
// failing the whole setup) when the tool is not installed.
func SetupStep(p *project.Project) string {
	if p.Config.Codegraph == nil {
		return "codegraph: not configured"
	}
	binary, err := lookPath("codegraph")
	if err != nil {
		return "codegraph: skipped (not installed; see docs/compatibility.md)"
	}
	if info, statErr := os.Stat(filepath.Join(p.Root, ".codegraph")); statErr == nil && info.IsDir() {
		return "codegraph: already indexed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "init", p.Root)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if err == nil {
		return "codegraph: indexed"
	}
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
	return "codegraph: init failed: " + msg
}

func orZero(value any) any {
	if value == nil {
		return 0
	}
	return value
}
