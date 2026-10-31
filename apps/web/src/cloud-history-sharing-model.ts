import type {CloudCollaborationSource,CloudHistoryLoad,CloudHistoryEntry,CloudHistorySharePreview,CloudHistorySharePreviewRequest,CloudHistoryShareRequest,CloudHistorySharedEntry,CloudHistorySharedObservation,TaskConversationMessage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';
import {validConversationBody,validConversationMessage} from './task-conversation-model';
import {readCloudHistoryPage} from './workbuddy-cloud-collaboration-model';
import {validCloudHistorySharedObservation} from './cloud-history-shared-observation-model';

const maxVersion=2147483647,encoder=new TextEncoder();
const bytes=(value:string)=>encoder.encode(value).length;
const object=(value:unknown):value is Record<string,unknown>=>{
 if(value===null||typeof value!=='object'||Array.isArray(value))return false;
 const prototype=Object.getPrototypeOf(value);return prototype===Object.prototype||prototype===null;
};
const fields=(value:Record<string,unknown>,expected:readonly string[])=>Reflect.ownKeys(value).length===expected.length&&expected.every(key=>Object.hasOwn(value,key));
const id=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&value.trim()===value&&value!=='.'&&value!=='..'&&wellFormed(value)&&bytes(value)<=max&&!/[\/\\\p{Cc}]/u.test(value);
const positive=(value:unknown,max=maxVersion):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0&&value<=max;
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const text=(value:unknown,max:number,multiline=false):value is string=>typeof value==='string'&&wellFormed(value)&&!!value.trim()&&bytes(value)<=max&&!/[\p{Cc}]/u.test(multiline?value.replace(/[\r\n\t]/g,''):value);
const terminal=(value:unknown)=>typeof value==='string'&&['loaded','interrupted','limited','rejected'].includes(value);
const supported=(value:unknown)=>typeof value==='string'&&['user_text','assistant_text','tool_summary','plan_summary'].includes(value);
const body=(value:unknown):value is string=>validConversationBody(value)&&text(value,32000,true);
const sourceKeys=['project_id','run_id','node_id','submission_id','submission_version','task_id','node_spec_sha256','request_sha256'] as const;
function validSource(value:unknown):value is CloudCollaborationSource{
 return object(value)&&fields(value,sourceKeys)&&id(value.project_id,64)&&id(value.run_id,100)&&id(value.node_id,150)&&id(value.submission_id,64)&&positive(value.submission_version)&&typeof value.task_id==='string'&&/^[A-Za-z0-9_-]{1,200}$/.test(value.task_id)&&hash(value.node_spec_sha256)&&hash(value.request_sha256);
}
export function historySharePreviewPath(loadID:string):string{
 if(!id(loadID,64))throw new Error();
 return '/workbuddy-cloud-history-loads/'+encodeURIComponent(loadID)+'/share-preview';
}
export function historySharePath(loadID:string):string{
 if(!id(loadID,64))throw new Error();
 return '/workbuddy-cloud-history-loads/'+encodeURIComponent(loadID)+'/shares';
}
export function validHistoryShareSequences(sequences:unknown):sequences is number[]{
 return Array.isArray(sequences)&&sequences.length>0&&sequences.length<=8&&sequences.every((sequence:unknown,index)=>positive(sequence,500)&&(index===0||sequence>sequences[index-1]));
}
function selectedEntry(value:unknown):value is CloudHistoryEntry{
 return object(value)&&fields(value,['sequence','kind','text'])&&positive(value.sequence,500)&&supported(value.kind)&&text(value.text,64*1024,true);
}
export function eligibleHistoryEntry(load:CloudHistoryLoad,entry:unknown):entry is CloudHistoryEntry{
 if(!object(load)||!terminal(load.state)||!selectedEntry(entry))return false;
 // Reuse the stored-load/source guard without treating selected entries as a
 // complete archive. This one-row local page proves membership bounds only.
 return !!readCloudHistoryPage({load,items:[entry],total:load.entry_count,next_after:entry.sequence,has_more:entry.sequence<load.entry_count},load.source,load.id,entry.sequence-1,1);
}
const previewRequestKeys=['project_id','task_id','expected_task_version','expected_load_version','sequences'];
function validPreviewRequest(value:unknown,project:string):value is CloudHistorySharePreviewRequest{
 return object(value)&&fields(value,previewRequestKeys)&&id(project,64)&&value.project_id===project&&id(value.task_id,64)&&positive(value.expected_task_version,maxVersion-1)&&positive(value.expected_load_version)&&validHistoryShareSequences(value.sequences);
}
function validShareBody(value:unknown,project:string):value is CloudHistoryShareRequest{
 if(!object(value)||!fields(value,[...previewRequestKeys,'preview_sha256','idempotency_key'])||!hash(value.preview_sha256)||!id(value.idempotency_key,128)||bytes(value.idempotency_key)<8)return false;
 return validPreviewRequest({project_id:value.project_id,task_id:value.task_id,expected_task_version:value.expected_task_version,expected_load_version:value.expected_load_version,sequences:value.sequences},project);
}
function requestLoadID(request:WriteRecoveryRequest):string|undefined{
 if(typeof request.path!=='string')return;
 const route=/^\/workbuddy-cloud-history-loads\/([^/?#]+)\/shares$/.exec(request.path);
 if(!route)return;
 try{const loadID=decodeURIComponent(route[1]);if(id(loadID,64)&&historySharePath(loadID)===request.path)return loadID;}catch{/* Invalid encoded locators never become another path. */}
}
/** Metadata-only recovery never depends on private load readability or the
 * target task's later version; the server owns the original operation receipt. */
export function validHistoryShareRequest(request:WriteRecoveryRequest,project:string):boolean{
 return object(request)&&fields(request,['path','body','method','idempotency_key'])&&request.method==='POST'&&!!requestLoadID(request)&&validShareBody(request.body,project)&&request.idempotency_key===request.body.idempotency_key;
}
const previewKeys=['project_id','task_id','task_title','task_version','load_id','load_version','load_state','coverage','source','entries','body','body_sha256','selection_sha256','preview_sha256','sharing_scope'];
function previewShape(value:unknown):value is CloudHistorySharePreview{
 if(!object(value)||!fields(value,previewKeys)||!id(value.project_id,64)||!id(value.task_id,64)||!text(value.task_title,500)||!positive(value.task_version)||!id(value.load_id,64)||!positive(value.load_version)||!terminal(value.load_state)||value.coverage!=='unverified'||!validSource(value.source)||value.source.project_id!==value.project_id||!Array.isArray(value.entries)||value.entries.length<1||value.entries.length>8||!value.entries.every(selectedEntry)||!validHistoryShareSequences(value.entries.map(entry=>entry.sequence))||!body(value.body)||!hash(value.body_sha256)||!hash(value.selection_sha256)||!hash(value.preview_sha256)||value.sharing_scope!=='project_task_readers')return false;
 return value.body===value.entries.map(entry=>entry.text).join('\n\n');
}
async function textSHA(value:string):Promise<string>{
 const result=await crypto.subtle.digest('SHA-256',encoder.encode(value));
 return Array.from(new Uint8Array(result),byte=>byte.toString(16).padStart(2,'0')).join('');
}
async function entryDigests(entries:CloudHistoryEntry[]):Promise<CloudHistorySharedEntry[]>{
 return Promise.all(entries.map(async entry=>({sequence:entry.sequence,kind:entry.kind,text_sha256:await textSHA(entry.text||'')})));
}
async function selectionSHA(entries:CloudHistorySharedEntry[]):Promise<string>{
 // The fixed property order is the Go struct JSON order. All values here are
 // ASCII enums, integers and lowercase digests; no text normalization occurs.
 return textSHA(JSON.stringify(entries.map(({sequence,kind,text_sha256})=>({sequence,kind,text_sha256}))));
}
async function previewDigests(value:CloudHistorySharePreview):Promise<boolean>{
 return value.body_sha256===await textSHA(value.body)&&value.selection_sha256===await selectionSHA(await entryDigests(value.entries));
}
export async function readHistorySharePreview(value:unknown,load:CloudHistoryLoad,request:CloudHistorySharePreviewRequest,selectedEntries:CloudHistoryEntry[]):Promise<CloudHistorySharePreview|undefined>{
 if(!previewShape(value)||!validPreviewRequest(request,load.source.project_id)||!Array.isArray(selectedEntries)||!validHistoryShareSequences(selectedEntries.map(entry=>entry.sequence))||!selectedEntries.every(entry=>eligibleHistoryEntry(load,entry))||value.project_id!==request.project_id||value.task_id!==request.task_id||value.task_version!==request.expected_task_version||value.load_id!==load.id||value.load_version!==load.version||value.load_version!==request.expected_load_version||value.load_state!==load.state||!sourceKeys.every(key=>value.source[key]===load.source[key])||value.entries.length!==selectedEntries.length||value.entries.some((entry,index)=>entry.sequence!==request.sequences[index]||entry.sequence!==selectedEntries[index].sequence||entry.kind!==selectedEntries[index].kind||entry.text!==selectedEntries[index].text)||request.sequences.length!==value.entries.length)return;
 // The wire snapshot cannot be replaced while Web Crypto yields control.
 const snapshot=structuredClone(value);
 try{return await previewDigests(snapshot)?snapshot:undefined;}catch{return;}
}
function samePublicEntries(left:CloudHistorySharedEntry[],right:CloudHistorySharedEntry[]):boolean{
 return left.length===right.length&&left.every((entry,index)=>entry.sequence===right[index].sequence&&entry.kind===right[index].kind&&entry.text_sha256===right[index].text_sha256);
}
function receiptMatchesPreview(value:TaskConversationMessage,observation:CloudHistorySharedObservation,request:WriteRecoveryRequest,preview:CloudHistorySharePreview):boolean{
 return preview.project_id===request.body.project_id&&preview.task_id===request.body.task_id&&preview.task_version===request.body.expected_task_version&&preview.load_id===observation.load_id&&preview.load_version===observation.load_version&&preview.load_state===observation.load_state&&preview.source.run_id===observation.source_run_id&&preview.source.node_id===observation.source_node_id&&preview.source.submission_id===observation.source_submission_id&&preview.preview_sha256===request.body.preview_sha256&&preview.body===value.body&&preview.body_sha256===observation.body_sha256&&preview.selection_sha256===observation.selection_sha256;
}
export async function readHistoryShareReceipt(value:unknown,request:WriteRecoveryRequest,userID:string,context:{runID:string;nodeID:string},preview?:CloudHistorySharePreview):Promise<TaskConversationMessage|undefined>{
 if(!object(request)||!object(request.body)||typeof request.body.project_id!=='string'||!validHistoryShareRequest(request,request.body.project_id)||!id(userID,64)||!id(context.runID,100)||!id(context.nodeID,150)||!validConversationMessage(value)||value.actor_kind!=='human'||value.author_id!==userID||value.task_version!==request.body.expected_task_version||!body(value.body)||value.mentions!==undefined||value.knowledge!==undefined||!validCloudHistorySharedObservation(value.cloud_observation))return;
 const observation=value.cloud_observation,sequences=request.body.sequences;
 if(observation.load_id!==requestLoadID(request)||observation.load_version!==request.body.expected_load_version||observation.source_run_id!==context.runID||observation.source_node_id!==context.nodeID||!Array.isArray(sequences)||observation.entries.length!==sequences.length||observation.entries.some((entry,index)=>entry.sequence!==sequences[index]))return;
 if(preview!==undefined&&(!previewShape(preview)||!receiptMatchesPreview(value,observation,request,preview)))return;
 const snapshot=structuredClone(value),attribution=structuredClone(observation),originalPreview=preview===undefined?undefined:structuredClone(preview);
 try{
  if(attribution.body_sha256!==await textSHA(snapshot.body)||attribution.selection_sha256!==await selectionSHA(attribution.entries))return;
  if(originalPreview&&(!await previewDigests(originalPreview)||!samePublicEntries(attribution.entries,await entryDigests(originalPreview.entries))))return;
  // Message has no project/task fields. The caller must additionally read its
  // exact MessageDetail under the original project/task before acknowledging.
  return snapshot;
 }catch{return;}
}
