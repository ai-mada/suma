import {useCallback,useEffect,useRef,useState} from 'react';
import type {ApiCaller} from './api';
import {UiError} from './i18n';

export type CatalogPage<T>={items:T[];total:number;next_cursor?:string};
/** Reads belong to the rendered account and path. Same-path refreshes retain
 * facts; a changed path or disabled surface never exposes the previous value. */
export function useCatalogRead<T>(api:ApiCaller,path:string,enabled:boolean,refresh:number,validate:(value:T)=>boolean,onError:(error:unknown)=>void){
 const [state,setState]=useState<{path:string;value?:T;loading:boolean;error?:unknown}>({path,loading:enabled}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async()=>{
  if(!enabled)return;const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setState(previous=>({path,value:previous.path===path?previous.value:undefined,loading:true}));
  try{const value=await api<T>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!validate(value))throw new UiError('skill-catalog.invalid_read');setState({path,value,loading:false});return value;}
  catch(error){if(abort.signal.aborted||generation!==epoch.current)return;setState({path,error,loading:false});onError(error);}
 },[api,path,enabled,validate,onError]);
 useEffect(()=>{if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 return {...(enabled&&state.path===path?state:{path,loading:enabled}),read};
}
export function useCatalogPage<T extends{id:string},P extends CatalogPage<T>>(api:ApiCaller,path:string,enabled:boolean,refresh:number,validate:(value:P)=>boolean,onError:(error:unknown)=>void,itemKey?:(item:T)=>string){
 const [state,setState]=useState<{path:string;value?:P;loading:boolean;error?:unknown}>({path,loading:enabled}),current=useRef<{path:string;value?:P}>({path}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async(more=false)=>{
  if(!enabled)return;const previous=current.current.path===path?current.current.value:undefined,cursor=more?previous?.next_cursor:'';if(more&&!cursor)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  setState({path,value:previous,loading:true});
  try{const page=await api<P>(path+(cursor?'&cursor='+encodeURIComponent(cursor):''),undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!validate(page))throw new UiError('skill-catalog.invalid_read');
   const value={...page,items:more?[...new Map([...(previous?.items||[]),...page.items].map(item=>[itemKey?itemKey(item):item.id,item])).values()]:page.items};if(value.items.length>value.total)throw new UiError('skill-catalog.invalid_read');current.current={path,value};setState({path,value,loading:false});return value;
  }catch(error){if(abort.signal.aborted||generation!==epoch.current)return;current.current={path};setState({path,error,loading:false});onError(error);}
 },[api,path,enabled,validate,onError,itemKey]);
 useEffect(()=>{current.current={path};if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 return {...(enabled&&state.path===path?state:{path,loading:enabled}),read};
}
