import {expect,it} from 'vitest';
import {appendNotificationInboxPage,appendNotificationOrganizationInboxPage,notificationInboxBatchPath,notificationInboxCommandPath,notificationInboxPath,notificationInboxReceiptIsStale,validNotificationInboxItem,validNotificationInboxPage,validNotificationInboxReceipt,validNotificationInboxRecovery,validNotificationInboxResult,validNotificationInboxSequence,validNotificationOrganizationInboxPage} from './notification-inbox-model';
import type {NotificationInboxCommandReceipt,NotificationInboxPage,NotificationOrganizationInboxPage} from '../../../packages/contracts/src';

const row=(patch:Record<string,unknown>={})=>({id:'notice-a',version:1,sequence:'9007199254740993',kind:'task_assigned',task_id:'task-a',source_task_version:2,source_resource_id:'human-a',created_at:'2026-10-29T02:00:00Z',read:false,archived:false,read_at:'',archived_at:'',source_available:true,assignment_current:true,current_task:{id:'task-a',version:4,title:'Current source title',status:'draft'},...patch});
const page=(patch:Record<string,unknown>={})=>({project_id:'project-a',user_id:'user-a',limit:20,items:[row()],total:1,counts:{inbox:1,unread:1,archived:0},next_cursor:'',as_of:'2026-10-29T03:00:00Z',...patch});
const request=(patch:Record<string,unknown>={})=>({path:notificationInboxCommandPath('notice-a'),method:'POST',idempotency_key:'original-operation-a',body:{project_id:'project-a',expected_version:1,action:'read',idempotency_key:'original-operation-a'},...patch});
const receipt=(patch:Record<string,unknown>={})=>({idempotency_key:'original-operation-a',notification_id:'notice-a',version:2,action:'read',read:true,archived:false,read_at:'2026-10-29T04:00:00Z',archived_at:'',updated_at:'2026-10-29T04:00:00Z',...patch});

it('requires the original message only for an available followed-task source and rejects incomplete or cross-kind locators',()=>{
 const followed={kind:'task_followed',assignment_current:false,source_resource_id:'subscription-a',message_id:'message-original'};
 expect(validNotificationInboxItem(row(followed))).toBe(true);
 expect(validNotificationInboxPage(page({items:[row(followed)]}),'project-a','user-a','inbox')).toBe(true);
 for(const patch of [{message_id:undefined},{message_id:'../other'},{assignment_current:true},{review_request_id:'review-a'},{handoff_id:'handoff-a'}])expect(validNotificationInboxItem(row({...followed,...patch}))).toBe(false);
 expect(validNotificationInboxItem(row({...followed,source_available:false,current_task:undefined}))).toBe(false);
 expect(validNotificationInboxItem(row({...followed,source_available:false,current_task:undefined,message_id:undefined}))).toBe(true);
 expect(validNotificationInboxItem(row({message_id:'message-original'}))).toBe(false);
});

it('opens only an intact handoff source and rejects cross-kind or unavailable handoff locators',()=>{
 const handoff={kind:'task_handoff_requested',assignment_current:false,handoff_id:'handoff-original'};
 expect(validNotificationInboxItem(row(handoff))).toBe(true);
 for(const patch of [{handoff_id:undefined},{handoff_id:'../foreign'},{handoff_id:''},{assignment_current:true},{message_id:'message-a'},{review_request_id:'request-a'}])expect(validNotificationInboxItem(row({...handoff,...patch}))).toBe(false);
 for(const kind of ['task_assigned','task_mentioned','task_review_requested'])expect(validNotificationInboxItem(row({kind,handoff_id:'handoff-original',message_id:kind==='task_mentioned'?'message-a':undefined,review_request_id:kind==='task_review_requested'?'request-a':undefined,assignment_current:kind==='task_assigned'}))).toBe(false);
 expect(validNotificationInboxItem(row({...handoff,source_available:false,current_task:undefined,handoff_id:undefined}))).toBe(true);
 expect(validNotificationInboxItem(row({...handoff,source_available:false,current_task:undefined}))).toBe(false);
});

