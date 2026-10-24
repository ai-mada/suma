"""Publishable, read-only T-P4-004 evidence collector.

Only explicit non-secret columns and schema allowlists are read. This script never
opens session/auth/key/config files, copies the database, or executes a tool/model.
Run from the Suma repository after the three browser journeys and profile pause.
Outputs are create-only to avoid rewriting an earlier observation.
"""
from __future__ import annotations

import collections
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import sqlite3
import subprocess
import sys


ROOT = Path.cwd().resolve()
STAGE = ROOT / ".cache/e2e/tool-stage"
DB = STAGE / "host-data/suma.db"
OUT = STAGE / "tool-browser-execution.json"
LIVE_OUT = STAGE / "live-runtime-final.json"
CUT = "20261024T045219750Z-715ba2c8"
SOURCE_SHA = "498300c9adffa8327982faa25da7d745d1a9ecb08f62265691d5673deacb318e"
RUNS = {
    "success": "10158abedf0d680f439ca0f2fac2d434",
    "failure": "68c1e6179992238819c5bf4aaef19183",
    "confirmed_stop": "230cfddc70b539f9b5b212ad675b9d05",
}
PROJECT = "00b793dfdfd9accf98481438992d8ce3"
CANDIDATE = "c31c01956f3098f3b5c07b2c94ceaaac3b388dfe"
PROFILE_FIELDS = ("command_id", "adapter", "protocol", "profile_sha256", "executable_sha256", "tool_version", "read_only")
USAGE_FIELDS = ("input_tokens", "cached_input_tokens", "cache_write_input_tokens", "output_tokens", "reasoning_output_tokens")
EVENT_FIELDS = ("schema_version", "command_id", "profile_sha256", "protocol", "kind", "session_id", "item_id", "message", "status")
WORKSPACE_FIELDS = (
    "admin_base_commit", "attempt_key", "base_commit", "captured_at", "changed_paths",
    "changed_paths_total", "changed_paths_truncated", "clean", "diff_sha256",
    "evidence_sha256", "exit_code", "head_commit", "outcome_unknown", "state",
    "status_sha256", "test_command", "untracked_sha256", "candidate_attempt",
    "candidate_evidence_sha256", "candidate_fence", "candidate_from",
)
REDACTIONS: collections.Counter[str] = collections.Counter()


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def bytes_receipt(raw: str | bytes) -> dict:
    data = raw.encode("utf-8") if isinstance(raw, str) else raw
    return {"sha256": digest(data), "size_bytes": len(data)}


def text(value: object, limit: int = 8192) -> str:
    s = str(value)
    patterns = {
        "bearer": r"(?i)\bBearer\s+[A-Za-z0-9._~+/=-]{8,}",
        "provider_key": r"\b(?:sk-(?:proj-)?[A-Za-z0-9_-]{12,}|gh[pousr]_[A-Za-z0-9_]{12,}|github_pat_[A-Za-z0-9_]{12,})\b",
        "credential_assignment": r'''(?ix)(["']?(?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|client[_-]?secret|master[_-]?key|authorization|cookie)["']?\s*[:=]\s*)["']?[^\s,"'\]\}]{6,}''',
        "url_credentials": r"(?i)(https?://)[^\s/@:]+:[^\s/@]+@",
    }
    for label, pattern in patterns.items():
        replacement = r"\1[REDACTED]" if label in {"credential_assignment", "url_credentials"} else "[REDACTED]"
        s, count = re.subn(pattern, replacement, s)
        REDACTIONS[label] += count
    s = "".join(c for c in s if c in "\n\r\t" or ord(c) >= 32)
    data = s.encode("utf-8")
    if len(data) > limit:
        REDACTIONS["bounded_text"] += 1
        s = data[:limit].decode("utf-8", errors="ignore") + " [TRUNCATED]"
    return s


def value(v: object) -> object:
    if isinstance(v, str):
        return text(v)
    if v is None or isinstance(v, (bool, int, float)):
        return v
    if isinstance(v, list):
        assert len(v) <= 1024, "Unexpected evidence array size"
        return [value(x) for x in v]
    raise ValueError("Nested object requires an explicit schema allowlist")


