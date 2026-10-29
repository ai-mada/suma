// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect, it, vi} from 'vitest';
import type {CodeBaselineBinding, ToolProfile, WorkspaceTerminalCandidatePlan, WorkspaceTerminalPage, WorkspaceTerminalSession} from '../../../packages/contracts/src';
import {TerminalPanel} from './TerminalPanel';
import {TerminalBaselineFacts} from './TerminalBaselineFacts';
import {validTerminalPage, validTerminalSession} from './terminal-model';
import {terminalCandidateMatchesSession, validTerminalCandidatePlan} from './terminal-candidate-model';
import {setLocale} from './i18n';

const harness = vi.hoisted(() => ({transport: vi.fn()}));
vi.mock('./api-context', () => ({useApi: () => harness.transport, useApiView: () => ({userID: 'owner-a', organizationID: 'org-a'})}));
vi.mock('./TerminalViewport', () => ({TerminalViewport: () => null}));
const project = 'project-a', task = 'task-a', stamp = '2026-10-29T12:00:00Z', originalB = 'a'.repeat(40), adoptedM = 'b'.repeat(40), candidateN = 'c'.repeat(40);
const binding: CodeBaselineBinding = {protocol: 'suma.code-baseline.v1', id: 'baseline-a', project_id: project, task_id: task, task_version: 5,
  terminal_candidate_plan_id: 'accepted-plan-a', terminal_candidate_plan_version: 2, accepted_basis_sha256: 'd'.repeat(64), binding_sha256: 'e'.repeat(64),
  source: {schema_version: 1, transfer_id: 'transfer-a', project_id: project, state: 'sealed', base_commit: originalB, head_commit: adoptedM,
    evidence_sha256: 'f'.repeat(64), bundle_sha256: 'a'.repeat(64), size_bytes: 100,
    source: {run_id: 'accepted-run-a', node_id: 'check', attempt: 1, fence: 4, worker_id: 'worker-a'}}};
const terminal: WorkspaceTerminalSession = {id: 'terminal-a', project_id: project, task_id: task, task_version: 5, version: 2, run_id: 'terminal-run-a', run_version: 1,
  state: 'stopped', tool_profile_id: 'shell-a', tool_name: 'Owned shell', directory: 'attempt-worktree', worker_id: 'worker-a', columns: 80, rows: 24,
  control_epoch: 1, next_input_sequence: 1, output_first_sequence: 1, output_last_sequence: 0, output_ack_sequence: 0,
  expires_at: stamp, created_at: stamp, allowed_actions: [], reason: '', source_snapshot_sha256: 'b'.repeat(64),
  base_commit: adoptedM, admin_base_commit: originalB, code_baseline: binding};
const tool: ToolProfile = {id: 'shell-a', project_id: project, worker_id: 'worker-a', name: 'Owned shell', version: 2, state: 'enabled',
  profile: {command_id: 'owned-shell', adapter: 'command', protocol: 'text-v1', tool_version: '1', read_only: false, terminal_session_protocol: 'suma-pty-v1',
    profile_sha256: 'c'.repeat(64), executable_sha256: 'd'.repeat(64)}, enabled_by: 'owner-a', created_at: stamp, updated_at: stamp, available: true, reason: ''};
const pageFixture = (): WorkspaceTerminalPage => ({items: [], profiles: [tool], task_version: 5, can_create: true, profiles_complete: true,
  code_baseline: binding, baseline_current: true, baseline_reason: ''});
function planFixture(): WorkspaceTerminalCandidatePlan {
  const source = {session_id: terminal.id, session_version: terminal.version, original: {run_id: terminal.run_id, node_id: 'terminal', attempt: 1, fence: 1, worker_id: terminal.worker_id},
    base_commit: adoptedM, admin_base_commit: originalB, code_baseline: binding, snapshot_sha256: terminal.source_snapshot_sha256, plan_id: 'plan-a'};
  return {id: source.plan_id, project_id: project, version: 1, state: 'ready', session_id: terminal.id, session_version: terminal.version, task_id: task, task_version: 5,
    run_id: 'candidate-run-a', created_at: stamp, expires_at: stamp, tool_profile_id: 'check-profile', tool_profile_version: 1, tool_profile_sha256: 'e'.repeat(64),
    worker_id: terminal.worker_id, workflow_id: 'flow-a', workflow_version: 1, producer_node_id: 'check', source,
    context: {task: {id: task, version: 5, title: 'Formal task', description: '', acceptance_criteria: ['Preserve adopted source']}, code_baseline: binding},
    workflow: {id: 'fixed-flow-a', name: 'Fixed checks', version: 1, edges: [], nodes: [{id: 'check', name: 'Check', type: 'command', command: ['check'],
      depends_on: [], join: 'all', quorum: 0, timeout_seconds: 60, max_retries: 0, idempotent: false, config: {terminal_candidate_source: source}}]}, valid: true, reason: ''};
}
const settle = async () => {for (let index = 0; index < 20; index++) await Promise.resolve();};
async function click(element: Element | null) {expect(element).not.toBeNull(); await act(async () => {element!.dispatchEvent(new MouseEvent('click', {bubbles: true})); await settle();});}

