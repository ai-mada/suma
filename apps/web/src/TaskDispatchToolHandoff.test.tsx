// @vitest-environment jsdom
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import type { Session, Workflow, ApiCaller } from './api';
import * as ApiContext from './api-context';
import { ApiViewProvider } from './api-context';
import { setLocale } from './i18n';
import { TaskDispatchDialog } from './TaskDispatch';
import { TaskDispatchToolHandoff } from './TaskDispatchToolHandoff';
import type { ToolTaskHandoffPin } from './tool-task-handoff-model';
import type { ToolProfile } from './tool-model';

const project = 'project-one', stamp = '2026-11-02T01:00:00Z';
const session: Session = { user: { id: 'user-one', name: 'Owner', email: 'owner@example.test' }, organization: { id: 'org-one', name: 'Team' }, projects: [] };
const definition = { command_id: 'coder', adapter: 'suma', protocol: 'suma-jsonl-v1', profile_sha256: 'a'.repeat(64), executable_sha256: 'b'.repeat(64), tool_version: '1.0.0', read_only: false };
const tool: ToolProfile = { id: 'outside-first-page', project_id: project, worker_id: 'worker-one', name: 'Chosen tool', version: 3, state: 'enabled', profile: definition,
  available: true, reason: '', enabled_by: session.user.id, created_at: stamp, updated_at: stamp, observation: { profile: { ...definition }, available: true, reason: '' } };
const pin: ToolTaskHandoffPin = { project_id: project, id: tool.id, version: tool.version, worker_id: tool.worker_id, profile_sha256: definition.profile_sha256, name: tool.name };
const flow: Workflow = { id: 'flow-one', name: 'Code and verify', version: 2, nodes: [{ id: 'code', name: 'Develop', type: 'agent' }, { id: 'test', name: 'Test independently', type: 'test', command: ['verify'], config: { candidate_from: 'code' } }], edges: [{ from: 'code', to: 'test' }] };
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
    if (path.startsWith('/tool-profiles/' + pin.id + '?')) return delayTool ? new Promise<Response>(resolve => { deferred = resolve; }) : Response.json(currentTool);
    return Response.json({ items: [], total: 0, next_cursor: '' });
  }));
  await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={{ id: 'task-one', project_id: project, version: 4, title: 'Develop task' }} initialWorkflow={flow} handoffTool={pin} onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()} /></ApiViewProvider>)); await settle();
  expect(requests.some(item => item.path.startsWith('/tool-profiles/' + pin.id))).toBe(false);
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe('');
  await click('[data-dispatch-handoff-adopt]');
  expect(document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]')?.value).toBe(pin.id);
  expect(document.querySelector('[data-dispatch-handoff-status=adopted]')).not.toBeNull();
  expect(requests.filter(item => item.method !== 'GET')).toEqual([]);
  currentTool = { ...tool, state: 'paused' };
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
  await act(async () => deferred!(Response.json(tool))); await settle();
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

it('T213 rejects changed or denied tool reads without substituting a profile and renders localized explicit adoption feedback', async () => {
  const adopt = vi.fn(); let value = { ...tool, version: 4 }, denied = false;
  const fetch = vi.fn(async (_input: string, init: RequestInit = {}) => {
    expect(init.method || 'GET').toBe('GET'); expect(init.body).toBeUndefined();
    return denied ? Response.json({ error: { code: 'forbidden', message: 'No access' } }, { status: 403 }) : Response.json(value);
  }); vi.stubGlobal('fetch', fetch);
  const draw = async () => { await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchToolHandoff project={project} pin={pin} scopeKey="exact-task-flow-node" adopted={false} onInvalidate={vi.fn()} onAdopt={adopt} /></ApiViewProvider>)); await settle(); };
  await draw(); expect(fetch).not.toHaveBeenCalled(); await click('[data-dispatch-handoff-adopt]');
  expect(document.querySelector('[data-dispatch-handoff-status=changed]')).not.toBeNull(); expect(adopt).not.toHaveBeenCalled();
  value = { ...tool, state: 'paused' }; await click('[data-dispatch-handoff-adopt]'); expect(adopt).not.toHaveBeenCalled();
  denied = true; await click('[data-dispatch-handoff-adopt]'); expect(document.querySelector('[data-dispatch-handoff-status=forbidden]')).not.toBeNull(); expect(adopt).not.toHaveBeenCalled();
  await setLocale('zh-CN', { persist: false }); await draw(); expect(host.textContent).toContain('核对并采用此工具');
});

it('T213 drops late scope or API ABA callbacks and blocks adoption while the original operation is pending', async () => {
  const adopt = vi.fn(), waiting: Array<(value: ToolProfile) => void> = [];
  const sourceA = vi.fn(() => new Promise<ToolProfile>(resolve => waiting.push(resolve))) as unknown as ApiCaller;
  const sourceB = vi.fn(async () => tool) as unknown as ApiCaller;
  let caller = sourceA, scopeKey = 'task-A', disabled = false;
  vi.spyOn(ApiContext, 'useApi').mockImplementation(() => caller);
  const draw = async () => { await act(async () => root.render(<ApiViewProvider session={session}><TaskDispatchToolHandoff project={project} pin={pin} scopeKey={scopeKey} disabled={disabled} adopted={false} onInvalidate={vi.fn()} onAdopt={adopt} /></ApiViewProvider>)); await settle(); };
  await draw(); await click('[data-dispatch-handoff-adopt]'); scopeKey = 'task-B'; await draw(); scopeKey = 'task-A'; await draw();
  await act(async () => waiting.shift()!(tool)); await settle(); expect(adopt).not.toHaveBeenCalled();
  await click('[data-dispatch-handoff-adopt]'); caller = sourceB; await draw(); caller = sourceA; await draw();
  await act(async () => waiting.shift()!(tool)); await settle(); expect(adopt).not.toHaveBeenCalled();
  disabled = true; await draw(); const before = waiting.length; await click('[data-dispatch-handoff-adopt]'); expect(waiting).toHaveLength(before);
  disabled = false; await draw(); await click('[data-dispatch-handoff-adopt]'); disabled = true; await draw(); disabled = false; await draw();
  await act(async () => waiting.shift()!(tool)); await settle(); expect(adopt).not.toHaveBeenCalled();
});
