import type {ToolProfileMatchRequest,ToolProfileMatchList} from '../../../packages/contracts/src';
import {UiError} from './i18n';
import {parseToolManifest} from './tool-model';

function record(value:unknown):value is Record<string,unknown>{
 return !!value && typeof value==='object' && !Array.isArray(value) && [Object.prototype,null].includes(Object.getPrototypeOf(value));
}
function closed(value:Record<string,unknown>,required:string[],optional:string[]=[]):boolean{
 return required.every(key=>Object.hasOwn(value,key)) && Reflect.ownKeys(value).every(key=>typeof key==='string' && [...required,...optional].includes(key));
}
function canonical(value:unknown):string{
 if(value===null || typeof value==='string' || typeof value==='boolean' || typeof value==='number' && Number.isFinite(value))return JSON.stringify(value);
 if(Array.isArray(value))return '['+value.map(canonical).join(',')+']';
 if(record(value))return '{'+Object.keys(value).sort().map(key=>JSON.stringify(key)+':'+canonical(value[key])).join(',')+'}';
 throw new Error('Invalid public JSON');
}
function validProfile(value:unknown):boolean{
 if(!record(value) || !['command_id','adapter','protocol','profile_sha256','executable_sha256','tool_version','read_only'].every(key=>Object.hasOwn(value,key)))return false;
 if(!['profile_sha256','executable_sha256'].every(key=>typeof value[key]==='string' && /^[a-f0-9]{64}$/.test(value[key])) || typeof value.tool_version!=='string' || !value.tool_version.trim() || typeof value.read_only!=='boolean')return false;
 try{parseToolManifest('['+canonical({profile:value,available:false,reason:'validation'})+']');return true;}catch{return false;}
}
/** Compare the complete immutable configuration, independent of JSON key order. */
export function sameToolProfile(a:unknown,b:unknown):boolean{
 return validProfile(a) && validProfile(b) && canonical(a)===canonical(b);
}
export function toolProfileMatchKey(request:ToolProfileMatchRequest):string{return canonical(request);}
function validDiagnostic(value:unknown):boolean{
 if(value===null)return true;
 if(!record(value) || !closed(value,['schema_version','message_key'],['message_params']) || value.schema_version!==1 || typeof value.message_key!=='string')return false;
 return !Object.hasOwn(value,'message_params') || record(value.message_params) && Object.values(value.message_params).every(param=>typeof param==='string' || typeof param==='number' && Number.isFinite(param));
}
function validObservation(value:unknown):boolean{
 if(value===null)return true;
 try{parseToolManifest('['+canonical(value)+']');return true;}catch{return false;}
}
function validList(value:unknown,request:ToolProfileMatchRequest):value is ToolProfileMatchList{
 if(!record(value) || !closed(value,['project_id','worker_id','profile','items','total']) || value.project_id!==request.project_id || value.worker_id!==request.worker_id || !sameToolProfile(value.profile,request.profile) || !Array.isArray(value.items) || value.items.length>1000 || value.total!==value.items.length)return false;
 const seen=new Set<string>();
 return value.items.every(item=>{
  if(!record(item) || !closed(item,['id','project_id','worker_id','name','version','state','profile','enabled_by','created_at','updated_at','available','reason'],['observation','last_seen_at','reason_diagnostic']))return false;
  if(typeof item.id!=='string' || !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/.test(item.id) || seen.has(item.id) || item.project_id!==request.project_id || item.worker_id!==request.worker_id || !sameToolProfile(item.profile,request.profile))return false;
  if(!Number.isSafeInteger(item.version) || (item.version as number)<1 || typeof item.state!=='string' || !['paused','enabled','revoked'].includes(item.state) || typeof item.available!=='boolean' || !['name','enabled_by','created_at','updated_at','reason'].every(key=>typeof item[key]==='string'))return false;
  if(Object.hasOwn(item,'last_seen_at') && typeof item.last_seen_at!=='string' || Object.hasOwn(item,'reason_diagnostic') && !validDiagnostic(item.reason_diagnostic) || Object.hasOwn(item,'observation') && !validObservation(item.observation))return false;
  seen.add(item.id);return true;
 });
}
export function readToolProfileMatches(value:unknown,request:ToolProfileMatchRequest):ToolProfileMatchList{
 if(validList(value,request))return value;
 throw new UiError('tool-profile-reuse.invalid_response');
}
