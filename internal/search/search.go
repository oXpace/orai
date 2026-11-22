// Package search is `orai search`: one query over the project's documents (the shelf),
// the caller's own mailbox and the code graph, printed as text any provider hands its
// model whole. Each source is searched on its own terms and reported under its own
// status line; scores from different sources are never mixed into one ranking.
//
// Searching only reads. It receives no message, starts no server and syncs no index:
// a source that cannot answer, or answers from a stale index, says so and names the
// command that fixes it.
package search

import (
	"fmt"
	"io"
	"strings"

	"github.com/oXpace/orai/internal/codegraph"
	"github.com/oXpace/orai/internal/lexical"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/shelf"
)

// Sources, in the order results are printed.
var Sources = []string{"docs", "mail", "code"}

// Mailbox is whose mail a search may read: a role's own, or the user's with --as user.
type Mailbox struct {
	Root mail.Root
	Me   string
}

// Options say what to search.
type Options struct {
	Query   string
	Also    []string        // other phrasings of the same question (documents only)
	Sources map[string]bool // requested explicitly; nil = every source that is set up
	Limit   int             // per source
	Mailbox *Mailbox        // nil outside a role session without --as user
	Cwd     string          // picks the code graph checkout
}

// Source is one source's outcome: State is "ok", "fallback" (answered another way,
// see Reason), "failed", or "skipped" (not set up, or not available here).
type Source struct {
	Name   string   `json:"name"`
	State  string   `json:"state"`
	Detail string   `json:"detail,omitempty"`
	Reason string   `json:"reason,omitempty"`
	Fix    string   `json:"fix,omitempty"`
	Notes  []string `json:"notes,omitempty"`
}

// Result is a whole search.
type Result struct {
	Query   string          `json:"query"`
	Sources []Source        `json:"sources"`
	Docs    []shelf.Hit     `json:"docs,omitempty"`
	Mail    []mail.Found    `json:"mail,omitempty"`
	Code    []codegraph.Hit `json:"code,omitempty"`
	Pending int             `json:"pending_mail,omitempty"`
}

// Problem reports whether a source that was asked for (or is set up) could not answer
// fully. The caller turns it into exit status 1 while still printing the results.
func (r Result) Problem() bool {
	for _, s := range r.Sources {
		if s.State == "failed" || s.State == "fallback" {
			return true
		}
	}
	return false
}

// Run searches every selected source.
func Run(p *project.Project, o Options) Result {
	r := Result{Query: o.Query}
	want := func(name string) bool { return o.Sources == nil || o.Sources[name] }
	explicit := func(name string) bool { return o.Sources != nil && o.Sources[name] }
	if want("docs") {
		r.Sources = append(r.Sources, docs(p, o, explicit("docs"), &r))
	}
	if want("mail") {
		r.Sources = append(r.Sources, mailbox(o, explicit("mail"), &r))
	}
	if want("code") {
		r.Sources = append(r.Sources, code(p, o, explicit("code"), &r))
	}
	return r
}

// skipped is the outcome of a source that is not set up: a quiet skip when it was not
// asked for, a failure when it was.
func skipped(name, reason, fix string, asked bool) Source {
	if asked {
		return Source{Name: name, State: "failed", Reason: reason, Fix: fix}
	}
	return Source{Name: name, State: "skipped", Reason: reason, Fix: fix}
}

func docs(p *project.Project, o Options, asked bool, r *Result) Source {
	if p.Config.Shelf == nil {
		return skipped("docs", "no [integrations.shelf] in orai.toml", "", asked)
	}
	s := shelf.NewSettings(p)
	out := shelf.Search(s, o.Query, o.Limit, o.Also...)
	r.Docs = out.Hits
	src := Source{Name: "docs", State: "ok", Detail: fmt.Sprintf("keyword + meaning (shelf :%d)", out.Port)}
	switch out.Mode {
	case "builtin":
		src.Detail = shelf.BuiltIn
	case "keyword":
		src.State, src.Detail, src.Reason, src.Fix = "fallback", "keyword scan of the files", out.Reason, out.Fix
	}
	if out.Mode != "engine" && out.Weak {
		note := "weak match: the top result covers few of the words"
		if len(out.Hits) == 0 {
			note = "weak match: nothing matched"
		}
		if len(out.Unknown) > 0 {
			note += "; no document contains " + strings.Join(first(out.Unknown, 5), ", ")
		}
		src.Notes = append(src.Notes, note+". Ask again with --also '<the same question in other words>'")
	}
	if n := len(out.Stale); n > 0 {
		src.Notes = append(src.Notes, fmt.Sprintf("%d document(s) changed since the last sync, so the index may be behind (%s): orai shelf sync",
			n, strings.Join(first(out.Stale, 3), ", ")))
	}
	return src
}

