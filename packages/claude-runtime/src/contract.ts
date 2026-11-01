import {createHash} from 'node:crypto';
import {isDeepStrictEqual} from 'node:util';

export const PROTOCOL='claude-sdk-v1',SDK_VERSION='0.3.314',CLI_VERSION='2.1.314',VERSION='1.0.0';
export const MAX_CALLBACKS=32,MAX_PAYLOAD_BYTES=65536;
export const TOOL_NAMES=['Read','Edit','Write','Glob','Grep','Bash','AskUserQuestion'] as const;
export type PermissionTool=Exclude<typeof TOOL_NAMES[number],'AskUserQuestion'>;
export interface RuntimeConfig{native_cli_executable:string;native_cli_sha256:string;home:string;source:string;model:string;credential_env_ref:string;configuration_sha256:string;sdk_version:typeof SDK_VERSION;cli_version:typeof CLI_VERSION}
export interface Basis{session_id:string;tool_use_id:string;request_id:string;callback_id:string;configuration_sha256:string;params_sha256:string}
export interface PermissionSource extends Basis{tool_name:PermissionTool;input_json:string}
export interface QuestionOption{id:string;label:string;description:string}
export interface Question{id:string;prompt:string;header:string;options:QuestionOption[];allow_multiple:boolean}
export interface InputRequest extends Basis{input_json:string;questions:Question[]}
export interface PermissionChoice{outcome:'allow_once'|'reject_once';reason?:string}
export interface InputSelection{question_id:string;option_ids:string[]}
export interface InputDecision{outcome:'answered'|'cancelled';answers?:InputSelection[];reason?:string}
export interface ExecuteRequest{v:1;type:'execute';request_id:string;goal:string;configuration_sha256:string}
export interface CancelRequest{v:1;type:'cancel';request_id:string}
export interface PermissionReply{v:1;type:'permission_reply';request_id:string;source:PermissionSource;decision:PermissionChoice}
export interface InputReply{v:1;type:'input_reply';request_id:string;source:InputRequest;answer:InputDecision}
export type HostRequest=ExecuteRequest|CancelRequest|PermissionReply|InputReply;
export type Frame={v:1;type:string;request_id:string;[key:string]:unknown};
export type Emit=(frame:Frame)=>void;
export class RuntimeError extends Error{constructor(readonly code:string){super(code);}}
export const object=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
export const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
export const sha256=(value:string|Buffer)=>createHash('sha256').update(value).digest('hex');
// Reject lone UTF-16 surrogates instead of hashing replacement characters that
// would make the native payload differ between JavaScript and Go.
export function validUnicode(value:string){for(let i=0;i<value.length;i++){const c=value.charCodeAt(i);if(c>=0xd800&&c<=0xdbff){const next=value.charCodeAt(++i);if(!(next>=0xdc00&&next<=0xdfff))return false;}else if(c>=0xdc00&&c<=0xdfff)return false;}return true;}
export function text(value:unknown,max:number,required=true):value is string{return typeof value==='string'&&(!required&&value===''||value!==''&&value.trim()===value&&Buffer.byteLength(value)<=max&&validUnicode(value)&&!/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/.test(value));}
export const id=(value:unknown):value is string=>text(value,150)&&!/[\s\p{Cc}]/u.test(value);
export function closed(value:unknown,keys:readonly string[]):asserts value is Record<string,unknown>{if(!object(value)||Object.keys(value).some(key=>!keys.includes(key)))throw new RuntimeError('request_invalid');}
export function required(value:Record<string,unknown>,keys:readonly string[]){if(keys.some(key=>!Object.hasOwn(value,key)))throw new RuntimeError('request_invalid');}
export function same(a:unknown,b:unknown){return isDeepStrictEqual(a,b);}
export function validBasis(value:unknown):value is Basis{return object(value)&&id(value.session_id)&&id(value.tool_use_id)&&id(value.request_id)&&id(value.callback_id)&&digest(value.configuration_sha256)&&digest(value.params_sha256);}

/** A single serialization is retained as the payload authority. No canonical
 * JSON or redacted replacement may silently change what a person approves. */
