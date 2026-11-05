// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { ApiViewProvider } from './api-context';
import { TaskDispatchTestBindings, type TestBindingsChoice } from './TaskDispatchTestBindings';
import { TaskDispatchDialog } from './TaskDispatch';
import { setLocale } from './i18n';
import type { Session, Workflow } from './api';
import type { ToolProfile } from './tool-model';

const session: Session = { user: { id: 'owner', name: 'Owner', email: 'owner@example.test' }, organization: { id: 'org', name: 'Team' }, projects: [] };
const definition = { command_id: 'verify-command', adapter: 'command', protocol: 'text-v1', test_report_protocol: 'suma-junit-report-v1', read_only: false, profile_sha256: 'a'.repeat(64), executable_sha256: 'b'.repeat(64), tool_version: '1.0.0' };
const tester: ToolProfile = { id: 'tester', project_id: 'project', worker_id: 'worker-test', name: 'Independent tester', version: 3, state: 'enabled', profile: definition, available: true, reason: '', enabled_by: 'owner', created_at: '2026-11-03T00:00:00Z', updated_at: '2026-11-03T00:00:00Z', observation: { profile: { ...definition }, available: true, reason: '' } };
const developer: ToolProfile = { ...tester, id: 'developer', worker_id: 'worker-code', name: 'Developer', profile: { ...definition, command_id: 'develop', adapter: 'suma', protocol: 'suma-jsonl-v1', test_report_protocol: undefined }, observation: undefined };
developer.observation = { profile: { ...developer.profile }, available: true, reason: '' };
const workflow: Workflow = { id: 'flow', name: 'Code and test', version: 2, nodes: [{ id: 'code', name: 'Code', type: 'agent' }, { id: 'verify', name: 'Independent test', type: 'test', command: ['published-test'], config: { candidate_from: 'code' } }], edges: [{ from: 'code', to: 'verify' }] };
const task = { id: 'task', version: 4 };
const route = { project_id: 'project', tenant_id: 'org', version: 2, pool_id: 'pool', grant_id: 'grant', grant_version: 3 };
const binding = { node_id: 'verify', tool_profile_id: tester.id, tool_profile_version: tester.version,
  tool_profile_sha256: tester.profile.profile_sha256, command_id: tester.profile.command_id, worker_id: tester.worker_id };
const preflight = { project_id: 'project', task_id: task.id, task_version: task.version, workflow_id: workflow.id,
  workflow_version: workflow.version, agent_node_id: 'code', development_tool_profile_id: developer.id,
  development_tool_profile_version: developer.version, development_tool_profile_sha256: developer.profile.profile_sha256,
  checked_at: '2026-11-03T00:00:00Z', execution_route: route, binding };
