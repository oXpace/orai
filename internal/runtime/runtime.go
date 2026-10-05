// Package runtime owns the role session lifecycle: launch, exact resume, identity
// capture, notification and status. Orai owns the mailbox; providers own conversations.
// Orai only records which exact conversation belongs to which role and keeps one live
// process per role.
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/mail"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/providers"
	"github.com/oXpace/orai/internal/scaffold"
	"github.com/oXpace/orai/internal/state"
)

// MCPServers returns per-process MCP entries for a provider (wired to the shelf by the CLI).
var MCPServers = func(*project.Project, string) map[string]map[string]any { return map[string]map[string]any{} }

// Executable is the running orai binary, used for hooks and the Claude channel.
var Executable = func() string {
	exe, err := os.Executable()
	if err != nil {
		return "orai"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

func NewUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CanonicalUUID validates and normalizes a UUID string.
func CanonicalUUID(s string) (string, error) {
	hex := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "")
	hex = strings.TrimPrefix(strings.TrimSuffix(strings.TrimPrefix(hex, "urn:uuid:"), "}"), "{")
	if len(hex) != 32 {
		return "", fmt.Errorf("badly formed UUID %q", s)
	}
	for _, c := range hex {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return "", fmt.Errorf("badly formed UUID %q", s)
		}
	}
	return hex[0:8] + "-" + hex[8:12] + "-" + hex[12:16] + "-" + hex[16:20] + "-" + hex[20:32], nil
}

// guideStart reports what the role's guide says about opening a session: whether it has
// a "세션 시작" section at all, and whether it tells a Claude role to call
// channel_ready. Without that call no notification arrives, so a guide that lacks it
// (written by hand, or for another provider) is not relied on for it.
func guideStart(p *project.Project, role config.Role) (section, channel bool) {
	if role.Guide == "" {
		return false, false
	}
	data, err := os.ReadFile(filepath.Join(p.Root, role.Guide))
	if err != nil {
		return false, false
	}
	text := string(data)
	return strings.Contains(text, scaffold.StartHeading), strings.Contains(text, "channel_ready")
}

// Kickoff is the prompt a role session starts with. It carries only the basics; working
// rules live in the documents it points at.
//
// A new conversation gets who it is, the project's root folder (the one absolute path,
// so the session knows where the project lives), the documents to read (project instructions, the
// role's guide from orai.toml, the messaging skill) and the start steps. A reopened one
// already knows all that, so it gets the steps and one line of pointers.
//
// The steps every role shares are written here. What only some roles need (a Claude
// role calling channel_ready) lives in the role's guide under "세션 시작", and the
// prompt points at that section when the guide has one.
func Kickoff(p *project.Project, role config.Role, resumed bool) string {
	// Document paths are relative to the project folder, exactly as orai.toml writes
	// them; the project folder itself is the one absolute path in the prompt.
	guide := role.Guide
	agents := "AGENTS.md"
	section, channel := guideStart(p, role)
	var own []string
	if role.Provider == "claude" && !(section && channel) {
		own = append(own, "orai MCP의 `channel_ready`를 호출해 수신 준비를 알린다.")
	}
	if section {
		own = append(own, "역할 지침의 \"세션 시작\" 절을 따른다.")
	}

	if resumed {
		docs := agents
		if guide != "" {
			docs += ", " + guide
		}
		steps := append(own,
			"`orai msg inbox`로 받은 메시지를 확인한다.",
			"메시지가 없으면 하던 작업을 이어간다. 할 일이 없으면 턴을 끝낸다.")
		return numbered([]string{fmt.Sprintf("Orai `%s` 역할 세션을 다시 열었다.", role.Name), ""}, steps) +
			"\n지침은 바뀌었거나 기억나지 않을 때만 다시 본다 (프로젝트 폴더 " + p.Root + " 기준): " + docs + "\n"
	}

	if guide == "" {
		guide = "없음 (`orai.toml`의 `[roles." + role.Name + "]`에 `guide`를 적으면 연결된다)"
	}
	lines := []string{
		"# Orai 역할 세션",
		"",
		"- 프로젝트: " + p.Name(),
		"- 역할: " + role.Name,
		"- 프로젝트 폴더: " + p.Root,
		"",
		"## 문서 (경로는 프로젝트 폴더 기준)",
		"",
		"- 프로젝트 지침: " + agents,
		"- 역할 지침: " + guide,
		"- 메시지 사용법: .agents/skills/orai/SKILL.md",
		"",
		"## 시작",
		"",
	}
	steps := append([]string{"위 문서를 읽는다."}, own...)
	steps = append(steps,
		"`orai msg inbox`로 받은 메시지를 확인한다.",
		"처리할 것이 없으면 다른 일을 찾지 말고 턴을 끝낸다. 새 메시지는 오라이가 알린다.")
	return numbered(lines, steps)
}

