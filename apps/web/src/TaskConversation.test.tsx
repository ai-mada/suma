// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {TaskConversationMessage, TaskConversationPage, WorkspaceTaskContext} from '../../../packages/contracts/src';
import {TaskConversation} from './TaskConversation';
import {WorkbenchTaskContext} from './WorkbenchTaskContext';
import {validConversationBody, validConversationMessage} from './task-conversation-model';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';

const session = (id = 'user-a'): Session => ({user: {id, name: id, email: id + '@example.test'},
  organization: {id: 'org-' + id, name: 'Organization'}, projects: []});
const message = (sequence = 1, body = 'Original task note', version = 7): TaskConversationMessage => ({
  id: 'message-' + sequence, sequence, task_version: version, body,
  author_id: 'user-a', author_name: 'Recorded person', actor_kind: 'human',
  created_at: '2026-10-29T04:00:00Z', requirement_id: 'req-a', requirement_version: 2, run_id: 'run-a',
  reply_id: '', reply_to_sequence: 0, execution_run_id: '',
});
const page = (items: TaskConversationMessage[] = [], task_version = 7): TaskConversationPage => ({
  project_id: 'project-a', task_id: 'task-a', task_version, can_post: true,
  items, total: items.length, limit: 20, next_before: 0,
});
const raw = '  原始上下文 🧪\n<script>never execute()</script>  ';
let host: HTMLDivElement, root: Root;
let reads: {path: string; signal?: AbortSignal | null}[];
let writes: {path: string; body: Record<string, unknown>; headers: Headers}[];

beforeEach(async () => {
  vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  await setLocale('en-US', {persist: false});
  sessionStorage.clear(); reads = []; writes = [];
  host = document.createElement('div'); document.body.append(host); root = createRoot(host);
});
afterEach(async () => {
  await act(async () => root.unmount()); host.remove();
  vi.unstubAllGlobals(); await setLocale('zh-CN', {persist: false});
});
async function settle() {
  for (let index = 0; index < 4; index++)
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
}
async function render(user = 'user-a', project = 'project-a', taskId = 'task-a', refresh = 0) {
  const current = session(user);
  await act(async () => root.render(<ApiViewProvider session={current}>
    <TaskConversation session={current} project={project} taskId={taskId} refresh={refresh} accessBlocked={false}/>
  </ApiViewProvider>));
  await settle();
}
function transport(reply: (path: string, options?: RequestInit) => Promise<Response>) {
  vi.stubGlobal('fetch', vi.fn(async (input: string, options?: RequestInit) => {
    const path = String(input).replace(/^\/api\/v1/, '');
    if (options?.body) writes.push({path, body: JSON.parse(String(options.body)), headers: new Headers(options.headers)});
    else reads.push({path, signal: options?.signal});
    return reply(path, options);
  }));
}
async function click(selector: string) {
  const item = host.querySelector<HTMLElement>(selector); expect(item, selector).not.toBeNull();
  await act(async () => item!.click()); await settle();
}
async function type(body: string) {
  const field = host.querySelector<HTMLTextAreaElement>('[name=conversation_body]'); expect(field).not.toBeNull();
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')!.set!.call(field, body);
    field!.dispatchEvent(new Event('input', {bubbles: true}));
  });
}
async function submit() {
  await act(async () => host.querySelector('[data-conversation-form]')!
    .dispatchEvent(new Event('submit', {bubbles: true, cancelable: true})));
  await settle();
}

it('T127 loads discussion only when opened, prepends bounded chronological history and renders API/user notes as inert text', async () => {
  expect(validConversationBody('not allowed\0inside')).toBe(false);
  expect(validConversationMessage({...message(), run_id: 'r'.repeat(100)})).toBe(true);
  const latest = {...page([message(3, 'Newer note'), {...message(4, raw), actor_kind: 'api' as const}]), total: 4, next_before: 3, can_post: false};
  transport(async path => Response.json(path.includes('&before=')
    ? {...page([message(1, 'Oldest note'), message(2, 'Second note')]), total: 4, can_post: false} : latest));
  await render(); expect(reads).toHaveLength(0);
  await click('[data-conversation-toggle]');
  expect(reads[0].path).toContain('limit=20'); expect(host.textContent).toContain(raw);
  expect(host.querySelector('script')).toBeNull(); expect(host.textContent).toContain('API collaborator');
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.disabled).toBe(true);
  await click('[data-conversation-more]'); expect(reads[1].path).toContain('before=3');
  expect([...host.querySelectorAll('[data-conversation-message] .task-conversation-message-text')]
    .map(item => item.textContent)).toEqual(['Oldest note', 'Second note', 'Newer note', raw]);
  expect(host.querySelector('[data-conversation-more]')).toBeNull(); expect(writes).toHaveLength(0);
});

