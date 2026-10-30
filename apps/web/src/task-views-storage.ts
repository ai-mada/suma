import {taskColumns} from './task-columns-model';

/** Personal display preferences only. This module stores no task or run facts. */
export type TaskViewPreferences={
 columns?:string[];
 q:string;
 status:string;
 owner_id:string;
 priority:string;
 sort:'updated_desc'|'created_desc'|'title_asc'|'priority_desc';
 layout:'list'|'board';
 group_by:'status'|'owner'|'priority'|'iteration';
 iteration_id?:string;
 iteration_unset?:boolean;
 requirement_id?:string;
 unassigned?:boolean;
 priority_unset?:boolean;
 status_unset?:boolean;
};
export type SavedTaskView={id:string;name:string;preferences:TaskViewPreferences};
// This envelope states literal filter semantics. An earlier numeric envelope
// cannot prove that intent and is preserved as invalid until explicit recovery.
export const taskViewsFormat='suma-task-views-literal-v1' as const;
export type TaskViewCollection={format:typeof taskViewsFormat;views:SavedTaskView[]};
export type TaskViewScope=readonly [userId:string,organizationId:string,projectId:string];
export type TaskViewStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
export type TaskViewReadResult={key:string;raw:string|null;collection:TaskViewCollection|null;error:'corrupt'|'unavailable'|null};
export type TaskViewWriteResult=Omit<TaskViewReadResult,'error'>&{error:TaskViewReadResult['error']|'conflict'|'invalid'};

const prefix='suma-task-views-v1:';
const byteLimit=64*1024;
const controls=/[\u0000-\u001f\u007f-\u009f]/;
const identifier=/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,128}$/;
const ownerIdentifier=/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/;
const bytes=(value:string)=>new TextEncoder().encode(value).length;
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&value.length<=max&&!controls.test(value)&&(value===''||!!value.trim());
const shortFilter=(value:unknown):value is string=>typeof value==='string'&&Array.from(value).length<=200&&!controls.test(value)&&(value===''||!!value.trim());

export function taskViewsKey(scope:TaskViewScope):string{return prefix+JSON.stringify(scope);}

function validScope(scope:TaskViewScope):boolean{
 return Array.isArray(scope)&&scope.length===3&&scope.every(id=>text(id,128)&&id!==''&&id.trim()===id);
}
function exactObject(value:unknown,keys:readonly string[]):value is Record<string,unknown>{
 if(value===null||typeof value!=='object'||Array.isArray(value))return false;
 const prototype=Object.getPrototypeOf(value);
 return (prototype===Object.prototype||prototype===null)&&Reflect.ownKeys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key));
}
function validPreferences(value:unknown):value is TaskViewPreferences{
 if(value===null||typeof value!=='object'||Array.isArray(value))return false;
 const prototype=Object.getPrototypeOf(value);if(prototype!==Object.prototype&&prototype!==null)return false;
 const required=['q','status','owner_id','priority','sort','layout','group_by'],optional=['columns','requirement_id','iteration_id','iteration_unset','unassigned','priority_unset','status_unset'];
 const keys=Reflect.ownKeys(value);if(!required.every(key=>Object.hasOwn(value,key))||keys.some(key=>typeof key!=='string'||!required.includes(key)&&!optional.includes(key)))return false;
 const candidate=value as Record<string,unknown>;if(!taskColumns(candidate.columns))return false;
 for(const key of ['requirement_id','iteration_id'])if(candidate[key]!==undefined&&(typeof candidate[key]!=='string'||candidate[key]!==''&&!ownerIdentifier.test(candidate[key])))return false;
 for(const key of ['unassigned','priority_unset','status_unset','iteration_unset'])if(candidate[key]!==undefined&&typeof candidate[key]!=='boolean')return false;
 if(candidate.unassigned&&candidate.owner_id!==''||candidate.priority_unset&&candidate.priority!==''||candidate.status_unset&&candidate.status!==''||candidate.iteration_unset&&!!candidate.iteration_id)return false;
 return typeof candidate.q==='string'&&Array.from(candidate.q).length<=200&&!controls.test(candidate.q)&&shortFilter(candidate.status)&&shortFilter(candidate.priority)
  &&typeof candidate.owner_id==='string'&&(candidate.owner_id===''||ownerIdentifier.test(candidate.owner_id))
  &&['updated_desc','created_desc','title_asc','priority_desc'].includes(candidate.sort as string)
  &&['list','board'].includes(candidate.layout as string)&&['status','owner','priority','iteration'].includes(candidate.group_by as string);
}
function validCollection(value:unknown):value is TaskViewCollection{
 if(!exactObject(value,['format','views'])||value.format!==taskViewsFormat||!Array.isArray(value.views)||value.views.length>20)return false;
 const ids=new Set<string>();
 for(const item of value.views){
  if(!exactObject(item,['id','name','preferences'])||typeof item.id!=='string'||!identifier.test(item.id)||ids.has(item.id)
   ||!text(item.name,80)||!item.name||item.name.trim()!==item.name||!validPreferences(item.preferences))return false;
  ids.add(item.id);
 }
 return true;
}

