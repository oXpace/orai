"""`orai setup` files: plan every change, refuse on any conflict, then apply with backups (each write atomic).

Existing AGENTS.md/.gitignore are never rewritten wholesale: only the marked Orai block
changes. Files Orai generates carry a marker; files without it belong to the user.
Tool steps (wiki index, code graph) and the final report live in bootstrap.py.
"""

from dataclasses import dataclass
from datetime import datetime
from importlib import resources
import json
import os
from pathlib import Path, PurePosixPath
import shutil
import subprocess
import tomllib

from orai import __version__
from orai import config as config_module
from orai.mail import clean_env
from orai.project import CONFIG_NAME, STATE_NAME, worktree_root
from orai.state import atomic_write, read_json

GENERATED = "<!-- orai:generated"
DEFAULT_BRANCH = "trunk"
# How a project installs Orai (mise pypi backend, GitHub source until a PyPI release exists).
INSTALL_SPEC = "pypi:oXpace/orai"
MISE_FILES = ("mise.toml", ".mise.toml", "mise.local.toml")
SKILL = ".agents/skills/orai/SKILL.md"
CLAUDE_SKILL_LINK = ".claude/skills/orai"
CLAUDE_SKILL_TARGET = "../../.agents/skills/orai"
PRESETS = {
    "minimal": {CONFIG_NAME: "minimal.toml"},
    "pm-staff": {CONFIG_NAME: "pm-staff.toml", ".agents/roles/pm.md": "pm.md", ".agents/roles/staff.md": "staff.md"},
}
MARKERS = {
    "md": ("<!-- orai:begin (managed by `orai setup`; edit outside this block) -->", "<!-- orai:end -->"),
    "hash": ("# orai:begin (managed by `orai setup`)", "# orai:end"),
}


class Conflict(ValueError):
    pass


def template(name):
    return (resources.files("orai") / "templates" / name).read_text(encoding="utf-8")


@dataclass
class Action:
    description: str
    path: Path
    data: bytes | None = None
    mode: int = 0o644
    backup: bool = False
    symlink: str | None = None
    command: list | None = None  # delegated to the tool that owns the file (AMQ)


def managed_block(existing, body, style):
    """Return the new file text with the Orai block inserted or replaced; None if malformed.

    The begin marker is matched by prefix so blocks written with older wording are replaced.
    """
    begin, end = MARKERS[style]
    prefix = begin.split(" (", 1)[0]
    block = f"{begin}\n{body.rstrip()}\n{end}\n"
    if existing is None:
        return block
    lines = existing.splitlines(keepends=True)
    starts = [i for i, line in enumerate(lines) if line.startswith(prefix)]
    ends = [i for i, line in enumerate(lines) if line.rstrip("\n") == end]
    if len(starts) != len(ends) or len(starts) > 1 or (starts and ends[0] < starts[0]):
        return None
    if not starts:
        separator = "" if not existing else ("\n" if existing.endswith("\n") else "\n\n")
        return existing + separator + block
    return "".join(lines[: starts[0]]) + block + "".join(lines[ends[0] + 1 :])


def has_block(existing, style):
    prefix = MARKERS[style][0].split(" (", 1)[0]
    return any(line.startswith(prefix) for line in existing.splitlines())


def parsed(config_text):
    try:
        return config_module.parse(tomllib.loads(config_text))
    except (tomllib.TOMLDecodeError, config_module.ConfigError):
        return None


def ignore_lines(cfg):
    """Local state and indexes, plus the AMQ root and nested role worktrees for projects with roles."""
    lines = ["/" + STATE_NAME + "/"]
    if cfg and cfg.codegraph:
        lines.append("/.codegraph/")
    if cfg and cfg.roles:
        lines += ["/.amqrc", "/.agent-mail/"]
        for role in cfg.roles.values():
            path = PurePosixPath(role.worktree)
            if role.worktree != "." and ".." not in path.parts:
                lines.append("/" + path.as_posix().strip("/") + "/")
    return sorted(set(lines), key=lines.index)


