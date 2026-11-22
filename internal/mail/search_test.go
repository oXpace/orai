package mail

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchReadsReceivedAndSentButOnlyCountsPending(t *testing.T) {
	root := newRoot(t, "lead", "dev")
	sent, err := root.Send("lead", []string{"dev"}, "배포 절차는 release.md에 있다", SendOptions{Kind: "answer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Send("lead", []string{"dev"}, "다른 배포 질문", SendOptions{Kind: "question"}); err != nil {
		t.Fatal(err)
	}
	// dev receives the first message only; the second stays new.
	if _, err := root.Read("dev", sent.ID); err != nil {
		t.Fatal(err)
	}
	result, err := root.Search("dev", "배포", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Found) != 1 || result.Found[0].ID != sent.ID || result.Found[0].Box != "received" || result.Pending != 1 {
		t.Fatalf("dev search = %+v", result)
	}
	if result.Found[0].Excerpt == "" {
		t.Fatal("a hit carries its matching line")
	}
	// Searching must not receive the pending message.
	if pending, _ := root.Pending("dev"); len(pending) != 1 {
		t.Fatalf("search received a message: pending = %v", pending)
	}
	mine, err := root.Search("lead", "release", 5)
	if err != nil || len(mine.Found) != 1 || mine.Found[0].Box != "sent" {
		t.Fatalf("lead sees its sent message: %+v %v", mine, err)
	}
}

func TestShowRefusesAMessageNotReceivedAndChangesNothing(t *testing.T) {
	root := newRoot(t, "lead", "dev")
	sent, err := root.Send("lead", []string{"dev"}, "본문", SendOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Show("dev", sent.ID); !errors.Is(err, ErrNotReceived) {
		t.Fatalf("show of a new message: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root.box("dev", "inbox/new"), sent.ID+".md")); err != nil {
		t.Fatal("show moved the message")
	}
	shown, err := root.Show("lead", sent.ID)
	if err != nil || shown.Box != "sent" || shown.Body == "" {
		t.Fatalf("sender sees its own message: %+v %v", shown, err)
	}
	if _, err := root.Show("dev", "../x"); err == nil {
		t.Fatal("an ID with a path must be refused")
	}
}

// A file that cannot be parsed, in any folder, never stops a search of the others.
func TestSearchSkipsUnreadableMessages(t *testing.T) {
	root := newRoot(t, "lead", "dev")
	sent, err := root.Send("lead", []string{"dev"}, "배포 절차", SendOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.Read("dev", sent.ID); err != nil {
		t.Fatal(err)
	}
	for _, leaf := range []string{"inbox/new", "inbox/cur"} {
		if err := os.WriteFile(filepath.Join(root.box("dev", leaf), "bad.md"), []byte("not a message"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := root.Search("dev", "배포", 5)
	if err != nil || len(result.Found) != 1 || result.Found[0].ID != sent.ID {
		t.Fatalf("search beside unreadable files = %+v %v", result, err)
	}
}
