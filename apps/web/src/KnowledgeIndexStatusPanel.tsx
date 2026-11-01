import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {Database,RefreshCw} from 'lucide-react';
import type {KnowledgeIndexSource,ProjectKnowledgeIndexStatus} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {knowledgeIndexStatusPath,readKnowledgeIndexStatus} from './knowledge-index-status-model';
import './KnowledgeIndexStatusPanel.css';

export type KnowledgeIndexStatusPanelProps={project:string;disabled?:boolean;accessBlocked?:boolean;refresh?:number|string;onAccessDenied?:()=>void};
const t=(key:string)=>tr('knowledge-index-status.'+key),refusal=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type ReaderProps=KnowledgeIndexStatusPanelProps&{api:ApiCaller;userID:string;organizationID:string};
type Snapshot={data?:ProjectKnowledgeIndexStatus;loading:boolean;error?:unknown;denied?:boolean};

/** One optional read surface. It never retries a projection or probes a service. */
export function KnowledgeIndexStatusPanel(props:KnowledgeIndexStatusPanelProps){
 useLocale();const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 return <StatusScope key={JSON.stringify([view?.userID,view?.organizationID,props.project,owner.current.epoch])} {...props} api={api} userID={view?.userID??''} organizationID={view?.organizationID??''}/>;
}
function StatusScope(props:ReaderProps){
 const [open,setOpen]=useState(false),id=useId();
 return <details className="knowledge-index-status" data-knowledge-index-status onToggle={event=>{const next=event.currentTarget.open;if(next&&(props.disabled||props.accessBlocked)){event.currentTarget.open=false;setOpen(false);}else setOpen(next);}}>
  <summary data-knowledge-index-status-toggle aria-controls={id} aria-disabled={props.disabled||props.accessBlocked||undefined} onClick={event=>{if((props.disabled||props.accessBlocked)&&!event.currentTarget.parentElement?.hasAttribute('open'))event.preventDefault();}}><Database size={16} aria-hidden="true"/>{t('title')}</summary>
  {open&&<div id={id} className="knowledge-index-status-content">{props.userID&&props.organizationID?<StatusReader {...props}/>:<p role="status">{t('view_required')}</p>}</div>}
 </details>;
}
function StatusReader({api,userID,organizationID,project,disabled=false,accessBlocked=false,refresh=0,onAccessDenied}:ReaderProps){
 const [snapshot,setSnapshot]=useState<Snapshot>({loading:true}),current=useRef(snapshot),alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef(false),refused=useRef(false),notified=useRef(false);
 const parent=useRef({disabled,accessBlocked}),callbacks=useRef({onAccessDenied});parent.current={disabled,accessBlocked};callbacks.current={onAccessDenied};
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};
 const deny=useCallback(()=>{refused.current=true;store({loading:false,denied:true});if(!notified.current){notified.current=true;callbacks.current.onAccessDenied?.();}},[]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;controller.current?.abort();flight.current=false;};},[]);
 useLayoutEffect(()=>{if(accessBlocked){++epoch.current;controller.current?.abort();flight.current=false;refused.current=true;store({loading:false,denied:true});}},[accessBlocked]);
 const read=useCallback(async()=>{
  if(!alive.current||parent.current.disabled||parent.current.accessBlocked||flight.current)return;
  const ticket=++epoch.current,abort=new AbortController(),previous=refused.current?undefined:current.current.data;controller.current?.abort();controller.current=abort;flight.current=true;store({data:previous,loading:true,denied:refused.current});
  const owns=()=>alive.current&&epoch.current===ticket&&!abort.signal.aborted&&!parent.current.accessBlocked;
  try{const value=await api<unknown>(knowledgeIndexStatusPath(project),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);if(!owns())return;const data=readKnowledgeIndexStatus(value,{project,organizationID});refused.current=false;notified.current=false;store({data,loading:false});}
  catch(error){if(!owns())return;if(refusal(error))deny();else store({data:previous,loading:false,error,denied:refused.current});}
  finally{if(controller.current===abort)flight.current=false;}
 },[api,project,userID,organizationID,deny]);
 useEffect(()=>{void read();return()=>{++epoch.current;controller.current?.abort();flight.current=false;};},[read,refresh]);
 const hidden=accessBlocked||!!snapshot.denied,data=hidden?undefined:snapshot.data,state=hidden?'denied':snapshot.loading?data?'refreshing':'loading':snapshot.error?data?'stale':'error':'current';
 return <div data-knowledge-index-status-reader><p className="knowledge-index-status-message" role={state==='denied'?'alert':'status'} data-knowledge-index-status-state={state}>{t(state)}</p>
  {!hidden&&snapshot.error!==undefined&&<ErrorNotice error={snapshot.error}/>}
  {data&&<KnowledgeIndexStatusFacts status={data}/>}
  <button type="button" className="text-button" data-knowledge-index-status-refresh disabled={disabled||accessBlocked||snapshot.loading} onClick={()=>void read()}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>
 </div>;
}
export function KnowledgeIndexStatusFacts({status}:{status:ProjectKnowledgeIndexStatus}){
 useLocale();const event=status.latest_event,ack=status.last_ack;
 return <div className="knowledge-index-status-facts">
  <p>{t(status.configured?'configured':'unconfigured')}</p><p className="knowledge-index-status-boundary">{t('boundary')}</p>
  <p className="knowledge-index-status-asof">{t('as_of')} <time dateTime={status.as_of} title={status.as_of}>{formatDateTime(status.as_of,{dateStyle:'short',timeStyle:'medium'})}</time></p>
  <dl className="knowledge-index-status-counts">{(['pending','indexed','failed','total','current_publications','untracked_publications'] as const).map(key=><div key={key}><dt>{t(key)}</dt><dd>{formatNumber(status[key])}</dd></div>)}</dl>
  <details data-knowledge-index-status-event><summary>{t('latest')}</summary>{event?<dl className="knowledge-index-status-source"><Fact label="event" value={event.event_id}/><Fact label="action" value={t('action.'+event.action)}/><Fact label="state" value={t('state.'+event.state)}/><Fact label="created" value={formatDateTime(event.created_at,{dateStyle:'short',timeStyle:'medium'})}/><SourceFacts source={event.source}/></dl>:<p>{t('no_latest')}</p>}</details>
  <details data-knowledge-index-status-ack><summary>{t('ack')}</summary>{ack?<dl className="knowledge-index-status-source"><Fact label="protocol" value={ack.protocol}/><Fact label="event" value={ack.event_id}/><Fact label="tenant" value={ack.tenant_id}/><Fact label="project" value={ack.project_id}/><Fact label="action" value={t('action.'+ack.action)}/><Fact label="disposition" value={t('disposition.'+ack.disposition)}/><Fact label="generation" value={ack.generation}/><Fact label="watermark" value={formatNumber(ack.index_watermark)}/><SourceFacts source={ack.source}/><Fact label="request_digest" value={ack.request_sha256}/></dl>:<p>{t('no_ack')}</p>}</details>
  <p className="knowledge-index-status-boundary">{t('independent')}</p>
 </div>;
}
function SourceFacts({source}:{source:KnowledgeIndexSource}){return <><Fact label="source" value={source.id}/><Fact label="sequence" value={formatNumber(source.sequence)}/>{source.revision===0?<div className="knowledge-index-status-unpublished"><dt>{t('revision')}</dt><dd>{t('unpublished')}</dd></div>:<><Fact label="revision" value={formatNumber(source.revision)}/><Fact label="digest" value={source.sha256}/></>}</>;}
function Fact({label,value}:{label:string;value:string}){return <div><dt>{t(label)}</dt><dd>{value}</dd></div>;}
