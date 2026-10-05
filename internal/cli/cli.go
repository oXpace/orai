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
	"github.com/oXpace/orai/internal/shelf"
	"github.com/oXpace/orai/internal/version"
)

const exitUsage = 2

var commands = map[string]bool{"setup": true, "run": true, "status": true, "doctor": true, "msg": true, "shelf": true,
	"_capture": true, "_channel": true, "help": true, "version": true}

var shelfActions = map[string]bool{"init": true, "recover": true, "refresh": true, "check": true, "stop": true}

var renamed = map[string]string{"inbox": "orai msg inbox", "send": "orai msg send", "reply": "orai msg reply",
	"qmd": "orai shelf", "wiki": "orai shelf", "init": "orai setup"}

const help = `usage: orai [--project PATH] <command> ...

Run Codex and Claude Code as role sessions with exact resume, notifications and
diagnostics. ` + "`orai <role>`" + ` is short for ` + "`orai run <role>`" + `.

commands:
  setup [--role NAME=PROVIDER ...] [--preset NAME] [--dry-run] [--no-tools]
                        Set up this project, or add roles to it (safe to re-run)
  run <role> [--fresh] [--dry-run]
                        Start or exactly resume a role
  status                Role processes, delivery readiness, pending mail
  doctor [--deep] [--json]
                        Read-only diagnosis with next steps
  msg inbox [ID] [--again] [--peek] [--limit N] [--as user]
  msg send <role|user> [--body TEXT | --file PATH] [--kind K] [--thread T] [--as user]
  msg reply <ID> [--body TEXT | --file PATH] [--kind K] [--as user]
                        Messages; the body defaults to piped stdin
  shelf init|recover|refresh|check|stop
                        Project shelf (docs search) lifecycle

options:
  --project PATH        project directory (default: nearest orai.toml above the current directory)
  --version             print the version
  -h, --help            show this help; ` + "`orai <command> --help`" + ` explains one command
`

const setupHelp = `usage: orai setup [--role NAME=PROVIDER[:WORKTREE] ... [--local]] [--preset minimal|pm-staff]
                  [--branch NAME] [--dry-run] [--no-tools] [--project PATH]

Set up the project in this folder: Git repository, orai.toml, agent instructions,
mailboxes, project shelf and code graph, then a diagnosis. Safe to re-run: existing
files are kept, and only what is missing is added.

roles:
  --role NAME=PROVIDER[:WORKTREE]
                        Declare a role; repeat for more. PROVIDER is codex or claude.
                        The project's first role works in the project folder; later
                        ones default to their own worktree, .worktrees/NAME. Give
                        :WORKTREE to choose (":." shares the project folder).
                        Works on an existing project too: new roles are added to
                        orai.toml with a guide in .agents/roles/NAME.md.
  --local               Declare the roles of this run in orai.local.toml instead, so
                        they exist on this computer only (see below).
  --preset pm-staff     Shorthand for --role pm=codex --role staff=claude, with
                        guides written for that pair. Default: minimal (no roles).

shared and local settings:
  orai.toml             what the project shares, committed: collections, contexts,
                        the code graph, and the roles and defaults the team agrees on.
  orai.local.toml       this computer's own settings, next to orai.toml and never
                        committed. Optional: without it orai.toml is used as it is.
  The local file is laid over the shared one key by key. A value replaces the shared
  value; a role, collection or context that is only local is added:
      [roles.pm]                  # orai.local.toml
      model = "..."               # replaces the shared model of pm, nothing else
      [integrations.shelf]
      port = 18401                # this computer uses another port
  Paths are relative to the project folder in both files. The local file cannot remove
  what orai.toml declares, and cannot set schema, name or session (the mailbox and the
  index are named from them). The merged result is checked as one declaration, and an
  error names the file. ` + "`orai doctor`" + ` (project) lists what the local file sets, and
  ` + "`orai status`" + ` shows each role's settings in effect.
  setup never writes orai.local.toml except to add a role with --local, and never
  changes what orai.toml already says.

examples:
  orai setup --role lead=codex --role dev=claude --role reviewer=claude
  orai setup --role reviewer=claude        # add one role later
  orai setup --dry-run                     # show what would change

options:
  --branch NAME         branch for a new repository (default trunk)
  --dry-run             print the plan and change nothing
  --no-tools            files only: skip the shelf index/server and the code graph
`