export function inputJSON(value:unknown,credential?:string):string{
 if(!object(value))throw new RuntimeError('callback_invalid');
 const seen=new Set<object>();
 const visit=(node:unknown,depth:number)=>{if(depth>20)throw new RuntimeError('callback_invalid');if(typeof node==='string'){if(!validUnicode(node)||credential&&node.includes(credential))throw new RuntimeError('callback_sensitive');return;}if(node===null||typeof node==='boolean')return;if(typeof node==='number'){if(!Number.isFinite(node))throw new RuntimeError('callback_invalid');return;}if(typeof node!=='object'||seen.has(node))throw new RuntimeError('callback_invalid');seen.add(node);for(const [key,item]of Object.entries(node)){if(!validUnicode(key)||key==='toJSON'||/^(?:api[_-]?key|password|authorization|access[_-]?token|secret)$/i.test(key)&&typeof item==='string'&&item!=='')throw new RuntimeError('callback_sensitive');visit(item,depth+1);}seen.delete(node);};
 visit(value,0);const raw=JSON.stringify(value);if(Buffer.byteLength(raw)>MAX_PAYLOAD_BYTES)throw new RuntimeError('callback_too_large');return raw;
}
export function questions(raw:string):Question[]{
 const input:unknown=JSON.parse(raw);closed(input,['questions','answers']);required(input,['questions']);
 if(input.answers!==undefined&&(!object(input.answers)||Object.keys(input.answers).length!==0)||!Array.isArray(input.questions)||input.questions.length<1||input.questions.length>4)throw new RuntimeError('question_invalid');
 const prompts=new Set<string>();return input.questions.map((rawQuestion,index)=>{closed(rawQuestion,['question','header','options','multiSelect']);required(rawQuestion,['question','header','options','multiSelect']);if(!text(rawQuestion.question,4000)||!text(rawQuestion.header,128)||typeof rawQuestion.multiSelect!=='boolean'||!Array.isArray(rawQuestion.options)||rawQuestion.options.length<2||rawQuestion.options.length>4||prompts.has(rawQuestion.question))throw new RuntimeError('question_invalid');prompts.add(rawQuestion.question);
  const labels=new Set<string>();const options=rawQuestion.options.map((rawOption,j)=>{closed(rawOption,['label','description']);required(rawOption,['label','description']);if(!text(rawOption.label,500)||!text(rawOption.description,4000,false)||labels.has(rawOption.label))throw new RuntimeError('question_invalid');labels.add(rawOption.label);return{id:'o'+(j+1),label:rawOption.label,description:rawOption.description};});
  return{id:'q'+(index+1),prompt:rawQuestion.question,header:rawQuestion.header,options,allow_multiple:rawQuestion.multiSelect};
 });
}
const basisKeys=['session_id','tool_use_id','request_id','callback_id','configuration_sha256','params_sha256'];
export function permissionSource(value:unknown):asserts value is PermissionSource{closed(value,[...basisKeys,'tool_name','input_json']);if(!validBasis(value)||!TOOL_NAMES.slice(0,-1).includes(value.tool_name as PermissionTool)||typeof value.input_json!=='string'||Buffer.byteLength(value.input_json)>MAX_PAYLOAD_BYTES||sha256(value.input_json)!==value.params_sha256||!object(JSON.parse(value.input_json)))throw new RuntimeError('callback_invalid');}
export function inputSource(value:unknown):asserts value is InputRequest{closed(value,[...basisKeys,'input_json','questions']);if(!validBasis(value)||typeof value.input_json!=='string'||Buffer.byteLength(value.input_json)>MAX_PAYLOAD_BYTES||sha256(value.input_json)!==value.params_sha256||!same(questions(value.input_json),value.questions))throw new RuntimeError('question_invalid');}
export function permissionChoice(value:unknown):asserts value is PermissionChoice{closed(value,['outcome','reason']);if(!['allow_once','reject_once'].includes(String(value.outcome))||value.reason!==undefined&&!text(value.reason,2000,false))throw new RuntimeError('decision_invalid');}
export function inputDecision(source:InputRequest,value:unknown):asserts value is InputDecision{
 closed(value,['outcome','answers','reason']);if(value.reason!==undefined&&!text(value.reason,2000,false))throw new RuntimeError('decision_invalid');if(value.outcome==='cancelled'&&(value.answers===undefined||Array.isArray(value.answers)&&value.answers.length===0))return;
 if(value.outcome!=='answered'||!Array.isArray(value.answers)||value.answers.length!==source.questions.length)throw new RuntimeError('answer_invalid');const seen=new Set<string>();
 for(const answer of value.answers){closed(answer,['question_id','option_ids']);const question=source.questions.find(q=>q.id===answer.question_id);if(!question||seen.has(question.id)||!Array.isArray(answer.option_ids)||answer.option_ids.length<1||!question.allow_multiple&&answer.option_ids.length!==1||new Set(answer.option_ids).size!==answer.option_ids.length||answer.option_ids.some(option=>typeof option!=='string'||!question.options.some(o=>o.id===option)))throw new RuntimeError('answer_invalid');seen.add(question.id);}
}
export function decisionDigest(source:Basis,kind:'permission'|'input',...parts:string[]){const values=[PROTOCOL,kind,source.session_id,source.tool_use_id,source.request_id,source.callback_id,source.configuration_sha256,source.params_sha256,...parts],hash=createHash('sha256');for(const v of values){hash.update(Buffer.byteLength(v)+':');hash.update(v);}return hash.digest('hex');}
export function permissionDigest(source:PermissionSource,choice:PermissionChoice){permissionSource(source);permissionChoice(choice);return decisionDigest(source,'permission',source.tool_name,choice.outcome,choice.reason||'');}
export function answerDigest(source:InputRequest,answer:InputDecision){inputSource(source);inputDecision(source,answer);const parts=[answer.outcome,answer.reason||''];for(const a of [...answer.answers||[]].sort((a,b)=>a.question_id<b.question_id?-1:a.question_id>b.question_id?1:0)){parts.push(a.question_id,...[...a.option_ids].sort());}return decisionDigest(source,'input',...parts);}
export function nativeAnswers(source:InputRequest,answer:InputDecision):Record<string,string>{inputDecision(source,answer);if(answer.outcome!=='answered')throw new RuntimeError('answer_invalid');return Object.fromEntries(source.questions.map(q=>[q.prompt,q.options.filter(o=>answer.answers!.find(a=>a.question_id===q.id)!.option_ids.includes(o.id)).map(o=>o.label).join(', ')]));}

