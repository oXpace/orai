package shelf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oXpace/orai/internal/lexical"
)

// Hit is one document a search returned, named by its path from the project folder so
// the reader can open it with any tool.
type Hit struct {
	Path    string  `json:"path"`                // from the project folder, slash-separated
	Line    int     `json:"line,omitempty"`      // best matching line, 1-based
	Title   string  `json:"title,omitempty"`     // engine searches only
	Score   float64 `json:"score,omitempty"`     // engine searches only
	Context string  `json:"context,omitempty"`   // the folder's declared purpose
	Excerpt string  `json:"excerpt,omitempty"`   // the matching lines
	URI     string  `json:"shelf_uri,omitempty"` // <collection>/<path>, as the MCP server names it
}

// SearchOutcome is what a document search did. Mode is "engine" (the shelf server
// answered: keyword + meaning) or "keyword" (the files were scanned directly because
// the server could not answer; Reason says why and Fix what to run).
type SearchOutcome struct {
	Mode   string   `json:"mode"`
	Reason string   `json:"reason,omitempty"`
	Fix    string   `json:"fix,omitempty"`
	Stale  []string `json:"stale,omitempty"` // documents changed since the last sync
	Port   int      `json:"port,omitempty"`
	Hits   []Hit    `json:"hits"`
}

// searchTimeout bounds one engine search: the first search after a start loads the
// models, so it is generous; a hung server still falls back to the keyword scan.
var searchTimeout = 120 * time.Second

// Search finds documents for query, at most limit. It asks the shelf server first and
// scans the declared folders itself when the server is missing, refuses, serves
// another project or fails; the outcome says which happened. It never starts, stops or
// re-indexes anything.
func Search(s *Settings, query string, limit int) SearchOutcome {
	outcome := SearchOutcome{Port: s.Port, Stale: s.staleDocuments()}
	hits, reason, fix := engineSearch(s, query, limit)
	if reason == "" {
		outcome.Mode, outcome.Hits = "engine", hits
		return outcome
	}
	outcome.Mode, outcome.Reason, outcome.Fix = "keyword", reason, fix
	outcome.Hits = s.keywordSearch(query, limit)
	return outcome
}

// engineSearch runs a lex+vec query on this project's server. A non-empty reason means
// it could not, with the command that fixes it.
func engineSearch(s *Settings, query string, limit int) ([]Hit, string, string) {
	if !exists(s.DB) {
		return nil, "the shelf has not been built for this project", "orai shelf init"
	}
	switch state, detail := reachability(s.Port); state {
	case "refused":
		return nil, "the shelf server is not running", "orai shelf recover"
	case "denied":
		return nil, "access to the shelf server is denied in this environment (" + detail + "); this does not show it is down", "run outside the sandbox"
	case "error":
		return nil, "the shelf port could not be probed: " + detail, "orai doctor"
	}
	client := newClient(s.Endpoint, searchTimeout)
	status, err := handshake(client)
	if err != nil {
		return nil, fmt.Sprintf("the shelf server did not answer: %v", err), "orai shelf stop && orai shelf recover"
	}
	if mismatch := identity(status, s); mismatch != "" {
		return nil, fmt.Sprintf("port %d serves another index (%s)", s.Port, mismatch), "orai doctor"
	}
	result, err := client.Tool("query", map[string]any{
		"searches":    []map[string]any{{"type": "lex", "query": query}, {"type": "vec", "query": query}},
		"collections": s.collectionNames(),
		"limit":       limit,
	})
	if err != nil {
		return nil, fmt.Sprintf("the shelf search failed: %v", err), "orai doctor --deep"
	}
	structured, _ := result["structuredContent"].(map[string]any)
	rows, _ := structured["results"].([]any)
	hits := make([]Hit, 0, len(rows))
	for _, item := range rows {
		row, _ := item.(map[string]any)
		uri, _ := row["file"].(string)
		hit := Hit{URI: strings.TrimPrefix(uri, "qmd://"), Path: s.ProjectPath(uri)}
		hit.Title, _ = row["title"].(string)
		hit.Context, _ = row["context"].(string)
		hit.Score, _ = row["score"].(float64)
		if line, ok := row["line"].(float64); ok {
			hit.Line = int(line)
		}
		snippet, _ := row["snippet"].(string)
		hit.Excerpt = strings.TrimSpace(snippet)
		hits = append(hits, hit)
	}
	return hits, "", ""
}

