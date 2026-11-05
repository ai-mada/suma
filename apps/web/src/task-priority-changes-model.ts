import type {TaskPriorityChangeResult} from '../../../packages/contracts/src';
import {label,type RecordItem} from './api';
import {ownerID,validOwnerSelection} from './task-owner-changes-model';
import {taskPriorities} from './task-views-model';
import type {WriteRecoveryRequest} from './write-action';

export const taskPriorityChangePath='/task-priority-changes';
export const maxTaskPriorityVersion=2147483647;
export type TaskPriority=(typeof taskPriorities)[number];
export type PriorityReviewRow={task_id:string;expected_version:number;title:string;priority:string};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));

export function isTaskPriority(value:unknown):value is TaskPriority{
  return taskPriorities.some(priority=>priority===value);
}

export function priorityReviewRows(project:string,tasks:RecordItem[]):PriorityReviewRow[]|undefined{
  const selection=tasks.map(task=>({task_id:task.id,expected_version:task.version}));
  if(!ownerID(project)||!validOwnerSelection(selection)||tasks.some(task=>task.project_id!==project||task.status==='archived'||task.status==='deleted'))return;
  return selection.map((task,index)=>({...task,title:label(tasks[index]),priority:tasks[index].priority??''}));
}

export function validPriorityRecovery(request:WriteRecoveryRequest,project:string){
  const {body,idempotency_key}=request;
  return request.path===taskPriorityChangePath&&request.method==='POST'&&ownerID(project)&&body.project_id===project&&
    body.idempotency_key===idempotency_key&&exact(body,['project_id','priority','tasks','idempotency_key'])&&
    isTaskPriority(body.priority)&&validOwnerSelection(body.tasks);
}

export function validPriorityResult(value:unknown,request:WriteRecoveryRequest,project:string):value is TaskPriorityChangeResult{
  if(!validPriorityRecovery(request,project)||!object(value)||!exact(value,['project_id','operation_id','changes'])||
    value.project_id!==project||value.operation_id!==request.idempotency_key||!Array.isArray(value.changes))return false;
  const tasks=request.body.tasks;
  if(!validOwnerSelection(tasks)||tasks.length!==value.changes.length)return false;
  return value.changes.every((row,index)=>{
    const task=tasks[index];
    return object(row)&&exact(row,['task_id','previous_version','version','previous_priority','priority','changed'])&&
      row.task_id===task.task_id&&row.previous_version===task.expected_version&&typeof row.previous_priority==='string'&&
      row.priority===request.body.priority&&row.changed===(row.previous_priority!==row.priority)&&
      row.version===task.expected_version+Number(row.changed)&&row.version<=maxTaskPriorityVersion;
  });
}
