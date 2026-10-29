import type {TaskDiscussionDevelopmentPlan} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validConversationMessage} from './task-conversation-model';
import {validModelDispatchReceipt, validModelPlan} from './model-execution-model';
import {validModelSelection} from './model-execution-input';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const closed = (value: Record<string, unknown>, names: string[]) => Object.keys(value).sort().join(',') === [...names].sort().join(',');
const id = (value: unknown): value is string => typeof value === 'string' && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const positive = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0;
const hash = (value: unknown) => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const text = (value: unknown, limit: number): value is string => typeof value === 'string' && !!value.trim() &&
  !value.includes('\0') && !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= limit;
export const developmentGoal = (value: unknown): value is string => text(value, 8000);
export const developmentCriteria = (value: unknown): value is string[] => Array.isArray(value) && value.length > 0 && value.length <= 32 && value.every(item => text(item, 1000));

/** The reply is provenance. Only the freshly reviewed task, tool and model
 * binding can authorize the separate code-development plan. */
export function validDiscussionDevelopment(value: unknown, project: string, task: string): value is TaskDiscussionDevelopmentPlan {
  if (!object(value) || !closed(value, ['id', 'project_id', 'task_id', 'version', 'reply_id', 'reply_version', 'message',
    'source_sha256', 'reviewed_goal', 'acceptance_criteria', 'created_by', 'created_at', 'dispatch_plan']) ||
    !id(value.id) || value.project_id !== project || value.task_id !== task || !positive(value.version) ||
    !id(value.reply_id) || !positive(value.reply_version) || !hash(value.source_sha256) || !validConversationMessage(value.message) ||
    value.message.actor_kind !== 'assistant' || value.message.reply_id !== value.reply_id || !developmentGoal(value.reviewed_goal) ||
    !developmentCriteria(value.acceptance_criteria) || !id(value.created_by) || typeof value.created_at !== 'string' ||
    !Number.isFinite(Date.parse(value.created_at)) || !object(value.dispatch_plan) || !validModelPlan(value.dispatch_plan)) return false;
  const plan = value.dispatch_plan;
  return plan.id === value.id && plan.version === value.version && plan.project_id === project && plan.task_id === task &&
    positive(plan.task_version) && typeof plan.valid === 'boolean' && typeof plan.reason === 'string' &&
    ['ready', 'started'].includes(String(plan.state)) && typeof plan.run_id === 'string' && !!plan.run_id &&
    object(plan.workflow) && typeof plan.workflow.id === 'string' && positive(plan.workflow.version) &&
    Array.isArray(plan.workflow.nodes) && object(plan.model_execution);
}

export function validDiscussionDevelopmentRequest(request: WriteRecoveryRequest, project: string, task: string): boolean {
  const body = request.body;
  if (request.method !== 'POST' || body.project_id !== project || body.idempotency_key !== request.idempotency_key) return false;
  if (request.path === '/task-discussion-development-plans') return closed(body, ['project_id', 'task_id', 'expected_task_version',
    'reply_id', 'expected_reply_version', 'workflow_id', 'workflow_version', 'agent_node_id', 'tool_profile_id', 'expected_tool_version',
    'model_execution_selection', 'reviewed_goal', 'acceptance_criteria', 'acknowledge_execution', 'idempotency_key']) &&
    body.task_id === task && positive(body.expected_task_version) && id(body.reply_id) && positive(body.expected_reply_version) &&
    id(body.workflow_id) && positive(body.workflow_version) && id(body.agent_node_id) && id(body.tool_profile_id) &&
    positive(body.expected_tool_version) && validModelSelection(body.model_execution_selection) && developmentGoal(body.reviewed_goal) &&
    developmentCriteria(body.acceptance_criteria) && body.acknowledge_execution === true;
  return /^\/task-discussion-development-plans\/[A-Za-z0-9_.-]{1,64}\/start$/.test(request.path) &&
    closed(body, ['project_id', 'expected_version', 'idempotency_key']) && positive(body.expected_version);
}

export function validDiscussionDevelopmentReceipt(value: unknown, request: WriteRecoveryRequest, project: string, task: string, user: string): boolean {
  if (!validDiscussionDevelopment(value, project, task)) return false;
  const body = request.body;
  return value.created_by === user && value.reply_id === body.reply_id && value.reply_version === body.expected_reply_version &&
    value.reviewed_goal === body.reviewed_goal && JSON.stringify(value.acceptance_criteria) === JSON.stringify(body.acceptance_criteria) &&
    validModelDispatchReceipt(value.dispatch_plan, body);
}
