import {useCallback,useEffect,useRef,useState} from 'react';
import {ApiError,query,type ApiCaller} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';
import {validRequirementTraceability,type RequirementTraceability} from './requirement-traceability-model';

type Snapshot={caller:ApiCaller;scope:string;value?:RequirementTraceability;error?:unknown;loading:boolean;forbidden:boolean};
export function useRequirementTraceabilityRead(project:string,requirementId:string,requirementVersion:number,enabled:boolean,refresh:number){
 const api=useApi(),scope=JSON.stringify([project,requirementId,requirementVersion,refresh]),[snapshot,setSnapshot]=useState<Snapshot>();
 const alive=useRef(true),sequence=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const current=snapshot?.caller===api&&snapshot.scope===scope?snapshot:undefined;
 const forbidden=!!current?.forbidden;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;sequence.current++;controller.current?.abort();};},[]);
 const read=useCallback(async()=>{
  if(!enabled||forbidden)return;const ticket=++sequence.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setSnapshot({caller:api,scope,loading:true,forbidden:false});
  try{
   const value=await api<unknown>('/requirements/'+encodeURIComponent(requirementId)+'/traceability'+query(project),undefined,'GET',undefined,abort.signal);
   if(!alive.current||abort.signal.aborted||ticket!==sequence.current)return;
   if(!validRequirementTraceability(value,project,requirementId))throw new UiError('requirement-traceability.invalid_read');
   setSnapshot({caller:api,scope,value,loading:false,forbidden:false});
  }catch(error){
   if(!alive.current||abort.signal.aborted||ticket!==sequence.current)return;
   setSnapshot({caller:api,scope,error,loading:false,forbidden:error instanceof ApiError&&[401,403].includes(error.status)});
  }
 },[api,scope,project,requirementId,enabled,forbidden]);
 useEffect(()=>{if(enabled)void read();else{sequence.current++;controller.current?.abort();setSnapshot(undefined);}},[read,enabled]);
 return {value:enabled?current?.value:undefined,error:enabled?current?.error:undefined,loading:enabled&&(!current||current.loading),forbidden,read};
}
