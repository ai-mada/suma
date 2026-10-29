import type {WorkspaceTaskContext,WorkspaceTaskPage} from '../../../packages/contracts/src';
import {validTaskDelivery} from './task-delivery-model';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown):value is string=>typeof value==='string';
const natural=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const positive=(value:unknown):value is number=>natural(value)&&value>0;
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const optionalID=(value:unknown)=>value===''||id(value);
const sha=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const strings=(value:unknown):value is string[]=>Array.isArray(value)&&value.every(text);
const reference=(value:unknown)=>object(value)&&id(value.id)&&positive(value.version)&&text(value.title)&&text(value.description)&&strings(value.acceptance_criteria);
export const workbenchQualityStates=['none','not_run','pending','passed','failed','incomplete','unknown','source_changed','retired','run_changed','stale'] as const;

/** Current display projection only. Neither selecting a task nor a green test
 * count grants file, execution, independent-test or human-review authority. */
export function validWorkbenchTaskContext(value:unknown,project:string,taskID:string):value is WorkspaceTaskContext{
 if(!object(value)||value.project_id!==project||!text(value.observed_at)||!Number.isFinite(Date.parse(value.observed_at))||!object(value.task))return false;
 const task=value.task;
 if(!reference(task)||task.id!==taskID||!text(task.status)||!text(task.priority)||!optionalID(task.owner_id)||!optionalID(task.requirement_id)||!optionalID(task.run_id))return false;
 if(value.requirement!==null&&(!reference(value.requirement)||!object(value.requirement)||value.requirement.id!==task.requirement_id))return false;
 if(value.owner!==null&&(!object(value.owner)||value.owner.id!==task.owner_id||!id(value.owner.id)||!positive(value.owner.version)||!text(value.owner.name)||!text(value.owner.kind)||!text(value.owner.status)))return false;
 const design=value.design;
 if(!object(design)||!['none','current','stale','unknown'].includes(String(design.state)))return false;
 if(design.context!==null){const context=design.context;if(!object(context)||context.requirement_id!==task.requirement_id||!positive(context.requirement_version)||!positive(context.revision)||!sha(context.sha256)||!text(context.task_key)||!id(context.adoption_id)||!object(context.content))return false;const content=context.content;if(!['product','experience','technical'].every(key=>text(content[key])))return false;}
 if(!validTaskDelivery(value.delivery,project,taskID)||value.delivery.task_version!==task.version||value.delivery.task_status!==task.status||value.delivery.run&&value.delivery.run.run_id!==task.run_id)return false;
 const quality=value.quality;
 if(!object(quality)||!optionalID(quality.plan_id)||!natural(quality.revision)||!(quality.plan_sha256===''||sha(quality.plan_sha256))||!optionalID(quality.run_id)||!text(quality.run_state)||!workbenchQualityStates.includes(quality.state as typeof workbenchQualityStates[number])||typeof quality.basis_current!=='boolean'||!['total','passed','failed','errors','skipped','not_run'].every(key=>natural(quality[key])))return false;
 return true;
}

export function validWorkbenchTaskPage(value:unknown,project:string,search:string,limit=20):value is WorkspaceTaskPage{
 return object(value)&&value.project_id===project&&value.search===search&&value.limit===limit&&natural(value.total)&&Array.isArray(value.items)&&value.items.length<=limit&&value.items.length<=value.total&&value.items.every(item=>object(item)&&id(item.id)&&positive(item.version)&&text(item.title)&&text(item.status)&&text(item.priority))&&new Set(value.items.map(item=>item.id)).size===value.items.length&&text(value.next_cursor)&&value.next_cursor.length<=4096&&!/[\x00-\x1f\x7f]/.test(value.next_cursor);
}

/** Links preserve exact source identities but leave navigation approval to the
 * existing workspace guard. A task context never changes the editor's root. */
export function workbenchTaskLinks(value:WorkspaceTaskContext){
 const href=(fields:Record<string,string>)=>'?'+new URLSearchParams({project:value.project_id,...fields});
 return {task:href({view:'development',sub:'tasks',record:value.task.id}),run:value.delivery.run?href({view:'development',sub:'runs',run:value.delivery.run.run_id}):undefined,quality:value.quality.plan_id?href({view:'quality',sub:'tests',record:value.quality.plan_id,...(value.quality.run_id?{run:value.quality.run_id,panel:'test-results'}:{})}):undefined,requirement:value.requirement?href({view:'requirements',record:value.requirement.id}):undefined};
}
