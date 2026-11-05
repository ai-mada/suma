import {query,type ApiCaller} from './api';
import type {TaskDispatchPlan} from './tool-model';
import type {ToolPendingRequest} from './tool-write';
import {codexBindingPresent,sameCodexBinding,validCodexDispatchReceipt} from './codex-execution-model';
import {validOpenCodeDispatchReceipt,validOpenCodePlan} from './opencode-execution-model';
import {validModelDispatchReceipt,validModelPlan} from './model-execution-model';
import {validClaudeDispatchReceipt,validClaudePlan} from './claude-cli-model';
import {validClaudeSDKDispatchReceipt,validClaudeSDKPlan} from './claude-sdk-execution-model';
import {validACPDispatchReceipt,validACPPlan} from './acp-execution-model';
import {validExecutionSkillPlan} from './codex-native-skills-model';
import {validTestBindingsPlan,validTestBindingsDispatchReceipt} from './task-dispatch-test-bindings-model';
import {sameKnowledgePins,validKnowledgeContext,validKnowledgePins} from './task-conversation-knowledge-model';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const hasText=(value:unknown):value is string=>typeof value==='string'&&value.trim().length>0;
const positiveVersion=(value:unknown)=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const stamp=(value:unknown)=>typeof value==='string'&&Number.isFinite(Date.parse(value));
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const planIdentifier=(value:unknown):value is string=>typeof value==='string'&&value!=='.'&&value!=='..'&&/^[A-Za-z0-9_.-]+$/.test(value);

type DispatchPlanScope={project:string;organization:string;planID?:string;taskID?:string};

function validPlanContext(plan:Record<string,unknown>):boolean {
 const record=(value:unknown,id:unknown,version:unknown)=>object(value)&&hasText(id)&&positiveVersion(version)&&
  value.id===id&&value.version===version&&typeof value.title==='string'&&typeof value.description==='string'&&
  Array.isArray(value.acceptance_criteria)&&value.acceptance_criteria.every(item=>typeof item==='string');
 const context=plan.context;
 if(!object(context)||!record(context.task,plan.task_id,plan.task_version))return false;
 if(context.requirement==null?(plan.requirement_id!==undefined||plan.requirement_version!==undefined):!record(context.requirement,plan.requirement_id,plan.requirement_version))return false;
 if(context.knowledge!==undefined&&!validKnowledgeContext(context.knowledge))return false;
 const design=context.design;
 return design==null||object(design)&&hasText(design.requirement_id)&&positiveVersion(design.requirement_version)&&positiveVersion(design.revision)&&
  hash(design.sha256)&&hasText(design.adoption_id)&&hasText(design.task_key)&&object(design.content)&&
  typeof design.content.product==='string'&&typeof design.content.experience==='string'&&typeof design.content.technical==='string';
}

function validFrozenGraph(plan:Record<string,unknown>):boolean {
 const workflow=plan.workflow;
 if(!object(workflow)||workflow.id!==plan.workflow_id||workflow.version!==plan.workflow_version||!Array.isArray(workflow.nodes))return false;
 const ids=new Set<string>();
 let agent:Record<string,unknown>|undefined;
 for(const node of workflow.nodes){
  if(!object(node)||!hasText(node.id)||ids.has(node.id)||!hasText(node.type)||typeof node.name!=='string')return false;
  ids.add(node.id);
  if(node.id===plan.agent_node_id)agent=node;
 }
 // A published graph can use depends_on without an explicit edge list. The
 // selected agent still has exactly one frozen command and profile binding.
 return !!agent&&agent.type==='agent'&&Array.isArray(agent.command)&&agent.command.length===1&&hasText(agent.command[0])&&
  object(agent.config)&&agent.config.tool_profile_id===plan.tool_profile_id&&agent.config.tool_profile_sha256===plan.tool_profile_sha256&&
  (plan.codex_execution==null?agent.config.codex_execution==null:
    codexBindingPresent(plan.codex_execution)&&sameCodexBinding(agent.config.codex_execution,plan.codex_execution));
}

/** Validate the frozen preview before presenting it, acknowledging a write,
 * or using it to identify a run. The server still authorizes every Start. */