def pick(obj: dict, fields: tuple | list) -> dict:
    return {k: value(obj[k]) for k in fields if k in obj}


def profile(obj: dict) -> dict:
    return pick(obj, PROFILE_FIELDS)


def tool_evidence(obj: dict | None) -> dict | None:
    if obj is None:
        return None
    out = pick(obj, ("session_id", "terminal", "event_count"))
    out["profile"] = profile(obj["profile"])
    if "usage" in obj and obj["usage"] is not None:
        out["usage"] = pick(obj["usage"], USAGE_FIELDS)
    return out


def tool_event(obj: dict) -> dict:
    out = pick(obj, EVENT_FIELDS)
    if "usage" in obj and obj["usage"] is not None:
        out["usage"] = pick(obj["usage"], USAGE_FIELDS)
    return out


def context_record(obj: dict) -> dict:
    return pick(obj, ("id", "version", "title", "description", "acceptance_criteria"))


def node_spec(obj: dict) -> dict:
    out = pick(obj, ("id", "name", "type", "command", "depends_on", "idempotent", "join", "max_retries", "quorum", "timeout_seconds"))
    config = obj.get("config") or {}
    out["config"] = pick(config, ("tool_profile_id", "tool_profile_sha256", "candidate_from"))
    if "goal" in config:
        out["config"]["goal_receipt"] = bytes_receipt(config["goal"])
    return out


def workflow(obj: dict) -> dict:
    out = pick(obj, ("id", "version", "name"))
    out["nodes"] = [node_spec(x) for x in obj.get("nodes", [])]
    out["edges"] = [pick(x, ("from", "to", "condition", "required", "on")) for x in obj.get("edges", [])]
    return out


def record_data(obj: dict) -> dict:
    out = pick(obj, ("run_id", "origin", "requirement_version", "run_task_basis", "verified_run_id", "source_issue_id"))
    if "dependency_basis" in obj:
        out["dependency_basis"] = {text(k): value(v) for k, v in obj["dependency_basis"].items()}
    if isinstance(obj.get("source"), dict):
        out["source"] = pick(obj["source"], ("origin", "run_id"))
    return out


def rows(db: sqlite3.Connection, sql: str, params: tuple = ()) -> list[dict]:
    assert "SELECT *" not in sql.upper()
    return [dict(r) for r in db.execute(sql, params)]


def scalar(db: sqlite3.Connection, sql: str, params: tuple = ()) -> object:
    return db.execute(sql, params).fetchone()[0]


def business_records(db: sqlite3.Connection, table: str) -> list[dict]:
    assert table in {"suma_tasks", "suma_requirements", "suma_issues"}
    fields = "id,tenant_id,project_id,version,title,description,status,priority,owner_id,requirement_id,run_id,depends_on,acceptance_criteria,kind,created_at,updated_at,data"
    result = []
    for row in rows(db, "SELECT " + fields + " FROM " + table + " WHERE project_id=? ORDER BY created_at,id", (PROJECT,)):
        out = pick(row, [k for k in row if k not in {"depends_on", "acceptance_criteria", "data"}])
        out["depends_on"] = value(json.loads(row["depends_on"]))
        out["acceptance_criteria"] = value(json.loads(row["acceptance_criteria"]))
        out["data"] = record_data(json.loads(row["data"]))
        out["stored_data_receipt"] = bytes_receipt(row["data"])
        result.append(out)
    return result


