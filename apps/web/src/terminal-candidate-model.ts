import type {WorkspaceTerminalCandidatePlan, WorkspaceTerminalCandidatePlanPage, WorkspaceTerminalSession} from '../../../packages/contracts/src';
import type {Workflow} from './api';
import type {ToolProfile} from './tool-model';
import type {WriteRecoveryRequest} from './write-action';
import {sameTerminalBaseline, terminalID, validTerminalBaseline} from './terminal-model';

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const keys = (value: Record<string, unknown>, expected: string[]) => Object.keys(value).sort().join(',') === [...expected].sort().join(',');
const positive = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0;
const digest = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const commit = (value: unknown): value is string => typeof value === 'string' && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
const date = (value: unknown): value is string => typeof value === 'string' && Number.isFinite(Date.parse(value));
const same = (left: unknown, right: unknown) => JSON.stringify(left, (_key, value: unknown) => object(value) ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b))) : value) ===
  JSON.stringify(right, (_key, value: unknown) => object(value) ? Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b))) : value);

/** Offer existing published workflows with a fixed command producer and
 * independent candidate-consuming tests. The server checks full execution rights. */
export function terminalCandidateProducers(flow?: Workflow) {
  return flow?.nodes.filter(node => node.type === 'command' && node.command?.length === 1 && !node.depends_on?.length &&
    !flow.edges.some(edge => edge.to === node.id) && !node.condition && flow.nodes.length === 2 &&
    flow.nodes.every(other => other === node || other.type === 'test' && other.command?.length === 1 &&
      other.config?.candidate_from === node.id && !other.condition &&
      (other.depends_on?.includes(node.id) || flow.edges.some(edge => edge.from === node.id && edge.to === other.id)))) || [];
}

export function validTerminalCandidateWorkflows(value: unknown): value is {items: Workflow[]} {
  return object(value) && Array.isArray(value.items) && value.items.length <= 1000 && value.items.every(flow => object(flow) && terminalID(flow.id) &&
    positive(flow.version) && typeof flow.name === 'string' && Array.isArray(flow.nodes) && flow.nodes.every(node => object(node) &&
      typeof node.id === 'string' && typeof node.type === 'string' && (node.depends_on === undefined || Array.isArray(node.depends_on)) &&
      (node.command === undefined || Array.isArray(node.command) && node.command.every(part => typeof part === 'string'))) &&
    Array.isArray(flow.edges) && flow.edges.every(edge => object(edge) && typeof edge.from === 'string' && typeof edge.to === 'string'));
}

export function validTerminalCandidateTools(value: unknown, project: string): value is {items: ToolProfile[]; total: number; next_cursor?: string} {
  return object(value) && Array.isArray(value.items) && value.items.length <= 100 && typeof value.total === 'number' && Number.isSafeInteger(value.total) &&
    value.total >= value.items.length && (value.next_cursor === undefined || typeof value.next_cursor === 'string' && value.next_cursor.length <= 4096) &&
    value.items.every(tool => object(tool) && terminalID(tool.id) && tool.project_id === project && positive(tool.version) &&
      typeof tool.name === 'string' && typeof tool.worker_id === 'string' && typeof tool.available === 'boolean' &&
      ['enabled', 'paused', 'revoked'].includes(String(tool.state)) && object(tool.profile) && typeof tool.profile.command_id === 'string' &&
      typeof tool.profile.read_only === 'boolean' && digest(tool.profile.profile_sha256));
}

export function terminalCandidateTool(tool: ToolProfile, worker: string, command?: string) {
  const profile = tool.profile;
  return tool.state === 'enabled' && tool.available && tool.worker_id === worker && profile.adapter === 'command' && profile.protocol === 'text-v1' &&
    !profile.read_only && !profile.terminal_session_protocol && !profile.model_execution_protocol && !profile.workspace_tools_version &&
    !profile.test_report_protocol && !profile.planning && !profile.knowledge_extraction && !profile.failure_diagnosis && !profile.intelligent_operations &&
    !profile.conflict_repair_protocol && !profile.opencode_execution && !profile.octop_execution && (!command || profile.command_id === command);
}

