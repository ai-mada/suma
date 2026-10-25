import {api,operation,ApiError,type ApiCaller} from './api';

// A lost response retains the same operation identity across route changes.
// The original envelope is replayed even if background reads see a new version.
const pending=new Map<string,{key:string;body:Record<string,unknown>;send:ApiCaller}>();
export async function mutate<T=any>(path:string,body:Record<string,unknown>,method?:string,identity?:string,send:ApiCaller=api):Promise<T>{
 const scope=identity||JSON.stringify([path,body,method]);
 const envelope=pending.get(scope)||{key:typeof body.idempotency_key==='string'&&body.idempotency_key?body.idempotency_key:operation(),body:structuredClone(body),send};
 pending.set(scope,envelope);
 try{const result=await envelope.send<T>(path,{...envelope.body,idempotency_key:envelope.key},method);pending.delete(scope);return result;}
 catch(e){if(e instanceof ApiError&&e.status>0&&e.status<500)pending.delete(scope);throw e;}
}
