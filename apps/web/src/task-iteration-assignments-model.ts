import type {TaskIterationAssignmentResult,TaskIterationAssignmentSelection,TaskWorkspacePlanRef} from '../../../packages/contracts/src';
import {label,type RecordItem} from './api';
import type {Iteration} from './planning-model';
import type {WriteRecoveryRequest} from './write-action';

export const iterationAssignmentPath='/planning/iteration-assignments';
export const maxIterationAssignmentVersion=2147483647;
export type AssignmentRow={
  task:TaskIterationAssignmentSelection;
  title:string;
  sourceID:string;
  source?:Iteration;
};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const hasFields=(value:Record<string,unknown>,fields:string[])=>Object.keys(value).length===fields.length&&fields.every(field=>Object.hasOwn(value,field));
const id=(value:unknown):value is string=>typeof value==='string'&&value!==''&&
  !/[\x00-\x20\x7f\/\\\uD800-\uDFFF]/u.test(value)&&new TextEncoder().encode(value).length<=64;
const version=(value:unknown,min:number):value is number=>typeof value==='number'&&Number.isInteger(value)&&value>=min&&value<=maxIterationAssignmentVersion;

export function assignableIteration(iteration:Iteration){
  return id(iteration.id)&&version(iteration.version,1)&&['planned','active'].includes(iteration.status);
}

export function assignmentRows(project:string,tasks:RecordItem[],plans:TaskWorkspacePlanRef[],iterations:Iteration[]):AssignmentRow[]|undefined{
  if(tasks.length<1||tasks.length>16)return;
  const selected=new Set<string>();
  const byIteration=new Map(iterations.map(iteration=>[iteration.id,iteration]));
  const byTask=new Map(plans.map(plan=>[plan.task_id,plan]));
  const rows:AssignmentRow[]=[];
  for(const task of tasks){
    const plan=byTask.get(task.id);
    if(task.project_id!==project||!id(task.id)||!version(task.version,1)||selected.has(task.id)||!plan||
      !version(plan.plan_version,0)||(plan.iteration_id!==''&&(!id(plan.iteration_id)||plan.plan_version===0)))return;
    rows.push({
      task:{task_id:task.id,expected_task_version:task.version,expected_plan_version:plan.plan_version},
      title:label(task),sourceID:plan.iteration_id,source:byIteration.get(plan.iteration_id),
    });
    selected.add(task.id);
  }
  return rows;
}

export function iterationAssignmentRecovery(request:WriteRecoveryRequest,project:string){
  const {body,idempotency_key:key}=request;
  if(request.path!==iterationAssignmentPath||request.method!=='POST'||body.project_id!==project||!id(project)||
    body.idempotency_key!==key||!hasFields(body,['project_id','iteration_id','expected_iteration_version','tasks','idempotency_key'])||
    (body.iteration_id===''?body.expected_iteration_version!==0:!id(body.iteration_id)||!version(body.expected_iteration_version,1))||
    !Array.isArray(body.tasks)||body.tasks.length<1||body.tasks.length>16)return false;
  const selected=new Set<string>();
  for(const task of body.tasks){
    if(!object(task)||!hasFields(task,['task_id','expected_task_version','expected_plan_version'])||!id(task.task_id)||
      !version(task.expected_task_version,1)||!version(task.expected_plan_version,0)||selected.has(task.task_id))return false;
    selected.add(task.task_id);
  }
  return true;
}

export function iterationAssignmentReceipt(value:unknown,request:WriteRecoveryRequest,project:string):value is TaskIterationAssignmentResult{
  if(!iterationAssignmentRecovery(request,project)||!object(value)||!hasFields(value,['project_id','operation_id','iteration_id','changes'])||
    value.project_id!==project||value.operation_id!==request.idempotency_key||value.iteration_id!==request.body.iteration_id||
    !Array.isArray(value.changes))return false;
  const tasks=request.body.tasks as TaskIterationAssignmentSelection[];
  if(value.changes.length!==tasks.length||!value.changes.some(row=>object(row)&&row.changed===true))return false;
  return value.changes.every((row,index)=>{
    const task=tasks[index];
    return object(row)&&hasFields(row,['task_id','plan_version','changed'])&&row.task_id===task.task_id&&
      typeof row.changed==='boolean'&&version(row.plan_version,0)&&row.plan_version===task.expected_plan_version+Number(row.changed);
  });
}
