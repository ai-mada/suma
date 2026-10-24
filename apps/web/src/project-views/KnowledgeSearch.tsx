import {useEffect,useRef,useState,type FormEvent} from 'react';
import {Search} from 'lucide-react';
import {api} from '../api';
import {Empty,ErrorNotice} from '../ui';
import {formatNumber,tr,useLocale} from '../i18n';
type SearchResult={items:{id:string;type?:string;title?:string;snippet?:string;source_ref?:string;revision?:number;locator?:unknown}[];mode?:string;index_watermark?:number|null};
type Snapshot={project:string;query:string;results?:SearchResult;error?:unknown;busy:boolean};
export function KnowledgeSearch({project}:{project:string}){
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
  <div className="search-results">{results?.items?.map(item=><article key={item.id}><span className="type-chip" title={item.type}>{['requirements','tasks','issues','documents'].includes(item.type||'')?tr('common.entities.'+item.type):item.type}</span><h3>{item.title}</h3><p>{item.snippet}</p><small>{tr('workbench.search.result_reference',{source:item.source_ref||tr('workbench.not_reported'),revision:typeof item.revision==='number'?formatNumber(item.revision):tr('workbench.not_reported'),locator:typeof item.locator==='object'?JSON.stringify(item.locator):String(item.locator??'')})}</small></article>)}</div>
  {results&&!results.items?.length&&<Empty title={tr('workbench.search.empty')} description={tr('workbench.search.empty_hint')}/>}
  {results&&<p className="muted">{tr('workbench.search.mode',{mode:results.mode==='keyword'?tr('workbench.search.keyword'):results.mode||tr('workbench.not_reported')})} · {tr('workbench.search.watermark',{value:typeof results.index_watermark==='number'?formatNumber(results.index_watermark):tr('workbench.not_reported')})}</p>}
 </section>;
}
