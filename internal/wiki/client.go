package wiki

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// McpError is an MCP protocol/application failure (a JSON-RPC error, a missing
// response, or an isError tool result).
type McpError struct{ Message string }

func (e *McpError) Error() string { return e.Message }

func mcpErrorf(format string, args ...any) error { return &McpError{fmt.Sprintf(format, args...)} }

// mcpClient is the surface probe(), ownServerRunning() and searchChecks() use. Tests
// swap the newClient factory for one returning a fake implementation, so no test ever
// talks to a real QMD server.
type mcpClient interface {
	Connect() error
	Tool(name string, arguments map[string]any) (map[string]any, error)
}

// Client is a minimal MCP streamable-HTTP client (JSON or SSE responses).
type Client struct {
	Endpoint string
	Timeout  time.Duration

	session string
	number  int
	http    *http.Client
}

// NewClient builds a Client talking only to endpoint, on a loopback connection that
// never routes through a proxy from the environment.
func NewClient(endpoint string, timeout time.Duration) *Client {
	return &Client{
		Endpoint: endpoint,
		Timeout:  timeout,
		http: &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

// newClient is the factory probe()/ownServerRunning() use; tests replace it.
var newClient = func(endpoint string, timeout time.Duration) mcpClient {
	return NewClient(endpoint, timeout)
}

func (c *Client) request(method string, params map[string]any, notification bool) (map[string]any, error) {
	c.number++
	if params == nil {
		params = map[string]any{}
	}
	payload := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	if !notification {
		payload["id"] = c.number
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.session = sid
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if notification {
		return nil, nil
	}
	var messages []map[string]any
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(line[len("data:"):])), &m); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	if len(messages) == 0 {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		messages = []map[string]any{m}
	}
	var result map[string]any
	for _, m := range messages {
		if idMatches(m["id"], c.number) {
			result = m
			break
		}
	}
	if result == nil {
		return nil, mcpErrorf("MCP %s failed: %v", method, result)
	}
	if _, hasErr := result["error"]; hasErr {
		return nil, mcpErrorf("MCP %s failed: %v", method, result)
	}
	res, _ := result["result"].(map[string]any)
	return res, nil
}

func idMatches(id any, number int) bool {
	switch v := id.(type) {
	case float64:
		return int(v) == number
	case int:
		return v == number
	default:
		return false
	}
}

// Connect performs the MCP initialize handshake.
func (c *Client) Connect() error {
	if _, err := c.request("initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "orai", "version": "1"},
	}, false); err != nil {
		return err
	}
	_, err := c.request("notifications/initialized", nil, true)
	return err
}

// Tool calls an MCP tool and returns its full result (structuredContent, content,
// isError, …).
func (c *Client) Tool(name string, arguments map[string]any) (map[string]any, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	result, err := c.request("tools/call", map[string]any{"name": name, "arguments": arguments}, false)
	if err != nil {
		return nil, err
	}
	if isErr, _ := result["isError"].(bool); isErr {
		return nil, mcpErrorf("QMD %s failed: %v", name, result)
	}
	return result, nil
}

// dialTimeout is the network primitive Reachability uses; tests swap it to exercise
// error classification without a real socket.
var dialTimeout = net.DialTimeout

// Reachability probes whether something accepts TCP connections on port: open |
// refused | denied | error. A denied probe proves nothing about the server (it may be
// blocked by a sandbox), so it is never read as "down".
func Reachability(port int) (string, string) {
	conn, err := dialTimeout("tcp", net.JoinHostPort(Host, strconv.Itoa(port)), 2*time.Second)
	if err == nil {
		conn.Close()
		return "open", ""
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "refused", ""
	}
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return "denied", err.Error()
	}
	return "error", err.Error()
}

// reachability is the seam probe()/ownServerRunning()/lifecycle() call through; tests
// replace it so no test ever binds or connects to a real port.
var reachability = Reachability

// identity reports "" when status (a QMD `status` tool structuredContent) indexes
// exactly this project's collection folders, or a description of the mismatch.
func identity(status map[string]any, s *Settings) string {
	served := map[string]string{}
	have := map[string]bool{}
	if list, ok := status["collections"].([]any); ok {
		for _, item := range list {
			row, _ := item.(map[string]any)
			name, _ := row["name"].(string)
			path, _ := row["path"].(string)
			served[name] = path
			have[name] = true
		}
	}
	for _, name := range s.collectionNames() {
		path := s.Collections[name]
		if !have[name] {
			return fmt.Sprintf("collection '%s' is not served", name)
		}
		if resolvePath(served[name]) != path {
			return fmt.Sprintf("collection '%s' serves %s, expected %s", name, served[name], path)
		}
	}
	return ""
}

// textOf concatenates the text of every content item in an MCP tool result (a `get`
// call's document body, or its resource's text).
func textOf(result map[string]any) string {
	content, _ := result["content"].([]any)
	var b strings.Builder
	for _, item := range content {
		row, _ := item.(map[string]any)
		text, _ := row["text"].(string)
		if text == "" {
			if resource, ok := row["resource"].(map[string]any); ok {
				text, _ = resource["text"].(string)
			}
		}
		b.WriteString(text)
	}
	return b.String()
}

// documentKey normalizes a QMD search hit's file field for comparison: QMD 2.8.3
// returns "<collection>/<path>"; older output used "qmd://<collection>/<path>".
func documentKey(file string) string {
	return strings.ToLower(strings.TrimPrefix(file, "qmd://"))
}

// expectedURI is the "<collection>/<path>" of the smoke fixture's expected document.
func expectedURI(s *Settings) string {
	target := resolvePath(filepath.Join(s.Project.Root, s.Smoke.Expect))
	for _, name := range s.collectionNames() {
		folder := s.Collections[name]
		rel, err := filepath.Rel(folder, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if rel == "." {
			return name
		}
		return name + "/" + filepath.ToSlash(rel)
	}
	return ""
}

// truthy reports whether a JSON-ish value from a QMD status/result should be treated
// as true: nil, false, "", and zero numbers are false; everything else is true.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != ""
	case float64:
		return t != 0
	case float32:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return true
	}
}
