// Package setup implements `orai setup`: from an empty project folder to a working Orai
// project in one command.
//
// Order: preflight every file change (scaffold.Plan) -> apply -> run injected tool steps
// (shelf index, code graph) -> read-only diagnosis. A tool step is skipped, never forced,
// when its tool is missing; each step reports what happened and the diagnosis summary
// says what is left. The caller supplies the tool steps (shelf.SetupStep,
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
	root string,
	opts scaffold.Options,
	dryRun bool,
	tools bool,
	out io.Writer,
	steps []func(*project.Project) string,
	diagnose func(*project.Project) []doctor.Check,
) (int, error) {
	actions, notes, err := scaffold.Plan(root, opts)
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
		lines = []string{"shelf, codegraph: skipped (--no-tools)"}
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	for _, note := range notes {
		fmt.Fprintln(out, "Note: "+note)
	}

	// The same view as `orai doctor`, limited to what still needs attention.
	fmt.Fprintln(out)
	checks := diagnose(proj)
	doctor.Render(out, checks, false, "Diagnosis")
	if status, _ := doctor.Overall(checks); status != doctor.Healthy {
		fmt.Fprintln(out, "\nSetup is done; the steps above are what is left. Check again with `orai doctor`.")
	} else {
		if names := proj.Config.RoleNames(); len(names) > 0 {
			fmt.Fprintf(out, "\nReady. Start a role in its own terminal: `orai %s`\n", names[0])
		} else {
			fmt.Fprintln(out, "\nReady. No roles yet: add them with `orai setup --role NAME=PROVIDER` (for example --role lead=codex --role dev=claude).")
		}
	}

	for _, line := range lines {
		if strings.Contains(line, "failed") {
			return 1, nil
		}
	}
	return 0, nil
}
