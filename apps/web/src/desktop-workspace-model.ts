import type {WorkspaceDesktopBindingRequest,WorkspaceDesktopGrant,WorkspaceMetadata} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {UiError} from './i18n';
import {mutate} from './operations';
import {uncertainWrite} from './write-action';

export type DesktopWorkspaceScope=Readonly<{userID:string;organizationID:string;project:string}>;
export type DesktopDirectoryRequest={project_id:string;user_id:string;tenant_id:string;locale:'zh-CN'|'en-US'};
export type DesktopDirectoryResult={status:'cancelled'}|{status:'selected';path:string;grant:WorkspaceDesktopGrant}|{status:'failed';code:string};
export type DesktopWorkspaceBridge=Readonly<{platform:string;version:string;chooseProjectDirectory:(request:DesktopDirectoryRequest)=>Promise<unknown>}>;
const hash=/^[a-f0-9]{64}$/,operation=/^desktop-workspace-[a-f0-9]{32}$/;
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const exact=(value:Record<string,unknown>,keys:string[])=>Object.keys(value).sort().join(',')===keys.sort().join(',');
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$/.test(value);
export const desktopScopeKey=(scope:DesktopWorkspaceScope)=>JSON.stringify([scope.userID,scope.organizationID,scope.project]);
export const validDesktopScope=(scope:DesktopWorkspaceScope)=>id(scope.userID)&&id(scope.organizationID)&&id(scope.project);
export function desktopWorkspaceBridge():DesktopWorkspaceBridge|undefined{
 const value=(window as unknown as {sumaDesktop?:unknown}).sumaDesktop;
 return record(value)&&typeof value.platform==='string'&&value.platform.length>0&&typeof value.version==='string'&&value.version.length>0&&typeof value.chooseProjectDirectory==='function'?value as DesktopWorkspaceBridge:undefined;
}
export function desktopWorkspacePath(path:unknown):path is string{
 if(typeof path!=='string'||!path||new TextEncoder().encode(path).length>4096||/[\x00-\x1f\x7f-\x9f]/.test(path)||path.trim()!==path)return false;
 if(path.startsWith('//')||path.startsWith('\\\\'))return false;
 if(/^[A-Za-z]:\\/.test(path))return path.length===3||!path.slice(3).includes(':')&&!path.includes('/')&&!path.slice(3).split('\\').some(part=>part==='.'||part==='..'||part===''||/[. ]$/.test(part))&&!path.endsWith('\\');
 return path.startsWith('/')&&!path.includes('\\')&&!path.split('/').slice(1).some(part=>part==='.'||part==='..'||part==='')||path==='/';
}
export function desktopWorkspaceName(path:string){return /^[A-Za-z]:\\$/.test(path)?'\\':path==='/'?'/':path.split(/[\\/]/).at(-1)!;}
export function selectedDesktopDirectory(value:unknown,scope:DesktopWorkspaceScope,now=Date.now()):{path:string;grant:WorkspaceDesktopGrant}|undefined{
 if(!record(value)||!exact(value,['status','path','grant'])||value.status!=='selected'||!desktopWorkspacePath(value.path)||!record(value.grant))return;
 const grant=value.grant;
 if(!exact(grant,['protocol','operation_id','user_id','tenant_id','project_id','path','expires_at','nonce','signature'])||grant.protocol!=='suma-desktop-workspace-v1'||typeof grant.operation_id!=='string'||!operation.test(grant.operation_id)||grant.user_id!==scope.userID||grant.tenant_id!==scope.organizationID||grant.project_id!==scope.project||grant.path!==value.path||typeof grant.nonce!=='string'||!/^[a-f0-9]{32}$/.test(grant.nonce)||typeof grant.signature!=='string'||!hash.test(grant.signature)||typeof grant.expires_at!=='string'||!/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,9})?Z$/.test(grant.expires_at)||!Number.isFinite(Date.parse(grant.expires_at))||Date.parse(grant.expires_at)<=now)return;
 return {path:value.path,grant:{protocol:'suma-desktop-workspace-v1',operation_id:grant.operation_id,user_id:scope.userID,tenant_id:scope.organizationID,project_id:scope.project,path:value.path,expires_at:grant.expires_at,nonce:grant.nonce,signature:grant.signature}};
}
export function validWorkspaceMetadata(value:unknown,project:string):value is WorkspaceMetadata{
 return record(value)&&value.project_id===project&&typeof value.configured==='boolean'&&typeof value.binding_available==='boolean'&&typeof value.desktop_binding_available==='boolean'&&typeof value.binding_sha256==='string'&&(value.binding_sha256===''||hash.test(value.binding_sha256))&&typeof value.binding_path_sha256==='string'&&(value.binding_path_sha256===''||hash.test(value.binding_path_sha256))&&typeof value.binding_operation_id==='string'&&(value.binding_operation_id===''||operation.test(value.binding_operation_id))&&(value.name===undefined||typeof value.name==='string')&&(value.reason===undefined||typeof value.reason==='string')&&(value.reason_message_key===undefined||typeof value.reason_message_key==='string');
}
export async function desktopBindingReceipt(value:unknown,request:WorkspaceDesktopBindingRequest):Promise<WorkspaceMetadata|undefined>{
 if(!validWorkspaceMetadata(value,request.project_id)||!value.configured||!hash.test(value.binding_sha256)||value.binding_operation_id!==request.grant.operation_id||value.name!==desktopWorkspaceName(request.grant.path))return;
 const bytes=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(request.grant.path));
 const expected=Array.from(new Uint8Array(bytes),byte=>byte.toString(16).padStart(2,'0')).join('');
 return value.binding_path_sha256===expected?value:undefined;
}