it('T127 preserves one original note, body, task version and caller through a lost receipt and language change', async () => {
  let lost = true, stored = false;
  transport(async (_path, options) => {
    if (options?.body) {
      if (lost) throw TypeError('local fixture lost original receipt');
      stored = true; return Response.json(message(5, raw), {status: 201});
    }
    return Response.json(stored ? page([message(5, raw)], 8) : page());
  });
  await render(); await click('[data-conversation-toggle]'); await type(raw); await submit();
  expect(writes).toHaveLength(1); expect(writes[0].body).toMatchObject({project_id: 'project-a', task_id: 'task-a', expected_task_version: 7, body: raw});
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe(raw);
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.disabled).toBe(true);
  const navigation = new Event('suma:navigate', {cancelable: true});
  await act(async () => window.dispatchEvent(navigation)); expect(navigation.defaultPrevented).toBe(true);
  await act(async () => { await setLocale('zh-CN', {persist: false}); });
  lost = false; await click('[data-conversation-restore]');
  expect(writes).toHaveLength(2); expect(writes[1].body).toEqual(writes[0].body);
  expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('user-a');
  expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe('org-user-a');
  expect(host.textContent).toContain('未启动智能体或执行');
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('');
  expect(writes.every(value => value.path === '/task-conversations')).toBe(true);
});

it('T127 aborts obsolete task/actor reads and hides previously authorized notes on permission loss', async () => {
  let release!: (response: Response) => void;
  transport(async () => new Promise<Response>(resolve => { release = resolve; }));
  await render(); await click('[data-conversation-toggle]');
  const old = release, signal = reads[0].signal;
  transport(async () => Response.json({...page([message(1, 'Current owner note')]), project_id: 'project-b', task_id: 'task-b'}));
  await render('user-b', 'project-b', 'task-b'); expect(signal?.aborted).toBe(true);
  await act(async () => old(Response.json(page([message(1, 'Previous private owner note')])))); await settle();
  expect(host.textContent).not.toContain('Previous private owner note');
  await click('[data-conversation-toggle]'); expect(host.textContent).toContain('Current owner note');
  let replyWrite!: (response: Response) => void, replyRead!: (response: Response) => void;
  transport(async (_path, options) => new Promise<Response>(resolve => {
    if (options?.body) replyWrite = resolve; else replyRead = resolve;
  }));
  await type('Preserve my own draft'); await submit();
  await render('user-b', 'project-b', 'task-b', 1);
  const refreshSignal = reads[reads.length - 1].signal;
  await act(async () => replyWrite(Response.json({error: {code: 'forbidden', message: 'Current access denied'}}, {status: 403}))); await settle();
  expect(refreshSignal?.aborted).toBe(true);
  await act(async () => replyRead(Response.json({...page([message(1, 'Late private note')]), project_id: 'project-b', task_id: 'task-b'}))); await settle();
  expect(host.textContent).not.toContain('Current owner note'); expect(host.querySelector('textarea')).toBeNull();
  expect(host.textContent).not.toContain('Late private note');
  expect(host.textContent).toContain('Previous notes are hidden'); expect(writes).toHaveLength(1);
  transport(async () => Response.json({...page(), project_id: 'project-b', task_id: 'task-b', can_post: false}));
  const retry = [...host.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent === 'Check current access');
  expect(retry).toBeDefined(); await act(async () => retry!.click()); await settle();
  expect(host.textContent).not.toContain('Late private note');
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.disabled).toBe(true);
});

