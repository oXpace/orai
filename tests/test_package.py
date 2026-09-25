"""Build the wheel, install it into a fresh venv and run it outside the checkout.

Fixture tests import from src/ and cannot see packaging drift (Pockets 72b4088 shipped
files that disagreed with each other). This test only trusts the installed artifact.
Run with `mise run test:package` (pytest -m package).
"""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import zipfile

import pytest
from support import fake_binary

pytestmark = pytest.mark.package
REPO = Path(__file__).resolve().parents[1]
TEMPLATES = REPO / "src/orai/templates"


@pytest.fixture(scope="module")
def installed(tmp_path_factory):
    work = tmp_path_factory.mktemp("package")
    uv = shutil.which("uv")
    assert uv, "uv is required"
    subprocess.run(
        [uv, "build", "--wheel", "--out-dir", str(work / "dist"), str(REPO)], check=True, capture_output=True
    )
    wheel = next((work / "dist").glob("orai-*.whl"))
    subprocess.run([uv, "venv", "-q", "--python", sys.executable, str(work / "venv")], check=True)
    python = work / "venv/bin/python"
    subprocess.run([uv, "pip", "install", "-q", "--python", str(python), str(wheel)], check=True)
    return work, wheel, python


def clean_env(work, extra_path=None):
    env = {
        k: v
        for k, v in os.environ.items()
        if k not in ("VIRTUAL_ENV", "PYTHONPATH") and not k.startswith(("AM_", "AMQ_", "ORAI_"))
    }
    path = [str(work / "venv/bin"), "/usr/bin", "/bin"]
    env["PATH"] = os.pathsep.join(([str(extra_path)] if extra_path else []) + path)
    return env


def orai(work, *argv, extra_path=None):
    return subprocess.run(
        [str(work / "venv/bin/orai"), *argv],
        cwd=work,
        env=clean_env(work, extra_path),
        capture_output=True,
        text=True,
        timeout=60,
    )


def test_wheel_ships_every_template(installed):
    _, wheel, _ = installed
    names = set(zipfile.ZipFile(wheel).namelist())
    for template in TEMPLATES.iterdir():
        assert f"orai/templates/{template.name}" in names


def test_installed_module_is_not_the_checkout(installed):
    work, _, python = installed
    where = subprocess.run(
        [str(python), "-c", "import orai; print(orai.__file__)"],
        cwd=work,
        env=clean_env(work),
        capture_output=True,
        text=True,
        check=True,
    ).stdout
    assert not where.startswith(str(REPO))
    assert orai(work, "--version").stdout.startswith("orai ")


def test_init_twice_is_a_no_op_and_uses_packaged_templates(installed):
    work, _, _ = installed
    project = work / "fixture"
    project.mkdir()
    first = orai(work, "setup", "--project", str(project), "--preset", "pm-staff", "--no-tools")
    assert first.returncode == 0, first.stderr
    assert (project / ".agents/skills/orai/SKILL.md").read_text() == (TEMPLATES / "skill.md").read_text()
    assert (project / "orai.toml").read_text() == (TEMPLATES / "pm-staff.toml").read_text()
    second = orai(work, "setup", "--project", str(project), "--no-tools")
    assert second.returncode == 0, second.stderr
    assert "Files already current." in second.stdout


def test_doctor_and_dry_run_from_outside_the_checkout(installed):
    work, _, python = installed
    project = work / "dry"
    project.mkdir()
    fakes = work / "fakes"
    fakes.mkdir()
    fake_binary(
        fakes,
        "amq",
        f"""
        import json, pathlib, sys
        if sys.argv[1:3] == ["coop", "init"]:
            pathlib.Path(".amqrc").write_text('{{"root": ".agent-mail"}}')
        elif sys.argv[1:3] == ["env", "--json"]:
            print(json.dumps({{"base_root": "{work}/mail"}}))
        elif sys.argv[1] == "env":
            print(json.dumps({{"root": "{work}/mail/orai", "base_root": "{work}/mail"}}))
        else:
            sys.exit("unexpected amq call: " + " ".join(sys.argv[1:]))
    """,
    )
    init = orai(work, "setup", "--project", str(project), "--preset", "pm-staff", "--no-tools", extra_path=fakes)
    assert init.returncode == 0, init.stderr
    assert "Initialize the project AMQ root" in init.stdout
    assert (project / ".amqrc").is_file()
    (project / ".worktrees/staff").mkdir(parents=True)
    result = orai(work, "--project", str(project), "pm", "--dry-run", extra_path=fakes)
    assert result.returncode == 0, result.stderr
    plan = json.loads(result.stdout)
    command = " ".join(plan["command"])
    assert str(REPO) not in command
    assert f"{python} -m orai --project {project} _capture" in command
    doctor = orai(work, "doctor", "--project", str(project), extra_path=fakes)
    report = json.loads(doctor.stdout)
    assert {c["component"] for c in report["checks"]} >= {"project", "tool.amq", "mail", "role.pm", "role.staff"}
    assert doctor.returncode in (1, 3)  # fake tools cannot be healthy; never a silent success


def test_channel_module_starts_from_the_installed_package(installed):
    work, _, python = installed
    env = clean_env(work) | {"ORAI_RUN_NONCE": "n", "ORAI_ROLE": "staff", "ORAI_STATE_DIR": str(work / "state")}
    result = subprocess.run(
        [str(python), "-m", "orai.providers.claude_channel"],
        cwd=work,
        env=env,
        input="",
        capture_output=True,
        text=True,
        timeout=20,
    )
    assert result.returncode == 0, result.stderr
    assert json.loads((work / "state/staff.channel.json").read_text())["ready"] is False
