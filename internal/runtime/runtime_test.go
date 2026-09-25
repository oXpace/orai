package runtime

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
)

const (
	sid      = "ed936671-e8a5-4d6d-a38f-c5bcc152ab10"
	otherSID = "741e3b28-1050-49ce-93d9-847ecf8334c3"
	twoRoles = `schema = 1
session = "orai"

[roles.lead]
provider = "codex"
worktree = "."
guide = "docs/lead.md"
model = "gpt-test"
effort = "medium"

[roles.dev]
provider = "claude"
worktree = "."
guide = "docs/dev.md"
model = "opus"
effort = "high"
`
)

func writeProject(t *testing.T, dir, text string) *project.Project {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "orai.toml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func clearOraiEnv(t *testing.T) {
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "ORAI_") || strings.HasPrefix(key, "AM_") {
			t.Setenv(key, "")
			os.Unsetenv(key)
		}
	}
}

type fixture struct {
	p     *project.Project
	files state.RoleFiles
}

func newFixture(t *testing.T) fixture {
	clearOraiEnv(t)
	p := writeProject(t, t.TempDir(), twoRoles)
	return fixture{p: p, files: p.RoleFiles("lead")}
}

func (f fixture) state(t *testing.T, changes map[string]any) map[string]any {
	t.Helper()
	value := map[string]any{"provider": "codex", "cwd": f.p.Root, "nonce": "run-one", "status": "starting", "session_id": sid}
	for k, v := range changes {
		if v == nil {
			delete(value, k)
			value[k] = nil
		} else {
			value[k] = v
		}
	}
	if err := state.WriteJSON(f.files.State(), value); err != nil {
		t.Fatal(err)
	}
	stored, _ := state.ReadJSON(f.files.State())
	return stored
}

func (f fixture) event(changes map[string]string) string {
	value := map[string]string{"hook_event_name": "SessionStart", "cwd": f.p.Root, "session_id": sid,
		"transcript_path": filepath.Join(f.p.Root, "transcript.jsonl")}
	for k, v := range changes {
		value[k] = v
	}
	data, _ := json.Marshal(value)
	return string(data)
}

func (f fixture) capture(t *testing.T, event, nonce, role string) (string, error) {
	t.Setenv("ORAI_ROLE", role)
	t.Setenv("ORAI_RUN_NONCE", nonce)
	var out bytes.Buffer
	err := Capture(f.p, strings.NewReader(event), &out)
	return out.String(), err
}

func TestProviderResumeUsesExactIDAndRoleModel(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"lead", "dev"} {
		role := f.p.Config.Roles[name]
		args, _ := ProviderArgs(f.p, role, sid)
		want := "resume"
		if role.Provider == "claude" {
			want = "--resume"
		}
		if args[1] != want || args[2] != sid {
			t.Fatalf("%s resume args %v", name, args[:3])
		}
		if i := index(args, "--model"); i < 0 || args[i+1] != role.Model {
			t.Fatalf("%s model missing", name)
		}
		if index(args, "--last") >= 0 || !strings.Contains(args[len(args)-1], "기존 대화") {
			t.Fatalf("%s must resume exactly with a resume prompt", name)
		}
	}
}

func index(args []string, value string) int {
	for i, a := range args {
		if a == value {
			return i
		}
	}
	return -1
}

func TestModelAndEffortAreOptional(t *testing.T) {
	clearOraiEnv(t)
	p := writeProject(t, t.TempDir(), "schema = 1\n[roles.a]\nprovider = \"codex\"\n[roles.b]\nprovider = \"claude\"\n")
	for _, role := range p.Config.Roles {
		args, _ := ProviderArgs(p, role, "")
		for _, a := range args {
			if a == "--model" || a == "--effort" || strings.HasPrefix(a, "model_reasoning_effort") {
				t.Fatalf("%s has %s without config", role.Name, a)
			}
		}
	}
}

func TestFreshClaudeBindsUUIDAndLoadsTheChannelFromThisBinary(t *testing.T) {
	f := newFixture(t)
	args, bound := ProviderArgs(f.p, f.p.Config.Roles["dev"], "")
	if args[1] != "--session-id" || args[2] != bound {
		t.Fatalf("args %v bound %s", args[:3], bound)
	}
	if _, err := CanonicalUUID(bound); err != nil {
		t.Fatal(err)
	}
	var servers struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	_ = json.Unmarshal([]byte(args[index(args, "--mcp-config")+1]), &servers)
	channel := servers.MCPServers["orai"]
	if channel["command"] != Executable() || channel["args"].([]any)[0] != "_channel" {
		t.Fatalf("channel %v", channel)
	}
	if args[index(args, "--dangerously-load-development-channels")+1] != "server:orai" {
		t.Fatal("channel not enabled")
	}
	if !strings.Contains(args[index(args, "--settings")+1], "SessionStart") {
		t.Fatal("hook missing")
	}
	if _, resumed := ProviderArgs(f.p, f.p.Config.Roles["dev"], sid); resumed != "" {
		t.Fatal("resume must not bind a new id")
	}
}

