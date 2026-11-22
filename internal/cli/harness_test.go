package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The short commands are the same commands as `orai msg …`: same identity rules, same
// JSON. This keeps the receive-once and identity contracts in one place.
func TestShortMessageCommandsMatchMsg(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	asRole(t, p, "lead")
	if r := run("", "send", "dev", "첫 번째 본문", "--kind", "question"); r.code != 0 {
		t.Fatalf("send with a positional body: %+v", r)
	}
	if r := run("", "send", "dev", "본문", "--body", "또"); r.code != 2 {
		t.Fatalf("a body given twice must be a usage error: %+v", r)
	}
	if r := run("", "msg", "send", "dev", "--body", "두 번째 본문"); r.code != 0 {
		t.Fatalf("msg send: %+v", r)
	}
	asRole(t, p, "dev")
	short, long := run("", "inbox", "--peek"), run("", "msg", "inbox", "--peek")
	if short.code != 0 || short.out != long.out {
		t.Fatalf("inbox --peek differs:\n%s\n%s", short.out, long.out)
	}
	var listed []map[string]any
	_ = json.Unmarshal([]byte(short.out), &listed)
	if len(listed) != 2 {
		t.Fatalf("listed = %v", listed)
	}
	id := listed[0]["id"].(string)
	if r := run("", "read", id); r.code != 1 || !strings.Contains(r.errOut, "orai inbox "+id) {
		t.Fatalf("read of a message not received yet: %+v", r)
	}
	first := run("", "inbox", id)
	if first.code != 0 || !strings.Contains(first.out, "첫 번째 본문") {
		t.Fatalf("inbox ID: %+v", first)
	}
	again := run("", "msg", "inbox", id)
	if !strings.Contains(again.out, `"already_received": true`) || strings.Contains(again.out, "첫 번째 본문") {
		t.Fatalf("the receive-once contract holds across both spellings: %+v", again)
	}
	shown := run("", "read", id)
	if shown.code != 0 || !strings.Contains(shown.out, "첫 번째 본문") || !strings.Contains(shown.out, "from: lead") {
		t.Fatalf("read of a received message: %+v", shown)
	}
	if r := run("", "reply", id, "답"); r.code != 0 {
		t.Fatalf("reply with a positional body: %+v", r)
	}
	if r := run("", "inbox", "--as", "user"); r.code != 1 || !strings.Contains(r.errOut, "cannot override") {
		t.Fatalf("a role cannot act as the user through the short form either: %+v", r)
	}
}

func TestSearchCoversTheRolesMailboxAndDocsWithoutReceiving(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	cfg := twoRoles + "\n[integrations.shelf]\ncollections = { docs = \"docs\" }\n"
	if err := os.WriteFile(filepath.Join(p.Root, "orai.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(p.Root, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(p.Root, "docs", "deploy.md"), []byte("# 배포\n\n배포 절차는 태그부터\n"), 0o644)
	asRole(t, p, "lead")
	if r := run("", "send", "dev", "배포 절차 질문"); r.code != 0 {
		t.Fatalf("send: %+v", r)
	}
	if r := run("", "send", "dev", "배포 절차 두 번째"); r.code != 0 {
		t.Fatalf("send: %+v", r)
	}
	asRole(t, p, "dev")
	if r := run("", "inbox", "--limit", "1"); r.code != 0 {
		t.Fatalf("receive one: %+v", r)
	}
	t.Chdir(p.Root)
	r := run("", "search", "배포", "절차")
	// The shelf was never built, so docs fall back to a keyword scan: results, exit 1.
	if r.code != 1 || !strings.Contains(r.out, "docs: keyword scan") || !strings.Contains(r.out, "orai shelf init") {
		t.Fatalf("search: %+v", r)
	}
	if !strings.Contains(r.out, "[docs] docs/deploy.md:3") || !strings.Contains(r.out, "[mail]") {
		t.Fatalf("hits missing: %s", r.out)
	}
	if !strings.Contains(r.out, "1 message(s) not received yet also match") {
		t.Fatalf("the pending message is counted, not shown: %s", r.out)
	}
	weak := run("", "search", "zzqx", "절차", "--in", "docs")
	if !strings.Contains(weak.out, "weak match: nothing matched; no document contains zzqx") || !strings.Contains(weak.out, "--also") {
		t.Fatalf("a weak result says so and how to retry: %s", weak.out)
	}
	retry := run("", "search", "zzqx", "절차", "--in", "docs", "--also", "태그", "--also", "  ")
	if !strings.Contains(retry.out, "[docs] docs/deploy.md:3  배포") || strings.Contains(retry.out, "weak match") {
		t.Fatalf("another phrasing finds the document: %s", retry.out)
	}
	peek := run("", "inbox", "--peek")
	var listed []map[string]any
	_ = json.Unmarshal([]byte(peek.out), &listed)
	if len(listed) != 1 {
		t.Fatalf("search received a message: %s", peek.out)
	}
	mailOnly := run("", "search", "절차는 질문", "--in", "mail", "--json")
	var parsed struct {
		Sources []struct{ Name, State string }
		Mail    []map[string]any
	}
	if err := json.Unmarshal([]byte(mailOnly.out), &parsed); err != nil || mailOnly.code != 0 || len(parsed.Sources) != 1 || len(parsed.Mail) != 1 {
		t.Fatalf("mail only: %+v %v", mailOnly, err)
	}
}

func TestSearchOutsideARoleSkipsMailUnlessAsked(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	t.Chdir(p.Root)
	r := run("", "search", "anything")
	if r.code != 0 || !strings.Contains(r.out, "mail: not searched") || !strings.Contains(r.out, "--as user") {
		t.Fatalf("default search outside a role: %+v", r)
	}
	if r := run("", "search", "anything", "--in", "mail"); r.code != 1 || !strings.Contains(r.out, "mail: FAILED") {
		t.Fatalf("asked for mail without an identity: %+v", r)
	}
	if r := run("", "search", "anything", "--in", "mail", "--as", "user"); r.code != 0 {
		t.Fatalf("user mailbox: %+v", r)
	}
	if r := run("", "search"); r.code != 2 {
		t.Fatalf("no query: %+v", r)
	}
	if r := run("", "search", "x", "--in", "web"); r.code != 2 {
		t.Fatalf("unknown source: %+v", r)
	}
}

func TestReadPrintsLineRangesAndStaysInsideTheProject(t *testing.T) {
	clearEnv(t)
	p := newProject(t, t.TempDir())
	t.Chdir(p.Root)
	_ = os.WriteFile(filepath.Join(p.Root, "notes.md"), []byte("one\ntwo\nthree\nfour\n"), 0o644)
	r := run("", "read", "notes.md:2-3")
	if r.code != 0 || !strings.Contains(r.out, "(lines 2-3 of 4)") || !strings.Contains(r.out, "     2  two") || strings.Contains(r.out, "four") {
		t.Fatalf("range: %+v", r)
	}
	outside := filepath.Join(filepath.Dir(p.Root), "secret.md")
	_ = os.WriteFile(outside, []byte("secret"), 0o644)
	if r := run("", "read", "../secret.md"); r.code != 1 || strings.Contains(r.out, "secret") {
		t.Fatalf("a file outside the project must not be read: %+v", r)
	}
	if r := run("", "read", outside); r.code != 1 {
		t.Fatalf("an absolute path outside the project must not be read: %+v", r)
	}
}
