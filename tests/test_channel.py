"""Exercise the real stdio bridge against a non-consuming fake AMQ binary.

Ported from Pockets scripts/tests/test_orai_channel.py (b91019e); the channel now runs as
`python -m orai.providers.claude_channel` from the installed package.
"""

import json
import os
from pathlib import Path
import queue
import subprocess
import sys
import tempfile
import threading
import time
import unittest

from orai.providers import claude_channel

ROLE = "reviewer-2"  # any configured role name, not a fixed roster


class ChannelTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.root = Path(self.temporary.name)
        self.messages = self.root / "messages.json"
        self.messages.write_text("[]")
        self.log = self.root / "calls.jsonl"
        fake = self.root / "amq"
        fake.write_text(
            "#!"
            + sys.executable
            + "\n"
            + """import json, os, pathlib, sys
root = pathlib.Path(os.environ['FAKE_AMQ_ROOT'])
with (root / 'calls.jsonl').open('a') as output:
    output.write(json.dumps(sys.argv[1:]) + '\\n')
if sys.argv[1:] != ['list', '--new', '--json']:
    sys.exit('consuming or unexpected command')
print((root / 'messages.json').read_text())
"""
        )
        fake.chmod(0o700)
        environment = dict(
            os.environ,
            PATH=str(self.root) + os.pathsep + os.environ["PATH"],
            FAKE_AMQ_ROOT=str(self.root),
            ORAI_RUN_NONCE="test-nonce",
            ORAI_ROLE=ROLE,
            ORAI_STATE_DIR=str(self.root / "state"),
            AM_ME=ROLE,
        )
        self.process = subprocess.Popen(
            [sys.executable, "-m", "orai.providers.claude_channel"],
            env=environment,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self.output = queue.Queue()
        self.reader = threading.Thread(target=self.read_output, daemon=True)
        self.reader.start()

    def read_output(self):
        for line in self.process.stdout:
            self.output.put(json.loads(line))

    def tearDown(self):
        if not self.process.stdin.closed:
            self.process.stdin.close()
        try:
            self.process.wait(timeout=12)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait()
        self.reader.join(timeout=2)
        self.process.stdout.close()
        self.process.stderr.close()
        self.temporary.cleanup()

    def send(self, value):
        self.process.stdin.write(json.dumps(value) + "\n")
        self.process.stdin.flush()

    def rpc(self, method, params=None, request_id=1):
        value = {"jsonrpc": "2.0", "id": request_id, "method": method}
        if params is not None:
            value["params"] = params
        self.send(value)
        result = self.output.get(timeout=3)
        self.assertEqual(result.get("id"), request_id)
        return result

    def initialize(self):
        result = self.rpc("initialize", {"protocolVersion": "2025-11-25"})
        self.assertEqual(result["result"]["capabilities"], {"experimental": {"claude/channel": {}}, "tools": {}})
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})

    def ready(self):
        self.rpc("tools/call", {"name": "channel_ready", "arguments": {}})

    def wait_state(self, **expected):
        deadline = time.monotonic() + 3
        while time.monotonic() < deadline:
            try:
                state = json.loads((self.root / f"state/{ROLE}.channel.json").read_text())
                if all(state.get(key) == value for key, value in expected.items()):
                    return state
            except FileNotFoundError:
                pass
            time.sleep(0.02)
        self.fail("channel state did not reach " + repr(expected))

    def test_ready_gate_dedup_and_eof(self):
        self.messages.write_text('[{"id":"mail-1"}]')
        self.initialize()
        self.assertEqual(self.rpc("tools/list")["result"]["tools"][0]["name"], "channel_ready")
        self.wait_state(ready=False)
        with self.assertRaises(queue.Empty):
            self.output.get(timeout=2.2)
        self.assertFalse(self.log.exists(), "Must not poll until role readiness")
        self.ready()
        state = self.wait_state(ready=True, nonce="test-nonce")
        self.assertEqual(state["pid"], self.process.pid)
        self.assertEqual((self.root / f"state/{ROLE}.channel.json").stat().st_mode & 0o777, 0o600)
        notification = self.output.get(timeout=3)
        self.assertEqual(notification["method"], "notifications/claude/channel")
        self.assertEqual(notification["params"]["content"], "오라이: 새 메시지\nIDs: mail-1")
        self.assertTrue(all(isinstance(v, str) for v in notification["params"]["meta"].values()))
        with self.assertRaises(queue.Empty):
            self.output.get(timeout=2.2)
        self.messages.write_text('[{"id":"mail-1"},{"id":"mail-2"}]')
        self.assertEqual(self.output.get(timeout=3)["params"]["meta"]["pending_count"], "2")
        self.process.stdin.close()
        self.assertEqual(self.process.wait(timeout=12), 0)
        self.wait_state(ready=False)
        calls = [json.loads(line) for line in self.log.read_text().splitlines()]
        self.assertGreaterEqual(len(calls), 3)
        self.assertTrue(all(call == ["list", "--new", "--json"] for call in calls))

    def test_malformed_rpc_and_errors_do_not_break_stream(self):
        self.process.stdin.write("{broken\n")
        self.process.stdin.flush()
        self.assertEqual(self.output.get(timeout=3)["error"]["code"], -32700)
        self.send([])
        self.assertEqual(self.output.get(timeout=3)["error"]["code"], -32600)
        self.assertEqual(self.rpc("tools/list")["error"]["code"], -32000)
        self.initialize()
        self.assertEqual(
            self.rpc("tools/call", {"name": "channel_ready", "arguments": {"bad": 1}})["error"]["code"], -32602
        )
        self.assertEqual(self.rpc("unknown")["error"]["code"], -32601)
        self.assertEqual(self.rpc("ping")["result"], {})
        self.wait_state(ready=False)

    def test_empty_and_invalid_mail_do_not_emit_notifications(self):
        self.initialize()
        self.ready()
        with self.assertRaises(queue.Empty):
            self.output.get(timeout=2.2)
        self.messages.write_text('{"wrong":"shape"}')
        state = self.wait_state(last_error="Unexpected amq list JSON; expected a list of message IDs")
        self.assertTrue(state["ready"])
        self.messages.write_text("[]")
        self.wait_state(last_error=None)
        with self.assertRaises(queue.Empty):
            self.output.get(timeout=0.2)

    def test_unchanged_mail_is_not_repeated_and_new_mail_is_announced(self):
        channel = claude_channel.Channel.__new__(claude_channel.Channel)
        channel.role = ROLE
        channel.ready = threading.Event()
        channel.ready.set()
        channel.last_error = None
        notifications = []
        pending = iter([frozenset({"one"}), frozenset({"one"}), frozenset({"one", "two"}), frozenset()])
        channel.pending_ids = lambda: next(pending)
        channel.emit = notifications.append

        class Stop:
            count = 0

            def is_set(self):
                return self.count == 4

            def wait(self, seconds):
                self.count += 1

        channel.stop = Stop()
        channel.poll()
        self.assertEqual(len(notifications), 2)
        self.assertEqual(notifications[1]["params"]["content"], "오라이: 새 메시지\nIDs: two")

    def test_invalid_role_name_is_rejected_at_start(self):
        environment = dict(os.environ, ORAI_RUN_NONCE="n", ORAI_ROLE="Bad Role", ORAI_STATE_DIR=str(self.root / "x"))
        result = subprocess.run(
            [sys.executable, "-m", "orai.providers.claude_channel"],
            env=environment,
            input="",
            capture_output=True,
            text=True,
            timeout=10,
        )
        self.assertEqual(result.returncode, 1)
        self.assertIn("invalid ORAI_ROLE", result.stderr)


if __name__ == "__main__":
    unittest.main()
