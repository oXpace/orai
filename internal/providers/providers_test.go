package providers

import (
	"errors"
	"strings"
	"testing"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/state"
)

const sid = "ed936671-e8a5-4d6d-a38f-c5bcc152ab10"

type harness struct {
	files   state.RoleFiles
	pending [][]string
	queue   []error
	calls   []string
}

func newHarness(t *testing.T, captured bool, nonce string) *harness {
	h := &harness{files: state.RoleFiles{Dir: t.TempDir(), Role: "lead"}}
	value := map[string]any{"nonce": nonce, "session_id": sid, "captured_at": nil}
	if captured {
		value["captured_at"] = 1.0
	}
	if err := state.WriteJSON(h.files.State(), value); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) notifier() *CodexNotifier {
	return &CodexNotifier{Files: h.files, Nonce: "run-one",
		Pending: func() ([]string, error) {
			if len(h.pending) == 0 {
				return nil, nil
			}
			next := h.pending[0]
			h.pending = h.pending[1:]
			return next, nil
		},
		Queue: func(thread, message string) error {
			if thread != sid {
				return errors.New("wrong thread " + thread)
			}
			h.calls = append(h.calls, message)
			if len(h.queue) > 0 {
				err := h.queue[0]
				h.queue = h.queue[1:]
				return err
			}
			return nil
		}}
}

func delivery(t *testing.T, h *harness) map[string]any {
	value, err := state.ReadJSON(h.files.CodexDelivery())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestQueueNotifiesExactSessionWithoutRepeating(t *testing.T) {
	h := newHarness(t, true, "run-one")
	h.pending = [][]string{{"message-one"}, {"message-one"}}
	n := h.notifier()
	n.Step()
	n.Step()
	if len(h.calls) != 1 || h.calls[0] != "오라이: 새 메시지\nIDs: message-one" {
		t.Fatalf("calls %q", h.calls)
	}
	if delivery(t, h)["ready"] != true {
		t.Fatal("not ready")
	}
}

func TestQueueOnlyNewIDsAndRetriesFailedDelivery(t *testing.T) {
	h := newHarness(t, true, "run-one")
	h.pending = [][]string{{"a"}, {"a"}, {"a", "b"}, {"a", "b"}, {}, {"c"}}
	h.queue = []error{errors.New("offline")}
	n := h.notifier()
	for range 6 {
		n.Step()
	}
	want := []string{"IDs: a", "IDs: a", "IDs: b", "IDs: c"}
	if len(h.calls) != len(want) {
		t.Fatalf("calls %q", h.calls)
	}
	for i, suffix := range want {
		if !strings.HasSuffix(h.calls[i], suffix) || strings.Contains(h.calls[i], "AGENTS") {
			t.Fatalf("call %d %q", i, h.calls[i])
		}
	}
	if delivery(t, h)["ready"] != true {
		t.Fatal("not ready after recovery")
	}
}

func TestQueueWaitsForMatchingCapture(t *testing.T) {
	for _, tc := range []struct {
		captured bool
		nonce    string
	}{{true, "old-run"}, {false, "run-one"}} {
		h := newHarness(t, tc.captured, tc.nonce)
		h.pending = [][]string{{"x"}}
		h.notifier().Step()
		if len(h.calls) != 0 || len(h.pending) != 1 {
			t.Fatalf("%+v: delivered before capture", tc)
		}
	}
}

func TestQueueFailureReportsErrorAndKeepsMailForRetry(t *testing.T) {
	h := newHarness(t, true, "run-one")
	h.pending = [][]string{{"m"}}
	h.queue = []error{errors.New("queue unavailable")}
	h.notifier().Step()
	d := delivery(t, h)
	if d["ready"] != false || d["last_error"] != "queue unavailable" {
		t.Fatalf("delivery %v", d)
	}
}

func TestCodexArgsInjectHookAndServers(t *testing.T) {
	role := config.Role{Name: "lead", Provider: "codex", Effort: "medium"}
	args := CodexArgs(role, "", "prompt", "hook cmd", "/root", map[string]map[string]any{"wiki-x": {"url": "u"}})
	joined := strings.Join(args, "\n")
	for _, want := range []string{`model_reasoning_effort="medium"`, `hooks.SessionStart=[{"hooks" = [{"command" = "hook cmd"`,
		`mcp_servers.wiki-x={"url" = "u"}`, "--add-dir\n/root\nprompt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, args)
		}
	}
}
