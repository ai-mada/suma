import {useCallback,useEffect,useMemo,useRef,useState} from 'react';
import type {TaskCollectionSummary} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {UiError} from './i18n';
import {appendTaskCollectionPage,TaskCollectionReadQueue,taskCollectionColumnScope,taskCollectionGroupBy,taskCollectionPath,validTaskCollectionFilter,validTaskCollectionPage,validTaskCollectionSummary,type TaskCollectionPageView,type TaskCollectionScope} from './task-collection-model';

export type TaskCollectionPageRead={page?:TaskCollectionPageView;loading:boolean;error?:unknown};
type CollectionState={summary?:TaskCollectionSummary;summaryLoading:boolean;summaryError?:unknown;groupsLoading:boolean;groupsError?:unknown;groupsResetRequired?:boolean;groupAsOf:Record<string,string>;pages:Record<string,TaskCollectionPageRead>;denied?:unknown};
const initial=():CollectionState=>({summaryLoading:true,groupsLoading:false,groupAsOf:{},pages:{}});
const denied=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);

/** Mounted for one exact route/account scope. Every column has its own read
 * generation, while authority loss invalidates all columns and pending jobs. */
export function useTaskCollection(api:ApiCaller,scope:TaskCollectionScope,onDenied:(error:unknown)=>void){
 const authority=useMemo(()=>({api,active:true,denied:false,epoch:0,summaryGeneration:0,controllers:new Set<AbortController>(),queue:new TaskCollectionReadQueue(),generations:new Map<string,number>()}),[api]);
 const current=useRef<{authority:typeof authority;state:CollectionState}>({authority,state:initial()});
 const [snapshot,setSnapshot]=useState(current.current),denyCallback=useRef(onDenied);denyCallback.current=onDenied;
 const install=useCallback((update:(state:CollectionState)=>CollectionState)=>{
  if(!authority.active||authority.denied)return;
  const previous=current.current.authority===authority?current.current.state:initial();
  const value={authority,state:update(previous)};current.current=value;setSnapshot(value);
 },[authority]);
 const refuse=useCallback((error:unknown)=>{
  if(!authority.active||authority.denied)return;
  authority.denied=true;
  for(const controller of authority.controllers)controller.abort();
  const value={authority,state:{...initial(),summaryLoading:false,denied:error}};current.current=value;setSnapshot(value);denyCallback.current(error);
 },[authority]);
 const request=useCallback(async(path:string)=>{
  const controller=new AbortController();authority.controllers.add(controller);
  try{return await authority.queue.run(()=>api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},controller.signal),controller.signal);}
  finally{authority.controllers.delete(controller);}
 },[api,authority,scope.userID,scope.organizationID]);
 const readPage=useCallback(async(status:string,more=false)=>{
  if(!authority.active||authority.denied)return;
  const previous=more?current.current.state.pages[status]?.page:undefined,cursor=previous?.next_cursor;
  if(more&&!cursor)return;
  const epoch=authority.epoch,generation=(authority.generations.get(status)??0)+1;authority.generations.set(status,generation);
  install(state=>({...state,pages:{...state.pages,[status]:{page:previous,loading:true}}}));
  try{
   const pageScope=scope.layout==='board'?taskCollectionColumnScope(scope,status):scope;
   const value=await request(taskCollectionPath(pageScope,'items','',cursor));
   if(!authority.active||authority.denied||authority.epoch!==epoch||authority.generations.get(status)!==generation)return;
   if(!validTaskCollectionPage(value,pageScope,pageScope.task_status||'')||cursor&&value.next_cursor===cursor)throw new UiError('task-collection.invalid_read');
   const page=previous?appendTaskCollectionPage(previous,value):value;
   if(!page)throw new UiError('task-collection.page_changed');
   install(state=>({...state,pages:{...state.pages,[status]:{page,loading:false}}}));
  }catch(error){
   if(denied(error)&&!authority.denied&&(!authority.active||authority.epoch!==epoch||authority.generations.get(status)!==generation)){denyCallback.current(error);return;}
   if(!authority.active||authority.denied||authority.epoch!==epoch||authority.generations.get(status)!==generation)return;
   if(denied(error)){refuse(error);return;}
   // A transient failure belongs to this column only. Its cursor/rows are
   // cleared; retry begins at page one while other columns remain usable.
   install(state=>({...state,pages:{...state.pages,[status]:{loading:false,error}}}));
  }
 },[authority,install,refuse,request,scope]);
 const readGroups=useCallback(async(more=false)=>{
  if(!authority.active||authority.denied)return;
  if(more&&(current.current.state.groupsLoading||current.current.state.groupsResetRequired))return;
  const previous=more?current.current.state.summary:undefined,cursor=previous?.next_group_cursor;
  if(more&&!cursor)return;
  const epoch=authority.epoch,generation=++authority.summaryGeneration;
  install(state=>({...state,summaryLoading:!more,summaryError:undefined,groupsLoading:more,groupsError:undefined}));
   let resetRequired=false;
   try{
    if(!scope.project||!scope.userID||!scope.organizationID||!validTaskCollectionFilter(scope))throw new UiError('task-collection.invalid_filters');
    const value=await request(taskCollectionPath(scope,'summary','','',cursor));
    if(!authority.active||authority.denied||authority.epoch!==epoch||authority.summaryGeneration!==generation)return;
    if(!validTaskCollectionSummary(value,scope.project,taskCollectionGroupBy(scope)))throw new UiError('task-collection.invalid_read');
    const summary=previous?{...value,groups:[...new Map([...previous.groups,...value.groups].map(group=>[group.key,group])).values()]}:value;
    const noProgress=!!previous&&summary.groups.length===previous.groups.length;
    if(summary.groups.length>value.groups_total||cursor&&value.next_group_cursor===cursor||noProgress&&(!!value.next_group_cursor||summary.groups.length<value.groups_total)||!value.next_group_cursor&&summary.groups.length<value.groups_total){resetRequired=true;throw new UiError('task-collection.page_changed');}
    install(state=>({...state,summary,summaryLoading:false,groupsLoading:false,groupsResetRequired:false,groupAsOf:{...state.groupAsOf,...Object.fromEntries(value.groups.map(group=>[group.key,value.as_of]))}}));
    if(scope.layout==='board'){
     for(const group of value.groups)if(group.total>0&&!Object.hasOwn(current.current.state.pages,group.key))void readPage(group.key);
    }else if(!more&&summary.total>0)void readPage('');
   }catch(error){if(denied(error)&&!authority.denied&&(!authority.active||authority.epoch!==epoch||authority.summaryGeneration!==generation)){denyCallback.current(error);return;}if(!authority.active||authority.denied||authority.epoch!==epoch||authority.summaryGeneration!==generation)return;if(denied(error)){refuse(error);return;}install(state=>({...state,summaryLoading:false,groupsLoading:false,...(more?{groupsError:error,groupsResetRequired:resetRequired}:{summaryError:error})}));}
 },[authority,install,readPage,refuse,request,scope]);
 useEffect(()=>{
  authority.active=true;authority.denied=false;authority.epoch++;install(()=>initial());void readGroups();
  return()=>{authority.active=false;authority.epoch++;for(const controller of authority.controllers)controller.abort();authority.controllers.clear();};
 },[authority,install,readGroups]);
 const visible=snapshot.authority===authority?snapshot.state:initial();
 return {...visible,readPage,readGroups};
}
