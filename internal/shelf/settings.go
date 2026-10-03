// Package shelf is the project's document search: the Markdown collections declared in
// orai.toml, indexed and served per project, with layered verification. Users and
// agents see "shelf" (`orai shelf …`, MCP server `shelf`); the engine, QMD, stays a
// detail. A shelf is where documents are put to be found; it does not curate them.
//
// Isolation (verified against QMD 2.8.3 source): `--index <name>` scopes the daemon
// PID/log files and config file name; QMD_CONFIG_DIR and INDEX_PATH keep config and DB
// inside .orai/shelf. The model cache stays shared so models are not downloaded per
// project. Never deletes an index and never stops a server this project did not start.
package shelf

import (
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

const (
	Model = "hf:Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf"
	Host  = "127.0.0.1"

	PortBase = 18200
	PortSpan = 800

	// ServerName is the MCP server name role sessions see. It is the same in every
	// project: a session is only ever connected to its own project's server, and one
	// name keeps guidance and tool approvals (`mcp__shelf__*`) portable.
	ServerName = "shelf"

	// InstallHint is the next step when the engine is missing. Orai never installs it.
	InstallHint = doctor.InstallQMD
)

// DefaultPort derives a stable per-project port from the project id: deterministic, and
// spread across a wide range so unrelated projects rarely collide.
func DefaultPort(projectID string) int {
	sum := sha256.Sum256([]byte(projectID))
	n := new(big.Int).SetBytes(sum[:])
	mod := new(big.Int).Mod(n, big.NewInt(PortSpan))
	return PortBase + int(mod.Int64())
}

// Settings is one project's resolved shelf configuration: absolute paths, the derived
// port and index name. Callers must only build this when project.Config.Shelf is set.
type Settings struct {
	Project     *project.Project
	Index       string
	Directory   string
	ConfigFile  string
	DB          string
	Port        int
	Endpoint    string
	ServerName  string
	Collections map[string]string // name -> absolute resolved folder
	Patterns    map[string]string // name -> file pattern inside that folder
	Model       string
	Smoke       *config.Smoke
}

// NewSettings resolves a project's shelf settings. The caller must have already
// confirmed project.Config.Shelf is not nil.
func NewSettings(p *project.Project) *Settings {
	cfg := p.Config.Shelf
	s := &Settings{
		Project: p,
		Index:   "orai-" + p.ID(),
		Model:   cfg.EmbedModel,
		Smoke:   cfg.Smoke,
	}
	if s.Model == "" {
		s.Model = Model
	}
	s.Directory = filepath.Join(p.StateDir(), "shelf")
	s.ConfigFile = filepath.Join(s.Directory, s.Index+".yml")
	s.DB = filepath.Join(s.Directory, "index.sqlite")
	s.Port = cfg.Port
	if s.Port == 0 {
		s.Port = DefaultPort(p.ID())
	}
	s.Endpoint = fmt.Sprintf("http://%s:%d/mcp", Host, s.Port)
	s.ServerName = ServerName
	s.Collections = make(map[string]string, len(cfg.Collections))
	s.Patterns = make(map[string]string, len(cfg.Collections))
	for name, rel := range cfg.Collections {
		s.Collections[name] = resolvePath(filepath.Join(p.Root, rel))
		s.Patterns[name] = cfg.Patterns[name]
		if s.Patterns[name] == "" {
			s.Patterns[name] = config.DefaultPattern
		}
	}
	return s
}

// LegacyDirectory is where Orai 0.3 kept this index (.orai/wiki). The index name, port
// and PID file did not change, so moving the folder is all an upgrade needs.
func (s *Settings) LegacyDirectory() string {
	return filepath.Join(s.Project.StateDir(), "wiki")
}

// legacyOnly reports an index that is still in the 0.3 location and nowhere else. An
// empty .orai/shelf (a move that was interrupted) does not count as "elsewhere".
func (s *Settings) legacyOnly() bool {
	return !exists(s.ConfigFile) && !exists(s.DB) && exists(s.LegacyDirectory())
}

// Env is the environment for a `qmd` invocation: the process environment plus
// QMD_CONFIG_DIR and INDEX_PATH, which keep the per-index config and DB inside
// .orai/shelf. The model cache is left on its default (shared) location.
func (s *Settings) Env() []string {
	base := os.Environ()
	env := make([]string, 0, len(base)+2)
	for _, kv := range base {
		if strings.HasPrefix(kv, "QMD_CONFIG_DIR=") || strings.HasPrefix(kv, "INDEX_PATH=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "QMD_CONFIG_DIR="+s.Directory, "INDEX_PATH="+s.DB)
	return env
}

// Argv builds a `qmd` invocation scoped to this project's index.
func (s *Settings) Argv(qmd string, args ...string) []string {
	argv := make([]string, 0, len(args)+3)
	argv = append(argv, qmd, "--index", s.Index)
	argv = append(argv, args...)
	return argv
}

// PIDFile is where the daemon this project started records its PID (and, with the
// .pid suffix swapped for .log, its log).
func (s *Settings) PIDFile() string {
	cache := os.Getenv("XDG_CACHE_HOME")
	if cache == "" {
		home, _ := os.UserHomeDir()
		cache = filepath.Join(home, ".cache")
	}
	return filepath.Join(cache, "qmd", "mcp-"+s.Index+".pid")
}

// configValue is the per-index QMD config. JSON is valid YAML; QMD reads it as its
// config file for this --index.
func (s *Settings) configValue() map[string]any {
	collections := make(map[string]any, len(s.Collections))
	for name, path := range s.Collections {
		collections[name] = map[string]any{"path": path, "pattern": s.Patterns[name]}
	}
	return map[string]any{
		"collections": collections,
		"models":      map[string]any{"embed": s.Model},
	}
}

// collectionNames returns collection names in a stable (sorted) order: the
// project.Config.Shelf.Collections map itself carries no ordering guarantee.
func (s *Settings) collectionNames() []string {
	names := make([]string, 0, len(s.Collections))
	for name := range s.Collections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// overlapping lists collection pairs that index the same documents twice: one folder
// lies inside another whose pattern reaches into subfolders (for example `docs` with the
// default pattern next to `docs/adr`). Each entry reads "outer and inner".
func (s *Settings) overlapping() []string {
	var pairs []string
	names := s.collectionNames()
	for _, outer := range names {
		if !strings.Contains(s.Patterns[outer], "**") {
			continue
		}
		for _, inner := range names {
			if inner == outer {
				continue
			}
			rel, err := filepath.Rel(s.Collections[outer], s.Collections[inner])
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			if rel == "." && inner < outer {
				continue // same folder: report the pair once
			}
			pairs = append(pairs, outer+" and "+inner)
		}
	}
	return pairs
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// resolvePath makes path absolute and resolves symlinks for the segments that exist;
// segments that do not exist yet are appended literally. filepath.EvalSymlinks alone
// would fail outright when any suffix of the path is missing (for example, a smoke
// fixture that has not been created yet), and on macOS the temp directory itself sits
// behind a /var -> /private/var symlink, so a naive fallback to the unresolved path
// would disagree with collection folders (which do exist and are fully resolved).
func resolvePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	var rest []string
	dir := abs
	for {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		rest = append([]string{filepath.Base(dir)}, rest...)
		dir = parent
	}
}
