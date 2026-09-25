"""Config validation (schema, roles, integrations) and project identity/location."""

import shutil
import subprocess
import tomllib

import pytest
from support import write_project

from orai import config as config_module
from orai import project as project_module
from orai.state import write_json

BASE = """
schema = 1
session = "orai"
"""


def parse(text):
    return config_module.parse(tomllib.loads(text))


def git(cwd, *args):
    subprocess.run(["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True)


def commit(cwd, message):
    git(cwd, "-c", "user.name=t", "-c", "user.email=t@t.com", "commit", "-q", "-m", message)


# --- config: roles -----------------------------------------------------------------


def test_arbitrary_role_names_and_counts():
    cfg = parse(
        BASE
        + """
        [roles.lead]
        provider = "codex"

        [roles."reviewer-2"]
        provider = "claude"

        [roles.qa]
        provider = "codex"
        """
    )
    assert set(cfg.roles) == {"lead", "reviewer-2", "qa"}
    assert cfg.handles() == ["lead", "qa", "reviewer-2", "user"]


@pytest.mark.parametrize(
    "name", ["init", "run", "user", "status", "all", "orai", "doctor", "qmd", "setup", "msg", "wiki"]
)
def test_reserved_role_names_rejected(name):
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(BASE + f'\n[roles.{name}]\nprovider = "codex"\n')
    assert "reserved" in str(excinfo.value)


@pytest.mark.parametrize(
    "name",
    [
        "Reviewer",  # uppercase
        "1abc",  # starts with a digit
        "role_name",  # underscore not allowed
        "a" * 32,  # exceeds the 31 character limit
    ],
)
def test_bad_regex_role_names_rejected(name):
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(BASE + f'\n[roles."{name}"]\nprovider = "codex"\n')
    assert "role names must match" in str(excinfo.value)


def test_unknown_top_level_key_rejected():
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(BASE + "\nfoo = 1\n")
    assert "unknown key(s) foo" in str(excinfo.value)


def test_unknown_role_key_rejected():
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + """
            [roles.lead]
            provider = "codex"
            nickname = "boss"
            """
        )
    assert "unknown key(s) nickname" in str(excinfo.value)


def test_unknown_integrations_key_rejected():
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(BASE + "\n[integrations.unknown]\n")
    assert "unknown key(s) unknown" in str(excinfo.value)


@pytest.mark.parametrize("schema_line", ["schema = 2\n", ""])
def test_wrong_schema_rejected(schema_line):
    text = f'{schema_line}session = "orai"\n'
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(text)
    assert "schema must be 1" in str(excinfo.value)


def test_bad_provider_rejected():
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + """
            [roles.lead]
            provider = "chatgpt"
            """
        )
    assert "provider must be one of" in str(excinfo.value)


# --- config: relative-path enforcement (guide / worktree / collections) ------------


@pytest.mark.parametrize(
    "guide",
    ["/etc/passwd", "~/secrets.md", "../outside.md"],
)
def test_guide_path_escapes_rejected(guide):
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + f"""
            [roles.lead]
            provider = "codex"
            guide = "{guide}"
            """
        )
    assert "roles.lead.guide" in str(excinfo.value)


@pytest.mark.parametrize(
    "path",
    ["/abs/docs", "~/docs", "../docs"],
)
def test_collection_path_escapes_rejected(path):
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + f"""
            [integrations.wiki]
            collections = {{ docs = "{path}" }}
            """
        )
    assert "integrations.wiki.collections.docs" in str(excinfo.value)


def test_worktree_dotdot_is_allowed():
    cfg = parse(
        BASE
        + """
        [roles.staff]
        provider = "claude"
        worktree = "../sibling"
        """
    )
    assert cfg.roles["staff"].worktree == "../sibling"


@pytest.mark.parametrize("worktree", ["/abs/worktree", "~/worktree"])
def test_worktree_absolute_or_tilde_rejected(worktree):
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + f"""
            [roles.staff]
            provider = "claude"
            worktree = "{worktree}"
            """
        )
    assert "roles.staff.worktree" in str(excinfo.value)


# --- config: qmd integration --------------------------------------------------------


@pytest.mark.parametrize("port", [1023, 65536, "true"])
def test_qmd_port_bounds_and_bool_rejected(port):
    port_literal = "true" if port == "true" else str(port)
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + f"""
            [integrations.wiki]
            port = {port_literal}
            """
        )
    assert "integrations.wiki.port" in str(excinfo.value)


@pytest.mark.parametrize("port", [1024, 65535])
def test_qmd_port_bounds_accepted(port):
    cfg = parse(
        BASE
        + f"""
        [integrations.wiki]
        port = {port}
        """
    )
    assert cfg.wiki.port == port


def test_qmd_collections_default_when_omitted():
    cfg = parse(BASE + "\n[integrations.wiki]\n")
    assert cfg.wiki.collections == {"docs": "docs"}


def test_qmd_smoke_requires_lex_vec_expect():
    with pytest.raises(config_module.ConfigError) as excinfo:
        parse(
            BASE
            + """
            [integrations.wiki.smoke]
            lex = "unique term"
            vec = "a question"
            """
        )
    assert "integrations.wiki.smoke.expect" in str(excinfo.value)


