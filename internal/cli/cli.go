// Package cli parses arguments, prints results and maps errors to exit codes:
// 0 success; 1 runtime failure (doctor: degraded); 2 usage/config error;
// 3 doctor: a core component is blocked.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oXpace/orai/internal/channel"
	"github.com/oXpace/orai/internal/codegraph"
	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/runtime"
	"github.com/oXpace/orai/internal/scaffold"
	"github.com/oXpace/orai/internal/setup"
	"github.com/oXpace/orai/internal/version"
	"github.com/oXpace/orai/internal/wiki"
)

const exitUsage = 2

var commands = map[string]bool{"setup": true, "run": true, "status": true, "doctor": true, "msg": true, "wiki": true,
	"_capture": true, "_channel": true, "help": true, "version": true}

var wikiActions = map[string]bool{"init": true, "recover": true, "refresh": true, "check": true, "stop": true}

var renamed = map[string]string{"inbox": "orai msg inbox", "send": "orai msg send", "reply": "orai msg reply",
	"qmd": "orai wiki", "init": "orai setup"}

const help = `usage: orai [--project PATH] <command> ...

Run Codex and Claude Code as role sessions with exact resume, notifications and
diagnostics. ` + "`orai <role>`" + ` is short for ` + "`orai run <role>`" + `.

commands:
  setup [--preset minimal|pm-staff] [--branch NAME] [--dry-run] [--no-tools]
                        Set up this project (safe to re-run)
  run <role> [--fresh] [--dry-run]
                        Start or exactly resume a role
  status                Role processes, delivery readiness, pending mail
  doctor [--deep]       Read-only diagnosis as JSON
  msg inbox [ID] [--peek] [--limit N] [--as user]
  msg send <role|user> [--body TEXT | --file PATH] [--kind K] [--thread T] [--as user]
  msg reply <ID> [--body TEXT | --file PATH] [--kind K] [--as user]
                        Messages; the body defaults to piped stdin
  wiki init|recover|refresh|check|stop
                        Project wiki (docs search) lifecycle

options:
  --project PATH        project directory (default: nearest orai.toml above the current directory)
  --version             print the version
  -h, --help            show this help
`

func init() {
	runtime.MCPServers = wiki.MCPServers
}

// Main runs the CLI and returns the process exit code.
func Main(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	code, err := dispatch(argv, stdin, stdout, stderr)
	if err == nil {
		return code
	}
	fmt.Fprintln(stderr, "orai: "+err.Error())
	var usage *UsageError
	var cfgErr *config.Error
	var runUsage *runtime.UsageError
	var conflict *scaffold.Conflict
	if errors.As(err, &usage) || errors.As(err, &cfgErr) || errors.As(err, &runUsage) || errors.As(err, &conflict) ||
		project.IsNotFound(err) {
		return exitUsage
	}
	if code == 0 {
		code = 1
	}
	return code
}

func dispatch(argv []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	index, word := firstWord(argv)
	if hint, ok := renamed[word]; ok {
		return exitUsage, usagef("`%s` moved; use `%s`", word, hint)
	}
	switch {
	case word == "--version" || word == "version":
		fmt.Fprintln(stdout, "orai "+version.String())
		return 0, nil
	case word == "" || word == "-h" || word == "--help" || word == "help":
		fmt.Fprint(stdout, help)
		if word == "" {
			return exitUsage, nil
		}
		return 0, nil
	case strings.HasPrefix(word, "-"):
		return exitUsage, usagef("unrecognized option %s", word)
	case !commands[word]:
		// `orai <role> ...` is short for `orai run <role> ...`.
		argv = append(append(append([]string{}, argv[:index]...), "run"), argv[index:]...)
		word = "run"
	}
	rest := append(append([]string{}, argv[:index]...), argv[index+1:]...)
	switch word {
	case "setup":
		return runSetup(rest, stdout)
	case "run":
		return runRole(rest, stdout)
	case "status":
		return withProject(rest, nil, func(p *project.Project, _ options) (int, error) {
			return 0, runtime.Status(p, stdout)
		})
	case "doctor":
		return runDoctor(rest, stdout)
	case "msg":
		return runMessage(rest, stdin, stdout)
	case "wiki":
		return withProject(rest, nil, func(p *project.Project, o options) (int, error) {
			if len(o.args) != 1 || !wikiActions[o.args[0]] {
				return exitUsage, usagef("usage: orai wiki init|recover|refresh|check|stop")
			}
			return 0, wiki.Lifecycle(p, o.args[0], stdout)
		})
	case "_capture":
		return withProject(rest, nil, func(p *project.Project, _ options) (int, error) {
			return 0, runtime.Capture(p, stdin, stdout)
		})
	case "_channel":
		ch, err := channel.FromEnv(version.String())
		if err != nil {
			return 1, err
		}
		return 0, ch.Run(stdin, stdout)
	}
	return exitUsage, usagef("unknown command %s", word)
}

