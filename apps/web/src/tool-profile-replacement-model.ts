import type {ToolExecutionProfile,ToolProfile,ToolProfileReplacement,ToolProfileReplacementList,ToolProfileReplacementRequest} from '../../../packages/contracts/src';
import type {ToolPendingRequest} from './tool-write';
import {UiError} from './i18n';
import {readToolProfileMatches,sameToolProfile} from './tool-profile-reuse-model';

/** Public UI draft only. The original source is separate from editable target
 * fields and never replaces an uncertain operation's persisted request. */
export type ToolProfileReplacementDraft={original:ToolProfile;workerID:string;profile:ToolExecutionProfile;name?:string;reuse?:ToolProfile;pauseOriginal:boolean};
export type ToolProfileReplacementBody=Omit<ToolProfileReplacementRequest,'idempotency_key'>;
export type ToolProfileReplacementDirection=ToolProfileReplacementList['direction'];
export type ToolProfileReplacementReadScope={project:string;originalID?:string;replacementID?:string};
export type ToolProfileReplacementListScope={project:string;profileID:string;direction:ToolProfileReplacementDirection};

const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const closed=(value:Record<string,unknown>,required:string[],optional:string[]=[])=>required.every(key=>Object.hasOwn(value,key))&&Reflect.ownKeys(value).every(key=>typeof key==='string'&&[...required,...optional].includes(key));
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(value);
const version=(value:unknown):value is number=>Number.isSafeInteger(value)&&(value as number)>0&&(value as number)<=2147483647;
const encoder=new TextEncoder();
const malformedUnicode=/(?:[\uD800-\uDBFF](?![\uDC00-\uDFFF]))|(?:(?<![\uD800-\uDBFF])[\uDC00-\uDFFF])/u;
const text=(value:unknown,max:number,min=1):value is string=>typeof value==='string'&&value.trim()===value&&encoder.encode(value).length>=min&&encoder.encode(value).length<=max&&!/\p{Cc}/u.test(value)&&!malformedUnicode.test(value);
function instant(value:unknown):value is string{
 if(typeof value!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.test(value)||!Number.isFinite(Date.parse(value)))return false;
 const date=new Date(value.slice(0,19)+'Z');return Number.isFinite(date.getTime())&&date.toISOString().slice(0,19)===value.slice(0,19);
}
function bytes(value:unknown):number{try{const json=JSON.stringify(value);return typeof json==='string'?encoder.encode(json).length:Infinity;}catch{return Infinity;}}
function snapshot(value:unknown,project:string):value is ToolProfile{
 if(!record(value)||!id(value.id)||value.project_id!==project||!id(value.worker_id)||!version(value.version)||!text(value.name,200)||!['paused','enabled'].includes(String(value.state))||value.state==='paused'&&value.available!==false||!instant(value.created_at)||!instant(value.updated_at))return false;
 try{readToolProfileMatches({project_id:project,worker_id:value.worker_id,profile:value.profile,items:[value],total:1},{project_id:project,worker_id:value.worker_id,profile:value.profile as ToolExecutionProfile});return true;}catch{return false;}
}
const operation=(value:unknown)=>text(value,128,8);
export function toolProfileReplacementPath(originalID:string):string{return '/tool-profiles/'+encodeURIComponent(originalID)+'/replacements';}
export function toolProfileReplacementOriginalID(path:string):string|undefined{return /^\/tool-profiles\/([A-Za-z0-9][A-Za-z0-9_.-]{0,99})\/replacements$/.exec(path)?.[1];}
export function isToolProfileReplacementPath(path:string):boolean{return toolProfileReplacementOriginalID(path)!==undefined;}
export function validToolProfileReplacementBody(value:unknown,originalID:string):value is ToolProfileReplacementBody{
 if(!id(originalID)||!record(value)||!closed(value,['project_id','expected_version','worker_id','profile','pause_original'],['name','reuse'])||!id(value.project_id)||!version(value.expected_version)||!id(value.worker_id)||!sameToolProfile(value.profile,value.profile)||typeof value.pause_original!=='boolean')return false;
 if(Object.hasOwn(value,'reuse'))return !Object.hasOwn(value,'name')&&record(value.reuse)&&closed(value.reuse,['profile_id','expected_version'])&&id(value.reuse.profile_id)&&value.reuse.profile_id!==originalID&&version(value.reuse.expected_version);
 return Object.hasOwn(value,'name')&&text(value.name,200);
}
export function validToolProfileReplacementRequest(value:unknown):value is ToolPendingRequest&{body:ToolProfileReplacementBody}{
 if(!record(value)||!closed(value,['path','body','method','idempotency_key'])||typeof value.path!=='string'||value.method!=='POST'||!operation(value.idempotency_key))return false;
 const originalID=toolProfileReplacementOriginalID(value.path);return !!originalID&&validToolProfileReplacementBody(value.body,originalID);
}
export function buildToolProfileReplacementBody(draft:ToolProfileReplacementDraft):ToolProfileReplacementBody{
 if(!draft||!snapshot(draft.original,draft.original?.project_id)||typeof draft.pauseOriginal!=='boolean')throw new UiError('tool-profile-replacement.invalid_request');
 const reuse=draft.reuse;
 if(reuse&&(!snapshot(reuse,draft.original.project_id)||reuse.id===draft.original.id||reuse.worker_id!==draft.workerID||!sameToolProfile(reuse.profile,draft.profile)))throw new UiError('tool-profile-replacement.invalid_request');
 const value={project_id:draft.original.project_id,expected_version:draft.original.version,worker_id:draft.workerID,profile:draft.profile,pause_original:draft.pauseOriginal,...(reuse?{reuse:{profile_id:reuse.id,expected_version:reuse.version}}:{name:draft.name?.trim()})};
 if(!validToolProfileReplacementBody(value,draft.original.id))throw new UiError('tool-profile-replacement.invalid_request');
 return JSON.parse(JSON.stringify(value)) as ToolProfileReplacementBody;
}
export function readToolProfileReplacement(value:unknown,scope:ToolProfileReplacementReadScope):ToolProfileReplacement{
 if(scope.originalID!==undefined&&!id(scope.originalID)||scope.replacementID!==undefined&&!id(scope.replacementID))throw new UiError('tool-profile-replacement.invalid_receipt');
 if(record(value)&&closed(value,['id','project_id','original','replacement','disposition','pause_original','created_at','created_by'])&&id(value.id)&&id(value.created_by)&&value.project_id===scope.project&&id(scope.project)&&(!scope.replacementID||value.id===scope.replacementID)&&snapshot(value.original,scope.project)&&snapshot(value.replacement,scope.project)&&value.original.id!==value.replacement.id&&(!scope.originalID||value.original.id===scope.originalID)&&typeof value.pause_original==='boolean'&&(!value.pause_original||value.original.state==='paused')&&instant(value.created_at)&&bytes(value)<=2*1024*1024){
  if(value.disposition==='reused'||value.disposition==='created'&&value.replacement.state==='paused'&&value.replacement.version===1&&value.replacement.enabled_by==='')return value as unknown as ToolProfileReplacement;
 }
 throw new UiError('tool-profile-replacement.invalid_receipt');
}
/** Bind only the submitted original request. A replay must never be compared
 * with a newer UI selection, profile version or current health observation. */
