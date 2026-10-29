import type {ProjectFocus} from '../../../../packages/contracts/src';

export const focusKeys=['planning','execution','review','attention'] as const;
export type FocusKey=typeof focusKeys[number];
export type FocusFilter=FocusKey|'all';
const reasons:Record<FocusKey,readonly string[]>={
 planning:['task_unstarted'],
 execution:['run_queued','run_active','run_waiting','run_paused'],
 review:['run_succeeded_review_pending'],
 attention:['task_blocked','run_failed','run_reconciling','run_cancelled','run_missing','run_basis_changed','run_basis_unavailable'],
};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown):value is string=>typeof value==='string';
const timestamp=(value:unknown):value is string=>text(value)&&!!value&&Number.isFinite(Date.parse(value));
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;

/** A partial or foreign response is unavailable, never a project with zero work. */
export function validProjectFocus(value:unknown,project:string):value is ProjectFocus{
 if(!object(value)||value.project_id!==project||!timestamp(value.observed_at)||!Array.isArray(value.sections)||value.sections.length!==focusKeys.length)return false;
 const seenKeys=new Set<string>(),seenTasks=new Set<string>();
 for(const section of value.sections){
  if(!object(section)||!text(section.key)||!focusKeys.includes(section.key as FocusKey)||seenKeys.has(section.key)||!count(section.total)||!Array.isArray(section.items)||section.items.length>6||section.items.length>section.total||section.truncated!==(section.total>section.items.length))return false;
  seenKeys.add(section.key);
  for(const item of section.items){
   if(!object(item)||!text(item.task_id)||!item.task_id||seenTasks.has(item.task_id)||!text(item.title)||!item.title||!count(item.task_version)||item.task_version<1||!text(item.task_status)||!item.task_status||['accepted','done','cancelled'].includes(item.task_status)||!text(item.run_id)||!text(item.run_state)||!text(item.priority)||!timestamp(item.updated_at)||!text(item.reason)||!reasons[section.key as FocusKey].includes(item.reason))return false;
   seenTasks.add(item.task_id);
  }
 }
 return true;
}
