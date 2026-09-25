// Package project identifies the target project. Three places are never conflated:
// the Orai installation (wherever the binary lives), the project root (the main
// checkout that owns orai.toml and local state) and a role worktree (where a role's
// CLI works).
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/state"
)

const (
	ConfigName = "orai.toml"
	StateName  = ".orai"
	// MailName is the mailbox base; the layout is AMQ-compatible so `amq` can read it.
	MailName = ".agent-mail"
)

type NotFoundError struct{ msg string }

func (e *NotFoundError) Error() string { return e.msg }

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func Slug(text string) string {
	value := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(text), "-"), "-")
	if len(value) > 24 {
		value = strings.Trim(value[:24], "-")
	}
	if value == "" {
		return "project"
	}
	return value
}

func git(dir string, args ...string) (string, bool) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimRight(string(out), "\n"), true
}

func resolve(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

// MainCheckout returns the main worktree of the repository at path, or "" outside Git
// (or for a bare repository).
func MainCheckout(path string) string {
	out, ok := git(path, "worktree", "list", "--porcelain")
	if !ok || !strings.HasPrefix(out, "worktree ") {
		return ""
	}
	block := strings.SplitN(out, "\n\n", 2)[0]
	if strings.Contains(block, "\nbare") {
		return ""
	}
	return resolve(strings.TrimPrefix(strings.SplitN(block, "\n", 2)[0], "worktree "))
}

// WorktreeRoot returns the top of the worktree containing path, or "" outside Git.
func WorktreeRoot(path string) string {
	out, ok := git(path, "rev-parse", "--show-toplevel")
	if !ok {
		return ""
	}
	return resolve(out)
}

func findConfigDir(start string) string {
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, ConfigName)); err == nil && info.Mode().IsRegular() {
			return dir
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Locate resolves the project root for a path without loading its config.
func Locate(path string) (string, error) {
	start := resolve(path)
	if info, err := os.Stat(start); err != nil || !info.IsDir() {
		return "", &NotFoundError{"Not a directory: " + start}
	}
	found := findConfigDir(start)
	if found == "" {
		return "", &NotFoundError{fmt.Sprintf(
			"No %s at or above %s. Pass --project <path> or run `orai setup` there.", ConfigName, start)}
	}
	main, top := MainCheckout(found), WorktreeRoot(found)
	// A linked role worktree carries its own copy of orai.toml; the same path inside the
	// main checkout owns state. Directories of the main checkout itself (monorepo
	// subprojects) and worktrees whose main checkout lacks the config stay as found.
	if main != "" && top != "" && top != main {
		if rel, err := filepath.Rel(top, found); err == nil {
			candidate := filepath.Join(main, rel)
			if info, err := os.Stat(filepath.Join(candidate, ConfigName)); err == nil && info.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	return found, nil
}

// SetupTarget is where `orai setup` works when no --project is given: an existing
// project in this repository (from any subdirectory or linked worktree), otherwise the
// repository's main checkout, or the current folder outside Git. A config above the
// repository (or above a non-Git folder) belongs to someone else and is ignored.
func SetupTarget(cwd string) string {
	cwd = resolve(cwd)
	repo := MainCheckout(cwd)
	if found, err := Locate(cwd); err == nil {
		if (repo != "" && within(found, repo)) || (repo == "" && found == cwd) {
			return found
		}
	}
	if repo != "" {
		return repo
	}
	return cwd
}

type Project struct {
	Root   string
	Config *config.Config
}

func Load(path string) (*Project, error) {
	root, err := Locate(path)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(root, ConfigName))
	if err != nil {
		return nil, err
	}
	return &Project{Root: root, Config: cfg}, nil
}

func (p *Project) ConfigPath() string { return filepath.Join(p.Root, ConfigName) }
func (p *Project) StateDir() string   { return filepath.Join(p.Root, StateName) }
func (p *Project) RolesDir() string   { return filepath.Join(p.StateDir(), "roles") }

// MailRoot is this project's mailbox tree for its session.
func (p *Project) MailRoot() string { return filepath.Join(p.Root, MailName, p.Config.Session) }

func (p *Project) Name() string {
	if p.Config.Name != "" {
		return p.Config.Name
	}
	return filepath.Base(p.Root)
}

// ID is path-derived: a copied or moved checkout is a different project and cannot
// collide with the original's mail, ports or wiki server.
func (p *Project) ID() string {
	sum := sha256.Sum256([]byte(p.Root))
	return Slug(p.Name()) + "-" + hex.EncodeToString(sum[:])[:10]
}

func (p *Project) RoleFiles(role string) state.RoleFiles {
	return state.RoleFiles{Dir: p.RolesDir(), Role: role}
}

// RecordedRoot is the root recorded in local state when it differs from the current one
// (the project was moved or copied); "" otherwise.
func (p *Project) RecordedRoot() string {
	value, err := state.ReadJSON(filepath.Join(p.StateDir(), "project.json"))
	if err != nil || value == nil {
		return ""
	}
	if recorded, _ := value["root"].(string); recorded != "" && recorded != p.Root {
		return recorded
	}
	return ""
}

func IsNotFound(err error) bool {
	var nf *NotFoundError
	return errors.As(err, &nf)
}
