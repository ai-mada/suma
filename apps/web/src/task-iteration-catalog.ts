import {UiError} from './i18n';
import type {Iteration} from './planning-model';

/** Directory labels are current-project facts, independent of task membership.
 * Never turn an invalid catalog into an empty/unassociated iteration choice. */
export function taskIterationCatalog(value:unknown,project:string):Iteration[]{
 const fail=():never=>{throw new UiError('task-collection.iterations_invalid');};
 if(!value||typeof value!=='object'||!Array.isArray((value as {items?:unknown}).items))return fail();
 const ids=new Set<string>(),out:Iteration[]=[];
 for(const item of (value as {items:unknown[]}).items){
  if(!item||typeof item!=='object')return fail();
  const row=item as Record<string,unknown>;
  if(typeof row.id!=='string'||!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(row.id)||ids.has(row.id)||row.project_id!==project||!Number.isSafeInteger(row.version)||(row.version as number)<1||typeof row.status!=='string'||!['planned','active','completed'].includes(row.status))return fail();
  if(['title','description','start_date','end_date','goal'].some(key=>typeof row[key]!=='string'))return fail();
  ids.add(row.id);
  out.push({id:row.id,version:row.version as number,title:row.title as string,description:row.description as string,status:row.status as string,start_date:row.start_date as string,end_date:row.end_date as string,goal:row.goal as string});
 }
 return out;
}
