import type {ToolInspection,ToolProfile} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {UiError} from './i18n';
import {readToolProfileMatches} from './tool-profile-reuse-model';

export type ToolReinspectionScope={project:string;toolID:string;userID:string;organizationID:string};
/** Read-only UI location; never an execution or recovery authorization. */
export type ToolInspectionLocator={inspectionID:string;recoveryID?:string};
export type ToolReinspectionIntent={scope:ToolReinspectionScope;tool:ToolProfile;request:WriteRecoveryRequest};
export const toolReinspectionRecoveryScope=(scope:ToolReinspectionScope)=>JSON.stringify([scope.organizationID,scope.userID,scope.project,scope.toolID]);
export const toolInspectionListPath=(scope:ToolReinspectionScope)=>'/tool-profiles/'+encodeURIComponent(scope.toolID)+'/inspections?project_id='+encodeURIComponent(scope.project);
export const toolInspectionPath=(scope:ToolReinspectionScope,id:string)=>'/tool-inspections/'+encodeURIComponent(id)+'?project_id='+encodeURIComponent(scope.project);
const record=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const closed=(v:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(v,key))&&Reflect.ownKeys(v).every(key=>typeof key==='string'&&[...required,...optional].includes(key));
const id=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$/.test(v);
const operation=(v:unknown):v is string=>typeof v==='string'&&new TextEncoder().encode(v).length>=8&&new TextEncoder().encode(v).length<=128&&v.trim()===v&&!/\p{Cc}/u.test(v)&&!/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(v);
const integer=(v:unknown,min:number)=>Number.isSafeInteger(v)&&(v as number)>=min;
function time(v:unknown):v is string{
 if(typeof v!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(v)||!Number.isFinite(Date.parse(v)))return false;
 return new Date(v).toISOString().slice(0,19)===v.slice(0,19);
}
function validScope(v:unknown):v is ToolReinspectionScope{return record(v)&&closed(v,['project','toolID','userID','organizationID'])&&Object.values(v).every(id);}
export function validToolInspectionLocator(v:unknown):v is ToolInspectionLocator{return record(v)&&closed(v,['inspectionID'],['recoveryID'])&&id(v.inspectionID)&&(!Object.hasOwn(v,'recoveryID')||id(v.recoveryID));}
/** An original local request, including unreadable metadata, takes precedence
 * over navigation to another server recommendation. Nothing is consumed here. */
export function hasToolInspectionLocalOperation(scope:ToolReinspectionScope,storage:Pick<Storage,'getItem'>|undefined):boolean{
 try{if(!validScope(scope)||!storage)return true;const base=toolReinspectionRecoveryScope(scope);return ['suma-tool-reinspection-intent:','suma-write-recovery-v1:tool-reinspection:','suma-tool-inspection-recovery-intent:','suma-write-recovery-v1:tool-inspection-recovery:'].some(prefix=>storage.getItem(prefix+base)!==null);}catch{return true;}
}
function validTool(v:unknown,scope:ToolReinspectionScope):v is ToolProfile{
 if(!record(v)||v.id!==scope.toolID||v.project_id!==scope.project||!id(v.worker_id))return false;
 try{readToolProfileMatches({project_id:scope.project,worker_id:v.worker_id,profile:v.profile,items:[v],total:1},{project_id:scope.project,worker_id:v.worker_id,profile:v.profile as ToolProfile['profile']});return true;}catch{return false;}
}
function validInspection(v:unknown,scope:ToolReinspectionScope,idExpected?:string):v is ToolInspection{
 if(!record(v)||!closed(v,['id','project_id','worker_id','tool_profile_id','tool_profile_version','profile_sha256','command_id','version','fence','state','requested_by','created_at','expires_at'],['claim_id','claimed_at','lease_until','observed_at','inventory_operation_id','result_code','recovery_id','closed_at','source_changed']))return false;
 if(!['id','project_id','worker_id','tool_profile_id','command_id','requested_by'].every(key=>id(v[key]))||v.project_id!==scope.project||v.tool_profile_id!==scope.toolID||idExpected!==undefined&&v.id!==idExpected||!integer(v.tool_profile_version,1)||!integer(v.version,1)||!integer(v.fence,0)||typeof v.profile_sha256!=='string'||!/^[a-f0-9]{64}$/.test(v.profile_sha256))return false;
 if(typeof v.state!=='string'||!['pending','claimed','completed','failed','unknown','expired','superseded','closed_unknown'].includes(v.state)||!time(v.created_at)||!time(v.expires_at)||Date.parse(v.expires_at)<=Date.parse(v.created_at)||Object.hasOwn(v,'source_changed')&&typeof v.source_changed!=='boolean')return false;
 if(['claimed_at','lease_until','observed_at'].some(key=>Object.hasOwn(v,key)&&!time(v[key]))||Object.hasOwn(v,'claim_id')&&!operation(v.claim_id)||Object.hasOwn(v,'inventory_operation_id')&&!operation(v.inventory_operation_id))return false;
 const claimed=(v.fence as number)>0;
 if(claimed&&(!operation(v.claim_id)||!time(v.claimed_at)||!time(v.lease_until))||!claimed&&['claim_id','claimed_at','lease_until'].some(key=>Object.hasOwn(v,key)))return false;
 if(claimed&&(Date.parse(v.claimed_at as string)<Date.parse(v.created_at)||Date.parse(v.lease_until as string)<=Date.parse(v.claimed_at as string)))return false;
 if(v.state!=='completed'&&['observed_at','inventory_operation_id'].some(key=>Object.hasOwn(v,key)))return false;
 if(v.state!=='closed_unknown'&&['recovery_id','closed_at'].some(key=>Object.hasOwn(v,key)))return false;
 const result=v.result_code??'';
 switch(v.state){
  case 'pending':return !claimed&&result==='';
  case 'claimed':return claimed&&result==='';
  case 'completed':return claimed&&time(v.observed_at)&&operation(v.inventory_operation_id)&&['reported_available','reported_unavailable','profile_changed','command_missing'].includes(String(result));
  case 'failed':return claimed&&result==='not_started';
  case 'unknown':return claimed&&['probe_interrupted','probe_timeout','probe_failed'].includes(String(result));
  case 'expired':return !claimed&&result==='request_expired';
  case 'superseded':return !claimed&&result==='source_changed';
  case 'closed_unknown':return claimed&&id(v.recovery_id)&&time(v.closed_at)&&['probe_interrupted','probe_timeout','probe_failed'].includes(String(result));
  default:return false;
 }
}
export function readToolInspection(value:unknown,scope:ToolReinspectionScope,idExpected?:string):ToolInspection{
 if(validScope(scope)&&validInspection(value,scope,idExpected))return value;
 throw new UiError('tool-reinspection.invalid_read');
}
/** Obtain the original profile only from a complete authorized inspection,
 * then use the ordinary exact-scope reader. Do not trust a naked profile ID. */