it('accepts a failed run only with its exact locator and keeps unavailable sources private',()=>{
 const failed={kind:'task_run_failed',assignment_current:false,run_id:'run-original'};
 expect(validNotificationInboxItem(row(failed))).toBe(true);
 for(const patch of [{run_id:undefined},{run_id:null},{run_id:'../other'},{kind:['task_run_failed'],run_id:undefined},{assignment_current:true},{message_id:'message-a'},{handoff_id:'handoff-a'},{review_request_id:'review-a'}]){
  expect(validNotificationInboxItem(row({...failed,...patch}))).toBe(false);
 }
 expect(validNotificationInboxItem(row({run_id:'run-original'}))).toBe(false);
 const unavailable={...failed,source_available:false,run_id:undefined};
 expect(validNotificationInboxItem(row(unavailable))).toBe(false);
 expect(validNotificationInboxItem(row({...unavailable,current_task:undefined}))).toBe(true);
 expect(validNotificationInboxItem(row({...unavailable,current_task:undefined,run_id:'run-original'}))).toBe(false);
});

it('opens only an intact review-request source and rejects cross-kind source aliases',()=>{
 expect(validNotificationInboxItem(row({kind:'task_review_requested',assignment_current:false,review_request_id:'request-a'}))).toBe(true);
 expect(validNotificationInboxItem(row({kind:'task_review_requested',assignment_current:false}))).toBe(false);
 expect(validNotificationInboxItem(row({kind:'task_review_requested',assignment_current:true,review_request_id:'request-a'}))).toBe(false);
 expect(validNotificationInboxItem(row({kind:'task_review_requested',assignment_current:false,review_request_id:'request-a',message_id:'message-a'}))).toBe(false);
 expect(validNotificationInboxItem(row({review_request_id:'request-a'}))).toBe(false);
 expect(validNotificationInboxItem(row({kind:'task_review_requested',assignment_current:false,source_available:false,current_task:undefined}))).toBe(true);
 expect(validNotificationInboxItem(row({kind:'task_review_requested',assignment_current:false,source_available:false,current_task:undefined,review_request_id:'request-a'}))).toBe(false);
});

it('requires an exact message locator only for an available mention source and keeps assignment semantics separate',()=>{
 expect(validNotificationInboxItem(row({kind:'task_mentioned',assignment_current:false,message_id:'message-a'}))).toBe(true);
 expect(validNotificationInboxItem(row({kind:'task_mentioned',assignment_current:false}))).toBe(false);
 expect(validNotificationInboxItem(row({kind:'task_mentioned',assignment_current:true,message_id:'message-a'}))).toBe(false);
 expect(validNotificationInboxItem(row({kind:'task_mentioned',assignment_current:false,source_available:false,current_task:undefined}))).toBe(true);
 expect(validNotificationInboxItem(row({kind:'task_mentioned',assignment_current:false,source_available:false,current_task:undefined,message_id:'message-a'}))).toBe(false);
 expect(validNotificationInboxItem(row({message_id:'message-a'}))).toBe(false);
});

it('keeps signed BIGINT sequences exact above JavaScript numeric precision and validates descending order',()=>{
 expect(validNotificationInboxSequence('9223372036854775807')).toBe(true);for(const value of ['0','01','1e3','9223372036854775808',9007199254740993])expect(validNotificationInboxSequence(value)).toBe(false);
 const ordered=page({items:[row(),row({id:'notice-b',sequence:'9007199254740992'})],total:2,counts:{inbox:2,unread:2,archived:0}});expect(validNotificationInboxPage(ordered,'project-a','user-a','inbox')).toBe(true);expect(validNotificationInboxPage({...ordered,items:[...ordered.items].reverse()},'project-a','user-a','inbox')).toBe(false);
 const params=new URLSearchParams(notificationInboxPath('project-a','unread','opaque+/=').split('?')[1]);expect(params.get('filter')).toBe('unread');expect(params.get('cursor')).toBe('opaque+/=');expect(params.has('user_id')).toBe(false);
});

