"""Wiki engine: QMD. Per-project docs index, server lifecycle and layered verification.

Users and agents see "wiki" (`orai wiki …`, MCP server `wiki-<project>`); QMD stays an engine detail.

Isolation (verified against QMD 2.8.3 source): `--index <name>` scopes the daemon PID/log
files and config file name; QMD_CONFIG_DIR and INDEX_PATH keep config and DB inside
.orai/wiki. The model cache stays shared so models are not downloaded per project.
Never deletes an index and never stops a server this project did not start.
"""

import errno
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import time
import urllib.request

from orai.doctor import BLOCKED, DEGRADED, HEALTHY, NOT_CHECKED, NOT_CONFIGURED, NOT_READY, Check, version_of
from orai.project import slug
from orai.state import private_dirs

MODEL = "hf:Qwen/Qwen3-Embedding-0.6B-GGUF/Qwen3-Embedding-0.6B-Q8_0.gguf"
HOST = "127.0.0.1"
PORT_BASE, PORT_SPAN = 18200, 800
INSTALL_HINT = "Install QMD (docs/compatibility.md); Orai never installs it globally"


def default_port(project_id):
    return PORT_BASE + int(hashlib.sha256(project_id.encode()).hexdigest(), 16) % PORT_SPAN


class Settings:
    def __init__(self, project):
        cfg = project.config.wiki
        self.project = project
        self.index = f"orai-{project.id}"
        self.directory = project.state_dir / "wiki"
        self.config_file = self.directory / f"{self.index}.yml"
        self.db = self.directory / "index.sqlite"
        self.port = cfg.port or default_port(project.id)
        self.endpoint = f"http://{HOST}:{self.port}/mcp"
        self.server_name = f"wiki-{slug(project.name)}"
        self.collections = {name: (project.root / rel).resolve() for name, rel in cfg.collections.items()}
        self.model = cfg.embed_model or MODEL
        self.smoke = cfg.smoke

    def env(self):
        return dict(os.environ, QMD_CONFIG_DIR=str(self.directory), INDEX_PATH=str(self.db))

    def argv(self, qmd, *args):
        return [qmd, "--index", self.index, *args]

    @property
    def pid_file(self):
        cache = Path(os.environ.get("XDG_CACHE_HOME") or Path.home() / ".cache")
        return cache / "qmd" / f"mcp-{self.index}.pid"

    def config_value(self):
        # JSON is valid YAML; QMD reads it as its per-index config.
        return {
            "collections": {name: {"path": str(path), "pattern": "**/*.md"} for name, path in self.collections.items()},
            "models": {"embed": self.model},
        }


class McpError(RuntimeError):
    pass


class Client:
    """Minimal MCP streamable-HTTP client (JSON or SSE responses)."""

    def __init__(self, endpoint, timeout=180):
        self.endpoint = endpoint
        self.timeout = timeout
        self.session = None
        self.number = 0
        # Loopback only: never route through a proxy from the environment.
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

    def request(self, method, params=None, notification=False):
        self.number += 1
        data = {"jsonrpc": "2.0", "method": method, "params": params or {}}
        if not notification:
            data["id"] = self.number
        headers = {
            "Content-Type": "application/json",
            "Accept": "application/json, text/event-stream",
            "MCP-Protocol-Version": "2025-03-26",
        }
        if self.session:
            headers["Mcp-Session-Id"] = self.session
        req = urllib.request.Request(self.endpoint, json.dumps(data).encode(), headers)
        with self.opener.open(req, timeout=self.timeout) as response:
            self.session = response.headers.get("Mcp-Session-Id", self.session)
            raw = response.read().decode()
        if notification:
            return None
        messages = [json.loads(line[5:].strip()) for line in raw.splitlines() if line.startswith("data:")]
        if not messages:
            messages = [json.loads(raw)]
        result = next((m for m in messages if m.get("id") == self.number), None)
        if not result or "error" in result:
            raise McpError(f"MCP {method} failed: {result}")
        return result["result"]

    def connect(self):
        self.request(
            "initialize",
            {"protocolVersion": "2025-03-26", "capabilities": {}, "clientInfo": {"name": "orai", "version": "1"}},
        )
        self.request("notifications/initialized", notification=True)

    def tool(self, name, arguments=None):
        result = self.request("tools/call", {"name": name, "arguments": arguments or {}})
        if result.get("isError"):
            raise McpError(f"QMD {name} failed: {result}")
        return result


