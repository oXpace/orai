package doctor

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderListsFixesInOrderAndKeepsOptionalApart(t *testing.T) {
	checks := []Check{
		New("project", Healthy, "orai.toml is valid", ""),
		New("tool.codex", Blocked, "codex is not on PATH", InstallCodex).WithCore(),
		New("codegraph", Blocked, "codegraph status failed: env: node: No such file or directory", ""),
		New("mail", NotConfigured, "no roles in orai.toml yet", "add a role"),
		New("role.dev", Degraded, "missing role worktree: .worktrees/dev", "`git worktree add .worktrees/dev`"),
		New("wiki.search", NotChecked, "search itself was not run", "`orai doctor --deep`"),
	}
	var out bytes.Buffer
	code := Render(&out, checks, true, "Status")
	text := out.String()
	if code != ExitBlocked {
		t.Fatalf("exit code %d", code)
	}
	for _, want := range []string{
		"  ✓ project      orai.toml is valid\n",
		"  ✗ tool.codex   blocked: codex is not on PATH\n",
		"  - mail         not-configured: no roles in orai.toml yet\n",
		// A problem without a known fix still counts and still gets a step.
		"Status: blocked (3 to fix)\n",
		"Next steps:\n  1. tool.codex: Install Codex CLI",
		"  2. codegraph: No automatic fix is known.",
		"  3. role.dev: `git worktree add .worktrees/dev`\n",
		"Optional:\n  - mail: add a role\n  - wiki.search: `orai doctor --deep`\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output lacks %q:\n%s", want, text)
		}
	}

	out.Reset()
	Render(&out, checks, false, "Diagnosis")
	if text := out.String(); strings.Contains(text, "project") || !strings.Contains(text, "✗ tool.codex") ||
		!strings.Contains(text, "Diagnosis: blocked (3 to fix)") {
		t.Fatalf("problems-only output:\n%s", text)
	}

	out.Reset()
	if code := Render(&out, checks[:1], false, "Diagnosis"); code != ExitOK || out.String() != "Diagnosis: healthy\n" {
		t.Fatalf("healthy: %d %q", code, out.String())
	}
}

// A project that uses Orai has no docs/ from this repository, so a hint must be a
// command or a full URL.
func TestInstallHintsAreCommands(t *testing.T) {
	for _, hint := range []string{InstallCodex, InstallClaude, InstallQMD, InstallCodegraph} {
		if !strings.Contains(hint, "`") || strings.Contains(hint, "(docs/") {
			t.Errorf("hint is not actionable on its own: %s", hint)
		}
	}
}
