import {query,type ApiCaller} from './api';
import type {TaskDispatchPlan} from './tool-model';
import type {ToolPendingRequest} from './tool-write';
import {validCodexDispatchReceipt} from './codex-execution-model';
import {validOpenCodeDispatchReceipt,validOpenCodePlan} from './opencode-execution-model';
import {validModelDispatchReceipt,validModelPlan} from './model-execution-model';
import {validClaudeDispatchReceipt,validClaudePlan} from './claude-cli-model';
import {validClaudeSDKDispatchReceipt,validClaudeSDKPlan} from './claude-sdk-execution-model';
import {validACPDispatchReceipt,validACPPlan} from './acp-execution-model';
import {validExecutionSkillPlan} from './codex-native-skills-model';
import {validTestBindingsPlan,validTestBindingsDispatchReceipt} from './task-dispatch-test-bindings-model';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const hasText=(value:unknown):value is string=>typeof value==='string'&&value.trim().length>0;
const positiveVersion=(value:unknown)=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;

/** Every preview receipt must acknowledge the original selection. Protocol
 * validators add evidence for selected capabilities, which can all be absent. */
function matchesPreviewBasis(value:unknown,body:Record<string,unknown>):boolean {
 return object(value)&&typeof value.id==='string'&&/^[A-Za-z0-9_.-]+$/.test(value.id)&&
  value.project_id===body.project_id&&hasText(body.task_id)&&value.task_id===body.task_id&&
  positiveVersion(body.expected_task_version)&&value.task_version===body.expected_task_version&&
  hasText(body.workflow_id)&&value.workflow_id===body.workflow_id&&
  positiveVersion(body.workflow_version)&&value.workflow_version===body.workflow_version&&
  hasText(body.agent_node_id)&&value.agent_node_id===body.agent_node_id&&
  hasText(body.tool_profile_id)&&value.tool_profile_id===body.tool_profile_id&&
  positiveVersion(body.expected_tool_version)&&value.tool_profile_version===body.expected_tool_version;
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
export async function validateTaskDispatchResponse(value:unknown,request:ToolPendingRequest,api:ApiCaller,project:string):Promise<boolean> {
 if(request.method!=='POST'||!hasText(project)||request.body.project_id!==project)return false;
 if(request.path==='/task-dispatch-plans')return matchesPreviewBasis(value,request.body)&&validTestBindingsDispatchReceipt(value,request.body)&&validACPDispatchReceipt(value,request.body)&&validClaudeDispatchReceipt(value,request.body)&&validClaudeSDKDispatchReceipt(value,request.body)&&validCodexDispatchReceipt(value,request.body)&&validOpenCodeDispatchReceipt(value,request.body)&&validModelDispatchReceipt(value,request.body);
 const route=request.path.match(/^\/task-dispatch-plans\/([A-Za-z0-9_.-]+)\/start$/);
 if(!route||!object(value)||typeof value.id!=='string'||value.project_id!==project||typeof value.workflow_id!=='string'||!Number.isSafeInteger(value.workflow_version))return false;
 const original=await api<TaskDispatchPlan>('/task-dispatch-plans/'+encodeURIComponent(route[1])+query(project));
 return original?.id===route[1]&&validManagedTaskDispatchRun(value,original,project);
}
