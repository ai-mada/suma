import type {CloudCollaborationSource,CloudCollaborationView,CloudHistoryLoadRequest,CloudHistoryLoad,CloudHistoryEntry,CloudHistoryPage} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';
import {wellFormed} from './i18n/catalog';

const maxVersion=2147483647,maxEntries=500,maxEntryBytes=64*1024,maxTextBytes=1024*1024;
const encoder=new TextEncoder();
const bytes=(value:string)=>encoder.encode(value).length;
const object=(value:unknown):value is Record<string,unknown>=>{
 if(value===null||typeof value!=='object'||Array.isArray(value))return false;
 const prototype=Object.getPrototypeOf(value);
 return prototype===Object.prototype||prototype===null;
};
const fields=(value:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&(required.includes(key)||optional.includes(key)));
const integer=(value:unknown,min:number,max:number):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min&&value<=max;
const identifier=(value:unknown,max:number):value is string=>typeof value==='string'&&!!value&&wellFormed(value)&&value.trim()===value&&bytes(value)<=max&&!/[\/\\\p{Cc}]/u.test(value);
const operationID=(value:unknown):value is string=>identifier(value,128)&&bytes(value)>=8;
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const sourceKeys=['project_id','run_id','node_id','submission_id','submission_version','task_id','node_spec_sha256','request_sha256'];
function validSource(value:unknown):value is CloudCollaborationSource{
 return object(value)&&fields(value,sourceKeys)&&identifier(value.project_id,64)&&identifier(value.run_id,100)&&identifier(value.node_id,150)&&identifier(value.submission_id,64)&&integer(value.submission_version,1,maxVersion)&&typeof value.task_id==='string'&&/^[A-Za-z0-9_-]{1,200}$/.test(value.task_id)&&hash(value.node_spec_sha256)&&hash(value.request_sha256);
}
/** A source locator is not caller authorization. The container adds its current
 * actor/organization boundary; observation-only version growth keeps this ID. */
export function collaborationIdentity(source:CloudCollaborationSource):string{
 if(!validSource(source))throw new Error();
 return JSON.stringify([source.project_id,source.run_id,source.node_id,source.submission_id,source.task_id,source.node_spec_sha256,source.request_sha256]);
}
function sameSource(value:CloudCollaborationSource,source:CloudCollaborationSource):boolean{
 return collaborationIdentity(value)===collaborationIdentity(source);
}
export function collaborationPath(source:CloudCollaborationSource):string{
 if(!validSource(source))throw new Error();
 return '/workbuddy-cloud-submissions/'+encodeURIComponent(source.submission_id)+'/collaboration?'+new URLSearchParams({project_id:source.project_id});
}
export function historyLoadPath(source:CloudCollaborationSource):string{
 if(!validSource(source))throw new Error();
 return '/workbuddy-cloud-submissions/'+encodeURIComponent(source.submission_id)+'/history-loads';
}
const validPageQuery=(after:number,limit:number)=>integer(after,0,maxEntries)&&integer(limit,1,100);
export function historyPagePath(loadID:string,project:string,after=0,limit=50):string{
 if(!identifier(loadID,64)||!identifier(project,64)||!validPageQuery(after,limit))throw new Error();
 return '/workbuddy-cloud-history-loads/'+encodeURIComponent(loadID)+'?'+new URLSearchParams({project_id:project,after:String(after),limit:String(limit)});
}

/** Validate UTC platform timestamps, including calendar and nanosecond order.
 * No browser clock is used to decide whether a load is overdue. */