def test_qmd_smoke_with_all_fields_accepted():
    cfg = parse(
        BASE
        + """
        [integrations.wiki.smoke]
        lex = "unique term"
        vec = "a question"
        expect = "docs/architecture.md"
        """
    )
    assert cfg.wiki.smoke == config_module.Smoke(lex="unique term", vec="a question", expect="docs/architecture.md")


# --- locate() ------------------------------------------------------------------------


def test_locate_from_nested_subdirectory_finds_root(tmp_path):
    root = tmp_path / "proj"
    write_project(root)
    nested = root / "a" / "b" / "c"
    nested.mkdir(parents=True)
    assert project_module.locate(nested) == root.resolve()


def test_locate_raises_when_no_config_anywhere(tmp_path):
    # tmp_path lives under the system temp directory, well outside this checkout and any
    # other repo, so no orai.toml can be found by walking up from it.
    empty = tmp_path / "empty"
    empty.mkdir()
    with pytest.raises(project_module.ProjectNotFound):
        project_module.locate(empty)


def test_locate_non_git_project_works(tmp_path):
    root = tmp_path / "proj"
    write_project(root, git=False)
    assert project_module.locate(root) == root.resolve()


def test_locate_from_worktree_returns_main_checkout_when_main_has_config(tmp_path):
    main_root = tmp_path / "repo"
    write_project(main_root, git=True)
    git(main_root, "add", "orai.toml")
    commit(main_root, "add config")

    worktree = tmp_path / "repo-role"
    git(main_root, "worktree", "add", "-q", "-b", "feature", str(worktree), "trunk")
    assert (worktree / "orai.toml").is_file()

    assert project_module.locate(worktree) == main_root.resolve()


def test_locate_from_worktree_is_root_when_main_lacks_config(tmp_path):
    main_root = tmp_path / "repo2"
    main_root.mkdir()
    git(main_root, "init", "-q", "-b", "trunk", str(main_root))
    (main_root / "README.md").write_text("placeholder\n")
    git(main_root, "add", "README.md")
    commit(main_root, "init")

    worktree = tmp_path / "repo2-role"
    git(main_root, "worktree", "add", "-q", "-b", "feature", str(worktree), "trunk")
    (worktree / "orai.toml").write_text('schema = 1\nsession = "orai"\n')
    git(worktree, "add", "orai.toml")
    commit(worktree, "add config only on feature")

    assert not (main_root / "orai.toml").exists()
    assert project_module.locate(worktree) == worktree.resolve()


# --- Project.id -----------------------------------------------------------------------


def test_project_id_is_stable_for_same_path(tmp_path):
    root = tmp_path / "proj"
    project = write_project(root)
    again = project_module.load(root)
    assert again.id == project.id


def test_project_id_differs_for_same_folder_name_in_different_parents(tmp_path):
    first = write_project(tmp_path / "a" / "myproj")
    second = write_project(tmp_path / "b" / "myproj")
    assert first.id != second.id


def test_project_id_from_worktree_matches_main(tmp_path):
    main_root = tmp_path / "repo"
    write_project(main_root, git=True)
    git(main_root, "add", "orai.toml")
    commit(main_root, "add config")

    worktree = tmp_path / "repo-role"
    git(main_root, "worktree", "add", "-q", "-b", "feature", str(worktree), "trunk")

    main_project = project_module.load(main_root)
    worktree_project = project_module.load(worktree)
    assert worktree_project.root == main_root.resolve()
    assert worktree_project.id == main_project.id


def test_project_id_and_recorded_root_after_copy(tmp_path):
    original_root = tmp_path / "original"
    write_project(original_root)
    write_json(original_root / ".orai" / "project.json", {"root": str(original_root)})

    original = project_module.load(original_root)
    assert original.recorded_root() is None

    copy_root = tmp_path / "copy"
    shutil.copytree(original_root, copy_root)

    copied = project_module.load(copy_root)
    assert copied.id != original.id
    assert copied.recorded_root() == str(original_root)


def test_monorepo_subproject_is_not_taken_over_by_the_repo_root(tmp_path):
    """Review finding: any directory of a repo whose main checkout had orai.toml resolved to the root."""
    mono = tmp_path / "mono"
    write_project(mono, git=True)
    sub = mono / "sub"
    write_project(sub)
    (sub / "deeper").mkdir()
    assert project_module.locate(sub) == sub.resolve()
    assert project_module.locate(sub / "deeper") == sub.resolve()
    assert project_module.locate(mono) == mono.resolve()


def test_subproject_in_a_linked_worktree_maps_to_the_same_subproject_in_main(tmp_path):
    main = tmp_path / "repo"
    write_project(main, git=True)
    write_project(main / "app")
    git(main, "add", "-A")
    commit(main, "init")
    worktree = tmp_path / "repo-dev"
    git(main, "worktree", "add", "-q", "-b", "dev", str(worktree))
    assert project_module.locate(worktree / "app") == (main / "app").resolve()
    assert project_module.locate(worktree) == main.resolve()
