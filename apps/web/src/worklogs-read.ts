import {useCallback,useEffect,useRef,useState} from 'react';
import {useApi} from './api-context';
import {ApiError} from './api';
import {UiError} from './i18n';

export const worklogAccessLost=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
/** Every result belongs to the exact API caller and requested path. Starting a
 * refresh hides the old private facts; cancellation and late reads cannot revive them. */
export function useWorklogRead<T>(path:string|undefined,validate:(value:unknown)=>value is T,refresh=0,onDenied?:()=>void){
 const api=useApi(),[state,setState]=useState<{api?:typeof api;path?:string;value?:T;loading:boolean;error?:unknown}>({loading:!!path}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),denied=useRef(onDenied);denied.current=onDenied;
 const read=useCallback(async()=>{
  if(!path)return;const attempt=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setState({api,path,loading:true});
  try{const value=await api<unknown>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||attempt!==epoch.current)return;if(!validate(value))throw new UiError('common.responseUnknown');setState({api,path,value,loading:false});return value;}
  catch(error){if(abort.signal.aborted||attempt!==epoch.current)return;setState({api,path,error,loading:false});if(worklogAccessLost(error))denied.current?.();}
 },[api,path,validate]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();};},[read,refresh]);
 const clear=useCallback(()=>{epoch.current++;controller.current?.abort();setState({api,path,loading:false});},[api,path]);
 return {...(state.api===api&&state.path===path?state:{loading:!!path}),read,clear};
}
