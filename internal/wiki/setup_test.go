// Covers SetupStep's wiki-step behavior and MCPServers, plus direct coverage of
// Diagnose()'s top-level branches, which are otherwise only exercised indirectly
// through `orai doctor` (a cli-package concern).
package wiki

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

func TestWikiStepSkipsWithoutEngineOrDocuments(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "p"), "schema = 1\nsession = \"orai\"\n\n[integrations.wiki]\n")

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

func TestWikiStepInitializesOnceThenRecovers(t *testing.T) {
	for _, tc := range []struct {
		existing bool
		want     string
	}{
		{false, "init"},
		{true, "recover"},
	} {
		p := writeProject(t, filepath.Join(t.TempDir(), "p"), "schema = 1\nsession = \"orai\"\n\n[integrations.wiki]\n")
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
			if err := os.MkdirAll(s.Directory, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(s.ConfigFile, []byte("{}"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(s.DB, []byte("db"), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		savedLifecycle := lifecycleCall
		var calls []string
		lifecycleCall = func(_ *project.Project, action string, _ io.Writer) error {
			calls = append(calls, action)
			return nil
		}

		if got := SetupStep(p, io.Discard); got != "wiki: ready ("+tc.want+")" {
			t.Fatalf("SetupStep() = %q, want wiki: ready (%s)", got, tc.want)
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
	if !strings.HasPrefix(s.ServerName, "wiki-") {
		t.Fatalf("ServerName = %q, want wiki- prefix", s.ServerName)
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

func TestMCPServersEmptyWhenWikiIsNotConfigured(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 1\nsession = \"orai\"\n")
	if got := MCPServers(p, "claude"); len(got) != 0 {
		t.Fatalf("MCPServers(claude) = %v, want empty", got)
	}
	if got := MCPServers(p, "codex"); len(got) != 0 {
		t.Fatalf("MCPServers(codex) = %v, want empty", got)
	}
}

// ---- Diagnose() top-level branches (covered here directly; `orai doctor` only exercises them indirectly) --

func TestDiagnoseNotConfigured(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 1\nsession = \"orai\"\n")
	checks := Diagnose(p, false)
	if len(checks) != 1 || checks[0].Component != "wiki" || checks[0].Status != doctor.NotConfigured {
		t.Fatalf("Diagnose() = %+v, want a single not-configured wiki check", checks)
	}
}

func TestDiagnoseBlockedWhenQMDMissing(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }

	checks := Diagnose(p, false)
	if len(checks) != 1 || checks[0].Status != doctor.Blocked {
		t.Fatalf("Diagnose() = %+v, want a single blocked check", checks)
	}
	if !strings.Contains(checks[0].Reason, "not on PATH") {
		t.Fatalf("reason = %q, want mention of PATH", checks[0].Reason)
	}
}

func TestDiagnoseNotReadyWhenCollectionFolderMissing(t *testing.T) {
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), "schema = 1\nsession = \"orai\"\n\n[integrations.wiki]\n")
	// docs/ deliberately not created.
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }

	checks := Diagnose(p, false)
	if len(checks) != 1 || checks[0].Status != doctor.NotReady {
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
	if len(checks) != 1 || checks[0].Status != doctor.NotReady {
		t.Fatalf("Diagnose() = %+v, want a single not-ready check", checks)
	}
	if checks[0].NextAction != "orai wiki init" {
		t.Fatalf("next action = %q, want orai wiki init", checks[0].NextAction)
	}
}