const runHelp = `usage: orai run <role> [--fresh] [--dry-run] [--project PATH]
       orai <role> ...

Start a role in this terminal, resuming exactly the conversation Orai captured for it.

  --fresh               start a new conversation (the old one stays with the provider)
  --dry-run             print the command, paths and mailbox; start nothing
`

const statusHelp = `usage: orai status [--project PATH]

Print each role as JSON: the settings in effect (provider, worktree, model, effort)
and which of them orai.local.toml set ("local"; "." means the whole role), whether it
is running, its saved session, whether notifications can be delivered, the last
delivery error, pending mail.
`

const doctorHelp = `usage: orai doctor [--deep] [--json] [--project PATH]

Check this project without changing anything, and list what to do next.

  --deep                also run real shelf searches and the code graph symbol lookup
                        (loads the search models; the first run may download them),
                        and ask Claude Code whether another MCP server is named shelf
  --json                machine-readable report (every field, including details)

marks: ✓ healthy   ✗ blocked/unsupported   ! degraded/not-ready
       - not configured (optional feature off)   · not checked in this mode

exit codes: 0 healthy, 1 something needs attention, 2 usage or orai.toml error,
3 a core component (provider CLI, login, mailboxes) is blocked.

What --deep verifies is set in orai.toml:
  [integrations.shelf.smoke]      lex / vec: two queries; expect: the document
                                 (path from the project folder) both must return
  [integrations.codegraph]       smoke_symbol: a symbol the index must contain
Without them, --deep only checks that searches return something.
`

// alreadyReceivedNotice is what `orai msg inbox <ID>` says for a message received before.
const alreadyReceivedNotice = "This message was already received, so it is not shown again and must not be processed a second time. To read it again on purpose: orai msg inbox <ID> --again"

var msgHelp = `usage: orai msg inbox [ID] [--again] [--peek] [--limit N] [--as user]
       orai msg send <role|user> [--body TEXT | --file PATH] [--kind K] [--thread T] [--as user]
       orai msg reply <ID> [--body TEXT | --file PATH] [--kind K] [--as user]

Messages between roles. Inside a role session the sender is that role. Outside one
(your own terminal), add --as user. The body comes from --body, --file or piped stdin.

  inbox                 receive pending messages (marks them read); --peek only lists
  inbox ID              receive one message. If it was already received (for
                        example by an earlier ` + "`orai msg inbox`" + `), only its header and
                        "already_received": true are printed, so a late notification
                        does not cause the same work twice; --again shows the body
  --kind K              one of: ` + strings.Join(mail.Kinds, ", ") + `

Output is JSON.
`

