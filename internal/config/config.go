// Package config parses the shared project declaration (orai.toml): schema, relative
// paths, roles and integrations. Only portable facts live here; PIDs, UUIDs, absolute
// paths and caches are local state.
package config

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	Schema     = 1
	UserHandle = "user"
)

var (
	Providers   = []string{"codex", "claude"}
	WikiEngines = []string{"qmd"}
	// Role names double as CLI aliases (`orai <role>`) and mailbox handles. Former or
	// likely command names stay reserved so a role never shadows one.
	Reserved = map[string]bool{
		"setup": true, "run": true, "status": true, "doctor": true, "msg": true, "wiki": true,
		"version": true, "help": true, "_capture": true, "_channel": true, UserHandle: true, "orai": true, "all": true,
		"init": true, "inbox": true, "send": true, "reply": true, "qmd": true, "codegraph": true,
	}
	NamePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	sessionPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	collectionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
)

type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

func errorf(format string, args ...any) error { return &Error{fmt.Sprintf(format, args...)} }

type Role struct {
	Name     string
	Provider string
	Worktree string
	Guide    string // "" when not set
	Model    string
	Effort   string
	Branch   string
}

// Smoke is a query whose expected document proves the wiki serves this project.
type Smoke struct{ Lex, Vec, Expect string }

type Wiki struct {
	Engine      string
	Collections map[string]string // name -> relative folder
	Port        int               // 0 = derived from the project id
	EmbedModel  string
	Smoke       *Smoke
}

type Codegraph struct{ SmokeSymbol string }

type Config struct {
	Name      string
	Session   string
	Roles     map[string]Role
	Wiki      *Wiki
	Codegraph *Codegraph
}

// Handles returns the mailbox handles: roles in name order, then the user.
func (c *Config) Handles() []string {
	return append(c.RoleNames(), UserHandle)
}

func (c *Config) RoleNames() []string {
	names := make([]string, 0, len(c.Roles))
	for name := range c.Roles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *Config) Providers() map[string]bool {
	used := map[string]bool{}
	for _, role := range c.Roles {
		used[role.Provider] = true
	}
	return used
}

func table(value any, where string, allowed ...string) (map[string]any, error) {
	t, ok := value.(map[string]any)
	if !ok {
		return nil, errorf("%s must be a table", where)
	}
	allow := map[string]bool{}
	for _, key := range allowed {
		allow[key] = true
	}
	var unknown []string
	for key := range t {
		if !allow[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, errorf("%s: unknown key(s) %s", where, strings.Join(unknown, ", "))
	}
	return t, nil
}

func optionalText(t map[string]any, key, where string) (string, error) {
	value, present := t[key]
	if !present {
		return "", nil
	}
	return requiredText(value, where)
}

func requiredText(value any, where string) (string, error) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", errorf("%s must be a non-empty string", where)
	}
	return text, nil
}

