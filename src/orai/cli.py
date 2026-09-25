"""Argument parsing, output and exit codes.

Exit codes: 0 success; 1 runtime failure (or doctor: degraded); 2 usage/config error;
3 doctor: a core component is blocked. Message actions return AMQ's own exit code.
"""

import argparse
import os
from pathlib import Path
import subprocess
import sys

from orai import __version__, bootstrap, mail, runtime, scaffold
from orai import project as project_module
from orai.config import USER_HANDLE, ConfigError
from orai.doctor import BLOCKED, NOT_CONFIGURED, Check, report, tool_checks
from orai.integrations import codegraph, qmd

COMMANDS = {"setup", "run", "status", "doctor", "msg", "wiki", "_capture"}
KINDS = ["brainstorm", "review_request", "review_response", "question", "answer", "decision", "status", "todo"]
EXIT_USAGE = 2
RENAMED = {
    "inbox": "orai msg inbox",
    "send": "orai msg send",
    "reply": "orai msg reply",
    "qmd": "orai wiki",
    "init": "orai setup",
}


class UsageError(Exception):
    pass


def build_parser():
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument(
        "--project",
        metavar="PATH",
        default=argparse.SUPPRESS,
        help="project directory (default: nearest orai.toml above the current directory)",
    )
    parser = argparse.ArgumentParser(
        prog="orai",
        parents=[common],
        description="Run role sessions of Codex/Claude over AMQ with exact resume, notifications and "
        "diagnostics. `orai <role>` is short for `orai run <role>`.",
    )
    parser.add_argument("--version", action="version", version=f"orai {__version__}")
    commands = parser.add_subparsers(dest="command", required=True, metavar="<command>")

    setup = commands.add_parser("setup", parents=[common], help="Set up this project (safe to re-run)")
    setup.add_argument("--preset", choices=sorted(scaffold.PRESETS), default="minimal")
    setup.add_argument("--branch", default=scaffold.DEFAULT_BRANCH, help="branch for a new repository")
    setup.add_argument("--dry-run", action="store_true", help="Preview every change; write nothing")
    setup.add_argument("--no-tools", action="store_true", help="Only files; skip the wiki server and code graph")

    launch = commands.add_parser("run", parents=[common], help="Start or exactly resume a role")
    launch.add_argument("role")
    launch.add_argument("--fresh", action="store_true", help="Start a new conversation (old one is kept)")
    launch.add_argument("--dry-run", action="store_true", help="Print the launch plan; change nothing")

    commands.add_parser("status", parents=[common], help="Role processes, delivery readiness, pending mail")
    doctor = commands.add_parser("doctor", parents=[common], help="Read-only diagnosis as JSON")
    doctor.add_argument("--deep", action="store_true", help="Also exercise semantic search and symbol lookup")

    msg = commands.add_parser("msg", parents=[common], help="Messages: inbox, send, reply")
    actions = msg.add_subparsers(dest="msg_command", required=True, metavar="<action>")
    for name, text in (
        ("inbox", "Receive pending messages"),
        ("send", "Send a new request"),
        ("reply", "Reply to a received message"),
    ):
        action = actions.add_parser(name, parents=[common], help=text)
        action.add_argument(
            "--as", dest="actor", choices=[USER_HANDLE], help="Desktop only; requires the user's authorization"
        )
        if name == "inbox":
            action.add_argument("message_id", nargs="?", help="Receive one message directly by ID")
            action.add_argument("--peek", action="store_true", help="List without consuming")
            action.add_argument("--limit", type=int, help="Receive at most N messages (default 20; 0 = all)")
            continue
        action.add_argument("target", help="role or user" if name == "send" else "received message ID")
        body = action.add_mutually_exclusive_group()
        body.add_argument("--body", help="Literal message text; default reads piped stdin")
        body.add_argument("--file", help="Read message text from a UTF-8 file")
        action.add_argument("--kind", choices=KINDS)
        if name == "send":
            action.add_argument("--thread")

    wiki = commands.add_parser("wiki", parents=[common], help="Project wiki (docs search) lifecycle")
    wiki.add_argument(
        "action",
        choices=["init", "recover", "refresh", "check", "stop"],
        help="init: create the index and start; recover: start the existing index; "
        "refresh: re-index changed docs; check: verify only; stop: stop this project's server",
    )

    # Internal SessionStart hook target: no help text keeps it out of the command list.
    commands.add_parser("_capture", parents=[common])
    return parser


