import type {AdapterDescriptor} from './AdapterConnections';
import type {ToolObservation,ToolProfile} from './tool-model';

export function quickConnectAdapters(items:AdapterDescriptor[]){
 return items.filter(item=>item.direction==='suma_to_tool'&&item.implementation==='implemented'&&item.setup_kind==='runner_profile'&&!!item.adapter&&!!item.protocol);
}
export function quickConnectWorkers(local:readonly {worker_id:string;status:string}[],shared:readonly {worker_id:string;status:string}[]){
 return [...new Set([...local,...shared].filter(item=>item.status==='active').map(item=>item.worker_id))];
}
export function matchingQuickTools(items:ToolObservation[],adapter:AdapterDescriptor){
 return items.filter(item=>item.profile.adapter===adapter.adapter&&item.profile.protocol===adapter.protocol);
}
const canonical=(value:unknown)=>JSON.stringify(value,(_key,item:unknown)=>item&&typeof item==='object'&&!Array.isArray(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,(item as Record<string,unknown>)[key]])):item);
/** Keep an uncertain original operation recoverable when its response is not
 * the exact paused configuration the human registered. */
export function validQuickConnectReceipt(value:unknown,request:Record<string,unknown>):value is ToolProfile{
 if(!value||typeof value!=='object'||Array.isArray(value))return false;
 const row=value as ToolProfile;
 return typeof row.id==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/.test(row.id)&&row.project_id===request.project_id&&row.worker_id===request.worker_id&&typeof request.name==='string'&&row.name===request.name.trim()&&Number.isSafeInteger(row.version)&&row.version>0&&row.version<=2147483647&&row.state==='paused'&&!!request.profile&&canonical(row.profile)===canonical(request.profile);
}

export type InstalledToolFact={adapter_id:string;product:string;installed:boolean;launch_supported:boolean;executable?:string;version?:string;reason_code:string};
export const localDiscoveryAdapters=new Set(['codex-jsonl','codebuddy-cli','dsh-multica','claude-code-cli']);
const localProducts:Record<string,string>={'codex-jsonl':'Codex','codebuddy-cli':'CodeBuddy Code','dsh-multica':'DeepSeek Harness / dsh','claude-code-cli':'Claude Code'};
const localReasons=new Set(['not_installed','unsupported_launcher','invalid_package','node_unavailable','probe_failed','workspace_required']);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const keys=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const encoder=new TextEncoder();
const safeText=(value:unknown,max:number)=>typeof value==='string'&&value.length>0&&encoder.encode(value).byteLength<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
/** Installed facts deliberately have no Profile, Runner identity or availability. */
export function readInstalledToolDiscovery(value:unknown,adapter:string):InstalledToolFact|undefined{
 if(!localDiscoveryAdapters.has(adapter)||!object(value)||!keys(value,['status','report'])||value.status!=='observed'||!object(value.report)||!keys(value.report,['protocol','items'])||value.report.protocol!=='suma-installed-tools-v1'||!Array.isArray(value.report.items)||value.report.items.length!==1)return;
 const row=value.report.items[0];
 if(!object(row)||!keys(row,['adapter_id','product','installed','launch_supported','executable','version','reason_code'])||row.adapter_id!==adapter||row.product!==localProducts[adapter]||typeof row.installed!=='boolean'||typeof row.launch_supported!=='boolean'||typeof row.reason_code!=='string'||!localReasons.has(row.reason_code)||row.executable!==undefined&&!safeText(row.executable,4096)||row.version!==undefined&&!safeText(row.version,256))return;
 if(row.launch_supported&&(!row.installed||row.reason_code!=='workspace_required'||!safeText(row.version,256)||typeof row.executable!=='string'||!(/^[a-z]:[\\/]/i.test(row.executable)||/^\/(?!\/)/.test(row.executable))))return;
 if(row.reason_code==='workspace_required'&&!row.launch_supported||row.reason_code==='not_installed'&&row.installed)return;
 return row as InstalledToolFact;
}
