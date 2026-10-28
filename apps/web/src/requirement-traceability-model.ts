import type {RequirementTraceability,RequirementTraceabilityCriterion,RequirementTraceabilityTask} from '../../../packages/contracts/src';
import {validTaskDeliveryRun} from './task-delivery-model';
export type {RequirementTraceability,RequirementTraceabilityCriterion,RequirementTraceabilityTask};

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const integer=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const positive=(value:unknown):value is number=>integer(value)&&value>0;
const text=(value:unknown):value is string=>typeof value==='string';
const identifier=(value:unknown):value is string=>text(value)&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const taskKey=(value:unknown):value is string=>text(value)&&/^[A-Za-z0-9_-]{1,64}$/.test(value);
const unique=(values:unknown[])=>new Set(values).size===values.length;
const eligibilities=['none','current','requirement_changed','retired','source_unavailable'];
const bindings=['not_adopted','current','excluded','retired','publication_changed','requirement_changed','task_changed','source_unavailable'];
const bases=['not_dispatched','current','changed','unknown'];

/** Criterion indices belong only to this requirement version. Validate the
 * complete bidirectional mapping before exposing task or execution references. */
export function validRequirementTraceability(value:unknown,project:string,requirement:string):value is RequirementTraceability{
 if(!object(value)||value.project_id!==project||value.requirement_id!==requirement||!positive(value.requirement_version)||!text(value.requirement_status)||!integer(value.design_version)||!integer(value.published_revision)||!text(value.design_sha256)||!eligibilities.includes(String(value.eligibility)))return false;
 if(value.design_sha256!==''&&!/^[a-f0-9]{64}$/.test(value.design_sha256))return false;
 if(!Array.isArray(value.criteria)||value.criteria.length>100||!Array.isArray(value.tasks)||value.tasks.length>100)return false;
 const criteria=value.criteria,tasks=value.tasks;
 if(!criteria.every((criterion,index)=>object(criterion)&&criterion.index===index+1&&text(criterion.text)&&Array.isArray(criterion.task_keys)&&criterion.task_keys.length<=100&&criterion.task_keys.every(taskKey)&&unique(criterion.task_keys)))return false;
 if(value.eligibility!=='current')return tasks.length===0&&criteria.every(criterion=>object(criterion)&&(criterion.task_keys as unknown[]).length===0);
 if(!positive(value.design_version)||!positive(value.published_revision)||!value.design_sha256||!criteria.length||!tasks.length)return false;
 if(!tasks.every(task=>{
  if(!object(task)||!taskKey(task.key)||!text(task.title)||!Array.isArray(task.covers)||task.covers.length>100||!task.covers.every(index=>positive(index)&&index<=criteria.length)||!unique(task.covers)||!text(task.task_id)||!integer(task.task_version)||!text(task.task_status)||!bindings.includes(String(task.binding_state))||!bases.includes(String(task.run_basis_state))||typeof task.formal_acceptance!=='boolean')return false;
  if(task.task_id===''?(task.task_version!==0||task.task_status!==''||task.run!==null):( !identifier(task.task_id)||!positive(task.task_version)))return false;
  if(task.run!==null&&!validTaskDeliveryRun(task.run))return false;
  if(task.run_basis_state==='current'&&task.run===null||task.run_basis_state==='not_dispatched'&&task.run!==null)return false;
  if(task.formal_acceptance&&(task.binding_state!=='current'||task.task_status!=='accepted'||task.run_basis_state!=='current'||!object(task.run)||task.run.state!=='succeeded'))return false;
  return true;
 }))return false;
 if(!unique(tasks.map(task=>task.key))||!unique(tasks.filter(task=>task.task_id!=='').map(task=>task.task_id)))return false;
 return criteria.every(criterion=>{
  const keys=new Set(criterion.task_keys as string[]),mapped=tasks.filter(task=>(task.covers as number[]).includes(criterion.index as number));
  return keys.size>0&&keys.size===mapped.length&&mapped.every(task=>keys.has(task.key as string));
 });
}

/** These are accepted mapped tasks, not test cases or criterion-level review. */
export function traceCriterionProgress(value:RequirementTraceability,criterion:RequirementTraceabilityCriterion){
 const tasks=value.tasks.filter(task=>criterion.task_keys.includes(task.key));
 return {mapped:tasks.length,accepted:value.eligibility==='current'?tasks.filter(task=>task.binding_state==='current'&&task.formal_acceptance).length:0};
}
