import {validOpenCodeCapability} from './opencode-execution-model';
import type {DesktopOpenCodeConnectedProof} from './desktop-opencode-connector-model';
import {validDesktopClaudeSDKConfiguration,type DesktopClaudeSDKConnectedProof} from './desktop-claude-sdk-connector-model';
import {sameClaudeSDKCapability,validClaudeSDKCapability} from './claude-sdk-execution-model';
import type {AdapterDescriptor} from './AdapterConnections';
import type {ToolObservation,ToolProfile} from './tool-model';
import {wellFormed} from './i18n/catalog';
import {validCodexCatalog} from './codex-execution-model';
import {validACPCapability} from './acp-execution-model';
import type {DesktopCodexConnectedProof} from './desktop-codex-connector-model';
import {validToolInstallationProbeDiagnosticField,type ToolInstallationProbeDiagnostic} from './tool-installation-probe-diagnostic-model';

export function isCursorConnectionGuide(adapter:AdapterDescriptor):boolean{
 return adapter.id==='cursor-cli'&&adapter.product==='Cursor'&&adapter.direction==='suma_to_tool'&&adapter.implementation==='planned'&&adapter.setup_kind==='manual_guide'&&(adapter.adapter===undefined||adapter.adapter==='')&&(adapter.protocol===undefined||adapter.protocol==='');
}

export function quickConnectAdapters(items:AdapterDescriptor[]){
 return items.filter(item=>item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&!!item.adapter&&!!item.protocol&&(item.id!=='cursor-cli'||cursorACPDescriptor(item)));
}
const cursorACPDescriptor=(item:AdapterDescriptor)=>item.id==='cursor-cli'&&item.product==='Cursor'&&item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&item.adapter==='cursor'&&item.protocol==='acp-stdio-v1';
export function quickConnectWorkers(local:readonly {worker_id:string;status:string}[],shared:readonly {worker_id:string;status:string}[]){
 return [...new Set([...local,...shared].filter(item=>item.status==='active').map(item=>item.worker_id))];
}
export type ConfirmedNativeToolProof=Pick<DesktopCodexConnectedProof,'configuration_sha256'|'executable_sha256'|'tool_version'|'catalog_sha256'>;
export type ConfirmedOpenCodeToolProof=Omit<DesktopOpenCodeConnectedProof,'worker_id'>;
export type ConfirmedClaudeSDKToolProof=Omit<DesktopClaudeSDKConnectedProof,'worker_id'>;
export function matchingQuickTools(items:ToolObservation[],adapter:AdapterDescriptor,proof?:ConfirmedNativeToolProof|ConfirmedOpenCodeToolProof|ConfirmedClaudeSDKToolProof){
 if(adapter.direction!=='suma_to_tool'||adapter.setup_kind!=='runner_profile'||adapter.implementation!=='implemented'||adapter.id==='cursor-cli'&&!cursorACPDescriptor(adapter))return [];
 return items.filter(item=>{
  if(item.profile.adapter!==adapter.adapter||item.profile.protocol!==adapter.protocol)return false;
  if(adapter.id==='cursor-cli'&&!validACPCapability(item.profile.acp_execution,'cursor'))return false;
  if(adapter.id==='claude-code-sdk'){
   const capability=item.profile.claude_sdk_execution;
   if(item.profile.tool_version!=='2.1.314'||!validClaudeSDKCapability(capability))return false;
   if(proof===undefined)return true;
   // The scan identifies the native CLI; the inventory executable is the Node
   // host. Both identities and the complete SDK proof remain distinct.
   return 'claude_sdk_execution' in proof&&validDesktopClaudeSDKConfiguration({model:proof.model,credential_env_ref:proof.credential_env_ref})&&proof.tool_version==='2.1.314'&&/^[a-f0-9]{64}$/.test(proof.executable_sha256)&&/^[a-f0-9]{64}$/.test(proof.runtime_executable_sha256)&&item.available===true&&item.profile.executable_sha256===proof.runtime_executable_sha256&&proof.model===capability.model&&proof.configuration_sha256===capability.configuration_sha256&&sameClaudeSDKCapability(capability,proof.claude_sdk_execution);
  }
  if(adapter.id==='opencode-sdk'&&proof!==undefined){
   const c=item.profile.opencode_execution;
   return 'provider_id' in proof&&proof.tool_version==='1.19.4'&&/^[a-f0-9]{64}$/.test(proof.executable_sha256)&&item.available===true&&item.profile.tool_version==='suma-opencode-runtime 1.0.0'&&validOpenCodeCapability(c)&&c.configuration_sha256===proof.configuration_sha256&&c.provider_id===proof.provider_id&&c.model===proof.model&&c.endpoint_origin===proof.endpoint_origin;
  }
  if(adapter.id!=='codex-app-server'||proof===undefined)return true;
  if(!('catalog_sha256' in proof))return false;
  return ['configuration_sha256','executable_sha256','catalog_sha256'].every(key=>typeof proof[key as keyof ConfirmedNativeToolProof]==='string'&&/^[a-f0-9]{64}$/.test(proof[key as keyof ConfirmedNativeToolProof]))&&typeof proof.tool_version==='string'&&!!proof.tool_version&&item.available===true&&item.profile.executable_sha256===proof.executable_sha256&&item.profile.tool_version===proof.tool_version&&validCodexCatalog(item.codex_catalog)&&item.codex_catalog.configuration_sha256===proof.configuration_sha256&&item.codex_catalog.catalog_sha256===proof.catalog_sha256;
 });
}
const canonical=(value:unknown)=>JSON.stringify(value,(_key,item:unknown)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,(item as Record<string,unknown>)[key]])):item);
/** Keep an uncertain original operation recoverable when its response is not
 * the exact paused configuration the human registered. */
