import type {MyWorkPage} from '../../../packages/contracts/src';

export const myWorkStatuses=['draft','todo','in_progress','in_review','accepted','blocked','cancelled'] as const;
export type MyWorkStatus=''|typeof myWorkStatuses[number];
export type MyWorkFilter={q:string;status:MyWorkStatus};
export type MyWorkScope=MyWorkFilter&{project:string;userID:string;organizationID:string};
export const myWorkLimit=20;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const text=(value:unknown):value is string=>typeof value==='string';
const status=(value:unknown):value is typeof myWorkStatuses[number]=>typeof value==='string'&&(myWorkStatuses as readonly string[]).includes(value);
// Filter choices are curated; returned business states are not a closed enum.
// Current custom states must remain visible on All and contribute to its total.
const returnedStatus=(value:unknown):value is string=>typeof value==='string'&&!!value.trim()&&Array.from(value).length<=200&&!/[\u0000-\u001f\u007f]/.test(value)&&value!=='archived';

/** Search is literal. Do not trim spaces or interpret query punctuation. */
export function validMyWorkQuery(value:string){return !value.includes('\0')&&Array.from(value).length<=200;}
export function myWorkStatus(value:string):MyWorkStatus{return value==='all'?'':status(value)?value:'';}
export function myWorkScopeKey(scope:MyWorkScope){return JSON.stringify([scope.organizationID,scope.userID,scope.project,scope.q,scope.status]);}
export function myWorkPath(scope:MyWorkFilter&{project:string},cursor=''){
 const params=new URLSearchParams({project_id:scope.project,limit:String(myWorkLimit)});
 if(scope.q)params.set('q',scope.q);
 if(scope.status)params.set('status',scope.status);
 if(cursor)params.set('cursor',cursor);
 return '/my-work?'+params.toString();
}

/** The server resolves the human resource. Session IDs must never be used as
 * task owner IDs; every visible record must match the returned resource. */
export function validMyWorkPage(value:unknown,scope:MyWorkScope):value is MyWorkPage{
 if(!object(value)||value.project_id!==scope.project||value.user_id!==scope.userID||!text(value.resource_id)||!text(value.resource_name)||!['active','unbound','inactive'].includes(String(value.resource_status))||!Array.isArray(value.items)||value.items.length>myWorkLimit||!count(value.total)||value.total<value.items.length||value.limit!==myWorkLimit||!text(value.next_cursor)||value.next_cursor.length>16384||!text(value.as_of)||!Number.isFinite(Date.parse(value.as_of))||!Array.isArray(value.status_counts))return false;
 const seen=new Set<string>();let total=0;
 for(const row of value.status_counts){if(!object(row)||!returnedStatus(row.status)||!count(row.total)||seen.has(row.status))return false;seen.add(row.status);total+=row.total;if(!Number.isSafeInteger(total))return false;}
 if(value.resource_status!=='active')return value.items.length===0&&value.total===0&&value.status_counts.length===0&&!value.next_cursor;
 if(!value.resource_id||value.total!==(scope.status?value.status_counts.find((row:{status:string})=>row.status===scope.status)?.total??0:total))return false;
 const ids=new Set<string>();
 return value.items.every(item=>{
  if(!object(item)||!text(item.id)||!item.id||ids.has(item.id)||item.project_id!==scope.project||item.owner_id!==value.resource_id||!count(item.version)||item.version<1||!returnedStatus(item.status)||(scope.status&&item.status!==scope.status)||!text(item.title)||item.name!==undefined&&!text(item.name)||item.priority!==undefined&&!text(item.priority)||item.requirement_id!==undefined&&!text(item.requirement_id)||!text(item.updated_at)||!Number.isFinite(Date.parse(item.updated_at)))return false;
  ids.add(item.id);return true;
 });
}

/** Pages are a live keyset read, not a historical snapshot. Updated records
 * replace duplicate rows; a changed resource invalidates the whole old page. */
export function appendMyWorkPage(previous:MyWorkPage,next:MyWorkPage):MyWorkPage|undefined{
 if(previous.project_id!==next.project_id||previous.user_id!==next.user_id||previous.resource_id!==next.resource_id||next.resource_status!=='active')return undefined;
 const rows=new Map(previous.items.map(item=>[item.id,item]));
 for(const item of next.items)rows.set(item.id,item);
 const items=Array.from(rows.values());
 if(items.length>next.total)return undefined;
 return {...next,items};
}
