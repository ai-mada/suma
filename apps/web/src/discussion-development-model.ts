import type {TaskDiscussionDevelopmentPlan} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validConversationMessage} from './task-conversation-model';
import {validModelDispatchReceipt, validModelPlan} from './model-execution-model';
import {validModelSelection} from './model-execution-input';
import {sameKnowledgePins, validKnowledgeContext} from './task-conversation-knowledge-model';
import {codexBindingPresent,sameCodexBinding,validCodexDispatchReceipt,validCodexSelection} from './codex-execution-model';
import {validExecutionSkillPlan} from './codex-native-skills-model';
import {validOpenCodeDispatchReceipt,validOpenCodePlan} from './opencode-execution-model';
import {validClaudeDispatchReceipt,validClaudePlan} from './claude-cli-model';
import {validACPDispatchReceipt,validACPPlan} from './acp-execution-model';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const closed = (value: Record<string, unknown>, names: string[]) => Object.keys(value).sort().join(',') === [...names].sort().join(',');
const fields = (value: Record<string, unknown>, required: string[], optional: string[]) => required.every(key=>Object.hasOwn(value,key)) && Object.keys(value).every(key=>required.includes(key)||optional.includes(key));
const id = (value: unknown): value is string => typeof value === 'string' && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const positive = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0;
const hash = (value: unknown) => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const text = (value: unknown, limit: number): value is string => typeof value === 'string' && !!value.trim() &&
  !value.includes('\0') && !/[\uD800-\uDFFF]/u.test(value) && new TextEncoder().encode(value).length <= limit;
export const developmentGoal = (value: unknown): value is string => text(value, 8000);
export const developmentCriteria = (value: unknown): value is string[] => Array.isArray(value) && value.length > 0 && value.length <= 32 && value.every(item => text(item, 1000));

/** The dispatcher owns eligibility. A readable preview still needs one exact
 * producer and mutually exclusive, complete bindings before exposing actions. */
function validDevelopmentExecution(plan:Record<string,unknown>):boolean {
  if (!validModelPlan(plan)||!validOpenCodePlan(plan)||!validClaudePlan(plan)||!validACPPlan(plan)||!validExecutionSkillPlan(plan)||
    !object(plan.workflow)||!Array.isArray(plan.workflow.nodes)) return false;
  const matching=plan.workflow.nodes.filter(node=>object(node)&&node.id===plan.agent_node_id),node=matching[0];
  if(matching.length!==1||!object(node)||node.type!=='agent'||!object(node.config)||
    node.config.tool_profile_id!==plan.tool_profile_id||node.config.tool_profile_sha256!==plan.tool_profile_sha256)return false;
  const bindingFields=['model_execution','codex_execution','opencode_execution','claude_execution','acp_execution'];
  const present=bindingFields.filter(key=>Object.hasOwn(plan,key));
  if(present.length>1||bindingFields.some(key=>Object.hasOwn(plan,key)!==Object.hasOwn(node.config as Record<string,unknown>,key))||
    present.some(key=>plan[key]===undefined||plan[key]===null))return false;
  if(Object.hasOwn(plan,'codex_execution')){
    if(!codexBindingPresent(plan.codex_execution)||!sameCodexBinding(node.config.codex_execution,plan.codex_execution)||
      node.max_retries!==undefined&&node.max_retries!==0)return false;
    const knowledge=object(plan.context)&&Array.isArray(plan.context.knowledge)?plan.context.knowledge:[];
    if(knowledge.some(item=>object(item)&&item.kind==='skill')&&!Object.hasOwn(plan,'execution_skills'))return false;
  }
  return true;
}

/** The reply is provenance. Only the freshly reviewed task, tool and model
 * binding can authorize the separate code-development plan. */
