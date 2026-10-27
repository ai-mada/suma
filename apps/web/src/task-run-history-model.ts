import type {TaskRunProvenance,TaskRunProvenancePage} from '../../../packages/contracts/src';
import {compositionCursor,compositionID,compositionInteger} from './candidate-composition-model';
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const stamp=(value:unknown)=>typeof value==='string'&&Number.isFinite(Date.parse(value));
const runID=(value:unknown)=>typeof value==='string'&&/^[A-Za-z0-9_.:-]{1,150}$/.test(value);
export function validTaskRunHistory(value:unknown,task:string):value is TaskRunProvenancePage{
 if(!object(value)||!Array.isArray(value.items)||value.items.length>20||!compositionInteger(value.total)||value.total<value.items.length||typeof value.next_cursor!=='string'||!compositionCursor(value.next_cursor))return false;
 const seen=new Set<string>();let current=0;return value.items.every(item=>{if(!object(item)||Object.keys(item).sort().join(',')!=='binding_kind,bound_at,current,observed_at,plan_id,run_id,task_id,task_version'||item.task_id!==task||!runID(item.run_id)||typeof item.current!=='boolean'||!stamp(item.observed_at)||typeof item.plan_id!=='string'||item.plan_id!==''&&!compositionID(item.plan_id)||!['bound','observed'].includes(String(item.binding_kind))||!(item.task_version===null||compositionInteger(item.task_version)&&item.task_version>0)||!(item.bound_at===null||stamp(item.bound_at))||item.binding_kind==='bound'&&(item.task_version===null||item.bound_at===null)||seen.has(String(item.run_id)))return false;seen.add(String(item.run_id));if(item.current)current++;return current<=1;});
}
export const taskRunHistoryKey=(item:TaskRunProvenance)=>item.run_id;
