import type {
  TaskPlanningStatusChangeResult,TaskPlanningStatusPreview,TaskPlanningStatusPreviewRequest,TaskPlanningStatusSelection,
} from '../../../packages/contracts/src';
import {label,type RecordItem} from './api';
import {ownerID,ownerVersion,validOwnerSelection} from './task-owner-changes-model';
import type {WriteRecoveryRequest} from './write-action';

export const taskPlanningStatusPath='/task-planning-status-changes';
export const taskPlanningStatuses=['draft','todo','in_progress','blocked','cancelled'] as const;
export type TaskPlanningStatus=(typeof taskPlanningStatuses)[number];
export type PlanningStatusRow=TaskPlanningStatusSelection&{title:string};
const eligibility=['editable','workflow_status','active_run','review_required','unverified_run'];
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));

export function isTaskPlanningStatus(value:unknown):value is TaskPlanningStatus{
  return taskPlanningStatuses.some(status=>status===value);
}

export function planningStatusRows(project:string,tasks:RecordItem[]):PlanningStatusRow[]|undefined{
  const selection=tasks.map(task=>({task_id:task.id,expected_version:task.version}));
  if(!ownerID(project)||!validOwnerSelection(selection)||tasks.some(task=>task.project_id!==project))return;
  return selection.map((task,index)=>({...task,title:label(tasks[index])}));
}

export function planningStatusSelections(rows:PlanningStatusRow[]):TaskPlanningStatusSelection[]{
  return rows.map(({task_id,expected_version})=>({task_id,expected_version}));
}

export function validPlanningStatusPreview(value:unknown,request:TaskPlanningStatusPreviewRequest):value is TaskPlanningStatusPreview{
  if(!object(value)||!exact(value,['project_id','items'])||value.project_id!==request.project_id||
    !Array.isArray(value.items)||value.items.length!==request.tasks.length)return false;
  return value.items.every((row,index)=>{
    const selected=request.tasks[index];
    return object(row)&&exact(row,['task_id','version','status','eligibility'])&&row.task_id===selected.task_id&&
      row.version===selected.expected_version&&typeof row.status==='string'&&typeof row.eligibility==='string'&&
      eligibility.includes(row.eligibility)&&(row.eligibility!=='editable'||isTaskPlanningStatus(row.status));
  });
}

export function validPlanningStatusRecovery(request:WriteRecoveryRequest,project:string){
  const {body,idempotency_key}=request;
  return request.path===taskPlanningStatusPath&&request.method==='POST'&&ownerID(project)&&body.project_id===project&&
    body.idempotency_key===idempotency_key&&exact(body,['project_id','status','tasks','idempotency_key'])&&
    isTaskPlanningStatus(body.status)&&validOwnerSelection(body.tasks);
}

export function validPlanningStatusReceipt(value:unknown,request:WriteRecoveryRequest,project:string):value is TaskPlanningStatusChangeResult{
  if(!validPlanningStatusRecovery(request,project)||!object(value)||!exact(value,['project_id','operation_id','changes'])||
    value.project_id!==project||value.operation_id!==request.idempotency_key||!Array.isArray(value.changes))return false;
  const tasks=request.body.tasks;
  if(!validOwnerSelection(tasks)||tasks.length!==value.changes.length)return false;
  return value.changes.every((row,index)=>{
    const task=tasks[index];
    return object(row)&&exact(row,['task_id','previous_version','version','previous_status','status','changed'])&&
      row.task_id===task.task_id&&row.previous_version===task.expected_version&&isTaskPlanningStatus(row.previous_status)&&
      row.status===request.body.status&&row.changed===(row.previous_status!==row.status)&&
      ownerVersion(row.version)&&row.version===task.expected_version+Number(row.changed);
  });
}
