import type {TaskOwnerChangePreview,TaskOwnerChangePreviewRequest,TaskOwnerChangeResult,TaskOwnerSelection,TaskOwnerTarget,TaskOwnerTargetPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
export const ownerID=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
export const ownerVersion=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=2147483647;
const natural=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const optionalID=(value:unknown)=>value===''||ownerID(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const cursor=(value:unknown):value is string=>typeof value==='string'&&value.length<=4096&&!/[\p{Cc}]/u.test(value);
export const validOwnerQuery=(value:string)=>Array.from(value).length<=200&&!/[\p{Cc}\uD800-\uDFFF]/u.test(value);

export function validOwnerSelection(value:unknown):value is TaskOwnerSelection[]{
 if(!Array.isArray(value)||value.length<1||value.length>16)return false;
 const seen=new Set<string>();
 return value.every(row=>{if(!object(row)||!exact(row,['task_id','expected_version'])||!ownerID(row.task_id)||!ownerVersion(row.expected_version)||seen.has(row.task_id))return false;seen.add(row.task_id);return true;});
}
export function ownerTargetsPath(project:string,q='',next=''){
 const params=new URLSearchParams({project_id:project,limit:'20'});if(q)params.set('q',q);if(next)params.set('cursor',next);return '/task-owner-targets?'+params;
}
function validTarget(value:unknown):value is TaskOwnerTarget{
 return object(value)&&exact(value,['resource_id','resource_version','user_id','name'])&&ownerID(value.resource_id)&&ownerVersion(value.resource_version)&&ownerID(value.user_id)&&typeof value.name==='string';
}
export function validOwnerTargets(value:unknown,project:string,pageSize:1|20=20):value is TaskOwnerTargetPage{
 if(!object(value)||!exact(value,['project_id','items','total','limit','next_cursor'])||value.project_id!==project||value.limit!==pageSize||!Array.isArray(value.items)||value.items.length>pageSize||!natural(value.total)||value.items.length>value.total||!cursor(value.next_cursor)||!!value.next_cursor&&!value.items.length)return false;
 const ids=new Set<string>(),users=new Set<string>();
 return value.items.every(row=>{if(!validTarget(row)||ids.has(row.resource_id)||users.has(row.user_id))return false;ids.add(row.resource_id);users.add(row.user_id);return true;});
}
export function appendOwnerTargets(previous:TaskOwnerTargetPage,next:TaskOwnerTargetPage):TaskOwnerTargetPage|undefined{
 if(previous.project_id!==next.project_id||!previous.next_cursor||next.next_cursor===previous.next_cursor)return;
 const items=[...previous.items,...next.items];
 if(items.length>next.total||new Set(items.map(row=>row.resource_id)).size!==items.length||new Set(items.map(row=>row.user_id)).size!==items.length)return;
 return {...next,items};
}
export function validOwnerPreview(value:unknown,request:TaskOwnerChangePreviewRequest):value is TaskOwnerChangePreview{
 if(!object(value)||!exact(value,['project_id','owner','tasks','changed_count','unchanged_count','preview_sha256','can_apply'])||value.project_id!==request.project_id||!validTarget(value.owner)||value.owner.resource_id!==request.owner_id||value.owner.resource_version!==request.expected_owner_version||!Array.isArray(value.tasks)||value.tasks.length!==request.tasks.length||!digest(value.preview_sha256)||!natural(value.changed_count)||!natural(value.unchanged_count)||typeof value.can_apply!=='boolean')return false;
 let changed=0;
 for(let index=0;index<value.tasks.length;index++){
  const row=value.tasks[index],selected=request.tasks[index];
  if(!object(row)||!exact(row,['task_id','version','title','status','owner_id','owner_name','run_id','will_change'])||row.task_id!==selected.task_id||row.version!==selected.expected_version||typeof row.title!=='string'||typeof row.status!=='string'||!optionalID(row.owner_id)||typeof row.owner_name!=='string'||typeof row.run_id!=='string'||row.run_id!==''&&!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(row.run_id)||row.will_change!==(row.owner_id!==request.owner_id))return false;
  if(row.will_change)changed++;
 }
 return value.changed_count===changed&&value.unchanged_count===value.tasks.length-changed&&value.can_apply===(changed>0);
}
export function validOwnerRecovery(request:WriteRecoveryRequest,project:string){
 if(!object(request)||!exact(request,['path','body','method','idempotency_key'])||!object(request.body))return false;
 const body=request.body,key=request.idempotency_key;
 return request.path==='/task-owner-changes'&&request.method==='POST'&&exact(body,['project_id','owner_id','expected_owner_version','tasks','preview_sha256','acknowledge_invalidation','idempotency_key'])&&body.project_id===project&&ownerID(body.owner_id)&&ownerVersion(body.expected_owner_version)&&validOwnerSelection(body.tasks)&&digest(body.preview_sha256)&&body.acknowledge_invalidation===true&&typeof key==='string'&&key.trim().length>0&&!/[\p{Cc}]/u.test(key)&&new TextEncoder().encode(key).length>=8&&new TextEncoder().encode(key).length<=128&&body.idempotency_key===key;
}
export function validOwnerResult(value:unknown,request:WriteRecoveryRequest,project:string):value is TaskOwnerChangeResult{
 if(!validOwnerRecovery(request,project)||!object(value)||!exact(value,['project_id','operation_id','changes'])||value.project_id!==project||value.operation_id!==request.idempotency_key||!Array.isArray(value.changes))return false;
 const tasks=request.body.tasks as TaskOwnerSelection[];if(value.changes.length!==tasks.length)return false;let changed=false;
 for(let index=0;index<tasks.length;index++){
  const row=value.changes[index],selected=tasks[index];
  if(!object(row)||!exact(row,['task_id','previous_version','version','previous_owner_id','owner_id','status','owner_changed'])||row.task_id!==selected.task_id||row.previous_version!==selected.expected_version||!ownerVersion(row.version)||row.version<selected.expected_version||!optionalID(row.previous_owner_id)||row.owner_id!==request.body.owner_id||typeof row.status!=='string'||row.owner_changed!==(row.previous_owner_id!==row.owner_id)||row.owner_changed&&row.version===row.previous_version)return false;
  if(row.owner_changed)changed=true;
 }
 return changed;
}