type Result={kind:'acknowledged';metadata:WorkspaceMetadata}|{kind:'failed';error:unknown}|{kind:'unknown'};
export type DesktopBindingSlot={readonly scope:DesktopWorkspaceScope;readonly api:ApiCaller;readonly request:WorkspaceDesktopBindingRequest;busy:boolean;uncertain:boolean;outcome?:Exclude<Result,{kind:'unknown'}>};
// Signed selections stay only in this bounded module memory. Do not expire or
// evict unconfirmed writes: replay needs the original API, bytes and operation.
const slots=new Map<string,DesktopBindingSlot>(),listeners=new Set<()=>void>();let revision=0,guarded=false;
const unload=(event:BeforeUnloadEvent)=>{event.preventDefault();event.returnValue='';};
const changed=()=>{
 if(typeof window!=='undefined'&&guarded!==(slots.size>0)){guarded=slots.size>0;if(guarded)window.addEventListener('beforeunload',unload);else window.removeEventListener('beforeunload',unload);}
 revision++;for(const listener of listeners)listener();
};
export const subscribeDesktopBindings=(listener:()=>void)=>{listeners.add(listener);return()=>{listeners.delete(listener);};};
export const desktopBindingsRevision=()=>revision;
export const desktopBindingSlot=(scope:DesktopWorkspaceScope)=>slots.get(desktopScopeKey(scope));
export function beginDesktopBinding(scope:DesktopWorkspaceScope,api:ApiCaller,grant:WorkspaceDesktopGrant,expected:string):DesktopBindingSlot{
 const key=desktopScopeKey(scope);
 if(!validDesktopScope(scope)||slots.has(key)||slots.size>=8||expected!==''&&!hash.test(expected)||!selectedDesktopDirectory({status:'selected',path:grant.path,grant},scope))throw new UiError('desktop-workspace.selection_unavailable');
 const request:WorkspaceDesktopBindingRequest={project_id:scope.project,expected_binding_sha256:expected,grant:structuredClone(grant),idempotency_key:grant.operation_id};
 Object.freeze(request.grant);Object.freeze(request);
 const slot:DesktopBindingSlot={scope:Object.freeze({...scope}),api,request,busy:false,uncertain:false};slots.set(key,slot);changed();return slot;
}
export function consumeDesktopBinding(slot:DesktopBindingSlot){const key=desktopScopeKey(slot.scope);if(!slot.busy&&slot.outcome&&slots.get(key)===slot){slots.delete(key);changed();}}
export async function runDesktopBinding(slot:DesktopBindingSlot):Promise<Result>{
 if(slot.busy)return {kind:'unknown'};
 if(slot.outcome)return slot.outcome;
 const restoring=slot.uncertain;slot.busy=true;changed();
 try{
  const value=await mutate<unknown>('/workspace/desktop-binding',slot.request as unknown as Record<string,unknown>,'POST',JSON.stringify(['desktop-workspace',desktopScopeKey(slot.scope),slot.request.idempotency_key]),slot.api);
  const receipt=await desktopBindingReceipt(value,slot.request);
  if(!receipt)throw new ApiError(0,'response_unknown','');
  slot.outcome={kind:'acknowledged',metadata:receipt};slot.uncertain=false;return slot.outcome;
 }catch(error){
  if(restoring||uncertainWrite(error)){slot.uncertain=true;return {kind:'unknown'};}
  slot.outcome={kind:'failed',error};return slot.outcome;
 }finally{slot.busy=false;changed();}
}
