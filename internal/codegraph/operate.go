package codegraph

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

// ServerName is the MCP server name role sessions see for the code graph.
const ServerName = "codegraph"

// runStreaming runs argv in cwd with its output going to out; tests replace it.
var runStreaming = func(out io.Writer, argv []string, cwd string) error {
	fmt.Fprintln(out, "+ "+strings.Join(argv, " "))
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = cwd, out, out
	cmd.Env = environment()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return nil
}

// environment is the process environment for codegraph. Orai never turns on anything
// the user did not; it passes the environment through as it is.
func environment() []string { return os.Environ() }

// Indexed reports whether dir holds a built code graph.
func Indexed(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".codegraph"))
	return err == nil && info.IsDir()
}

// Folders are the checkouts a code graph is kept for: the project folder, then each
// role worktree that exists and is not the project folder. A role working in its own
// worktree edits files there, so the graph it reads must be built from that checkout,
// not from the project folder.
func Folders(p *project.Project) []string {
	folders := []string{p.Root}
	seen := map[string]bool{p.Root: true}
	for _, name := range p.Config.RoleNames() {
		dir := roleFolder(p, p.Config.Roles[name])
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		folders = append(folders, dir)
	}
	return folders
}

func roleFolder(p *project.Project, role config.Role) string {
	dir := filepath.Join(p.Root, role.Worktree)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

// Lifecycle runs `orai code init|sync|check`. init builds the graph for every folder
// that lacks one (the project folder and the role worktrees), sync brings existing
// graphs up to date, and check prints the diagnosis. Building and syncing only write
// each folder's own .codegraph; nothing global is installed or registered.
func Lifecycle(p *project.Project, action string, out io.Writer) error {
	if p.Config.Codegraph == nil {
		return errors.New("the code graph is not configured: add [integrations.codegraph] to orai.toml")
	}
	binary, err := lookPath("codegraph")
	if err != nil {
		return errors.New("CodeGraph is not installed. " + installHint)
	}
	switch action {
	case "init":
		for _, dir := range Folders(p) {
			if Indexed(dir) {
				fmt.Fprintf(out, "%s: already built\n", dir)
				continue
			}
			if err := runStreaming(out, []string{binary, "init", dir}, dir); err != nil {
				return err
			}
		}
	case "sync":
		synced := 0
		for _, dir := range Folders(p) {
			if !Indexed(dir) {
				fmt.Fprintf(out, "%s: not built (orai code init)\n", dir)
				continue
			}
			if err := runStreaming(out, []string{binary, "sync", dir}, dir); err != nil {
				return err
			}
			synced++
		}
		if synced == 0 {
			return errors.New("no code graph is built yet: run `orai code init`")
		}
	case "check":
		if code := doctor.Render(out, Diagnose(p, true), true, "Code graph"); code != 0 {
			return errors.New("the code graph needs attention (see above)")
		}
	default:
		return fmt.Errorf("unknown action %s", action)
	}
	return nil
}

// installHint is the next step when the tool is missing. Orai never installs it.
const installHint = "Install it (`npm install -g @colbymchenry/codegraph`), then `orai code init`"

// MCPServers returns the code graph MCP server for a role, served from the role's own
// checkout. It is empty when the code graph is not configured, the tool is missing, or
// that checkout has no graph yet: a server would otherwise build or read the wrong
// tree. The server keeps its graph in step with edits itself (CodeGraph's watcher), so
// a role session needs no separate code sync.
func MCPServers(p *project.Project, role config.Role) map[string]map[string]any {
	servers := map[string]map[string]any{}
	if p.Config.Codegraph == nil {
		return servers
	}
	binary, err := lookPath("codegraph")
	if err != nil {
		return servers
	}
	dir := roleFolder(p, role)
	if dir == "" || !Indexed(dir) {
		return servers
	}
	entry := map[string]any{"command": binary, "args": []string{"serve", "--mcp", "--path", dir}}
	if role.Provider == "codex" {
		entry["startup_timeout_sec"] = 30
		entry["tool_timeout_sec"] = 120
	} else {
		entry["type"] = "stdio"
	}
	servers[ServerName] = entry
	return servers
}

// Hit is one symbol a code search returned.
type Hit struct {
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	Path      string  `json:"path"` // from the searched folder
	Line      int     `json:"line"`
	EndLine   int     `json:"end_line,omitempty"`
	Signature string  `json:"signature,omitempty"`
	Doc       string  `json:"doc,omitempty"`
	Score     float64 `json:"score,omitempty"`
}

// SearchOutcome is what a code search did. Folder is the checkout searched; Reason and
// Fix are set when nothing could be searched; Pending counts files changed since the
// graph was last synced.
type SearchOutcome struct {
	Folder  string `json:"folder,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Fix     string `json:"fix,omitempty"`
	Pending int    `json:"pending,omitempty"`
	Hits    []Hit  `json:"hits"`
}

// FolderFor picks the checkout to search from cwd: the nearest folder with a graph
// between cwd and the project folder, else the project folder.
func FolderFor(p *project.Project, cwd string) string {
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		rel, err := filepath.Rel(p.Root, dir)
		if err != nil || strings.HasPrefix(rel, "..") {
			break
		}
		if Indexed(dir) {
			return dir
		}
		if dir == p.Root || filepath.Dir(dir) == dir {
			break
		}
	}
	return p.Root
}

// Search finds symbols for query in folder's graph. It reads only: a graph that is
// behind is reported (Pending), not synced.
func Search(p *project.Project, folder, query string, limit int) SearchOutcome {
	outcome := SearchOutcome{Folder: folder, Hits: []Hit{}}
	if p.Config.Codegraph == nil {
		outcome.Reason, outcome.Fix = "the code graph is not configured", "add [integrations.codegraph] to orai.toml"
		return outcome
	}
	binary, err := lookPath("codegraph")
	if err != nil {
		outcome.Reason, outcome.Fix = "CodeGraph is not installed", installHint
		return outcome
	}
	if !Indexed(folder) {
		outcome.Reason, outcome.Fix = "no code graph is built for "+folder, "orai code init"
		return outcome
	}
	if raw, err := runJSON([]string{binary, "status", "--json", folder}, folder); err == nil {
		status, _ := raw.(map[string]any)
		pending, _ := status["pendingChanges"].(map[string]any)
		for _, key := range []string{"added", "modified", "removed"} {
			if n, ok := pending[key].(float64); ok {
				outcome.Pending += int(n)
			}
		}
	}
	raw, err := runJSON([]string{binary, "query", query, "--json", "--limit", fmt.Sprint(limit), "--path", folder}, folder)
	if err != nil {
		outcome.Reason, outcome.Fix = fmt.Sprintf("codegraph query failed: %v", err), "orai code check"
		return outcome
	}
	rows, _ := raw.([]any)
	for _, item := range rows {
		row, _ := item.(map[string]any)
		node, _ := row["node"].(map[string]any)
		hit := Hit{}
		hit.Name, _ = node["name"].(string)
		hit.Kind, _ = node["kind"].(string)
		hit.Path, _ = node["filePath"].(string)
		hit.Signature, _ = node["signature"].(string)
		hit.Doc, _ = node["docstring"].(string)
		hit.Score, _ = row["score"].(float64)
		if n, ok := node["startLine"].(float64); ok {
			hit.Line = int(n)
		}
		if n, ok := node["endLine"].(float64); ok {
			hit.EndLine = int(n)
		}
		outcome.Hits = append(outcome.Hits, hit)
	}
	return outcome
}
