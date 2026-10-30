import type {WorkBuddyAuthorization,WorkBuddyConnection,WorkBuddyConnectionPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

export const workBuddyPageSize=20;
export const workBuddyStates=['connected','authorizing','refreshing','authorization_unknown','refresh_unknown','reauthorization_required','disconnected'] as const;
const scopes=['user.task.readable','user.task.invokable'];
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown,max=512):value is string=>typeof value==='string'&&value.length<=max&&!/[\x00-\x1f\x7f]/.test(value);
const positive=(value:unknown):value is number=>Number.isSafeInteger(value)&&Number(value)>0;
const date=(value:unknown,empty=false):value is string=>text(value,80)&&(empty&&value===''||Number.isFinite(Date.parse(value)));
const strings=(value:unknown,maximum:number,allowed?:readonly string[]):value is string[]=>Array.isArray(value)&&value.length<=maximum&&value.every(item=>text(item,200)&&!!item&&(!allowed||allowed.includes(item)))&&new Set(value).size===value.length;
export function readWorkBuddyConnection(value:unknown,project:string,owner?:string):WorkBuddyConnection|undefined{
 if(!object(value)||!text(value.id,200)||!value.id||value.project_id!==project||!positive(value.version)||!text(value.owner_id,200)||!value.owner_id||owner&&value.owner_id!==owner||!text(value.client_id)||!text(value.open_id)||!strings(value.scopes,16)||typeof value.state!=='string'||!workBuddyStates.includes(value.state as typeof workBuddyStates[number])||!Number.isSafeInteger(value.grant_generation)||Number(value.grant_generation)<0||!Number.isSafeInteger(value.token_generation)||Number(value.token_generation)<0||!date(value.expires_at,true)||!date(value.created_at)||!date(value.updated_at)||!text(value.last_error,200)||!strings(value.allowed_actions,3,['authorize','refresh','disconnect']))return;
 return value as unknown as WorkBuddyConnection;
}
export function readWorkBuddyPage(value:unknown,project:string,owner?:string):WorkBuddyConnectionPage|undefined{
 if(!object(value)||value.project_id!==project||typeof value.configured!=='boolean'||typeof value.can_authorize!=='boolean'||!strings(value.available_scopes,2,scopes)||!positive(value.limit)||value.limit>workBuddyPageSize||!text(value.next_cursor,2048)||!Array.isArray(value.items)||value.items.length>value.limit)return;
 const ids=new Set<string>();for(const item of value.items){const row=readWorkBuddyConnection(item,project,owner);if(!row||ids.has(row.id))return;ids.add(row.id);}
 return value as unknown as WorkBuddyConnectionPage;
}
/** Only the official fixed authorization page can become a clickable link. No
 * callback code/state is consumed here, and the URL is never a recovery field. */
export function officialWorkBuddyAuthorizationURL(value:unknown):value is string{
 if(!text(value,8192)||!value)return false;
 try{const url=new URL(value);return url.origin==='https://www.workbuddy.cn'&&url.pathname==='/openapi/v2/authorize'&&!url.username&&!url.password&&!url.hash;}catch{return false;}
}
export function readWorkBuddyAuthorization(value:unknown,project:string,owner?:string):WorkBuddyAuthorization|undefined{
 if(!object(value)||!text(value.id,200)||!value.id||!positive(value.version)||!readWorkBuddyConnection(value.connection,project,owner)||!date(value.expires_at)||!text(value.state,100)||!value.state||value.authorization_url!==undefined&&!officialWorkBuddyAuthorizationURL(value.authorization_url))return;
 return value as unknown as WorkBuddyAuthorization;
}
/** Persist only the original safe command, never provider data or a URL. */
export function validWorkBuddyRecovery(request:WriteRecoveryRequest,project:string):boolean{
 const body=request.body;if(request.method!=='POST'||!object(body)||body.project_id!==project||body.idempotency_key!==request.idempotency_key||!text(request.idempotency_key,128)||request.idempotency_key.length<8)return false;
 const keys=Object.keys(body).sort().join(',');
 if(request.path==='/workbuddy-authorizations')return strings(body.scopes,2,scopes)&&body.scopes.length>0&&(keys==='idempotency_key,project_id,scopes'||keys==='connection_id,expected_version,idempotency_key,project_id,scopes'&&text(body.connection_id,200)&&!!body.connection_id&&positive(body.expected_version));
 if(!/^\/workbuddy-connections\/[A-Za-z0-9_.%~-]+\/(refresh|disconnect)$/.test(request.path))return false;
 return positive(body.expected_version)&&(request.path.endsWith('/refresh')?keys==='expected_version,idempotency_key,project_id':keys==='expected_version,idempotency_key,project_id,reason'&&body.reason==='user_requested_local_disconnect');
}
export function validWorkBuddyReceipt(value:unknown,request:WriteRecoveryRequest,project:string,owner?:string):boolean{
 if(request.path==='/workbuddy-authorizations'){const result=readWorkBuddyAuthorization(value,project,owner);return !!result&&(!request.body.connection_id||result.connection.id===request.body.connection_id);}
 const result=readWorkBuddyConnection(value,project,owner),id=request.path.split('/')[2];return !!result&&result.id===decodeURIComponent(id)&&result.version>Number(request.body.expected_version);
}