/** JSON.parse accepts duplicate properties; reject them within each object. */
function hasDuplicateKeys(raw:string):boolean{
 const scopes:(Set<string>|null)[]=[];
 for(const token of raw.matchAll(/"(?:\\.|[^"\\])*"|[{}\[\]]/g)){
  if(token[0]==='{'){scopes.push(new Set());continue;}
  if(token[0]==='['){scopes.push(null);continue;}
  if(token[0]==='}'||token[0]===']'){scopes.pop();continue;}
  if(!/^\s*:/.test(raw.slice(token.index+token[0].length)))continue;
  const scope=scopes.at(-1),key=JSON.parse(token[0]) as string;
  if(!scope||scope.has(key))return true;
  scope.add(key);
 }
 return false;
}
function decode(key:string,raw:string|null):TaskViewReadResult{
 if(raw===null)return {key,raw,collection:{format:taskViewsFormat,views:[]},error:null};
 try{
  if(raw.length>byteLimit||bytes(raw)>byteLimit)throw new Error('oversized');
  const collection:unknown=JSON.parse(raw);
  if(!validCollection(collection)||hasDuplicateKeys(raw))throw new Error('invalid');
  return {key,raw,collection,error:null};
 }catch{return {key,raw,collection:null,error:'corrupt'};}
}
export function readTaskViews(storage:TaskViewStorage|null|undefined,scope:TaskViewScope):TaskViewReadResult{
 const key=taskViewsKey(scope);
 try{
  if(!storage||!validScope(scope))throw new Error('unavailable');
  return decode(key,storage.getItem(key));
 }catch{return {key,raw:null,collection:null,error:'unavailable'};}
}

/** Compare the last observed raw value and verify after writing. Web Storage
 * provides no cross-process transaction: a later storage event still requires
 * an explicit reload. Never compensate a race by overwriting the newer value. */
export function writeTaskViews(storage:TaskViewStorage|null|undefined,scope:TaskViewScope,views:readonly SavedTaskView[],expectedRaw:string|null):TaskViewWriteResult{
 const current=readTaskViews(storage,scope);
 if(current.error==='unavailable')return current;
 if(typeof expectedRaw!=='string'&&expectedRaw!==null)return {...current,error:'invalid'};
 if(current.raw!==expectedRaw)return {...current,error:'conflict'};
 if(current.error)return current;
 let raw:string;
 try{
  const collection={format:taskViewsFormat,views};
  if(!validCollection(collection))return {...current,error:'invalid'};
  raw=JSON.stringify(collection);
  if(bytes(raw)>byteLimit)return {...current,error:'invalid'};
 }catch{return {...current,error:'invalid'};}
 try{storage!.setItem(current.key,raw);}catch{return {...current,error:'unavailable'};}
 const confirmed=readTaskViews(storage,scope);
 if(confirmed.error==='unavailable')return confirmed;
 if(confirmed.raw!==raw)return {...confirmed,error:'conflict'};
 return confirmed;
}

/** Call only after explicit user confirmation, including for corrupt data. */
export function discardTaskViews(storage:TaskViewStorage|null|undefined,scope:TaskViewScope,expectedRaw:string|null):TaskViewWriteResult{
 const current=readTaskViews(storage,scope);
 if(current.error==='unavailable')return current;
 if(typeof expectedRaw!=='string'&&expectedRaw!==null)return {...current,error:'invalid'};
 if(current.raw!==expectedRaw)return {...current,error:'conflict'};
 try{storage!.removeItem(current.key);}catch{return {...current,error:'unavailable'};}
 const confirmed=readTaskViews(storage,scope);
 if(confirmed.error==='unavailable')return confirmed;
 if(confirmed.raw!==null)return {...confirmed,error:'conflict'};
 return confirmed;
}