def reachability(port):
    """open | refused | denied | error — a denied probe proves nothing about the server."""
    try:
        with socket.create_connection((HOST, port), timeout=2):
            return "open", None
    except ConnectionRefusedError:
        return "refused", None
    except PermissionError as exc:
        return "denied", str(exc)
    except OSError as exc:
        if exc.errno == errno.ECONNREFUSED:
            return "refused", None
        if exc.errno in (errno.EPERM, errno.EACCES):
            return "denied", str(exc)
        return "error", str(exc)


def identity(status, settings):
    """None when the server indexes exactly this project's collection folders."""
    served = {c.get("name"): c.get("path") for c in status.get("collections", [])}
    for name, path in settings.collections.items():
        if name not in served:
            return f"collection {name!r} is not served"
        if Path(served[name] or "").resolve() != path:
            return f"collection {name!r} serves {served[name]}, expected {path}"
    return None


def text_of(result):
    return "".join((c.get("text") or c.get("resource", {}).get("text", "")) for c in result.get("content", []))


def document_key(file):
    """QMD 2.8.3 returns `<collection>/<path>`; older output used `qmd://<collection>/<path>`."""
    return file.removeprefix("qmd://").lower()


def expected_uri(settings):
    """`<collection>/<path>` of the smoke fixture's expected document."""
    target = (settings.project.root / settings.smoke.expect).resolve()
    for name, folder in settings.collections.items():
        if target.is_relative_to(folder):
            return f"{name}/{target.relative_to(folder).as_posix()}"
    return None


def probe(settings, deep=False):
    """Layered checks: reachable → MCP handshake → identity → index → (deep) vec → hybrid → get."""
    c = "wiki"
    state, error = reachability(settings.port)
    detail = {"engine": "qmd", "endpoint": settings.endpoint, "index": settings.index, "db": str(settings.db)}
    if state == "denied":
        return [
            Check(
                c + ".server",
                BLOCKED,
                f"access to {settings.endpoint} denied in this environment "
                f"({error}); this does not show the server is down",
                "Run the check outside the sandbox; a result there applies only to that environment",
                detail=detail,
            )
        ]
    if state == "refused":
        return [
            Check(
                c + ".server",
                BLOCKED,
                "connection refused: no server on the project port",
                "orai wiki recover",
                detail=detail,
            )
        ]
    if state == "error":
        return [Check(c + ".server", BLOCKED, f"port probe failed: {error}", None, detail=detail)]
    checks = [Check(c + ".server", HEALTHY, "port accepts connections", detail=detail)]
    client = Client(settings.endpoint, timeout=180 if deep else 15)
    try:
        client.connect()
        status = client.tool("status").get("structuredContent", {})
    except (McpError, OSError, ValueError) as exc:
        checks.append(
            Check(
                c + ".mcp", BLOCKED, f"MCP handshake/status failed: {exc}", "Check the QMD log, then orai wiki recover"
            )
        )
        return checks
    checks.append(Check(c + ".mcp", HEALTHY, "MCP initialize and status succeeded"))
    mismatch = identity(status, settings)
    if mismatch:
        checks.append(
            Check(
                c + ".identity",
                BLOCKED,
                f"port {settings.port} serves another index: {mismatch}",
                "Left untouched. Set integrations.wiki.port to a free port or stop that server",
            )
        )
        return checks
    checks.append(Check(c + ".identity", HEALTHY, "server indexes this project's collections"))
    if not status.get("totalDocuments"):
        checks.append(Check(c + ".index", NOT_READY, "index is empty", "Add Markdown docs, then orai wiki refresh"))
        return checks
    if not status.get("hasVectorIndex") or status.get("needsEmbedding"):
        checks.append(
            Check(
                c + ".index",
                DEGRADED,
                "embeddings missing or incomplete",
                "orai wiki refresh",
                detail={"needsEmbedding": status.get("needsEmbedding")},
            )
        )
        return checks
    checks.append(Check(c + ".index", HEALTHY, f"{status.get('totalDocuments')} documents with vectors"))
    if not deep:
        checks.append(
            Check(c + ".search", NOT_CHECKED, "semantic search not exercised (loads models)", "orai doctor --deep")
        )
        return checks
    checks.extend(search_checks(client, settings))
    return checks


