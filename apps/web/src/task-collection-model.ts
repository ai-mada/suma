import type {TaskWorkspacePage as TaskWorkspaceWirePage,TaskCollectionSummary} from '../../../packages/contracts/src';
import type {RecordItem} from './api';

export type TaskCollectionFilters={q:string;status:string;layout:string;task_invalid?:string;task_status?:string;task_owner?:string;task_priority?:string;task_requirement?:string;task_unassigned?:boolean;task_priority_unset?:boolean;task_status_unset?:boolean;task_sort?:string;task_group?:string};
export type TaskCollectionScope=TaskCollectionFilters&{project:string;userID:string;organizationID:string};
/** A route may carry record/panel/run fields. Filter forms own only this
 * explicit projection and can never write an old detail locator back. */
export function projectTaskCollectionFilters(value:TaskCollectionFilters):TaskCollectionFilters{
 return {q:value.q,status:value.status,layout:value.layout,task_invalid:value.task_invalid,task_status:value.task_status,task_owner:value.task_owner,task_priority:value.task_priority,task_requirement:value.task_requirement,task_unassigned:value.task_unassigned,task_priority_unset:value.task_priority_unset,task_status_unset:value.task_status_unset,task_sort:value.task_sort,task_group:value.task_group};
}
// api() applies the existing business-record normalizer before these rows reach
// the forms/details that already consume RecordItem (including criteria text).
export type TaskCollectionPageView=Omit<TaskWorkspaceWirePage,'items'>&{items:RecordItem[]};
export const taskCollectionLimit=20;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const text=(value:unknown):value is string=>typeof value==='string';
const boundedText=(value:unknown):value is string=>text(value)&&!!value.trim()&&Array.from(value).length<=200&&!/[\p{Cc}]/u.test(value);
export const validTaskState=(value:unknown):value is string=>boundedText(value);
export function validTaskCollectionFilter(filters:TaskCollectionFilters){
 return filters.task_invalid!=='1'&&Array.from(filters.q).length<=200&&!/[\p{Cc}]/u.test(filters.q)&&(!filters.task_owner||!filters.task_unassigned)&&(!filters.task_priority||!filters.task_priority_unset)&&(!filters.task_priority||boundedText(filters.task_priority))&&(!filters.task_status||!filters.task_status_unset)&&(!filters.task_status||validTaskState(filters.task_status))&&(!filters.task_sort||['updated_desc','created_desc','title_asc','priority_desc'].includes(filters.task_sort))&&(!filters.task_group||['status','owner','priority'].includes(filters.task_group));
}
export function taskCollectionScopeKey(scope:TaskCollectionScope){return JSON.stringify([scope.organizationID,scope.userID,scope.project,scope.q,scope.task_status||'',scope.layout==='board'?'board':'list',scope.task_owner||'',scope.task_priority||'',scope.task_requirement||'',!!scope.task_unassigned,!!scope.task_priority_unset,scope.task_sort||'updated_desc',taskCollectionGroupBy(scope),scope.task_invalid||'',!!scope.task_status_unset]);}
export function taskCollectionPath(scope:TaskCollectionScope,kind:'summary'|'items',status='',cursor='',groupCursor=''){
 const params=new URLSearchParams({project_id:scope.project});
 if(scope.q)params.set('q',scope.q);
 if(scope.task_owner)params.set('owner_id',scope.task_owner);
 if(scope.task_unassigned)params.set('unassigned','true');
 if(scope.task_priority)params.set('priority',scope.task_priority);
 if(scope.task_requirement)params.set('requirement_id',scope.task_requirement);
 if(scope.task_priority_unset)params.set('priority_unset','true');
 if(scope.task_status)params.set('status',scope.task_status);
 if(scope.task_status_unset)params.set('status_unset','true');
 if(kind==='items'){
  if(status)params.set('status',status);
  params.set('sort',scope.task_sort||'updated_desc');
  params.set('limit',String(taskCollectionLimit));
  if(cursor)params.set('cursor',cursor);
 }
 if(kind==='summary'){params.set('group_by',taskCollectionGroupBy(scope));params.set('group_limit',String(taskCollectionLimit));if(groupCursor)params.set('group_cursor',groupCursor);}
 return (kind==='summary'?'/task-collection/summary':'/task-workspace')+'?'+params.toString();
}
export function validTaskCollectionSummary(value:unknown,project:string,groupBy='status'):value is TaskCollectionSummary{
 if(!object(value)||value.project_id!==project||value.group_by!==groupBy||!text(value.as_of)||!Number.isFinite(Date.parse(value.as_of))||!count(value.total)||!Array.isArray(value.groups)||!count(value.groups_total)||value.groups_total<value.groups.length||value.groups_limit!==taskCollectionLimit||value.groups.length>taskCollectionLimit||!text(value.next_group_cursor)||value.next_group_cursor.length>4096)return false;
 let total=0;const states=new Set<string>();
 for(const group of value.groups){if(!object(group)||!text(group.key)||!boundedText(group.key)&&group.key!==''||!count(group.total)||states.has(group.key))return false;states.add(group.key);total+=group.total;if(!Number.isSafeInteger(total))return false;}
 return total<=value.total&&(value.groups_total!==value.groups.length||total===value.total);
}
export function validTaskCollectionPage(value:unknown,scope:TaskCollectionScope,status:string):value is TaskCollectionPageView{
 if(!object(value)||value.project_id!==scope.project||!text(value.as_of)||!Number.isFinite(Date.parse(value.as_of))||!count(value.total)||value.limit!==taskCollectionLimit||!text(value.next_cursor)||value.next_cursor.length>4096||!Array.isArray(value.items)||value.items.length>taskCollectionLimit||value.total<value.items.length)return false;
 const ids=new Set<string>();
 for(const item of value.items){
  if(!object(item)||!text(item.id)||!item.id||ids.has(item.id)||item.project_id!==scope.project||!count(item.version)||item.version<1||!text(item.title)||!text(item.status)||!boundedText(item.status)&&item.status!==''||status&&item.status!==status||!text(item.owner_id)||scope.task_owner&&item.owner_id!==scope.task_owner||scope.task_unassigned&&!!item.owner_id||scope.task_priority&&item.priority!==scope.task_priority||scope.task_requirement&&item.requirement_id!==scope.task_requirement||scope.task_priority_unset&&!!item.priority||scope.task_status_unset&&!!item.status||!text(item.updated_at)||!Number.isFinite(Date.parse(item.updated_at)))return false;
  for(const field of ['name','description','priority','requirement_id'])if(item[field]!==undefined&&!text(item[field]))return false;
  ids.add(item.id);
 }
 return true;
}
export function appendTaskCollectionPage(previous:TaskCollectionPageView,next:TaskCollectionPageView):TaskCollectionPageView|undefined{
 if(previous.project_id!==next.project_id)return;
 const merged=new Map(previous.items.map(row=>[row.id,row]));
 for(const row of next.items)if(!merged.has(row.id)||merged.get(row.id)!.version<=row.version)merged.set(row.id,row);
 const items=[...merged.values()];
 return items.length<=next.total?{...next,items}:undefined;
}

