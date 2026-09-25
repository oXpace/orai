// Covers reachability, probe, smoke-result formatting, and MCPServers. Every QMD MCP
// call goes through a fake mcpClient; no test binds or connects to a real QMD server or
// port 8181.
package wiki

import (
	"net"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/oXpace/orai/internal/doctor"
)

// ---- reachability(): permission-denied must never be read as "down" ---------------

func TestReachabilityPermissionErrorIsDenied(t *testing.T) {
	saved := dialTimeout
	t.Cleanup(func() { dialTimeout = saved })
	dialTimeout = func(string, string, time.Duration) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EPERM)}
	}

	state, detail := Reachability(12345)
	if state != "denied" {
		t.Fatalf("state = %q, want denied", state)
	}
	if !strings.Contains(detail, "operation not permitted") {
		t.Fatalf("detail = %q, want mention of the permission error", detail)
	}
}

func TestReachabilityConnectionRefusedIsRefused(t *testing.T) {
	saved := dialTimeout
	t.Cleanup(func() { dialTimeout = saved })
	dialTimeout = func(string, string, time.Duration) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	}

	state, _ := Reachability(12345)
	if state != "refused" {
		t.Fatalf("state = %q, want refused", state)
	}
}

func TestProbeDeniedReasonDoesNotClaimTheServerIsDown(t *testing.T) {
	saved := reachability
	t.Cleanup(func() { reachability = saved })
	reachability = func(int) (string, string) { return "denied", "operation not permitted" }

	s := qmdSettings(t, false, 19222)
	checks := probe(s, false)
	if len(checks) != 1 {
		t.Fatalf("len(checks) = %d, want 1", len(checks))
	}
	if checks[0].Status != doctor.Blocked {
		t.Fatalf("status = %q, want blocked", checks[0].Status)
	}
	if !strings.Contains(checks[0].Reason, "does not show the server is down") {
		t.Fatalf("reason = %q, want it to say a denied probe proves nothing", checks[0].Reason)
	}
}

// ---- probe() layers, with a patched newClient --------------------------------------

func withOpenPort(t *testing.T) {
	t.Helper()
	saved := reachability
	t.Cleanup(func() { reachability = saved })
	reachability = func(int) (string, string) { return "open", "" }
}

func statusResult(s *Settings, total int, hasVector bool, needsEmbedding any) map[string]any {
	served := make([]any, 0, len(s.Collections))
	for _, name := range s.collectionNames() {
		served = append(served, map[string]any{"name": name, "path": s.Collections[name]})
	}
	return map[string]any{"structuredContent": map[string]any{
		"collections":    served,
		"totalDocuments": total,
		"hasVectorIndex": hasVector,
		"needsEmbedding": needsEmbedding,
	}}
}

func byComponent(checks []doctor.Check) map[string]doctor.Check {
	m := make(map[string]doctor.Check, len(checks))
	for _, c := range checks {
		m[c.Component] = c
	}
	return m
}

func TestProbeMcpHandshakeFailureBlocksMcp(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, false, 19222)
	setClient(t, &fakeClient{connectErr: mcpErrorf("handshake failed")})

	checks := probe(s, false)
	by := byComponent(checks)
	if by["wiki.server"].Status != doctor.Healthy {
		t.Fatalf("wiki.server status = %q, want healthy", by["wiki.server"].Status)
	}
	if by["wiki.mcp"].Status != doctor.Blocked {
		t.Fatalf("wiki.mcp status = %q, want blocked", by["wiki.mcp"].Status)
	}
	if !strings.Contains(strings.ToLower(by["wiki.mcp"].Reason), "handshake") {
		t.Fatalf("wiki.mcp reason = %q, want mention of handshake", by["wiki.mcp"].Reason)
	}
}

func TestProbeIdentityMismatchBlocksAndStopsFurtherChecks(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, false, 19222)
	other := map[string]any{"structuredContent": map[string]any{
		"collections": []any{map[string]any{"name": "docs", "path": "/somewhere/else"}},
	}}
	setClient(t, &fakeClient{status: func() map[string]any { return other }})

	checks := probe(s, false)
	var components []string
	for _, c := range checks {
		components = append(components, c.Component)
	}
	want := []string{"wiki.server", "wiki.mcp", "wiki.identity"}
	if len(components) != len(want) {
		t.Fatalf("components = %v, want %v", components, want)
	}
	for i := range want {
		if components[i] != want[i] {
			t.Fatalf("components = %v, want %v", components, want)
		}
	}
	last := checks[len(checks)-1]
	if last.Status != doctor.Blocked {
		t.Fatalf("status = %q, want blocked", last.Status)
	}
	if !strings.Contains(last.Reason, "serves another index") {
		t.Fatalf("reason = %q, want mention of another index", last.Reason)
	}
}

