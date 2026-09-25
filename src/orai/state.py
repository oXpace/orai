"""Private local state: atomic JSON, per-role locks and superseded-session history."""

import fcntl
import json
import os
from pathlib import Path
import tempfile
import time


def read_json(path, default=None):
    try:
        return json.loads(Path(path).read_text())
    except FileNotFoundError:
        return default


def private_dirs(directory):
    """Create missing directories as 0700; mkdir(parents=True) would apply the mode to the last one only."""
    directory = Path(directory)
    missing = []
    while not directory.exists():
        missing.append(directory)
        directory = directory.parent
    for folder in reversed(missing):
        folder.mkdir(mode=0o700, exist_ok=True)


def atomic_write(path, data, mode=0o600):
    path = Path(path)
    private_dirs(path.parent)
    fd, temporary = tempfile.mkstemp(prefix=".orai-", dir=str(path.parent))
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
        os.chmod(temporary, mode)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def json_bytes(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode()


def write_json(path, value):
    atomic_write(path, json_bytes(value))


class RoleFiles:
    """File layout of one role inside a project's role-state directory."""

    def __init__(self, directory, role):
        self.directory = Path(directory)
        self.role = role
        self.state = self.directory / (role + ".json")
        self.lock = self.directory / (role + ".lock")
        # Written by the provider-specific notifier; readiness is judged per run nonce.
        self.codex_delivery = self.directory / (role + ".delivery.json")
        self.claude_channel = self.directory / (role + ".channel.json")

    def delivery(self, provider):
        return self.claude_channel if provider == "claude" else self.codex_delivery

    def archive(self, value):
        history = self.directory.parent / "history"
        write_json(history / f"{self.role}-{time.time_ns()}.json", value)


def role_lock(files):
    private_dirs(files.directory)
    handle = open(files.lock, "a+")  # noqa: SIM115 - the open handle *is* the lock
    os.fchmod(handle.fileno(), 0o600)
    try:
        fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        handle.close()
        raise RuntimeError(f"{files.role} is already running; use its existing terminal") from None
    return handle


def locked(files):
    if not files.lock.exists():
        return False
    with files.lock.open("r") as handle:
        try:
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
            return False
        except BlockingIOError:
            return True
