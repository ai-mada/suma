"""Create-only, read-only supplement for the fourth confirmed-stop journey.

Preparation alone does not open the database or inspect running processes. Run
only after the main integrator supplies the new run/build IDs and process IDs.
No private context, credentials, environment, process arguments, raw goal, raw
stdout/stderr, or unknown JSON fields are published. Historical files are inputs
whose bytes must remain unchanged, never rewrite targets.
"""
from __future__ import annotations

import argparse
import collections
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sqlite3
import subprocess


ROOT = Path.cwd().resolve()
STAGE = ROOT / '.cache/e2e/tool-stage'
DB = STAGE / 'host-data/suma.db'
OLD_COLLECTOR = STAGE / 'collect-final-tool-evidence.py'
OLD_COLLECTOR_SHA = '5176d546f1973f46795a64d37665f385e6dc9c8773085ee41a3b5b30249a3337'
BASELINE = STAGE / 'tool-browser-execution.json'
BASELINE_SHA = 'cc690d964ee0228f0bd17c2db1a91a467daf628e63a4c5518cd718efbb1e6af7'
PROJECT = '00b793dfdfd9accf98481438992d8ce3'
OLD_RUNS = {'success': '10158abedf0d680f439ca0f2fac2d434', 'failure': '68c1e6179992238819c5bf4aaef19183', 'confirmed_stop': '230cfddc70b539f9b5b212ad675b9d05'}
EXPECTED_PROFILE = '63fce9b8a2cb783e3344e7e0ebdc6494'
EXPECTED_ACCEPTED_TASK = '3418219185fe4c3311eceff6b853bed2'
EXPECTED_ISSUE = '76e136e48e633dbf4d8ad2581bba7b9b'
HEX32 = re.compile(r'^[0-9a-f]{32}$')
HEX64 = re.compile(r'^[0-9a-f]{64}$')
STAMP = re.compile(r'^\d{8}T\d{9}Z-[0-9a-f]{8}$')


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def load_helpers():
    assert digest(OLD_COLLECTOR.read_bytes()) == OLD_COLLECTOR_SHA, 'Original collector bytes changed'
    spec = importlib.util.spec_from_file_location('suma_historical_evidence_helpers', OLD_COLLECTOR)
    assert spec and spec.loader
    helper = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helper)  # Its __main__ block never runs.
    return helper


def raw_receipt(helper, raw: str) -> dict:
    return helper.bytes_receipt(raw)


def read_baseline() -> dict:
    raw = BASELINE.read_bytes()
    assert digest(raw) == BASELINE_SHA, 'Historical observation bytes changed'
    baseline = json.loads(raw.decode('utf-8'))
    assert {r['id'] for r in baseline['runs']} == set(OLD_RUNS.values())
    assert baseline['counts']['runs'] == 3 and baseline['counts']['nodes'] == 6 and baseline['counts']['attempts'] == 4
    return baseline


def project_business(db, helper, tenant: str, table: str) -> list[dict]:
    assert table in {'suma_tasks', 'suma_issues', 'suma_requirements'}
    result = []
    for r in helper.rows(db, 'SELECT id,version,status,run_id,requirement_id,data FROM '+table+' WHERE tenant_id=? AND project_id=? ORDER BY id', (tenant, PROJECT)):
        out = helper.pick(r, ('id', 'version', 'status', 'run_id', 'requirement_id'))
        out['data'] = helper.record_data(json.loads(r['data']))
        out['stored_data_receipt'] = raw_receipt(helper, r['data'])
        result.append(out)
    return result


