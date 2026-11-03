import type {ManagedToolPreparation,ManagedToolPreparationConfiguration,ManagedToolPreparationStopProof} from '../../../packages/contracts/src';
import {validDesktopACPModel,validDesktopACPCredentialReference} from './desktop-acp-connector-model';
import {validManagedToolPreparationCapability} from './managed-tool-preparation-capability-model';
import {readToolMaintenanceStatus} from './tool-maintenance-model';
import {validRemoteToolScanItem,validRemoteToolScanScope,type RemoteToolScanScope} from './remote-tool-scan-model';
import {parseToolManifest} from './tool-model';
import {sameToolProfile} from './tool-profile-reuse-model';
import {toolDiscoveryTimestampNs} from './tool-discovery-model';
import type {WriteRecoveryRequest} from './write-action';
import {UiError} from './i18n';

export type ToolPreparationOperation=ManagedToolPreparation;
export const managedToolPreparationPath=(project:string,id:string)=>'/tool-preparations/'+encodeURIComponent(id)+'?project_id='+encodeURIComponent(project);
export const managedToolPreparationListPath=(project:string,workerID:string)=>'/tool-preparations?project_id='+encodeURIComponent(project)+'&worker_id='+encodeURIComponent(workerID)+'&limit=20';
export const managedToolPreparationUnresolved=(operation:ToolPreparationOperation)=>['pending','claimed','awaiting_inventory','unknown'].includes(operation.state);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(row:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(row,key))&&Reflect.ownKeys(row).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$/.test(value);
const operationID=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,149}$/.test(value);
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const integer=(value:unknown,min:number,max=Number.MAX_SAFE_INTEGER):value is number=>Number.isSafeInteger(value)&&(value as number)>=min&&(value as number)<=max;
const time=(value:unknown):value is string=>{try{toolDiscoveryTimestampNs(value);return true;}catch{return false;}};
const equal=(left:unknown,right:unknown)=>JSON.stringify(left)===JSON.stringify(right);
const unknownCodes=['preparation_timeout','preparation_interrupted','worker_restarted','outcome_unknown','source_changed'];
const failedCodes=['not_started','source_changed','rescan_required','configuration_invalid','credential_reference_unavailable','native_source_changed','generation_changed','preparation_not_supported','preparation_failed'];
function invalid():never{throw new UiError('managed-tool-preparation.invalid_response');}
export function validManagedToolPreparationConfiguration(adapter:unknown,value:unknown):value is ManagedToolPreparationConfiguration{
 return (adapter==='qoder-acp'||adapter==='cursor-cli')&&object(value)&&closed(value,['model','credential_env_ref'])&&validDesktopACPModel(value.model)&&(value.credential_env_ref===''?adapter==='qoder-acp':validDesktopACPCredentialReference(value.credential_env_ref));
}
export function canPrepareManagedToolItem(value:unknown):boolean{
 return validRemoteToolScanItem(value)&&(value.adapter_id==='qoder-acp'||value.adapter_id==='cursor-cli')&&value.installed&&!value.launch_supported&&value.reason_code==='acp_configuration_required'&&validDesktopACPModel(value.version)&&hash(value.executable_sha256);
}
function proof(value:unknown,row:Record<string,unknown>):value is ManagedToolPreparationStopProof{
 return object(value)&&closed(value,['protocol','preparation','source_sha256','receipt_sha256','probe_returned','all_native_trees_stopped','native_process_count','stopped_process_count','observed_at'])&&value.protocol==='suma-managed-tool-preparation-stop-v1'&&object(value.preparation)&&closed(value.preparation,['preparation_id','claim_id','fence'])&&value.preparation.preparation_id===row.id&&value.preparation.claim_id===row.claim_id&&value.preparation.fence===row.fence&&value.source_sha256===row.source_sha256&&hash(value.receipt_sha256)&&value.probe_returned===true&&value.all_native_trees_stopped===true&&integer(value.native_process_count,0,2048)&&value.stopped_process_count===value.native_process_count&&time(value.observed_at);
}
/** Strict public facts only. Native locations and local acceptance files never
 * enter browser state. The API remains the authority for the source digest. */
