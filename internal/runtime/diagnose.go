package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
)

func nextStep(p *project.Project, role config.Role) string {
	if info, err := os.Stat(filepath.Join(p.Root, role.Worktree)); err != nil || !info.IsDir() {
		if project.MainCheckout(p.Root) != "" {
			return fmt.Sprintf("Create it with `git worktree add %s` (the repository needs a first commit)", role.Worktree)
		}
		return "Create the folder " + role.Worktree
	}
	return fmt.Sprintf("Fix the worktree, or `orai run %s --fresh` for a new conversation", role.Name)
}

// Diagnose checks local state, mail and each role's resume readiness (read-only).
func Diagnose(p *project.Project) []doctor.Check {
	var checks []doctor.Check
	if moved := p.RecordedRoot(); moved != "" {
		checks = append(checks, doctor.New("project", doctor.Degraded,
			fmt.Sprintf("local state was recorded for %s (moved or copied)", moved),
			"Review .orai/, then `orai setup` to re-bind; saved sessions need --fresh").
			WithDetail(map[string]any{"root": p.Root, "id": p.ID()}))
	} else {
		checks = append(checks, doctor.New("project", doctor.Healthy, p.ConfigPath()+" is valid", "").
			WithDetail(map[string]any{"root": p.Root, "id": p.ID(), "roles": p.Config.RoleNames()}))
	}
	if len(p.Config.Roles) == 0 {
		return append(checks, doctor.New("mail", doctor.NotConfigured, "no roles in orai.toml; messaging is not used", ""))
	}
	root := mail.Root(p.MailRoot())
	switch missing := root.Missing(p.Config.Handles()); {
	case !root.Initialized():
		checks = append(checks, doctor.New("mail", doctor.NotReady, "no mailboxes yet at "+string(root),
			"`orai setup` or the first `orai run <role>` creates them").WithCore())
	case len(missing) > 0:
		checks = append(checks, doctor.New("mail", doctor.Degraded, "missing mailbox(es): "+strings.Join(missing, ", "),
			"The next `orai run <role>` adds them").WithCore())
	default:
		checks = append(checks, doctor.New("mail", doctor.Healthy, "mailboxes present at "+string(root), "").WithCore())
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
			checks = append(checks, doctor.New("role."+name, doctor.Degraded, err.Error(), nextStep(p, role)).WithDetail(detail))
			continue
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
