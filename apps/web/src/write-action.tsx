import {useEffect,useRef,useState,type ReactNode} from 'react';
import {ApiError} from './api';
import {mutate} from './operations';
import {ErrorNotice} from './ui';

export const uncertainWrite=(error:unknown)=>!(error instanceof ApiError)||error.status===0||error.status>=500;
export function useLeaveGuard(blocked:boolean,setError:(message:string)=>void){useEffect(()=>{const guard=(event:Event)=>{if(blocked){event.preventDefault();setError('请先保存或取消当前草稿；结果未知时请恢复同一操作。');}};const unload=(event:BeforeUnloadEvent)=>{if(blocked){event.preventDefault();event.returnValue='';}};window.addEventListener('suma:navigate',guard);window.addEventListener('beforeunload',unload);return()=>{window.removeEventListener('suma:navigate',guard);window.removeEventListener('beforeunload',unload);};},[blocked,setError]);}
type Request={path:string;body:Record<string,unknown>;method?:string};
export function useWriteAction(){const[busy,setBusy]=useState(false),[error,setError]=useState(''),[unknown,setUnknown]=useState(false),[conflict,setConflict]=useState(false);const pending=useRef<Request|undefined>(undefined);const identity=useRef(crypto.randomUUID());useLeaveGuard(busy||unknown,setError);
 async function run<T=any>(path:string,body:Record<string,unknown>,method?:string):Promise<T|undefined>{if(busy)return;const request=pending.current||{path,body,method};pending.current=request;setBusy(true);setError('');try{const result=await mutate<T>(request.path,request.body,request.method,identity.current);pending.current=undefined;identity.current=crypto.randomUUID();setUnknown(false);setConflict(false);return result;}catch(e){setUnknown(uncertainWrite(e));if(!uncertainWrite(e)){pending.current=undefined;identity.current=crypto.randomUUID();}setConflict(e instanceof ApiError&&e.status===409);setError(e instanceof ApiError&&e.status===403?'当前账号没有执行此操作的权限。':(e as Error).message);return undefined;}finally{setBusy(false);}}
 return {run,busy,error,setError,unknown,conflict,clearConflict:()=>{setConflict(false);setError('');}};
}
export type WriteAction=ReturnType<typeof useWriteAction>;
export function WriteNotice({action,onRebase}:{action:WriteAction;onRebase?:()=>Promise<void>}){return <>{action.error&&<ErrorNotice error={action.error}/>} {action.unknown&&<p className="warning" role="status">服务尚未确认结果。草稿与原操作已保留，请点击恢复回执。</p>}{action.conflict&&onRebase&&<div className="conflict-banner"><div><strong>服务端版本或状态已变化</strong><p>草稿已保留。读取最新记录并确认后，再次提交。</p><button type="button" disabled={action.busy} onClick={async()=>{try{await onRebase();action.clearConflict();}catch(e){action.setError((e as Error).message);}}}>读取最新版本，保留草稿</button></div></div>}</>;}
export function WriteFields({action,children}:{action:WriteAction;children:ReactNode}){return <fieldset className="form-fields" disabled={action.busy||action.unknown}>{children}</fieldset>;}
export function WriteButton({action,children,disabled=false}:{action:WriteAction;children:ReactNode;disabled?:boolean}){return <button className="primary" disabled={action.busy||disabled||action.conflict}>{action.busy?'提交中…':action.unknown?'恢复同一操作回执':children}</button>;}
