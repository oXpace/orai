package shelf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/oXpace/orai/internal/doctor"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
)

// runCommand executes argv (the qmd.run equivalent): it prints "+ argv" to out, then
// runs the command with cwd set to the project root and settings.Env(). Tests replace
// this var so lifecycle() never spawns a real qmd.
var runCommand = func(out io.Writer, argv []string, s *Settings, capture bool) (string, error) {
	fmt.Fprintln(out, "+ "+strings.Join(argv, " "))
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = s.Project.Root
	cmd.Env = s.Env()
	cmd.Stderr = os.Stderr
	var buf bytes.Buffer
	if capture {
		cmd.Stdout = &buf
	} else {
		cmd.Stdout = os.Stdout
	}
	if err := cmd.Run(); err != nil {
		return buf.String(), fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return buf.String(), nil
}

// ownServerRunning reports whether our server is up. It raises when the port is
// unusable (denied/error) or serves something else, so callers never silently proceed
// against another project's server. It is a var so lifecycle tests can fix the
// "running" outcome without a fake server.
var ownServerRunning = func(s *Settings) (bool, error) {
	st, detail := reachability(s.Port)
	if st == "refused" {
		return false, nil
	}
	if st != "open" {
		return false, fmt.Errorf("cannot probe %s (%s: %s); run outside the sandbox", s.Endpoint, st, detail)
	}
	client := newClient(s.Endpoint, 15*time.Second)
	status, err := handshake(client)
	if err != nil {
		return false, err
	}
	if mismatch := identity(status, s); mismatch != "" {
		return false, fmt.Errorf("port %d serves another index (%s). Left untouched; set integrations.shelf.port.",
			s.Port, mismatch)
	}
	return true, nil
}

// Lifecycle runs init | recover | refresh | check | stop. See docs/operations.md.
// It never stops or deletes another project's server or DB: init/refresh refuse while
// our own server is running, and a port already serving a different index is left
// untouched.
func Lifecycle(p *project.Project, action string, out io.Writer) error {
	if p.Config.Shelf == nil {
		return errors.New("Shelf is not configured: add [integrations.shelf] to orai.toml (`orai shelf --help` lists the settings)")
	}
	s := NewSettings(p)
	for _, name := range s.collectionNames() {
		path := s.Collections[name]
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("collection '%s' folder is missing: %s. Create it or fix integrations.shelf.collections in orai.toml", name, path)
		}
	}
	if action == "check" {
		// check only talks to the server, so it never needs the engine on PATH. A project
		// that was never initialized should hear that, though, not "connection refused".
		if !exists(s.ConfigFile) || !exists(s.DB) {
			if _, err := lookPath("qmd"); err != nil {
				return errors.New("The shelf is not set up yet and its engine QMD is not installed. " + InstallHint)
			}
			if s.legacyOnly() {
				return errors.New("The index is still where Orai 0.3 kept it (.orai/wiki). Run `orai shelf recover` to move it to .orai/shelf.")
			}
			return errors.New("The shelf is not set up yet. Run `orai shelf init`.")
		}
		return verify(s, out)
	}
	qmd, err := lookPath("qmd")
	if err != nil {
		return errors.New("The shelf engine QMD is not installed. " + InstallHint)
	}
	if err := adoptLegacy(s, qmd, action, out); err != nil {
		return err
	}
	if err := state.PrivateDirs(s.Directory); err != nil {
		return err
	}

	lockPath := filepath.Join(s.Directory, "setup.lock")
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return errors.New("Another orai shelf command is running for this project")
		}
		return err
	}

	if action == "stop" {
		// The PID file is scoped to this project's index name; other servers are untouched.
		_, err := runCommand(out, s.Argv(qmd, "mcp", "stop"), s, false)
		return err
	}
	if action != "init" && (!exists(s.ConfigFile) || !exists(s.DB)) {
		return errors.New("Project shelf config/index missing. Run `orai shelf init`.")
	}
	running, err := ownServerRunning(s)
	if err != nil {
		return err
	}
	if running && (action == "init" || action == "refresh") {
		return fmt.Errorf("This project's shelf server is running. When no search is in progress, run "+
			"`orai shelf stop && orai shelf %s`.", action)
	}
	if !exists(s.ConfigFile) {
		// Exclusive creation: an existing config (model, collections) is never overwritten.
		if err := createConfigExclusive(s); err != nil {
			return err
		}
	}
	// Make the index hold exactly the collections orai.toml declares. init and refresh
	// apply the difference (they re-index anyway); recover only starts the server, so it
	// names the difference and the command that applies it. The model and the DB are
	// left as they are. Contexts are applied by all three (see context.go).
	apply := action == "init" || action == "refresh"
	stale := func(what string) error {
		return fmt.Errorf("%s. Run `orai shelf stop && orai shelf refresh` to apply orai.toml", what)
	}
	for _, name := range s.collectionNames() {
		path, pattern := s.Collections[name], s.Patterns[name]
		add := s.Argv(qmd, "collection", "add", path, "--name", name, "--mask", pattern)
		shown, err := runCommand(out, s.Argv(qmd, "collection", "show", name), s, true)
		if err != nil {
			if !apply {
				return stale(fmt.Sprintf("collection '%s' is in orai.toml but not in the index yet", name))
			}
			if _, err := runCommand(out, add, s, false); err != nil {
				return err
			}
			continue
		}
		havePath, havePattern := shownField(shown, "Path:"), shownField(shown, "Pattern:")
		if havePath == path && (havePattern == "" || havePattern == pattern) {
			continue
		}
		change := fmt.Sprintf("collection '%s' is indexed as %s (%s) but orai.toml says %s (%s)", name, havePath, havePattern, path, pattern)
		if !apply {
			return stale(change)
		}
		fmt.Fprintln(out, "Re-registering: "+change)
		if _, err := runCommand(out, s.Argv(qmd, "collection", "remove", name), s, false); err != nil {
			return err
		}
		if _, err := runCommand(out, add, s, false); err != nil {
			return err
		}
	}
	if apply {
		listed, err := runCommand(out, s.Argv(qmd, "collection", "list"), s, true)
		if err != nil {
			return err
		}
		for _, name := range listedCollections(listed) {
			if _, declared := s.Collections[name]; declared {
				continue
			}
			fmt.Fprintf(out, "Removing collection '%s' from the index: orai.toml no longer declares it\n", name)
			if _, err := runCommand(out, s.Argv(qmd, "collection", "remove", name), s, false); err != nil {
				return err
			}
		}
		for _, pair := range s.overlapping() {
			fmt.Fprintf(out, "Note: collections %s cover the same documents, so they are indexed twice. %s\n", pair, overlapFix)
		}
	}
	if err := applyContexts(s, qmd, out); err != nil {
		return err
	}
	if action == "init" || action == "refresh" {
		if _, err := runCommand(out, s.Argv(qmd, "update"), s, false); err != nil {
			return err
		}
		if _, err := runCommand(out, s.Argv(qmd, "embed"), s, false); err != nil {
			return err
		}
	}
	if !running {
		daemon := s.Argv(qmd, "mcp", "--http", "--daemon", "--host", Host, "--port", strconv.Itoa(s.Port))
		if _, err := runCommand(out, daemon, s, false); err != nil {
			return err
		}
		ready := false
		for i := 0; i < 30; i++ {
			if st, _ := reachability(s.Port); st == "open" {
				ready = true
				break
			}
			time.Sleep(time.Second)
		}
		if !ready {
			return fmt.Errorf("Server did not become reachable within 30 seconds; see %s", withSuffix(s.PIDFile(), ".log"))
		}
	}
	return verify(s, out)
}