def assert_historical(db, helper, baseline: dict, tenant: str, run_rows: list[dict]) -> dict:
    current_runs = {r['id']: r for r in run_rows}
    expected_states = {'success': 'succeeded', 'failure': 'failed', 'confirmed_stop': 'cancelled'}
    receipts = []
    for label, rid in OLD_RUNS.items():
        old_run = next(r for r in baseline['runs'] if r['id'] == rid)
        now = current_runs[rid]
        stable_run_fields = ('id', 'tenant_id', 'project_id', 'workflow_id', 'workflow_version', 'workflow_name', 'version', 'state', 'created_at', 'updated_at')
        assert helper.pick(now, stable_run_fields) == helper.pick(old_run, stable_run_fields)
        assert now['state'] == expected_states[label]
        old_plan = next(p for p in baseline['task_dispatch_plans'] if p['run_id'] == rid)
        plans = helper.rows(db, 'SELECT id,version,state,run_id,payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?', (tenant, PROJECT, rid))
        assert len(plans) == 1
        plan = plans[0]
        assert helper.pick(plan, ('id', 'version', 'state', 'run_id')) == helper.pick(old_plan['stored_row'], ('id', 'version', 'state', 'run_id'))
        plan_receipt = raw_receipt(helper, plan['payload'])
        assert plan_receipt == old_plan['stored_payload_receipt']
        specs = helper.rows(db, 'SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?', (tenant, PROJECT, now['workflow_id'], now['workflow_version']))
        assert len(specs) == 1
        workflow_receipt = raw_receipt(helper, specs[0]['spec'])
        assert workflow_receipt == old_run['stored_workflow_receipt']
        old_nodes = [n for n in baseline['nodes'] if n['run_id'] == rid]
        now_nodes = helper.rows(db, 'SELECT id,state,attempt,fence,spec,output FROM dag_nodes WHERE run_id=? ORDER BY id', (rid,))
        assert len(now_nodes) == len(old_nodes) == 2
        for n in now_nodes:
            old = next(x for x in old_nodes if x['id'] == n['id'])
            assert helper.pick(n, ('id', 'state', 'attempt', 'fence')) == helper.pick(old, ('id', 'state', 'attempt', 'fence'))
            assert raw_receipt(helper, n['spec']) == old['stored_spec_receipt']
            assert raw_receipt(helper, n['output']) == old['stored_output_receipt']
        old_attempts = [a for a in baseline['attempts'] if a['run_id'] == rid]
        now_attempts = helper.rows(db, 'SELECT node_id,attempt,fence,state,result FROM dag_attempts WHERE run_id=? ORDER BY node_id,attempt', (rid,))
        assert len(now_attempts) == len(old_attempts)
        for a in now_attempts:
            old = next(x for x in old_attempts if x['node_id'] == a['node_id'] and x['attempt'] == a['attempt'])
            assert helper.pick(a, ('node_id', 'attempt', 'fence', 'state')) == helper.pick(old, ('node_id', 'attempt', 'fence', 'state'))
            assert raw_receipt(helper, a['result']) == old['stored_result_receipt']
        old_events = [e for e in baseline['events'] if e['run_id'] == rid]
        now_events = helper.rows(db, 'SELECT sequence,event_type,at_time,data FROM dag_events WHERE run_id=? ORDER BY sequence', (rid,))
        assert len(now_events) == len(old_events)
        for event, old in zip(now_events, old_events):
            assert helper.pick(event, ('sequence', 'event_type', 'at_time')) == helper.pick(old, ('sequence', 'event_type', 'at_time'))
            assert raw_receipt(helper, event['data']) == old['stored_data_receipt']
        receipts.append({'label': label, **helper.pick(now, stable_run_fields), 'stored_plan_payload_receipt': plan_receipt, 'stored_workflow_receipt': workflow_receipt, 'nodes_unchanged': len(now_nodes), 'attempt_results_unchanged': len(now_attempts), 'event_payloads_unchanged': len(now_events)})
    return {'all_unchanged': True, 'baseline_sha256': BASELINE_SHA, 'runs': receipts, 'total_old_event_payloads_unchanged': sum(r['event_payloads_unchanged'] for r in receipts)}


