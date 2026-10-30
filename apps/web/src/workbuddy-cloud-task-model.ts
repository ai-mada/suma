import type {CloudDelegateRequest,CloudDelegation,CloudNodeView,CloudSubmission,WorkBuddyConnection} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
import {readWorkBuddyConnection} from './workbuddy-connections-model';
import type {WriteRecoveryRequest} from './write-action';

export type CloudTaskScope={project:string;runID:string;nodeID:string;userID:string;organizationID:string};
export type CloudRunFacts={runVersion:number;runState:string;nodeState:string;reason:string;attempt:number;fence:number;workerID:string};
export type CloudRunnerPhase='not_delegated'|'delegated'|'revoked'|'unknown'|'known'|'not_created'|'waiting_qualification'|'awaiting_submission'|'run_paused'|'remote_unknown'|'waiting_input'|'delivery_required'|'interrupted'|'authorization_changed'|'observation_unconfirmed'|'runner_disconnected'|'stop_unconfirmed';
type CloudRunnerSource={execution?:CloudRunFacts;delegation?:{state:string};submission?:{state:string;providerStatus?:string}};
/** A presentation of persisted facts, never a second execution state machine. */
export function cloudRunnerPhase({execution,delegation,submission}:CloudRunnerSource):CloudRunnerPhase{
 const reason=execution?.reason;
 if(reason==='cloud.stop_unconfirmed'||execution?.runState==='cancelling'&&!!submission)return 'stop_unconfirmed';
 if(reason==='cloud.runner_disconnected')return 'runner_disconnected';
 if(submission?.state==='prepared'||submission?.state==='unknown'||reason==='cloud.creation_unconfirmed')return 'unknown';
 const reasons:Record<string,CloudRunnerPhase>={'cloud.remote_unknown':'remote_unknown','cloud.waiting_input':'waiting_input','cloud.delivery_required':'delivery_required','cloud.interrupted':'interrupted','cloud.authorization_changed':'authorization_changed','cloud.observation_unconfirmed':'observation_unconfirmed'};
 if(reason&&reasons[reason])return reasons[reason];
 if(submission?.state==='task_known')return 'known';
 if(!submission&&reason==='cloud.not_created'&&['failed','cancelled'].includes(execution?.nodeState||''))return 'not_created';
 if(delegation?.state==='revoked')return 'revoked';
 if(delegation){
  if(execution?.runState==='paused')return 'run_paused';
  if(execution?.nodeState==='running'&&execution.attempt>0&&execution.fence>0&&execution.workerID)return 'awaiting_submission';
  return execution?'waiting_qualification':'delegated';
 }
 return 'not_delegated';
}
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const fields=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Object.keys(value).every(key=>required.includes(key)||optional.includes(key));
const bytes=(value:string)=>new TextEncoder().encode(value).length;
const text=(value:unknown,max:number,empty=false):value is string=>typeof value==='string'&&wellFormed(value)&&bytes(value)<=max&&(empty||value.trim()!=='')&&!/[\p{Cc}]/u.test(value);
const identifier=(value:unknown,max=64):value is string=>text(value,max)&&value.trim()===value&&!/[\/\\]/.test(value);
const version=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=1&&value<=2147483647;
const expectedVersion=(value:unknown):value is number=>version(value)&&value<2147483647;
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const sameScope=(value:Record<string,unknown>,scope:CloudTaskScope)=>value.project_id===scope.project&&value.run_id===scope.runID&&value.node_id===scope.nodeID;
export const validCloudScope=(scope:CloudTaskScope)=>identifier(scope.project)&&identifier(scope.runID,100)&&identifier(scope.nodeID,150)&&identifier(scope.userID)&&identifier(scope.organizationID);
const timestamp=(value:unknown):value is string=>{
 if(typeof value!=='string'||value.length>40||!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value))return false;
 const date=new Date(value);if(!Number.isFinite(date.getTime())||value.startsWith('0001-01-01T00:00:00'))return false;
 return date.toISOString().slice(0,19)===value.slice(0,19);
};
// Provider task timestamps are RFC3339 source values, including explicit
// offsets. They are distinct from the platform's own UTC-only timestamps.
const supplierTimestamp=(value:unknown):value is string=>{
 if(typeof value!=='string'||value.length>64)return false;
 const match=/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?)(Z|[+-]\d{2}:\d{2})$/.exec(value);
 if(!match)return false;
 const zone=match[2];
 if(zone!=='Z'&&(Number(zone.slice(1,3))>23||Number(zone.slice(4,6))>59))return false;
 // Check calendar components before the offset is applied: Date.parse alone
 // normalizes invalid dates such as February30 instead of rejecting them.
 const local=new Date(match[1]+'Z');
 return Number.isFinite(local.getTime())&&local.toISOString().slice(0,19)===value.slice(0,19)&&Number.isFinite(Date.parse(value));
};
const operation=(value:unknown):value is string=>identifier(value,128)&&bytes(value)>=8;
const strings=(value:unknown,allowed:string[])=>Array.isArray(value)&&value.length<=allowed.length&&value.every(item=>typeof item==='string'&&allowed.includes(item))&&new Set(value).size===value.length;