it('accepts only coherent lifecycle facts and current authorized source hydration',()=>{
 expect(validNotificationInboxItem(row())).toBe(true);expect(validNotificationInboxItem(row({source_available:false,assignment_current:false,current_task:undefined}))).toBe(true);
 for(const patch of [{read:'false'},{read_at:'2026-10-29T02:00:00Z'},{archived:true},{source_available:false},{source_available:false,assignment_current:false,current_task:null},{source_available:true,current_task:undefined},{current_task:{id:'other-task',version:1,title:'Foreign',status:'todo'}},{version:2147483648},{current_task:{id:'task-a',version:0,title:'Invalid source',status:'todo'}}])expect(validNotificationInboxItem(row(patch))).toBe(false);
});

it('binds full counts and row membership to the current receiver/project/filter',()=>{
 expect(validNotificationInboxPage(page(),'project-a','user-a','unread')).toBe(true);
 for(const patch of [{project_id:'other-project'},{user_id:'other-user'},{items:[row({archived:true,archived_at:'2026-10-29T04:00:00Z'})]},{items:[row({read:true,read_at:'2026-10-29T04:00:00Z'})]},{counts:{inbox:1,unread:2,archived:0}},{total:8},{next_cursor:'x'.repeat(4097)}])expect(validNotificationInboxPage(page(patch),'project-a','user-a','unread')).toBe(false);
 expect(validNotificationInboxPage(page({items:[row({archived:true,archived_at:'2026-10-29T04:00:00Z'})],counts:{inbox:0,unread:0,archived:1}}),'project-a','user-a','archived')).toBe(true);
});

it('appends only a lower sequence window and refuses overlapping or shrinking live pages',()=>{
 const before=page({next_cursor:'next'}) as unknown as NotificationInboxPage,next=page({items:[row({id:'notice-b',sequence:'9007199254740992'})],total:2,counts:{inbox:2,unread:2,archived:0}}) as unknown as NotificationInboxPage;
 expect(appendNotificationInboxPage(before,next)?.items.map(item=>item.sequence)).toEqual(['9007199254740993','9007199254740992']);expect(appendNotificationInboxPage(before,{...next,items:before.items})).toBeUndefined();expect(appendNotificationInboxPage(before,{...next,total:1})).toBeUndefined();expect(appendNotificationInboxPage(before,{...next,user_id:'other-user'})).toBeUndefined();
});

it('validates the exact original request, byte-bounded operation key and supported CAS version',()=>{
 expect(validNotificationInboxRecovery(request(),'project-a')).toBe(true);
 const oversized='界'.repeat(128);for(const key of [oversized,' '.repeat(20),'tiny'])expect(validNotificationInboxRecovery(request({idempotency_key:key,body:{...request().body,idempotency_key:key}}),'project-a')).toBe(false);
 expect(validNotificationInboxRecovery(request({body:{...request().body,expected_version:2147483647}}),'project-a')).toBe(false);expect(validNotificationInboxRecovery(request({body:{...request().body,task_id:'task-a'}}),'project-a')).toBe(false);expect(validNotificationInboxRecovery(request(),'other-project')).toBe(false);
});

it('acknowledges only the original lifecycle receipt and never regresses a newer notification row',()=>{
 expect(validNotificationInboxReceipt(receipt(),request())).toBe(true);expect(validNotificationInboxReceipt(receipt({version:1}),request())).toBe(true);
 for(const patch of [{idempotency_key:'other-operation'},{notification_id:'other-notice'},{action:'archive'},{read:false,read_at:''},{version:4},{current_task:{title:'Forbidden source snapshot'}}])expect(validNotificationInboxReceipt(receipt(patch),request())).toBe(false);
 expect(notificationInboxReceiptIsStale(receipt() as NotificationInboxCommandReceipt,page({items:[row({version:5})]}) as NotificationInboxPage)).toBe(true);expect(notificationInboxReceiptIsStale(receipt() as NotificationInboxCommandReceipt,page() as NotificationInboxPage)).toBe(false);
});

