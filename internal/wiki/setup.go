package wiki

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/oXpace/orai/internal/project"
)

// lifecycleCall is the seam SetupStep calls Lifecycle through; tests replace it so
// `orai setup` never runs a real qmd during a test.
var lifecycleCall = Lifecycle

// SetupStep is `orai setup`'s wiki step: bring the project wiki up once Markdown
// documents exist, skipping (never failing the whole setup) when the engine is not
// installed or there is nothing to index yet.
func SetupStep(p *project.Project, out io.Writer) string {
	if p.Config.Wiki == nil {
		return "wiki: not configured"
	}
	if _, err := lookPath("qmd"); err != nil {
		return "wiki: skipped (engine qmd is not installed; see docs/compatibility.md)"
	}
	s := NewSettings(p)
	found := false
	for _, name := range s.collectionNames() {
		folder := s.Collections[name]
		if info, err := os.Stat(folder); err == nil && info.IsDir() && hasMarkdown(folder) {
			found = true
			break
		}
	}
	if !found {
		return "wiki: skipped (no Markdown documents yet; add docs, then `orai wiki init`)"
	}
	action := "init"
	if exists(s.ConfigFile) && exists(s.DB) {
		action = "recover"
	}
	if err := lifecycleCall(p, action, out); err != nil {
		return fmt.Sprintf("wiki: %s failed: %v", action, err)
	}
	return fmt.Sprintf("wiki: ready (%s)", action)
}

// hasMarkdown reports whether folder contains a *.md file at any depth. Matches
// pathlib's rglob("*.md"), which is case-sensitive on every platform Orai supports.
func hasMarkdown(folder string) bool {
	found := false
	_ = filepath.WalkDir(folder, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// MCPServers returns the per-process MCP entries injected at role launch for provider
// (codex or claude); empty when the wiki is not configured. Global CLI config is left
// alone; CodeGraph registration stays the user's own choice
// (`codegraph install --location local`) and is never injected here.
func MCPServers(p *project.Project, provider string) map[string]map[string]any {
	servers := map[string]map[string]any{}
	if p.Config.Wiki == nil {
		return servers
	}
	s := NewSettings(p)
	if provider == "codex" {
		servers[s.ServerName] = map[string]any{
			"url":                 s.Endpoint,
			"startup_timeout_sec": 20,
			"tool_timeout_sec":    180,
		}
	} else {
		servers[s.ServerName] = map[string]any{"type": "http", "url": s.Endpoint}
	}
	return servers
}
