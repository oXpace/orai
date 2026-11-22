package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oXpace/orai/internal/codegraph"
	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/runtime"
	"github.com/oXpace/orai/internal/search"
)

// mailIdentity resolves the project and the mailbox handle a command acts as. Inside
// a role session that is the role, bound to its own project; outside one it is the
// user, and only with --as user. With required false, a missing identity outside a
// role is not an error: me comes back "" and the project is located normally.
func mailIdentity(o options, required bool) (*project.Project, string, int, error) {
	role, err := runtime.RoleIdentity()
	if err != nil {
		return nil, "", 1, err
	}
	if role != "" {
		// A role's identity is bound to its own project; never mix in another one.
		p, err := project.Load(os.Getenv("ORAI_PROJECT"))
		if err != nil {
			return nil, "", 1, err
		}
		if explicit := o.get("project"); explicit != "" {
			if located, err := project.Locate(explicit); err != nil || located != p.Root {
				return nil, "", exitUsage, usagef("a role session cannot act on another project")
			}
		}
		if o.has("as") {
			return nil, "", 1, errors.New("an Orai role cannot override its messaging identity")
		}
		return p, role, 0, nil
	}
	if o.has("as") && o.get("as") != config.UserHandle {
		return nil, "", exitUsage, usagef("--as only accepts %q", config.UserHandle)
	}
	if !o.has("as") && required {
		return nil, "", 1, errors.New("outside Orai, use --as user for an authorized Desktop action")
	}
	target := o.get("project")
	if target == "" {
		if target, err = os.Getwd(); err != nil {
			return nil, "", 1, err
		}
	}
	p, err := project.Load(target)
	if err != nil {
		return nil, "", exitUsage, err
	}
	if !o.has("as") {
		return p, "", 0, nil
	}
	return p, config.UserHandle, 0, nil
}

const searchHelp = `usage: orai search <words or a question> [--also <other words>]... [--in docs,mail,code] [--limit N] [--json] [--as user]

Search the project's documents, your own mailbox and the code graph at once, and print
what was found as text: where (path and line, message ID, symbol), the matching lines,
and the folder's declared purpose. Then open what you need with ` + "`orai read`" + `.

  --in LIST     only these sources (comma-separated). Default: every source set up
                here. docs = the shelf folders in orai.toml, mail = your received and
                sent messages (a role session's own, or the user's with --as user),
                code = the code graph of this checkout
  --also TEXT   the same question in other words (repeatable). Documents are searched
                with every phrasing and the rankings fused, so a document several
                phrasings agree on comes first. Worth it when a docs line says
                "weak match"; mail and code use the first phrasing only
  --limit N     results per source (default 5)
  --json        machine-readable output

Each source gets a status line first. Documents use the built-in keyword search
(sections ranked by rare words, title and heading hits first). Where QMD is on and its
server cannot answer, that search stands in and the line says why and what fixes it;
an index that is behind its documents is noted too. Searching changes nothing: it receives no
message (messages not yet received are only counted; ` + "`orai inbox`" + ` receives them),
starts no server and syncs no index.

exit codes: 0 every source answered, 1 a source failed or fell back (results are still
printed), 2 usage error. An empty result does not show the thing is absent.
`

const readHelp = `usage: orai read <ref>... [--json] [--as user]

Print what a search result points at.

  path                  a file in the project, numbered by line (the first 400 lines;
                        give a range for more)
  path:N                80 lines from line N
  path:N-M              lines N to M
  <collection>/<path>   a shelf name, as the MCP server shows it, works too
  <message ID>          a message you already received or sent: header and body.
                        A message not received yet is not shown; ` + "`orai inbox <ID>`" + `
                        receives it (once)

Paths are taken from the current folder first, then from the project folder, and
must stay inside the project. Reading changes nothing.
`

