// Package scaffold plans every change `orai setup` would make, refuses to touch anything
// on any conflict, then applies the plan with backups (each write atomic).
//
// Existing AGENTS.md/.gitignore are never rewritten wholesale: only the marked Orai block
// changes. Files Orai generates carry a marker; files without it belong to the user.
//
// Orai does not depend on AMQ: it owns its own mailbox queue (internal/mail). There is
// no `.amqrc` and no `amq coop init` action; a project with roles instead gets a
// mailbox-initializing action that calls mail.Init directly.
package scaffold

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
	"github.com/oXpace/orai/internal/version"
)

//go:embed templates/*
var templatesFS embed.FS

// Generated marks files Orai owns outright (e.g. SKILL.md): a file carrying it may be
// rewritten wholesale on template change; one without it belongs to the user.
const Generated = "<!-- orai:generated"

// DefaultBranch is the branch a new Git repository is initialized on.
const DefaultBranch = "trunk"

// InstallSpec is how a project pins Orai: a mise github-backend source until a release
// binary ships under a shorter identifier.
const InstallSpec = "github:oXpace/orai"

const (
	// Skill is the project-owned copy of the messaging skill Orai regenerates.
	Skill = ".agents/skills/orai/SKILL.md"
	// AgentsIgnore keeps the role guides (.agents/roles/) untracked.
	AgentsIgnore = ".agents/.gitignore"
	// ClaudeSkillLink is the Claude Code skill symlink Orai maintains for Skill.
	ClaudeSkillLink = ".claude/skills/orai"
	// ClaudeSkillTarget is ClaudeSkillLink's relative link target.
	ClaudeSkillTarget = "../../.agents/skills/orai"
)

// miseFiles are the mise config files checked for an Orai pin, in lookup order.
var miseFiles = []string{"mise.toml", ".mise.toml", "mise.local.toml"}

// RoleSpec is one role `orai setup` should declare: `--role NAME=PROVIDER[:WORKTREE]`.
// An empty Worktree means the default: the project root for the project's first role,
// `.worktrees/<name>` for every later one.
type RoleSpec struct{ Name, Provider, Worktree string }

// ParseRole reads a `--role` value. Names and providers are checked here so a typo is
// reported as a usage error before anything is planned.
func ParseRole(text string) (RoleSpec, error) {
	name, rest, ok := strings.Cut(text, "=")
	provider, worktree, _ := strings.Cut(rest, ":")
	spec := RoleSpec{Name: name, Provider: provider, Worktree: worktree}
	known := false
	for _, p := range config.Providers {
		known = known || p == provider
	}
	switch {
	case !ok || !known:
		return spec, fmt.Errorf("--role %q: expected NAME=PROVIDER[:WORKTREE] with provider %s",
			text, strings.Join(config.Providers, " or "))
	case !config.NamePattern.MatchString(name) || config.Reserved[name]:
		return spec, fmt.Errorf("--role %q: the name must match %s and not be a reserved word", text, config.NamePattern)
	}
	return spec, nil
}

// Preset is a named starting set of roles. Guides names the embedded template for a
// role's guide file; a role without one gets the generic role.md.
type Preset struct {
	Roles  []RoleSpec
	Guides map[string]string
}

// Presets are shorthands for `--role` lists: `--preset pm-staff` equals
// `--role pm=codex --role staff=claude` with role guides written for that pair.
var Presets = map[string]Preset{
	"minimal": {},
	"pm-staff": {
		Roles:  []RoleSpec{{"pm", "codex", ""}, {"staff", "claude", ""}},
		Guides: map[string]string{"pm": "pm.md", "staff": "staff.md"},
	},
}

// Options selects what Plan sets up beyond the files every project gets.
type Options struct {
	Preset string     // "" means "minimal"
	Branch string     // "" means DefaultBranch; used only when a repository is created
	Roles  []RoleSpec // roles to declare, after the preset's
}

