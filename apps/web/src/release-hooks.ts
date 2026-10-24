import { tr, useLocale, renderDiagnostic, UiError } from "./i18n";
import {useCallback,useEffect,useRef,useState} from 'react';
import {api,ApiError} from './api';
import {useLeaveGuard} from './write-action';
import {ReleaseWriteRecovery} from './release-write';
import {releaseItemID,releaseListPath,type ReleaseListItem,type ReleaseListKind,type ReleasePage} from './release-model';

export function releaseError(error:unknown){return error instanceof ApiError?tr('development.errors.with_code',{message:renderDiagnostic(error),code:error.code}):error instanceof Error||typeof error==='string'?renderDiagnostic(error):tr('common.readIncomplete');}
export function useReleaseRead<T>(path:string,refresh=0){
 useLocale();
 const[state,setState]=useState<{path:string;data?:T;loading:boolean;error:string;failure?:unknown}>({path,loading:!!path,error:''}),epoch=useRef(0);
 const read=useCallback(async()=>{const generation=++epoch.current;if(!path){setState({path,loading:false,error:''});return;}setState(before=>({path,data:before.path===path?before.data:undefined,loading:true,error:''}));try{const data=await api<T>(path);if(generation===epoch.current)setState({path,data,loading:false,error:''});}catch(error){if(generation===epoch.current)setState(before=>({...before,loading:false,error:releaseError(error),failure:error}));}},[path]);
 useEffect(()=>{void read();return()=>{epoch.current++;};},[read,refresh]);
 return {...(state.path===path?state:{path,data:undefined,loading:!!path,error:''}),error:state.path===path?(state.failure?releaseError(state.failure):renderDiagnostic(state.error)):'',read};
}
export function useReleaseCollection(project:string,kind:ReleaseListKind,refresh:number){
 useLocale();
 const scope=project+'\0'+kind,epoch=useRef(0);
 const[state,setState]=useState<{scope:string;items?:ReleaseListItem[];total:number;next_cursor:string;allowed_actions?:string[];loading:boolean;error:string;failure?:unknown}>({scope,total:0,next_cursor:'',loading:true,error:''});
 const cursor=state.scope===scope?state.next_cursor:'';
 const read=async(more=false)=>{const generation=++epoch.current;setState(before=>({scope,items:before.scope===scope?before.items:undefined,total:before.scope===scope?before.total:0,next_cursor:more?cursor:'',loading:true,error:''}));try{const page=await api<ReleasePage<ReleaseListItem>>(releaseListPath(project,kind,more?cursor:''));if(generation!==epoch.current)return;if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total))throw new UiError('development.errors.release_page_invalid');setState(before=>({scope,items:more?[...new Map([...(before.items||[]),...page.items].map(item=>[releaseItemID(item),item])).values()]:page.items,total:page.total,next_cursor:page.next_cursor||'',allowed_actions:page.allowed_actions,loading:false,error:''}));}catch(error){if(generation===epoch.current)setState(before=>({...before,loading:false,error:releaseError(error),failure:error}));}};
 useEffect(()=>{void read();return()=>{epoch.current++;};},[project,kind,refresh]);
 return {...(state.scope===scope?state:{scope,items:undefined,total:0,next_cursor:'',loading:true,error:''}),error:state.scope===scope?(state.failure?releaseError(state.failure):renderDiagnostic(state.error)):'',read};
}
export function useReleaseAction(recovery:ReleaseWriteRecovery,onDefer:()=>void){
 useLocale();
 const[busy,setBusy]=useState(false),[error,setError]=useState<unknown>(''),[conflict,setConflict]=useState(false),[deferred,setDeferred]=useState(false),[,render]=useState(0);
 const pending=recovery.pending;useLeaveGuard(busy||!!pending&&!deferred,setError);
 async function perform<T>(send:()=>Promise<T>){if(busy)return;setBusy(true);setDeferred(false);setError('');setConflict(false);try{return await send();}catch(error){setError({source:error,originalRefusal:!!recovery.pending&&error instanceof ApiError&&error.status>0&&error.status<500});setConflict(error instanceof ApiError&&error.status===409&&!recovery.pending);return undefined;}finally{setBusy(false);render(n=>n+1);}}
 return {busy,error:renderDiagnostic(recovery.error)||(error&&typeof error==='object'&&'source'in error?releaseError(error.source)+('originalRefusal'in error&&error.originalRefusal?tr('common.originalRefusal'):''):error?releaseError(error):''),conflict,pending,deferred,setError,defer:()=>{if(!busy&&recovery.pending){setDeferred(true);onDefer();}},clearError:()=>{setError('');setConflict(false);},run:<T,>(path:string,body:Record<string,unknown>,method:'POST'|'PATCH'='POST')=>perform<T>(()=>recovery.run<T>(path,body,method)),restore:<T,>()=>perform<T>(()=>recovery.restore<T>())};
}
export type ReleaseAction=ReturnType<typeof useReleaseAction>;
