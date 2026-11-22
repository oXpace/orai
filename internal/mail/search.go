package mail

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oXpace/orai/internal/lexical"
)

// ErrNotReceived marks a message that is still waiting in inbox/new. Searching and
// showing never receive a message: only `inbox` does, so a notification's "receive
// once" contract holds whichever command touched the message first.
var ErrNotReceived = errors.New("message not received yet")

// Found is one message a search matched.
type Found struct {
	ID      string   `json:"id"`
	From    string   `json:"from"`
	To      []string `json:"to"`
	Thread  string   `json:"thread"`
	Subject string   `json:"subject,omitempty"`
	Created string   `json:"created"`
	Kind    string   `json:"kind,omitempty"`
	Box     string   `json:"box"` // "received" (inbox/cur) or "sent" (outbox/sent)
	Line    int      `json:"line,omitempty"`
	Excerpt string   `json:"excerpt,omitempty"`

	match lexical.Match
}

// SearchResult is what Search found in one handle's mailbox.
type SearchResult struct {
	Found   []Found `json:"found"`
	Scanned int     `json:"scanned"` // received and sent messages read
	Pending int     `json:"pending"` // not yet received messages that also match; bodies withheld
}

// Search matches the handle's received and sent messages against query, best first
// and newest first among equals. Messages still in inbox/new are only counted: their
// bodies are not shown before they are received.
func (r Root) Search(me, query string, limit int) (SearchResult, error) {
	result := SearchResult{Found: []Found{}}
	terms := lexical.Terms(query)
	if len(terms) == 0 {
		return result, nil
	}
	var all []Found
	for _, box := range []struct{ leaf, name string }{{"inbox/cur", "received"}, {"outbox/sent", "sent"}, {"inbox/new", "new"}} {
		messages, err := r.messagesIn(r.box(me, box.leaf))
		if err != nil {
			return result, err
		}
		for _, msg := range messages {
			m := lexical.Score(msg.Header.Subject+"\n"+msg.Body, terms)
			if box.name != "new" {
				result.Scanned++
			}
			if !m.Enough(terms) {
				continue
			}
			if box.name == "new" {
				result.Pending++
				continue
			}
			h := msg.Header
			line := m.Line - 1 // the subject was line 1
			all = append(all, Found{ID: h.ID, From: h.From, To: h.To, Thread: h.Thread, Subject: h.Subject,
				Created: h.Created, Kind: h.Kind, Box: box.name, Line: max(line, 0), Excerpt: m.Excerpt, match: m})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Created > all[j].Created })
	lexical.Rank(all, func(f Found) lexical.Match { return f.match })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	result.Found = append(result.Found, all...)
	return result, nil
}

// messagesIn parses every message file in dir; a missing dir is empty, and a file that
// vanishes mid-read (received concurrently) or cannot be parsed is skipped.
func (r Root) messagesIn(dir string) ([]Message, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var messages []Message
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		msg, err := readMessage(path)
		var bad *MalformedError
		if errors.Is(err, fs.ErrNotExist) || errors.As(err, &bad) {
			continue // one unreadable file must not stop the search; a drain dead-letters it
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// Shown is a message displayed by Show, with where it lives in the mailbox.
type Shown struct {
	Box    string `json:"box"` // "received" or "sent"
	Header Header `json:"header"`
	Body   string `json:"body"`
}

// Show returns a message the handle already received or sent, without changing any
// state. A message still waiting in inbox/new is ErrNotReceived.
func (r Root) Show(me, id string) (Shown, error) {
	id, err := validID(id)
	if err != nil {
		return Shown{}, err
	}
	for _, box := range []struct{ leaf, name string }{{"inbox/cur", "received"}, {"outbox/sent", "sent"}, {"inbox/new", "new"}} {
		path := filepath.Join(r.box(me, box.leaf), id+".md")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if box.name == "new" {
			return Shown{}, fmt.Errorf("%w: %s", ErrNotReceived, id)
		}
		msg, err := readMessage(path)
		if err != nil {
			return Shown{}, err
		}
		return Shown{Box: box.name, Header: msg.Header, Body: msg.Body}, nil
	}
	return Shown{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}
