// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect, it, vi} from 'vitest';
import type {ModelBinding, ModelExecutionOptions, TaskConversationMessage, TaskConversationPage, TaskConversationReply,
  TaskDiscussionDevelopmentPlan, TaskDispatchOptions, TaskDispatchPlan, ToolProfile, UsageRateRevision, Workflow} from '../../../packages/contracts/src';
import {DiscussionDevelopmentPanel} from './DiscussionDevelopmentPanel';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {modelBinding} from './model-execution.test-support';

const project = 'project-a', task = 'task-a', stamp = '2026-10-29T05:00:00Z';
const identity: Session = {user: {id: 'user-a', name: 'Owner', email: 'owner@example.test'}, organization: {id: 'org-a', name: 'Org'}, projects: []};
const sourceTask = {id: task, version: 7, title: 'Original task', description: 'Original formal scope', acceptance_criteria: ['Original formal criterion']};
const note: TaskConversationMessage = {id: 'note-one', sequence: 1, task_version: 7, body: 'Discuss the change', author_id: 'user-a', author_name: 'Owner',
  actor_kind: 'human', created_at: stamp, requirement_id: '', requirement_version: 0, run_id: '', reply_id: '', reply_to_sequence: 0, execution_run_id: ''};
const assistant: TaskConversationMessage = {...note, id: 'assistant-one', sequence: 2, body: 'Original assistant proposal', actor_kind: 'assistant',
  author_id: 'reply-profile', author_name: 'Reply expert', reply_id: 'reply-one', reply_to_sequence: 1, execution_run_id: 'reply-run'};
const discussion = (version = 7): TaskConversationPage => ({project_id: project, task_id: task, task_version: version, can_post: true,
  items: [note, assistant], total: 2, limit: 20, next_before: 0});
const previousReply: TaskConversationReply = {id: 'reply-one', project_id: project, task_id: task, version: 3, state: 'completed', locale: 'en-US',
  run_id: 'reply-run', run_version: 3, run_state: 'succeeded', source: {sha256: '1'.repeat(64), task: sourceTask, requirement: null, task_run_id: '',
    messages: [note], coverage: {total: 1, included: 1, first_sequence: 1, last_sequence: 1, complete: true}, reply_to_sequence: 1,
    execution_route: {tenant_id: 'org-a', project_id: project, version: 0, pool_id: '', grant_id: '', grant_version: 0}},
  tool_profile_id: 'reply-profile', tool_profile_version: 2, tool_profile_sha256: '2'.repeat(64), assistant_name: 'Reply expert',
  model_execution: {...modelBinding, deployment: {...modelBinding.deployment, deployment_id: 'old-reply-model'},
    parameters: {max_steps: 1, max_output_tokens: 512}, input_token_reservation: 8192, output_token_reservation: 512, budget: {token_limit: 10000}},
  message_id: assistant.id, message_sequence: 2, created_by: 'user-a', created_at: stamp, updated_at: stamp,
  basis_current: true, reason: '', allowed_actions: []};
const tool: ToolProfile = {id: 'native-profile', project_id: project, worker_id: 'worker-a', name: 'Native coding expert', version: 4, state: 'enabled',
  profile: {command_id: 'native-code', adapter: 'suma', protocol: 'suma-jsonl-v1', profile_sha256: '3'.repeat(64), executable_sha256: '4'.repeat(64),
    tool_version: '1.0.0', read_only: false, workspace_tools_version: 'attempt-workspace-v1', model_execution_protocol: 'suma-model-execution-v1'},
  enabled_by: 'user-a', created_at: stamp, updated_at: stamp, available: true, reason: ''};
tool.observation = {profile: {...tool.profile}, available: true, reason: ''};
const workflow: Workflow = {id: 'flow-native', version: 2, name: 'Develop and independently verify', nodes: [
  {id: 'code', name: 'Develop', type: 'agent', command: [tool.profile.command_id], depends_on: [], join: 'all', quorum: 0,
    timeout_seconds: 300, max_retries: 0, idempotent: false},
  {id: 'verify', name: 'Independent tests', type: 'test', command: ['independent-test'], depends_on: ['code'], join: 'all', quorum: 0,
    timeout_seconds: 300, max_retries: 0, idempotent: false, config: {candidate_from: 'code'}}], edges: [{from: 'code', to: 'verify'}]};
