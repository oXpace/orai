"""Role session lifecycle: launch, exact resume, identity capture, notification and status.

AMQ owns mail, providers own conversations. Orai only records which exact conversation
belongs to which role and keeps one live process per role.
"""

import json
import os
from pathlib import Path
import shlex
import signal
import subprocess
import sys
import threading
import time
import uuid

from orai import mail
from orai.doctor import BLOCKED, DEGRADED, HEALTHY, NOT_CONFIGURED, NOT_READY, Check
from orai.integrations import mcp_servers
from orai.process import checked
from orai.providers import claude, codex
from orai.state import RoleFiles, locked, read_json, role_lock, write_json


def files(project, role):
    return RoleFiles(project.roles_dir, role)


def skill_path(project):
    return project.root / ".agents/skills/orai/SKILL.md"


def kickoff(project, role, resumed=False):
    root = project.root
    guide = f", 역할 지침은 {root / role.guide}" if role.guide else ""
    reading = (
        "기존 대화의 역할·작업 요약을 이어받고 지침 변경 또는 누락된 맥락에 해당하는 원문만 확인하라. "
        if resumed
        else "AGENTS.md와 역할 지침을 읽고 작업에 필요한 원천만 추가로 확인하라. "
    )
    return (
        f"당신은 {project.name} 프로젝트의 {role.name} 역할이다. 지침 기준 checkout은 {root}{guide}이다. "
        "역할 worktree보다 이 기준 checkout의 지침을 우선한다. "
        + reading
        + f"메시지 작업은 {skill_path(project)}를 참고해 orai msg inbox로 시작하라. "
        "실행 대상이 없으면 전체 백로그를 조사하지 말고 턴을 종료하라. 새 메시지 알림은 오라이가 전달한다. "
        "보조가 본 세션 ID·AMQ 설정을 변경하지 않게 하라. "
        + ("orai MCP의 channel_ready를 호출해 수신 준비를 알리라. " if role.provider == "claude" else "")
    )


def recovery_context(project, role):
    guide = role.guide or "없음"
    return (
        f"오라이 역할={role.name}, 지침 기준={project.root}, 역할 지침={guide}. "
        "현재 작업·요약을 이어가고 문서 확인 범위는 AGENTS.md를 따른다."
    )


def hook_command(project):
    return shlex.join([sys.executable, "-m", "orai", "--project", str(project.root), "_capture"])


def provider_args(project, role, saved):
    """Return (argv, bound session id or None) for the role's provider CLI."""
    prompt = kickoff(project, role, resumed=bool(saved))
    servers = mcp_servers(project, role.provider)
    if role.provider == "codex":
        return codex.arguments(role, saved, prompt, hook_command(project), project.root, servers), None
    return claude.arguments(
        role, saved, prompt, hook_command(project), project.root, servers, f"{project.name}/{role.name}"
    )


def capture(project):
    """SessionStart hook: bind the provider's conversation ID to the current run only."""
    role_name = os.environ.get("ORAI_ROLE")
    nonce = os.environ.get("ORAI_RUN_NONCE")
    if not role_name or not nonce:
        return 0
    event = json.load(sys.stdin)
    if event.get("hook_event_name") != "SessionStart":
        return 0
    role = project.config.roles.get(role_name)
    if role is None:
        raise RuntimeError(f"Role {role_name!r} is not configured in {project.config_path}")
    sid = str(uuid.UUID(event["session_id"]))
    path = files(project, role_name).state
    state = read_json(path, {})
    if state.get("nonce") != nonce or state.get("status") not in ("starting", "running"):
        raise RuntimeError("Stale launch hook; refusing identity capture")
    if Path(event["cwd"]).resolve() != Path(state["cwd"]).resolve():
        raise RuntimeError("Session cwd does not match the role worktree")
    if state.get("session_id") and state["session_id"] != sid:
        raise RuntimeError("Unexpected conversation identity; use --fresh explicitly")
    transcript = event.get("transcript_path")
    state.update(
        session_id=sid, transcript=transcript or state.get("transcript"), status="running", captured_at=time.time()
    )
    write_json(path, state)
    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "SessionStart",
                    "additionalContext": recovery_context(project, role),
                }
            }
        )
    )
    return 0


def validate_saved(saved, role, cwd):
    if not saved:
        return
    if saved.get("provider") != role.provider or saved.get("cwd") != str(cwd):
        raise RuntimeError("Saved provider/worktree changed; inspect state and use --fresh")
    if not saved.get("session_id"):
        raise RuntimeError("Previous identity was not captured. Review CLI hook trust, then use --fresh")
    uuid.UUID(saved["session_id"])
    # Only read the exact transcript recorded by the provider hook; never scan for 'latest'.
    if not saved.get("transcript") or not Path(saved["transcript"]).is_file():
        raise RuntimeError("Saved conversation transcript is unavailable; restore it or use --fresh")


