"""`orai init`: plan/apply, conflicts, backups, presets and template/loader drift."""

from importlib import resources
import json
import os
from pathlib import Path
import stat
import tomllib

import pytest
from support import TWO_ROLES

from orai import cli, scaffold
from orai import config as config_module
from orai.project import CONFIG_NAME, STATE_NAME


def snapshot(root):
    """Record every path under root (kind + content/target) to detect any write."""
    root = Path(root)
    entries = {}
    for path in sorted(root.rglob("*")):
        rel = str(path.relative_to(root))
        if path.is_symlink():
            entries[rel] = ("symlink", os.readlink(path))
        elif path.is_dir():
            entries[rel] = ("dir", None)
        else:
            entries[rel] = ("file", path.read_bytes())
    return entries


def backup_files(root):
    backups_dir = Path(root) / STATE_NAME / "backups"
    stamps = sorted(backups_dir.iterdir())
    assert len(stamps) == 1
    return stamps[0]


# --- plan()/apply() basics -----------------------------------------------------------


def test_plan_preview_writes_nothing(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    before = snapshot(root)
    actions, _notes = scaffold.plan(root)
    assert actions
    assert snapshot(root) == before


def test_apply_then_plan_is_idempotent(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    actions, _notes = scaffold.plan(root)
    scaffold.apply(root, actions)
    actions2, _notes2 = scaffold.plan(root)
    assert actions2 == []


def test_cli_init_twice_prints_already_current(tmp_path, capsys):
    root = tmp_path / "proj"
    root.mkdir()

    rc1 = cli.main(["setup", "--project", str(root), "--no-tools"])
    out1 = capsys.readouterr().out
    assert rc1 == 0
    assert "Files already current." not in out1

    rc2 = cli.main(["setup", "--project", str(root), "--no-tools"])
    out2 = capsys.readouterr().out
    assert rc2 == 0
    assert "Files already current." in out2


def test_state_dir_and_project_json_modes(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    actions, _notes = scaffold.plan(root)
    scaffold.apply(root, actions)
    state_dir = root / STATE_NAME
    project_json = state_dir / "project.json"
    assert stat.S_IMODE(state_dir.stat().st_mode) == 0o700
    assert stat.S_IMODE(project_json.stat().st_mode) == 0o600


# --- AGENTS.md managed block -----------------------------------------------------------


def test_agents_md_preserves_existing_content_and_backs_up(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    original = "# My Project\n\nSome custom notes.\n"
    (root / "AGENTS.md").write_text(original)

    actions, _notes = scaffold.plan(root)
    scaffold.apply(root, actions)

    updated = (root / "AGENTS.md").read_text()
    assert updated.startswith(original)
    begin, end = scaffold.MARKERS["md"]
    assert updated.count(begin) == 1
    assert updated.count(end) == 1

    backup = backup_files(root) / "AGENTS.md"
    assert backup.read_text() == original


def test_agents_md_idempotent_then_replaces_block_on_template_change(tmp_path, monkeypatch):
    root = tmp_path / "proj"
    root.mkdir()
    original = "# My Project\n\nSome custom notes.\n"
    (root / "AGENTS.md").write_text(original)

    actions, _notes = scaffold.plan(root)
    scaffold.apply(root, actions)

    # Re-applying with no template change touches nothing.
    actions2, _notes2 = scaffold.plan(root)
    assert not any(a.path == root / "AGENTS.md" for a in actions2)

    real_template = scaffold.template

    def changed_template(name, _orig=real_template):
        if name == "agents-block.md":
            return "A brand new Orai block body.\n"
        return _orig(name)

    monkeypatch.setattr(scaffold, "template", changed_template)
    actions3, _notes3 = scaffold.plan(root)
    scaffold.apply(root, actions3)

    final = (root / "AGENTS.md").read_text()
    begin, end = scaffold.MARKERS["md"]
    assert final.startswith(original)
    assert final.count(begin) == 1
    assert final.count(end) == 1
    assert "A brand new Orai block body." in final


# --- CLAUDE.md ----------------------------------------------------------------------


def test_claude_md_created_when_absent(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    actions, _notes = scaffold.plan(root)
    matches = [a for a in actions if a.path == root / "CLAUDE.md"]
    assert len(matches) == 1
    assert matches[0].data == b"@AGENTS.md\n"
    scaffold.apply(root, actions)
    assert (root / "CLAUDE.md").read_text() == "@AGENTS.md\n"


def test_claude_md_present_without_reference_gets_note_only(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    before = "# My CLAUDE notes\n"
    (root / "CLAUDE.md").write_text(before)

    actions, notes = scaffold.plan(root)
    assert not any(a.path == root / "CLAUDE.md" for a in actions)
    assert any("AGENTS.md" in note for note in notes)

    scaffold.apply(root, actions)
    assert (root / "CLAUDE.md").read_text() == before


# --- .gitignore and presets -----------------------------------------------------------


def test_gitignore_minimal_contains_state_dir_only(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    actions, _notes = scaffold.plan(root)
    scaffold.apply(root, actions)
    text = (root / ".gitignore").read_text()
    assert "/.orai/" in text
    assert "/.worktrees/staff/" not in text


def test_gitignore_pm_staff_contains_worktree_and_guides_created(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    actions, _notes = scaffold.plan(root, preset="pm-staff")
    scaffold.apply(root, actions)
    text = (root / ".gitignore").read_text()
    assert "/.orai/" in text
    assert "/.worktrees/staff/" in text
    assert (root / ".agents/roles/pm.md").is_file()
    assert (root / ".agents/roles/staff.md").is_file()


def test_existing_guide_files_are_never_overwritten(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / ".agents" / "roles").mkdir(parents=True)
    custom = "Custom PM guide, do not touch.\n"
    (root / ".agents/roles/pm.md").write_text(custom)

    actions, _notes = scaffold.plan(root, preset="pm-staff")
    scaffold.apply(root, actions)

    assert (root / ".agents/roles/pm.md").read_text() == custom
    assert (root / ".agents/roles/staff.md").is_file()


# --- generated SKILL.md ---------------------------------------------------------------


def test_generated_skill_outdated_is_updated_with_backup(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    wanted = scaffold.template("skill.md")
    outdated = wanted + "\n<!-- stale extra content -->\n"
    skill_path = root / scaffold.SKILL
    skill_path.parent.mkdir(parents=True)
    skill_path.write_text(outdated)

    actions, _notes = scaffold.plan(root)
    matches = [a for a in actions if a.path == skill_path]
    assert len(matches) == 1
    assert matches[0].backup is True

    scaffold.apply(root, actions)
    assert skill_path.read_text() == wanted
    backup = backup_files(root) / scaffold.SKILL
    assert backup.read_text() == outdated


# --- conflicts: each raises Conflict and writes nothing ------------------------------


def test_conflict_skill_not_generated(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text(TWO_ROLES)
    skill_path = root / scaffold.SKILL
    skill_path.parent.mkdir(parents=True)
    skill_path.write_text("# My own skill\n")

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    assert "was not generated by Orai" in str(excinfo.value)
    assert snapshot(root) == before


def test_conflict_claude_skill_link_wrong_target(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text(TWO_ROLES)
    link_path = root / scaffold.CLAUDE_SKILL_LINK
    link_path.parent.mkdir(parents=True)
    os.symlink("../../somewhere-else", link_path)

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    assert "points to" in str(excinfo.value)
    assert snapshot(root) == before


def test_conflict_agents_md_two_begin_markers(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text(TWO_ROLES)
    begin, end = scaffold.MARKERS["md"]
    (root / "AGENTS.md").write_text(f"{begin}\nfirst\n{end}\n{begin}\nsecond\n{end}\n")

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    assert "unbalanced or duplicated" in str(excinfo.value)
    assert snapshot(root) == before


def test_conflict_agents_md_begin_without_end(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text(TWO_ROLES)
    begin, _end = scaffold.MARKERS["md"]
    (root / "AGENTS.md").write_text(f"pre\n{begin}\nbody\n")

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    assert "unbalanced or duplicated" in str(excinfo.value)
    assert snapshot(root) == before


def test_conflict_invalid_existing_config(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text("schema = 2\n")

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    assert "orai.toml is invalid" in str(excinfo.value)
    assert snapshot(root) == before


def test_conflict_agents_md_is_symlink(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text(TWO_ROLES)
    (root / "elsewhere.md").write_text("not agents\n")
    os.symlink("elsewhere.md", root / "AGENTS.md")

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    assert "is a symlink" in str(excinfo.value)
    assert snapshot(root) == before


def test_conflicts_are_all_reported_together(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    (root / CONFIG_NAME).write_text("schema = 2\n")

    skill_path = root / scaffold.SKILL
    skill_path.parent.mkdir(parents=True)
    skill_path.write_text("# My own skill\n")

    link_path = root / scaffold.CLAUDE_SKILL_LINK
    link_path.parent.mkdir(parents=True)
    os.symlink("../../somewhere-else", link_path)

    begin, end = scaffold.MARKERS["md"]
    (root / "AGENTS.md").write_text(f"{begin}\nfirst\n{end}\n{begin}\nsecond\n{end}\n")

    before = snapshot(root)
    with pytest.raises(scaffold.Conflict) as excinfo:
        scaffold.plan(root)
    message = str(excinfo.value)
    assert "orai.toml is invalid" in message
    assert "was not generated by Orai" in message
    assert "points to" in message
    assert "unbalanced or duplicated" in message
    assert snapshot(root) == before


# --- project moved (re-bind) -----------------------------------------------------------


def test_plan_rebind_action_and_note_when_project_moved(tmp_path):
    root = tmp_path / "proj"
    root.mkdir()
    actions, _notes = scaffold.plan(root)
    scaffold.apply(root, actions)

    state_path = root / STATE_NAME / "project.json"
    recorded = json.loads(state_path.read_text())
    recorded["root"] = str(tmp_path / "elsewhere")
    state_path.write_text(json.dumps(recorded))

    actions2, notes2 = scaffold.plan(root)
    rebinds = [a for a in actions2 if a.description.startswith("Re-bind")]
    assert len(rebinds) == 1
    assert any("--fresh" in note for note in notes2)


# --- template/loader drift -------------------------------------------------------------


def test_template_files_exist_in_package_resources():
    names = {"skill.md", "agents-block.md"}
    for mapping in scaffold.PRESETS.values():
        names.update(mapping.values())
    templates_dir = resources.files("orai") / "templates"
    missing = [name for name in sorted(names) if not (templates_dir / name).is_file()]
    assert missing == []


def test_preset_config_templates_parse():
    for mapping in scaffold.PRESETS.values():
        text = scaffold.template(mapping[CONFIG_NAME])
        cfg = config_module.parse(tomllib.loads(text))
        assert isinstance(cfg, config_module.Config)


def test_skill_template_has_frontmatter_and_generated_marker():
    text = scaffold.template("skill.md")
    assert text.startswith("---\nname: orai")
    assert scaffold.GENERATED in text


def test_this_repository_is_setup_current():
    """Orai dogfoods `orai init`: a template change must be followed by `orai init --apply` here."""
    repo = Path(__file__).resolve().parents[1]
    actions, _ = scaffold.plan(repo)
    pending = [a.description for a in actions if scaffold.STATE_NAME not in a.path.relative_to(repo).parts]
    assert pending == []


def test_apply_reports_only_actions_that_succeeded(tmp_path):
    """Review finding: `init --apply` printed every action as applied before running them."""
    ok = scaffold.Action("write a", tmp_path / "a.txt", b"a")
    failing = scaffold.Action("run tool", tmp_path / ".amqrc", command=["false"])
    never = scaffold.Action("write b", tmp_path / "b.txt", b"b")
    done = []
    with pytest.raises(RuntimeError, match="false failed"):
        scaffold.apply(tmp_path, [ok, failing, never], done=lambda action: done.append(action.description))
    assert done == ["write a"]
    assert not (tmp_path / "b.txt").exists()
