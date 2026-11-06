import {useCallback,useEffect,useRef,useState,type MouseEvent} from 'react';
import {ArrowRight,Cloud,ListFilter,RefreshCw,X} from 'lucide-react';
import type {CloudNodeProgress,CloudProgressTaskLink} from '../../../packages/contracts/src';
import {ApiError,query,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {wellFormed} from './i18n/catalog';
import {executionHref} from './scheduling-model';
import {followTaskDeliveryHref,taskDeliveryHref,workBuddyProgressTaskHref} from './task-delivery-navigation-model';
import {Badge,Empty,ErrorNotice,Loading} from './ui';

type Props={
 project:string;
 session:Session;
 refresh:number;
 filter?:'attention';
 invalidFilter?:boolean;
 onFilter?:(filter?:'attention')=>void;
 task?:string;
 invalidTask?:boolean;
 onTask?:(task?:string)=>void;
 onRun:(run:string,node:string,attempt?:number)=>void;
};
const pageSize=10;
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-progress.'+key,values);
const reasons:Record<string,string>={
 'cloud.authorization_changed':'runner_status_authorization_changed',
 'cloud.creation_unconfirmed':'status_unknown',
 'cloud.delivery_required':'runner_status_delivery_required',
 'cloud.interrupted':'runner_status_interrupted',
 'cloud.not_created':'runner_status_not_created',
 'cloud.observation_unconfirmed':'runner_status_observation_unconfirmed',
 'cloud.remote_unknown':'runner_status_remote_unknown',
 'cloud.retry_requires_reconciliation':'status_unknown',
 'cloud.runner_disconnected':'runner_status_runner_disconnected',
 'cloud.stop_unconfirmed':'runner_status_stop_unconfirmed',
 'cloud.waiting_input':'runner_status_waiting_input',
};
const providerStatuses=new Set(['pending','queued','running','working','waiting_input','completed','failed','cancelled','canceled','unknown']);
const required=['project_id','run_id','node_id','run_version','run_state','node_state','attempt','submission_state','observation_failed'];
const optional=['node_reason','provider_status','observed_at'];
const record=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const identifier=(value:unknown,max:number):value is string=>typeof value==='string'&&value.length>0&&value.trim()===value&&
 new TextEncoder().encode(value).length<=max&&!/[\/\\\u0000-\u001f\u007f-\u009f]/.test(value)&&value!=='.'&&value!=='..';
const integer=(value:unknown,min:number):value is number=>typeof value==='number'&&Number.isInteger(value)&&value>=min&&value<=2147483647;
const state=(value:unknown):value is string=>typeof value==='string'&&/^[a-z][a-z0-9_]{0,79}$/.test(value);
const timestamp=(value:unknown):value is string=>typeof value==='string'&&value.length<=40&&
 /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value)&&Number.isFinite(Date.parse(value));
const key=(item:Pick<CloudNodeProgress,'run_id'|'node_id'>)=>JSON.stringify([item.run_id,item.node_id]);

function readItem(value:unknown,project:string):value is CloudNodeProgress{
 if(!record(value)||!required.every(field=>Object.hasOwn(value,field))||
    !Object.keys(value).every(field=>required.includes(field)||optional.includes(field))||
    value.project_id!==project||!identifier(value.run_id,100)||!identifier(value.node_id,150)||
    !integer(value.run_version,1)||!state(value.run_state)||!state(value.node_state)||!integer(value.attempt,0)||
    typeof value.observation_failed!=='boolean'||
    value.node_reason!==undefined&&(typeof value.node_reason!=='string'||!Object.hasOwn(reasons,value.node_reason)))return false;
 if(value.submission_state==='task_known')return typeof value.provider_status==='string'&&providerStatuses.has(value.provider_status)&&timestamp(value.observed_at);
 return (value.submission_state==='prepared'||value.submission_state==='unknown')&&
  value.provider_status===undefined&&value.observed_at===undefined&&!value.observation_failed;
}

const needsAttention=(item:CloudNodeProgress)=>!!item.node_reason||item.submission_state==='unknown'||item.observation_failed||
 ['waiting_input','failed','unknown'].includes(item.provider_status||'');

type ProgressTask=CloudProgressTaskLink & {href:string};
type ProgressEntry={item:CloudNodeProgress;task?:ProgressTask};
type ProgressPage={entries:ProgressEntry[];next?:string};
const requiredTaskFields=['run_id','node_id','task_id','task_version','run_relation'];