it('keeps original B, adopted M and frozen source binding consistent without treating stale qualification as current', () => {
  expect(validTerminalSession(terminal, project, task)).toBe(true);
  expect(validTerminalSession({...terminal, base_commit: originalB}, project, task)).toBe(false);
  expect(validTerminalSession({...terminal, admin_base_commit: adoptedM}, project, task)).toBe(false);
  const {code_baseline: adoptedBinding, ...withoutBinding} = terminal;
  expect(adoptedBinding?.source.head_commit).toBe(adoptedM);
  const unbound = {...withoutBinding, base_commit: originalB, admin_base_commit: originalB};
  expect(validTerminalSession(unbound, project, task)).toBe(true);
  expect(validTerminalPage(pageFixture(), project, task)).toBe(true);
  expect(validTerminalPage({...pageFixture(), baseline_current: false, can_create: false, baseline_reason: 'Original accepted source changed'}, project, task)).toBe(true);
  expect(validTerminalPage({...pageFixture(), baseline_current: false, baseline_reason: 'Original accepted source changed'}, project, task)).toBe(false);
  expect(validTerminalPage({...pageFixture(), baseline_current: false, can_create: false}, project, task)).toBe(false);
  const plan = planFixture();
  expect(validTerminalCandidatePlan(plan, project, task, terminal.id)).toBe(true);
  expect(terminalCandidateMatchesSession(plan, terminal)).toBe(true);
  expect(terminalCandidateMatchesSession(plan, {...terminal, code_baseline: {...binding, binding_sha256: 'f'.repeat(64)}})).toBe(false);
  expect(validTerminalCandidatePlan({...plan, context: {...plan.context, code_baseline: {...binding, binding_sha256: 'f'.repeat(64)}}}, project, task, terminal.id)).toBe(false);
});

it('shows the adopted commit before creation and revokes shell consent across invalidation and refresh', async () => {
  await setLocale('en-US', {persist: false}); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host), writes: Record<string, unknown>[] = [];
  let page = pageFixture();
  harness.transport.mockImplementation(async (path: string, body?: Record<string, unknown>) => {
    if (body) {writes.push(body); return {...terminal, task_version: page.task_version, code_baseline: page.code_baseline, state: 'queued', base_commit: '', admin_base_commit: '', source_snapshot_sha256: ''};}
    if (path.startsWith('/workspace/terminal-sessions?')) return page;
    return {...terminal, task_version: page.task_version, code_baseline: page.code_baseline, state: 'queued', base_commit: '', admin_base_commit: '', source_snapshot_sha256: ''};
  });
  try {
    await act(async () => {root.render(<TerminalPanel project={project} taskId={task}/>); await settle();});
    await click(host.querySelector('[data-terminal-toggle]'));
    expect(host.querySelector('[data-terminal-admin-base]')?.textContent).toBe(originalB);
    expect(host.querySelector('[data-terminal-selected-base]')?.textContent).toBe(adoptedM);
    const profile = host.querySelector<HTMLSelectElement>('.terminal-create select')!;
    await act(async () => {Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!.call(profile, tool.id); profile.dispatchEvent(new Event('change', {bubbles: true})); await settle();});
    await click(host.querySelector('.terminal-create-consent input'));
    expect(host.querySelector<HTMLButtonElement>('[data-terminal-create]')!.disabled).toBe(false);
    const changed = {...binding, task_version: 6, binding_sha256: 'f'.repeat(64), source: {...binding.source, head_commit: 'd'.repeat(40)}};
    page = {...page, task_version: 6, code_baseline: changed, can_create: false, baseline_current: false, baseline_reason: 'Original accepted source changed'};
    await click(host.querySelector('.terminal-session-list button'));
    expect(host.querySelector('[data-terminal-baseline-invalid]')?.textContent).toContain('Original accepted source changed');
    expect(host.querySelector('[data-terminal-selected-base]')?.textContent).toBe(changed.source.head_commit);
    expect(host.querySelector('[data-terminal-create]')).toBeNull(); expect(writes).toHaveLength(0);
    page = {...page, can_create: true, baseline_current: true, baseline_reason: ''};
    await click(host.querySelector('.terminal-session-list button'));
    expect(host.querySelector<HTMLInputElement>('.terminal-create-consent input')!.checked).toBe(false);
    expect(host.querySelector<HTMLButtonElement>('[data-terminal-create]')!.disabled).toBe(true);
    await click(host.querySelector('.terminal-create-consent input')); await click(host.querySelector('[data-terminal-create]'));
    expect(writes).toHaveLength(1); expect(writes[0].expected_task_version).toBe(6); expect(writes[0]).not.toHaveProperty('base_commit');
  } finally {await act(async () => root.unmount()); host.remove(); sessionStorage.clear(); vi.unstubAllGlobals(); vi.restoreAllMocks();}
});

it('displays N only when an actual candidate is supplied and leaves an unreported administrator baseline unknown', async () => {
  await setLocale('en-US', {persist: false}); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host);
  try {
    await act(async () => {root.render(<TerminalBaselineFacts project={project} binding={binding}/>);});
    expect(host.querySelector('[data-terminal-admin-base]')?.textContent).toBe(originalB); expect(host.querySelector('[data-terminal-selected-base]')?.textContent).toBe(adoptedM);
    expect(host.querySelector('[data-terminal-new-candidate]')).toBeNull();
    await act(async () => {root.render(<TerminalBaselineFacts project={project} binding={binding} candidateCommit={candidateN}/>);});
    expect(host.querySelector('[data-terminal-new-candidate]')?.textContent).toBe(candidateN);
    await act(async () => {root.render(<TerminalBaselineFacts project={project}/>);});
    expect(host.querySelector('code')).toBeNull(); expect(host.textContent).toContain('Exact commit not yet reported');
  } finally {await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals(); vi.restoreAllMocks();}
});
