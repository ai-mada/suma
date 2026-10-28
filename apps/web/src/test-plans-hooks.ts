import {useCallback,useEffect,useRef,useState} from 'react';
import type {ApiCaller} from './api';
import {UiError} from './i18n';

/** Reads belong to one current caller and path. Losing permission clears
 * protected facts and waits for an explicit retry, without refreshing parents. */
export function useTestPlanRead<T>(api:ApiCaller,path:string|undefined,refresh:number,valid:(value:unknown)=>value is T,failed:(error:unknown)=>void){
 const [state,setState]=useState<{path?:string;value?:T;error?:unknown;loading:boolean}>({path,loading:!!path}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async()=>{
  if(!path)return;const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setState({path,loading:true});
  try{const value=await api<unknown>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!valid(value))throw new UiError('test-plans.invalid_read');setState({path,value,loading:false});return value;}
  catch(error){if(abort.signal.aborted||generation!==epoch.current)return;setState({path,error,loading:false});failed(error);}
 },[api,path,valid,failed]);
 useEffect(()=>{if(path)void read();else setState({path,loading:false});return()=>{epoch.current++;controller.current?.abort();};},[read,path,refresh]);
 return {...(state.path===path?state:{path,loading:!!path}),read};
}

export function useTestPlanPage<T>(api:ApiCaller,path:string|undefined,refresh:number,valid:(value:unknown)=>value is {items:T[];total:number;next_cursor:string},failed:(error:unknown)=>void,cursorName='before'){
 const [state,setState]=useState<{path?:string;value?:{items:T[];total:number;next_cursor:string};error?:unknown;loading:boolean}>({path,loading:!!path}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),current=useRef<typeof state>({path,loading:false});
 const read=useCallback(async(more=false)=>{
  if(!path)return;const previous=current.current.path===path?current.current.value:undefined,cursor=more?previous?.next_cursor:'';if(more&&!cursor)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setState({path,value:previous,loading:true});
  try{const page=await api<unknown>(path+(cursor?'&'+cursorName+'='+encodeURIComponent(cursor):''),undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!valid(page))throw new UiError('test-plans.invalid_read');const value={...page,items:more?[...(previous?.items||[]),...page.items]:page.items};if(value.items.length>value.total)throw new UiError('test-plans.invalid_read');current.current={path,value,loading:false};setState(current.current);return value;}
  catch(error){if(abort.signal.aborted||generation!==epoch.current)return;current.current={path,loading:false};setState({path,error,loading:false});failed(error);}
 },[api,path,valid,failed,cursorName]);
 useEffect(()=>{current.current={path,loading:false};if(path)void read();else setState({path,loading:false});return()=>{epoch.current++;controller.current?.abort();};},[read,path,refresh]);
 return {...(state.path===path?state:{path,loading:!!path}),read};
}
