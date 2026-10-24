import {api,ApiError,operation} from './api';
export type TokenScope='runner'|'agent'|'api';
export type ScopedToken={token:string;expires_at:string;label:string;scope:TokenScope;project_id:string};
export class TokenCreation{
 private envelope?:{project_id:string;label:string;scope:TokenScope;idempotency_key:string};
 private running=false;
 unknown=false;
 get operationID(){return this.envelope?.idempotency_key;}
 async create(project:string,label:string,scope:TokenScope):Promise<ScopedToken>{
  if(this.unknown)throw new ApiError(0,'token_outcome_unknown','凭证创建结果尚未确认，已停止重复提交。请由管理员核实原操作。');
  if(this.running)throw new ApiError(409,'token_request_pending','凭证创建请求正在处理中。');
  this.envelope??={project_id:project,label,scope,idempotency_key:operation()};
  this.running=true;
  try{const result=await api<ScopedToken>('/auth/tokens',this.envelope);this.envelope=undefined;return result;}
  catch(e){if(e instanceof ApiError&&e.status>0&&e.status<500)this.envelope=undefined;else this.unknown=true;throw e;}
  finally{this.running=false;}
 }
}
