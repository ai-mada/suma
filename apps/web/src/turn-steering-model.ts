import type {SteeringContext,SteeringInstruction,SteeringMessage,SteeringSession} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';
export type TurnSteeringScope={project:string;task:string;userID:string;organizationID:string};
export type TurnSteeringIntent={scope:TurnSteeringScope;session:SteeringSession;message:Pick<SteeringMessage,'id'|'sequence'|'author_id'|'body_sha256'>;request:WriteRecoveryRequest};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const fields=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const bytes=(v:string)=>new TextEncoder().encode(v).length;
const trim=(v:string)=>v.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g,'');
const id=(v:unknown,max=100):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=max&&!!v&&trim(v)===v&&!/\p{Cc}/u.test(v);
const nativeID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(v);
const positive=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>0;
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const body=(v:unknown):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=32000&&Array.from(v).length<=8000&&!!trim(v)&&!v.includes('\0');
const stamp=(v:unknown):v is string=>typeof v==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(v)&&Number.isFinite(Date.parse(v))&&new Date(Date.parse(v.slice(0,19)+'Z')).toISOString().slice(0,19)===v.slice(0,19);
const same=(a:unknown,b:unknown):boolean=>a===b||Array.isArray(a)&&Array.isArray(b)&&a.length===b.length&&a.every((v,i)=>same(v,b[i]))||object(a)&&object(b)&&Object.keys(a).length===Object.keys(b).length&&Object.keys(a).every(k=>Object.hasOwn(b,k)&&same(a[k],b[k]));
function validScope(v:unknown):v is TurnSteeringScope{return object(v)&&fields(v,['project','task','userID','organizationID'])&&id(v.project,64)&&id(v.task,64)&&id(v.userID)&&id(v.organizationID);}
function rpcID(v:unknown):boolean{
 if(!object(v)||!fields(v,['kind','value'])||typeof v.value!=='string')return false;
 if(v.kind==='string')return wellFormed(v.value)&&bytes(v.value)<=256&&!!trim(v.value)&&!/\p{Cc}/u.test(v.value);
 if(v.kind!=='number'||!/^(-?[1-9]\d*|0)$/.test(v.value))return false;
 try{const n=BigInt(v.value);return n>=-9223372036854775808n&&n<=9223372036854775807n;}catch{return false;}
}
function source(v:unknown):boolean{return object(v)&&fields(v,['protocol','thread_id','turn_id','binding_sha256'])&&v.protocol==='codex-turn-steer-v1'&&nativeID(v.thread_id)&&nativeID(v.turn_id)&&hash(v.binding_sha256);}
function messageMetadata(v:unknown):v is TurnSteeringIntent['message']{return object(v)&&fields(v,['id','sequence','author_id','body_sha256'])&&id(v.id)&&positive(v.sequence)&&id(v.author_id)&&hash(v.body_sha256);}
function message(v:unknown):v is SteeringMessage{return object(v)&&fields(v,['id','sequence','author_id','body','body_sha256'])&&id(v.id)&&positive(v.sequence)&&id(v.author_id)&&body(v.body)&&hash(v.body_sha256);}
function observation(v:unknown):boolean{
 if(!object(v)||!fields(v,['sequence','request_id','params_sha256','observed','write_started','write_written','native_accepted','native_rejected','turn_closed'],['failure_code'])||!positive(v.sequence)||!rpcID(v.request_id)||!hash(v.params_sha256)||!['observed','write_started','write_written','native_accepted','native_rejected','turn_closed'].every(k=>typeof v[k]==='boolean')||v.observed!==true||v.write_written&&!v.write_started||(v.native_accepted||v.native_rejected)&&!v.write_started||v.native_accepted&&v.native_rejected)return false;
 return !Object.hasOwn(v,'failure_code')||typeof v.failure_code==='string'&&(v.failure_code===''||/^[a-z][a-z0-9_]{0,79}$/.test(v.failure_code));
}
export function turnSteeringRecoveryScope(scope:TurnSteeringScope):string{
 if(!validScope(scope))throw new UiError('turn-steering.invalid_read');
 return JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.task]);
}
export function turnSteeringContextPath(scope:TurnSteeringScope,messageID:string):string{
 turnSteeringRecoveryScope(scope);if(!id(messageID))throw new UiError('turn-steering.invalid_read');
 return '/tasks/'+encodeURIComponent(scope.task)+'/execution-steering?project_id='+encodeURIComponent(scope.project)+'&message_id='+encodeURIComponent(messageID);
}
export function validTurnSteeringSession(v:unknown,scope:TurnSteeringScope):v is SteeringSession{
 return validScope(scope)&&object(v)&&fields(v,['id','project_id','task_id','run_id','node_id','attempt','fence','worker_id','dispatch_id','profile_sha256','source','version','state','opened_at','expires_at','allowed_actions'])&&id(v.id)&&v.project_id===scope.project&&v.task_id===scope.task&&id(v.run_id)&&id(v.node_id,150)&&positive(v.attempt)&&positive(v.fence)&&id(v.worker_id)&&typeof v.dispatch_id==='string'&&wellFormed(v.dispatch_id)&&hash(v.profile_sha256)&&source(v.source)&&positive(v.version)&&(v.state==='active'||v.state==='closed')&&stamp(v.opened_at)&&stamp(v.expires_at)&&Array.isArray(v.allowed_actions)&&v.allowed_actions.length<=1&&v.allowed_actions.every(a=>a==='send'&&v.state==='active');
}
export function validTurnSteeringInstruction(v:unknown,scope:TurnSteeringScope):v is SteeringInstruction{
 return validScope(scope)&&object(v)&&fields(v,['id','session_id','project_id','task_id','run_id','node_id','attempt','fence','version','message','actor_id','created_at','expires_at'],['observation'])&&id(v.id)&&id(v.session_id)&&v.project_id===scope.project&&v.task_id===scope.task&&id(v.run_id)&&id(v.node_id,150)&&positive(v.attempt)&&positive(v.fence)&&positive(v.version)&&message(v.message)&&v.actor_id===v.message.author_id&&stamp(v.created_at)&&stamp(v.expires_at)&&(!Object.hasOwn(v,'observation')||observation(v.observation));
}
const sameTuple=(instruction:SteeringInstruction,session:SteeringSession)=>instruction.session_id===session.id&&instruction.project_id===session.project_id&&instruction.task_id===session.task_id&&instruction.run_id===session.run_id&&instruction.node_id===session.node_id&&instruction.attempt===session.attempt&&instruction.fence===session.fence;
export function readTurnSteeringContext(v:unknown,scope:TurnSteeringScope,messageID:string):SteeringContext{
 if(!validScope(scope)||!id(messageID)||!object(v)||!fields(v,['task_id','task_version','message','targets','instructions','can_send'],['reason'])||v.task_id!==scope.task||!positive(v.task_version)||!message(v.message)||v.message.id!==messageID||typeof v.can_send!=='boolean'||(v.can_send?Object.hasOwn(v,'reason'):!['not_own_message','task_accepted','no_live_turn','current_execution_unavailable'].includes(typeof v.reason==='string'?v.reason:''))||!Array.isArray(v.targets)||!v.targets.every(t=>validTurnSteeringSession(t,scope))||new Set(v.targets.map(t=>t.id)).size!==v.targets.length||!Array.isArray(v.instructions)||!v.instructions.every(i=>validTurnSteeringInstruction(i,scope))||new Set(v.instructions.map(i=>i.id)).size!==v.instructions.length)throw new UiError('turn-steering.invalid_read');
 const context=v as unknown as SteeringContext,sessionIDs=new Set<string>();
 for(const instruction of context.instructions){
  if(!same(instruction.message,context.message)||!context.targets.some(t=>sameTuple(instruction,t))||sessionIDs.has(instruction.session_id))throw new UiError('turn-steering.invalid_read');
  sessionIDs.add(instruction.session_id);
 }
 return context;
}
export function canSendTurnSteering(context:SteeringContext,target:SteeringSession,scope:TurnSteeringScope):boolean{
 try{readTurnSteeringContext(context,scope,context.message.id);}catch{return false;}
 return context.can_send&&context.message.author_id===scope.userID&&validTurnSteeringSession(target,scope)&&target.state==='active'&&target.allowed_actions.includes('send')&&context.targets.some(t=>same(t,target))&&!context.instructions.some(i=>i.session_id===target.id);
}
const requestKeys=['project_id','task_id','message_id','expected_task_version','session_id','expected_session_version','idempotency_key'] as const;
export function validTurnSteeringRecovery(v:unknown,scope:TurnSteeringScope):v is WriteRecoveryRequest{
 return validScope(scope)&&object(v)&&fields(v,['path','method','idempotency_key','body'])&&v.path==='/task-execution-steering'&&v.method==='POST'&&id(v.idempotency_key,128)&&bytes(v.idempotency_key)>=8&&object(v.body)&&fields(v.body,requestKeys)&&v.body.project_id===scope.project&&v.body.task_id===scope.task&&id(v.body.message_id)&&positive(v.body.expected_task_version)&&id(v.body.session_id)&&positive(v.body.expected_session_version)&&v.body.idempotency_key===v.idempotency_key;
}
export function buildTurnSteeringIntent(context:SteeringContext,target:SteeringSession,scope:TurnSteeringScope,request:WriteRecoveryRequest):TurnSteeringIntent|undefined{
 if(!canSendTurnSteering(context,target,scope)||!validTurnSteeringRecovery(request,scope)||request.body.message_id!==context.message.id||request.body.expected_task_version!==context.task_version||request.body.session_id!==target.id||request.body.expected_session_version!==target.version)return;
 const m=context.message,value={scope,session:target,message:{id:m.id,sequence:m.sequence,author_id:m.author_id,body_sha256:m.body_sha256},request};
 return validTurnSteeringIntent(value,scope)?structuredClone(value):undefined;
}
export function validTurnSteeringIntent(v:unknown,scope:TurnSteeringScope):v is TurnSteeringIntent{
 return validScope(scope)&&object(v)&&fields(v,['scope','session','message','request'])&&validScope(v.scope)&&same(v.scope,scope)&&validTurnSteeringSession(v.session,v.scope)&&messageMetadata(v.message)&&v.message.author_id===scope.userID&&validTurnSteeringRecovery(v.request,v.scope)&&v.request.body.message_id===v.message.id&&v.request.body.session_id===v.session.id&&v.request.body.expected_session_version===v.session.version;
}
export function turnSteeringInstructionMatchesIntent(v:unknown,intent:TurnSteeringIntent):boolean{
 if(!validTurnSteeringIntent(intent,intent.scope)||!validTurnSteeringInstruction(v,intent.scope)||!sameTuple(v,intent.session))return false;
 return v.message.id===intent.message.id&&v.message.sequence===intent.message.sequence&&v.message.author_id===intent.message.author_id&&v.message.body_sha256===intent.message.body_sha256&&v.actor_id===intent.scope.userID;
}
export function validTurnSteeringReceipt(v:unknown,request:WriteRecoveryRequest,scope:TurnSteeringScope,intent:TurnSteeringIntent):boolean{
 return validTurnSteeringIntent(intent,scope)&&validTurnSteeringRecovery(request,scope)&&same(request,intent.request)&&turnSteeringInstructionMatchesIntent(v,intent)&&validTurnSteeringInstruction(v,scope)&&v.version===1&&!Object.hasOwn(v,'observation');
}
