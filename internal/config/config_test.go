// Covers config.Parse's schema, roles, and integrations validation.
package config_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/config"
)

const base = "schema = 1\nsession = \"orai\"\n"

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
	for _, name := range []string{"init", "run", "user", "status", "all", "orai", "doctor", "qmd", "setup", "msg", "wiki"} {
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
	for _, schemaLine := range []string{"schema = 2\n", ""} {
		t.Run(schemaLine, func(t *testing.T) {
			err := mustFail(t, schemaLine+"session = \"orai\"\n")
			contains(t, err, "schema must be 1")
		})
	}
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
			err := mustFail(t, base+"\n[integrations.wiki]\ncollections = { docs = \""+p+"\" }\n")
			contains(t, err, "integrations.wiki.collections.docs")
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

// --- config: wiki integration --------------------------------------------------------

func TestWikiPortBoundsAndBoolRejected(t *testing.T) {
	for _, literal := range []string{"1023", "65536", "true"} {
		t.Run(literal, func(t *testing.T) {
			err := mustFail(t, base+"\n[integrations.wiki]\nport = "+literal+"\n")
			contains(t, err, "integrations.wiki.port")
		})
	}
}

func TestWikiPortBoundsAccepted(t *testing.T) {
	for _, port := range []int{1024, 65535} {
		t.Run("", func(t *testing.T) {
			cfg := mustParse(t, base+"\n[integrations.wiki]\nport = "+strconv.Itoa(port)+"\n")
			if cfg.Wiki.Port != port {
				t.Fatalf("port = %d, want %d", cfg.Wiki.Port, port)
			}
		})
	}
}

func TestWikiCollectionsDefaultWhenOmitted(t *testing.T) {
	cfg := mustParse(t, base+"\n[integrations.wiki]\n")
	if len(cfg.Wiki.Collections) != 1 || cfg.Wiki.Collections["docs"] != "docs" {
		t.Fatalf("collections = %v, want {docs: docs}", cfg.Wiki.Collections)
	}
}

func TestWikiSmokeRequiresLexVecExpect(t *testing.T) {
	err := mustFail(t, base+`
[integrations.wiki.smoke]
lex = "unique term"
vec = "a question"
`)
	contains(t, err, "integrations.wiki.smoke.expect")
}

func TestWikiSmokeWithAllFieldsAccepted(t *testing.T) {
	cfg := mustParse(t, base+`
[integrations.wiki.smoke]
lex = "unique term"
vec = "a question"
expect = "docs/architecture.md"
`)
	if cfg.Wiki.Smoke == nil || cfg.Wiki.Smoke.Lex != "unique term" || cfg.Wiki.Smoke.Vec != "a question" || cfg.Wiki.Smoke.Expect != "docs/architecture.md" {
		t.Fatalf("smoke = %+v", cfg.Wiki.Smoke)
	}
}