const codeHelp = `usage: orai code init|sync|check [--project PATH]

The code graph lets ` + "`orai search`" + ` and role sessions find symbols, their source and
who calls them. The engine is CodeGraph (installed separately); Orai builds and syncs
the graph for the project folder and for every role worktree, and connects role
sessions to it as the MCP server ` + "`codegraph`" + `, served from the role's own checkout.

  init      build the graph where it is missing (the project folder, role worktrees)
  sync      bring the built graphs up to date with the files
  check     diagnose the code graph, including a symbol lookup

While a role session runs, its codegraph server watches the files and syncs by
itself; ` + "`orai code sync`" + ` is for searches outside sessions.

first time:
  1. npm install -g @colbymchenry/codegraph
  2. [integrations.codegraph] in orai.toml (smoke_symbol = "<a symbol>" lets
     ` + "`orai code check`" + ` and ` + "`orai doctor --deep`" + ` verify lookups)
  3. orai code init      (` + "`orai setup`" + ` does this too)

Orai never runs ` + "`codegraph install`" + ` or changes global agent settings.
`

// searchLimitDefault is how many results each source returns unless --limit says.
const searchLimitDefault = 5

func runSearch(argv []string, stdout io.Writer) (int, error) {
	o, err := parse(argv, map[string]bool{"project": true, "as": true, "in": true, "limit": true, "also": true, "json": false})
	if err != nil {
		return exitUsage, err
	}
	var also []string
	for _, phrasing := range o.lists["also"] {
		if phrasing = strings.TrimSpace(phrasing); phrasing != "" {
			also = append(also, phrasing)
		}
	}
	query := strings.TrimSpace(strings.Join(o.args, " "))
	if query == "" {
		return exitUsage, usagef("usage: orai search <words or a question> (`orai search --help`)")
	}
	limit, hasLimit, err := o.intValue("limit")
	if err != nil {
		return exitUsage, err
	}
	if !hasLimit {
		limit = searchLimitDefault
	}
	if limit < 1 {
		return exitUsage, usagef("--limit must be at least 1")
	}
	var sources map[string]bool
	if o.has("in") {
		sources = map[string]bool{}
		for _, name := range strings.Split(o.get("in"), ",") {
			name = strings.TrimSpace(name)
			valid := false
			for _, known := range search.Sources {
				valid = valid || known == name
			}
			if !valid {
				return exitUsage, usagef("--in takes %s", strings.Join(search.Sources, ", "))
			}
			sources[name] = true
		}
	}
	p, me, code, err := mailIdentity(o, false)
	if err != nil {
		return code, err
	}
	cwd, _ := os.Getwd()
	opts := search.Options{Query: query, Also: also, Sources: sources, Limit: limit, Cwd: cwd}
	if me != "" {
		opts.Mailbox = &search.Mailbox{Root: mail.Root(p.MailRoot()), Me: me}
	}
	result := search.Run(p, opts)
	status := 0
	if result.Problem() {
		status = 1
	}
	if o.flag("json") {
		return status, printJSON(stdout, result)
	}
	search.Render(stdout, result)
	return status, nil
}

// readMaxLines is how much of a file `orai read <path>` prints without a range.
const readMaxLines = 400

func runRead(argv []string, stdout io.Writer) (int, error) {
	o, err := parse(argv, map[string]bool{"project": true, "as": true, "json": false})
	if err != nil {
		return exitUsage, err
	}
	if len(o.args) == 0 {
		return exitUsage, usagef("usage: orai read <path[:N[-M]] | message ID>... (`orai read --help`)")
	}
	p, me, code, err := mailIdentity(o, false)
	if err != nil {
		return code, err
	}
	cwd, _ := os.Getwd()
	var shown []any
	for i, ref := range o.args {
		if !o.flag("json") && i > 0 {
			fmt.Fprintln(stdout)
		}
		file, from, to, ok := locate(p, cwd, ref)
		if ok {
			text, err := numbered(file, from, to)
			if err != nil {
				return 1, err
			}
			rel, _ := filepath.Rel(p.Root, file)
			if o.flag("json") {
				shown = append(shown, map[string]any{"path": filepath.ToSlash(rel), "from": text.from, "to": text.to,
					"total": text.total, "text": text.body})
				continue
			}
			fmt.Fprintf(stdout, "== %s (lines %d-%d of %d) ==\n%s", filepath.ToSlash(rel), text.from, text.to, text.total, text.body)
			if text.to < text.total {
				fmt.Fprintf(stdout, "… %d more lines: orai read %s:%d-%d\n", text.total-text.to, filepath.ToSlash(rel), text.to+1, min(text.total, text.to+readMaxLines))
			}
			continue
		}
		if me == "" {
			return 1, fmt.Errorf("%s: no such file in the project. To read a message outside a role session, add --as user", ref)
		}
		message, err := mail.Root(p.MailRoot()).Show(me, ref)
		switch {
		case errors.Is(err, mail.ErrNotReceived):
			return 1, fmt.Errorf("%s has not been received yet; `orai inbox %s` receives it (once) and prints it", ref, ref)
		case errors.Is(err, mail.ErrNotFound):
			return 1, fmt.Errorf("%s: no such file in the project and no such message in %s's mailbox", ref, me)
		case err != nil:
			return 1, err
		}
		if o.flag("json") {
			shown = append(shown, message)
			continue
		}
		h := message.Header
		fmt.Fprintf(stdout, "== message %s (%s) ==\nfrom: %s\nto: %s\nthread: %s\ncreated: %s\n", h.ID, message.Box, h.From,
			strings.Join(h.To, ", "), h.Thread, h.Created)
		if h.Kind != "" {
			fmt.Fprintf(stdout, "kind: %s\n", h.Kind)
		}
		if h.Subject != "" {
			fmt.Fprintf(stdout, "subject: %s\n", h.Subject)
		}
		fmt.Fprintf(stdout, "\n%s", message.Body)
		if !strings.HasSuffix(message.Body, "\n") {
			fmt.Fprintln(stdout)
		}
	}
	if o.flag("json") {
		return 0, printJSON(stdout, shown)
	}
	return 0, nil
}

