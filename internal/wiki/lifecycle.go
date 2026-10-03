package wiki

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
		return false, fmt.Errorf("port %d serves another index (%s). Left untouched; set integrations.wiki.port.",
			s.Port, mismatch)
	}
	return true, nil
}

// Lifecycle runs init | recover | refresh | check | stop. See docs/operations.md.
// It never stops or deletes another project's server or DB: init/refresh refuse while
// our own server is running, and a port already serving a different index is left
// untouched.
func Lifecycle(p *project.Project, action string, out io.Writer) error {
	if p.Config.Wiki == nil {
		return errors.New("Wiki is not configured: add [integrations.wiki] to orai.toml (`orai wiki --help` lists the settings)")
	}
	s := NewSettings(p)
	for _, name := range s.collectionNames() {
		path := s.Collections[name]
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("collection '%s' folder is missing: %s. Create it or fix integrations.wiki.collections in orai.toml", name, path)
		}
	}
	if action == "check" {
		// check only talks to the server, so it never needs the engine on PATH. A project
		// that was never initialized should hear that, though, not "connection refused".
		if !exists(s.ConfigFile) || !exists(s.DB) {
			if _, err := lookPath("qmd"); err != nil {
				return errors.New("The wiki is not set up yet and its engine QMD is not installed. " + InstallHint)
			}
			return errors.New("The wiki is not set up yet. Run `orai wiki init`.")
		}
		return verify(s, out)
	}
	qmd, err := lookPath("qmd")
	if err != nil {
		return errors.New("The wiki engine QMD is not installed. " + InstallHint)
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
			return errors.New("Another orai wiki command is running for this project")
		}
		return err
	}

	if action == "stop" {
		// The PID file is scoped to this project's index name; other servers are untouched.
		_, err := runCommand(out, s.Argv(qmd, "mcp", "stop"), s, false)
		return err
	}
	if action != "init" && (!exists(s.ConfigFile) || !exists(s.DB)) {
		return errors.New("Project wiki config/index missing. Run `orai wiki init`.")
	}
	running, err := ownServerRunning(s)
	if err != nil {
		return err
	}
	if running && (action == "init" || action == "refresh") {
		return fmt.Errorf("This project's wiki server is running. When no search is in progress, run "+
			"`orai wiki stop && orai wiki %s`.", action)
	}
	if !exists(s.ConfigFile) {
		// Exclusive creation: an existing config (model, collections) is never overwritten.
		if err := createConfigExclusive(s); err != nil {
			return err
		}
	}
	for _, name := range s.collectionNames() {
		path := s.Collections[name]
		shown, err := runCommand(out, s.Argv(qmd, "collection", "show", name), s, true)
		if err != nil {
			// A collection declared in orai.toml after the index was created. Adding it
			// leaves the existing collections, model and DB as they are.
			if action == "recover" {
				return fmt.Errorf("collection '%s' is in orai.toml but not in the wiki index yet. Run "+
					"`orai wiki stop && orai wiki refresh` to add it", name)
			}
			if _, err := runCommand(out, s.Argv(qmd, "collection", "add", path, "--name", name, "--mask", "**/*.md"), s, false); err != nil {
				return err
			}
			continue
		}
		var paths []string
		for _, line := range strings.Split(shown, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Path:") {
				parts := strings.SplitN(trimmed, ":", 2)
				paths = append(paths, strings.TrimSpace(parts[1]))
			}
		}
		if len(paths) != 1 || paths[0] != path {
			return fmt.Errorf("collection '%s' points elsewhere (%v); fix %s. Config and DB preserved.",
				name, paths, s.ConfigFile)
		}
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
		return errors.New("Wiki is not verified: " + strings.Join(failed, "; ") + next)
	}
	fmt.Fprintf(out, "Ready: %s (MCP server name: %s)\n", s.Endpoint, s.ServerName)
	return nil
}

func withSuffix(path, suffix string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + suffix
}
