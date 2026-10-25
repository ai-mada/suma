import {api,operation,ApiError,type ApiCaller} from './api';

type PendingWrite={
 key:string;body:Record<string,unknown>;send:ApiCaller;
 path:string;method?:string;uncertain:boolean;
};
// Keep the first target, payload and caller across route/session changes. A later
// refusal says nothing about whether a previously unacknowledged write committed.
const pending=new Map<string,PendingWrite>();
export async function mutate<T=any>(path:string,body:Record<string,unknown>,method?:string,identity?:string,send:ApiCaller=api):Promise<T>{
 const scope=identity||JSON.stringify([path,body,method]);
 const envelope=pending.get(scope)||{key:typeof body.idempotency_key==='string'&&body.idempotency_key?body.idempotency_key:operation(),body:structuredClone(body),send,path,method,uncertain:false};
 pending.set(scope,envelope);
 // A late completion must not delete a different operation that reused this key.
 const settle=()=>{if(pending.get(scope)===envelope)pending.delete(scope);};
 try{
  const result=await envelope.send<T>(envelope.path,{...envelope.body,idempotency_key:envelope.key},envelope.method);
  settle();return result;
 }catch(e){
  const definite=e instanceof ApiError&&e.status>0&&e.status<500;
  if(!definite)envelope.uncertain=true;
  else if(!envelope.uncertain)settle();
  throw e;
 }
}
