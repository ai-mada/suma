import type {TaskConversationReply, TaskConversationReplyPage, TaskConversationReplySource, ToolProfile} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validConversationMessage} from './task-conversation-model';
import {validModelBinding} from './model-execution-model';
import {modelBindingMatchesSelection, validModelSelection} from './model-execution-input';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const closed = (value: Record<string, unknown>, names: string[]) => Object.keys(value).sort().join(',') === [...names].sort().join(',');
const text = (value: unknown): value is string => typeof value === 'string';
const natural = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value >= 0;
const positive = (value: unknown): value is number => natural(value) && value > 0;
export const replyID = (value: unknown): value is string => text(value) && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const runID = (value: unknown): value is string => text(value) && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const hash = (value: unknown): value is string => text(value) && /^[a-f0-9]{64}$/.test(value);
const stamp = (value: unknown) => text(value) && Number.isFinite(Date.parse(value));
const record = (value: unknown) => object(value) && closed(value, ['id', 'version', 'title', 'description', 'acceptance_criteria']) &&
  replyID(value.id) && positive(value.version) && text(value.title) && text(value.description) &&
  Array.isArray(value.acceptance_criteria) && value.acceptance_criteria.every(text);
export const replyStates = ['running', 'reconciling', 'cancelling', 'completed', 'failed', 'cancelled'] as const;
export const activeReply = (value: TaskConversationReply) => ['running', 'reconciling', 'cancelling'].includes(value.state);

/** The complete frozen context is checked before display. A context hash is
 * identity, never a grant; the server still owns source and run authorization. */
export function validReplySource(value: unknown, project: string, task: string, tenant: string): value is TaskConversationReplySource {
  if (!object(value) || !closed(value, ['sha256', 'task', 'requirement', 'task_run_id', 'messages', 'coverage', 'reply_to_sequence', 'execution_route']) ||
    !hash(value.sha256) || !record(value.task) || !object(value.task) || value.task.id !== task ||
    value.requirement !== null && !record(value.requirement) || value.task_run_id !== '' && !runID(value.task_run_id) ||
    !Array.isArray(value.messages) || !value.messages.length || value.messages.length > 20 || !value.messages.every(validConversationMessage) ||
    !positive(value.reply_to_sequence) || !object(value.coverage) || !object(value.execution_route)) return false;
  const messages = value.messages, coverage = value.coverage, route = value.execution_route;
  if (!closed(coverage, ['total', 'included', 'first_sequence', 'last_sequence', 'complete']) ||
    !positive(coverage.total) || coverage.included !== messages.length || coverage.total < messages.length ||
    coverage.first_sequence !== messages[0].sequence || coverage.last_sequence !== messages[messages.length - 1].sequence ||
    typeof coverage.complete !== 'boolean' || coverage.complete !== (coverage.total === messages.length) ||
    new Set(messages.map(message => message.id)).size !== messages.length ||
    messages.some((message, index) => index > 0 && messages[index - 1].sequence >= message.sequence) ||
    !messages.some(message => message.sequence === value.reply_to_sequence && ['human', 'api'].includes(message.actor_kind))) return false;
  return closed(route, ['tenant_id', 'project_id', 'version', 'pool_id', 'grant_id', 'grant_version']) &&
    route.tenant_id === tenant && replyID(route.tenant_id) && route.project_id === project && natural(route.version) &&
    (route.pool_id === '' ? route.grant_id === '' && route.grant_version === 0 : replyID(route.pool_id) && replyID(route.grant_id) && positive(route.grant_version));
}

const replyKeys = ['id', 'project_id', 'task_id', 'version', 'state', 'locale', 'run_id', 'run_version', 'run_state', 'source',
  'tool_profile_id', 'tool_profile_version', 'tool_profile_sha256', 'assistant_name', 'model_execution', 'message_id', 'message_sequence',
  'created_by', 'created_at', 'updated_at', 'basis_current', 'reason', 'allowed_actions'];
export function validReply(value: unknown, project: string, task: string, tenant: string): value is TaskConversationReply {
  if (!object(value) || !closed(value, replyKeys) || value.project_id !== project || value.task_id !== task ||
    !replyID(value.id) || !positive(value.version) || !replyStates.includes(value.state as typeof replyStates[number]) ||
    !['en-US', 'zh-CN'].includes(String(value.locale)) || !runID(value.run_id) || !positive(value.run_version) || !text(value.run_state) ||
    !validReplySource(value.source, project, task, tenant) || !replyID(value.tool_profile_id) || !positive(value.tool_profile_version) ||
    !hash(value.tool_profile_sha256) || !text(value.assistant_name) || !validModelBinding(value.model_execution) ||
    value.model_execution.parameters.max_steps !== 1 || value.model_execution.parameters.max_output_tokens > 8192 ||
    !replyID(value.created_by) || !stamp(value.created_at) || !stamp(value.updated_at) || typeof value.basis_current !== 'boolean' ||
    !text(value.reason) || !Array.isArray(value.allowed_actions) || value.allowed_actions.some(action => action !== 'cancel') ||
    new Set(value.allowed_actions).size !== value.allowed_actions.length) return false;
  return value.run_id !== value.source.task_run_id && (value.state === 'completed'
    ? replyID(value.message_id) && positive(value.message_sequence) && value.message_sequence > value.source.coverage.last_sequence
    : value.message_id === '' && value.message_sequence === 0);
}

