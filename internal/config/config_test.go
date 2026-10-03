// Covers config.Parse's schema, roles, and integrations validation.
package config_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/config"
)

const base = "schema = 2\nsession = \"orai\"\n"

func parse(t *testing.T, text string) (*config.Config, error) {
	t.Helper()
	return config.Parse([]byte(text))
}

func mustParse(t *testing.T, text string) *config.Config {
	t.Helper()
	cfg, err := parse(t, text)
	if err != nil {
		t.Fatalf("parse(%q): unexpected error: %v", text, err)
	}
	return cfg
}

func mustFail(t *testing.T, text string) error {
	t.Helper()
	cfg, err := parse(t, text)
	if err == nil {
		t.Fatalf("parse(%q): expected error, got config %+v", text, cfg)
	}
	return err
}

func contains(t *testing.T, err error, substr string) {
	t.Helper()
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("error %q does not contain %q", err.Error(), substr)
	}
}

// --- config: roles -----------------------------------------------------------------

func TestArbitraryRoleNamesAndCounts(t *testing.T) {
	cfg := mustParse(t, base+`
[roles.lead]
provider = "codex"

[roles."reviewer-2"]
provider = "claude"

[roles.qa]
provider = "codex"
`)
	names := map[string]bool{}
	for name := range cfg.Roles {
		names[name] = true
	}
	want := map[string]bool{"lead": true, "reviewer-2": true, "qa": true}
	if len(names) != len(want) {
		t.Fatalf("roles = %v, want %v", names, want)
	}
	for name := range want {
		if !names[name] {
			t.Fatalf("roles missing %q: %v", name, names)
		}
	}
	handles := cfg.Handles()
	wantHandles := []string{"lead", "qa", "reviewer-2", "user"}
	if len(handles) != len(wantHandles) {
		t.Fatalf("handles = %v, want %v", handles, wantHandles)
	}
	for i, h := range wantHandles {
		if handles[i] != h {
			t.Fatalf("handles = %v, want %v", handles, wantHandles)
		}
	}
}

func TestReservedRoleNamesRejected(t *testing.T) {
	for _, name := range []string{"init", "run", "user", "status", "all", "orai", "doctor", "qmd", "setup", "msg", "shelf"} {
		t.Run(name, func(t *testing.T) {
			err := mustFail(t, base+"\n[roles."+name+"]\nprovider = \"codex\"\n")
			contains(t, err, "reserved")
		})
	}
}

func TestBadRegexRoleNamesRejected(t *testing.T) {
	for _, name := range []string{"Reviewer", "1abc", "role_name", strings.Repeat("a", 32)} {
		t.Run(name, func(t *testing.T) {
			err := mustFail(t, base+"\n[roles.\""+name+"\"]\nprovider = \"codex\"\n")
			contains(t, err, "role names must match")
		})
	}
}

func TestUnknownTopLevelKeyRejected(t *testing.T) {
	err := mustFail(t, base+"\nfoo = 1\n")
	contains(t, err, "unknown key(s) foo")
}

func TestUnknownRoleKeyRejected(t *testing.T) {
	err := mustFail(t, base+`
[roles.lead]
provider = "codex"
nickname = "boss"
`)
	contains(t, err, "unknown key(s) nickname")
}

func TestUnknownIntegrationsKeyRejected(t *testing.T) {
	err := mustFail(t, base+"\n[integrations.unknown]\n")
	contains(t, err, "unknown key(s) unknown")
}

func TestWrongSchemaRejected(t *testing.T) {
	for _, schemaLine := range []string{"schema = 99\n", ""} {
		t.Run(schemaLine, func(t *testing.T) {
			err := mustFail(t, schemaLine+"session = \"orai\"\n")
			contains(t, err, "schema must be 2")
		})
	}
}

