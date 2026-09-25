"""Message actions (ported from Pockets scripts/tests/test_orai_messages.py, b91019e).

Isolated queues only; never touches role sessions or a real project's mail.
"""

import contextlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from support import write_project

from orai import cli, mail


def role_env(project, role, mail_root="/q"):
    return {
        "ORAI_ROLE": role,
        "ORAI_PROJECT": str(project.root),
        "ORAI_SESSION": "orai",
        "AM_ME": role,
        "AM_SESSION": "orai",
        "AM_ROOT": str(mail_root),
        "AM_BASE_ROOT": str(Path(mail_root).parent),
    }


class Terminal(io.StringIO):
    def isatty(self):
        return True

    def read(self, *args):
        raise AssertionError("interactive stdin must not be read")


class MessageTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.project = write_project(self.root / "project")

    def test_identity_is_required_and_cannot_be_overridden(self):
        complete = role_env(self.project, "lead")
        for env, actor in [
            ({}, None),
            ({"AM_ME": "lead"}, None),
            ({"ORAI_ROLE": "lead"}, None),
            (complete, "user"),
            ({**complete, "AM_ME": "dev"}, None),
            ({**complete, "AM_SESSION": "other"}, None),
            ({**complete, "ORAI_ROLE": "Bad Role"}, None),
        ]:
            with (
                self.subTest(env=env, actor=actor),
                patch.dict(os.environ, env, clear=True),
                self.assertRaises(RuntimeError),
            ):
                mail.message_env(actor, lambda: self.project)

    def test_desktop_resolves_project_queue_not_caller_pin(self):
        with (
            patch.dict(os.environ, {"AM_ME": "senior", "AM_ROOT": "/wrong"}, clear=True),
            patch.object(mail, "mail_env", return_value={"AM_ME": "user"}) as resolve,
        ):
            self.assertEqual(mail.message_env("user", lambda: self.project), {"AM_ME": "user"})
        resolve.assert_called_once_with("user", self.project.root, "orai")

    def test_desktop_from_role_worktree_uses_main_checkout(self):
        git = ["git", "-c", "user.name=t", "-c", "user.email=t@t"]
        main = self.root / "repo"
        project = write_project(main, git=True)
        subprocess.run([*git, "-C", str(main), "add", "orai.toml"], check=True)
        subprocess.run([*git, "-C", str(main), "commit", "-qm", "init"], check=True)
        worktree = main / ".worktrees/dev"
        subprocess.run([*git, "-C", str(main), "worktree", "add", "-q", "-b", "dev", str(worktree)], check=True)
        with (
            patch.dict(os.environ, {}, clear=True),
            patch.object(mail, "mail_env", return_value={}) as resolve,
            patch.object(mail, "run") as run,
            contextlib.chdir(worktree),
        ):
            run.return_value = subprocess.CompletedProcess([], 0, "[]", "")
            self.assertEqual(cli.main(["msg", "inbox", "--as", "user", "--peek"]), 0)
        resolve.assert_called_once_with("user", project.root, "orai")

    def test_interactive_stdin_without_body_is_rejected_before_reading(self):
        for argv in (["msg", "send", "dev"], ["msg", "reply", "message-id"]):
            with (
                self.subTest(argv=argv),
                patch.dict(os.environ, role_env(self.project, "lead"), clear=True),
                patch.object(sys, "stdin", Terminal()),
                patch.object(mail, "run") as run,
                contextlib.redirect_stderr(io.StringIO()) as err,
            ):
                self.assertEqual(cli.main(argv), 1)
                run.assert_not_called()
                self.assertIn("--body, --file, or piped stdin", err.getvalue())

    def test_unknown_recipient_is_a_usage_error_without_delivery(self):
        with (
            patch.dict(os.environ, role_env(self.project, "lead"), clear=True),
            patch.object(mail, "run") as run,
            contextlib.redirect_stderr(io.StringIO()) as err,
        ):
            self.assertEqual(cli.main(["msg", "send", "senior", "--body", "hello"]), 2)
        run.assert_not_called()
        self.assertIn("dev, lead, user", err.getvalue())

    def test_role_session_cannot_act_on_another_project(self):
        other = write_project(self.root / "other")
        with (
            patch.dict(os.environ, role_env(self.project, "lead"), clear=True),
            patch.object(mail, "run") as run,
            contextlib.redirect_stderr(io.StringIO()),
        ):
            self.assertEqual(cli.main(["msg", "inbox", "--project", str(other.root)]), 2)
        run.assert_not_called()

    @unittest.skipUnless(shutil.which("amq"), "AMQ CLI required for isolated integration")
    def test_send_peek_receive_reply_and_failure_with_real_amq(self):
        queue = self.root / "mail" / "orai"
        subprocess.run(
            ["amq", "init", "--root", str(queue), "--agents", "dev,lead,user"], check=True, capture_output=True
        )
        base = {k: v for k, v in os.environ.items() if not k.startswith(("AM_", "AMQ_", "ORAI_"))}

        def call(role, *argv, stdin=""):
            with (
                patch.dict(os.environ, {**base, **role_env(self.project, role, queue)}, clear=True),
                patch.object(sys, "stdin", io.StringIO(stdin)),
                contextlib.redirect_stdout(io.StringIO()) as out,
                contextlib.redirect_stderr(io.StringIO()) as err,
            ):
                code = cli.main(["msg", *argv])
            return code, out.getvalue(), err.getvalue()

        body = "@literal $(do-not-run) `literal`\n두 번째 줄"
        self.assertEqual(call("lead", "send", "dev", "--kind", "question", "--body", body)[0], 0)
        peek = call("dev", "inbox", "--peek")
        self.assertEqual(peek[0], 0, peek)
        self.assertEqual(len(json.loads(peek[1])), 1)
        received = call("dev", "inbox")
        self.assertEqual(received[0], 0, received)
        rows = json.loads(received[1])["drained"]
        self.assertEqual(rows[0]["body"].rstrip("\n"), body)
        msg_id = json.loads(peek[1])[0]["id"]
        self.assertIn(msg_id, received[1])
        self.assertEqual(call("dev", "reply", msg_id, stdin="answer\n근거")[0], 0)
        reply = call("lead", "inbox")
        self.assertEqual(reply[0], 0, reply)
        replies = json.loads(reply[1])["drained"]
        self.assertEqual(replies[0]["thread"], rows[0]["thread"])
        self.assertEqual(replies[0]["kind"], "answer")
        self.assertTrue(any(msg_id in p.read_text() for p in (queue / "agents/lead/inbox/cur").iterdir()))
        self.assertEqual(json.loads(call("dev", "inbox")[1])["count"], 0)
        self.assertEqual(call("lead", "send", "dev", "--body", " ")[0], 1)
        self.assertNotEqual(call("dev", "reply", "missing", "--body", "answer")[0], 0)
        text_file = self.root / "body.md"
        text_file.write_text("파일 본문\n근거", encoding="utf-8")
        self.assertEqual(call("lead", "send", "dev", "--file", str(text_file))[0], 0)
        self.assertIn("파일 본문", call("dev", "inbox")[1])
        self.assertEqual(call("lead", "send", "dev", "--body", "first")[0], 0)
        self.assertEqual(call("lead", "send", "dev", "--body", "second")[0], 0)
        pending = json.loads(call("dev", "inbox", "--peek")[1])
        target = pending[0]["id"]
        self.assertEqual(call("dev", "inbox", target, "--peek")[0], 1)
        self.assertEqual(call("dev", "inbox", target, "--limit", "1")[0], 1)
        self.assertEqual(len(json.loads(call("dev", "inbox", "--peek")[1])), 2)
        direct = call("dev", "inbox", target)
        self.assertEqual(direct[0], 0, direct)
        self.assertIn(target, direct[1])
        remaining = json.loads(call("dev", "inbox", "--peek")[1])
        self.assertEqual(len(remaining), 1)
        self.assertNotEqual(remaining[0]["id"], target)
        self.assertNotEqual(call("dev", "inbox", "missing")[0], 0)
        self.assertEqual(len(json.loads(call("dev", "inbox", "--peek")[1])), 1)


if __name__ == "__main__":
    unittest.main()
