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
  this.reconciliation=error instanceof ApiError&&error.code==='workspace_reconciliation_required';
  this.reconciliationDetails=this.reconciliation?(error as ApiError).details as {operation_id?:string;path?:string}:undefined;
  this.unknown=this.reconciliation||!(error instanceof ApiError)||error.status===0||error.status>=500;
  if(!this.unknown){this.request=undefined;this.source=undefined;}
 }
}
export function draftAfterFileSave(current:string,submitted:string,saved:string){return current===submitted?saved:current;}
