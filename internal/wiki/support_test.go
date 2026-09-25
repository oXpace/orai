// Shared test helpers: temporary projects and fake MCP clients. Every qmd invocation
// goes through the runCommand/lookPath/reachability/newClient/probe/verify/
// ownServerRunning package vars, so no test binds or connects to a real port, spawns a
// real qmd, or talks to a real QMD server.
package wiki

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oXpace/orai/internal/project"
)

func writeProject(t *testing.T, root, text string) *project.Project {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, project.ConfigName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := project.Load(root)
	if err != nil {
		t.Fatalf("project.Load(%s): %v", root, err)
	}
	return p
}

// makeQMDProject creates a project with [integrations.wiki] (an explicit port when
// != 0) and an empty docs/ folder.
func makeQMDProject(t *testing.T, root string, port int) *project.Project {
	t.Helper()
	text := "schema = 1\nsession = \"orai\"\n\n[integrations.wiki]\n"
	if port != 0 {
		text += fmt.Sprintf("port = %d\n", port)
	}
	p := writeProject(t, root, text)
	if err := os.MkdirAll(filepath.Join(p.Root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// qmdSettings builds a project's wiki Settings for the given port, optionally with a
// [integrations.wiki.smoke] block when smoke is true.
func qmdSettings(t *testing.T, smoke bool, port int) *Settings {
	t.Helper()
	text := fmt.Sprintf("schema = 1\nsession = \"orai\"\n\n[integrations.wiki]\nport = %d\n", port)
	if smoke {
		text += "\n[integrations.wiki.smoke]\nlex = \"overview\"\nvec = \"architecture\"\nexpect = \"docs/found.md\"\n"
	}
	p := writeProject(t, filepath.Join(t.TempDir(), "proj"), text)
	return NewSettings(p)
}

func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	return m
}

func indexOf(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

func containsStr(items []string, want string) bool {
	return indexOf(items, want) >= 0
}

// installSuccessMocks patches every side-effecting collaborator lifecycle() uses so it
// runs hermetically, and restores the real implementations when the test ends.
func installSuccessMocks(t *testing.T, running bool, verifyErr error, binary string) *[][]string {
	t.Helper()
	savedLookPath, savedOwnServerRunning, savedReachability, savedVerify, savedRunCommand :=
		lookPath, ownServerRunning, reachability, verify, runCommand
	t.Cleanup(func() {
		lookPath, ownServerRunning, reachability, verify, runCommand =
			savedLookPath, savedOwnServerRunning, savedReachability, savedVerify, savedRunCommand
	})

	lookPath = func(string) (string, error) { return binary, nil }
	ownServerRunning = func(*Settings) (bool, error) { return running, nil }
	reachability = func(int) (string, string) { return "open", "" }
	verify = func(*Settings, io.Writer) error { return verifyErr }

	calls := &[][]string{}
	runCommand = func(_ io.Writer, argv []string, s *Settings, capture bool) (string, error) {
		*calls = append(*calls, append([]string(nil), argv...))
		if capture {
			name := argv[len(argv)-1]
			return fmt.Sprintf("Path: %s\n", s.Collections[name]), nil
		}
		return "", nil
	}
	return calls
}

// fakeClient is a stand-in for the real MCP Client that never opens a socket.
// status/query/get return the *full* tool result, matching what Client.Tool would
// return.
type fakeClient struct {
	connectErr error
	status     func() map[string]any
	query      func(args map[string]any) map[string]any
	get        func(args map[string]any) map[string]any
	calls      *[]toolCall
}

type toolCall struct {
	Name string
	Args map[string]any
}

func (c *fakeClient) Connect() error { return c.connectErr }

func (c *fakeClient) Tool(name string, args map[string]any) (map[string]any, error) {
	if c.calls != nil {
		*c.calls = append(*c.calls, toolCall{name, args})
	}
	switch name {
	case "status":
		if c.status != nil {
			return c.status(), nil
		}
	case "query":
		if c.query != nil {
			return c.query(args), nil
		}
	case "get":
		if c.get != nil {
			return c.get(args), nil
		}
	}
	return nil, fmt.Errorf("unexpected tool call: %s", name)
}

// setClient installs a newClient factory that always returns client, restored at the
// end of the test.
func setClient(t *testing.T, client mcpClient) {
	t.Helper()
	saved := newClient
	t.Cleanup(func() { newClient = saved })
	newClient = func(string, time.Duration) mcpClient { return client }
}
