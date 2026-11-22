package codegraph_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/oXpace/orai/internal/codegraph"
	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/project"
)

const worktreeTOML = codegraphTOML + "\n[roles.lead]\nprovider = \"codex\"\nworktree = \".\"\n\n[roles.dev]\nprovider = \"claude\"\nworktree = \".worktrees/dev\"\n"

func worktreeProject(t *testing.T) *project.Project {
	t.Helper()
	p := codegraphProject(t, "{}", "[]")
	if err := os.WriteFile(filepath.Join(p.Root, project.ConfigName), []byte(worktreeTOML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(p.Root, ".worktrees", "dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	loaded, err := project.Load(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestMCPServerIsServedFromTheRolesOwnBuiltCheckout(t *testing.T) {
	p := worktreeProject(t)
	lead, dev := p.Config.Roles["lead"], p.Config.Roles["dev"]
	if got := codegraph.MCPServers(p, lead); len(got) != 0 {
		t.Fatalf("no graph yet, no server: %v", got)
	}
	if err := os.Mkdir(filepath.Join(p.Root, ".codegraph"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := codegraph.MCPServers(p, lead)["codegraph"]
	args, _ := got["args"].([]string)
	if len(args) != 4 || args[0] != "serve" || args[1] != "--mcp" || args[3] != p.Root || got["tool_timeout_sec"] == nil {
		t.Fatalf("codex entry = %v", got)
	}
	if got := codegraph.MCPServers(p, dev); len(got) != 0 {
		t.Fatalf("a worktree without its own graph must not be served the project folder's: %v", got)
	}
	devDir := filepath.Join(p.Root, ".worktrees", "dev")
	if err := os.Mkdir(filepath.Join(devDir, ".codegraph"), 0o755); err != nil {
		t.Fatal(err)
	}
	entry := codegraph.MCPServers(p, dev)["codegraph"]
	if entry["type"] != "stdio" || entry["args"].([]string)[3] != devDir {
		t.Fatalf("claude entry = %v", entry)
	}
	if got := codegraph.MCPServers(p, config.Role{Provider: "codex", Worktree: "missing"}); len(got) != 0 {
		t.Fatalf("missing worktree: %v", got)
	}
}

func TestFoldersAndFolderForPreferTheNearestGraph(t *testing.T) {
	p := worktreeProject(t)
	devDir := filepath.Join(p.Root, ".worktrees", "dev")
	if folders := codegraph.Folders(p); len(folders) != 2 || folders[0] != p.Root || folders[1] != devDir {
		t.Fatalf("folders = %v", folders)
	}
	sub := filepath.Join(devDir, "pkg")
	_ = os.MkdirAll(sub, 0o755)
	if got := codegraph.FolderFor(p, sub); got != p.Root {
		t.Fatalf("no graph in the worktree: %s", got)
	}
	_ = os.Mkdir(filepath.Join(devDir, ".codegraph"), 0o755)
	if got := codegraph.FolderFor(p, sub); got != devDir {
		t.Fatalf("worktree graph: %s", got)
	}
}

func TestSearchParsesSymbolsAndReportsPendingChanges(t *testing.T) {
	status := completeStatus()
	status["pendingChanges"] = map[string]any{"added": 1, "modified": 2, "removed": 0}
	statusData, _ := json.Marshal(status)
	query := `[{"node":{"name":"Kickoff","kind":"function","filePath":"internal/runtime/runtime.go","startLine":101,"endLine":151,"signature":"(p) string","docstring":"Kickoff is the prompt."},"score":12.5}]`
	p := codegraphProject(t, string(statusData), query)
	if out := codegraph.Search(p, p.Root, "kickoff", 5); out.Reason == "" || out.Fix != "orai code init" {
		t.Fatalf("no graph: %+v", out)
	}
	_ = os.Mkdir(filepath.Join(p.Root, ".codegraph"), 0o755)
	out := codegraph.Search(p, p.Root, "kickoff", 5)
	if out.Reason != "" || out.Pending != 3 || len(out.Hits) != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	if h := out.Hits[0]; h.Name != "Kickoff" || h.Line != 101 || h.EndLine != 151 || h.Path != "internal/runtime/runtime.go" {
		t.Fatalf("hit = %+v", h)
	}
}