function readTaskLink(value:unknown,project:string):ProgressTask|undefined{
 if(!record(value)||!requiredTaskFields.every(field=>Object.hasOwn(value,field))||
    !Object.keys(value).every(field=>requiredTaskFields.includes(field)||field==='task_title')||
    !identifier(value.run_id,100)||!identifier(value.node_id,150)||typeof value.task_id!=='string'||!integer(value.task_version,1)||
    value.run_relation!=='current'&&value.run_relation!=='changed'&&value.run_relation!=='unbound')return;
 let title:string|undefined;
 if(Object.hasOwn(value,'task_title')){
  if(typeof value.task_title!=='string'||!wellFormed(value.task_title)||
     new TextEncoder().encode(value.task_title).length>500||/[\u0000-\u001f\u007f-\u009f]/.test(value.task_title))return;
  title=value.task_title;
 }
 const href=taskDeliveryHref(project,{kind:'delivery',id:value.task_id,runID:value.run_relation==='current'?value.run_id:undefined});
 if(href)return {run_id:value.run_id,node_id:value.node_id,task_id:value.task_id,task_version:value.task_version,
  run_relation:value.run_relation,task_title:title,href};
}

function readPage(value:unknown,project:string,filter:Props['filter'],taskID?:string):ProgressPage|undefined{
 if(!record(value)||value.project_id!==project||value.filter!==filter||value.task_id!==taskID||
    !Object.keys(value).every(field=>['project_id','items','next_cursor','filter','task_id','task_links'].includes(field))||
    !Array.isArray(value.items)||value.items.length>pageSize||
    !value.items.every((item):item is CloudNodeProgress=>readItem(item,project)&&(!filter||needsAttention(item)))||
    new Set(value.items.map(key)).size!==value.items.length)return;
 const next=value.next_cursor;
 if(next!==undefined&&(typeof next!=='string'||!/^[A-Za-z0-9_-]{1,1024}$/.test(next)||
    !filter&&!taskID&&value.items.length!==pageSize))return;
 const entries:ProgressEntry[]=value.items.map(item=>({item}));
 if(Object.hasOwn(value,'task_links')){
  if(!Array.isArray(value.task_links)||value.task_links.length>entries.length)return;
  const byPair=new Map(entries.map(entry=>[key(entry.item),entry]));
  for(const row of value.task_links){
   const task=readTaskLink(row,project);
   if(!task)return;
   const entry=byPair.get(key(task));
   if(!entry||entry.task)return;
   entry.task=task;
  }
 }
 if(taskID&&entries.some(entry=>entry.task?.task_id!==taskID))return;
 return {entries,next};
}

type Snapshot=ProgressPage & {scope:string;lastBatchCount:number;cursors:Set<string>};
type ReadState={scope:string;snapshot?:Snapshot;loading:boolean;error?:UiError};

export function WorkBuddyProgressWorkspace(props:Props){
 return <WorkBuddyProgressScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>;
}

