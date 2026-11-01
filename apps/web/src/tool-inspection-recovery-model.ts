import type {ToolInspection,ToolInspectionRecovery,ToolInspectionStopProof} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {readToolInspection,type ToolReinspectionScope} from './tool-reinspection-model';
import {UiError} from './i18n';
export type ToolInspectionRecoveryIntent={scope:ToolReinspectionScope;inspection:ToolInspection;source_sha256:string;request:WriteRecoveryRequest};
const record=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const closed=(v:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(v,key))&&Reflect.ownKeys(v).every(key=>typeof key==='string'&&[...required,...optional].includes(key));
const id=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$/.test(v);
const operation=(v:unknown):v is string=>typeof v==='string'&&v.trim()===v&&!/\p{Cc}/u.test(v)&&new TextEncoder().encode(v).length>=8&&new TextEncoder().encode(v).length<=128;
const integer=(v:unknown,min=1)=>Number.isSafeInteger(v)&&(v as number)>=min;
const digest=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const canonical=(v:unknown)=>JSON.stringify(v,(_key,value)=>record(value)?Object.fromEntries(Object.keys(value).sort().map(key=>[key,value[key]])):value);
const fail=()=>new UiError('tool-reinspection.recovery.invalid_read');
function time(v:unknown):v is string{return typeof v==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(v)&&Number.isFinite(Date.parse(v))&&new Date(v).toISOString().slice(0,19)===v.slice(0,19);}
function reference(v:unknown):v is ToolInspectionRecovery['inspection']{return record(v)&&closed(v,['inspection_id','claim_id','fence'])&&id(v.inspection_id)&&operation(v.claim_id)&&integer(v.fence);}
function sameReference(a:ToolInspectionRecovery['inspection'],b:ToolInspectionRecovery['inspection']){return a.inspection_id===b.inspection_id&&a.claim_id===b.claim_id&&a.fence===b.fence;}
const ref=(i:ToolInspection)=>({inspection_id:i.id,claim_id:i.claim_id??'',fence:i.fence});
async function sha(value:unknown){const encoded=JSON.stringify(value).replaceAll('<','\\u003c').replaceAll('>','\\u003e').replaceAll('&','\\u0026').replaceAll('\u2028','\\u2028').replaceAll('\u2029','\\u2029');return Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(encoded))),value=>value.toString(16).padStart(2,'0')).join('');}
/** Match Go InspectionSourceSHA256's explicit exported struct field order. */
export async function toolInspectionSourceDigest(i:ToolInspection):Promise<string>{return sha({ID:i.id,ProjectID:i.project_id,WorkerID:i.worker_id,ToolProfileID:i.tool_profile_id,ToolProfileVersion:i.tool_profile_version,ProfileSHA256:i.profile_sha256,CommandID:i.command_id,RequestedBy:i.requested_by,CreatedAt:i.created_at,ExpiresAt:i.expires_at,ClaimID:i.claim_id??'',ClaimedAt:i.claimed_at??'',LeaseUntil:i.lease_until??'',Fence:i.fence});}
export async function toolInspectionStopProofDigest(p:ToolInspectionStopProof):Promise<string>{return sha({protocol:p.protocol,inspection:{inspection_id:p.inspection.inspection_id,claim_id:p.inspection.claim_id,fence:p.inspection.fence},source_sha256:p.source_sha256,receipt_sha256:p.receipt_sha256,probe_returned:p.probe_returned,all_native_trees_stopped:p.all_native_trees_stopped,no_pending_inventory:p.no_pending_inventory,native_process_count:p.native_process_count,stopped_process_count:p.stopped_process_count,observed_at:p.observed_at});}
function validProof(v:unknown):v is ToolInspectionStopProof{return record(v)&&closed(v,['protocol','inspection','source_sha256','receipt_sha256','probe_returned','all_native_trees_stopped','no_pending_inventory','native_process_count','stopped_process_count','observed_at'])&&v.protocol==='suma-tool-probe-stop-v1'&&reference(v.inspection)&&digest(v.source_sha256)&&digest(v.receipt_sha256)&&v.probe_returned===true&&v.all_native_trees_stopped===true&&v.no_pending_inventory===true&&integer(v.native_process_count,0)&&(v.native_process_count as number)<=2048&&v.stopped_process_count===v.native_process_count&&time(v.observed_at);}
export function validToolInspectionRecoveryWrite(request:unknown,scope:ToolReinspectionScope):request is WriteRecoveryRequest{
 if(!record(request)||!closed(request,['path','method','body','idempotency_key'])||request.method!=='POST'||!operation(request.idempotency_key)||!record(request.body)||!closed(request.body,['project_id','expected_version','inspection','idempotency_key'])||!reference(request.body.inspection))return false;
 return request.path==='/tool-inspections/'+encodeURIComponent(request.body.inspection.inspection_id)+'/recoveries'&&request.body.project_id===scope.project&&integer(request.body.expected_version)&&request.body.idempotency_key===request.idempotency_key;
}
export async function readToolInspectionRecovery(raw:unknown,scope:ToolReinspectionScope,inspection:ToolInspection):Promise<ToolInspectionRecovery>{
 readToolInspection(inspection,scope);if(!record(raw)||!closed(raw,['id','project_id','worker_id','inspection','inspection_version','source_sha256','version','state','requested_by','created_at','expires_at'],['proof','proof_sha256','proof_stored_at','closed_at'])||!id(raw.id)||raw.project_id!==scope.project||raw.worker_id!==inspection.worker_id||!id(raw.requested_by)||!reference(raw.inspection)||!sameReference(raw.inspection,ref(inspection))||!integer(raw.inspection_version)||!integer(raw.version)||!digest(raw.source_sha256)||raw.source_sha256!==await toolInspectionSourceDigest(inspection)||!time(raw.created_at)||!time(raw.expires_at)||Date.parse(raw.expires_at)<=Date.parse(raw.created_at))throw fail();
 if(Object.hasOwn(raw,'proof')){
  if(!validProof(raw.proof)||!sameReference(raw.proof.inspection,raw.inspection)||raw.proof.source_sha256!==raw.source_sha256||!digest(raw.proof_sha256)||raw.proof_sha256!==await toolInspectionStopProofDigest(raw.proof)||!time(raw.proof_stored_at))throw fail();
 }else if(Object.hasOwn(raw,'proof_sha256')||Object.hasOwn(raw,'proof_stored_at'))throw fail();
 if(Object.hasOwn(raw,'closed_at')&&!time(raw.closed_at))throw fail();
 switch(raw.state){case 'requested':if(Object.hasOwn(raw,'proof')||Object.hasOwn(raw,'closed_at'))throw fail();break;case 'proof_stored':if(!raw.proof||Object.hasOwn(raw,'closed_at'))throw fail();break;case 'closed_unknown':if(!raw.proof||!time(raw.closed_at))throw fail();break;case 'expired':if(Object.hasOwn(raw,'closed_at'))throw fail();break;default:throw fail();}
 return raw as unknown as ToolInspectionRecovery;
}
export async function readToolInspectionRecoveries(raw:unknown,scope:ToolReinspectionScope,inspection:ToolInspection):Promise<ToolInspectionRecovery[]>{
 if(!record(raw)||!closed(raw,['items'])||!Array.isArray(raw.items)||raw.items.length>10)throw fail();const seen=new Set<string>(),items:ToolInspectionRecovery[]=[];
 for(const item of raw.items){const row=await readToolInspectionRecovery(item,scope,inspection);if(seen.has(row.id))throw fail();seen.add(row.id);items.push(row);}return items;
}
export async function buildToolInspectionRecoveryIntent(inspection:ToolInspection,scope:ToolReinspectionScope,request:WriteRecoveryRequest):Promise<ToolInspectionRecoveryIntent|undefined>{
 try{readToolInspection(inspection,scope);if(inspection.state!=='unknown'||!validToolInspectionRecoveryWrite(request,scope)||!sameReference(request.body.inspection as ToolInspectionRecovery['inspection'],ref(inspection))||request.body.expected_version!==inspection.version)return;return structuredClone({scope,inspection,source_sha256:await toolInspectionSourceDigest(inspection),request});}catch{return;}
}
export async function readToolInspectionRecoveryIntent(raw:unknown,scope:ToolReinspectionScope):Promise<ToolInspectionRecoveryIntent>{
 if(!record(raw)||!closed(raw,['scope','inspection','source_sha256','request'])||canonical(raw.scope)!==canonical(scope)||!record(raw.inspection)||!validToolInspectionRecoveryWrite(raw.request,scope))throw fail();const built=await buildToolInspectionRecoveryIntent(raw.inspection as unknown as ToolInspection,scope,raw.request);if(!built||built.source_sha256!==raw.source_sha256)throw fail();return built;
}
export async function validToolInspectionRecoveryReceipt(raw:unknown,request:WriteRecoveryRequest,scope:ToolReinspectionScope,intent:ToolInspectionRecoveryIntent):Promise<boolean>{
 try{const original=await readToolInspectionRecoveryIntent(intent,scope);if(!validToolInspectionRecoveryWrite(request,scope)||canonical(request)!==canonical(original.request))return false;const row=await readToolInspectionRecovery(raw,scope,original.inspection);return row.state==='requested'&&row.version===1&&[original.inspection.version,original.inspection.version+1].includes(row.inspection_version)&&row.requested_by===scope.userID;}catch{return false;}
}