// numbered appends steps to lines as a numbered list and joins everything.
func numbered(lines, steps []string) string {
	for i, step := range steps {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, step))
	}
	return strings.Join(lines, "\n") + "\n"
}

// RecoveryContext is re-injected by the SessionStart hook (startup, resume, compact): a
// one-line reminder of identity and where the documents are, never a second bootstrap.
func RecoveryContext(p *project.Project, role config.Role) string {
	guide := role.Guide
	if guide == "" {
		guide = "없음"
	}
	return fmt.Sprintf("Orai 역할 세션: 역할=%s, 프로젝트 폴더=%s, 프로젝트 지침=AGENTS.md, 역할 지침=%s (프로젝트 폴더 기준 경로). 하던 작업을 이어간다.",
		role.Name, p.Root, guide)
}

// ShellQuote quotes a word for POSIX shells (hook commands run through a shell).
func ShellQuote(word string) string {
	if word != "" && strings.Trim(word, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./-_") == "" {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'"'"'`) + "'"
}

func HookCommand(p *project.Project) string {
	words := []string{Executable(), "--project", p.Root, "_capture"}
	for i, w := range words {
		words[i] = ShellQuote(w)
	}
	return strings.Join(words, " ")
}

// ProviderArgs returns the provider argv and the session id bound before launch (Claude fresh).
func ProviderArgs(p *project.Project, role config.Role, savedID string) ([]string, string) {
	prompt := Kickoff(p, role, savedID != "")
	servers := MCPServers(p, role.Provider)
	if role.Provider == "codex" {
		return providers.CodexArgs(role, savedID, prompt, HookCommand(p), p.Root, servers), ""
	}
	args, bound := providers.ClaudeArgs(role, savedID, NewUUID(), prompt, HookCommand(p), p.Root, servers,
		p.Name()+"/"+role.Name, []string{Executable(), "_channel"})
	if savedID != "" {
		bound = ""
	}
	return args, bound
}

func str(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

// Capture is the SessionStart hook: bind the provider's conversation ID to this run only.
func Capture(p *project.Project, in io.Reader, out io.Writer) error {
	roleName, nonce := os.Getenv("ORAI_ROLE"), os.Getenv("ORAI_RUN_NONCE")
	if roleName == "" || nonce == "" {
		return nil
	}
	var event map[string]any
	if err := json.NewDecoder(in).Decode(&event); err != nil {
		return fmt.Errorf("hook input: %w", err)
	}
	if str(event, "hook_event_name") != "SessionStart" {
		return nil
	}
	role, ok := p.Config.Roles[roleName]
	if !ok {
		return fmt.Errorf("role %q is not configured in %s", roleName, p.ConfigPath())
	}
	sid, err := CanonicalUUID(str(event, "session_id"))
	if err != nil {
		return err
	}
	files := p.RoleFiles(roleName)
	current, err := state.ReadJSON(files.State())
	if err != nil {
		return err
	}
	if current == nil {
		current = map[string]any{}
	}
	if str(current, "nonce") != nonce || (str(current, "status") != "starting" && str(current, "status") != "running") {
		return errors.New("stale launch hook; refusing identity capture")
	}
	if !samePath(str(event, "cwd"), str(current, "cwd")) {
		return errors.New("session cwd does not match the role worktree")
	}
	if saved := str(current, "session_id"); saved != "" && saved != sid {
		return errors.New("unexpected conversation identity; use --fresh explicitly")
	}
	current["session_id"] = sid
	if transcript := str(event, "transcript_path"); transcript != "" {
		current["transcript"] = transcript
	}
	current["status"] = "running"
	current["captured_at"] = float64(time.Now().UnixNano()) / 1e9
	if err := state.WriteJSON(files.State(), current); err != nil {
		return err
	}
	data, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": "SessionStart", "additionalContext": RecoveryContext(p, role)}})
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	if ra == "" {
		ra = filepath.Clean(a)
	}
	if rb == "" {
		rb = filepath.Clean(b)
	}
	return ra == rb
}

// ValidateSaved accepts only the exact recorded conversation; never "latest".
func ValidateSaved(saved map[string]any, role config.Role, cwd string) error {
	if saved == nil {
		return nil
	}
	if str(saved, "provider") != role.Provider || str(saved, "cwd") != cwd {
		return errors.New("saved provider/worktree changed; inspect state and use --fresh")
	}
	if str(saved, "session_id") == "" {
		return errors.New("previous identity was not captured. Review CLI hook trust, then use --fresh")
	}
	if _, err := CanonicalUUID(str(saved, "session_id")); err != nil {
		return err
	}
	transcript := str(saved, "transcript")
	if info, err := os.Stat(transcript); transcript == "" || err != nil || !info.Mode().IsRegular() {
		return errors.New("saved conversation transcript is unavailable; restore it or use --fresh")
	}
	return nil
}

func RoleWorktree(p *project.Project, role config.Role) (string, error) {
	cwd := filepath.Join(p.Root, role.Worktree)
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		cwd = resolved
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return "", fmt.Errorf("missing role worktree: %s", cwd)
	}
	return cwd, nil
}

// CheckWorktree: a separate role directory must be a worktree root, on its declared
// branch. worktree = "." is the project root itself, which may be a repo subdirectory.
func CheckWorktree(role config.Role, cwd string) error {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return fmt.Errorf("cannot run git: %v", err)
		}
		if role.Branch != "" {
			return fmt.Errorf("%s declares branch %q but %s is not a Git worktree", role.Name, role.Branch, cwd)
		}
		return nil
	}
	top := strings.TrimSpace(string(out))
	if resolved, err := filepath.EvalSymlinks(top); err == nil {
		top = resolved
	}
	if role.Worktree != "." && top != cwd {
		return errors.New("role directory is not a Git worktree root")
	}
	if role.Branch != "" {
		cmd := exec.Command("git", "branch", "--show-current")
		cmd.Dir = cwd
		current, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(current)) != role.Branch {
			return fmt.Errorf("%s worktree must be on branch %s", role.Name, role.Branch)
		}
	}
	return nil
}

// RoleIdentity is the fixed identity of the role session this process runs in, or "".
func RoleIdentity() (string, error) {
	role := os.Getenv("ORAI_ROLE")
	if role == "" {
		return "", nil
	}
	if !config.NamePattern.MatchString(role) {
		return "", errors.New("invalid ORAI_ROLE; reconnect the role")
	}
	if os.Getenv("ORAI_PROJECT") == "" || os.Getenv("ORAI_SESSION") == "" ||
		(os.Getenv("AM_ME") != "" && os.Getenv("AM_ME") != role) {
		return "", errors.New("incomplete Orai messaging environment; reconnect the role")
	}
	return role, nil
}

// foreign context inherited from another project's shell must never reach a role.
func childEnv(p *project.Project, role config.Role, nonce string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AM_") || strings.HasPrefix(kv, "AMQ_GLOBAL_ROOT=") || strings.HasPrefix(kv, "ORAI_") {
			continue
		}
		env = append(env, kv)
	}
	root := p.MailRoot()
	return append(env,
		"ORAI_ROLE="+role.Name, "ORAI_RUN_NONCE="+nonce, "ORAI_STATE_DIR="+p.RolesDir(), "ORAI_PROJECT="+p.Root,
		"ORAI_SESSION="+p.Config.Session, "ORAI_MAIL_ROOT="+root,
		// The mailbox is AMQ-compatible; these let `amq` itself work inside a role session.
		"AM_ROOT="+root, "AM_BASE_ROOT="+filepath.Dir(root), "AM_ME="+role.Name, "AM_SESSION="+p.Config.Session)
}

func interactive() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Launch starts or exactly resumes a role and blocks until its CLI exits.
func Launch(p *project.Project, roleName string, fresh, dry bool, out io.Writer) (int, error) {
	if role, _ := RoleIdentity(); role != "" {
		return 1, errors.New("already inside an Orai role session; start roles from a normal terminal")
	}
	role, ok := p.Config.Roles[roleName]
	if !ok {
		known := strings.Join(p.Config.RoleNames(), ", ")
		if known == "" {
			known = "none"
		}
		return 2, &UsageError{fmt.Sprintf("unknown role %q (configured: %s)", roleName, known)}
	}
	cwd, err := RoleWorktree(p, role)
	if err != nil {
		return 1, err
	}
	if err := CheckWorktree(role, cwd); err != nil {
		return 1, err
	}
	files := p.RoleFiles(role.Name)
	var saved map[string]any
	if !fresh {
		if saved, err = state.ReadJSON(files.State()); err != nil {
			return 1, err
		}
	}
	if err := ValidateSaved(saved, role, cwd); err != nil {
		return 1, err
	}
	savedID := str(saved, "session_id")
	args, bound := ProviderArgs(p, role, savedID)
	if dry {
		var sid any
		if savedID != "" {
			sid = savedID
		}
		data, _ := json.MarshalIndent(map[string]any{"project": p.Root, "role": role.Name, "cwd": cwd,
			"resume": savedID != "", "session_id": sid, "mail_root": p.MailRoot(), "command": args}, "", "  ")
		fmt.Fprintln(out, string(data))
		return 0, nil
	}
	if !interactive() {
		return 1, errors.New("run orai in an interactive terminal; --dry-run is non-interactive")
	}
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return 1, fmt.Errorf("%s is not installed: %w", args[0], err)
	}
	lock, err := state.RoleLock(files)
	if err != nil {
		return 1, err
	}
	defer lock.Close()
	root := mail.Root(p.MailRoot())
	if missing := root.Missing(p.Config.Handles()); len(missing) > 0 && root.Initialized() {
		fmt.Fprintf(out, "orai: adding mailbox(es) %s to %s\n", strings.Join(missing, ", "), root)
	}
	if err := mail.Init(root, p.Config.Handles()); err != nil {
		return 1, err
	}
	previous, err := state.ReadJSON(files.State())
	if err != nil {
		return 1, err
	}
	next := map[string]any{}
	for key, value := range saved {
		next[key] = value
	}
	if fresh && previous != nil {
		if err := files.Archive(previous); err != nil {
			return 1, err
		}
	}
	nonce := NewUUID()
	next["role"], next["provider"], next["cwd"], next["nonce"] = role.Name, role.Provider, cwd, nonce
	next["status"], next["captured_at"], next["owner_pid"] = "starting", nil, os.Getpid()
	if bound != "" {
		next["session_id"] = bound
	}
	if err := state.WriteJSON(files.State(), next); err != nil {
		return 1, err
	}
	env := childEnv(p, role, nonce)
	model := role.Model
	if model == "" {
		model = "provider default"
	}
	mode := "fresh"
	if savedID != "" {
		mode = "resume"
	}
	fmt.Fprintf(out, "orai: %s/%s / %s / %s\n세션 ID 수집에는 CLI SessionStart hook 신뢰가 필요합니다. Codex에서 건너뛰면 /hooks로 확인하세요.\n",
		p.Name(), role.Name, model, mode)
	cmd := exec.Command(binary, args[1:]...)
	cmd.Dir, cmd.Env = cwd, env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.ExtraFiles = []*os.File{lock} // the lock lives as long as the provider does
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	if err := cmd.Start(); err != nil {
		// Nothing started: keep the previous resumable identity instead of a stale "starting".
		if previous == nil {
			_ = os.Remove(files.State())
		} else {
			_ = state.WriteJSON(files.State(), previous)
		}
		return 1, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	notified := make(chan struct{})
	go func() {
		defer close(notified)
		if role.Provider != "codex" {
			return
		}
		notifier := &providers.CodexNotifier{Files: files, Nonce: nonce,
			Pending: func() ([]string, error) { return root.Pending(role.Name) }, Queue: providers.CodexQueue(env)}
		if err := root.Watch(ctx, role.Name, 2*time.Second, notifier.Step); err != nil {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					notifier.Step()
				}
			}
		}
	}()
	go func() {
		for sig := range signals {
			if sig != syscall.SIGINT { // the terminal already delivers SIGINT to the child
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
		}
	}()
	waitErr := cmd.Wait()
	cancel()
	select {
	case <-notified:
	case <-time.After(22 * time.Second):
	}
	current, _ := state.ReadJSON(files.State())
	if current == nil {
		current = next
	}
	current["status"], current["stopped_at"] = "stopped", float64(time.Now().UnixNano())/1e9
	_ = state.WriteJSON(files.State(), current)
	if exit, ok := waitErr.(*exec.ExitError); ok {
		return exit.ExitCode(), nil
	}
	if waitErr != nil {
		return 1, waitErr
	}
	return 0, nil
}

type RoleStatus struct {
	Role     string `json:"role"`
	Provider string `json:"provider"`
	// The settings in effect, and which of them orai.local.toml set ("." = the whole role).
	Worktree      string   `json:"worktree"`
	Model         string   `json:"model,omitempty"`
	Effort        string   `json:"effort,omitempty"`
	Local         []string `json:"local,omitempty"`
	Running       bool     `json:"running"`
	SessionID     any      `json:"session_id"`
	DeliveryReady bool     `json:"delivery_ready"`
	LastError     any      `json:"last_error"`
	Pending       *int     `json:"pending,omitempty"`
	MailError     string   `json:"mail_error,omitempty"`
}

func Status(p *project.Project, out io.Writer) error {
	root := mail.Root(p.MailRoot())
	result := []RoleStatus{}
	for _, name := range p.Config.RoleNames() {
		role := p.Config.Roles[name]
		files := p.RoleFiles(name)
		current, _ := state.ReadJSON(files.State())
		delivery, _ := state.ReadJSON(files.Delivery(role.Provider))
		active := state.Locked(files)
		item := RoleStatus{Role: name, Provider: role.Provider, Running: active, SessionID: current["session_id"],
			Worktree: role.Worktree, Model: role.Model, Effort: role.Effort, Local: p.Config.LocalUnder("roles." + name)}
		item.DeliveryReady = active && current["captured_at"] != nil && delivery["nonce"] == current["nonce"] &&
			delivery["ready"] == true
		if active {
			item.LastError = delivery["last_error"]
		}
		if root.Initialized() {
			if ids, err := root.Pending(name); err != nil {
				item.MailError = err.Error()
			} else {
				count := len(ids)
				item.Pending = &count
			}
		} else {
			zero := 0
			item.Pending = &zero
		}
		result = append(result, item)
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	_, err := fmt.Fprintln(out, string(data))
	return err
}
