import type {ToolInstallationScan,ToolInstallationItem} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {readToolMaintenanceStatus} from './tool-maintenance-model';
import {preparationForLocalTool} from './tool-preparation-target';
import {toolDiscoveryTimestampNs} from './tool-discovery-model';
import {wellFormed} from './i18n/catalog';
import {UiError} from './i18n';
import {validToolInstallationProbeDiagnosticField} from './tool-installation-probe-diagnostic-model';

export type RemoteToolScanScope={project:string;workerID:string;userID:string;organizationID:string};
export type RemoteToolScan=ToolInstallationScan;
export type RemoteToolScanItem=ToolInstallationItem;
export const remoteToolScanScopeKey=(scope:RemoteToolScanScope)=>JSON.stringify([scope.organizationID,scope.userID,scope.project,scope.workerID]);
export const remoteToolScanListPath=(scope:RemoteToolScanScope)=>'/tool-installation-scans?project_id='+encodeURIComponent(scope.project)+'&worker_id='+encodeURIComponent(scope.workerID)+'&limit=20';
export const remoteToolScanPath=(scope:RemoteToolScanScope,id:string)=>'/tool-installation-scans/'+encodeURIComponent(id)+'?project_id='+encodeURIComponent(scope.project);
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$/.test(value);
const operation=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,149}$/.test(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const integer=(value:unknown,min:number,max=Number.MAX_SAFE_INTEGER)=>Number.isSafeInteger(value)&&(value as number)>=min&&(value as number)<=max;
const text=(value:unknown,max:number)=>typeof value==='string'&&wellFormed(value)&&new TextEncoder().encode(value).length<=max&&!/[\p{Cc}\u2028\u2029]/u.test(value);
const same=(left:unknown,right:unknown)=>JSON.stringify(left)===JSON.stringify(right);
const time=(value:unknown):value is string=>{try{toolDiscoveryTimestampNs(value);return true;}catch{return false;}};
export const validRemoteToolScanScope=(value:unknown):value is RemoteToolScanScope=>record(value)&&closed(value,['project','workerID','userID','organizationID'])&&Object.values(value).every(id);
export const validRemoteToolScanAdapters=(value:unknown):value is string[]=>Array.isArray(value)&&value.length>0&&value.length<=9&&Reflect.ownKeys(value).length===value.length+1&&new Set(value).size===value.length&&value.every(item=>!!preparationForLocalTool(item));
const unknownCode=(value:unknown)=>['probe_timeout','probe_interrupted','worker_restarted'].includes(String(value));
function invalid():never{throw new UiError('remote-tool-scan.invalid_response');}

export function validRemoteToolScanItem(value:unknown):value is RemoteToolScanItem{
 if(!record(value)||!closed(value,['adapter_id','protocol','installed','launch_supported','reason_code'],['version','executable_sha256','diagnostic'])||!validToolInstallationProbeDiagnosticField(value)||preparationForLocalTool(value.adapter_id)?.protocol!==value.protocol||!preparationForLocalTool(value.adapter_id)||typeof value.installed!=='boolean'||typeof value.launch_supported!=='boolean'||Object.hasOwn(value,'version')&&!text(value.version,256)||Object.hasOwn(value,'executable_sha256')&&!digest(value.executable_sha256))return false;
 const absent=['not_installed','scan_incomplete','cursor_editor_only','cursor_scan_incomplete'].includes(String(value.reason_code));
 if(absent)return !value.installed&&!value.launch_supported&&!Object.hasOwn(value,'version')&&!Object.hasOwn(value,'executable_sha256');
 if(['unsupported_launcher','invalid_package','node_unavailable','probe_failed','cursor_agent_unverified','cursor_launcher_unsupported'].includes(String(value.reason_code)))return !value.launch_supported;
 if(value.reason_code==='workspace_required')return value.installed&&value.launch_supported&&typeof value.version==='string'&&value.version!=='';
 return ['configuration_required','version_unsupported','acp_configuration_required'].includes(String(value.reason_code))&&value.installed&&!value.launch_supported&&typeof value.version==='string'&&value.version!==''&&digest(value.executable_sha256);
}
function validProof(value:unknown,scan:Record<string,unknown>){
 if(!record(value)||!closed(value,['protocol','scan','source_sha256','receipt_sha256','probe_returned','all_native_trees_stopped','native_process_count','stopped_process_count','observed_at'])||value.protocol!=='suma-installed-tool-stop-v1'||!record(value.scan)||!closed(value.scan,['scan_id','claim_id','fence'])||value.scan.scan_id!==scan.id||value.scan.claim_id!==scan.claim_id||value.scan.fence!==scan.fence||value.source_sha256!==scan.source_sha256||!digest(value.receipt_sha256)||value.probe_returned!==true||value.all_native_trees_stopped!==true||!integer(value.native_process_count,0,2048)||value.stopped_process_count!==value.native_process_count||!time(value.observed_at))return false;
 return true;
}
async function sourceDigest(value:Record<string,unknown>){
 // Match the sole Go ScanSourceSHA256 shape. Validated source fields here are
 // ASCII identities/timestamps, so JSON.stringify needs no HTML escaping.
 const source={ID:value.id,ProjectID:value.project_id,WorkerID:value.worker_id,AdapterIDs:value.adapter_ids,InstanceID:value.instance_id,RequestedBy:value.requested_by,CreatedAt:value.created_at,ExpiresAt:value.expires_at};
 const hash=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(JSON.stringify(source)));return [...new Uint8Array(hash)].map(byte=>byte.toString(16).padStart(2,'0')).join('');
}
/** Exact public scan, native-tree proof and original-source binding. No local
 * installation parser, path, product label, credential or Command is accepted. */
