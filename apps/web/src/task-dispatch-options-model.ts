import type {TaskDispatchOptions,TaskDispatchOptionsRequest,TaskDispatchToolOption,ToolExecutionProfile} from '../../../packages/contracts/src';
import {taskToolProtocol,type ToolProfile} from './tool-model';

export type DispatchOptionsScope=Pick<TaskDispatchOptionsRequest,'project_id'|'task_id'|'expected_task_version'|'workflow_id'|'workflow_version'|'agent_node_id'>;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown,max=200):value is string=>typeof value==='string'&&value.length>0&&value.length<=max&&!/[\u0000-\u001f\u007f]/.test(value);
const integer=(value:unknown,min=0):value is number=>Number.isSafeInteger(value)&&Number(value)>=min;
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const stamp=(value:unknown)=>typeof value==='string'&&Number.isFinite(Date.parse(value));
const cursor=(value:unknown):value is string=>typeof value==='string'&&new TextEncoder().encode(value).length<=4096&&!/[\u0000-\u001f\u007f]/.test(value);
type OptionReason = 'eligible' | 'tool_unavailable' | 'workflow_incompatible' | 'route_incompatible';
type CheckedOption = TaskDispatchToolOption & { reason_code: OptionReason };
function validReason(value: unknown): value is OptionReason {
 return value === 'eligible' || value === 'tool_unavailable' || value === 'workflow_incompatible' || value === 'route_incompatible';
}
const record=(value:unknown)=>object(value)&&text(value.id,150)&&integer(value.version,1)&&typeof value.title==='string'&&typeof value.description==='string'&&Array.isArray(value.acceptance_criteria)&&value.acceptance_criteria.every(item=>typeof item==='string');
function profile(value:unknown):value is ToolExecutionProfile{
 return object(value)&&text(value.command_id,150)&&text(value.adapter)&&text(value.protocol)&&hash(value.profile_sha256)&&hash(value.executable_sha256)&&text(value.tool_version)&&typeof value.read_only==='boolean';
}
function tool(value:unknown,project:string):boolean{
 if(!object(value)||!text(value.id,100)||value.project_id!==project||!text(value.worker_id,100)||!text(value.name,200)||!integer(value.version,1)||!['enabled','paused','revoked'].includes(String(value.state))||!profile(value.profile)||typeof value.available!=='boolean'||typeof value.reason!=='string'||typeof value.enabled_by!=='string'||!stamp(value.created_at)||!stamp(value.updated_at))return false;
 if(value.observation!=null&&(!object(value.observation)||!profile(value.observation.profile)||typeof value.observation.available!=='boolean'||typeof value.observation.reason!=='string'))return false;
 return value.last_seen_at===undefined||value.last_seen_at===''||stamp(value.last_seen_at);
}
export const dispatchOptionsScopeKey=(scope:DispatchOptionsScope)=>JSON.stringify([scope.project_id,scope.task_id,scope.expected_task_version,scope.workflow_id,scope.workflow_version,scope.agent_node_id]);
function dispatchOptionsParameters(scope: DispatchOptionsScope) {
 return new URLSearchParams({
  project_id: scope.project_id,
  expected_task_version: String(scope.expected_task_version),
  workflow_id: scope.workflow_id,
  workflow_version: String(scope.workflow_version),
  agent_node_id: scope.agent_node_id,
 });
}

function dispatchOptionsURL(task: string, parameters: URLSearchParams) {
 return '/tasks/' + encodeURIComponent(task) + '/dispatch-options?' + parameters;
}

export function dispatchOptionsPath(scope: DispatchOptionsScope, before = '') {
 const parameters = dispatchOptionsParameters(scope);
 parameters.set('limit', '50');
 if (before) parameters.set('before', before);
 return dispatchOptionsURL(scope.task_id, parameters);
}

export function exactDispatchOptionPath(scope: DispatchOptionsScope, toolID: string) {
 const parameters = dispatchOptionsParameters(scope);
 parameters.set('tool_profile_id', toolID);
 return dispatchOptionsURL(scope.task_id, parameters);
}
/** Check the whole response before exposing any candidate. These checks reject
 * cross-organization/project/task/template pages; current authorization remains server-owned. */
export function validDispatchOptions(value:unknown,scope:DispatchOptionsScope,organizationID:string):value is Omit<TaskDispatchOptions, 'items'> & {items:CheckedOption[]}{
 if(!object(value)||value.project_id!==scope.project_id||value.task_id!==scope.task_id||value.task_version!==scope.expected_task_version||value.workflow_id!==scope.workflow_id||value.workflow_version!==scope.workflow_version||value.agent_node_id!==scope.agent_node_id||!stamp(value.observed_at)||!object(value.context)||!record(value.context.task)||!object(value.context.task)||value.context.task.id!==scope.task_id||value.context.task.version!==scope.expected_task_version||value.context.requirement!=null&&!record(value.context.requirement)||!object(value.execution_route)||value.execution_route.project_id!==scope.project_id||!text(organizationID,100)||value.execution_route.tenant_id!==organizationID||!integer(value.execution_route.version)||typeof value.execution_route.pool_id!=='string'||typeof value.execution_route.grant_id!=='string'||!integer(value.execution_route.grant_version)||!Array.isArray(value.items)||value.items.length>50||!integer(value.total)||value.total<value.items.length||!cursor(value.next_cursor)||typeof value.recommended_tool_profile_id!=='string')return false;
 const route=value.execution_route;
 if(route.pool_id?( !text(route.pool_id,100)||!text(route.grant_id,100)||Number(route.grant_version)<1||Number(route.version)<1):(route.grant_id!==''||route.grant_version!==0))return false;
 let previous='',recommended='';
 for(const option of value.items){
  if(!object(option)||!tool(option.tool,scope.project_id)||!object(option.tool)||typeof option.eligible!=='boolean'||!validReason(option.reason_code)||typeof option.reason!=='string'||option.model_state!=='fixed_by_tool'||option.cost_state!=='unknown'||option.eligible_basis!=='current_observation_and_compatibility'||option.eligible!==(option.reason_code==='eligible'))return false;
  const item=option.tool;
  // The page policy is stable ID order, never a fabricated AI score. A positive
  // recommendation must also carry a matching real inventory observation.
  if(String(item.id)<=previous)return false;
  previous=String(item.id);
  if(option.eligible){
   if(item.state!=='enabled'||!item.available||!profile(item.profile)||item.profile.read_only||!taskToolProtocol(item.profile)||!object(item.observation)||!item.observation.available||!profile(item.observation.profile)||item.observation.profile.profile_sha256!==item.profile.profile_sha256)return false;
   recommended||=String(item.id);
  }
 }
 return value.recommended_tool_profile_id===recommended&&(!value.next_cursor||value.items.length>0);
}
/** Adopt the validated contract profile directly, preserving its exact facts. */
export function selectedDispatchOption(page:TaskDispatchOptions):ToolProfile|undefined{
 const option=page.items.find(item=>item.eligible&&item.tool.id===page.recommended_tool_profile_id);
 return option?.tool;
}

/** A handoff expects one current option, never a directory page or a substitute. */
export function readExactDispatchOption(value: unknown, scope: DispatchOptionsScope, organizationID: string, toolID: string) {
 if (!validDispatchOptions(value, scope, organizationID) || value.total !== 1 ||
     value.items.length !== 1 || value.next_cursor || value.items[0].tool.id !== toolID) return undefined;
 return value.items[0];
}
