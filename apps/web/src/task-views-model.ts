import {useCallback,useEffect,useRef,useState} from 'react';
import {ApiError,type ApiCaller,type RecordItem} from './api';
import {tr,UiError} from './i18n';
import type {Iteration} from './planning-model';
import {mergeTaskCollectionWindow,validTaskPlanProjection,type TaskPlanProjection} from './task-collection-model';
import type {TaskViewPreferences} from './task-views-storage';

/** A missing directory name never changes the authoritative iteration relation. */
export function taskIterationLabel(id:string,iterations:readonly Iteration[]){
 if(!id)return tr('task-collection.iteration_unset');
 const iteration=iterations.find(item=>item.id===id);
 return iteration?iteration.title+(iteration.status==='completed'?' · '+tr('task-collection.iteration_completed'):''):tr('task-collection.iteration_unknown',{id});
}
export const taskStatuses=['draft','todo','in_progress','blocked','in_review','accepted','cancelled'] as const;
export const taskPriorities=['urgent','high','medium','low'] as const;
export const taskSorts=['updated_desc','created_desc','title_asc','priority_desc'] as const;
export const defaultTaskView:TaskViewPreferences={q:'',status:'',owner_id:'',priority:'',sort:'updated_desc',layout:'list',group_by:'status'};
export const taskFilterText=(value:unknown)=>typeof value==='string'&&Array.from(value).length<=200&&!/[\p{Cc}]/u.test(value)&&(value===''||!!value.trim())?value:'';
export type TaskViewRoute={q:string;status:string;layout:string;task_invalid?:string;task_status?:string;task_owner?:string;task_priority?:string;task_sort?:string;task_group?:string;task_requirement?:string;task_iteration?:string;task_iteration_unset?:boolean;task_unassigned?:boolean;task_priority_unset?:boolean;task_status_unset?:boolean};
export function taskViewFromRoute(route:TaskViewRoute):TaskViewPreferences {
  return {q:route.q,status:taskFilterText(route.task_status),owner_id:route.task_owner||'',priority:taskFilterText(route.task_priority),sort:taskSorts.includes(route.task_sort as typeof taskSorts[number])?route.task_sort as TaskViewPreferences['sort']:'updated_desc',layout:route.layout==='board'?'board':'list',group_by:['status','owner','priority','iteration'].includes(route.task_group||'')?route.task_group as TaskViewPreferences['group_by']:'status',...(route.task_requirement?{requirement_id:route.task_requirement}:{}),...(route.task_iteration?{iteration_id:route.task_iteration}:{}),...(route.task_iteration_unset?{iteration_unset:true}:{}),...(route.task_unassigned?{unassigned:true}:{}),...(route.task_priority_unset?{priority_unset:true}:{}),...(route.task_status_unset?{status_unset:true}:{})};
}
export function taskViewToRoute(view:TaskViewPreferences):TaskViewRoute {
  return {task_invalid:undefined,q:view.q,status:'all',task_status:view.status||undefined,layout:view.layout,task_owner:view.owner_id||undefined,task_unassigned:view.unassigned||undefined,task_priority:view.priority||undefined,task_priority_unset:view.priority_unset||undefined,task_status_unset:view.status_unset||undefined,task_requirement:view.requirement_id||undefined,task_iteration:view.iteration_id||undefined,task_iteration_unset:view.iteration_unset||undefined,task_sort:view.sort==='updated_desc'?undefined:view.sort,task_group:view.group_by==='status'?undefined:view.group_by};
}
export function taskQueryKey(view:TaskViewPreferences):string {
  return JSON.stringify([view.q,view.status,view.owner_id,view.priority,view.sort,view.requirement_id||'',!!view.unassigned,!!view.priority_unset,!!view.status_unset,view.iteration_id||'',!!view.iteration_unset]);
}
export function taskWorkspacePath(project:string,view:TaskViewPreferences,cursor=''):string {
  const params=new URLSearchParams({project_id:project,sort:view.sort,limit:'100'});
  for(const [key,value] of Object.entries({q:view.q,status:view.status,owner_id:view.owner_id,priority:view.priority,requirement_id:view.requirement_id||'',iteration_id:view.iteration_id||'',cursor}))if(value)params.set(key,value);
  if(view.unassigned)params.set('unassigned','true');
  if(view.priority_unset)params.set('priority_unset','true');
  if(view.status_unset)params.set('status_unset','true');
  if(view.iteration_unset)params.set('iteration_unset','true');
  return '/task-workspace?'+params;
}
export type TaskWorkspacePage={items:RecordItem[];task_plans:TaskPlanProjection[];total:number;next_cursor?:string;as_of:string};
export type TaskWorkspaceRead={items:RecordItem[]|undefined;task_plans:TaskPlanProjection[];total:number;next_cursor:string;as_of:string;loading:boolean;error:unknown;action:'first'|'more'};
const emptyRead=():TaskWorkspaceRead=>({items:undefined,task_plans:[],total:0,next_cursor:'',as_of:'',loading:true,error:null,action:'first'});
export function taskReadDenied(error:unknown){return error instanceof ApiError&&[401,403,404].includes(error.status);}