func withProject(argv []string, spec map[string]bool, fn func(*project.Project, options) (int, error)) (int, error) {
	full := map[string]bool{"project": true}
	for k, v := range spec {
		full[k] = v
	}
	o, err := parse(argv, full)
	if err != nil {
		return exitUsage, err
	}
	target := o.get("project")
	if target == "" {
		if target, err = os.Getwd(); err != nil {
			return 1, err
		}
	}
	p, err := project.Load(target)
	if err != nil {
		return exitUsage, err
	}
	return fn(p, o)
}

func runSetup(argv []string, stdout io.Writer) (int, error) {
	o, err := parse(argv, map[string]bool{"project": true, "preset": true, "branch": true, "dry-run": false, "no-tools": false})
	if err != nil {
		return exitUsage, err
	}
	if len(o.args) > 0 {
		return exitUsage, usagef("setup takes no positional arguments")
	}
	preset := o.get("preset")
	if preset == "" {
		preset = "minimal"
	}
	if _, ok := scaffold.Presets[preset]; !ok {
		names := make([]string, 0, len(scaffold.Presets))
		for name := range scaffold.Presets {
			names = append(names, name)
		}
		sort.Strings(names)
		return exitUsage, usagef("--preset must be one of: %s", strings.Join(names, ", "))
	}
	branch := o.get("branch")
	if branch == "" {
		branch = scaffold.DefaultBranch
	}
	var root string
	if explicit := o.get("project"); explicit != "" {
		if root, err = filepath.Abs(explicit); err != nil {
			return 1, err
		}
		if resolved, err := filepath.EvalSymlinks(root); err == nil {
			root = resolved
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return 1, err
		}
		root = project.SetupTarget(cwd)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return exitUsage, usagef("not a directory: %s", root)
	}
	steps := []func(*project.Project) string{
		func(p *project.Project) string { return wiki.SetupStep(p, stdout) },
		codegraph.SetupStep,
	}
	return setup.Run(root, preset, o.flag("dry-run"), branch, !o.flag("no-tools"), stdout, steps,
		func(p *project.Project) []doctor.Check { return Diagnose(p, false) })
}

func runRole(argv []string, stdout io.Writer) (int, error) {
	return withProject(argv, map[string]bool{"fresh": false, "dry-run": false}, func(p *project.Project, o options) (int, error) {
		if len(o.args) != 1 {
			return exitUsage, usagef("usage: orai run <role> [--fresh] [--dry-run]")
		}
		return runtime.Launch(p, o.args[0], o.flag("fresh"), o.flag("dry-run"), stdout)
	})
}

// Diagnose is the full component list shown by `orai doctor` and `orai setup`.
func Diagnose(p *project.Project, deep bool) []doctor.Check {
	local := runtime.Diagnose(p)
	checks := append([]doctor.Check{}, local[:1]...)
	checks = append(checks, doctor.ToolChecks(p.Config.Providers())...)
	checks = append(checks, local[1:]...)
	checks = append(checks, wiki.Diagnose(p, deep)...)
	return append(checks, codegraph.Diagnose(p, deep)...)
}

func runDoctor(argv []string, stdout io.Writer) (int, error) {
	o, err := parse(argv, map[string]bool{"project": true, "deep": false})
	if err != nil {
		return exitUsage, err
	}
	target := o.get("project")
	if target == "" {
		if target, err = os.Getwd(); err != nil {
			return 1, err
		}
	}
	p, err := project.Load(target)
	if project.IsNotFound(err) {
		return doctor.Report(stdout, []doctor.Check{
			doctor.New("project", doctor.NotConfigured, err.Error(), "orai setup").WithCore()}, nil), nil
	}
	var cfgErr *config.Error
	if errors.As(err, &cfgErr) {
		return doctor.Report(stdout, []doctor.Check{
			doctor.New("project", doctor.Blocked, err.Error(), "Fix orai.toml").WithCore()}, nil), nil
	}
	if err != nil {
		return 1, err
	}
	return doctor.Report(stdout, Diagnose(p, o.flag("deep")), map[string]any{"project": p.Root, "deep": o.flag("deep")}), nil
}

