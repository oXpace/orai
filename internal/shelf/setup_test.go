// Covers SetupStep's shelf-step behavior and MCPServers, plus direct coverage of
// Diagnose()'s top-level branches, which are otherwise only exercised indirectly
// through `orai doctor` (a cli-package concern).
package shelf

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

func TestShelfStepSkipsWithoutEngineOrDocuments(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "p"), "schema = 2\nsession = \"orai\"\n\n[integrations.shelf]\n")

	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if got := SetupStep(p, io.Discard); !strings.Contains(got, "not installed") {
		t.Fatalf("SetupStep() = %q, want mention of not installed", got)
	}

	lookPath = func(name string) (string, error) { return "/fake/" + name, nil }
	if err := os.MkdirAll(filepath.Join(p.Root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := SetupStep(p, io.Discard); !strings.Contains(got, "no Markdown documents") {
		t.Fatalf("SetupStep() = %q, want mention of no Markdown documents", got)
	}
}

func TestShelfStepInitializesOnceThenRecovers(t *testing.T) {
	for _, tc := range []struct {
		existing bool
		legacy   bool // the index is still in .orai/wiki, where Orai 0.3 kept it
		want     string
	}{
		{false, false, "init"},
		{true, false, "recover"},
		{true, true, "recover"}, // recover moves it; init would build a second index
	} {
		p := writeProject(t, filepath.Join(t.TempDir(), "p"), "schema = 2\nsession = \"orai\"\n\n[integrations.shelf]\n")
		if err := os.MkdirAll(filepath.Join(p.Root, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p.Root, "docs", "a.md"), []byte("# A\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		savedLookPath := lookPath
		lookPath = func(name string) (string, error) { return "/fake/" + name, nil }

		if tc.existing {
			s := NewSettings(p)
			dir := s.Directory
			if tc.legacy {
				dir = s.LegacyDirectory()
			}
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, filepath.Base(s.ConfigFile)), []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, filepath.Base(s.DB)), []byte("db"), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		savedLifecycle := lifecycleCall
		var calls []string
		lifecycleCall = func(_ *project.Project, action string, _ io.Writer) error {
			calls = append(calls, action)
			return nil
		}

		if got := SetupStep(p, io.Discard); got != "shelf: ready ("+tc.want+")" {
			t.Fatalf("SetupStep() = %q, want shelf: ready (%s)", got, tc.want)
		}
		if len(calls) != 1 || calls[0] != tc.want {
			t.Fatalf("calls = %v, want [%s]", calls, tc.want)
		}

		lifecycleCall = func(*project.Project, string, io.Writer) error { return errors.New("port busy") }
		if got := SetupStep(p, io.Discard); !strings.Contains(got, "failed: port busy") {
			t.Fatalf("SetupStep() = %q, want mention of the failure", got)
		}

		lookPath = savedLookPath
		lifecycleCall = savedLifecycle
	}
}

func TestMCPServersReturnsHttpEntryForClaudeAndUrlEntryForCodex(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	// One name in every project: a session only ever sees its own project's server.
	if s.ServerName != "shelf" {
		t.Fatalf("ServerName = %q, want shelf", s.ServerName)
	}

	claudeServers := MCPServers(p, "claude")
	want := map[string]map[string]any{s.ServerName: {"type": "http", "url": s.Endpoint}}
	if len(claudeServers) != len(want) || claudeServers[s.ServerName]["type"] != "http" ||
		claudeServers[s.ServerName]["url"] != s.Endpoint {
		t.Fatalf("MCPServers(claude) = %v, want %v", claudeServers, want)
	}

	codexServers := MCPServers(p, "codex")
	if codexServers[s.ServerName]["url"] != s.Endpoint {
		t.Fatalf("codex url = %v, want %q", codexServers[s.ServerName]["url"], s.Endpoint)
	}
	if _, ok := codexServers[s.ServerName]["startup_timeout_sec"]; !ok {
		t.Fatalf("codex entry missing startup_timeout_sec: %v", codexServers[s.ServerName])
	}
}

func TestMCPServersEmptyWhenShelfIsNotConfigured(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 2\nsession = \"orai\"\n")
	if got := MCPServers(p, "claude"); len(got) != 0 {
		t.Fatalf("MCPServers(claude) = %v, want empty", got)
	}
	if got := MCPServers(p, "codex"); len(got) != 0 {
		t.Fatalf("MCPServers(codex) = %v, want empty", got)
	}
}

// ---- Diagnose() top-level branches (covered here directly; `orai doctor` only exercises them indirectly) --

func TestDiagnoseNotConfigured(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 2\nsession = \"orai\"\n")
	checks := Diagnose(p, false)
	if len(checks) != 1 || checks[0].Component != "shelf" || checks[0].Status != doctor.NotConfigured {
		t.Fatalf("Diagnose() = %+v, want a single not-configured shelf check", checks)
	}
}

func TestDiagnoseBlockedWhenQMDMissing(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }

	checks := Diagnose(p, false)
	if len(checks) != 2 || checks[1].Component != "shelf.registration" || checks[0].Status != doctor.Blocked {
		t.Fatalf("Diagnose() = %+v, want a single blocked check", checks)
	}
	if !strings.Contains(checks[0].Reason, "not on PATH") {
		t.Fatalf("reason = %q, want mention of PATH", checks[0].Reason)
	}
}

