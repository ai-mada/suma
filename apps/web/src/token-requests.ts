import {api,ApiError,operation} from './api';
import {tr} from './i18n';
export type TokenScope='runner'|'agent'|'api';
export type ScopedToken={token:string;expires_at:string;label:string;scope:TokenScope;project_id:string};
export class TokenCreation{
 constructor(private send:typeof api=api){}
 private envelope?:{project_id:string;label:string;scope:TokenScope;idempotency_key:string};
 private running=false;
 unknown=false;
 get operationID(){return this.envelope?.idempotency_key;}
 async create(project:string,label:string,scope:TokenScope):Promise<ScopedToken>{
  if(this.unknown)throw new ApiError(0,'token_outcome_unknown',tr('workspace.token.outcome_unknown'));
  if(this.running)throw new ApiError(409,'token_request_pending',tr('workspace.token.request_pending'));
  if(!project)throw new ApiError(422,'project_required',tr('workspace.select_project'));
  this.envelope??={project_id:project,label,scope,idempotency_key:operation()};
  this.running=true;
  try{const result=await this.send<ScopedToken>('/auth/tokens',this.envelope);this.envelope=undefined;return result;}
  catch(e){if(e instanceof ApiError&&e.status>0&&e.status<500)this.envelope=undefined;else this.unknown=true;throw e;}
  finally{this.running=false;}
 }
}
