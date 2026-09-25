"""Shared fixtures: temporary projects and fake CLIs. Nothing here touches real sessions."""

import os
from pathlib import Path
import subprocess
import sys
import textwrap

from orai import project as project_module

TWO_ROLES = """
schema = 1
session = "orai"

[roles.lead]
provider = "codex"
worktree = "."
guide = "docs/lead.md"
model = "gpt-test"
effort = "medium"

[roles.dev]
provider = "claude"
worktree = "."
guide = "docs/dev.md"
model = "opus"
effort = "high"
"""


def write_project(root, text=TWO_ROLES, git=False):
    """Create a project at root and return the loaded Project."""
    root = Path(root)
    root.mkdir(parents=True, exist_ok=True)
    if git:
        subprocess.run(["git", "init", "-q", "-b", "trunk", str(root)], check=True)
    (root / "orai.toml").write_text(textwrap.dedent(text))
    return project_module.load(root)


def fake_binary(directory, name, body):
    """Write an executable Python script named `name`; `body` is the script after the shebang."""
    path = Path(directory) / name
    path.write_text("#!" + sys.executable + "\n" + textwrap.dedent(body))
    path.chmod(0o755)
    return path


def path_with(directory):
    return str(directory) + os.pathsep + os.environ.get("PATH", "")
