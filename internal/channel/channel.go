// Package channel is the minimal Claude MCP channel: it announces pending mail and never
// consumes it. Transport is newline-delimited JSON-RPC on stdio. The launcher owns
// identity, session selection and process lifetime; the channel owns only readiness.
package channel

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/providers"
	"github.com/oXpace/orai/internal/state"
)

var protocolVersions = map[string]bool{"2024-11-05": true, "2025-03-26": true, "2025-06-18": true, "2025-11-25": true}

// Fallback is how often pending mail is re-checked when no inbox event arrives.
var Fallback = 2 * time.Second

type Channel struct {
	Role, Nonce, Version string
	StateDir             string
	Pending              func() ([]string, error)
	Watch                func(ctx context.Context, wake func()) error

	out       io.Writer
	outMu     sync.Mutex
	stateMu   sync.Mutex
	handshake bool
	inited    bool
	client    bool
	ready     bool
	lastError *string
	wake      chan struct{}
}

// FromEnv builds the channel the launcher configured through ORAI_* variables.
func FromEnv(version string) (*Channel, error) {
	role, nonce, dir := os.Getenv("ORAI_ROLE"), os.Getenv("ORAI_RUN_NONCE"), os.Getenv("ORAI_STATE_DIR")
	if nonce == "" || dir == "" {
		return nil, fmt.Errorf("ORAI_RUN_NONCE and ORAI_STATE_DIR are required")
	}
	if !config.NamePattern.MatchString(role) {
		return nil, fmt.Errorf("invalid ORAI_ROLE")
	}
	root := mail.Root(os.Getenv("ORAI_MAIL_ROOT"))
	return &Channel{
		Role: role, Nonce: nonce, Version: version, StateDir: dir,
		Pending: func() ([]string, error) {
			if root == "" {
				return nil, fmt.Errorf("ORAI_MAIL_ROOT is not set")
			}
			return root.Pending(role)
		},
		Watch: func(ctx context.Context, wake func()) error { return root.Watch(ctx, role, Fallback, wake) },
	}, nil
}

func (c *Channel) statePath() string { return filepath.Join(c.StateDir, c.Role+".channel.json") }

func (c *Channel) writeState() {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	var lastError any
	if c.lastError != nil {
		lastError = *c.lastError
	}
	_ = state.WriteJSON(c.statePath(), map[string]any{"nonce": c.Nonce, "pid": os.Getpid(), "ready": c.ready,
		"last_error": lastError})
}

// setError records a changed error ("" clears it) and persists it once per change.
func (c *Channel) setError(message string) {
	c.stateMu.Lock()
	changed := (c.lastError == nil) != (message == "") || (c.lastError != nil && *c.lastError != message)
	if message == "" {
		c.lastError = nil
	} else {
		c.lastError = &message
	}
	c.stateMu.Unlock()
	if changed {
		c.writeState()
	}
}

func (c *Channel) emit(value map[string]any) {
	data, _ := json.Marshal(value)
	c.outMu.Lock()
	defer c.outMu.Unlock()
	_, _ = c.out.Write(append(data, '\n'))
}

func (c *Channel) result(id any, value any) {
	c.emit(map[string]any{"jsonrpc": "2.0", "id": id, "result": value})
}

