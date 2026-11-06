// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { TaskDispatchOptions, TaskDispatchToolOption } from '../../../packages/contracts/src';
import type { Session, Workflow, ApiCaller } from './api';
import * as ApiContext from './api-context';
import { ApiViewProvider } from './api-context';
import { setLocale } from './i18n';
import { TaskDispatchDialog } from './TaskDispatch';
import { TaskDispatchToolHandoff } from './TaskDispatchToolHandoff';
import type { DispatchOptionsScope } from './task-dispatch-options-model';
import type { ToolTaskHandoffPin } from './tool-task-handoff-model';
import type { ToolProfile } from './tool-model';

const project = 'project-one', stamp = '2026-11-02T01:00:00Z';
const session: Session = { user: { id: 'user-one', name: 'Owner', email: 'owner@example.test' }, organization: { id: 'org-one', name: 'Team' }, projects: [] };
const definition = { command_id: 'coder', adapter: 'suma', protocol: 'suma-jsonl-v1', profile_sha256: 'a'.repeat(64), executable_sha256: 'b'.repeat(64), tool_version: '1.0.0', read_only: false };
const tool: ToolProfile = { id: 'outside-first-page', project_id: project, worker_id: 'worker-one', name: 'Chosen tool', version: 3, state: 'enabled', profile: definition,
  available: true, reason: '', enabled_by: session.user.id, created_at: stamp, updated_at: stamp, observation: { profile: { ...definition }, available: true, reason: '' } };
const pin: ToolTaskHandoffPin = { project_id: project, id: tool.id, version: tool.version, worker_id: tool.worker_id, profile_sha256: definition.profile_sha256, name: tool.name };
const flow: Workflow = { id: 'flow-one', name: 'Code and verify', version: 2, nodes: [{ id: 'code', name: 'Develop', type: 'agent' }, { id: 'test', name: 'Test independently', type: 'test', command: ['verify'], config: { candidate_from: 'code' } }], edges: [{ from: 'code', to: 'test' }] };
const scope: DispatchOptionsScope = {
  project_id: project, task_id: 'task-one', expected_task_version: 4,
  workflow_id: flow.id, workflow_version: flow.version, agent_node_id: 'code',
};

function exactOption(
  profile: ToolProfile = tool,
  reasonCode: TaskDispatchToolOption['reason_code'] = profile.available ? 'eligible' : 'tool_unavailable',
  reason = profile.reason,
): TaskDispatchOptions {
  return {
    project_id: project, task_id: scope.task_id, task_version: scope.expected_task_version,
    workflow_id: flow.id, workflow_version: flow.version, agent_node_id: scope.agent_node_id,
    observed_at: stamp,
    context: { task: { id: scope.task_id, version: scope.expected_task_version, title: 'Develop task', description: '', acceptance_criteria: [] } },
    execution_route: { tenant_id: session.organization.id, project_id: project, version: 0, pool_id: '', grant_id: '', grant_version: 0 },
    items: [{ tool: profile, eligible: reasonCode === 'eligible', reason_code: reasonCode, reason, model_state: 'fixed_by_tool',
      cost_state: 'unknown', eligible_basis: 'current_observation_and_compatibility' }],
    total: 1, next_cursor: '', recommended_tool_profile_id: reasonCode === 'eligible' ? profile.id : '',
  };
}

