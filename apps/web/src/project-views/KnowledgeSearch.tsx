import {useApi,useApiView} from '../api-context';
import {useEffect,useRef,useState,type FormEvent} from 'react';
import {Search,ArrowUpRight} from 'lucide-react';
import {sourceHref} from '../navigation';
import {knowledgeSourceLabel} from '../knowledge-model';
import {api} from '../api';
import {Empty,ErrorNotice} from '../ui';
import {formatNumber,tr,useLocale} from '../i18n';
import {PublishedKnowledgeDiscovery} from '../PublishedKnowledgeDiscovery';
import {PublishedKnowledgePreview} from '../PublishedKnowledgePreview';
import {knowledgePreviewTarget,type KnowledgePreviewTarget} from '../knowledge-preview-target';
import '../knowledge.css';
import './KnowledgeSearch.css';
type SearchResult={items:{id:string;type?:string;title?:string;snippet?:string;source_ref?:string;revision?:number;locator?:unknown}[];mode?:string;index_watermark?:number|null};
type Snapshot={project:string;query:string;results?:SearchResult;error?:unknown;busy:boolean};
type Props={project:string;onOpenSource?:(source:unknown)=>void};
export function KnowledgeSearch(props:Props){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 return <KnowledgeSearchScope key={JSON.stringify([view?.userID,view?.organizationID,props.project,owner.current.epoch])} {...props}/>;
}
function KnowledgeSearchScope({project,onOpenSource}:Props){
 useLocale();const [mode,setMode]=useState<'records'|'published'>('records'),[preview,setPreview]=useState<KnowledgePreviewTarget>(),[denied,setDenied]=useState(false),[refresh,setRefresh]=useState(0),alive=useRef(true);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 function accessDenied(){if(!alive.current)return;setPreview(undefined);setDenied(true);}
 function closePreview(){const original=preview?.reference.id;setPreview(undefined);requestAnimationFrame(()=>{if(!alive.current||!original)return;const target=[...document.querySelectorAll<HTMLButtonElement>('[data-knowledge-discovery-preview]')].find(button=>button.getAttribute('data-knowledge-discovery-preview')===original);target?.focus({preventScroll:true});});}
 return <div className="knowledge-search-workspace"><div className="knowledge-search-modes" role="group" aria-label={tr('knowledge-consumers.mode_label')}>{(['records','published'] as const).map(value=><button key={value} type="button" data-knowledge-search-mode={value} aria-pressed={mode===value} onClick={()=>setMode(value)}>{tr('knowledge-consumers.mode.'+value)}</button>)}</div>
  {denied?<section className="panel" role="alert"><p>{tr('knowledge-consumers.access_denied')}</p><button type="button" onClick={()=>{setDenied(false);setRefresh(value=>value+1);}}>{tr('knowledge-consumers.recheck_access')}</button></section>:mode==='records'?<RecordKnowledgeSearch project={project} onOpenSource={onOpenSource}/>:<div className="knowledge-published-search">
   <PublishedKnowledgeDiscovery project={project} refresh={refresh} onPreview={hit=>{const target=knowledgePreviewTarget(hit,project);if(target)setPreview(target);}} onAccessDenied={accessDenied}/>
   {preview&&<PublishedKnowledgePreview project={project} reference={preview.reference} location={preview.location} refresh={refresh} onClose={closePreview} onAccessDenied={accessDenied}/>}
  </div>}
 </div>;
}
function RecordKnowledgeSearch({project,onOpenSource}:Props){ const api=useApi();
 useLocale();const [q,setQ]=useState(''),[snapshot,setSnapshot]=useState<Snapshot>({project,query:'',busy:false});
 const current=useRef(project),generation=useRef(0);current.current=project;
 useEffect(()=>()=>{generation.current++;},[project]);
 const state=snapshot.project===project?snapshot:{project,query:'',busy:false};
 async function search(event:FormEvent){
  event.preventDefault();if(state.busy||!q.trim())return;
  const originalQuery=q,scope=project,request=++generation.current;
  setSnapshot({project:scope,query:originalQuery,busy:true});
  try{const results=await api<SearchResult>('/search',{project_id:scope,query:originalQuery,types:[]});if(current.current===scope&&generation.current===request)setSnapshot({project:scope,query:originalQuery,busy:false,results});}
  catch(error){if(current.current===scope&&generation.current===request)setSnapshot({project:scope,query:originalQuery,busy:false,error});}
 }
 const results=state.results;
 return <section className="panel knowledge-search"><div className="panel-heading"><div><h2>{tr('workbench.search.title')}</h2><p>{tr('workbench.search.description')}</p></div><Search size={22}/></div><form className="search-form" onSubmit={event=>void search(event)}><Search size={19}/><input aria-label={tr('workbench.search.query')} value={q} onChange={event=>setQ(event.target.value)} placeholder={tr('workbench.search.placeholder')} required/><button className="primary" disabled={state.busy}>{tr(state.busy?'workbench.search.searching':'workbench.search.submit')}</button></form>
  {Boolean(state.error)&&<ErrorNotice error={state.error}/>}
  {results&&<p className="loaded-scope">{tr('workbench.search.results_query',{query:state.query})}{q!==state.query&&<> · {tr('workbench.search.previous_query')}</>}</p>}
  <div className="search-results">{results?.items?.map((item,index)=>{const href=sourceHref(project,item);return <article key={item.id+':'+index}><span className="type-chip" title={item.type}>{['requirements','tasks','issues','documents'].includes(item.type||'')?knowledgeSourceLabel(item.type!):item.type}</span><h3>{item.title}</h3><p>{item.snippet}</p><small>{tr('workbench.search.result_reference',{source:item.source_ref||item.id||tr('workbench.not_reported'),revision:typeof item.revision==='number'?formatNumber(item.revision):tr('workbench.not_reported'),locator:typeof item.locator==='object'?JSON.stringify(item.locator):String(item.locator??'')})}</small><div className="knowledge-result-actions">{href?<a href={href} onClick={event=>{if(!onOpenSource||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;event.preventDefault();onOpenSource(item);}}>{tr('knowledge.source_open')}<ArrowUpRight size={13}/></a>:<small className="muted">{tr('knowledge.source_outside')}</small>}</div></article>;})}</div>
  {results&&!results.items?.length&&<Empty title={tr('workbench.search.empty')} description={tr('workbench.search.empty_hint')}/>}
  {results&&<p className="muted">{tr('workbench.search.mode',{mode:results.mode==='keyword'?tr('workbench.search.keyword'):results.mode||tr('workbench.not_reported')})} · {tr('workbench.search.watermark',{value:typeof results.index_watermark==='number'?formatNumber(results.index_watermark):tr('workbench.not_reported')})}</p>}
 </section>;
}
