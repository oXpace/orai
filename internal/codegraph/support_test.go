package codegraph_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/oXpace/orai/internal/project"
)

func writeProject(t *testing.T, root, text string) *project.Project {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, project.ConfigName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatalf("project.Load(%s): %v", root, err)
	}
	return p
}

// fakeBinary writes an executable POSIX shell script named name in directory and
// returns its path, so tests can stand up a fake CLI on PATH without depending on any
// other runtime.
func fakeBinary(t *testing.T, directory, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script binaries require a POSIX shell")
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func pathWith(directory string) string {
	return directory + string(os.PathListSeparator) + os.Getenv("PATH")
}

const codegraphTOML = "schema = 1\nsession = \"orai\"\n\n[integrations.codegraph]\nsmoke_symbol = \"kickoff\"\n"

func completeStatus() map[string]any {
	return map[string]any{
		"initialized": true,
		"version":     "1.0.0",
		"lastIndexed": "2026-09-01T00:00:00Z",
		"fileCount":   12,
		"nodeCount":   100,
		"index":       map[string]any{"state": "complete", "reindexRecommended": false},
		"pendingChanges": map[string]any{
			"added": 0, "modified": 0, "deleted": 0,
		},
	}
}

// codegraphScript is a fake `codegraph` CLI reading its canned answers from
// CODEGRAPH_STATUS_JSON / CODEGRAPH_QUERY_JSON: two plain env vars avoid needing a JSON
// tool inside the shell script.
const codegraphScript = `
case "$1" in
  status) printf '%s' "$CODEGRAPH_STATUS_JSON" ;;
  query) printf '%s' "${CODEGRAPH_QUERY_JSON:-[]}" ;;
  *) printf '{}' ;;
esac
`

// codegraphProject sets PATH to a fake `codegraph` binary answering from statusJSON and
// queryJSON, and returns a project with [integrations.codegraph] configured.
func codegraphProject(t *testing.T, statusJSON, queryJSON string) *project.Project {
	t.Helper()
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBinary(t, binDir, "codegraph", codegraphScript)
	t.Setenv("PATH", pathWith(binDir))
	t.Setenv("CODEGRAPH_STATUS_JSON", statusJSON)
	t.Setenv("CODEGRAPH_QUERY_JSON", queryJSON)
	return writeProject(t, filepath.Join(tmp, "proj"), codegraphTOML)
}