// ProjectPath turns the server's "<collection>/<path>" into a path from the project
// folder. A name it cannot place is returned as given.
func (s *Settings) ProjectPath(uri string) string {
	uri = strings.TrimPrefix(uri, "qmd://")
	name, rest, _ := strings.Cut(uri, "/")
	folder, ok := s.Collections[name]
	if !ok {
		return uri
	}
	rel, err := filepath.Rel(s.Project.Root, filepath.Join(folder, filepath.FromSlash(rest)))
	if err != nil || strings.HasPrefix(rel, "..") {
		return uri
	}
	return filepath.ToSlash(rel)
}

// Documents lists every file the declared collections cover, as absolute paths in a
// stable order, each once.
func (s *Settings) Documents() []string {
	seen := map[string]bool{}
	var files []string
	for _, name := range s.collectionNames() {
		root, pattern := s.Collections[name], s.Patterns[name]
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil || !d.Type().IsRegular() || !MatchPattern(pattern, filepath.ToSlash(rel)) || seen[path] {
				return nil
			}
			seen[path] = true
			files = append(files, path)
			return nil
		})
	}
	sort.Strings(files)
	return files
}

// MatchPattern reports whether a slash-separated path inside a collection matches its
// pattern: path.Match per segment, with "**" standing for any number of folders.
func MatchPattern(pattern, rel string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchSegments(pattern, parts []string) bool {
	for len(pattern) > 0 {
		head := pattern[0]
		if head == "**" {
			for i := 0; i <= len(parts); i++ {
				if matchSegments(pattern[1:], parts[i:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 {
			return false
		}
		if ok, err := filepath.Match(head, parts[0]); err != nil || !ok {
			return false
		}
		pattern, parts = pattern[1:], parts[1:]
	}
	return len(parts) == 0
}

// staleDocuments lists documents (from the project folder) modified after the index
// was last written, so a search can say its results may be behind. Empty when the
// index does not exist yet.
func (s *Settings) staleDocuments() []string {
	if !exists(s.DB) {
		return nil
	}
	// The marker the last init/refresh/sync wrote; an index built before markers existed
	// falls back to the DB file, which searches may also touch (so it can only err fresh).
	info, err := os.Stat(s.SyncedFile())
	if err != nil {
		if info, err = os.Stat(s.DB); err != nil {
			return nil
		}
	}
	var stale []string
	for _, path := range s.Documents() {
		if doc, err := os.Stat(path); err == nil && doc.ModTime().After(info.ModTime()) {
			if rel, err := filepath.Rel(s.Project.Root, path); err == nil {
				stale = append(stale, filepath.ToSlash(rel))
			}
		}
	}
	return stale
}

// keywordSearch scans the declared documents directly, best first.
func (s *Settings) keywordSearch(query string, limit int) []Hit {
	terms := lexical.Terms(query)
	type scored struct {
		hit   Hit
		match lexical.Match
	}
	var found []scored
	for _, path := range s.Documents() {
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) || err != nil {
			continue
		}
		m := lexical.Score(string(data), terms)
		if !m.Enough(terms) {
			continue
		}
		rel, _ := filepath.Rel(s.Project.Root, path)
		found = append(found, scored{Hit{Path: filepath.ToSlash(rel), Line: m.Line, Excerpt: m.Excerpt,
			Context: s.contextFor(path)}, m})
	}
	lexical.Rank(found, func(f scored) lexical.Match { return f.match })
	hits := []Hit{}
	for i, f := range found {
		if limit > 0 && i >= limit {
			break
		}
		hits = append(hits, f.hit)
	}
	return hits
}

// contextFor is the declared folder purpose ([integrations.shelf.context]) of the
// deepest declared folder containing path, as the engine would attach it.
func (s *Settings) contextFor(path string) string {
	best, text := -1, ""
	for folder, value := range s.Project.Config.Shelf.Context {
		abs := resolvePath(filepath.Join(s.Project.Root, folder))
		rel, err := filepath.Rel(abs, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(abs) > best {
			best, text = len(abs), value
		}
	}
	return text
}
