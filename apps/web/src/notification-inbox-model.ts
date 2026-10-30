import type {NotificationInboxCommandReceipt,NotificationInboxItem,NotificationInboxPage} from '../../../packages/contracts/src';

export const notificationInboxFilters=['inbox','unread','archived'] as const;
export type NotificationInboxFilter=typeof notificationInboxFilters[number];
export const notificationInboxActions=['read','unread','archive','restore'] as const;
export type NotificationInboxAction=typeof notificationInboxActions[number];
export const notificationInboxLimit=20;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const version=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=2147483647;
const count=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const instant=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value)&&Number.isFinite(Date.parse(value));
const stateTime=(state:boolean,value:unknown)=>state?instant(value):value==='';
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const operationKey=(value:unknown):value is string=>typeof value==='string'&&!!value.trim()&&!/[\p{Cc}]/u.test(value)&&new TextEncoder().encode(value).length>=8&&new TextEncoder().encode(value).length<=128;

export function validNotificationInboxFilter(value:unknown):value is NotificationInboxFilter{return typeof value==='string'&&(notificationInboxFilters as readonly string[]).includes(value);}
export function validNotificationInboxSequence(value:unknown):value is string{return typeof value==='string'&&/^[1-9]\d{0,18}$/.test(value)&&BigInt(value)<=9223372036854775807n;}
export function notificationInboxScope(userID:string,organizationID:string,project:string){return 'notification-inbox:'+JSON.stringify([userID,organizationID,project]);}
export function notificationInboxPath(project:string,filter:NotificationInboxFilter,cursor=''){
 const params=new URLSearchParams({project_id:project,filter,limit:String(notificationInboxLimit)});
 if(cursor)params.set('cursor',cursor);
 return '/notification-inbox?'+params.toString();
}
export function notificationInboxCommandPath(notificationID:string){return '/notification-inbox/'+encodeURIComponent(notificationID)+'/commands';}

/** Source facts are hydrated by this authorized read only. An unavailable
 * source has no cached task title or actionable task locator in the UI. */
export function validNotificationInboxItem(value:unknown):value is NotificationInboxItem{
 if(!object(value)||!id(value.id)||!version(value.version)||!validNotificationInboxSequence(value.sequence)||!['task_assigned','task_mentioned','task_followed','task_review_requested','task_handoff_requested'].includes(String(value.kind))||!id(value.task_id)||!version(value.source_task_version)||!id(value.source_resource_id)||!instant(value.created_at)||typeof value.read!=='boolean'||typeof value.archived!=='boolean'||!stateTime(value.read,value.read_at)||!stateTime(value.archived,value.archived_at)||typeof value.source_available!=='boolean'||typeof value.assignment_current!=='boolean')return false;
 const messageSource=value.kind==='task_mentioned'||value.kind==='task_followed';
 if(!messageSource&&value.message_id!==undefined||value.kind!=='task_review_requested'&&value.review_request_id!==undefined||value.kind!=='task_handoff_requested'&&value.handoff_id!==undefined||value.kind!=='task_assigned'&&value.assignment_current)return false;
 if(!value.source_available)return value.current_task===undefined&&value.message_id===undefined&&value.review_request_id===undefined&&value.handoff_id===undefined&&!value.assignment_current;
 if(messageSource&&!id(value.message_id))return false;
 if(value.kind==='task_review_requested'&&!id(value.review_request_id))return false;
 if(value.kind==='task_handoff_requested'&&!id(value.handoff_id))return false;
 const task=value.current_task;
 return object(task)&&task.id===value.task_id&&version(task.version)&&typeof task.title==='string'&&typeof task.status==='string';
}
export function validNotificationInboxPage(value:unknown,project:string,userID:string,filter:NotificationInboxFilter):value is NotificationInboxPage{
 if(!object(value)||value.project_id!==project||value.user_id!==userID||value.limit!==notificationInboxLimit||!Array.isArray(value.items)||value.items.length>notificationInboxLimit||!count(value.total)||value.total<value.items.length||!object(value.counts)||!exact(value.counts,['inbox','unread','archived'])||!count(value.counts.inbox)||!count(value.counts.unread)||!count(value.counts.archived)||value.counts.unread>value.counts.inbox||value.total!==value.counts[filter]||typeof value.next_cursor!=='string'||value.next_cursor.length>4096||!!value.next_cursor&&!value.items.length||!instant(value.as_of))return false;
 const ids=new Set<string>();let previous:bigint|undefined;
 for(const row of value.items){
  if(!validNotificationInboxItem(row)||ids.has(row.id)||filter==='archived'&&!row.archived||filter!=='archived'&&row.archived||filter==='unread'&&row.read)return false;
  const sequence=BigInt(row.sequence);if(previous!==undefined&&sequence>=previous)return false;
  previous=sequence;ids.add(row.id);
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

type RecoveryRequest={path:string;method:string;idempotency_key:string;body:Record<string,unknown>};
function requestID(request:RecoveryRequest){const match=request.path.match(/^\/notification-inbox\/([^/]+)\/commands$/);return match&&id(match[1])?match[1]:undefined;}
export function validNotificationInboxRecovery(request:RecoveryRequest,project:string){
 const body=request.body;
 return request.method==='POST'&&!!requestID(request)&&object(body)&&exact(body,['project_id','expected_version','action','idempotency_key'])&&body.project_id===project&&version(body.expected_version)&&body.expected_version<2147483647&&typeof body.action==='string'&&(notificationInboxActions as readonly string[]).includes(body.action)&&operationKey(request.idempotency_key)&&body.idempotency_key===request.idempotency_key;
}
export function validNotificationInboxReceipt(value:unknown,request:RecoveryRequest):value is NotificationInboxCommandReceipt{
 if(!object(value)||!exact(value,['idempotency_key','notification_id','version','action','read','archived','read_at','archived_at','updated_at'])||value.idempotency_key!==request.idempotency_key||value.notification_id!==requestID(request)||value.action!==request.body.action||!version(value.version)||typeof request.body.expected_version!=='number'||value.version<request.body.expected_version||value.version>request.body.expected_version+1||typeof value.read!=='boolean'||typeof value.archived!=='boolean'||!stateTime(value.read,value.read_at)||!stateTime(value.archived,value.archived_at)||!instant(value.updated_at))return false;
 return value.action==='read'?value.read:value.action==='unread'?!value.read:value.action==='archive'?value.archived:value.action==='restore'?!value.archived:false;
}
/** An original receipt acknowledges that original command, not a newer row.
 * Never replace a later server read or rehydrate a title from a receipt. */
export function notificationInboxReceiptIsStale(receipt:NotificationInboxCommandReceipt,page:NotificationInboxPage|undefined){return !!page?.items.some(row=>row.id===receipt.notification_id&&row.version>receipt.version);}
