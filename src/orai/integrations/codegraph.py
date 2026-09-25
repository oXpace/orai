"""CodeGraph (colbymchenry/codegraph) diagnosis. Installing, registering MCP and indexing
are separate user steps; Orai only reports their state and never edits global config."""

import json
import shutil
import subprocess

from orai.doctor import BLOCKED, DEGRADED, HEALTHY, NOT_CONFIGURED, NOT_READY, Check

C = "codegraph"


def _json(argv, cwd):
    result = subprocess.run(argv, cwd=cwd, capture_output=True, text=True, timeout=60)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip() or f"exit {result.returncode}")
    return json.loads(result.stdout)


def diagnose(project, deep=False):
    cfg = project.config.codegraph
    if cfg is None:
        return [Check(C, NOT_CONFIGURED, "no [integrations.codegraph] in orai.toml")]
    binary = shutil.which("codegraph")
    if not binary:
        return [
            Check(
                C,
                BLOCKED,
                "codegraph is not on PATH",
                "Install colbymchenry/codegraph (npm @colbymchenry/codegraph); see docs/compatibility.md",
            )
        ]
    root = str(project.root)
    try:
        status = _json([binary, "status", "--json", root], project.root)
    except (RuntimeError, OSError, ValueError, subprocess.TimeoutExpired) as exc:
        return [Check(C, BLOCKED, f"codegraph status failed: {exc}", None)]
    detail = {k: status.get(k) for k in ("version", "lastIndexed", "fileCount", "nodeCount")}
    if not status.get("initialized"):
        return [Check(C, NOT_READY, "project is not indexed", f"codegraph init {root}", detail=detail)]
    index = status.get("index") or {}
    pending = status.get("pendingChanges") or {}
    if index.get("state") != "complete" or index.get("reindexRecommended"):
        return [
            Check(
                C,
                DEGRADED,
                f"index state {index.get('state')!r}, reindex recommended={index.get('reindexRecommended')}",
                f"codegraph index {root}",
                detail=detail,
            )
        ]
    checks = []
    if any(pending.values()):
        checks.append(
            Check(C + ".index", DEGRADED, f"unsynced changes: {pending}", f"codegraph sync {root}", detail=detail)
        )
    else:
        checks.append(Check(C + ".index", HEALTHY, "index complete and in sync", detail=detail))
    if deep and cfg.smoke_symbol:
        try:
            rows = _json([binary, "query", cfg.smoke_symbol, "--json", "--limit", "5", "--path", root], project.root)
        except (RuntimeError, OSError, ValueError, subprocess.TimeoutExpired) as exc:
            checks.append(Check(C + ".query", BLOCKED, f"symbol query failed: {exc}", None))
            return checks
        found = [r["node"]["filePath"] for r in rows if r.get("node", {}).get("name") == cfg.smoke_symbol]
        # An empty result is a lookup failure, never evidence that the symbol is absent.
        checks.append(
            Check(
                C + ".query",
                HEALTHY if found else DEGRADED,
                f"{cfg.smoke_symbol} found in {found[0]}"
                if found
                else f"{cfg.smoke_symbol} not returned; the index may be stale",
                None if found else f"codegraph sync {root}",
            )
        )
    return checks