def amq_root_action(root, notes):
    """Roles need a project-local AMQ root (.amqrc). Without it AMQ may fall back to a global
    ~/.amqrc outside Git, and unrelated projects would share the same session mailboxes."""
    if (root / ".amqrc").exists():
        return None
    amq = shutil.which("amq")
    if not amq:
        notes.append("AMQ is not installed; install it and re-run `orai setup` to prepare the mail root")
        return None
    # Verified on AMQ 0.80.1: an existing .agent-mail config and pending mail are kept.
    return Action(
        "Initialize the project AMQ root .agent-mail (amq coop init)",
        root / ".amqrc",
        command=[amq, "coop", "init", "--root", ".agent-mail", "--agents", "user", "--no-gitignore"],
    )


def git_action(root, branch):
    """A new project folder becomes a repository; a folder inside one (monorepo) is left alone."""
    if worktree_root(root):
        return None
    return Action(
        f"Initialize a Git repository on branch {branch}", root / ".git", command=["git", "init", "-q", "-b", branch]
    )


def mise_pin_note(root):
    """Orai is installed per project through mise; say so when this project does not pin it."""
    try:
        if tomllib.loads((root / "pyproject.toml").read_text()).get("project", {}).get("name") == "orai":
            return None  # the Orai source checkout runs its own code
    except (OSError, tomllib.TOMLDecodeError):
        pass
    for name in MISE_FILES:
        path = root / name
        if path.is_file():
            try:
                tools = tomllib.loads(path.read_text(encoding="utf-8")).get("tools", {})
            except tomllib.TOMLDecodeError:
                return f"{name} is not valid TOML; cannot check the Orai pin"
            if any(key.startswith("pypi:") and key.rstrip("/").endswith("orai") for key in tools):
                return None
    return f"This project does not pin Orai. Pin it with `mise use {INSTALL_SPEC}@<version>` (docs/operations.md)"


