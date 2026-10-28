import {useCallback,useEffect,useRef,useState} from 'react';
import type {ApiCaller} from './api';
import {UiError} from './i18n';

export type OperationPage<T>={items:T[];total:number;next_cursor?:string};

/** Explicit, scoped reads. A denied read is never converted into a parent
 * refresh, retry timer or mutation. Obsolete requests cannot publish facts. */
export function useOperationRead<T>(api:ApiCaller,path:string,enabled:boolean,refresh:number,valid:(value:unknown)=>value is T,failed:(error:unknown)=>void){
 const [state,setState]=useState<{path:string;value?:T;loading:boolean;error?:unknown}>({path,loading:enabled}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async()=>{
  if(!enabled)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setState(before=>({path,value:before.path===path?before.value:undefined,loading:true}));
  try{const value=await api<T>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!valid(value))throw new UiError('intent-operations.invalid_read');setState({path,value,loading:false});return value;}
  catch(error){if(abort.signal.aborted||generation!==epoch.current)return;setState({path,error,loading:false});failed(error);}
 },[api,path,enabled,valid,failed]);
 useEffect(()=>{if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 const store=useCallback((target:string,value:T)=>setState({path:target,value,loading:false}),[]);
 return {...(enabled&&state.path===path?state:{path,loading:enabled}),read,store};
}

export function useOperationPage<T extends{id:string}>(api:ApiCaller,path:string,enabled:boolean,refresh:number,valid:(value:unknown)=>value is OperationPage<T>,failed:(error:unknown)=>void,cursorName='before'){
 const [state,setState]=useState<{path:string;value?:OperationPage<T>;loading:boolean;error?:unknown}>({path,loading:enabled}),current=useRef<{path:string;value?:OperationPage<T>}>({path}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async(more=false)=>{
  if(!enabled)return;const previous=current.current.path===path?current.current.value:undefined,cursor=more?previous?.next_cursor:'';if(more&&!cursor)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setState({path,value:previous,loading:true});
  try{const value=await api<OperationPage<T>>(path+(cursor?'&'+cursorName+'='+encodeURIComponent(cursor):''),undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!valid(value))throw new UiError('intent-operations.invalid_read');const page={...value,items:more?[...new Map([...(previous?.items||[]),...value.items].map(item=>[item.id,item])).values()]:value.items};if(page.items.length>page.total)throw new UiError('intent-operations.invalid_read');current.current={path,value:page};setState({path,value:page,loading:false});return page;}
  catch(error){if(abort.signal.aborted||generation!==epoch.current)return;current.current={path};setState({path,error,loading:false});failed(error);}
 },[api,path,enabled,valid,failed,cursorName]);
 useEffect(()=>{current.current={path};if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 return {...(enabled&&state.path===path?state:{path,loading:enabled}),read};
}
