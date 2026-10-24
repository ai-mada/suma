import {getLocale,languageHeaders,tr,formatDateTime,UiError} from "./i18n";
import type {ToolEvidence} from './tool-model';
export type RecordItem = {id:string;project_id?:string;version:number;title?:string;name?:string;description?:string;status?:string;priority?:string;owner_id?:string;requirement_id?:string;depends_on?:string[];acceptance_criteria?:string;created_at?:string;updated_at?:string;[key:string]:any};
export type Session={user:{id:string;name:string;email:string;role?:string};organization:{id:string;name:string};projects:RecordItem[]};
export type WNode={id:string;name:string;type:string;command?:string[];depends_on?:string[];join?:string;quorum?:number;condition?:string;branches?:Record<string,string>;timeout_seconds?:number;max_retries?:number;priority?:number;idempotent?:boolean;resources?:Record<string,number>;config?:Record<string,unknown>};
export type Workflow={id:string;name:string;version:number;nodes:WNode[];edges:{from:string;to:string;condition?:string}[]};
export type WorkspaceEvidence={candidate_from?:string;candidate_attempt?:number;candidate_fence?:number;candidate_evidence_sha256?:string;admin_base_commit?:string;base_commit?:string;head_commit?:string;diff_sha256?:string;evidence_sha256?:string;changed_paths?:string[];changed_paths_total?:number;changed_paths_truncated?:boolean;[key:string]:unknown};
export type Run={id:string;workflow_id:string;workflow_name:string;workflow_version:number;version:number;state:string;nodes:(WNode & {state:string;attempt:number;fence?:number;compensatable?:boolean;workspace_evidence?:WorkspaceEvidence;child_run_id?:string;reason?:string;output?:string;error?:string;worker_id?:string;tool_profile_id?:string;tool_profile_sha256?:string;tool_evidence?:ToolEvidence})[];created_at:string;updated_at:string;[key:string]:any};
export class ApiError extends Error{
 constructor(public status:number,public code:string,message:string,public details?:unknown,public presentation?:Record<string,unknown>,public localDiagnostic?:UiError){
  super(message);
  // Presentation metadata cannot replace transport status, error identity or
  // a locally-created diagnostic. In particular, do not assign __proto__.
  for(const key of ['message_key','message_params','locale','localized','fallback'])if(presentation&&Object.hasOwn(presentation,key))Object.defineProperty(this,key,{value:presentation[key],enumerable:true,configurable:true});
 }
}
function localApiError(status:number,code:string,key:string,params?:Record<string,unknown>){const diagnostic=new UiError(key,params);return new ApiError(status,code,diagnostic.message,undefined,undefined,diagnostic);}
/** Raw evidence reads share transport/errors but bypass business-record normalization. */
export async function rawApi<T=unknown>(path:string,signal?:AbortSignal):Promise<T>{return request<T>(path,undefined,undefined,undefined,signal,false);}
export async function api<T=any>(path:string,body?:unknown,method?:string,headers?:Record<string,string>,signal?:AbortSignal):Promise<T>{return request<T>(path,body,method,headers,signal,true);}
async function request<T>(path:string,body:unknown,method:string|undefined,headers:Record<string,string>|undefined,signal:AbortSignal|undefined,normalize:boolean):Promise<T>{
 const cancelled=()=>new DOMException('Aborted','AbortError');
 if(signal?.aborted)throw cancelled();
 const controller=new AbortController(),cancel=()=>controller.abort(),timeout=setTimeout(()=>controller.abort(),30000);
 signal?.addEventListener('abort',cancel,{once:true});
 try{
  let response:Response;
  try{response=await fetch('/api/v1'+path,{credentials:'same-origin',signal:controller.signal,headers:{'Content-Type':'application/json',...languageHeaders(),...headers},method:method??(body?'POST':'GET'),...(body?{body:JSON.stringify(body)}:{})});}
  catch{if(signal?.aborted)throw cancelled();throw localApiError(0,controller.signal.aborted?'request_timeout':'network_error',controller.signal.aborted?'common.timeout':'common.network');}
  // Headers already establish a refusal. An unreadable body cannot turn it
  // into an unknown write result or suppress authorization-loss clearing.
  const incomplete=()=>response.ok
   ?localApiError(0,'request_timeout','common.timeout')
   :localApiError(response.status,'request_failed','common.httpFailure',{status:response.status});
  if(signal?.aborted)throw cancelled();
  if(controller.signal.aborted)throw incomplete();
  let data:any;
  try{data=await response.json();}catch{
   if(signal?.aborted)throw cancelled();
   if(!response.ok||controller.signal.aborted)throw incomplete();
   if(response.status===204)return {} as T;
   throw localApiError(0,'response_unknown','common.responseUnknown');
  }
  if(signal?.aborted)throw cancelled();
  if(!response.ok){
   const error=data?.error,code=typeof error?.code==='string'?error.code:'request_failed';
   const diagnostic=typeof error?.message==='string'?undefined:new UiError('common.httpFailed',{status:response.status});
   throw new ApiError(response.status,code,diagnostic?.message??error.message,error?.details,error,diagnostic);
  }
  if(controller.signal.aborted)throw incomplete();
  return (normalize?normalizeResponse(data):data) as T;
 }finally{clearTimeout(timeout);signal?.removeEventListener('abort',cancel);}
}
export function normalizeResponse(value:any):any{if(Array.isArray(value))return value.map(normalizeResponse);if(value&&typeof value==='object'){const out=Object.fromEntries(Object.entries(value).map(([key,item])=>[key,normalizeResponse(item)]));if(typeof out.version==='number'&&'title' in out&&out.data){return {...out.data,...out,acceptance_criteria:Array.isArray(out.acceptance_criteria)?out.acceptance_criteria.join('\n'):out.acceptance_criteria};}return out;}return value;}
export function recordPayload(kind:string,values:Record<string,unknown>):Record<string,unknown>{const body={...values};if('acceptance_criteria'in body)body.acceptance_criteria=String(body.acceptance_criteria||'').split('\n').map(s=>s.trim()).filter(Boolean);if(kind==='tasks')body.depends_on=String(values.depends_on||'').split(',').map(s=>s.trim()).filter(Boolean);const data:Record<string,unknown>={};for(const key of ['role','email','user_id','content','task_id','result','source_ref','release_version','task_ids']){if(key in body){data[key]=key==='task_ids'?String(body[key]||'').split(',').map(s=>s.trim()).filter(Boolean):body[key];delete body[key];}}if(Object.keys(data).length)body.data=data;return body;}
export const operation=()=>crypto.randomUUID();
export const query=(project:string)=>'?project_id='+encodeURIComponent(project);
export const list=<T=RecordItem>(path:string,project:string)=>api<{items:T[];total:number;next_cursor?:string}>(path+query(project));
export const label=(r:RecordItem)=>r.title||r.name||r.id;
export const statusNames:Record<string,string>={get draft(){return tr("status.draft");},get open(){return tr("status.open");},get pending(){return tr("status.pending");},get ready(){return tr("status.ready");},get running(){return tr("status.running");},get paused(){return tr("status.paused");},get cancelling(){return tr("status.cancelling");},get waiting(){return tr("status.waiting");},get reconciling(){return tr("status.reconciling");},get succeeded(){return tr("status.succeeded");},get failed(){return tr("status.failed");},get skipped(){return tr("status.skipped");},get cancelled(){return tr("status.cancelled");},get todo(){return tr("status.todo");},get in_progress(){return tr("status.in_progress");},get in_review(){return tr("status.in_review");},get accepted(){return tr("status.accepted");},get done(){return tr("status.done");},get active(){return tr("status.active");},get resolved(){return tr("status.resolved");},get published(){return tr("status.published");},get blocked(){return tr("status.blocked");},get planned(){return tr("status.planned");},get completed(){return tr("status.completed");},get archived(){return tr("status.archived");},get removed(){return tr("status.removed");},get revoked(){return tr("status.revoked");},get expired(){return tr("status.expired");}};
export const stateLabel=(s?:string)=>s&&s in statusNames?tr('status.'+s):s||tr('status.unset');
export const time=(s?:string)=>s?formatDateTime(s,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):'—';
