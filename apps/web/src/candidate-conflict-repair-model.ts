import type {CandidateCompositionConflictContext,CandidateCompositionRepairDraft,CandidateCompositionPlan} from '../../../packages/contracts/src';
import type {RecordItem} from './api';
import type {ToolProfile} from './tool-model';
import type {WriteRecoveryRequest} from './write-action';
import {compositionConflictBasis,compositionCursor,compositionID,compositionInteger,compositionText,validCompositionPlan} from './candidate-composition-model';
import {modelBindingMatchesSelection, sameModelBinding, validModelSelection} from './model-execution-input';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export const conflictRepairProtocol='suma.candidate-conflict-repair.v1';
export function validConflictContext(value:unknown,project:string,id:string):value is CandidateCompositionConflictContext{
 return object(value)&&value.project_id===project&&value.composition_id===id&&compositionInteger(value.composition_version)&&value.composition_version>0&&typeof value.can_create==='boolean'&&Array.isArray(value.blockers)&&value.blockers.every(item=>typeof item==='string')&&typeof value.repair_plan_id==='string'&&(!value.repair_plan_id||compositionID(value.repair_plan_id))&&(value.basis==null?!value.can_create:compositionConflictBasis(value.basis,project)&&value.basis.composition_id===id&&value.basis.composition_version===value.composition_version);
}
export function validRepairDraft(value:unknown):value is CandidateCompositionRepairDraft{
 return object(value)&&Object.keys(value).every(key=>['title','description','priority','owner_id','acceptance_criteria'].includes(key))&&compositionText(value.title,200)&&compositionText(value.description,16000)&&['low','medium','high'].includes(String(value.priority))&&(value.owner_id===undefined||value.owner_id===''||compositionID(value.owner_id))&&Array.isArray(value.acceptance_criteria)&&value.acceptance_criteria.length>=1&&value.acceptance_criteria.length<=16&&value.acceptance_criteria.every(item=>compositionText(item,1000));
}
export function repairRecovery(request:WriteRecoveryRequest,project:string,originalID:string){
 const b=request.body;return request.method==='POST'&&request.path==='/candidate-compositions/'+encodeURIComponent(originalID)+'/repairs'&&Object.keys(b).every(key=>['project_id','expected_composition_version','expected_conflict_sha256','repair','workflow_id','workflow_version','producer_node_id','tool_profile_id','expected_tool_version','model_execution_selection','idempotency_key'].includes(key))&&b.project_id===project&&compositionInteger(b.expected_composition_version)&&b.expected_composition_version>0&&typeof b.expected_conflict_sha256==='string'&&/^[a-f0-9]{64}$/.test(b.expected_conflict_sha256)&&validRepairDraft(b.repair)&&compositionID(b.workflow_id)&&compositionInteger(b.workflow_version)&&b.workflow_version>0&&typeof b.producer_node_id==='string'&&b.producer_node_id.length>0&&compositionID(b.tool_profile_id)&&compositionInteger(b.expected_tool_version)&&b.expected_tool_version>0&&(!Object.hasOwn(b,'model_execution_selection')||validModelSelection(b.model_execution_selection));
}
export function repairToolEligible(value:ToolProfile){return value.state==='enabled'&&value.available&&!value.profile.read_only&&value.profile.adapter==='suma'&&value.profile.protocol==='suma-jsonl-v1'&&value.profile.workspace_tools_version==='attempt-workspace-v1'&&value.profile.conflict_repair_protocol===conflictRepairProtocol;}
export function validRepairTools(value:unknown):value is {items:ToolProfile[];total:number;next_cursor?:string}{
 return object(value)&&Array.isArray(value.items)&&value.items.length<=100&&compositionInteger(value.total)&&value.total>=value.items.length&&compositionCursor(value.next_cursor)&&value.items.every(item=>object(item)&&compositionID(item.id)&&typeof item.name==='string'&&typeof item.worker_id==='string'&&compositionInteger(item.version)&&item.version>0&&typeof item.available==='boolean'&&['enabled','paused','revoked'].includes(String(item.state))&&object(item.profile)&&typeof item.profile.read_only==='boolean');
}
export function validRepairMembers(value:unknown):value is {items:RecordItem[];total:number;next_cursor?:string}{
 return object(value)&&Array.isArray(value.items)&&value.items.length<=100&&compositionInteger(value.total)&&value.total>=value.items.length&&compositionCursor(value.next_cursor)&&value.items.every(item=>object(item)&&compositionID(item.id)&&compositionInteger(item.version)&&item.version>0&&(typeof item.name==='string'||typeof item.title==='string'));
}
export function validRepairPlan(value:unknown,project:string,original:string):value is CandidateCompositionPlan{
 return object(value)&&typeof value.task_id==='string'&&validCompositionPlan(value,project,value.task_id)&&!!value.conflict_basis&&value.conflict_basis.composition_id===original&&value.id!==original;
}
export function repairModelReceipt(value:CandidateCompositionPlan,selection:unknown):boolean{
 if (!modelBindingMatchesSelection(value.model_execution,selection)) return false;
 const node=value.workflow.nodes.find(node=>node.id===value.producer_node_id);
 if ((value as unknown as Record<string,unknown>).codex_execution || node?.config?.codex_execution) return false;
 return value.model_execution ? !!node?.config&&sameModelBinding(node.config.model_execution,value.model_execution) : !node?.config?.model_execution;
}
export function validRepairCreateResponse(value:unknown,project:string,original:string,request:WriteRecoveryRequest):value is CandidateCompositionPlan{
 if (!validRepairPlan(value,project,original)) return false;
 const b=request.body;
 return value.conflict_basis?.composition_version===b.expected_composition_version&&value.conflict_basis?.sha256===b.expected_conflict_sha256&&value.workflow_id===b.workflow_id&&value.workflow_version===b.workflow_version&&value.producer_node_id===b.producer_node_id&&value.tool_profile_id===b.tool_profile_id&&value.tool_profile_version===b.expected_tool_version&&repairModelReceipt(value,b.model_execution_selection);
}
export const repairTaskURL=(project:string,task:string,plan:string)=>'?'+new URLSearchParams({project,view:'development',sub:'tasks',record:task,panel:'composition:'+plan});