let root: Root, host: HTMLDivElement;
beforeEach(async () => { await setLocale('en-US', { persist: false }); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true); host = document.createElement('div'); document.body.append(host); root = createRoot(host); });
afterEach(async () => { await act(async () => root.unmount()); host.remove(); sessionStorage.clear(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
async function settle() { for (let i = 0; i < 3; i++) await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); }); }
async function choose(selector: string, value: string) { const element = document.querySelector<HTMLSelectElement>(selector)!; expect(element).not.toBeNull(); await act(async () => { Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')!.set!.call(element, value); element.dispatchEvent(new Event('change', { bubbles: true })); }); await settle(); }
async function click(selector: string) { await act(async () => document.querySelector<HTMLElement>(selector)!.click()); await settle(); }
async function open() { const details = document.querySelector<HTMLDetailsElement>('[data-dispatch-test-bindings]')!; await act(async () => { details.open = true; details.dispatchEvent(new Event('toggle')); }); await settle(); }

it('T216 rechecks exact test tools on demand, keeps off-page selections and discards refresh, source ABA and pending callbacks', async () => {
  const changed = vi.fn<(value: TestBindingsChoice) => void>(), reads: string[] = []; let deferred = false, release: ((value: Response) => void) | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string, options: RequestInit = {}) => { expect(options.method || 'GET').toBe('GET'); reads.push(String(input)); return deferred ? new Promise<Response>(resolve => { release = resolve; }) : Response.json(preflight); }));
  const inventory = { items: [tester], total: 1, loading: false, error: '', next_cursor: '', read: vi.fn().mockResolvedValue(true) };
  const draw = async (scope = 'A', disabled = false) => { await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchTestBindings project="project" task={task} workflow={workflow} agentID="code" developer={developer} scopeKey={scope} inventory={inventory} disabled={disabled} onChange={changed} /></ApiViewProvider>)); await settle(); };
  await draw(); expect(host.querySelector<HTMLDetailsElement>('[data-dispatch-test-bindings]')!.open).toBe(false); expect(reads).toEqual([]);
  await open(); await choose('[data-dispatch-test-select=verify]', tester.id);
  expect(changed).toHaveBeenLastCalledWith({ ready: true, hasIntent: true, selections: [{ node_id: 'verify', tool_profile_id: tester.id, expected_tool_version: 3 }] });
  expect(host.querySelector('[data-dispatch-test-route]')?.textContent).toContain(route.pool_id);
  inventory.items = []; await draw(); expect(host.querySelector<HTMLSelectElement>('[data-dispatch-test-select=verify]')!.value).toBe(tester.id);
  deferred = true; await click('[data-dispatch-test-recheck=verify]'); await click('[data-dispatch-test-refresh]'); await act(async () => release!(Response.json(preflight))); await settle();
  expect(host.querySelector<HTMLSelectElement>('[data-dispatch-test-select=verify]')!.value).toBe(''); expect(changed).toHaveBeenLastCalledWith({ ready: true, hasIntent: false, selections: [] });
  inventory.items = [tester]; await draw();
  for (const failure of [false, new Error('Directory unavailable')]) {
    await choose('[data-dispatch-test-select=verify]', tester.id);
    if (failure instanceof Error) inventory.read.mockRejectedValueOnce(failure); else inventory.read.mockResolvedValueOnce(false);
    await click('[data-dispatch-test-refresh]'); await act(async () => release!(Response.json(preflight))); await settle();
    expect(host.querySelector('[data-dispatch-test-status=unknown]')).not.toBeNull(); expect(host.querySelector('[data-dispatch-test-status=reading]')).toBeNull();
    expect(changed.mock.calls.at(-1)?.[0].ready).toBe(false); await choose('[data-dispatch-test-select=verify]', '');
  }
  inventory.items = [tester]; await draw(); await choose('[data-dispatch-test-select=verify]', tester.id); await draw('B'); await draw('A'); await act(async () => release!(Response.json(preflight))); await settle();
  expect(host.querySelector('[data-dispatch-test-status=ready]')).toBeNull(); await open(); await choose('[data-dispatch-test-select=verify]', tester.id); await draw('A', true); await draw('A', false); await act(async () => release!(Response.json(preflight))); await settle();
  expect(host.querySelector('[data-dispatch-test-status=ready]')).toBeNull(); expect(changed.mock.calls.at(-1)?.[0].ready).toBe(false);
});

