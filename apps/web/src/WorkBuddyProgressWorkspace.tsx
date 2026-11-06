import {useCallback,useEffect,useRef,useState,type MouseEvent} from 'react';
import {ArrowRight,Cloud,RefreshCw} from 'lucide-react';
import type {CloudNodeProgress,CloudNodeProgressPage} from '../../../packages/contracts/src';
import {ApiError,query,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {executionHref} from './scheduling-model';
import {Badge,Empty,ErrorNotice,Loading} from './ui';

type Props={
 project:string;
 session:Session;
 refresh:number;
 filter?:'attention';
 invalidFilter?:boolean;
 onFilter?:(filter?:'attention')=>void;
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
const key=(item:CloudNodeProgress)=>JSON.stringify([item.run_id,item.node_id]);

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

function readPage(value:unknown,project:string,filter:Props['filter']):value is CloudNodeProgressPage{
 if(!record(value)||value.project_id!==project||value.filter!==filter||
    !Object.keys(value).every(field=>['project_id','items','next_cursor','filter'].includes(field))||
    !Array.isArray(value.items)||value.items.length>pageSize||
    !value.items.every(item=>readItem(item,project)&&(!filter||needsAttention(item)))||
    new Set(value.items.map(key)).size!==value.items.length)return false;
 return value.next_cursor===undefined||typeof value.next_cursor==='string'&&/^[A-Za-z0-9_-]{1,1024}$/.test(value.next_cursor)&&
  (filter==='attention'||value.items.length===pageSize);
}

type ReadScope='all'|'attention'|'invalid';
type Snapshot={scope:ReadScope;items:CloudNodeProgress[];next?:string;lastBatchCount:number;cursors:Set<string>};
type ReadState={scope:ReadScope;snapshot?:Snapshot;loading:boolean;error?:UiError};

export function WorkBuddyProgressWorkspace(props:Props){
 return <WorkBuddyProgressScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>;
}

function WorkBuddyProgressScope({project,session,refresh,onRun,filter,invalidFilter,onFilter}:Props){
 useLocale();
 const api=useApi(),user=session.user.id,organization=session.organization.id;
 const scope:ReadScope=invalidFilter?'invalid':filter||'all';
 const [state,setState]=useState<ReadState>({scope,loading:scope!=='invalid'});
 const snapshot=useRef<Snapshot|undefined>(undefined),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async (append=false)=>{
  if(scope==='invalid')return;
  const previous=snapshot.current?.scope===scope?snapshot.current:undefined,before=append?previous?.next:undefined;
  if(append&&!before)return;
  controller.current?.abort();
  const abort=new AbortController();
  controller.current=abort;
  setState({scope,snapshot:previous,loading:true});
  try{
   const url='/workbuddy-cloud-progress'+query(project)+'&limit='+pageSize+
    (filter?'&filter='+filter:'')+(before?'&before='+encodeURIComponent(before):'');
   const value=await api<unknown>(url,undefined,'GET',{
    'X-Suma-Expected-User':user,'X-Suma-Expected-Organization':organization,
   },abort.signal);
   if(abort.signal.aborted)return;
   if(!readPage(value,project,filter))throw new UiError('workbuddy-progress.invalid');
   const loaded=new Set(append?previous?.items.map(key):[]);
   const cursors=new Set(append?previous?.cursors:[]);
   if(value.next_cursor&&cursors.has(value.next_cursor)||value.items.some(item=>loaded.has(key(item))))throw new UiError('workbuddy-progress.invalid');
   if(value.next_cursor)cursors.add(value.next_cursor);
   const next={scope,items:append&&previous?[...previous.items,...value.items]:value.items,next:value.next_cursor,lastBatchCount:value.items.length,cursors};
   snapshot.current=next;
   setState({scope,snapshot:next,loading:false});
  }catch(error){
   if(abort.signal.aborted)return;
   const denied=error instanceof ApiError&&[401,403,404].includes(error.status);
   const invalid=error instanceof UiError;
   snapshot.current=denied||invalid?undefined:previous;
   setState({scope,snapshot:snapshot.current,loading:false,error:new UiError(denied?'workbuddy-progress.denied':invalid?'workbuddy-progress.invalid':'workbuddy-progress.failed')});
  }
 },[api,project,user,organization,scope,filter]);
 useEffect(()=>{
  if(snapshot.current?.scope!==scope)snapshot.current=undefined;
  void read();
  return()=>controller.current?.abort();
 },[read,refresh,scope]);

 // Never show another filter's snapshot while the new query is starting.
 const current=state.scope===scope?state:{scope,loading:scope!=='invalid'};
 const rows=current.snapshot?.items??[];
 const available=!!current.snapshot&&!current.loading&&!current.error;
 const emptyBatch=available&&scope==='attention'&&current.snapshot?.lastBatchCount===0;
 return <section className="workbuddy-progress" aria-label={t('title')} aria-busy={current.loading} data-workbuddy-progress>
  <header className="workbuddy-progress-heading">
   <div><h2><Cloud size={18} aria-hidden="true"/>{t('title')}</h2><p>{t('intro')}</p></div>
   <button type="button" data-workbuddy-progress-refresh disabled={current.loading||scope==='invalid'} onClick={()=>void read()}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>
  </header>
  <p className="workbuddy-progress-boundary">{t('boundary')}</p>
  <div className="workbuddy-progress-facets" role="group" aria-label={t('filter_label')}>
   <button type="button" data-workbuddy-progress-filter="all" aria-pressed={scope==='all'} disabled={!onFilter} onClick={()=>{if(scope!=='all')onFilter?.();}}>{t('filter_all')}</button>
   <button type="button" data-workbuddy-progress-filter="attention" aria-pressed={scope==='attention'} disabled={!onFilter} onClick={()=>{if(scope!=='attention')onFilter?.('attention');}}>{t('filter_attention')}</button>
  </div>
  {scope==='invalid'&&<div data-workbuddy-progress-invalid-filter><ErrorNotice error={new UiError('workbuddy-progress.invalid_filter')}/></div>}
  {current.error&&<ErrorNotice error={current.error} onRetry={()=>void read()}/>}
  {current.error&&current.snapshot&&<p className="workbuddy-progress-note" role="status">{t('stale')}</p>}
  {current.loading&&!current.snapshot&&<Loading/>}
  {current.snapshot&&<>
   {rows.length>0&&<p className="workbuddy-progress-note" role="status">{t('loaded',{count:formatNumber(rows.length)})}</p>}
   {available&&scope==='all'&&rows.length===0&&<Empty title={t('empty_title')} description={t('empty_detail')}/>}
   {rows.length>0&&<ul className="workbuddy-progress-rows" aria-label={t('rows')}>{rows.map(item=>
    <ProgressRow key={key(item)} project={project} item={item} available={available} onRun={onRun}/>
   )}</ul>}
   {emptyBatch&&current.snapshot.next&&<p className="workbuddy-progress-note" role="status" data-workbuddy-progress-empty-batch>{t('empty_batch')}</p>}
   {available&&scope==='attention'&&!current.snapshot.next&&rows.length===0&&<p className="workbuddy-progress-note" role="status" data-workbuddy-progress-empty-attention>{t('empty_attention')}</p>}
   {current.snapshot.next&&<div className="workbuddy-progress-more"><button type="button" data-workbuddy-progress-more disabled={!available} onClick={()=>void read(true)}>{t(scope==='attention'?'more_attention':'more')}</button></div>}
  </>}
 </section>;
}

function ProgressRow({project,item,available,onRun}:{project:string;item:CloudNodeProgress;available:boolean;onRun:Props['onRun']}){
 const href=executionHref(project,item.run_id,item.node_id,item.attempt||undefined);
 const follow=(event:MouseEvent<HTMLAnchorElement>)=>{
  if(event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;
  event.preventDefault();
  onRun(item.run_id,item.node_id,item.attempt||undefined);
 };
 const vendor=item.provider_status==='canceled'?'cancelled':item.provider_status;
 return <li className="workbuddy-progress-row" data-workbuddy-progress-row={item.run_id} data-workbuddy-progress-node={item.node_id} data-attention={needsAttention(item)||undefined}>
  <div className="workbuddy-progress-row-heading">
   <div>
    <h3>{tr('workbuddy-cloud-task.source_run',{id:item.run_id.slice(0,12)})}</h3>
    <p><code>{item.node_id}</code> · {item.attempt?t('attempt',{count:formatNumber(item.attempt)}):t('no_attempt')}</p>
   </div>
   {available&&<a data-workbuddy-progress-open href={href} onClick={follow} aria-label={t('open_label',{run:item.run_id,node:item.node_id})}>{t('open')}<ArrowRight size={15} aria-hidden="true"/></a>}
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
