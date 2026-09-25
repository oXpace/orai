// Covers the QMD lifecycle: init, recover, refresh, stop, and verify. Every `qmd`
// invocation goes through installSuccessMocks (a patched runCommand); reachability,
// ownServerRunning and the MCP client are patched too. No test binds or connects to a
// real QMD server or port 8181.
package wiki

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/oXpace/orai/internal/doctor"
)

// ---- init -----------------------------------------------------------------------

func TestInitCreatesConfigWithAbsoluteCollectionsAndDefaultModel(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	installSuccessMocks(t, false, nil, "/usr/bin/qmd")

	if err := Lifecycle(p, "init", io.Discard); err != nil {
		t.Fatalf("Lifecycle(init) = %v", err)
	}

	data, err := os.ReadFile(s.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]any
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	collections, _ := written["collections"].(map[string]any)
	docs, _ := collections["docs"].(map[string]any)
	if docs["path"] != s.Collections["docs"] {
		t.Fatalf("collections.docs.path = %v, want %q", docs["path"], s.Collections["docs"])
	}
	if !filepath.IsAbs(s.Collections["docs"]) {
		t.Fatalf("collections[docs] = %q, want absolute", s.Collections["docs"])
	}
	models, _ := written["models"].(map[string]any)
	if models["embed"] != Model {
		t.Fatalf("models.embed = %v, want %q", models["embed"], Model)
	}
}

func TestInitRunsShowThenUpdateThenEmbedThenDaemonWithIndexPrefixedArgv(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	calls := installSuccessMocks(t, false, nil, "/usr/bin/qmd")

	if err := Lifecycle(p, "init", io.Discard); err != nil {
		t.Fatalf("Lifecycle(init) = %v", err)
	}

	var subcommands []string
	for _, call := range *calls {
		if len(call) < 4 || call[0] != "/usr/bin/qmd" || call[1] != "--index" || call[2] != s.Index {
			t.Fatalf("call %v missing the --index prefix", call)
		}
		subcommands = append(subcommands, call[3])
	}
	want := []string{"collection", "update", "embed", "mcp"}
	if !reflect.DeepEqual(subcommands, want) {
		t.Fatalf("subcommands = %v, want %v", subcommands, want)
	}

	daemon := (*calls)[len(*calls)-1]
	if daemon[indexOf(daemon, "--host")+1] != Host {
		t.Fatalf("daemon call %v missing --host %s", daemon, Host)
	}
	if daemon[indexOf(daemon, "--port")+1] != strconv.Itoa(s.Port) {
		t.Fatalf("daemon call %v missing --port %d", daemon, s.Port)
	}
	if s.Port == 8181 {
		t.Fatalf("default port must never be 8181")
	}
	if containsStr(daemon, "8181") {
		t.Fatalf("daemon call %v unexpectedly mentions 8181", daemon)
	}
}

func TestInitStartsDaemonOnConfigured8181WhenExplicitlySet(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 8181)
	s := NewSettings(p)
	if s.Port != 8181 {
		t.Fatalf("s.Port = %d, want 8181", s.Port)
	}
	calls := installSuccessMocks(t, false, nil, "/usr/bin/qmd")

	if err := Lifecycle(p, "init", io.Discard); err != nil {
		t.Fatalf("Lifecycle(init) = %v", err)
	}

	daemon := (*calls)[len(*calls)-1]
	if daemon[3] != "mcp" {
		t.Fatalf("daemon call = %v, want subcommand mcp", daemon)
	}
	if daemon[indexOf(daemon, "--port")+1] != "8181" {
		t.Fatalf("daemon call %v missing --port 8181", daemon)
	}
}

// ---- recover --------------------------------------------------------------------

func TestRecoverPreservesConfigAndDBByteForByteAndSkipsUpdateAndEmbed(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	originalConfig, err := json.MarshalIndent(map[string]any{
		"collections": map[string]any{"docs": map[string]any{"path": s.Collections["docs"], "pattern": "**/*.md"}},
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	originalConfig = append(originalConfig, '\n')
	if err := os.WriteFile(s.ConfigFile, originalConfig, 0o644); err != nil {
		t.Fatal(err)
	}
	originalDB := []byte{0x00, 0x01, 'n', 'o', 't', '-', 'a', '-', 'r', 'e', 'a', 'l', '-', 's', 'q', 'l', 'i', 't', 'e', '-', 'f', 'i', 'l', 'e'}
	if err := os.WriteFile(s.DB, originalDB, 0o644); err != nil {
		t.Fatal(err)
	}

	calls := installSuccessMocks(t, false, nil, "/usr/bin/qmd")
	if err := Lifecycle(p, "recover", io.Discard); err != nil {
		t.Fatalf("Lifecycle(recover) = %v", err)
	}

	gotConfig, err := os.ReadFile(s.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotConfig) != string(originalConfig) {
		t.Fatalf("config file changed:\n got: %s\nwant: %s", gotConfig, originalConfig)
	}
	gotDB, err := os.ReadFile(s.DB)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotDB) != string(originalDB) {
		t.Fatalf("db file changed: got %v, want %v", gotDB, originalDB)
	}

	var subcommands []string
	for _, call := range *calls {
		subcommands = append(subcommands, call[3])
	}
	if containsStr(subcommands, "update") || containsStr(subcommands, "embed") {
		t.Fatalf("subcommands = %v, want no update/embed", subcommands)
	}
	// Not running yet: recover still brings the server up.
	if subcommands[len(subcommands)-1] != "mcp" {
		t.Fatalf("last subcommand = %q, want mcp", subcommands[len(subcommands)-1])
	}
}

func TestRecoverWithOurServerAlreadyRunningStartsNothing(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s.configValue())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ConfigFile, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.DB, []byte("db-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	calls := installSuccessMocks(t, true, nil, "/usr/bin/qmd")
	if err := Lifecycle(p, "recover", io.Discard); err != nil {
		t.Fatalf("Lifecycle(recover) = %v", err)
	}

	var subcommands []string
	for _, call := range *calls {
		subcommands = append(subcommands, call[3])
	}
	for _, unexpected := range []string{"mcp", "update", "embed"} {
		if containsStr(subcommands, unexpected) {
			t.Fatalf("subcommands = %v, did not expect %q", subcommands, unexpected)
		}
	}
}

func TestRecoverWithMissingConfigOrDBRaisesPointingToInit(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	saved := lookPath
	t.Cleanup(func() { lookPath = saved })
	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }

	err := Lifecycle(p, "recover", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "orai wiki init") {
		t.Fatalf("Lifecycle(recover) error = %v, want mention of `orai wiki init`", err)
	}
}