def first_word(argv):
    index = 0
    while index < len(argv):
        token = argv[index]
        if token == "--project":
            index += 2
        elif token.startswith("--project="):
            index += 1
        else:
            return index, token
    return None, None


def normalize(argv):
    """Rewrite `orai <role> ...` to `orai run <role> ...`."""
    index, token = first_word(argv)
    if token is None or token.startswith("-") or token in COMMANDS:
        return argv
    return argv[:index] + ["run"] + argv[index:]


def setup(args, explicit):
    root = Path(explicit).expanduser().resolve() if explicit else project_module.setup_target(Path.cwd())
    if not root.is_dir():
        raise UsageError(f"Not a directory: {root}")
    return bootstrap.setup(root, args.preset, args.dry_run, args.branch, diagnose, tools=not args.no_tools)


def diagnose(project, deep=False):
    local = runtime.diagnose(project)
    providers = {role.provider for role in project.config.roles.values()}
    return (
        local[:1] + tool_checks(providers) + local[1:] + qmd.diagnose(project, deep) + codegraph.diagnose(project, deep)
    )


def doctor(explicit, deep):
    try:
        project = project_module.load(explicit or Path.cwd())
    except project_module.ProjectNotFound as exc:
        return report([Check("project", NOT_CONFIGURED, str(exc), "orai setup", core=True), *tool_checks(set())])
    except ConfigError as exc:
        return report([Check("project", BLOCKED, str(exc), "Fix orai.toml", core=True)])
    return report(diagnose(project, deep), project=str(project.root), deep=deep)


def message(args, explicit):
    if args.command == "inbox" and args.limit is not None and args.limit < 0:
        raise UsageError("--limit must be non-negative")
    if mail.role_identity():
        # A role's identity is bound to its own project; never mix in another one.
        project = project_module.load(os.environ["ORAI_PROJECT"])
        if explicit and project_module.locate(explicit) != project.root:
            raise UsageError("A role session cannot act on another project")
    else:
        project = project_module.load(explicit or Path.cwd())
    if args.command == "send" and args.target not in project.config.handles():
        raise UsageError(f"Unknown recipient {args.target!r}; choose from {', '.join(project.config.handles())}")
    env = mail.message_env(args.actor, lambda: project)
    return mail.message_action(args, env, project.root)


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    _, word = first_word(argv)
    if word in RENAMED:
        print(f"orai: `{word}` moved; use `{RENAMED[word]}`", file=sys.stderr)
        return EXIT_USAGE
    args = build_parser().parse_args(normalize(argv))
    explicit = getattr(args, "project", None)
    try:
        if args.command == "msg":
            args.command = args.msg_command  # mail.message_action dispatches on the AMQ verb
            return message(args, explicit)
        if args.command == "setup":
            return setup(args, explicit)
        if args.command == "doctor":
            return doctor(explicit, args.deep)
        project = project_module.load(explicit or Path.cwd())
        if args.command == "_capture":
            return runtime.capture(project)
        if args.command == "status":
            return runtime.status(project)
        if args.command == "wiki":
            return qmd.lifecycle(project, args.action)
        return runtime.launch(project, args.role, args.fresh, args.dry_run)
    except KeyError as exc:  # a missing field is a defect or corrupt state, not a usage error
        print(f"orai: missing {exc}", file=sys.stderr)
        return 1
    except (UsageError, ConfigError, LookupError, project_module.ProjectNotFound, scaffold.Conflict) as exc:
        print("orai: " + str(exc), file=sys.stderr)
        return EXIT_USAGE
    except (RuntimeError, OSError, ValueError, TypeError, subprocess.SubprocessError) as exc:
        print("orai: " + str(exc), file=sys.stderr)
        return 1
