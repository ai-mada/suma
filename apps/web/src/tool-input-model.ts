import type {ToolInput,ToolInputList,ToolInputRequest,CodexUserInputRequest,CodexUserInputSelection} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';
export type ToolInputScope={project:string;runID:string;nodeID:string;attempt:number;fence:number;userID:string;organizationID:string;protocol:'codex-app-server-v1'};
type SourceIdentity={idempotency_key:string;protocol:'codex-app-server-v1';codex:Pick<CodexUserInputRequest,'request_id'|'method'|'thread_id'|'turn_id'|'item_id'|'binding_sha256'|'params_sha256'>};
export type ToolInputIntent={scope:ToolInputScope;input_id:string;profile_sha256:string;dispatch_id:string;worker_id:string;source_identity:SourceIdentity;request:WriteRecoveryRequest};
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const fields=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const integer=(v:unknown,min=0):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min;
const bytes=(v:string)=>new TextEncoder().encode(v).length;
const trim=(v:string)=>v.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g,'');
const text=(v:unknown,max:number,required=false,multiline=false):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=max&&(!required||!!trim(v))&&!/[\p{Cc}]/u.test(multiline?v.replace(/[\r\n\t]/g,''):v);
const textID=(v:unknown,max=100):v is string=>text(v,max,true)&&trim(v)===v;
const operation=(v:unknown):v is string=>textID(v,128)&&bytes(v)>=8;
const nativeID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const stamp=(v:unknown):v is string=>typeof v==='string'&&/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-]\d\d:\d\d)$/.test(v)&&Number.isFinite(Date.parse(v))&&new Date(Date.parse(v.slice(0,19)+'Z')).toISOString().slice(0,19)===v.slice(0,19);
const same=(a:unknown,b:unknown):boolean=>a===b||Array.isArray(a)&&Array.isArray(b)&&a.length===b.length&&a.every((v,i)=>same(v,b[i]))||object(a)&&object(b)&&Object.keys(a).length===Object.keys(b).length&&Object.keys(a).every(k=>Object.hasOwn(b,k)&&same(a[k],b[k]));
const scopeKeys=['project','runID','nodeID','attempt','fence','userID','organizationID','protocol'] as const;
function validScope(v:unknown):v is ToolInputScope{return object(v)&&fields(v,scopeKeys)&&textID(v.project,64)&&textID(v.runID,100)&&textID(v.nodeID,150)&&integer(v.attempt,1)&&integer(v.fence,1)&&textID(v.userID,100)&&textID(v.organizationID,100)&&v.protocol==='codex-app-server-v1';}
export function toolInputRecoveryScope(scope:ToolInputScope):string{
 if(!validScope(scope))throw new UiError('tool-inputs.invalid_read');
 return JSON.stringify([scope.userID,scope.organizationID,scope.project,scope.runID,scope.nodeID,scope.protocol]);
}
export function toolInputListPath(scope:ToolInputScope):string{
 toolInputRecoveryScope(scope);
 return '/runs/'+encodeURIComponent(scope.runID)+'/nodes/'+encodeURIComponent(scope.nodeID)+'/tool-inputs?project_id='+encodeURIComponent(scope.project)+'&attempt='+scope.attempt+'&fence='+scope.fence;
}
function rpcID(v:unknown):boolean{
 if(!object(v)||!fields(v,['kind','value'])||typeof v.value!=='string')return false;
 if(v.kind==='string')return text(v.value,256,true);
 if(v.kind!=='number'||!/^(-?[1-9]\d*|0)$/.test(v.value))return false;
 try{const n=BigInt(v.value);return n.toString()===v.value&&n>=-9223372036854775808n&&n<=9223372036854775807n;}catch{return false;}
}
const identityKeys=['request_id','method','thread_id','turn_id','item_id','binding_sha256','params_sha256'] as const;
function nativeIdentity(v:unknown):boolean{return object(v)&&rpcID(v.request_id)&&v.method==='item/tool/requestUserInput'&&nativeID(v.thread_id)&&nativeID(v.turn_id)&&nativeID(v.item_id)&&hash(v.binding_sha256)&&hash(v.params_sha256);}
function validRequest(v:unknown):v is ToolInputRequest{
 if(!object(v)||!fields(v,['idempotency_key','protocol','codex'])||!operation(v.idempotency_key)||v.protocol!=='codex-app-server-v1'||!object(v.codex)||!fields(v.codex,[...identityKeys,'is_blocking','questions'],['auto_resolution_ms'])||!nativeIdentity(v.codex)||typeof v.codex.is_blocking!=='boolean'||Object.hasOwn(v.codex,'auto_resolution_ms')&&!integer(v.codex.auto_resolution_ms)||!Array.isArray(v.codex.questions)||v.codex.questions.length<1||v.codex.questions.length>3)return false;
 const ids=new Set<string>();
 for(const q of v.codex.questions){
  if(!object(q)||!fields(q,['id','header','question','is_other','is_secret','options'])||!text(q.id,100,true)||ids.has(q.id)||!text(q.header,80,true)||!text(q.question,4000,true,true)||typeof q.is_other!=='boolean'||q.is_secret!==false||!Array.isArray(q.options)||q.options.length>12)return false;
  ids.add(q.id);const labels=new Set<string>();
  for(const option of q.options){if(!object(option)||!fields(option,['label','description'])||!text(option.label,200,true)||labels.has(option.label)||!text(option.description,1000,false,true))return false;labels.add(option.label);}
 }
 return true;
}
function selectionShape(v:unknown):v is CodexUserInputSelection{
 if(!object(v)||!fields(v,['question_id','kind'],['option','text'])||!text(v.question_id,100,true)||typeof v.kind!=='string'||!['option','text','other','skip'].includes(v.kind)||Object.hasOwn(v,'option')&&!text(v.option,200)||Object.hasOwn(v,'text')&&!text(v.text,4000,false,true))return false;
 if(v.kind==='skip')return !v.option&&!v.text;
 return v.kind==='option'?text(v.option,200,true):!v.option&&(v.kind!=='text'||typeof v.text==='string'&&!!trim(v.text));
}
function selectionsShape(v:unknown):v is CodexUserInputSelection[]{return Array.isArray(v)&&v.length>=1&&v.length<=3&&v.every(selectionShape)&&new Set(v.map(s=>s.question_id)).size===v.length;}
export function validToolInputAnswers(request:unknown,answers:unknown):answers is CodexUserInputSelection[]{
 if(!validRequest(request)||!selectionsShape(answers)||answers.length!==request.codex.questions.length)return false;
 return answers.every(answer=>{const q=request.codex.questions.find(q=>q.id===answer.question_id);return !!q&&(answer.kind==='skip'||answer.kind==='option'&&q.options.some(o=>o.label===answer.option)||answer.kind==='text'&&q.options.length===0||answer.kind==='other'&&q.options.length>0&&q.is_other);});
}
const normalizedAnswers=(v:CodexUserInputSelection[])=>v.map(a=>({question_id:a.question_id,kind:a.kind,option:a.option||'',text:a.text||''}));
function observation(v:unknown):boolean{
 if(!object(v)||!fields(v,['sequence','params_sha256','answer_observed','reply_started','reply_written','request_resolved','turn_closed'],['answer_sha256','failure_code'])||!integer(v.sequence,1)||!hash(v.params_sha256)||!['answer_observed','reply_started','reply_written','request_resolved','turn_closed'].every(k=>typeof v[k]==='boolean')||Object.hasOwn(v,'answer_sha256')&&v.answer_sha256!==''&&!hash(v.answer_sha256)||v.answer_observed!==!!v.answer_sha256||v.reply_started&&!v.answer_observed||v.reply_written&&!v.reply_started)return false;
 return !Object.hasOwn(v,'failure_code')||v.failure_code===''||nativeID(v.failure_code)&&v.failure_code.length<=80;
}
export function validToolInput(v:unknown,scope:ToolInputScope):v is ToolInput{
 if(!validScope(scope)||!object(v)||!fields(v,['id','project_id','run_id','node_id','attempt','fence','dispatch_id','worker_id','profile_sha256','version','state','request','created_at','expires_at','allowed_actions'],['answer','observation'])||!textID(v.id)||v.project_id!==scope.project||v.run_id!==scope.runID||v.node_id!==scope.nodeID||v.attempt!==scope.attempt||v.fence!==scope.fence||!text(v.dispatch_id,128)||!textID(v.worker_id)||!hash(v.profile_sha256)||!integer(v.version,1)||typeof v.state!=='string'||!['pending','answered','closed','expired'].includes(v.state)||!validRequest(v.request)||!stamp(v.created_at)||!stamp(v.expires_at)||!Array.isArray(v.allowed_actions)||v.allowed_actions.length>1||v.allowed_actions.some(a=>a!=='answer'||v.state!=='pending'))return false;
 const answer=v.answer;
 if(Object.hasOwn(v,'answer')&&(!object(answer)||!fields(answer,['answers','answer_sha256','actor_id','answered_at'])||!validToolInputAnswers(v.request,answer.answers)||!hash(answer.answer_sha256)||!textID(answer.actor_id)||!stamp(answer.answered_at))||v.state==='pending'&&Object.hasOwn(v,'answer')||v.state==='answered'&&!object(answer))return false;
 if(Object.hasOwn(v,'observation')){const o=v.observation;if(!observation(o)||!object(o)||o.params_sha256!==v.request.codex.params_sha256||o.answer_observed&&(!object(answer)||o.answer_sha256!==answer.answer_sha256))return false;}
 return true;
}
export function readToolInputList(v:unknown,scope:ToolInputScope):ToolInputList{
 if(!validScope(scope)||!object(v)||!fields(v,['protocol','items'])||!['','codex-user-input-v1'].includes(String(v.protocol))||typeof v.protocol!=='string'||!Array.isArray(v.items)||v.items.length>32||!v.items.every(row=>validToolInput(row,scope))||new Set(v.items.map(row=>row.id)).size!==v.items.length)throw new UiError('tool-inputs.invalid_read');
 return v as unknown as ToolInputList;
}
function answerID(path:unknown):string|undefined{if(typeof path!=='string')return;const match=/^\/tool-inputs\/([^/?#]+)\/answer$/.exec(path);if(!match)return;try{const id=decodeURIComponent(match[1]);if(textID(id)&&!/[\/\\]/.test(id)&&encodeURIComponent(id)===match[1])return id;}catch{/* A malformed locator is never a different resource. */}}
export function validToolInputRecovery(v:unknown,scope:ToolInputScope):v is WriteRecoveryRequest{
 if(!validScope(scope)||!object(v)||!fields(v,['path','method','idempotency_key','body'])||v.method!=='POST'||!answerID(v.path)||!operation(v.idempotency_key)||!object(v.body)||!fields(v.body,['project_id','expected_version','idempotency_key','answers'])||v.body.project_id!==scope.project||v.body.idempotency_key!==v.idempotency_key||!integer(v.body.expected_version,1)||!selectionsShape(v.body.answers))return false;
 return true;
}
function sourceIdentity(row:ToolInput):SourceIdentity{const c=row.request.codex;return {idempotency_key:row.request.idempotency_key,protocol:'codex-app-server-v1',codex:{request_id:{...c.request_id},method:c.method,thread_id:c.thread_id,turn_id:c.turn_id,item_id:c.item_id,binding_sha256:c.binding_sha256,params_sha256:c.params_sha256}};}
function validSourceIdentity(v:unknown):v is SourceIdentity{return object(v)&&fields(v,['idempotency_key','protocol','codex'])&&operation(v.idempotency_key)&&v.protocol==='codex-app-server-v1'&&object(v.codex)&&fields(v.codex,identityKeys)&&nativeIdentity(v.codex);}
export function validToolInputIntent(v:unknown,scope:ToolInputScope):v is ToolInputIntent{
 return validScope(scope)&&object(v)&&fields(v,['scope','input_id','profile_sha256','dispatch_id','worker_id','source_identity','request'])&&validScope(v.scope)&&toolInputRecoveryScope(v.scope)===toolInputRecoveryScope(scope)&&textID(v.input_id)&&hash(v.profile_sha256)&&text(v.dispatch_id,128)&&textID(v.worker_id)&&validSourceIdentity(v.source_identity)&&validToolInputRecovery(v.request,v.scope)&&answerID(v.request.path)===v.input_id;
}
export function toolInputMatchesIntent(row:unknown,intent:ToolInputIntent):boolean{
 return validToolInputIntent(intent,intent.scope)&&validToolInput(row,intent.scope)&&row.id===intent.input_id&&row.profile_sha256===intent.profile_sha256&&row.dispatch_id===intent.dispatch_id&&row.worker_id===intent.worker_id&&same(sourceIdentity(row),intent.source_identity)&&validToolInputAnswers(row.request,intent.request.body.answers);
}
export function canAnswerToolInput(row:ToolInput):boolean{
 const o=row.observation;return row.state==='pending'&&!row.answer&&row.allowed_actions.includes('answer')&&(!o||!o.answer_observed&&!o.reply_started&&!o.reply_written&&!o.request_resolved&&!o.turn_closed&&!o.failure_code);
}
export function buildToolInputIntent(row:ToolInput,scope:ToolInputScope,request:WriteRecoveryRequest):ToolInputIntent|undefined{
 if(!validToolInput(row,scope)||!canAnswerToolInput(row)||!validToolInputRecovery(request,scope)||answerID(request.path)!==row.id||request.body.expected_version!==row.version||!validToolInputAnswers(row.request,request.body.answers))return;
 const value={scope,input_id:row.id,profile_sha256:row.profile_sha256,dispatch_id:row.dispatch_id,worker_id:row.worker_id,source_identity:sourceIdentity(row),request};
 return validToolInputIntent(value,scope)?structuredClone(value):undefined;
}
export function validToolInputReceipt(value:unknown,request:WriteRecoveryRequest,scope:ToolInputScope,intent:ToolInputIntent):boolean{
 if(!validToolInputIntent(intent,scope)||!validToolInputRecovery(request,intent.scope)||!same(request,intent.request)||!toolInputMatchesIntent(value,intent)||!validToolInput(value,intent.scope)||value.version!==(request.body.expected_version as number)+1||value.state!=='answered'||!value.answer||value.answer.actor_id!==intent.scope.userID||!selectionsShape(request.body.answers))return false;
 return same(normalizedAnswers(value.answer.answers),normalizedAnswers(request.body.answers));
}