export function validDiscussionDevelopment(value: unknown, project: string, task: string): value is TaskDiscussionDevelopmentPlan {
  if (!object(value) || !closed(value, ['id', 'project_id', 'task_id', 'version', 'reply_id', 'reply_version', 'message',
    'source_sha256', 'reviewed_goal', 'acceptance_criteria', 'created_by', 'created_at', 'dispatch_plan']) ||
    !id(value.id) || value.project_id !== project || value.task_id !== task || !positive(value.version) ||
    !id(value.reply_id) || !positive(value.reply_version) || !hash(value.source_sha256) || !validConversationMessage(value.message) ||
    value.message.actor_kind !== 'assistant' || value.message.reply_id !== value.reply_id || !developmentGoal(value.reviewed_goal) ||
    !developmentCriteria(value.acceptance_criteria) || !id(value.created_by) || typeof value.created_at !== 'string' ||
    !Number.isFinite(Date.parse(value.created_at)) || !object(value.dispatch_plan)) return false;
  const plan = value.dispatch_plan;
  if (!object(plan.context) || plan.context.knowledge !== undefined && !validKnowledgeContext(plan.context.knowledge) ||
    !sameKnowledgePins(value.message.knowledge || [], (plan.context.knowledge || []) as NonNullable<TaskDiscussionDevelopmentPlan['dispatch_plan']['context']['knowledge']>)) return false;
  return plan.id === value.id && plan.version === value.version && plan.project_id === project && plan.task_id === task &&
    positive(plan.task_version) && id(plan.workflow_id) && positive(plan.workflow_version) && id(plan.agent_node_id) &&
    id(plan.tool_profile_id) && positive(plan.tool_profile_version) && hash(plan.tool_profile_sha256) &&
    typeof plan.valid === 'boolean' && typeof plan.reason === 'string' &&
    ['ready', 'started'].includes(String(plan.state)) && typeof plan.run_id === 'string' && !!plan.run_id &&
    object(plan.workflow) && typeof plan.workflow.id === 'string' && positive(plan.workflow.version) &&
    Array.isArray(plan.workflow.nodes) && validDevelopmentExecution(plan);
}

export function validDiscussionDevelopmentRequest(request: WriteRecoveryRequest, project: string, task: string): boolean {
  const body = request.body;
  if (request.method !== 'POST' || body.project_id !== project || body.idempotency_key !== request.idempotency_key) return false;
  if (request.path === '/task-discussion-development-plans') return fields(body, ['project_id', 'task_id', 'expected_task_version',
    'reply_id', 'expected_reply_version', 'workflow_id', 'workflow_version', 'agent_node_id', 'tool_profile_id', 'expected_tool_version',
    'reviewed_goal', 'acceptance_criteria', 'acknowledge_execution', 'idempotency_key'],['model_execution_selection','codex_execution_selection']) &&
    body.task_id === task && positive(body.expected_task_version) && id(body.reply_id) && positive(body.expected_reply_version) &&
    id(body.workflow_id) && positive(body.workflow_version) && id(body.agent_node_id) && id(body.tool_profile_id) &&
    positive(body.expected_tool_version) && !(Object.hasOwn(body,'model_execution_selection')&&Object.hasOwn(body,'codex_execution_selection')) &&
    (!Object.hasOwn(body,'model_execution_selection')||validModelSelection(body.model_execution_selection)) &&
    (!Object.hasOwn(body,'codex_execution_selection')||validCodexSelection(body.codex_execution_selection)) && developmentGoal(body.reviewed_goal) &&
    developmentCriteria(body.acceptance_criteria) && body.acknowledge_execution === true;
  return /^\/task-discussion-development-plans\/[A-Za-z0-9_.-]{1,64}\/start$/.test(request.path) &&
    closed(body, ['project_id', 'expected_version', 'idempotency_key']) && positive(body.expected_version);
}

export function validDiscussionDevelopmentReceipt(value: unknown, request: WriteRecoveryRequest, project: string, task: string, user: string): boolean {
  if (!validDiscussionDevelopmentRequest(request,project,task)||request.path!=='/task-discussion-development-plans'||!validDiscussionDevelopment(value, project, task)) return false;
  const body = request.body;
  const plan=value.dispatch_plan;
  // Only pins already checked against the protected assistant message may be
  // projected for the ordinary Codex receipt guard; never accept request pins.
  const codexBody={...body,knowledge:(plan.context.knowledge||[]).map(({id,revision,sha256})=>({id,revision,sha256}))};
  return value.created_by === user && value.reply_id === body.reply_id && value.reply_version === body.expected_reply_version &&
    value.reviewed_goal === body.reviewed_goal && JSON.stringify(value.acceptance_criteria) === JSON.stringify(body.acceptance_criteria) &&
    plan.project_id===body.project_id&&plan.task_id===body.task_id&&plan.task_version===body.expected_task_version&&
    plan.workflow_id===body.workflow_id&&plan.workflow_version===body.workflow_version&&plan.agent_node_id===body.agent_node_id&&
    plan.tool_profile_id===body.tool_profile_id&&plan.tool_profile_version===body.expected_tool_version&&
    Object.hasOwn(body,'codex_execution_selection')===Object.hasOwn(plan,'codex_execution')&&
    validModelDispatchReceipt(plan,body)&&validCodexDispatchReceipt(plan,codexBody)&&
    validOpenCodeDispatchReceipt(plan,body)&&validClaudeDispatchReceipt(plan,body)&&validACPDispatchReceipt(plan,body);
}
