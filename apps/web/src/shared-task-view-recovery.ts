import {exactTaskViewObject,hasDuplicateTaskViewJSONKeys,validTaskViewPreferences,validTaskViewScope,type TaskViewScope} from './task-views-storage';

export type SharedTaskViewWrite={path:string;method:string;body:Record<string,unknown>;remove:boolean;id?:string};
type RecoveryRead={command:SharedTaskViewWrite|null;error:'invalid'|'unavailable'|null};

const format='suma-shared-task-view-write-v1';
const collection='/task-views';
const maxBytes=8192;
const encoder=new TextEncoder();
const controls=/[\u0000-\u001f\u007f-\u009f]/;
const unsafeID=/[\u0000-\u0020\u007f/\\]/;
const bounded=(raw:string)=>raw.length<=maxBytes&&encoder.encode(raw).length<=maxBytes;
const referenceID=(id:string)=>id!==''&&encoder.encode(id).length<=64&&!unsafeID.test(id);
const mutationID=(id:unknown):id is string=>typeof id==='string'&&id!=='.'&&id!=='..'&&referenceID(id);
const version=(value:unknown)=>typeof value==='number'&&Number.isInteger(value)&&value>0&&value<2147483647;

export function sharedTaskViewRecoveryKey(scope:TaskViewScope):string{
 return format+':'+JSON.stringify(scope);
}

function validCommand(value:unknown,project:string):value is SharedTaskViewWrite{
 const targeting=!!value&&typeof value==='object'&&Object.hasOwn(value,'id');
 const fields=targeting?['path','method','body','remove','id']:['path','method','body','remove'];
 if(!exactTaskViewObject(value,fields)||typeof value.remove!=='boolean'||targeting&&!mutationID(value.id))return false;
 const path=targeting?collection+'/'+encodeURIComponent(value.id as string):collection;
 const method=targeting&&!value.remove?'PATCH':'POST';
 if(value.path!==path+(value.remove?'/archive':'')||value.method!==method||value.remove&&!targeting)return false;

 const base=['project_id','idempotency_key'];
 const bodyFields=value.remove?[...base,'expected_version']:[...base,'name','visibility','preferences',...(targeting?['expected_version']:[])];
 const body=value.body;
 if(!exactTaskViewObject(body,bodyFields)||body.project_id!==project||typeof body.idempotency_key!=='string'||!/^[A-Za-z0-9_.-]{8,128}$/.test(body.idempotency_key))return false;
 if(targeting&&!version(body.expected_version))return false;
 return value.remove||typeof body.name==='string'&&body.name.trim()===body.name&&body.name.length>0&&Array.from(body.name).length<=80&&!controls.test(body.name)
  &&(body.visibility==='private'||body.visibility==='project')&&validTaskViewPreferences(body.preferences,referenceID);
}

function decode(raw:string|null,scope:TaskViewScope):RecoveryRead{
 if(raw===null)return {command:null,error:null};
 try{
  if(bounded(raw)){
   const envelope:unknown=JSON.parse(raw);
   if(exactTaskViewObject(envelope,['format','scope','command'])&&envelope.format===format&&JSON.stringify(envelope.scope)===JSON.stringify(scope)
    &&validCommand(envelope.command,scope[2])&&!hasDuplicateTaskViewJSONKeys(raw))return {command:envelope.command,error:null};
  }
 }catch{/* Leave the original entry available for explicit discard. */}
 return {command:null,error:'invalid'};
}

function scopedKey(scope:TaskViewScope):string{
 if(!validTaskViewScope(scope)||!referenceID(scope[2]))throw new Error('Invalid shared view recovery scope');
 return sharedTaskViewRecoveryKey(scope);
}

export function readSharedTaskViewRecovery(scope:TaskViewScope):RecoveryRead{
 try{return decode(sessionStorage.getItem(scopedKey(scope)),scope);}
 catch{return {command:null,error:'unavailable'};}
}

/** Save before sending so even an interrupted first request can be reconciled. */
export function persistSharedTaskViewRecovery(scope:TaskViewScope,command:SharedTaskViewWrite):boolean{
 try{
  const key=scopedKey(scope);
  const raw=JSON.stringify({format,scope,command});
  if(!decode(raw,scope).command)return false;
  const original=sessionStorage.getItem(key);
  if(original!==null)return JSON.stringify(decode(original,scope).command)===JSON.stringify(command);
  sessionStorage.setItem(key,raw);
  return sessionStorage.getItem(key)===raw;
 }catch{return false;}
}

export function clearSharedTaskViewRecovery(scope:TaskViewScope,command:SharedTaskViewWrite):boolean{
 try{
  const key=scopedKey(scope);
  const original=sessionStorage.getItem(key);
  if(original===null)return true;
  const persisted=decode(original,scope).command;
  if(!persisted||JSON.stringify(persisted)!==JSON.stringify(command))return false;
  sessionStorage.removeItem(key);
  return sessionStorage.getItem(key)===null;
 }catch{return false;}
}

/** Access denial invalidates the entire recovery entry for this exact scope. */
export function purgeSharedTaskViewRecovery(scope:TaskViewScope):boolean{
 try{
  const key=scopedKey(scope);
  sessionStorage.removeItem(key);
  return sessionStorage.getItem(key)===null;
 }catch{return false;}
}

export function discardInvalidSharedTaskViewRecovery(scope:TaskViewScope):boolean{
 try{
  const key=scopedKey(scope);
  if(decode(sessionStorage.getItem(key),scope).error!=='invalid')return false;
  sessionStorage.removeItem(key);
  return sessionStorage.getItem(key)===null;
 }catch{return false;}
}
