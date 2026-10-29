import {useEffect,useRef,useState} from 'react';
import {ArrowDownToLine,ArrowUpFromLine,ChevronRight,RefreshCw,Search} from 'lucide-react';
import {useApi,useApiView} from './api-context';
import {type ApiCaller} from './api';
import {tr,useLocale,UiError} from './i18n';
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
type Snapshot={context:string;caller:ApiCaller;revision:number;page?:AdapterCatalogPage;error?:unknown};
export function AdapterConnections({project,onSetup}:{project:string;onSetup:(descriptor:AdapterDescriptor)=>void}){
 useLocale();const caller=useApi(),view=useApiView(),context=JSON.stringify([project,view?.userID,view?.organizationID]);
 const [revision,setRevision]=useState(0),[snapshot,setSnapshot]=useState<Snapshot>(),[direction,setDirection]=useState<'suma_to_tool'|'tool_to_suma'>('suma_to_tool'),[search,setSearch]=useState(''),[selected,setSelected]=useState('');const epoch=useRef(0),detail=useRef<HTMLElement|null>(null);
 useEffect(()=>{const generation=++epoch.current,controller=new AbortController();setSnapshot(undefined);setSelected('');if(project)void caller<unknown>('/adapter-catalog?project_id='+encodeURIComponent(project),undefined,'GET',undefined,controller.signal).then(value=>{if(generation!==epoch.current)return;const page=readAdapterCatalog(value,project);setSnapshot({context,caller,revision,...(page?{page}:{error:new UiError('adapter-connections.invalid_response')})});}).catch(error=>{if(generation===epoch.current&&!controller.signal.aborted)setSnapshot({context,caller,revision,error});});return()=>{++epoch.current;controller.abort();};},[context,caller,revision,project]);
 const current=snapshot?.context===context&&snapshot.caller===caller&&snapshot.revision===revision?snapshot:undefined,page=current?.page,rows=page?.items.filter(row=>row.direction===direction&&(!search.trim()||(row.product+' '+row.entrypoint_kind+' '+(row.protocol||'')).toLowerCase().includes(search.trim().toLowerCase())))||[],active=rows.find(row=>row.id===selected);
 const choose=(row:AdapterDescriptor)=>{setSelected(row.id);if(typeof matchMedia==='function'&&matchMedia('(max-width:767px)').matches)requestAnimationFrame(()=>detail.current?.scrollIntoView({block:'start',behavior:'auto'}));};
 if(!project)return null;
 return <section className="adapter-connections" aria-label={tr('adapter-connections.title')}>
  <header className="adapter-connections-heading"><div><h3>{tr('adapter-connections.title')}</h3><p>{tr('adapter-connections.intro')}</p></div><button type="button" disabled={!current} onClick={()=>setRevision(n=>n+1)}><RefreshCw size={16}/>{tr('adapter-connections.refresh')}</button></header>
  <div className="adapter-connections-directions" role="group" aria-label={tr('adapter-connections.direction')}>{(['suma_to_tool','tool_to_suma'] as const).map(key=><button type="button" key={key} aria-pressed={direction===key} onClick={()=>{setDirection(key);setSelected('');}}>{key==='suma_to_tool'?<ArrowUpFromLine size={18}/>:<ArrowDownToLine size={18}/>}<span><strong>{tr('adapter-connections.direction.'+key)}</strong><small>{tr('adapter-connections.direction_hint.'+key)}</small></span></button>)}</div>
  {!current?<Loading/>:current.error?<ErrorNotice error={current.error} onRetry={()=>setRevision(n=>n+1)}/>:<>
   <label className="adapter-connections-search"><Search size={16}/><span className="adapter-connections-search-label">{tr('adapter-connections.search')}</span><input value={search} maxLength={100} onChange={event=>{setSearch(event.target.value);setSelected('');}} placeholder={tr('adapter-connections.search_placeholder')}/></label>
   <p className="adapter-connections-boundary">{tr('adapter-connections.boundary')}</p>
   <div className={'adapter-connections-workspace'+(active?' has-detail':'')}><div className="adapter-connections-grid">{rows.map(row=><button type="button" key={row.id} className={'adapter-connection-card '+(active?.id===row.id?'selected':'')} aria-pressed={active?.id===row.id} data-adapter-id={row.id} onClick={()=>choose(row)}><span className="adapter-connection-card-top"><strong>{row.product}</strong><ChevronRight size={17}/></span><span>{message('entrypoint',row.entrypoint_kind)}</span><span className={'adapter-connection-state '+row.implementation}>{tr('adapter-connections.implementation.'+row.implementation)}</span></button>)}{!rows.length&&<Empty title={tr('adapter-connections.no_matches')} description={tr('adapter-connections.no_matches_hint')}/>}</div>
   {active&&<aside ref={detail} className="adapter-connection-detail" aria-label={tr('adapter-connections.details')}><header><div><h4>{active.product}</h4><p>{message('entrypoint',active.entrypoint_kind)}</p></div><button type="button" onClick={()=>setSelected('')}>{tr('adapter-connections.close')}</button></header><p className={'adapter-connection-state '+active.implementation}>{tr('adapter-connections.implementation.'+active.implementation)}</p>
    {active.implementation!=='planned'?<button type="button" className="primary adapter-connection-next" onClick={()=>onSetup(active)}>{tr('adapter-connections.action.'+active.setup_kind)}<ChevronRight size={16}/></button>:<p className="adapter-connections-boundary">{tr('adapter-connections.planned_hint')}</p>}
    <ul className="adapter-connection-limits">{active.limitations.map((key,index)=><li key={key+index}>{message('limit',key)}</li>)}</ul>
    <details className="adapter-connection-capabilities"><summary>{tr('adapter-connections.capabilities')}</summary><p>{tr('adapter-connections.capabilities_hint')}</p><dl>{active.capabilities.map(cap=><div key={cap.key}><dt>{message('capability',cap.key)}</dt><dd><span className={'adapter-capability-'+cap.support}>{tr('adapter-connections.support.'+cap.support)}</span>{cap.limitations.map((key,index)=><small key={key+index}>{message('limit',key)}</small>)}</dd></div>)}</dl></details>
    <details className="adapter-connection-sources"><summary>{tr('adapter-connections.sources')}</summary>{active.protocol&&<p>{tr('adapter-connections.protocol')}: <code>{active.protocol}</code></p>}<ul>{active.source_refs.map((source,index)=><li key={source+index}><code>{source}</code></li>)}</ul></details>
   </aside>}
   </div>
  </>}
 </section>;
}
