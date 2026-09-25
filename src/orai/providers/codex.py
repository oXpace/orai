"""Codex: per-process hook/MCP overrides and notifications through `codex queue --thread`."""

import json
import subprocess
import time

from orai.process import checked
from orai.providers import notice, session_start_hook
from orai.state import read_json, write_json


def toml(value):
    if isinstance(value, dict):
        return "{" + ", ".join(json.dumps(k) + " = " + toml(v) for k, v in value.items()) + "}"
    if isinstance(value, list):
        return "[" + ", ".join(toml(v) for v in value) + "]"
    return json.dumps(value, ensure_ascii=False)


def arguments(role, saved, prompt, hook, root, mcp_servers):
    args = ["codex"] + (["resume", saved["session_id"]] if saved else [])
    if role.model:
        args += ["--model", role.model]
    if role.effort:
        args += ["-c", "model_reasoning_effort=" + json.dumps(role.effort)]
    args += ["-c", "hooks.SessionStart=" + toml(session_start_hook(hook))]
    for name, server in mcp_servers.items():
        args += ["-c", f"mcp_servers.{name}=" + toml(server)]
    # Project docs, queue and local runtime must be reachable from role worktrees.
    args += ["--add-dir", str(root), prompt]
    return args


def notify(files, nonce, env, stop, pending):
    """Queue new IDs into the captured thread; never drain mail on the role's behalf."""
    delivered = set()
    while not stop.wait(2):
        state = read_json(files.state, {})
        if state.get("nonce") != nonce or not state.get("captured_at"):
            continue
        status = {"nonce": nonce, "ready": True, "last_error": None}
        try:
            ids = set(pending())
            delivered.intersection_update(ids)
            if ids - delivered:
                checked(
                    ["codex", "queue", "--thread", state["session_id"], "--message", notice(ids - delivered)], env=env
                )
                delivered = ids
                status["last_enqueued_at"] = time.time()
        except (RuntimeError, subprocess.TimeoutExpired, ValueError, OSError) as exc:
            status.update(ready=False, last_error=str(exc))
        write_json(files.codex_delivery, status)
