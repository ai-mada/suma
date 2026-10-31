import type {AdapterDescriptor} from './AdapterConnections';
import type {ToolObservation,ToolProfile} from './tool-model';
import {wellFormed} from './i18n/catalog';
import {validCodexCatalog} from './codex-execution-model';
import type {DesktopCodexConnectedProof} from './desktop-codex-connector-model';

export function isCursorConnectionGuide(adapter:AdapterDescriptor):boolean{
 return adapter.id==='cursor-cli'&&adapter.product==='Cursor'&&adapter.direction==='suma_to_tool'&&adapter.implementation==='planned'&&adapter.setup_kind==='manual_guide'&&(adapter.adapter===undefined||adapter.adapter==='')&&(adapter.protocol===undefined||adapter.protocol==='');
}

export function quickConnectAdapters(items:AdapterDescriptor[]){
 return items.filter(item=>item.id!=='cursor-cli'&&item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&!!item.adapter&&!!item.protocol);
}
export function quickConnectWorkers(local:readonly {worker_id:string;status:string}[],shared:readonly {worker_id:string;status:string}[]){
 return [...new Set([...local,...shared].filter(item=>item.status==='active').map(item=>item.worker_id))];
}
export type ConfirmedNativeToolProof=Pick<DesktopCodexConnectedProof,'configuration_sha256'|'executable_sha256'|'tool_version'|'catalog_sha256'>;
export function matchingQuickTools(items:ToolObservation[],adapter:AdapterDescriptor,proof?:ConfirmedNativeToolProof){
 if(adapter.id==='cursor-cli'||adapter.direction!=='suma_to_tool'||adapter.setup_kind!=='runner_profile'||adapter.implementation!=='implemented')return [];
 return items.filter(item=>{
  if(item.profile.adapter!==adapter.adapter||item.profile.protocol!==adapter.protocol)return false;
  if(adapter.id!=='codex-app-server'||proof===undefined)return true;
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

export type InstalledToolFact={adapter_id:string;product:string;installed:boolean;launch_supported:boolean;executable?:string;version?:string;executable_sha256?:string;reason_code:string};
export const localDiscoveryAdapters=new Set(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli','cursor-cli','codex-app-server']);
const localProducts:Record<string,string>={'codex-jsonl':'Codex','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code'};
const localReasons=new Set(['not_installed','unsupported_launcher','invalid_package','node_unavailable','probe_failed','workspace_required']);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const keys=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const encoder=new TextEncoder();
const safeText=(value:unknown,max:number)=>typeof value==='string'&&value.length>0&&encoder.encode(value).byteLength<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const cursorObject=(value:unknown):value is Record<string,unknown>=>object(value)&&(Object.getPrototypeOf(value)===Object.prototype||Object.getPrototypeOf(value)===null);
const cursorFields=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
function readCursorDiscovery(value:unknown):InstalledToolFact|undefined{
 if(!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1')return;
 const items=value.report.items;
 if(!Array.isArray(items)||Object.getPrototypeOf(items)!==Array.prototype||items.length!==1||Reflect.ownKeys(items).length!==2||!Object.hasOwn(items,'0'))return;
 const row=items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable'])||row.adapter_id!=='cursor-cli'||row.product!=='Cursor'||typeof row.installed!=='boolean'||row.launch_supported!==false)return;
 if(['not_installed','cursor_editor_only','cursor_scan_incomplete'].includes(row.reason_code as string)){
  if(row.installed||Object.hasOwn(row,'executable'))return;
 }else if(row.reason_code==='cursor_agent_unverified'||row.reason_code==='cursor_launcher_unsupported'){
  if(!row.installed||typeof row.executable!=='string'||!wellFormed(row.executable)||!safeText(row.executable,4096)||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable)))return;
 }else return;
 return row as InstalledToolFact;
}
/** Installed facts deliberately have no Profile, Runner identity or availability. */
export function readInstalledToolDiscovery(value:unknown,adapter:string):InstalledToolFact|undefined{
 if(adapter==='cursor-cli')return readCursorDiscovery(value);
 if(adapter==='codex-app-server')return readCodexDiscovery(value);
 if(!localDiscoveryAdapters.has(adapter)||!object(value)||!keys(value,['status','report'])||value.status!=='observed'||!object(value.report)||!keys(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.report.items)||value.report.items.length!==1)return;
 const row=value.report.items[0];
 if(!object(row)||!keys(row,['adapter_id','product','installed','launch_supported','executable','version','reason_code'])||row.adapter_id!==adapter||row.product!==localProducts[adapter]||typeof row.installed!=='boolean'||typeof row.launch_supported!=='boolean'||typeof row.reason_code!=='string'||!localReasons.has(row.reason_code)||row.executable!==undefined&&!safeText(row.executable,4096)||row.version!==undefined&&!safeText(row.version,256))return;
 if(row.launch_supported&&(!row.installed||row.reason_code!=='workspace_required'||!safeText(row.version,256)||typeof row.executable!=='string'||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable))))return;
 if(row.reason_code==='workspace_required'&&!row.launch_supported||row.reason_code==='not_installed'&&row.installed)return;
 return row as InstalledToolFact;
}
function readCodexDiscovery(value:unknown):InstalledToolFact|undefined{
 if(!cursorObject(value)||!cursorFields(value,['status','report'])||value.status!=='observed'||!cursorObject(value.report)||!cursorFields(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.report.items)||value.report.items.length!==1)return;
 const row=value.report.items[0];
 if(!cursorObject(row)||!cursorFields(row,['adapter_id','product','installed','launch_supported','reason_code'],['executable','version','executable_sha256'])||row.adapter_id!=='codex-app-server'||row.product!=='Codex'||typeof row.installed!=='boolean'||row.launch_supported!==false||typeof row.reason_code!=='string'||!['not_installed','unsupported_launcher','probe_failed','configuration_required'].includes(row.reason_code))return;
 if(Object.hasOwn(row,'executable')&&(typeof row.executable!=='string'||!wellFormed(row.executable)||!safeText(row.executable,4096)||row.executable.trim()!==row.executable||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable))))return;
 if(Object.hasOwn(row,'version')&&(typeof row.version!=='string'||!wellFormed(row.version)||!safeText(row.version,256)||row.version.trim()!==row.version))return;
 if(Object.hasOwn(row,'executable_sha256')&&(typeof row.executable_sha256!=='string'||!/^[a-f0-9]{64}$/.test(row.executable_sha256)))return;
 if(row.reason_code==='configuration_required'&&(!row.installed||!row.executable||!row.version||!row.executable_sha256))return;
 if(row.reason_code==='not_installed'&&(row.installed||['executable','version','executable_sha256'].some(key=>Object.hasOwn(row,key))))return;
 return row as InstalledToolFact;
}