func relative(value any, where string, contained bool) (string, error) {
	text, err := requiredText(value, where)
	if err != nil {
		return "", err
	}
	if path.IsAbs(text) || strings.HasPrefix(text, "~") {
		return "", errorf("%s must be relative to the project root", where)
	}
	if contained {
		for _, part := range strings.Split(text, "/") {
			if part == ".." {
				return "", errorf("%s must stay inside the project root", where)
			}
		}
	}
	return text, nil
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func parseRole(name string, value any) (Role, error) {
	where := "roles." + name
	if !NamePattern.MatchString(name) || Reserved[name] {
		return Role{}, errorf("%s: role names must match %s and not be a reserved word", where, NamePattern)
	}
	t, err := table(value, where, "provider", "worktree", "guide", "model", "effort", "branch")
	if err != nil {
		return Role{}, err
	}
	provider, _ := t["provider"].(string)
	if !contains(Providers, provider) {
		return Role{}, errorf("%s.provider must be one of: %s", where, strings.Join(Providers, ", "))
	}
	role := Role{Name: name, Provider: provider, Worktree: "."}
	if raw, ok := t["worktree"]; ok {
		// Sibling worktrees (../project-role) are allowed; absolute paths are not portable.
		if role.Worktree, err = relative(raw, where+".worktree", false); err != nil {
			return Role{}, err
		}
	}
	if raw, ok := t["guide"]; ok {
		if role.Guide, err = relative(raw, where+".guide", true); err != nil {
			return Role{}, err
		}
	}
	for key, target := range map[string]*string{"model": &role.Model, "effort": &role.Effort, "branch": &role.Branch} {
		if *target, err = optionalText(t, key, where+"."+key); err != nil {
			return Role{}, err
		}
	}
	return role, nil
}

func parseWiki(value any) (*Wiki, error) {
	t, err := table(value, "integrations.wiki", "engine", "collections", "port", "embed_model", "smoke")
	if err != nil {
		return nil, err
	}
	wiki := &Wiki{Engine: "qmd", Collections: map[string]string{"docs": "docs"}}
	if raw, ok := t["engine"]; ok {
		engine, _ := raw.(string)
		if !contains(WikiEngines, engine) {
			return nil, errorf("integrations.wiki.engine must be one of: %s", strings.Join(WikiEngines, ", "))
		}
		wiki.Engine = engine
	}
	if raw, ok := t["collections"]; ok {
		collections, isTable := raw.(map[string]any)
		if !isTable || len(collections) == 0 {
			return nil, errorf("integrations.wiki.collections must map at least one name to a folder")
		}
		wiki.Collections = map[string]string{}
		for key, folder := range collections {
			if !collectionPattern.MatchString(key) {
				return nil, errorf("integrations.wiki.collections: invalid collection name %q", key)
			}
			if wiki.Collections[key], err = relative(folder, "integrations.wiki.collections."+key, true); err != nil {
				return nil, err
			}
		}
	}
	if raw, ok := t["port"]; ok {
		port, isInt := raw.(int64)
		if !isInt || port < 1024 || port > 65535 {
			return nil, errorf("integrations.wiki.port must be an integer between 1024 and 65535")
		}
		wiki.Port = int(port)
	}
	if wiki.EmbedModel, err = optionalText(t, "embed_model", "integrations.wiki.embed_model"); err != nil {
		return nil, err
	}
	if raw, ok := t["smoke"]; ok {
		s, err := table(raw, "integrations.wiki.smoke", "lex", "vec", "expect")
		if err != nil {
			return nil, err
		}
		smoke := &Smoke{}
		for key, target := range map[string]*string{"lex": &smoke.Lex, "vec": &smoke.Vec, "expect": &smoke.Expect} {
			if *target, err = requiredText(s[key], "integrations.wiki.smoke."+key); err != nil {
				return nil, err
			}
		}
		wiki.Smoke = smoke
	}
	return wiki, nil
}

// Parse validates a decoded orai.toml.
func Parse(data []byte) (*Config, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, errorf("orai.toml: %v", err)
	}
	t, err := table(raw, "orai.toml", "schema", "name", "session", "roles", "integrations")
	if err != nil {
		return nil, err
	}
	if schema, _ := t["schema"].(int64); schema != Schema {
		return nil, errorf("orai.toml: schema must be %d (found %v)", Schema, t["schema"])
	}
	cfg := &Config{Session: "orai", Roles: map[string]Role{}}
	if raw, ok := t["session"]; ok {
		session, _ := raw.(string)
		if !sessionPattern.MatchString(session) {
			return nil, errorf("session must be a lowercase mailbox session name")
		}
		cfg.Session = session
	}
	if cfg.Name, err = optionalText(t, "name", "name"); err != nil {
		return nil, err
	}
	if raw, ok := t["roles"]; ok {
		roles, isTable := raw.(map[string]any)
		if !isTable {
			return nil, errorf("roles must be a table")
		}
		for name, value := range roles {
			if cfg.Roles[name], err = parseRole(name, value); err != nil {
				return nil, err
			}
		}
	}
	if raw, ok := t["integrations"]; ok {
		integrations, err := table(raw, "integrations", "wiki", "codegraph")
		if err != nil {
			return nil, err
		}
		if value, ok := integrations["wiki"]; ok {
			if cfg.Wiki, err = parseWiki(value); err != nil {
				return nil, err
			}
		}
		if value, ok := integrations["codegraph"]; ok {
			c, err := table(value, "integrations.codegraph", "smoke_symbol")
			if err != nil {
				return nil, err
			}
			cfg.Codegraph = &Codegraph{}
			if cfg.Codegraph.SmokeSymbol, err = optionalText(c, "smoke_symbol", "integrations.codegraph.smoke_symbol"); err != nil {
				return nil, err
			}
		}
	}
	return cfg, nil
}

func Load(file string) (*Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, errorf("%s: %v", file, strings.TrimPrefix(err.Error(), "orai.toml: "))
	}
	return cfg, nil
}
