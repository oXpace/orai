"""Launcher contracts ported from Pockets scripts/tests/test_orai_runtime.py (b91019e).

Never starts a provider or consumes real mail.
"""

import contextlib
import io
import json
import os
from pathlib import Path
import shlex
import sys
import tempfile
import unittest
from unittest.mock import Mock, patch

from support import write_project

from orai import cli, mail, runtime
from orai.state import RoleFiles, locked, read_json, role_lock, write_json

SID = "ed936671-e8a5-4d6d-a38f-c5bcc152ab10"
OTHER_SID = "741e3b28-1050-49ce-93d9-847ecf8334c3"
BARE_ROLES = """
schema = 1
[roles.a]
provider = "codex"
[roles.b]
provider = "claude"
"""


class RuntimeTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.project = write_project(self.root)
        self.lead = self.project.config.roles["lead"]
        self.dev = self.project.config.roles["dev"]
        self.files = runtime.files(self.project, "lead")

    def state(self, **changes):
        state = {
            "provider": "codex",
            "cwd": str(self.root),
            "nonce": "run-one",
            "status": "starting",
            "session_id": SID,
        }
        state.update(changes)
        write_json(self.files.state, state)
        return state

    def event(self, **changes):
        event = {
            "hook_event_name": "SessionStart",
            "cwd": str(self.root),
            "session_id": SID,
            "transcript_path": str(self.root / "transcript.jsonl"),
        }
        event.update(changes)
        return event

    def capture(self, event, nonce="run-one", role="lead"):
        with (
            patch.dict(os.environ, {"ORAI_ROLE": role, "ORAI_RUN_NONCE": nonce}),
            patch.object(sys, "stdin", io.StringIO(json.dumps(event))),
            contextlib.redirect_stdout(io.StringIO()) as output,
        ):
            code = runtime.capture(self.project)
        return code, output.getvalue()

    def test_roles_come_from_config_and_unknown_role_fails_without_mail(self):
        self.assertEqual(set(self.project.config.roles), {"lead", "dev"})
        for argv in (["senior"], ["run", "senior"]):
            with (
                self.subTest(argv=argv),
                patch.object(mail, "mail_env") as resolve,
                contextlib.redirect_stderr(io.StringIO()) as err,
            ):
                self.assertEqual(cli.main(["--project", str(self.root), *argv]), 2)
                resolve.assert_not_called()
                self.assertIn("Unknown role 'senior'", err.getvalue())

    def test_role_alias_is_rewritten_to_run(self):
        self.assertEqual(cli.normalize(["lead", "--fresh"]), ["run", "lead", "--fresh"])
        self.assertEqual(cli.normalize(["--project", "/p", "lead"]), ["--project", "/p", "run", "lead"])
        self.assertEqual(cli.normalize(["status"]), ["status"])
        self.assertEqual(cli.normalize(["--version"]), ["--version"])

    def test_provider_resume_uses_exact_id_and_role_model(self):
        for role in (self.lead, self.dev):
            with self.subTest(role=role.name):
                args, _ = runtime.provider_args(self.project, role, {"session_id": SID})
                self.assertEqual(args[1:3], ["--resume" if role.provider == "claude" else "resume", SID])
                self.assertEqual(args[args.index("--model") + 1], role.model)
                self.assertNotIn("--last", args)
                self.assertIn("기존 대화", args[-1])

    def test_model_and_effort_are_optional(self):
        project = write_project(self.root / "bare", BARE_ROLES)
        for role in project.config.roles.values():
            args, _ = runtime.provider_args(project, role, None)
            self.assertNotIn("--model", args)
            self.assertNotIn("--effort", args)
            self.assertFalse(any(a.startswith("model_reasoning_effort") for a in args))

    def test_provider_passes_role_effort(self):
        for role in (self.lead, self.dev):
            with self.subTest(role=role.name):
                args, _ = runtime.provider_args(self.project, role, None)
                if role.provider == "claude":
                    self.assertEqual(args[args.index("--effort") + 1], role.effort)
                else:
                    self.assertIn("model_reasoning_effort=" + json.dumps(role.effort), args)

    def test_fresh_claude_binds_a_uuid_before_launch(self):
        args, bound = runtime.provider_args(self.project, self.dev, None)
        self.assertEqual(args[1], "--session-id")
        self.assertEqual(args[2], bound)
        self.assertEqual(str(runtime.uuid.UUID(bound)), bound)
        settings = json.loads(args[args.index("--settings") + 1])
        self.assertIn("SessionStart", settings["hooks"])
        server = json.loads(args[args.index("--mcp-config") + 1])
        self.assertEqual(set(server["mcpServers"]), {"orai"})
        self.assertEqual(args[args.index("--dangerously-load-development-channels") + 1], "server:orai")

    def test_hooks_and_channel_run_the_installed_package_not_a_source_path(self):
        hook = shlex.split(runtime.hook_command(self.project))
        self.assertEqual(hook, [sys.executable, "-m", "orai", "--project", str(self.root), "_capture"])
        args, _ = runtime.provider_args(self.project, self.dev, None)
        channel = json.loads(args[args.index("--mcp-config") + 1])["mcpServers"]["orai"]
        self.assertEqual(channel, {"command": sys.executable, "args": ["-m", "orai.providers.claude_channel"]})

    def test_configured_qmd_is_injected_per_process(self):
        project = write_project(self.root / "q", open_config(self.root) + "\n[integrations.wiki]\nport = 18555\n")
        (self.root / "q/docs").mkdir()
        claude_args, _ = runtime.provider_args(project, project.config.roles["dev"], None)
        servers = json.loads(claude_args[claude_args.index("--mcp-config") + 1])["mcpServers"]
        self.assertEqual(servers["wiki-q"], {"type": "http", "url": "http://127.0.0.1:18555/mcp"})
        codex_args, _ = runtime.provider_args(project, project.config.roles["lead"], None)
        self.assertTrue(
            any(a.startswith('mcp_servers.wiki-q={"url" = "http://127.0.0.1:18555/mcp"') for a in codex_args)
        )

    def test_capture_valid_identity(self):
        self.state(session_id=None)
        self.assertEqual(self.capture(self.event())[0], 0)
        state = read_json(self.files.state)
        self.assertEqual(state["session_id"], SID)
        self.assertEqual(state["status"], "running")
        self.assertGreater(state["captured_at"], 0)

    def test_hook_restores_identity_without_repeating_bootstrap(self):
        for source in ("startup", "resume", "compact", None):
            with self.subTest(source=source):
                self.state()
                _, output = self.capture(self.event(source=source))
                text = json.loads(output)["hookSpecificOutput"]["additionalContext"]
                self.assertIn("lead", text)
                self.assertIn(str(self.root), text)
                self.assertNotIn("읽어라", text)
                self.assertLess(len(text), 300)

    def test_fresh_prompt_bootstraps_each_role_once(self):
        for role in (self.lead, self.dev):
            with self.subTest(role=role.name):
                args, _ = runtime.provider_args(self.project, role, None)
                self.assertEqual(sum("역할 지침을 읽고" in arg for arg in args), 1)
                self.assertIn(role.guide, args[-1])
                self.assertIn(str(runtime.skill_path(self.project)), args[-1])
                # Channel readiness follows the provider, not the role name.
                self.assertEqual("channel_ready" in args[-1], role.provider == "claude")

    def test_capture_rejects_stale_nonce_cwd_identity_or_stopped_run(self):
        for changes, event, nonce in [
            ({}, self.event(), "old-run"),
            ({}, self.event(cwd=str(self.root / "other")), "run-one"),
            ({}, self.event(session_id=OTHER_SID), "run-one"),
            ({"status": "stopped"}, self.event(), "run-one"),
            ({}, self.event(session_id="not-a-uuid"), "run-one"),
        ]:
            with self.subTest(changes=changes, event=event, nonce=nonce):
                original = self.state(**changes)
                with self.assertRaises((RuntimeError, ValueError)):
                    self.capture(event, nonce)
                self.assertEqual(read_json(self.files.state), original)

    def test_capture_rejects_unconfigured_role(self):
        with self.assertRaisesRegex(RuntimeError, "not configured"):
            self.capture(self.event(), role="ghost")

    def test_capture_ignores_other_hook_events_and_non_orai_sessions(self):
        original = self.state()
        self.assertEqual(self.capture(self.event(hook_event_name="Stop"))[0], 0)
        with patch.dict(os.environ, {}, clear=True):
            self.assertEqual(runtime.capture(self.project), 0)
        self.assertEqual(read_json(self.files.state), original)

    def test_saved_session_requires_exact_recorded_transcript(self):
        transcript = self.root / "recorded.jsonl"
        transcript.write_text("{}\n")
        state = self.state(transcript=str(transcript))
        runtime.validate_saved(state, self.lead, self.root)
        transcript.unlink()
        (self.root / "newer.jsonl").write_text("{}\n")
        with self.assertRaisesRegex(RuntimeError, "transcript is unavailable"):
            runtime.validate_saved(state, self.lead, self.root)

    def test_saved_session_rejects_provider_cwd_or_uncaptured_identity(self):
        for changes in [{"provider": "claude"}, {"cwd": "/wrong"}, {"session_id": None}]:
            with self.subTest(changes=changes), self.assertRaises(RuntimeError):
                runtime.validate_saved(self.state(**changes), self.lead, self.root)

    def test_lock_blocks_second_launch_and_releases_on_close(self):
        self.assertFalse(locked(self.files))
        first = role_lock(self.files)
        try:
            self.assertTrue(locked(self.files))
            with self.assertRaisesRegex(RuntimeError, "already running"):
                role_lock(self.files)
        finally:
            first.close()
        self.assertFalse(locked(self.files))
        role_lock(self.files).close()

    def test_locks_are_per_project(self):
        other = write_project(self.root / "other")
        first = role_lock(self.files)
        try:
            role_lock(RoleFiles(other.roles_dir, "lead")).close()
        finally:
            first.close()

    def test_dry_run_does_not_create_state_or_launch_or_init_mail(self):
        before = sorted(str(p.relative_to(self.root)) for p in self.root.rglob("*"))
        with (
            patch.object(runtime, "checked", side_effect=AssertionError("no subprocess during dry run")),
            patch.object(runtime, "check_worktree"),
            patch.object(mail, "mail_env", return_value={"AM_ROOT": str(self.root / "mail")}),
            patch.object(runtime.subprocess, "Popen") as popen,
            contextlib.redirect_stdout(io.StringIO()) as output,
        ):
            self.assertEqual(runtime.launch(self.project, "lead", dry=True), 0)
        self.assertFalse(popen.called)
        self.assertEqual(before, sorted(str(p.relative_to(self.root)) for p in self.root.rglob("*")))
        plan = json.loads(output.getvalue())
        self.assertIn("--no-init", plan["command"])
        self.assertIn("--no-wake", plan["command"])
        self.assertEqual(plan["project"], str(self.root))

    def test_launch_refuses_inside_a_role_session(self):
        env = {
            "ORAI_ROLE": "dev",
            "AM_ME": "dev",
            "ORAI_SESSION": "orai",
            "AM_SESSION": "orai",
            "AM_ROOT": "/q",
            "AM_BASE_ROOT": "/q",
            "ORAI_PROJECT": str(self.root),
        }
        with patch.dict(os.environ, env, clear=True), self.assertRaisesRegex(RuntimeError, "Already inside"):
            runtime.launch(self.project, "lead", dry=True)

    def test_declared_branch_is_enforced(self):
        project = write_project(
            self.root / "git",
            open_config(self.root).replace('provider = "codex"', 'provider = "codex"\nbranch = "release"'),
            git=True,
        )
        role = project.config.roles["lead"]
        with self.assertRaisesRegex(RuntimeError, "must be on branch release"):
            runtime.check_worktree(project, role, project.root)
        with self.assertRaisesRegex(RuntimeError, "not a Git worktree"):
            runtime.check_worktree(self.project, role, self.root)

    def test_added_role_extends_mailboxes_keeping_known_handles(self):
        mail_root = self.root / "mail"
        for handle in ("lead", "user", "senior"):
            (mail_root / "agents" / handle / "inbox/new").mkdir(parents=True)
        write_json(mail_root / "meta/config.json", {"agents": ["lead", "senior", "user"]})
        calls = []

        def checked(argv, **kwargs):
            calls.append(argv)
            (mail_root / "agents/dev/inbox/new").mkdir(parents=True)
            return ""

        with patch.object(runtime, "checked", side_effect=checked), contextlib.redirect_stdout(io.StringIO()):
            runtime.ensure_mailboxes(self.project, {"AM_ROOT": str(mail_root)})
        self.assertEqual(
            calls, [["amq", "init", "--root", str(mail_root), "--agents", "dev,lead,senior,user", "--force"]]
        )

    def test_mail_requires_the_project_amqrc(self):
        with (
            patch.object(mail, "checked", side_effect=AssertionError("must not ask AMQ to guess a root")),
            self.assertRaisesRegex(RuntimeError, "no .amqrc"),
        ):
            mail.mail_env("lead", self.root, "orai")

    def test_mail_environment_does_not_inherit_other_project_context(self):
        (self.root / ".amqrc").write_text('{"root": ".agent-mail"}\n')
        contexts = [
            {"base_root": "/base"},
            {"root": "/base/orai", "base_root": "/base", "root_id": "r", "base_root_id": "b"},
        ]
        seen_env = []

        def read_context(argv, **kwargs):
            seen_env.append({k: v for k, v in kwargs["env"].items() if k.startswith(("AM", "ORAI_"))})
            return json.dumps(contexts[len(seen_env) - 1])

        leaked = {
            "AM_ME": "wrong",
            "AM_ROOT": "/wrong",
            "AM_SESSION": "wrong",
            "AMQ_GLOBAL_ROOT": "/other",
            "ORAI_PROJECT": "/other",
        }
        with patch.dict(os.environ, leaked, clear=True), patch.object(mail, "checked", side_effect=read_context):
            env = mail.mail_env("lead", self.root, "orai")
        self.assertEqual((env["AM_ME"], env["AM_ROOT"], env["AM_SESSION"]), ("lead", "/base/orai", "orai"))
        self.assertEqual(seen_env[0], {})
        self.assertNotIn("AMQ_GLOBAL_ROOT", env)

    def test_pending_only_lists_new_mail_without_consuming(self):
        with patch.object(mail, "checked", return_value='[{"id":"b"},{"id":"a"}]') as checked:
            self.assertEqual(mail.pending({"AM_ME": "lead"}, self.root), ["a", "b"])
        self.assertEqual(checked.call_args.args[0], ["amq", "list", "--new", "--json"])