function instant(value:unknown):bigint|undefined{
 if(typeof value!=='string'||value.length>40)return;
 const match=/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/.exec(value);
 if(!match)return;
 const millis=Date.parse(match[1]+'Z');
 if(!Number.isFinite(millis)||new Date(millis).toISOString().slice(0,19)!==match[1])return;
 const nanos=BigInt(millis)*1000000n+BigInt((match[2]||'').padEnd(9,'0'));
 if(match[1]==='0001-01-01T00:00:00'&&(!match[2]||/^0+$/.test(match[2])))return;
 return nanos;
}
const loadKeys=['id','version','operation_id','head_version_at_start','source','state','coverage','started_at','deadline_at','overdue','entry_count','unsupported_count','text_bytes'];
const safeErrors=['workbuddy_cloud_history_unavailable','workbuddy_cloud_history_interrupted','workbuddy_cloud_history_limited','workbuddy_cloud_history_rejected'];
function validLoad(value:unknown,source:CloudCollaborationSource):value is CloudHistoryLoad{
 if(!object(value)||!fields(value,loadKeys,['finished_at','error_code'])||!identifier(value.id,64)||!integer(value.version,1,maxVersion)||!operationID(value.operation_id)||!integer(value.head_version_at_start,0,maxVersion-1)||!validSource(value.source)||!sameSource(value.source,source)||value.source.submission_version>source.submission_version||typeof value.state!=='string'||value.coverage!=='unverified'||typeof value.overdue!=='boolean'||!integer(value.entry_count,0,maxEntries)||!integer(value.unsupported_count,0,value.entry_count)||!integer(value.text_bytes,0,maxTextBytes)||value.entry_count===0&&value.text_bytes!==0)return false;
 const started=instant(value.started_at),deadline=instant(value.deadline_at);
 if(started===undefined||deadline===undefined||deadline<=started)return false;
 if(value.state==='loading')return !Object.hasOwn(value,'finished_at')&&!Object.hasOwn(value,'error_code');
 const finished=instant(value.finished_at);
 if(finished===undefined||finished<started||value.overdue)return false;
 if(value.state==='loaded')return !Object.hasOwn(value,'error_code');
 return ['interrupted','limited','rejected'].includes(value.state)&&typeof value.error_code==='string'&&safeErrors.includes(value.error_code);
}
function validView(value:unknown,source:CloudCollaborationSource):value is CloudCollaborationView{
 if(!validSource(source)||!object(value)||!fields(value,['source','version','allowed_actions'],['latest_load','last_loaded'])||!validSource(value.source)||!sameSource(value.source,source)||value.source.submission_version<source.submission_version||!integer(value.version,0,maxVersion)||!Array.isArray(value.allowed_actions)||value.allowed_actions.length>1||value.allowed_actions.some(action=>action!=='load_history'))return false;
 if(value.version===0&&(Object.hasOwn(value,'latest_load')||Object.hasOwn(value,'last_loaded'))||value.version>0&&!Object.hasOwn(value,'latest_load'))return false;
 if(Object.hasOwn(value,'latest_load')&&!validLoad(value.latest_load,value.source))return false;
 if(Object.hasOwn(value,'last_loaded')&&(!validLoad(value.last_loaded,value.source)||value.last_loaded.state!=='loaded'))return false;
 return true;
}
export function readCloudCollaboration(value:unknown,source:CloudCollaborationSource):CloudCollaborationView|undefined{
 return validView(value,source)?value:undefined;
}
function validEntry(value:unknown):value is CloudHistoryEntry{
 if(!object(value)||!integer(value.sequence,1,maxEntries)||typeof value.kind!=='string')return false;
 if(value.kind==='artifact'){
  return fields(value,['sequence','kind','artifact_type','artifact_event'])&&
   typeof value.artifact_type==='string'&&['plan','tasks','media','overview'].includes(value.artifact_type)&&
   typeof value.artifact_event==='string'&&['created','updated','deleted'].includes(value.artifact_event);
 }
 if(!fields(value,['sequence','kind'],['text']))return false;
 if(value.kind==='unsupported')return !Object.hasOwn(value,'text');
 if(!['user_text','assistant_text','tool_summary','plan_summary'].includes(value.kind))return false;
 return !Object.hasOwn(value,'text')||typeof value.text==='string'&&wellFormed(value.text)&&bytes(value.text)<=maxEntryBytes&&!/[\p{Cc}]/u.test(value.text.replace(/[\r\n\t]/g,''));
}
function validPage(value:unknown,source:CloudCollaborationSource,loadID:string,after:number,limit:number):value is CloudHistoryPage{
 if(!validSource(source)||!identifier(loadID,64)||!validPageQuery(after,limit)||!object(value)||!fields(value,['load','items','total','next_after','has_more'])||!validLoad(value.load,source)||value.load.id!==loadID||!Array.isArray(value.items)||!integer(value.total,0,maxEntries)||value.total!==value.load.entry_count||!integer(value.next_after,0,maxEntries)||typeof value.has_more!=='boolean')return false;
 // The API pages a dense locally stored sequence, not a supplier cursor. A
 // missing row or wrong continuation must not silently discard conversation.
 if(value.items.length!==Math.min(limit,Math.max(0,value.total-after))||value.next_after!==after+value.items.length||value.has_more!==(value.next_after<value.total))return false;
 let textBytes=0,unsupported=0;
 for(let index=0;index<value.items.length;index++){
  const entry:unknown=value.items[index];
  if(!validEntry(entry)||entry.sequence!==after+index+1||entry.sequence>value.total)return false;
  if(entry.kind==='unsupported')unsupported++;
  else textBytes+=bytes(entry.text||'');
 }
 if(textBytes>value.load.text_bytes||unsupported>value.load.unsupported_count)return false;
 if(after===0&&!value.has_more&&(textBytes!==value.load.text_bytes||unsupported!==value.load.unsupported_count))return false;
 return true;
}
export function readCloudHistoryPage(value:unknown,source:CloudCollaborationSource,loadID:string,after=0,limit=50):CloudHistoryPage|undefined{
 return validPage(value,source,loadID,after,limit)?value:undefined;
}
function validLoadRequest(value:unknown,source:CloudCollaborationSource):value is CloudHistoryLoadRequest{
 return object(value)&&fields(value,['project_id','expected_submission_version','expected_version','idempotency_key'])&&value.project_id===source.project_id&&integer(value.expected_submission_version,1,maxVersion-1)&&value.expected_submission_version<=source.submission_version&&integer(value.expected_version,0,maxVersion-1)&&operationID(value.idempotency_key);
}
export function validCloudHistoryRequest(request:WriteRecoveryRequest,source:CloudCollaborationSource):boolean{
 if(!validSource(source)||!object(request)||!fields(request,['path','body','method','idempotency_key'])||request.method!=='POST'||request.path!==historyLoadPath(source)||!validLoadRequest(request.body,source))return false;
 return request.idempotency_key===request.body.idempotency_key;
}
/** Bind the immutable admission facts, never compare the independent mutable
 * load.version against the collaboration head's original precondition. */
export function readCloudHistoryReceipt(value:unknown,request:WriteRecoveryRequest,source:CloudCollaborationSource):CloudHistoryLoad|undefined{
 if(!validCloudHistoryRequest(request,source)||!validLoad(value,source))return;
 return value.source.submission_version===request.body.expected_submission_version&&value.operation_id===request.idempotency_key&&value.head_version_at_start===request.body.expected_version?value:undefined;
}
