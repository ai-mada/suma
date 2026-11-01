import type {DesktopOpenCodeSetup} from './desktop-opencode-connector-model';
import {validOpenCodeCapability} from './opencode-execution-model';
import {desktopACPRuntimeForAdapter,type DesktopACPSelection} from './desktop-acp-connector-model';
import type {DesktopCodexSelection,DesktopCodexRequest} from './desktop-codex-connector-model';
import {wellFormed} from './i18n/catalog';
export type DesktopToolScope=Readonly<{project:string;userID:string;organizationID:string;adapterID:string}>;
export type DesktopToolAction='connect'|'status'|'stop'|'renew'|'recover'|'recovery-status';
export type DesktopToolRequest=({project_id:string;user_id:string;tenant_id:string;adapter_id:string;locale:'zh-CN'|'en-US'}&(({action:'connect';expected_configuration_sha256?:string}&({acp_selection?:DesktopACPSelection;codex_selection?:never;opencode_setup?:never}|{codex_selection:DesktopCodexSelection;acp_selection?:never;opencode_setup?:never}|{opencode_setup:DesktopOpenCodeSetup;acp_selection?:never;codex_selection?:never}))|{action:'status'|'stop'|'renew'|'recover'|'recovery-status'}))|DesktopCodexRequest;
export type DesktopToolPhase='unconfigured'|'needs_workspace'|'not_installed'|'unavailable'|'prepared'|'issuance_unknown'|'renewal_unknown'|'starting'|'connected'|'stopping'|'stopped'|'authorization_required'|'binding_changed'|'origin_changed'|'credential_unrecoverable'|'stop_unconfirmed'|'failed';
export type DesktopToolRenewalReason='execution_pending'|'local_state_pending'|'credential_changed'|'access_changed'|'context_changed'|'result_unknown';
export type DesktopToolRecoveryReason='no_saved_results'|'receipt_missing'|'receipt_mismatch'|'unsupported_pending'|'state_changed'|'source_changed'|'access_changed'|'invalid_state'|'active_worker'|'result_unknown'|'proof_expired';
export type DesktopToolRecovery={operation_id:string;phase:'prepared'|'checking'|'unknown'|'complete'|'partial'|'blocked';total:number;recovered:number;remaining:number;reason_code?:DesktopToolRecoveryReason};
export type DesktopToolConnection={project_id:string;adapter_id:string;phase:DesktopToolPhase;recovery?:DesktopToolRecovery;renewal_reason?:DesktopToolRenewalReason;worker_id?:string;base_commit?:string;expires_at?:string;credential_expires_at?:string;credential_version?:number;repository_name?:string;model?:string;tool_version?:string;executable_sha256?:string;configuration_sha256?:string;catalog_sha256?:string;provider_id?:string;endpoint_origin?:string};
export type DesktopToolResult={status:'ok';connection:DesktopToolConnection}|{status:'failed';code:string};
export type DesktopToolBridge=Readonly<{platform:'win32'|'darwin'|'linux';version:'1.0.0';manageLocalTool:(request:DesktopToolRequest)=>Promise<unknown>}>;
const adapters=new Set(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','qoder-acp','cursor-cli','codex-app-server','opencode-sdk']);
const phases=new Set<DesktopToolPhase>(['unconfigured','needs_workspace','not_installed','unavailable','prepared','issuance_unknown','renewal_unknown','starting','connected','stopping','stopped','authorization_required','binding_changed','origin_changed','credential_unrecoverable','stop_unconfirmed','failed']);
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
 const c=value.connection,allowed=['project_id','adapter_id','phase','worker_id','base_commit','expires_at','credential_expires_at','credential_version','renewal_reason','recovery','repository_name'];
 if(scope.adapterID==='opencode-sdk')allowed.push('model','tool_version','executable_sha256','configuration_sha256','provider_id','endpoint_origin');
 const acp=!!desktopACPRuntimeForAdapter(scope.adapterID);
 if(acp)allowed.push('model','tool_version','executable_sha256','configuration_sha256');
 if(scope.adapterID==='codex-app-server')allowed.push('tool_version','executable_sha256','configuration_sha256','catalog_sha256');
 if(!['project_id','adapter_id','phase'].every(key=>Object.hasOwn(c,key))||Object.keys(c).some(key=>!allowed.includes(key))||c.project_id!==scope.project||c.adapter_id!==scope.adapterID||!phases.has(c.phase as DesktopToolPhase))return;
 if(Object.hasOwn(c,'recovery')&&!validRecovery(c.recovery))return;
 if(Object.hasOwn(c,'renewal_reason')&&(typeof c.renewal_reason!=='string'||!['execution_pending','local_state_pending','credential_changed','access_changed','context_changed','result_unknown'].includes(c.renewal_reason)))return;
 if(Object.hasOwn(c,'worker_id')&&(typeof c.worker_id!=='string'||!/^desktop-[a-f0-9]{32}$/.test(c.worker_id)))return;
 if(c.phase==='connected'&&!c.worker_id)return;
 if(Object.hasOwn(c,'base_commit')&&(typeof c.base_commit!=='string'||!/^([a-f0-9]{40}|[a-f0-9]{64})$/.test(c.base_commit)))return;
 if(Object.hasOwn(c,'expires_at')&&(typeof c.expires_at!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(c.expires_at)||!Number.isFinite(Date.parse(c.expires_at))||c.phase==='connected'&&Date.parse(c.expires_at)<=Date.now()))return;
 if(Object.hasOwn(c,'credential_expires_at')&&!credentialInstant(c.credential_expires_at))return;
 if(Object.hasOwn(c,'credential_version')&&(typeof c.credential_version!=='number'||!Number.isSafeInteger(c.credential_version)||c.credential_version<=0))return;
 if(Object.hasOwn(c,'repository_name')&&(typeof c.repository_name!=='string'||!c.repository_name.trim()||new TextEncoder().encode(c.repository_name).length>256||/[\x00-\x1f\x7f-\x9f\u2028\u2029]/.test(c.repository_name)))return;
 for(const key of ['model','tool_version'])if(Object.hasOwn(c,key)&&(typeof c[key]!=='string'||!c[key]||c[key].trim()!==c[key]||!wellFormed(c[key])||new TextEncoder().encode(c[key]).length>256||/\p{Cc}/u.test(c[key])))return;
 for(const key of ['executable_sha256','configuration_sha256','catalog_sha256'])if(Object.hasOwn(c,key)&&(typeof c[key]!=='string'||!/^[a-f0-9]{64}$/.test(c[key])))return;
 if(acp&&c.phase==='connected'&&!['model','tool_version','executable_sha256','configuration_sha256'].every(key=>Object.hasOwn(c,key)))return;
 if(scope.adapterID==='codex-app-server'&&c.phase==='connected'&&!['tool_version','executable_sha256','configuration_sha256','catalog_sha256'].every(key=>Object.hasOwn(c,key)))return;
 if(scope.adapterID==='opencode-sdk'){
  const fields=['model','tool_version','executable_sha256','configuration_sha256','provider_id','endpoint_origin'];
  if(c.phase==='connected'&&!fields.every(k=>Object.hasOwn(c,k)))return;
  if(Object.hasOwn(c,'tool_version')&&c.tool_version!=='1.19.4')return;
  if(fields.some(k=>Object.hasOwn(c,k))&&!validOpenCodeCapability({sdk_version:'1.19.4',server_version:'1.19.4',provider_id:c.provider_id,model:c.model,endpoint_origin:c.endpoint_origin,configuration_sha256:c.configuration_sha256,budget_mode:'observe_only',identity_evidence:'configuration_ack_only'}))return;
 }
 return {status:'ok',connection:{...c} as DesktopToolConnection};
}

