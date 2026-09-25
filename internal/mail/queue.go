package mail

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/oXpace/orai/internal/state"
)

var ErrNotFound = errors.New("message not found")

// validID rejects anything that could escape the mailbox directory.
func validID(id string) (string, error) {
	id = strings.TrimSuffix(id, ".md")
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") || strings.HasPrefix(id, ".") {
		return "", fmt.Errorf("invalid message id %q", id)
	}
	return id, nil
}

func (r Root) session() string { return filepath.Base(string(r)) }

func (r Root) requireHandles(handles ...string) error {
	if !r.Initialized() {
		return fmt.Errorf("mailboxes are not initialized at %s", r)
	}
	if missing := r.Missing(handles); len(missing) > 0 {
		return fmt.Errorf("unknown or incomplete mailbox: %s", strings.Join(missing, ", "))
	}
	return nil
}

// deliver publishes data as inbox/new/<name> without ever replacing an existing file.
func (r Root) deliver(handle, name string, data []byte) error {
	tmp := filepath.Join(r.box(handle, "inbox/tmp"), name+".tmp-"+randomHex(8))
	file, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// link(2) fails if the target exists, so delivery is atomic and no-replace.
	return os.Link(tmp, filepath.Join(r.box(handle, "inbox/new"), name))
}

type OutboxResult struct {
	Error   string `json:"error"`
	Written bool   `json:"written"`
}

type SendResult struct {
	ID         string       `json:"id"`
	InReplyTo  string       `json:"in_reply_to,omitempty"`
	OrigBox    string       `json:"original_box,omitempty"`
	Outbox     OutboxResult `json:"outbox"`
	Root       string       `json:"root"`
	Session    string       `json:"session"`
	SourceRoot string       `json:"source_root,omitempty"`
	Subject    string       `json:"subject"`
	Thread     string       `json:"thread"`
	To         []string     `json:"to"`
}

type SendOptions struct {
	Kind, Thread, Subject string
	Refs                  []string
}

func (r Root) Send(from string, to []string, body string, opt SendOptions) (SendResult, error) {
	if strings.TrimSpace(body) == "" {
		return SendResult{}, errors.New("message body must not be empty")
	}
	if !ValidKind(opt.Kind) {
		return SendResult{}, fmt.Errorf("invalid kind %q (valid: %s)", opt.Kind, strings.Join(Kinds, ", "))
	}
	if err := r.requireHandles(append([]string{from}, to...)...); err != nil {
		return SendResult{}, err
	}
	thread := opt.Thread
	if thread == "" {
		if len(to) != 1 {
			return SendResult{}, errors.New("--thread is required for multiple recipients")
		}
		thread = canonicalP2P(from, to[0])
	}
	now := time.Now()
	msg := Message{Header: Header{
		Schema: schema, ID: NewID(now), From: from, To: to, Thread: thread, Subject: opt.Subject,
		Created: now.UTC().Format(time.RFC3339Nano), Refs: opt.Refs, Priority: defaultPriority(opt.Kind), Kind: opt.Kind,
	}, Body: body}
	data, err := msg.Marshal()
	if err != nil {
		return SendResult{}, err
	}
	name := msg.Header.ID + ".md"
	for _, recipient := range to {
		if err := r.deliver(recipient, name, data); err != nil {
			return SendResult{}, fmt.Errorf("deliver %s to %s: %w", msg.Header.ID, recipient, err)
		}
	}
	result := SendResult{ID: msg.Header.ID, Root: string(r), SourceRoot: string(r), Session: r.session(),
		Subject: opt.Subject, Thread: thread, To: to, Outbox: OutboxResult{Written: true}}
	// Audit copy in the sender's outbox; delivery already succeeded, so report instead of failing.
	if err := state.AtomicWrite(filepath.Join(r.box(from, "outbox/sent"), name), data, 0o600); err != nil {
		result.Outbox = OutboxResult{Error: err.Error()}
	}
	return result, nil
}

// find locates a message in the handle's inbox, preferring new/.
func (r Root) find(handle, id string) (string, string, error) {
	id, err := validID(id)
	if err != nil {
		return "", "", err
	}
	for _, box := range []string{"new", "cur"} {
		path := filepath.Join(r.box(handle, "inbox/"+box), id+".md")
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			return path, box, nil
		}
	}
	return "", "", fmt.Errorf("%w: %s", ErrNotFound, id)
}

func readMessage(path string) (Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Message{}, err
	}
	return Parse(data)
}

func (r Root) Reply(me, id, body, kind string) (SendResult, error) {
	path, box, err := r.find(me, id)
	if err != nil {
		return SendResult{}, err
	}
	original, err := readMessage(path)
	if err != nil {
		return SendResult{}, err
	}
	subject := "Re: (no subject)"
	if s := original.Header.Subject; s != "" {
		subject = s
		if !strings.HasPrefix(strings.ToLower(s), "re:") {
			subject = "Re: " + s
		}
	}
	if kind == "" {
		switch original.Header.Kind {
		case "review_request":
			kind = "review_response"
		case "question":
			kind = "answer"
		default:
			kind = original.Header.Kind
		}
	}
	result, err := r.Send(me, []string{original.Header.From}, body,
		SendOptions{Kind: kind, Thread: original.Header.Thread, Subject: subject, Refs: []string{original.Header.ID}})
	if err != nil {
		return SendResult{}, err
	}
	result.InReplyTo, result.OrigBox, result.SourceRoot = original.Header.ID, box, ""
	return result, nil
}

