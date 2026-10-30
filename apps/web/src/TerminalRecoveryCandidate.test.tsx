// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect, it, vi} from 'vitest';
import type {CandidateBinding, CandidateCompositionBaseline, ToolProfile, Workflow, WorkspaceTerminalSession, WorkspaceTerminalRecoveryView, WorkspaceTerminalCandidatePlan} from '../../../packages/contracts/src';
import {TerminalRecoveryPanel} from './TerminalRecoveryPanel';
import {TerminalCandidatePanel} from './TerminalCandidatePanel';
import {TerminalCandidateHandoffPanel} from './TerminalCandidateHandoffPanel';
import {terminalCandidateProducers, terminalCandidatePreviewReceipt, validTerminalCandidatePlan} from './terminal-candidate-model';
import {validTerminalRecoveryView, validTerminalRecoverReceipt, terminalRecoveryReason} from './terminal-recovery-model';
import {setLocale} from './i18n';
import {compositionCodeBaseline, compositionRecovery} from './candidate-composition-model';

const harness = vi.hoisted(() => ({transport: vi.fn()}));
vi.mock('./api-context', () => ({useApi: () => harness.transport}));
const stamp = '2026-10-29T12:00:00Z', project = 'project-a', task = 'task-a';
const terminal: WorkspaceTerminalSession = {id: 'terminal-a', project_id: project, task_id: task, task_version: 3, version: 7, run_id: 'terminal-run-a', run_version: 4,
  state: 'reconciling', tool_profile_id: 'shell-a', tool_name: 'Original shell', directory: 'attempt-worktree', worker_id: 'worker-a', columns: 80, rows: 24,
  control_epoch: 1, next_input_sequence: 3, output_first_sequence: 1, output_last_sequence: 2, output_ack_sequence: 2,
  expires_at: stamp, created_at: stamp, allowed_actions: [], reason: '', source_snapshot_sha256: 'e'.repeat(64), base_commit: 'a'.repeat(40), admin_base_commit: 'a'.repeat(40)};
const recovery: WorkspaceTerminalRecoveryView = {session_id: terminal.id, run_id: terminal.run_id, run_version: 4, run_state: 'reconciling', attempt: 1,
  fence: 8, dispatch_id: 'dispatch-original-a', tree_stopped: true, can_recover: true, reason: '',
  unknown_inputs: [{input_id: 'input-one', sequence: 1, sha256: 'a'.repeat(64)}, {input_id: 'input-two', sequence: 2, sha256: 'b'.repeat(64)}]};
const flow: Workflow = {id: 'flow-a', name: 'Check then independently test', version: 2, nodes: [
  {id: 'check', name: 'Fixed check', type: 'command', command: ['fixed-check'], depends_on: [], join: 'all', quorum: 0, timeout_seconds: 300, max_retries: 0, idempotent: false},
  {id: 'verify', name: 'Independent test', type: 'test', command: ['fixed-test'], depends_on: ['check'], join: 'all', quorum: 0, timeout_seconds: 300, max_retries: 0, idempotent: false,
    config: {candidate_from: 'check', tool_profile_id: 'test-profile', tool_profile_sha256: 'f'.repeat(64)}}], edges: [{from: 'check', to: 'verify'}]};
const tool: ToolProfile = {id: 'check-profile', project_id: project, worker_id: terminal.worker_id, version: 2, name: 'Fixed check profile', state: 'enabled',
  profile: {command_id: 'fixed-check', adapter: 'command', protocol: 'text-v1', read_only: false, profile_sha256: 'c'.repeat(64), executable_sha256: 'd'.repeat(64), tool_version: '1'},
  enabled_by: 'owner-a', created_at: stamp, updated_at: stamp, available: true, reason: ''};