class CodexNotifyTest(unittest.TestCase):
    """Ported queue-delivery contracts; `pending` is injected instead of patched."""

    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.files = RoleFiles(Path(temp.name), "lead")

    def state(self, **changes):
        state = {"nonce": "run-one", "session_id": SID, "captured_at": 1}
        state.update(changes)
        write_json(self.files.state, state)

    def notify(self, waits, pending, checked):
        from orai.providers import codex

        stop = Mock()
        stop.wait.side_effect = waits
        with patch.object(codex, "checked", checked):
            codex.notify(self.files, "run-one", {}, stop, pending)

    def test_queue_notifies_exact_session_without_draining_or_repeating(self):
        self.state()
        checked = Mock(return_value="queued")
        self.notify([False, False, True], Mock(return_value=["message-one"]), checked)
        checked.assert_called_once()
        argv = checked.call_args.args[0]
        self.assertEqual(argv[:4], ["codex", "queue", "--thread", SID])
        self.assertEqual(argv[-1], "오라이: 새 메시지\nIDs: message-one")
        self.assertTrue(read_json(self.files.codex_delivery)["ready"])

    def test_queue_only_new_ids_and_failed_delivery_retry(self):
        self.state()
        pending = Mock(side_effect=[["a"], ["a"], ["a", "b"], ["a", "b"], [], ["c"]])
        checked = Mock(side_effect=[RuntimeError("offline"), "ok", "ok", "ok"])
        self.notify([False] * 6 + [True], pending, checked)
        self.assertEqual(checked.call_count, 4)
        self.assertEqual(checked.call_args_list[2].args[0][-1], "오라이: 새 메시지\nIDs: b")
        self.assertTrue(read_json(self.files.codex_delivery)["ready"])
        for call in checked.call_args_list:
            self.assertNotIn("AGENTS", call.args[0][-1])

    def test_queue_waits_for_matching_capture(self):
        for changes in [{"nonce": "old-run"}, {"captured_at": None}]:
            with self.subTest(changes=changes):
                self.state(**changes)
                pending, checked = Mock(), Mock()
                self.notify([False, True], pending, checked)
                pending.assert_not_called()
                checked.assert_not_called()

    def test_queue_failure_reports_error_and_keeps_mail_for_retry(self):
        self.state()
        self.notify([False, True], Mock(return_value=["m"]), Mock(side_effect=RuntimeError("queue unavailable")))
        state = read_json(self.files.codex_delivery)
        self.assertFalse(state["ready"])
        self.assertEqual(state["last_error"], "queue unavailable")