let root: Root, host: HTMLDivElement;
beforeEach(async () => { await setLocale('en-US', { persist: false }); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); host = document.createElement('div'); document.body.append(host); root = createRoot(host); });
afterEach(async () => { await act(async () => root.unmount()); host.remove(); document.body.innerHTML = ''; sessionStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
async function settle() { await act(async () => { for (let i = 0; i < 32; i++) await Promise.resolve(); }); }
async function click(selector: string) { const element = document.querySelector<HTMLElement>(selector); expect(element, selector).not.toBeNull(); await act(async () => element!.click()); await settle(); }
async function select(selector: string, value: string) { const node = document.querySelector<HTMLSelectElement>(selector)!; await act(async () => { Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!.call(node, value); node.dispatchEvent(new Event('change', { bubbles: true })); }); await settle(); }

it('T213 explicitly adopts an exact off-page tool into the original dispatch scope and preserves the original pending preview', async () => {
  const requests: Array<{ path: string; method: string; body?: Record<string, unknown> }> = [];
  const manual = { ...tool, id: 'manual-tool', name: 'Manual tool' };
  let currentTool = tool, deferred: ((response: Response) => void) | undefined, delayTool = false;
  vi.stubGlobal('fetch', vi.fn(async (input: string, init: RequestInit = {}) => {
    const path = String(input).replace('/api/v1', ''), method = init.method || 'GET';
    requests.push({ path, method, body: init.body ? JSON.parse(String(init.body)) : undefined });
    if (method !== 'GET') throw new Error('Preview response lost after sending original operation');
    if (path.startsWith('/workflows?')) return Response.json({ items: [flow, { ...flow, id: 'flow-two', name: 'Alternate flow' }] });
    if (path.startsWith('/tool-profiles?')) return Response.json({ items: [manual], total: 2, next_cursor: 'page-two' });
    if (path.startsWith('/tasks/' + scope.task_id + '/dispatch-options?')) {
      return delayTool ? new Promise<Response>(resolve => { deferred = resolve; }) : Response.json(exactOption(currentTool));
    }
    return Response.json({ items: [], total: 0, next_cursor: '' });
  }));
  await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={{ id: 'task-one', project_id: project, version: 4, title: 'Develop task' }} initialWorkflow={flow} handoffTool={pin} onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()} /></ApiViewProvider>)); await settle();
  expect(requests.some(item => item.path.includes('/dispatch-options?'))).toBe(false);
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe('');
  await click('[data-dispatch-handoff-adopt]');
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe(pin.id);
  expect(document.querySelector('[data-dispatch-handoff-status=adopted]')).not.toBeNull();
  expect(requests.filter(item => item.method !== 'GET')).toEqual([]);
  const check = requests.find(item => item.path.includes('/dispatch-options?'))!;
  expect(Object.fromEntries(new URLSearchParams(check.path.split('?')[1]))).toEqual({
    project_id: project, expected_task_version: '4', workflow_id: flow.id, workflow_version: '2',
    agent_node_id: 'code', tool_profile_id: pin.id,
  });
  currentTool = { ...tool, version: 4, state: 'paused', available: false, reason: 'profile_paused' };
  await click('[data-dispatch-handoff-adopt]');
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe('');
  expect(document.querySelector('[data-dispatch-handoff-status=changed]')).not.toBeNull();
  await select('[data-dispatch-manual-tool]', manual.id);
  expect(document.querySelector('[data-dispatch-handoff-status=adopted]')).toBeNull();
  await click('[data-dispatch-handoff-adopt]');
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe(manual.id);
  currentTool = tool; await click('[data-dispatch-handoff-adopt]');
  delayTool = true; await click('[data-dispatch-handoff-adopt]');
  expect(document.querySelector('[data-dispatch-handoff-status=reading]')).not.toBeNull();
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe('');
  await select('[data-dispatch-manual-tool]', manual.id);
  await act(async () => deferred!(Response.json(exactOption()))); await settle();
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe(manual.id);
  expect(document.querySelector('[data-dispatch-handoff-status=adopted]')).toBeNull(); delayTool = false;
  const workflowSelector = '.tool-form fieldset select';
  await select(workflowSelector, 'flow-two@2');
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe('');
  expect(document.querySelector('[data-dispatch-handoff-status=idle]')).not.toBeNull();
  await select(workflowSelector, 'flow-one@2'); await click('[data-dispatch-handoff-adopt]');
  await act(async () => document.querySelector('form.tool-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))); await settle();
  const writes = requests.filter(item => item.method === 'POST');
  expect(writes).toHaveLength(1); expect(writes[0].body).toMatchObject({ expected_task_version: 4, tool_profile_id: pin.id, expected_tool_version: pin.version });
  expect(document.querySelector<HTMLButtonElement>('[data-dispatch-handoff-adopt]')?.disabled).toBe(true);
  const original = sessionStorage.getItem('suma-tool-recovery-v1:org-one:user-one:project-one:dispatch');
  expect(original).not.toBeNull(); expect(JSON.parse(original!).body.tool_profile_id).toBe(pin.id);
});

it('blocks incompatible or changed configurations and recovers the same scoped read', async () => {
  const adopt = vi.fn(), refresh = vi.fn(), back = vi.fn(), find = vi.fn();
  let result: unknown = exactOption({ ...tool, version: 4 });
  let status = 200;
  const fetch = vi.fn(async (_input: string, init: RequestInit = {}) => {
    expect(init.method || 'GET').toBe('GET');
    expect(init.body).toBeUndefined();
    return Response.json(result, { status });
  });
  vi.stubGlobal('fetch', fetch);
  await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchToolHandoff
    scope={scope} pin={pin} scopeKey="exact-task-flow-node" adopted={false} onInvalidate={vi.fn()}
    onAdopt={adopt} onRefreshContext={refresh} onReturn={back} onFindCompatible={find} /></ApiViewProvider>));
  await settle();
  expect(fetch).not.toHaveBeenCalled();
  const findButton = '[data-dispatch-handoff-find]';
  expect(host.querySelector(findButton)).toBeNull();

  const button = host.querySelector<HTMLButtonElement>('[data-dispatch-handoff-adopt]')!;
  button.focus();
  const phase = () => host.querySelector('[data-dispatch-handoff-status]')?.getAttribute('data-dispatch-handoff-status');
  await click('[data-dispatch-handoff-adopt]');
  expect(phase()).toBe('changed');
  expect(host.querySelector(findButton)?.textContent).toBe('Find compatible tools');
  await click('[data-dispatch-handoff-return]');
  expect(back).toHaveBeenCalledOnce();

  result = exactOption({ ...tool, available: false, reason: 'inventory_stale' });
  await click('[data-dispatch-handoff-adopt]');
  expect(phase()).toBe('tool_unavailable');
  expect(host.querySelector(findButton)).not.toBeNull();
  result = exactOption(tool, 'workflow_incompatible', 'Workflow requires a different tool capability');
  await click('[data-dispatch-handoff-adopt]');
  expect(phase()).toBe('workflow_incompatible');
  expect(host.querySelector(findButton)).not.toBeNull();
  expect(host.textContent).toContain('Change the workflow, step or tool');
  result = exactOption(tool, 'route_incompatible', 'Worker is outside the selected pool');
  await click('[data-dispatch-handoff-adopt]');
  expect(phase()).toBe('route_incompatible');
  expect(host.textContent).toContain('Choose a tool on that route');
  await act(async () => setLocale('zh-CN', { persist: false }));
  expect(host.textContent).toContain('原工具不符合当前执行路径');
  expect(host.querySelector(findButton)?.textContent).toBe('查找兼容工具');
  expect(find).not.toHaveBeenCalled();
  const checkedReads = fetch.mock.calls.length;
  await click(findButton);
  expect(find).toHaveBeenCalledOnce();
  expect(fetch).toHaveBeenCalledTimes(checkedReads);
  expect(phase()).toBe('route_incompatible');
  button.focus();
  expect(adopt).not.toHaveBeenCalled();

  status = 409;
  result = { error: { code: 'conflict', message: 'Task changed' } };
  await click('[data-dispatch-handoff-adopt]');
  expect(phase()).toBe('stale');
  expect(host.querySelector(findButton)).toBeNull();
  const reads = fetch.mock.calls.length;
  await click('[data-dispatch-handoff-adopt]');
  expect(refresh).toHaveBeenCalledOnce();
  expect(fetch).toHaveBeenCalledTimes(reads);

  status = 503;
  result = { error: { code: 'unavailable', message: 'Connection lost' } };
  await click('[data-dispatch-handoff-adopt]');
  expect(phase()).toBe('unknown');
  expect(host.querySelector(findButton)).toBeNull();
  const original = fetch.mock.lastCall?.[0];
  expect(document.activeElement).toBe(button);
  status = 200;
  result = exactOption();
  await click('[data-dispatch-handoff-adopt]');
  expect(fetch.mock.lastCall?.[0]).toBe(original);
  expect(adopt).toHaveBeenCalledOnce();
  expect(adopt).toHaveBeenCalledWith(tool);
  expect(host.querySelector(findButton)).toBeNull();
});

