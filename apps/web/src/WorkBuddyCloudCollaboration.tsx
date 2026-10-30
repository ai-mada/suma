import {useEffect,useId,useRef,useState} from 'react';
import {FileText,MessagesSquare,RefreshCw} from 'lucide-react';
import type {CloudCollaborationSource,CloudCollaborationView,CloudHistoryLoad,CloudHistoryPage} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction} from './write-action';
import {collaborationIdentity,collaborationPath,historyLoadPath,historyPagePath,readCloudCollaboration,readCloudHistoryPage,readCloudHistoryReceipt,validCloudHistoryRequest} from './workbuddy-cloud-collaboration-model';
import './WorkBuddyCloudCollaboration.css';

type Props={source:CloudCollaborationSource;revision?:number;disabled?:boolean};
type Meta={owner:ApiCaller;value?:CloudCollaborationView;loading:boolean;failed:boolean;denied:boolean};
type Page={owner:ApiCaller;value:CloudHistoryPage};
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-cloud-collaboration.'+key,values);
// A saved GET can invalidate its source; a POST head/quota conflict does not.
const refusedRead=(error:unknown)=>error instanceof ApiError&&([401,403,404].includes(error.status)||error.status===409&&error.code==='workbuddy_cloud_changed');
const date=(value:string)=>formatDateTime(value,{dateStyle:'medium',timeStyle:'short'});

/** The public node and private session archive have separate read authority. */
export function WorkBuddyCloudCollaboration(props:Props){
 const view=useApiView(),identity=collaborationIdentity(props.source);
 return <CollaborationController key={JSON.stringify([view?.userID,view?.organizationID,identity])} {...props}/>;
}

