import type {RecordItem} from './api';
import {parseToolManifest,taskToolProtocol,type ToolProfile} from './tool-model';
import {sameToolProfile} from './tool-profile-reuse-model';

/** An in-memory navigation intent, never a permission or an execution binding.
 * Keep model choices, budgets, credentials and the full configuration out of it.
 */
export type ToolTaskHandoffPin={
  project_id:string;
  id:string;
  version:number;
  worker_id:string;
  profile_sha256:string;
  name:string;
};
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,199}$/.test(value);
const positive=(value:unknown):value is number=>Number.isSafeInteger(value)&&Number(value)>0;
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const display=(value:unknown):value is string=>typeof value==='string'&&value.length>0&&value.length<=1000&&!/[\u0000-\u001f\u007f]/.test(value);

/** Reuse the original complete profile and observation parsers. Availability is
 * the server's observation at this read, not a task/route compatibility verdict.
 * Preview and Start remain responsible for all current domain gates.
 */
function currentWritableProfile(raw:unknown,project:string):raw is ToolProfile{
  if(!object(raw)||!id(project)||raw.project_id!==project||!id(raw.id)||!id(raw.worker_id)||!positive(raw.version)||!display(raw.name)||raw.state!=='enabled'||raw.available!==true||!object(raw.profile)||raw.profile.read_only!==false||!sameToolProfile(raw.profile,raw.profile))return false;
  if(!['enabled_by','created_at','updated_at','reason'].every(key=>typeof raw[key]==='string'))return false;
  try{
    const [observation]=parseToolManifest(JSON.stringify([raw.observation]));
    return !!observation&&observation.available&&sameToolProfile(observation.profile,raw.profile)&&taskToolProtocol(observation.profile);
  }catch{return false;}
}

export function validToolTaskHandoffPin(value:unknown):value is ToolTaskHandoffPin{
  return object(value)&&Object.keys(value).length===6&&id(value.project_id)&&id(value.id)&&positive(value.version)&&id(value.worker_id)&&digest(value.profile_sha256)&&display(value.name);
}

export function createToolTaskHandoffPin(tool:ToolProfile,project:string):ToolTaskHandoffPin|undefined{
  if(!currentWritableProfile(tool,project))return undefined;
  return {project_id:project,id:tool.id,version:tool.version,worker_id:tool.worker_id,profile_sha256:tool.profile.profile_sha256,name:tool.name};
}

/** Resolve by exact ID, independent of directory pagination or display name.
 * A changed version, machine or immutable hash needs a new explicit handoff.
 */
export function readToolTaskHandoffProfile(raw:unknown,pin:ToolTaskHandoffPin):ToolProfile|undefined{
  if(!validToolTaskHandoffPin(pin)||!currentWritableProfile(raw,pin.project_id))return undefined;
  return raw.id===pin.id&&raw.version===pin.version&&raw.worker_id===pin.worker_id&&raw.profile.profile_sha256===pin.profile_sha256?raw:undefined;
}

/** The exact record is reread before opening the original dispatch dialog.
 * Adopt its current version; a list row's version never authorizes a write.
 */
export function readToolTaskHandoffTask(raw:unknown,project:string,taskID:string):RecordItem|undefined{
  if(!id(project)||!id(taskID)||!object(raw)||raw.project_id!==project||raw.id!==taskID||!positive(raw.version))return undefined;
  for(const key of ['title','name','description','status','priority'])if(raw[key]!==undefined&&typeof raw[key]!=='string')return undefined;
  // useApi normalizes the canonical Record's criteria array to multiline text.
  // The picker and exact task read share that current caller boundary. Empty
  // projections may omit criteria; null carries the original empty value.
  if(raw.acceptance_criteria!=null&&typeof raw.acceptance_criteria!=='string')return undefined;
  return raw as RecordItem;
}