it('rejects response scope, substitute rows and missing or denied objects', async () => {
  const adopt = vi.fn();
  let status = 200, result: unknown = exactOption();
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(result, { status })));
  await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchToolHandoff
    scope={scope} pin={pin} scopeKey="exact-task-flow-node" adopted={false} onInvalidate={vi.fn()}
    onAdopt={adopt} onRefreshContext={vi.fn()} onReturn={vi.fn()} onFindCompatible={vi.fn()} /></ApiViewProvider>));
  await settle();

  const wrongOrganization = exactOption();
  wrongOrganization.execution_route.tenant_id = 'other-org';
  for (const malformed of [
    wrongOrganization,
    { ...exactOption(), workflow_id: 'other-flow' },
    { ...exactOption(), task_version: 5 },
    { ...exactOption(), next_cursor: 'unexpected-page' },
    { ...exactOption(), total: 2 },
    { ...exactOption(), items: [{ ...exactOption().items[0], reason_code: 'invented' }] },
    exactOption({ ...tool, id: 'other-profile' }),
  ]) {
    result = malformed;
    await click('[data-dispatch-handoff-adopt]');
    expect(host.querySelector('[data-dispatch-handoff-status=unknown]')).not.toBeNull();
  }

  result = exactOption({ ...tool, worker_id: 'other-worker' });
  await click('[data-dispatch-handoff-adopt]');
  expect(host.querySelector('[data-dispatch-handoff-status=changed]')).not.toBeNull();
  result = exactOption({ ...tool, profile: { ...definition, profile_sha256: 'd'.repeat(64) },
    observation: { profile: { ...definition, profile_sha256: 'd'.repeat(64) }, available: true, reason: '' } });
  await click('[data-dispatch-handoff-adopt]');
  expect(host.querySelector('[data-dispatch-handoff-status=changed]')).not.toBeNull();
  for (const [code, phase] of [[404, 'missing'], [403, 'forbidden']] as const) {
    status = code;
    result = { error: { code: 'unavailable', message: 'Cannot read original' } };
    await click('[data-dispatch-handoff-adopt]');
    expect(host.querySelector('[data-dispatch-handoff-status=' + phase + ']')).not.toBeNull();
    expect(host.querySelector('[data-dispatch-handoff-find]')).toBeNull();
  }
  expect(adopt).not.toHaveBeenCalled();
});