const shelfHelp = `usage: orai shelf init|recover|refresh|check|stop [--project PATH]

The shelf makes this project's Markdown documents searchable. Role sessions reach it
as the MCP server ` + "`shelf`" + `. It is a search index over the folders you register,
not a copy: the documents stay where they are. The engine is QMD, run only for this
project: its index lives in .orai/shelf and its server listens on 127.0.0.1.

  init      first time: build the index and embeddings, start the server, verify
  recover   start the server again (after a reboot) without rebuilding, apply the
            contexts in orai.toml, verify
  refresh   re-index after documents or the collections in orai.toml changed (a
            collection that was removed or re-declared is dropped or re-registered),
            then start and verify
  check     verify the running server end to end; changes nothing
  stop      stop this project's server

first time:
  1. Install QMD:  npm install -g @tobilu/qmd   (needs Node 22+)
  2. Put Markdown under docs/ (or the folders in orai.toml).
  3. orai shelf init
     The first run downloads the embedding model (about 0.6 GB) into ~/.cache/qmd;
     the first real search downloads the search models (about 2 GB). Projects share them.
  ` + "`orai setup`" + ` does steps 2-3 for you when QMD is installed.

after editing documents:
  orai shelf stop && orai shelf refresh
  (refresh refuses while the server runs, so a search in progress is never cut off)

settings, in orai.toml under [integrations.shelf]:
  collections = { docs = "docs" }           name = folder (relative): every .md under it
  [integrations.shelf.collections]          or one line per collection, to split a folder:
  core = { path = "docs", pattern = "*.md" }    only the files directly in docs/
  adr  = "docs/adr"                             everything under docs/adr
                                After changing them: orai shelf stop && orai shelf refresh
  port = 18181                  fixes the server port (the default is derived from the
                                project path). Set it when the address is written in a
                                settings file, as below
  embed_model = "hf:..."        read by the first ` + "`orai shelf init`" + ` only
  [integrations.shelf.smoke]    lex, vec, expect: a query pair and the document (path
                                from the project folder) they must return, checked by
                                ` + "`orai doctor --deep`" + `
  [integrations.shelf.context]  what the documents under a folder are for, one line per
  "docs/adr" = "..."            folder (path from the project folder; "." is everything).
                                Search results carry it as ` + "`context`" + `, so a reader can tell
                                a decision record from a spec before opening either. It
                                does not change what is found. After changing it:
                                orai shelf recover (no stop, nothing is re-indexed).
                                Once the table exists the index holds exactly what it
                                declares; without it Orai leaves contexts alone

reading results:
  A result names a document as <collection>/<path inside it>; the collection's folder
  is in orai.toml. Not every client hands its model the whole result (context, snippet,
  or the text a ` + "`get`" + ` returns), so find with the shelf, then read the file itself.
  What each provider was seen to pass on: ` + doctor.DocsURL + `/compatibility.md

outside role sessions (a desktop app, a plain claude or codex):
  Role sessions are connected by Orai. Anything else reads the project's own MCP
  settings, so write the server there under the name ` + "`shelf`" + `:
    .codex/config.toml    [mcp_servers.shelf]
                          url = "http://127.0.0.1:<port>/mcp"
    Claude Code           claude mcp add --transport http --scope project shelf <url>
  ` + "`orai doctor`" + ` (shelf.registration) prints the exact address and checks that what
  those files say still matches it.

More: ` + doctor.DocsURL + `/operations.md
`

// commandHelp is what `orai <command> --help` prints.
var commandHelp = map[string]string{
	"setup": setupHelp, "run": runHelp, "status": statusHelp, "doctor": doctorHelp, "msg": msgHelp, "shelf": shelfHelp,
}

