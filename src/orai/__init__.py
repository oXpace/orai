"""Orai: local role-session launcher and diagnostics over AMQ."""

from importlib.metadata import PackageNotFoundError, version

try:
    __version__ = version("orai")
except PackageNotFoundError:  # running from an unbuilt source tree
    __version__ = "0+unknown"
