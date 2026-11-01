import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ArrowUpRight,BookOpen,FileCheck,Layers,RefreshCw,Search} from 'lucide-react';
import type {ProjectKnowledgeDiscovery,ProjectKnowledgeDiscoveryHit,ProjectKnowledgeIndexHit,ProjectKnowledgeIndexSearch} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice} from './ui';
import {knowledgeDiscoveryPath,readProjectKnowledgeDiscovery,type KnowledgeDiscoveryKind} from './knowledge-discovery-model';
import {knowledgeIndexSearchPath,readProjectKnowledgeIndexSearch} from './knowledge-index-search-model';
import {KnowledgeIndexStatusPanel} from './KnowledgeIndexStatusPanel';
import './PublishedKnowledgeDiscovery.css';
export type PublishedKnowledgeDiscoveryHit=ProjectKnowledgeDiscoveryHit|ProjectKnowledgeIndexHit;
export type PublishedKnowledgeDiscoveryProps={project:string;disabled?:boolean;accessBlocked?:boolean;refresh?:number|string;onPreview:(hit:PublishedKnowledgeDiscoveryHit)=>void;onAccessDenied?:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('knowledge-discovery.'+key,values);
type SearchMode='keyword'|'index';
type Criteria={query:string;kind:KnowledgeDiscoveryKind;mode:SearchMode;revision:number};
type Snapshot={key:string;tag:string;items:PublishedKnowledgeDiscoveryHit[];page?:ProjectKnowledgeDiscovery;index?:ProjectKnowledgeIndexSearch;loading:boolean;stale?:boolean;denied?:boolean;error?:unknown};
const pinKey=(hit:PublishedKnowledgeDiscoveryHit)=>JSON.stringify(hit.reference);
export function PublishedKnowledgeDiscovery(props:PublishedKnowledgeDiscoveryProps){
 useLocale();const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 if(!view||!props.project)return <p role="status">{t('view_required')}</p>;
 return <DiscoveryScope key={JSON.stringify([view.userID,view.organizationID,props.project,owner.current.epoch])} {...props} userID={view.userID} organizationID={view.organizationID}/>;
}
function DiscoveryScope({project,disabled=false,accessBlocked=false,refresh=0,onPreview,onAccessDenied,userID,organizationID}:PublishedKnowledgeDiscoveryProps&{userID:string;organizationID:string}){
 useLocale();const api=useApi(),id=useId(),[query,setQuery]=useState(''),[kind,setKind]=useState<KnowledgeDiscoveryKind>(''),[mode,setMode]=useState<SearchMode>('keyword'),[criteria,setCriteria]=useState<Criteria>({query:'',kind:'',mode:'keyword',revision:0}),[validation,setValidation]=useState<unknown>(),[previewed,setPreviewed]=useState<{tag:string;pin:string}>();
 const key=JSON.stringify([criteria.query,criteria.kind,criteria.mode]),tag=JSON.stringify([key,criteria.revision,refresh]);
 const [snapshot,setSnapshot]=useState<Snapshot>({key,tag,items:[],loading:false}),latest=useRef(snapshot),source=useRef(tag),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),inflight=useRef(false),alive=useRef(true),refused=useRef(false),notified=useRef(false);
 const moreButton=useRef<HTMLButtonElement>(null),resultStatus=useRef<HTMLParagraphElement>(null),previewButtons=useRef(new Map<string,HTMLButtonElement>()),focusAfterMore=useRef<{tag:string;id?:string}|undefined>(undefined);
 const control=useRef({disabled,accessBlocked}),callbacks=useRef({onPreview,onAccessDenied});control.current={disabled,accessBlocked};callbacks.current={onPreview,onAccessDenied};source.current=tag;
 const dirty=query.trim()!==criteria.query||kind!==criteria.kind||mode!==criteria.mode;
 const store=(value:Snapshot)=>{latest.current=value;setSnapshot(value);};
 useLayoutEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;controller.current?.abort();inflight.current=false;};},[]);
 useLayoutEffect(()=>{const pending=focusAfterMore.current;if(!pending||pending.tag!==tag)return;focusAfterMore.current=undefined;if(control.current.accessBlocked||document.activeElement!==document.body&&document.activeElement!==moreButton.current)return;const target=pending.id?previewButtons.current.get(pending.id):resultStatus.current;target?.focus({preventScroll:true});},[snapshot,tag]);
 useLayoutEffect(()=>{if(!accessBlocked)return;refused.current=true;++epoch.current;controller.current?.abort();inflight.current=false;setPreviewed(undefined);store({key,tag,items:[],loading:false,denied:true});},[accessBlocked]);
 const read=useCallback(async(more=false)=>{
  if(!alive.current||inflight.current||control.current.disabled||control.current.accessBlocked||source.current!==tag)return;
  const before=latest.current;if(more&&(criteria.mode!=='keyword'||before.key!==key||before.tag!==tag||before.loading||before.stale||before.error||before.denied||!before.page?.next_cursor))return;
  const previous=before.key===key&&!refused.current?before.items:[],cursor=more?before.page!.next_cursor:undefined,ticket=++epoch.current,abort=new AbortController(),keepFocus=more&&document.activeElement===moreButton.current;controller.current?.abort();controller.current=abort;inflight.current=true;
  const previousMeta:Pick<Snapshot,'page'|'index'>=before.key===key&&!refused.current?{page:before.page,index:before.index}:{},hasPrevious=!!previous.length||!!previousMeta.page||!!previousMeta.index;
  store({key,tag,items:previous,...previousMeta,loading:true,stale:hasPrevious,denied:refused.current});
  const owns=()=>alive.current&&epoch.current===ticket&&source.current===tag&&!abort.signal.aborted&&!control.current.accessBlocked;
  try{
   const scope={project,query:criteria.query,kind:criteria.kind,limit:10},path=criteria.mode==='keyword'?knowledgeDiscoveryPath({...scope,cursor}):knowledgeIndexSearchPath(scope),raw=await api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);
   if(!owns())return;
   if(criteria.mode==='index'){const index=readProjectKnowledgeIndexSearch(raw,scope);refused.current=false;notified.current=false;store({key,tag,items:index.items,index,loading:false});return;}
   const page=readProjectKnowledgeDiscovery(raw,scope);
   if(more&&page.next_cursor===cursor)throw new UiError('knowledge-discovery.invalid_response');
   const rows=more?[...previous,...page.items]:page.items;if(new Set(rows.map(hit=>hit.reference.id)).size!==rows.length)throw new UiError('knowledge-discovery.invalid_response');
   refused.current=false;notified.current=false;if(keepFocus&&page.exhausted)focusAfterMore.current={tag,id:page.items[0]?.reference.id};store({key,tag,items:rows,page,loading:false});
  }catch(error){
   if(!owns())return;
   if(error instanceof ApiError&&[401,403,404].includes(error.status)){
    refused.current=true;setPreviewed(undefined);store({key,tag,items:[],loading:false,denied:true});if(!notified.current){notified.current=true;callbacks.current.onAccessDenied?.();}
   }else store({key,tag,items:previous,...previousMeta,loading:false,stale:hasPrevious,denied:refused.current,error});
  }finally{if(owns())inflight.current=false;}
 },[api,project,criteria.query,criteria.kind,criteria.mode,key,tag,userID,organizationID]);
 useEffect(()=>{
  if(!disabled&&!accessBlocked)void read();
  else if(disabled&&latest.current.loading)store({...latest.current,loading:false,stale:!!latest.current.items.length||!!latest.current.page||!!latest.current.index});
  return()=>{++epoch.current;controller.current?.abort();inflight.current=false;};
 },[read,disabled,accessBlocked]);
 function search(){
  if(disabled||accessBlocked||inflight.current||!alive.current)return;
  try{(mode==='keyword'?knowledgeDiscoveryPath:knowledgeIndexSearchPath)({project,query,kind,limit:10});setValidation(undefined);setPreviewed(undefined);setCriteria(current=>({query:query.trim(),kind,mode,revision:current.revision+1}));}catch(error){setValidation(error);}
 }
 const current=snapshot.key===key?snapshot:undefined,hidden=accessBlocked||refused.current||!!current?.denied,rows=!hidden&&!dirty?current?.items??[]:[],page=!hidden&&!dirty?current?.page:undefined,index=!hidden&&!dirty?current?.index:undefined,loading=!!current?.loading,stale=!!current&&(current.stale||current.tag!==tag),blocked=disabled||accessBlocked||hidden||dirty||loading||stale||!!current?.error;
 function preview(hit:PublishedKnowledgeDiscoveryHit){
  const now=latest.current;if(blocked||!alive.current||source.current!==tag||now.tag!==tag||now.loading||now.stale||now.denied||now.error)return;
  const exact=now.items.find(value=>pinKey(value)===pinKey(hit));if(!exact)return;setPreviewed({tag,pin:pinKey(exact)});callbacks.current.onPreview(exact);
 }
 return <section className="published-knowledge-discovery" data-knowledge-discovery aria-labelledby={id+'-title'} aria-busy={loading}>
  <header className="knowledge-discovery-heading"><div><Search size={19} aria-hidden="true"/><h3 id={id+'-title'}>{t('title')}</h3></div><p>{t('intro')}</p></header>
  <div role="search" aria-labelledby={id+'-title'} className="knowledge-discovery-search">
   <label className="knowledge-discovery-query" htmlFor={id+'-query'}><span>{t('query')}</span><input id={id+'-query'} data-knowledge-discovery-query value={query} disabled={disabled||accessBlocked} maxLength={200} aria-describedby={id+'-hint'} onChange={event=>{setQuery(event.target.value);setValidation(undefined);setPreviewed(undefined);}} onKeyDown={event=>{if(event.key==='Enter'){event.preventDefault();event.stopPropagation();search();}}}/></label>
   <label htmlFor={id+'-mode'}><span>{t('mode')}</span><select id={id+'-mode'} data-knowledge-discovery-mode value={mode} disabled={disabled||accessBlocked} onChange={event=>{setMode(event.target.value as SearchMode);setValidation(undefined);setPreviewed(undefined);}}><option value="keyword">{t('mode.keyword')}</option><option value="index">{t('mode.index')}</option></select></label>
   <label htmlFor={id+'-kind'}><span>{t('kind')}</span><select id={id+'-kind'} data-knowledge-discovery-kind value={kind} disabled={disabled||accessBlocked} onChange={event=>{setKind(event.target.value as KnowledgeDiscoveryKind);setPreviewed(undefined);}}><option value="">{t('kind.all')}</option>{(['memory','spec','skill'] as const).map(value=><option key={value} value={value}>{t('kind.'+value)}</option>)}</select></label>
   <button type="button" data-knowledge-discovery-search disabled={disabled||accessBlocked} aria-disabled={disabled||accessBlocked||loading} aria-busy={loading} onClick={search}><Search size={16} aria-hidden="true"/>{t(loading?'searching':'search')}</button>
   <button type="button" className="text-button" data-knowledge-discovery-refresh disabled={disabled||accessBlocked||dirty} aria-disabled={disabled||accessBlocked||dirty||loading} aria-busy={loading} onClick={()=>{if(!disabled&&!accessBlocked&&!dirty&&!inflight.current){setValidation(undefined);setPreviewed(undefined);setCriteria(current=>({...current,revision:current.revision+1}));}}}><RefreshCw size={16} aria-hidden="true"/>{t('refresh')}</button>
  </div>
  <p id={id+'-hint'} className="knowledge-discovery-meta">{t(mode==='keyword'?'query_hint':'index_query_hint')}</p>
  {validation!==undefined&&<ErrorNotice error={validation}/>}
  {hidden?<p role="alert" className="knowledge-discovery-notice">{t('access_denied')}</p>:<>
   {dirty?<p role="status" className="knowledge-discovery-notice">{t('dirty')}</p>:<>
    {loading&&<p role="status" className="knowledge-discovery-meta">{t('loading')}</p>}
    {stale&&(rows.length>0||!!page||!!index)&&<p role="status" className="knowledge-discovery-notice" data-knowledge-discovery-stale>{t('stale')}</p>}
    {current?.error!==undefined&&<ErrorNotice error={current.error}/>}
    {!!rows.length&&<ul className="knowledge-discovery-results">{rows.map(hit=>{const Icon=hit.kind==='memory'?BookOpen:hit.kind==='spec'?FileCheck:Layers;return <li key={pinKey(hit)} data-knowledge-discovery-hit={hit.reference.id} data-selected={previewed?.tag===tag&&previewed.pin===pinKey(hit)||undefined}>
     <div className="knowledge-discovery-row-heading"><span className="knowledge-discovery-type"><Icon size={16} aria-hidden="true"/>{t('kind.'+hit.kind)}</span><span className="knowledge-discovery-meta">{t('revision',{revision:formatNumber(hit.reference.revision)})}</span></div>
     <h4>{hit.title}</h4>{hit.summary&&<p>{hit.summary}</p>}{hit.snippet&&<p className="knowledge-discovery-snippet">{hit.snippet}</p>}
     <div className="knowledge-discovery-row-meta"><span>{'matched_field' in hit?t('matched.'+hit.matched_field):t('index_match')}</span><span>{t('sources',{count:formatNumber(hit.source_count)})}</span><time dateTime={hit.published_at} title={hit.published_at}>{t('published_at')} · {formatDateTime(hit.published_at,{dateStyle:'short',timeStyle:'short'})}</time></div>
     <button ref={element=>{if(element)previewButtons.current.set(hit.reference.id,element);else previewButtons.current.delete(hit.reference.id);}} type="button" className="text-button knowledge-discovery-preview" data-knowledge-discovery-preview={hit.reference.id} disabled={blocked} aria-pressed={previewed?.tag===tag&&previewed.pin===pinKey(hit)} onClick={()=>preview(hit)}>{t('preview')}<ArrowUpRight size={16} aria-hidden="true"/></button>
    </li>;})}</ul>}
    {page&&<>
     <p ref={resultStatus} tabIndex={-1} role="status" className="knowledge-discovery-meta">{t('scanned',{scanned:formatNumber(page.scanned),limit:formatNumber(page.scan_limit),count:formatNumber(rows.length)})}</p>
     {!loading&&!current?.error&&(!rows.length&&!page.exhausted?<p role="status" className="knowledge-discovery-notice">{t('continuing')}</p>:!rows.length&&page.exhausted?<div className="knowledge-discovery-empty" role="status"><strong>{t('empty')}</strong><p>{t('empty_hint')}</p></div>:page.exhausted&&<p className="knowledge-discovery-meta">{t('exhausted')}</p>)}
     {!page.exhausted&&<button ref={moreButton} type="button" data-knowledge-discovery-more disabled={disabled||accessBlocked||dirty||hidden||!!current?.error||!!stale&&!loading} aria-disabled={blocked} aria-busy={loading} onClick={()=>void read(true)}>{t('more')}</button>}
    </>}
    {index&&<div className="knowledge-discovery-index-result" data-knowledge-discovery-index>
     <p role="status">{t('index_mode.'+index.mode)}</p>
     <p className="knowledge-discovery-meta">{t('index_candidates',{examined:formatNumber(index.candidates_examined),discarded:formatNumber(index.discarded),count:formatNumber(rows.length)})}</p>
     <p className="knowledge-discovery-meta">{t('index_generation',{generation:index.generation,watermark:formatNumber(index.index_watermark)})}</p>
     <p className="knowledge-discovery-meta">{t('index_coverage')}</p>
     {!loading&&!current?.error&&!rows.length&&<div className="knowledge-discovery-empty" role="status"><strong>{t('index_empty')}</strong><p>{t('index_empty_hint')}</p></div>}
    </div>}
   </>}
  </>}
  <p className="knowledge-discovery-boundary">{t('boundary')}</p>
  <KnowledgeIndexStatusPanel project={project} disabled={disabled} accessBlocked={accessBlocked||hidden} refresh={refresh} onAccessDenied={()=>{if(!alive.current)return;refused.current=true;++epoch.current;controller.current?.abort();inflight.current=false;setPreviewed(undefined);store({key,tag,items:[],loading:false,denied:true});if(!notified.current){notified.current=true;callbacks.current.onAccessDenied?.();}}}/>
 </section>;
}