function WorkBuddyProgressScope({project,session,refresh,onRun,filter,invalidFilter,onFilter,task,invalidTask,onTask}:Props){
 useLocale();
 const api=useApi(),user=session.user.id,organization=session.organization.id;
 const badTask=!!invalidTask||task!==undefined&&!workBuddyProgressTaskHref(project,task);
 const blocked=!!invalidFilter||badTask;
 const scope=JSON.stringify([filter,task,blocked]);
 const [state,setState]=useState<ReadState>({scope,loading:!blocked});
 const snapshot=useRef<Snapshot|undefined>(undefined),controller=useRef<AbortController|undefined>(undefined);
 const heading=useRef<HTMLHeadingElement>(null);
 const selectTask=onTask?(id?:string)=>{heading.current?.focus();onTask(id);}:undefined;
 const read=useCallback(async (append=false)=>{
  if(blocked)return;
  const previous=snapshot.current?.scope===scope?snapshot.current:undefined,before=append?previous?.next:undefined;
  if(append&&!before)return;
  controller.current?.abort();
  const abort=new AbortController();
  controller.current=abort;
  setState({scope,snapshot:previous,loading:true});
  try{
   const url='/workbuddy-cloud-progress'+query(project)+'&limit='+pageSize+
    (filter?'&filter='+filter:'')+(task?'&task_id='+encodeURIComponent(task):'')+(before?'&before='+encodeURIComponent(before):'');
   const value=await api<unknown>(url,undefined,'GET',{
    'X-Suma-Expected-User':user,'X-Suma-Expected-Organization':organization,
   },abort.signal);
   if(abort.signal.aborted)return;
   const page=readPage(value,project,filter,task);
   if(!page)throw new UiError('workbuddy-progress.invalid');
   const loaded=new Set(append?previous?.entries.map(({item})=>key(item)):[]);
   const cursors=new Set(append?previous?.cursors:[]);
   if(page.next&&cursors.has(page.next)||page.entries.some(({item})=>loaded.has(key(item))))throw new UiError('workbuddy-progress.invalid');
   if(page.next)cursors.add(page.next);
   const next={scope,entries:append&&previous?[...previous.entries,...page.entries]:page.entries,next:page.next,lastBatchCount:page.entries.length,cursors};
   snapshot.current=next;
   setState({scope,snapshot:next,loading:false});
  }catch(error){
   if(abort.signal.aborted)return;
   const denied=error instanceof ApiError&&[401,403,404].includes(error.status);
   const invalid=error instanceof UiError;
   snapshot.current=denied||invalid?undefined:previous;
   setState({scope,snapshot:snapshot.current,loading:false,error:new UiError(denied?'workbuddy-progress.denied':invalid?'workbuddy-progress.invalid':'workbuddy-progress.failed')});
  }
 },[api,project,user,organization,scope,filter,task,blocked]);
 useEffect(()=>{
  if(snapshot.current?.scope!==scope)snapshot.current=undefined;
  void read();
  return()=>controller.current?.abort();
 },[read,refresh,scope]);

 // Hide the previous scope immediately, before its pending request is aborted.
 const current=state.scope===scope?state:{scope,loading:!blocked};
 const rows=current.snapshot?.entries??[];
 const available=!!current.snapshot&&!current.loading&&!current.error;
 const filtered=!!filter||!!task;
 const emptyBatch=available&&filtered&&current.snapshot?.lastBatchCount===0;
 const taskTitle=task?rows[0]?.task?.task_title?.trim():undefined;
 return <section className="workbuddy-progress" aria-label={t('title')} aria-busy={current.loading} data-workbuddy-progress>
  <header className="workbuddy-progress-heading">
   <div><h2 ref={heading} tabIndex={-1}><Cloud size={18} aria-hidden="true"/>{t('title')}</h2><p>{t('intro')}</p></div>
   <button type="button" data-workbuddy-progress-refresh disabled={current.loading||blocked} onClick={()=>void read()}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>
  </header>
  <p className="workbuddy-progress-boundary">{t('boundary')}</p>
  <div className="workbuddy-progress-facets" role="group" aria-label={t('filter_label')}>
   <button type="button" data-workbuddy-progress-filter="all" aria-pressed={!invalidFilter&&!filter} disabled={!onFilter} onClick={()=>{if(invalidFilter||filter)onFilter?.();}}>{t('filter_all')}</button>
   <button type="button" data-workbuddy-progress-filter="attention" aria-pressed={!invalidFilter&&filter==='attention'} disabled={!onFilter} onClick={()=>{if(invalidFilter||filter!=='attention')onFilter?.('attention');}}>{t('filter_attention')}</button>
   {(task||badTask)&&<div className="workbuddy-progress-task-scope" data-workbuddy-progress-task-scope>
    <ListFilter size={15} aria-hidden="true"/>
    <span>{badTask?t('task_scope_invalid'):t('task_scope')} {!badTask&&<bdi>{taskTitle||task}</bdi>}</span>
    {selectTask&&<button type="button" data-workbuddy-progress-task-clear onClick={()=>selectTask()}><X size={15} aria-hidden="true"/>{t('task_clear')}</button>}
   </div>}
  </div>
  {invalidFilter&&<div data-workbuddy-progress-invalid-filter><ErrorNotice error={new UiError('workbuddy-progress.invalid_filter')}/></div>}
  {badTask&&<div data-workbuddy-progress-invalid-task><ErrorNotice error={new UiError('workbuddy-progress.invalid_task')}/></div>}
  {current.error&&<ErrorNotice error={current.error} onRetry={()=>void read()}/>}
  {current.error&&current.snapshot&&<p className="workbuddy-progress-note" role="status">{t('stale')}</p>}
  {current.loading&&!current.snapshot&&<Loading/>}
  {current.snapshot&&<>
   {rows.length>0&&<p className="workbuddy-progress-note" role="status">{t('loaded',{count:rows.length,amount:formatNumber(rows.length)})}</p>}
   {available&&!filter&&!current.snapshot.next&&rows.length===0&&<Empty
    title={t(task?'empty_task_title':'empty_title')} description={t(task?'empty_task_detail':'empty_detail')}/>}
   {rows.length>0&&<ul className="workbuddy-progress-rows" aria-label={t('rows')}>{rows.map(({item,task:linkedTask})=>
    <ProgressRow key={key(item)} project={project} item={item} task={linkedTask} available={available} onRun={onRun}
     filter={filter} onTask={task?undefined:selectTask}/>
   )}</ul>}
   {emptyBatch&&current.snapshot.next&&<p className="workbuddy-progress-note" role="status" data-workbuddy-progress-empty-batch>{t(filter?'empty_batch':'empty_task_batch')}</p>}
   {available&&filter==='attention'&&!current.snapshot.next&&rows.length===0&&<p className="workbuddy-progress-note" role="status" data-workbuddy-progress-empty-attention>{t('empty_attention')}</p>}
   {current.snapshot.next&&<div className="workbuddy-progress-more"><button type="button" data-workbuddy-progress-more disabled={!available} onClick={()=>void read(true)}>{t(filtered?'more_attention':'more')}</button></div>}
  </>}
 </section>;
}