export function validTaskDispatchPlan(value:unknown,{project,organization,planID,taskID}:DispatchPlanScope):value is TaskDispatchPlan {
 if(!object(value)||!hasText(organization)||!planIdentifier(value.id)||(planID!==undefined&&value.id!==planID)||value.project_id!==project||
  !hasText(value.task_id)||(taskID!==undefined&&value.task_id!==taskID)||!positiveVersion(value.task_version)||
  !hasText(value.workflow_id)||!positiveVersion(value.workflow_version)||!hasText(value.agent_node_id)||
  !hasText(value.tool_profile_id)||!positiveVersion(value.tool_profile_version)||!hash(value.tool_profile_sha256)||!planIdentifier(value.run_id)||
  (value.state==='ready'?value.version!==1:value.state!=='started'||value.version!==2)||
  typeof value.valid!=='boolean'||typeof value.reason!=='string'||!stamp(value.created_at)||!stamp(value.expires_at))return false;
 const route=value.execution_route;
 // A persisted history row may have no public route; it remains display-only.
 if(route!=null&&(!object(route)||route.tenant_id!==organization||route.project_id!==project||
  typeof route.pool_id!=='string'||typeof route.version!=='number'||!Number.isSafeInteger(route.version)||route.version<0||
  (route.pool_id===''?route.grant_id!==''||route.grant_version!==0:!hasText(route.grant_id)||!positiveVersion(route.grant_version)||!positiveVersion(route.version))))return false;
 return validPlanContext(value)&&validFrozenGraph(value)&&validTestBindingsPlan(value)&&validExecutionSkillPlan(value)&&
  validACPPlan(value)&&validClaudePlan(value)&&validClaudeSDKPlan(value)&&validOpenCodePlan(value)&&validModelPlan(value);
}

/** Every preview receipt must acknowledge the original selection. Protocol
 * validators add evidence for selected capabilities, which can all be absent. */
function matchesPreviewBasis(value:TaskDispatchPlan,body:Record<string,unknown>):boolean {
 const requested=body.knowledge===undefined?[]:body.knowledge, fixed=value.context.knowledge??[];
 return value.task_id===body.task_id&&value.task_version===body.expected_task_version&&
  value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&
  value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&
  value.tool_profile_version===body.expected_tool_version&&validKnowledgePins(requested)&&sameKnowledgePins(requested,fixed);
}
/** Exact identity from CreateManagedRunBound; domain authorization stays with
 * each caller's current plan, source and permission checks. */
export function validManagedRunIdentity(value:unknown,project:string,reservedRunID:string):boolean {
 return object(value)&&typeof reservedRunID==='string'&&!!reservedRunID&&value.project_id===project&&value.id===reservedRunID&&value.workflow_id==='managed-'+reservedRunID&&value.workflow_version===1;
}
/** Both direct and discussion-based Start commands use the same managed run.
 * The authorized original plan owns its reserved ID and immutable bindings. */
export function validManagedTaskDispatchRun(value:unknown,original:unknown,project:string):boolean {
 if(!object(value)||!object(original))return false;
 // CreateManagedRunBound keeps the published workflow in the preview, then
 // assigns the private workflow managed-<reserved run> with version 1.
 return validTestBindingsPlan(original)&&validExecutionSkillPlan(original)&&validACPPlan(original)&&validClaudePlan(original)&&validClaudeSDKPlan(original)&&validOpenCodePlan(original)&&validModelPlan(original)&&original.project_id===project&&original.state==='started'&&typeof original.run_id==='string'&&validManagedRunIdentity(value,project,original.run_id);
}
/** A Start response has no plan ID of its own. Confirm it against the original
 * plan's reserved run and derived workflow before clearing durable intent.
 * Failure to read that plan preserves the original key; it never starts again. */
export async function validateTaskDispatchResponse(value:unknown,request:ToolPendingRequest,api:ApiCaller,project:string,organization:string):Promise<boolean> {
 if(request.method!=='POST'||!hasText(project)||request.body.project_id!==project)return false;
 if(request.path==='/task-dispatch-plans')return validTaskDispatchPlan(value,{project,organization})&&matchesPreviewBasis(value,request.body)&&validTestBindingsDispatchReceipt(value,request.body)&&validACPDispatchReceipt(value,request.body)&&validClaudeDispatchReceipt(value,request.body)&&validClaudeSDKDispatchReceipt(value,request.body)&&validCodexDispatchReceipt(value,request.body)&&validOpenCodeDispatchReceipt(value,request.body)&&validModelDispatchReceipt(value,request.body);
 const route=request.path.match(/^\/task-dispatch-plans\/([A-Za-z0-9_.-]+)\/start$/);
 const expectedVersion=request.body.expected_version;
 if(!route||!planIdentifier(route[1])||typeof expectedVersion!=='number'||!positiveVersion(expectedVersion)||
  !object(value)||!planIdentifier(value.id)||value.tenant_id!==organization||value.project_id!==project||value.workflow_id!=='managed-'+value.id||value.workflow_version!==1)return false;
 const original=await api<unknown>('/task-dispatch-plans/'+encodeURIComponent(route[1])+query(project));
 return validTaskDispatchPlan(original,{project,organization,planID:route[1]})&&original.state==='started'&&
  original.version===expectedVersion+1&&validManagedRunIdentity(value,project,original.run_id);
}
