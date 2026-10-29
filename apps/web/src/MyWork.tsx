import {useCallback,useEffect,useRef,useState,type FormEvent} from 'react';
import {ArrowRight,BriefcaseBusiness,FolderOpen,RefreshCw,Search} from 'lucide-react';
import type {MyWorkPage} from '../../../packages/contracts/src';
import {ApiError,label,stateLabel,type ApiCaller} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {Badge,Empty,ErrorNotice,Loading} from './ui';
import {appendMyWorkPage,myWorkPath,myWorkScopeKey,myWorkStatus,myWorkStatuses,validMyWorkPage,validMyWorkQuery,type MyWorkFilter,type MyWorkScope} from './my-work-model';
import './MyWork.css';

export type MyWorkProps={project:string;userID:string;organizationID:string;q:string;status:string;refresh:number;onFilter:(filter:MyWorkFilter)=>void;onTask:(id:string)=>void;onWorkspace:(taskID:string)=>void;onResources:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('my-work.'+key,values);
const accessLost=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type ReadState={api:ApiCaller;page?:MyWorkPage;loading:boolean;error?:unknown;denied?:boolean};

/** Remount before rendering when account, organization, filters or refresh
 * changes. A previous scope is never a presentation cache for this queue. */
export function MyWork(props:MyWorkProps){
 const api=useApi(),scope:MyWorkScope={project:props.project,userID:props.userID,organizationID:props.organizationID,q:props.q,status:myWorkStatus(props.status)};
 return <MyWorkQueue key={JSON.stringify([myWorkScopeKey(scope),props.refresh])} {...props} api={api} scope={scope}/>;
}

function MyWorkQueue({scope,api,onFilter,onTask,onWorkspace,onResources}:MyWorkProps&{scope:MyWorkScope;api:ApiCaller}){
 useLocale();
 const [draft,setDraft]=useState(scope.q),[queryError,setQueryError]=useState(false),[state,setState]=useState<ReadState>({api,loading:true});
 const epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),pageRef=useRef<MyWorkPage|undefined>(undefined);
 const read=useCallback(async(more=false)=>{
  const previous=more?pageRef.current:undefined,cursor=previous?.next_cursor;
  if(more&&!cursor)return;
  const current=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  if(!more)pageRef.current=undefined;
  setState({api,page:previous,loading:true});
  try{
   if(!scope.project||!scope.userID||!scope.organizationID||!validMyWorkQuery(scope.q))throw new UiError('my-work.invalid_read');
   const value=await api<unknown>(myWorkPath(scope,cursor),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
   if(abort.signal.aborted||epoch.current!==current)return;
   if(!validMyWorkPage(value,scope)||cursor&&value.next_cursor===cursor)throw new UiError('my-work.invalid_read');
   const page=previous?appendMyWorkPage(previous,value):value;
   if(!page)throw new UiError('my-work.page_changed');
   pageRef.current=page;setState({api,page,loading:false});
  }catch(error){
   if(abort.signal.aborted||epoch.current!==current)return;
   // Even a transient failure clears rows, resource facts, counts and cursor.
   // Retry always starts at the beginning under current authorization.
   pageRef.current=undefined;setState({api,loading:false,error,denied:accessLost(error)});
  }
 },[api,scope.project,scope.userID,scope.organizationID,scope.q,scope.status]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();pageRef.current=undefined;};},[read]);
 const visible:ReadState=state.api===api?state:{api,loading:true},page=visible.page;
 const counts=new Map(page?.status_counts.map(row=>[row.status,row.total])??[]),all=page?.status_counts.reduce((sum,row)=>sum+row.total,0);
 function submit(event:FormEvent){event.preventDefault();if(!validMyWorkQuery(draft)){setQueryError(true);return;}setQueryError(false);onFilter({q:draft,status:scope.status});}
 function clear(){setDraft('');setQueryError(false);onFilter({q:'',status:''});}
 return <section className="my-work" aria-label={t('title')}>
  <header className="my-work-heading"><div><h2><BriefcaseBusiness size={20} aria-hidden="true"/>{t('title')}</h2><p>{t('intro')}</p></div><button type="button" disabled={visible.loading} onClick={()=>void read()} data-my-work-refresh><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button></header>
  <form className="my-work-search" role="search" aria-label={t('search_label')} onSubmit={submit}>
   <label htmlFor="my-work-q">{t('search_label')}</label><div><Search size={17} aria-hidden="true"/><input id="my-work-q" name="my-work-q" value={draft} placeholder={t('search_placeholder')} aria-invalid={queryError||undefined} aria-describedby={queryError?'my-work-query-error':undefined} onChange={event=>{setDraft(event.target.value);setQueryError(false);}}/><button type="submit">{t('search')}</button></div>
   {queryError&&<p id="my-work-query-error" role="alert">{t('query_length')}</p>}
  </form>
  <div className="my-work-statuses" role="group" aria-label={t('filter_label')}>
   {(['',...myWorkStatuses] as const).map(status=><button type="button" key={status} aria-pressed={scope.status===status} onClick={()=>onFilter({q:scope.q,status})} data-my-work-status={status||'all'}><span>{status?stateLabel(status):t('all')}</span><span className="my-work-count">{page?formatNumber(status?counts.get(status)??0:all??0):'—'}</span></button>)}
  </div>
  {Boolean(visible.error)&&<div><ErrorNotice error={visible.error} onRetry={()=>void read()}/>{visible.denied&&<p className="my-work-note" role="status">{t('access_cleared')}</p>}</div>}
  {visible.loading&&!page&&<Loading/>}
  {page&&<>
   <div className="my-work-scope"><p>{page.resource_status==='active'?t('owner',{name:page.resource_name||page.resource_id}):t('scope')}</p><small>{t('as_of',{at:formatDateTime(page.as_of,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})})}</small></div>
   {page.resource_status!=='active'?<Empty title={t(page.resource_status+'.title')} description={t(page.resource_status+'.hint')} action={<button type="button" onClick={onResources}>{t('resources')}<ArrowRight size={14} aria-hidden="true"/></button>}/>:<>
    <div className="my-work-list-heading"><h3>{scope.status?stateLabel(scope.status):t('tasks')}</h3><p aria-live="polite">{t('showing',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}</p></div>
    {!page.items.length?<Empty title={t(scope.q||scope.status?'filtered.title':'empty.title')} description={t(scope.q||scope.status?'filtered.hint':'empty.hint')} action={scope.q||scope.status?<button type="button" onClick={clear}>{t('clear_filters')}</button>:undefined}/>:<ul className="my-work-list" aria-label={t('tasks')} aria-busy={visible.loading}>{page.items.map(item=><TaskRow key={item.id} task={item} onTask={onTask} onWorkspace={onWorkspace}/>)}</ul>}
    {page.next_cursor&&<div className="my-work-more"><button type="button" disabled={visible.loading} onClick={()=>void read(true)} data-my-work-more>{visible.loading?t('loading_more'):t('more')}</button></div>}
    <p className="my-work-note">{t('live_scope')}</p>
   </>}
  </>}
 </section>;
}

