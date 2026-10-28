import {ApiError,operation} from './api';
import type {WorkspaceFileDocument,WorkspaceFileSaveRequest} from '../../../packages/contracts/src';
export type FileDocument=WorkspaceFileDocument;
export type FileSaveEnvelope=Readonly<WorkspaceFileSaveRequest>;
export class FileSaveRequest{
 private request?:FileSaveEnvelope;
 source?:FileDocument;
 unknown=false;
 reconciliation=false;
 reconciliationDetails?:{operation_id?:string;path?:string};
 get pending(){return this.request;}
 begin(project:string,file:FileDocument,content:string):FileSaveEnvelope{
  if(!this.request){this.source=Object.freeze({...file});this.request=Object.freeze({project_id:project,path:file.path,content,expected_hash:file.hash,expected_context_sha256:file.context_sha256,idempotency_key:operation()});}
  return this.request;
 }
 completed(){this.request=undefined;this.source=undefined;this.unknown=false;this.reconciliation=false;this.reconciliationDetails=undefined;}
 failed(error:unknown){
  // Only this authenticated, locked original-operation proof establishes that
  // an uncertain request was never accepted. A general 409/403 is not proof.
  const proof=error instanceof ApiError&&error.status===409&&error.code==='workspace_context_changed'?error.details:undefined;
  const request=this.request;
  if(!this.reconciliation&&request&&proof&&typeof proof==='object'&&!Array.isArray(proof)){
   const details=proof as Record<string,unknown>;
   if(details.not_accepted===true&&details.project_id===request.project_id&&details.path===request.path&&details.operation_id===request.idempotency_key&&details.expected_context_sha256===request.expected_context_sha256){this.completed();return;}
  }
  const reconciliation=error instanceof ApiError&&error.code==='workspace_reconciliation_required';
  if(reconciliation){this.reconciliation=true;this.reconciliationDetails=error.details as {operation_id?:string;path?:string};}
  // A later permission/hash refusal cannot prove whether an earlier uncertain
  // write committed. Keep its original envelope and caller until its own
  // validated success receipt arrives; never reinterpret it as a new save.
  this.unknown=this.unknown||this.reconciliation||!(error instanceof ApiError)||error.status===0||error.status>=500;
  if(!this.unknown){this.request=undefined;this.source=undefined;}
 }
}
export function draftAfterFileSave(current:string,submitted:string,saved:string){return current===submitted?saved:current;}

/**
 * The workspace API returns the exact UTF-8 bytes written, SHA-256 in lowercase
 * hex, and the request's slash-separated path (workspace.go:fileResponse).
 * Do not normalize newlines or accept a different file/content merely because
 * transport returned 200. This check precedes clearing the pending operation.
 */
export async function fileReadReceipt(value:unknown,project:string,path:string):Promise<FileDocument|undefined>{
 if(!value||typeof value!=='object'||Array.isArray(value))return;
 const document=value as Partial<FileDocument>;
 if(document.project_id!==project||document.path!==path||typeof document.content!=='string'||typeof document.hash!=='string'||!/^[a-f0-9]{64}$/.test(document.hash)||typeof document.context_sha256!=='string'||!/^[a-f0-9]{64}$/.test(document.context_sha256)||typeof document.workspace_name!=='string'||!document.workspace_name.trim()||typeof document.language!=='string')return;
 try{
  const bytes=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(document.content));
  const expected=Array.from(new Uint8Array(bytes),byte=>byte.toString(16).padStart(2,'0')).join('');
  return document.hash===expected?document as FileDocument:undefined;
 }catch{
  // A missing secure-context hashing capability is not evidence of a failed
  // server write. The caller keeps the pending operation available to recover.
  return;
 }
}
export async function fileSaveReceipt(value:unknown,submitted:FileSaveEnvelope):Promise<FileDocument|undefined>{
 const document=await fileReadReceipt(value,submitted.project_id,submitted.path);
 return document?.content===submitted.content&&document.context_sha256===submitted.expected_context_sha256?document:undefined;
}