func (c *Channel) fail(id any, code int, message string) {
	c.emit(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}

func (c *Channel) activate() {
	if c.inited && c.client && !c.ready {
		c.stateMu.Lock()
		c.ready = true
		c.stateMu.Unlock()
		c.writeState()
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
}

func validID(raw json.RawMessage) (any, bool) {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	switch v := value.(type) {
	case string:
		return v, true
	case float64:
		return int64(v), v == float64(int64(v))
	}
	return nil, false
}

func (c *Channel) dispatch(line []byte) {
	var request map[string]json.RawMessage
	if err := json.Unmarshal(line, &request); err != nil {
		var probe any
		if json.Unmarshal(line, &probe) != nil {
			c.fail(nil, -32700, "Parse error")
		} else {
			c.fail(nil, -32600, "Invalid Request")
		}
		return
	}
	var version, method string
	if json.Unmarshal(request["jsonrpc"], &version) != nil || version != "2.0" ||
		json.Unmarshal(request["method"], &method) != nil {
		c.fail(nil, -32600, "Invalid Request")
		return
	}
	rawID, hasID := request["id"]
	var id any
	if hasID {
		var ok bool
		if id, ok = validID(rawID); !ok {
			c.fail(nil, -32600, "Invalid Request")
			return
		}
	}
	if !hasID {
		if method == "notifications/initialized" && c.handshake {
			c.inited = true
			c.activate()
		}
		return
	}
	params := map[string]any{}
	if raw, ok := request["params"]; ok && json.Unmarshal(raw, &params) != nil {
		c.fail(id, -32602, "Invalid params")
		return
	}
	switch {
	case method == "initialize":
		requested, ok := params["protocolVersion"].(string)
		if !ok {
			c.fail(id, -32602, "protocolVersion is required")
			return
		}
		c.handshake = true
		if !protocolVersions[requested] {
			requested = "2025-06-18"
		}
		c.result(id, map[string]any{
			"protocolVersion": requested,
			"capabilities":    map[string]any{"experimental": map[string]any{"claude/channel": map[string]any{}}, "tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "orai", "version": c.Version},
			"instructions":    "Call channel_ready when this channel connects. Follow the orai skill when a mail notification arrives.",
		})
	case method == "ping":
		c.result(id, map[string]any{})
	case !c.inited:
		c.fail(id, -32000, "Initialize the channel first")
	case method == "tools/list":
		c.result(id, map[string]any{"tools": []any{map[string]any{
			"name": "channel_ready", "description": "Enable mail notifications for this channel connection.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		}}})
	case method == "tools/call":
		arguments, hasArgs := params["arguments"]
		args, _ := arguments.(map[string]any)
		if params["name"] != "channel_ready" || (hasArgs && (args == nil || len(args) != 0)) {
			c.fail(id, -32602, "Expected channel_ready with no arguments")
			return
		}
		c.result(id, map[string]any{"content": []any{map[string]any{"type": "text", "text": "Orai channel ready."}}})
		c.client = true
		c.activate()
	default:
		c.fail(id, -32601, "Method not found")
	}
}

func (c *Channel) poll(ctx context.Context) {
	announced := map[string]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
		}
		c.stateMu.Lock()
		ready := c.ready
		c.stateMu.Unlock()
		if !ready {
			continue // never read mail before the role is ready
		}
		ids, err := c.Pending()
		if err != nil {
			c.setError(err.Error())
			continue
		}
		current := map[string]bool{}
		var fresh []string
		for _, id := range ids {
			current[id] = true
			if !announced[id] {
				fresh = append(fresh, id)
			}
		}
		if len(fresh) > 0 {
			c.emit(map[string]any{"jsonrpc": "2.0", "method": "notifications/claude/channel", "params": map[string]any{
				"content": providers.Notice(fresh),
				"meta":    map[string]any{"source": "orai", "role": c.Role, "pending_count": strconv.Itoa(len(ids))},
			}})
		}
		announced = current
		c.setError("")
	}
}

// Run serves until stdin closes.
func (c *Channel) Run(in io.Reader, out io.Writer) error {
	c.out = out
	c.wake = make(chan struct{}, 1)
	if err := state.PrivateDirs(c.StateDir); err != nil {
		return err
	}
	c.writeState()
	ctx, cancel := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Add(2)
	go func() { defer workers.Done(); c.poll(ctx) }()
	go func() {
		defer workers.Done()
		push := func() {
			select {
			case c.wake <- struct{}{}:
			default:
			}
		}
		if err := c.Watch(ctx, push); err != nil {
			// No inbox to watch yet (or no inotify/kqueue): keep checking on a timer.
			ticker := time.NewTicker(Fallback)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					push()
				}
			}
		}
	}()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		c.dispatch(scanner.Bytes())
	}
	cancel()
	workers.Wait()
	c.stateMu.Lock()
	c.ready = false
	c.stateMu.Unlock()
	c.writeState()
	return scanner.Err()
}
