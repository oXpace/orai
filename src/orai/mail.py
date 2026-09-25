"""AMQ calls: identity, pending IDs and message actions. AMQ owns queues and receipts."""

import json
import os
from pathlib import Path
import sys

from orai.config import NAME
from orai.process import checked, run

# Context inherited from another project's shell must never pick our mailbox.
FOREIGN_CONTEXT = ("AM_", "AMQ_GLOBAL_ROOT", "ORAI_")


def clean_env(environ=None):
    environ = os.environ if environ is None else environ
    return {k: v for k, v in environ.items() if not k.startswith(FOREIGN_CONTEXT)}


def mail_env(handle, root, session):
    """Resolve the project's AMQ base (its own .amqrc) with the caller's AMQ context removed."""
    if not (Path(root) / ".amqrc").is_file():
        raise RuntimeError(f"{root} has no .amqrc; run `orai setup` to prepare the project's AMQ root")
    env = clean_env()
    base = json.loads(checked(["amq", "env", "--json"], cwd=root, env=dict(env)))["base_root"]
    env.update(AM_ROOT=base, AM_BASE_ROOT=base, AM_SESSION="")
    ctx = json.loads(checked(["amq", "env", "--session", session, "--me", handle, "--json"], cwd=root, env=dict(env)))
    env.update(AM_ROOT=ctx["root"], AM_BASE_ROOT=ctx["base_root"], AM_SESSION=session, AM_ME=handle)
    for key in ("root_id", "base_root_id"):
        if ctx.get(key):
            env["AM_" + key.upper()] = ctx[key]
    return env


def role_identity():
    """The fixed identity of the role session this process runs in, or None."""
    role = os.environ.get("ORAI_ROLE")
    if not role:
        return None
    if not NAME.match(role):
        raise RuntimeError("Invalid ORAI_ROLE; reconnect the role")
    session = os.environ.get("ORAI_SESSION")
    if (
        os.environ.get("AM_ME") != role
        or not session
        or os.environ.get("AM_SESSION") != session
        or not os.environ.get("AM_ROOT")
        or not os.environ.get("AM_BASE_ROOT")
        or not os.environ.get("ORAI_PROJECT")
    ):
        raise RuntimeError("Incomplete Orai messaging environment; reconnect the role")
    return role


def message_env(actor, project_loader):
    """Role sessions keep their identity; outside them only `--as user` is allowed."""
    if role_identity():
        if actor:
            raise RuntimeError("An Orai role cannot override its messaging identity")
        return dict(os.environ)
    if actor != "user":
        raise RuntimeError("Outside Orai, use --as user for an authorized Desktop action")
    project = project_loader()
    return mail_env("user", project.root, project.config.session)


def pending(env, cwd):
    value = json.loads(checked(["amq", "list", "--new", "--json"], env=env, cwd=cwd))
    return sorted(item["id"] for item in (value or []))


def read_body(args):
    if args.file:
        body = Path(args.file).read_text(encoding="utf-8")
    elif args.body is not None:
        body = args.body
    elif sys.stdin.isatty():
        # An interactive terminal would block on read() waiting for EOF.
        raise RuntimeError("Provide the message body with --body, --file, or piped stdin")
    else:
        body = sys.stdin.read()
    if not body.strip():
        raise RuntimeError("Message body must not be empty")
    return body


def message_action(args, env, cwd):
    if args.command == "inbox":
        if args.message_id:
            if args.peek or args.limit is not None:
                raise RuntimeError("An inbox ID cannot be combined with --peek or --limit")
            argv = ["amq", "read", "--id", args.message_id, "--json"]
        else:
            argv = (
                ["amq", "list", "--new", "--json"]
                if args.peek
                else [
                    "amq",
                    "drain",
                    "--include-body",
                    "--json",
                    "--limit",
                    str(20 if args.limit is None else args.limit),
                ]
            )
        result = run(argv, env=env, cwd=cwd)
    else:
        body = read_body(args)
        argv = ["amq", args.command, "--to" if args.command == "send" else "--id", args.target, "--body", "-", "--json"]
        for key in ("kind", "thread"):
            value = getattr(args, key, None)
            if value:
                argv += ["--" + key, value]
        result = run(argv, env=env, cwd=cwd, input=body)
    # Keep AMQ's diagnostics (including sibling-session warnings) and exit code.
    if result.stderr:
        print(result.stderr, file=sys.stderr, end="")
    if result.stdout:
        print(result.stdout, end="")
    elif not result.returncode:
        print("[]")
    return result.returncode