export function validQuickConnectReceipt(value:unknown,request:Record<string,unknown>):value is ToolProfile{
 if(!value||typeof value!=='object'||Array.isArray(value))return false;
 const row=value as ToolProfile;
 return typeof row.id==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/.test(row.id)&&row.project_id===request.project_id&&row.worker_id===request.worker_id&&typeof request.name==='string'&&row.name===request.name.trim()&&Number.isSafeInteger(row.version)&&row.version>0&&row.version<=2147483647&&row.state==='paused'&&!!request.profile&&canonical(row.profile)===canonical(request.profile);
}

export type InstalledToolFact={adapter_id:string;product:string;installed:boolean;launch_supported:boolean;executable?:string;version?:string;executable_sha256?:string;reason_code:string;diagnostic?:ToolInstallationProbeDiagnostic};
export const localDiscoveryAdapters=new Set(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','cursor-cli','codex-app-server','opencode-sdk','claude-code-sdk']);
const localProducts:Record<string,string>={'codex-jsonl':'Codex','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code'};
const localReasons=new Set(['not_installed','scan_incomplete','unsupported_launcher','invalid_package','node_unavailable','probe_failed','workspace_required']);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const encoder=new TextEncoder();
const safeText=(value:unknown,max:number)=>typeof value==='string'&&value.length>0&&encoder.encode(value).byteLength<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const cursorObject=(value:unknown):value is Record<string,unknown>=>object(value)&&(Object.getPrototypeOf(value)===Object.prototype||Object.getPrototypeOf(value)===null);
const cursorFields=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
function readCursorDiscovery(value:unknown):InstalledToolFact|undefined{
 if(!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1')return;
 const items=value.report.items;
 if(!Array.isArray(items)||Object.getPrototypeOf(items)!==Array.prototype||items.length!==1||Reflect.ownKeys(items).length!==2||!Object.hasOwn(items,'0'))return;
 const row=items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256','diagnostic'])||!validToolInstallationProbeDiagnosticField(row)||row.adapter_id!=='cursor-cli'||row.product!=='Cursor'||typeof row.installed!=='boolean'||row.launch_supported!==false)return;
 const sourceFields=['executable','version','executable_sha256'];
 const localPath=(value:unknown):value is string=>typeof value==='string'&&wellFormed(value)&&safeText(value,4096)&&(/^[a-z]:[\\/]/i.test(value)||/^\/(?!\/)/.test(value));
 if(['not_installed','scan_incomplete','cursor_editor_only','cursor_scan_incomplete'].includes(row.reason_code as string)){
  if(row.installed||sourceFields.some(key=>Object.hasOwn(row,key)))return;
 }else if(row.reason_code==='cursor_agent_unverified'||row.reason_code==='cursor_launcher_unsupported'){
  if(!row.installed||!localPath(row.executable)||['version','executable_sha256'].some(key=>Object.hasOwn(row,key)))return;
 }else if(row.reason_code==='acp_configuration_required'){
  if(!row.installed||!localPath(row.executable)||row.executable.trim()!==row.executable||!/[\\/]node(?:\.exe)?$/i.test(row.executable)||typeof row.version!=='string'||!wellFormed(row.version)||!safeText(row.version,256)||row.version.trim()!==row.version||typeof row.executable_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.executable_sha256))return;
 }else if(row.reason_code==='probe_failed'){
  if(!row.installed||sourceFields.some(key=>Object.hasOwn(row,key)))return;
 }else return;
 return row as InstalledToolFact;
}
/** Installed facts deliberately have no Profile, Runner identity or availability. */
export function readInstalledToolDiscovery(value:unknown,adapter:string):InstalledToolFact|undefined{
 if(adapter==='claude-code-sdk')return readClaudeSDKDiscovery(value);
 if(adapter==='opencode-sdk')return readOpenCodeDiscovery(value);
 if(adapter==='cursor-cli')return readCursorDiscovery(value);
 if(adapter==='codex-app-server')return readCodexDiscovery(value);
 if(!localDiscoveryAdapters.has(adapter)||!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.report.items)||Object.getPrototypeOf(value.report.items)!==Array.prototype||value.report.items.length!==1||Reflect.ownKeys(value.report.items).length!==2)return;
 const row=value.report.items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256','diagnostic'])||!validToolInstallationProbeDiagnosticField(row)||row.adapter_id!==adapter||row.product!==localProducts[adapter]||typeof row.installed!=='boolean'||typeof row.launch_supported!=='boolean'||typeof row.reason_code!=='string'||!localReasons.has(row.reason_code)||Object.hasOwn(row,'executable')&&!safeText(row.executable,4096)||Object.hasOwn(row,'version')&&!safeText(row.version,256)||Object.hasOwn(row,'executable_sha256')&&(typeof row.executable_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
 if(row.launch_supported&&(!row.installed||row.reason_code!=='workspace_required'||!safeText(row.version,256)||typeof row.executable!=='string'||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable))))return;
 if(row.reason_code==='workspace_required'&&!row.launch_supported)return;
 if(['not_installed','scan_incomplete'].includes(row.reason_code)&&(row.installed||row.launch_supported||['executable','version','executable_sha256'].some(key=>Object.hasOwn(row,key))))return;
 return row as InstalledToolFact;
}

