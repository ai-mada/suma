import {query,type ApiCaller} from './api';
import type {TaskDispatchPlan} from './tool-model';
import type {ToolPendingRequest} from './tool-write';
import {validCodexDispatchReceipt} from './codex-execution-model';
import {validOpenCodeDispatchReceipt,validOpenCodePlan} from './opencode-execution-model';
import {validModelDispatchReceipt,validModelPlan} from './model-execution-model';
import {validClaudeDispatchReceipt,validClaudePlan} from './claude-cli-model';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
/** A Start response has no plan ID of its own. Confirm it against the original
 * plan's reserved run and derived workflow before clearing durable intent.
 * Failure to read that plan preserves the original key; it never starts again. */
export async function validateTaskDispatchResponse(value:unknown,request:ToolPendingRequest,api:ApiCaller,project:string):Promise<boolean> {
 if(request.body.project_id!==project)return false;
 if(request.path==='/task-dispatch-plans')return validClaudeDispatchReceipt(value,request.body)&&validCodexDispatchReceipt(value,request.body)&&validOpenCodeDispatchReceipt(value,request.body)&&validModelDispatchReceipt(value,request.body);
 const route=request.path.match(/^\/task-dispatch-plans\/([A-Za-z0-9_.-]+)\/start$/);
 if(!route||!object(value)||typeof value.id!=='string'||value.project_id!==project||typeof value.workflow_id!=='string'||!Number.isSafeInteger(value.workflow_version))return false;
 const original=await api<TaskDispatchPlan>('/task-dispatch-plans/'+encodeURIComponent(route[1])+query(project));
 return validClaudePlan(original)&&validOpenCodePlan(original)&&validModelPlan(original)&&original?.id===route[1]&&original.project_id===project&&!!original.run_id&&value.id===original.run_id&&value.workflow_id===original.workflow?.id&&value.workflow_version===original.workflow?.version;
}
