package runtime

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/providers"
	"github.com/oXpace/orai/internal/state"
)

// Contract tests for following /clear (openspec role-sessions).

func (f fixture) history(t *testing.T) []map[string]any {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(f.files.Dir), "history", "lead-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var values []map[string]any
	for _, path := range paths {
		value, err := state.ReadJSON(path)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	return values
}

func (f fixture) clearEvent() string {
	return f.event(map[string]string{"source": "clear", "session_id": otherSID,
		"transcript_path": filepath.Join(f.p.Root, "cleared.jsonl")})
}

// Scenario: 같은 실행에서 clear한다
func TestClearReplacesIdentityAndKeepsThePreviousOne(t *testing.T) {
	f := newFixture(t)
	before := f.state(t, map[string]any{"status": "running", "captured_at": 1.0,
		"transcript": filepath.Join(f.p.Root, "transcript.jsonl")})
	if _, err := f.capture(t, f.clearEvent(), "run-one", "lead"); err != nil {
		t.Fatal(err)
	}
	after, _ := state.ReadJSON(f.files.State())
	// The transcript file may not exist yet when the hook runs; resume checks it.
	if after["session_id"] != otherSID || after["transcript"] != filepath.Join(f.p.Root, "cleared.jsonl") ||
		after["status"] != "running" || after["nonce"] != "run-one" {
		t.Fatalf("state %v", after)
	}
	if history := f.history(t); len(history) != 1 || !equalJSON(history[0], before) {
		t.Fatalf("history %v", history)
	}

	// A late hook of the conversation before the clear cannot take the identity back.
	for _, source := range []string{"startup", "resume", "compact"} {
		if _, err := f.capture(t, f.event(map[string]string{"source": source}), "run-one", "lead"); err == nil {
			t.Fatalf("late %s hook accepted", source)
		}
	}
	if late, _ := state.ReadJSON(f.files.State()); !equalJSON(late, after) {
		t.Fatalf("late hook changed state: %v", late)
	}
}

// Scenario: 같은 실행에서 clear한다 (이후 Codex 알림은 새 ID로 간다)
func TestClearMovesCodexNotificationsToTheNewThread(t *testing.T) {
	f := newFixture(t)
	f.state(t, map[string]any{"status": "running", "captured_at": 1.0})
	pending := []string{"one"}
	var threads, messages []string
	notifier := &providers.CodexNotifier{Files: f.files, Nonce: "run-one",
		Pending: func() ([]string, error) { return pending, nil },
		Queue: func(thread, message string) error {
			threads, messages = append(threads, thread), append(messages, message)
			return nil
		}}
	notifier.Step()
	if _, err := f.capture(t, f.clearEvent(), "run-one", "lead"); err != nil {
		t.Fatal(err)
	}
	pending = append(pending, "two")
	notifier.Step()
	if strings.Join(threads, ",") != sid+","+otherSID || messages[1] != providers.Notice([]string{"two"}) {
		t.Fatalf("threads %v messages %v", threads, messages)
	}
}

// Scenario: 시작 hook보다 빠른 clear
func TestClearBeforeTheFirstCaptureIsFollowed(t *testing.T) {
	f := newFixture(t)
	f.state(t, map[string]any{"captured_at": nil}) // starting, with the ID bound at launch
	if _, err := f.capture(t, f.clearEvent(), "run-one", "lead"); err != nil {
		t.Fatal(err)
	}
	if after, _ := state.ReadJSON(f.files.State()); after["session_id"] != otherSID || after["status"] != "running" || after["captured_at"] == nil {
		t.Fatalf("state %v", after)
	}
}

// Scenarios: 기록된 ID와 다른 ID, 이전 실행이 남긴 hook, 다른 폴더에서 온 hook, transcript 경로가 없는 clear
func TestClearDoesNotLoosenTheOtherCaptureRules(t *testing.T) {
	f := newFixture(t)
	other := map[string]string{"session_id": otherSID}
	with := func(changes ...string) map[string]string {
		event := map[string]string{"source": "clear"}
		for key, value := range other {
			event[key] = value
		}
		for i := 0; i < len(changes); i += 2 {
			event[changes[i]] = changes[i+1]
		}
		return event
	}
	cases := []struct {
		name   string
		status string
		event  map[string]string
		nonce  string
	}{
		{"startup (Codex /new)", "running", with("source", "startup"), "run-one"},
		{"resume", "running", with("source", "resume"), "run-one"},
		{"compact", "running", with("source", "compact"), "run-one"},
		{"fork", "running", with("source", "fork"), "run-one"},
		{"no source", "running", with("source", ""), "run-one"},
		{"stale nonce", "running", with(), "old-run"},
		{"other cwd", "running", with("cwd", filepath.Join(f.p.Root, "other")), "run-one"},
		{"stopped run", "stopped", with(), "run-one"},
		{"not a UUID", "running", with("session_id", "not-a-uuid"), "run-one"},
		{"no transcript", "running", with("transcript_path", ""), "run-one"},
	}
	for _, tc := range cases {
		before := f.state(t, map[string]any{"status": tc.status, "captured_at": 1.0, "transcript": "before.jsonl"})
		if _, err := f.capture(t, f.event(tc.event), tc.nonce, "lead"); err == nil {
			t.Fatalf("%s: accepted", tc.name)
		}
		if after, _ := state.ReadJSON(f.files.State()); !equalJSON(after, before) {
			t.Fatalf("%s: state changed to %v", tc.name, after)
		}
		if history := f.history(t); len(history) != 0 {
			t.Fatalf("%s: archived %v", tc.name, history)
		}
	}
}

// The previous identity is never dropped: without a history copy there is no replacement.
func TestClearKeepsIdentityWhenHistoryCannotBeWritten(t *testing.T) {
	f := newFixture(t)
	before := f.state(t, map[string]any{"status": "running", "captured_at": 1.0})
	if err := os.WriteFile(filepath.Join(filepath.Dir(f.files.Dir), "history"), []byte("not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.capture(t, f.clearEvent(), "run-one", "lead"); err == nil {
		t.Fatal("replaced without history")
	}
	if after, _ := state.ReadJSON(f.files.State()); !equalJSON(after, before) {
		t.Fatalf("state %v", after)
	}
}

// Scenario: clear한 뒤 다시 실행한다
func TestRunAfterClearResumesTheClearedConversation(t *testing.T) {
	f := newFixture(t)
	f.state(t, map[string]any{"status": "running", "captured_at": 1.0})
	if _, err := f.capture(t, f.clearEvent(), "run-one", "lead"); err != nil {
		t.Fatal(err)
	}
	stopped, _ := state.ReadJSON(f.files.State())
	stopped["status"] = "stopped"
	if err := state.WriteJSON(f.files.State(), stopped); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.p.Root, "cleared.jsonl"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code, err := Launch(f.p, "lead", false, true, &out); err != nil || code != 0 {
		t.Fatal(code, err)
	}
	var plan struct {
		Resume    bool     `json:"resume"`
		SessionID string   `json:"session_id"`
		Command   []string `json:"command"`
	}
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Resume || plan.SessionID != otherSID || !strings.Contains(strings.Join(plan.Command, " "), "resume "+otherSID) {
		t.Fatalf("plan %+v", plan)
	}
	if history := f.history(t); len(history) != 1 || history[0]["session_id"] != sid {
		t.Fatalf("history %v", history)
	}
}

// Scenario: clear 뒤 첫 응답 전
func TestClearContextPointsAtGuidesAndInbox(t *testing.T) {
	f := newFixture(t)
	f.state(t, map[string]any{"status": "running", "captured_at": 1.0})
	out, err := f.capture(t, f.clearEvent(), "run-one", "lead")
	if err != nil {
		t.Fatal(err)
	}
	var hook struct {
		HookSpecificOutput struct{ HookEventName, AdditionalContext string }
	}
	if err := json.Unmarshal([]byte(out), &hook); err != nil {
		t.Fatal(err)
	}
	text := hook.HookSpecificOutput.AdditionalContext
	for _, want := range []string{"역할=lead", "AGENTS.md", "역할 지침=docs/lead.md", "읽", "`orai inbox`"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "하던 작업을 이어간다") || strings.Count(text, f.p.Root) != 1 || len([]rune(text)) >= 300 ||
		hook.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatalf("context %q", text)
	}

	// A role without a guide is not told to read one.
	role := f.p.Config.Roles["lead"]
	role.Guide = ""
	f.p.Config.Roles["lead"] = role
	f.state(t, map[string]any{"status": "running", "captured_at": 1.0})
	if out, err = f.capture(t, f.clearEvent(), "run-one", "lead"); err != nil || strings.Contains(out, "역할 지침") {
		t.Fatalf("no guide: %v %s", err, out)
	}
}
