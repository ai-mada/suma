import type {
 NotificationInboxBatchReceipt,NotificationInboxBatchTarget,NotificationInboxCommandReceipt,NotificationInboxItem,NotificationInboxPage,
 NotificationOrganizationInboxItem,NotificationOrganizationInboxPage,
} from '../../../packages/contracts/src';

export const notificationInboxFilters=['inbox','unread','archived'] as const;
export type NotificationInboxFilter=typeof notificationInboxFilters[number];
export const notificationInboxActions=['read','unread','archive','restore'] as const;
export type NotificationInboxAction=typeof notificationInboxActions[number];
export type NotificationInboxScopeMode='project'|'organization';
export type InboxReadPage=NotificationInboxPage|NotificationOrganizationInboxPage;
export const notificationInboxLimit=20;
export const notificationInboxSelectionLimit=20;
export const notificationInboxBatchPath='/notification-inbox/batch-commands';
export type InboxWriteReceipt=NotificationInboxCommandReceipt|NotificationInboxBatchReceipt;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const version=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=2147483647;
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const instant=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value)&&Number.isFinite(Date.parse(value));
const stateTime=(state:boolean,value:unknown)=>state?instant(value):value==='';
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const operationKey=(value:unknown):value is string=>typeof value==='string'&&!!value.trim()&&!/[\p{Cc}]/u.test(value)&&new TextEncoder().encode(value).length>=8&&new TextEncoder().encode(value).length<=128;

export type NotificationToolInputTarget=NonNullable<NotificationInboxItem['tool_input_target']>;

export function validNotificationToolInputTarget(value:unknown):value is NotificationToolInputTarget{
 return object(value)&&exact(value,['run_id','node_id','attempt'])&&
  typeof value.run_id==='string'&&value.run_id.trim()===value.run_id&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value.run_id)&&
  typeof value.node_id==='string'&&value.node_id.trim()===value.node_id&&value.node_id.length<=150&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}(?:\[[0-9]+\])?$/.test(value.node_id)&&
  typeof value.attempt==='number'&&Number.isSafeInteger(value.attempt)&&value.attempt>0;
}

export function validNotificationInboxFilter(value:unknown):value is NotificationInboxFilter{return typeof value==='string'&&(notificationInboxFilters as readonly string[]).includes(value);}
export function validNotificationInboxSequence(value:unknown):value is string{return typeof value==='string'&&/^[1-9]\d{0,18}$/.test(value)&&BigInt(value)<=9223372036854775807n;}
export function notificationInboxScope(userID:string,organizationID:string,project:string){return 'notification-inbox:'+JSON.stringify([userID,organizationID,project]);}
export function notificationOrganizationInboxScope(userID:string,organizationID:string){return 'notification-inbox:organization:'+JSON.stringify([userID,organizationID]);}
export function notificationInboxPath(project:string,filter:NotificationInboxFilter,cursor=''){
 const params=new URLSearchParams({project_id:project,filter,limit:String(notificationInboxLimit)});
 if(cursor)params.set('cursor',cursor);
 return '/notification-inbox?'+params.toString();
}
export function notificationOrganizationInboxPath(filter:NotificationInboxFilter,cursor=''){
 const params=new URLSearchParams({filter,limit:String(notificationInboxLimit)});
 if(cursor)params.set('cursor',cursor);
 return '/notification-inbox/organization?'+params.toString();
}
export function notificationInboxCommandPath(notificationID:string){return '/notification-inbox/'+encodeURIComponent(notificationID)+'/commands';}

/** Source facts are hydrated by this authorized read only. An unavailable
 * source has no cached task title or actionable task locator in the UI. */