// PresetNames returns the known preset names in sorted order.
func PresetNames() []string {
	names := make([]string, 0, len(Presets))
	for name := range Presets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

const rolesPlaceholder = "{{roles}}\n"

// noRoles is the roles section of a new orai.toml that declares none.
const noRoles = `# Roles: one resident agent session each, started with ` + "`orai <name>`" + `. Any name and count works.
# Add one with ` + "`orai setup --role NAME=PROVIDER`" + ` (for example --role lead=codex --role dev=claude),
# or by hand:
# [roles.lead]
# provider = "codex"        # codex | claude
# worktree = "."            # relative to this file; sibling worktrees like "../repo-lead" also work
# guide = ".agents/roles/lead.md"
# model = "..."             # optional; the provider's default otherwise
# effort = "..."            # optional
# branch = "trunk"          # optional; refuse to start on another branch
`

const rolesHeader = `# Roles: one resident agent session each, started with ` + "`orai <name>`" + `. Any name and count works.
# Add one with ` + "`orai setup --role NAME=PROVIDER`" + ` or by editing this file. Optional keys per role:
# model = "...", effort = "..." (the provider's defaults otherwise), branch = "trunk" (refuse another branch).
`

// guidePath is where setup writes a new role's guide.
func guidePath(name string) string { return ".agents/roles/" + name + ".md" }

// roleTable renders one [roles.<name>] table.
func roleTable(role RoleSpec) string {
	hint := ""
	if role.Worktree != "." {
		hint = "   # create with: git worktree add " + role.Worktree
	}
	return fmt.Sprintf("[roles.%s]\nprovider = %q\nworktree = %q%s\nguide = %q\n",
		role.Name, role.Provider, role.Worktree, hint, guidePath(role.Name))
}

// newConfig is the orai.toml of a project that has none yet.
func newConfig(roles []RoleSpec) string {
	section := noRoles
	if len(roles) > 0 {
		tables := make([]string, len(roles))
		for i, role := range roles {
			tables[i] = roleTable(role)
		}
		section = rolesHeader + strings.Join(tables, "\n")
	}
	section += "\n"
	return strings.Replace(Template("config.toml"), rolesPlaceholder, section, 1)
}

// StartHeading opens the section of a role guide that holds what this role must do
// every time its session opens, beyond the steps every role shares. The session's
// prompt lists the shared steps and points at this section (see runtime.Kickoff).
const StartHeading = "## 세션 시작"

// startSection is the provider-specific part of a session's start, written into a new
// role's guide followed by a blank line; empty when the provider needs nothing beyond
// the shared steps (Codex).
func startSection(provider string) string {
	if provider != "claude" {
		return ""
	}
	return StartHeading + "\n\n세션을 새로 시작하거나 다시 열 때마다 한다.\n\n" +
		"1. orai MCP의 `channel_ready` 도구를 호출해 수신 준비를 알린다. 호출하기 전에는 새 메시지 알림이 오지 않는다.\n\n"
}

// roleGuide renders the guide file for a role from the preset's template, or the
// generic one, with the start routine of the role's provider.
func roleGuide(preset Preset, role RoleSpec) string {
	name, ok := preset.Guides[role.Name]
	if !ok {
		name = "role.md"
	}
	return strings.NewReplacer("{{name}}", role.Name, "{{worktree}}", role.Worktree,
		"{{start}}", startSection(role.Provider)).Replace(Template(name))
}

// marker is one style's begin/end pair.
type marker struct{ Begin, End string }

// Markers are the managed-block delimiters, keyed by style: "md" (AGENTS.md, HTML
// comments) and "hash" (.gitignore, shell comments).
var Markers = map[string]marker{
	"md":   {"<!-- orai:begin (managed by `orai setup`; edit outside this block) -->", "<!-- orai:end -->"},
	"hash": {"# orai:begin (managed by `orai setup`)", "# orai:end"},
}

// Conflict lists every problem Plan found; Apply is never reached while any remain.
type Conflict struct{ messages []string }

func (c *Conflict) Error() string { return strings.Join(c.messages, "\n") }

// Template returns the embedded template's text. Templates are a fixed, build-time
// resource: a missing name is a programmer error, not a runtime condition to recover
// from, so this panics instead of returning an error.
func Template(name string) string {
	data, err := templatesFS.ReadFile("templates/" + name)
	if err != nil {
		panic(fmt.Sprintf("scaffold: missing embedded template %q: %v", name, err))
	}
	return string(data)
}

// Action is one planned filesystem change.
type Action struct {
	Description string
	Path        string
	Data        []byte
	Mode        os.FileMode // ignored when the file already exists: its current mode wins
	Backup      bool
	Symlink     string       // relative link target; mutually exclusive with Data/Run
	Run         func() error // delegated action (git init, mailbox init); mutually exclusive with Data/Symlink
}

// ManagedBlock returns the file text with the Orai block inserted or replaced, and
// whether the result is well-formed. existing is nil for a file that does not exist yet.
// The begin marker is matched by prefix, so a block written with older wording (for
// example the legacy "managed by `orai init`" phrasing) is replaced rather than
// duplicated.
func ManagedBlock(existing *string, body, style string) (string, bool) {
	m := Markers[style]
	prefix := strings.SplitN(m.Begin, " (", 2)[0]
	block := m.Begin + "\n" + strings.TrimRight(body, " \t\r\n") + "\n" + m.End + "\n"
	if existing == nil {
		return block, true
	}
	text := *existing
	lines := splitLinesKeepEnds(text)
	var starts, ends []int
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			starts = append(starts, i)
		}
		if strings.TrimRight(line, "\n") == m.End {
			ends = append(ends, i)
		}
	}
	if len(starts) != len(ends) || len(starts) > 1 || (len(starts) > 0 && ends[0] < starts[0]) {
		return "", false
	}
	if len(starts) == 0 {
		separator := ""
		if text != "" {
			if strings.HasSuffix(text, "\n") {
				separator = "\n"
			} else {
				separator = "\n\n"
			}
		}
		return text + separator + block, true
	}
	return strings.Join(lines[:starts[0]], "") + block + strings.Join(lines[ends[0]+1:], ""), true
}

