import {useEffect,useId,useLayoutEffect,useMemo,useRef,useState} from 'react';
import {ArrowDownToLine,ArrowUpFromLine,ChevronRight,Layers3,RefreshCw,Search} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {type ApiCaller} from './api';
import {formatNumber,tr,useLocale,UiError} from './i18n';
import {Empty,ErrorNotice,Loading} from './ui';
import './AdapterConnections.css';

export type AdapterCapability={key:string;support:'supported'|'unsupported'|'unverified';source_refs:string[];limitations:string[]};
export type AdapterDescriptor={id:string;product:string;entrypoint_kind:string;direction:'suma_to_tool'|'tool_to_suma';adapter?:string;protocol?:string;implementation:'implemented'|'guide_only'|'planned';setup_kind:'runner_profile'|'client_export'|'manual_guide';capabilities:AdapterCapability[];source_refs:string[];limitations:string[]};
export type AdapterCatalogPage={project_id:string;as_of:string;items:AdapterDescriptor[]};
const knownDirections=['suma_to_tool','tool_to_suma'],knownStates=['implemented','guide_only','planned'],knownSetup=['runner_profile','client_export','manual_guide'];
const strings=(v:unknown):v is string[]=>Array.isArray(v)&&v.length<=100&&v.every(x=>typeof x==='string'&&x.length<=1000);
/** Reject malformed routing data rather than offering an invented setup action. */
export function readAdapterCatalog(value:unknown,project:string):AdapterCatalogPage|undefined{
 if(!value||typeof value!=='object')return;const page=value as AdapterCatalogPage;if(page.project_id!==project||typeof page.as_of!=='string'||!Number.isFinite(Date.parse(page.as_of))||!Array.isArray(page.items)||page.items.length>64)return;
 const ids=new Set<string>();for(const row of page.items){if(!row||typeof row.id!=='string'||!row.id||row.id.length>100||ids.has(row.id)||typeof row.product!=='string'||!row.product||row.product.length>100||typeof row.entrypoint_kind!=='string'||!knownDirections.includes(row.direction)||!knownStates.includes(row.implementation)||!knownSetup.includes(row.setup_kind)||!strings(row.source_refs)||!strings(row.limitations)||!Array.isArray(row.capabilities)||row.capabilities.length>100)return;if(row.direction==='tool_to_suma'&&row.setup_kind==='runner_profile'||row.direction==='suma_to_tool'&&row.setup_kind==='client_export'||row.implementation==='guide_only'&&row.setup_kind!=='manual_guide')return;ids.add(row.id);if(row.adapter!==undefined&&typeof row.adapter!=='string'||row.protocol!==undefined&&typeof row.protocol!=='string')return;const caps=new Set<string>();for(const cap of row.capabilities){if(!cap||typeof cap.key!=='string'||!cap.key||caps.has(cap.key)||!['supported','unsupported','unverified'].includes(cap.support)||!strings(cap.source_refs)||!strings(cap.limitations))return;caps.add(cap.key);}}
 return page;
}
const message=(category:string,key:string)=>{const id='adapter-connections.'+category+'.'+key,value=tr(id);return value===id?tr('adapter-connections.unknown_detail',{value:key}):value;};
const directionChoices=[
 {key:'all',Icon:Layers3},
 {key:'suma_to_tool',Icon:ArrowUpFromLine},
 {key:'tool_to_suma',Icon:ArrowDownToLine},
] as const;

type DirectionFilter=typeof directionChoices[number]['key'];
type CatalogScope={project:string;caller:ApiCaller;userID?:string;organizationID?:string;revision:number};
type Snapshot={scope:CatalogScope;page?:AdapterCatalogPage;error?:unknown};

function matchesSearch(row:AdapterDescriptor,terms:string[]){
 if(!terms.length)return true;
 const searchable=[
  row.id,row.product,row.entrypoint_kind,row.adapter,row.protocol,
  message('entrypoint',row.entrypoint_kind),
  tr('adapter-connections.direction.'+row.direction),
 ].join(' ').toLowerCase();
 return terms.every(term=>searchable.includes(term));
}

