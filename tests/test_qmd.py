"""QMD lifecycle safety: init/recover/refresh/check/stop never touch real QMD, models or
daemons. Every `qmd` invocation goes through a patched `qmd.run`; reachability, identity
and the MCP client are patched too. No test binds or connects to a real QMD server, AMQ, or
port 8181.
"""

import json

import pytest
from support import write_project

from orai.doctor import BLOCKED, Check
from orai.integrations import mcp_servers, qmd


def make_qmd_project(root, port=None):
    text = 'schema = 1\nsession = "orai"\n\n[integrations.wiki]\n'
    if port is not None:
        text += f"port = {port}\n"
    project = write_project(root, text=text)
    (project.root / "docs").mkdir(parents=True, exist_ok=True)
    return project


def install_success_mocks(monkeypatch, running=False, verify_result=0, binary="/usr/bin/qmd"):
    """Patch every side-effecting collaborator lifecycle() uses so it runs hermetically."""
    calls = []
    monkeypatch.setattr(qmd.shutil, "which", lambda _name: binary)
    monkeypatch.setattr(qmd, "own_server_running", lambda _settings: running)
    monkeypatch.setattr(qmd, "reachability", lambda _port: ("open", None))
    monkeypatch.setattr(qmd, "verify", lambda _settings: verify_result)

    def fake_run(argv, settings, capture=False):
        calls.append(list(argv))
        if capture:
            name = argv[-1]
            return f"Path: {settings.collections[name]}\n"
        return ""

    monkeypatch.setattr(qmd, "run", fake_run)
    return calls


# ---- settings ------------------------------------------------------------------------------


def test_settings_env_points_inside_dot_orai_wiki(tmp_path):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    env = settings.env()
    assert env["QMD_CONFIG_DIR"] == str(settings.directory)
    assert env["INDEX_PATH"] == str(settings.db)
    assert settings.directory == project.root / ".orai" / "wiki"
    assert settings.db.parent == settings.directory


# ---- init -----------------------------------------------------------------------------------


