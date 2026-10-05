package shelf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/oXpace/orai/internal/doctor"
)

// A context is one line saying what the documents under a folder are for. The engine
// attaches it to every search result from that folder, so a reader can tell a decision
// record from a product spec before opening either. It does not change which documents
// are found or how they rank.
//
// orai.toml declares contexts by folder ([integrations.shelf.context]); the engine keeps
// them per collection, in the index's own config under .orai/shelf, which a new checkout
// does not have. So the declaration is the source and the index is made to match it:
// init, refresh and recover all apply it, because unlike a collection change it needs no
// re-indexing and a running server reads it at the next search. A project that declares
// none is left alone, whatever its index holds.

// contextKey is where the engine keeps one context: a collection and a path inside it
// ("" for the collection's root). The collection "*" is the engine's index-wide context,
// which Orai never writes but removes when contexts are declared.
type contextKey struct{ Collection, Path string }

func (k contextKey) uri() string {
	if k.Collection == "*" {
		return "/"
	}
	return "qmd://" + k.Collection + "/" + k.Path
}

// declaredContexts is what orai.toml asks the index to hold. ok is false when the
// project declares no context table at all.
func (s *Settings) declaredContexts() (want map[contextKey]string, ok bool) {
	shelf := s.Project.Config.Shelf
	if shelf.Context == nil {
		return nil, false
	}
	want = map[contextKey]string{}
	for collection, paths := range shelf.ContextPlan() {
		for path, text := range paths {
			want[contextKey{collection, path}] = text
		}
	}
	return want, true
}

// parseContexts reads `qmd context list`: a collection name on its own line, then for
// each context its path indented by two spaces ("/ (root)" for the collection's root)
// and its text indented by four.
func parseContexts(listed string) map[contextKey]string {
	have := map[contextKey]string{}
	collection, path, pending := "", "", false
	for _, line := range strings.Split(listed, "\n") {
		switch {
		case strings.TrimSpace(line) == "" || strings.HasPrefix(line, "Configured Contexts") || strings.HasPrefix(line, "No contexts configured"):
		case strings.HasPrefix(line, "    "):
			if pending {
				have[contextKey{collection, path}] = strings.TrimSpace(line)
				pending = false
			}
		case strings.HasPrefix(line, "  "):
			path, pending = strings.TrimPrefix(line, "  "), collection != ""
			if path == "/ (root)" || (collection == "*" && path == "/") {
				path = ""
			}
		default:
			collection, pending = strings.TrimSpace(line), false
		}
	}
	return have
}

func sortedContextKeys(m map[contextKey]string) []contextKey {
	keys := make([]contextKey, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Collection != keys[j].Collection {
			return keys[i].Collection < keys[j].Collection
		}
		return keys[i].Path < keys[j].Path
	})
	return keys
}

// contextDifference names what separates the index from orai.toml, in a stable order.
func contextDifference(want, have map[contextKey]string) []string {
	var diff []string
	for _, key := range sortedContextKeys(want) {
		if text, present := have[key]; !present {
			diff = append(diff, key.uri()+" is missing")
		} else if text != want[key] {
			diff = append(diff, key.uri()+" has an older text")
		}
	}
	for _, key := range sortedContextKeys(have) {
		if _, declared := want[key]; !declared {
			diff = append(diff, key.uri()+" is not declared")
		}
	}
	return diff
}

// applyContexts makes the index hold exactly the declared contexts. It runs after the
// collections are settled: re-registering a collection drops the contexts it had.
func applyContexts(s *Settings, qmd string, out io.Writer) error {
	want, declared := s.declaredContexts()
	if !declared {
		return nil
	}
	listed, err := runCommand(out, s.Argv(qmd, "context", "list"), s, true)
	if err != nil {
		return err
	}
	have := parseContexts(listed)
	for _, key := range sortedContextKeys(want) {
		if text, present := have[key]; present && text == want[key] {
			continue
		}
		if _, err := runCommand(out, s.Argv(qmd, "context", "add", key.uri(), want[key]), s, false); err != nil {
			return err
		}
	}
	for _, key := range sortedContextKeys(have) {
		if _, kept := want[key]; kept {
			continue
		}
		// The text exists nowhere else once removed, so it is printed first.
		fmt.Fprintf(out, "Removing the context of %s from the index: it is not declared. It read: %s\n", key.uri(), have[key])
		if _, err := runCommand(out, s.Argv(qmd, "context", "rm", key.uri()), s, false); err != nil {
			return err
		}
	}
	return nil
}

// listContexts asks the engine what contexts the index holds, quietly (doctor prints
// nothing of its own). The server does not report them, so this runs the CLI. It is a
// var so tests never spawn a real qmd.
var listContexts = func(s *Settings) (string, error) {
	qmd, err := lookPath("qmd")
	if err != nil {
		return "", err
	}
	argv := s.Argv(qmd, "context", "list")
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = s.Project.Root
	cmd.Env = s.Env()
	// Only stdout is the list: a warning on stderr must not be read as a context.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := cmd.Output()
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return "", fmt.Errorf("%w: %s", r.err, strings.TrimSpace(stderr.String()))
		}
		return string(r.out), nil
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		return "", errors.New("`qmd context list` did not answer within 20 seconds")
	}
}

// contextCheck compares the declared contexts with the index. A project that declares
// none gets no check: having no contexts is not a fault.
func contextCheck(s *Settings) (doctor.Check, bool) {
	const component = "shelf.context"
	want, declared := s.declaredContexts()
	if !declared {
		return doctor.Check{}, false
	}
	listed, err := listContexts(s)
	if err != nil {
		return doctor.New(component, doctor.NotChecked,
			fmt.Sprintf("could not read the contexts from the index (%v); this does not show they are missing", err),
			"Run `orai doctor` where .orai/shelf is writable (outside the sandbox)"), true
	}
	if diff := contextDifference(want, parseContexts(listed)); len(diff) > 0 {
		return doctor.New(component, doctor.Degraded,
			"the contexts in the index differ from [integrations.shelf.context]: "+strings.Join(diff, "; "),
			"`orai shelf recover` applies them (nothing is re-indexed and a running server keeps running)"), true
	}
	return doctor.New(component, doctor.Healthy,
		fmt.Sprintf("the %d declared context(s) are applied to the index", len(s.Project.Config.Shelf.Context)), ""), true
}
