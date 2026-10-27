import type {WorkspaceObservation,WorkspaceObservationRead,WorkspaceObservationScope,WorkspaceWriteCommand,WorkspaceWriteCreateRequest} from '../../../packages/contracts/src';
import {remoteScopeKey,validRemoteScope,remotePath} from './remote-workspace-model';

export const remoteWriteLimit=48*1024;
export const validWriteSHA=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const integer=(value:unknown,min=0):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=min;

export async function remoteContentSHA(content:string):Promise<string>{
 const bytes=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(content));
 return [...new Uint8Array(bytes)].map(value=>value.toString(16).padStart(2,'0')).join('');
}

export function validRemoteText(content:string):boolean{
 if(content.includes('\0')||new TextEncoder().encode(content).length>remoteWriteLimit)return false;
 for(let i=0;i<content.length;i++){
  const unit=content.charCodeAt(i);
  if(unit>=0xd800&&unit<=0xdbff){const next=content.charCodeAt(++i);if(!(next>=0xdc00&&next<=0xdfff))return false;}
  else if(unit>=0xdc00&&unit<=0xdfff)return false;
 }
 return true;
}

/** Editing requires complete bytes, never a page or an assembled mixed revision. */
export async function editableRemoteSource(source:WorkspaceObservation):Promise<boolean>{
 const read=source.result?.read;
 return !!read&&source.state==='completed'&&validRemoteScope(source.scope)&&validWriteSHA(source.workspace_policy_sha256)&&read.start_line===1&&read.complete&&!read.next_line&&read.bytes===new TextEncoder().encode(read.content).length&&validRemoteText(read.content)&&await remoteContentSHA(read.content)===read.sha256;
}

export function makeRemoteWrite(project:string,scope:WorkspaceObservationScope,policy:string,file:WorkspaceObservationRead,content:string,sha:string,key:string):WorkspaceWriteCreateRequest{
 return {project_id:project,scope:{...scope},workspace_policy_sha256:policy,path:file.path,expected_sha256:file.sha256,content,content_sha256:sha,idempotency_key:key};
}

/** A receipt is useful only for the exact original request. The control API
 * returns metadata; it never echoes submitted source back into this editor. */
export function validRemoteWrite(value:WorkspaceWriteCommand,body:WorkspaceWriteCreateRequest,prior?:WorkspaceWriteCommand):boolean{
 if(!value||Object.hasOwn(value,'content')||typeof value.id!=='string'||!/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$/.test(value.id)||!integer(value.version,1)||value.project_id!==body.project_id||!validRemoteScope(value.scope)||remoteScopeKey(value.scope)!==remoteScopeKey(body.scope)||value.workspace_policy_sha256!==body.workspace_policy_sha256||value.path!==body.path||!remotePath(value.path)||value.expected_sha256!==body.expected_sha256||value.content_sha256!==body.content_sha256||!validWriteSHA(value.request_sha256)||!integer(value.execution_fence)||!['requested','leased','applied','not_applied','reconciling','expired'].includes(value.state))return false;
 if(prior&&(value.id!==prior.id||value.request_sha256!==prior.request_sha256||value.version<prior.version||prior.execution_fence>0&&value.execution_fence!==prior.execution_fence))return false;
 if(value.state==='applied'){
  const result=value.result;
  return value.execution_fence>0&&!!result&&!value.error_code&&result.native_operation_id==='remote-write:'+value.id&&result.path===body.path&&result.previous_sha256===body.expected_sha256&&result.sha256===body.content_sha256&&integer(result.generation,1);
 }
 return !value.result;
}

export const remoteWritePending=(value?:WorkspaceWriteCommand)=>!!value&&(value.state==='requested'||value.state==='leased'||value.inspection_state==='requested'||value.inspection_state==='leased');

/** Receipt history has no original source body. Its immutable metadata and
 * exact effect transition still have to validate before showing applied. */
export function validWriteMetadata(value:WorkspaceWriteCommand,project:string):boolean{
 if(!value||!validRemoteScope(value.scope)||!remotePath(value.path)||!validWriteSHA(value.content_sha256)||!validWriteSHA(value.expected_sha256)||!validWriteSHA(value.workspace_policy_sha256))return false;
 return validRemoteWrite(value,{project_id:project,scope:value.scope,workspace_policy_sha256:value.workspace_policy_sha256,path:value.path,expected_sha256:value.expected_sha256,content:'',content_sha256:value.content_sha256,idempotency_key:''});
}