/** Independent live columns can observe a task before and after a move. The
 * highest record version appears once. Conflicting states at the same version
 * have no chosen winner and require a fresh read rather than invented order. */
export type TaskCollectionGroupBy='status'|'owner'|'priority';
export function taskCollectionGroupBy(scope:Pick<TaskCollectionFilters,'task_group'>):TaskCollectionGroupBy{return scope.task_group==='owner'||scope.task_group==='priority'?scope.task_group:'status';}
export function taskCollectionGroupKey(row:RecordItem,groupBy:TaskCollectionGroupBy){return groupBy==='owner'?row.owner_id||'':groupBy==='priority'?row.priority||'':row.status||'';}
export function taskCollectionColumnScope(scope:TaskCollectionScope,key:string):TaskCollectionScope{
 const groupBy=taskCollectionGroupBy(scope);
 return groupBy==='owner'?{...scope,task_owner:key,task_unassigned:!key}:groupBy==='priority'?{...scope,task_priority:key,task_priority_unset:!key}:{...scope,task_status:key,task_status_unset:!key};
}
export function reconcileTaskCollectionPages(groups:{key:string}[],pages:Record<string,{page?:{items:RecordItem[]}}>){
 const winners=new Map<string,{row:RecordItem;key:string;conflict:boolean}>(),changed=new Set<string>();
 for(const {key} of groups)for(const row of pages[key]?.page?.items??[]){
  const before=winners.get(row.id);
  if(!before){winners.set(row.id,{row,key,conflict:false});continue;}
  if(before.key!==key){changed.add(before.key);changed.add(key);}
  if(row.version>before.row.version)winners.set(row.id,{row,key,conflict:false});
  else if(row.version===before.row.version&&before.key!==key)winners.set(row.id,{...before,conflict:true});
 }
 const rowsByGroup=new Map(groups.map(({key})=>[key,(pages[key]?.page?.items??[]).filter(row=>{const winner=winners.get(row.id);return winner?.key===key&&winner.row.version===row.version&&!winner.conflict;})]));
 return {rowsByGroup,changed,rows:[...rowsByGroup.values()].flat()};
}

/** One queue bounds automatic board reads and explicit retries together. An
 * aborted queued job never reaches the transport or consumes a request slot. */
export class TaskCollectionReadQueue{
 private active=0;
 private pending:(()=>void)[]=[];
 constructor(private readonly concurrency=3){}
 run<T>(read:()=>Promise<T>,signal:AbortSignal):Promise<T>{
  return new Promise((resolve,reject)=>{
   this.pending.push(()=>{
    if(signal.aborted){reject(new DOMException('Aborted','AbortError'));return;}
    this.active++;
    Promise.resolve().then(read).then(resolve,reject).finally(()=>{this.active--;this.pump();});
   });
   this.pump();
  });
 }
 private pump(){while(this.active<this.concurrency&&this.pending.length)this.pending.shift()!();}
}