it('T127 keeps a changed-basis draft, explicitly reloads its current task version and blocks leaving until cleared or saved', async () => {
  let changed = false, first = true;
  transport(async (_path, options) => {
    if (options?.body) {
      if (first) { first = false; changed = true; return Response.json({error: {code: 'conflict', message: 'Task basis changed'}}, {status: 409}); }
      return Response.json(message(1, raw, 8), {status: 201});
    }
    return Response.json(page([], changed ? 8 : 7));
  });
  await render(); await click('[data-conversation-toggle]'); await type(raw);
  const navigation = new Event('suma:navigate', {cancelable: true});
  await act(async () => window.dispatchEvent(navigation)); expect(navigation.defaultPrevented).toBe(true);
  await submit(); expect(host.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe(raw);
  expect(host.querySelector('.conflict-banner')).not.toBeNull();
  await click('.conflict-banner button');
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe(raw);
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.disabled).toBe(false);
  await submit(); expect(writes).toHaveLength(2);
  expect(writes[1].body).toMatchObject({expected_task_version: 8, body: raw});
  expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('');
});

function context(): WorkspaceTaskContext {
  const refs = {scope: 'current_run' as const, total: 0, limit: 20, complete: true, items: []};
  return {project_id: 'project-a', observed_at: '2026-10-29T04:00:00Z',
    task: {id: 'task-a', version: 7, title: 'Current task', description: '', acceptance_criteria: [],
      status: 'todo', priority: 'medium', owner_id: '', requirement_id: '', run_id: ''},
    requirement: null, owner: null, design: {state: 'none', context: null},
    delivery: {project_id: 'project-a', task_id: 'task-a', task_version: 7, task_status: 'todo',
      run_basis_state: 'not_dispatched', formal_acceptance: false, run: null,
      expert_plans: {...refs}, issues: {...refs}},
    quality: {plan_id: '', revision: 0, plan_sha256: '', run_id: '', run_state: '', state: 'none',
      basis_current: false, total: 0, passed: 0, failed: 0, errors: 0, skipped: 0, not_run: 0}};
}

it('T127 keeps the same-owner draft and navigation guard across parent context denial, restores it after access checks and permits local clearing', async () => {
  let contextDenied = false;
  transport(async path => path.startsWith('/workspace/task-context?')
    ? contextDenied
      ? Response.json({error: {code: 'forbidden', message: 'Current project access denied'}}, {status: 403})
      : Response.json(context())
    : Response.json(page([message(1, 'Authorized private history')])));
  const current = session();
  await act(async () => root.render(<ApiViewProvider session={current}>
    <WorkbenchTaskContext session={current} project="project-a" selectedTask="task-a" refresh={0}
      onTaskSelect={() => window.dispatchEvent(new Event('suma:navigate', {cancelable: true}))}/>
  </ApiViewProvider>));
  await settle(); await click('[data-conversation-toggle]');
  await type('Keep this local draft across an access check');
  contextDenied = true; await click('[aria-label="Refresh context"]');
  expect(host.textContent).not.toContain('Authorized private history');
  expect(host.querySelector('textarea')).toBeNull();
  expect(host.querySelector('[data-conversation-toggle]')).not.toBeNull();
  const blocked = new Event('suma:navigate', {cancelable: true});
  await act(async () => window.dispatchEvent(blocked)); expect(blocked.defaultPrevented).toBe(true);
  contextDenied = false;
  const reload = [...host.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent === 'Reload');
  expect(reload).toBeDefined(); await act(async () => reload!.click()); await settle();
  expect(host.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe('Keep this local draft across an access check');
  contextDenied = true; await click('[aria-label="Refresh context"]');
  const clear = [...host.querySelectorAll<HTMLButtonElement>('button')].find(item => item.textContent === 'Clear draft');
  expect(clear?.disabled).toBe(false); await act(async () => clear!.click()); await settle();
  const leave = new Event('suma:navigate', {cancelable: true});
  await act(async () => window.dispatchEvent(leave)); expect(leave.defaultPrevented).toBe(false);
  expect(host.textContent).not.toContain('Authorized private history'); expect(writes).toHaveLength(0);
});