type ListItem struct {
	ID       string    `json:"id"`
	From     string    `json:"from"`
	Subject  string    `json:"subject"`
	Thread   string    `json:"thread"`
	Created  string    `json:"created"`
	Box      string    `json:"box"`
	Path     string    `json:"path"`
	Priority string    `json:"priority,omitempty"`
	Kind     string    `json:"kind,omitempty"`
	sortKey  time.Time `json:"-"`
}

// ListNew lists unread messages oldest first without consuming them.
func (r Root) ListNew(me string) ([]ListItem, error) {
	dir := r.box(me, "inbox/new")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []ListItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []ListItem{}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		msg, err := readMessage(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // consumed concurrently
			}
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		h := msg.Header
		key, _ := time.Parse(time.RFC3339Nano, h.Created)
		items = append(items, ListItem{ID: h.ID, From: h.From, Subject: h.Subject, Thread: h.Thread, Created: h.Created,
			Box: "new", Path: path, Priority: h.Priority, Kind: h.Kind, sortKey: key})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].sortKey.Equal(items[j].sortKey) {
			return items[i].sortKey.Before(items[j].sortKey)
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

// Pending returns unread message IDs, sorted.
func (r Root) Pending(me string) ([]string, error) {
	items, err := r.ListNew(me)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	sort.Strings(ids)
	return ids, nil
}

// consume moves a new message to cur/ and records a drained receipt (AMQ semantics).
func (r Root) consume(me string, msg Message, path string) error {
	target := filepath.Join(r.box(me, "inbox/cur"), filepath.Base(path))
	if err := os.Rename(path, target); err != nil {
		return err
	}
	receipt := map[string]any{"schema": schema, "msg_id": msg.Header.ID, "thread": msg.Header.Thread,
		"sender": msg.Header.From, "consumer": me, "stage": "drained", "emitted_at": time.Now().UTC().Format(time.RFC3339Nano)}
	return state.AtomicWrite(filepath.Join(r.box(me, "receipts"), msg.Header.ID+"__"+me+"__drained.json"),
		state.JSONBytes(receipt), 0o600)
}

type DrainedItem struct {
	ID         string   `json:"id"`
	From       string   `json:"from"`
	To         []string `json:"to"`
	Thread     string   `json:"thread"`
	Subject    string   `json:"subject"`
	Created    string   `json:"created"`
	Body       *string  `json:"body,omitempty"`
	Priority   string   `json:"priority,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	MovedToCur bool     `json:"moved_to_cur"`
}

type DrainResult struct {
	Drained []DrainedItem `json:"drained"`
	Count   int           `json:"count"`
}

// Drain receives up to limit messages (0 = all), oldest first.
func (r Root) Drain(me string, limit int, includeBody bool) (DrainResult, error) {
	if err := r.requireHandles(me); err != nil {
		return DrainResult{}, err
	}
	items, err := r.ListNew(me)
	if err != nil {
		return DrainResult{}, err
	}
	result := DrainResult{Drained: []DrainedItem{}}
	for _, item := range items {
		if limit > 0 && result.Count >= limit {
			break
		}
		msg, err := readMessage(item.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		if err := r.consume(me, msg, item.Path); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue // another reader took it
			}
			return result, err
		}
		h := msg.Header
		drained := DrainedItem{ID: h.ID, From: h.From, To: h.To, Thread: h.Thread, Subject: h.Subject, Created: h.Created,
			Priority: h.Priority, Kind: h.Kind, MovedToCur: true}
		if includeBody {
			body := msg.Body
			drained.Body = &body
		}
		result.Drained = append(result.Drained, drained)
		result.Count++
	}
	return result, nil
}

type ReadResult struct {
	Body   string `json:"body"`
	Header Header `json:"header"`
}

// Read returns one message by ID and receives it if it was still new.
func (r Root) Read(me, id string) (ReadResult, error) {
	path, box, err := r.find(me, id)
	if err != nil {
		return ReadResult{}, err
	}
	msg, err := readMessage(path)
	if err != nil {
		return ReadResult{}, err
	}
	if box == "new" {
		if err := r.consume(me, msg, path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return ReadResult{}, err
		}
	}
	return ReadResult{Body: msg.Body, Header: msg.Header}, nil
}

// Watch calls wake whenever the handle's inbox/new changes, and at least every
// fallback interval so a missed event only delays, never loses, a notification.
func (r Root) Watch(ctx context.Context, me string, fallback time.Duration, wake func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	if err := watcher.Add(r.box(me, "inbox/new")); err != nil {
		return err
	}
	ticker := time.NewTicker(fallback)
	defer ticker.Stop()
	wake()
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
				wake()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			_ = err // fall back to the ticker
		case <-ticker.C:
			wake()
		}
	}
}