function CollaborationController({source,revision,disabled=false}:Props){
 useLocale();const id=useId(),api=useApi(),identity=useApiView();
 const [meta,setMeta]=useState<Meta>(),[page,setPage]=useState<Page>(),[selected,setSelected]=useState<string>(),[receipt,setReceipt]=useState<CloudHistoryLoad>(),[pageLoading,setPageLoading]=useState(false),[pageFailed,setPageFailed]=useState(false);
 const alive=useRef(true),owner=useRef(api),sourceRef=useRef(source),metaRef=useRef<Meta|undefined>(undefined),pageRef=useRef<Page|undefined>(undefined),selectedRef=useRef('');
 const metaEpoch=useRef(0),pageEpoch=useRef(0),metaAbort=useRef<AbortController|undefined>(undefined),pageAbort=useRef<AbortController|undefined>(undefined),pageBusy=useRef(false),commandBusy=useRef(false);
 owner.current=api;sourceRef.current=source;metaRef.current=meta?.owner===api?meta:undefined;pageRef.current=page?.owner===api?page:undefined;
 const current=metaRef.current,view=current?.value,currentSource=view?.source||source;
 const scope=JSON.stringify([identity?.userID,identity?.organizationID,collaborationIdentity(source)]);
 const action=useWriteAction({scope:'workbuddy-cloud-collaboration:'+scope,storage:toolSessionStorage(),
  validateRequest:request=>validCloudHistoryRequest(request,metaRef.current?.value?.source||sourceRef.current),
  validateResponse:(value,request)=>!!readCloudHistoryReceipt(value,request,metaRef.current?.value?.source||sourceRef.current)});
 const live=()=>alive.current&&owner.current===api;
 const clearProtected=()=>{
  ++metaEpoch.current;++pageEpoch.current;metaAbort.current?.abort();pageAbort.current?.abort();pageBusy.current=false;
  metaRef.current={owner:api,loading:false,failed:false,denied:true};pageRef.current=undefined;
  setMeta(metaRef.current);setPage(undefined);setSelected(undefined);setReceipt(undefined);setPageLoading(false);setPageFailed(false);
 };
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++metaEpoch.current;++pageEpoch.current;metaAbort.current?.abort();pageAbort.current?.abort();};},[]);
 useEffect(()=>{if(action.errorStatus&&[401,403,404].includes(action.errorStatus))clearProtected();},[action.errorStatus]);
 const headers=()=>({'X-Suma-Expected-User':identity?.userID||'','X-Suma-Expected-Organization':identity?.organizationID||''});
 const readView=async()=>{
  if(!identity)return;
  const generation=++metaEpoch.current,binding=sourceRef.current,abort=new AbortController();metaAbort.current?.abort();metaAbort.current=abort;
  const pending:Meta={owner:api,value:metaRef.current?.value,loading:true,failed:false,denied:false};metaRef.current=pending;setMeta(pending);
  try{
   const raw=await api<unknown>(collaborationPath(binding),undefined,'GET',headers(),abort.signal);
   if(!live()||abort.signal.aborted||metaEpoch.current!==generation)return;
   const value=readCloudCollaboration(raw,sourceRef.current);if(!value)throw Error();
   const next:Meta={owner:api,value,loading:false,failed:false,denied:false};metaRef.current=next;setMeta(next);action.clearConflict();return value;
  }catch(error){
   if(!live()||abort.signal.aborted||metaEpoch.current!==generation)return;
   if(refusedRead(error)){clearProtected();return;}
   const next:Meta={owner:api,value:metaRef.current?.value,loading:false,failed:true,denied:false};metaRef.current=next;setMeta(next);
  }
 };
 useEffect(()=>{void readView();},[api,source.submission_version,revision]);

 const candidateLoads=[view?.latest_load,view?.last_loaded,receipt,pageRef.current?.value.load];
 const selectedID=selected||view?.last_loaded?.id||view?.latest_load?.id||'';
 const selectedLoad=candidateLoads.find(load=>load?.id===selectedID);
 selectedRef.current=selectedID;
 const readPage=async(loadID:string,more=false)=>{
  if(!identity||!loadID||more&&pageBusy.current)return;
  const prior=pageRef.current?.value.load.id===loadID?pageRef.current.value:undefined;
  if(more&&(!prior||!prior.has_more))return;
  const after=more?prior!.next_after:0,binding=metaRef.current?.value?.source||sourceRef.current;
  const generation=++pageEpoch.current,abort=new AbortController();pageAbort.current?.abort();pageAbort.current=abort;pageBusy.current=true;setPageLoading(true);setPageFailed(false);
  try{
   const raw=await api<unknown>(historyPagePath(loadID,binding.project_id,after,50),undefined,'GET',headers(),abort.signal);
   if(!live()||abort.signal.aborted||pageEpoch.current!==generation||selectedRef.current!==loadID)return;
   const value=readCloudHistoryPage(raw,metaRef.current?.value?.source||binding,loadID,after,50);if(!value)throw Error();
   const next:Page={owner:api,value:more&&prior?{...value,items:[...prior.items,...value.items]}:value};pageRef.current=next;setPage(next);
  }catch(error){
   if(!live()||abort.signal.aborted||pageEpoch.current!==generation||selectedRef.current!==loadID)return;
   if(refusedRead(error)){clearProtected();return;}
   setPageFailed(true);
  }finally{if(live()&&pageEpoch.current===generation){pageBusy.current=false;setPageLoading(false);}}
 };
 useEffect(()=>{if(selectedID)void readPage(selectedID);},[api,selectedID,selectedLoad?.version,selectedLoad?.entry_count]);
 const reload=async()=>{const value=await readView();if(value&&selectedRef.current)await readPage(selectedRef.current);};
 const load=async(recover=false)=>{
  const basis=metaRef.current?.value;
  if(commandBusy.current||!basis||metaRef.current?.denied||metaRef.current?.loading||metaRef.current?.failed)return;
  if(!recover&&(disabled||action.pending||action.persistenceBlocked||!basis.allowed_actions.includes('load_history')))return;
  const original=recover?action.pending:undefined;if(recover&&!original)return;
  commandBusy.current=true;
  try{
   const result=await action.run<CloudHistoryLoad>(original?.path||historyLoadPath(basis.source),original?.body||{project_id:basis.source.project_id,expected_submission_version:basis.source.submission_version,expected_version:basis.version},original?.method||'POST');
   if(!live())return;
   if(result){setReceipt(result);setSelected(result.id);await readView();}
  }finally{commandBusy.current=false;}
 };
 const visiblePage=pageRef.current?.value.load.id===selectedID?pageRef.current.value:undefined;
 // Overdue may change without a load revision; prefer the fresh head at equal versions.
 const shownLoad=visiblePage&&(!selectedLoad||visiblePage.load.version>selectedLoad.version)?visiblePage.load:selectedLoad||visiblePage?.load,latest=view?.latest_load;
 const canLoad=!!view?.allowed_actions.includes('load_history')&&!disabled&&!current?.loading&&!current?.failed&&!action.busy&&!action.conflict&&!action.pending&&!action.persistenceBlocked&&view.version<2147483647&&currentSource.submission_version<2147483647;
 const state=(value:CloudHistoryLoad)=>t(value.overdue?'state_overdue':'state_'+value.state);
 const loadSummary=(value:CloudHistoryLoad)=> <div className="cloud-collaboration-attempt"><strong>{state(value)}</strong><span>{t('started')} <time dateTime={value.started_at}>{date(value.started_at)}</time></span>{value.finished_at&&<span>{t('finished')} <time dateTime={value.finished_at}>{date(value.finished_at)}</time></span>}{value.error_code&&<p>{t('error_'+value.error_code)}</p>}{value.overdue&&<p>{t('overdue_hint')}</p>}</div>;

 return <section className="cloud-collaboration" aria-labelledby={id+'-title'} aria-busy={!!current?.loading||action.busy} data-cloud-collaboration>
  <header><MessagesSquare size={19} aria-hidden="true"/><div><h3 id={id+'-title'}>{t('title')}</h3><p>{t('intro')}</p></div></header>
  {!identity?<p role="status">{t('unavailable')}</p>:current?.denied?<div className="cloud-collaboration-notice" role="alert"><strong>{t('denied')}</strong><p>{t('denied_hint')}</p><button type="button" onClick={()=>void reload()} data-collaboration-action="read">{t('read_saved')}</button></div>:<>
   {current?.loading&&!view&&<p role="status">{t('reading')}</p>}
   {current?.failed&&<p className="cloud-collaboration-notice" role="alert">{t('read_failed')}</p>}
   <div className="cloud-collaboration-actions"><button type="button" onClick={()=>void reload()} disabled={!!current?.loading} data-collaboration-action="read"><RefreshCw size={15} aria-hidden="true"/>{t(current?.loading?'reading':'read_saved')}</button>{view?.allowed_actions.includes('load_history')&&<button type="button" onClick={()=>void load()} disabled={!canLoad} data-collaboration-action="load">{t(action.busy?'connecting':'connect')}</button>}</div>
   <p className="cloud-collaboration-hint">{t('coverage')}</p>
   {action.pending?<div className="cloud-collaboration-notice" role="status"><strong>{t('original_pending')}</strong><p>{t('original_hint')}</p><button type="button" data-collaboration-action="recover" disabled={action.busy||!view||!!current?.loading||!!current?.failed} onClick={()=>void load(true)}>{t('recover')}</button></div>:action.conflict?<p role="alert">{t('conflict')}</p>:action.error?<p role="alert">{t('command_failed')}</p>:null}
   {view&&!latest&&!view.last_loaded&&<p className="cloud-collaboration-empty">{t('not_loaded')}</p>}
   {latest&&<div className="cloud-collaboration-latest"><h4>{t('latest_attempt')}</h4>{loadSummary(latest)}</div>}
   {view?.last_loaded&&latest?.id!==view.last_loaded.id&&<div className="cloud-collaboration-previous"><p>{t('previous_retained')} <time dateTime={view.last_loaded.finished_at||view.last_loaded.started_at}>{date(view.last_loaded.finished_at||view.last_loaded.started_at)}</time></p><div className="cloud-collaboration-actions"><button type="button" disabled={selectedID===view.last_loaded.id} onClick={()=>setSelected(view.last_loaded!.id)} data-collaboration-action="previous">{t('show_previous')}</button>{latest&&<button type="button" disabled={selectedID===latest.id} onClick={()=>setSelected(latest.id)} data-collaboration-action="latest">{t('show_latest')}</button>}</div></div>}
   {shownLoad&&<div className="cloud-collaboration-history" aria-labelledby={id+'-history'}><h4 id={id+'-history'}>{t('saved_observation')}</h4><p className="cloud-collaboration-hint">{state(shownLoad)} · <time dateTime={shownLoad.finished_at||shownLoad.started_at}>{date(shownLoad.finished_at||shownLoad.started_at)}</time></p>
    {pageLoading&&<p role="status">{t('reading_entries')}</p>}{pageFailed&&<p role="alert">{t('page_failed')}</p>}
    {visiblePage&&visiblePage.items.length===0&&!pageLoading&&<p>{t(shownLoad.state==='loaded'?'empty_load':'no_saved_entries')}</p>}
    {visiblePage&&<><ol className="cloud-collaboration-entries" role="list">{visiblePage.items.map(entry=><li key={entry.sequence} value={entry.sequence}><span className="cloud-collaboration-kind">{t('kind_'+entry.kind)}</span>{entry.kind==='unsupported'?<p>{t('unsupported')}</p>:<pre tabIndex={entry.text&&entry.text.length>1000?0:undefined} aria-label={t('entry_text')}>{entry.text||t('empty_text')}</pre>}</li>)}</ol><div className="cloud-collaboration-paging"><span>{t('stored_count',{shown:formatNumber(visiblePage.items.length),total:formatNumber(visiblePage.total)})}</span>{visiblePage.has_more&&<button type="button" disabled={pageLoading} onClick={()=>void readPage(selectedID,true)} data-collaboration-action="more">{t('more_saved')}</button>}</div></>}
    <details className="cloud-collaboration-source"><summary><FileText size={15} aria-hidden="true"/>{t('technical_source')}</summary><dl><div><dt>{t('load_id')}</dt><dd><code>{shownLoad.id}</code></dd></div><div><dt>{t('task_id')}</dt><dd><code>{shownLoad.source.task_id}</code></dd></div><div><dt>{t('deadline')}</dt><dd><time dateTime={shownLoad.deadline_at}>{date(shownLoad.deadline_at)}</time></dd></div><div><dt>{t('source_version')}</dt><dd>{formatNumber(shownLoad.source.submission_version)}</dd></div></dl></details>
   </div>}
  </>}
 </section>;
}
