// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import type {ModelBinding, ModelExecutionOptions, TaskConversationMessage, TaskConversationPage, TaskConversationReply as Reply, ToolProfile} from '../../../packages/contracts/src';
import {TaskConversationReply} from './TaskConversationReply';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import type {connectRunEvents} from './run-events';

const streams: Parameters<typeof connectRunEvents>[0][] = [];
vi.mock('./run-events', async original => ({...await original<typeof import('./run-events')>(),
  connectRunEvents: vi.fn(async (value: Parameters<typeof connectRunEvents>[0]) => {streams.push(value);})}));
const sha = (letter: string) => letter.repeat(64), stamp = '2026-10-29T05:00:00Z';
const user = (id = 'user-a', org = 'org-a'): Session => ({user: {id, name: 'Person', email: id + '@example.test'},
  organization: {id: org, name: 'Organization'}, projects: []});
function note(sequence = 1): TaskConversationMessage {return {id: 'note-' + sequence, sequence, task_version: 7,
  body: 'Saved task question ' + sequence, author_id: 'user-a', author_name: 'Person', actor_kind: 'human',
  created_at: stamp, requirement_id: '', requirement_version: 0, run_id: '', reply_id: '', reply_to_sequence: 0, execution_run_id: ''};}
const page = (sequence = 1): TaskConversationPage => ({project_id: 'project-a', task_id: 'task-a', task_version: 7,
  can_post: true, items: Array.from({length: sequence}, (_, index) => note(index + 1)), total: sequence, limit: 20, next_before: 0});
const profile: ToolProfile = {id: 'reply-profile', project_id: 'project-a', worker_id: 'worker-a', name: 'Reply expert', version: 2,
  state: 'enabled', profile: {command_id: 'native-reply', adapter: 'suma', protocol: 'suma-jsonl-v1', profile_sha256: sha('c'),
    executable_sha256: sha('d'), tool_version: '1.0.0', read_only: true, model_execution_protocol: 'suma-model-execution-v1',
    task_conversation_reply: {protocol: 'suma-task-conversation-reply-v1'}}, enabled_by: 'user-a', created_at: stamp,
  updated_at: stamp, available: true, reason: ''};
const deployment = {deployment_id: 'selected-model', revision: 2, deployment_sha256: sha('a'), protocol: 'openai-chat-completions-v1',
  model: 'Explicit model', endpoint_origin: 'https://provider.example.test', endpoint_config_sha256: sha('b'), context_window: 32};
function options(): ModelExecutionOptions {return {project_id: 'project-a', tool_profile_id: profile.id, tool_profile_version: 2,
  tool_profile_sha256: sha('c'), observed_at: stamp, supported: true, reason: '', codex_supported: false, codex_reason: '',
  parameter_limits: {max_steps: 1, max_output_tokens: 8192}, items: [{deployment, cost_state: 'unpriced',
    default_parameters: {max_steps: 1, max_output_tokens: 512}, input_token_reservation: 32, output_token_reservation: 512}]};}
