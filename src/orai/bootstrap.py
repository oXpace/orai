"""`orai setup`: from an empty project folder to a working Orai project in one command.

Order: preflight every file change (scaffold.plan) → apply → bring up the project wiki →
index the code graph → read-only diagnosis. Tool steps are skipped, never forced, when the
tool is missing; each step reports what happened and the doctor summary says what is left.
"""

import shutil
import subprocess

from orai import project as project_module
from orai import scaffold
from orai.doctor import HEALTHY, NOT_CHECKED, NOT_CONFIGURED, overall
from orai.integrations import qmd


def wiki_step(project):
    if not project.config.wiki:
        return "wiki: not configured"
    if not shutil.which("qmd"):
        return "wiki: skipped (engine qmd is not installed; see docs/compatibility.md)"
    settings = qmd.Settings(project)
    if not any(any(folder.rglob("*.md")) for folder in settings.collections.values() if folder.is_dir()):
        return "wiki: skipped (no Markdown documents yet; add docs, then `orai wiki init`)"
    action = "recover" if settings.config_file.exists() and settings.db.exists() else "init"
    try:
        qmd.lifecycle(project, action)
    except (RuntimeError, OSError, ValueError, subprocess.SubprocessError) as exc:
        return f"wiki: {action} failed: {exc}"
    return f"wiki: ready ({action})"


def codegraph_step(project):
    if not project.config.codegraph:
        return "codegraph: not configured"
    binary = shutil.which("codegraph")
    if not binary:
        return "codegraph: skipped (not installed; see docs/compatibility.md)"
    if (project.root / ".codegraph").is_dir():
        return "codegraph: already indexed"
    result = subprocess.run([binary, "init", str(project.root)], capture_output=True, text=True, timeout=600)
    if result.returncode:
        return f"codegraph: init failed: {(result.stderr or result.stdout).strip()[-300:]}"
    return "codegraph: indexed"


def setup(root, preset, dry_run, branch, diagnose, tools=True):
    actions, notes = scaffold.plan(root, preset, branch)
    print(f"Project: {root}")
    if dry_run:
        for action in actions:
            print("Would: " + action.description)
        for note in notes:
            print("Note: " + note)
        print("Already current." if not actions else "Preview only. Run `orai setup` without --dry-run to apply.")
        return 0
    scaffold.apply(root, actions, done=lambda action: print("Applied: " + action.description, flush=True))
    if not actions:
        print("Files already current.")
    project = project_module.load(root)
    steps = [wiki_step(project), codegraph_step(project)] if tools else ["wiki, codegraph: skipped (--no-tools)"]
    for line in steps:
        print(line)
    for note in notes:
        print("Note: " + note)
    checks = diagnose(project)
    status, _ = overall(checks)
    print(f"\nDiagnosis: {status}")
    for check in checks:
        if check.status not in (HEALTHY, NOT_CONFIGURED, NOT_CHECKED):
            hint = f"  → {check.next_action}" if check.next_action else ""
            print(f"  {check.status:>9}  {check.component}: {check.reason}{hint}")
    failed = any("failed" in line for line in steps)
    return 1 if failed else 0