// locate resolves a read reference to a regular file inside the project, with its
// requested line range (0 = unspecified). ok is false when ref names no such file.
func locate(p *project.Project, cwd, ref string) (string, int, int, bool) {
	path, from, to := ref, 0, 0
	if i := strings.LastIndex(ref, ":"); i > 0 {
		if a, b, ok := lineRange(ref[i+1:]); ok {
			path, from, to = ref[:i], a, b
		}
	}
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		candidates = []string{filepath.Join(cwd, path), filepath.Join(p.Root, path)}
		if p.Config.Shelf != nil {
			name, rest, _ := strings.Cut(filepath.ToSlash(path), "/")
			if folder, ok := p.Config.Shelf.Collections[name]; ok {
				candidates = append(candidates, filepath.Join(p.Root, folder, filepath.FromSlash(rest)))
			}
		}
	}
	root := p.Root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if info, err := os.Stat(resolved); err == nil && info.Mode().IsRegular() {
			return resolved, from, to, true
		}
	}
	return "", 0, 0, false
}

// lineRange parses "N" or "N-M" (1-based); "N" means 80 lines from N.
func lineRange(text string) (int, int, bool) {
	a, b, isRange := strings.Cut(text, "-")
	from, err := strconv.Atoi(a)
	if err != nil || from < 1 {
		return 0, 0, false
	}
	if !isRange {
		return from, from + 79, true
	}
	to, err := strconv.Atoi(b)
	if err != nil || to < from {
		return 0, 0, false
	}
	return from, to, true
}

type numberedText struct {
	from, to, total int
	body            string
}

// numbered returns lines from..to of file (1-based, clamped), each prefixed with its
// number. from = 0 means the start, up to readMaxLines.
func numbered(file string, from, to int) (numberedText, error) {
	handle, err := os.Open(file)
	if err != nil {
		return numberedText{}, err
	}
	defer handle.Close()
	if from == 0 {
		from, to = 1, readMaxLines
	}
	var b strings.Builder
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	n, last := 0, 0
	for scanner.Scan() {
		n++
		if n >= from && n <= to {
			fmt.Fprintf(&b, "%6d  %s\n", n, scanner.Text())
			last = n
		}
	}
	if err := scanner.Err(); err != nil {
		return numberedText{}, err
	}
	if last == 0 {
		from = min(from, n)
	}
	return numberedText{from: from, to: last, total: n, body: b.String()}, nil
}

func runCode(argv []string, stdout io.Writer) (int, error) {
	return withProject(argv, nil, func(p *project.Project, o options) (int, error) {
		if len(o.args) != 1 || (o.args[0] != "init" && o.args[0] != "sync" && o.args[0] != "check") {
			return exitUsage, usagef("usage: orai code init|sync|check (`orai code --help` explains each)")
		}
		return 0, codegraph.Lifecycle(p, o.args[0], stdout)
	})
}
