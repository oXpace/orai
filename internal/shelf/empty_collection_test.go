package shelf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
)

func twoCollections(t *testing.T) *Settings {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	p := writeProject(t, root, "schema = 2\n\n[integrations.shelf]\nport = 19222\ncollections = { docs = \"docs\", specs = \"openspec/specs\" }\n")
	for _, dir := range []string{"docs", "openspec/specs"} {
		if err := os.MkdirAll(filepath.Join(p.Root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return NewSettings(p)
}

func served(rows ...string) map[string]any {
	var list []any
	for i := 0; i < len(rows); i += 2 {
		list = append(list, map[string]any{"name": rows[i], "path": rows[i+1]})
	}
	return map[string]any{"collections": list}
}

// QMD 2.8.3 leaves collections without documents out of `status`.
func TestIdentityAcceptsACollectionWithoutDocuments(t *testing.T) {
	s := twoCollections(t)
	if got := identity(served("docs", s.Collections["docs"]), s); got != "" {
		t.Fatal(got)
	}
}

// The first document must not lock out the sync that would index it.
func TestIdentityAcceptsTheFirstDocumentBeforeItIsSynced(t *testing.T) {
	s := twoCollections(t)
	if err := os.WriteFile(filepath.Join(s.Collections["specs"], "spec.md"), []byte("# S\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := identity(served("docs", s.Collections["docs"]), s); got != "" {
		t.Fatal(got)
	}
}

func TestIdentityStillRefusesAnotherIndex(t *testing.T) {
	s := twoCollections(t)
	for name, status := range map[string]map[string]any{
		"nothing served":       served(),
		"same name, elsewhere": served("docs", "/somewhere/else"),
		"other names":          served("wiki", s.Collections["docs"]),
		"one right, one wrong": served("docs", s.Collections["docs"], "specs", "/somewhere/else"),
	} {
		if identity(status, s) == "" {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestProbeReportsDocumentsMissingFromTheIndex(t *testing.T) {
	withOpenPort(t)
	s := twoCollections(t)
	if err := os.WriteFile(filepath.Join(s.Collections["specs"], "spec.md"), []byte("# S\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := served("docs", s.Collections["docs"])
	status["totalDocuments"], status["hasVectorIndex"] = 1, true
	setClient(t, &fakeClient{status: func() map[string]any { return map[string]any{"structuredContent": status} }})
	if got := byComponent(probe(s, false))["shelf.collections"].Status; got != doctor.Degraded {
		t.Fatalf("shelf.collections = %q", got)
	}
}