def search_checks(client, settings):
    c = "wiki"
    names = list(settings.collections)
    smoke = settings.smoke
    expect = expected_uri(settings) if smoke else None
    vec_query = smoke.vec if smoke else "project overview and architecture"
    checks = []

    def hits(searches):
        result = client.tool("query", {"searches": searches, "collections": names, "limit": 5})
        return [h.get("file", "") for h in result.get("structuredContent", {}).get("results", [])]

    try:
        # Vector-only first so a lexical fallback cannot hide an embedding failure.
        vector = hits([{"type": "vec", "query": vec_query}])
    except (McpError, OSError, ValueError) as exc:
        return [
            Check(c + ".vector", BLOCKED, f"vector search failed: {exc}", "Check model/GPU availability in the QMD log")
        ]
    if not vector:
        return [Check(c + ".vector", DEGRADED, "vector search returned no results", "orai wiki refresh")]
    if expect and not any(document_key(h) == document_key(expect) for h in vector):
        checks.append(
            Check(
                c + ".vector",
                DEGRADED,
                f"vector search missed the smoke document {expect}",
                "Review recall for this model (docs/operations.md)",
                detail={"hits": vector},
            )
        )
    else:
        checks.append(
            Check(
                c + ".vector",
                HEALTHY,
                "vector-only search returned "
                + ("the smoke document" if expect else "results (no smoke fixture configured)"),
            )
        )
    try:
        hybrid = hits(
            [{"type": "lex", "query": smoke.lex if smoke else "overview"}, {"type": "vec", "query": vec_query}]
        )
        if not hybrid:
            raise McpError("no results")
        body = text_of(client.tool("get", {"file": expect or hybrid[0], "maxLines": 20}))
    except (McpError, OSError, ValueError) as exc:
        checks.append(Check(c + ".hybrid", BLOCKED, f"lex+vec search or document read failed: {exc}", None))
        return checks
    checks.append(
        Check(
            c + ".hybrid",
            HEALTHY if body.strip() else DEGRADED,
            "lex+vec search and document read succeeded" if body.strip() else "document read returned no text",
        )
    )
    return checks


def diagnose(project, deep=False):
    if not project.config.wiki:
        return [Check("wiki", NOT_CONFIGURED, "no [integrations.wiki] in orai.toml")]
    settings = Settings(project)
    if not shutil.which("qmd"):
        return [Check("wiki", BLOCKED, "wiki engine qmd is not on PATH", INSTALL_HINT)]
    missing = [name for name, path in settings.collections.items() if not path.is_dir()]
    if missing:
        return [
            Check(
                "wiki",
                NOT_READY,
                f"collection folder(s) missing: {', '.join(missing)}",
                "Create the folders or fix integrations.wiki.collections",
            )
        ]
    if not settings.config_file.exists() or not settings.db.exists():
        return [
            Check(
                "wiki",
                NOT_READY,
                "project index not initialized",
                "orai wiki init",
                detail={"version": version_of("qmd")},
            )
        ]
    return probe(settings, deep)


# ---- lifecycle (explicit, state-changing) ----------------------------------------------