function readClaudeSDKDiscovery(value:unknown):InstalledToolFact|undefined{
 if(!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1')return;
 const items=value.report.items;
 if(!Array.isArray(items)||Object.getPrototypeOf(items)!==Array.prototype||items.length!==1||Reflect.ownKeys(items).length!==2||!Object.hasOwn(items,'0'))return;
 const row=items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256','diagnostic'])||!validToolInstallationProbeDiagnosticField(row)||row.adapter_id!=='claude-code-sdk'||row.product!=='Claude Code'||typeof row.installed!=='boolean'||row.launch_supported!==false||typeof row.reason_code!=='string'||!['not_installed','scan_incomplete','unsupported_launcher','probe_failed','configuration_required','version_unsupported'].includes(row.reason_code))return;
 if(Object.hasOwn(row,'executable')&&(typeof row.executable!=='string'||!wellFormed(row.executable)||!safeText(row.executable,4096)||row.executable.trim()!==row.executable||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable)||/^\\\\[^\\]+\\[^\\]+/.test(row.executable))))return;
 if(Object.hasOwn(row,'version')&&(typeof row.version!=='string'||!wellFormed(row.version)||!safeText(row.version,256)||row.version.trim()!==row.version))return;
 if(Object.hasOwn(row,'executable_sha256')&&(typeof row.executable_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
 if(['configuration_required','version_unsupported'].includes(row.reason_code)&&(!row.installed||!row.executable||!row.version||!row.executable_sha256||(row.reason_code==='configuration_required')!==(row.version==='2.1.314')))return;
 if(['not_installed','scan_incomplete'].includes(row.reason_code)&&(row.installed||['executable','version','executable_sha256'].some(k=>Object.hasOwn(row,k))))return;
 return row as InstalledToolFact;
}
function readCodexDiscovery(value:unknown):InstalledToolFact|undefined{
 if(!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.report.items)||value.report.items.length!==1)return;
 const row=value.report.items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256','diagnostic'])||!validToolInstallationProbeDiagnosticField(row)||row.adapter_id!=='codex-app-server'||row.product!=='Codex'||typeof row.installed!=='boolean'||row.launch_supported!==false||typeof row.reason_code!=='string'||!['not_installed','scan_incomplete','unsupported_launcher','probe_failed','configuration_required'].includes(row.reason_code))return;
 if(Object.hasOwn(row,'executable')&&(typeof row.executable!=='string'||!wellFormed(row.executable)||!safeText(row.executable,4096)||row.executable.trim()!==row.executable||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable))))return;
 if(Object.hasOwn(row,'version')&&(typeof row.version!=='string'||!wellFormed(row.version)||!safeText(row.version,256)||row.version.trim()!==row.version))return;
 if(Object.hasOwn(row,'executable_sha256')&&(typeof row.executable_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
 if(row.reason_code==='configuration_required'&&(!row.installed||!row.executable||!row.version||!row.executable_sha256))return;
 if(['not_installed','scan_incomplete'].includes(row.reason_code)&&(row.installed||['executable','version','executable_sha256'].some(key=>Object.hasOwn(row,key))))return;
 return row as InstalledToolFact;
}

function readOpenCodeDiscovery(value:unknown):InstalledToolFact|undefined{
 if(!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.report.items)||Object.getPrototypeOf(value.report.items)!==Array.prototype||value.report.items.length!==1||Reflect.ownKeys(value.report.items).length!==2)return;
 const row=value.report.items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256','diagnostic'])||!validToolInstallationProbeDiagnosticField(row)||row.adapter_id!=='opencode-sdk'||row.product!=='OpenCode'||typeof row.installed!=='boolean'||row.launch_supported!==false||typeof row.reason_code!=='string'||!['not_installed','scan_incomplete','unsupported_launcher','probe_failed','configuration_required','version_unsupported'].includes(row.reason_code))return;
 if(Object.hasOwn(row,'executable')&&(typeof row.executable!=='string'||!wellFormed(row.executable)||!safeText(row.executable,4096)||row.executable.trim()!==row.executable||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable)||/^\\\\[^\\]+\\[^\\]+/.test(row.executable))))return;
 if(Object.hasOwn(row,'version')&&(typeof row.version!=='string'||!wellFormed(row.version)||!safeText(row.version,256)||row.version.trim()!==row.version))return;
 if(Object.hasOwn(row,'executable_sha256')&&(typeof row.executable_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
 if(['configuration_required','version_unsupported'].includes(row.reason_code)&&(!row.installed||!row.executable||!row.version||!row.executable_sha256||(row.reason_code==='configuration_required')!==(row.version==='1.19.4')))return;
 if(['not_installed','scan_incomplete'].includes(row.reason_code)&&(row.installed||['executable','version','executable_sha256'].some(k=>Object.hasOwn(row,k))))return;
 return row as InstalledToolFact;
}