export function validToolProfileReplacementReceipt(value:unknown,request:ToolPendingRequest,actorID?:string):boolean{
 if(!validToolProfileReplacementRequest(request))return false;
 const originalID=toolProfileReplacementOriginalID(request.path)!,body=request.body;
 try{
  const reply=readToolProfileReplacement(value,{project:body.project_id,originalID});
  if(actorID!==undefined&&reply.created_by!==actorID||reply.pause_original!==body.pause_original||reply.replacement.worker_id!==body.worker_id||!sameToolProfile(reply.replacement.profile,body.profile))return false;
  if(body.pause_original?reply.original.version!==body.expected_version&&reply.original.version!==body.expected_version+1:reply.original.version!==body.expected_version)return false;
  return body.reuse?reply.disposition==='reused'&&reply.replacement.id===body.reuse.profile_id&&reply.replacement.version===body.reuse.expected_version:reply.disposition==='created'&&reply.replacement.name===body.name;
 }catch{return false;}
}
export function readToolProfileReplacementList(value:unknown,scope:ToolProfileReplacementListScope):ToolProfileReplacementList{
 try{
  if(!record(value)||!closed(value,['project_id','profile_id','direction','items'],['next_cursor'])||!id(scope.project)||!id(scope.profileID)||!['outgoing','incoming'].includes(scope.direction)||value.project_id!==scope.project||value.profile_id!==scope.profileID||value.direction!==scope.direction||!Array.isArray(value.items)||value.items.length>100||Object.hasOwn(value,'next_cursor')&&!text(value.next_cursor,2048))throw Error();
  const seen=new Set<string>();let pageBytes=0;
  for(const item of value.items){const row=readToolProfileReplacement(item,{project:scope.project});if(seen.has(row.id)||(scope.direction==='outgoing'?row.original.id:row.replacement.id)!==scope.profileID)throw Error();seen.add(row.id);pageBytes+=bytes(row);}
  if(pageBytes>8*1024*1024)throw Error();
  return value as unknown as ToolProfileReplacementList;
 }catch{throw new UiError('tool-profile-replacement.invalid_history');}
}