def collect_database() -> dict:
    db = sqlite3.connect(DB.resolve().as_uri() + "?mode=ro", uri=True)
    db.row_factory = sqlite3.Row
    db.execute("PRAGMA query_only=ON")
    db.execute("BEGIN")
    try:
        observed = scalar(db, "SELECT strftime('%Y-%m-%dT%H:%M:%fZ','now')")
        project_rows = rows(db, "SELECT id,tenant_id,version,title,status,created_at,updated_at FROM suma_projects WHERE id=?", (PROJECT,))
        assert len(project_rows) == 1
        projects = [pick(r, list(r)) for r in project_rows]
        tenant = project_rows[0]["tenant_id"]
        params = (tenant, PROJECT)
        profiles = []
        for r in rows(db, "SELECT id,tenant_id,project_id,worker_id,name,version,state,profile,enabled_by,created_at,updated_at FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? ORDER BY created_at,id", params):
            out = pick(r, [k for k in r if k != "profile"])
            out["profile"] = profile(json.loads(r["profile"]))
            out["stored_profile_receipt"] = bytes_receipt(r["profile"])
            profiles.append(out)
        inventories = []
        for r in rows(db, "SELECT tenant_id,project_id,worker_id,observed_at,items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? ORDER BY worker_id", params):
            out = pick(r, ("tenant_id", "project_id", "worker_id", "observed_at"))
            out["items"] = [{"profile": profile(x["profile"]), **pick(x, ("available", "reason"))} for x in json.loads(r["items"])]
            out["stored_items_receipt"] = bytes_receipt(r["items"])
            inventories.append(out)
        workers = [pick(r, [k for k in r if k != "profiles"]) | {"profiles": value(json.loads(r["profiles"]))} for r in rows(db, "SELECT id,tenant_id,project_id,worker_id,version,status,profiles,deployment,created_by,created_at,expires_at FROM suma_release_workers WHERE tenant_id=? AND project_id=? ORDER BY id", params)]
        plans = []
        for r in rows(db, "SELECT id,tenant_id,project_id,run_id,version,state,created_by,created_at,expires_at,payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? ORDER BY created_at,id", params):
            payload = json.loads(r["payload"])
            out = pick(payload, ("id", "project_id", "version", "state", "task_id", "task_version", "requirement_id", "requirement_version", "workflow_id", "workflow_version", "agent_node_id", "tool_profile_id", "tool_profile_version", "tool_profile_sha256", "run_id", "created_at", "expires_at", "valid", "reason"))
            out["context"] = {k: context_record(v) for k, v in payload["context"].items() if k in {"task", "requirement"} and v is not None}
            out["workflow"] = workflow(payload["workflow"])
            out["stored_row"] = pick(r, [k for k in r if k != "payload"])
            out["stored_payload_receipt"] = bytes_receipt(r["payload"])
            plans.append(out)
        runs = [pick(r, list(r)) for r in rows(db, "SELECT id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at FROM dag_runs WHERE tenant_id=? AND project_id=? ORDER BY created_at,id", params)]
        assert {r["id"] for r in runs} == set(RUNS.values()), "Unexpected project run set"
        nodes, attempts, candidates, events = [], [], [], []
        for run in runs:
            rid = run["id"]
            published = rows(db, "SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?", (tenant, PROJECT, run["workflow_id"], run["workflow_version"]))
            assert len(published) == 1
            run["workflow_definition"] = workflow(json.loads(published[0]["spec"]))
            run["stored_workflow_receipt"] = bytes_receipt(published[0]["spec"])
            for r in rows(db, "SELECT run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec FROM dag_nodes WHERE run_id=? ORDER BY id", (rid,)):
                out = pick(r, [k for k in r if k not in {"output", "spec"}])
                out["spec"] = node_spec(json.loads(r["spec"]))
                out["stored_spec_receipt"] = bytes_receipt(r["spec"])
                out["stored_output_receipt"] = bytes_receipt(r["output"])
                nodes.append(out)
            for r in rows(db, "SELECT run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result FROM dag_attempts WHERE run_id=? ORDER BY node_id,attempt", (rid,)):
                raw = json.loads(r["result"])
                out = pick(r, [k for k in r if k != "result"])
                out["result"] = pick(raw, ("run_id", "node_id", "attempt", "fence", "worker_id", "operation_id", "exit_code", "error", "outcome_unknown", "stopped"))
                if "workspace_evidence" in raw:
                    out["result"]["workspace_evidence"] = pick(raw["workspace_evidence"], WORKSPACE_FIELDS)
                if "tool_evidence" in raw:
                    out["result"]["tool_evidence"] = tool_evidence(raw["tool_evidence"])
                out["stored_result_receipt"] = bytes_receipt(r["result"])
                out["stored_output_receipt"] = bytes_receipt(raw.get("output", ""))
                attempts.append(out)
            candidates.extend([pick(r, list(r)) for r in rows(db, "SELECT run_id,node_id,attempt,commit_id,source_node,source_attempt,source_fence,evidence_sha256 FROM dag_candidate_revisions WHERE run_id=? ORDER BY node_id,attempt", (rid,))])
            for r in rows(db, "SELECT sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data FROM dag_events WHERE run_id=? ORDER BY sequence", (rid,)):
                raw = json.loads(r["data"])
                out = pick(r, [k for k in r if k != "data"])
                out["data"] = None if raw is None else pick(raw, ("workflow_version", "attempt", "fence", "previous_state", "reason", "operation_id", "worker_id", "message", "commit", "evidence_sha256", "source_attempt", "source_fence", "source_node"))
                if isinstance(raw, dict) and "tool_event" in raw:
                    out["data"]["tool_event"] = tool_event(raw["tool_event"])
                out["stored_data_receipt"] = bytes_receipt(r["data"])
                events.append(out)
        reviews = [pick(r, list(r)) for r in rows(db, "SELECT id,tenant_id,project_id,task_id,task_version,run_id,reviewer_id,decision,reason,created_at FROM suma_reviews WHERE tenant_id=? AND project_id=? ORDER BY created_at,id", params)]
        activity = [pick(r, list(r)) for r in rows(db, "SELECT id,tenant_id,project_id,actor_id,kind,object_id,title,created_at FROM suma_activity WHERE tenant_id=? AND project_id=? ORDER BY created_at,id", params)]
        records = {k: business_records(db, "suma_" + k) for k in ("tasks", "requirements", "issues")}
        result = {
            "schema_version": 1, "stage": "T-P4-004", "observed_at_database_utc": observed,
            "collection": {
                "read_only": True, "sqlite_open_mode": "ro", "query_only": bool(scalar(db, "PRAGMA query_only")),
                "consistent_read_transaction": True, "database_archived": False,
                "scope": "Only this independent fixture project; persisted facts do not prove browser clicks or vendor/model execution.",
                "collector_path": str(Path(__file__).resolve().relative_to(ROOT)).replace("\\", "/"),
                "collector_sha256": digest(Path(__file__).read_bytes()),
                "excluded": ["all auth/session/user/password/token/key tables and columns", "runner command/env config", "process command lines", "goal/prompt raw bytes", "raw stdout/stderr", "unknown arbitrary JSON fields"],
                "retained_text": "Bounded redacted business text and schema-allowlisted public fixture messages; original selected JSON only via SHA/size.",
            },
            "project": projects[0], "tool_profiles": profiles, "tool_inventories": inventories,
            "registered_workers_without_credentials": workers, "task_dispatch_plans": plans,
            **records, "reviews": reviews, "runs": runs, "nodes": nodes, "attempts": attempts,
            "candidate_revisions": candidates, "events": sorted(events, key=lambda r: r["sequence"]), "activity": activity,
        }
        result["counts"] = {k: len(v) for k, v in result.items() if isinstance(v, list)}
        result["event_counts"] = dict(collections.Counter(x["event_type"] for x in events))
        result["journey_assertions"] = assert_journeys(result)
        result["collection"]["redaction_counts"] = {k: v for k, v in REDACTIONS.items() if v}
        return result
    finally:
        db.rollback()
        db.close()


