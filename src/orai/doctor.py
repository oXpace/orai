"""Read-only, per-component diagnosis with explicit states and safe next actions."""

from dataclasses import asdict, dataclass
from datetime import UTC, datetime
import json
import re
import shutil
import subprocess

HEALTHY = "healthy"
DEGRADED = "degraded"
NOT_READY = "not-ready"
BLOCKED = "blocked"
NOT_CONFIGURED = "not-configured"
UNSUPPORTED = "unsupported"
# Deliberately not exercised in this mode (e.g. model-loading search without --deep).
# Visible in the report, never counted as verified, and not a problem either.
NOT_CHECKED = "not-checked"

EXIT_OK, EXIT_DEGRADED, EXIT_BLOCKED = 0, 1, 3


def now():
    return datetime.now(UTC).isoformat(timespec="seconds")


@dataclass
class Check:
    component: str
    status: str
    reason: str
    next_action: str | None = None
    core: bool = False
    detail: dict | None = None
    checked_at: str = ""

    def __post_init__(self):
        self.checked_at = self.checked_at or now()


def overall(checks):
    if any(c.core and c.status in (BLOCKED, UNSUPPORTED) for c in checks):
        return BLOCKED, EXIT_BLOCKED
    if any(c.core and c.status == NOT_CONFIGURED for c in checks):
        return NOT_CONFIGURED, EXIT_DEGRADED  # nothing project-specific was verified
    if any(c.status in (BLOCKED, DEGRADED, NOT_READY, UNSUPPORTED) for c in checks):
        return DEGRADED, EXIT_DEGRADED
    return HEALTHY, EXIT_OK


def report(checks, **context):
    status, code = overall(checks)
    print(
        json.dumps(
            {
                "status": status,
                "checked_at": now(),
                **context,
                "checks": [{k: v for k, v in asdict(c).items() if v is not None} for c in checks],
            },
            ensure_ascii=False,
            indent=2,
        )
    )
    return code


def help_text(argv):
    try:
        result = subprocess.run(argv, capture_output=True, text=True, timeout=15)
    except (OSError, subprocess.TimeoutExpired) as exc:
        return None, str(exc)
    return (result.stdout + result.stderr), None


def version_of(binary, argv=None):
    text, _ = help_text(argv or [binary, "--version"])
    match = re.search(r"\d+\.\d+\.\d+", text or "")
    return match.group(0) if match else None


def tool(component, binary, core, requirements, install_hint, note=None):
    """Presence, version and the flags Orai relies on, from the installed binary's own help."""
    path = shutil.which(binary)
    if not path:
        return Check(component, BLOCKED, f"{binary} is not on PATH", install_hint, core)
    detail = {"path": path, "version": version_of(binary)}
    missing = []
    for argv, needles in requirements:
        text, error = help_text(argv)
        if error:
            return Check(component, BLOCKED, f"`{' '.join(argv)}` failed: {error}", None, core, detail)
        missing += [f"{' '.join(argv[1:-1]) or binary} {needle}" for needle in needles if needle not in text]
    if missing:
        return Check(
            component,
            UNSUPPORTED,
            "installed version lacks: " + ", ".join(missing),
            "Install a version listed in docs/compatibility.md",
            core,
            detail,
        )
    if note:
        detail["note"] = note
    return Check(component, HEALTHY, "required capabilities present", None, core, detail)


def account(component, argv, signed_in, hint):
    """Login is diagnosed separately from installation: a working binary may still be signed out."""
    if not shutil.which(argv[0]):
        return Check(component, NOT_READY, f"{argv[0]} is not installed; login not checked", None, True)
    text, error = help_text(argv)
    if error:
        return Check(component, BLOCKED, f"`{' '.join(argv)}` failed: {error}", hint, True)
    if signed_in(text):
        return Check(component, HEALTHY, "signed in", core=True)
    return Check(component, BLOCKED, "not signed in", hint, True)


def claude_signed_in(text):
    try:
        return json.loads(text).get("loggedIn") is True
    except ValueError:
        return False


def tool_checks(providers):
    checks = [
        tool(
            "tool.amq",
            "amq",
            True,
            [
                (["amq", "coop", "exec", "--help"], ["-no-wake", "-no-init", "-named", "-root"]),
                (["amq", "drain", "--help"], ["-include-body", "-limit"]),
            ],
            "brew install avivsinai/tap/amq",
        )
    ]
    if "codex" in providers:
        checks.append(
            tool(
                "tool.codex",
                "codex",
                True,
                [
                    (["codex", "queue", "--help"], ["--thread", "--message"]),
                    (["codex", "resume", "--help"], ["SESSION_ID"]),
                    (["codex", "--help"], ["--add-dir", "--config"]),
                ],
                "Install Codex CLI (docs/compatibility.md)",
                "SessionStart hook trust is confirmed by Codex at launch (/hooks).",
            )
        )
        checks.append(account("account.codex", ["codex", "login", "status"], lambda t: "Logged in" in t, "codex login"))
    if "claude" in providers:
        checks.append(
            tool(
                "tool.claude",
                "claude",
                True,
                [
                    (
                        ["claude", "--help"],
                        ["--session-id", "--resume", "--settings", "--mcp-config", "--name", "--effort", "--add-dir"],
                    )
                ],
                "Install Claude Code (docs/compatibility.md)",
                "Development channel consent is hidden from --help; Claude asks at launch.",
            )
        )
        checks.append(account("account.claude", ["claude", "auth", "status"], claude_signed_in, "claude auth login"))
    return checks
