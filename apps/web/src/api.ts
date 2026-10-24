import type {ToolEvidence} from './tool-model';
export type RecordItem = {id:string;project_id?:string;version:number;title?:string;name?:string;description?:string;status?:string;priority?:string;owner_id?:string;requirement_id?:string;depends_on?:string[];acceptance_criteria?:string;created_at?:string;updated_at?:string;[key:string]:any};
export type Session={user:{id:string;name:string;email:string;role?:string};organization:{id:string;name:string};projects:RecordItem[]};
export type WNode={id:string;name:string;type:string;command?:string[];depends_on?:string[];join?:string;quorum?:number;condition?:string;branches?:Record<string,string>;timeout_seconds?:number;max_retries?:number;idempotent?:boolean;resources?:Record<string,number>;config?:Record<string,unknown>};
export type Workflow={id:string;name:string;version:number;nodes:WNode[];edges:{from:string;to:string;condition?:string}[]};
export type WorkspaceEvidence={candidate_from?:string;candidate_attempt?:number;candidate_fence?:number;candidate_evidence_sha256?:string;admin_base_commit?:string;base_commit?:string;head_commit?:string;diff_sha256?:string;evidence_sha256?:string;changed_paths?:string[];changed_paths_total?:number;changed_paths_truncated?:boolean;[key:string]:unknown};
export type Run={id:string;workflow_id:string;workflow_name:string;workflow_version:number;version:number;state:string;nodes:(WNode & {state:string;attempt:number;fence?:number;compensatable?:boolean;workspace_evidence?:WorkspaceEvidence;child_run_id?:string;reason?:string;output?:string;error?:string;worker_id?:string;tool_profile_id?:string;tool_profile_sha256?:string;tool_evidence?:ToolEvidence})[];created_at:string;updated_at:string;[key:string]:any};
export class ApiError extends Error{constructor(public status:number,public code:string,message:string,public details?:unknown){super(message);}}
export async function api<T=any>(path:string,body?:unknown,method?:string,headers?:Record<string,string>):Promise<T>{
 const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),30000);
 try{
  let response:Response;
  try{response=await fetch('/api/v1'+path,{credentials:'same-origin',signal:controller.signal,headers:{'Content-Type':'application/json',...headers},method:method??(body?'POST':'GET'),...(body?{body:JSON.stringify(body)}:{})});}
  catch{throw new ApiError(0,controller.signal.aborted?'request_timeout':'network_error',controller.signal.aborted?'请求等待超时，操作结果尚未确认。请恢复同一操作。':'连接中断，操作结果尚未确认。请恢复连接后重试同一操作。');}
  let data:any;
  try{data=await response.json();}catch{
   if(response.status===204)return {} as T;
   if(response.ok)throw new ApiError(0,'response_unknown','服务已返回成功状态，但回执未完整读取。操作结果尚未确认，请恢复同一操作。');
   throw new ApiError(response.status,'request_failed',`请求失败（${response.status}），错误回执未完整读取。`);
  }
  if(!response.ok)throw new ApiError(response.status,data.error?.code??'request_failed',data.error?.message??`请求失败（${response.status}）`,data.error?.details);
  return normalizeResponse(data) as T;
 }finally{clearTimeout(timeout);}
}
export function normalizeResponse(value:any):any{if(Array.isArray(value))return value.map(normalizeResponse);if(value&&typeof value==='object'){const out=Object.fromEntries(Object.entries(value).map(([key,item])=>[key,normalizeResponse(item)]));if(typeof out.version==='number'&&'title' in out&&out.data){return {...out.data,...out,acceptance_criteria:Array.isArray(out.acceptance_criteria)?out.acceptance_criteria.join('\n'):out.acceptance_criteria};}return out;}return value;}
export function recordPayload(kind:string,values:Record<string,unknown>):Record<string,unknown>{const body={...values};if('acceptance_criteria'in body)body.acceptance_criteria=String(body.acceptance_criteria||'').split('\n').map(s=>s.trim()).filter(Boolean);if(kind==='tasks')body.depends_on=String(values.depends_on||'').split(',').map(s=>s.trim()).filter(Boolean);const data:Record<string,unknown>={};for(const key of ['role','email','user_id','content','task_id','result','source_ref','release_version','task_ids']){if(key in body){data[key]=key==='task_ids'?String(body[key]||'').split(',').map(s=>s.trim()).filter(Boolean):body[key];delete body[key];}}if(Object.keys(data).length)body.data=data;return body;}
export const operation=()=>crypto.randomUUID();
export const query=(project:string)=>'?project_id='+encodeURIComponent(project);
export const list=<T=RecordItem>(path:string,project:string)=>api<{items:T[];total:number;next_cursor?:string}>(path+query(project));
export const label=(r:RecordItem)=>r.title||r.name||r.id;
export const statusNames:Record<string,string>={draft:'草稿',open:'待处理',pending:'待执行',ready:'可执行',running:'进行中',paused:'已暂停',cancelling:'取消中',waiting:'等待审批',reconciling:'结果未知',succeeded:'已成功',failed:'失败',skipped:'已跳过',cancelled:'已取消',todo:'待开始',in_progress:'进行中',in_review:'待验收',accepted:'已验收',done:'已完成',active:'已启用',resolved:'已解决',published:'已发布',blocked:'受阻',planned:'计划中',completed:'已完成',archived:'已归档',removed:'已移除',revoked:'已撤销',expired:'已过期'};
export const stateLabel=(s?:string)=>statusNames[s??'']||s||'未设置';
export const time=(s?:string)=>s?new Date(s).toLocaleString('zh-CN',{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):'—';
