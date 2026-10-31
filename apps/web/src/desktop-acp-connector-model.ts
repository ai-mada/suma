import type {ACPModelDiscoveryResult} from '../../../packages/contracts/src';
import type {DesktopToolScope} from './desktop-tool-connector-model';
import {validQuickACPCatalogue} from './acp-quick-connect-selection-model';
import {wellFormed} from './i18n/catalog';

export type DesktopACPScope=DesktopToolScope;
export type DesktopACPSelection={installation_id:string;probe_id:string;model_id:string};
type Base={project_id:string;user_id:string;tenant_id:string;adapter_id:'qoder-acp';locale:'zh-CN'|'en-US'};
export type DesktopACPRequest=Base&({action:'detect'}|{action:'models';installation_id:string;operation_id:string;credential_env_ref?:string}|{action:'status';operation_id?:string});
export type DesktopACPInstallation={installation_id:string;label:string;tool_version?:string;executable_sha256?:string;kind:'native'|'wrapper_only'|'unverified'};
export type DesktopACPProbe={id:string;operation_id:string;installation_id:string;source_sha256:string;created_at:string;expires_at:string;discovery?:ACPModelDiscoveryResult};
export type DesktopACPPhase='unconfigured'|'detected'|'not_installed'|'wrapper_only'|'unavailable'|'needs_workspace'|'authorization_required'|'binding_changed'|'reading'|'ready'|'failed'|'unknown'|'expired'|'stop_unconfirmed';
export type DesktopACPObservation={project_id:string;user_id:string;tenant_id:string;adapter_id:'qoder-acp';phase:DesktopACPPhase;installations:DesktopACPInstallation[];probe?:DesktopACPProbe;error_code?:string};
export type DesktopACPResult={status:'ok';observation:DesktopACPObservation}|{status:'failed';code:string};
export type DesktopACPBridge={platform:'win32'|'darwin'|'linux';version:'1.0.0';inspectLocalACP:(request:DesktopACPRequest)=>Promise<unknown>};
export type DesktopACPRequestOptions={locale:'zh-CN'|'en-US';installation_id?:string;operation_id?:string;credential_env_ref?:string};
const encoder=new TextEncoder();
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const closed=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const id=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{32}$/.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&wellFormed(v)&&encoder.encode(v).length<=max&&!/\p{Cc}/u.test(v);
const token=(v:unknown,max:number):v is string=>text(v,max)&&!!v&&v.trim()===v;
const code=(v:unknown):v is string=>typeof v==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(v);
const phases=new Set<DesktopACPPhase>(['unconfigured','detected','not_installed','wrapper_only','unavailable','needs_workspace','authorization_required','binding_changed','reading','ready','failed','unknown','expired','stop_unconfirmed']);
const reservedPrefixes=['SUMA_','CODEX','OPENAI','ANTHROPIC','CLAUDE','AZURE_','GEMINI','GOOGLE_','QWEN','DASHSCOPE','DEEPSEEK','OPENCODE','DSH_','WORKBUDDY','CODEBUDDY','AWS_','HUGGINGFACE','HF_','GIT_','NODE_','PYTHON'];
const reserved=new Set(['HOME','USERPROFILE','PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','TMPDIR','COMSPEC','SYSTEMROOT','WINDIR','PATHEXT','HOMEDRIVE','HOMEPATH','SYSTEMDRIVE','PROGRAMDATA','PROGRAMFILES','PUBLIC','LANG','GOFLAGS','GOCACHE','GOTMPDIR']);
function credential(v:unknown):v is string{return typeof v==='string'&&/^[A-Z_][A-Z0-9_]{0,127}$/.test(v)&&!reservedPrefixes.some(prefix=>v.startsWith(prefix))&&!reserved.has(v);}
function instant(v:unknown):bigint|undefined{
 if(typeof v!=='string')return;
 const match=/^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.(\d{1,9}))?Z$/.exec(v);if(!match)return;
 const milliseconds=Date.parse(match[1]+'Z');if(!Number.isFinite(milliseconds)||new Date(milliseconds).toISOString().slice(0,19)!==match[1])return;
 return BigInt(milliseconds)*1000000n+BigInt((match[2]||'').padEnd(9,'0'));
}
export function validDesktopACPScope(scope:DesktopACPScope):boolean{return object(scope)&&id(scope.project)&&id(scope.userID)&&id(scope.organizationID)&&scope.adapterID==='qoder-acp';}
export function desktopACPRecoveryScope(scope:DesktopACPScope):string{return 'desktop-acp:'+JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.adapterID]);}
export function desktopACPBridge(value:unknown=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop):DesktopACPBridge|undefined{
 return object(value)&&['win32','darwin','linux'].includes(String(value.platform))&&value.version==='1.0.0'&&typeof value.inspectLocalACP==='function'?value as DesktopACPBridge:undefined;
}
export function validDesktopACPSelection(value:unknown):value is DesktopACPSelection{
 return object(value)&&closed(value,['installation_id','probe_id','model_id'])&&id(value.installation_id)&&id(value.probe_id)&&token(value.model_id,256);
}
export function validDesktopACPRequest(value:unknown,scope:DesktopACPScope):value is DesktopACPRequest{
 if(!validDesktopACPScope(scope)||!object(value)||value.project_id!==scope.project||value.user_id!==scope.userID||value.tenant_id!==scope.organizationID||value.adapter_id!=='qoder-acp'||!['zh-CN','en-US'].includes(String(value.locale)))return false;
 const base=['project_id','user_id','tenant_id','adapter_id','locale','action'];
 if(value.action==='detect')return closed(value,base);
 if(value.action==='status')return closed(value,base,['operation_id'])&&(!Object.hasOwn(value,'operation_id')||id(value.operation_id));
 return value.action==='models'&&closed(value,[...base,'installation_id','operation_id'],['credential_env_ref'])&&id(value.installation_id)&&id(value.operation_id)&&(!Object.hasOwn(value,'credential_env_ref')||credential(value.credential_env_ref));
}
export function makeDesktopACPRequest(action:DesktopACPRequest['action'],scope:DesktopACPScope,options:DesktopACPRequestOptions):DesktopACPRequest{
 const {locale,...selection}=options;
 const request={project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:scope.adapterID,action,locale,...selection};
 if(!validDesktopACPRequest(request,scope))throw Error('Invalid desktop ACP request');return request;
}
function validDiscovery(value:unknown):value is ACPModelDiscoveryResult{
 if(!object(value)||!closed(value,['protocol','runtime','state','executable_sha256','tool_version','evidence'],['agent_name','agent_version','catalogue','error_code'])||value.protocol!=='suma-acp-model-discovery-v1'||value.runtime!=='qoder'||!hash(value.executable_sha256)||!token(value.tool_version,256)||!object(value.evidence))return false;
 for(const name of ['agent_name','agent_version'])if(Object.hasOwn(value,name)&&!text(value[name],256))return false;
 const e=value.evidence,flags=['session_created','session_close_requested','session_close_acknowledged','host_process_stopped','host_process_tree_stopped'],counts=['prompt_requests','configuration_writes','permission_requests','unexpected_activity'];
 if(!closed(e,[...flags,...counts])||!flags.every(k=>typeof e[k]==='boolean')||!counts.every(k=>typeof e[k]==='number'&&Number.isSafeInteger(e[k])&&(e[k] as number)>=0&&(e[k] as number)<=(k==='permission_requests'?32:4096))||e.host_process_tree_stopped&&!e.host_process_stopped||e.session_close_acknowledged&&!e.session_close_requested||e.session_close_requested&&!e.session_created)return false;
 if(value.state==='ready'){
  const c=value.catalogue;
  return !Object.hasOwn(value,'error_code')&&validQuickACPCatalogue(c)&&Object.hasOwn(c,'current_value')&&token(c.config_id,128)&&c.models.length>0&&c.models.every(m=>token(m.name,4096))&&(c.current_value===''||token(c.current_value,256))&&e.session_created===true&&e.host_process_stopped===true&&e.host_process_tree_stopped===true&&counts.every(k=>e[k]===0)&&(!e.session_close_requested||e.session_close_acknowledged===true);
 }
 return (value.state==='failed'||value.state==='unknown')&&!Object.hasOwn(value,'catalogue')&&['acp_discovery_invalid','acp_discovery_unknown','acp_discovery_unexpected_activity','acp_discovery_model_unavailable','acp_discovery_not_stopped','acp_discovery_failed'].includes(String(value.error_code));
}
export function readDesktopACPResult(value:unknown,scope:DesktopACPScope,operationID?:string):DesktopACPResult|undefined{
 if(!validDesktopACPScope(scope)||operationID!==undefined&&!id(operationID)||!object(value))return;
 if(value.status==='failed'&&closed(value,['status','code'])&&code(value.code))return value as DesktopACPResult;
 if(value.status!=='ok'||!closed(value,['status','observation'])||!object(value.observation))return;
 const o=value.observation;
 if(!closed(o,['project_id','user_id','tenant_id','adapter_id','phase','installations'],['probe','error_code'])||o.project_id!==scope.project||o.user_id!==scope.userID||o.tenant_id!==scope.organizationID||o.adapter_id!==scope.adapterID||!phases.has(o.phase as DesktopACPPhase)||Object.hasOwn(o,'error_code')&&!code(o.error_code)||!Array.isArray(o.installations)||o.installations.length>256)return;
 const installations=new Map<string,DesktopACPInstallation>();
 for(const item of o.installations){
  if(!object(item)||!closed(item,['installation_id','label','kind'],['tool_version','executable_sha256'])||!id(item.installation_id)||installations.has(item.installation_id)||!text(item.label,4096)||!['native','wrapper_only','unverified'].includes(String(item.kind))||Object.hasOwn(item,'tool_version')&&!token(item.tool_version,256)||Object.hasOwn(item,'executable_sha256')&&!hash(item.executable_sha256))return;
  installations.set(item.installation_id,item as DesktopACPInstallation);
 }
 if(Object.hasOwn(o,'probe')){
  const p=o.probe;
  if(!object(p)||!closed(p,['id','operation_id','installation_id','source_sha256','created_at','expires_at'],['discovery'])||!id(p.id)||!id(p.operation_id)||!id(p.installation_id)||!hash(p.source_sha256)||operationID!==undefined&&p.operation_id!==operationID)return;
  const start=instant(p.created_at),end=instant(p.expires_at),now=BigInt(Date.now())*1000000n;
  if(start===undefined||end===undefined||end<=start||end-start>60000000000n||start>now)return;
  const installation=installations.get(p.installation_id);
  if(!installation||installation.kind!=='native'||!token(installation.tool_version,256)||!hash(installation.executable_sha256))return;
  if(Object.hasOwn(p,'discovery')&&(!validDiscovery(p.discovery)||p.discovery.executable_sha256!==installation.executable_sha256||p.discovery.tool_version!==installation.tool_version))return;
  if(o.phase==='ready'&&(end<=now||!validDiscovery(p.discovery)||p.discovery.state!=='ready'||Object.hasOwn(o,'error_code')))return;
 }else if(o.phase==='ready')return;
 return value as DesktopACPResult;
}
