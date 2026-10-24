import {api,ApiError,operation,type Session} from './api';

// A single-use secret cannot be replayed to recover a lost acceptance response.
// Recover the account session with ordinary login instead.
export class InvitationAcceptanceRequest {
 private key=operation();
 private inFlight=false;
 unknown=false;
 async accept(values:{token:string;email:string;name:string;password:string}):Promise<Session>{
  if(this.unknown)throw new ApiError(0,'invitation_outcome_unknown','接受结果尚未确认，请使用普通登录恢复组织会话。');
  if(this.inFlight)throw new ApiError(409,'request_in_progress','邀请正在确认，请等待回执。');
  this.inFlight=true;
  try{return await api<Session>('/auth/invitations/accept',{...values,idempotency_key:this.key});}
  catch(error){this.unknown=!(error instanceof ApiError)||error.status===0||error.status>=500;if(!this.unknown)this.key=operation();throw error;}
  finally{this.inFlight=false;}
 }
}
