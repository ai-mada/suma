import {tr,UiError} from './i18n';
import {api,ApiError,operation} from './api';

export type ReleasePendingRequest={path:string;method:'POST'|'PATCH';body:Record<string,unknown>;idempotency_key:string};
type RecoveryStorage=Pick<Storage,'getItem'|'setItem'|'removeItem'>;
const writePath=/^\/(?:environments(?:\/[A-Za-z0-9._-]+)?|release-candidates|release-plans(?:\/[A-Za-z0-9._-]+\/(?:approve|execute))?|deployments\/[A-Za-z0-9._-]+\/reconcile)$/;
function validate(path:string,body:unknown){
 if(!writePath.test(path))throw new UiError('release.recovery.path_invalid');
 function inspect(value:unknown){if(!value||typeof value!=='object')return;for(const [key,item]of Object.entries(value)){if(/password|token|secret|credential|authorization/i.test(key))throw new UiError('release.recovery.secret');inspect(item);}}
 inspect(body);
}
function copy<T>(value:T):T{return JSON.parse(JSON.stringify(value));}

// Only non-secret browser release requests are saved in this tab's session.
// Runner/provider credentials and one-time tickets are never accepted here.
export class ReleaseWriteRecovery{
 private value?:ReleasePendingRequest;
 private initialError=false;
 private busy=false;
 private key:string;
 constructor(scope:string,private storage:RecoveryStorage|undefined,private send:typeof api=api){
  this.key='suma-release-recovery-v1:'+scope;
  try{const raw=storage?.getItem(this.key);if(raw){const parsed=JSON.parse(raw) as ReleasePendingRequest;validate(parsed.path,parsed.body);if(!parsed.idempotency_key||!['POST','PATCH'].includes(parsed.method)||!parsed.body||typeof parsed.body!=='object')throw new Error('invalid recovery');this.value=parsed;}}
  catch{this.initialError=true;}
 }
 get pending(){return this.value?copy(this.value):undefined;}
 get error(){return this.initialError?tr('release.recovery.corrupt'):'';}
 async restore<T=unknown>():Promise<T>{if(!this.value)throw new UiError('release.recovery.none');return this.run<T>(this.value.path,this.value.body,this.value.method);}
 async run<T=unknown>(path:string,body:Record<string,unknown>,method:'POST'|'PATCH'='POST'):Promise<T>{
  if(this.initialError)throw new UiError('release.recovery.corrupt');
  if(this.busy)throw new UiError('release.recovery.pending');
  const restoring=!!this.value;
  if(!this.value){
   validate(path,body);const request:ReleasePendingRequest={path,method,body:copy(body),idempotency_key:operation()};
   try{if(!this.storage)throw new Error('storage unavailable');this.storage.setItem(this.key,JSON.stringify(request));}
   catch{throw new UiError('release.recovery.storage');}
   this.value=request;
  }
  const request=this.value;this.busy=true;
  try{
   const result=await this.send<T>(request.path,{...request.body,idempotency_key:request.idempotency_key},request.method);
   this.clear();return result;
  }catch(error){
   // A later denial cannot prove that the original unknown request had no effect.
   if(!restoring&&error instanceof ApiError&&error.status>0&&error.status<500)this.clear();
   throw error;
  }finally{this.busy=false;}
 }
 private clear(){this.value=undefined;try{this.storage?.removeItem(this.key);}catch{/* Same-key replay after a tab refresh remains idempotent. */}}
}
