package shelf

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

// The built-in keyword search (internal/lexical) is the standard: no install, server or
// index. QMD is opt-in and adds meaning search.

// BuiltIn is what setup, doctor and search call the standard engine.
const BuiltIn = "built-in keyword search"

// errNotQMD answers a lifecycle command in a project that has not turned QMD on.
var errNotQMD = errors.New("This project searches its documents with the " + BuiltIn + ", which needs no index or server. " +
	"For meaning search with QMD, set `engine = \"qmd\"` under [integrations.shelf] in orai.toml, then `orai shelf init`")

// UsesQMD reports whether this project's shelf runs on QMD here: orai.toml says so, or
// it names no engine and this computer already has an index. The second case keeps
// projects from 0.6 and earlier, where QMD was the only engine and was not written down.
func UsesQMD(p *project.Project) bool {
	if p.Config.Shelf == nil {
		return false
	}
	return NewSettings(p).usesQMD()
}

func (s *Settings) usesQMD() bool {
	switch s.Project.Config.Shelf.Engine {
	case config.EngineQMD:
		return true
	case config.EngineBuiltIn:
		return false
	}
	return exists(s.DB) && exists(s.ConfigFile)
}

// diagnoseBuiltIn is the shelf diagnosis where QMD is off. deep runs the smoke queries.
func diagnoseBuiltIn(s *Settings, deep bool) []doctor.Check {
	if check, missing := missingFolders(s); missing {
		return []doctor.Check{check}
	}
	count := len(s.Documents())
	if count == 0 {
		return []doctor.Check{doctor.New("shelf", doctor.NotReady, "no Markdown documents to search",
			"Add Markdown files under "+strings.Join(s.collectionNames(), ", "))}
	}
	checks := []doctor.Check{doctor.New("shelf", doctor.Healthy,
		fmt.Sprintf("%s over %d document(s); no server or index", BuiltIn, count), "").
		WithDetail(map[string]any{"engine": config.EngineBuiltIn, "documents": count})}

	// An address left in the project's MCP settings has no server behind it now.
	entries, _ := localEntries(s.Project.Root)
	var files []string
	for _, e := range entries {
		if e.Name == ServerName || sameEndpoint(e.URL, s.Port) {
			files = append(files, e.File)
		}
	}
	if len(files) > 0 {
		checks = append(checks, doctor.New("shelf.registration", doctor.Degraded,
			strings.Join(files, ", ")+" registers a shelf MCP server, but this project uses the "+BuiltIn+", which has none",
			"Remove that entry, or set `engine = \"qmd\"` under [integrations.shelf] in orai.toml and `orai shelf init`"))
	}

	if !deep || s.Smoke == nil {
		return checks
	}
	want := strings.ToLower(s.ProjectPath(expectedURI(s)))
	var missed []string
	for _, query := range []string{s.Smoke.Lex, s.Smoke.Vec} {
		hits, _, _ := s.keywordSearch([]string{query}, 5)
		if !slices.ContainsFunc(hits, func(h Hit) bool { return strings.ToLower(h.Path) == want }) {
			missed = append(missed, fmt.Sprintf("%q", query))
		}
	}
	if len(missed) > 0 {
		return append(checks, doctor.New("shelf.search", doctor.Degraded,
			fmt.Sprintf("%s missed the smoke document %s for %s", BuiltIn, s.Smoke.Expect, strings.Join(missed, ", ")),
			"Use words that document contains in [integrations.shelf.smoke], or turn on QMD for meaning search"))
	}
	return append(checks, doctor.New("shelf.search", doctor.Healthy, BuiltIn+" returned the smoke document", ""))
}