it('validates the organization scope, UTC nanosecond ordering and cursor continuation across different projects',()=>{
 const before=row({id:'notice-z',created_at:'2026-10-29T02:00:00.000000002Z'});
 const tie=row({id:'notice-a',created_at:'2026-10-29T02:00:00.000000002Z'});
 const lower=row({id:'notice-y',created_at:'2026-10-29T02:00:00.000000001Z'});
 const whole=row({id:'notice-x',created_at:'2026-10-29T02:00:00Z'});
 const notices=[before,tie,lower,whole].map((notification,index)=>({project_id:index%2?'project-a':'project-b',project_title:'Current project',notification}));
 const value={organization_id:'org-a',user_id:'user-a',limit:20,items:notices,total:4,counts:{inbox:4,unread:4,archived:0},next_cursor:'',as_of:'2026-10-29T03:00:00Z',scope_revision:'a'.repeat(64)};
 expect(validNotificationOrganizationInboxPage(value,'org-a','user-a','inbox')).toBe(true);
 expect(validNotificationOrganizationInboxPage({...value,items:[notices[1],notices[0],...notices.slice(2)]},'org-a','user-a','inbox')).toBe(false);
 expect(validNotificationOrganizationInboxPage({...value,items:[...notices.slice(0,2),notices[3],notices[2]]},'org-a','user-a','inbox')).toBe(false);
 expect(validNotificationOrganizationInboxPage({...value,organization_id:'other-org'},'org-a','user-a','inbox')).toBe(false);
 expect(validNotificationOrganizationInboxPage({...value,scope_revision:'unexpected'},'org-a','user-a','inbox')).toBe(false);
 const first={...value,items:notices.slice(0,2),next_cursor:'next'} as NotificationOrganizationInboxPage;
 const next={...value,items:notices.slice(2)} as NotificationOrganizationInboxPage;
 expect(appendNotificationOrganizationInboxPage(first,next)?.items).toEqual(notices);
 expect(appendNotificationOrganizationInboxPage(first,{...next,scope_revision:'b'.repeat(64)})).toBeUndefined();
});

it('requires unique scoped targets and a complete batch receipt in the original order',()=>{
 const items=[
  {project_id:'project-a',notification_id:'notice-a',expected_version:1},
  {project_id:'project-b',notification_id:'notice-b',expected_version:5},
 ];
 const batch=request({path:notificationInboxBatchPath,body:{items,action:'read',idempotency_key:request().idempotency_key}});
 const results=[
  {project_id:'project-a',receipt:receipt()},
  {project_id:'project-b',receipt:receipt({notification_id:'notice-b',version:6})},
 ];
 const result={idempotency_key:batch.idempotency_key,action:'read',results};
 expect(validNotificationInboxRecovery(batch)).toBe(true);
 expect(validNotificationInboxRecovery(batch,'project-a')).toBe(false);
 const invalidTargets=[[],[items[0],{...items[1],notification_id:'notice-a'}],Array.from({length:21},(_,index)=>({...items[0],notification_id:'notice-'+index}))];
 for(const targets of invalidTargets)expect(validNotificationInboxRecovery({...batch,body:{...batch.body,items:targets}})).toBe(false);
 expect(validNotificationInboxResult(result,batch)).toBe(true);
 expect(validNotificationInboxResult({...result,results:[results[1],results[0]]},batch)).toBe(false);
 expect(validNotificationInboxResult({...result,results:[results[0]]},batch)).toBe(false);
 expect(validNotificationInboxResult({...result,results:[results[0],{...results[1],project_id:'project-a'}]},batch)).toBe(false);
 expect(validNotificationInboxResult({...result,results:[results[0],{...results[1],receipt:receipt({notification_id:'notice-b',version:7})}]},batch)).toBe(false);
});