export function validReplyPage(value: unknown, project: string, task: string, tenant: string): value is TaskConversationReplyPage {
  return object(value) && closed(value, ['items', 'total', 'limit', 'next_cursor']) && natural(value.total) && value.limit === 20 &&
    Array.isArray(value.items) && value.items.length <= 20 && value.items.length <= value.total &&
    value.items.every(item => validReply(item, project, task, tenant)) && new Set(value.items.map(item => item.id)).size === value.items.length &&
    (value.next_cursor === '' || replyID(value.next_cursor));
}

/** Discovery labels cannot turn an ambient-model or host-writing command into
 * the dedicated read-only, gateway-bound conversation capability. */
export function replyProfile(value: ToolProfile): boolean {
  const profile = value.profile;
  return value.state === 'enabled' && value.available && profile.adapter === 'suma' && profile.protocol === 'suma-jsonl-v1' &&
    profile.read_only === true && profile.model_execution_protocol === 'suma-model-execution-v1' &&
    profile.task_conversation_reply?.protocol === 'suma-task-conversation-reply-v1' &&
    !profile.workspace_tools_version && !profile.test_report_protocol && !profile.conflict_repair_protocol &&
    !profile.planning && !profile.knowledge_extraction && !profile.failure_diagnosis &&
    !profile.intelligent_operations && !profile.dispatch_recommendation && !profile.opencode_execution && !profile.octop_execution;
}
export type ReplyProfilePage = {items: ToolProfile[]; total: number; next_cursor?: string};
export function validReplyProfiles(value: unknown, project: string): value is ReplyProfilePage {
  return object(value) && Array.isArray(value.items) && value.items.length <= 50 && natural(value.total) && value.total >= value.items.length &&
    value.items.every(item => object(item) && replyID(item.id) && item.project_id === project && positive(item.version) && text(item.name) &&
      text(item.state) && typeof item.available === 'boolean' && object(item.profile) && hash(item.profile.profile_sha256)) &&
    new Set(value.items.map(item => item.id)).size === value.items.length &&
    (value.next_cursor === undefined || text(value.next_cursor) && value.next_cursor.length <= 4096 && !/[\x00-\x1f\x7f]/.test(value.next_cursor));
}

export function validReplyRequest(request: WriteRecoveryRequest, project: string, task: string): boolean {
  const body = request.body;
  if (request.method !== 'POST' || body.project_id !== project || body.idempotency_key !== request.idempotency_key) return false;
  if (request.path === '/task-conversation-replies') return closed(body, ['project_id', 'task_id', 'expected_task_version', 'expected_last_sequence',
    'reply_to_sequence', 'context_message_limit', 'tool_profile_id', 'expected_tool_version', 'model_execution_selection', 'locale', 'acknowledge_model_use', 'idempotency_key']) &&
    body.task_id === task && positive(body.expected_task_version) && positive(body.expected_last_sequence) &&
    positive(body.reply_to_sequence) && body.reply_to_sequence <= body.expected_last_sequence &&
    positive(body.context_message_limit) && body.context_message_limit <= 20 && replyID(body.tool_profile_id) &&
    positive(body.expected_tool_version) && validModelSelection(body.model_execution_selection) &&
    body.model_execution_selection.parameters.max_steps === 1 && body.model_execution_selection.parameters.max_output_tokens <= 8192 &&
    ['zh-CN', 'en-US'].includes(String(body.locale)) && body.acknowledge_model_use === true;
  const match = /^\/task-conversation-replies\/([A-Za-z0-9_.-]{1,64})\/commands$/.exec(request.path);
  return !!match && replyID(match[1]) && closed(body, ['project_id', 'expected_version', 'expected_run_version', 'action', 'idempotency_key']) &&
    positive(body.expected_version) && positive(body.expected_run_version) && body.action === 'cancel';
}

/** An HTTP201 settles only the exact original human request and binding. */
export function validReplyReceipt(value: unknown, request: WriteRecoveryRequest, project: string, task: string, user: string, tenant: string): boolean {
  if (!validReply(value, project, task, tenant)) return false;
  if (request.path.endsWith('/commands')) return request.path === '/task-conversation-replies/' + value.id + '/commands' &&
    value.version >= Number(request.body.expected_version);
  return value.created_by === user && value.source.task.version === request.body.expected_task_version &&
    value.source.coverage.last_sequence === request.body.expected_last_sequence && value.source.reply_to_sequence === request.body.reply_to_sequence &&
    value.source.coverage.included === Math.min(Number(request.body.context_message_limit), value.source.coverage.total) &&
    value.tool_profile_id === request.body.tool_profile_id && value.tool_profile_version === request.body.expected_tool_version &&
    value.locale === request.body.locale && modelBindingMatchesSelection(value.model_execution, request.body.model_execution_selection);
}
