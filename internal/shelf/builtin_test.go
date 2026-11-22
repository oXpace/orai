// The built-in search is the standard; QMD runs only where it was turned on.
package shelf

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
)

func engineProject(t *testing.T, engine, extra string, indexed bool) *project.Project {
	t.Helper()
	text := "schema = 2\n[integrations.shelf]\n"
	if engine != "" {
		text += "engine = \"" + engine + "\"\n"
	}
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), text+extra)
	writeDoc(t, filepath.Join(p.Root, "docs", "deploy.md"), "# 배포\n\n배포는 release 절차를 따른다\n")
	if indexed {
		s := NewSettings(p)
		writeDoc(t, s.DB, "db")
		writeDoc(t, s.ConfigFile, "{}")
	}
	return p
}

// noQMD fails the test if anything looks for qmd, runs it, or opens its port.
func noQMD(t *testing.T) {
	t.Helper()
	savedLook, savedRun, savedReach, savedCall, savedSync := lookPath, runCommand, reachability, lifecycleCall, autoSyncRun
	t.Cleanup(func() {
		lookPath, runCommand, reachability, lifecycleCall, autoSyncRun = savedLook, savedRun, savedReach, savedCall, savedSync
	})
	lookPath = func(name string) (string, error) { t.Errorf("looked for %s", name); return "/fake/" + name, nil }
	runCommand = func(_ io.Writer, argv []string, _ *Settings, _ bool) (string, error) {
		t.Errorf("ran %v", argv)
		return "", nil
	}
	reachability = func(int) (string, string) { t.Error("opened the shelf port"); return "refused", "" }
	lifecycleCall = func(*project.Project, string, io.Writer) error { t.Error("ran a lifecycle"); return nil }
	autoSyncRun = func(*Settings, io.Writer) error { t.Error("synced"); return nil }
}

// 시나리오: QMD를 켰다 / 0.6.0에서 쓰던 색인 / 새 프로젝트 / 내장 검색으로 명시
func TestEngineChoice(t *testing.T) {
	for _, c := range []struct {
		engine  string
		indexed bool
		want    bool
	}{
		{"", false, false}, {"", true, true},
		{"qmd", false, true}, {"qmd", true, true},
		{"builtin", false, false}, {"builtin", true, false},
	} {
		if got := UsesQMD(engineProject(t, c.engine, "", c.indexed)); got != c.want {
			t.Errorf("engine %q, indexed %v: UsesQMD = %v, want %v", c.engine, c.indexed, got, c.want)
		}
	}
}

// 시나리오: 새 프로젝트
func TestBuiltInNeedsNoQMD(t *testing.T) {
	noQMD(t)
	p := engineProject(t, "", "", false)
	if got := SetupStep(p, io.Discard); !strings.Contains(got, "built-in keyword search") {
		t.Errorf("setup = %q", got)
	}
	if servers := MCPServers(p, "claude"); len(servers) != 0 {
		t.Errorf("servers = %v", servers)
	}
	if got := byComponent(Diagnose(p, false))["shelf"]; got.Status != doctor.Healthy || !strings.Contains(got.Reason, "1 document") {
		t.Errorf("shelf = %+v", got)
	}
	out := Search(NewSettings(p), "release 절차", 5)
	if out.Mode != "builtin" || out.Reason != "" || out.Fix != "" || out.Port != 0 || len(out.Hits) != 1 {
		t.Errorf("search = %+v", out)
	}
}

// 시나리오: 내장 검색으로 명시 / 내장 검색으로 껐다
func TestExplicitBuiltInIgnoresALeftoverIndex(t *testing.T) {
	noQMD(t)
	p := engineProject(t, "builtin", "", true)
	if out := Search(NewSettings(p), "release", 5); out.Mode != "builtin" || len(out.Stale) != 0 {
		t.Errorf("search = %+v", out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	AutoSync(ctx, p)
	if _, err := os.Stat(filepath.Join(NewSettings(p).Directory, "autosync.log")); err == nil {
		t.Error("autosync started")
	}
}

// 시나리오: 켜지 않고 init
func TestLifecycleSaysHowToTurnQMDOn(t *testing.T) {
	noQMD(t)
	p := engineProject(t, "", "", false)
	for _, action := range []string{"init", "recover", "sync", "refresh", "check"} {
		if err := Lifecycle(p, action, io.Discard); err == nil || !strings.Contains(err.Error(), `engine = "qmd"`) {
			t.Errorf("%s: %v", action, err)
		}
	}
}

func TestStopWorksAfterQMDIsTurnedOff(t *testing.T) {
	calls := installSuccessMocks(t, false, nil, "/fake/qmd")
	if err := Lifecycle(engineProject(t, "builtin", "", true), "stop", io.Discard); err != nil || len(*calls) != 1 {
		t.Fatalf("%v %v", err, *calls)
	}
}

// 시나리오: smoke
func TestBuiltInDeepChecksTheSmokeDocument(t *testing.T) {
	noQMD(t)
	smoke := "[integrations.shelf.smoke]\nexpect = \"docs/deploy.md\"\nlex = \"release\"\nvec = "
	for query, want := range map[string]string{"\"배포 절차\"\n": doctor.Healthy, "\"zzqx\"\n": doctor.Degraded} {
		if got := byComponent(Diagnose(engineProject(t, "", smoke+query, false), true))["shelf.search"]; got.Status != want {
			t.Errorf("vec %s: %+v", query, got)
		}
	}
}

// 시나리오: 남은 MCP 등록
func TestBuiltInFlagsALeftoverRegistration(t *testing.T) {
	noQMD(t)
	p := engineProject(t, "", "", false)
	writeDoc(t, filepath.Join(p.Root, ".mcp.json"), `{"mcpServers":{"shelf":{"type":"http","url":"http://127.0.0.1:18338/mcp"}}}`)
	if got := byComponent(Diagnose(p, false))["shelf.registration"]; got.Status != doctor.Degraded {
		t.Fatalf("%+v", got)
	}
}