func init() {
	runtime.MCPServers = shelf.MCPServers
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
	if text, ok := commandHelp[word]; ok && wantsHelp(rest) {
		fmt.Fprint(stdout, text)
		return 0, nil
	}
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
	case "shelf":
		if o, err := parse(rest, map[string]bool{"project": true}); err == nil && len(o.args) == 0 {
			fmt.Fprint(stdout, shelfHelp)
			return exitUsage, nil
		}
		return withProject(rest, nil, func(p *project.Project, o options) (int, error) {
			if len(o.args) != 1 || !shelfActions[o.args[0]] {
				return exitUsage, usagef("usage: orai shelf init|recover|refresh|check|stop (`orai shelf --help` explains each)")
			}
			return 0, shelf.Lifecycle(p, o.args[0], stdout)
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

// wantsHelp reports whether argv asks for help: `-h`/`--help` in flag position (not
// after `--`, and not as the value of the preceding flag, e.g. `--body --help`).
func wantsHelp(argv []string) bool {
	o, _ := parse(argv, map[string]bool{"project": true, "preset": true, "branch": true, "role": true, "as": true,
		"limit": true, "body": true, "file": true, "kind": true, "thread": true,
		"dry-run": false, "no-tools": false, "local": false, "fresh": false, "deep": false, "json": false, "peek": false, "again": false})
	return o.flag("help")
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
	o, err := parse(argv, map[string]bool{"project": true, "preset": true, "branch": true, "role": true,
		"dry-run": false, "no-tools": false, "local": false})
	if err != nil {
		return exitUsage, err
	}
	if len(o.args) > 0 {
		return exitUsage, usagef("setup takes no positional arguments")
	}
	if o.flag("local") && len(o.lists["role"]) == 0 {
		return exitUsage, usagef("--local goes with --role: it says where the new role is declared")
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
	opts := scaffold.Options{Preset: preset, Branch: o.get("branch"), Local: o.flag("local")}
	for _, text := range o.lists["role"] {
		role, err := scaffold.ParseRole(text)
		if err != nil {
			return exitUsage, usagef("%v", err)
		}
		opts.Roles = append(opts.Roles, role)
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
		func(p *project.Project) string { return shelf.SetupStep(p, stdout) },
		codegraph.SetupStep,
	}
	return setup.Run(root, opts, o.flag("dry-run"), !o.flag("no-tools"), stdout, steps,
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
	checks = append(checks, shelf.Diagnose(p, deep)...)
	return append(checks, codegraph.Diagnose(p, deep)...)
}

func runDoctor(argv []string, stdout io.Writer) (int, error) {
	o, err := parse(argv, map[string]bool{"project": true, "deep": false, "json": false})
	if err != nil {
		return exitUsage, err
	}
	target := o.get("project")
	if target == "" {
		if target, err = os.Getwd(); err != nil {
			return 1, err
		}
	}
	deep := o.flag("deep")
	var checks []doctor.Check
	context := map[string]any{}
	header := "No Orai project here: " + target
	p, err := project.Load(target)
	var cfgErr *config.Error
	switch {
	case project.IsNotFound(err):
		checks = []doctor.Check{doctor.New("project", doctor.NotConfigured, err.Error(),
			"`orai setup` (add roles with --role NAME=PROVIDER; see `orai setup --help`)").WithCore()}
	case errors.As(err, &cfgErr):
		checks = []doctor.Check{doctor.New("project", doctor.Blocked, err.Error(),
			"Fix orai.toml; the message names the key").WithCore()}
	case err != nil:
		return 1, err
	default:
		checks = Diagnose(p, deep)
		context = map[string]any{"project": p.Root, "deep": deep}
		header = "Orai project: " + p.Root
	}
	if o.flag("json") {
		return doctor.Report(stdout, checks, context), nil
	}
	fmt.Fprintf(stdout, "%s (orai %s)\n\n", header, version.String())
	code := doctor.Render(stdout, checks, true, "Status")
	fmt.Fprintln(stdout, "\nDetails as JSON: `orai doctor --json`. What each mark means: `orai doctor --help`.")
	return code, nil
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
		spec["peek"], spec["limit"], spec["again"] = false, true, false
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
			return exitUsage, usagef("usage: orai msg inbox [ID] [--again] [--peek] [--limit N]")
		}
		if o.flag("again") && len(o.args) == 0 {
			return exitUsage, usagef("--again needs a message ID")
		}
		if len(o.args) == 1 {
			if o.flag("peek") || hasLimit {
				return 1, errors.New("an inbox ID cannot be combined with --peek or --limit")
			}
			result, err := root.Read(me, o.args[0])
			if err != nil {
				return 1, err
			}
			if result.AlreadyReceived && !o.flag("again") {
				// A late notification for a message that was already drained: say so
				// instead of handing the body over as if it were new work.
				return 0, printJSON(stdout, struct {
					AlreadyReceived bool        `json:"already_received"`
					ReceivedAt      string      `json:"received_at,omitempty"`
					Notice          string      `json:"notice"`
					Header          mail.Header `json:"header"`
				}{true, result.ReceivedAt, alreadyReceivedNotice, result.Header})
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
