package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
)

// hasCommit reports whether the repository at root has a first commit (a worktree
// cannot be added before one exists).
var hasCommit = func(root string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "HEAD")
	cmd.Dir = root
	return cmd.Run() == nil
}

func nextStep(p *project.Project, role config.Role) string {
	if info, err := os.Stat(filepath.Join(p.Root, role.Worktree)); err != nil || !info.IsDir() {
		if project.MainCheckout(p.Root) == "" {
			return "Create the folder " + role.Worktree
		}
		add := fmt.Sprintf("`git worktree add %s`", role.Worktree)
		if !hasCommit(p.Root) {
			return "Make the first commit (`git add -A && git commit -m \"Set up Orai\"`), then " + add
		}
		return add + " (from the project folder)"
	}
	return fmt.Sprintf("Fix the worktree, or `orai run %s --fresh` for a new conversation", role.Name)
}

// shown shortens a path under the project root to its relative form for messages.
func shown(p *project.Project, path string) string {
	if rel, err := filepath.Rel(p.Root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

// Diagnose checks local state, mail and each role's resume readiness (read-only).
func Diagnose(p *project.Project) []doctor.Check {
	var checks []doctor.Check
	if moved := p.RecordedRoot(); moved != "" {
		checks = append(checks, doctor.New("project", doctor.Degraded,
			fmt.Sprintf("local state was recorded for %s (moved or copied)", moved),
			"Review .orai/, then `orai setup` to re-bind; saved sessions need --fresh").
			WithDetail(map[string]any{"root": p.Root, "id": p.ID()}))
	} else if p.Config.Schema < config.Schema {
		// Still read, so nothing stops working; the edit is two lines and Orai never
		// rewrites the body of orai.toml itself.
		action := fmt.Sprintf("Edit orai.toml: set `schema = %d`", config.Schema)
		if p.Config.Shelf != nil {
			action += " and rename `[integrations.wiki]` to `[integrations.shelf]` (also `[integrations.wiki.smoke]`)"
		}
		checks = append(checks, doctor.New("project", doctor.Degraded,
			fmt.Sprintf("%s uses schema %d; this version writes schema %d", project.ConfigName, p.Config.Schema, config.Schema), action).
			WithDetail(map[string]any{"root": p.Root, "id": p.ID(), "roles": p.Config.RoleNames()}))
	} else {
		checks = append(checks, doctor.New("project", doctor.Healthy, project.ConfigName+" is valid", "").
			WithDetail(map[string]any{"root": p.Root, "id": p.ID(), "roles": p.Config.RoleNames()}))
	}
	if len(p.Config.Roles) == 0 {
		return append(checks, doctor.New("mail", doctor.NotConfigured, "no roles in orai.toml yet",
			"`orai setup --role NAME=PROVIDER` adds one (for example --role lead=codex --role dev=claude)"))
	}
	root := mail.Root(p.MailRoot())
	switch missing := root.Missing(p.Config.Handles()); {
	case !root.Initialized():
		checks = append(checks, doctor.New("mail", doctor.NotReady, "no mailboxes yet at "+shown(p, string(root)),
			"`orai setup` or the first `orai run <role>` creates them").WithCore())
	case len(missing) > 0:
		checks = append(checks, doctor.New("mail", doctor.Degraded, "missing mailbox(es): "+strings.Join(missing, ", "),
			"The next `orai run <role>` adds them").WithCore())
	default:
		checks = append(checks, doctor.New("mail", doctor.Healthy, "mailboxes present at "+shown(p, string(root)), "").WithCore())
	}
	for _, name := range p.Config.RoleNames() {
		role := p.Config.Roles[name]
		files := p.RoleFiles(name)
		detail := map[string]any{"provider": role.Provider, "running": state.Locked(files)}
		cwd, err := RoleWorktree(p, role)
		if err == nil {
			err = CheckWorktree(role, cwd)
		}
		var saved map[string]any
		if err == nil {
			saved, err = state.ReadJSON(files.State())
		}
		if err == nil {
			err = ValidateSaved(saved, role, cwd)
		}
		if err != nil {
			reason := strings.ReplaceAll(err.Error(), p.Root+string(filepath.Separator), "")
			checks = append(checks, doctor.New("role."+name, doctor.Degraded, reason, nextStep(p, role)).WithDetail(detail))
			continue
		}
		if role.Guide != "" {
			if info, err := os.Stat(filepath.Join(p.Root, role.Guide)); err != nil || info.IsDir() {
				checks = append(checks, doctor.New("role."+name, doctor.Degraded, "role guide is missing: "+role.Guide,
					"`orai setup` writes a starter guide (or fix `guide` under [roles."+name+"] in orai.toml)").WithDetail(detail))
				continue
			}
		}
		reason := "no saved conversation; the next run starts fresh"
		detail["session_id"] = nil
		if saved != nil {
			reason, detail["session_id"] = "exact resume ready", saved["session_id"]
		}
		checks = append(checks, doctor.New("role."+name, doctor.Healthy, reason, "").WithDetail(detail))
	}
	return checks
}
