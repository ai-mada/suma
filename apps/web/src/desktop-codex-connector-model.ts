import type {CodexModelDiscoveryResult} from '../../../packages/contracts/src';
import type {DesktopToolScope,DesktopToolConnection,DesktopToolRequest} from './desktop-tool-connector-model';
import {validCodexCatalog} from './codex-execution-model';
import {wellFormed} from './i18n/catalog';
export type DesktopCodexScope=DesktopToolScope;
export type DesktopCodexConfiguration={provider_id:string;base_url:string;api_key_env_ref?:string;windows_sandbox?:'unelevated'|'elevated'|'mxc';enable_user_input?:boolean};
export type DesktopCodexSelection={installation_id:string;probe_id:string};
type Platform='win32'|'darwin'|'linux';
type Base={project_id:string;user_id:string;tenant_id:string;adapter_id:'codex-app-server';locale:'zh-CN'|'en-US'};
export type DesktopCodexRequest=Base&({action:'detect'}|{action:'models';installation_id:string;operation_id:string;configuration:DesktopCodexConfiguration}|{action:'model-status';operation_id?:string});
export type DesktopCodexInstallation={installation_id:string;label:string;tool_version?:string;executable_sha256?:string;kind:'native'|'wrapper_only'|'unverified'};
export type DesktopCodexProbe={id:string;operation_id:string;installation_id:string;source_sha256:string;created_at:string;expires_at:string;discovery?:CodexModelDiscoveryResult};
export type DesktopCodexPhase='unconfigured'|'detected'|'not_installed'|'wrapper_only'|'unavailable'|'needs_workspace'|'authorization_required'|'binding_changed'|'reading'|'ready'|'failed'|'unknown'|'expired'|'stop_unconfirmed';
export type DesktopCodexObservation={project_id:string;user_id:string;tenant_id:string;adapter_id:'codex-app-server';phase:DesktopCodexPhase;installations:DesktopCodexInstallation[];probe?:DesktopCodexProbe};
export type DesktopCodexResult={status:'ok';observation:DesktopCodexObservation}|{status:'failed';code:string};
export type DesktopCodexBridge={platform:Platform;version:'1.0.0';manageLocalTool:(request:DesktopCodexRequest|DesktopToolRequest)=>Promise<unknown>};
export type DesktopCodexRequestOptions={locale:'zh-CN'|'en-US';installation_id?:string;operation_id?:string;configuration?:DesktopCodexConfiguration};
export type DesktopCodexConnectedProof=Required<Pick<DesktopToolConnection,'worker_id'|'tool_version'|'executable_sha256'|'configuration_sha256'|'catalog_sha256'>>;
const encoder=new TextEncoder();
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const closed=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const id=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{32}$/.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const text=(v:unknown,max:number):v is string=>typeof v==='string'&&wellFormed(v)&&encoder.encode(v).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(v);
const token=(v:unknown,max:number):v is string=>text(v,max)&&!!v&&v.trim()===v;
const code=(v:unknown):v is string=>typeof v==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(v);
const phases=new Set<DesktopCodexPhase>(['unconfigured','detected','not_installed','wrapper_only','unavailable','needs_workspace','authorization_required','binding_changed','reading','ready','failed','unknown','expired','stop_unconfirmed']);
const reserved=new Set(['CODEX_HOME','HOME','USERPROFILE','PATH','APPDATA','LOCALAPPDATA','TEMP','TMP','TMPDIR','SYSTEMROOT','WINDIR','COMSPEC','PATHEXT','NODE_OPTIONS','NODE_PATH']);
function credential(v:unknown):v is string{return typeof v==='string'&&/^[A-Za-z_][A-Za-z0-9_]{0,127}$/.test(v)&&!v.toUpperCase().startsWith('SUMA_')&&!reserved.has(v.toUpperCase());}
function instant(v:unknown):bigint|undefined{
 if(typeof v!=='string')return;
 const match=/^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.(\d{1,9}))?Z$/.exec(v);if(!match)return;
 const milliseconds=Date.parse(match[1]+'Z');if(!Number.isFinite(milliseconds)||new Date(milliseconds).toISOString().slice(0,19)!==match[1])return;
 return BigInt(milliseconds)*1000000n+BigInt((match[2]||'').padEnd(9,'0'));
}
export function desktopCodexBridge(value:unknown=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop):DesktopCodexBridge|undefined{
 return object(value)&&typeof value.platform==='string'&&['win32','darwin','linux'].includes(value.platform)&&value.version==='1.0.0'&&typeof value.manageLocalTool==='function'?value as DesktopCodexBridge:undefined;
}
export function validDesktopCodexScope(scope:DesktopCodexScope):boolean{return object(scope)&&id(scope.project)&&id(scope.userID)&&id(scope.organizationID)&&scope.adapterID==='codex-app-server';}
export function validDesktopCodexConfiguration(value:unknown,platform?:Platform):value is DesktopCodexConfiguration{
 if(!object(value)||!closed(value,['provider_id','base_url'],['api_key_env_ref','windows_sandbox','enable_user_input'])||typeof value.provider_id!=='string'||!/^[A-Za-z][A-Za-z0-9_-]{0,63}$/.test(value.provider_id)||!token(value.base_url,2048)||Object.hasOwn(value,'api_key_env_ref')&&!credential(value.api_key_env_ref)||Object.hasOwn(value,'windows_sandbox')&&(typeof value.windows_sandbox!=='string'||!['unelevated','elevated','mxc'].includes(value.windows_sandbox))||Object.hasOwn(value,'enable_user_input')&&typeof value.enable_user_input!=='boolean'||platform==='win32'&&!Object.hasOwn(value,'windows_sandbox'))return false;
 try{const url=new URL(value.base_url);return !!url.hostname&&!url.username&&!url.password&&!url.search&&!value.base_url.includes('?')&&!url.hash&&!value.base_url.includes('#')&&(url.protocol==='https:'||url.protocol==='http:'&&['127.0.0.1','localhost','[::1]'].includes(url.hostname));}catch{return false;}
}
export function validDesktopCodexSelection(value:unknown):value is DesktopCodexSelection{return object(value)&&closed(value,['installation_id','probe_id'])&&id(value.installation_id)&&id(value.probe_id);}
export function validDesktopCodexRequest(value:unknown,scope:DesktopCodexScope,platform?:Platform):value is DesktopCodexRequest{
 if(!validDesktopCodexScope(scope)||!object(value)||value.project_id!==scope.project||value.user_id!==scope.userID||value.tenant_id!==scope.organizationID||value.adapter_id!==scope.adapterID||typeof value.locale!=='string'||!['zh-CN','en-US'].includes(value.locale))return false;
 const base=['project_id','user_id','tenant_id','adapter_id','locale','action'];
 if(value.action==='detect')return closed(value,base);
 if(value.action==='model-status')return closed(value,base,['operation_id'])&&(!Object.hasOwn(value,'operation_id')||id(value.operation_id));
 return value.action==='models'&&closed(value,[...base,'installation_id','operation_id','configuration'])&&id(value.installation_id)&&id(value.operation_id)&&validDesktopCodexConfiguration(value.configuration,platform);
}
export function makeDesktopCodexRequest(action:DesktopCodexRequest['action'],scope:DesktopCodexScope,options:DesktopCodexRequestOptions,platform?:Platform):DesktopCodexRequest{
 const request={project_id:scope.project,user_id:scope.userID,tenant_id:scope.organizationID,adapter_id:scope.adapterID,action,...options};
 if(!validDesktopCodexRequest(request,scope,platform))throw Error('Invalid desktop Codex request');return request;
}
export function desktopCodexRecoveryScope(scope:DesktopCodexScope):string{return 'desktop-codex:'+JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.adapterID]);}
function validDiscovery(value:unknown):value is CodexModelDiscoveryResult{
 if(!object(value)||!closed(value,['protocol','state','executable_sha256','tool_version','configuration_sha256'],['catalogue','error_code'])||value.protocol!=='suma-codex-model-discovery-v1'||!hash(value.executable_sha256)||!token(value.tool_version,256)||!hash(value.configuration_sha256))return false;
 if(value.state==='ready')return !Object.hasOwn(value,'error_code')&&validCodexCatalog(value.catalogue)&&value.catalogue.configuration_sha256===value.configuration_sha256;
 return (value.state==='failed'||value.state==='unknown')&&!Object.hasOwn(value,'catalogue')&&typeof value.error_code==='string'&&['codex_discovery_invalid','codex_discovery_unknown','codex_discovery_installation_changed','codex_discovery_credential_unavailable','codex_discovery_catalogue_unavailable','codex_discovery_not_stopped','codex_discovery_failed'].includes(value.error_code);
}
export function readDesktopCodexResult(value:unknown,scope:DesktopCodexScope,operationID?:string):DesktopCodexResult|undefined{
 if(!validDesktopCodexScope(scope)||operationID!==undefined&&!id(operationID)||!object(value))return;
 if(value.status==='failed'&&closed(value,['status','code'])&&code(value.code))return value as DesktopCodexResult;
 if(value.status!=='ok'||!closed(value,['status','observation'])||!object(value.observation))return;
 const o=value.observation;
 if(!closed(o,['project_id','user_id','tenant_id','adapter_id','phase','installations'],['probe'])||o.project_id!==scope.project||o.user_id!==scope.userID||o.tenant_id!==scope.organizationID||o.adapter_id!==scope.adapterID||!phases.has(o.phase as DesktopCodexPhase)||!Array.isArray(o.installations)||o.installations.length>256)return;
 const installations=new Map<string,DesktopCodexInstallation>();
 for(const item of o.installations){
  if(!object(item)||!closed(item,['installation_id','label','kind'],['tool_version','executable_sha256'])||!id(item.installation_id)||installations.has(item.installation_id)||!text(item.label,4096)||typeof item.kind!=='string'||!['native','wrapper_only','unverified'].includes(item.kind)||Object.hasOwn(item,'tool_version')&&!token(item.tool_version,256)||Object.hasOwn(item,'executable_sha256')&&!hash(item.executable_sha256))return;
  installations.set(item.installation_id,item as DesktopCodexInstallation);
 }
 if(Object.hasOwn(o,'probe')){
  const p=o.probe;
  if(!object(p)||!closed(p,['id','operation_id','installation_id','source_sha256','created_at','expires_at'],['discovery'])||!id(p.id)||!id(p.operation_id)||!id(p.installation_id)||!hash(p.source_sha256)||operationID!==undefined&&p.operation_id!==operationID)return;
  const start=instant(p.created_at),end=instant(p.expires_at),now=BigInt(Date.now())*1000000n;
  if(start===undefined||end===undefined||end<=start||end-start>60000000000n||start>now)return;
  const installation=installations.get(p.installation_id);
  if(!installation||installation.kind!=='native'||!token(installation.tool_version,256)||!hash(installation.executable_sha256))return;
  if(Object.hasOwn(p,'discovery')&&(!validDiscovery(p.discovery)||p.discovery.executable_sha256!==installation.executable_sha256||p.discovery.tool_version!==installation.tool_version))return;
  if(o.phase==='ready'&&(end<=now||!validDiscovery(p.discovery)||p.discovery.state!=='ready'))return;
 }else if(o.phase==='ready')return;
 return value as DesktopCodexResult;
}
