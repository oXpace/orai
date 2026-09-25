"""Claude Code: exact session binding, per-process settings and the local Orai channel."""

import json
import sys
import uuid

from orai.providers import session_start_hook

CHANNEL_SERVER = "orai"


def channel_server():
    # The installed interpreter, not a source checkout path, runs the channel.
    return {"command": sys.executable, "args": ["-m", "orai.providers.claude_channel"]}


def arguments(role, saved, prompt, hook, root, mcp_servers, display_name):
    session_id = saved["session_id"] if saved else str(uuid.uuid4())
    args = ["claude", "--resume" if saved else "--session-id", session_id]
    if role.model:
        args += ["--model", role.model]
    if role.effort:
        args += ["--effort", role.effort]
    args += ["--name", display_name]
    settings = {"hooks": {"SessionStart": session_start_hook(hook)}}
    servers = {CHANNEL_SERVER: channel_server(), **mcp_servers}
    args += [
        "--settings",
        json.dumps(settings),
        "--mcp-config",
        json.dumps({"mcpServers": servers}),
        "--dangerously-load-development-channels",
        "server:" + CHANNEL_SERVER,
        "--add-dir",
        str(root),
        "--",
        prompt,
    ]
    return args, session_id