func TestHookRunsThisBinaryWithTheProject(t *testing.T) {
	f := newFixture(t)
	hook := HookCommand(f.p)
	if !strings.HasPrefix(hook, ShellQuote(Executable())+" --project ") || !strings.HasSuffix(hook, " _capture") {
		t.Fatalf("hook %q", hook)
	}
	out, err := exec.Command("sh", "-c", "printf '%s|' "+strings.TrimSuffix(strings.SplitN(hook, " ", 2)[1], " _capture")).Output()
	if err != nil || string(out) != "--project|"+f.p.Root+"|" {
		t.Fatalf("shell quoting: %q %v", out, err)
	}
	if ShellQuote("it's") != `'it'"'"'s'` {
		t.Fatal("quote escaping")
	}
}

func TestInjectedMCPServersReachBothProviders(t *testing.T) {
	f := newFixture(t)
	saved := MCPServers
	defer func() { MCPServers = saved }()
	MCPServers = func(_ *project.Project, provider string) map[string]map[string]any {
		if provider == "codex" {
			return map[string]map[string]any{"wiki-x": {"url": "http://127.0.0.1:18555/mcp"}}
		}
		return map[string]map[string]any{"wiki-x": {"type": "http", "url": "http://127.0.0.1:18555/mcp"}}
	}
	codex, _ := ProviderArgs(f.p, f.p.Config.Roles["lead"], "")
	if index(codex, `mcp_servers.wiki-x={"url" = "http://127.0.0.1:18555/mcp"}`) < 0 {
		t.Fatalf("codex args %v", codex)
	}
	claude, _ := ProviderArgs(f.p, f.p.Config.Roles["dev"], "")
	if !strings.Contains(claude[index(claude, "--mcp-config")+1], `"wiki-x":{"type":"http"`) {
		t.Fatal("claude mcp-config lacks the wiki")
	}
}

func TestFreshPromptBootstrapsOnceAndChannelFollowsProvider(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"lead", "dev"} {
		role := f.p.Config.Roles[name]
		args, _ := ProviderArgs(f.p, role, "")
		prompt := args[len(args)-1]
		if strings.Count(strings.Join(args, "\n"), "역할 지침을 읽고") != 1 || !strings.Contains(prompt, role.Guide) ||
			!strings.Contains(prompt, SkillPath(f.p)) || !strings.Contains(prompt, "orai msg inbox") {
			t.Fatalf("%s prompt %q", name, prompt)
		}
		if strings.Contains(prompt, "channel_ready") != (role.Provider == "claude") {
			t.Fatalf("%s channel_ready mismatch", name)
		}
	}
}

func TestCaptureValidIdentity(t *testing.T) {
	f := newFixture(t)
	f.state(t, map[string]any{"session_id": nil})
	if _, err := f.capture(t, f.event(nil), "run-one", "lead"); err != nil {
		t.Fatal(err)
	}
	saved, _ := state.ReadJSON(f.files.State())
	if saved["session_id"] != sid || saved["status"] != "running" || saved["captured_at"] == nil {
		t.Fatalf("state %v", saved)
	}
}

func TestHookRestoresIdentityWithoutRepeatingBootstrap(t *testing.T) {
	f := newFixture(t)
	for _, source := range []string{"startup", "resume", "compact", ""} {
		f.state(t, nil)
		out, err := f.capture(t, f.event(map[string]string{"source": source}), "run-one", "lead")
		if err != nil {
			t.Fatal(err)
		}
		var hook struct {
			HookSpecificOutput struct{ AdditionalContext string } `json:"hookSpecificOutput"`
		}
		_ = json.Unmarshal([]byte(out), &hook)
		text := hook.HookSpecificOutput.AdditionalContext
		if !strings.Contains(text, "lead") || !strings.Contains(text, f.p.Root) || strings.Contains(text, "읽어라") ||
			len([]rune(text)) >= 300 {
			t.Fatalf("%s context %q", source, text)
		}
	}
}

