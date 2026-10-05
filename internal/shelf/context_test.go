// Covers the folder contexts: orai.toml declares them, the index is made to match, and
// doctor says when it does not. No test here runs a real qmd.
package shelf

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
)

// listedContexts is `qmd context list` as QMD 2.8.3 prints it: an index-wide context
// under "*", then each collection with its paths and their texts.
const listedContexts = `
Configured Contexts

*
  /
    set by hand for every collection
docs
  adr
    an older text
  / (root)
    the whole project
  notes/
    added by hand
`

func contextProject(t *testing.T, tables string) (*Settings, *[][]string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	s := NewSettings(writeProject(t, root, "schema = 2\n[integrations.shelf]\n"+tables))
	for _, folder := range s.Collections {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{s.ConfigFile, s.DB} {
		if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return s, installSuccessMocks(t, true, nil, "/usr/bin/qmd")
}

// answerContextList makes the mocked engine report the given contexts.
func answerContextList(t *testing.T, calls *[][]string, listed string) {
	t.Helper()
	indexed := runCommand
	runCommand = func(out io.Writer, argv []string, s *Settings, capture bool) (string, error) {
		if capture && strings.HasSuffix(strings.Join(argv, " "), "context list") {
			*calls = append(*calls, append([]string(nil), argv...))
			return listed, nil
		}
		return indexed(out, argv, s, capture)
	}
}

func TestContextListIsReadByCollectionAndPath(t *testing.T) {
	want := map[contextKey]string{
		{"*", ""}:          "set by hand for every collection",
		{"docs", "adr"}:    "an older text",
		{"docs", ""}:       "the whole project",
		{"docs", "notes/"}: "added by hand",
	}
	if got := parseContexts(listedContexts); !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed %v, want %v", got, want)
	}
	if got := parseContexts("No contexts configured. Use 'qmd context add' to add one.\n"); len(got) != 0 {
		t.Fatalf("an empty list parsed as %v", got)
	}
	for key, uri := range map[contextKey]string{{"*", ""}: "/", {"docs", ""}: "qmd://docs/", {"docs", "adr"}: "qmd://docs/adr"} {
		if key.uri() != uri {
			t.Fatalf("%v addresses %q, want %q", key, key.uri(), uri)
		}
	}
}

// The declaration is the source: recover adds what is missing, replaces a text that
// changed, leaves a matching one alone, and removes what orai.toml does not declare
// after printing it. The server is running throughout and is not stopped.
func TestRecoverMakesTheIndexHoldTheDeclaredContexts(t *testing.T) {
	s, calls := contextProject(t, "[integrations.shelf.context]\n\".\" = \"the whole project\"\n\"docs/adr\" = \"why each decision was made\"\n\"docs/product\" = \"what each screen does\"\n")
	answerContextList(t, calls, listedContexts)
	var out bytes.Buffer
	if err := Lifecycle(s.Project, "recover", &out); err != nil {
		t.Fatalf("recover: %v", err)
	}
	lines := joined(calls)
	for _, want := range []string{
		"context add qmd://docs/adr why each decision was made",
		"context add qmd://docs/product what each screen does",
		"context rm qmd://docs/notes/",
		"context rm /",
	} {
		if !containsStr(lines, want) {
			t.Fatalf("missing %q in %v", want, lines)
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "context add qmd://docs/ ") || line == "mcp stop" || line == "update" || line == "embed" {
			t.Fatalf("recover ran %q: %v", line, lines)
		}
	}
	if !strings.Contains(out.String(), "It read: added by hand") {
		t.Fatalf("a removed context was not printed:\n%s", out.String())
	}
}

// Without the table Orai does not look at contexts at all: ones added by hand stay.
// An empty table is a declaration of none.
func TestContextsAreLeftAloneUntilTheyAreDeclared(t *testing.T) {
	s, calls := contextProject(t, "")
	answerContextList(t, calls, listedContexts)
	if err := Lifecycle(s.Project, "recover", io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, line := range joined(calls) {
		if strings.HasPrefix(line, "context ") {
			t.Fatalf("undeclared contexts were touched: %q", line)
		}
	}
	if _, declared := contextCheck(s); declared {
		t.Fatal("doctor reports contexts a project never declared")
	}

	s, calls = contextProject(t, "[integrations.shelf.context]\n")
	answerContextList(t, calls, "\nConfigured Contexts\n\ndocs\n  adr\n    added by hand\n")
	if err := Lifecycle(s.Project, "recover", io.Discard); err != nil {
		t.Fatal(err)
	}
	if lines := joined(calls); !containsStr(lines, "context rm qmd://docs/adr") {
		t.Fatalf("an empty table kept a context: %v", lines)
	}
}

// A context belongs to a folder. Split the folder into collections and each one gets
// what applies to it: the folder's own text on its root, joined after what an outer
// folder says.
func TestContextsFollowTheFolderWhenCollectionsAreSplit(t *testing.T) {
	s, _ := contextProject(t, "collections = { core = { path = \"docs\", pattern = \"*.md\" }, adr = \"docs/adr\" }\n"+
		"[integrations.shelf.context]\n\"docs\" = \"project documents\"\n\"docs/adr\" = \"decision records\"\n\"docs/adr/2024\" = \"decided in 2024\"\n")
	want := map[contextKey]string{
		{"core", ""}:    "project documents",
		{"core", "adr"}: "decision records",
		{"adr", ""}:     "project documents / decision records",
		{"adr", "2024"}: "decided in 2024",
		// The engine matches by path, so the nested folder is described in core as well.
		{"core", "adr/2024"}: "decided in 2024",
	}
	if got, _ := s.declaredContexts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("declared %v, want %v", got, want)
	}
}

func TestDoctorComparesDeclaredContextsWithTheIndex(t *testing.T) {
	s, _ := contextProject(t, "[integrations.shelf.context]\n\"docs\" = \"the whole project\"\n\"docs/adr\" = \"why each decision was made\"\n")
	saved := listContexts
	t.Cleanup(func() { listContexts = saved })

	listContexts = func(*Settings) (string, error) { return listedContexts, nil }
	check, declared := contextCheck(s)
	if !declared || check.Component != "shelf.context" || check.Status != doctor.Degraded ||
		!strings.Contains(check.NextAction, "orai shelf recover") {
		t.Fatalf("difference: %+v", check)
	}
	for _, want := range []string{"qmd://docs/adr has an older text", "qmd://docs/notes/ is not declared", "/ is not declared"} {
		if !strings.Contains(check.Reason, want) {
			t.Fatalf("reason lacks %q: %s", want, check.Reason)
		}
	}
	if strings.Contains(check.Reason, "qmd://docs/ ") {
		t.Fatalf("a matching context is reported: %s", check.Reason)
	}

	listContexts = func(*Settings) (string, error) {
		return "\nConfigured Contexts\n\ndocs\n  / (root)\n    the whole project\n  adr\n    why each decision was made\n", nil
	}
	if check, _ := contextCheck(s); check.Status != doctor.Healthy || !strings.Contains(check.Reason, "2 declared context(s)") {
		t.Fatalf("matching: %+v", check)
	}

	// Not being able to ask is not the same as the contexts being absent.
	listContexts = func(*Settings) (string, error) { return "", errors.New("unable to open database file") }
	if check, _ := contextCheck(s); check.Status != doctor.NotChecked || !strings.Contains(check.Reason, "does not show they are missing") {
		t.Fatalf("unreadable: %+v", check)
	}
}

// probe carries the check, so `orai doctor` and `orai shelf check` both show it.
func TestProbeReportsContexts(t *testing.T) {
	withOpenPort(t)
	s, _ := contextProject(t, "port = 19223\n[integrations.shelf.context]\n\"docs\" = \"the whole project\"\n")
	saved := listContexts
	t.Cleanup(func() { listContexts = saved })
	listContexts = func(*Settings) (string, error) { return "No contexts configured.\n", nil }
	setClient(t, &fakeClient{status: func() map[string]any { return statusResult(s, 5, true, 0) }})
	check, ok := byComponent(probe(s, false))["shelf.context"]
	if !ok || check.Status != doctor.Degraded || !strings.Contains(check.Reason, "qmd://docs/ is missing") {
		t.Fatalf("probe: %+v", check)
	}
}

// Re-registering a collection drops the contexts it had, so contexts are applied after
// the collections are settled and before anything is indexed.
func TestRefreshAppliesContextsAfterTheCollections(t *testing.T) {
	s, calls := contextProject(t, "collections = { adr = \"docs/adr\" }\n[integrations.shelf.context]\n\"docs/adr\" = \"decision records\"\n")
	ownServerRunning = func(*Settings) (bool, error) { return false, nil }
	indexed := runCommand
	runCommand = func(out io.Writer, argv []string, s *Settings, capture bool) (string, error) {
		line := strings.Join(argv, " ")
		switch {
		case capture && strings.HasSuffix(line, "collection show adr"):
			*calls = append(*calls, append([]string(nil), argv...))
			return "", errors.New("collection not found")
		case capture && strings.HasSuffix(line, "context list"):
			*calls = append(*calls, append([]string(nil), argv...))
			return "No contexts configured.\n", nil
		}
		return indexed(out, argv, s, capture)
	}
	if err := Lifecycle(s.Project, "refresh", io.Discard); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	lines := joined(calls)
	add := indexOf(lines, "collection add "+s.Collections["adr"]+" --name adr --mask **/*.md")
	described, update := indexOf(lines, "context add qmd://adr/ decision records"), indexOf(lines, "update")
	if add < 0 || described < add || update < described {
		t.Fatalf("order: %v", lines)
	}
}
