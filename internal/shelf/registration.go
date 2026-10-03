package shelf

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/oXpace/orai/internal/doctor"
)

// Role sessions get the shelf from Orai at launch. Anything else that works in this
// project (a desktop app, a plain `claude` or `codex`) reads the project's own MCP
// settings, where the server address is written out. Those files are the user's; Orai
// only reads them and says whether what they hold still points at this project's shelf.
//
//	.mcp.json            Claude Code, project scope: mcpServers.<name>.url
//	.codex/config.toml   Codex, project config:      mcp_servers.<name>.url
var registrationFiles = []string{".mcp.json", ".codex/config.toml"}

// entry is one MCP server found in a project settings file.
type entry struct{ File, Name, URL string }

// localEntries reads the URL-based MCP servers declared in the project's own settings
// files. A file that is missing declares nothing; one that cannot be parsed is reported.
func localEntries(root string) (entries []entry, unreadable []string) {
	for _, rel := range registrationFiles {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		var parsed map[string]any
		key := "mcpServers"
		if strings.HasSuffix(rel, ".toml") {
			key = "mcp_servers"
			err = toml.Unmarshal(data, &parsed)
		} else {
			err = json.Unmarshal(data, &parsed)
		}
		if err != nil {
			unreadable = append(unreadable, rel)
			continue
		}
		servers, _ := parsed[key].(map[string]any)
		names := make([]string, 0, len(servers))
		for name := range servers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			server, _ := servers[name].(map[string]any)
			address, _ := server["url"].(string)
			entries = append(entries, entry{File: rel, Name: name, URL: address})
		}
	}
	return entries, unreadable
}

// sameEndpoint reports whether address is this project's shelf server: the loopback
// host under any of its spellings, the same port and the /mcp path.
func sameEndpoint(address string, port int) bool {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	switch parsed.Hostname() {
	case Host, "localhost", "::1":
	default:
		return false
	}
	return parsed.Port() == strconv.Itoa(port) && strings.TrimSuffix(parsed.Path, "/") == "/mcp"
}

// claudeServer asks Claude Code for a server by name across all of its scopes (user,
// project, local). It returns the CLI's description, or "" when there is none. It is a
// var so tests never run the real CLI or read the user's own configuration.
var claudeServer = func(name string) string {
	path, err := exec.LookPath("claude")
	if err != nil {
		return ""
	}
	cmd := exec.Command(path, "mcp", "get", name)
	done := make(chan []byte, 1)
	go func() {
		out, err := cmd.Output()
		if err != nil {
			out = nil
		}
		done <- out
	}()
	select {
	case out := <-done:
		return string(out)
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		return ""
	}
}

// registration is the doctor check for the project's own MCP settings: whether the
// shelf is written there, and whether what is written is this project's server.
func registration(s *Settings, deep bool) doctor.Check {
	const component = "shelf.registration"
	entries, unreadable := localEntries(s.Project.Root)
	pinned := s.Project.Config.Shelf.Port != 0
	pin := fmt.Sprintf("set `port = %d` under [integrations.shelf] in orai.toml so the address is the same in every checkout", s.Port)

	var registered, problems, fixes []string
	written := false // the address is written in a project file, so the port should be fixed
	for _, e := range entries {
		written = written || e.Name == ServerName || sameEndpoint(e.URL, s.Port)
		switch {
		case e.Name == ServerName && sameEndpoint(e.URL, s.Port):
			registered = append(registered, e.File)
		case e.Name == ServerName:
			problems = append(problems, fmt.Sprintf("%s has `%s` at %s, but this project's shelf is %s", e.File, e.Name, orNone(e.URL), s.Endpoint))
			fixes = append(fixes, fmt.Sprintf("In %s set the url of `%s` to %s", e.File, ServerName, s.Endpoint))
		case sameEndpoint(e.URL, s.Port):
			// The server under an earlier name (0.3 registered it as wiki-<project>).
			problems = append(problems, fmt.Sprintf("%s registers this project's shelf as `%s`", e.File, e.Name))
			fixes = append(fixes, fmt.Sprintf("In %s rename `%s` to `%s`", e.File, e.Name, ServerName))
		}
	}
	for _, rel := range unreadable {
		problems = append(problems, rel+" could not be parsed")
		fixes = append(fixes, "Fix the syntax of "+rel)
	}
	// Claude Code also has user and local scopes, kept outside the project. Which entry
	// a role session ends up with when one of those is also named `shelf` is not
	// documented, so one that points elsewhere is reported. It runs the CLI: --deep only.
	if deep && s.usesClaude() {
		if found := claudeServer(ServerName); found != "" && !strings.Contains(found, strconv.Itoa(s.Port)+"/mcp") {
			problems = append(problems, "Claude Code has another MCP server named `"+ServerName+"` in its own settings; a role session may end up searching that one")
			fixes = append(fixes, "`claude mcp get "+ServerName+"` shows it; rename or remove it")
		}
	}
	detail := map[string]any{"endpoint": s.Endpoint, "port_pinned": pinned, "files": registered}
	if len(problems) > 0 {
		if !pinned && written {
			fixes = append(fixes, strings.ToUpper(pin[:1])+pin[1:])
		}
		return doctor.New(component, doctor.Degraded, strings.Join(problems, "; "), strings.Join(fixes, ". ")).WithDetail(detail)
	}
	if len(registered) == 0 {
		// Optional, and shown on every run: one line, with the details in --help.
		return doctor.New(component, doctor.NotConfigured,
			"not written in this project's own MCP settings ("+strings.Join(registrationFiles, ", ")+"); role sessions are connected by Orai and do not need it",
			fmt.Sprintf("Only for use outside role sessions: register `%s` at %s (`orai shelf --help` shows how)", ServerName, s.Endpoint)).WithDetail(detail)
	}
	reason := "registered in " + strings.Join(registered, ", ") + " at " + s.Endpoint
	if !pinned {
		// Right on this machine, but the port comes from this checkout's path.
		return doctor.New(component, doctor.Degraded, reason+", but the port is derived from this checkout's path and is not fixed in orai.toml",
			strings.ToUpper(pin[:1])+pin[1:]).WithDetail(detail)
	}
	return doctor.New(component, doctor.Healthy, reason, "").WithDetail(detail)
}

func (s *Settings) usesClaude() bool {
	for _, role := range s.Project.Config.Roles {
		if role.Provider == "claude" {
			return true
		}
	}
	return false
}

func orNone(text string) string {
	if text == "" {
		return "no url"
	}
	return text
}
