// Package mail is Orai's own message queue. The on-disk layout and message format are
// compatible with AMQ (github.com/avivsinai/agent-message-queue, schema 1) so `amq`
// can read and write the same mailboxes:
//
//	<root>/meta/config.json                      {"agents": [...], "created_utc": ..., "version": 1}
//	<root>/agents/<handle>/inbox/{tmp,new,cur}/<id>.md
//	<root>/agents/<handle>/outbox/sent/<id>.md
//	<root>/agents/<handle>/dlq/{tmp,new,cur}/
//	<root>/agents/<handle>/receipts/<id>__<handle>__drained.json
//
// A message is "---json\n{header}\n---\n" followed by the body.
package mail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/oXpace/orai/internal/state"
)

// Every leaf AMQ requires for a complete mailbox (dlq included, even though Orai never
// dead-letters), so `amq` accepts trees Orai creates.
var leaves = []string{"inbox/tmp", "inbox/new", "inbox/cur", "outbox/sent", "dlq/tmp", "dlq/new", "dlq/cur", "receipts"}

// Root is one mailbox tree (for Orai: <project>/.agent-mail/<session>).
type Root string

func (r Root) Path(parts ...string) string {
	return filepath.Join(append([]string{string(r)}, parts...)...)
}
func (r Root) meta() string { return r.Path("meta", "config.json") }
func (r Root) box(handle, leaf string) string {
	return r.Path("agents", handle, filepath.FromSlash(leaf))
}

// Agents lists the handles recorded in meta/config.json (nil if not initialized).
func (r Root) Agents() ([]string, error) {
	value, err := state.ReadJSON(r.meta())
	if err != nil || value == nil {
		return nil, err
	}
	raw, _ := value["agents"].([]any)
	agents := make([]string, 0, len(raw))
	for _, item := range raw {
		if name, ok := item.(string); ok {
			agents = append(agents, name)
		}
	}
	return agents, nil
}

// Missing returns the handles whose mailbox is incomplete.
func (r Root) Missing(handles []string) []string {
	var missing []string
	for _, handle := range handles {
		for _, leaf := range leaves {
			if info, err := os.Stat(r.box(handle, leaf)); err != nil || !info.IsDir() {
				missing = append(missing, handle)
				break
			}
		}
	}
	return missing
}

func (r Root) Initialized() bool {
	_, err := os.Stat(r.meta())
	return err == nil
}

// Init creates or extends the tree. Handles already recorded (retired roles included)
// are kept; existing messages are never touched.
func Init(root Root, handles []string) error {
	known, err := root.Agents()
	if err != nil {
		return err
	}
	union := map[string]bool{}
	for _, h := range append(known, handles...) {
		union[h] = true
	}
	agents := make([]string, 0, len(union))
	for h := range union {
		agents = append(agents, h)
	}
	sort.Strings(agents)
	for _, handle := range agents {
		for _, leaf := range leaves {
			if err := state.PrivateDirs(root.box(handle, leaf)); err != nil {
				return err
			}
		}
	}
	if len(known) == len(agents) && root.Initialized() {
		return nil
	}
	created := time.Now().UTC().Format(time.RFC3339)
	if value, _ := state.ReadJSON(root.meta()); value != nil {
		if stamp, ok := value["created_utc"].(string); ok {
			created = stamp
		}
	}
	data, err := json.MarshalIndent(map[string]any{"agents": agents, "created_utc": created, "version": 1}, "", "  ")
	if err != nil {
		return err
	}
	if err := state.AtomicWrite(root.meta(), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", root.meta(), err)
	}
	return nil
}
