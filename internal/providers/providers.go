// Package providers builds provider CLI arguments and the native channel each CLI uses
// for notifications: `codex queue --thread` for Codex, a local MCP channel for Claude.
package providers

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/state"
)

const NoticePrefix = "오라이: 새 메시지\nIDs: "

// Notice carries only new message IDs; the role receives them with `orai msg inbox <ID>`.
func Notice(ids []string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return NoticePrefix + strings.Join(sorted, ", ")
}

func SessionStartHook(command string) []any {
	return []any{map[string]any{"matcher": "startup|resume|compact",
		"hooks": []any{map[string]any{"type": "command", "command": command, "timeout": 10}}}}
}

// TOML renders a value as an inline TOML literal for `codex -c key=value`.
func TOML(value any) string {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, key := range keys {
			parts[i] = jsonString(key) + " = " + TOML(v[key])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = TOML(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func toAny(servers map[string]map[string]any) map[string]any {
	out := map[string]any{}
	for name, server := range servers {
		out[name] = map[string]any(server)
	}
	return out
}

// CodexArgs starts a new conversation, or resumes exactly sessionID when it is set.
func CodexArgs(role config.Role, sessionID, prompt, hook, root string, servers map[string]map[string]any) []string {
	args := []string{"codex"}
	if sessionID != "" {
		args = append(args, "resume", sessionID)
	}
	if role.Model != "" {
		args = append(args, "--model", role.Model)
	}
	if role.Effort != "" {
		args = append(args, "-c", "model_reasoning_effort="+jsonString(role.Effort))
	}
	args = append(args, "-c", "hooks.SessionStart="+TOML(SessionStartHook(hook)))
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		args = append(args, "-c", "mcp_servers."+name+"="+TOML(map[string]any(servers[name])))
	}
	// Project docs, mail and local state must be reachable from role worktrees.
	return append(args, "--add-dir", root, prompt)
}

const ChannelServer = "orai"

// ClaudeArgs binds a new UUID (newID) for a fresh conversation or resumes sessionID.
func ClaudeArgs(role config.Role, sessionID, newID, prompt, hook, root string, servers map[string]map[string]any,
	displayName string, channel []string) ([]string, string) {
	bound := sessionID
	flag := "--resume"
	if bound == "" {
		bound, flag = newID, "--session-id"
	}
	args := []string{"claude", flag, bound}
	if role.Model != "" {
		args = append(args, "--model", role.Model)
	}
	if role.Effort != "" {
		args = append(args, "--effort", role.Effort)
	}
	args = append(args, "--name", displayName)
	settings := map[string]any{"hooks": map[string]any{"SessionStart": SessionStartHook(hook)}}
	all := toAny(servers)
	all[ChannelServer] = map[string]any{"command": channel[0], "args": channel[1:]}
	settingsJSON, _ := json.Marshal(settings)
	serversJSON, _ := json.Marshal(map[string]any{"mcpServers": all})
	args = append(args, "--settings", string(settingsJSON), "--mcp-config", string(serversJSON),
		"--dangerously-load-development-channels", "server:"+ChannelServer, "--add-dir", root, "--", prompt)
	return args, bound
}

// CodexNotifier queues new IDs into the captured thread and never drains mail.
type CodexNotifier struct {
	Files   state.RoleFiles
	Nonce   string
	Pending func() ([]string, error)
	Queue   func(thread, message string) error

	delivered map[string]bool
}

// Step runs one delivery attempt; call it on every inbox change and periodically.
func (n *CodexNotifier) Step() {
	current, _ := state.ReadJSON(n.Files.State())
	if current == nil || current["nonce"] != n.Nonce || current["captured_at"] == nil {
		return // wait until this run's identity is captured
	}
	if n.delivered == nil {
		n.delivered = map[string]bool{}
	}
	status := map[string]any{"nonce": n.Nonce, "ready": true, "last_error": nil}
	if enqueued, err := n.deliver(current); err != nil {
		status["ready"], status["last_error"] = false, err.Error()
	} else if enqueued {
		status["last_enqueued_at"] = float64(time.Now().UnixNano()) / 1e9
	}
	_ = state.WriteJSON(n.Files.CodexDelivery(), status)
}

func (n *CodexNotifier) deliver(current map[string]any) (bool, error) {
	ids, err := n.Pending()
	if err != nil {
		return false, err
	}
	pending := map[string]bool{}
	var fresh []string
	for _, id := range ids {
		pending[id] = true
		if !n.delivered[id] {
			fresh = append(fresh, id)
		}
	}
	for id := range n.delivered {
		if !pending[id] {
			delete(n.delivered, id) // consumed; a reappearing ID is new again
		}
	}
	if len(fresh) == 0 {
		return false, nil
	}
	thread, _ := current["session_id"].(string)
	if err := n.Queue(thread, Notice(fresh)); err != nil {
		return false, err // delivered stays unchanged, so the next step retries
	}
	n.delivered = pending
	return true, nil
}

// CodexQueue delivers through the installed Codex CLI.
func CodexQueue(env []string) func(thread, message string) error {
	return func(thread, message string) error {
		cmd := exec.Command("codex", "queue", "--thread", thread, "--message", message)
		cmd.Env = env
		done := make(chan error, 1)
		var out []byte
		go func() {
			var err error
			out, err = cmd.CombinedOutput()
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("codex queue: %v: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
			return fmt.Errorf("codex queue timed out")
		}
	}
}
