import {useCallback,useEffect,useRef,useState} from 'react';
import type {WorkspaceTaskContext,WorkspaceTaskPage} from '../../../packages/contracts/src';
import {ApiError,query,type ApiCaller} from './api';
import {useApi} from './api-context';
import {UiError} from './i18n';
import {validWorkbenchTaskContext,validWorkbenchTaskPage} from './workbench-task-model';

type Snapshot<T>={api:ApiCaller;scope:string;value?:T;error?:unknown;loading:boolean};
/** No polling or graph-wide fanout. Every result is installed only for its
 * original view and exact task/search; old private facts are hidden on render. */
export function useWorkbenchTaskRead(project:string,taskID:string,refresh:number,disabled:boolean,onForbidden:()=>void){
 const api=useApi(),scope=JSON.stringify([project,taskID]),[snapshot,setSnapshot]=useState<Snapshot<WorkspaceTaskContext>>();
 const epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),owner=useRef({api,scope});owner.current={api,scope};
 const callback=useRef(onForbidden);callback.current=onForbidden;
 const read=useCallback(async()=>{
  if(!taskID||disabled)return;const generation=++epoch.current,controller=new AbortController();abort.current?.abort();abort.current=controller;
  const live=()=>!controller.signal.aborted&&generation===epoch.current&&owner.current.api===api&&owner.current.scope===scope;
  setSnapshot({api,scope,loading:true});
  try{const value=await api<unknown>('/workspace/task-context'+query(project)+'&task_id='+encodeURIComponent(taskID),undefined,'GET',undefined,controller.signal);if(!live())return;if(!validWorkbenchTaskContext(value,project,taskID))throw new UiError('workbench-task.invalid_context');setSnapshot({api,scope,value,loading:false});}
  catch(error){if(!live())return;setSnapshot({api,scope,error,loading:false});if(error instanceof ApiError&&[401,403].includes(error.status))callback.current();}
 },[api,scope,project,taskID,disabled]);
 useEffect(()=>{setSnapshot(undefined);if(taskID&&!disabled)void read();return()=>{epoch.current++;abort.current?.abort();};},[read,refresh]);
 return {...(snapshot?.api===api&&snapshot.scope===scope&&!disabled?snapshot:undefined),read};
}

export function useWorkbenchTaskPicker(project:string,enabled:boolean,disabled:boolean,onForbidden:()=>void){
 const api=useApi(),[search,setSearch]=useState(''),[snapshot,setSnapshot]=useState<Snapshot<WorkspaceTaskPage>>();
 const epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),owner=useRef({api,project});owner.current={api,project};
 const callback=useRef(onForbidden);callback.current=onForbidden;
 const read=useCallback(async(term:string,cursor='',previous?:WorkspaceTaskPage)=>{
  if(!enabled||disabled)return;const generation=++epoch.current,controller=new AbortController();abort.current?.abort();abort.current=controller;
  const live=()=>!controller.signal.aborted&&generation===epoch.current&&owner.current.api===api&&owner.current.project===project;
  setSnapshot({api,scope:project,value:previous,loading:true});
  try{const value=await api<unknown>('/workspace/task-context/tasks'+query(project)+'&search='+encodeURIComponent(term)+'&limit=20'+(cursor?'&cursor='+encodeURIComponent(cursor):''),undefined,'GET',undefined,controller.signal);if(!live())return;if(!validWorkbenchTaskPage(value,project,term)||cursor&&value.next_cursor===cursor)throw new UiError('workbench-task.invalid_page');
   const items=previous?[...new Map([...previous.items,...value.items].map(item=>[item.id,item])).values()]:value.items;
   setSearch(term);setSnapshot({api,scope:project,value:{...value,items},loading:false});
  }catch(error){if(!live())return;const forbidden=error instanceof ApiError&&[401,403].includes(error.status);setSnapshot({api,scope:project,value:forbidden?undefined:previous,error,loading:false});if(forbidden)callback.current();}
 },[api,project,enabled,disabled]);
 useEffect(()=>{if(disabled)setSnapshot(undefined);else if(enabled)void read(search);return()=>{epoch.current++;abort.current?.abort();};},[api,project,enabled,disabled]);
 const visible=snapshot?.api===api&&snapshot.scope===project&&!disabled?snapshot:undefined;
 return {...visible,search,submit:(term:string)=>read(term),more:()=>visible?.value?.next_cursor?read(search,visible.value.next_cursor,visible.value):Promise.resolve()};
}
