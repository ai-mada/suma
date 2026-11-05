import type {TaskDeliveryRepositoryIntake,TaskDispatchPlan} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {validTestBindingFields} from './task-dispatch-test-bindings-model';

export type RepositoryWorkbenchScope = {
  taskVersion: number;
  runID: string;
  nodeID: string;
  attempt: number;
  targetID: string;
  targetVersion: number;
};
export type RepositoryWorkbenchProfile = {id: string; version: number; name: string; worker_id: string; command_id: string};
export type RepositoryWorkbenchOptions = {
  project_id: string; task_id: string; task_version: number; run_id: string; node_id: string; attempt: number; target_id: string; target_version: number;
  head_commit: string; base_commit: string; baseline_ref: string; live_state: 'ready'|'unchanged'|'unavailable';
  published_case_count: number; active_terminal: boolean; profiles_truncated: boolean;
  retained_intakes: TaskDeliveryRepositoryIntake[]; producer_profiles: RepositoryWorkbenchProfile[]; test_profiles: RepositoryWorkbenchProfile[];
};
export type RepositoryWorkbenchSource = {intake_id: string; plan_id: string; task_id: string; task_version: number; base_commit: string; head_commit: string; bundle_sha256: string; size_bytes: number};
export type RepositoryWorkbenchPlan = TaskDispatchPlan & {test_bindings: [NonNullable<TaskDispatchPlan['test_bindings']>[number]]};

const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const exact = (value: Record<string, unknown>, keys: string[]) => Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key));
const id = (value: unknown): value is string => typeof value === 'string' && /^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const version = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0 && value <= 2147483647;
const gitOID = (value: unknown): value is string => typeof value === 'string' && /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(value);
const hash = (value: unknown): value is string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value);
const size = (value: unknown): value is number => version(value) && value <= 64 * 1024 * 1024;
const timestamp = (value: unknown): value is string => typeof value === 'string' && value.length <= 64 && Number.isFinite(Date.parse(value));

export function workbenchScope(value: unknown): value is RepositoryWorkbenchScope {
  return object(value) && exact(value, ['taskVersion','runID','nodeID','attempt','targetID','targetVersion']) && version(value.taskVersion) &&
    id(value.runID) && id(value.nodeID) && version(value.attempt) && id(value.targetID) && version(value.targetVersion);
}

export function workbenchScopeKey(owner: string, scope: RepositoryWorkbenchScope) {
  return JSON.stringify(['repository-workbench', owner, scope.taskVersion, scope.runID, scope.nodeID, scope.attempt, scope.targetID, scope.targetVersion]);
}

export function repositoryWorkbenchQuery(project: string, scope: RepositoryWorkbenchScope) {
  return '?' + new URLSearchParams({project_id: project, task_version: String(scope.taskVersion), run_id: scope.runID, target_id: scope.targetID, target_version: String(scope.targetVersion)});
}

function matchesScope(value: Record<string, unknown>, project: string, task: string, scope: RepositoryWorkbenchScope) {
  return value.project_id === project && value.task_id === task && value.task_version === scope.taskVersion && value.run_id === scope.runID &&
    value.node_id === scope.nodeID && value.attempt === scope.attempt && value.target_id === scope.targetID && value.target_version === scope.targetVersion;
}

export function validRepositoryIntake(value: unknown, project: string, task: string, scope: RepositoryWorkbenchScope): value is TaskDeliveryRepositoryIntake {
  return object(value) && exact(value, ['id','project_id','task_id','task_version','run_id','node_id','attempt','target_id','target_version','base_commit','head_commit','bundle_sha256','size_bytes','approved_at']) &&
    matchesScope(value, project, task, scope) && id(value.id) && gitOID(value.base_commit) && gitOID(value.head_commit) && value.head_commit.length === value.base_commit.length &&
    value.head_commit !== value.base_commit && hash(value.bundle_sha256) && size(value.size_bytes) && timestamp(value.approved_at);
}