def role_worktree(project, role):
    cwd = (project.root / role.worktree).resolve()
    if not cwd.is_dir():
        raise RuntimeError(f"Missing role worktree: {cwd}")
    return cwd


def check_worktree(project, role, cwd):
    """Git projects: a separate role directory must be a worktree root, on its declared branch.

    worktree = "." is the project root itself, which may be a subdirectory of its repository.
    """
    try:
        top = subprocess.run(["git", "rev-parse", "--show-toplevel"], cwd=cwd, capture_output=True, text=True)
    except OSError as exc:
        raise RuntimeError(f"Cannot run git: {exc}") from None
    if top.returncode:
        if role.branch:
            raise RuntimeError(f"{role.name} declares branch {role.branch!r} but {cwd} is not a Git worktree")
        return
    if role.worktree != "." and Path(top.stdout.strip()).resolve() != cwd:
        raise RuntimeError("Role directory is not a Git worktree root")
    if role.branch and checked(["git", "branch", "--show-current"], cwd=cwd).strip() != role.branch:
        raise RuntimeError(f"{role.name} worktree must be on branch {role.branch}")


def missing_mailboxes(project, mail_root):
    return [h for h in project.config.handles() if not (mail_root / "agents" / h / "inbox/new").is_dir()]


def ensure_mailboxes(project, env):
    root = Path(env["AM_ROOT"])
    handles = project.config.handles()
    meta = root / "meta/config.json"
    if not meta.exists():
        checked(["amq", "init", "--root", env["AM_ROOT"], "--agents", ",".join(handles)], env=env, cwd=project.root)
    elif missing_mailboxes(project, root):
        # A role was added. Keep every handle AMQ already knows (retired roles included);
        # verified on AMQ 0.80.1 that --force rewrites meta/config.json and keeps pending mail.
        known = (read_json(meta) or {}).get("agents") or []
        union = sorted(set(known) | set(handles))
        print(f"orai: adding mailbox(es) {', '.join(missing_mailboxes(project, root))} to {root}", flush=True)
        checked(
            ["amq", "init", "--root", env["AM_ROOT"], "--agents", ",".join(union), "--force"], env=env, cwd=project.root
        )
    missing = missing_mailboxes(project, root)
    if missing:
        raise RuntimeError(f"Mailbox for {', '.join(missing)} is missing in {root}; inspect with amq doctor")


def launch(project, role_name, fresh=False, dry=False):
    if mail.role_identity():
        raise RuntimeError("Already inside an Orai role session; start roles from a normal terminal")
    role = project.config.roles.get(role_name)
    if role is None:
        known = ", ".join(sorted(project.config.roles)) or "none"
        raise LookupError(f"Unknown role {role_name!r} (configured: {known})")
    cwd = role_worktree(project, role)
    check_worktree(project, role, cwd)
    paths = files(project, role.name)
    saved = None if fresh else read_json(paths.state)
    validate_saved(saved, role, cwd)
    env = mail.mail_env(role.name, project.root, project.config.session)
    nonce = str(uuid.uuid4())
    args, bound = provider_args(project, role, saved)
    command = [
        "amq",
        "coop",
        "exec",
        "--no-wake",
        "--no-init",
        "--named=false",
        "--root",
        env["AM_ROOT"],
        "--me",
        role.name,
        args[0],
        "--",
    ] + args[1:]
    if dry:
        print(
            json.dumps(
                {
                    "project": str(project.root),
                    "role": role.name,
                    "cwd": str(cwd),
                    "resume": bool(saved),
                    "session_id": saved.get("session_id") if saved else None,
                    "mail_root": env["AM_ROOT"],
                    "command": command,
                },
                ensure_ascii=False,
                indent=2,
            )
        )
        return 0
    if not sys.stdin.isatty():
        raise RuntimeError("Run orai in an interactive terminal; --dry-run is non-interactive")
    lock = role_lock(paths)
    try:
        ensure_mailboxes(project, env)
        previous = read_json(paths.state)
        state = dict(saved or {})
        if fresh and previous:
            paths.archive(previous)
        state.update(
            role=role.name,
            provider=role.provider,
            cwd=str(cwd),
            nonce=nonce,
            status="starting",
            captured_at=None,
            owner_pid=os.getpid(),
        )
        if bound:
            state["session_id"] = bound
        write_json(paths.state, state)
        env.update(
            ORAI_ROLE=role.name,
            ORAI_RUN_NONCE=nonce,
            ORAI_STATE_DIR=str(paths.directory),
            ORAI_PROJECT=str(project.root),
            ORAI_SESSION=project.config.session,
        )
        print(
            f"orai: {project.name}/{role.name} / {role.model or 'provider default'} / "
            f"{'resume' if saved else 'fresh'}\n"
            "세션 ID 수집에는 CLI SessionStart hook 신뢰가 필요합니다. Codex에서 건너뛰면 /hooks로 확인하세요.",
            flush=True,
        )
        try:
            child = subprocess.Popen(command, cwd=cwd, env=env, pass_fds=(lock.fileno(),))
        except OSError:
            # Nothing started: keep the previous resumable identity instead of a stale "starting".
            if previous is None:
                paths.state.unlink(missing_ok=True)
            else:
                write_json(paths.state, previous)
            raise
        stop = threading.Event()
        worker = None
        if role.provider == "codex":
            worker = threading.Thread(
                target=codex.notify,
                daemon=True,
                args=(paths, nonce, env, stop, lambda: mail.pending(env, project.root)),
            )
            worker.start()
        old_int = signal.signal(signal.SIGINT, lambda *_: None)
        old_term = signal.signal(signal.SIGTERM, lambda *_: child.terminate())
        old_hup = signal.signal(signal.SIGHUP, lambda *_: child.terminate())
        try:
            result = child.wait()
        finally:
            stop.set()
            if worker:
                worker.join(timeout=22)
            signal.signal(signal.SIGINT, old_int)
            signal.signal(signal.SIGTERM, old_term)
            signal.signal(signal.SIGHUP, old_hup)
            current = read_json(paths.state, state)
            current.update(status="stopped", stopped_at=time.time())
            write_json(paths.state, current)
        return result
    finally:
        lock.close()


