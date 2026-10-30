import type {TaskReviewRequest,TaskReviewRequestDetail,TaskReviewRequestPage,TaskReviewTargetPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
export const reviewID=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(value);
const reviewRunID=(value:unknown):value is string=>typeof value==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,100}$/.test(value);
const natural=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
export const reviewVersion=(value:unknown):value is number=>natural(value)&&value>0&&value<=2147483647;
const instant=(value:unknown):value is string=>typeof value==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(value)&&Number.isFinite(Date.parse(value));
const cursor=(value:unknown):value is string=>typeof value==='string'&&value.length<=4096&&!/[\p{Cc}]/u.test(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
export const validReviewText=(value:unknown):value is string=>typeof value==='string'&&!!value.trim()&&!/[\0\uD800-\uDFFF]/u.test(value)&&Array.from(value).length<=2000&&new TextEncoder().encode(value).length<=8000;
export const validReviewQuery=(value:string)=>Array.from(value).length<=200&&!/[\p{Cc}\uD800-\uDFFF]/u.test(value);
export function reviewRequestsPath(project:string,task:string){return '/task-review-requests?'+new URLSearchParams({project_id:project,task_id:task,limit:'20'});}
export function reviewRequestPath(project:string,id:string){return '/task-review-requests/'+encodeURIComponent(id)+'?'+new URLSearchParams({project_id:project});}
export function reviewTargetsPath(project:string,task:string,q='',selected=''){const params=new URLSearchParams({project_id:project,task_id:task,limit:'20'});if(selected)params.set('resource_ids',selected);else if(q)params.set('q',q);return '/task-review-targets?'+params;}

/** All statuses here are responsibility lifecycle facts. None grants formal
 * acceptance, which remains exclusively in the existing Review command. */
export function validReviewRequest(value:unknown,project:string,task:string):value is TaskReviewRequest{
 if(!object(value)||!reviewID(value.id)||!reviewVersion(value.version)||value.project_id!==project||value.task_id!==task||!reviewVersion(value.task_version)||!reviewRunID(value.run_id)||!reviewVersion(value.run_version)||typeof value.task_basis_sha256!=='string'||!/^[a-f0-9]{64}$/.test(value.task_basis_sha256)||!reviewID(value.requester_user_id)||typeof value.requester_name_at_request!=='string'||!reviewID(value.reviewer_resource_id)||!reviewID(value.reviewer_user_id)||typeof value.reviewer_name_at_request!=='string'||!validReviewText(value.note)||!['pending','cancelled','closed'].includes(String(value.state))||!instant(value.created_at)||!instant(value.updated_at)||!['binding_current','reviewer_available','is_current_assignee','can_cancel','source_available'].every(key=>typeof value[key]==='boolean'))return false;
 if(value.state==='cancelled'){if(!reviewID(value.cancelled_by_user_id)||!validReviewText(value.cancel_reason)||value.can_cancel)return false;}else if(value.cancelled_by_user_id!==''||value.cancel_reason!=='')return false;
 if(value.state==='closed'){if(!reviewID(value.closed_review_id)||value.can_cancel)return false;}else if(value.closed_review_id!==''||value.decision!==undefined)return false;
 if(value.decision!==undefined){const decision=value.decision;if(!value.source_available||!object(decision)||decision.review_id!==value.closed_review_id||!reviewID(decision.reviewer_user_id)||!['accepted','rejected'].includes(String(decision.decision))||typeof decision.reason!=='string'||!decision.reason.trim()||!instant(decision.created_at))return false;}
 return true;
}
export function validReviewRequestPage(value:unknown,project:string,task:string):value is TaskReviewRequestPage{
 if(!object(value)||value.project_id!==project||value.task_id!==task||!reviewVersion(value.task_version)||typeof value.can_request!=='boolean'||value.limit!==20||!natural(value.total)||!Array.isArray(value.items)||value.items.length>20||value.items.length>value.total||!cursor(value.next_cursor)||!!value.next_cursor&&!value.items.length||!(value.pending_request_id===''||reviewID(value.pending_request_id)))return false;
 const seen=new Set<string>();let previous:TaskReviewRequest|undefined;
 for(const row of value.items){if(!validReviewRequest(row,project,task)||seen.has(row.id)||previous&&(previous.created_at<row.created_at||previous.created_at===row.created_at&&previous.id<=row.id))return false;seen.add(row.id);previous=row;}
 return true;
}
export function appendReviewRequests(previous:TaskReviewRequestPage,next:TaskReviewRequestPage):TaskReviewRequestPage|undefined{
 const last=previous.items.at(-1),first=next.items[0];
 if(previous.project_id!==next.project_id||previous.task_id!==next.task_id||last&&first&&(last.created_at<first.created_at||last.created_at===first.created_at&&last.id<=first.id))return;
 const items=[...previous.items,...next.items];if(new Set(items.map(item=>item.id)).size!==items.length||items.length>next.total||next.next_cursor&&next.next_cursor===previous.next_cursor)return;
 return {...next,items};
}
export function validReviewRequestDetail(value:unknown,project:string,task:string,id:string):value is TaskReviewRequestDetail{return object(value)&&value.project_id===project&&value.task_id===task&&typeof value.task_title==='string'&&reviewVersion(value.task_version)&&typeof value.can_request==='boolean'&&validReviewRequest(value.request,project,task)&&value.request.id===id;}
export function validReviewTargets(value:unknown,project:string,task:string,selected=''):value is TaskReviewTargetPage{
 if(!object(value)||value.project_id!==project||value.task_id!==task||value.limit!==20||!Array.isArray(value.items)||value.items.length>20||!natural(value.total)||value.total<value.items.length||!cursor(value.next_cursor)||!!value.next_cursor&&!value.items.length)return false;
 const ids=new Set(),users=new Set();for(const row of value.items){if(!object(row)||!reviewID(row.resource_id)||!reviewID(row.user_id)||typeof row.name!=='string'||!reviewVersion(row.resource_version)||ids.has(row.resource_id)||users.has(row.user_id)||selected&&row.resource_id!==selected)return false;ids.add(row.resource_id);users.add(row.user_id);}
 return !selected||value.total===value.items.length&&value.next_cursor==='';
}
export function validReviewRequestRecovery(request:WriteRecoveryRequest,project:string,task:string){
 const body=request.body,key=request.idempotency_key;
 if(request.method!=='POST'||body.project_id!==project||body.idempotency_key!==key||typeof key!=='string'||!key.trim()||/[\p{Cc}]/u.test(key)||new TextEncoder().encode(key).length<8||new TextEncoder().encode(key).length>128)return false;
 if(request.path==='/task-review-requests')return exact(body,['project_id','task_id','expected_task_version','run_id','expected_run_version','reviewer_resource_id','note','idempotency_key'])&&body.task_id===task&&reviewVersion(body.expected_task_version)&&body.expected_task_version<2147483647&&reviewVersion(body.expected_run_version)&&body.expected_run_version<2147483647&&reviewRunID(body.run_id)&&reviewID(body.reviewer_resource_id)&&validReviewText(body.note);
 return /^\/task-review-requests\/[A-Za-z0-9_.-]{1,64}\/commands$/.test(request.path)&&reviewID(request.path.split('/')[2])&&exact(body,['project_id','expected_version','action','reason','idempotency_key'])&&reviewVersion(body.expected_version)&&body.expected_version<2147483647&&body.action==='cancel'&&validReviewText(body.reason);
}
export function validReviewRequestReceipt(value:unknown,request:WriteRecoveryRequest,project:string,task:string,user:string){
 if(!validReviewRequest(value,project,task))return false;const body=request.body;
 return request.path==='/task-review-requests'?value.task_version===body.expected_task_version&&value.run_id===body.run_id&&value.run_version===body.expected_run_version&&value.requester_user_id===user&&value.reviewer_resource_id===body.reviewer_resource_id&&value.note===body.note:value.id===request.path.split('/')[2]&&value.state==='cancelled'&&value.cancelled_by_user_id===user&&value.cancel_reason===body.reason;
}
export type ReviewBasis={task_version:number;run_id:string;run_version:number};
export type ReviewDraft={reviewer_resource_id:string;note:string;basis?:ReviewBasis;cancel?:{id:string;version:number;reason:string}};
type DraftStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export const reviewDraftKey=(user:string,organization:string,project:string,task:string)=>'suma-task-review-draft-v1:'+JSON.stringify([user,organization,project,task]);
export const emptyReviewDraft=():ReviewDraft=>({reviewer_resource_id:'',note:''});
export function readReviewDraft(storage:DraftStorage|undefined,key:string):{draft:ReviewDraft;invalid?:boolean}{try{const raw=storage?.getItem(key);if(!raw)return {draft:emptyReviewDraft()};const value:unknown=JSON.parse(raw);if(!object(value)||value.format!=='suma-task-review-draft-v1'||!(value.reviewer_resource_id===''||reviewID(value.reviewer_resource_id))||typeof value.note!=='string'||value.note.length>4000||/[\0\uD800-\uDFFF]/u.test(value.note)||Object.keys(value).some(key=>!['format','reviewer_resource_id','note','basis','cancel'].includes(key)))return {draft:emptyReviewDraft(),invalid:true};if(value.basis!==undefined&&(!object(value.basis)||!reviewVersion(value.basis.task_version)||!reviewRunID(value.basis.run_id)||!reviewVersion(value.basis.run_version)))return {draft:emptyReviewDraft(),invalid:true};if(value.cancel!==undefined&&(!object(value.cancel)||!reviewID(value.cancel.id)||!reviewVersion(value.cancel.version)||typeof value.cancel.reason!=='string'||value.cancel.reason.length>4000||/[\0\uD800-\uDFFF]/u.test(value.cancel.reason)))return {draft:emptyReviewDraft(),invalid:true};return {draft:{reviewer_resource_id:value.reviewer_resource_id,note:value.note,...(value.basis?{basis:value.basis as ReviewBasis}:{}),...(value.cancel?{cancel:value.cancel as ReviewDraft['cancel']}:{})}};}catch{return {draft:emptyReviewDraft(),invalid:true};}}
export function saveReviewDraft(storage:DraftStorage|undefined,key:string,draft:ReviewDraft){if(!storage)throw new Error('draft storage unavailable');if(!draft.reviewer_resource_id&&!draft.note&&!draft.cancel)storage.removeItem(key);else storage.setItem(key,JSON.stringify({format:'suma-task-review-draft-v1',...draft}));}