func hasBlock(existing, style string) bool {
	prefix := strings.SplitN(Markers[style].Begin, " (", 2)[0]
	for _, line := range strings.Split(existing, "\n") {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

// splitLinesKeepEnds splits "\n"-terminated text into lines that each keep their
// trailing newline, without producing a spurious empty trailing element for a final
// trailing newline.
func splitLinesKeepEnds(text string) []string {
	parts := strings.SplitAfter(text, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func parsedConfig(text string) *config.Config {
	cfg, err := config.Parse([]byte(text))
	if err != nil {
		return nil
	}
	return cfg
}

// ignoreLines returns the .gitignore block body: local state and indexes, plus the
// mailbox root and nested role worktrees for projects with roles.
//
// Role order comes from config.Config.RoleNames (sorted) rather than TOML declaration
// order, since the Go config parser (like Go maps in general) does not preserve source
// order; the result is deterministic.
func ignoreLines(cfg *config.Config) []string {
	lines := []string{"/" + project.StateName + "/"}
	add := func(v string) {
		for _, existing := range lines {
			if existing == v {
				return
			}
		}
		lines = append(lines, v)
	}
	if cfg != nil && cfg.Codegraph != nil {
		add("/.codegraph/")
	}
	if cfg != nil && len(cfg.Roles) > 0 {
		add("/" + project.MailName + "/")
		for _, name := range cfg.RoleNames() {
			role := cfg.Roles[name]
			if role.Worktree == "." {
				continue
			}
			var parts []string
			dotdot := false
			for _, part := range strings.Split(role.Worktree, "/") {
				if part == "" || part == "." {
					continue
				}
				if part == ".." {
					dotdot = true
					break
				}
				parts = append(parts, part)
			}
			if dotdot || len(parts) == 0 {
				continue
			}
			add("/" + strings.Join(parts, "/") + "/")
		}
	}
	return lines
}

// mailboxAction returns the action that brings the project's mailbox root up to date
// with the configured roles, or nil when it already covers every handle.
func mailboxAction(root string, cfg *config.Config) *Action {
	handles := cfg.Handles()
	mailRoot := mail.Root(filepath.Join(root, project.MailName, cfg.Session))
	if mailRoot.Initialized() && len(mailRoot.Missing(handles)) == 0 {
		return nil
	}
	return &Action{
		Description: fmt.Sprintf("Initialize mailboxes %s/%s", project.MailName, cfg.Session),
		Path:        string(mailRoot),
		Run:         func() error { return mail.Init(mailRoot, handles) },
	}
}

// gitAction returns the action that turns a bare folder into a Git repository, or nil
// when root is already inside one (a monorepo subproject is left alone).
func gitAction(root, branch string) *Action {
	if project.WorktreeRoot(root) != "" {
		return nil
	}
	return &Action{
		Description: fmt.Sprintf("Initialize a Git repository on branch %s", branch),
		Path:        filepath.Join(root, ".git"),
		Run: func() error {
			cmd := exec.Command("git", "init", "-q", "-b", branch)
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				msg := strings.TrimSpace(string(out))
				if msg == "" {
					msg = err.Error()
				}
				return fmt.Errorf("git init failed: %s", msg)
			}
			return nil
		},
	}
}

// isOraiSource reports whether root is the Orai source checkout itself (its own module
// path is github.com/oXpace/orai), in which case it runs its own code and the "pin
// Orai" note does not apply.
func isOraiSource(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")) == "github.com/oXpace/orai"
		}
	}
	return false
}

// misePinNote reports whether root pins Orai in a mise config; empty when it does (or
// when root does not need to, being the Orai source itself).
func misePinNote(root string) string {
	if isOraiSource(root) {
		return ""
	}
	for _, name := range miseFiles {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var doc map[string]any
		if err := toml.Unmarshal(data, &doc); err != nil {
			return fmt.Sprintf("%s is not valid TOML; cannot check the Orai pin", name)
		}
		tools, _ := doc["tools"].(map[string]any)
		for key := range tools {
			if !strings.HasPrefix(key, "github:") && !strings.HasPrefix(key, "pypi:") {
				continue
			}
			if strings.HasSuffix(strings.TrimRight(key, "/"), "/orai") {
				return ""
			}
		}
	}
	return fmt.Sprintf("This project does not pin Orai. Pin it with `mise use --pin %s` so everyone on it runs the same version", InstallSpec)
}

func resolveRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return abs, nil
}