// adoptLegacy moves an index Orai 0.3 built in .orai/wiki to .orai/shelf. The server
// that index started is stopped first (its PID file is keyed by the index name, which
// did not change), so nothing keeps the old path open. The index itself is not rebuilt.
// For `stop` the move is all that happens here: the action stops the server itself, and
// stopping twice would fail the second time.
func adoptLegacy(s *Settings, qmd, action string, out io.Writer) error {
	if !s.legacyOnly() {
		return nil
	}
	fmt.Fprintf(out, "Moving the index from %s to %s (made by Orai 0.3)\n", s.LegacyDirectory(), s.Directory)
	if action != "stop" {
		// A server that is not running makes `mcp stop` fail; that is the state we want.
		_, _ = runCommand(out, s.Argv(qmd, "mcp", "stop"), s, false)
	}
	// Left behind by an interrupted move; os.Remove only succeeds on an empty folder.
	_ = os.Remove(filepath.Join(s.Directory, "setup.lock"))
	_ = os.Remove(s.Directory)
	if err := os.Rename(s.LegacyDirectory(), s.Directory); err != nil {
		return fmt.Errorf("cannot move %s to %s: %w", s.LegacyDirectory(), s.Directory, err)
	}
	return nil
}

// overlapFix is what to change when two collections cover the same documents.
const overlapFix = "Give the outer one a pattern that stays in its own folder, such as { path = \"docs\", pattern = \"*.md\" }"

// shownField returns one field of `qmd collection show` ("" when absent).
func shownField(shown, label string) string {
	for _, line := range strings.Split(shown, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, label) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, label))
		}
	}
	return ""
}

// listedCollections returns the collection names in `qmd collection list` output, where
// each collection starts a line as "name (qmd://name/)".
func listedCollections(listed string) []string {
	var names []string
	for _, line := range strings.Split(listed, "\n") {
		name, rest, found := strings.Cut(line, " (qmd://")
		if found && name != "" && !strings.ContainsAny(name, " \t") && strings.HasPrefix(rest, name+"/)") {
			names = append(names, name)
		}
	}
	return names
}

func createConfigExclusive(s *Settings) error {
	file, err := os.OpenFile(s.ConfigFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.MarshalIndent(s.configValue(), "", "  ")
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	_, err = file.Write([]byte("\n"))
	return err
}

// verify runs a deep probe and reports it: every check line, then success or a
// RuntimeError-equivalent that never claims a failed probe is healthy. It is a var so
// lifecycle tests can stub the whole verification step.
var verify = func(s *Settings, out io.Writer) error {
	checks := probe(s, true)
	var failed []string
	next := ""
	for _, c := range checks {
		fmt.Fprintf(out, "%9s  %s: %s\n", c.Status, c.Component, c.Reason)
		if c.Status != doctor.Healthy {
			failed = append(failed, c.Component+" "+c.Status)
			if next == "" {
				next = c.NextAction
			}
		}
	}
	if len(failed) > 0 {
		if next != "" {
			next = ". Next: " + next
		}
		return errors.New("Shelf is not verified: " + strings.Join(failed, "; ") + next)
	}
	fmt.Fprintf(out, "Ready: %s (MCP server name: %s)\n", s.Endpoint, s.ServerName)
	return nil
}

func withSuffix(path, suffix string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + suffix
}