function reply(body?: Record<string, any>, state = 'running'): Reply {
  const target = Number(body?.reply_to_sequence || 1), head = Number(body?.expected_last_sequence || 1);
  const binding: ModelBinding = {protocol: 'suma-model-execution-v1', deployment,
    parameters: body?.model_execution_selection?.parameters || {max_steps: 1, max_output_tokens: 512},
    parameters_sha256: sha('e'), binding_sha256: sha('f'), input_token_reservation: 32, output_token_reservation: 512,
    ...(body?.model_execution_selection?.budget ? {budget: body.model_execution_selection.budget} : {})};
  return {id: 'reply-' + target, project_id: 'project-a', task_id: 'task-a', version: 1, state, locale: 'en-US',
    run_id: 'reply-run-' + target, run_version: 1, run_state: state === 'completed' ? 'succeeded' : 'running',
    source: {sha256: sha('1'), task: {id: 'task-a', version: 7, title: 'Task', description: 'Original task', acceptance_criteria: []},
      requirement: null, task_run_id: '', messages: Array.from({length: head}, (_, index) => note(index + 1)),
      coverage: {total: head, included: head, first_sequence: 1, last_sequence: head, complete: true}, reply_to_sequence: target,
      execution_route: {tenant_id: 'org-a', project_id: 'project-a', version: 0, pool_id: '', grant_id: '', grant_version: 0}},
    tool_profile_id: profile.id, tool_profile_version: 2, tool_profile_sha256: sha('c'), assistant_name: 'Reply expert',
    model_execution: binding, message_id: state === 'completed' ? 'assistant-' + target : '', message_sequence: state === 'completed' ? head + 1 : 0,
    created_by: 'user-a', created_at: stamp, updated_at: stamp, basis_current: true, reason: '',
    allowed_actions: state === 'running' ? ['cancel'] : []};
}
let host: HTMLDivElement, root: Root, server: Reply | undefined;
let requests: {path: string; body?: Record<string, any>; headers: Headers}[];
let send: (path: string, body?: Record<string, any>) => Promise<Response>;
const refreshed = vi.fn(async () => {}), lock = vi.fn();
beforeEach(async () => {
  await setLocale('en-US', {persist: false}); sessionStorage.clear(); streams.length = 0; server = undefined; requests = [];
  refreshed.mockClear(); lock.mockClear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  host = document.createElement('div'); document.body.append(host); root = createRoot(host);
  send = async (path, body) => {
    if (path.startsWith('/tool-profiles?')) return Response.json({items: [profile], total: 1, next_cursor: ''});
    if (path.startsWith('/model-execution/options?')) return Response.json(options());
    if (path === '/task-conversation-replies' && body) {server = reply(body); return Response.json(server, {status: 201});}
    if (path.startsWith('/task-conversation-replies?')) return Response.json({items: server ? [server] : [], total: server ? 1 : 0, limit: 20, next_cursor: ''});
    if (path.startsWith('/task-conversation-replies/')) return Response.json(server);
    throw Error('Unexpected reply endpoint ' + path);
  };
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const path = String(input).replace(/^\/api\/v1/, ''), body = init?.body ? JSON.parse(String(init.body)) : undefined;
    requests.push({path, body, headers: new Headers(init?.headers)}); return send(path, body);
  }));
});
afterEach(async () => {await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals(); await setLocale('zh-CN', {persist: false});});
async function settle() {for (let index = 0; index < 6; index++) await act(async () => {await new Promise(resolve => setTimeout(resolve, 0));});}
async function render(messages = page(), identity = user(), accessBlocked = false, disabled = false) {
  await act(async () => root.render(<ApiViewProvider session={identity}><TaskConversationReply session={identity}
    project="project-a" taskId="task-a" page={messages} accessBlocked={accessBlocked} disabled={disabled} refresh={0}
    onMessagesRefresh={refreshed} onLockChange={lock}/></ApiViewProvider>)); await settle();
}
async function click(selector: string) {const element = host.querySelector<HTMLButtonElement>(selector); expect(element).not.toBeNull();
  await act(async () => element!.click()); await settle();}