// Plan returns every action `orai setup` would take on root, in application order, plus
// advisory notes. It writes nothing. When any conflict is found, it returns a *Conflict
// listing every one found (not just the first) and no actions.
//
// Roles (the preset's, then opts.Roles) are declared in a new orai.toml, or appended to
// an existing one as new [roles.<name>] tables; a role the file already declares is
// left as it is. Each newly declared role gets a guide file unless one exists.
func Plan(root string, opts Options) ([]Action, []string, error) {
	root, err := resolveRoot(root)
	if err != nil {
		return nil, nil, err
	}
	if opts.Preset == "" {
		opts.Preset = "minimal"
	}
	if opts.Branch == "" {
		opts.Branch = DefaultBranch
	}
	preset, ok := Presets[opts.Preset]
	if !ok {
		return nil, nil, &Conflict{[]string{fmt.Sprintf("Unknown preset %q; choose from %s", opts.Preset, strings.Join(PresetNames(), ", "))}}
	}

	var actions []Action
	var notes []string
	var conflicts []string

	if initialize := gitAction(root, opts.Branch); initialize != nil {
		actions = append(actions, *initialize)
	}

	regular := func(rel string) (string, bool) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			conflicts = append(conflicts, rel+" is a symlink; refusing to write through it")
			return "", false
		}
		if fi, err := os.Stat(p); err == nil && !fi.Mode().IsRegular() {
			conflicts = append(conflicts, rel+" exists and is not a file")
			return "", false
		}
		return p, true
	}

	// Existing declaration, if any. An invalid one is a conflict; nothing is added to it.
	configPath := filepath.Join(root, project.ConfigName)
	var existingText string
	var existing *config.Config
	hasConfig := false
	if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
		data, rerr := os.ReadFile(configPath)
		if rerr != nil {
			return nil, nil, rerr
		}
		hasConfig, existingText = true, string(data)
		if existing, err = config.Parse(data); err != nil {
			conflicts = append(conflicts, fmt.Sprintf("%s is invalid: %v", project.ConfigName, err))
		}
	}

	// Resolve the requested roles against what is already declared.
	rootTaken := false
	if existing != nil {
		for _, role := range existing.Roles {
			rootTaken = rootTaken || role.Worktree == "."
		}
	}
	var added []RoleSpec
	seen := map[string]bool{}
	for _, role := range append(append([]RoleSpec{}, preset.Roles...), opts.Roles...) {
		if seen[role.Name] {
			conflicts = append(conflicts, fmt.Sprintf("role %s is requested twice", role.Name))
			continue
		}
		seen[role.Name] = true
		if existing != nil {
			if declared, ok := existing.Roles[role.Name]; ok {
				if declared.Provider != role.Provider {
					conflicts = append(conflicts, fmt.Sprintf("role %s is already declared with provider %s; edit %s to change it",
						role.Name, declared.Provider, project.ConfigName))
				}
				continue
			}
		}
		if role.Worktree == "" {
			role.Worktree = ".worktrees/" + role.Name
			if !rootTaken {
				role.Worktree = "."
			}
		}
		rootTaken = rootTaken || role.Worktree == "."
		added = append(added, role)
	}

	configText := existingText
	switch {
	case !hasConfig:
		configText = newConfig(added)
		if p, ok := regular(project.ConfigName); ok {
			actions = append(actions, Action{Description: "Create " + project.ConfigName, Path: p, Data: []byte(configText), Mode: 0o644})
		}
	case existing != nil && len(added) > 0:
		names := make([]string, len(added))
		for i, role := range added {
			configText = strings.TrimRight(configText, "\n") + "\n\n" + roleTable(role)
			names[i] = role.Name
		}
		if _, err := config.Parse([]byte(configText)); err != nil {
			conflicts = append(conflicts, fmt.Sprintf("cannot add role(s) %s to %s automatically (%v); declare them by hand",
				strings.Join(names, ", "), project.ConfigName, err))
			configText = existingText
		} else if p, ok := regular(project.ConfigName); ok {
			actions = append(actions, Action{
				Description: fmt.Sprintf("Add role(s) %s to %s", strings.Join(names, ", "), project.ConfigName),
				Path:        p, Data: []byte(configText), Mode: 0o644, Backup: true,
			})
		}
	}
	if !hasConfig {
		if _, err := config.Parse([]byte(configText)); err != nil {
			conflicts = append(conflicts, fmt.Sprintf("cannot declare the requested roles: %v", err))
		}
	}

	wanted := Template("skill.md")
	if skillPath, ok := regular(Skill); ok {
		if _, err := os.Stat(skillPath); err == nil {
			current, rerr := os.ReadFile(skillPath)
			if rerr == nil && string(current) != wanted {
				if strings.Contains(string(current), Generated) {
					actions = append(actions, Action{Description: "Update generated " + Skill, Path: skillPath, Data: []byte(wanted), Mode: 0o644, Backup: true})
				} else {
					conflicts = append(conflicts, Skill+" exists and was not generated by Orai")
				}
			}
		} else {
			actions = append(actions, Action{Description: "Create " + Skill, Path: skillPath, Data: []byte(wanted), Mode: 0o644})
		}
	}

	linkPath := filepath.Join(root, ClaudeSkillLink)
	if fi, err := os.Lstat(linkPath); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		if target, rerr := os.Readlink(linkPath); rerr == nil && target != ClaudeSkillTarget {
			conflicts = append(conflicts, fmt.Sprintf("%s points to %s, not %s", ClaudeSkillLink, target, ClaudeSkillTarget))
		}
	} else if err == nil {
		conflicts = append(conflicts, ClaudeSkillLink+" exists and is not the Orai skill link")
	} else {
		actions = append(actions, Action{
			Description: fmt.Sprintf("Link %s -> %s for Claude Code", ClaudeSkillLink, ClaudeSkillTarget),
			Path:        linkPath,
			Symlink:     ClaudeSkillTarget,
		})
	}

	cfg := parsedConfig(configText)

	// A guide for every declared role that names one and lacks the file: the roles just
	// added, and any the user declared or renamed by hand. Existing guides are never
	// rewritten.
	if cfg != nil {
		fromPreset := map[string]bool{}
		for _, role := range added {
			fromPreset[role.Name] = true
		}
		for _, name := range cfg.RoleNames() {
			role := cfg.Roles[name]
			if role.Guide == "" {
				continue
			}
			p, ok := regular(role.Guide)
			if !ok {
				continue
			}
			if _, err := os.Stat(p); err != nil {
				chosen := Preset{}
				if fromPreset[name] {
					chosen = preset
				}
				spec := RoleSpec{Name: name, Provider: role.Provider, Worktree: role.Worktree}
				actions = append(actions, Action{Description: "Create " + role.Guide, Path: p, Data: []byte(roleGuide(chosen, spec)), Mode: 0o644})
			}
		}
	}

	if cfg != nil && cfg.Wiki != nil {
		folderSet := map[string]bool{}
		for _, folder := range cfg.Wiki.Collections {
			folderSet[folder] = true
		}
		folders := make([]string, 0, len(folderSet))
		for folder := range folderSet {
			folders = append(folders, folder)
		}
		sort.Strings(folders)
		for _, folder := range folders {
			if _, err := os.Stat(filepath.Join(root, folder)); err != nil {
				if p, ok := regular(folder + "/README.md"); ok {
					actions = append(actions, Action{
						Description: fmt.Sprintf("Create %s/README.md (wiki starter page)", folder),
						Path:        p,
						Data:        []byte(Template("docs-readme.md")),
						Mode:        0o644,
					})
				}
			}
		}
	}

	if cfg != nil && len(cfg.Roles) > 0 {
		if action := mailboxAction(root, cfg); action != nil {
			actions = append(actions, *action)
		}
	}

	blocks := []struct{ Rel, Body, Style string }{
		{"AGENTS.md", Template("agents-block.md"), "md"},
		{".gitignore", strings.Join(ignoreLines(cfg), "\n"), "hash"},
		// Role guides are written per machine and stay out of the repository; the skill
		// next to them is shared.
		{AgentsIgnore, "/roles/", "hash"},
	}
	for _, block := range blocks {
		p, ok := regular(block.Rel)
		if !ok {
			continue
		}
		var existing *string
		if data, err := os.ReadFile(p); err == nil {
			s := string(data)
			existing = &s
		}
		updated, valid := ManagedBlock(existing, block.Body, block.Style)
		if !valid {
			conflicts = append(conflicts, block.Rel+" has an unbalanced or duplicated orai block; fix it by hand")
			continue
		}
		if existing == nil || updated != *existing {
			verb := "Create"
			if existing != nil {
				if hasBlock(*existing, block.Style) {
					verb = "Update the Orai block in"
				} else {
					verb = "Add the Orai block to"
				}
			}
			actions = append(actions, Action{
				Description: verb + " " + block.Rel,
				Path:        p,
				Data:        []byte(updated),
				Mode:        0o644,
				Backup:      existing != nil,
			})
		}
	}

	if claudePath, ok := regular("CLAUDE.md"); ok {
		if _, err := os.Stat(claudePath); err != nil {
			actions = append(actions, Action{Description: "Create CLAUDE.md importing AGENTS.md", Path: claudePath, Data: []byte("@AGENTS.md\n"), Mode: 0o644})
		} else if data, rerr := os.ReadFile(claudePath); rerr == nil && !strings.Contains(string(data), "AGENTS.md") {
			notes = append(notes, "CLAUDE.md does not reference AGENTS.md; add `@AGENTS.md` so Claude sees the Orai block")
		}
	}

	localPath := filepath.Join(root, project.StateName, "project.json")
	recorded, _ := state.ReadJSON(localPath)
	recordedRoot, _ := recorded["root"].(string)
	if recorded == nil || recordedRoot != root {
		if recorded != nil {
			notes = append(notes, fmt.Sprintf("Local state was recorded for %v; saved sessions will need --fresh", recorded["root"]))
		}
		value := struct {
			Root          string `json:"root"`
			InitializedBy string `json:"initialized_by"`
			InitializedAt string `json:"initialized_at"`
		}{Root: root, InitializedBy: version.String(), InitializedAt: time.Now().Format("2006-01-02T15:04:05-07:00")}
		data := state.JSONBytes(value)
		verb := "Create"
		if recorded != nil {
			verb = "Re-bind"
		}
		actions = append(actions, Action{
			Description: verb + " local state " + project.StateName + "/project.json",
			Path:        localPath,
			Data:        data,
			Mode:        0o600,
		})
	}

	if pin := misePinNote(root); pin != "" {
		notes = append(notes, pin)
	}

	if len(conflicts) > 0 {
		return nil, nil, &Conflict{conflicts}
	}
	return actions, notes, nil
}