def run(argv, settings, capture=False):
    print("+ " + " ".join(map(str, argv)), flush=True)
    return subprocess.run(
        argv,
        cwd=settings.project.root,
        env=settings.env(),
        check=True,
        text=True,
        stdout=subprocess.PIPE if capture else None,
    ).stdout


def own_server_running(settings):
    """True if our server is up; raises when the port is unusable or serves something else."""
    state, error = reachability(settings.port)
    if state == "refused":
        return False
    if state != "open":
        raise RuntimeError(f"Cannot probe {settings.endpoint} ({state}: {error}); run outside the sandbox")
    client = Client(settings.endpoint, timeout=15)
    client.connect()
    mismatch = identity(client.tool("status").get("structuredContent", {}), settings)
    if mismatch:
        raise RuntimeError(
            f"Port {settings.port} serves another index ({mismatch}). Left untouched; set integrations.wiki.port."
        )
    return True


def lifecycle(project, action):
    """init | recover | refresh | check | stop — see docs/operations.md."""
    if not project.config.wiki:
        raise RuntimeError("Wiki is not configured: add [integrations.wiki] to orai.toml")
    settings = Settings(project)
    for name, path in settings.collections.items():
        if not path.is_dir():
            raise RuntimeError(f"Collection {name!r} folder is missing: {path}")
    if action == "check":
        return verify(settings)
    qmd = shutil.which("qmd")
    if not qmd:
        raise RuntimeError("wiki engine qmd is not on PATH. " + INSTALL_HINT)
    private_dirs(settings.directory)
    with (settings.directory / "setup.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise RuntimeError("Another orai wiki command is running for this project") from None
        if action == "stop":
            # The PID file is scoped to this project's index name; other servers are untouched.
            run(settings.argv(qmd, "mcp", "stop"), settings)
            return 0
        if action != "init" and (not settings.config_file.exists() or not settings.db.exists()):
            raise RuntimeError("Project wiki config/index missing. Run `orai wiki init`.")
        running = own_server_running(settings)
        if running and action in ("init", "refresh"):
            raise RuntimeError(
                "This project's wiki server is running. Run `orai wiki stop` at a safe "
                "boundary (no search in progress), then retry."
            )
        if not settings.config_file.exists():
            # Exclusive creation: an existing config (model, collections) is never overwritten.
            with settings.config_file.open("x") as stream:
                json.dump(settings.config_value(), stream, indent=2)
                stream.write("\n")
        for name, path in settings.collections.items():
            shown = run(settings.argv(qmd, "collection", "show", name), settings, capture=True)
            paths = [line.split(":", 1)[1].strip() for line in shown.splitlines() if line.strip().startswith("Path:")]
            if paths != [str(path)]:
                raise RuntimeError(
                    f"Collection {name!r} points elsewhere ({paths}); fix {settings.config_file}. "
                    "Config and DB preserved."
                )
        if action in ("init", "refresh"):
            run(settings.argv(qmd, "update"), settings)
            run(settings.argv(qmd, "embed"), settings)
        if not running:
            run(settings.argv(qmd, "mcp", "--http", "--daemon", "--host", HOST, "--port", str(settings.port)), settings)
            for _ in range(30):
                if reachability(settings.port)[0] == "open":
                    break
                time.sleep(1)
            else:
                raise RuntimeError(
                    f"Server did not become reachable within 30 seconds; see {settings.pid_file.with_suffix('.log')}"
                )
    return verify(settings)


def verify(settings):
    checks = probe(settings, deep=True)
    failed = [c for c in checks if c.status != HEALTHY]
    for check in checks:
        print(f"{check.status:>9}  {check.component}: {check.reason}")
    if failed:
        raise RuntimeError("Wiki is not verified: " + "; ".join(f"{c.component} {c.status}" for c in failed))
    print(f"Ready: {settings.endpoint} (MCP server name: {settings.server_name})")
    return 0
