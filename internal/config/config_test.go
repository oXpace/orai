// Covers config.Parse's schema, roles, and integrations validation.
package config_test

import (
	"os"
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

// Schema 1 (Orai 0.3 and earlier) called the document search `wiki`. It is no longer
// read; the error names the edit that brings the file to the current schema.
func TestSchemaOneIsRefusedWithTheEdit(t *testing.T) {
	err := mustFail(t, "schema = 1\n[integrations.wiki]\n")
	contains(t, err, "schema 1 is from Orai 0.3 or earlier")
	contains(t, err, "rename `[integrations.wiki]` to `[integrations.shelf]`")
	current, err := config.Parse([]byte("schema = 2\n[integrations.shelf]\n"))
	if err != nil || current.Schema != config.Schema || current.Shelf == nil {
		t.Fatalf("schema 2 shelf table: %v %+v", err, current)
	}
	err = mustFail(t, "schema = 2\n[integrations.wiki]\n")
	contains(t, err, "unknown key(s) wiki")
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

// A context says what the documents under a folder are for. It is keyed by folder, so
// it must touch a collection, and it is one line because that is how the engine lists it.
func TestContextDescribesFoldersOfTheCollections(t *testing.T) {
	if cfg := mustParse(t, base+"[integrations.shelf]\n"); cfg.Shelf.Context != nil {
		t.Fatalf("context without a table = %v, want nil", cfg.Shelf.Context)
	}
	if cfg := mustParse(t, base+"[integrations.shelf.context]\n"); cfg.Shelf.Context == nil || len(cfg.Shelf.Context) != 0 {
		t.Fatalf("an empty table = %v, want an empty declaration", cfg.Shelf.Context)
	}
	cfg := mustParse(t, base+"[integrations.shelf]\ncollections = { adr = \"docs/adr\", notes = \"notes\" }\n"+
		"[integrations.shelf.context]\n\".\" = \" everything \"\n\"docs\" = \"documents\"\n\"docs/adr/\" = \"decisions\"\n\"notes/2024\" = \"this year\"\n")
	want := map[string]string{".": "everything", "docs": "documents", "docs/adr": "decisions", "notes/2024": "this year"}
	if got := cfg.Shelf.Context; len(got) != len(want) {
		t.Fatalf("context = %v, want %v", got, want)
	}
	for folder, text := range want {
		if cfg.Shelf.Context[folder] != text {
			t.Fatalf("context[%q] = %q, want %q", folder, cfg.Shelf.Context[folder], text)
		}
	}
	plan := cfg.Shelf.ContextPlan()
	if plan["adr"][""] != "everything / documents / decisions" || plan["notes"][""] != "everything" || plan["notes"]["2024"] != "this year" || len(plan["adr"]) != 1 {
		t.Fatalf("plan = %v", plan)
	}
	for text, want := range map[string]string{
		`"design" = "x"`:                      `integrations.shelf.context.design is not part of any collection`,
		`"../docs" = "x"`:                     "must stay inside the project root",
		`"/docs" = "x"`:                       "must be relative to the project root",
		`"docs" = ""`:                         "must be a non-empty string",
		`"docs" = 3`:                          "must be a non-empty string",
		`"docs" = "one\ntwo"`:                 "must be one line of text",
		`"docs" = "- a list item"`:            "must be one line of text",
		"\"docs\" = \"a\"\n\"docs/\" = \"b\"": "a second time",
	} {
		contains(t, mustFail(t, base+"[integrations.shelf.context]\n"+text+"\n"), want)
	}
	contains(t, mustFail(t, base+"[integrations.shelf]\ncontext = \"docs\"\n"), "must map folders to one-line descriptions")
}

// --- config: orai.local.toml over orai.toml -----------------------------------------

const sharedRoles = base + `
[roles.pm]
provider = "codex"
worktree = "."
guide = "docs/pm.md"
model = "shared-model"

[integrations.shelf]
collections = { docs = "docs" }
port = 18300

[integrations.shelf.context]
"docs" = "shared words"
`

func overlay(t *testing.T, shared, local string) *config.Config {
	t.Helper()
	cfg, err := config.Overlay([]byte(shared), []byte(local))
	if err != nil {
		t.Fatalf("Overlay: %v", err)
	}
	return cfg
}

// The local file is laid over the shared one key by key: a value replaces the shared
// value and leaves its neighbours alone, and a table that is only local is added.
func TestLocalFileReplacesValuesAndAddsTables(t *testing.T) {
	cfg := overlay(t, sharedRoles, `
[roles.pm]
model = "my-model"
effort = "high"

[roles.scratch]
provider = "claude"
worktree = ".worktrees/scratch"

[integrations.shelf]
port = 18401

[integrations.shelf.collections]
notes = "notes"

[integrations.shelf.context]
"docs" = "my words"
"notes" = "my notes"

[integrations.codegraph]
`)
	pm := cfg.Roles["pm"]
	if pm.Model != "my-model" || pm.Effort != "high" || pm.Provider != "codex" || pm.Worktree != "." || pm.Guide != "docs/pm.md" {
		t.Fatalf("pm = %+v", pm)
	}
	if cfg.Roles["scratch"].Provider != "claude" || len(cfg.Roles) != 2 {
		t.Fatalf("roles = %+v", cfg.Roles)
	}
	shelf := cfg.Shelf
	if shelf.Port != 18401 || shelf.Collections["docs"] != "docs" || shelf.Collections["notes"] != "notes" ||
		shelf.Context["docs"] != "my words" || shelf.Context["notes"] != "my notes" || cfg.Codegraph == nil {
		t.Fatalf("integrations = %+v %+v", shelf, cfg.Codegraph)
	}
	want := []string{"integrations.codegraph", "integrations.shelf.collections.notes", "integrations.shelf.context.docs",
		"integrations.shelf.context.notes", "integrations.shelf.port", "roles.pm.effort", "roles.pm.model", "roles.scratch"}
	if strings.Join(cfg.LocalKeys, " ") != strings.Join(want, " ") {
		t.Fatalf("local keys = %v, want %v", cfg.LocalKeys, want)
	}
	if !cfg.FromLocal("roles.pm.model") || !cfg.FromLocal("roles.scratch.provider") || cfg.FromLocal("roles.pm.provider") || cfg.FromLocal("roles.p") {
		t.Fatalf("FromLocal disagrees with %v", cfg.LocalKeys)
	}
	if got := cfg.LocalUnder("roles.pm"); strings.Join(got, " ") != "effort model" {
		t.Fatalf("LocalUnder(roles.pm) = %v", got)
	}
	if got := cfg.LocalUnder("roles.scratch"); len(got) != 1 || got[0] != "." {
		t.Fatalf("LocalUnder(roles.scratch) = %v", got)
	}
}

// An empty local file, or one that only opens tables, changes nothing and sets nothing.
func TestLocalFileThatSetsNothingChangesNothing(t *testing.T) {
	alone := mustParse(t, sharedRoles)
	for _, local := range []string{"", "# nothing yet\n", "[roles.pm]\n[integrations.shelf]\n"} {
		cfg := overlay(t, sharedRoles, local)
		if len(cfg.LocalKeys) != 0 || cfg.Roles["pm"] != alone.Roles["pm"] || cfg.Shelf.Port != alone.Shelf.Port ||
			cfg.Session != alone.Session || len(cfg.Roles) != len(alone.Roles) {
			t.Fatalf("local %q changed the result: %+v keys %v", local, cfg, cfg.LocalKeys)
		}
	}
}

// What the local file may not do, each refused with the file named: set the project's
// identity, use an unknown key, produce an invalid declaration, or stand in for a shared
// file that is invalid by itself.
func TestLocalFileErrorsNameTheFile(t *testing.T) {
	for local, want := range map[string]string{
		`session = "mine"`:                                 "orai.local.toml: `session` belongs in orai.toml",
		`name = "mine"`:                                    "orai.local.toml: `name` belongs in orai.toml",
		`schema = 2`:                                       "orai.local.toml: `schema` belongs in orai.toml",
		`colour = "red"`:                                   "orai.local.toml: unknown key(s) colour",
		"[roles.pm]\nnickname = \"boss\"":                  "orai.local.toml, applied over orai.toml: roles.pm: unknown key(s) nickname",
		"[roles.scratch]\nmodel = \"x\"":                   "orai.local.toml, applied over orai.toml: roles.scratch.provider must be one of",
		"[roles.pm]\nprovider = \"chatgpt\"":               "orai.local.toml, applied over orai.toml: roles.pm.provider",
		"[integrations.shelf]\nport = 80":                  "orai.local.toml, applied over orai.toml: integrations.shelf.port",
		"[integrations.shelf.context]\n\"design\" = \"x\"": "is not part of any collection",
		"[roles.pm":                                        "orai.local.toml: ",
	} {
		_, err := config.Overlay([]byte(sharedRoles), []byte(local))
		if err == nil {
			t.Fatalf("local %q was accepted", local)
		}
		contains(t, err, want)
	}
	// A local file cannot repair or hide a shared file that is invalid alone.
	_, err := config.Overlay([]byte(base+"[roles.pm]\nworktree = \".\"\n"), []byte("[roles.pm]\nprovider = \"codex\"\n"))
	if err == nil || strings.Contains(err.Error(), "orai.local.toml") {
		t.Fatalf("an invalid shared file: %v", err)
	}
	contains(t, err, "roles.pm.provider")
	// A schema 1 shared file is refused before the local file is looked at.
	_, err = config.Overlay([]byte("schema = 1\n"), []byte(""))
	contains(t, err, "schema 1 is from Orai 0.3 or earlier")
}

// A project without the local file loads exactly as before, and an error in the local
// file points at the file that was read.
func TestLoadWithLocalTreatsAMissingFileAsNormal(t *testing.T) {
	dir := t.TempDir()
	shared, local := dir+"/orai.toml", dir+"/orai.local.toml"
	if err := os.WriteFile(shared, []byte(sharedRoles), 0o644); err != nil {
		t.Fatal(err)
	}
	alone, err := config.Load(shared)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadWithLocal(shared, local)
	if err != nil || cfg.LocalFile != "" || len(cfg.LocalKeys) != 0 || cfg.Roles["pm"] != alone.Roles["pm"] || cfg.Shelf.Port != alone.Shelf.Port {
		t.Fatalf("without a local file: %v %+v", err, cfg)
	}
	if err := os.WriteFile(local, []byte("[roles.pm]\nmodel = \"mine\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg, err = config.LoadWithLocal(shared, local); err != nil || cfg.LocalFile != local || cfg.Roles["pm"].Model != "mine" {
		t.Fatalf("with a local file: %v %+v", err, cfg)
	}
	if err := os.WriteFile(local, []byte("session = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = config.LoadWithLocal(shared, local)
	if err == nil {
		t.Fatal("an invalid local file was accepted")
	}
	contains(t, err, local+": `session` belongs in orai.toml")
}

// --- config: openspec --------------------------------------------------------------

func TestOpenSpecTableKeepsTelemetryOffUnlessTurnedOn(t *testing.T) {
	if cfg := mustParse(t, base); cfg.OpenSpec != nil {
		t.Fatalf("no table should mean no OpenSpec: %+v", cfg.OpenSpec)
	}
	if cfg := mustParse(t, base+"\n[integrations.openspec]\n"); cfg.OpenSpec == nil || cfg.OpenSpec.Telemetry {
		t.Fatalf("an empty table should declare OpenSpec with telemetry off: %+v", cfg.OpenSpec)
	}
	if cfg := mustParse(t, base+"\n[integrations.openspec]\ntelemetry = true\n"); !cfg.OpenSpec.Telemetry {
		t.Fatal("telemetry = true was not read")
	}
	contains(t, mustFail(t, base+"\n[integrations.openspec]\ntelemetry = \"no\"\n"), "true or false")
	contains(t, mustFail(t, base+"\n[integrations.openspec]\ntools = \"claude\"\n"), "unknown key(s) tools")
	contains(t, mustFail(t, base+"\n[roles.openspec]\nprovider = \"codex\"\n"), "openspec")
}
