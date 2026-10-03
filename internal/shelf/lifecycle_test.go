// Covers the QMD lifecycle: init, recover, refresh, stop, and verify. Every `qmd`
// invocation goes through installSuccessMocks (a patched runCommand); reachability,
// ownServerRunning and the MCP client are patched too. No test binds or connects to a
// real QMD server or port 8181.
package shelf

import (
	"bytes"
	"encoding/json"
	"errors"
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
	// show (the declared collection), list (anything no longer declared), then the index.
	want := []string{"collection", "collection", "update", "embed", "mcp"}
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
	if err == nil || !strings.Contains(err.Error(), "orai shelf init") {
		t.Fatalf("Lifecycle(recover) error = %v, want mention of `orai shelf init`", err)
	}
}

// A collection declared in orai.toml after the index was created is added on refresh
// (existing collections untouched); recover, which never re-indexes, says what to run.
func TestCollectionAddedLaterIsAddedOnRefreshOnly(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{s.ConfigFile, s.DB} {
		if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	name := s.collectionNames()[0]
	calls := installSuccessMocks(t, false, nil, "/usr/bin/qmd")
	known := runCommand
	runCommand = func(out io.Writer, argv []string, s *Settings, capture bool) (string, error) {
		if capture && argv[len(argv)-2] == "show" {
			*calls = append(*calls, append([]string(nil), argv...))
			return "", errors.New("Collection not found")
		}
		return known(out, argv, s, capture)
	}

	err := Lifecycle(p, "recover", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "orai shelf stop && orai shelf refresh") {
		t.Fatalf("recover: %v", err)
	}
	*calls = nil
	if err := Lifecycle(p, "refresh", io.Discard); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	want := []string{"/usr/bin/qmd", "--index", s.Index, "collection", "add", s.Collections[name], "--name", name, "--mask", "**/*.md"}
	if len(*calls) < 2 || !reflect.DeepEqual((*calls)[1], want) {
		t.Fatalf("calls = %v, want the second to be %v", *calls, want)
	}
}

// ---- refresh/init refuse while our server is running -----------------------------

func TestInitRefusesWhileServerRunningAndRunsNoQMDCommands(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	calls := installSuccessMocks(t, true, nil, "/usr/bin/qmd")

	err := Lifecycle(p, "init", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "orai shelf stop && orai shelf init") {
		t.Fatalf("Lifecycle(init) error = %v, want the stop-then-init command", err)
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
	if err == nil || !strings.Contains(err.Error(), "orai shelf stop && orai shelf refresh") {
		t.Fatalf("Lifecycle(refresh) error = %v, want the stop-then-refresh command", err)
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
		return []doctor.Check{doctor.New("shelf.server", doctor.Blocked, "connection refused", "")}
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

// A shelf that was never built must say so (and how to get there), not report the
// missing server as "connection refused".
func TestCheckOnAnUninitializedShelfSaysWhatToRun(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	saved, savedVerify := lookPath, verify
	t.Cleanup(func() { lookPath, verify = saved, savedVerify })
	verify = func(*Settings, io.Writer) error { t.Fatal("verify must not run"); return nil }

	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }
	if err := Lifecycle(p, "check", io.Discard); err == nil || !strings.Contains(err.Error(), "orai shelf init") {
		t.Fatalf("with qmd installed: %v", err)
	}
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	if err := Lifecycle(p, "check", io.Discard); err == nil || !strings.Contains(err.Error(), "npm install -g @tobilu/qmd") {
		t.Fatalf("without qmd: %v", err)
	}
}

// ---- check delegates to verify (real probe/ownServerRunning bypassed by "check") ---

func TestCheckActionNeverInspectsQMDOnPath(t *testing.T) {
	// "check" is read-only against an already-running server: once the project shelf
	// has been initialized, it must not require qmd on PATH at all.
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{s.ConfigFile, s.DB} {
		if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
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
	if err == nil || !strings.Contains(err.Error(), "Another orai shelf command is running") {
		t.Fatalf("Lifecycle(stop) error = %v, want mention of a concurrent command", err)
	}
}

// ---- upgrade from 0.3 -----------------------------------------------------------

// Orai 0.3 kept the index in .orai/wiki. The first lifecycle command after the upgrade
// stops the server that index started, moves the folder as it is, and carries on: the
// config and DB are the same bytes, and nothing is re-indexed.
func TestRecoverAdoptsTheIndexOraiZeroThreeLeftInWiki(t *testing.T) {
	p := makeQMDProject(t, filepath.Join(t.TempDir(), "proj"), 0)
	s := NewSettings(p)
	legacy := s.LegacyDirectory()
	if filepath.Base(legacy) != "wiki" || filepath.Base(s.Directory) != "shelf" {
		t.Fatalf("directories: %s %s", legacy, s.Directory)
	}
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(s.configValue())
	if err := os.WriteFile(filepath.Join(legacy, filepath.Base(s.ConfigFile)), config, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, filepath.Base(s.DB)), []byte("db-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Doctor says what to run and changes nothing.
	savedLookPath := lookPath
	t.Cleanup(func() { lookPath = savedLookPath })
	lookPath = func(string) (string, error) { return "/usr/bin/qmd", nil }
	checks := Diagnose(p, false)
	if len(checks) != 2 || checks[1].Component != "shelf.registration" || checks[0].Status != doctor.NotReady || !strings.Contains(checks[0].NextAction, "orai shelf recover") {
		t.Fatalf("diagnosis before the move: %+v", checks)
	}
	if exists(s.Directory) {
		t.Fatal("diagnosis moved the index")
	}

	calls := installSuccessMocks(t, false, nil, "/usr/bin/qmd")
	var out bytes.Buffer
	if err := Lifecycle(p, "recover", &out); err != nil {
		t.Fatalf("Lifecycle(recover) = %v", err)
	}
	if exists(legacy) {
		t.Fatal(".orai/wiki is still there")
	}
	db, _ := os.ReadFile(s.DB)
	moved, _ := os.ReadFile(s.ConfigFile)
	if string(db) != "db-bytes" || string(moved) != string(config) {
		t.Fatalf("index changed while moving: %q %q", db, moved)
	}
	var subcommands []string
	for _, call := range *calls {
		subcommands = append(subcommands, strings.Join(call[3:], " "))
	}
	// The old server is stopped before the move; then recover starts it again.
	if subcommands[0] != "mcp stop" || containsStr(subcommands, "update") || containsStr(subcommands, "embed") {
		t.Fatalf("subcommands = %v", subcommands)
	}
	if !strings.Contains(out.String(), "Moving the index") {
		t.Fatalf("output does not say the index was moved:\n%s", out.String())
	}

	// `stop` on a 0.3 index moves it and stops the server exactly once, so the documented
	// `orai shelf stop && orai shelf refresh` does not break at the &&.
	if err := os.Rename(s.Directory, legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil { // left by an interrupted move
		t.Fatal(err)
	}
	*calls = nil
	if err := Lifecycle(p, "stop", io.Discard); err != nil {
		t.Fatalf("Lifecycle(stop) = %v", err)
	}
	if len(*calls) != 1 || strings.Join((*calls)[0][3:], " ") != "mcp stop" || !exists(s.DB) || exists(legacy) {
		t.Fatalf("stop on a 0.3 index: calls %v, moved %v", *calls, exists(s.DB))
	}

	// An index already in .orai/shelf is never replaced by a stray .orai/wiki.
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if s.legacyOnly() {
		t.Fatal("a second move would overwrite the current index")
	}
}

// ---- collections follow orai.toml -----------------------------------------------

func collectionProject(t *testing.T, collections string) (*Settings, *[][]string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	p := writeProject(t, root, "schema = 2\n[integrations.shelf]\ncollections = "+collections+"\n")
	s := NewSettings(p)
	for _, folder := range s.Collections {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(s.Directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{s.ConfigFile, s.DB} {
		if err := os.WriteFile(file, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return s, installSuccessMocks(t, false, nil, "/usr/bin/qmd")
}

func joined(calls *[][]string) []string {
	var lines []string
	for _, call := range *calls {
		lines = append(lines, strings.Join(call[3:], " "))
	}
	return lines
}

// Splitting `docs` into the folder itself and a subfolder: the index was built with
// `docs` taking everything and a collection orai.toml no longer has. recover names the
// difference; refresh re-registers what changed, removes what is gone, and re-indexes.
func TestRefreshMakesTheIndexMatchTheDeclaredCollections(t *testing.T) {
	s, calls := collectionProject(t, `{ core = { path = "docs", pattern = "*.md" }, adr = "docs/adr" }`)
	if s.Patterns["core"] != "*.md" || s.Patterns["adr"] != "**/*.md" {
		t.Fatalf("patterns: %v", s.Patterns)
	}
	indexed := runCommand
	runCommand = func(out io.Writer, argv []string, s *Settings, capture bool) (string, error) {
		last := argv[len(argv)-1]
		switch {
		case capture && last == "list":
			*calls = append(*calls, append([]string(nil), argv...))
			return "Collections (3):\n\nadr (qmd://adr/)\n  Pattern:  **/*.md\n\ncore (qmd://core/)\n  Pattern:  **/*.md\n\nnotes (qmd://notes/)\n  Files:    3\n", nil
		case capture && last == "core":
			*calls = append(*calls, append([]string(nil), argv...))
			return "Collection: core\n  Path:     " + s.Collections["core"] + "\n  Pattern:  **/*.md\n", nil
		}
		return indexed(out, argv, s, capture)
	}

	err := Lifecycle(s.Project, "recover", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "collection 'core'") || !strings.Contains(err.Error(), "orai shelf stop && orai shelf refresh") {
		t.Fatalf("recover: %v", err)
	}
	if lines := joined(calls); containsStr(lines, "collection remove core") {
		t.Fatalf("recover changed the index: %v", lines)
	}

	*calls = nil
	var out bytes.Buffer
	if err := Lifecycle(s.Project, "refresh", &out); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	lines := joined(calls)
	add := "collection add " + s.Collections["core"] + " --name core --mask *.md"
	remove, gone, update := indexOf(lines, "collection remove core"), indexOf(lines, "collection remove notes"), indexOf(lines, "update")
	if remove < 0 || indexOf(lines, add) != remove+1 || gone < 0 || update < gone || containsStr(lines, "collection remove adr") {
		t.Fatalf("refresh commands: %v", lines)
	}
	for _, want := range []string{"Re-registering: collection 'core'", "Removing collection 'notes'"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestOverlappingCollectionsAreNamed(t *testing.T) {
	for collections, want := range map[string][]string{
		`{ docs = "docs", adr = "docs/adr" }`:                                   {"docs and adr"},
		`{ core = { path = "docs", pattern = "*.md" }, adr = "docs/adr" }`:      nil,
		`{ a = "docs", b = "docs" }`:                                            {"a and b"},
		`{ docs = "docs", design = "design" }`:                                  nil,
		`{ adr = { path = "docs/adr", pattern = "**/*.md" }, guides = "docs" }`: {"guides and adr"},
	} {
		s, _ := collectionProject(t, collections)
		if got := s.overlapping(); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: overlapping = %v, want %v", collections, got, want)
		}
	}
}

// With part of a folder in one collection and a subfolder in another, the smoke
// document belongs to the deepest folder that holds it.
func TestSmokeDocumentBelongsToTheDeepestCollection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	p := writeProject(t, root, "schema = 2\n[integrations.shelf]\ncollections = { core = { path = \"docs\", pattern = \"*.md\" }, zadr = \"docs/adr\" }\n"+
		"[integrations.shelf.smoke]\nlex = \"a\"\nvec = \"b\"\nexpect = \"docs/adr/0001.md\"\n")
	if err := os.MkdirAll(filepath.Join(root, "docs/adr"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewSettings(p)
	if got := expectedURI(s); got != "zadr/0001.md" {
		t.Fatalf("expectedURI = %q", got)
	}
	s.Smoke.Expect = "docs/overview.md"
	if got := expectedURI(s); got != "core/overview.md" {
		t.Fatalf("expectedURI = %q", got)
	}
}