/** One authorized, query-scoped window. Display changes never alter membership. */
export function useTaskWorkspacePage(api:ApiCaller,scope:readonly string[],view:TaskViewPreferences,refresh:number,invalid=false){
  const blocked=invalid||Array.from(view.q).length>200||(/[\p{Cc}]/u.test(view.q))||!!view.unassigned&&!!view.owner_id||!!view.priority_unset&&!!view.priority||!!view.status_unset&&!!view.status||!!view.iteration_unset&&!!view.iteration_id||!!view.iteration_id&&!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(view.iteration_id);
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
      const [q,status,owner_id,priority,sort,requirement_id,unassigned,priority_unset,status_unset,iteration_id,iteration_unset]=JSON.parse(queryKey) as [string,string,string,string,string,string,boolean,boolean,boolean,string,boolean];
      const preferences={...defaultTaskView,q,status,owner_id,priority,requirement_id,unassigned,priority_unset,status_unset,iteration_id,iteration_unset,sort:sort as TaskViewPreferences['sort']};
      const page=await api<TaskWorkspacePage>(taskWorkspacePath(project,preferences,cursor),undefined,'GET',{'X-Suma-Expected-User':scope[0],'X-Suma-Expected-Organization':scope[1]},controller.signal);
      if(sequence!==generation.current||active.current!==requestKey||controller.signal.aborted)return;
      if(!Array.isArray(page.items)||!Number.isSafeInteger(page.total)||page.total<0||typeof page.as_of!=='string'||(page.next_cursor!==undefined&&typeof page.next_cursor!=='string')||page.items.some(row=>!row||typeof row.id!=='string'||row.project_id!==project))throw new UiError('workspace.pagination.invalid_page');
      if(!validTaskPlanProjection(page.task_plans,page.items.map(row=>row.id),{task_iteration:iteration_id,task_iteration_unset:iteration_unset}))throw new UiError('workspace.pagination.invalid_page');
      const merged=append?mergeTaskCollectionWindow({items:before.items||[],task_plans:before.task_plans,total:before.total},page):{items:page.items,task_plans:page.task_plans};
      if(!merged){setState({key:requestKey,read:{...emptyRead(),loading:false,error:new UiError('task-views.window_changed'),action:'first'}});return;}
      const {items,task_plans}=merged;
      const next=page.next_cursor||'',noNewItems=append&&items.length===(before.items?.length||0);
      // A continuation is not a snapshot. If its current count or cursor proves
      // this accumulated window inconsistent, retain no stale task objects.
      if(items.length>page.total||append&&next===cursor||noNewItems&&(Boolean(next)||items.length<page.total)){
        setState({key:requestKey,read:{...emptyRead(),loading:false,error:new UiError('task-views.window_changed'),action:'first'}});
        return;
      }
      setState({key:requestKey,read:{items,task_plans,total:page.total,next_cursor:page.next_cursor||'',as_of:page.as_of,loading:false,error:null,action:append?'more':'first'}});
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
  useEffect(()=>{const scheduled=generation.current,timer=setTimeout(()=>{if(generation.current===scheduled)void perform('',false);},view.q?180:0);return()=>{clearTimeout(timer);generation.current++;request.current?.abort();};},[perform,refresh]);
  return {...read,reload:()=>perform('',false),more:()=>read.loading||!read.next_cursor?Promise.resolve():perform(read.next_cursor,true),invalidate:(error:unknown)=>{if(authority.current!==identity)return;generation.current++;request.current?.abort();setState({key:active.current,read:{...emptyRead(),loading:false,error}});}};
}
