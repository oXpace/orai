package mail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newRoot(t *testing.T, handles ...string) Root {
	t.Helper()
	root := Root(filepath.Join(t.TempDir(), "mail", "orai"))
	if err := Init(root, handles); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestInitKeepsKnownHandlesAndIsPrivate(t *testing.T) {
	root := newRoot(t, "lead", "user", "senior")
	if err := Init(root, []string{"dev", "lead", "user"}); err != nil {
		t.Fatal(err)
	}
	agents, _ := root.Agents()
	if strings.Join(agents, ",") != "dev,lead,senior,user" {
		t.Fatalf("agents = %v", agents)
	}
	info, _ := os.Stat(root.box("dev", "inbox/new"))
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mailbox mode %v", info.Mode().Perm())
	}
	if missing := root.Missing([]string{"dev", "ghost"}); len(missing) != 1 || missing[0] != "ghost" {
		t.Fatalf("missing = %v", missing)
	}
}

func TestSendListDrainReplyContract(t *testing.T) {
	root := newRoot(t, "dev", "lead", "user")
	body := "@literal $(do-not-run) `literal`\n두 번째 줄"
	sent, err := root.Send("lead", []string{"dev"}, body, SendOptions{Kind: "question"})
	if err != nil {
		t.Fatal(err)
	}
	if sent.Thread != "p2p/dev__lead" || sent.Session != "orai" || !sent.Outbox.Written {
		t.Fatalf("send result %+v", sent)
	}
	if _, err := os.Stat(root.box("lead", "outbox/sent/") + "/" + sent.ID + ".md"); err != nil {
		t.Fatal("no outbox copy:", err)
	}
	listed, _ := root.ListNew("dev")
	if len(listed) != 1 || listed[0].ID != sent.ID || listed[0].Kind != "question" || listed[0].Priority != "normal" {
		t.Fatalf("list %+v", listed)
	}
	// Listing never consumes.
	if again, _ := root.ListNew("dev"); len(again) != 1 {
		t.Fatal("list consumed a message")
	}
	drained, err := root.Drain("dev", 20, true)
	if err != nil || drained.Count != 1 || strings.TrimRight(*drained.Drained[0].Body, "\n") != body {
		t.Fatalf("drain %+v %v", drained, err)
	}
	if _, err := os.Stat(filepath.Join(root.box("dev", "receipts"), sent.ID+"__dev__drained.json")); err != nil {
		t.Fatal("no receipt:", err)
	}
	reply, err := root.Reply("dev", sent.ID, "answer\n근거", "")
	if err != nil {
		t.Fatal(err)
	}
	if reply.InReplyTo != sent.ID || reply.OrigBox != "cur" || reply.Subject != "Re: (no subject)" {
		t.Fatalf("reply %+v", reply)
	}
	got, _ := root.Drain("lead", 0, true)
	if got.Count != 1 || got.Drained[0].Kind != "answer" || got.Drained[0].Thread != sent.Thread {
		t.Fatalf("reply delivery %+v", got)
	}
	empty, _ := root.Drain("dev", 20, true)
	if empty.Count != 0 || empty.Drained == nil {
		t.Fatalf("empty drain must be an empty list: %+v", empty)
	}
}

func TestReadByIDReceivesOnlyThatMessage(t *testing.T) {
	root := newRoot(t, "dev", "lead", "user")
	first, _ := root.Send("lead", []string{"dev"}, "first", SendOptions{})
	time.Sleep(2 * time.Millisecond)
	second, _ := root.Send("lead", []string{"dev"}, "second", SendOptions{})
	got, err := root.Read("dev", first.ID)
	if err != nil || got.Header.ID != first.ID || got.Body != "first\n" {
		t.Fatalf("read %+v %v", got, err)
	}
	pending, _ := root.Pending("dev")
	if len(pending) != 1 || pending[0] != second.ID {
		t.Fatalf("pending %v", pending)
	}
	if _, err := root.Read("dev", first.ID); err != nil {
		t.Fatal("an already received message is still readable:", err)
	}
	for _, bad := range []string{"missing", "../lead/inbox/new/x", ""} {
		if _, err := root.Read("dev", bad); err == nil {
			t.Fatalf("read %q succeeded", bad)
		}
	}
	limited, _ := root.Drain("dev", 1, false)
	if limited.Count != 1 || limited.Drained[0].Body != nil {
		t.Fatalf("limit/include-body %+v", limited)
	}
}