/** These exact two Go fields are encoded in declaration order. Go escapes
 * HTML and line separators, including when name is omitted. */
export async function cloudRequestDigest(request:CloudNodeView['binding']['request']):Promise<string>{
 const value:unknown=request;
 if(!object(value)||!fields(value,['prompt'],['name'])||typeof value.prompt!=='string'||!wellFormed(value.prompt)||!value.prompt.trim()||bytes(value.prompt)>256*1024||/[\p{Cc}]/u.test(value.prompt.replace(/[\r\n\t]/g,''))||value.name!==undefined&&(!text(value.name,2048,true)||Array.from(value.name).length>512||value.name!==''&&!value.name.trim()))throw Error('invalid cloud request');
 const body={prompt:value.prompt,...(value.name?{name:value.name}:{})};
 const escaped:Record<string,string>={'<':'\\u003c','>':'\\u003e','&':'\\u0026','\u2028':'\\u2028','\u2029':'\\u2029'};
 const encoded=JSON.stringify(body).replace(/[<>&\u2028\u2029]/g,character=>escaped[character]);
 const digest=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(encoded));
 return Array.from(new Uint8Array(digest),byte=>byte.toString(16).padStart(2,'0')).join('');
}

export function readCloudDelegation(value:unknown,scope:CloudTaskScope):CloudDelegation|undefined{
 const keys=['id','project_id','owner_id','connection_id','version','run_id','node_id','node_spec_sha256','connection_version_at_issue','grant_generation','client_binding_sha256','request_sha256','expires_at','max_creates','creates_consumed','state','created_at','updated_at'];
 if(!validCloudScope(scope)||!object(value)||!fields(value,keys)||!sameScope(value,scope)||!identifier(value.id)||value.owner_id!==scope.userID||!identifier(value.connection_id)||!version(value.version)||!version(value.connection_version_at_issue)||!version(value.grant_generation)||![value.node_spec_sha256,value.client_binding_sha256,value.request_sha256].every(hash)||![value.expires_at,value.created_at,value.updated_at].every(timestamp)||value.max_creates!==1||value.creates_consumed!==0&&value.creates_consumed!==1||value.state!=='active'&&value.state!=='revoked')return;
 return value as unknown as CloudDelegation;
}

