package runtime

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oXpace/orai/internal/config"
	"github.com/oXpace/orai/internal/project"
	"github.com/oXpace/orai/internal/state"
)

// What a person reads: a short summary when a run starts and ends, and a table for the
// state of one role. A provider's screen covers the summary while it runs, so anything
// to act on is said when it ends.

const (
	styleBold    = "1"
	styleDim     = "2"
	styleWarning = "33"
)

var providerTitles = map[string]string{"codex": "Codex", "claude": "Claude"}

var terminal = func(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

type view struct {
	out          io.Writer
	color, quiet bool
}

func newView(out io.Writer, quiet bool) view {
	color := os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && terminal(out)
	return view{out: out, color: color, quiet: quiet}
}

func (v view) paint(style, text string) string {
	if !v.color {
		return text
	}
	return "\x1b[" + style + "m" + text + "\x1b[0m"
}

func title(p *project.Project, role config.Role) string { return p.Name() + " › " + role.Name }

func engine(role config.Role) string {
	model := role.Model
	if model == "" {
		model = "기본 모델"
	}
	parts := []string{providerTitles[role.Provider], model}
	if role.Effort != "" {
		parts = append(parts, role.Effort)
	}
	return strings.Join(parts, " · ")
}

func folder(role config.Role) string {
	if role.Worktree == "." {
		return "프로젝트 폴더"
	}
	return role.Worktree
}

// unread never turns a failed read into "no messages".
func unread(pending *int) string {
	if pending == nil {
		return "새 메시지 확인 실패"
	}
	return fmt.Sprintf("새 메시지 %d", *pending)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func (v view) started(p *project.Project, role config.Role, sessionID string, pending *int, added []string) {
	if v.quiet {
		return
	}
	talk := "새 대화"
	if sessionID != "" {
		talk = "이어서 " + shortID(sessionID)
	}
	fmt.Fprintf(v.out, "● %s  %s\n", v.paint(styleBold, title(p, role)), v.paint(styleDim, talk))
	fmt.Fprintf(v.out, "  %s\n", v.paint(styleDim, engine(role)))
	fmt.Fprintf(v.out, "  %s\n", v.paint(styleDim, folder(role)+" · "+unread(pending)))
	if len(added) > 0 {
		fmt.Fprintf(v.out, "  %s\n", v.paint(styleDim, "메일함 추가: "+strings.Join(added, ", ")))
	}
}

// nextRun is what `orai <role>` would do now.
type nextRun int

const (
	runStarts     nextRun = iota // nothing saved: a new conversation
	runResumes                   // the saved conversation
	runNeedsFresh                // refused until --fresh
	runBlocked                   // refused, and --fresh would not help
)

// judgeNextRun applies the checks Launch applies, in the same order.
func judgeNextRun(p *project.Project, role config.Role) nextRun {
	cwd, err := RoleWorktree(p, role)
	if err != nil || CheckWorktree(role, cwd) != nil {
		return runBlocked
	}
	saved, err := state.ReadJSON(p.RoleFiles(role.Name).State())
	switch {
	case err != nil:
		return runBlocked
	case saved == nil:
		return runStarts
	case ValidateSaved(saved, role, cwd) != nil:
		return runNeedsFresh
	}
	return runResumes
}

func freshAdvice(role config.Role) string {
	return "이어갈 수 없음 → orai " + role.Name + " --fresh"
}

const blockedAdvice = "다시 실행할 수 없음 → orai doctor"

// stopped reports the end of a run. Whether the conversation resumes and whether this
// run was captured are separate questions: right after /clear the identity can be
// captured while its transcript does not exist yet.
func (v view) stopped(p *project.Project, role config.Role, next nextRun, captured bool, exitCode int) {
	// Only a Codex that ended normally without capture points at hook trust. After a
	// failed exit the cause is unknown, and Claude's channel delivers without capture.
	hookSkipped := role.Provider == "codex" && !captured && exitCode == 0
	var resume, warning string
	switch next {
	case runResumes:
		resume = " · 이어가기: orai " + role.Name
		if hookSkipped {
			warning = "이번엔 알림 꺼짐 → 다음 실행에서 hook 승인"
		}
	case runNeedsFresh:
		warning = freshAdvice(role)
		if hookSkipped {
			warning += " 후 hook 승인"
		}
	default: // blocked, or the state this run wrote could not be saved
		warning = blockedAdvice
	}
	if !v.quiet {
		fmt.Fprintf(v.out, "○ %s 종료%s\n", v.paint(styleBold, title(p, role)), v.paint(styleDim, resume))
	}
	if warning != "" {
		fmt.Fprintln(v.out, v.paint(styleWarning, "! "+warning))
	}
}

// StatusRole prints one role as a table, from the same values as the status JSON.
func StatusRole(p *project.Project, name string, out io.Writer) error {
	role, err := findRole(p, name)
	if err != nil {
		return err
	}
	item := roleStatus(p, role)

	run, notify := "실행 중", "-"
	if item.Running {
		switch {
		case item.LastError != nil && item.deliveryOfThisRun:
			reason, _, _ := strings.Cut(fmt.Sprint(item.LastError), "\n")
			notify = "오류: " + reason
		case item.DeliveryReady:
			notify = "준비됨"
		default:
			notify = "준비 안 됨"
		}
	} else {
		switch judgeNextRun(p, role) {
		case runStarts:
			run = "멈춤 · 시작: orai " + name
		case runResumes:
			run = "멈춤 · 이어가기: orai " + name
		case runNeedsFresh:
			run = "멈춤 · " + freshAdvice(role)
		default:
			run = "멈춤 · " + blockedAdvice
		}
	}
	talk := "-"
	if id, _ := item.SessionID.(string); id != "" {
		talk = shortID(id)
	}

	v := newView(out, false)
	if _, err := fmt.Fprintln(out, v.paint(styleBold, title(p, role))); err != nil {
		return err
	}
	// The labels are all two Hangul syllables, so plain spaces line the values up.
	for _, row := range [][2]string{{"상태", run}, {"실행", engine(role)}, {"대화", talk},
		{"폴더", folder(role)}, {"알림", notify}, {"메일", unread(item.Pending)}} {
		if _, err := fmt.Fprintf(out, "  %s    %s\n", v.paint(styleDim, row[0]), row[1]); err != nil {
			return err
		}
	}
	return nil
}