def collect_database(args, helper, baseline: dict) -> dict:
    db = sqlite3.connect(DB.resolve().as_uri()+'?mode=ro', uri=True)
    db.row_factory = sqlite3.Row
    db.execute('PRAGMA query_only=ON')
    db.execute('BEGIN')
    try:
        observed = helper.scalar(db, "SELECT strftime('%Y-%m-%dT%H:%M:%fZ','now')")
        project = helper.rows(db, 'SELECT id,tenant_id,version,title,status FROM suma_projects WHERE id=?', (PROJECT,))
        assert len(project) == 1
        tenant = project[0]['tenant_id']
        params = (tenant, PROJECT)
        run_rows = helper.rows(db, 'SELECT id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at FROM dag_runs WHERE tenant_id=? AND project_id=? ORDER BY created_at,id', params)
        assert len(run_rows) == 4 and {r['id'] for r in run_rows} == set(OLD_RUNS.values()) | {args.run_id}, 'Expected exactly the original three runs and one new run'
        counts = {'runs': len(run_rows)}
        for key, table in [('nodes', 'dag_nodes'), ('attempts', 'dag_attempts')]:
            counts[key] = helper.scalar(db, 'SELECT count(*) FROM '+table+' WHERE run_id IN (SELECT id FROM dag_runs WHERE tenant_id=? AND project_id=?)', params)
        counts['task_dispatch_plans'] = helper.scalar(db, 'SELECT count(*) FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=?', params)
        assert counts == {'runs': 4, 'nodes': 8, 'attempts': 5, 'task_dispatch_plans': 4}, 'Unexpected project counts'
        historical = assert_historical(db, helper, baseline, tenant, run_rows)
        assert historical['total_old_event_payloads_unchanged'] == 35
        profiles = helper.rows(db, 'SELECT id,tenant_id,project_id,worker_id,name,version,state,profile,enabled_by,created_at,updated_at FROM suma_tool_profiles WHERE tenant_id=? AND project_id=?', params)
        assert len(profiles) == 1 and profiles[0]['id'] == EXPECTED_PROFILE and profiles[0]['state'] == 'paused' and profiles[0]['version'] == 5
        p = profiles[0]
        current_profile = helper.pick(p, ('id', 'tenant_id', 'project_id', 'worker_id', 'name', 'version', 'state', 'enabled_by', 'created_at', 'updated_at'))
        current_profile['profile'] = helper.profile(json.loads(p['profile']))
        current_profile['stored_profile_receipt'] = raw_receipt(helper, p['profile'])
        assert current_profile['stored_profile_receipt'] == baseline['tool_profiles'][0]['stored_profile_receipt']
        rid = args.run_id
        run = next(r for r in run_rows if r['id'] == rid)
        assert run['state'] == 'cancelled' and run['workflow_id'] == 'managed-'+rid and run['workflow_version'] == 1
        plan_rows = helper.rows(db, 'SELECT id,tenant_id,project_id,run_id,version,state,created_by,created_at,expires_at,payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?', (*params, rid))
        assert len(plan_rows) == 1
        pr = plan_rows[0]
        raw_plan = json.loads(pr['payload'])
        plan = helper.pick(raw_plan, ('id', 'project_id', 'version', 'state', 'task_id', 'task_version', 'requirement_id', 'requirement_version', 'workflow_id', 'workflow_version', 'agent_node_id', 'tool_profile_id', 'tool_profile_version', 'tool_profile_sha256', 'run_id', 'created_at', 'expires_at', 'valid', 'reason'))
        plan['stored_row'] = helper.pick(pr, ('id', 'tenant_id', 'project_id', 'run_id', 'version', 'state', 'created_by', 'created_at', 'expires_at'))
        plan['stored_payload_receipt'] = raw_receipt(helper, pr['payload'])
        plan['workflow'] = helper.workflow(raw_plan['workflow'])
        assert plan['stored_row']['state'] == 'started' and plan['run_id'] == rid
        assert plan['tool_profile_id'] == EXPECTED_PROFILE and plan['tool_profile_version'] == 4 and plan['tool_profile_sha256'] == current_profile['profile']['profile_sha256']
        context = raw_plan['context']['task']
        assert context['id'] == plan['task_id'] and context['version'] == plan['task_version']
        # Only identity/version and a byte receipt are retained, never task prompt text.
        plan['context_basis'] = {'task_id': context['id'], 'task_version': context['version']}
        published = helper.rows(db, 'SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?', (tenant, PROJECT, run['workflow_id'], run['workflow_version']))
        assert len(published) == 1
        run['workflow_definition'] = helper.workflow(json.loads(published[0]['spec']))
        run['stored_workflow_receipt'] = raw_receipt(helper, published[0]['spec'])
        assert run['workflow_definition']['nodes'] == plan['workflow']['nodes'] and run['workflow_definition']['edges'] == plan['workflow']['edges']
        nodes = []
        for row in helper.rows(db, 'SELECT run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec FROM dag_nodes WHERE run_id=? ORDER BY id', (rid,)):
            node = helper.pick(row, ('run_id', 'id', 'state', 'reason', 'attempt', 'fence', 'worker_id', 'error_text', 'started_at', 'completed_at', 'lease_until', 'wake_at', 'parent_id'))
            node['spec'] = helper.node_spec(json.loads(row['spec']))
            node['stored_spec_receipt'] = raw_receipt(helper, row['spec'])
            node['stored_output_receipt'] = raw_receipt(helper, row['output'])
            nodes.append(node)
        assert len(nodes) == 2
        agent = next(n for n in nodes if n['id'] == plan['agent_node_id'])
        quality = next(n for n in nodes if n['id'] != agent['id'])
        assert agent['state'] == quality['state'] == 'cancelled' and agent['attempt'] == 1 and quality['attempt'] == 0
        assert agent['spec']['config']['tool_profile_id'] == EXPECTED_PROFILE and agent['spec']['config']['tool_profile_sha256'] == plan['tool_profile_sha256']
        assert agent['spec']['command'] == [current_profile['profile']['command_id']]
        attempt_rows = helper.rows(db, 'SELECT run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result FROM dag_attempts WHERE run_id=? ORDER BY node_id,attempt', (rid,))
        assert len(attempt_rows) == 1
        a = attempt_rows[0]
        result = json.loads(a['result'])
        attempt = helper.pick(a, ('run_id', 'node_id', 'attempt', 'fence', 'worker_id', 'state', 'started_at', 'completed_at'))
        attempt['result'] = helper.pick(result, ('run_id', 'node_id', 'attempt', 'fence', 'worker_id', 'operation_id', 'exit_code', 'error', 'outcome_unknown', 'stopped'))
        attempt['result']['tool_evidence'] = helper.tool_evidence(result.get('tool_evidence'))
        attempt['result']['workspace_evidence'] = helper.pick(result.get('workspace_evidence') or {}, helper.WORKSPACE_FIELDS)
        attempt['stored_result_receipt'] = raw_receipt(helper, a['result'])
        attempt['stored_output_receipt'] = raw_receipt(helper, result.get('output', ''))
        assert a['node_id'] == agent['id'] and a['attempt'] == agent['attempt'] and a['fence'] == agent['fence'] and a['worker_id'] == agent['worker_id']
        assert attempt['state'] == 'cancelled', 'Confirmed stop must never be a successful attempt'
        assert result['stopped'] is True and result['outcome_unknown'] is False
        assert attempt['result']['tool_evidence']['terminal'] == 'unknown' and attempt['result']['tool_evidence']['profile'] == current_profile['profile']
        assert attempt['result']['workspace_evidence']['changed_paths_total'] == 0
        assert not helper.scalar(db, 'SELECT count(*) FROM dag_candidate_revisions WHERE run_id=?', (rid,))
        events = []
        for row in helper.rows(db, 'SELECT sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data FROM dag_events WHERE run_id=? ORDER BY sequence', (rid,)):
            raw = json.loads(row['data'])
            event = helper.pick(row, ('sequence', 'tenant_id', 'project_id', 'run_id', 'node_id', 'event_type', 'at_time'))
            event['data'] = None if raw is None else helper.pick(raw, ('workflow_version', 'attempt', 'fence', 'previous_state', 'reason', 'operation_id', 'worker_id', 'message', 'commit', 'evidence_sha256', 'source_attempt', 'source_fence', 'source_node'))
            if isinstance(raw, dict) and 'tool_event' in raw:
                event['data']['tool_event'] = helper.tool_event(raw['tool_event'])
            event['stored_data_receipt'] = raw_receipt(helper, row['data'])
            if event['event_type'] == 'tool_progress':
                fact = event['data']['tool_event']
                assert fact['profile_sha256'] == plan['tool_profile_sha256'] and fact['command_id'] == current_profile['profile']['command_id']
                assert event['data']['attempt'] == agent['attempt'] and event['data']['fence'] == agent['fence'] and event['data']['worker_id'] == agent['worker_id']
            events.append(event)
        assert {'command_cancel', 'stop_requested', 'run_cancelled'} <= {e['event_type'] for e in events}
        assert not any(e['event_type'] == 'run_succeeded' for e in events)
        business = {name: project_business(db, helper, tenant, 'suma_'+name) for name in ('tasks', 'issues', 'requirements')}
        assert len(business['tasks']) == 3 and len(business['issues']) == 1 and len(business['requirements']) == 2
        accepted = next(t for t in business['tasks'] if t['id'] == EXPECTED_ACCEPTED_TASK)
        old_accepted = next(t for t in baseline['tasks'] if t['id'] == EXPECTED_ACCEPTED_TASK)
        assert accepted['status'] == 'accepted' and accepted['version'] == 3 and accepted['run_id'] == OLD_RUNS['success'] and accepted['data']['verified_run_id'] == OLD_RUNS['success']
        assert accepted['stored_data_receipt'] == old_accepted['stored_data_receipt']
        stop_task = next(t for t in business['tasks'] if t['id'] == plan['task_id'])
        assert stop_task['run_id'] == rid and stop_task['status'] != 'accepted'
        for name in ('issues', 'requirements'):
            assert {x['id'] for x in business[name]} == {x['id'] for x in baseline[name]}
            for now in business[name]:
                old = next(x for x in baseline[name] if x['id'] == now['id'])
                assert helper.pick(now, ('id', 'version', 'status', 'run_id')) == helper.pick(old, ('id', 'version', 'status', 'run_id'))
                assert now['stored_data_receipt'] == old['stored_data_receipt']
        issue = business['issues'][0]
        assert issue['id'] == EXPECTED_ISSUE and issue['data']['run_id'] == OLD_RUNS['failure']
        intakes = [r for r in business['requirements'] if r['data'].get('source_issue_id') == EXPECTED_ISSUE]
        assert len(intakes) == 1 and intakes[0]['data']['source']['run_id'] == OLD_RUNS['failure']
        reviews = helper.rows(db, 'SELECT id,tenant_id,project_id,task_id,task_version,run_id,reviewer_id,decision,reason,created_at FROM suma_reviews WHERE tenant_id=? AND project_id=? ORDER BY created_at,id', params)
        assert len(reviews) == 1 and [helper.pick(r, list(r)) for r in reviews] == baseline['reviews']
        assert reviews[0]['task_id'] == EXPECTED_ACCEPTED_TASK and reviews[0]['decision'] == 'accepted' and reviews[0]['run_id'] == OLD_RUNS['success']
        return {
            'schema_version': 1, 'stage': 'T-P4-004-post-review', 'observed_at_database_utc': observed,
            'collection': {'read_only': True, 'sqlite_open_mode': 'ro', 'query_only': bool(helper.scalar(db, 'PRAGMA query_only')), 'consistent_read_transaction': True, 'database_archived': False, 'baseline_sha256': BASELINE_SHA, 'historical_helper_sha256': OLD_COLLECTOR_SHA, 'collector_sha256': digest(Path(__file__).read_bytes()), 'excluded': ['auth/session/user/password/token/key tables and columns', 'private context', 'process arguments and environment', 'runner command config', 'raw goal/stdout/stderr', 'unknown JSON fields'], 'redaction_counts': {k: v for k, v in helper.REDACTIONS.items() if v}},
            'project': helper.pick(project[0], ('id', 'tenant_id', 'version', 'title', 'status')),
            'counts': counts | {'new_run_nodes': len(nodes), 'new_run_attempts': 1, 'new_run_events': len(events), 'tasks': 3, 'issues': 1, 'requirements': 2, 'reviews': 1},
            'historical_runs': historical, 'new_run': helper.pick(run, ('id', 'tenant_id', 'project_id', 'workflow_id', 'workflow_version', 'workflow_name', 'version', 'state', 'created_at', 'updated_at')) | {'workflow_definition': run['workflow_definition'], 'stored_workflow_receipt': run['stored_workflow_receipt']},
            'new_plan': plan, 'nodes': nodes, 'attempts': [attempt], 'events': events, 'event_counts': dict(collections.Counter(e['event_type'] for e in events)),
            'profile_now': current_profile, 'business': business,
            'journey_assertions': {'four_runs_eight_nodes_five_attempts': True, 'original_three_run_states_and_payloads_unchanged': True, 'current_stop_result_is_not_success': True, 'confirmed_stopped_unknown_protocol_cancelled_no_test_candidate_or_acceptance': True, 'original_accepted_task_v3_review_unchanged': True, 'original_single_failure_issue_and_single_intake_requirement_unchanged': True, 'profile_paused_v5_new_plan_pinned_v4_old_plans_v2': True, 'browser_actions': 'DB facts alone do not prove GUI actions; root records browser evidence separately', 'real_vendor_model_execution': False},
        }
    finally:
        db.rollback()
        db.close()