function assignment(value:unknown,scope:CloudTaskScope):boolean{
 const required=['run_id','node_id','attempt','fence','worker_id'],pool=['tenant_id','project_id','pool_id','pool_runner_id','pool_slot_id','pool_grant_id','pool_grant_version','pool_credential_id','pool_credential_version','dispatch_id','dispatch_state','lease_until'];
 if(!object(value)||!fields(value,required,pool)||value.run_id!==scope.runID||value.node_id!==scope.nodeID||!version(value.attempt)||typeof value.fence!=='number'||!Number.isSafeInteger(value.fence)||value.fence<1||!identifier(value.worker_id,100))return false;
 if(!pool.some(key=>Object.hasOwn(value,key)))return true;
 return pool.every(key=>Object.hasOwn(value,key))&&value.tenant_id===scope.organizationID&&value.project_id===scope.project&&['pool_id','pool_runner_id','pool_slot_id','pool_grant_id','pool_credential_id','dispatch_id','dispatch_state'].every(key=>identifier(value[key],100))&&version(value.pool_grant_version)&&version(value.pool_credential_version)&&timestamp(value.lease_until);
}
function supplierTask(value:unknown):boolean{
 if(!object(value)||!fields(value,['task_id','status'],['name','created_at','updated_at'])||typeof value.task_id!=='string'||! /^[A-Za-z0-9_-]{1,200}$/.test(value.task_id)||!text(value.status,512)||Array.from(value.status).length>128)return false;
 return (value.name===undefined||text(value.name,2048,true)&&Array.from(value.name).length<=512)&&['created_at','updated_at'].every(key=>value[key]===undefined||value[key]===''||supplierTimestamp(value[key]));
}
export function readCloudSubmission(value:unknown,scope:CloudTaskScope):CloudSubmission|undefined{
 const keys=['id','project_id','delegation_id','version','run_id','node_id','assignment','request_sha256','state','observed_at','last_error','usage_state','created_at','updated_at'];
 if(!validCloudScope(scope)||!object(value)||!fields(value,keys,['task'])||!sameScope(value,scope)||!identifier(value.id)||!identifier(value.delegation_id)||!version(value.version)||!assignment(value.assignment,scope)||!hash(value.request_sha256)||![value.created_at,value.updated_at].every(timestamp)||value.usage_state!=='unreported'||typeof value.last_error!=='string'||value.last_error!==''&&!/^[a-z][a-z0-9_]{0,79}$/.test(value.last_error))return;
 if(value.state==='task_known'){
  if(!supplierTask(value.task)||!timestamp(value.observed_at))return;
 }else if(value.state==='prepared'||value.state==='unknown'){
  if(Object.hasOwn(value,'task')||value.observed_at!=='')return;
 }else return;
 return value as unknown as CloudSubmission;
}
export async function readCloudNode(value:unknown,scope:CloudTaskScope):Promise<CloudNodeView|undefined>{
 const keys=['project_id','run_id','node_id','run_version','node_spec_sha256','request_sha256','binding','allowed_actions'];
 if(!validCloudScope(scope)||!object(value)||!fields(value,keys,['delegation','submission'])||!sameScope(value,scope)||!version(value.run_version)||!hash(value.node_spec_sha256)||!hash(value.request_sha256)||!object(value.binding)||!fields(value.binding,['connection_id','grant_generation','request'])||!identifier(value.binding.connection_id)||!version(value.binding.grant_generation)||!strings(value.allowed_actions,['delegate','revoke','observe']))return;
 try{if(await cloudRequestDigest(value.binding.request as CloudNodeView['binding']['request'])!==value.request_sha256)return;}catch{return;}
 let delegation:CloudDelegation|undefined,submission:CloudSubmission|undefined;
 if(Object.hasOwn(value,'delegation')){
  delegation=readCloudDelegation(value.delegation,scope);
  if(!delegation||delegation.node_spec_sha256!==value.node_spec_sha256||delegation.request_sha256!==value.request_sha256||delegation.connection_id!==value.binding.connection_id||delegation.grant_generation!==value.binding.grant_generation)return;
 }
 if(Object.hasOwn(value,'submission')){
  submission=readCloudSubmission(value.submission,scope);
  if(!submission||submission.request_sha256!==value.request_sha256||delegation&&submission.delegation_id!==delegation.id)return;
 }
 const actions=value.allowed_actions as string[];
 if(actions.includes('delegate')&&(delegation||submission)||actions.includes('revoke')&&(!delegation||delegation.state!=='active')||actions.includes('observe')&&(!delegation||submission?.state!=='task_known'))return;
 return value as unknown as CloudNodeView;
}

export function readCloudConnection(value:unknown,scope:CloudTaskScope,node:CloudNodeView):WorkBuddyConnection|undefined{
 const keys=['id','project_id','version','owner_id','client_id','open_id','scopes','state','grant_generation','token_generation','expires_at','created_at','updated_at','last_error','allowed_actions'];
 if(!object(value)||!fields(value,keys))return;
 const connection=readWorkBuddyConnection(value,scope.project,scope.userID);
 return connection&&connection.id===node.binding.connection_id&&version(connection.version)&&version(connection.grant_generation)?connection:undefined;
}

