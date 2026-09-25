"""Doctor: exit codes, tool detection and the QMD/CodeGraph diagnosis layers.

Hermetic: every subprocess a check might run (amq, qmd, codegraph, codex, claude) is a fake
binary on a PATH we control, and every QMD MCP call goes through a patched `qmd.Client`.
Nothing here talks to a real AMQ root, a real QMD server or a real CodeGraph index, and no
test binds or connects to port 8181.
"""

import json
import socket

from support import fake_binary, path_with, write_project

from orai import cli
from orai.doctor import (
    BLOCKED,
    DEGRADED,
    EXIT_BLOCKED,
    EXIT_DEGRADED,
    EXIT_OK,
    HEALTHY,
    NOT_CHECKED,
    NOT_CONFIGURED,
    NOT_READY,
    UNSUPPORTED,
    Check,
    overall,
    report,
    tool,
)
from orai.integrations import codegraph, qmd

# ---- fake CLIs ---------------------------------------------------------------------------

AMQ_SCRIPT = """
    import json
    import sys

    argv = sys.argv[1:]
    if argv == ["--version"]:
        print("amq 1.0.0")
    elif argv == ["coop", "exec", "--help"]:
        print("-no-wake -no-init -named -root")
    elif argv == ["drain", "--help"]:
        print("-include-body -limit")
    elif argv and argv[0] == "env":
        print(json.dumps({"base_root": "/tmp/orai-test-amq", "root": "/tmp/orai-test-amq"}))
    else:
        print("{}")
"""

FOO_SCRIPT_TEMPLATE = """
    import sys

    argv = sys.argv[1:]
    if argv == ["--version"]:
        print("foo 1.2.3")
    elif argv == ["sub", "--help"]:
        print("{flags}")
"""

CODEGRAPH_SCRIPT = """
    import json
    import os
    import sys

    config = json.loads(os.environ["CODEGRAPH_FAKE_CONFIG"])
    argv = sys.argv[1:]
    if argv[0] == "status":
        print(json.dumps(config["status"]))
    elif argv[0] == "query":
        print(json.dumps(config.get("query", [])))
    else:
        print(json.dumps({}))
"""

CODEGRAPH_TOML = """
schema = 1
session = "orai"

[integrations.codegraph]
smoke_symbol = "kickoff"
"""


