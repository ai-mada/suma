import type {DesktopOpenCodeSetup} from './desktop-opencode-connector-model';
import {validOpenCodeCapability} from './opencode-execution-model';
import type {DesktopACPSelection} from './desktop-acp-connector-model';
import type {DesktopCodexSelection,DesktopCodexRequest} from './desktop-codex-connector-model';
import {wellFormed} from './i18n/catalog';
export type DesktopToolScope=Readonly<{project:string;userID:string;organizationID:string;adapterID:string}>;
export type DesktopToolAction='connect'|'status'|'stop';
export type DesktopToolRequest=({project_id:string;user_id:string;tenant_id:string;adapter_id:string;locale:'zh-CN'|'en-US'}&(({action:'connect';expected_configuration_sha256?:string}&({acp_selection?:DesktopACPSelection;codex_selection?:never;opencode_setup?:never}|{codex_selection:DesktopCodexSelection;acp_selection?:never;opencode_setup?:never}|{opencode_setup:DesktopOpenCodeSetup;acp_selection?:never;codex_selection?:never}))|{action:'status'|'stop'}))|DesktopCodexRequest;
export type DesktopToolPhase='unconfigured'|'needs_workspace'|'not_installed'|'unavailable'|'prepared'|'issuance_unknown'|'starting'|'connected'|'stopping'|'stopped'|'authorization_required'|'binding_changed'|'origin_changed'|'credential_unrecoverable'|'stop_unconfirmed'|'failed';
export type DesktopToolConnection={project_id:string;adapter_id:string;phase:DesktopToolPhase;worker_id?:string;base_commit?:string;expires_at?:string;repository_name?:string;model?:string;tool_version?:string;executable_sha256?:string;configuration_sha256?:string;catalog_sha256?:string;provider_id?:string;endpoint_origin?:string};
export type DesktopToolResult={status:'ok';connection:DesktopToolConnection}|{status:'failed';code:string};
export type DesktopToolBridge=Readonly<{platform:'win32'|'darwin'|'linux';version:'1.0.0';manageLocalTool:(request:DesktopToolRequest)=>Promise<unknown>}>;
const adapters=new Set(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','qoder-acp','codex-app-server','opencode-sdk']);
const phases=new Set<DesktopToolPhase>(['unconfigured','needs_workspace','not_installed','unavailable','prepared','issuance_unknown','starting','connected','stopping','stopped','authorization_required','binding_changed','origin_changed','credential_unrecoverable','stop_unconfirmed','failed']);
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
const id=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{32}$/.test(value);
export function desktopToolBridge(value:unknown=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop):DesktopToolBridge|undefined{
 return record(value)&&['win32','darwin','linux'].includes(String(value.platform))&&value.version==='1.0.0'&&typeof value.manageLocalTool==='function'?value as DesktopToolBridge:undefined;
}
export function validDesktopToolScope(scope:DesktopToolScope):boolean{return id(scope.project)&&id(scope.userID)&&id(scope.organizationID)&&adapters.has(scope.adapterID);}
export function readDesktopToolResult(value:unknown,scope:DesktopToolScope):DesktopToolResult|undefined{
 if(!validDesktopToolScope(scope)||!record(value))return;
 if(exact(value,['status','code'])&&value.status==='failed'&&typeof value.code==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(value.code))return {status:'failed',code:value.code};
 if(!exact(value,['status','connection'])||value.status!=='ok'||!record(value.connection))return;
 const c=value.connection,allowed=['project_id','adapter_id','phase','worker_id','base_commit','expires_at','repository_name'];
 if(scope.adapterID==='opencode-sdk')allowed.push('model','tool_version','executable_sha256','configuration_sha256','provider_id','endpoint_origin');
 if(scope.adapterID==='qoder-acp')allowed.push('model','tool_version','executable_sha256','configuration_sha256');
 if(scope.adapterID==='codex-app-server')allowed.push('tool_version','executable_sha256','configuration_sha256','catalog_sha256');
 if(!['project_id','adapter_id','phase'].every(key=>Object.hasOwn(c,key))||Object.keys(c).some(key=>!allowed.includes(key))||c.project_id!==scope.project||c.adapter_id!==scope.adapterID||!phases.has(c.phase as DesktopToolPhase))return;
 if(Object.hasOwn(c,'worker_id')&&(typeof c.worker_id!=='string'||!/^desktop-[a-f0-9]{32}$/.test(c.worker_id)))return;
 if(c.phase==='connected'&&!c.worker_id)return;
 if(Object.hasOwn(c,'base_commit')&&(typeof c.base_commit!=='string'||!/^([a-f0-9]{40}|[a-f0-9]{64})$/.test(c.base_commit)))return;
 if(Object.hasOwn(c,'expires_at')&&(typeof c.expires_at!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(c.expires_at)||!Number.isFinite(Date.parse(c.expires_at))||c.phase==='connected'&&Date.parse(c.expires_at)<=Date.now()))return;
 if(Object.hasOwn(c,'repository_name')&&(typeof c.repository_name!=='string'||!c.repository_name.trim()||new TextEncoder().encode(c.repository_name).length>256||/[\x00-\x1f\x7f-\x9f\u2028\u2029]/.test(c.repository_name)))return;
 for(const key of ['model','tool_version'])if(Object.hasOwn(c,key)&&(typeof c[key]!=='string'||!c[key]||c[key].trim()!==c[key]||!wellFormed(c[key])||new TextEncoder().encode(c[key]).length>256||/\p{Cc}/u.test(c[key])))return;
 for(const key of ['executable_sha256','configuration_sha256','catalog_sha256'])if(Object.hasOwn(c,key)&&(typeof c[key]!=='string'||!/^[a-f0-9]{64}$/.test(c[key])))return;
 if(scope.adapterID==='codex-app-server'&&c.phase==='connected'&&!['tool_version','executable_sha256','configuration_sha256','catalog_sha256'].every(key=>Object.hasOwn(c,key)))return;
 if(scope.adapterID==='opencode-sdk'){
  const fields=['model','tool_version','executable_sha256','configuration_sha256','provider_id','endpoint_origin'];
  if(c.phase==='connected'&&!fields.every(k=>Object.hasOwn(c,k)))return;
  if(Object.hasOwn(c,'tool_version')&&c.tool_version!=='1.19.4')return;
  if(fields.some(k=>Object.hasOwn(c,k))&&!validOpenCodeCapability({sdk_version:'1.19.4',server_version:'1.19.4',provider_id:c.provider_id,model:c.model,endpoint_origin:c.endpoint_origin,configuration_sha256:c.configuration_sha256,budget_mode:'observe_only',identity_evidence:'configuration_ack_only'}))return;
 }
 return {status:'ok',connection:{...c} as DesktopToolConnection};
}
