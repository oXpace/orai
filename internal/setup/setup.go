// Package setup implements `orai setup`: from an empty project folder to a working Orai
// project in one command.
//
// Order: preflight every file change (scaffold.Plan) -> apply -> run injected tool steps
// (wiki index, code graph) -> read-only diagnosis. A tool step is skipped, never forced,
// when its tool is missing; each step reports what happened and the diagnosis summary
// says what is left. The caller supplies the tool steps (wiki.SetupStep,
// codegraph.SetupStep) and the diagnosis function so this package does not depend on
// them.
package setup

import (
	"fmt"
	"io"
	"strings"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/scaffold"
)

// Run plans and applies `orai setup` on root, then reports tool steps and a diagnosis to
// out. It returns the exit code (1 when a tool step failed) and an error when planning,
// applying or loading failed outright; the caller prints errors (a *scaffold.Conflict is
// a usage error).
//
// steps runs only when tools is true; each receives the freshly loaded project and
// returns one report line. When tools is false, a single fixed line is printed instead
// and steps is not called.
func Run(
	root, preset string,
	dryRun bool,
	branch string,
	tools bool,
	out io.Writer,
	steps []func(*project.Project) string,
	diagnose func(*project.Project) []doctor.Check,
) (int, error) {
	actions, notes, err := scaffold.Plan(root, preset, branch)
	if err != nil {
		return 1, err
	}

	fmt.Fprintf(out, "Project: %s\n", root)

	if dryRun {
		for _, action := range actions {
			fmt.Fprintln(out, "Would: "+action.Description)
		}
		for _, note := range notes {
			fmt.Fprintln(out, "Note: "+note)
		}
		if len(actions) == 0 {
			fmt.Fprintln(out, "Already current.")
		} else {
			fmt.Fprintln(out, "Preview only. Run `orai setup` without --dry-run to apply.")
		}
		return 0, nil
	}

	if err := scaffold.Apply(root, actions, func(action scaffold.Action) {
		fmt.Fprintln(out, "Applied: "+action.Description)
	}); err != nil {
		return 1, err
	}
	if len(actions) == 0 {
		fmt.Fprintln(out, "Files already current.")
	}

	proj, err := project.Load(root)
	if err != nil {
		return 1, err
	}

	var lines []string
	if tools {
		for _, step := range steps {
			lines = append(lines, step(proj))
		}
	} else {
		lines = []string{"wiki, codegraph: skipped (--no-tools)"}
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	for _, note := range notes {
		fmt.Fprintln(out, "Note: "+note)
	}

	checks := diagnose(proj)
	status, _ := doctor.Overall(checks)
	fmt.Fprintf(out, "\nDiagnosis: %s\n", status)
	for _, check := range checks {
		if check.Status == doctor.Healthy || check.Status == doctor.NotConfigured || check.Status == doctor.NotChecked {
			continue
		}
		hint := ""
		if check.NextAction != "" {
			hint = "  → " + check.NextAction
		}
		fmt.Fprintf(out, "  %9s  %s: %s%s\n", check.Status, check.Component, check.Reason, hint)
	}

	for _, line := range lines {
		if strings.Contains(line, "failed") {
			return 1, nil
		}
	}
	return 0, nil
}
