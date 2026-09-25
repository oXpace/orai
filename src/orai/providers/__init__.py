"""Provider adapters: CLI arguments and the native channel each CLI uses for notifications."""

NOTICE = "오라이: 새 메시지\nIDs: "


def notice(ids):
    """Notifications carry only new message IDs; the role receives them with `orai msg inbox <ID>`."""
    return NOTICE + ", ".join(sorted(ids))


def session_start_hook(command):
    return [{"matcher": "startup|resume|compact", "hooks": [{"type": "command", "command": command, "timeout": 10}]}]