it('refreshes the original task version after a conflict and rejects a mismatched refresh', async () => {
  let taskRead = { id: 'wrong-task', project_id: project, version: 5, title: 'Changed task' };
  const requests: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init: RequestInit = {}) => {
    expect(init.method || 'GET').toBe('GET');
    const path = String(input).replace('/api/v1', '');
    requests.push(path);
    if (path.startsWith('/workflows?')) return Response.json({ items: [flow] });
    if (path.startsWith('/tool-profiles?')) return Response.json({ items: [], total: 0, next_cursor: '' });
    if (path.startsWith('/tasks/' + scope.task_id + '?')) return Response.json(taskRead);
    if (path.startsWith('/tasks/' + scope.task_id + '/dispatch-options?')) {
      if (new URLSearchParams(path.split('?')[1]).get('expected_task_version') === '4')
        return Response.json({ error: { code: 'conflict', message: 'Task changed' } }, { status: 409 });
      const current = exactOption();
      return Response.json({ ...current, task_version: 5, context: { task: { ...current.context.task, version: 5 } } });
    }
    return Response.json({ items: [], total: 0, next_cursor: '' });
  }));
  await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchDialog project={project}
    session={session} task={{ id: scope.task_id, project_id: project, version: 4, title: 'Develop task' }}
    initialWorkflow={flow} handoffTool={pin} onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()} /></ApiViewProvider>));
  await settle();

  await click('[data-dispatch-handoff-adopt]');
  await click('[data-dispatch-handoff-adopt]');
  expect(document.body.textContent).toContain('The returned record does not match this task');
  expect(requests.some(path => path.includes('wrong-task'))).toBe(false);
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe('');
  taskRead = { ...taskRead, id: scope.task_id };
  await click('[data-dispatch-handoff-adopt]');
  await click('[data-dispatch-handoff-adopt]');
  await click('[data-dispatch-handoff-adopt]');
  expect(requests.at(-1)).toContain('expected_task_version=5');
  expect(document.querySelector('[data-dispatch-handoff-status=adopted]')).not.toBeNull();
});

it('T213 drops late scope or API ABA callbacks and blocks adoption while the original operation is pending', async () => {
  const adopt = vi.fn(), waiting: Array<(value: TaskDispatchOptions) => void> = [];
  const sourceA = vi.fn(() => new Promise<TaskDispatchOptions>(resolve => waiting.push(resolve))) as unknown as ApiCaller;
  const sourceB = vi.fn(async () => exactOption()) as unknown as ApiCaller;
  let caller = sourceA, scopeKey = 'task-A', disabled = false;
  vi.spyOn(ApiContext, 'useApi').mockImplementation(() => caller);
  const draw = async () => { await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchToolHandoff scope={scope} pin={pin} scopeKey={scopeKey} disabled={disabled} adopted={false} onInvalidate={vi.fn()} onAdopt={adopt} onRefreshContext={vi.fn()} onReturn={vi.fn()} /></ApiViewProvider>)); await settle(); };
  await draw(); await click('[data-dispatch-handoff-adopt]'); scopeKey = 'task-B'; await draw(); scopeKey = 'task-A'; await draw();
  await act(async () => waiting.shift()!(exactOption())); await settle(); expect(adopt).not.toHaveBeenCalled();
  await click('[data-dispatch-handoff-adopt]'); caller = sourceB; await draw(); caller = sourceA; await draw();
  await act(async () => waiting.shift()!(exactOption())); await settle(); expect(adopt).not.toHaveBeenCalled();
  disabled = true; await draw(); const before = waiting.length; await click('[data-dispatch-handoff-adopt]'); expect(waiting).toHaveLength(before);
  disabled = false; await draw(); await click('[data-dispatch-handoff-adopt]'); disabled = true; await draw(); disabled = false; await draw();
  await act(async () => waiting.shift()!(exactOption())); await settle(); expect(adopt).not.toHaveBeenCalled();
});