// Apply carries out actions in order. Each write is atomic; the plan as a whole is not
// (a later action can still fail after earlier ones succeeded). Shared project folders
// get normal permissions; anything under the project's state directory (.orai) stays
// private (0700 dirs, 0600 files by default, or an action's own Mode). done is called
// once per action, only after that action has fully succeeded.
func Apply(root string, actions []Action, done func(Action)) error {
	stamp := time.Now().Format("20060102T150405")
	backupsDir := filepath.Join(root, project.StateName, "backups", stamp)
	for _, action := range actions {
		rel, err := filepath.Rel(root, action.Path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		underState := false
		for _, p := range parts {
			if p == project.StateName {
				underState = true
				break
			}
		}
		if action.Backup {
			if _, err := os.Stat(action.Path); err == nil {
				data, err := os.ReadFile(action.Path)
				if err != nil {
					return err
				}
				target := filepath.Join(backupsDir, rel)
				if err := state.AtomicWrite(target, data, 0o600); err != nil {
					return err
				}
			}
		}
		if !underState {
			if err := os.MkdirAll(filepath.Dir(action.Path), 0o777); err != nil {
				return err
			}
		}
		switch {
		case action.Run != nil:
			if err := action.Run(); err != nil {
				return err
			}
		case action.Symlink != "":
			if err := os.Symlink(action.Symlink, action.Path); err != nil {
				return err
			}
		default:
			mode := action.Mode
			if mode == 0 {
				mode = 0o644
			}
			if info, err := os.Stat(action.Path); err == nil {
				mode = info.Mode().Perm()
			}
			if underState {
				if err := state.AtomicWrite(action.Path, action.Data, mode); err != nil {
					return err
				}
			} else {
				if err := state.WriteReplace(action.Path, action.Data, mode); err != nil {
					return err
				}
			}
		}
		if done != nil {
			done(action)
		}
	}
	return nil
}
