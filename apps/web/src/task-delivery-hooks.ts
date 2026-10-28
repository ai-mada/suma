import {useCallback,useEffect,useRef,useState} from 'react';
import {ApiError,query} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';
import {validTaskDelivery,type TaskDelivery} from './task-delivery-model';

/** One explicitly opened task, one bounded projection; no graph-wide polling,
 * per-node logs or side effects. A new scope's first render has no old facts. */
export function useTaskDeliveryRead(project:string,taskID:string,enabled:boolean,refresh:number){
 const api=useApi(),[value,setValue]=useState<TaskDelivery>(),[error,setError]=useState<unknown>(),[loading,setLoading]=useState(false),[forbidden,setForbidden]=useState(false);
 const alive=useRef(true),sequence=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;sequence.current++;controller.current?.abort();};},[]);
 const read=useCallback(async()=>{
  if(!enabled||forbidden)return;const current=++sequence.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setValue(undefined);setError(undefined);setLoading(true);
  try{const result=await api<unknown>('/tasks/'+encodeURIComponent(taskID)+'/delivery'+query(project),undefined,'GET',undefined,abort.signal);if(!alive.current||abort.signal.aborted||current!==sequence.current)return;if(!validTaskDelivery(result,project,taskID))throw new UiError('task-delivery.invalid_read');setValue(result);return result;}
  catch(error){if(!alive.current||abort.signal.aborted||current!==sequence.current)return;setValue(undefined);setError(error);if(error instanceof ApiError&&[401,403].includes(error.status))setForbidden(true);}
  finally{if(alive.current&&current===sequence.current)setLoading(false);}
 },[api,project,taskID,enabled,forbidden]);
 useEffect(()=>{if(enabled)void read();else{sequence.current++;controller.current?.abort();setValue(undefined);setError(undefined);setLoading(false);}},[read,enabled,refresh]);
 return {value,error,loading,forbidden,read};
}