function validProfiles(value: unknown): value is RepositoryWorkbenchProfile[] {
  return Array.isArray(value) && value.length <= 200 && value.every(row => object(row) && exact(row, ['id','version','name','worker_id','command_id']) &&
    id(row.id) && version(row.version) && typeof row.name === 'string' && !!row.name.trim() && row.name.length <= 200 && id(row.worker_id) && id(row.command_id)) &&
    new Set(value.map(row => row.id)).size === value.length;
}

export function validRepositoryOptions(value: unknown, project: string, task: string, scope: RepositoryWorkbenchScope): value is RepositoryWorkbenchOptions {
  if (!object(value) || !matchesScope(value, project, task, scope) || !Array.isArray(value.retained_intakes) || value.retained_intakes.length > 8 ||
    !value.retained_intakes.every(row => validRepositoryIntake(row, project, task, scope)) || new Set(value.retained_intakes.map(row => row.id)).size !== value.retained_intakes.length ||
    !validProfiles(value.producer_profiles) || !validProfiles(value.test_profiles) || typeof value.active_terminal !== 'boolean' ||
    typeof value.profiles_truncated !== 'boolean' || (value.published_case_count !== 0 && !version(value.published_case_count))) return false;
  if (value.live_state === 'unavailable') return value.head_commit === '' && value.base_commit === '' && value.baseline_ref === '';
  return (value.live_state === 'ready' || value.live_state === 'unchanged') && gitOID(value.head_commit) && gitOID(value.base_commit) &&
    value.head_commit.length === value.base_commit.length && (value.head_commit === value.base_commit) === (value.live_state === 'unchanged') &&
    typeof value.baseline_ref === 'string' && value.baseline_ref.length <= 512 && value.baseline_ref.startsWith('refs/heads/');
}

export function repositoryPlanSource(plan: RepositoryWorkbenchPlan): RepositoryWorkbenchSource {
  return plan.workflow.nodes.find(node => node.id === 'repository-source')!.config!.repository_source_intake as RepositoryWorkbenchSource;
}

/** A managed repository plan is a command producer and a fixed test, so the
 * ordinary agent-dispatch plan validator is deliberately not used here. */
export function validRepositoryPlan(value: unknown, project: string, task: string, scope: RepositoryWorkbenchScope): value is RepositoryWorkbenchPlan {
  if (!object(value) || value.project_id !== project || value.task_id !== task || value.task_version !== scope.taskVersion || !id(value.id) || !id(value.run_id) ||
    !(value.state === 'ready' && value.version === 1 || value.state === 'started' && value.version === 2) || typeof value.valid !== 'boolean' || typeof value.reason !== 'string' ||
    value.workflow_id !== 'managed-' + value.run_id || value.workflow_version !== 1 || value.agent_node_id !== 'repository-source' || !id(value.tool_profile_id) ||
    !version(value.tool_profile_version) || !hash(value.tool_profile_sha256) || !timestamp(value.expires_at) || !object(value.workflow) ||
    value.workflow.id !== value.workflow_id || value.workflow.version !== 1 || !Array.isArray(value.workflow.nodes) || value.workflow.nodes.length !== 2 ||
    !Array.isArray(value.workflow.edges) || value.workflow.edges.length !== 1 || !validTestBindingFields(value.test_bindings) || value.test_bindings.length !== 1 ||
    !object(value.context) || !object(value.context.task) || value.context.task.id !== task || value.context.task.version !== scope.taskVersion ||
    !object(value.context.test_plan) || !version(value.context.test_plan.case_count)) return false;
  const producer = value.workflow.nodes.find(node => object(node) && node.id === 'repository-source');
  const test = value.workflow.nodes.find(node => object(node) && node.id === 'repository-test');
  const [edge] = value.workflow.edges, [binding] = value.test_bindings;
  if (!object(producer) || producer.type !== 'command' || !object(producer.config) || producer.config.tool_profile_id !== value.tool_profile_id ||
    producer.config.tool_profile_sha256 !== value.tool_profile_sha256 || !Array.isArray(producer.command) || producer.command.length !== 1 || !id(producer.command[0]) ||
    !object(test) || test.type !== 'test' || !object(test.config) || test.config.candidate_from !== producer.id || binding.node_id !== test.id ||
    test.config.tool_profile_id !== binding.tool_profile_id || test.config.tool_profile_sha256 !== binding.tool_profile_sha256 ||
    !Array.isArray(test.command) || test.command.length !== 1 || test.command[0] !== binding.command_id ||
    !object(edge) || edge.from !== producer.id || edge.to !== test.id) return false;
  const source = producer.config.repository_source_intake;
  return object(source) && exact(source, ['intake_id','plan_id','task_id','task_version','base_commit','head_commit','bundle_sha256','size_bytes']) &&
    id(source.intake_id) && source.plan_id === value.id && source.task_id === task && source.task_version === scope.taskVersion &&
    gitOID(source.base_commit) && gitOID(source.head_commit) && source.base_commit !== source.head_commit && source.base_commit.length === source.head_commit.length &&
    hash(source.bundle_sha256) && size(source.size_bytes);
}