async function change(selector: string, value: string) {
  const element = host.querySelector<HTMLInputElement | HTMLSelectElement>(selector)!; expect(element).not.toBeNull();
  await act(async () => {const proto = element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value); element.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', {bubbles: true}));}); await settle();
}
async function configure() {await click('[data-reply-toggle]'); await change('[data-reply-profile]', profile.id);
  await change('[data-model-select]', sha('a')); await change('[data-model-token-budget]', '600'); await click('[data-reply-consent]');}
const writes = () => requests.filter(request => !!request.body);

it('T128 starts only an explicit saved-note reply with the selected model, one-call parameters and budget, preserving bilingual source text', async () => {
  await render(); expect(requests).toHaveLength(0); await configure();
  expect(host.textContent).toContain('Selected model context window: 32 tokens'); expect(writes()).toHaveLength(0);
  await render(page(), user(), false, true); expect(host.querySelector<HTMLButtonElement>('[data-reply-create]')!.disabled).toBe(true);
  await render(); await click('[data-reply-create]'); expect(writes()).toHaveLength(1);
  expect(writes()[0].body).toMatchObject({project_id: 'project-a', task_id: 'task-a', expected_task_version: 7,
    expected_last_sequence: 1, reply_to_sequence: 1, context_message_limit: 20, tool_profile_id: profile.id, expected_tool_version: 2,
    acknowledge_model_use: true, model_execution_selection: {deployment_id: deployment.deployment_id,
      parameters: {max_steps: 1, max_output_tokens: 512}, budget: {token_limit: 600}}});
  expect(writes()[0].headers.get('X-Suma-Expected-User')).toBe('user-a');
  expect(host.textContent).toContain('The original agent is generating a reply');
  await act(async () => setLocale('zh-CN', {persist: false})); await settle();
  expect(host.textContent).toContain('原智能体正在生成回复'); expect(writes()).toHaveLength(1);
  expect(host.textContent).not.toContain('回复已保存');
});

it('T128 retains the original unknown operation and validates the same binding before clearing its recovery row', async () => {
  const normal = send; let first = true;
  send = async (path, body) => {if (path === '/task-conversation-replies' && body && first) {
    first = false; server = reply(body); throw new TypeError('Original committed ACK lost');} return normal(path, body);};
  await render(); await configure(); await click('[data-reply-create]'); expect(host.querySelector('[data-reply-restore]')).not.toBeNull();
  const original = structuredClone(writes()[0].body);
  await render(page(2)); await click('[data-reply-restore]'); expect(writes()).toHaveLength(2);
  expect(writes()[1].body).toEqual(original); expect(host.querySelector('[data-reply-restore]')).toBeNull();
  expect(writes().every(request => request.headers.get('X-Suma-Expected-Organization') === 'org-a')).toBe(true);
});

it('T128 uses actual run events only to refresh persisted reply facts, keeps stop-request distinct and supports an explicit next turn', async () => {
  await render(); await configure(); await click('[data-reply-create]'); expect(streams.length).toBeGreaterThan(0);
  const first = streams[streams.length - 1], before = writes().length;
  await act(async () => first.onEvent({type: 'run_succeeded'} as Parameters<typeof first.onEvent>[0])); await settle();
  expect(host.textContent).not.toContain('The original result was verified and stored'); expect(writes()).toHaveLength(before);
  const normal = send;
  send = async (path, body) => {if (path.endsWith('/commands') && body) {
    server = {...server!, state: 'cancelling', version: 2, run_version: 2, run_state: 'cancelling', allowed_actions: []}; return Response.json(server);}
    return normal(path, body);};
  await click('[data-reply-cancel]'); expect(host.textContent).toContain('Actual stopping must still be confirmed');
  server = {...server!, state: 'completed', version: 3, run_version: 3, run_state: 'succeeded', message_id: 'assistant-1', message_sequence: 2};
  await click('[data-reply-refresh]'); expect(host.textContent).toContain('The original result was verified and stored');
  expect(refreshed).toHaveBeenCalled(); await render(page(3));
  expect(host.querySelector('[data-reply-profile]')).not.toBeNull(); expect(host.querySelector<HTMLInputElement>('[data-reply-consent]')!.checked).toBe(false);
  expect(writes().filter(request => request.path === '/task-conversation-replies')).toHaveLength(1);
});

it('T128 clears old reply facts and aborts original streams on permission or account ownership changes', async () => {
  await render(); await configure(); await click('[data-reply-create]'); const originalStream = streams[streams.length - 1];
  const normal = send; send = async (path, body) => path.startsWith('/task-conversation-replies/reply-1?')
    ? Response.json({error: {code: 'forbidden', message: 'Current access revoked'}}, {status: 403}) : normal(path, body);
  await click('[data-reply-refresh]'); expect(host.textContent).toContain('Protected reply details are hidden');
  expect(host.textContent).not.toContain('Explicit model'); expect(originalStream.signal.aborted).toBe(true);
  await render(page(), user('user-b', 'org-b')); expect(host.querySelector('[data-reply-restore]')).toBeNull();
  expect(host.textContent).not.toContain('The original agent is generating a reply');
});