func TestCaptureRejectsStaleNonceCwdIdentityOrStoppedRun(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		changes map[string]any
		event   map[string]string
		nonce   string
	}{
		{nil, nil, "old-run"},
		{nil, map[string]string{"cwd": filepath.Join(f.p.Root, "other")}, "run-one"},
		{nil, map[string]string{"session_id": otherSID}, "run-one"},
		{map[string]any{"status": "stopped"}, nil, "run-one"},
		{nil, map[string]string{"session_id": "not-a-uuid"}, "run-one"},
	}
	for i, tc := range cases {
		original := f.state(t, tc.changes)
		if _, err := f.capture(t, f.event(tc.event), tc.nonce, "lead"); err == nil {
			t.Fatalf("case %d accepted", i)
		}
		if after, _ := state.ReadJSON(f.files.State()); !equalJSON(after, original) {
			t.Fatalf("case %d changed state", i)
		}
	}
	if _, err := f.capture(t, f.event(nil), "run-one", "ghost"); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("unconfigured role: %v", err)
	}
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func TestCaptureIgnoresOtherEventsAndNonOraiSessions(t *testing.T) {
	f := newFixture(t)
	original := f.state(t, nil)
	if _, err := f.capture(t, f.event(map[string]string{"hook_event_name": "Stop"}), "run-one", "lead"); err != nil {
		t.Fatal(err)
	}
	clearOraiEnv(t)
	if err := Capture(f.p, strings.NewReader("not json"), &bytes.Buffer{}); err != nil {
		t.Fatal("outside a role session the hook is a no-op:", err)
	}
	if after, _ := state.ReadJSON(f.files.State()); !equalJSON(after, original) {
		t.Fatal("state changed")
	}
}

func TestSavedSessionRequiresExactRecordedTranscript(t *testing.T) {
	f := newFixture(t)
	transcript := filepath.Join(f.p.Root, "recorded.jsonl")
	_ = os.WriteFile(transcript, []byte("{}\n"), 0o600)
	saved := f.state(t, map[string]any{"transcript": transcript})
	role := f.p.Config.Roles["lead"]
	if err := ValidateSaved(saved, role, f.p.Root); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(transcript)
	_ = os.WriteFile(filepath.Join(f.p.Root, "newer.jsonl"), []byte("{}\n"), 0o600)
	if err := ValidateSaved(saved, role, f.p.Root); err == nil || !strings.Contains(err.Error(), "transcript is unavailable") {
		t.Fatalf("latest transcript must not be guessed: %v", err)
	}
	for _, changes := range []map[string]any{{"provider": "claude"}, {"cwd": "/wrong"}, {"session_id": nil}} {
		if err := ValidateSaved(f.state(t, changes), role, f.p.Root); err == nil {
			t.Fatalf("accepted %v", changes)
		}
	}
}

func TestLockBlocksSecondLaunchAndIsPerProject(t *testing.T) {
	f := newFixture(t)
	if state.Locked(f.files) {
		t.Fatal("locked before start")
	}
	first, err := state.RoleLock(f.files)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Locked(f.files) {
		t.Fatal("not locked")
	}
	if _, err := state.RoleLock(f.files); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second lock: %v", err)
	}
	other := writeProject(t, filepath.Join(t.TempDir(), "other"), twoRoles)
	if lock, err := state.RoleLock(other.RoleFiles("lead")); err != nil {
		t.Fatal("another project's role must not be blocked:", err)
	} else {
		lock.Close()
	}
	first.Close()
	if state.Locked(f.files) {
		t.Fatal("lock not released")
	}
}

func TestDryRunChangesNothingAndLaunchesNothing(t *testing.T) {
	f := newFixture(t)
	// Only git on PATH: a dry run must not need the provider binaries.
	bin := t.TempDir()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	before := listTree(f.p.Root)
	var out bytes.Buffer
	code, err := Launch(f.p, "lead", false, true, &out)
	if err != nil || code != 0 {
		t.Fatal(code, err)
	}
	if after := listTree(f.p.Root); strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("dry run wrote files: %v", after)
	}
	var plan map[string]any
	_ = json.Unmarshal(out.Bytes(), &plan)
	if plan["project"] != f.p.Root || plan["mail_root"] != f.p.MailRoot() || plan["resume"] != false {
		t.Fatalf("plan %v", plan)
	}
	if command, _ := plan["command"].([]any); len(command) == 0 || command[0] != "codex" {
		t.Fatalf("command %v", plan["command"])
	}
}

func listTree(root string) []string {
	var paths []string
	_ = filepath.Walk(root, func(path string, _ os.FileInfo, _ error) error {
		paths = append(paths, path)
		return nil
	})
	return paths
}