func TestProbeEmptyIndexIsNotReady(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, false, 19222)
	setClient(t, &fakeClient{status: func() map[string]any { return statusResult(s, 0, true, 0) }})

	checks := probe(s, false)
	last := checks[len(checks)-1]
	if last.Component != "wiki.index" {
		t.Fatalf("component = %q, want wiki.index", last.Component)
	}
	if last.Status != doctor.NotReady {
		t.Fatalf("status = %q, want not-ready", last.Status)
	}
}

func TestProbeNeedsEmbeddingIsDegraded(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, false, 19222)
	setClient(t, &fakeClient{status: func() map[string]any { return statusResult(s, 5, true, 3) }})

	checks := probe(s, false)
	last := checks[len(checks)-1]
	if last.Component != "wiki.index" {
		t.Fatalf("component = %q, want wiki.index", last.Component)
	}
	if last.Status != doctor.Degraded {
		t.Fatalf("status = %q, want degraded", last.Status)
	}
	if last.Detail["needsEmbedding"] != 3 {
		t.Fatalf("detail[needsEmbedding] = %v, want 3", last.Detail["needsEmbedding"])
	}
}

func TestProbeHealthyIndexNotDeepMarksSearchNotChecked(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, false, 19222)
	setClient(t, &fakeClient{status: func() map[string]any { return statusResult(s, 5, true, 0) }})

	checks := probe(s, false)
	last := checks[len(checks)-1]
	if last.Component != "wiki.search" {
		t.Fatalf("component = %q, want wiki.search", last.Component)
	}
	if last.Status != doctor.NotChecked {
		t.Fatalf("status = %q, want not-checked", last.Status)
	}
	if last.NextAction != "orai doctor --deep" {
		t.Fatalf("next action = %q, want orai doctor --deep", last.NextAction)
	}
	status, code := doctor.Overall(checks)
	if status != doctor.Healthy || code != doctor.ExitOK {
		t.Fatalf("Overall() = (%q, %d), want (healthy, 0)", status, code)
	}
}

func TestProbeDeepVectorFailureBlocksVectorAndLexicalSuccessCannotMaskIt(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, false, 19222)
	var calls []toolCall
	client := &fakeClient{
		status: func() map[string]any { return statusResult(s, 5, true, 0) },
		get: func(map[string]any) map[string]any {
			return map[string]any{"content": []any{map[string]any{"text": "hi"}}}
		},
		calls: &calls,
	}
	setClient(t, &erroringQueryClient{fakeClient: client})

	checks := probe(s, true)
	last := checks[len(checks)-1]
	if last.Component != "wiki.vector" {
		t.Fatalf("component = %q, want wiki.vector", last.Component)
	}
	if last.Status != doctor.Blocked {
		t.Fatalf("status = %q, want blocked", last.Status)
	}
	if !strings.Contains(last.Reason, "vector search failed") {
		t.Fatalf("reason = %q, want mention of a vector search failure", last.Reason)
	}
	for _, c := range calls {
		if c.Name == "get" {
			t.Fatalf("get must never run once vector search fails")
		}
		if c.Name == "query" {
			if searches, ok := c.Args["searches"].([]map[string]any); ok && len(searches) == 2 {
				t.Fatalf("the hybrid (lex+vec) query must never run once vector search fails")
			}
		}
	}
}

// erroringQueryClient wraps fakeClient so its "query" tool call fails for a
// single-search (vector-only) call while still recording every call.
type erroringQueryClient struct{ fakeClient *fakeClient }

func (c *erroringQueryClient) Connect() error { return c.fakeClient.Connect() }

