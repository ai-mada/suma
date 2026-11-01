import type {DevelopmentContinuationBinding,DevelopmentContinuationSource,TaskDevelopmentContinuationOptions} from '../../../packages/contracts/src';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';
import {compositionBinding} from './candidate-composition-model';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const integer=(value:unknown,min=0,max=Number.MAX_SAFE_INTEGER):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min&&value<=max;
const identity=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&wellFormed(value)&&value.trim()===value&&new TextEncoder().encode(value).length<=max&&!/[\p{Cc}/\\]/u.test(value);
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const sourceKeys=['run_id','node_id','attempt','fence','expected_run_version'] as const;
const cursor=(value:unknown):value is string=>typeof value==='string'&&wellFormed(value)&&new TextEncoder().encode(value).length<=4096&&!/[\p{Cc}]/u.test(value);
const invalid=()=>new UiError('development-continuation.invalid_options');

export function validDevelopmentContinuationSource(value:unknown):value is DevelopmentContinuationSource{
 return object(value)&&closed(value,sourceKeys)&&identity(value.run_id,100)&&identity(value.node_id,150)&&integer(value.attempt,1)&&integer(value.fence,1)&&integer(value.expected_run_version,1);
}
/** Reuse the formal sealed candidate guard and keep its complete public wire
 * closed. Scope consistency does not grant a lease or establish acceptance. */
function sealedCandidate(value:unknown,project:string):boolean{
 if(!compositionBinding(value,project)||!object(value)||!closed(value,['schema_version','project_id','transfer_id','state','source','base_commit','head_commit','evidence_sha256','bundle_sha256','size_bytes'],['bundle_base_commit'])||!object(value.source))return false;
 const source:Record<string,unknown>=value.source;
 const textFields=['tenant_id','project_id','pool_id','pool_runner_id','pool_slot_id','pool_grant_id','pool_credential_id','dispatch_id','dispatch_state','lease_until'],numberFields=['pool_grant_version','pool_credential_version'];
 if(!closed(source,['run_id','node_id','attempt','fence','worker_id'],[...textFields,...numberFields])||
  textFields.some(key=>Object.hasOwn(source,key)&&(typeof source[key]!=='string'||!wellFormed(source[key] as string)))||
  numberFields.some(key=>Object.hasOwn(source,key)&&!integer(source[key])))return false;
 if(Object.hasOwn(source,'project_id')&&source.project_id!==project||Object.hasOwn(value,'bundle_base_commit')&&typeof value.bundle_base_commit!=='string')return false;
 const anchor=value.bundle_base_commit??value.base_commit;
 return typeof anchor==='string'&&typeof value.base_commit==='string'&&typeof value.head_commit==='string'&&anchor.length===value.head_commit.length&&value.base_commit.length===value.head_commit.length&&anchor!==value.head_commit;
}
export function validDevelopmentContinuationBinding(value:unknown,project:string,task?:string,taskVersion?:number):value is DevelopmentContinuationBinding{
 return object(value)&&closed(value,['protocol','id','project_id','task_id','task_version','source_task_run_id','source_task_version','source_run_version','source','binding_sha256'])&&
  value.protocol==='suma-task-development-continuation-v1'&&identity(value.id,64)&&identity(project,64)&&value.project_id===project&&identity(value.task_id,64)&&(task===undefined||value.task_id===task)&&integer(value.task_version,1)&&(taskVersion===undefined||value.task_version===taskVersion)&&integer(value.source_task_version,1)&&value.source_task_version<=value.task_version&&integer(value.source_run_version,1)&&sealedCandidate(value.source,project)&&object(value.source)&&object(value.source.source)&&value.source_task_run_id===value.source.source.run_id&&hash(value.binding_sha256);
}
export function sameDevelopmentContinuationSource(left:unknown,right:unknown):boolean{
 return validDevelopmentContinuationSource(left)&&validDevelopmentContinuationSource(right)&&sourceKeys.every(key=>left[key]===right[key]);
}
export function readDevelopmentContinuationOptions(value:unknown,expected:{project:string;task:string;taskVersion?:number;limit:number}):TaskDevelopmentContinuationOptions{
 if(!identity(expected.project,64)||!identity(expected.task,64)||!integer(expected.limit,1,100)||expected.taskVersion!==undefined&&!integer(expected.taskVersion,1)||
  !object(value)||!closed(value,['project_id','task_id','task_version','source_task_run_id','source_run_version','source_run_state','items','total','limit','complete'],['next_cursor','reason'])||
  value.project_id!==expected.project||value.task_id!==expected.task||!integer(value.task_version,1)||expected.taskVersion!==undefined&&value.task_version!==expected.taskVersion||value.limit!==expected.limit||!integer(value.total)||typeof value.complete!=='boolean'||!Array.isArray(value.items)||value.items.length>expected.limit||value.total<value.items.length||
  Object.hasOwn(value,'next_cursor')&&!cursor(value.next_cursor)||Object.hasOwn(value,'reason')&&(typeof value.reason!=='string'||!['task_accepted','no_run','run_not_terminal','no_sealed_candidate'].includes(value.reason)))throw invalid();
 const hasNext=!!value.next_cursor;
 if(value.complete===hasNext||hasNext&&(!value.items.length||value.total<=value.items.length))throw invalid();
 if(value.source_task_run_id===''){
  if(value.source_run_version!==0||value.source_run_state!==''||value.items.length||value.total!==0||!value.complete)throw invalid();
 }else if(!identity(value.source_task_run_id,100)||!integer(value.source_run_version,1)||!identity(value.source_run_state,100))throw invalid();
 const seen=new Set<string>();
 for(const option of value.items){
  if(!['succeeded','failed','skipped','cancelled'].includes(String(value.source_run_state))||!object(option)||!closed(option,['selection','candidate','node_name'])||!validDevelopmentContinuationSource(option.selection)||!sealedCandidate(option.candidate,expected.project)||!object(option.candidate)||!object(option.candidate.source)||typeof option.node_name!=='string'||!wellFormed(option.node_name))throw invalid();
  const selection=option.selection,source=option.candidate.source;
  if(selection.run_id!==value.source_task_run_id||selection.expected_run_version!==value.source_run_version||selection.run_id!==source.run_id||selection.node_id!==source.node_id||selection.attempt!==source.attempt||selection.fence!==source.fence)throw invalid();
  const key=JSON.stringify(sourceKeys.map(field=>selection[field]));if(seen.has(key))throw invalid();seen.add(key);
 }
 return value as unknown as TaskDevelopmentContinuationOptions;
}
export function developmentContinuationOptionsPath(input:{project:string;task:string;limit:number;before?:string}):string{
 if(!identity(input.project,64)||!identity(input.task,64)||!integer(input.limit,1,100)||input.before!==undefined&&!cursor(input.before))throw invalid();
 const query=new URLSearchParams({project_id:input.project,limit:String(input.limit)});if(input.before)query.set('before',input.before);
 return '/tasks/'+encodeURIComponent(input.task)+'/development-continuation-options?'+query;
}
