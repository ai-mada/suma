import {useEffect,useRef,useState,type ReactNode} from 'react';
import {tr,useLocale} from './i18n';
import {ApiError} from './api';
import {mutate} from './operations';
import {ErrorNotice} from './ui';

export const uncertainWrite=(error:unknown)=>!(error instanceof ApiError)||error.status===0||error.status>=500;
export function useLeaveGuard(blocked:boolean,setError:(message:string)=>void){useEffect(()=>{const guard=(event:Event)=>{if(blocked){event.preventDefault();setError(tr('preferences.write.guard'));}};const unload=(event:BeforeUnloadEvent)=>{if(blocked){event.preventDefault();event.returnValue='';}};window.addEventListener('suma:navigate',guard);window.addEventListener('beforeunload',unload);return()=>{window.removeEventListener('suma:navigate',guard);window.removeEventListener('beforeunload',unload);};},[blocked,setError]);}
type Request={path:string;body:Record<string,unknown>;method?:string};
export function useWriteAction(){const[busy,setBusy]=useState(false),[error,setError]=useState(''),[unknown,setUnknown]=useState(false),[conflict,setConflict]=useState(false);const pending=useRef<Request|undefined>(undefined);const identity=useRef(crypto.randomUUID());useLeaveGuard(busy||unknown,setError);
 async function run<T=any>(path:string,body:Record<string,unknown>,method?:string):Promise<T|undefined>{if(busy)return;const request=pending.current||{path,body,method};pending.current=request;setBusy(true);setError('');try{const result=await mutate<T>(request.path,request.body,request.method,identity.current);pending.current=undefined;identity.current=crypto.randomUUID();setUnknown(false);setConflict(false);return result;}catch(e){setUnknown(uncertainWrite(e));if(!uncertainWrite(e)){pending.current=undefined;identity.current=crypto.randomUUID();}setConflict(e instanceof ApiError&&e.status===409);setError(e instanceof ApiError&&e.status===403?tr('preferences.write.forbidden'):(e as Error).message);return undefined;}finally{setBusy(false);}}
 return {run,busy,error,setError,unknown,conflict,clearConflict:()=>{setConflict(false);setError('');}};
}
export type WriteAction=ReturnType<typeof useWriteAction>;
export function WriteNotice({action,onRebase}:{action:WriteAction;onRebase?:()=>Promise<void>}){useLocale();return <>{action.error&&<ErrorNotice error={action.error}/>} {action.unknown&&<p className="warning" role="status">{tr('preferences.write.unknown')}</p>}{action.conflict&&onRebase&&<div className="conflict-banner"><div><strong>{tr('preferences.write.conflict')}</strong><p>{tr('preferences.write.preserved')}</p><button type="button" disabled={action.busy} onClick={async()=>{try{await onRebase();action.clearConflict();}catch(e){action.setError((e as Error).message);}}}>{tr('preferences.write.rebase')}</button></div></div>}</>;}
export function WriteFields({action,children}:{action:WriteAction;children:ReactNode}){return <fieldset className="form-fields" disabled={action.busy||action.unknown}>{children}</fieldset>;}
export function WriteButton({action,children,disabled=false}:{action:WriteAction;children:ReactNode;disabled?:boolean}){useLocale();return <button className="primary" disabled={action.busy||disabled||action.conflict}>{action.busy?tr('preferences.write.submitting'):action.unknown?tr('preferences.write.restore'):children}</button>;}