def status(project):
    result = []
    for name, role in project.config.roles.items():
        paths = files(project, name)
        state = read_json(paths.state, {})
        active = locked(paths)
        delivery = read_json(paths.delivery(role.provider), {})
        ready = (
            active
            and state.get("captured_at")
            and delivery.get("nonce") == state.get("nonce")
            and delivery.get("ready")
        )
        item = {
            "role": name,
            "provider": role.provider,
            "running": active,
            "session_id": state.get("session_id"),
            "delivery_ready": bool(ready),
            "last_error": delivery.get("last_error") if active else None,
        }
        try:
            env = mail.mail_env(name, project.root, project.config.session)
            item["pending"] = len(mail.pending(env, project.root)) if Path(env["AM_ROOT"]).exists() else 0
        except (RuntimeError, ValueError, OSError, subprocess.TimeoutExpired) as exc:
            item["mail_error"] = str(exc)
        result.append(item)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


def diagnose(project):
    """Read-only project checks: local state, mail and each role's resume readiness."""
    checks = []
    moved = project.recorded_root()
    if moved:
        checks.append(
            Check(
                "project",
                DEGRADED,
                f"local state was recorded for {moved} (moved or copied)",
                "Review .orai/, then `orai setup` to re-bind; saved sessions need --fresh",
                detail={"root": str(project.root), "id": project.id},
            )
        )
    else:
        checks.append(
            Check(
                "project",
                HEALTHY,
                f"{project.config_path} is valid",
                detail={"root": str(project.root), "id": project.id, "roles": sorted(project.config.roles)},
            )
        )
    if not project.config.roles:
        checks.append(Check("mail", NOT_CONFIGURED, "no roles in orai.toml; messaging is not used"))
        return checks
    try:
        env = mail.mail_env("user", project.root, project.config.session)
        mail_root = Path(env["AM_ROOT"])
        if not (mail_root / "meta/config.json").exists():
            checks.append(
                Check(
                    "mail",
                    NOT_READY,
                    f"no mailboxes yet at {mail_root}",
                    "Created by the first `orai run <role>`",
                    True,
                )
            )
        elif missing_mailboxes(project, mail_root):
            checks.append(
                Check(
                    "mail",
                    DEGRADED,
                    "missing mailbox(es): " + ", ".join(missing_mailboxes(project, mail_root)),
                    "The next `orai run <role>` adds them",
                    True,
                )
            )
        else:
            checks.append(Check("mail", HEALTHY, f"mailboxes present at {mail_root}", core=True))
    except (RuntimeError, ValueError, KeyError, OSError, subprocess.TimeoutExpired) as exc:
        checks.append(
            Check(
                "mail",
                BLOCKED,
                f"cannot resolve the AMQ root: {exc}",
                "`orai setup` prepares the project's AMQ root; otherwise see `amq doctor`",
                True,
            )
        )
    for name, role in project.config.roles.items():
        paths = files(project, name)
        detail = {"provider": role.provider, "running": locked(paths)}
        try:
            cwd = role_worktree(project, role)
            check_worktree(project, role, cwd)
            saved = read_json(paths.state)
            validate_saved(saved, role, cwd)
        except (RuntimeError, ValueError, OSError) as exc:
            checks.append(
                Check(
                    f"role.{name}",
                    DEGRADED,
                    str(exc),
                    f"Fix the worktree, or `orai run {name} --fresh` for a new conversation",
                    detail=detail,
                )
            )
            continue
        detail["session_id"] = saved.get("session_id") if saved else None
        checks.append(
            Check(
                f"role.{name}",
                HEALTHY,
                "exact resume ready" if saved else "no saved conversation; the next run starts fresh",
                detail=detail,
            )
        )
    return checks