def test_init_creates_config_with_absolute_collections_and_default_model(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    install_success_mocks(monkeypatch, running=False)

    assert qmd.lifecycle(project, "init") == 0

    written = json.loads(settings.config_file.read_text())
    assert written["collections"]["docs"]["path"] == str(settings.collections["docs"])
    assert settings.collections["docs"].is_absolute()
    assert written["models"]["embed"] == qmd.MODEL


def test_init_runs_show_then_update_then_embed_then_daemon_with_index_prefixed_argv(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    calls = install_success_mocks(monkeypatch, running=False)

    assert qmd.lifecycle(project, "init") == 0

    for call in calls:
        assert call[:3] == ["/usr/bin/qmd", "--index", settings.index]
        assert settings.index == f"orai-{project.id}"

    subcommands = [call[3] for call in calls]
    assert subcommands == ["collection", "update", "embed", "mcp"]

    daemon_call = calls[-1]
    assert daemon_call[daemon_call.index("--host") + 1] == qmd.HOST
    assert daemon_call[daemon_call.index("--port") + 1] == str(settings.port)
    assert settings.port != 8181
    assert "8181" not in daemon_call


def test_init_starts_daemon_on_configured_8181_when_explicitly_set(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj", port=8181)
    settings = qmd.Settings(project)
    assert settings.port == 8181
    calls = install_success_mocks(monkeypatch, running=False)

    assert qmd.lifecycle(project, "init") == 0

    daemon_call = calls[-1]
    assert daemon_call[3] == "mcp"
    assert daemon_call[daemon_call.index("--port") + 1] == "8181"


# ---- recover --------------------------------------------------------------------------------


def test_recover_preserves_config_and_db_byte_for_byte_and_skips_update_and_embed(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    settings.directory.mkdir(parents=True)
    original_config = (
        json.dumps(
            {"collections": {"docs": {"path": str(settings.collections["docs"]), "pattern": "**/*.md"}}},
            indent=2,
        )
        + "\n"
    )
    settings.config_file.write_text(original_config)
    original_db = b"\x00\x01not-a-real-sqlite-file"
    settings.db.write_bytes(original_db)

    calls = install_success_mocks(monkeypatch, running=False)
    assert qmd.lifecycle(project, "recover") == 0

    assert settings.config_file.read_text() == original_config
    assert settings.db.read_bytes() == original_db
    subcommands = [call[3] for call in calls]
    assert "update" not in subcommands
    assert "embed" not in subcommands
    # Not running yet: recover still brings the server up.
    assert subcommands[-1] == "mcp"


def test_recover_with_our_server_already_running_starts_nothing(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    settings.directory.mkdir(parents=True)
    settings.config_file.write_text(json.dumps(settings.config_value()) + "\n")
    settings.db.write_bytes(b"db-bytes")

    calls = install_success_mocks(monkeypatch, running=True)
    assert qmd.lifecycle(project, "recover") == 0

    subcommands = [call[3] for call in calls]
    assert "mcp" not in subcommands
    assert "update" not in subcommands
    assert "embed" not in subcommands


def test_recover_with_missing_config_or_db_raises_pointing_to_init(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    monkeypatch.setattr(qmd.shutil, "which", lambda _name: "/usr/bin/qmd")
    with pytest.raises(RuntimeError, match="orai wiki init"):
        qmd.lifecycle(project, "recover")


# ---- refresh/init refuse while our server is running ---------------------------------------


def test_init_refuses_while_server_running_and_runs_no_qmd_commands(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    calls = install_success_mocks(monkeypatch, running=True)
    with pytest.raises(RuntimeError, match="safe boundary"):
        qmd.lifecycle(project, "init")
    assert calls == []


def test_refresh_refuses_while_server_running_and_runs_no_qmd_commands(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    settings.directory.mkdir(parents=True)
    settings.config_file.write_text(json.dumps(settings.config_value()) + "\n")
    settings.db.write_bytes(b"db-bytes")

    calls = install_success_mocks(monkeypatch, running=True)
    with pytest.raises(RuntimeError, match="safe boundary"):
        qmd.lifecycle(project, "refresh")
    assert calls == []


# ---- a port serving another project's index is left untouched ------------------------------


def test_port_serving_another_index_raises_and_runs_no_qmd_commands(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    settings.directory.mkdir(parents=True)
    settings.config_file.write_text(json.dumps(settings.config_value()) + "\n")
    settings.db.write_bytes(b"db-bytes")

    monkeypatch.setattr(qmd.shutil, "which", lambda _name: "/usr/bin/qmd")
    monkeypatch.setattr(qmd, "reachability", lambda _port: ("open", None))

    class OtherProjectClient:
        def __init__(self, endpoint, timeout=15):
            pass

        def connect(self):
            pass

        def tool(self, _name, _arguments=None):
            return {"structuredContent": {"collections": [{"name": "docs", "path": "/elsewhere"}]}}

    monkeypatch.setattr(qmd, "Client", OtherProjectClient)
    calls = []
    monkeypatch.setattr(qmd, "run", lambda argv, _settings, capture=False: calls.append(list(argv)) or "")

    with pytest.raises(RuntimeError, match="Left untouched"):
        qmd.lifecycle(project, "recover")
    assert calls == []


# ---- verify() never reports a failed probe as success ---------------------------------------


def test_verify_raises_when_any_check_is_not_healthy(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    monkeypatch.setattr(
        qmd, "probe", lambda _settings, deep=True: [Check("wiki.server", BLOCKED, "connection refused")]
    )
    with pytest.raises(RuntimeError, match="not verified"):
        qmd.verify(settings)


# ---- stop -------------------------------------------------------------------------------------


def test_stop_runs_only_mcp_stop(tmp_path, monkeypatch):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    calls = install_success_mocks(monkeypatch)

    assert qmd.lifecycle(project, "stop") == 0
    assert calls == [["/usr/bin/qmd", "--index", settings.index, "mcp", "stop"]]


# ---- two projects never collide -------------------------------------------------------------


def test_two_projects_get_distinct_index_names_and_pid_files(tmp_path, monkeypatch):
    cache = tmp_path / "cache"
    monkeypatch.setenv("XDG_CACHE_HOME", str(cache))
    project_a = make_qmd_project(tmp_path / "a")
    project_b = make_qmd_project(tmp_path / "b")
    settings_a = qmd.Settings(project_a)
    settings_b = qmd.Settings(project_b)

    assert settings_a.index != settings_b.index
    assert settings_a.index.startswith("orai-")
    assert settings_b.index.startswith("orai-")
    assert settings_a.pid_file != settings_b.pid_file
    assert settings_a.pid_file == cache / "qmd" / f"mcp-{settings_a.index}.pid"
    assert settings_b.pid_file == cache / "qmd" / f"mcp-{settings_b.index}.pid"


# ---- mcp_servers() ----------------------------------------------------------------------------


def test_mcp_servers_returns_http_entry_for_claude_and_url_entry_for_codex(tmp_path):
    project = make_qmd_project(tmp_path / "proj")
    settings = qmd.Settings(project)
    assert settings.server_name.startswith("wiki-")

    claude_servers = mcp_servers(project, "claude")
    assert claude_servers == {settings.server_name: {"type": "http", "url": settings.endpoint}}

    codex_servers = mcp_servers(project, "codex")
    assert codex_servers[settings.server_name]["url"] == settings.endpoint
    assert "startup_timeout_sec" in codex_servers[settings.server_name]


def test_mcp_servers_empty_when_qmd_is_not_configured(tmp_path):
    project = write_project(tmp_path / "proj")
    assert mcp_servers(project, "claude") == {}
    assert mcp_servers(project, "codex") == {}