function TaskRow({task,onTask,onWorkspace}:{task:MyWorkPage['items'][number];onTask:(id:string)=>void;onWorkspace:(id:string)=>void}){
 const name=label({id:task.id,version:task.version,title:task.title,name:task.name}),priority=typeof task.priority==='string'?task.priority:'',knownPriority=['urgent','high','medium','low'].includes(priority);
 return <li className="my-work-row" data-my-work-task={task.id}>
  <div className="my-work-task-main"><button type="button" className="my-work-task-title" onClick={()=>onTask(task.id)} aria-label={t('open_task',{name})}>{name}<ArrowRight size={15} aria-hidden="true"/></button><div className="my-work-task-facts"><span>{t('business_status')} <Badge state={task.status}/></span><span>{t('priority',{value:knownPriority?tr('common.priority.'+priority):priority||t('priority_unset')})}</span></div><p className="my-work-task-source">{task.requirement_id?t('requirement',{id:task.requirement_id}):t('no_requirement')}</p><small className="my-work-task-updated"><time dateTime={task.updated_at}>{t('updated',{at:formatDateTime(task.updated_at!,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})})}</time></small></div>
  <button type="button" className="my-work-workspace" onClick={()=>onWorkspace(task.id)} aria-label={t('open_workspace',{name})}><FolderOpen size={16} aria-hidden="true"/>{t('workspace')}</button>
 </li>;
}
