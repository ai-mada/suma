import {useCallback,useEffect,useRef,useState} from 'react';
import {ApiError} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';

/** One authorized read belongs to one API identity and exact URL. Late replies
 * cannot cross a project, selected application, immutable version or account. */
export function useApplicationRead<T>(path:string|undefined,refresh:number,validate:(value:unknown)=>value is T,onDenied?:()=>void){
 const api=useApi(),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),denied=useRef(onDenied);denied.current=onDenied;
 const [state,setState]=useState<{path?:string;api?:typeof api;value?:T;loading:boolean;error?:unknown}>({loading:false});
 const read=useCallback(async()=>{
  if(!path)return;const revision=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setState({path,api,loading:true});
  try{const value=await api<unknown>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||revision!==epoch.current)return;
   if(!validate(value))throw new UiError('project-applications.invalid_read');setState({path,api,value,loading:false});return value;
  }catch(error){if(!abort.signal.aborted&&revision===epoch.current){setState({path,api,loading:false,error});if(error instanceof ApiError&&[401,403,404].includes(error.status))denied.current?.();}}
 },[path,api,validate]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();};},[read,refresh]);
 const clear=useCallback(()=>{epoch.current++;controller.current?.abort();setState({path,api,loading:false});},[path,api]);
 const install=useCallback((value:T)=>{if(validate(value))setState({path,api,value,loading:false});},[path,api,validate]);
 return {...(state.path===path&&state.api===api?state:{loading:!!path}),read,clear,install};
}