// Schema 1 called the document search `wiki`. Such a file is still read as written, so
// a project keeps working after the binary is upgraded; mixing the two is refused with
// the edit that fixes it.
func TestSchemaOneWikiTableIsReadAsShelf(t *testing.T) {
	cfg, err := config.Parse([]byte("schema = 1\n[integrations.wiki]\ncollections = { notes = \"notes\" }\n[integrations.wiki.smoke]\nlex = \"a\"\nvec = \"b\"\nexpect = \"notes/x.md\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Schema != 1 || cfg.Shelf == nil || cfg.Shelf.Collections["notes"] != "notes" || cfg.Shelf.Smoke.Expect != "notes/x.md" {
		t.Fatalf("schema 1 wiki table not read: %+v %+v", cfg, cfg.Shelf)
	}
	current, err := config.Parse([]byte("schema = 2\n[integrations.shelf]\n"))
	if err != nil || current.Schema != config.Schema || current.Shelf == nil {
		t.Fatalf("schema 2 shelf table: %v %+v", err, current)
	}
	err = mustFail(t, "schema = 2\n[integrations.wiki]\n")
	contains(t, err, "integrations.wiki is now integrations.shelf")
	err = mustFail(t, "schema = 1\n[integrations.shelf]\n")
	contains(t, err, "integrations.shelf needs `schema = 2`")
	// Errors inside a schema 1 table name the table as the file spells it.
	err = mustFail(t, "schema = 1\n[integrations.wiki]\nport = 80\n")
	contains(t, err, "integrations.wiki.port")
}

func TestBadProviderRejected(t *testing.T) {
	err := mustFail(t, base+`
[roles.lead]
provider = "chatgpt"
`)
	contains(t, err, "provider must be one of")
}

// --- config: relative-path enforcement (guide / worktree / collections) ------------

func TestGuidePathEscapesRejected(t *testing.T) {
	for _, guide := range []string{"/etc/passwd", "~/secrets.md", "../outside.md"} {
		t.Run(guide, func(t *testing.T) {
			err := mustFail(t, base+"\n[roles.lead]\nprovider = \"codex\"\nguide = \""+guide+"\"\n")
			contains(t, err, "roles.lead.guide")
		})
	}
}

func TestCollectionPathEscapesRejected(t *testing.T) {
	for _, p := range []string{"/abs/docs", "~/docs", "../docs"} {
		t.Run(p, func(t *testing.T) {
			err := mustFail(t, base+"\n[integrations.shelf]\ncollections = { docs = \""+p+"\" }\n")
			contains(t, err, "integrations.shelf.collections.docs")
		})
	}
}

func TestWorktreeDotdotIsAllowed(t *testing.T) {
	cfg := mustParse(t, base+`
[roles.staff]
provider = "claude"
worktree = "../sibling"
`)
	if cfg.Roles["staff"].Worktree != "../sibling" {
		t.Fatalf("worktree = %q, want ../sibling", cfg.Roles["staff"].Worktree)
	}
}

func TestWorktreeAbsoluteOrTildeRejected(t *testing.T) {
	for _, worktree := range []string{"/abs/worktree", "~/worktree"} {
		t.Run(worktree, func(t *testing.T) {
			err := mustFail(t, base+"\n[roles.staff]\nprovider = \"claude\"\nworktree = \""+worktree+"\"\n")
			contains(t, err, "roles.staff.worktree")
		})
	}
}

// --- config: shelf integration --------------------------------------------------------

func TestShelfPortBoundsAndBoolRejected(t *testing.T) {
	for _, literal := range []string{"1023", "65536", "true"} {
		t.Run(literal, func(t *testing.T) {
			err := mustFail(t, base+"\n[integrations.shelf]\nport = "+literal+"\n")
			contains(t, err, "integrations.shelf.port")
		})
	}
}

func TestShelfPortBoundsAccepted(t *testing.T) {
	for _, port := range []int{1024, 65535} {
		t.Run("", func(t *testing.T) {
			cfg := mustParse(t, base+"\n[integrations.shelf]\nport = "+strconv.Itoa(port)+"\n")
			if cfg.Shelf.Port != port {
				t.Fatalf("port = %d, want %d", cfg.Shelf.Port, port)
			}
		})
	}
}

func TestShelfCollectionsDefaultWhenOmitted(t *testing.T) {
	cfg := mustParse(t, base+"\n[integrations.shelf]\n")
	if len(cfg.Shelf.Collections) != 1 || cfg.Shelf.Collections["docs"] != "docs" {
		t.Fatalf("collections = %v, want {docs: docs}", cfg.Shelf.Collections)
	}
}

func TestShelfSmokeRequiresLexVecExpect(t *testing.T) {
	err := mustFail(t, base+`
[integrations.shelf.smoke]
lex = "unique term"
vec = "a question"
`)
	contains(t, err, "integrations.shelf.smoke.expect")
}

func TestShelfSmokeWithAllFieldsAccepted(t *testing.T) {
	cfg := mustParse(t, base+`
[integrations.shelf.smoke]
lex = "unique term"
vec = "a question"
expect = "docs/architecture.md"
`)
	if cfg.Shelf.Smoke == nil || cfg.Shelf.Smoke.Lex != "unique term" || cfg.Shelf.Smoke.Vec != "a question" || cfg.Shelf.Smoke.Expect != "docs/architecture.md" {
		t.Fatalf("smoke = %+v", cfg.Shelf.Smoke)
	}
}

// A collection is a folder, or part of one: { path, pattern } keeps a collection to the
// files its pattern matches, so `docs` and `docs/adr` can be separate collections.
func TestCollectionsTakeAFolderOrAPathWithAPattern(t *testing.T) {
	cfg := mustParse(t, base+"[integrations.shelf]\ncollections = { core = { path = \"docs\", pattern = \"*.md\" }, adr = \"docs/adr\", all = { path = \"notes\" } }\n")
	shelf := cfg.Shelf
	if shelf.Collections["core"] != "docs" || shelf.Patterns["core"] != "*.md" ||
		shelf.Collections["adr"] != "docs/adr" || shelf.Patterns["adr"] != config.DefaultPattern ||
		shelf.Collections["all"] != "notes" || shelf.Patterns["all"] != config.DefaultPattern {
		t.Fatalf("collections %v patterns %v", shelf.Collections, shelf.Patterns)
	}
	if got := mustParse(t, base+"[integrations.shelf]\n").Shelf.Patterns["docs"]; got != config.DefaultPattern {
		t.Fatalf("default collection pattern = %q", got)
	}
	for text, want := range map[string]string{
		`{ core = { path = "docs", pattern = "../*.md" } }`: "must stay inside the collection folder",
		`{ core = { path = "docs", pattern = "/x/*.md" } }`: "file pattern relative to the folder",
		`{ core = { path = "docs", pattern = "" } }`:        "file pattern relative to the folder",
		`{ core = { path = "docs", mask = "*.md" } }`:       "unknown key(s) mask",
		`{ core = { pattern = "*.md" } }`:                   "integrations.shelf.collections.core.path",
		`{ core = { path = "../docs" } }`:                   "must stay inside the project root",
	} {
		contains(t, mustFail(t, base+"[integrations.shelf]\ncollections = "+text+"\n"), want)
	}
}