function planFixture(): WorkspaceTerminalCandidatePlan {
  const source = {session_id: terminal.id, session_version: 7, original: {run_id: terminal.run_id, node_id: 'terminal', attempt: 1, fence: 8, worker_id: terminal.worker_id},
    base_commit: 'a'.repeat(40), admin_base_commit: 'a'.repeat(40), snapshot_sha256: 'e'.repeat(64), plan_id: 'candidate-plan-a'};
  return {id: source.plan_id, project_id: project, version: 1, state: 'ready', session_id: terminal.id, session_version: 7, task_id: task, task_version: 3,
    run_id: 'candidate-run-a', created_at: stamp, expires_at: '2099-10-29T12:00:00Z', tool_profile_id: tool.id, tool_profile_version: 2, tool_profile_sha256: tool.profile.profile_sha256,
    worker_id: terminal.worker_id, workflow_id: flow.id, workflow_version: 2, producer_node_id: 'check',
    context: {task: {id: task, version: 3, title: 'Formal task', description: 'Original scope', acceptance_criteria: ['Original formal criterion']}}, source,
    workflow: {...flow, nodes: [{...flow.nodes[0], config: {terminal_candidate_source: source}}, flow.nodes[1]]}, valid: true, reason: ''};
}
const candidateFixture = (): CandidateBinding => ({schema_version: 1, transfer_id: 'transfer-a', project_id: project, state: 'sealed',
  source: {run_id: 'candidate-run-a', node_id: 'check', attempt: 1, fence: 1, worker_id: 'worker-a'}, base_commit: 'a'.repeat(40), head_commit: 'f'.repeat(40),
  evidence_sha256: 'c'.repeat(64), bundle_sha256: 'd'.repeat(64), size_bytes: 100});