// ---- refresh/init refuse while our server is running -----------------------------

func TestInitRefusesWhileServerRunningAndRunsNoQMDCommands(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	calls := installSuccessMocks(t, true, nil, "/usr/bin/qmd")

	err := Lifecycle(p, "init", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "safe boundary") {
		t.Fatalf("Lifecycle(init) error = %v, want mention of a safe boundary", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("calls = %v, want none", *calls)
	}
}

func TestRefreshRefusesWhileServerRunningAndRunsNoQMDCommands(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s.configValue())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ConfigFile, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.DB, []byte("db-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	calls := installSuccessMocks(t, true, nil, "/usr/bin/qmd")
	err = Lifecycle(p, "refresh", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "safe boundary") {
		t.Fatalf("Lifecycle(refresh) error = %v, want mention of a safe boundary", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("calls = %v, want none", *calls)
	}
}

// ---- a port serving another project's index is left untouched ---------------------

type otherProjectClient struct{}

func (otherProjectClient) Connect() error { return nil }

func (otherProjectClient) Tool(string, map[string]any) (map[string]any, error) {
	return map[string]any{"structuredContent": map[string]any{
		"collections": []any{map[string]any{"name": "docs", "path": "/elsewhere"}},
	}}, nil
}

func TestPortServingAnotherIndexRaisesAndRunsNoQMDCommands(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s.configValue())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.ConfigFile, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.DB, []byte("db-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	savedLookPath, savedReachability := lookPath, reachability
	t.Cleanup(func() { lookPath, reachability = savedLookPath, savedReachability })
	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }
	reachability = func(int) (string, string) { return "open", "" }
	setClient(t, otherProjectClient{})

	var calls [][]string
	savedRunCommand := runCommand
	t.Cleanup(func() { runCommand = savedRunCommand })
	runCommand = func(_ io.Writer, argv []string, _ *Settings, _ bool) (string, error) {
		calls = append(calls, append([]string(nil), argv...))
		return "", nil
	}

	err = Lifecycle(p, "recover", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "Left untouched") {
		t.Fatalf("Lifecycle(recover) error = %v, want mention of Left untouched", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %v, want none", calls)
	}
}

// ---- verify() never reports a failed probe as success ------------------------------

func TestVerifyRaisesWhenAnyCheckIsNotHealthy(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	saved := probe
	t.Cleanup(func() { probe = saved })
	probe = func(*Settings, bool) []doctor.Check {
		return []doctor.Check{doctor.New("wiki.server", doctor.Blocked, "connection refused", "")}
	}

	err := verify(s, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not verified") {
		t.Fatalf("verify() error = %v, want mention of `not verified`", err)
	}
}

// ---- stop ---------------------------------------------------------------------------

func TestStopRunsOnlyMcpStop(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	calls := installSuccessMocks(t, false, nil, "/usr/bin/qmd")

	if err := Lifecycle(p, "stop", io.Discard); err != nil {
		t.Fatalf("Lifecycle(stop) = %v", err)
	}

	want := [][]string{{"/usr/bin/qmd", "--index", s.Index, "mcp", "stop"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("calls = %v, want %v", *calls, want)
	}
}

// ---- check delegates to verify (real probe/ownServerRunning bypassed by "check") ---

func TestCheckActionNeverInspectsQMDOnPath(t *testing.T) {
	// "check" is read-only against an already-running server: it must not require qmd
	// on PATH at all.
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	saved := verify
	t.Cleanup(func() { verify = saved })
	called := false
	verify = func(got *Settings, _ io.Writer) error {
		called = true
		if got.Index != s.Index {
			t.Fatalf("verify called with settings for a different project")
		}
		return nil
	}
	savedLookPath := lookPath
	t.Cleanup(func() { lookPath = savedLookPath })
	lookPath = func(string) (string, error) { t.Fatal("check must not look up qmd on PATH"); return "", nil }

	if err := Lifecycle(p, "check", io.Discard); err != nil {
		t.Fatalf("Lifecycle(check) = %v", err)
	}
	if !called {
		t.Fatalf("verify was never called")
	}
}

func TestLifecycleRefusesWhenCollectionFolderIsMissing(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	if err := os.RemoveAll(filepath.Join(p.Root, "docs")); err != nil {
		t.Fatal(err)
	}
	err := Lifecycle(p, "check", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "folder is missing") {
		t.Fatalf("Lifecycle(check) error = %v, want mention of a missing folder", err)
	}
}

func TestLifecycleRejectsConcurrentSetupCommandsViaTheSameLock(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(s.Directory, "setup.lock")
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	installSuccessMocks(t, false, nil, "/usr/bin/qmd")
	err = Lifecycle(p, "stop", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "Another orai wiki command is running") {
		t.Fatalf("Lifecycle(stop) error = %v, want mention of a concurrent command", err)
	}
}
