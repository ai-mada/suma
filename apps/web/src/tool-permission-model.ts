import type {ACPToolPermissionSource,ACPToolPermissionChoice,CodexApprovalRequest,CodexApprovalChoice,ToolPermission} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';

export type ToolPermissionScope={project:string;runID:string;nodeID:string;attempt:number;fence:number;userID:string;organizationID:string;protocol:'acp-stdio-v1'|'codex-app-server-v1'};
export type ToolPermissionChoice=ACPToolPermissionChoice|CodexApprovalChoice;
type SourceIdentity={protocol:'acp-stdio-v1';idempotency_key:string;acp:Pick<ACPToolPermissionSource,'session_id'|'tool_call_id'>}|{protocol:'codex-app-server-v1';idempotency_key:string;codex:Pick<CodexApprovalRequest,'request_id'|'method'|'thread_id'|'turn_id'|'item_id'|'started_at_ms'|'approval_id'|'binding_sha256'|'params_sha256'|'item_sha256'>};
export type ToolPermissionIntent={scope:ToolPermissionScope;permission_id:string;profile_sha256:string;dispatch_id:string;worker_id:string;source_identity:SourceIdentity;native_choice:ToolPermissionChoice;request:WriteRecoveryRequest};

const maximum=Number.MAX_SAFE_INTEGER,encoder=new TextEncoder();
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const fields=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const integer=(v:unknown,min=1,max=maximum):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min&&v<=max;
const text=(v:unknown,limit:number):v is string=>typeof v==='string'&&wellFormed(v)&&encoder.encode(v).length<=limit&&!/[\p{Cc}]/u.test(v.replace(/[\r\n\t]/g,''));
const token=(v:unknown,limit=128):v is string=>text(v,limit)&&!!v&&v.trim()===v&&!/[\p{Cc}]/u.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const routeID=(v:unknown,limit=64):v is string=>typeof v==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]+$/.test(v)&&v.length<=limit;
const permissionID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,149}$/.test(v);
const operation=(v:unknown):v is string=>token(v)&&encoder.encode(v).length>=8;
const protocol=(v:unknown):v is ToolPermissionScope['protocol']=>v==='acp-stdio-v1'||v==='codex-app-server-v1';
const codexID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(v);
const codexText=(v:unknown,limit:number,required=false,multiline=false):v is string=>text(v,limit)&&(!required||!!v.trim())&&(multiline||!/[\p{Cc}]/u.test(v));
const codexMethods=['item/commandExecution/requestApproval','item/fileChange/requestApproval'];
const identityKeys=['request_id','method','thread_id','turn_id','item_id','started_at_ms','binding_sha256','params_sha256','item_sha256'];
function rpcID(v:unknown):boolean{
 if(!object(v)||!fields(v,['kind','value']))return false;
 if(v.kind==='string')return codexText(v.value,256,true);
 if(v.kind!=='number'||typeof v.value!=='string'||!/^(?:0|-[1-9]\d*|[1-9]\d*)$/.test(v.value)||v.value.length>20)return false;
 const n=BigInt(v.value);return n>=-9223372036854775808n&&n<=9223372036854775807n;
}
function validCodexIdentity(value:Record<string,unknown>):boolean{
 return rpcID(value.request_id)&&typeof value.method==='string'&&codexMethods.includes(value.method)&&['thread_id','turn_id','item_id'].every(k=>codexID(value[k]))&&integer(value.started_at_ms)&&['binding_sha256','params_sha256','item_sha256'].every(k=>hash(value[k]))&&(!Object.hasOwn(value,'approval_id')||codexText(value.approval_id,256));
}
function relativePath(v:unknown,root=false):v is string{
 if(root&&v==='.')return true;
 return codexText(v,2048,true)&&!/[\\:]/.test(v)&&!v.startsWith('/')&&v.split('/').every(p=>p!==''&&p!=='.'&&p!=='..');
}
function validCodexSource(value:unknown):value is CodexApprovalRequest{
 if(!object(value)||!fields(value,[...identityKeys,'title','details','details_complete','scope_supported'],['approval_id','unsupported_reason','command','cwd','changes','reason'])||!validCodexIdentity(value)||!codexText(value.title,4096,true,true)||!codexText(value.details,32768,false,true)||typeof value.details_complete!=='boolean'||typeof value.scope_supported!=='boolean')return false;
 if(Object.hasOwn(value,'unsupported_reason')&&value.unsupported_reason!==''&&!codexID(value.unsupported_reason)||!value.scope_supported&&!value.unsupported_reason)return false;
 if(Object.hasOwn(value,'command')&&!codexText(value.command,32768,false,true)||Object.hasOwn(value,'reason')&&!codexText(value.reason,4096,false,true)||Object.hasOwn(value,'cwd')&&value.cwd!==''&&!relativePath(value.cwd,true))return false;
 const changes=Object.hasOwn(value,'changes')?value.changes:[];
 if(!Array.isArray(changes)||changes.length>128)return false;
 const seen=new Set<string>();
 for(const change of changes){
  if(!object(change)||!fields(change,['path','kind','diff'],['move_path'])||!relativePath(change.path)||seen.has(change.path)||typeof change.kind!=='string'||!['add','delete','update'].includes(change.kind)||!codexText(change.diff,32768,false,true)||Object.hasOwn(change,'move_path')&&change.move_path!==''&&(!relativePath(change.move_path)||change.kind!=='update'))return false;
  seen.add(change.path);
 }
 if(value.method===codexMethods[0]&&changes.length!==0||value.method===codexMethods[1]&&!!value.command)return false;
 if(value.scope_supported&&value.details_complete&&(value.method===codexMethods[0]&&(!value.command||!(value.command as string).trim()||!value.cwd)||value.method===codexMethods[1]&&changes.length===0))return false;
 return true;
}
function validObservation(value:unknown):boolean{
 if(!object(value)||!fields(value,['sequence','params_sha256','decision_observed','reply_started','reply_written','request_resolved'],['decision','item_status','failure_code'])||!integer(value.sequence)||!hash(value.params_sha256)||!['decision_observed','reply_started','reply_written','request_resolved'].every(k=>typeof value[k]==='boolean'))return false;
 if(Object.hasOwn(value,'decision')&&(typeof value.decision!=='string'||value.decision!==''&&!['accept','decline','cancel'].includes(value.decision))||value.decision_observed!==!!value.decision||value.reply_started&&!value.decision_observed||value.reply_written&&!value.reply_started)return false;
 return (!Object.hasOwn(value,'item_status')||typeof value.item_status==='string'&&['','inProgress','completed','failed','declined'].includes(value.item_status))&&(!Object.hasOwn(value,'failure_code')||value.failure_code===''||codexID(value.failure_code));
}
function boundedJSON(value:unknown,limit:number):boolean{try{return encoder.encode(JSON.stringify(value)).length<=limit;}catch{return false;}}
function validACPSource(value:unknown):value is ACPToolPermissionSource{
 if(!object(value)||!fields(value,['session_id','tool_call_id','title','kind','details','details_complete','options'])||!token(value.session_id)||!token(value.tool_call_id)||!text(value.title,4096)||!value.title.trim()||!token(value.kind)||!text(value.details,32768)||typeof value.details_complete!=='boolean'||!Array.isArray(value.options)||value.options.length<1||value.options.length>16)return false;
 const seen=new Set<string>();
 return value.options.every(option=>{
  if(!object(option)||!fields(option,['id','name','kind'])||!token(option.id)||!text(option.name,512)||!option.name.trim()||/[\p{Cc}]/u.test(option.name)||typeof option.kind!=='string'||!['allow_once','reject_once','allow_always','reject_always'].includes(option.kind)||seen.has(option.id))return false;
  seen.add(option.id);return true;
 });
}
function validScope(v:unknown):v is ToolPermissionScope{return object(v)&&fields(v,['project','runID','nodeID','attempt','fence','userID','organizationID','protocol'])&&['project','userID','organizationID'].every(k=>routeID(v[k]))&&routeID(v.runID,100)&&routeID(v.nodeID,100)&&integer(v.attempt)&&integer(v.fence)&&protocol(v.protocol);}
export function permissionListPath(scope:ToolPermissionScope):string{
 if(!validScope(scope))throw Error('Invalid tool permission scope');
 return `/runs/${encodeURIComponent(scope.runID)}/nodes/${encodeURIComponent(scope.nodeID)}/tool-permissions?project_id=${encodeURIComponent(scope.project)}`;
}
function timestamp(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value))return false;
 const time=new Date(value);return Number.isFinite(time.valueOf())&&time.toISOString().slice(0,19)===value.slice(0,19);
}
export function validPermission(value:unknown,scope:ToolPermissionScope):value is ToolPermission{
 if(!validScope(scope)||!object(value)||!fields(value,['id','project_id','run_id','node_id','attempt','fence','dispatch_id','worker_id','profile_sha256','version','state','request','created_at','expires_at','allowed_actions'],['decision','observation'])||!['id','project_id','run_id','node_id','dispatch_id','worker_id'].every(k=>permissionID(value[k]))||value.project_id!==scope.project||value.run_id!==scope.runID||value.node_id!==scope.nodeID||!integer(value.attempt)||!integer(value.fence)||!integer(value.version)||!hash(value.profile_sha256)||typeof value.state!=='string'||!['pending','decided','expired','superseded'].includes(value.state)||!timestamp(value.created_at)||!timestamp(value.expires_at)||Date.parse(value.expires_at)<=Date.parse(value.created_at)||!object(value.request))return false;
 const request=value.request,native=scope.protocol,key=native==='acp-stdio-v1'?'acp':'codex';
 if(!fields(request,['idempotency_key','protocol',key])||request.protocol!==native||!operation(request.idempotency_key)||!boundedJSON(request,65536)||(native==='acp-stdio-v1'?!validACPSource(request.acp):!validCodexSource(request.codex)))return false;
 const decision=value.decision;
 if(value.state==='decided'&&!Object.hasOwn(value,'decision')||value.state==='pending'&&Object.hasOwn(value,'decision'))return false;
 if(Object.hasOwn(value,'decision')&&(!object(decision)||!branch(decision,native,['protocol','actor_id','decided_at'])||!permissionID(decision.actor_id)||!timestamp(decision.decided_at)||!choiceMatches(request,choiceOf(decision,native))))return false;
 let terminalObservation=false;
 if(Object.hasOwn(value,'observation')){
  const observation=value.observation;
  if(native!=='codex-app-server-v1'||!validObservation(observation)||!object(observation)||observation.params_sha256!==(request.codex as CodexApprovalRequest).params_sha256||observation.decision_observed&&(!object(decision)||!object(decision.codex)||decision.codex.decision!==observation.decision))return false;
  terminalObservation=observation.request_resolved===true||!!observation.item_status&&observation.item_status!=='inProgress';
 }
 const order=native==='acp-stdio-v1'?['allow_once','reject_once']:['accept','decline','cancel'];let last=-1;
 if(!Array.isArray(value.allowed_actions))return false;
 for(const action of value.allowed_actions){
  const index=typeof action==='string'?order.indexOf(action):-1;
  if(index<=last||value.state!=='pending'||terminalObservation)return false;
  if(native==='acp-stdio-v1'&&!(request.acp as ACPToolPermissionSource).options.some(option=>option.kind===action))return false;
  if(native==='acp-stdio-v1'&&action==='allow_once'&&!(request.acp as ACPToolPermissionSource).details_complete||native==='codex-app-server-v1'&&action==='accept'&&(!(request.codex as CodexApprovalRequest).details_complete||!(request.codex as CodexApprovalRequest).scope_supported))return false;
  last=index;
 }
 return true;
}
function validChoice(value:unknown,native:ToolPermissionScope['protocol']):value is ToolPermissionChoice{
 if(!object(value))return false;
 return native==='acp-stdio-v1'?fields(value,['option_id','kind'])&&token(value.option_id)&&typeof value.kind==='string'&&['allow_once','reject_once'].includes(value.kind):fields(value,['decision'])&&typeof value.decision==='string'&&['accept','decline','cancel'].includes(value.decision);
}
function branch(value:Record<string,unknown>,native:ToolPermissionScope['protocol'],required:readonly string[]):boolean{
 const key=native==='acp-stdio-v1'?'acp':'codex';
 return value.protocol===native&&fields(value,[...required,key])&&validChoice(value[key],native);
}
function decisionID(path:unknown):string|undefined{
 if(typeof path!=='string')return;
 const match=/^\/tool-permissions\/([^/?#]+)\/decision$/.exec(path);if(!match)return;
 try{const id=decodeURIComponent(match[1]);return permissionID(id)&&id.length<=64&&!id.includes('/')&&encodeURIComponent(id)===match[1]?id:undefined;}catch{return;}
}
export function validPermissionRecovery(request:unknown,scope:ToolPermissionScope):request is WriteRecoveryRequest{
 if(!validScope(scope)||!object(request)||!fields(request,['path','method','body','idempotency_key'])||!decisionID(request.path)||request.method!=='POST'||!operation(request.idempotency_key)||!object(request.body))return false;
 const body=request.body;
 return branch(body,scope.protocol,['project_id','expected_version','idempotency_key','protocol'])&&body.project_id===scope.project&&integer(body.expected_version,1,maximum-1)&&body.idempotency_key===request.idempotency_key;
}
export function validPermissionReceipt(value:unknown,request:WriteRecoveryRequest,scope:ToolPermissionScope):value is ToolPermission{
 return validPermissionRecovery(request,scope)&&validPermission(value,scope)&&value.id===decisionID(request.path)&&value.attempt===scope.attempt&&value.fence===scope.fence&&value.state==='decided'&&value.version===(request.body.expected_version as number)+1&&!!value.decision&&value.decision.actor_id===scope.userID&&same(choiceOf(value.decision as unknown as Record<string,unknown>,scope.protocol),choiceOf(request.body,scope.protocol));
}
export function canChoosePermission(permission:ToolPermission,choice:ToolPermissionChoice|undefined,scope?:ToolPermissionScope):boolean{
 const actualScope=scope??{project:permission.project_id,runID:permission.run_id,nodeID:permission.node_id,attempt:permission.attempt,fence:permission.fence,userID:'view',organizationID:'view',protocol:permission.request.protocol as ToolPermissionScope['protocol']};
 if(!validPermission(permission,actualScope)||permission.attempt!==actualScope.attempt||permission.fence!==actualScope.fence||permission.state!=='pending'||permission.version>=maximum||Date.parse(permission.expires_at)<=Date.now()||!choiceMatches(permission.request,choice))return false;
 const action=choice&&('option_id' in choice?choice.kind:choice.decision);
 return typeof action==='string'&&permission.allowed_actions.includes(action);
}
export function permissionRecoveryScope(scope:ToolPermissionScope):string{
 if(!validScope(scope))throw Error('Invalid tool permission scope');
 return JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.runID,scope.nodeID,scope.protocol]);
}
function same(a:unknown,b:unknown):boolean{
 if(a===b)return true;
 if(!object(a)||!object(b))return false;
 const keys=Object.keys(a);return keys.length===Object.keys(b).length&&keys.every(k=>Object.hasOwn(b,k)&&same(a[k],b[k]));
}
function sourceIdentity(permission:ToolPermission):SourceIdentity{
 const request=permission.request;
 if(request.protocol==='acp-stdio-v1')return {protocol:'acp-stdio-v1',idempotency_key:request.idempotency_key,acp:{session_id:request.acp!.session_id,tool_call_id:request.acp!.tool_call_id}};
 const c=request.codex!;
 return {protocol:'codex-app-server-v1',idempotency_key:request.idempotency_key,codex:{request_id:{...c.request_id},method:c.method,thread_id:c.thread_id,turn_id:c.turn_id,item_id:c.item_id,started_at_ms:c.started_at_ms,...(Object.hasOwn(c,'approval_id')?{approval_id:c.approval_id}:{}),binding_sha256:c.binding_sha256,params_sha256:c.params_sha256,item_sha256:c.item_sha256}};
}
function validSourceIdentity(value:unknown,native:ToolPermissionScope['protocol']):value is SourceIdentity{
 if(!object(value)||value.protocol!==native||!operation(value.idempotency_key))return false;
 if(native==='acp-stdio-v1')return fields(value,['protocol','idempotency_key','acp'])&&object(value.acp)&&fields(value.acp,['session_id','tool_call_id'])&&token(value.acp.session_id)&&token(value.acp.tool_call_id);
 return fields(value,['protocol','idempotency_key','codex'])&&object(value.codex)&&fields(value.codex,identityKeys,['approval_id'])&&validCodexIdentity(value.codex);
}
function choiceOf(value:Record<string,unknown>,native:ToolPermissionScope['protocol']):unknown{return value[native==='acp-stdio-v1'?'acp':'codex'];}
function choiceMatches(request:unknown,choice:unknown):boolean{
 if(!object(request)||!protocol(request.protocol)||!validChoice(choice,request.protocol))return false;
 if(request.protocol==='acp-stdio-v1'){
  if(!validACPSource(request.acp)||!('option_id' in choice))return false;
  const source=request.acp;
  return source.options.some(option=>option.id===choice.option_id&&option.kind===choice.kind)&&(choice.kind!=='allow_once'||source.details_complete);
 }
 return validCodexSource(request.codex)&&'decision' in choice&&(choice.decision!=='accept'||request.codex.details_complete&&request.codex.scope_supported);
}
export function validPermissionIntent(value:unknown,currentScope:ToolPermissionScope):value is ToolPermissionIntent{
 if(!validScope(currentScope)||!object(value)||!fields(value,['scope','permission_id','profile_sha256','dispatch_id','worker_id','source_identity','native_choice','request'])||!validScope(value.scope)||permissionRecoveryScope(value.scope)!==permissionRecoveryScope(currentScope)||!permissionID(value.permission_id)||!hash(value.profile_sha256)||!permissionID(value.dispatch_id)||!permissionID(value.worker_id)||!validSourceIdentity(value.source_identity,currentScope.protocol)||!validPermissionRecovery(value.request,value.scope))return false;
 return value.permission_id===decisionID(value.request.path)&&validChoice(value.native_choice,currentScope.protocol)&&same(value.native_choice,choiceOf(value.request.body,currentScope.protocol));
}
export function permissionMatchesIntent(permission:unknown,intent:ToolPermissionIntent):boolean{
 return validPermissionIntent(intent,intent.scope)&&validPermission(permission,intent.scope)&&permission.id===intent.permission_id&&permission.attempt===intent.scope.attempt&&permission.fence===intent.scope.fence&&permission.profile_sha256===intent.profile_sha256&&permission.dispatch_id===intent.dispatch_id&&permission.worker_id===intent.worker_id&&same(sourceIdentity(permission),intent.source_identity)&&choiceMatches(permission.request,intent.native_choice);
}
export function buildPermissionIntent(permission:ToolPermission,scope:ToolPermissionScope,request:WriteRecoveryRequest):ToolPermissionIntent|undefined{
 if(!validScope(scope)||!validPermission(permission,scope)||permission.attempt!==scope.attempt||permission.fence!==scope.fence||!validPermissionRecovery(request,scope)||decisionID(request.path)!==permission.id||request.body.expected_version!==permission.version)return;
 const choice=choiceOf(request.body,scope.protocol);if(!choiceMatches(permission.request,choice))return;
 const value={scope,permission_id:permission.id,profile_sha256:permission.profile_sha256,dispatch_id:permission.dispatch_id,worker_id:permission.worker_id,source_identity:sourceIdentity(permission),native_choice:choice,request};
 return validPermissionIntent(value,scope)?JSON.parse(JSON.stringify(value)) as ToolPermissionIntent:undefined;
}
