"""Target project identity.

Three different places are never conflated:
  * the Orai installation (wherever this package is installed),
  * the project root: the main checkout that owns orai.toml and the local state,
  * a role worktree: the directory a role's CLI works in.
"""

from dataclasses import dataclass
import hashlib
from pathlib import Path
import re
import subprocess

from orai import config as config_module
from orai.state import read_json

CONFIG_NAME = "orai.toml"
STATE_NAME = ".orai"


class ProjectNotFound(RuntimeError):
    pass


def slug(text):
    value = re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")
    return value[:24].strip("-") or "project"


def main_checkout(path):
    """Return the main worktree of the Git repository at path, or None outside Git."""
    try:
        output = subprocess.run(
            ["git", "-C", str(path), "worktree", "list", "--porcelain"], capture_output=True, text=True, timeout=10
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return None
    if output.returncode or not output.stdout.startswith("worktree "):
        return None
    first = output.stdout.splitlines()[0].removeprefix("worktree ")
    block = output.stdout.split("\n\n", 1)[0]
    if "\nbare" in block:
        return None
    return Path(first).resolve()


def worktree_root(path):
    try:
        output = subprocess.run(
            ["git", "-C", str(path), "rev-parse", "--show-toplevel"], capture_output=True, text=True, timeout=10
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return None
    return Path(output.stdout.strip()).resolve() if not output.returncode else None


def find_config_dir(start):
    for directory in (start, *start.parents):
        if (directory / CONFIG_NAME).is_file():
            return directory
    return None


@dataclass(frozen=True)
class Project:
    root: Path
    config: config_module.Config

    @property
    def config_path(self):
        return self.root / CONFIG_NAME

    @property
    def state_dir(self):
        return self.root / STATE_NAME

    @property
    def roles_dir(self):
        return self.state_dir / "roles"

    @property
    def name(self):
        return self.config.name or self.root.name

    @property
    def id(self):
        # Path-derived: a copied or moved checkout is a different project and cannot
        # collide with the original's mail, ports or QMD daemon.
        digest = hashlib.sha256(str(self.root).encode()).hexdigest()[:10]
        return f"{slug(self.name)}-{digest}"

    def recorded_root(self):
        """Root recorded in local state, when it differs from the current one (moved/copied)."""
        recorded = (read_json(self.state_dir / "project.json") or {}).get("root")
        return recorded if recorded and recorded != str(self.root) else None


def locate(path):
    """Resolve the project root for a path without loading its config."""
    start = Path(path).expanduser().resolve()
    if not start.is_dir():
        raise ProjectNotFound(f"Not a directory: {start}")
    found = find_config_dir(start)
    if not found:
        raise ProjectNotFound(f"No {CONFIG_NAME} at or above {start}. Pass --project <path> or run `orai setup` there.")
    main = main_checkout(found)
    top = worktree_root(found)
    # A linked role worktree carries its own copy of orai.toml; the same path inside the main
    # checkout owns state. Directories of the main checkout itself (monorepo subprojects) and
    # worktrees whose main checkout lacks the config stay as found.
    if main and top and top != main:
        candidate = main / found.relative_to(top)
        if (candidate / CONFIG_NAME).is_file():
            return candidate
    return found


def load(path):
    root = locate(path)
    return Project(root=root, config=config_module.load(root / CONFIG_NAME))


def setup_target(cwd):
    """Where `orai setup` works when no --project is given.

    An existing project in this repository wins (from any subdirectory or linked worktree);
    otherwise the repository's main checkout, or the current folder outside Git. A config found
    above the repository (or above a non-Git folder) is someone else's project and is ignored.
    """
    cwd = Path(cwd).resolve()
    repo = main_checkout(cwd)
    try:
        found = locate(cwd)
    except ProjectNotFound:
        found = None
    if found and ((repo and found.is_relative_to(repo)) or (not repo and found == cwd)):
        return found
    return repo or cwd