export function validWorkbenchWrite(request: WriteRecoveryRequest, project: string, intakePath: string, scope: RepositoryWorkbenchScope, planID?: string) {
  const {path,body,method} = request;
  if (method !== 'POST' || body.project_id !== project) return false;
  if (path === intakePath) return exact(body, ['project_id','task_version','run_id','target_id','target_version','expected_head','expected_base','idempotency_key']) &&
    body.task_version === scope.taskVersion && body.run_id === scope.runID && body.target_id === scope.targetID && body.target_version === scope.targetVersion &&
    gitOID(body.expected_head) && gitOID(body.expected_base) && body.expected_head.length === body.expected_base.length && body.expected_head !== body.expected_base;
  if (path.startsWith(intakePath + '/') && path.endsWith('/dispatch-plans')) {
    const intake = path.slice(intakePath.length + 1, -'/dispatch-plans'.length);
    return id(intake) && exact(body, ['project_id','expected_task_version','producer_profile_id','expected_producer_profile_version','test_profile_id','expected_test_profile_version','idempotency_key']) &&
      body.expected_task_version === scope.taskVersion && id(body.producer_profile_id) && id(body.test_profile_id) && body.producer_profile_id !== body.test_profile_id &&
      version(body.expected_producer_profile_version) && version(body.expected_test_profile_version);
  }
  return id(planID) && path === '/task-dispatch-plans/' + planID + '/start' && exact(body, ['project_id','expected_version','idempotency_key']) && body.expected_version === 1;
}

export type RepositoryPlanLocator = {id: string; identity: string};

export function repositoryPlanLocator(plan: RepositoryWorkbenchPlan): RepositoryPlanLocator {
  const source=repositoryPlanSource(plan),test=plan.test_bindings[0];
  return {id:plan.id,identity:JSON.stringify([plan.run_id,source.intake_id,source.base_commit,source.head_commit,source.bundle_sha256,source.size_bytes,
    plan.tool_profile_id,plan.tool_profile_version,plan.tool_profile_sha256,test.tool_profile_id,test.tool_profile_version,test.tool_profile_sha256,test.worker_id,test.command_id])};
}

export function savedWorkbenchPlan(value: string | null | undefined): RepositoryPlanLocator | undefined {
  if(!value||value.length>2048)return;
  try{
    const parsed:unknown=JSON.parse(value);
    if(object(parsed)&&exact(parsed,['id','identity'])&&id(parsed.id)&&typeof parsed.identity==='string'&&parsed.identity.length<=1536)return {id:parsed.id,identity:parsed.identity};
  }catch{/* Invalid local state never becomes write authority. */}
}