func mailbox(o Options, asked bool, r *Result) Source {
	if o.Mailbox == nil {
		return skipped("mail", "outside a role session no mailbox is read", "add --as user to search the user's mailbox", asked)
	}
	found, err := o.Mailbox.Root.Search(o.Mailbox.Me, o.Query, o.Limit)
	if err != nil {
		return Source{Name: "mail", State: "failed", Reason: err.Error(), Fix: "orai doctor"}
	}
	r.Mail, r.Pending = found.Found, found.Pending
	src := Source{Name: "mail", State: "ok", Detail: fmt.Sprintf("%s mailbox, %d messages, keyword", o.Mailbox.Me, found.Scanned)}
	if found.Pending > 0 {
		src.Notes = append(src.Notes, fmt.Sprintf("%d message(s) not received yet also match; their bodies are not shown: orai inbox", found.Pending))
	}
	return src
}

func code(p *project.Project, o Options, asked bool, r *Result) Source {
	if p.Config.Codegraph == nil {
		return skipped("code", "no [integrations.codegraph] in orai.toml", "", asked)
	}
	folder := codegraph.FolderFor(p, o.Cwd)
	out := codegraph.Search(p, folder, o.Query, o.Limit)
	r.Code = out.Hits
	if out.Reason != "" {
		return Source{Name: "code", State: "failed", Reason: out.Reason, Fix: out.Fix}
	}
	src := Source{Name: "code", State: "ok", Detail: "symbols (codegraph " + relative(p, folder) + ")"}
	if out.Pending > 0 {
		src.Notes = append(src.Notes, fmt.Sprintf("%d file(s) changed since the graph was synced: orai code sync", out.Pending))
	}
	return src
}

func relative(p *project.Project, dir string) string {
	if dir == p.Root {
		return "."
	}
	if rel, ok := strings.CutPrefix(dir, p.Root+"/"); ok {
		return rel
	}
	return dir
}

func first(items []string, n int) []string {
	if len(items) > n {
		return append(append([]string{}, items[:n]...), "…")
	}
	return items
}

// Render prints a result as text: one status line per source, then the hits grouped
// by source, then the commands that open them.
func Render(w io.Writer, r Result) {
	for _, s := range r.Sources {
		line := s.Name + ": "
		switch s.State {
		case "ok":
			line += s.Detail + fmt.Sprintf(" · %d hit(s)", r.count(s.Name))
		case "fallback":
			line += fmt.Sprintf("%s · %d hit(s) — the shelf could not answer: %s. Fix: %s", s.Detail, r.count(s.Name), s.Reason, s.Fix)
		case "failed":
			line += "FAILED — " + s.Reason
			if s.Fix != "" {
				line += ". Fix: " + s.Fix
			}
		default:
			line += "not searched (" + s.Reason
			if s.Fix != "" {
				line += "; " + s.Fix
			}
			line += ")"
		}
		fmt.Fprintln(w, line)
		for _, note := range s.Notes {
			fmt.Fprintln(w, "  note: "+note)
		}
	}
	var next []string
	lastContext := ""
	for _, h := range r.Docs {
		ref := h.Path
		if h.Line > 0 {
			ref = fmt.Sprintf("%s:%d", h.Path, h.Line)
		}
		head := "\n[docs] " + ref
		if h.Title != "" {
			head += "  " + h.Title
		}
		if h.Score > 0 {
			head += fmt.Sprintf("  (score %.2f)", h.Score)
		}
		fmt.Fprintln(w, head)
		if h.Context != "" && h.Context != lastContext {
			fmt.Fprintln(w, "  context: "+h.Context)
		} else if h.Context != "" {
			fmt.Fprintln(w, "  context: (same as above)")
		}
		lastContext = h.Context
		excerpt(w, h.Excerpt)
		next = append(next, ref)
	}
	for _, m := range r.Mail {
		fmt.Fprintf(w, "\n[mail] %s  %s → %s  (%s, thread %s)\n", m.ID, m.From, strings.Join(m.To, ","), m.Box, m.Thread)
		if m.Subject != "" {
			fmt.Fprintln(w, "  subject: "+m.Subject)
		}
		excerpt(w, m.Excerpt)
		next = append(next, m.ID)
	}
	for _, c := range r.Code {
		ref := fmt.Sprintf("%s:%d", c.Path, c.Line)
		if c.EndLine > c.Line {
			ref = fmt.Sprintf("%s:%d-%d", c.Path, c.Line, c.EndLine)
		}
		fmt.Fprintf(w, "\n[code] %s %s  %s\n", c.Kind, c.Name, ref)
		if c.Signature != "" {
			fmt.Fprintln(w, "  "+c.Name+c.Signature)
		}
		if c.Doc != "" {
			fmt.Fprintln(w, "  "+lexical.Trim(strings.Join(strings.Fields(c.Doc), " "), 200))
		}
		next = append(next, ref)
	}
	if len(next) == 0 {
		fmt.Fprintln(w, "\nNo hits. An empty result does not show the thing is absent: try other words, or `rg` over the files.")
		return
	}
	if len(next) > 4 {
		next = next[:4]
	}
	fmt.Fprintln(w, "\nopen: orai read "+strings.Join(next, " · orai read "))
}

func excerpt(w io.Writer, text string) {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(w, "  │ "+line)
		}
	}
}

func (r Result) count(source string) int {
	switch source {
	case "docs":
		return len(r.Docs)
	case "mail":
		return len(r.Mail)
	default:
		return len(r.Code)
	}
}