/** Registration expiry is historical metadata, not a live inventory deadline. */
function credentialInstant(value:unknown):value is string{
 if(typeof value!=='string')return false;
 const match=/^(\d{4}-\d\d-\d\dT(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d)(?:\.\d{1,9})?Z$/.exec(value);
 if(!match)return false;
 const ms=Date.parse(match[1]+'Z');return Number.isFinite(ms)&&new Date(ms).toISOString().slice(0,19)===match[1];
}

/** Recovery counts describe receipt reconciliation, never execution permission. */
function validRecovery(value:unknown):value is DesktopToolRecovery{
 if(!record(value)||(Object.getPrototypeOf(value)!==Object.prototype&&Object.getPrototypeOf(value)!==null))return false;
 const required=['operation_id','phase','total','recovered','remaining'];
 if(!required.every(key=>Object.hasOwn(value,key))||!Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||key==='reason_code'))||!id(value.operation_id)||typeof value.phase!=='string'||!['prepared','checking','unknown','complete','partial','blocked'].includes(value.phase))return false;
 if(!['total','recovered','remaining'].every(key=>typeof value[key]==='number'&&Number.isSafeInteger(value[key])&&value[key]>=0))return false;
 return !Object.hasOwn(value,'reason_code')||typeof value.reason_code==='string'&&['no_saved_results','receipt_missing','receipt_mismatch','unsupported_pending','state_changed','source_changed','access_changed','invalid_state','active_worker','result_unknown','proof_expired'].includes(value.reason_code);
}