const settle = async () => {for (let index = 0; index < 18; index++) await Promise.resolve();};
async function click(element: Element | null) {expect(element).not.toBeNull(); await act(async () => {element!.dispatchEvent(new MouseEvent('click', {bubbles: true})); await settle();});}
async function enter(element: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | null, value: string) {
  expect(element).not.toBeNull(); const prototype = element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  await act(async () => {Object.getOwnPropertyDescriptor(prototype, 'value')!.set!.call(element, value); element!.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', {bubbles: true})); await settle();});
}

it('rejects unproven recovery, success receipts, lost unknown inputs and mismatched candidate provenance', () => {
  expect(validTerminalRecoveryView({...recovery, tree_stopped: false}, terminal.id, terminal.run_id)).toBe(false);
  const request = {path: '/workspace/terminal-sessions/terminal-a/recover', method: 'POST', idempotency_key: 'original-operation-a', body: {
    project_id: project, expected_version: 7, expected_run_version: 4, run_id: terminal.run_id, attempt: 1, fence: 8, dispatch_id: recovery.dispatch_id,
    unknown_inputs: recovery.unknown_inputs, reason: 'Reviewed unknown effects', idempotency_key: 'original-operation-a'}};
  const receipt = {session: {...terminal, version: 8, state: 'stopped'}, recovery: {...recovery, run_version: 5, run_state: 'failed', can_recover: false}};
  expect(validTerminalRecoverReceipt(receipt, request, project, task, terminal.id, terminal.run_id)).toBe(true);
  expect(validTerminalRecoverReceipt({...receipt, recovery: {...receipt.recovery, run_state: 'succeeded'}}, request, project, task, terminal.id, terminal.run_id)).toBe(false);
  expect(validTerminalRecoverReceipt({...receipt, recovery: {...receipt.recovery, unknown_inputs: []}}, request, project, task, terminal.id, terminal.run_id)).toBe(false);
  expect(terminalRecoveryReason('核'.repeat(667))).toBe(false);
  const plan = planFixture();
  expect(validTerminalCandidatePlan(plan, project, task, terminal.id)).toBe(true);
  expect(validTerminalCandidatePlan({...plan, source: {...plan.source, plan_id: 'another-plan'}}, project, task, terminal.id)).toBe(false);
  expect(terminalCandidateProducers({...flow, nodes: [...flow.nodes, {...flow.nodes[1], id: 'extra'}]})).toEqual([]);
  const preview = {path: '/workspace/terminal-sessions/terminal-a/candidate-plans', method: 'POST', idempotency_key: 'preview-operation-a', body: {
    expected_session_version: 7, expected_task_version: 3, tool_profile_id: tool.id, expected_tool_version: 2, workflow_id: 'another-workflow', workflow_version: 2, producer_node_id: 'check'}};
  expect(terminalCandidatePreviewReceipt(plan, preview, project, task, terminal.id, terminal.run_id)).toBe(false);
  const binding = {protocol: 'suma.code-baseline.v1', id: 'baseline-a', project_id: project, task_id: 'target-a', task_version: 2,
    terminal_candidate_plan_id: plan.id, terminal_candidate_plan_version: 2, accepted_basis_sha256: 'c'.repeat(64), binding_sha256: 'd'.repeat(64), source: candidateFixture()};
  expect(compositionCodeBaseline(binding, project, 'target-a')).toBe(true);
  expect(compositionCodeBaseline({...binding, composition_id: 'composition-a', composition_version: 1}, project, 'target-a')).toBe(false);
  const bindRequest = {path: '/tasks/target-a/code-baseline', method: 'POST', idempotency_key: 'binding-original-a', body: {project_id: project,
    expected_task_version: 1, expected_binding_version: 0, terminal_candidate_plan_id: plan.id, expected_terminal_candidate_plan_version: 2,
    acknowledge: true, reason: 'Follow-up uses accepted source', idempotency_key: 'binding-original-a'}};
  expect(compositionRecovery(bindRequest, project, '')).toBe(true);
  expect(compositionRecovery({...bindRequest, body: {...bindRequest.body, composition_id: 'composition-a', expected_composition_version: 1}}, project, '')).toBe(false);
});

it('requires individual unknown-input review and explicit failed-only closure without changing delivery facts', async () => {
  await setLocale('en-US', {persist: false}); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host), writes: {path: string; body: Record<string, unknown>}[] = [];
  let current = recovery;
  harness.transport.mockImplementation(async (path: string, body?: Record<string, unknown>) => {
    if (!body) return current;
    writes.push({path, body}); current = {...current, run_version: 5, run_state: 'failed', can_recover: false};
    return {session: {...terminal, state: 'stopped', version: 8}, recovery: current};
  });
  try {
    await act(async () => {root.render(<TerminalRecoveryPanel project={project} task={task} session={terminal} scope="review-scope" active disabled={false}
      onSession={() => {}} onDenied={() => {}} onLockChange={() => {}}/>); await settle();});
    await enter(host.querySelector('textarea'), 'Reviewed original stopped attempt and unresolved input effects');
    const boxes = () => Array.from(host.querySelectorAll<HTMLInputElement>('input[type=checkbox]'));
    const submit = () => host.querySelector<HTMLButtonElement>('[data-terminal-recover]')!;
    await click(boxes()[0]); await click(boxes()[2]); expect(submit().disabled).toBe(true);
    await click(boxes()[1]); expect(submit().disabled).toBe(true); // Input review invalidates final consent.
    await click(boxes()[2]); expect(submit().disabled).toBe(false);
    await click(submit());
    expect(writes).toHaveLength(1); expect(writes[0].path).toBe('/workspace/terminal-sessions/terminal-a/recover');
    expect(writes[0].body.unknown_inputs).toEqual(recovery.unknown_inputs); expect(writes[0].body).not.toHaveProperty('outcome');
    expect(host.textContent).toContain('Unknown inputs remain unknown'); expect(host.textContent).toContain('input-one'); expect(host.textContent).toContain('input-two');
  } finally {await act(async () => root.unmount()); host.remove(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks();}
});

it('previews stopped changes first and requires fresh separate consent to start the pinned check and independent test run', async () => {
  await setLocale('en-US', {persist: false}); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host), writes: {path: string; body: Record<string, unknown>}[] = [];
  let saved: WorkspaceTerminalCandidatePlan | undefined;
  harness.transport.mockImplementation(async (path: string, body?: Record<string, unknown>) => {
    if (body) {
      writes.push({path, body});
      if (path.endsWith('/candidate-plans')) {saved = planFixture(); return saved;}
      saved = {...saved!, state: 'started', version: 2};
      return {id: saved.run_id, project_id: project, workflow_id: 'managed-' + saved.run_id, workflow_version: 1};
    }
    if (path.includes('/recovery?')) return {...recovery, run_state: 'failed', can_recover: false};
    if (path.startsWith('/workspace/terminal-sessions/terminal-a/candidate-plans?')) return {items: saved ? [saved] : [], task_version: 3};
    if (path.startsWith('/workflows?')) return {items: [flow]};
    if (path.startsWith('/tool-profiles?')) return {items: [tool, {...tool, id: 'wrong-worker', worker_id: 'worker-b'}], total: 2};
    return saved;
  });
  try {
    await act(async () => {root.render(<TerminalCandidatePanel project={project} task={task} taskVersion={3} session={{...terminal, state: 'stopped'}} scope="candidate-scope" active disabled={false}
      onDenied={() => {}} onLockChange={() => {}} onRefresh={() => {}}/>); await settle();});
    await click(host.querySelector('[data-terminal-candidate-toggle]'));
    await enter(host.querySelector<HTMLSelectElement>('[data-terminal-candidate-workflow]'), flow.id + '@2');
    expect(host.querySelector<HTMLSelectElement>('[data-terminal-candidate-profile]')!.options).toHaveLength(2); // Placeholder plus original-worker profile only.
    await enter(host.querySelector<HTMLSelectElement>('[data-terminal-candidate-profile]'), tool.id); await click(host.querySelector('input[type=checkbox]'));
    await click(host.querySelector('[data-terminal-candidate-preview]'));
    expect(writes).toHaveLength(1); expect(writes[0].body.acknowledge_stopped_changes).toBe(true);
    expect(host.textContent).toContain('Fixed preview created; not started');
    const start = host.querySelector<HTMLButtonElement>('[data-terminal-candidate-start]')!; expect(start.disabled).toBe(true);
    await click(host.querySelector('input[type=checkbox]')); await click(start);
    expect(writes).toHaveLength(2); expect(writes[1].path).toBe('/workspace/terminal-candidate-plans/candidate-plan-a/start');
    expect(writes[1].body.expected_version).toBe(1); expect(writes.every(write => !write.path.includes('/review'))).toBe(true);
    expect(host.textContent).toContain('Checks and candidate quality workflow started');
    expect(host.querySelector('a')?.getAttribute('href')).toContain('run=candidate-run-a');
    sessionStorage.clear(); saved = {...saved!, valid: false, reason: 'Current source task basis changed'};
    await act(async () => {root.render(<TerminalCandidatePanel key="fresh-readback" project={project} task={task} taskVersion={8} session={{...terminal, state: 'stopped'}} scope="candidate-scope" active disabled
      onDenied={() => {}} onLockChange={() => {}} onRefresh={() => {}}/>); await settle();});
    await click(host.querySelector('[data-terminal-candidate-toggle]'));
    expect(host.textContent).toContain('Checks and candidate quality workflow started'); expect(host.textContent).toContain('Current source task basis changed');
    expect(host.querySelector('a')?.getAttribute('href')).toContain('run=candidate-run-a'); expect(writes).toHaveLength(2);
    expect(host.querySelector('[data-terminal-candidate-preview]')).toBeNull(); expect(host.querySelector('[data-terminal-handoff-toggle]')).not.toBeNull();
  } finally {await act(async () => root.unmount()); host.remove(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks();}
});

it('binds a terminal source only after current formal acceptance of its exact run, without starting the follow-up', async () => {
  await setLocale('en-US', {persist: false}); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host), writes: {path: string; body: Record<string, unknown>}[] = [];
  const plan = {...planFixture(), state: 'started', version: 2}, candidate = candidateFixture();
  const summary = {total: 1, waiting: 0, running: 0, succeeded: 1, failed: 0, skipped: 0, reconciling: 0, other: 0, limit: 20, complete: true,
    items: [{id: 'verify', name: 'Independent tests', type: 'test', state: 'succeeded', attempt: 1}]};
  let accepted = false, targetVersion = 1;
  let baseline: CandidateCompositionBaseline = {project_id: project, task_id: 'target-a', version: 0, can_bind: true, blockers: []};
  harness.transport.mockImplementation(async (path: string, body?: Record<string, unknown>) => {
    if (body) {
      writes.push({path, body}); targetVersion = 2;
      baseline = {...baseline, version: 1, binding: {protocol: 'suma.code-baseline.v1', id: 'baseline-a', project_id: project, task_id: 'target-a', task_version: 2,
        terminal_candidate_plan_id: plan.id, terminal_candidate_plan_version: 2, accepted_basis_sha256: 'c'.repeat(64), binding_sha256: 'd'.repeat(64), source: candidate}};
      return {id: 'target-a', project_id: project, title: 'Follow-up', version: targetVersion, status: 'draft', run_id: ''};
    }
    if (path.startsWith('/tasks/task-a/delivery?')) return {project_id: project, task_id: task, task_version: 4, task_status: accepted ? 'accepted' : 'awaiting_review',
      formal_acceptance: accepted, run_basis_state: 'current', run: {run_id: plan.run_id, version: 5, state: 'succeeded', nodes: summary, tests: summary},
      expert_plans: {scope: 'current_run', total: 0, limit: 20, complete: true, items: []}, issues: {scope: 'current_run', total: 0, limit: 20, complete: true, items: []}};
    if (path.startsWith('/runs/')) return {id: plan.run_id, project_id: project, version: 5, state: 'succeeded', workflow_id: 'managed-' + plan.run_id, workflow_version: 1,
      nodes: [{id: 'check', state: 'succeeded', workspace_evidence: {candidate_bundle: candidate}}, {id: 'verify', state: 'succeeded'}]};
    if (path.startsWith('/tasks?')) return {items: [{id: 'target-a', project_id: project, title: 'Follow-up', version: targetVersion, status: 'draft', run_id: ''},
      {id: 'started-task', project_id: project, title: 'Already started', version: 1, run_id: 'another-run'}], total: 2};
    if (path.startsWith('/tasks/target-a/code-baseline?')) return baseline;
    return {id: 'target-a', project_id: project, title: 'Follow-up', version: targetVersion, status: 'draft', run_id: ''};
  });
  try {
    await act(async () => {root.render(<TerminalCandidateHandoffPanel project={project} plan={plan} scope="handoff-scope" active disabled={false}
      onDenied={() => {}} onLockChange={() => {}} onRefresh={() => {}}/>); await settle();});
    await click(host.querySelector('[data-terminal-handoff-toggle]'));
    expect(host.querySelector('select')).toBeNull(); expect(writes).toHaveLength(0);
    accepted = true; await click(Array.from(host.querySelectorAll('button')).find(button => button.textContent?.includes('Read current acceptance'))!);
    expect(host.querySelectorAll('option')).toHaveLength(2); // Source and already-started tasks are excluded.
    await enter(host.querySelector('select'), 'target-a'); await enter(host.querySelector('textarea'), 'Continue from this formally accepted exact candidate');
    expect(host.querySelector<HTMLButtonElement>('[data-terminal-handoff-bind]')!.disabled).toBe(true);
    await click(host.querySelector('input[type=checkbox]')); await click(host.querySelector('[data-terminal-handoff-bind]'));
    expect(writes).toHaveLength(1); expect(writes[0].path).toBe('/tasks/target-a/code-baseline');
    expect(writes[0].body).toMatchObject({terminal_candidate_plan_id: plan.id, expected_terminal_candidate_plan_version: 2, expected_task_version: 1, expected_binding_version: 0});
    expect(writes[0].body).not.toHaveProperty('composition_id'); expect(host.textContent).toContain('Follow-up execution has not started');
  } finally {await act(async () => root.unmount()); host.remove(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks();}
});