const dispatchOptions: TaskDispatchOptions = {project_id: project, task_id: task, task_version: 7, workflow_id: workflow.id, workflow_version: 2,
  agent_node_id: 'code', observed_at: stamp, context: {task: sourceTask}, execution_route: previousReply.source.execution_route,
  items: [{tool, eligible: true, reason_code: 'eligible', reason: '', model_state: 'fixed_by_tool', cost_state: 'unknown', eligible_basis: 'current_observation_and_compatibility'}],
  total: 1, next_cursor: '', recommended_tool_profile_id: tool.id};
const rateSnapshot = modelBinding.rate;
if (!rateSnapshot) throw new Error('The shared priced-model fixture must include its frozen rate snapshot');
const currentRate: UsageRateRevision = {...rateSnapshot, project_id: project, tool_profile_id: tool.id, profile_sha256: tool.profile.profile_sha256,
  model: modelBinding.deployment.model, endpoint_origin: modelBinding.deployment.endpoint_origin,
  model_deployment_id: modelBinding.deployment.deployment_id, model_deployment_revision: modelBinding.deployment.revision,
  model_deployment_sha256: modelBinding.deployment.deployment_sha256, input_token_reservation: modelBinding.input_token_reservation,
  output_token_reservation: modelBinding.output_token_reservation, reason: 'Reviewed model-rate protocol fixture', created_by: identity.user.id, created_at: stamp};
const models: ModelExecutionOptions = {project_id: project, tool_profile_id: tool.id, tool_profile_version: tool.version, tool_profile_sha256: tool.profile.profile_sha256,
  observed_at: stamp, supported: true, reason: '', codex_supported: false, codex_reason: '', parameter_limits: {max_steps: 128, max_output_tokens: 131072},
  items: [{deployment: modelBinding.deployment, cost_state: 'priced', rate: currentRate, default_parameters: modelBinding.parameters,
    input_token_reservation: modelBinding.input_token_reservation, output_token_reservation: modelBinding.output_token_reservation}]};

