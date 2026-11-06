import {useCallback,useEffect,useRef,useState} from 'react';
import type {ToolUsage,UsageAttemptFactPage} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import './UsageAttemptFacts.css';

const t=(key:string)=>tr('usage-accounting.'+key);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9._:-]{1,64}$/.test(value);
const counters=['input_tokens','output_tokens','cached_input_tokens','cache_write_input_tokens','reasoning_output_tokens'] as const;
type Counter=typeof counters[number];
type View=NonNullable<ReturnType<typeof useApiView>>;
type State={page?:UsageAttemptFactPage;loading:boolean;error?:unknown;retryEarlier?:boolean};
const counterValid=(value:unknown)=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;

function validPage(page:UsageAttemptFactPage,project:string,attempt:string,before:string){
 if(!page||page.project_id!==project||page.attempt_id!==attempt||!Array.isArray(page.items)||page.items.length>20||typeof page.next_cursor!=='string')return false;
 if(page.next_cursor&&(!id(page.next_cursor)||page.next_cursor===before||page.next_cursor!==page.items.at(-1)?.id))return false;
 return new Set(page.items.map(item=>item?.id)).size===page.items.length&&page.items.every(item=>
  !!item&&id(item.id)&&item.id!==before&&['result','resolution','confirmation'].includes(item.kind)&&
  typeof item.created_at==='string'&&Number.isFinite(Date.parse(item.created_at))&&
  typeof item.identity_sha256==='string'&&/^[a-f0-9]{64}$/.test(item.identity_sha256)&&
  typeof item.original_result==='boolean'&&(!item.original_result||item.kind==='result')&&
  (item.usage_state===undefined||['complete','partial','unreported'].includes(item.usage_state))&&
  (item.usage===undefined||item.usage!==null&&typeof item.usage==='object'&&!Array.isArray(item.usage)&&
   Object.entries(item.usage).every(([key,value])=>counters.includes(key as Counter)&&counterValid(value))));
}

export function UsageAttemptFacts({project,attempt}:{project:string;attempt:string}){
 useLocale();
 const view=useApiView();
 if(!view||!id(project)||!id(attempt))return null;
 return <UsageFactsScope key={JSON.stringify([view.userID,view.organizationID,project,attempt])} project={project} attempt={attempt} view={view}/>;
}

function UsageFactsScope({project,attempt,view}:{project:string;attempt:string;view:View}){
 const [open,setOpen]=useState(false);
 return <details className="ua-disclosure ua-evidence" onToggle={event=>setOpen(event.currentTarget.open)}>
  <summary data-usage-facts-toggle>{t('facts.title')}</summary>
  {open&&<UsageFactsRead project={project} attempt={attempt} view={view}/>}
 </details>;
}

function UsageFactsRead({project,attempt,view:{userID,organizationID}}:{project:string;attempt:string;view:View}){
 const api=useApi();
 const [state,setState]=useState<State>({loading:true});
 const current=useRef<UsageAttemptFactPage|undefined>(undefined);
 const active=useRef<AbortController|undefined>(undefined);
 const panel=useRef<HTMLDivElement>(null),earlierButton=useRef<HTMLButtonElement>(null);
 const read=useCallback(async(earlier=false)=>{
  if(active.current)return;
  const previous=earlier?current.current:undefined,before=previous?.next_cursor||'';
  if(earlier&&!before)return;
  const request=new AbortController();
  active.current=request;
  setState({page:previous,loading:true});
  try{
   const path='/usage/attempts/'+encodeURIComponent(attempt)+'/facts'+query(project)+'&limit=20'+(before?'&before='+encodeURIComponent(before):'');
   const page=await api<UsageAttemptFactPage>(path,undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},request.signal);
   if(request.signal.aborted)return;
   if(!validPage(page,project,attempt,before))throw new UiError('usage-accounting.invalid_read');
   const previousIDs=new Set(previous?.items.map(item=>item.id));
   if(page.items.some(item=>previousIDs.has(item.id)))throw new UiError('usage-accounting.invalid_read');
   const items=[...(previous?.items||[]),...page.items];
   if(earlier&&!page.next_cursor&&document.activeElement===earlierButton.current)panel.current?.focus();
   current.current={...page,items};
   setState({page:current.current,loading:false});
  }catch(error){
   if(request.signal.aborted)return;
   if(earlier&&document.activeElement===earlierButton.current)panel.current?.focus();
   const denied=error instanceof ApiError&&[401,403,404].includes(error.status);
   current.current=denied?undefined:previous;
   setState({page:current.current,loading:false,error,retryEarlier:earlier&&!denied});
  }finally{if(active.current===request)active.current=undefined;}
 },[api,project,attempt,userID,organizationID]);
 useEffect(()=>{
  void read();
  return()=>{active.current?.abort();active.current=undefined;};
 },[read]);
 const denied=state.error instanceof ApiError&&[401,403,404].includes(state.error.status);
 return <div ref={panel} tabIndex={-1} className="ua-evidence-body" aria-busy={state.loading}>
  <p className="ua-muted">{t('facts.help')}</p>
  {state.loading&&<p role="status">{t('facts.loading')}</p>}
  {state.error!==undefined&&<div className="ua-evidence-error" role="status">
   <p>{t(denied?'facts.unavailable':'facts.failed')}</p>
   {!denied&&<button type="button" onClick={()=>void read(state.retryEarlier)}>{t('facts.retry')}</button>}
  </div>}
  {state.page&&state.page.items.length===0&&<p>{t('facts.empty')}</p>}
  {!!state.page?.items.length&&<ol className="ua-evidence-list">
   {state.page.items.map(fact=><li key={fact.id} data-usage-fact={fact.kind}>
    <div className="ua-evidence-heading">
     <strong>{t('facts.'+(fact.kind==='result'?(fact.original_result?'original_result':'additional_result'):fact.kind))}</strong>
     <time dateTime={fact.created_at} title={fact.created_at}>{formatDateTime(fact.created_at,{timeZone:'UTC',year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false})} UTC</time>
    </div>
    {fact.usage_state&&<p className="ua-muted">{t('coverage.'+fact.usage_state)}</p>}
    {fact.kind!=='resolution'&&<dl className="ua-evidence-counters">
     {counters.filter((key,index)=>index<2||fact.usage?.[key]!==undefined).map(key=><div key={key}>
      <dt>{t('counter.'+key)}</dt><dd>{displayCounter(fact.usage,key)}</dd>
     </div>)}
    </dl>}
    <details className="ua-evidence-identity"><summary>{t('facts.digest')}</summary><code>{fact.identity_sha256}</code></details>
   </li>)}
  </ol>}
  {state.page?.next_cursor&&state.error===undefined&&<button ref={earlierButton} type="button" data-usage-facts-earlier disabled={state.loading} onClick={()=>void read(true)}>{t('facts.earlier')}</button>}
 </div>;
}

function displayCounter(usage:ToolUsage|undefined,key:Counter){
 const value=usage?.[key];
 return typeof value==='number'?formatNumber(value):t('unknown');
}