it('checks the exact task and tool pins, separates dispatch blockers from uncertain reads and adopts only a matching response', async () => {
  const changed = vi.fn<(value: TestBindingsChoice) => void>(), reads: string[] = [];
  let reply = () => Response.json({ error: { code: 'route_mismatch', message: 'Testing worker is outside the selected pool' } }, { status: 409 });
  vi.stubGlobal('fetch', vi.fn(async (path: string, init: RequestInit = {}) => {
    expect(init.method).toBe('GET');
    reads.push(path);
    return reply();
  }));
  const inventory = { items: [{ ...tester, reason: 'Prior inventory observation' }], total: 1, loading: false, error: '', next_cursor: '', read: vi.fn() };
  const draw = async () => act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchTestBindings project="project" task={task}
    workflow={workflow} agentID="code" developer={developer} scopeKey="scope" inventory={inventory} onChange={changed} /></ApiViewProvider>));
  await draw(); await open(); await choose('[data-dispatch-test-select=verify]', tester.id);
  const url = new URL(reads[0], 'https://example.test');
  expect(url.pathname).toBe('/api/v1/tasks/task/dispatch-test-option');
  expect(Object.fromEntries(url.searchParams)).toEqual({
    project_id: 'project', expected_task_version: '4', workflow_id: 'flow', workflow_version: '2', agent_node_id: 'code',
    development_tool_profile_id: developer.id, development_tool_profile_version: String(developer.version),
    test_node_id: 'verify', test_tool_profile_id: tester.id, test_tool_profile_version: String(tester.version),
  });
  expect(host.querySelector('[data-dispatch-test-status=blocked]')).not.toBeNull();
  expect(host.querySelector('[role=alert]')?.textContent).toContain('Testing worker is outside the selected pool');
  expect(host.textContent).not.toContain('Prior inventory observation');
  expect(changed.mock.lastCall?.[0]).toEqual({ ready: false, hasIntent: true, selections: [] });

  inventory.items = [{ ...tester, version: 4 }];
  await draw(); await click('[data-dispatch-test-recheck=verify]');
  expect(new URL(reads.at(-1)!, 'https://example.test').searchParams.get('test_tool_profile_version')).toBe('3');

  for (const invalid of [
    { ...preflight, execution_route: { ...route, tenant_id: 'other' } },
    { ...preflight, binding: { ...binding, tool_profile_sha256: 'f'.repeat(64) } },
    { ...preflight, development_tool_profile_sha256: 'f'.repeat(64) },
    { ...preflight, task_version: 5 },
  ]) {
    reply = () => Response.json(invalid);
    await click('[data-dispatch-test-recheck=verify]');
    expect(host.querySelector('[data-dispatch-test-status=changed]')).not.toBeNull();
    expect(host.querySelector('[data-dispatch-test-route]')).toBeNull();
    expect(changed.mock.lastCall?.[0].selections).toEqual([]);
  }

  reply = () => { throw new Error('Disconnected'); };
  await click('[data-dispatch-test-recheck=verify]');
  expect(host.querySelector('[data-dispatch-test-status=unknown]')).not.toBeNull();
  reply = () => Response.json(preflight);
  await click('[data-dispatch-test-recheck=verify]');
  expect(host.querySelector('[data-dispatch-test-status=ready]')).not.toBeNull();
  expect(changed.mock.lastCall?.[0].selections).toEqual([{ node_id: 'verify', tool_profile_id: tester.id, expected_tool_version: 3 }]);

  reply = () => Response.json({ error: { code: 'forbidden', message: 'Access changed' } }, { status: 403 });
  await click('[data-dispatch-test-recheck=verify]');
  expect(host.querySelector('[data-dispatch-test-route]')).toBeNull();
  expect(host.querySelector('[data-dispatch-test-select=verify]')).toBeNull();
  expect(changed.mock.lastCall?.[0]).toEqual({ ready: false, hasIntent: true, selections: [] });
});

it('T216 fixes explicit test selections into the original preview body and retains that body while the current task changes', async () => {
  const writes: Record<string, unknown>[] = [], before = JSON.stringify(workflow);
  vi.stubGlobal('fetch', vi.fn(async (input: string, init: RequestInit = {}) => {
    const path = String(input).replace('/api/v1', '');
    if ((init.method || 'GET') !== 'GET') { writes.push(JSON.parse(String(init.body))); throw new Error('Preview response lost'); }
    if (path.startsWith('/workflows?')) return Response.json({ items: [workflow] });
    if (path.startsWith('/tool-profiles?')) return Response.json({ items: [developer, tester], total: 2 });
    if (path.startsWith('/tasks/task/dispatch-test-option?')) return Response.json(preflight);
    return Response.json({ items: [], total: 0 });
  }));
  const draw = async (version = 4) => { await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchDialog project="project" session={session} task={{ id: 'task', project_id: 'project', version, title: 'Develop feature' }} initialWorkflow={workflow} onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()} /></ApiViewProvider>)); await settle(); };
  await draw(); await choose('[data-dispatch-manual-tool]', developer.id); await open(); await choose('[data-dispatch-test-select=verify]', tester.id);
  expect(writes).toEqual([]); expect(JSON.stringify(workflow)).toBe(before);
  await act(async () => document.querySelector('form.tool-form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))); await settle();
  expect(writes).toHaveLength(1); expect(writes[0]).toMatchObject({ tool_profile_id: developer.id, test_bindings: [{ node_id: 'verify', tool_profile_id: tester.id, expected_tool_version: 3 }] });
  const original = sessionStorage.getItem('suma-tool-recovery-v1:org:owner:project:dispatch')!; expect(original).not.toBeNull();
  await draw(5); expect(sessionStorage.getItem('suma-tool-recovery-v1:org:owner:project:dispatch')).toBe(original); expect(writes).toHaveLength(1);
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-test-select=verify]')?.disabled).not.toBe(false);
});
