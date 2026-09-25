// Covers Settings construction, including that two projects never collide on env,
// index names, or PID files.
package wiki

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsEnvPointsInsideDotOraiWiki(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	env := envMap(s.Env())

	if env["QMD_CONFIG_DIR"] != s.Directory {
		t.Fatalf("QMD_CONFIG_DIR = %q, want %q", env["QMD_CONFIG_DIR"], s.Directory)
	}
	if env["INDEX_PATH"] != s.DB {
		t.Fatalf("INDEX_PATH = %q, want %q", env["INDEX_PATH"], s.DB)
	}
	want := filepath.Join(p.Root, ".orai", "wiki")
	if s.Directory != want {
		t.Fatalf("Directory = %q, want %q", s.Directory, want)
	}
	if filepath.Dir(s.DB) != s.Directory {
		t.Fatalf("DB parent = %q, want %q", filepath.Dir(s.DB), s.Directory)
	}
}

func TestEnvNeverDuplicatesOverriddenKeys(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	t.Setenv("QMD_CONFIG_DIR", "/should-be-replaced")
	env := s.Env()
	count := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "QMD_CONFIG_DIR=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("QMD_CONFIG_DIR appears %d times, want 1", count)
	}
}

func TestTwoProjectsGetDistinctIndexNamesAndPIDFiles(t *testing.T) {
	tmp := t.TempDir()
	cache := filepath.Join(tmp, "cache")
	t.Setenv("XDG_CACHE_HOME", cache)
	pa := makeQMDProject(t, filepath.Join(tmp, "a"), 0)
	pb := makeQMDProject(t, filepath.Join(tmp, "b"), 0)
	sa, sb := NewSettings(pa), NewSettings(pb)

	if sa.Index == sb.Index {
		t.Fatalf("index should differ, both are %q", sa.Index)
	}
	if !strings.HasPrefix(sa.Index, "orai-") || !strings.HasPrefix(sb.Index, "orai-") {
		t.Fatalf("index should start with orai-: %q, %q", sa.Index, sb.Index)
	}
	if sa.PIDFile() == sb.PIDFile() {
		t.Fatalf("pid files should differ, both are %q", sa.PIDFile())
	}
	wantA := filepath.Join(cache, "qmd", "mcp-"+sa.Index+".pid")
	wantB := filepath.Join(cache, "qmd", "mcp-"+sb.Index+".pid")
	if sa.PIDFile() != wantA {
		t.Fatalf("PIDFile() = %q, want %q", sa.PIDFile(), wantA)
	}
	if sb.PIDFile() != wantB {
		t.Fatalf("PIDFile() = %q, want %q", sb.PIDFile(), wantB)
	}
}

func TestDefaultPortNeverConfiguredValueIs8181(t *testing.T) {
	// Regression guard for the "never port 8181 unless configured" rule: the derived
	// default must be a different port for any project id encountered in this suite.
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	if DefaultPort(p.ID()) == 8181 {
		t.Fatalf("DefaultPort(%q) unexpectedly derived 8181", p.ID())
	}
}
