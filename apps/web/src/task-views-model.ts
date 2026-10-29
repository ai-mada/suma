import {useCallback,useEffect,useRef,useState} from 'react';
import {ApiError,type ApiCaller,type RecordItem} from './api';
import {UiError} from './i18n';
import type {TaskViewPreferences} from './task-views-storage';

export const taskStatuses=['draft','todo','in_progress','blocked','in_review','accepted','cancelled'] as const;
export const taskPriorities=['urgent','high','medium','low'] as const;
export const taskSorts=['updated_desc','created_desc','title_asc','priority_desc'] as const;
export const defaultTaskView:TaskViewPreferences={q:'',status:'',owner_id:'',priority:'',sort:'updated_desc',layout:'list',group_by:'status'};
export const taskFilterText=(value:unknown)=>typeof value==='string'&&new TextEncoder().encode(value).length<=64&&!/[\p{Cc}]/u.test(value)?value:'';
export type TaskViewRoute={q:string;status:string;layout:string;task_invalid?:string;task_status?:string;task_owner?:string;task_priority?:string;task_sort?:string;task_group?:string};
export function taskViewFromRoute(route:TaskViewRoute):TaskViewPreferences {
  return {q:route.q,status:route.task_status!==undefined?taskFilterText(route.task_status):route.status==='all'?'':taskFilterText(route.status),owner_id:route.task_owner||'',priority:taskFilterText(route.task_priority),sort:taskSorts.includes(route.task_sort as typeof taskSorts[number])?route.task_sort as TaskViewPreferences['sort']:'updated_desc',layout:route.layout==='board'?'board':'list',group_by:['status','owner','priority'].includes(route.task_group||'')?route.task_group as TaskViewPreferences['group_by']:'status'};
}
export function taskViewToRoute(view:TaskViewPreferences):TaskViewRoute {
  return {task_invalid:undefined,q:view.q,status:'all',task_status:view.status||undefined,layout:view.layout,task_owner:view.owner_id||undefined,task_priority:view.priority||undefined,task_sort:view.sort==='updated_desc'?undefined:view.sort,task_group:view.group_by==='status'?undefined:view.group_by};
}
export function taskQueryKey(view:TaskViewPreferences):string {
  return JSON.stringify([view.q.trim(),view.status,view.owner_id,view.priority,view.sort]);
}
export function taskWorkspacePath(project:string,view:TaskViewPreferences,cursor=''):string {
  const params=new URLSearchParams({project_id:project,sort:view.sort,limit:'100'});
  for(const [key,value] of Object.entries({q:view.q.trim(),status:view.status,owner_id:view.owner_id,priority:view.priority,cursor}))if(value)params.set(key,value);
  return '/task-workspace?'+params;
}
export type TaskWorkspacePage={items:RecordItem[];total:number;next_cursor?:string;as_of:string};
export type TaskWorkspaceRead={items:RecordItem[]|undefined;total:number;next_cursor:string;as_of:string;loading:boolean;error:unknown;action:'first'|'more'};
const emptyRead=():TaskWorkspaceRead=>({items:undefined,total:0,next_cursor:'',as_of:'',loading:true,error:null,action:'first'});
export function taskReadDenied(error:unknown){return error instanceof ApiError&&(error.status===401||error.status===403);}

/** One authorized, query-scoped window. Display changes never alter membership. */
export function useTaskWorkspacePage(api:ApiCaller,scope:readonly string[],view:TaskViewPreferences,refresh:number,invalid=false){
  const blocked=invalid||Array.from(view.q.trim()).length>200||/[\u0000-\u001f\u007f-\u009f]/.test(view.q);
  const identity=JSON.stringify(scope),queryKey=taskQueryKey(view),key=identity+queryKey+String(blocked),project=scope[2];
  const generation=useRef(0),active=useRef(key),authority=useRef(identity);active.current=key;authority.current=identity;
  const request=useRef<AbortController|undefined>(undefined);
  const [state,setState]=useState<{key:string;read:TaskWorkspaceRead}>(()=>({key,read:emptyRead()}));
  const current=useRef(state);current.current=state;
  const read=blocked?{...emptyRead(),loading:false,error:new UiError('task-views.invalid_filters')}:state.key===key?state.read:emptyRead();
  const perform=useCallback(async(cursor:string,append:boolean)=>{
    if(blocked)return;
    const requestKey=key,sequence=++generation.current;
    request.current?.abort();const controller=new AbortController();request.current=controller;
    const before=current.current.key===requestKey?current.current.read:emptyRead();
    setState({key:requestKey,read:{...before,loading:true,error:null,action:append?'more':'first'}});
    try {
      const [q,status,owner_id,priority,sort]=JSON.parse(queryKey) as string[];
      const preferences={...defaultTaskView,q,status,owner_id,priority,sort:sort as TaskViewPreferences['sort']};
      const page=await api<TaskWorkspacePage>(taskWorkspacePath(project,preferences,cursor),undefined,'GET',undefined,controller.signal);
      if(sequence!==generation.current||active.current!==requestKey||controller.signal.aborted)return;
      if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total)||page.total<0||typeof page.as_of!=='string'||(page.next_cursor!==undefined&&typeof page.next_cursor!=='string')||page.items.some(row=>!row||typeof row.id!=='string'||row.project_id!==project))throw new UiError('workspace.pagination.invalid_page');
      const items=[...new Map([...(append?before.items||[]:[]),...page.items].map(row=>[row.id,row])).values()];
      const next=page.next_cursor||'',noNewItems=append&&items.length===(before.items?.length||0);
      // A continuation is not a snapshot. If its current count or cursor proves
      // this accumulated window inconsistent, retain no stale task objects.
      if(items.length>page.total||append&&next===cursor||noNewItems&&(Boolean(next)||items.length<page.total)){
        setState({key:requestKey,read:{...emptyRead(),loading:false,error:new UiError('task-views.window_changed'),action:'first'}});
        return;
      }
      setState({key:requestKey,read:{items,total:page.total,next_cursor:page.next_cursor||'',as_of:page.as_of,loading:false,error:null,action:append?'more':'first'}});
    }catch(error){
      // Denial belongs to the account/organization/project, not the filter that
      // happened to observe it. A cancelled older query can still prove it.
      if(taskReadDenied(error)&&authority.current===identity){
        generation.current++;request.current?.abort();
        setState({key:active.current,read:{...emptyRead(),loading:false,error}});
        return;
      }
      if(sequence!==generation.current||active.current!==requestKey||controller.signal.aborted)return;
      setState({key:requestKey,read:{...(taskReadDenied(error)?emptyRead():before),loading:false,error,action:append?'more':'first'}});
    }
  },[api,key,project,queryKey,blocked,identity]);
  useEffect(()=>{const scheduled=generation.current,timer=setTimeout(()=>{if(generation.current===scheduled)void perform('',false);},view.q.trim()?180:0);return()=>{clearTimeout(timer);generation.current++;request.current?.abort();};},[perform,refresh]);
  return {...read,reload:()=>perform('',false),more:()=>read.loading||!read.next_cursor?Promise.resolve():perform(read.next_cursor,true),invalidate:(error:unknown)=>{if(authority.current!==identity)return;generation.current++;request.current?.abort();setState({key:active.current,read:{...emptyRead(),loading:false,error}});}};
}
