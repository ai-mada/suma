import {api,operation,ApiError} from './api';

// A lost response retains the same operation identity across route changes.
// The original envelope is replayed even if background reads see a new version.
const pending=new Map<string,{key:string;body:Record<string,unknown>}>();
export async function mutate<T=any>(path:string,body:Record<string,unknown>,method?:string,identity?:string):Promise<T>{
 const scope=identity||JSON.stringify([path,body,method]);
 const envelope=pending.get(scope)||{key:operation(),body};
 pending.set(scope,envelope);
 try{const result=await api<T>(path,{...envelope.body,idempotency_key:envelope.key},method);pending.delete(scope);return result;}
 catch(e){if(e instanceof ApiError&&e.status>0&&e.status<500)pending.delete(scope);throw e;}
}