export function readToolInspectionTarget(value:unknown,scope:ToolReinspectionScope,locator:ToolInspectionLocator):ToolInspection{
 if(!validScope(scope)||!validToolInspectionLocator(locator)||!record(value)||!id(value.tool_profile_id))throw new UiError('tool-reinspection.invalid_read');
 return readToolInspection(value,{...scope,toolID:value.tool_profile_id},locator.inspectionID);
}
export function readToolInspections(value:unknown,scope:ToolReinspectionScope):ToolInspection[]{
 if(!validScope(scope)||!record(value)||!closed(value,['items'])||!Array.isArray(value.items)||value.items.length>10)throw new UiError('tool-reinspection.invalid_read');
 const seen=new Set<string>();return value.items.map(item=>{const row=readToolInspection(item,scope);if(seen.has(row.id))throw new UiError('tool-reinspection.invalid_read');seen.add(row.id);return row;});
}
export function inspectionMatchesTool(row:ToolInspection,tool:ToolProfile):boolean{return row.project_id===tool.project_id&&row.tool_profile_id===tool.id&&row.worker_id===tool.worker_id&&row.tool_profile_version===tool.version&&row.profile_sha256===tool.profile.profile_sha256&&row.command_id===tool.profile.command_id;}
export function validToolReinspectionRecovery(value:unknown,scope:ToolReinspectionScope):value is WriteRecoveryRequest{
 if(!validScope(scope)||!record(value)||!closed(value,['path','method','idempotency_key','body'])||value.path!=='/tool-profiles/'+encodeURIComponent(scope.toolID)+'/inspections'||value.method!=='POST'||!operation(value.idempotency_key)||!record(value.body)||!closed(value.body,['project_id','expected_version','idempotency_key']))return false;
 return value.body.project_id===scope.project&&integer(value.body.expected_version,1)&&value.body.idempotency_key===value.idempotency_key;
}
export function buildToolReinspectionIntent(tool:ToolProfile,scope:ToolReinspectionScope,request:WriteRecoveryRequest):ToolReinspectionIntent|undefined{
 if(!validScope(scope)||!validTool(tool,scope)||!validToolReinspectionRecovery(request,scope)||request.body.expected_version!==tool.version)return;
 return structuredClone({scope,tool,request});
}
export function validToolReinspectionIntent(value:unknown,scope:ToolReinspectionScope):value is ToolReinspectionIntent{
 return record(value)&&closed(value,['scope','tool','request'])&&validScope(value.scope)&&toolReinspectionRecoveryScope(value.scope)===toolReinspectionRecoveryScope(scope)&&validTool(value.tool,scope)&&validToolReinspectionRecovery(value.request,scope)&&value.request.body.expected_version===value.tool.version;
}
const canonical=(value:unknown)=>JSON.stringify(value,(_key,item)=>record(item)?Object.fromEntries(Object.keys(item).sort().map(key=>[key,item[key]])):item);
export function validToolInspectionReceipt(value:unknown,request:WriteRecoveryRequest,scope:ToolReinspectionScope,intent:ToolReinspectionIntent):boolean{
 return validToolReinspectionIntent(intent,scope)&&validToolReinspectionRecovery(request,scope)&&canonical(request)===canonical(intent.request)&&validInspection(value,scope)&&inspectionMatchesTool(value,intent.tool)&&value.requested_by===scope.userID&&value.version===1&&value.fence===0&&value.state==='pending'&&!['claim_id','claimed_at','lease_until','observed_at','inventory_operation_id','result_code','recovery_id','closed_at','source_changed'].some(key=>Object.hasOwn(value,key));
}
