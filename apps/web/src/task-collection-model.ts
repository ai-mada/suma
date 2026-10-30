import type {TaskWorkspacePage as TaskWorkspaceWirePage,TaskCollectionSummary} from '../../../packages/contracts/src';
import type {RecordItem} from './api';

export type TaskCollectionFilters={q:string;status:string;layout:string;task_invalid?:string;task_status?:string;task_owner?:string;task_priority?:string;task_requirement?:string;task_iteration?:string;task_iteration_unset?:boolean;task_unassigned?:boolean;task_priority_unset?:boolean;task_status_unset?:boolean;task_sort?:string;task_group?:string};
export type TaskCollectionScope=TaskCollectionFilters&{project:string;userID:string;organizationID:string};
/** A route may carry record/panel/run fields. Filter forms own only this
 * explicit projection and can never write an old detail locator back. */
export function projectTaskCollectionFilters(value:TaskCollectionFilters):TaskCollectionFilters{
 return {q:value.q,status:value.status,layout:value.layout,task_invalid:value.task_invalid,task_status:value.task_status,task_owner:value.task_owner,task_priority:value.task_priority,task_requirement:value.task_requirement,task_iteration:value.task_iteration,task_iteration_unset:value.task_iteration_unset,task_unassigned:value.task_unassigned,task_priority_unset:value.task_priority_unset,task_status_unset:value.task_status_unset,task_sort:value.task_sort,task_group:value.task_group};
}
// api() applies the existing business-record normalizer before these rows reach
// the forms/details that already consume RecordItem (including criteria text).
export type TaskCollectionPageView=Omit<TaskWorkspaceWirePage,'items'>&{items:RecordItem[]};
export type TaskPlanProjection=TaskWorkspaceWirePage['task_plans'][number];
export const taskCollectionLimit=20;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const text=(value:unknown):value is string=>typeof value==='string';
const boundedText=(value:unknown):value is string=>text(value)&&!!value.trim()&&Array.from(value).length<=200&&!/[\p{Cc}]/u.test(value);
const resourceID=(value:unknown):value is string=>text(value)&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
export const validTaskState=(value:unknown):value is string=>boundedText(value);
export function validTaskCollectionFilter(filters:TaskCollectionFilters){
 return filters.task_invalid!=='1'&&Array.from(filters.q).length<=200&&!/[\p{Cc}]/u.test(filters.q)&&(!filters.task_owner||!filters.task_unassigned)&&(!filters.task_priority||!filters.task_priority_unset)&&(!filters.task_priority||boundedText(filters.task_priority))&&(!filters.task_status||!filters.task_status_unset)&&(!filters.task_status||validTaskState(filters.task_status))&&(!filters.task_iteration||resourceID(filters.task_iteration))&&(filters.task_iteration_unset===undefined||typeof filters.task_iteration_unset==='boolean')&&(!filters.task_iteration||!filters.task_iteration_unset)&&(!filters.task_sort||['updated_desc','created_desc','title_asc','priority_desc'].includes(filters.task_sort))&&(!filters.task_group||['status','owner','priority','iteration'].includes(filters.task_group));
}
export function taskCollectionScopeKey(scope:TaskCollectionScope){return JSON.stringify([scope.organizationID,scope.userID,scope.project,scope.q,scope.task_status||'',scope.layout==='board'?'board':'list',scope.task_owner||'',scope.task_priority||'',scope.task_requirement||'',!!scope.task_unassigned,!!scope.task_priority_unset,scope.task_sort||'updated_desc',taskCollectionGroupBy(scope),scope.task_invalid||'',!!scope.task_status_unset,scope.task_iteration||'',!!scope.task_iteration_unset]);}
export function taskCollectionPath(scope:TaskCollectionScope,kind:'summary'|'items',status='',cursor='',groupCursor=''){
 const params=new URLSearchParams({project_id:scope.project});
 if(scope.q)params.set('q',scope.q);
 if(scope.task_owner)params.set('owner_id',scope.task_owner);
 if(scope.task_unassigned)params.set('unassigned','true');
 if(scope.task_priority)params.set('priority',scope.task_priority);
 if(scope.task_requirement)params.set('requirement_id',scope.task_requirement);
 if(scope.task_iteration)params.set('iteration_id',scope.task_iteration);
 if(scope.task_iteration_unset)params.set('iteration_unset','true');
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
 for(const group of value.groups){if(!object(group)||!text(group.key)||!boundedText(group.key)&&group.key!==''||groupBy==='iteration'&&group.key!==''&&!resourceID(group.key)||!count(group.total)||states.has(group.key))return false;states.add(group.key);total+=group.total;if(!Number.isSafeInteger(total))return false;}
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
 return validTaskPlanProjection(value.task_plans,[...ids],scope);
}

/** Planning owns this relation. An absent projection cannot prove that a task
 * has no iteration; every returned row needs its own exact planning version. */
export function validTaskPlanProjection(value:unknown,taskIDs:readonly string[],scope:Pick<TaskCollectionFilters,'task_iteration'|'task_iteration_unset'>={}):value is TaskPlanProjection[]{
 if(!Array.isArray(value)||value.length!==taskIDs.length)return false;
 const expected=new Set(taskIDs),seen=new Set<string>();
 for(const plan of value){
  if(!object(plan)||Object.keys(plan).length!==3||!['task_id','iteration_id','plan_version'].every(key=>Object.hasOwn(plan,key))||!text(plan.task_id)||!expected.has(plan.task_id)||seen.has(plan.task_id)||!text(plan.iteration_id)||plan.iteration_id!==''&&!resourceID(plan.iteration_id)||!count(plan.plan_version)||plan.plan_version===0&&plan.iteration_id!==''||scope.task_iteration&&plan.iteration_id!==scope.task_iteration||scope.task_iteration_unset&&plan.iteration_id!=='')return false;
  seen.add(plan.task_id);
 }
 return true;
}

