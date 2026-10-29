import {useCallback,useEffect,useRef,useState} from 'react';
import {ApiError} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';
import {useApplicationRead} from './project-applications-hooks';

/** Reuse the application domain's account/project/URL isolation and abort
 * handling, while keeping malformed release-set responses in their own domain. */
export function useReleaseSetRead<T>(path:string|undefined,refresh:number,validate:(value:unknown)=>value is T,onDenied?:()=>void){
 const checked=useCallback((value:unknown):value is T=>{if(!validate(value))throw new UiError('application-release-sets.invalid_read');return true;},[validate]);
 return useApplicationRead(path,refresh,checked,onDenied);
}

/** Previews are pure reads. Any form edit aborts their old basis; only a result
 * for this mounted path and exact request becomes a registrable preview. */
export function useReleaseSetPreview<Request extends object,Response>(path:string,validate:(value:unknown,request:Request)=>value is Response,onDenied:()=>void,verify?:(value:Response,request:Request)=>boolean|Promise<boolean>){
 const api=useApi(),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),alive=useRef(true),[state,setState]=useState<{loading:boolean;error?:unknown;value?:Response;request?:Request}>({loading:false});
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;controller.current?.abort();};},[api,path]);
 const invalidate=()=>{epoch.current++;controller.current?.abort();setState({loading:false});};
 async function inspect(input:Request){const revision=++epoch.current,request=structuredClone(input),abort=new AbortController();controller.current?.abort();controller.current=abort;setState({loading:true});
  try{const value=await api<unknown>(path,request,'POST',undefined,abort.signal);if(!alive.current||abort.signal.aborted||revision!==epoch.current)return;if(!validate(value,request)||verify&&!await verify(value,request))throw new UiError('application-release-sets.invalid_read');if(!alive.current||abort.signal.aborted||revision!==epoch.current)return;setState({loading:false,value,request});}
  catch(error){if(alive.current&&!abort.signal.aborted&&revision===epoch.current){setState({loading:false,error});if(error instanceof ApiError&&[401,403,404].includes(error.status))onDenied();}}
 }
 return {...state,inspect,invalidate};
}
