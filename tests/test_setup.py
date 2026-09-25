"""`orai setup`: from an empty folder to a project, target resolution and tool steps.

Tool steps run against fakes or are skipped; no real QMD server or code graph is touched.
"""

import subprocess

import pytest
from support import TWO_ROLES, fake_binary, write_project

from orai import bootstrap, cli, scaffold
from orai import project as project_module


def git(cwd, *args):
    return subprocess.run(["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True).stdout


def tree(root):
    return sorted(str(p.relative_to(root)) for p in root.rglob("*"))


def test_empty_folder_becomes_a_trunk_repository_with_a_wiki_starter(tmp_path, capsys, monkeypatch):
    monkeypatch.setenv("PATH", "/usr/bin:/bin")  # no amq/qmd/codegraph: tool steps must degrade, not fail
    root = tmp_path / "my-app"
    root.mkdir()
    assert cli.main(["setup", "--project", str(root), "--no-tools"]) == 0
    out = capsys.readouterr().out
    assert "Applied: Initialize a Git repository on branch trunk" in out
    assert git(root, "symbolic-ref", "--short", "HEAD").strip() == "trunk"
    assert (root / "docs/README.md").is_file()
    assert (root / "orai.toml").is_file()
    ignored = (root / ".gitignore").read_text()
    assert "/.orai/" in ignored and "/.codegraph/" in ignored
    assert "does not pin Orai" in out and scaffold.INSTALL_SPEC in out
    assert "Diagnosis:" in out


def test_setup_is_idempotent_and_dry_run_writes_nothing(tmp_path, capsys):
    root = tmp_path / "app"
    root.mkdir()
    before = tree(root)
    assert cli.main(["setup", "--project", str(root), "--dry-run"]) == 0
    assert tree(root) == before
    assert "Would: Initialize a Git repository" in capsys.readouterr().out
    assert cli.main(["setup", "--project", str(root), "--no-tools"]) == 0
    capsys.readouterr()
    assert cli.main(["setup", "--project", str(root), "--dry-run"]) == 0
    assert "Already current." in capsys.readouterr().out


def test_existing_repository_is_not_reinitialized_and_branch_is_configurable(tmp_path):
    root = tmp_path / "repo"
    root.mkdir()
    git(root, "init", "-q", "-b", "develop")
    actions, _ = scaffold.plan(root, branch="trunk")
    assert not any(a.command and a.command[0] == "git" for a in actions)
    fresh = tmp_path / "fresh"
    fresh.mkdir()
    actions, _ = scaffold.plan(fresh, branch="release")
    assert actions[0].command == ["git", "init", "-q", "-b", "release"]


def test_legacy_managed_block_is_replaced_not_duplicated(tmp_path):
    legacy = "keep\n\n<!-- orai:begin (managed by `orai init`; edit outside this block) -->\nold\n<!-- orai:end -->\n"
    updated = scaffold.managed_block(legacy, "new", "md")
    assert updated.count("orai:begin") == 1
    assert "old" not in updated and "new" in updated and updated.startswith("keep\n")


def test_setup_target_prefers_this_repository_over_configs_above_it(tmp_path):
    outer = write_project(tmp_path / "outer")  # someone else's project above
    repo = outer.root / "apps/new"
    repo.mkdir(parents=True)
    git(repo, "init", "-q", "-b", "trunk")
    (repo / "src").mkdir()
    assert project_module.setup_target(repo / "src") == repo.resolve()
    plain = outer.root / "plain"
    plain.mkdir()
    assert project_module.setup_target(plain) == plain.resolve()
    existing = write_project(repo)  # once set up, subdirectories resolve to it
    assert project_module.setup_target(repo / "src") == existing.root


def test_wiki_step_skips_without_engine_or_documents(tmp_path, monkeypatch):
    project = write_project(tmp_path / "p", TWO_ROLES + "\n[integrations.wiki]\n")
    monkeypatch.setattr(bootstrap.shutil, "which", lambda name: None)
    assert "not installed" in bootstrap.wiki_step(project)
    monkeypatch.setattr(bootstrap.shutil, "which", lambda name: "/fake/" + name)
    (project.root / "docs").mkdir()
    assert "no Markdown documents" in bootstrap.wiki_step(project)


@pytest.mark.parametrize("existing, expected", [(False, "init"), (True, "recover")])
def test_wiki_step_initializes_once_then_recovers(tmp_path, monkeypatch, existing, expected):
    project = write_project(tmp_path / "p", TWO_ROLES + "\n[integrations.wiki]\n")
    (project.root / "docs").mkdir()
    (project.root / "docs/a.md").write_text("# A\n")
    monkeypatch.setattr(bootstrap.shutil, "which", lambda name: "/fake/" + name)
    if existing:
        settings = bootstrap.qmd.Settings(project)
        settings.directory.mkdir(parents=True)
        settings.config_file.write_text("{}")
        settings.db.write_bytes(b"db")
    calls = []
    monkeypatch.setattr(bootstrap.qmd, "lifecycle", lambda _project, action: calls.append(action) or 0)
    assert bootstrap.wiki_step(project) == f"wiki: ready ({expected})"
    assert calls == [expected]
    monkeypatch.setattr(bootstrap.qmd, "lifecycle", lambda *_: (_ for _ in ()).throw(RuntimeError("port busy")))
    assert "failed: port busy" in bootstrap.wiki_step(project)


def test_codegraph_step_indexes_once(tmp_path, monkeypatch):
    project = write_project(tmp_path / "p", TWO_ROLES + "\n[integrations.codegraph]\n")
    fakes = tmp_path / "bin"
    fakes.mkdir()
    fake_binary(
        fakes,
        "codegraph",
        """
        import pathlib, sys
        assert sys.argv[1] == "init"
        pathlib.Path(sys.argv[2], ".codegraph").mkdir()
    """,
    )
    monkeypatch.setenv("PATH", str(fakes))
    assert bootstrap.codegraph_step(project) == "codegraph: indexed"
    assert bootstrap.codegraph_step(project) == "codegraph: already indexed"


def test_a_failed_tool_step_makes_setup_exit_nonzero(tmp_path, monkeypatch, capsys):
    root = tmp_path / "app"
    root.mkdir()
    monkeypatch.setattr(bootstrap, "wiki_step", lambda project: "wiki: init failed: boom")
    monkeypatch.setattr(bootstrap, "codegraph_step", lambda project: "codegraph: indexed")
    assert cli.main(["setup", "--project", str(root)]) == 1
    assert "wiki: init failed: boom" in capsys.readouterr().out


def test_messages_live_under_msg_and_wiki_replaces_qmd(capsys):
    assert cli.normalize(["msg", "inbox"]) == ["msg", "inbox"]
    assert cli.normalize(["wiki", "check"]) == ["wiki", "check"]
    for retired, replacement in (
        (["inbox"], "orai msg inbox"),
        (["qmd", "check"], "orai wiki"),
        (["init"], "orai setup"),
    ):
        assert cli.main(retired) == 2
        assert replacement in capsys.readouterr().err