def plan(root, preset="minimal", branch=DEFAULT_BRANCH):
    """Return (actions, notes). Raises Conflict listing every problem before any write."""
    root = Path(root).resolve()
    if preset not in PRESETS:
        raise Conflict(f"Unknown preset {preset!r}; choose from {', '.join(PRESETS)}")
    actions, notes, conflicts = [], [], []
    initialize = git_action(root, branch)
    if initialize:
        actions.append(initialize)

    def regular(rel):
        path = root / rel
        if path.is_symlink():
            conflicts.append(f"{rel} is a symlink; refusing to write through it")
            return None
        if path.exists() and not path.is_file():
            conflicts.append(f"{rel} exists and is not a file")
            return None
        return path

    config_path = root / CONFIG_NAME
    if config_path.exists():
        config_text = config_path.read_text(encoding="utf-8")
        try:
            config_module.parse(tomllib.loads(config_text))
        except (tomllib.TOMLDecodeError, config_module.ConfigError) as error:
            conflicts.append(f"{CONFIG_NAME} is invalid: {error}")
        if preset != "minimal":
            notes.append(f"{CONFIG_NAME} exists; preset {preset!r} only adds missing role guides")
    else:
        config_text = template(PRESETS[preset][CONFIG_NAME])

    for rel, name in PRESETS[preset].items():
        path = regular(rel)
        if path and not path.exists():
            actions.append(
                Action(f"Create {rel}", path, (config_text if rel == CONFIG_NAME else template(name)).encode())
            )

    skill = regular(SKILL)
    wanted = template("skill.md")
    if skill and skill.exists():
        current = skill.read_text(encoding="utf-8")
        if current != wanted:
            if GENERATED in current:
                actions.append(Action(f"Update generated {SKILL}", skill, wanted.encode(), backup=True))
            else:
                conflicts.append(f"{SKILL} exists and was not generated by Orai")
    elif skill:
        actions.append(Action(f"Create {SKILL}", skill, wanted.encode()))

    link = root / CLAUDE_SKILL_LINK
    if link.is_symlink():
        if os.readlink(link) != CLAUDE_SKILL_TARGET:
            conflicts.append(f"{CLAUDE_SKILL_LINK} points to {os.readlink(link)}, not {CLAUDE_SKILL_TARGET}")
    elif link.exists():
        conflicts.append(f"{CLAUDE_SKILL_LINK} exists and is not the Orai skill link")
    else:
        actions.append(
            Action(
                f"Link {CLAUDE_SKILL_LINK} -> {CLAUDE_SKILL_TARGET} for Claude Code", link, symlink=CLAUDE_SKILL_TARGET
            )
        )

    cfg = parsed(config_text)
    if cfg and cfg.wiki:
        # The wiki needs something to index; a missing folder gets a starter page.
        for folder in sorted(set(cfg.wiki.collections.values())):
            if not (root / folder).exists():
                page = regular(f"{folder}/README.md")
                if page:
                    actions.append(
                        Action(
                            f"Create {folder}/README.md (wiki starter page)", page, template("docs-readme.md").encode()
                        )
                    )
    if cfg and cfg.roles:
        action = amq_root_action(root, notes)
        if action:
            actions.append(action)

    for rel, body, style in (
        ("AGENTS.md", template("agents-block.md"), "md"),
        (".gitignore", "\n".join(ignore_lines(cfg)), "hash"),
    ):
        path = regular(rel)
        if not path:
            continue
        existing = path.read_text(encoding="utf-8") if path.exists() else None
        updated = managed_block(existing, body, style)
        if updated is None:
            conflicts.append(f"{rel} has an unbalanced or duplicated orai block; fix it by hand")
        elif updated != existing:
            if existing is None:
                verb = "Create"
            elif has_block(existing, style):
                verb = "Update the Orai block in"
            else:
                verb = "Add the Orai block to"
            actions.append(Action(f"{verb} {rel}", path, updated.encode(), backup=existing is not None))

    claude = regular("CLAUDE.md")
    if claude and not claude.exists():
        actions.append(Action("Create CLAUDE.md importing AGENTS.md", claude, b"@AGENTS.md\n"))
    elif claude and "AGENTS.md" not in claude.read_text(encoding="utf-8"):
        notes.append("CLAUDE.md does not reference AGENTS.md; add `@AGENTS.md` so Claude sees the Orai block")

    local = root / STATE_NAME / "project.json"
    recorded = read_json(local) if local.exists() else None
    if not recorded or recorded.get("root") != str(root):
        if recorded:
            notes.append(f"Local state was recorded for {recorded.get('root')}; saved sessions will need --fresh")
        value = {
            "root": str(root),
            "initialized_by": __version__,
            "initialized_at": datetime.now().astimezone().isoformat(timespec="seconds"),
        }
        actions.append(
            Action(
                ("Re-bind" if recorded else "Create") + f" local state {STATE_NAME}/project.json",
                local,
                (json.dumps(value, indent=2) + "\n").encode(),
                mode=0o600,
            )
        )

    pin = mise_pin_note(root)
    if pin:
        notes.append(pin)
    if conflicts:
        raise Conflict("\n".join(conflicts))
    return actions, notes


def apply(root, actions, done=lambda action: None):
    """Apply in order; each write is atomic, the whole plan is not. `done` reports each success."""
    stamp = datetime.now().strftime("%Y%m%dT%H%M%S")
    backups = Path(root) / STATE_NAME / "backups" / stamp
    for action in actions:
        if action.backup and action.path.exists():
            target = backups / action.path.relative_to(root)
            atomic_write(target, action.path.read_bytes())
        if STATE_NAME not in action.path.relative_to(root).parts:
            # Shared project folders get normal permissions; atomic_write keeps .orai private.
            action.path.parent.mkdir(parents=True, exist_ok=True)
        if action.command:
            result = subprocess.run(
                action.command, cwd=root, env=clean_env(), capture_output=True, text=True, timeout=60
            )
            if result.returncode:
                raise RuntimeError(f"{' '.join(action.command)} failed: {result.stderr.strip()}")
        elif action.symlink:
            action.path.symlink_to(action.symlink)
        else:
            mode = action.mode
            if action.path.exists():
                mode = action.path.stat().st_mode & 0o777
            atomic_write(action.path, action.data, mode)
        done(action)