def assert_journeys(data: dict) -> dict:
    run = {r["id"]: r for r in data["runs"]}
    plans = {p["run_id"]: p for p in data["task_dispatch_plans"]}
    tasks = {t["id"]: t for t in data["tasks"]}
    profiles = {p["id"]: p for p in data["tool_profiles"]}
    nodes = {(n["run_id"], n["id"]): n for n in data["nodes"]}
    attempts = {(n["run_id"], n["node_id"], n["attempt"]): n for n in data["attempts"]}
    success, failure, stop = (RUNS[k] for k in ("success", "failure", "confirmed_stop"))
    checks = {}
    for rid, expected in ((success, "succeeded"), (failure, "failed"), (stop, "cancelled")):
        assert run[rid]["state"] == expected
        p = plans[rid]
        assert p["stored_row"]["state"] == "started" and p["stored_row"]["run_id"] == rid
        assert tasks[p["task_id"]]["run_id"] == rid
        assert run[rid]["workflow_id"] == "managed-" + rid and run[rid]["workflow_version"] == 1
        assert run[rid]["workflow_definition"]["nodes"] == p["workflow"]["nodes"]
        assert run[rid]["workflow_definition"]["edges"] == p["workflow"]["edges"]
        assert p["context"]["task"]["id"] == p["task_id"] and p["context"]["task"]["version"] == p["task_version"]
        assert profiles[p["tool_profile_id"]]["profile"]["profile_sha256"] == p["tool_profile_sha256"]
        agent = nodes[(rid, p["agent_node_id"])]
        assert agent["spec"]["config"]["tool_profile_id"] == p["tool_profile_id"]
        assert agent["spec"]["config"]["tool_profile_sha256"] == p["tool_profile_sha256"]
        assert agent["spec"]["command"] == [profiles[p["tool_profile_id"]]["profile"]["command_id"]]
        a = attempts[(rid, agent["id"], agent["attempt"])]
        assert a["result"]["tool_evidence"]["profile"] == profiles[p["tool_profile_id"]]["profile"]
    checks["three_terminal_runs_and_exact_plan_task_profile_binding"] = True
    task = tasks[plans[success]["task_id"]]
    assert task["id"] == "3418219185fe4c3311eceff6b853bed2" and task["status"] == "accepted" and task["version"] == 3
    assert task["data"]["verified_run_id"] == success
    review = [r for r in data["reviews"] if r["task_id"] == task["id"] and r["run_id"] == success]
    assert len(review) == 1 and review[0]["decision"] == "accepted" and review[0]["task_version"] == 2
    assert nodes[(success, "develop")]["state"] == nodes[(success, "quality")]["state"] == "succeeded"
    agent_attempt = attempts[(success, "develop", 1)]["result"]
    test_attempt = attempts[(success, "quality", 1)]["result"]
    assert agent_attempt["tool_evidence"]["terminal"] == "completed" and agent_attempt["exit_code"] == 0
    assert agent_attempt["workspace_evidence"]["head_commit"] == CANDIDATE
    assert test_attempt["workspace_evidence"]["head_commit"] == CANDIDATE and test_attempt["exit_code"] == 0
    assert test_attempt["workspace_evidence"]["candidate_from"] == "develop"
    assert test_attempt["workspace_evidence"]["candidate_evidence_sha256"] == agent_attempt["workspace_evidence"]["evidence_sha256"]
    assert [c for c in data["candidate_revisions"] if c["run_id"] == success and c["commit_id"] == CANDIDATE]
    checks["success_fixed_candidate_independent_test_and_current_human_acceptance_v3"] = True
    failures = [i for i in data["issues"] if i["run_id"] == failure or i["data"].get("run_id") == failure]
    assert len(failures) == 1 and failures[0]["id"] == "76e136e48e633dbf4d8ad2581bba7b9b"
    intakes = [r for r in data["requirements"] if r["data"].get("source_issue_id") == failures[0]["id"]]
    assert len(intakes) == 1 and len(data["requirements"]) == 2
    assert intakes[0]["data"]["source"]["run_id"] == failure
    assert nodes[(failure, "develop")]["state"] == "failed" and nodes[(failure, "quality")]["state"] == "skipped"
    assert attempts[(failure, "develop", 1)]["result"]["tool_evidence"]["terminal"] == "failed"
    checks["failure_one_issue_one_intake_requirement_two_total_requirements"] = True
    a = attempts[(stop, "develop", 1)]["result"]
    assert a["stopped"] is True and a["outcome_unknown"] is False and a["tool_evidence"]["terminal"] == "unknown"
    assert nodes[(stop, "develop")]["state"] == "cancelled" and nodes[(stop, "quality")]["state"] == "cancelled"
    assert nodes[(stop, "quality")]["attempt"] == 0
    assert a["workspace_evidence"]["changed_paths_total"] == 0
    assert not [r for r in data["reviews"] if r["run_id"] == stop]
    assert tasks[plans[stop]["task_id"]]["status"] != "accepted"
    stop_events = [r for r in data["events"] if r["run_id"] == stop]
    assert {"command_cancel", "stop_requested", "run_cancelled"} <= {e["event_type"] for e in stop_events}
    checks["confirmed_process_stop_unknown_protocol_cancelled_without_test_or_acceptance"] = True
    assert len(profiles) == 1 and all(p["state"] == "paused" and p["version"] == 3 for p in profiles.values())
    assert all(p["tool_profile_version"] == 2 for p in plans.values())
    checks["profile_now_paused_v3_historical_plans_retain_v2_and_original_digest"] = True
    for event in data["events"]:
        if event["event_type"] == "tool_progress":
            p = plans[event["run_id"]]
            event_profile = event["data"]["tool_event"]
            assert event_profile["profile_sha256"] == p["tool_profile_sha256"]
            assert event_profile["command_id"] == profiles[p["tool_profile_id"]]["profile"]["command_id"]
            node = nodes[(event["run_id"], event["node_id"])]
            assert event["data"]["attempt"] == node["attempt"] and event["data"]["fence"] == node["fence"]
            assert event["data"]["worker_id"] == node["worker_id"]
    checks["typed_events_match_pinned_profile_worker_attempt_fence"] = True
    checks["repeated_intake_gui_clicks"] = "Not established by DB alone; root browser evidence separately records both actions."
    return checks