export function validTerminalCandidatePlan(value: unknown, project: string, task: string, session: string): value is WorkspaceTerminalCandidatePlan {
  if (!object(value) || !terminalID(value.id) || value.project_id !== project || value.task_id !== task || value.session_id !== session ||
    !positive(value.version) || !positive(value.session_version) || !positive(value.task_version) || !['ready', 'started'].includes(String(value.state)) ||
    !terminalID(value.run_id) || !date(value.created_at) || !date(value.expires_at) || !terminalID(value.tool_profile_id) ||
    !positive(value.tool_profile_version) || !digest(value.tool_profile_sha256) || !terminalID(value.worker_id) ||
    !terminalID(value.workflow_id) || !positive(value.workflow_version) || typeof value.producer_node_id !== 'string' || !value.producer_node_id ||
    typeof value.valid !== 'boolean' || typeof value.reason !== 'string' || !object(value.context) || !object(value.context.task) ||
    value.context.task.id !== task || value.context.task.version !== value.task_version || !object(value.workflow) ||
    !Array.isArray(value.workflow.nodes) || !Array.isArray(value.workflow.edges) || !object(value.source)) return false;
  const source = value.source;
  const contextBaseline = value.context.code_baseline === null ? undefined : value.context.code_baseline;
  if (!keys(source, ['session_id', 'session_version', 'original', 'base_commit', 'admin_base_commit', 'snapshot_sha256', 'plan_id',
    ...(Object.hasOwn(source, 'code_baseline') ? ['code_baseline'] : []), ...(Object.hasOwn(source, 'workbuddy_origin') ? ['workbuddy_origin'] : [])]) ||
    source.session_id !== session || source.session_version !== value.session_version || source.plan_id !== value.id ||
    !commit(source.base_commit) || !commit(source.admin_base_commit) || !digest(source.snapshot_sha256) || !object(source.original) ||
    !validTerminalBaseline(source.code_baseline, project, task, source.base_commit, source.admin_base_commit) ||
    !validTerminalBaseline(contextBaseline, project, task, source.base_commit, source.admin_base_commit) ||
    !sameTerminalBaseline(source.code_baseline, contextBaseline)) return false;
  const original = source.original;
  if (Object.hasOwn(source, 'workbuddy_origin')) {
    const origin = source.workbuddy_origin;
    if (!object(origin) || !keys(origin, ['run_id', 'run_version', 'node_id', 'attempt', 'fence', 'assignment_sha256']) ||
      typeof origin.run_id !== 'string' || !/^[A-Za-z0-9_-]{1,100}$/.test(origin.run_id) || origin.run_id === original.run_id || origin.run_id === value.run_id ||
      origin.node_id !== 'workbuddy-cloud' || !positive(origin.run_version) || !positive(origin.attempt) || !positive(origin.fence) || !digest(origin.assignment_sha256)) return false;
  }
  return terminalID(original.run_id) && original.node_id === 'terminal' && original.attempt === 1 && positive(original.fence) && original.worker_id === value.worker_id &&
    (!original.project_id || original.project_id === project) && value.workflow.nodes.every(node => object(node) && typeof node.id === 'string' && typeof node.type === 'string') &&
    value.workflow.nodes.some(node => object(node) && node.id === value.producer_node_id && node.type === 'command' && object(node.config) && same(node.config.terminal_candidate_source, source));
}

export const terminalCandidateMatchesSession = (plan: WorkspaceTerminalCandidatePlan, session: WorkspaceTerminalSession) =>
  plan.session_id === session.id && plan.source.original.run_id === session.run_id && plan.source.snapshot_sha256 === session.source_snapshot_sha256 &&
  plan.source.base_commit === session.base_commit && plan.source.admin_base_commit === session.admin_base_commit && plan.worker_id === session.worker_id &&
  sameTerminalBaseline(plan.source.code_baseline, session.code_baseline);

export function validTerminalCandidatePage(value: unknown, project: string, task: string, session: string, originalRun: string): value is WorkspaceTerminalCandidatePlanPage {
  return object(value) && keys(value, ['items', 'task_version']) && positive(value.task_version) && Array.isArray(value.items) && value.items.length <= 20 &&
    value.items.every(plan => validTerminalCandidatePlan(plan, project, task, session) && plan.source.original.run_id === originalRun) &&
    new Set(value.items.map(plan => plan.id)).size === value.items.length;
}

export function validTerminalCandidateRequest(request: WriteRecoveryRequest, project: string, session: string): boolean {
  const body = request.body;
  if (request.method !== 'POST' || body.project_id !== project || body.idempotency_key !== request.idempotency_key) return false;
  if (request.path === '/workspace/terminal-sessions/' + encodeURIComponent(session) + '/candidate-plans') return keys(body,
    ['project_id', 'expected_session_version', 'expected_task_version', 'tool_profile_id', 'expected_tool_version', 'workflow_id', 'workflow_version', 'producer_node_id', 'acknowledge_stopped_changes', 'idempotency_key']) &&
    positive(body.expected_session_version) && positive(body.expected_task_version) && terminalID(body.tool_profile_id) && positive(body.expected_tool_version) &&
    terminalID(body.workflow_id) && positive(body.workflow_version) && typeof body.producer_node_id === 'string' && !!body.producer_node_id && body.producer_node_id.length <= 150 && body.acknowledge_stopped_changes === true;
  return /^\/workspace\/terminal-candidate-plans\/[A-Za-z0-9_.-]{1,100}\/start$/.test(request.path) &&
    keys(body, ['project_id', 'expected_version', 'idempotency_key']) && positive(body.expected_version);
}

export function terminalCandidatePreviewReceipt(value: unknown, request: WriteRecoveryRequest, project: string, task: string, session: string, originalRun: string): value is WorkspaceTerminalCandidatePlan {
  if (!validTerminalCandidatePlan(value, project, task, session)) return false;
  const body = request.body;
  return value.session_version === body.expected_session_version && value.task_version === body.expected_task_version &&
    value.tool_profile_id === body.tool_profile_id && value.tool_profile_version === body.expected_tool_version && value.source.original.run_id === originalRun &&
    value.workflow_id === body.workflow_id && value.workflow_version === body.workflow_version && value.producer_node_id === body.producer_node_id &&
    value.workflow.nodes.some(node => node.id === body.producer_node_id && node.type === 'command');
}
