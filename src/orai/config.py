"""Shared project declaration (orai.toml): schema, relative paths, roles and integrations.

Only portable facts live here. PIDs, UUIDs, absolute paths and caches are local state.
"""

from dataclasses import dataclass, field
from pathlib import PurePosixPath
import re
import tomllib

SCHEMA = 1
PROVIDERS = ("codex", "claude")
USER_HANDLE = "user"
# Role names double as CLI aliases (`orai <role>`) and AMQ handles.
RESERVED = frozenset(
    {"setup", "run", "status", "doctor", "msg", "wiki", "version", "help", "_capture", USER_HANDLE, "orai", "all"}
    # Former or likely command names stay reserved so a role never shadows one.
    | {"init", "inbox", "send", "reply", "qmd", "codegraph"}
)
WIKI_ENGINES = ("qmd",)
NAME = re.compile(r"^[a-z][a-z0-9-]{0,30}$")
SESSION = re.compile(r"^[a-z0-9][a-z0-9_-]{0,62}$")
COLLECTION = re.compile(r"^[a-z0-9][a-z0-9_-]{0,62}$")


class ConfigError(ValueError):
    pass


@dataclass(frozen=True)
class Role:
    name: str
    provider: str
    worktree: str = "."
    guide: str | None = None
    model: str | None = None
    effort: str | None = None
    branch: str | None = None


@dataclass(frozen=True)
class Smoke:
    """A query whose expected document proves the index serves this project."""

    lex: str
    vec: str
    expect: str


@dataclass(frozen=True)
class Wiki:
    engine: str = "qmd"
    collections: dict = field(default_factory=dict)
    port: int | None = None
    embed_model: str | None = None
    smoke: Smoke | None = None


@dataclass(frozen=True)
class Codegraph:
    smoke_symbol: str | None = None


@dataclass(frozen=True)
class Config:
    name: str | None
    session: str
    roles: dict
    wiki: Wiki | None = None
    codegraph: Codegraph | None = None

    def handles(self):
        return sorted(self.roles) + [USER_HANDLE]


def _table(value, where, allowed):
    if not isinstance(value, dict):
        raise ConfigError(f"{where} must be a table")
    unknown = set(value) - set(allowed)
    if unknown:
        raise ConfigError(f"{where}: unknown key(s) {', '.join(sorted(unknown))}")
    return value


def _text(value, where, required=False):
    if value is None and not required:
        return None
    if not isinstance(value, str) or not value.strip():
        raise ConfigError(f"{where} must be a non-empty string")
    return value


def _relative(value, where, contained=True):
    value = _text(value, where, required=True)
    path = PurePosixPath(value)
    if path.is_absolute() or value.startswith("~"):
        raise ConfigError(f"{where} must be relative to the project root")
    if contained and ".." in path.parts:
        raise ConfigError(f"{where} must stay inside the project root")
    return value


def _role(name, value):
    where = f"roles.{name}"
    if not NAME.match(name) or name in RESERVED:
        raise ConfigError(f"{where}: role names must match {NAME.pattern} and not be a reserved word")
    value = _table(value, where, ("provider", "worktree", "guide", "model", "effort", "branch"))
    provider = value.get("provider")
    if provider not in PROVIDERS:
        raise ConfigError(f"{where}.provider must be one of: {', '.join(PROVIDERS)}")
    return Role(
        name=name,
        provider=provider,
        # Sibling worktrees (../project-role) are allowed; absolute paths are not portable.
        worktree=_relative(value.get("worktree", "."), where + ".worktree", contained=False),
        guide=_relative(value["guide"], where + ".guide") if "guide" in value else None,
        model=_text(value.get("model"), where + ".model"),
        effort=_text(value.get("effort"), where + ".effort"),
        branch=_text(value.get("branch"), where + ".branch"),
    )


def _wiki(value):
    value = _table(value, "integrations.wiki", ("engine", "collections", "port", "embed_model", "smoke"))
    engine = value.get("engine", "qmd")
    if engine not in WIKI_ENGINES:
        raise ConfigError(f"integrations.wiki.engine must be one of: {', '.join(WIKI_ENGINES)}")
    collections = value.get("collections", {"docs": "docs"})
    if not isinstance(collections, dict) or not collections:
        raise ConfigError("integrations.wiki.collections must map at least one name to a folder")
    for key, path in collections.items():
        if not COLLECTION.match(key):
            raise ConfigError(f"integrations.wiki.collections: invalid collection name {key!r}")
        _relative(path, f"integrations.wiki.collections.{key}")
    port = value.get("port")
    if port is not None and (not isinstance(port, int) or isinstance(port, bool) or not 1024 <= port <= 65535):
        raise ConfigError("integrations.wiki.port must be an integer between 1024 and 65535")
    smoke = None
    if "smoke" in value:
        raw = _table(value["smoke"], "integrations.wiki.smoke", ("lex", "vec", "expect"))
        smoke = Smoke(*(_text(raw.get(k), f"integrations.wiki.smoke.{k}", True) for k in ("lex", "vec", "expect")))
    return Wiki(
        engine=engine,
        collections=dict(collections),
        port=port,
        embed_model=_text(value.get("embed_model"), "integrations.wiki.embed_model"),
        smoke=smoke,
    )


def parse(data):
    data = _table(data, "orai.toml", ("schema", "name", "session", "roles", "integrations"))
    if data.get("schema") != SCHEMA:
        raise ConfigError(f"orai.toml: schema must be {SCHEMA} (found {data.get('schema')!r})")
    session = data.get("session", "orai")
    if not isinstance(session, str) or not SESSION.match(session):
        raise ConfigError("session must be a lowercase AMQ session name")
    name = _text(data.get("name"), "name")
    roles = data.get("roles", {})
    if not isinstance(roles, dict):
        raise ConfigError("roles must be a table")
    integrations = _table(data.get("integrations", {}), "integrations", ("wiki", "codegraph"))
    codegraph = None
    if "codegraph" in integrations:
        raw = _table(integrations["codegraph"], "integrations.codegraph", ("smoke_symbol",))
        codegraph = Codegraph(smoke_symbol=_text(raw.get("smoke_symbol"), "integrations.codegraph.smoke_symbol"))
    return Config(
        name=name,
        session=session,
        roles={role: _role(role, value) for role, value in roles.items()},
        wiki=_wiki(integrations["wiki"]) if "wiki" in integrations else None,
        codegraph=codegraph,
    )


def load(path):
    try:
        with open(path, "rb") as stream:
            data = tomllib.load(stream)
    except tomllib.TOMLDecodeError as error:
        raise ConfigError(f"{path}: {error}") from None
    return parse(data)
