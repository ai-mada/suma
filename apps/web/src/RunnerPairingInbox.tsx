import {useEffect,useId,useRef,useState,type FormEvent} from 'react';
import {ChevronLeft,ChevronRight,RefreshCw,Search} from 'lucide-react';
import type {RunnerPairingPage} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {readRunnerPairingPage,runnerPairingInboxPath,runnerPairingInboxSearch,runnerPairingInboxStatuses,type RunnerPairingInboxStatus} from './runner-pairing-model';
import './RunnerPairingInbox.css';

export type RunnerPairingInboxProps={project:string;disabled?:boolean;accessBlocked?:boolean;selectedID?:string;refreshToken?:number;onSelect:(id:string)=>void;onDenied:()=>void};
type Identity={userID:string;organizationID:string};
type Query={status:RunnerPairingInboxStatus;q:string;limit:number;cursors:string[]};
type Snapshot={api:ApiCaller;path:string;page?:RunnerPairingPage;loading:boolean;error?:unknown};
const t=(key:string,values?:Record<string,unknown>)=>tr('runner-pairing-inbox.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);

/** Only an exact public ID leaves this current authorized list. The parent
 * rereads its detail before any independent approval or continuation. */
export function RunnerPairingInbox(props:RunnerPairingInboxProps){
 const api=useApi(),identity=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 return <ScopedInbox key={JSON.stringify([props.project,identity?.userID,identity?.organizationID,owner.current.epoch])} {...props} api={api} identity={identity}/>;
}
function ScopedInbox({project,disabled=false,accessBlocked=false,selectedID,refreshToken=0,onSelect,onDenied,api,identity}:RunnerPairingInboxProps&{api:ApiCaller;identity?:Identity}){
 useLocale();const heading=useId(),searchHint=useId(),[query,setQuery]=useState<Query>({status:'active',q:'',limit:20,cursors:['']}),[draft,setDraft]=useState(''),[searchError,setSearchError]=useState(false),[revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>();
 const request={project,status:query.status,q:query.q,limit:query.limit,cursor:query.cursors.at(-1)||''};
 let path='',queryError:unknown;try{path=runnerPairingInboxPath(request);}catch(error){queryError=error;}
 const enabled=!!identity&&!accessBlocked&&!!path,epoch=useRef(0),owner=useRef({api,path,enabled}),callbacks=useRef({onSelect,onDenied});owner.current={api,path,enabled};callbacks.current={onSelect,onDenied};
 useEffect(()=>{
  const ticket=++epoch.current,abort=new AbortController();
  if(!enabled||!identity){setSnapshot(undefined);return()=>abort.abort();}
  const live=()=>!abort.signal.aborted&&epoch.current===ticket&&owner.current.api===api&&owner.current.path===path&&owner.current.enabled;
  setSnapshot(previous=>({api,path,page:previous?.api===api&&previous.path===path?previous.page:undefined,loading:true}));
  void api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':identity.userID,'X-Suma-Expected-Organization':identity.organizationID},abort.signal).then(value=>{
   if(!live())return;const page=readRunnerPairingPage(value,request);setSnapshot({api,path,page,loading:false});
  }).catch(error=>{
   if(!live())return;setSnapshot(previous=>({api,path,page:refused(error)?undefined:previous?.api===api&&previous.path===path?previous.page:undefined,loading:false,error}));
   if(refused(error))callbacks.current.onDenied();
  });
  return()=>{epoch.current++;abort.abort();};
 },[api,path,enabled,identity?.userID,identity?.organizationID,revision,refreshToken]);
 const current=enabled&&snapshot?.api===api&&snapshot.path===path?snapshot:undefined,page=current?.page,loading=enabled&&(!current||current.loading),error=current?.error||queryError,locked=disabled||!identity||accessBlocked,controlsLocked=locked||refused(error),ready=!locked&&!loading&&!error&&!!page;
 function submit(event:FormEvent){event.preventDefault();if(controlsLocked)return;const normalized=runnerPairingInboxSearch(draft);if(normalized===undefined){setSearchError(true);return;}setSearchError(false);setDraft(normalized);setQuery(previous=>({...previous,q:normalized,cursors:['']}));setRevision(value=>value+1);}
 function select(id:string){if(ready&&page?.items.some(item=>item.id===id))callbacks.current.onSelect(id);}
 if(accessBlocked)return null;
 return <section className="runner-pairing-inbox" data-runner-pairing-inbox aria-labelledby={heading} aria-busy={loading}>
  <header><div><h4 id={heading}>{t('title')}</h4><p>{t('intro')}</p></div><button type="button" data-runner-pairing-inbox-refresh disabled={locked||loading||!path} onClick={()=>{if(!locked&&!loading)setRevision(value=>value+1);}}><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button></header>
  {!identity?<p role="status">{t('waiting_identity')}</p>:<>
   <form className="runner-pairing-inbox-filters" onSubmit={submit}>
    <label>{t('status')}<select data-runner-pairing-inbox-status value={query.status} disabled={controlsLocked} onChange={event=>{const status=event.target.value as RunnerPairingInboxStatus;if(!controlsLocked&&runnerPairingInboxStatuses.includes(status))setQuery(previous=>({...previous,status,cursors:['']}));}}>{runnerPairingInboxStatuses.map(status=><option key={status} value={status}>{t('status.'+status)}</option>)}</select></label>
    <div className="runner-pairing-inbox-search"><label htmlFor={searchHint+'-query'}>{t('query')}</label><span><input id={searchHint+'-query'} type="search" data-runner-pairing-inbox-query value={draft} maxLength={1000} disabled={controlsLocked} aria-describedby={searchHint} onChange={event=>{if(!controlsLocked){setDraft(event.target.value);setSearchError(false);}}}/><button type="submit" data-runner-pairing-inbox-search disabled={controlsLocked}><Search size={15} aria-hidden="true"/>{t('search')}</button></span></div>
    <label>{t('limit')}<select data-runner-pairing-inbox-limit value={query.limit} disabled={controlsLocked} onChange={event=>{const limit=Number(event.target.value);if(!controlsLocked&&[20,50,100].includes(limit))setQuery(previous=>({...previous,limit,cursors:['']}));}}>{[20,50,100].map(limit=><option key={limit} value={limit}>{formatNumber(limit)}</option>)}</select></label>
   </form><p id={searchHint} className="runner-pairing-inbox-help">{t('query_hint')}</p>
   {searchError&&<p className="runner-pairing-inbox-warning" role="alert">{t('invalid_query')}</p>}
   {!!error&&<ErrorNotice error={error}/>}
   {loading&&<p role="status">{t('loading')}</p>}
   {page&&(loading||!!error)&&<p className="runner-pairing-inbox-warning" data-runner-pairing-inbox-stale role="status">{t('stale')}</p>}
   {page&&<><div className="runner-pairing-inbox-source"><span>{t('count',{shown:formatNumber(page.items.length),total:formatNumber(page.total)})}</span><span>{t('query_source',{query:page.q||t('no_query')})}</span><span>{t('as_of')} <time dateTime={page.as_of} title={page.as_of}>{formatDateTime(page.as_of,{dateStyle:'short',timeStyle:'medium'})}</time></span></div>
    <ul className="runner-pairing-inbox-items">{page.items.map(item=><li key={item.id}><button type="button" data-runner-pairing-inbox-select={item.id} aria-pressed={selectedID===item.id} disabled={!ready} onClick={()=>select(item.id)}><span className="runner-pairing-inbox-row-heading"><strong>{item.device_name?t('device',{name:item.device_name}):t('no_device')}</strong><span className="runner-pairing-inbox-state">{t('state.'+item.state)}</span></span><code>{item.id}</code><span>{item.reason_code?t('reason.'+item.reason_code):t('detail_required')}</span>{item.worker_id&&<small>{t('worker',{id:item.worker_id})}</small>}<small>{t('expires')} {formatDateTime(item.expires_at,{dateStyle:'short',timeStyle:'medium'})}</small></button></li>)}</ul>
    {!loading&&!error&&!page.items.length&&<p className="runner-pairing-inbox-empty" role="status">{t('empty')}</p>}
   </>}
   <footer><p>{t('live')}</p><div><button type="button" data-runner-pairing-inbox-previous disabled={controlsLocked||loading||query.cursors.length===1} onClick={()=>{if(!controlsLocked&&!loading&&query.cursors.length>1)setQuery(previous=>previous===query?{...previous,cursors:previous.cursors.slice(0,-1)}:previous);}}><ChevronLeft size={15} aria-hidden="true"/>{t('previous')}</button><button type="button" data-runner-pairing-inbox-next disabled={!ready||!page?.next_cursor} onClick={()=>{const cursor=page?.next_cursor;if(ready&&cursor)setQuery(previous=>previous===query?{...previous,cursors:[...previous.cursors,cursor]}:previous);}}>{t('next')}<ChevronRight size={15} aria-hidden="true"/></button></div></footer>
  </>}
 </section>;
}
