import {query,type ApiCaller} from './api';
import type {TaskDispatchPlan} from './tool-model';
import type {ToolPendingRequest} from './tool-write';
import {validCodexDispatchReceipt} from './codex-execution-model';
import {validOpenCodeDispatchReceipt,validOpenCodePlan} from './opencode-execution-model';
import {validModelDispatchReceipt,validModelPlan} from './model-execution-model';
import {validClaudeDispatchReceipt,validClaudePlan} from './claude-cli-model';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
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
 return validClaudePlan(original)&&validOpenCodePlan(original)&&validModelPlan(original)&&original.project_id===project&&original.state==='started'&&typeof original.run_id==='string'&&validManagedRunIdentity(value,project,original.run_id);
}
/** A Start response has no plan ID of its own. Confirm it against the original
 * plan's reserved run and derived workflow before clearing durable intent.
 * Failure to read that plan preserves the original key; it never starts again. */
export async function validateTaskDispatchResponse(value:unknown,request:ToolPendingRequest,api:ApiCaller,project:string):Promise<boolean> {
 if(request.body.project_id!==project)return false;
 if(request.path==='/task-dispatch-plans')return validClaudeDispatchReceipt(value,request.body)&&validCodexDispatchReceipt(value,request.body)&&validOpenCodeDispatchReceipt(value,request.body)&&validModelDispatchReceipt(value,request.body);
 const route=request.path.match(/^\/task-dispatch-plans\/([A-Za-z0-9_.-]+)\/start$/);
 if(!route||!object(value)||typeof value.id!=='string'||value.project_id!==project||typeof value.workflow_id!=='string'||!Number.isSafeInteger(value.workflow_version))return false;
 const original=await api<TaskDispatchPlan>('/task-dispatch-plans/'+encodeURIComponent(route[1])+query(project));
 return original?.id===route[1]&&validManagedTaskDispatchRun(value,original,project);
}