export function validNotificationInboxItem(value:unknown):value is NotificationInboxItem{
 if(!object(value)||!id(value.id)||!version(value.version)||!validNotificationInboxSequence(value.sequence)||typeof value.kind!=='string'||!['task_assigned','task_mentioned','task_followed','task_review_requested','task_handoff_requested','task_run_failed','task_tool_input_requested'].includes(value.kind)||!id(value.task_id)||!version(value.source_task_version)||!id(value.source_resource_id)||!instant(value.created_at)||typeof value.read!=='boolean'||typeof value.archived!=='boolean'||!stateTime(value.read,value.read_at)||!stateTime(value.archived,value.archived_at)||typeof value.source_available!=='boolean'||typeof value.assignment_current!=='boolean')return false;
 const messageSource=value.kind==='task_mentioned'||value.kind==='task_followed';
 const runSource=value.kind==='task_run_failed';
 const toolInputSource=value.kind==='task_tool_input_requested';
 if(!messageSource&&value.message_id!==undefined||value.kind!=='task_review_requested'&&value.review_request_id!==undefined||value.kind!=='task_handoff_requested'&&value.handoff_id!==undefined||!runSource&&value.run_id!==undefined||!toolInputSource&&value.tool_input_target!==undefined||value.kind!=='task_assigned'&&value.assignment_current)return false;
 if(!value.source_available)return value.current_task===undefined&&value.message_id===undefined&&value.review_request_id===undefined&&value.handoff_id===undefined&&value.run_id===undefined&&value.tool_input_target===undefined&&!value.assignment_current;
 if(messageSource&&!id(value.message_id))return false;
 if(value.kind==='task_review_requested'&&!id(value.review_request_id))return false;
 if(value.kind==='task_handoff_requested'&&!id(value.handoff_id))return false;
 if(runSource&&!id(value.run_id))return false;
 if(toolInputSource&&!validNotificationToolInputTarget(value.tool_input_target))return false;
 const task=value.current_task;
 return object(task)&&task.id===value.task_id&&version(task.version)&&typeof task.title==='string'&&typeof task.status==='string';
}
export function validNotificationInboxPage(value:unknown,project:string,userID:string,filter:NotificationInboxFilter):value is NotificationInboxPage{
 if(!validPageFacts(value,userID,filter)||value.project_id!==project)return false;
 const ids=new Set<string>();let previous:bigint|undefined;
 for(const row of value.items){
  if(!validNotificationInboxItem(row)||ids.has(row.id)||!inFilter(row,filter))return false;
  const sequence=BigInt(row.sequence);if(previous!==undefined&&sequence>=previous)return false;
  previous=sequence;ids.add(row.id);
 }
 return true;
}
function validPageFacts(value:unknown,userID:string,filter:NotificationInboxFilter):value is Record<string,unknown>&{items:unknown[]}{
 return object(value)&&value.user_id===userID&&value.limit===notificationInboxLimit&&Array.isArray(value.items)&&value.items.length<=notificationInboxLimit&&count(value.total)&&value.total>=value.items.length&&object(value.counts)&&exact(value.counts,['inbox','unread','archived'])&&count(value.counts.inbox)&&count(value.counts.unread)&&count(value.counts.archived)&&value.counts.unread<=value.counts.inbox&&value.total===value.counts[filter]&&typeof value.next_cursor==='string'&&value.next_cursor.length<=4096&&(!value.next_cursor||!!value.items.length)&&instant(value.as_of);
}
function inFilter(row:NotificationInboxItem,filter:NotificationInboxFilter){return row.archived===(filter==='archived')&&(filter!=='unread'||!row.read);}
function orderedInstant(value:string){
 // Normalize optional RFC3339Nano fractions before comparing. Date.parse
 // alone would collapse two events from the same millisecond.
 return value.slice(0,19)+value.slice(20,-1).padEnd(9,'0');
}
function organizationOrderKey(row:NotificationInboxItem){return orderedInstant(row.created_at)+' '+row.id;}
export function validNotificationOrganizationInboxPage(value:unknown,organizationID:string,userID:string,filter:NotificationInboxFilter):value is NotificationOrganizationInboxPage{
 if(!validPageFacts(value,userID,filter)||!exact(value,['organization_id','user_id','limit','items','total','counts','next_cursor','as_of','scope_revision'])||value.organization_id!==organizationID||typeof value.scope_revision!=='string'||!/^[a-f0-9]{64}$/.test(value.scope_revision))return false;
 const ids=new Set<string>();
 let previous:string|undefined;
 for(const row of value.items){
  if(!object(row)||!exact(row,['project_id','project_title','notification'])||!id(row.project_id)||typeof row.project_title!=='string'||!validNotificationInboxItem(row.notification)||ids.has(row.notification.id)||!inFilter(row.notification,filter))return false;
  const current=organizationOrderKey(row.notification);
  if(previous!==undefined&&current>=previous)return false;
  previous=current;ids.add(row.notification.id);
 }
 return true;
}
export function appendNotificationInboxPage(previous:NotificationInboxPage,next:NotificationInboxPage):NotificationInboxPage|undefined{
 if(previous.project_id!==next.project_id||previous.user_id!==next.user_id)return;
 const last=previous.items.at(-1),first=next.items[0];
 if(last&&first&&BigInt(first.sequence)>=BigInt(last.sequence))return;
 const ids=new Set(previous.items.map(row=>row.id));if(next.items.some(row=>ids.has(row.id)))return;
 const items=[...previous.items,...next.items];return items.length<=next.total?{...next,items}:undefined;
}
export function appendNotificationOrganizationInboxPage(previous:NotificationOrganizationInboxPage,next:NotificationOrganizationInboxPage):NotificationOrganizationInboxPage|undefined{
 if(previous.organization_id!==next.organization_id||previous.user_id!==next.user_id||previous.scope_revision!==next.scope_revision)return;
 const last=previous.items.at(-1),first=next.items[0];
 if(last&&first&&organizationOrderKey(first.notification)>=organizationOrderKey(last.notification))return;
 const ids=new Set(previous.items.map(row=>row.notification.id));
 if(next.items.some(row=>ids.has(row.notification.id)))return;
 const items=[...previous.items,...next.items];return items.length<=next.total?{...next,items}:undefined;
}
export function inboxRows(page:InboxReadPage):NotificationOrganizationInboxItem[]{
 return 'organization_id' in page?page.items:page.items.map(notification=>({notification,project_id:page.project_id,project_title:''}));
}