func TestDiagnoseNotReadyWhenCollectionFolderMissing(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 2\nsession = \"orai\"\n\n[integrations.shelf]\n")
	// docs/ deliberately not created.
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }

	checks := Diagnose(p, false)
	if len(checks) != 2 || checks[1].Component != "shelf.registration" || checks[0].Status != doctor.NotReady {
		t.Fatalf("Diagnose() = %+v, want a single not-ready check", checks)
	}
	if !strings.Contains(checks[0].Reason, "collection folder(s) missing") {
		t.Fatalf("reason = %q, want mention of the missing folder", checks[0].Reason)
	}
}

func TestDiagnoseNotReadyWhenIndexNotInitialized(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }

	checks := Diagnose(p, false)
	if len(checks) != 2 || checks[1].Component != "shelf.registration" || checks[0].Status != doctor.NotReady {
		t.Fatalf("Diagnose() = %+v, want a single not-ready check", checks)
	}
	if !strings.Contains(checks[0].NextAction, "orai shelf init") {
		t.Fatalf("next action = %q, want orai shelf init", checks[0].NextAction)
	}
}

// ---- the project's own MCP settings ----------------------------------------------

// Outside role sessions the shelf is reached through the project's own MCP settings,
// where its address is written out. Doctor reads those files and says whether they
// still point at this project's server; it never edits them.
func TestRegistrationComparesProjectMCPSettingsWithTheShelfAddress(t *testing.T) {
	saved := claudeServer
	t.Cleanup(func() { claudeServer = saved })
	claudeServer = func(string) string { t.Fatal("the Claude CLI is only asked with --deep"); return "" }

	root := filepath.Join(t.TempDir(), "proj")
	text := "schema = 2\n[roles.dev]\nprovider = \"claude\"\n[integrations.shelf]\nport = 18338\n"
	p := writeProject(t, root, text)
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	check := func() doctor.Check { return registration(NewSettings(p), false) }

	// Nothing written: fine, and the next step is the exact entry to add.
	if c := check(); c.Status != doctor.NotConfigured || c.Core ||
		!strings.Contains(c.NextAction, "register `shelf` at http://127.0.0.1:18338/mcp") || strings.Contains(c.NextAction, "\n") {
		t.Fatalf("nothing registered: %+v", c)
	}

	// Both files point at this project's server (localhost is the same host).
	write(".codex/config.toml", "[mcp_servers.shelf]\nurl = \"http://localhost:18338/mcp\"\n\n[mcp_servers.other]\ncommand = \"x\"\n")
	write(".mcp.json", `{"mcpServers": {"shelf": {"type": "http", "url": "http://127.0.0.1:18338/mcp"}, "penpot": {"type": "http", "url": "https://example.test/mcp"}}}`)
	if c := check(); c.Status != doctor.Healthy || !strings.Contains(c.Reason, ".mcp.json, .codex/config.toml") {
		t.Fatalf("registered: %+v", c)
	}

	// A different port, and the server still under its 0.3 name.
	write(".codex/config.toml", "[mcp_servers.shelf]\nurl = \"http://127.0.0.1:8181/mcp\"\n")
	write(".mcp.json", `{"mcpServers": {"wiki-proj": {"type": "http", "url": "http://127.0.0.1:18338/mcp"}}}`)
	c := check()
	if c.Status != doctor.Degraded || !strings.Contains(c.Reason, ".codex/config.toml has `shelf` at http://127.0.0.1:8181/mcp") ||
		!strings.Contains(c.Reason, "as `wiki-proj`") || !strings.Contains(c.NextAction, "rename `wiki-proj` to `shelf`") ||
		!strings.Contains(c.NextAction, "to http://127.0.0.1:18338/mcp") {
		t.Fatalf("wrong port and old name: %+v", c)
	}

	// A file that does not parse is said so, not skipped.
	write(".mcp.json", `{"mcpServers": `)
	if c := check(); c.Status != doctor.Degraded || !strings.Contains(c.Reason, ".mcp.json could not be parsed") {
		t.Fatalf("unparsable file: %+v", c)
	}

	// The address is written down but the port comes from the checkout path: right
	// here, wrong in the next checkout, so doctor asks for the port to be fixed.
	if err := os.Remove(filepath.Join(root, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
	p = writeProject(t, root, strings.Replace(text, "port = 18338\n", "", 1))
	derived := NewSettings(p)
	write(".codex/config.toml", "[mcp_servers.shelf]\nurl = \""+derived.Endpoint+"\"\n")
	if c := check(); c.Status != doctor.Degraded || !strings.Contains(c.Reason, "not fixed in orai.toml") ||
		!strings.Contains(c.NextAction, fmt.Sprintf("`port = %d`", derived.Port)) {
		t.Fatalf("derived port: %+v", c)
	}

	// --deep also asks Claude Code about its user and local scopes.
	p = writeProject(t, root, text)
	write(".codex/config.toml", "[mcp_servers.shelf]\nurl = \"http://127.0.0.1:18338/mcp\"\n")
	claudeServer = func(name string) string { return name + ":\n  Scope: User config\n  URL: http://127.0.0.1:9999/mcp\n" }
	if c := registration(NewSettings(p), true); c.Status != doctor.Degraded || !strings.Contains(c.Reason, "another MCP server named `shelf`") {
		t.Fatalf("user-scope server with the same name: %+v", c)
	}
	claudeServer = func(name string) string {
		return name + ":\n  Scope: Project config\n  URL: http://127.0.0.1:18338/mcp\n"
	}
	if c := registration(NewSettings(p), true); c.Status != doctor.Healthy {
		t.Fatalf("the same server seen through the CLI: %+v", c)
	}
}
