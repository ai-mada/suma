import {api,ApiError,operation} from './api';

export type ReleasePendingRequest={path:string;method:'POST'|'PATCH';body:Record<string,unknown>;idempotency_key:string};
type RecoveryStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
const writePath=/^\/(?:environments(?:\/[A-Za-z0-9._-]+)?|release-candidates|release-plans(?:\/[A-Za-z0-9._-]+\/(?:approve|execute))?|deployments\/[A-Za-z0-9._-]+\/reconcile)$/;
function validate(path:string,body:unknown){
 if(!writePath.test(path))throw new Error('此操作不属于发布工作台可恢复写入范围。');
 function inspect(value:unknown){if(!value||typeof value!=='object')return;for(const [key,item]of Object.entries(value)){if(/password|token|secret|credential|authorization/i.test(key))throw new Error('发布恢复记录不能包含秘密凭证。');inspect(item);}}
 inspect(body);
}
function copy<T>(value:T):T{return JSON.parse(JSON.stringify(value));}

// Only non-secret browser release requests are saved in this tab's session.
// Runner/provider credentials and one-time tickets are never accepted here.
export class ReleaseWriteRecovery{
 private value?:ReleasePendingRequest;
 private initialError='';
 private busy=false;
 private key:string;
 constructor(scope:string,private storage:RecoveryStorage|undefined){
  this.key='suma-release-recovery-v1:'+scope;
  try{const raw=storage?.getItem(this.key);if(raw){const parsed=JSON.parse(raw) as ReleasePendingRequest;validate(parsed.path,parsed.body);if(!parsed.idempotency_key||!['POST','PATCH'].includes(parsed.method)||!parsed.body||typeof parsed.body!=='object')throw new Error('invalid recovery');this.value=parsed;}}
  catch{this.initialError='发布恢复记录无法读取。请保留当前页面并由管理员核对，未发送新请求。';}
 }
 get pending(){return this.value?copy(this.value):undefined;}
 get error(){return this.initialError;}
 async restore<T=unknown>():Promise<T>{if(!this.value)throw new Error('没有需要恢复的发布操作。');return this.run<T>(this.value.path,this.value.body,this.value.method);}
 async run<T=unknown>(path:string,body:Record<string,unknown>,method:'POST'|'PATCH'='POST'):Promise<T>{
  if(this.initialError)throw new Error(this.initialError);
  if(this.busy)throw new Error('原发布操作仍在等待回执，请勿重复提交。');
  const restoring=!!this.value;
  if(!this.value){
   validate(path,body);const request:ReleasePendingRequest={path,method,body:copy(body),idempotency_key:operation()};
   try{if(!this.storage)throw new Error('storage unavailable');this.storage.setItem(this.key,JSON.stringify(request));}
   catch{throw new Error('无法保存本页的发布恢复记录，未发送请求。请启用会话存储后重试。');}
   this.value=request;
  }
  const request=this.value;this.busy=true;
  try{
   const result=await api<T>(request.path,{...request.body,idempotency_key:request.idempotency_key},request.method);
   this.clear();return result;
  }catch(error){
   // A later denial cannot prove that the original unknown request had no effect.
   if(!restoring&&error instanceof ApiError&&error.status>0&&error.status<500)this.clear();
   throw error;
  }finally{this.busy=false;}
 }
 private clear(){this.value=undefined;try{this.storage?.removeItem(this.key);}catch{/* Same-key replay after a tab refresh remains idempotent. */}}
}
