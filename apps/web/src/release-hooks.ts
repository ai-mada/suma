import {useCallback,useEffect,useRef,useState} from 'react';
import {api,ApiError} from './api';
import {useLeaveGuard} from './write-action';
import {ReleaseWriteRecovery} from './release-write';
import {releaseItemID,releaseListPath,type ReleaseListItem,type ReleaseListKind,type ReleasePage} from './release-model';

export function releaseError(error:unknown){return error instanceof ApiError?`${error.message}（${error.code}）`:error instanceof Error?error.message:'读取未完成，请重新读取。';}
export function useReleaseRead<T>(path:string,refresh=0){
 const[state,setState]=useState<{path:string;data?:T;loading:boolean;error:string}>({path,loading:!!path,error:''}),epoch=useRef(0);
 const read=useCallback(async()=>{const generation=++epoch.current;if(!path){setState({path,loading:false,error:''});return;}setState(before=>({path,data:before.path===path?before.data:undefined,loading:true,error:''}));try{const data=await api<T>(path);if(generation===epoch.current)setState({path,data,loading:false,error:''});}catch(error){if(generation===epoch.current)setState(before=>({...before,loading:false,error:releaseError(error)}));}},[path]);
 useEffect(()=>{void read();return()=>{epoch.current++;};},[read,refresh]);
 return {...(state.path===path?state:{path,data:undefined,loading:!!path,error:''}),read};
}
export function useReleaseCollection(project:string,kind:ReleaseListKind,refresh:number){
 const scope=project+'\0'+kind,epoch=useRef(0);
 const[state,setState]=useState<{scope:string;items?:ReleaseListItem[];total:number;next_cursor:string;allowed_actions?:string[];loading:boolean;error:string}>({scope,total:0,next_cursor:'',loading:true,error:''});
 const cursor=state.scope===scope?state.next_cursor:'';
 const read=async(more=false)=>{const generation=++epoch.current;setState(before=>({scope,items:before.scope===scope?before.items:undefined,total:before.scope===scope?before.total:0,next_cursor:more?cursor:'',loading:true,error:''}));try{const page=await api<ReleasePage<ReleaseListItem>>(releaseListPath(project,kind,more?cursor:''));if(generation!==epoch.current)return;if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total))throw new Error('发布列表响应缺少真实记录或总数。');setState(before=>({scope,items:more?[...new Map([...(before.items||[]),...page.items].map(item=>[releaseItemID(item),item])).values()]:page.items,total:page.total,next_cursor:page.next_cursor||'',allowed_actions:page.allowed_actions,loading:false,error:''}));}catch(error){if(generation===epoch.current)setState(before=>({...before,loading:false,error:releaseError(error)}));}};
 useEffect(()=>{void read();return()=>{epoch.current++;};},[project,kind,refresh]);
 return {...(state.scope===scope?state:{scope,items:undefined,total:0,next_cursor:'',loading:true,error:''}),read};
}
export function useReleaseAction(recovery:ReleaseWriteRecovery,onDefer:()=>void){
 const[busy,setBusy]=useState(false),[error,setError]=useState(''),[conflict,setConflict]=useState(false),[deferred,setDeferred]=useState(false),[,render]=useState(0);
 const pending=recovery.pending;useLeaveGuard(busy||!!pending&&!deferred,setError);
 async function perform<T>(send:()=>Promise<T>){if(busy)return;setBusy(true);setDeferred(false);setError('');setConflict(false);try{return await send();}catch(error){setError(releaseError(error)+(recovery.pending&&error instanceof ApiError&&error.status>0&&error.status<500?' 原提交结果仍需核对，当前拒绝不能证明先前没有副作用。':''));setConflict(error instanceof ApiError&&error.status===409&&!recovery.pending);return undefined;}finally{setBusy(false);render(n=>n+1);}}
 return {busy,error:recovery.error||error,conflict,pending,deferred,setError,defer:()=>{if(!busy&&recovery.pending){setDeferred(true);onDefer();}},clearError:()=>{setError('');setConflict(false);},run:<T,>(path:string,body:Record<string,unknown>,method:'POST'|'PATCH'='POST')=>perform<T>(()=>recovery.run<T>(path,body,method)),restore:<T,>()=>perform<T>(()=>recovery.restore<T>())};
}
export type ReleaseAction=ReturnType<typeof useReleaseAction>;