// Exercises the real panel, option/model selectors, durable write validation
// and original-run receipt read. HTTP/model facts here are protocol fixtures.
it('T129 reviews fresh coding selection and edited instructions, creates only a preview, then explicitly starts without accepting work', async () => {
  await setLocale('en-US', {persist: false}); sessionStorage.clear(); vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT', true);
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host);
  const writes: {path: string; body: Record<string, any>; headers: Headers}[] = [];
  let saved: TaskDiscussionDevelopmentPlan | undefined;
  const onRun = vi.fn(), lock = vi.fn();
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const path = String(input).replace('/api/v1', ''), body = init?.body ? JSON.parse(String(init.body)) : undefined;
    if (body) writes.push({path, body, headers: new Headers(init?.headers)});
    if (path.startsWith('/workflows?')) return Response.json({items: [workflow]});
    if (path.startsWith('/tasks/task-a/dispatch-options?')) return Response.json(dispatchOptions);
    if (path.startsWith('/model-execution/options?')) return Response.json(models);
    if (path === '/task-discussion-development-plans' && body) {
      const binding: ModelBinding = {...modelBinding, parameters: body.model_execution_selection.parameters, budget: body.model_execution_selection.budget};
      const plan: TaskDispatchPlan = {id: 'development-plan', project_id: project, version: 1, state: 'ready', task_id: task, task_version: 7,
        workflow_id: workflow.id, workflow_version: 2, agent_node_id: 'code', tool_profile_id: tool.id, tool_profile_version: tool.version,
        tool_profile_sha256: tool.profile.profile_sha256, run_id: 'development-run', created_at: stamp, expires_at: '2099-10-29T06:00:00Z', context: {task: sourceTask},
        workflow: {...workflow, id: 'fixed-development-flow', version: 1, nodes: [{...workflow.nodes[0], max_retries: 0,
          config: {goal: body.reviewed_goal, tool_profile_id: tool.id, tool_profile_sha256: tool.profile.profile_sha256, model_execution: binding}}, workflow.nodes[1]]},
        model_execution: binding, valid: true, reason: ''};
      saved = {id: plan.id, project_id: project, task_id: task, version: plan.version, reply_id: previousReply.id, reply_version: previousReply.version,
        source_sha256: '5'.repeat(64), message: assistant, reviewed_goal: body.reviewed_goal, acceptance_criteria: body.acceptance_criteria,
        created_by: 'user-a', created_at: stamp, dispatch_plan: plan};
      return Response.json(saved, {status: 201});
    }
    if (path === '/task-discussion-development-plans/development-plan/start' && body && saved) {
      saved = {...saved, version: 2, dispatch_plan: {...saved.dispatch_plan, version: 2, state: 'started'}};
      return Response.json({id: 'development-run', project_id: project, workflow_id: 'fixed-development-flow', workflow_version: 1, state: 'running'}, {status: 201});
    }
    if (path.startsWith('/task-discussion-development-plans/development-plan?') && saved) return Response.json(saved);
    return Response.json({error: {code: 'not_found', message: 'Unexpected fixture request'}}, {status: 404});
  }));
  const settle = async () => {for (let index = 0; index < 5; index++) await act(async () => {await new Promise(resolve => setTimeout(resolve, 0));});};
  const render = async (version = 7) => {await act(async () => root.render(<ApiViewProvider session={identity}><DiscussionDevelopmentPanel session={identity}
    project={project} taskId={task} reply={previousReply} page={discussion(version)} disabled={false} accessBlocked={false} onLockChange={lock} onRun={onRun}/></ApiViewProvider>)); await settle();};
  const click = async (selector: string) => {const element = host.querySelector<HTMLElement>(selector); expect(element).not.toBeNull(); await act(async () => element!.click()); await settle();};
  const change = async (selector: string, value: string) => {
    const element = host.querySelector<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>(selector); expect(element).not.toBeNull();
    await act(async () => {const proto = element instanceof HTMLSelectElement ? HTMLSelectElement.prototype : element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
      Object.getOwnPropertyDescriptor(proto, 'value')!.set!.call(element, value); element!.dispatchEvent(new Event(element instanceof HTMLSelectElement ? 'change' : 'input', {bubbles: true}));}); await settle();
  };
  try {
    await render(); expect(writes).toHaveLength(0); await click('[data-development-toggle]');
    await change('[data-development-goal]', 'Reviewed goal: implement the explicit behavior');
    await change('[data-development-criteria]', 'Verify the exact candidate\nCheck independent test results');
    await change('.discussion-development-fields select', 'flow-native@2');
    await click('.discussion-development-tools button');
    await change('[data-model-select]', modelBinding.deployment.deployment_sha256);
    await change('[data-model-token-budget]', '400000');
    await click('.discussion-development-consent input');
    expect(writes).toHaveLength(0);
    expect(host.querySelector<HTMLButtonElement>('[data-development-create]')!.disabled).toBe(false);
    await click('[data-development-create]');
    expect(writes).toHaveLength(1); expect(writes[0].path).toBe('/task-discussion-development-plans');
    expect(writes[0].body).toMatchObject({expected_task_version: 7, reply_id: 'reply-one', expected_reply_version: 3,
      reviewed_goal: 'Reviewed goal: implement the explicit behavior', acceptance_criteria: ['Verify the exact candidate', 'Check independent test results'],
      model_execution_selection: {deployment_id: modelBinding.deployment.deployment_id, parameters: {max_steps: 32, max_output_tokens: 65536}, budget: {token_limit: 400000}}});
    expect(writes[0].body.model_execution_selection.deployment_id).not.toBe(previousReply.model_execution!.deployment.deployment_id);
    expect(writes[0].headers.get('X-Suma-Expected-User')).toBe('user-a');
    expect(host.querySelector<HTMLButtonElement>('[data-development-start]')!.disabled).toBe(true); expect(onRun).not.toHaveBeenCalled();
    await click('.discussion-development-consent input'); await click('[data-development-start]');
    expect(writes).toHaveLength(2); expect(writes[1].path).toBe('/task-discussion-development-plans/development-plan/start');
    expect(writes[1].body.expected_version).toBe(1); expect(onRun).toHaveBeenCalledExactlyOnceWith('development-run');
    expect(writes.every(write => !write.path.includes('/review') && !write.path.includes('/accept'))).toBe(true);
    await render(8); // A current-context refresh retains the original plan/run.
    expect(host.querySelector('a[href*="run=development-run"]')).not.toBeNull(); expect(writes).toHaveLength(2);
    expect(host.textContent).toContain('Original assistant proposal');
  } finally {await act(async () => root.unmount()); host.remove(); vi.unstubAllGlobals(); vi.restoreAllMocks(); await setLocale('zh-CN', {persist: false});}
});