func TestSendRejectsEmptyBodyBadKindAndUnknownHandle(t *testing.T) {
	root := newRoot(t, "dev", "lead", "user")
	for _, tc := range []struct {
		to, body, kind string
	}{{"dev", " ", ""}, {"dev", "x", "chat"}, {"ghost", "x", ""}} {
		if _, err := root.Send("lead", []string{tc.to}, tc.body, SendOptions{Kind: tc.kind}); err == nil {
			t.Fatalf("send %+v succeeded", tc)
		}
	}
	if _, err := root.Reply("dev", "missing", "x", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reply to missing: %v", err)
	}
}

func TestDeliveryNeverReplacesAnExistingMessage(t *testing.T) {
	root := newRoot(t, "dev", "lead", "user")
	if err := root.deliver("dev", "same.md", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := root.deliver("dev", "same.md", []byte("two")); err == nil {
		t.Fatal("second delivery replaced the first")
	}
	data, _ := os.ReadFile(filepath.Join(root.box("dev", "inbox/new"), "same.md"))
	if string(data) != "one" {
		t.Fatalf("content %q", data)
	}
	if left, _ := os.ReadDir(root.box("dev", "inbox/tmp")); len(left) != 0 {
		t.Fatalf("tmp leftovers: %v", left)
	}
}

func TestWatchWakesOnDelivery(t *testing.T) {
	root := newRoot(t, "dev", "lead", "user")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wakes := make(chan struct{}, 16)
	go func() { _ = root.Watch(ctx, "dev", time.Hour, func() { wakes <- struct{}{} }) }()
	<-wakes // initial wake
	start := time.Now()
	if _, err := root.Send("lead", []string{"dev"}, "ping", SendOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-wakes:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("wake after %v", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no wake on delivery")
	}
}

// amqEnv points the real AMQ CLI at an exact root with a fixed identity.
func amqEnv(root Root, me string) []string {
	env := []string{"AM_ROOT=" + string(root), "AM_BASE_ROOT=" + string(root), "AM_ME=" + me, "AMQ_NO_UPDATE_CHECK=1"}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "AM_") && !strings.HasPrefix(kv, "AMQ_") {
			env = append(env, kv)
		}
	}
	return env
}

func amq(t *testing.T, root Root, me string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("amq", args...)
	cmd.Env = amqEnv(root, me)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exit, ok := err.(*exec.ExitError); ok {
			stderr = string(exit.Stderr)
		}
		t.Fatalf("amq %v: %v %s", args, err, stderr)
	}
	return out
}

// The disk format is shared with AMQ: each side reads and answers what the other wrote.
func TestInteroperatesWithRealAMQ(t *testing.T) {
	if _, err := exec.LookPath("amq"); err != nil {
		t.Skip("amq not installed")
	}
	root := newRoot(t, "dev", "lead", "user")
	sent, err := root.Send("lead", []string{"dev"}, "from orai 본문", SendOptions{Kind: "question"})
	if err != nil {
		t.Fatal(err)
	}
	var drained struct {
		Drained []struct{ ID, Body, Kind, Thread string }
		Count   int
	}
	if err := json.Unmarshal(amq(t, root, "dev", "drain", "--include-body", "--json"), &drained); err != nil {
		t.Fatal(err)
	}
	if drained.Count != 1 || drained.Drained[0].ID != sent.ID || drained.Drained[0].Body != "from orai 본문\n" {
		t.Fatalf("amq drained %+v", drained)
	}
	amq(t, root, "dev", "reply", "--id", sent.ID, "--body", "from amq", "--json")
	got, err := root.Drain("lead", 0, true)
	if err != nil || got.Count != 1 || *got.Drained[0].Body != "from amq\n" || got.Drained[0].Kind != "answer" ||
		got.Drained[0].Thread != sent.Thread {
		t.Fatalf("orai received %+v %v", got, err)
	}
	amq(t, root, "user", "send", "--to", "dev", "--kind", "todo", "--body", "desktop", "--json")
	pending, _ := root.Pending("dev")
	if len(pending) != 1 {
		t.Fatalf("pending %v", pending)
	}
	if _, err := root.Reply("dev", pending[0], "done", ""); err != nil {
		t.Fatal(err)
	}
	var listed []struct{ Kind, From string }
	if err := json.Unmarshal(amq(t, root, "user", "list", "--new", "--json"), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].From != "dev" || listed[0].Kind != "todo" {
		t.Fatalf("amq listed %+v", listed)
	}
}