def open_config(root):
    return (Path(root) / "orai.toml").read_text()


if __name__ == "__main__":
    unittest.main()


class ReviewFixesTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()

    def test_project_in_a_repo_subdirectory_can_use_its_own_root_as_worktree(self):
        write_project(self.root / "repo", git=True)
        project = write_project(self.root / "repo/app")
        runtime.check_worktree(project, project.config.roles["lead"], project.root)

    def test_missing_git_is_reported_not_raised_as_oserror(self):
        project = write_project(self.root / "p")
        with (
            patch.object(runtime.subprocess, "run", side_effect=FileNotFoundError("git")),
            self.assertRaisesRegex(RuntimeError, "Cannot run git"),
        ):
            runtime.check_worktree(project, project.config.roles["lead"], project.root)

    def test_launch_that_never_starts_keeps_the_previous_identity(self):
        project = write_project(self.root / "p")
        paths = runtime.files(project, "lead")
        (project.root / "t.jsonl").write_text("{}\n")
        saved = {
            "provider": "codex",
            "cwd": str(project.root),
            "session_id": SID,
            "transcript": str(project.root / "t.jsonl"),
            "status": "stopped",
            "nonce": "old",
        }
        write_json(paths.state, saved)
        tty = io.StringIO()
        tty.isatty = lambda: True
        for fresh in (False, True):
            with (
                self.subTest(fresh=fresh),
                patch.object(mail, "mail_env", return_value={"AM_ROOT": str(self.root / "mail")}),
                patch.object(runtime, "ensure_mailboxes"),
                patch.object(runtime, "check_worktree"),
                patch.object(runtime.subprocess, "Popen", side_effect=FileNotFoundError("amq")),
                patch.object(sys, "stdin", tty),
                contextlib.redirect_stdout(io.StringIO()),
                self.assertRaises(FileNotFoundError),
            ):
                runtime.launch(project, "lead", fresh=fresh)
            self.assertEqual(read_json(paths.state), saved)
            self.assertFalse(locked(paths))