export async function readRemoteToolScan(value:unknown,scope:RemoteToolScanScope,expectedID?:string):Promise<RemoteToolScan>{
 if(!validRemoteToolScanScope(scope)||!record(value)||!closed(value,['id','project_id','worker_id','adapter_ids','instance_id','version','state','requested_by','created_at','expires_at','source_sha256','fence'],['claim_id','claimed_at','lease_until','finished_at','result_code','result','source_changed'])||!id(value.id)||expectedID!==undefined&&value.id!==expectedID||value.project_id!==scope.project||value.worker_id!==scope.workerID||!id(value.requested_by)||!operation(value.instance_id)||!validRemoteToolScanAdapters(value.adapter_ids)||!integer(value.version,1,2147483647)||!integer(value.fence,0)||!digest(value.source_sha256)||!time(value.created_at)||!time(value.expires_at)||toolDiscoveryTimestampNs(value.expires_at)-toolDiscoveryTimestampNs(value.created_at)!==300000000000n||Object.hasOwn(value,'source_changed')&&typeof value.source_changed!=='boolean')return invalid();
 const claimed=(value.fence as number)>0,finished=Object.hasOwn(value,'finished_at');
 if(claimed){if(!operation(value.claim_id)||!time(value.claimed_at)||!time(value.lease_until)||toolDiscoveryTimestampNs(value.lease_until)-toolDiscoveryTimestampNs(value.claimed_at)!==60000000000n)return invalid();}
 else if(['claim_id','claimed_at','lease_until'].some(key=>Object.hasOwn(value,key)))return invalid();
 if(finished&&!time(value.finished_at))return invalid();
 const result=Object.hasOwn(value,'result');
 if(value.state==='pending'){if(claimed||finished||result||Object.hasOwn(value,'result_code'))return invalid();}
 else if(value.state==='claimed'){if(!claimed||finished||result||Object.hasOwn(value,'result_code'))return invalid();}
 else if(value.state==='unknown'){if(!claimed||finished||result||!unknownCode(value.result_code))return invalid();}
 else if(value.state==='expired'||value.state==='superseded'){if(claimed||finished||result||value.result_code!==(value.state==='expired'?'request_expired':'source_changed'))return invalid();}
 else if(['completed','failed','closed_unknown'].includes(String(value.state))){
  if(!claimed||!finished||!record(value.result)||!closed(value.result,['items','stop_proof'])||!Array.isArray(value.result.items)||value.result.items.length>9||Reflect.ownKeys(value.result.items).length!==value.result.items.length+1||!validProof(value.result.stop_proof,value))return invalid();
  if(value.state==='completed'){if(value.result_code!=='scan_completed'||value.result.items.length!==value.adapter_ids.length||!value.result.items.every((item,index)=>validRemoteToolScanItem(item)&&item.adapter_id===(value.adapter_ids as string[])[index]))return invalid();}
  else if(value.result.items.length!==0||value.state==='failed'&&(value.result_code!=='not_started'||(value.result.stop_proof as Record<string,unknown>).native_process_count!==0)||value.state==='closed_unknown'&&!unknownCode(value.result_code))return invalid();
 }else return invalid();
 if(await sourceDigest(value)!==value.source_sha256)return invalid();return structuredClone(value) as unknown as RemoteToolScan;
}
export async function readRemoteToolScans(value:unknown,scope:RemoteToolScanScope){
 if(!validRemoteToolScanScope(scope)||!record(value)||!closed(value,['project_id','worker_id','as_of','maintenance','items'],['next_cursor'])||value.project_id!==scope.project||value.worker_id!==scope.workerID||!time(value.as_of)||!Array.isArray(value.items)||value.items.length>20||Reflect.ownKeys(value.items).length!==value.items.length+1||Object.hasOwn(value,'next_cursor')&&(!text(value.next_cursor,4096)||value.next_cursor===''))return invalid();
 const maintenance=readToolMaintenanceStatus(value.maintenance);if(!maintenance||maintenance.status.checked_at!==value.as_of)return invalid();
 const items=await Promise.all(value.items.map(item=>readRemoteToolScan(item,scope)));if(new Set(items.map(item=>item.id)).size!==items.length)return invalid();
 return {project_id:scope.project,worker_id:scope.workerID,as_of:value.as_of,maintenance,items,...(typeof value.next_cursor==='string'?{next_cursor:value.next_cursor}:{})};
}
export function validRemoteToolScanRequest(request:WriteRecoveryRequest,scope:RemoteToolScanScope){
 const body=request.body;return validRemoteToolScanScope(scope)&&request.path==='/tool-installation-scans'&&request.method==='POST'&&record(body)&&closed(body,['project_id','worker_id','adapter_ids','idempotency_key'])&&body.project_id===scope.project&&body.worker_id===scope.workerID&&validRemoteToolScanAdapters(body.adapter_ids)&&body.idempotency_key===request.idempotency_key&&operation(request.idempotency_key);
}
export async function validRemoteToolScanReceipt(value:unknown,request:WriteRecoveryRequest,scope:RemoteToolScanScope){
 if(!validRemoteToolScanRequest(request,scope))return false;try{const scan=await readRemoteToolScan(value,scope);return scan.requested_by===scope.userID&&same(scan.adapter_ids,request.body.adapter_ids);}catch{return false;}
}