export function parseRequest(value:unknown):HostRequest{
 if(!object(value)||value.v!==1||!id(value.request_id))throw new RuntimeError('request_invalid');
 if(value.type==='execute'){closed(value,['v','type','request_id','goal','configuration_sha256']);if(typeof value.goal!=='string'||!value.goal.trim()||!validUnicode(value.goal)||Buffer.byteLength(value.goal)>262144||!digest(value.configuration_sha256))throw new RuntimeError('request_invalid');return value as unknown as ExecuteRequest;}
 if(value.type==='cancel'){closed(value,['v','type','request_id']);return value as unknown as CancelRequest;}
 if(value.type==='permission_reply'){closed(value,['v','type','request_id','source','decision']);permissionSource(value.source);permissionChoice(value.decision);return value as unknown as PermissionReply;}
 if(value.type==='input_reply'){closed(value,['v','type','request_id','source','answer']);inputSource(value.source);inputDecision(value.source,value.answer);return value as unknown as InputReply;}
 throw new RuntimeError('request_invalid');
}

/** JSON.parse supplies syntax validation; a lexical object-key pass refuses
 * duplicate keys before parsing can erase their first values. */
export function parseLine(raw:string):unknown{
 if(Buffer.byteLength(raw)>1024*1024||!validUnicode(raw))throw new RuntimeError('request_invalid');
 const stack:Array<{object:boolean;keys:Set<string>;key:boolean}>=[];for(let i=0;i<raw.length;i++){const ch=raw[i];if(ch==='"'){let end=i+1;for(;end<raw.length;end++){if(raw[end]==='\\'){end++;continue;}if(raw[end]==='"')break;}if(end>=raw.length)throw new RuntimeError('request_invalid');const token=JSON.parse(raw.slice(i,end+1));if(!validUnicode(token))throw new RuntimeError('request_invalid');const parent=stack.at(-1);if(parent?.object&&parent.key){if(parent.keys.has(token))throw new RuntimeError('request_invalid');parent.keys.add(token);parent.key=false;}i=end;}else if(ch==='{'||ch==='['){if(stack.length>=32)throw new RuntimeError('request_invalid');stack.push({object:ch==='{',keys:new Set(),key:ch==='{'});}else if(ch==='}'||ch===']'){stack.pop();}else if(ch===','&&stack.at(-1)?.object)stack.at(-1)!.key=true;}
 return JSON.parse(raw);
}