def run_metadata(command: list[str]) -> str:
    proc = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, encoding='utf-8', timeout=20, creationflags=getattr(subprocess, 'CREATE_NO_WINDOW', 0))
    if proc.returncode:
        raise RuntimeError('Read-only process metadata query failed; raw command output withheld')
    return proc.stdout.strip()


def collect_runtime(args, helper, database: dict, database_out: Path) -> dict:
    receipt_dir = ROOT / 'dist/runtime/builds' / args.build_stamp
    manifest_path, source_path = receipt_dir/'manifest.json', receipt_dir/'source-manifest.json'
    manifest, sources = (json.loads(p.read_text(encoding='utf-8')) for p in (manifest_path, source_path))
    assert manifest['stamp'] == args.build_stamp and manifest['status'] == 'succeeded' and manifest['source_unchanged'] is True
    assert HEX64.fullmatch(manifest['source_sha256'])
    source_rows = sources if isinstance(sources, list) else sources.get('files', sources.get('sources', []))
    assert len(source_rows) == 116
    for row in source_rows:
        path = (ROOT/row['path']).resolve()
        assert path.is_relative_to(ROOT) and path.is_file() and digest(path.read_bytes()) == row['sha256'], 'Source differs from delivery cut'
    target = next(t for t in manifest['targets'] if t['target'] == 'windows-amd64')
    assert target['status'] == 'succeeded'
    expected_dir = (ROOT/'dist/runtime/windows-amd64'/args.build_stamp).resolve()
    assert expected_dir.is_relative_to(ROOT) and (ROOT/target['directory']).resolve() == expected_dir
    artifacts = {a['name']: a for a in target['artifacts']}
    script = "$ErrorActionPreference='Stop'; @{ processes=@(Get-CimInstance Win32_Process -Filter 'ProcessId=%d OR ProcessId=%d' | Select-Object ProcessId,ExecutablePath); listeners=@(Get-NetTCPConnection -LocalPort 4322 -State Listen | Select-Object LocalAddress,LocalPort,OwningProcess) } | ConvertTo-Json -Depth 5 -Compress" % (args.control_pid, args.runner_pid)
    observation = json.loads(run_metadata(['pwsh', '-NoLogo', '-NoProfile', '-NonInteractive', '-Command', script]))
    processes = {p['ProcessId']: p for p in observation['processes']}
    assert set(processes) == {args.control_pid, args.runner_pid}
    binaries = []
    for name, pid in [('suma-control.exe', args.control_pid), ('suma-runner.exe', args.runner_pid)]:
        path = expected_dir/name
        assert Path(processes[pid]['ExecutablePath']).resolve() == path.resolve()
        raw = path.read_bytes()
        assert digest(raw) == artifacts[name]['sha256'] and len(raw) == artifacts[name]['size_bytes']
        binaries.append({'program': name, 'pid': pid, 'executable_path': str(path), 'sha256': digest(raw), 'size_bytes': len(raw), 'process_image_path_matches': True, 'delivery_manifest_matches': True})
    assert observation['listeners'] == [{'LocalAddress': '127.0.0.1', 'LocalPort': 4322, 'OwningProcess': args.control_pid}]
    web = (ROOT/'apps/web/dist').resolve()
    index = web/'index.html'
    raw_index = index.read_bytes()
    references = re.findall(r'(?:src|href)="(/assets/[^"?#]+)"', raw_index.decode('utf-8'))
    assert references and len(references) == len(set(references))
    assets = []
    for reference in references:
        path = (web/reference.lstrip('/')).resolve()
        assert path.is_relative_to(web) and path.is_file()
        assets.append({'reference': reference, 'path': str(path), **helper.bytes_receipt(path.read_bytes())})
    return {'schema_version': 1, 'stage': 'T-P4-004-post-review', 'observed_at_utc': dt.datetime.now(dt.timezone.utc).isoformat(), 'read_only': True, 'runtime_cut': args.build_stamp, 'source_sha256': manifest['source_sha256'], 'current_runtime_source_files_verified': len(source_rows), 'binaries': binaries, 'listeners': observation['listeners'], 'runner_port': None, 'runner_port_note': 'Outbound Runner; no listener claim.', 'source_manifest': {'path': str(source_path), **helper.bytes_receipt(source_path.read_bytes())}, 'delivery_manifest': {'path': str(manifest_path), **helper.bytes_receipt(manifest_path.read_bytes())}, 'web': {'entry': {'path': str(index), **helper.bytes_receipt(raw_index)}, 'entry_assets': assets, 'scope': 'Current on-disk web bytes only; browser responses captured separately by root.'}, 'database_snapshot': {'path': database_out.relative_to(ROOT).as_posix(), 'new_run_id': args.run_id, 'observed_at_database_utc': database['observed_at_database_utc']}, 'collector_sha256': digest(Path(__file__).read_bytes()), 'scope': 'Actual running executable paths and their on-disk delivery hashes plus loopback listener ownership. No service changed or model/task invoked; no process arguments or environment read. File hashes do not independently measure in-memory image pages.'}


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--build-stamp', required=True)
    parser.add_argument('--control-pid', type=int, required=True)
    parser.add_argument('--runner-pid', type=int, required=True)
    parser.add_argument('--output-prefix', default='tool-post-review')
    args = parser.parse_args()
    assert str(ROOT).lower().replace('\\', '/') == 'd:/codes/ai-dev-os/suma' and DB.is_file()
    assert HEX32.fullmatch(args.run_id) and args.run_id not in OLD_RUNS.values()
    assert STAMP.fullmatch(args.build_stamp) and args.build_stamp != '20261024T045219750Z-715ba2c8'
    assert args.control_pid > 0 and args.runner_pid > 0 and args.control_pid != args.runner_pid
    assert re.fullmatch(r'[A-Za-z0-9_-]{1,64}', args.output_prefix)
    database_out, live_out = (STAGE/(args.output_prefix+suffix) for suffix in ('-database.json', '-runtime.json'))
    assert not database_out.exists() and not live_out.exists(), 'Create-only observations; original files never overwritten'
    helper = load_helpers()
    baseline = read_baseline()
    database = collect_database(args, helper, baseline)
    live = collect_runtime(args, helper, database, database_out)
    assert digest(BASELINE.read_bytes()) == BASELINE_SHA and digest(OLD_COLLECTOR.read_bytes()) == OLD_COLLECTOR_SHA
    for path, data in [(database_out, database), (live_out, live)]:
        raw = (json.dumps(data, ensure_ascii=False, indent=2)+'\n').encode('utf-8')
        assert not re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----|\bsk-[A-Za-z0-9_-]{20,}|\bgh[pousr]_[A-Za-z0-9]{20,}|Bearer\s+[A-Za-z0-9_.-]{12,}', raw)
        with path.open('xb') as output:
            output.write(raw)
    print(json.dumps({'read_only': True, 'new_run_id': args.run_id, 'runtime_cut': args.build_stamp, 'counts': database['counts'], 'journey_assertions': database['journey_assertions'], 'files': [{'path': p.relative_to(ROOT).as_posix(), **helper.bytes_receipt(p.read_bytes())} for p in (database_out, live_out)], 'collector_sha256': digest(Path(__file__).read_bytes())}, ensure_ascii=False))


if __name__ == '__main__':
    main()
