"""Minimal Claude MCP channel: announce pending AMQ mail, never consume it.

Transport is newline-delimited JSON-RPC on stdio. The launcher owns identity,
session selection and process lifetime; this process owns only channel readiness.
"""

import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading

from orai import __version__
from orai.config import NAME
from orai.providers import notice
from orai.state import private_dirs

POLL_SECONDS = 2
PROTOCOL_VERSIONS = {"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25"}


class Channel:
    def __init__(self):
        self.stop = threading.Event()
        self.ready = threading.Event()
        self.output_lock = threading.Lock()
        self.state_lock = threading.Lock()
        self.initialized = False
        self.client_ready = False
        self.handshake = False
        self.last_error = None
        self.nonce = os.environ["ORAI_RUN_NONCE"]
        self.role = os.environ["ORAI_ROLE"]
        if not NAME.match(self.role):
            raise ValueError("invalid ORAI_ROLE")
        self.state_dir = Path(os.environ["ORAI_STATE_DIR"])
        private_dirs(self.state_dir)
        self.state_path = self.state_dir / (self.role + ".channel.json")
        self.write_state()

    def write_state(self):
        with self.state_lock:
            value = {
                "nonce": self.nonce,
                "pid": os.getpid(),
                "ready": self.ready.is_set(),
                "last_error": self.last_error,
            }
            fd, temporary = tempfile.mkstemp(prefix=".channel-", dir=str(self.state_dir))
            try:
                with os.fdopen(fd, "w") as stream:
                    json.dump(value, stream)
                    stream.write("\n")
                os.replace(temporary, self.state_path)
            finally:
                if os.path.exists(temporary):
                    os.unlink(temporary)

    def emit(self, value):
        with self.output_lock:
            try:
                sys.stdout.write(json.dumps(value, separators=(",", ":")) + "\n")
                sys.stdout.flush()
            except BrokenPipeError:
                self.stop.set()

    def result(self, request_id, value):
        self.emit({"jsonrpc": "2.0", "id": request_id, "result": value})

    def error(self, request_id, code, message):
        self.emit({"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}})

    def activate(self):
        if self.initialized and self.client_ready:
            self.ready.set()
            self.write_state()

    def dispatch(self, request):
        if (
            not isinstance(request, dict)
            or request.get("jsonrpc") != "2.0"
            or not isinstance(request.get("method"), str)
            or ("id" in request and (isinstance(request["id"], bool) or not isinstance(request["id"], (str, int))))
        ):
            self.error(None, -32600, "Invalid Request")
            return
        method = request["method"]
        request_id = request.get("id")
        if "id" not in request:
            if method == "notifications/initialized" and self.handshake:
                self.initialized = True
                self.activate()
            return
        params = request.get("params", {})
        if not isinstance(params, dict):
            self.error(request_id, -32602, "Invalid params")
            return
        if method == "initialize":
            version = params.get("protocolVersion")
            if not isinstance(version, str):
                self.error(request_id, -32602, "protocolVersion is required")
                return
            self.handshake = True
            self.result(
                request_id,
                {
                    "protocolVersion": version if version in PROTOCOL_VERSIONS else "2025-06-18",
                    "capabilities": {"experimental": {"claude/channel": {}}, "tools": {}},
                    "serverInfo": {"name": "orai", "version": __version__},
                    "instructions": "Call channel_ready when this channel connects. "
                    "Follow the orai skill when a mail notification arrives.",
                },
            )
        elif method == "ping":
            self.result(request_id, {})
        elif not self.initialized:
            self.error(request_id, -32000, "Initialize the channel first")
        elif method == "tools/list":
            self.result(
                request_id,
                {
                    "tools": [
                        {
                            "name": "channel_ready",
                            "description": "Enable AMQ notifications for this channel connection.",
                            "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
                        }
                    ]
                },
            )
        elif method == "tools/call":
            if params.get("name") != "channel_ready" or params.get("arguments", {}) != {}:
                self.error(request_id, -32602, "Expected channel_ready with no arguments")
                return
            self.result(request_id, {"content": [{"type": "text", "text": "Orai channel ready."}]})
            self.client_ready = True
            self.activate()
        else:
            self.error(request_id, -32601, "Method not found")

    def pending_ids(self):
        outcome = subprocess.run(["amq", "list", "--new", "--json"], capture_output=True, text=True, timeout=10)
        if outcome.returncode:
            raise RuntimeError("amq list failed: " + outcome.stderr.strip()[:500])
        rows = json.loads(outcome.stdout)
        if not isinstance(rows, list) or any(
            not isinstance(row, dict) or not isinstance(row.get("id"), str) or not row["id"] for row in rows
        ):
            raise ValueError("Unexpected amq list JSON; expected a list of message IDs")
        return frozenset(row["id"] for row in rows)

    def poll(self):
        announced = frozenset()
        while not self.stop.is_set():
            if self.ready.is_set():
                try:
                    pending = self.pending_ids()
                    if pending - announced:
                        self.emit(
                            {
                                "jsonrpc": "2.0",
                                "method": "notifications/claude/channel",
                                "params": {
                                    "content": notice(pending - announced),
                                    "meta": {"source": "orai", "role": self.role, "pending_count": str(len(pending))},
                                },
                            }
                        )
                    announced = pending
                    if self.last_error is not None:
                        self.last_error = None
                        self.write_state()
                except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
                    message = str(error)
                    if message != self.last_error:
                        self.last_error = message
                        self.write_state()
            self.stop.wait(POLL_SECONDS)

    def run(self):
        worker = threading.Thread(target=self.poll, daemon=True)
        worker.start()
        try:
            for line in sys.stdin:
                if self.stop.is_set():
                    break
                try:
                    request = json.loads(line)
                except ValueError:
                    self.error(None, -32700, "Parse error")
                    continue
                self.dispatch(request)
        finally:
            self.stop.set()
            worker.join(timeout=11)
            self.ready.clear()
            self.write_state()


if __name__ == "__main__":
    try:
        Channel().run()
    except (KeyError, OSError, ValueError) as error:
        print("orai channel: " + str(error), file=sys.stderr)
        sys.exit(1)