func printJSON(out io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

// isTerminal is a variable so tests can simulate an interactive terminal.
var isTerminal = func(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func readBody(o options, stdin io.Reader) (string, error) {
	var body string
	switch {
	case o.has("file") && o.has("body"):
		return "", usagef("--body and --file are mutually exclusive")
	case o.has("file"):
		data, err := os.ReadFile(o.get("file"))
		if err != nil {
			return "", err
		}
		body = string(data)
	case o.has("body"):
		body = o.get("body")
	case isTerminal(stdin):
		// An interactive terminal would block waiting for EOF.
		return "", errors.New("provide the message body with --body, --file, or piped stdin")
	default:
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		body = string(data)
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("message body must not be empty")
	}
	return body, nil
}

func runMessage(argv []string, stdin io.Reader, stdout io.Writer) (int, error) {
	if len(argv) == 0 || (argv[0] != "inbox" && argv[0] != "send" && argv[0] != "reply") {
		return exitUsage, usagef("usage: orai msg inbox|send|reply ...")
	}
	action := argv[0]
	spec := map[string]bool{"project": true, "as": true}
	switch action {
	case "inbox":
		spec["peek"], spec["limit"] = false, true
	case "send":
		spec["body"], spec["file"], spec["kind"], spec["thread"] = true, true, true, true
	case "reply":
		spec["body"], spec["file"], spec["kind"] = true, true, true
	}
	o, err := parse(argv[1:], spec)
	if err != nil {
		return exitUsage, err
	}
	if o.has("as") && o.get("as") != config.UserHandle {
		return exitUsage, usagef("--as only accepts %q", config.UserHandle)
	}
	if kind := o.get("kind"); !mail.ValidKind(kind) {
		return exitUsage, usagef("--kind must be one of: %s", strings.Join(mail.Kinds, ", "))
	}
	limit, hasLimit, err := o.intValue("limit")
	if err != nil {
		return exitUsage, err
	}
	if hasLimit && limit < 0 {
		return exitUsage, usagef("--limit must be non-negative")
	}
	role, err := runtime.RoleIdentity()
	if err != nil {
		return 1, err
	}
	var p *project.Project
	me := config.UserHandle
	if role != "" {
		// A role's identity is bound to its own project; never mix in another one.
		if p, err = project.Load(os.Getenv("ORAI_PROJECT")); err != nil {
			return 1, err
		}
		if explicit := o.get("project"); explicit != "" {
			if located, err := project.Locate(explicit); err != nil || located != p.Root {
				return exitUsage, usagef("a role session cannot act on another project")
			}
		}
		if o.has("as") {
			return 1, errors.New("an Orai role cannot override its messaging identity")
		}
		me = role
	} else {
		if !o.has("as") {
			return 1, errors.New("outside Orai, use --as user for an authorized Desktop action")
		}
		target := o.get("project")
		if target == "" {
			if target, err = os.Getwd(); err != nil {
				return 1, err
			}
		}
		if p, err = project.Load(target); err != nil {
			return exitUsage, err
		}
	}
	root := mail.Root(p.MailRoot())
	switch action {
	case "inbox":
		if len(o.args) > 1 {
			return exitUsage, usagef("usage: orai msg inbox [ID] [--peek] [--limit N]")
		}
		if len(o.args) == 1 {
			if o.flag("peek") || hasLimit {
				return 1, errors.New("an inbox ID cannot be combined with --peek or --limit")
			}
			result, err := root.Read(me, o.args[0])
			if err != nil {
				return 1, err
			}
			return 0, printJSON(stdout, result)
		}
		if o.flag("peek") {
			items, err := root.ListNew(me)
			if err != nil {
				return 1, err
			}
			return 0, printJSON(stdout, items)
		}
		if !hasLimit {
			limit = 20
		}
		result, err := root.Drain(me, limit, true)
		if err != nil {
			return 1, err
		}
		return 0, printJSON(stdout, result)
	case "send":
		if len(o.args) != 1 {
			return exitUsage, usagef("usage: orai msg send <role|user> [--body TEXT | --file PATH]")
		}
		target := o.args[0]
		handles := p.Config.Handles()
		known := false
		for _, h := range handles {
			known = known || h == target
		}
		if !known {
			return exitUsage, usagef("unknown recipient %q; choose from %s", target, strings.Join(handles, ", "))
		}
		body, err := readBody(o, stdin)
		if err != nil {
			return 1, err
		}
		result, err := root.Send(me, []string{target}, body, mail.SendOptions{Kind: o.get("kind"), Thread: o.get("thread")})
		if err != nil {
			return 1, err
		}
		return 0, printJSON(stdout, result)
	default: // reply
		if len(o.args) != 1 {
			return exitUsage, usagef("usage: orai msg reply <ID> [--body TEXT | --file PATH]")
		}
		body, err := readBody(o, stdin)
		if err != nil {
			return 1, err
		}
		result, err := root.Reply(me, o.args[0], body, o.get("kind"))
		if err != nil {
			return 1, err
		}
		return 0, printJSON(stdout, result)
	}
}
