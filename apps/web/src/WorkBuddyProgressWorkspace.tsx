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

function readPage(value:unknown,project:string):value is CloudNodeProgressPage{
 if(!record(value)||value.project_id!==project||
    !Object.keys(value).every(field=>['project_id','items','next_cursor'].includes(field))||
    !Array.isArray(value.items)||value.items.length>pageSize||
    !value.items.every(item=>readItem(item,project))||new Set(value.items.map(key)).size!==value.items.length)return false;
 return value.next_cursor===undefined||typeof value.next_cursor==='string'&&/^[A-Za-z0-9_-]{1,1024}$/.test(value.next_cursor)&&value.items.length===pageSize;
}

const needsAttention=(item:CloudNodeProgress)=>!!item.node_reason||item.submission_state==='unknown'||item.observation_failed||
 ['waiting_input','failed','unknown'].includes(item.provider_status||'');

type Snapshot={items:CloudNodeProgress[];next?:string;};
type ReadState={snapshot?:Snapshot;loading:boolean;error?:UiError};

export function WorkBuddyProgressWorkspace(props:Props){
 return <WorkBuddyProgressScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>;
}

function WorkBuddyProgressScope({project,session,refresh,onRun}:Props){
 useLocale();
 const api=useApi(),user=session.user.id,organization=session.organization.id;
 const [current,setCurrent]=useState<ReadState>({loading:true});
 const [facet,setFacet]=useState<'all'|'attention'>('all');
 const snapshot=useRef<Snapshot|undefined>(undefined),controller=useRef<AbortController|undefined>(undefined);
 const read=useCallback(async (append=false)=>{
  const previous=snapshot.current,before=append?previous?.next:undefined;
  if(append&&!before)return;
  controller.current?.abort();
  const abort=new AbortController();
  controller.current=abort;
  setCurrent({snapshot:previous,loading:true});
  try{
   const value=await api<unknown>('/workbuddy-cloud-progress'+query(project)+'&limit='+pageSize+(before?'&before='+encodeURIComponent(before):''),undefined,'GET',{
    'X-Suma-Expected-User':user,'X-Suma-Expected-Organization':organization,
   },abort.signal);
   if(abort.signal.aborted)return;
   if(!readPage(value,project))throw new UiError('workbuddy-progress.invalid');
   const loaded=new Set(append?previous?.items.map(key):[]);
   if((before&&value.next_cursor===before)||value.items.some(item=>loaded.has(key(item))))throw new UiError('workbuddy-progress.invalid');
   const next={items:append&&previous?[...previous.items,...value.items]:value.items,next:value.next_cursor};
   snapshot.current=next;
   setCurrent({snapshot:next,loading:false});
  }catch(error){
   if(abort.signal.aborted)return;
   const denied=error instanceof ApiError&&[401,403,404].includes(error.status);
   const invalid=error instanceof UiError;
   if(denied||invalid)snapshot.current=undefined;
   setCurrent({snapshot:snapshot.current,loading:false,error:new UiError(denied?'workbuddy-progress.denied':invalid?'workbuddy-progress.invalid':'workbuddy-progress.failed')});
  }
 },[api,project,user,organization]);
 useEffect(()=>{void read();return()=>controller.current?.abort();},[read,refresh]);

 const rows=current.snapshot?.items??[],attention=rows.filter(needsAttention);
 const visible=facet==='attention'?attention:rows;
 // A retained snapshot is display-only until the current read succeeds.
 const available=!!current.snapshot&&!current.loading&&!current.error;
 return <section className="workbuddy-progress" aria-label={t('title')} aria-busy={current.loading} data-workbuddy-progress>
  <header className="workbuddy-progress-heading">
   <div><h2><Cloud size={18} aria-hidden="true"/>{t('title')}</h2><p>{t('intro')}</p></div>
   <button type="button" data-workbuddy-progress-refresh disabled={current.loading} onClick={()=>void read()}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>
  </header>
  <p className="workbuddy-progress-boundary">{t('boundary')}</p>
  {current.error&&<ErrorNotice error={current.error} onRetry={()=>void read()}/>}
  {current.error&&current.snapshot&&<p className="workbuddy-progress-note" role="status">{t('stale')}</p>}
  {current.loading&&!current.snapshot&&<Loading/>}
  {current.snapshot&&<>
   <p className="workbuddy-progress-note" role="status">{t('loaded',{count:formatNumber(rows.length)})}</p>
   {rows.length>0&&<div className="workbuddy-progress-facets" role="group" aria-label={t('filter_label')}>
    <button type="button" data-workbuddy-progress-filter="all" aria-pressed={facet==='all'} disabled={!available} onClick={()=>setFacet('all')}>{t('filter_all')}<strong>{formatNumber(rows.length)}</strong></button>
    <button type="button" data-workbuddy-progress-filter="attention" aria-pressed={facet==='attention'} disabled={!available} onClick={()=>setFacet('attention')}>{t('filter_attention')}<strong>{formatNumber(attention.length)}</strong></button>
   </div>}
   {available&&rows.length===0&&<Empty title={t('empty_title')} description={t('empty_detail')}/>}
   {available&&rows.length>0&&visible.length===0&&<p className="workbuddy-progress-note" role="status">{t('empty_attention')}</p>}
   {visible.length>0&&<ul className="workbuddy-progress-rows" aria-label={t('rows')}>{visible.map(item=>
    <ProgressRow key={key(item)} project={project} item={item} available={available} onRun={onRun}/>
   )}</ul>}
   {current.snapshot.next&&<div className="workbuddy-progress-more"><button type="button" data-workbuddy-progress-more disabled={!available} onClick={()=>void read(true)}>{t('more')}</button></div>}
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