export async function readManagedToolPreparation(value:unknown,scope:RemoteToolScanScope,expectedID?:string):Promise<ToolPreparationOperation>{
 const required=['id','tenant_id','project_id','worker_id','version','state','adapter_id','configuration','instance_id','installation_scan_id','installation_scan_version','installation_reference','installation_source_sha256','installation_item','installation_stop_proof','expected_generation_sha256','requested_by','created_at','expires_at','source_sha256','fence'];
 const optional=['claim_id','claimed_at','lease_until','finished_at','result_code','receipt','stop_proof','inventory_operation_id','inventory_observed_at','source_changed'];
 if(!validRemoteToolScanScope(scope)||!object(value)||!closed(value,required,optional)||!id(value.id)||expectedID!==undefined&&value.id!==expectedID||value.tenant_id!==scope.organizationID||value.project_id!==scope.project||value.worker_id!==scope.workerID||!integer(value.version,1,2147483647)||!validManagedToolPreparationConfiguration(value.adapter_id,value.configuration)||!operationID(value.instance_id)||!id(value.installation_scan_id)||!integer(value.installation_scan_version,1,2147483647)||!hash(value.installation_source_sha256)||!hash(value.expected_generation_sha256)||!id(value.requested_by)||!time(value.created_at)||!time(value.expires_at)||toolDiscoveryTimestampNs(value.expires_at)-toolDiscoveryTimestampNs(value.created_at)!==300000000000n||!hash(value.source_sha256)||!integer(value.fence,0)||Object.hasOwn(value,'source_changed')&&typeof value.source_changed!=='boolean')return invalid();
 const ref=value.installation_reference,sourceProof=value.installation_stop_proof,item=value.installation_item;
 if(!object(ref)||!closed(ref,['scan_id','claim_id','fence'])||ref.scan_id!==value.installation_scan_id||!operationID(ref.claim_id)||!integer(ref.fence,1)||!object(sourceProof)||sourceProof.protocol!=='suma-installed-tool-stop-v1'||!equal(sourceProof.scan,ref)||sourceProof.source_sha256!==value.installation_source_sha256||!hash(sourceProof.receipt_sha256)||sourceProof.probe_returned!==true||sourceProof.all_native_trees_stopped!==true||!integer(sourceProof.native_process_count,0,2048)||sourceProof.native_process_count!==sourceProof.stopped_process_count||!time(sourceProof.observed_at)||!object(item)||item.adapter_id!==value.adapter_id||!item.installed)return invalid();
 // Reuse the scanner's sole item contract; no browser-side native discovery.
 if(!canPrepareManagedToolItem(item))return invalid();
 if(!closed(sourceProof,['protocol','scan','source_sha256','receipt_sha256','probe_returned','all_native_trees_stopped','native_process_count','stopped_process_count','observed_at']))return invalid();
 const claimed=value.fence>0,has=(key:string)=>Object.hasOwn(value,key);
 if(claimed){if(!operationID(value.claim_id)||!time(value.claimed_at)||!time(value.lease_until)||toolDiscoveryTimestampNs(value.lease_until)-toolDiscoveryTimestampNs(value.claimed_at)!==60000000000n)return invalid();}
 else if(['claim_id','claimed_at','lease_until'].some(has))return invalid();
 if(has('finished_at')&&!time(value.finished_at)||has('stop_proof')&&!proof(value.stop_proof,value))return invalid();
 if(has('receipt')){
  const receipt=value.receipt;if(!object(receipt)||!closed(receipt,['parent_generation_sha256','generation_sha256','command_id','artifact_sha256','local_receipt_sha256','profile','stop_proof'])||receipt.parent_generation_sha256!==value.expected_generation_sha256||!hash(receipt.generation_sha256)||receipt.generation_sha256===receipt.parent_generation_sha256||!hash(receipt.artifact_sha256)||!hash(receipt.local_receipt_sha256)||!proof(receipt.stop_proof,value))return invalid();
  let observation;try{observation=parseToolManifest(JSON.stringify([{profile:receipt.profile,available:true,reason:''}]))[0];}catch{return invalid();}
  const encoded=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(JSON.stringify(value.id))),expectedCommand='managed-'+[...new Uint8Array(encoded)].map(byte=>byte.toString(16).padStart(2,'0')).join('');
  if(receipt.command_id!==expectedCommand||receipt.command_id!==observation.profile.command_id||!sameToolProfile(observation.profile,observation.profile)||observation.profile.executable_sha256!==item.executable_sha256||observation.profile.tool_version!==item.version||observation.profile.protocol!=='acp-stdio-v1'||observation.profile.acp_execution?.runtime!==(value.adapter_id==='qoder-acp'?'qoder':'cursor')||observation.profile.acp_execution.model!==(value.configuration as ManagedToolPreparationConfiguration).model)return invalid();
 }
 const finished=has('finished_at'),receipt=has('receipt'),stopped=has('stop_proof'),inventory=has('inventory_operation_id')||has('inventory_observed_at');
 switch(value.state){
  case 'pending':if(claimed||finished||receipt||stopped||inventory||has('result_code'))return invalid();break;
  case 'claimed':if(!claimed||finished||receipt||stopped||inventory||has('result_code'))return invalid();break;
  case 'unknown':if(!claimed||finished||receipt||stopped||inventory||!unknownCodes.includes(String(value.result_code)))return invalid();break;
  case 'expired':case 'superseded':if(claimed||finished||receipt||stopped||inventory||value.result_code!==(value.state==='expired'?'request_expired':'source_changed'))return invalid();break;
  case 'awaiting_inventory':case 'completed':if(!claimed||!receipt||stopped||value.result_code!==(value.state==='completed'?'completed':'prepared')||value.state==='awaiting_inventory'&&(inventory||finished)||value.state==='completed'&&(!finished||!operationID(value.inventory_operation_id)||!time(value.inventory_observed_at)))return invalid();break;
  case 'failed':case 'closed_unknown':if(!claimed||!finished||receipt||!stopped||inventory||!(value.state==='failed'?failedCodes:unknownCodes).includes(String(value.result_code))||value.result_code==='not_started'&&(value.stop_proof as ManagedToolPreparationStopProof).native_process_count!==0)return invalid();break;
  default:return invalid();
 }
 return structuredClone(value) as unknown as ToolPreparationOperation;
}
export async function readManagedToolPreparationPage(value:unknown,scope:RemoteToolScanScope){
 if(!object(value)||!closed(value,['project_id','worker_id','as_of','maintenance','items'],['next_cursor'])||value.project_id!==scope.project||value.worker_id!==scope.workerID||!time(value.as_of)||!Array.isArray(value.items)||value.items.length>20||Reflect.ownKeys(value.items).length!==value.items.length+1||Object.hasOwn(value,'next_cursor')&&(typeof value.next_cursor!=='string'||!value.next_cursor||new TextEncoder().encode(value.next_cursor).length>4096))return invalid();
 const maintenance=readToolMaintenanceStatus(value.maintenance);if(!maintenance||maintenance.status.checked_at!==value.as_of)return invalid();
 const items=await Promise.all(value.items.map(item=>readManagedToolPreparation(item,scope)));if(new Set(items.map(item=>item.id)).size!==items.length)return invalid();
 return {maintenance,items,asOf:value.as_of};
}
export function validManagedToolPreparationRequest(request:WriteRecoveryRequest,scope:RemoteToolScanScope){
 const body=request.body;
 return validRemoteToolScanScope(scope)&&request.path==='/tool-preparations'&&request.method==='POST'&&object(body)&&closed(body,['project_id','worker_id','installation_scan_id','expected_scan_version','adapter_id','configuration','expected_generation_sha256','acknowledge_configuration_write','idempotency_key'])&&body.project_id===scope.project&&body.worker_id===scope.workerID&&id(body.installation_scan_id)&&integer(body.expected_scan_version,1,2147483646)&&validManagedToolPreparationConfiguration(body.adapter_id,body.configuration)&&hash(body.expected_generation_sha256)&&body.acknowledge_configuration_write===true&&body.idempotency_key===request.idempotency_key&&operationID(request.idempotency_key);
}
export async function validManagedToolPreparationReceipt(value:unknown,request:WriteRecoveryRequest,scope:RemoteToolScanScope){
 if(!validManagedToolPreparationRequest(request,scope))return false;
 try{const row=await readManagedToolPreparation(value,scope),configuration=request.body.configuration as ManagedToolPreparationConfiguration;return row.requested_by===scope.userID&&row.installation_scan_id===request.body.installation_scan_id&&row.installation_scan_version===request.body.expected_scan_version&&row.adapter_id===request.body.adapter_id&&row.expected_generation_sha256===request.body.expected_generation_sha256&&row.configuration.model===configuration.model&&row.configuration.credential_env_ref===configuration.credential_env_ref;}catch{return false;}
}
export {validManagedToolPreparationCapability};