function ProgressRow({project,item,task,available,onRun,filter,onTask}:ProgressEntry & {project:string;available:boolean;onRun:Props['onRun'];filter?:Props['filter'];onTask?:Props['onTask']}){
 const href=executionHref(project,item.run_id,item.node_id,item.attempt||undefined);
 const follow=(event:MouseEvent<HTMLAnchorElement>)=>{
  if(event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
  event.preventDefault();
  onRun(item.run_id,item.node_id,item.attempt||undefined);
 };
 const vendor=item.provider_status==='canceled'?'cancelled':item.provider_status;
 const runTitle=tr('workbuddy-cloud-task.source_run',{id:item.run_id.slice(0,12)});
 const taskTitle=task?.task_title?.trim();
 const taskScopeHref=task&&workBuddyProgressTaskHref(project,task.task_id,filter);
 const deliveryAction=task?.run_relation==='current'?'task_open':'task_open_current';
 return <li className="workbuddy-progress-row" data-workbuddy-progress-row={item.run_id} data-workbuddy-progress-node={item.node_id} data-attention={needsAttention(item)||undefined}>
  <div className="workbuddy-progress-row-heading">
   <div>
    <h3 data-workbuddy-progress-task={task?.task_id}>{task?<bdi>{taskTitle||t('task_id',{id:task.task_id})}</bdi>:runTitle}</h3>
    {task&&<>
     <p className="workbuddy-progress-meta">
      {taskTitle&&<code>{task.task_id}</code>}
      <span>{tr('workbuddy-cloud-task.delivery_version',{version:formatNumber(task.task_version)})}</span>
     </p>
     <p data-workbuddy-progress-task-relation={task.run_relation}>{tr('workbuddy-cloud-task.delivery_relation_'+task.run_relation)}</p>
     {available&&onTask&&taskScopeHref&&<a className="workbuddy-progress-task-scope-action" data-workbuddy-progress-task-filter
      href={taskScopeHref} onClick={event=>{
       if(event.defaultPrevented||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
       event.preventDefault();
       onTask(task.task_id);
      }} aria-label={t('task_scope_open_label',{task:taskTitle||task.task_id})}>
      <ListFilter size={15} aria-hidden="true"/>{t('task_history')}</a>}
    </>}
    <p className="workbuddy-progress-meta">
     {task&&<span>{runTitle}</span>}
     <span><code>{item.node_id}</code> · {item.attempt?t('attempt',{count:formatNumber(item.attempt)}):t('no_attempt')}</span>
    </p>
   </div>
   {available&&<div className="workbuddy-progress-row-actions">
    {task&&<a className="workbuddy-progress-task-action" data-workbuddy-progress-task-link href={task.href}
     onClick={event=>followTaskDeliveryHref(event,task.href)}
     aria-label={t(deliveryAction+'_label',{task:taskTitle||task.task_id})}>{t(deliveryAction)}<ArrowRight size={15} aria-hidden="true"/></a>}
    <a data-workbuddy-progress-open href={href} onClick={follow} aria-label={t('open_label',{run:item.run_id,node:item.node_id})}>{t('open')}<ArrowRight size={15} aria-hidden="true"/></a>
   </div>}
  </div>
  <dl className="workbuddy-progress-facts">
   <div><dt>{tr('workbuddy-cloud-task.platform_run_state')}</dt><dd><Badge state={item.run_state}/></dd></div>
   <div><dt>{tr('workbuddy-cloud-task.platform_node_state')}</dt><dd>{item.node_state==='waiting'
    ?<span className="badge state-waiting"><i aria-hidden="true"/>{t('node_waiting')}</span>
    :<Badge state={item.node_state}/>}</dd></div>
   <div><dt>{t('submission')}</dt><dd>{t('submission_'+item.submission_state)}</dd></div>
   <div><dt>{tr('workbuddy-cloud-task.provider_status')}</dt><dd>{vendor?t('provider_'+vendor):tr('workbuddy-cloud-task.not_reported')}</dd></div>
   <div><dt>{tr('workbuddy-cloud-task.last_observed')}</dt><dd>{item.observed_at?<time dateTime={item.observed_at}>{formatDateTime(item.observed_at,{dateStyle:'medium',timeStyle:'short'})}</time>:tr('workbuddy-cloud-task.not_verified')}</dd></div>
  </dl>
  {item.node_reason&&<p className="workbuddy-progress-attention">{tr('workbuddy-cloud-task.'+reasons[item.node_reason])}</p>}
  {item.observation_failed&&<p className="workbuddy-progress-attention">{t('observation_failed')}</p>}
 </li>;
}