export function AdapterConnections({project,onSetup}:{project:string;onSetup:(descriptor:AdapterDescriptor)=>void}){
 useLocale();
 const caller=useApi(),view=useApiView(),userID=view?.userID,organizationID=view?.organizationID;
 const [revision,setRevision]=useState(0);
 const scope=useMemo(()=>({project,caller,userID,organizationID,revision}),[project,caller,userID,organizationID,revision]);
 const [snapshot,setSnapshot]=useState<Snapshot>();
 const [direction,setDirection]=useState<DirectionFilter>('all');
 const [search,setSearch]=useState('');
 const [selection,setSelection]=useState<{scope:CatalogScope;id:string}>();
 const searchInput=useRef<HTMLInputElement>(null);
 const selectedCard=useRef<HTMLButtonElement>(null);
 const detail=useRef<HTMLElement>(null),detailHeading=useRef<HTMLHeadingElement>(null),detailId=useId();

 useEffect(()=>{
  const controller=new AbortController();
  setSnapshot(undefined);
  setSelection(undefined);
  selectedCard.current=null;
  const headers=scope.userID&&scope.organizationID?{
   'X-Suma-Expected-User':scope.userID,
   'X-Suma-Expected-Organization':scope.organizationID,
  }:undefined;
  if(scope.project){
   void scope.caller<unknown>('/adapter-catalog?project_id='+encodeURIComponent(scope.project),undefined,'GET',headers,controller.signal)
    .then(value=>{
     if(controller.signal.aborted)return;
     const page=readAdapterCatalog(value,scope.project);
     setSnapshot({scope,...(page?{page}:{error:new UiError('adapter-connections.invalid_response')})});
    }).catch(error=>{
     if(!controller.signal.aborted)setSnapshot({scope,error});
    });
  }
  return()=>controller.abort();
 },[scope]);

 const current=snapshot?.scope===scope?snapshot:undefined;
 const terms=search.toLowerCase().split(/\s+/u).filter(Boolean);
 const rows=current?.page?.items.filter(row=>(direction==='all'||row.direction===direction)&&matchesSearch(row,terms))??[];
 const active=selection?.scope===scope?rows.find(row=>row.id===selection.id):undefined;

 useLayoutEffect(()=>{
  if(!active)return;
  const stacked=typeof matchMedia==='function'&&matchMedia('(max-width:1100px)').matches;
  detailHeading.current?.focus({preventScroll:stacked});
  if(stacked)detail.current?.scrollIntoView?.({block:'start'});
 },[active,selection]);

 function clearSelection(){
  setSelection(undefined);
  selectedCard.current=null;
 }
 function updateSearch(value:string){
  setSearch(value);
  clearSelection();
 }
 function closeDetails(){
  const trigger=selectedCard.current;
  clearSelection();
  trigger?.focus();
 }

 if(!project)return null;
 return <section className="adapter-connections" aria-label={tr('adapter-connections.title')}>
  <header className="adapter-connections-heading"><div><h3>{tr('adapter-connections.title')}</h3><p>{tr('adapter-connections.intro')}</p></div><button type="button" disabled={!current} onClick={()=>setRevision(n=>n+1)}><RefreshCw size={16} aria-hidden="true"/>{tr('adapter-connections.refresh')}</button></header>
  <div className="adapter-connections-directions" role="group" aria-label={tr('adapter-connections.direction')}>
   {directionChoices.map(({key,Icon})=><button type="button" key={key} data-adapter-direction={key} aria-pressed={direction===key} onClick={()=>{
    if(direction!==key){setDirection(key);clearSelection();}
   }}><Icon size={18} aria-hidden="true"/><span><strong>{tr('adapter-connections.direction.'+key)}</strong><small>{tr('adapter-connections.direction_hint.'+key)}</small></span></button>)}
  </div>
  {!current?<Loading/>:current.error?<ErrorNotice error={current.error} onRetry={()=>setRevision(n=>n+1)}/>:<>
   <div className="adapter-connections-search-row">
    <label className="adapter-connections-search"><Search size={16} aria-hidden="true"/><span className="adapter-connections-search-label">{tr('adapter-connections.search')}</span><input ref={searchInput} value={search} maxLength={100} onChange={event=>updateSearch(event.target.value)} placeholder={tr('adapter-connections.search_placeholder')}/></label>
    {search&&<button type="button" className="adapter-connections-clear-search" onClick={()=>{updateSearch('');searchInput.current?.focus();}}>{tr('adapter-connections.clear_search')}</button>}
   </div>
   <p className="adapter-connections-results" role="status">{tr('adapter-connections.results',{count:formatNumber(rows.length)})}</p>
   <p className="adapter-connections-boundary">{tr('adapter-connections.boundary')}</p>
   <div className={'adapter-connections-workspace'+(active?' has-detail':'')}>
    <div className="adapter-connections-grid">
     {rows.map(row=><button type="button" key={row.id} className={'adapter-connection-card '+(active?.id===row.id?'selected':'')}
      aria-pressed={active?.id===row.id} aria-controls={active?.id===row.id?detailId:undefined} data-adapter-id={row.id}
      onClick={event=>{selectedCard.current=event.currentTarget;setSelection({scope,id:row.id});}}>
      <span className="adapter-connection-card-top"><strong>{row.product}</strong><ChevronRight size={17} aria-hidden="true"/></span>
      <span className="adapter-connection-card-meta"><span>{message('entrypoint',row.entrypoint_kind)}</span><span className="adapter-connection-direction">{tr('adapter-connections.direction.'+row.direction)}</span></span>
      <span className={'adapter-connection-state '+row.implementation}>{tr('adapter-connections.implementation.'+row.implementation)}</span>
     </button>)}
     {!rows.length&&<Empty title={tr('adapter-connections.no_matches')} description={tr('adapter-connections.no_matches_hint')}/>}
    </div>
    {active&&<aside ref={detail} id={detailId} className="adapter-connection-detail" aria-label={tr('adapter-connections.details')}>
     <header><div><h4 ref={detailHeading} tabIndex={-1}>{active.product}</h4><p>{message('entrypoint',active.entrypoint_kind)}</p></div><button type="button" onClick={closeDetails}>{tr('adapter-connections.close')}</button></header>
     <p className={'adapter-connection-state '+active.implementation}>{tr('adapter-connections.implementation.'+active.implementation)}</p>
     {active.implementation!=='planned'?<button type="button" className="primary adapter-connection-next" onClick={()=>onSetup(active)}>{tr(active.id==='workbuddy-cloud'?'adapter-connections.action.workbuddy_account':'adapter-connections.action.'+active.setup_kind)}<ChevronRight size={16} aria-hidden="true"/></button>:<p className="adapter-connections-boundary">{tr('adapter-connections.planned_hint')}</p>}
     <ul className="adapter-connection-limits">{active.limitations.map((key,index)=><li key={key+index}>{message('limit',key)}</li>)}</ul>
     <details className="adapter-connection-capabilities"><summary>{tr('adapter-connections.capabilities')}</summary><p>{tr('adapter-connections.capabilities_hint')}</p><dl>{active.capabilities.map(cap=><div key={cap.key}><dt>{message('capability',cap.key)}</dt><dd><span className={'adapter-capability-'+cap.support}>{tr('adapter-connections.support.'+cap.support)}</span>{cap.limitations.map((key,index)=><small key={key+index}>{message('limit',key)}</small>)}</dd></div>)}</dl></details>
     <details className="adapter-connection-sources"><summary>{tr('adapter-connections.sources')}</summary>{active.protocol&&<p>{tr('adapter-connections.protocol')}: <code>{active.protocol}</code></p>}<ul>{active.source_refs.map((source,index)=><li key={source+index}><code>{source}</code></li>)}</ul></details>
    </aside>}
   </div>
  </>}
 </section>;
}
