package channel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/oXpace/orai/internal/state"
)

// fakeMail is the pending-ID source; it records reads so tests can prove the channel
// never looks at mail before the role is ready.
type fakeMail struct {
	mu    sync.Mutex
	ids   []string
	err   error
	reads int
	wake  func()
}

func (f *fakeMail) set(ids []string, err error) {
	f.mu.Lock()
	f.ids, f.err = ids, err
	wake := f.wake
	f.mu.Unlock()
	if wake != nil {
		wake()
	}
}

type session struct {
	t        *testing.T
	in       io.WriteCloser
	lines    chan map[string]any
	done     chan error
	finished chan struct{}
	mail     *fakeMail
	state    string
}

func start(t *testing.T) *session {
	t.Helper()
	fake := &fakeMail{}
	dir := t.TempDir()
	c := &Channel{Role: "reviewer-2", Nonce: "test-nonce", Version: "test", StateDir: dir,
		Pending: func() ([]string, error) {
			fake.mu.Lock()
			defer fake.mu.Unlock()
			fake.reads++
			return append([]string(nil), fake.ids...), fake.err
		},
		Watch: func(ctx context.Context, wake func()) error {
			fake.mu.Lock()
			fake.wake = wake
			fake.mu.Unlock()
			<-ctx.Done()
			return nil
		}}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &session{t: t, in: inW, lines: make(chan map[string]any, 64), done: make(chan error, 1), mail: fake,
		finished: make(chan struct{}), state: filepath.Join(dir, "reviewer-2.channel.json")}
	go func() { s.done <- c.Run(inR, outW); outW.Close(); close(s.finished) }()
	go func() {
		scanner := bufio.NewScanner(outR)
		for scanner.Scan() {
			var value map[string]any
			_ = json.Unmarshal(scanner.Bytes(), &value)
			s.lines <- value
		}
	}()
	// Registered after TempDir, so it runs first: stop the channel before its state dir goes away.
	t.Cleanup(func() {
		s.in.Close()
		select {
		case <-s.finished:
		case <-time.After(5 * time.Second):
			t.Error("channel did not stop")
		}
	})
	return s
}

func (s *session) send(raw string) { _, _ = s.in.Write([]byte(raw + "\n")) }

func (s *session) next(timeout time.Duration) map[string]any {
	select {
	case line := <-s.lines:
		return line
	case <-time.After(timeout):
		return nil
	}
}

func (s *session) rpc(method string, params any, id int) map[string]any {
	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		request["params"] = params
	}
	data, _ := json.Marshal(request)
	s.send(string(data))
	reply := s.next(3 * time.Second)
	if reply == nil || reply["id"] != float64(id) {
		s.t.Fatalf("%s: reply %v", method, reply)
	}
	return reply
}

func (s *session) initialize() {
	reply := s.rpc("initialize", map[string]any{"protocolVersion": "2025-11-25"}, 1)
	caps, _ := json.Marshal(reply["result"].(map[string]any)["capabilities"])
	if string(caps) != `{"experimental":{"claude/channel":{}},"tools":{}}` {
		s.t.Fatalf("capabilities %s", caps)
	}
	s.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
}

func (s *session) waitState(key string, want any) map[string]any {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if value, _ := state.ReadJSON(s.state); value != nil && value[key] == want {
			return value
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("state never had %s=%v", key, want)
	return nil
}

func TestReadyGateDedupAndEOF(t *testing.T) {
	s := start(t)
	s.mail.set([]string{"mail-1"}, nil)
	s.initialize()
	tools := s.rpc("tools/list", nil, 2)["result"].(map[string]any)["tools"].([]any)
	if tools[0].(map[string]any)["name"] != "channel_ready" {
		t.Fatal("channel_ready not listed")
	}
	s.waitState("ready", false)
	s.mail.set([]string{"mail-1"}, nil)
	if extra := s.next(300 * time.Millisecond); extra != nil {
		t.Fatalf("notified before ready: %v", extra)
	}
	if s.mail.reads != 0 {
		t.Fatal("read mail before the role was ready")
	}
	s.rpc("tools/call", map[string]any{"name": "channel_ready", "arguments": map[string]any{}}, 3)
	st := s.waitState("ready", true)
	if st["nonce"] != "test-nonce" || st["pid"] != float64(os.Getpid()) {
		t.Fatalf("state %v", st)
	}
	info, _ := os.Stat(s.state)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode %v", info.Mode().Perm())
	}
	note := s.next(3 * time.Second)
	params := note["params"].(map[string]any)
	if note["method"] != "notifications/claude/channel" || params["content"] != "오라이: 새 메시지\nIDs: mail-1" {
		t.Fatalf("notification %v", note)
	}
	for _, v := range params["meta"].(map[string]any) {
		if _, isString := v.(string); !isString {
			t.Fatal("meta values must be strings")
		}
	}
	s.mail.set([]string{"mail-1"}, nil)
	if repeat := s.next(300 * time.Millisecond); repeat != nil {
		t.Fatalf("repeated: %v", repeat)
	}
	s.mail.set([]string{"mail-1", "mail-2"}, nil)
	second := s.next(3 * time.Second)
	meta := second["params"].(map[string]any)["meta"].(map[string]any)
	if meta["pending_count"] != "2" || second["params"].(map[string]any)["content"] != "오라이: 새 메시지\nIDs: mail-2" {
		t.Fatalf("second %v", second)
	}
	s.in.Close()
	select {
	case err := <-s.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop on EOF")
	}
	s.waitState("ready", false)
}

func TestMalformedRPCAndErrorsDoNotBreakStream(t *testing.T) {
	s := start(t)
	codes := func(raw string) float64 {
		s.send(raw)
		return s.next(3 * time.Second)["error"].(map[string]any)["code"].(float64)
	}
	if codes("{broken") != -32700 || codes("[]") != -32600 {
		t.Fatal("parse/invalid request codes")
	}
	if s.rpc("tools/list", nil, 5)["error"].(map[string]any)["code"] != float64(-32000) {
		t.Fatal("must initialize first")
	}
	s.initialize()
	bad := s.rpc("tools/call", map[string]any{"name": "channel_ready", "arguments": map[string]any{"bad": 1}}, 6)
	if bad["error"].(map[string]any)["code"] != float64(-32602) {
		t.Fatal("arguments must be empty")
	}
	if s.rpc("unknown", nil, 7)["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatal("unknown method")
	}
	if len(s.rpc("ping", nil, 8)["result"].(map[string]any)) != 0 {
		t.Fatal("ping")
	}
	s.waitState("ready", false)
}

func TestMailErrorsAreRecordedThenClearedWithoutNotifications(t *testing.T) {
	s := start(t)
	s.initialize()
	s.rpc("tools/call", map[string]any{"name": "channel_ready", "arguments": map[string]any{}}, 3)
	s.waitState("ready", true)
	s.mail.set(nil, errors.New("inbox unreadable"))
	st := s.waitState("last_error", "inbox unreadable")
	if st["ready"] != true {
		t.Fatal("an error must not drop readiness")
	}
	s.mail.set(nil, nil)
	s.waitState("last_error", nil)
	if extra := s.next(200 * time.Millisecond); extra != nil {
		t.Fatalf("unexpected output %v", extra)
	}
}

func TestFromEnvRejectsInvalidRole(t *testing.T) {
	t.Setenv("ORAI_RUN_NONCE", "n")
	t.Setenv("ORAI_STATE_DIR", t.TempDir())
	t.Setenv("ORAI_ROLE", "Bad Role")
	if _, err := FromEnv("test"); err == nil {
		t.Fatal("accepted an invalid role")
	}
}