def run_readonly(command: list[str], *, env: dict | None = None) -> str:
    p = subprocess.run(command, cwd=ROOT, env=env, capture_output=True, text=True, encoding="utf-8", timeout=20, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0))
    if p.returncode:
        raise RuntimeError("Read-only metadata command failed; no raw stdout/stderr published")
    return p.stdout.strip()


def collect_runtime(database: dict) -> dict:
    expected = {
        "suma-control.exe": (131196, "c06d3553c46278704dd976f5ab1dcef52d3b25a96f48d76e3bed4d08092b4eb8"),
        "suma-runner.exe": (144792, "12c8681ec9d7e1ba325252e264e5d7aaa7d73d9c053cc3d6c3466292b44f56cb"),
    }
    script = "$ErrorActionPreference='Stop'; @{ processes=@(Get-CimInstance Win32_Process -Filter 'ProcessId=131196 OR ProcessId=144792' | Select-Object ProcessId,ExecutablePath); listeners=@(Get-NetTCPConnection -LocalPort 4322 -State Listen | Select-Object LocalAddress,LocalPort,OwningProcess) } | ConvertTo-Json -Depth 5 -Compress"
    observed = json.loads(run_readonly(["pwsh", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script]))
    processes = {p["ProcessId"]: p for p in observed["processes"]}
    binaries = []
    for name, (pid, sha) in expected.items():
        path = ROOT / "dist/runtime/windows-amd64" / CUT / name
        assert Path(processes[pid]["ExecutablePath"]).resolve() == path.resolve()
        binary = path.read_bytes()
        assert digest(binary) == sha
        binaries.append({"program": name, "pid": pid, "executable_path": str(path), "size_bytes": len(binary), "sha256": sha, "process_image_path_matches": True})
    assert len(observed["listeners"]) == 1
    assert observed["listeners"][0] == {"LocalAddress": "127.0.0.1", "LocalPort": 4322, "OwningProcess": 131196}
    receipt_dir = ROOT / "dist/runtime/builds" / CUT
    source = json.loads((receipt_dir / "source-manifest.json").read_text(encoding="utf-8"))
    manifest = json.loads((receipt_dir / "manifest.json").read_text(encoding="utf-8"))
    assert manifest["source_sha256"] == SOURCE_SHA
    source_rows = source if isinstance(source, list) else source.get("files", [])
    # The original source manifest schema is detected explicitly rather than guessing a value.
    if not source_rows and isinstance(source, dict):
        source_rows = source.get("sources", [])
    assert len(source_rows) == 116, "Unexpected runtime source manifest shape"
    for row in source_rows:
        path = ROOT / row["path"]
        assert digest(path.read_bytes()) == row["sha256"], "Runtime source changed after frozen cut"
    web = ROOT / "apps/web/dist"
    index = web / "index.html"
    index_bytes = index.read_bytes()
    assets = re.findall(r'(?:src|href)="(/assets/[^"?#]+)"', index_bytes.decode("utf-8"))
    asset_receipts = []
    for asset in assets:
        path = web / asset.lstrip("/")
        assert path.resolve().is_relative_to(web.resolve()) and path.is_file()
        asset_receipts.append({"reference": asset, "path": str(path), **bytes_receipt(path.read_bytes())})
    fixture = STAGE / "task-fixture/repository"
    env = {k: v for k, v in os.environ.items() if not k.upper().startswith("GIT_")}
    env.update(GIT_TERMINAL_PROMPT="0", GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull, GIT_OPTIONAL_LOCKS="0")

    def git(*args: str) -> str:
        return run_readonly(["git", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + str(STAGE / "task-fixture/disabled-hooks"), "-C", str(fixture), *args], env=env)

    top = Path(git("rev-parse", "--show-toplevel")).resolve()
    head = git("rev-parse", "HEAD")
    status = git("status", "--porcelain=v1", "--untracked-files=all")
    seed = json.loads((STAGE / "task-fixture/fixture-receipt.json").read_text(encoding="utf-8"))
    assert top == fixture.resolve() and head == seed["head"] and not status
    success_result = next(a["result"] for a in database["attempts"] if a["run_id"] == RUNS["success"] and a["node_id"] == "develop")
    assert success_result["workspace_evidence"]["admin_base_commit"] == head
    assert success_result["workspace_evidence"]["base_commit"] == head
    candidate_parent = git("rev-parse", CANDIDATE + "^1")
    assert candidate_parent == head
    changed = git("diff", "--no-ext-diff", "--no-textconv", "--name-only", head, CANDIDATE).splitlines()
    assert changed == ["app.py"]
    return {
        "schema_version": 1, "stage": "T-P4-004", "observed_at_utc": dt.datetime.now(dt.timezone.utc).isoformat(),
        "read_only": True, "runtime_cut": CUT, "source_sha256": SOURCE_SHA,
        "current_runtime_source_files_verified": len(source_rows), "binaries": binaries,
        "listeners": observed["listeners"], "runner_port": None,
        "runner_port_note": "Runner uses outbound requests; no listening port is asserted.",
        "source_manifest": {"path": str(receipt_dir / "source-manifest.json"), **bytes_receipt((receipt_dir / "source-manifest.json").read_bytes())},
        "web": {"dist": str(web), "entry": {"path": str(index), **bytes_receipt(index_bytes)}, "entry_assets": asset_receipts, "scope": "Bytes currently on disk only; root records the browser-observed request/response assets separately."},
        "fixture_repository": {"path": str(fixture), "git_top": str(top), "top_matches_exact": True, "head": head, "baseline_head_from_original_receipt": seed["head"], "baseline_tests_exit": seed["baseline_test_exit"], "status_porcelain_v1": status, "clean": not status, "candidate": CANDIDATE, "candidate_parent": candidate_parent, "candidate_changed_paths": changed, "candidate_is_original_checkout_head": head == CANDIDATE},
        "database_snapshot": {"path": str(OUT.relative_to(ROOT)).replace("\\", "/"), "observed_at_database_utc": database["observed_at_database_utc"]},
        "collector_sha256": digest(Path(__file__).read_bytes()),
        "scope": "Process image, actual on-disk binary hashes, listener owner, runtime source cut, disk web hashes and original fixture Git state. No service was started/stopped; no model or target executable was invoked by this collector.",
    }


def main() -> None:
    assert ROOT.name == "suma" and DB.is_file()
    assert not OUT.exists() and not LIVE_OUT.exists(), "Refusing to overwrite existing observations"
    database = collect_database()
    live = collect_runtime(database)
    # Publish only after both independent read phases have passed all assertions.
    for path, obj in ((OUT, database), (LIVE_OUT, live)):
        with path.open("x", encoding="utf-8", newline="\n") as f:
            json.dump(obj, f, ensure_ascii=False, indent=2)
            f.write("\n")
    print(json.dumps({"stage": "T-P4-004", "read_only": True, "journey_assertions": database["journey_assertions"], "counts": database["counts"], "files": [{"path": str(p.relative_to(ROOT)).replace("\\", "/"), **bytes_receipt(p.read_bytes())} for p in (OUT, LIVE_OUT)], "collector_sha256": digest(Path(__file__).read_bytes())}, ensure_ascii=False))


if __name__ == "__main__":
    main()