type TaskWindow={items:RecordItem[];task_plans:TaskPlanProjection[];total:number};
/** A continuation must advance both independent authorities monotonically.
 * Crossed or decreasing versions require a new read, never a guessed merge. */
export function mergeTaskCollectionWindow(previous:TaskWindow,next:TaskWindow):Pick<TaskWindow,'items'|'task_plans'>|undefined{
 if(!validTaskPlanProjection(previous.task_plans,previous.items.map(row=>row.id))||!validTaskPlanProjection(next.task_plans,next.items.map(row=>row.id)))return;
 const plans=new Map(previous.task_plans.map(plan=>[plan.task_id,plan])),incoming=new Map(next.task_plans.map(plan=>[plan.task_id,plan]));
 const merged=new Map(previous.items.map(row=>[row.id,row]));
 for(const row of next.items){
  const before=merged.get(row.id),plan=incoming.get(row.id)!;
  if(before){const beforePlan=plans.get(row.id)!;
   if(row.version<before.version||plan.plan_version<beforePlan.plan_version||plan.plan_version===beforePlan.plan_version&&plan.iteration_id!==beforePlan.iteration_id||row.version===before.version&&taskRecordGroupIdentity(row)!==taskRecordGroupIdentity(before))return;
  }
  merged.set(row.id,row);plans.set(row.id,plan);
 }
 const items=[...merged.values()];
 return items.length<=next.total?{items,task_plans:items.map(row=>plans.get(row.id)!)}:undefined;
}
export function appendTaskCollectionPage(previous:TaskCollectionPageView,next:TaskCollectionPageView):TaskCollectionPageView|undefined{
 if(previous.project_id!==next.project_id)return;
 const merged=mergeTaskCollectionWindow(previous,next);
 return merged?{...next,...merged}:undefined;
}

/** Record and plan versions are separate facts, not one combined revision. */
export type TaskCollectionGroupBy='status'|'owner'|'priority'|'iteration';
export function taskCollectionGroupBy(scope:Pick<TaskCollectionFilters,'task_group'>):TaskCollectionGroupBy{return scope.task_group==='owner'||scope.task_group==='priority'||scope.task_group==='iteration'?scope.task_group:'status';}
export function taskCollectionGroupKey(row:RecordItem,groupBy:TaskCollectionGroupBy,plan?:TaskPlanProjection){return groupBy==='iteration'?plan?.task_id===row.id?plan.iteration_id:undefined:groupBy==='owner'?row.owner_id||'':groupBy==='priority'?row.priority||'':row.status||'';}
function taskRecordGroupIdentity(row:RecordItem){return JSON.stringify([row.status||'',row.owner_id||'',row.priority||'',row.requirement_id||'']);}
export function taskCollectionColumnScope(scope:TaskCollectionScope,key:string):TaskCollectionScope{
 const groupBy=taskCollectionGroupBy(scope);
 return groupBy==='iteration'?{...scope,task_iteration:key,task_iteration_unset:!key}:groupBy==='owner'?{...scope,task_owner:key,task_unassigned:!key}:groupBy==='priority'?{...scope,task_priority:key,task_priority_unset:!key}:{...scope,task_status:key,task_status_unset:!key};
}
export function reconcileTaskCollectionPages(groups:{key:string}[],pages:Record<string,{page?:Pick<TaskCollectionPageView,'items'|'task_plans'>}>,groupBy:TaskCollectionGroupBy='status'){
 const winners=new Map<string,{row:RecordItem;plan:TaskPlanProjection;key:string;conflict:boolean}>(),changed=new Set<string>();
 for(const {key} of groups){
  const page=pages[key]?.page;if(!page)continue;
  if(!validTaskPlanProjection(page.task_plans,page.items.map(row=>row.id))){changed.add(key);continue;}
  const plans=new Map(page.task_plans.map(plan=>[plan.task_id,plan]));
  for(const row of page.items){
   const plan=plans.get(row.id)!,before=winners.get(row.id),belongs=taskCollectionGroupKey(row,groupBy,plan)===key;
   if(!before){winners.set(row.id,{row,plan,key,conflict:!belongs});if(!belongs)changed.add(key);continue;}
   changed.add(before.key);changed.add(key);
   const sameRecord=row.version===before.row.version,samePlan=plan.plan_version===before.plan.plan_version;
   const incomingDominates=row.version>=before.row.version&&plan.plan_version>=before.plan.plan_version;
   const previousDominates=row.version<=before.row.version&&plan.plan_version<=before.plan.plan_version;
   // Version evidence, rather than request completion order, decides dominance.
   // Equal planning versions may never name different relations.
   const conflict=before.conflict||!belongs||samePlan&&plan.iteration_id!==before.plan.iteration_id||sameRecord&&taskRecordGroupIdentity(row)!==taskRecordGroupIdentity(before.row)||sameRecord&&samePlan&&before.key!==key||!incomingDominates&&!previousDominates;
   if(conflict)winners.set(row.id,{...before,conflict:true});
   else if(incomingDominates)winners.set(row.id,{row,plan,key,conflict:false});
  }
 }
 const rowsByGroup=new Map(groups.map(({key})=>[key,(pages[key]?.page?.items??[]).filter(row=>{const winner=winners.get(row.id);return winner?.key===key&&winner.row.version===row.version&&!winner.conflict;})]));
 const rows=[...rowsByGroup.values()].flat();
 return {rowsByGroup,changed,rows,task_plans:rows.map(row=>winners.get(row.id)!.plan)};
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
