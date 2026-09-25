"""Subprocess helpers that keep a child's diagnostics instead of hiding them."""

import subprocess


def run(argv, timeout=20, **kwargs):
    return subprocess.run(argv, text=True, capture_output=True, timeout=timeout, **kwargs)


def checked(argv, **kwargs):
    result = run(argv, **kwargs)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or str(argv))
    return result.stdout