export function cloudNodePath(scope:CloudTaskScope):string{
 return '/workbuddy-cloud-nodes?'+new URLSearchParams({project_id:scope.project,run_id:scope.runID,node_id:scope.nodeID});
}
export function cloudRecoveryAction(request:WriteRecoveryRequest):'delegate'|'revoke'|'refresh'|undefined{
 if(request.path==='/workbuddy-cloud-delegations')return 'delegate';
 if(/^\/workbuddy-cloud-delegations\/[^/]+\/revoke$/.test(request.path))return 'revoke';
 if(/^\/workbuddy-cloud-submissions\/[^/]+\/observe$/.test(request.path))return 'refresh';
}
function recoveryID(request:WriteRecoveryRequest):string|undefined{
 try{const decoded=decodeURIComponent(request.path.split('/')[2]||'');return identifier(decoded)?decoded:undefined;}catch{return;}
}
export function validCloudRecovery(request:WriteRecoveryRequest,scope:CloudTaskScope,node?:CloudNodeView):boolean{
 if(!validCloudScope(scope)||!object(request)||!fields(request as unknown as Record<string,unknown>,['path','body','method','idempotency_key'])||request.method!=='POST'||!object(request.body)||request.body.project_id!==scope.project||request.body.idempotency_key!==request.idempotency_key||!operation(request.idempotency_key))return false;
 const b=request.body,action=cloudRecoveryAction(request);
 if(action==='delegate'){
  const keys=['project_id','connection_id','expected_connection_version','grant_generation','run_id','node_id','expected_run_version','node_spec_sha256','request_sha256','expires_at','max_creates','idempotency_key'];
  return fields(b,keys)&&sameScope(b,scope)&&identifier(b.connection_id)&&expectedVersion(b.expected_connection_version)&&version(b.grant_generation)&&expectedVersion(b.expected_run_version)&&hash(b.node_spec_sha256)&&hash(b.request_sha256)&&timestamp(b.expires_at)&&b.max_creates===1&&(!node||b.connection_id===node.binding.connection_id&&b.grant_generation===node.binding.grant_generation&&b.node_spec_sha256===node.node_spec_sha256&&b.request_sha256===node.request_sha256);
 }
 const id=recoveryID(request);
 if(!action||!id||!fields(b,['project_id','expected_version','idempotency_key'])||!expectedVersion(b.expected_version))return false;
 return !node||(action==='revoke'?node.delegation?.id===id:node.submission?.id===id&&node.submission.state==='task_known');
}
export function validCloudReceipt(value:unknown,request:WriteRecoveryRequest,scope:CloudTaskScope,node?:CloudNodeView):boolean{
 if(!validCloudRecovery(request,scope,node))return false;
 const action=cloudRecoveryAction(request),body=request.body;
 if(action==='refresh'){
  const result=readCloudSubmission(value,scope),original=node?.submission;
  const sameAssignment=!!result&&!!original&&Object.keys(result.assignment).length===Object.keys(original.assignment).length&&Object.entries(result.assignment).every(([key,value])=>(original.assignment as unknown as Record<string,unknown>)[key]===value);
  return !!result&&!!original&&!!node?.delegation&&sameAssignment&&result.id===original.id&&result.delegation_id===original.delegation_id&&result.request_sha256===original.request_sha256&&result.state==='task_known'&&result.task?.task_id===original.task?.task_id&&result.version>Number(body.expected_version);
 }
 const result=readCloudDelegation(value,scope);if(!result)return false;
 if(action==='revoke')return !!node?.delegation&&result.id===node.delegation.id&&result.connection_id===node.binding.connection_id&&result.grant_generation===node.binding.grant_generation&&result.client_binding_sha256===node.delegation.client_binding_sha256&&result.node_spec_sha256===node.node_spec_sha256&&result.request_sha256===node.request_sha256&&result.state==='revoked'&&result.version>Number(body.expected_version);
 return result.connection_id===body.connection_id&&result.grant_generation===body.grant_generation&&result.connection_version_at_issue===body.expected_connection_version&&result.node_spec_sha256===body.node_spec_sha256&&result.request_sha256===body.request_sha256&&Date.parse(result.expires_at)===Date.parse(String(body.expires_at))&&result.max_creates===1;
}

export function cloudDelegateBody(node:CloudNodeView,connection:WorkBuddyConnection,expiresAt:string):Omit<CloudDelegateRequest,'idempotency_key'>{
 return {project_id:node.project_id,connection_id:connection.id,expected_connection_version:connection.version,grant_generation:connection.grant_generation,run_id:node.run_id,node_id:node.node_id,expected_run_version:node.run_version,node_spec_sha256:node.node_spec_sha256,request_sha256:node.request_sha256,expires_at:expiresAt,max_creates:1};
}
