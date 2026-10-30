import {expect,it} from 'vitest';
import {appendReviewRequests,readReviewDraft,reviewDraftKey,saveReviewDraft,validReviewRequest,validReviewRequestDetail,validReviewRequestPage,validReviewRequestRecovery,validReviewRequestReceipt,validReviewText} from './task-review-requests-model';

const item=(patch:Record<string,unknown>={})=>({id:'request-a',version:1,project_id:'p',task_id:'t',task_version:3,run_id:'run-a',run_version:4,task_basis_sha256:'a'.repeat(64),requester_user_id:'author',requester_name_at_request:'Author',reviewer_resource_id:'human-reviewer',reviewer_user_id:'reviewer',reviewer_name_at_request:'Reviewer',note:'Review this delivery',state:'pending',created_at:'2026-10-30T01:00:00.000000000Z',updated_at:'2026-10-30T01:00:00.000000000Z',cancelled_by_user_id:'',cancel_reason:'',closed_review_id:'',binding_current:true,reviewer_available:true,is_current_assignee:false,can_cancel:true,source_available:true,...patch});
const page=(items=[item()],patch:Record<string,unknown>={})=>({project_id:'p',task_id:'t',task_version:3,can_request:true,limit:20,items,total:items.length,next_cursor:'',pending_request_id:'request-a',...patch});
const original={path:'/task-review-requests',method:'POST',idempotency_key:'original-op',body:{project_id:'p',task_id:'t',expected_task_version:3,run_id:'run-a',expected_run_version:4,reviewer_resource_id:'human-reviewer',note:'Review this delivery',idempotency_key:'original-op'}};

it('validates exact current task request facts without treating binding or assignee as acceptance permission',()=>{
 expect(validReviewRequest(item(),'p','t')).toBe(true);expect(validReviewRequest(item({binding_current:false,reviewer_available:false}),'p','t')).toBe(true);
 expect(validReviewRequest(item({run_id:'r'.repeat(100)}),'p','t')).toBe(true);expect(validReviewRequest(item({run_id:'r'.repeat(101)}),'p','t')).toBe(false);
 expect(validReviewRequest(item({project_id:'other'}),'p','t')).toBe(false);expect(validReviewRequest(item({state:'accepted'}),'p','t')).toBe(false);
 expect(validReviewRequest(item({state:'closed',can_cancel:false,closed_review_id:'review-real',decision:{review_id:'review-real',reviewer_user_id:'other-admin',decision:'rejected',reason:'Original quality reason',created_at:'2026-10-30T02:00:00Z'}}),'p','t')).toBe(true);
 expect(validReviewRequest(item({state:'closed',can_cancel:false,closed_review_id:'review-real',source_available:false}),'p','t')).toBe(true);
 expect(validReviewRequest(item({decision:{review_id:'invented'}}),'p','t')).toBe(false);
 expect(validReviewRequestDetail({project_id:'p',task_id:'t',task_title:'Task',task_version:5,can_request:true,request:item()},'p','t','request-a')).toBe(true);
 expect(validReviewRequestDetail({project_id:'p',task_id:'other',task_title:'Task',task_version:5,can_request:true,request:item()},'p','t','request-a')).toBe(false);
});
it('bounds live history, permits a pending reference outside the page and rejects duplicate/no-progress merges',()=>{
 const first=page([item()],{total:2,pending_request_id:'request-outside',next_cursor:'next'});
 expect(validReviewRequestPage(first,'p','t')).toBe(true);expect(appendReviewRequests(first as never,page([item()]) as never)).toBeUndefined();
 const older=item({id:'request-old',state:'cancelled',can_cancel:false,cancelled_by_user_id:'author',cancel_reason:'Explicit cancellation',created_at:'2026-10-29T01:00:00.000000000Z'});
 expect(appendReviewRequests(first as never,page([older],{total:2}) as never)?.items).toHaveLength(2);
 expect(validReviewRequestPage(page([older,item()]),'p','t')).toBe(false);
});
it('recovers one exact request or cancel command and verifies its unchanged returned basis',()=>{
 expect(validReviewRequestRecovery(original,'p','t')).toBe(true);expect(validReviewRequestReceipt(item(),original,'p','t','author')).toBe(true);
 expect(validReviewRequestReceipt(item({run_version:5}),original,'p','t','author')).toBe(false);
 expect(validReviewRequestRecovery({...original,body:{...original.body,can_accept:true}},'p','t')).toBe(false);
 expect(validReviewRequestRecovery({...original,body:{...original.body,expected_run_version:2147483647}},'p','t')).toBe(false);
 const cancel={path:'/task-review-requests/request-a/commands',method:'POST',idempotency_key:'cancel-op',body:{project_id:'p',expected_version:1,action:'cancel',reason:'No longer current',idempotency_key:'cancel-op'}};
 expect(validReviewRequestRecovery(cancel,'p','t')).toBe(true);
 expect(validReviewRequestReceipt(item({state:'cancelled',version:2,can_cancel:false,cancelled_by_user_id:'author',cancel_reason:'No longer current'}),cancel,'p','t','author')).toBe(true);
 for(const text of ['', ' \n ', 'x\0y','🧪'.repeat(2001)])expect(validReviewText(text)).toBe(false);
});
it('isolates unsent basis/recipient/note and cancellation drafts without storing private directory facts',()=>{
 const data=new Map<string,string>(),storage={getItem:(key:string)=>data.get(key)??null,setItem:(key:string,value:string)=>{data.set(key,value);},removeItem:(key:string)=>{data.delete(key);}};
 const key=reviewDraftKey('author','org','p','t'),draft={reviewer_resource_id:'human-reviewer',note:'  Original note  ',basis:{task_version:3,run_id:'run-a',run_version:4}};
 saveReviewDraft(storage,key,draft);expect(readReviewDraft(storage,key).draft).toEqual(draft);expect(readReviewDraft(storage,reviewDraftKey('other','org','p','t')).draft.note).toBe('');expect(data.get(key)).not.toContain('name');
 data.set(key,'{"format":"unknown","note":"unique"}');expect(readReviewDraft(storage,key).invalid).toBe(true);expect(data.get(key)).toContain('unique');
});