func (c *erroringQueryClient) Tool(name string, args map[string]any) (map[string]any, error) {
	if c.fakeClient.calls != nil {
		*c.fakeClient.calls = append(*c.fakeClient.calls, toolCall{name, args})
	}
	if name == "query" {
		searches, _ := args["searches"].([]map[string]any)
		if len(searches) == 1 {
			return nil, mcpErrorf("vector backend unavailable")
		}
		return map[string]any{"structuredContent": map[string]any{"results": []any{map[string]any{"file": "docs/found.md"}}}}, nil
	}
	if name == "status" {
		return c.fakeClient.status(), nil
	}
	if name == "get" {
		return c.fakeClient.get(args), nil
	}
	return nil, mcpErrorf("unexpected tool call: %s", name)
}

func TestProbeDeepVectorOkButMissesSmokeDocumentIsDegraded(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, true, 19222)
	client := &fakeClient{
		status: func() map[string]any { return statusResult(s, 5, true, 0) },
		query: func(map[string]any) map[string]any {
			return map[string]any{"structuredContent": map[string]any{"results": []any{map[string]any{"file": "docs/other.md"}}}}
		},
		get: func(map[string]any) map[string]any {
			return map[string]any{"content": []any{map[string]any{"text": "hi"}}}
		},
	}
	setClient(t, client)

	checks := probe(s, true)
	var vector doctor.Check
	for _, c := range checks {
		if c.Component == "wiki.vector" {
			vector = c
		}
	}
	if vector.Status != doctor.Degraded {
		t.Fatalf("status = %q, want degraded", vector.Status)
	}
	if !strings.Contains(vector.Reason, "missed the smoke document") {
		t.Fatalf("reason = %q, want mention of the missed smoke document", vector.Reason)
	}
}

func TestProbeDeepSmokeConfiguredAndHitIsHealthy(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, true, 19222)
	expect := expectedURI(s)
	client := &fakeClient{
		status: func() map[string]any { return statusResult(s, 5, true, 0) },
		query: func(map[string]any) map[string]any {
			return map[string]any{"structuredContent": map[string]any{"results": []any{map[string]any{"file": expect}}}}
		},
		get: func(map[string]any) map[string]any {
			return map[string]any{"content": []any{map[string]any{"text": "hello world"}}}
		},
	}
	setClient(t, client)

	checks := probe(s, true)
	for _, c := range checks {
		if c.Status != doctor.Healthy {
			t.Fatalf("component %s status = %q, want healthy (all checks): %+v", c.Component, c.Status, checks)
		}
	}
}

func TestProbeDeepGetReturningEmptyTextIsDegraded(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, true, 19222)
	expect := expectedURI(s)
	client := &fakeClient{
		status: func() map[string]any { return statusResult(s, 5, true, 0) },
		query: func(map[string]any) map[string]any {
			return map[string]any{"structuredContent": map[string]any{"results": []any{map[string]any{"file": expect}}}}
		},
		get: func(map[string]any) map[string]any { return map[string]any{"content": []any{}} },
	}
	setClient(t, client)

	checks := probe(s, true)
	var hybrid doctor.Check
	for _, c := range checks {
		if c.Component == "wiki.hybrid" {
			hybrid = c
		}
	}
	if hybrid.Status != doctor.Degraded {
		t.Fatalf("status = %q, want degraded", hybrid.Status)
	}
	if !strings.Contains(hybrid.Reason, "no text") {
		t.Fatalf("reason = %q, want mention of no text", hybrid.Reason)
	}
}

// ---- smoke hit matches both real QMD result formats --------------------------------

func TestSmokeHitMatchesRealQMDResultFormats(t *testing.T) {
	withOpenPort(t)
	s := qmdSettings(t, true, 19222)
	if got := expectedURI(s); got != "docs/found.md" {
		t.Fatalf("expectedURI() = %q, want docs/found.md", got)
	}
	for _, returned := range []string{"docs/found.md", "qmd://docs/FOUND.md"} {
		client := &fakeClient{
			status: func() map[string]any { return statusResult(s, 5, true, 0) },
			query: func(map[string]any) map[string]any {
				return map[string]any{"structuredContent": map[string]any{"results": []any{map[string]any{"file": returned}}}}
			},
			get: func(map[string]any) map[string]any {
				return map[string]any{"content": []any{map[string]any{"text": "body"}}}
			},
		}
		setClient(t, client)
		checks := probe(s, true)
		var vector doctor.Check
		for _, c := range checks {
			if c.Component == "wiki.vector" {
				vector = c
			}
		}
		if vector.Status != doctor.Healthy {
			t.Fatalf("returned=%q: status = %q, want healthy", returned, vector.Status)
		}
	}
}
