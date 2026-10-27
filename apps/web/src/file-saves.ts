import {ApiError,operation} from './api';
export type FileDocument={path:string;content:string;hash:string;language?:string};
export type FileSaveEnvelope={project_id:string;path:string;content:string;expected_hash:string;idempotency_key:string};
export class FileSaveRequest{
 private request?:FileSaveEnvelope;
 source?:FileDocument;
 unknown=false;
 reconciliation=false;
 reconciliationDetails?:{operation_id?:string;path?:string};
 get pending(){return this.request;}
 begin(project:string,file:FileDocument,content:string):FileSaveEnvelope{
  if(!this.request){this.source={...file};this.request={project_id:project,path:file.path,content,expected_hash:file.hash,idempotency_key:operation()};}
  return this.request;
 }
 completed(){this.request=undefined;this.source=undefined;this.unknown=false;this.reconciliation=false;this.reconciliationDetails=undefined;}
 failed(error:unknown){
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
export async function fileSaveReceipt(value:unknown,submitted:FileSaveEnvelope):Promise<FileDocument|undefined>{
 if(!value||typeof value!=='object'||Array.isArray(value))return;
 const document=value as Partial<FileDocument>;
 if(document.path!==submitted.path||document.content!==submitted.content||typeof document.hash!=='string'||!/^[a-f0-9]{64}$/.test(document.hash)||document.language!==undefined&&typeof document.language!=='string')return;
 try{
  const bytes=await crypto.subtle.digest('SHA-256',new TextEncoder().encode(submitted.content));
  const expected=Array.from(new Uint8Array(bytes),byte=>byte.toString(16).padStart(2,'0')).join('');
  return document.hash===expected?document as FileDocument:undefined;
 }catch{
  // A missing secure-context hashing capability is not evidence of a failed
  // server write. The caller keeps the pending operation available to recover.
  return;
 }
}
