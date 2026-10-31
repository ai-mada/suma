import type {ACPExecutionCapability,ACPExecutionEvidence,ToolPermission,ToolPermissionOption} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';
export type ACPPermissionScope={project:string;runID:string;nodeID:string;attempt:number;fence:number;userID:string;organizationID:string};
export type ACPPermissionIntent={scope:ACPPermissionScope;permission_id:string;profile_sha256:string;dispatch_id:string;worker_id:string;option_kind:'allow_once'|'reject_once';request:WriteRecoveryRequest};
const max=2147483647,permissionMax=Number.MAX_SAFE_INTEGER,encoder=new TextEncoder(),bytes=(v:string)=>encoder.encode(v).length;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const fields=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const integer=(v:unknown,min=0,limit=max):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min&&v<=limit;
const id=(v:unknown,limit=64):v is string=>typeof v==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]+$/.test(v)&&v.length<=limit;
const permissionID=(v:unknown,limit=150):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]*$/.test(v)&&v.length<=limit;
const text=(v:unknown,limit=32768):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=limit&&!/[\p{Cc}]/u.test(v.replace(/[\r\n\t]/g,''));
const token=(v:unknown,limit=128):v is string=>text(v,limit)&&!!v&&v.trim()===v&&!/[\p{Cc}]/u.test(v);
const operation=(v:unknown):v is string=>token(v)&&bytes(v)>=8;
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const timestamp=(v:unknown):v is string=>typeof v==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(v)&&Number.isFinite(Date.parse(v))&&new Date(v).toISOString().slice(0,19)===v.slice(0,19);
const capabilityKeys=['runtime','protocol_version','sdk_version','configuration_sha256','model','model_source','budget_mode','identity_evidence','auth_strategy','permission_mode'] as const;
export function validACPCapability(value:unknown):value is ACPExecutionCapability{
 return object(value)&&fields(value,capabilityKeys)&&value.runtime==='qoder'&&value.protocol_version===1&&value.sdk_version==='0.13.5'&&hash(value.configuration_sha256)&&token(value.model,256)&&value.model_source==='administrator_session_config'&&value.budget_mode==='observe_only'&&value.identity_evidence==='configuration_ack_only'&&value.auth_strategy==='existing_cli_login'&&value.permission_mode==='human_once';
}
export function validACPEvidence(value:unknown):value is ACPExecutionEvidence{
 const required=['configuration_sha256','requested_model','model_selection_acknowledged','prompt_started','prompt_completed','tools_complete','permissions_requested','permissions_denied','cancel_requested','host_process_stopped','host_process_tree_stopped'];
 if(!object(value)||!fields(value,required,['selected_model','session_id','protocol_version','agent_name','agent_version','stop_reason'])||!hash(value.configuration_sha256)||!token(value.requested_model,256)||!['model_selection_acknowledged','prompt_started','prompt_completed','tools_complete','cancel_requested','host_process_stopped','host_process_tree_stopped'].every(k=>typeof value[k]==='boolean')||!integer(value.permissions_requested,0,32)||!integer(value.permissions_denied,0,value.permissions_requested))return false;
 if(['selected_model','session_id','agent_name','agent_version'].some(k=>Object.hasOwn(value,k)&&!token(value[k],256))||Object.hasOwn(value,'protocol_version')&&value.protocol_version!==1)return false;
 if(Object.hasOwn(value,'stop_reason')&&(typeof value.stop_reason!=='string'||!['end_turn','max_tokens','max_turn_requests','refusal','cancelled'].includes(value.stop_reason)))return false;
 if(value.model_selection_acknowledged&&value.selected_model!==value.requested_model||value.prompt_started&&(!value.model_selection_acknowledged||!value.session_id||value.protocol_version!==1)||value.prompt_completed&&(!value.prompt_started||!value.stop_reason)||value.host_process_tree_stopped&&!value.host_process_stopped)return false;
 return true;
}
/** The original workflow and preview must freeze the same administrator
 * capability; no fresh inventory is substituted into historical evidence. */