type RecoveryRequest={path:string;method:string;idempotency_key:string;body:Record<string,unknown>};
function requestID(request:RecoveryRequest){const match=request.path.match(/^\/notification-inbox\/([^/]+)\/commands$/);return match&&id(match[1])?match[1]:undefined;}
function validBatchTargets(value:unknown,project?:string):value is NotificationInboxBatchTarget[]{
 if(!Array.isArray(value)||value.length<1||value.length>notificationInboxSelectionLimit)return false;
 const seen=new Set<string>();
 for(const target of value){
  if(!object(target)||!exact(target,['project_id','notification_id','expected_version'])||!id(target.project_id)||
   project!==undefined&&target.project_id!==project||!id(target.notification_id)||seen.has(target.notification_id)||
   !version(target.expected_version)||target.expected_version===2147483647)return false;
  seen.add(target.notification_id);
 }
 return true;
}

export function validNotificationInboxRecovery(request:RecoveryRequest,project?:string){
 const body=request.body;
 if(request.method!=='POST'||!object(body)||typeof body.action!=='string'||
  !(notificationInboxActions as readonly string[]).includes(body.action)||
  !operationKey(request.idempotency_key)||body.idempotency_key!==request.idempotency_key)return false;
 if(request.path===notificationInboxBatchPath){
  return exact(body,['items','action','idempotency_key'])&&validBatchTargets(body.items,project);
 }
 return !!requestID(request)&&exact(body,['project_id','expected_version','action','idempotency_key'])&&
  id(body.project_id)&&(project===undefined||body.project_id===project)&&version(body.expected_version)&&body.expected_version<2147483647;
}
export function validNotificationInboxReceipt(value:unknown,request:RecoveryRequest):value is NotificationInboxCommandReceipt{
 if(!object(value)||!exact(value,['idempotency_key','notification_id','version','action','read','archived','read_at','archived_at','updated_at'])||value.idempotency_key!==request.idempotency_key||value.notification_id!==requestID(request)||value.action!==request.body.action||!version(value.version)||typeof request.body.expected_version!=='number'||value.version<request.body.expected_version||value.version>request.body.expected_version+1||typeof value.read!=='boolean'||typeof value.archived!=='boolean'||!stateTime(value.read,value.read_at)||!stateTime(value.archived,value.archived_at)||!instant(value.updated_at))return false;
 return value.action==='read'?value.read:value.action==='unread'?!value.read:value.action==='archive'?value.archived:value.action==='restore'?!value.archived:false;
}
/** A batch receipt must acknowledge the exact chosen targets in order. */
export function validNotificationInboxResult(value:unknown,request:RecoveryRequest):value is InboxWriteReceipt{
 if(request.path!==notificationInboxBatchPath)return validNotificationInboxReceipt(value,request);
 const items=request.body.items;
 if(!validBatchTargets(items)||!object(value)||!exact(value,['idempotency_key','action','results'])||
  value.idempotency_key!==request.idempotency_key||value.action!==request.body.action||
  !Array.isArray(value.results)||value.results.length!==items.length)return false;
 return value.results.every((result,index)=>{
  const target=items[index];
  return object(result)&&exact(result,['project_id','receipt'])&&result.project_id===target.project_id&&
   validNotificationInboxReceipt(result.receipt,{
    path:notificationInboxCommandPath(target.notification_id),method:request.method,idempotency_key:request.idempotency_key,
    body:{action:request.body.action,expected_version:target.expected_version},
   });
 });
}

/** An original receipt acknowledges that original command, not a newer row.
 * Never replace a later server read or rehydrate a title from a receipt. */
export function notificationInboxReceiptIsStale(receipt:NotificationInboxCommandReceipt,page:InboxReadPage|undefined){return !!page&&inboxRows(page).some(({notification})=>notification.id===receipt.notification_id&&notification.version>receipt.version);}
