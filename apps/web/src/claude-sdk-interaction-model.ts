import type {ClaudePermissionSource,ClaudePermissionChoice,ClaudeInputRequest,ClaudeInputDecision,ClaudePermissionEvidence,ClaudeInputEvidence,ToolInputDeliveryObservation} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
const basisKeys=['session_id','tool_use_id','request_id','callback_id','configuration_sha256','params_sha256'] as const;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&[Object.prototype,null].includes(Object.getPrototypeOf(v));
const fields=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const bytes=(v:string)=>new TextEncoder().encode(v).length;
const trim=(v:string)=>v.replace(/^[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+|[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028\u2029\u202f\u205f\u3000]+$/g,'');
const text=(v:unknown,max:number,required=true):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=max&&(!required||v!=='')&&trim(v)===v&&!/\p{Cc}/u.test(v.replace(/[\n\t]/g,''));
const id=(v:unknown):v is string=>text(v,150)&&!/[\p{White_Space}]/u.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const integer=(v:unknown,min:number,max=Number.MAX_SAFE_INTEGER):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min&&v<=max;
const tool=(v:unknown)=>typeof v==='string'&&['Read','Edit','Write','Glob','Grep','Bash'].includes(v);
function basis(v:Record<string,unknown>):boolean{return ['session_id','tool_use_id','request_id','callback_id'].every(k=>id(v[k]))&&hash(v.configuration_sha256)&&hash(v.params_sha256);}
export function validClaudeCallbackBasis(v:unknown):boolean{return object(v)&&fields(v,basisKeys)&&basis(v);}
// Read the exact original JSON without losing duplicate members or escaped
// invalid Unicode. Its digest is checked by the authoritative Go/Node source,
// rather than treating a browser recomputation as an authorization grant.
function payload(raw:unknown):Record<string,unknown>|undefined{
 if(typeof raw!=='string'||!wellFormed(raw)||bytes(raw)>65536)return;
 try{
  const parsed:unknown=JSON.parse(raw);if(!object(parsed))return;
  let at=0,count=0;
  const ws=()=>{while(/[\t\r\n ]/.test(raw[at]||'x'))at++;};
  const string=()=>{const start=at++;while(at<raw.length){const c=raw[at++];if(c==='\\'){at++;continue;}if(c==='"'){const value:unknown=JSON.parse(raw.slice(start,at));if(typeof value!=='string'||!wellFormed(value))throw Error('unicode');return value;}}throw Error('string');};
  const walk=(depth:number):void=>{
   if(depth>24||++count>262144)throw Error('limit');ws();const c=raw[at];
   if(c==='"'){string();return;}
   if(c==='{'||c==='['){at++;ws();const end=c==='{'?'}':']',seen=new Set<string>();if(raw[at]===end){at++;return;}for(;;){ws();if(c==='{'){const key=string();if(seen.has(key))throw Error('duplicate');seen.add(key);ws();if(raw[at++]!==':')throw Error('colon');}walk(depth+1);ws();const next=raw[at++];if(next===end)return;if(next!==',')throw Error('separator');}}
   const token=/^(?:true|false|null|-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?)/.exec(raw.slice(at));if(!token)throw Error('token');at+=token[0].length;
  };
  walk(0);ws();return at===raw.length?parsed:undefined;
 }catch{return;}
}
export function validClaudePermissionSource(v:unknown):v is ClaudePermissionSource{return object(v)&&fields(v,[...basisKeys,'tool_name','input_json'])&&basis(v)&&tool(v.tool_name)&&!!payload(v.input_json);}
export function validClaudePermissionChoice(v:unknown):v is ClaudePermissionChoice{return object(v)&&fields(v,['outcome'],['reason'])&&(v.outcome==='allow_once'||v.outcome==='reject_once')&&(!Object.hasOwn(v,'reason')||text(v.reason,2000,false));}
export function validClaudeInputRequest(v:unknown):v is ClaudeInputRequest{
 if(!object(v)||!fields(v,[...basisKeys,'input_json','questions'])||!basis(v)||!Array.isArray(v.questions))return false;
 const raw=payload(v.input_json);if(!raw||!fields(raw,['questions'],['answers'])||Object.hasOwn(raw,'answers')&&(!object(raw.answers)||Reflect.ownKeys(raw.answers).length!==0)||!Array.isArray(raw.questions)||raw.questions.length<1||raw.questions.length>4||v.questions.length!==raw.questions.length)return false;
 const prompts=new Set<string>();
 for(let i=0;i<raw.questions.length;i++){
  const native=raw.questions[i],q=v.questions[i];
  if(!object(native)||!fields(native,['question','header','options','multiSelect'])||!text(native.question,4000)||prompts.has(native.question)||!text(native.header,128)||typeof native.multiSelect!=='boolean'||!Array.isArray(native.options)||native.options.length<2||native.options.length>4||!object(q)||!fields(q,['id','prompt','header','options','allow_multiple'])||q.id!=='q'+(i+1)||q.prompt!==native.question||q.header!==native.header||q.allow_multiple!==native.multiSelect||!Array.isArray(q.options)||q.options.length!==native.options.length)return false;
  prompts.add(native.question);const labels=new Set<string>();
  for(let j=0;j<native.options.length;j++){const option=native.options[j],shown=q.options[j];if(!object(option)||!fields(option,['label','description'])||!text(option.label,500)||labels.has(option.label)||!text(option.description,4000,false)||!object(shown)||!fields(shown,['id','label','description'])||shown.id!=='o'+(j+1)||shown.label!==option.label||shown.description!==option.description)return false;labels.add(option.label);}
 }
 return true;
}
export function validClaudeInputDecisionShape(v:unknown):v is ClaudeInputDecision{
 if(!object(v)||!fields(v,['outcome'],['answers','reason'])||!['answered','cancelled'].includes(String(v.outcome))||Object.hasOwn(v,'reason')&&!text(v.reason,2000,false)||Object.hasOwn(v,'answers')&&!Array.isArray(v.answers))return false;
 const answers=v.answers??[];if(!Array.isArray(answers)||answers.length>4)return false;
 if(v.outcome==='cancelled')return answers.length===0;
 if(answers.length===0)return false;
 const ids=new Set<string>();
 for(const answer of answers){if(!object(answer)||!fields(answer,['question_id','option_ids'])||typeof answer.question_id!=='string'||!/^q[1-4]$/.test(answer.question_id)||ids.has(answer.question_id)||!Array.isArray(answer.option_ids)||answer.option_ids.length<1||answer.option_ids.length>4||!answer.option_ids.every((id:unknown)=>typeof id==='string'&&/^o[1-4]$/.test(id))||new Set(answer.option_ids).size!==answer.option_ids.length)return false;ids.add(answer.question_id);}
 return true;
}
export function validClaudeInputDecision(request:unknown,v:unknown):v is ClaudeInputDecision{
 if(!validClaudeInputRequest(request)||!validClaudeInputDecisionShape(v))return false;
 if(v.outcome==='cancelled')return true;
 return !!v.answers&&v.answers.length===request.questions.length&&v.answers.every(a=>{const q=request.questions.find(q=>q.id===a.question_id);return !!q&&(q.allow_multiple||a.option_ids.length===1)&&a.option_ids.every(id=>q.options.some(o=>o.id===id));});
}
export function validClaudeDeliveryObservation(v:unknown):v is ToolInputDeliveryObservation{
 return object(v)&&fields(v,['sequence','params_sha256','answer_observed','reply_started','reply_written','request_resolved','execution_closed'],['answer_sha256','failure_code'])&&integer(v.sequence,1)&&hash(v.params_sha256)&&['answer_observed','reply_started','reply_written','execution_closed'].every(k=>typeof v[k]==='boolean')&&v.request_resolved===false&&(!Object.hasOwn(v,'answer_sha256')||v.answer_sha256===''||hash(v.answer_sha256))&&v.answer_observed===!!v.answer_sha256&&(!v.reply_started||v.answer_observed===true)&&(!v.reply_written||v.reply_started===true)&&(!Object.hasOwn(v,'failure_code')||v.failure_code===''||typeof v.failure_code==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(v.failure_code));
}
function evidenceObservation(v:Record<string,unknown>):boolean{return !Object.hasOwn(v,'observation')||validClaudeDeliveryObservation(v.observation)&&v.observation.params_sha256===v.params_sha256;}
export function validClaudeSDKPermissionEvidence(v:unknown):v is ClaudePermissionEvidence{return object(v)&&fields(v,[...basisKeys,'permission_id','tool_name'],['observation'])&&basis(v)&&id(v.permission_id)&&tool(v.tool_name)&&evidenceObservation(v);}
export function validClaudeSDKInputEvidence(v:unknown):v is ClaudeInputEvidence{return object(v)&&fields(v,[...basisKeys,'input_id','question_count'],['observation'])&&basis(v)&&id(v.input_id)&&integer(v.question_count,1,4)&&evidenceObservation(v);}