def free_port():
    """Bind then release a loopback port so a subsequent connect is refused, not accepted."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind((qmd.HOST, 0))
        return sock.getsockname()[1]


def qmd_settings(tmp_path, smoke=False, port=19222):
    smoke_block = ""
    if smoke:
        smoke_block = '[integrations.wiki.smoke]\nlex = "overview"\nvec = "architecture"\nexpect = "docs/found.md"\n'
    text = f"""
    schema = 1
    session = "orai"

    [integrations.wiki]
    port = {port}

    {smoke_block}
    """
    project = write_project(tmp_path / "proj", text=text)
    return qmd.Settings(project)


def status_result(settings, total=5, has_vector=True, needs_embedding=0, collections=None):
    served = collections
    if served is None:
        served = [{"name": name, "path": str(path)} for name, path in settings.collections.items()]
    return {
        "structuredContent": {
            "collections": served,
            "totalDocuments": total,
            "hasVectorIndex": has_vector,
            "needsEmbedding": needs_embedding,
        }
    }


def make_client(status=None, query=None, get=None, connect_error=None):
    """Build a fake replacement for qmd.Client; returns (class, call log)."""
    calls = []

    class FakeClient:
        def __init__(self, endpoint, timeout=15):
            self.endpoint = endpoint
            self.timeout = timeout

        def connect(self):
            if connect_error:
                raise connect_error

        def tool(self, name, arguments=None):
            calls.append((name, arguments))
            if name == "status":
                return status() if callable(status) else status
            if name == "query":
                return query(arguments) if callable(query) else query
            if name == "get":
                return get(arguments) if callable(get) else get
            raise AssertionError(f"unexpected tool call: {name}")

    return FakeClient, calls


# ---- overall()/report() ------------------------------------------------------------------


def test_overall_core_blocked_is_exit_3():
    checks = [Check("tool.amq", BLOCKED, "amq is not on PATH", core=True)]
    status, code = overall(checks)
    assert status == BLOCKED
    assert code == EXIT_BLOCKED


def test_overall_optional_blocked_is_exit_1():
    checks = [
        Check("project", HEALTHY, "orai.toml is valid", core=True),
        Check("wiki.server", BLOCKED, "connection refused"),
    ]
    status, code = overall(checks)
    assert status == DEGRADED
    assert code == EXIT_DEGRADED


def test_overall_core_not_configured_is_exit_1():
    checks = [Check("project", NOT_CONFIGURED, "no orai.toml above this directory", core=True)]
    status, code = overall(checks)
    assert status == NOT_CONFIGURED
    assert code == EXIT_DEGRADED


def test_overall_all_healthy_with_optional_not_configured_is_exit_0():
    checks = [
        Check("project", HEALTHY, "orai.toml is valid", core=True),
        Check("wiki", NOT_CONFIGURED, "no [integrations.wiki] in orai.toml"),
    ]
    status, code = overall(checks)
    assert status == HEALTHY
    assert code == EXIT_OK


def test_report_prints_component_status_reason_checked_at_and_matches_exit_code(capsys):
    checks = [Check("project", HEALTHY, "orai.toml is valid", core=True, detail={"root": "/x"})]
    code = report(checks, project="/x")
    assert code == EXIT_OK
    payload = json.loads(capsys.readouterr().out)
    assert payload["status"] == HEALTHY
    assert payload["project"] == "/x"
    assert "checked_at" in payload
    entry = payload["checks"][0]
    assert entry["component"] == "project"
    assert entry["status"] == HEALTHY
    assert entry["reason"] == "orai.toml is valid"
    assert "checked_at" in entry


# ---- tool() -------------------------------------------------------------------------------


def test_tool_missing_binary_is_blocked(tmp_path, monkeypatch):
    monkeypatch.setenv("PATH", str(tmp_path))
    check = tool("tool.foo", "foo", True, [], "brew install foo")
    assert check.status == BLOCKED
    assert "not on PATH" in check.reason


def test_tool_unsupported_when_help_lacks_a_required_flag(tmp_path, monkeypatch):
    fake_binary(tmp_path, "foo", FOO_SCRIPT_TEMPLATE.format(flags="--alpha"))
    monkeypatch.setenv("PATH", path_with(tmp_path))
    check = tool("tool.foo", "foo", True, [(["foo", "sub", "--help"], ["--alpha", "--beta"])], "brew install foo")
    assert check.status == UNSUPPORTED
    assert "--beta" in check.reason


def test_tool_healthy_with_version_parsed_from_dash_dash_version(tmp_path, monkeypatch):
    fake_binary(tmp_path, "foo", FOO_SCRIPT_TEMPLATE.format(flags="--alpha --beta"))
    monkeypatch.setenv("PATH", path_with(tmp_path))
    check = tool("tool.foo", "foo", True, [(["foo", "sub", "--help"], ["--alpha", "--beta"])], "brew install foo")
    assert check.status == HEALTHY
    assert check.detail["version"] == "1.2.3"


# ---- cli.main(["doctor", ...]) outside any project ---------------------------------------


def test_cli_doctor_outside_any_project_is_not_configured(tmp_path, monkeypatch, capsys):
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    fake_binary(bin_dir, "amq", AMQ_SCRIPT)
    monkeypatch.setenv("PATH", path_with(bin_dir))
    target = tmp_path / "not-a-project"
    target.mkdir()
    code = cli.main(["doctor", "--project", str(target)])
    payload = json.loads(capsys.readouterr().out)
    assert payload["status"] == NOT_CONFIGURED
    assert code == EXIT_DEGRADED


# ---- the motivating incident: doctor must not report healthy when QMD refuses connections -


def test_doctor_reports_qmd_server_refused_and_is_never_healthy(tmp_path, monkeypatch, capsys):
    port = free_port()
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    fake_binary(bin_dir, "amq", AMQ_SCRIPT)
    fake_binary(bin_dir, "qmd", "pass")
    monkeypatch.setenv("PATH", path_with(bin_dir))

    project = write_project(
        tmp_path / "proj",
        text=f"""
        schema = 1
        session = "orai"

        [integrations.wiki]
        port = {port}
        """,
    )
    (project.root / "docs").mkdir()
    settings = qmd.Settings(project)
    settings.directory.mkdir(parents=True)
    settings.config_file.write_text(json.dumps(settings.config_value()))
    settings.db.write_bytes(b"fake-sqlite-bytes")

    code = cli.main(["doctor", "--project", str(project.root)])
    payload = json.loads(capsys.readouterr().out)

    assert code != EXIT_OK
    assert payload["status"] != HEALTHY
    server_check = next(c for c in payload["checks"] if c["component"] == "wiki.server")
    assert server_check["status"] == BLOCKED
    assert "refused" in server_check["reason"]


# ---- reachability(): permission-denied must never be read as "down" ----------------------


def test_reachability_permission_error_is_denied(monkeypatch):
    def raise_denied(*_args, **_kwargs):
        raise PermissionError("operation not permitted")

    monkeypatch.setattr(qmd.socket, "create_connection", raise_denied)
    state, error = qmd.reachability(12345)
    assert state == "denied"
    assert "operation not permitted" in error


def test_probe_denied_reason_does_not_claim_the_server_is_down(monkeypatch, tmp_path):
    def raise_denied(*_args, **_kwargs):
        raise PermissionError("operation not permitted")

    monkeypatch.setattr(qmd.socket, "create_connection", raise_denied)
    settings = qmd_settings(tmp_path)
    checks = qmd.probe(settings)
    assert len(checks) == 1
    assert checks[0].status == BLOCKED
    assert "does not show the server is down" in checks[0].reason


# ---- probe() layers, with a patched qmd.Client --------------------------------------------


def test_probe_mcp_handshake_failure_blocks_mcp(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    client_cls, _calls = make_client(connect_error=qmd.McpError("handshake failed"))
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings)
    by_component = {c.component: c for c in checks}
    assert by_component["wiki.server"].status == HEALTHY
    assert by_component["wiki.mcp"].status == BLOCKED
    assert "handshake" in by_component["wiki.mcp"].reason.lower()


def test_probe_identity_mismatch_blocks_and_stops_further_checks(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    other = {"structuredContent": {"collections": [{"name": "docs", "path": "/somewhere/else"}]}}
    client_cls, _calls = make_client(status=other)
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings)
    assert [c.component for c in checks] == [
        "wiki.server",
        "wiki.mcp",
        "wiki.identity",
    ]
    assert checks[-1].status == BLOCKED
    assert "serves another index" in checks[-1].reason


def test_probe_empty_index_is_not_ready(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    client_cls, _calls = make_client(status=status_result(settings, total=0))
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings)
    assert checks[-1].component == "wiki.index"
    assert checks[-1].status == NOT_READY


def test_probe_needs_embedding_is_degraded(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    status = status_result(settings, total=5, has_vector=True, needs_embedding=3)
    client_cls, _calls = make_client(status=status)
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings)
    assert checks[-1].component == "wiki.index"
    assert checks[-1].status == DEGRADED
    assert checks[-1].detail["needsEmbedding"] == 3


def test_probe_healthy_index_not_deep_marks_search_not_checked(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    client_cls, _calls = make_client(status=status_result(settings))
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings, deep=False)
    assert checks[-1].component == "wiki.search"
    # Not claimed as verified (not healthy), yet not a problem that fails the default doctor.
    assert checks[-1].status == NOT_CHECKED
    assert checks[-1].next_action == "orai doctor --deep"
    assert overall(checks) == (HEALTHY, EXIT_OK)


def test_probe_deep_vector_failure_blocks_vector_and_lexical_success_cannot_mask_it(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))

    def query(arguments):
        if len(arguments["searches"]) == 1:
            raise qmd.McpError("vector backend unavailable")
        return {"structuredContent": {"results": [{"file": "docs/found.md"}]}}

    client_cls, calls = make_client(status=status_result(settings), query=query, get={"content": [{"text": "hi"}]})
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings, deep=True)
    assert checks[-1].component == "wiki.vector"
    assert checks[-1].status == BLOCKED
    assert "vector search failed" in checks[-1].reason
    # The hybrid (lex+vec) query and the document read must never run once vector fails.
    assert not any(name == "get" for name, _args in calls)
    assert not any(name == "query" and len(args["searches"]) == 2 for name, args in calls)


def test_probe_deep_vector_ok_but_misses_smoke_document_is_degraded(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path, smoke=True)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))

    def query(_arguments):
        return {"structuredContent": {"results": [{"file": "docs/other.md"}]}}

    client_cls, _calls = make_client(status=status_result(settings), query=query, get={"content": [{"text": "hi"}]})
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings, deep=True)
    vector = next(c for c in checks if c.component == "wiki.vector")
    assert vector.status == DEGRADED
    assert "missed the smoke document" in vector.reason


def test_probe_deep_smoke_configured_and_hit_is_healthy(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path, smoke=True)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    expect = qmd.expected_uri(settings)

    def query(_arguments):
        return {"structuredContent": {"results": [{"file": expect}]}}

    client_cls, _calls = make_client(
        status=status_result(settings), query=query, get={"content": [{"text": "hello world"}]}
    )
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings, deep=True)
    assert all(c.status == HEALTHY for c in checks)


def test_probe_deep_get_returning_empty_text_is_degraded(monkeypatch, tmp_path):
    settings = qmd_settings(tmp_path, smoke=True)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    expect = qmd.expected_uri(settings)

    def query(_arguments):
        return {"structuredContent": {"results": [{"file": expect}]}}

    client_cls, _calls = make_client(status=status_result(settings), query=query, get={"content": []})
    monkeypatch.setattr(qmd, "Client", client_cls)

    checks = qmd.probe(settings, deep=True)
    hybrid = next(c for c in checks if c.component == "wiki.hybrid")
    assert hybrid.status == DEGRADED
    assert "no text" in hybrid.reason


# ---- codegraph.diagnose() ------------------------------------------------------------------


def test_codegraph_not_configured(tmp_path):
    project = write_project(tmp_path / "proj")
    checks = codegraph.diagnose(project)
    assert len(checks) == 1
    assert checks[0].component == "codegraph"
    assert checks[0].status == NOT_CONFIGURED


def test_codegraph_binary_missing_is_blocked(tmp_path, monkeypatch):
    empty_bin = tmp_path / "empty-bin"
    empty_bin.mkdir()
    monkeypatch.setenv("PATH", str(empty_bin))
    project = write_project(tmp_path / "proj", text=CODEGRAPH_TOML)
    checks = codegraph.diagnose(project)
    assert checks[0].status == BLOCKED
    assert "not on PATH" in checks[0].reason


def codegraph_project(tmp_path, config, monkeypatch):
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir()
    fake_binary(bin_dir, "codegraph", CODEGRAPH_SCRIPT)
    monkeypatch.setenv("PATH", path_with(bin_dir))
    monkeypatch.setenv("CODEGRAPH_FAKE_CONFIG", json.dumps(config))
    return write_project(tmp_path / "proj", text=CODEGRAPH_TOML)


COMPLETE_STATUS = {
    "initialized": True,
    "version": "1.0.0",
    "lastIndexed": "2026-09-01T00:00:00Z",
    "fileCount": 12,
    "nodeCount": 100,
    "index": {"state": "complete", "reindexRecommended": False},
    "pendingChanges": {"added": 0, "modified": 0, "deleted": 0},
}


def test_codegraph_not_initialized(tmp_path, monkeypatch):
    config = {"status": {"initialized": False}}
    project = codegraph_project(tmp_path, config, monkeypatch)
    checks = codegraph.diagnose(project)
    assert checks[0].status == NOT_READY
    assert checks[0].next_action == f"codegraph init {project.root}"


def test_codegraph_complete_index_is_healthy(tmp_path, monkeypatch):
    project = codegraph_project(tmp_path, {"status": COMPLETE_STATUS}, monkeypatch)
    checks = codegraph.diagnose(project, deep=False)
    assert len(checks) == 1
    assert checks[0].component == "codegraph.index"
    assert checks[0].status == HEALTHY


def test_codegraph_pending_changes_is_degraded(tmp_path, monkeypatch):
    status = dict(COMPLETE_STATUS, pendingChanges={"added": 1, "modified": 0, "deleted": 0})
    project = codegraph_project(tmp_path, {"status": status}, monkeypatch)
    checks = codegraph.diagnose(project, deep=False)
    assert checks[0].component == "codegraph.index"
    assert checks[0].status == DEGRADED
    assert "unsynced changes" in checks[0].reason


def test_codegraph_deep_smoke_symbol_found_is_healthy(tmp_path, monkeypatch):
    config = {
        "status": COMPLETE_STATUS,
        "query": [{"node": {"name": "kickoff", "filePath": "src/orai/runtime.py"}}],
    }
    project = codegraph_project(tmp_path, config, monkeypatch)
    checks = codegraph.diagnose(project, deep=True)
    query_check = next(c for c in checks if c.component == "codegraph.query")
    assert query_check.status == HEALTHY
    assert "kickoff found in src/orai/runtime.py" in query_check.reason


def test_codegraph_deep_smoke_symbol_not_found_is_degraded(tmp_path, monkeypatch):
    config = {"status": COMPLETE_STATUS, "query": []}
    project = codegraph_project(tmp_path, config, monkeypatch)
    checks = codegraph.diagnose(project, deep=True)
    query_check = next(c for c in checks if c.component == "codegraph.query")
    assert query_check.status == DEGRADED
    assert "not returned" in query_check.reason


def test_smoke_hit_matches_real_qmd_result_formats(monkeypatch, tmp_path):
    """QMD 2.8.3 returns `docs/<path>` (observed 2026-09-25); older output used `qmd://docs/<path>`.

    Literal values on purpose: deriving the hit from expected_uri() hid a real format mismatch.
    """
    settings = qmd_settings(tmp_path, smoke=True)
    monkeypatch.setattr(qmd, "reachability", lambda port: ("open", None))
    assert qmd.expected_uri(settings) == "docs/found.md"
    for returned in ("docs/found.md", "qmd://docs/FOUND.md"):

        def query(_arguments, returned=returned):
            return {"structuredContent": {"results": [{"file": returned}]}}

        client_cls, _calls = make_client(
            status=status_result(settings), query=query, get={"content": [{"text": "body"}]}
        )
        monkeypatch.setattr(qmd, "Client", client_cls)
        vector = next(c for c in qmd.probe(settings, deep=True) if c.component == "wiki.vector")
        assert vector.status == HEALTHY, returned