func TestUnknownRoleAndNestedLaunchAreRefused(t *testing.T) {
	f := newFixture(t)
	if code, err := Launch(f.p, "senior", false, true, &bytes.Buffer{}); code != 2 || err == nil ||
		!strings.Contains(err.Error(), `unknown role "senior"`) {
		t.Fatal(code, err)
	}
	t.Setenv("ORAI_ROLE", "dev")
	t.Setenv("ORAI_PROJECT", f.p.Root)
	t.Setenv("ORAI_SESSION", "orai")
	if _, err := Launch(f.p, "lead", false, true, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "already inside") {
		t.Fatal(err)
	}
}

func TestDeclaredBranchIsEnforced(t *testing.T) {
	clearOraiEnv(t)
	dir := t.TempDir()
	if err := exec.Command("git", "init", "-q", "-b", "trunk", dir).Run(); err != nil {
		t.Fatal(err)
	}
	p := writeProject(t, dir, strings.Replace(twoRoles, `provider = "codex"`, "provider = \"codex\"\nbranch = \"release\"", 1))
	role := p.Config.Roles["lead"]
	if err := CheckWorktree(role, p.Root); err == nil || !strings.Contains(err.Error(), "must be on branch release") {
		t.Fatal(err)
	}
	plain := writeProject(t, t.TempDir(), twoRoles)
	if err := CheckWorktree(role, plain.Root); err == nil || !strings.Contains(err.Error(), "not a Git worktree") {
		t.Fatal(err)
	}
	sub := writeProject(t, filepath.Join(dir, "app"), twoRoles)
	if err := CheckWorktree(sub.Config.Roles["dev"], sub.Root); err != nil {
		t.Fatal("a project in a repo subdirectory may use its root as worktree:", err)
	}
}

func TestChildEnvDropsForeignContextAndBindsIdentity(t *testing.T) {
	f := newFixture(t)
	t.Setenv("AM_ME", "wrong")
	t.Setenv("AM_ROOT", "/wrong")
	t.Setenv("AMQ_GLOBAL_ROOT", "/other")
	t.Setenv("ORAI_PROJECT", "/other")
	env := map[string]string{}
	for _, kv := range childEnv(f.p, f.p.Config.Roles["lead"], "n1") {
		key, value, _ := strings.Cut(kv, "=")
		if _, dup := env[key]; dup {
			t.Fatalf("duplicate %s", key)
		}
		env[key] = value
	}
	if env["AM_ME"] != "lead" || env["AM_ROOT"] != f.p.MailRoot() || env["ORAI_PROJECT"] != f.p.Root ||
		env["ORAI_MAIL_ROOT"] != f.p.MailRoot() || env["ORAI_RUN_NONCE"] != "n1" {
		t.Fatalf("env %v", env)
	}
	if _, leaked := env["AMQ_GLOBAL_ROOT"]; leaked {
		t.Fatal("AMQ_GLOBAL_ROOT leaked")
	}
}

func TestStatusCountsPendingWithoutConsuming(t *testing.T) {
	f := newFixture(t)
	root := mail.Root(f.p.MailRoot())
	if err := mail.Init(root, f.p.Config.Handles()); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Send("user", []string{"lead"}, "hi", mail.SendOptions{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	for i := 0; i < 2; i++ {
		out.Reset()
		if err := Status(f.p, &out); err != nil {
			t.Fatal(err)
		}
	}
	var rows []RoleStatus
	_ = json.Unmarshal(out.Bytes(), &rows)
	if len(rows) != 2 || rows[1].Role != "lead" || rows[1].Pending == nil || *rows[1].Pending != 1 || rows[1].Running {
		t.Fatalf("status %s", out.String())
	}
}

func TestDiagnoseReportsRolesMailAndMissingWorktree(t *testing.T) {
	clearOraiEnv(t)
	dir := t.TempDir()
	_ = exec.Command("git", "init", "-q", "-b", "trunk", dir).Run()
	p := writeProject(t, dir, strings.Replace(twoRoles, "worktree = \".\"\nguide = \"docs/dev.md\"", "worktree = \".worktrees/dev\"\nguide = \"docs/dev.md\"", 1))
	checks := Diagnose(p)
	byName := map[string]string{}
	for _, c := range checks {
		byName[c.Component] = c.Status + "|" + c.NextAction
	}
	if !strings.HasPrefix(byName["mail"], "not-ready") || !strings.HasPrefix(byName["role.lead"], "healthy") ||
		!strings.Contains(byName["role.dev"], "git worktree add .worktrees/dev") {
		t.Fatalf("checks %v", byName)
	}
	noRoles := writeProject(t, t.TempDir(), "schema = 1\n")
	if got := Diagnose(noRoles); got[1].Status != "not-configured" {
		t.Fatalf("mail without roles: %+v", got[1])
	}
}
