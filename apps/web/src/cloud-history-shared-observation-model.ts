import type {CloudHistorySharedObservation} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';

const bytes=(value:string)=>new TextEncoder().encode(value).length;
const object=(value:unknown):value is Record<string,unknown>=>{
 if(value===null||typeof value!=='object'||Array.isArray(value))return false;
 const prototype=Object.getPrototypeOf(value);return prototype===Object.prototype||prototype===null;
};
const fields=(value:Record<string,unknown>,expected:readonly string[])=>Reflect.ownKeys(value).length===expected.length&&expected.every(key=>Object.hasOwn(value,key));
const id=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&value.trim()===value&&value!=='.'&&value!=='..'&&wellFormed(value)&&bytes(value)<=max&&!/[\/\\\p{Cc}]/u.test(value);
const positive=(value:unknown,max:number):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=max;
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const supported=['user_text','assistant_text','tool_summary','plan_summary'];

/** Public attribution is deliberately smaller than the private observation.
 * This synchronous render guard checks only closed shape and digest encoding;
 * asynchronous share receipts also verify the body and selection digests. */
export function validCloudHistorySharedObservation(value:unknown):value is CloudHistorySharedObservation{
 if(!object(value)||!fields(value,['load_id','load_version','load_state','coverage','source_run_id','source_node_id','source_submission_id','entries','body_sha256','selection_sha256'])||!id(value.load_id,64)||!positive(value.load_version,2147483647)||typeof value.load_state!=='string'||!['loaded','interrupted','limited','rejected'].includes(value.load_state)||value.coverage!=='unverified'||!id(value.source_run_id,100)||!id(value.source_node_id,150)||!id(value.source_submission_id,64)||!hash(value.body_sha256)||!hash(value.selection_sha256)||!Array.isArray(value.entries)||value.entries.length<1||value.entries.length>8)return false;
 let previous=0;
 for(const entry of value.entries){
  if(!object(entry)||!fields(entry,['sequence','kind','text_sha256'])||!positive(entry.sequence,500)||entry.sequence<=previous||typeof entry.kind!=='string'||!supported.includes(entry.kind)||!hash(entry.text_sha256))return false;
  previous=entry.sequence;
 }
 return true;
}
