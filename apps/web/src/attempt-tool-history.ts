// A page is evidence for one durable identity and one saved scan window.
// Reject the entire response on broken attribution; never clip/repair raw rows.
import type {AttemptToolEventPage, ExecutionAttempt, Event as SavedEvent} from '../../../packages/contracts/src';
import {UiError} from './i18n';
import {wellFormed} from './i18n/catalog';
export type AttemptToolScope = Pick<ExecutionAttempt,'run_id'|'node_id'|'attempt'|'fence'|'worker_id'> & {project_id:string};
export type SavedPosition = {cursor:string;after:number;watermark?:number};
export const firstSavedPosition = ():SavedPosition=>({cursor:'',after:0});
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const integer=(value:unknown,min=0):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min;
const bytes=(value:unknown)=>new TextEncoder().encode(JSON.stringify(value)).length;
// Iterative semantic comparison stays bounded by the already checked Event
// byte budget and never assigns arbitrary keys to an object/prototype.
function sameJSON(left:unknown,right:unknown):boolean {
 const pending:[unknown,unknown][]=[[left,right]];
 while(pending.length){const [a,b]=pending.pop()!;if(a===b)continue;
  if(Array.isArray(a)||Array.isArray(b)){if(!Array.isArray(a)||!Array.isArray(b)||a.length!==b.length)return false;for(let index=0;index<a.length;index++)pending.push([a[index],b[index]]);}
  else if(object(a)&&object(b)){const keys=Object.keys(a);if(keys.length!==Object.keys(b).length)return false;for(const key of keys){if(!Object.hasOwn(b,key))return false;pending.push([a[key],b[key]]);}}
  else return false;
 }
 return true;
}
const utf8Encoder=new TextEncoder(),utf8Decoder=new TextDecoder('utf-8',{fatal:true,ignoreBOM:true});
// Go encoding/json replaces lone UTF16 escapes. Normalize only this ephemeral
// comparison input, before parsing object keys, so normalized-key collisions
// retain Go's original source-order last-value behavior (including duplicates).
function goJSONProjection(source:string):unknown {
 const pieces:string[]=[];let start=0,index=0;
 while(index<source.length){if(source[index]!=='"'){index++;continue;}const tokenStart=index++;
  while(index<source.length){const character=source[index++];if(character==='\\')index++;else if(character==='"')break;}
  const original=JSON.parse(source.slice(tokenStart,index)) as string,projected=utf8Decoder.decode(utf8Encoder.encode(original));
  if(projected!==original){pieces.push(source.slice(start,tokenStart),JSON.stringify(projected));start=index;}
 }
 return JSON.parse(pieces.length?pieces.join('')+source.slice(start):source);
}
// Decoded/repaired tool strings cannot manufacture native message/catalog
// proof. Keep that row raw-only unless its whole original tool view agrees.
export function savedToolDisplayMatchesSource(event:Pick<SavedEvent,'data'|'raw_data'>):boolean {
 if(typeof event.raw_data!=='string')return false;
 try{const original=JSON.parse(event.raw_data);return object(original)&&!Object.hasOwn(original,'host_operation')&&!Object.hasOwn(event.data,'host_operation')&&sameJSON(original.tool_event,event.data.tool_event);}catch{return false;}
}
// JSON.parse supplies the display projection, but cannot prove duplicate keys
// or integer lexemes. Inspect top-level source tokens before accepting proof.
function rawParent(source:string,scope:AttemptToolScope,data:unknown):boolean {
 let parsed:unknown;
 try{parsed=JSON.parse(source);if(!object(parsed)||!sameJSON(goJSONProjection(source),data))return false;}catch{return false;}
 let index=0;
 const space=()=>{while(/\s/.test(source[index]||'')&&index<source.length)index++;};
 const stringEnd=()=>{index++;while(index<source.length){const character=source[index++];if(character==='\\')index++;else if(character==='"')return;}};
 const members=new Map<string,string>();
 space();if(source[index++]!=='{')return false;
 while(index<source.length){
  space();if(source[index]==='}')break;
  const keyStart=index;stringEnd();const key=JSON.parse(source.slice(keyStart,index)) as string;
  space();index++;space();const start=index;
  let depth=0;
  while(index<source.length){const character=source[index];if(character==='"'){stringEnd();continue;}if(character==='{'||character==='[')depth++;else if(character==='}'||character===']'){if(depth===0)break;depth--;}else if(character===','&&depth===0)break;index++;}
  const normalized=key.toLowerCase();
  if(['attempt','fence','worker_id'].includes(normalized)){
   if(key!==normalized||members.has(key))return false;
   members.set(key,source.slice(start,index).trim());
  }
  space();if(source[index]==='}')break;index++;
 }
 const attempt=members.get('attempt'),fence=members.get('fence'),worker=members.get('worker_id');
 return !!attempt&&/^[1-9][0-9]*$/.test(attempt)&&Number(attempt)===scope.attempt
  &&!!fence&&/^(0|[1-9][0-9]*)$/.test(fence)&&Number(fence)===scope.fence
  &&!!worker&&JSON.parse(worker)===scope.worker_id;
}
export function validateAttemptToolPage(value:unknown,scope:AttemptToolScope,position:SavedPosition,limit:number):AttemptToolEventPage {
 const bad=()=>{throw new UiError('development.savedFacts.invalid');};
 if(!object(value))return bad();
 const fields=['schema_version','project_id','run_id','node_id','attempt','fence','worker_id','items','watermark','scanned_through','has_more','page_cursor','next_cursor'];
 if(Object.keys(value).some(key=>!fields.includes(key))||value.schema_version!==1||!integer(value.attempt,1)||!integer(value.fence)||!integer(value.watermark)||!integer(value.scanned_through)||value.scanned_through>value.watermark||value.scanned_through<position.after||!Array.isArray(value.items)||value.items.length>limit||typeof value.has_more!=='boolean')return bad();
 for(const key of ['project_id','run_id','node_id','worker_id'] as const)if(typeof value[key]!=='string')return bad();
 for(const key of ['project_id','run_id','node_id','attempt','fence','worker_id'] as const)if(value[key]!==scope[key])return bad();
 if(position.watermark!==undefined&&value.watermark!==position.watermark)return bad();
 if(typeof value.page_cursor!=='string'||!value.page_cursor||new TextEncoder().encode(value.page_cursor).length>4096||(position.cursor&&value.page_cursor!==position.cursor))return bad();
 const hasCursor=Object.hasOwn(value,'next_cursor');
 if(value.has_more!==hasCursor||(hasCursor&&(typeof value.next_cursor!=='string'||!value.next_cursor||new TextEncoder().encode(value.next_cursor).length>4096||value.next_cursor===value.page_cursor||!integer(value.scanned_through,1)||value.scanned_through<=position.after||value.scanned_through>=value.watermark))||(!value.has_more&&value.scanned_through!==value.watermark))return bad();
 let previous=position.after;
 for(const item of value.items){
  if(!object(item)||!integer(item.sequence,1)||item.sequence<=previous||item.sequence>value.scanned_through||item.run_id!==scope.run_id||item.node_id!==scope.node_id||item.type!=='tool_progress'||typeof item.at!=='string'||!item.at||!Number.isFinite(Date.parse(item.at))||!object(item.data))return bad();
  const parent=item.data;
  if(parent.attempt!==scope.attempt||!integer(parent.attempt,1)||parent.fence!==scope.fence||!integer(parent.fence)||parent.worker_id!==scope.worker_id||Object.keys(parent).some(key=>['attempt','fence','worker_id'].includes(key.toLowerCase())&&!['attempt','fence','worker_id'].includes(key)))return bad();
  if(bytes(item)>2*1024*1024)return bad();
  if(typeof item.raw_data!=='string'||!wellFormed(item.raw_data)||!rawParent(item.raw_data,scope,parent))return bad();
  previous=item.sequence;
 }
 if(bytes(value.items)>4*1024*1024)return bad();
 return value as unknown as AttemptToolEventPage;
}
