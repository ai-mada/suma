import {useCallback,useEffect,useRef,useState} from 'react';
import type {ApiCaller} from './api';
import {UiError} from './i18n';
export type IntelligencePage<T>={items:T[];total:number;next_cursor?:string};
/** Keeps same-path facts during refresh and aborts obsolete scope requests. */
export function useIntelligenceRead<T>(api:ApiCaller,path:string,enabled:boolean,refresh:number,valid:(value:T)=>boolean,failed:(error:unknown)=>void){
 const [state,setState]=useState<{path:string;value?:T;loading:boolean;error?:unknown}>({path,loading:enabled}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async()=>{if(!enabled)return;const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setState(previous=>({path,value:previous.path===path?previous.value:undefined,loading:true}));try{const value=await api<T>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!valid(value))throw new UiError('knowledge-intelligence.invalid_read');setState({path,value,loading:false});return value;}catch(error){if(abort.signal.aborted||generation!==epoch.current)return;setState({path,error,loading:false});failed(error);}},[api,path,enabled,valid,failed]);
 useEffect(()=>{if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 return {...(enabled&&state.path===path?state:{path,loading:enabled}),read};
}
export function useIntelligencePage<T extends{id:string}>(api:ApiCaller,path:string,enabled:boolean,refresh:number,valid:(value:IntelligencePage<T>)=>boolean,failed:(error:unknown)=>void,cursorName='cursor'){
 const [state,setState]=useState<{path:string;value?:IntelligencePage<T>;loading:boolean;error?:unknown}>({path,loading:enabled}),current=useRef<{path:string;value?:IntelligencePage<T>}>({path}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async(more=false)=>{if(!enabled)return;const previous=current.current.path===path?current.current.value:undefined,cursor=more?previous?.next_cursor:'';if(more&&!cursor)return;const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setState({path,value:previous,loading:true});try{const page=await api<IntelligencePage<T>>(path+(cursor?'&'+cursorName+'='+encodeURIComponent(cursor):''),undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||generation!==epoch.current)return;if(!valid(page))throw new UiError('knowledge-intelligence.invalid_read');const value={...page,items:more?[...new Map([...(previous?.items||[]),...page.items].map(item=>[item.id,item])).values()]:page.items};if(value.items.length>value.total)throw new UiError('knowledge-intelligence.invalid_read');current.current={path,value};setState({path,value,loading:false});return value;}catch(error){if(abort.signal.aborted||generation!==epoch.current)return;current.current={path};setState({path,error,loading:false});failed(error);}},[api,path,enabled,valid,failed,cursorName]);
 useEffect(()=>{current.current={path};if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 return {...(enabled&&state.path===path?state:{path,loading:enabled}),read};
}