export function validACPPlan(value:unknown):boolean{
 if(!object(value))return false;
 const nodes=object(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[],matching=nodes.filter(n=>object(n)&&n.id===value.agent_node_id),node=matching[0],config=object(node)&&object(node.config)?node.config:undefined;
 if(value.acp_execution===undefined&&config?.acp_execution===undefined)return true;
 const original=value.acp_execution,fixed=config?.acp_execution;
 if(matching.length!==1||!object(node)||node.type!=='agent'||node.max_retries!==undefined&&node.max_retries!==0||!validACPCapability(original)||!validACPCapability(fixed))return false;
 for(const key of ['model_execution','codex_execution','opencode_execution','octop_execution','claude_execution'])if(Object.hasOwn(value,key)||config&&Object.hasOwn(config,key))return false;
 return capabilityKeys.every(k=>original[k]===fixed[k]);
}
export function validACPDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean{
 if(!validACPPlan(value)||!object(value))return false;
 if(value.acp_execution===undefined)return true;
 return id(value.id)&&value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&!Object.hasOwn(body,'model_execution_selection')&&!Object.hasOwn(body,'codex_execution_selection');
}
const validScope=(scope:ACPPermissionScope)=>object(scope)&&permissionID(scope.project,64)&&!scope.project.includes('/')&&id(scope.runID,100)&&id(scope.nodeID,100)&&permissionID(scope.userID)&&permissionID(scope.organizationID)&&integer(scope.attempt,1,permissionMax)&&integer(scope.fence,1,permissionMax);
export function permissionListPath(scope:ACPPermissionScope):string{
 if(!validScope(scope))throw Error('Invalid permission scope');
 return '/runs/'+encodeURIComponent(scope.runID)+'/nodes/'+encodeURIComponent(scope.nodeID)+'/tool-permissions?'+new URLSearchParams({project_id:scope.project});
}
function permissionRequest(value:unknown):boolean{
 if(!object(value)||!fields(value,['idempotency_key','session_id','tool_call_id','title','kind','details','details_complete','options'])||!operation(value.idempotency_key)||!token(value.session_id,256)||!token(value.tool_call_id)||!text(value.title)||!text(value.kind,128)||!text(value.details)||typeof value.details_complete!=='boolean'||!Array.isArray(value.options)||value.options.length<1||value.options.length>16)return false;
 if(!value.options.every(o=>object(o)&&fields(o,['id','name','kind'])&&token(o.id)&&text(o.name,1024)&&['allow_once','allow_always','reject_once','reject_always'].includes(String(o.kind)))||new Set(value.options.map(o=>o.id)).size!==value.options.length)return false;
 return bytes(JSON.stringify(value))<=65536;
}
export function validPermission(value:unknown,scope:ACPPermissionScope):value is ToolPermission{
 if(!validScope(scope)||!object(value)||!fields(value,['id','project_id','run_id','node_id','attempt','fence','dispatch_id','worker_id','profile_sha256','version','state','request','created_at','expires_at','allowed_actions'],['decision'])||!permissionID(value.id)||value.project_id!==scope.project||value.run_id!==scope.runID||value.node_id!==scope.nodeID||!integer(value.attempt,1,permissionMax)||!integer(value.fence,1,Number.MAX_SAFE_INTEGER)||!permissionID(value.dispatch_id)||!permissionID(value.worker_id)||!hash(value.profile_sha256)||!integer(value.version,1,permissionMax)||!['pending','decided','expired','superseded'].includes(String(value.state))||!permissionRequest(value.request)||!timestamp(value.created_at)||!timestamp(value.expires_at)||Date.parse(value.expires_at)<=Date.parse(value.created_at)||!Array.isArray(value.allowed_actions)||!value.allowed_actions.every(a=>a==='allow_once'||a==='reject_once')||new Set(value.allowed_actions).size!==value.allowed_actions.length)return false;
 if(value.state==='pending')return !Object.hasOwn(value,'decision');
 if(value.allowed_actions.length)return false;
 if(!Object.hasOwn(value,'decision'))return value.state!=='decided';
 const decision=value.decision,request=value.request as ToolPermission['request'];
 return object(decision)&&fields(decision,['option_id','kind','actor_id','decided_at'])&&token(decision.option_id)&&['allow_once','reject_once'].includes(String(decision.kind))&&permissionID(decision.actor_id)&&timestamp(decision.decided_at)&&Date.parse(decision.decided_at)>=Date.parse(value.created_at)&&request.options.some(o=>o.id===decision.option_id&&o.kind===decision.kind);
}
export function validPermissionRecovery(request:unknown,scope:ACPPermissionScope):request is WriteRecoveryRequest{
 if(!validScope(scope)||!object(request)||!fields(request,['path','body','method','idempotency_key'])||typeof request.path!=='string'||request.method!=='POST'||!operation(request.idempotency_key)||!object(request.body))return false;
 const matched=/^\/tool-permissions\/([^/]+)\/decision$/.exec(request.path),body=request.body;
 let permission='';try{permission=matched?decodeURIComponent(matched[1]):'';}catch{return false;}
 return permissionID(permission,64)&&!permission.includes('/')&&request.path==='/tool-permissions/'+encodeURIComponent(permission)+'/decision'&&fields(body,['project_id','expected_version','idempotency_key','option_id'])&&body.project_id===scope.project&&body.idempotency_key===request.idempotency_key&&integer(body.expected_version,1,permissionMax-1)&&token(body.option_id);
}
export function validPermissionReceipt(value:unknown,request:WriteRecoveryRequest,scope:ACPPermissionScope):value is ToolPermission{
 return validPermissionRecovery(request,scope)&&validPermission(value,scope)&&request.path==='/tool-permissions/'+encodeURIComponent(value.id)+'/decision'&&value.attempt===scope.attempt&&value.fence===scope.fence&&value.version===(request.body.expected_version as number)+1&&value.state==='decided'&&value.decision?.actor_id===scope.userID&&value.decision.option_id===request.body.option_id;
}
export function canChoosePermission(permission:ToolPermission,option:ToolPermissionOption|undefined,scope?:ACPPermissionScope):boolean{
 if(!option)return false;
 if(scope&&(!validPermission(permission,scope)||permission.attempt!==scope.attempt||permission.fence!==scope.fence))return false;
 return permission.state==='pending'&&permission.version<permissionMax&&timestamp(permission.expires_at)&&Date.parse(permission.expires_at)>Date.now()&&permission.allowed_actions.includes(option.kind)&&['allow_once','reject_once'].includes(option.kind)&&(option.kind!=='allow_once'||permission.request.details_complete)&&permission.request.options.some(o=>o.id===option.id&&o.kind===option.kind);
}
/** Recovery belongs to the original caller and node, independently of the
 * currently displayed attempt. The intent retains the original attempt fence. */
export function permissionRecoveryScope(scope:ACPPermissionScope):string{
 return JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.runID,scope.nodeID]);
}
export function validPermissionIntent(value:unknown,currentScope:ACPPermissionScope):value is ACPPermissionIntent{
 if(!validScope(currentScope)||!object(value)||!fields(value,['scope','permission_id','profile_sha256','dispatch_id','worker_id','option_kind','request'])||!object(value.scope)||!fields(value.scope,['project','runID','nodeID','attempt','fence','userID','organizationID']))return false;
 const original=value.scope as ACPPermissionScope;
 return validScope(original)&&permissionRecoveryScope(original)===permissionRecoveryScope(currentScope)&&permissionID(value.permission_id,64)&&!value.permission_id.includes('/')&&hash(value.profile_sha256)&&permissionID(value.dispatch_id)&&permissionID(value.worker_id)&&(value.option_kind==='allow_once'||value.option_kind==='reject_once')&&validPermissionRecovery(value.request,original)&&value.request.path==='/tool-permissions/'+encodeURIComponent(value.permission_id)+'/decision';
}
export function permissionMatchesIntent(permission:unknown,intent:ACPPermissionIntent):boolean{
 return validPermissionIntent(intent,intent.scope)&&validPermission(permission,intent.scope)&&permission.id===intent.permission_id&&permission.attempt===intent.scope.attempt&&permission.fence===intent.scope.fence&&permission.profile_sha256===intent.profile_sha256&&permission.dispatch_id===intent.dispatch_id&&permission.worker_id===intent.worker_id&&permission.request.options.some(option=>option.id===intent.request.body.option_id&&option.kind===intent.option_kind);
}
