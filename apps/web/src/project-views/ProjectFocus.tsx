import {useEffect,useState} from 'react';
import {ArrowRight,ChevronRight,RefreshCw} from 'lucide-react';
import type {ProjectFocus as FocusSnapshot,ProjectFocusItem} from '../../../../packages/contracts/src';
import {query,stateLabel,time} from '../api';
import {useApi} from '../api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from '../i18n';
import {Empty,ErrorNotice,Loading} from '../ui';
import {focusKeys,validProjectFocus,type FocusFilter} from './project-focus-model';
import './ProjectFocus.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('project-focus.'+key,values);
type Props={project:string;refresh:number;navigate:(view:'requirements'|'development',sub?:string)=>void;openRecord:(id:string)=>void};
type ReadState={project:string;api:ReturnType<typeof useApi>;value?:FocusSnapshot;error?:unknown};

/** Remount the local filter on project changes; a language change keeps focus. */
export function ProjectFocus(props:Props){return <FocusContent key={props.project} {...props}/>;}
function FocusContent({project,refresh,navigate,openRecord}:Props){
 useLocale();const api=useApi(),[tick,setTick]=useState(0),[filter,setFilter]=useState<FocusFilter>('all'),[snapshot,setSnapshot]=useState<ReadState>({project,api});
 useEffect(()=>{
  const controller=new AbortController();let active=true;setSnapshot({project,api});
  if(project)void api<unknown>('/project-focus'+query(project),undefined,'GET',undefined,controller.signal).then(value=>{
   if(!active||controller.signal.aborted)return;
   if(!validProjectFocus(value,project))throw new UiError('project-focus.invalid_read');
   setSnapshot({project,api,value});
  }).catch(error=>{if(active&&!controller.signal.aborted)setSnapshot({project,api,error});});
  return()=>{active=false;controller.abort();};
 },[project,api,refresh,tick]);
 const current=snapshot.project===project&&snapshot.api===api?snapshot:undefined,value=current?.value,error=current?.error,loading=!value&&!error;
 const empty=value?.sections.every(section=>section.total===0),sections=value?.sections.filter(section=>!empty&&(filter==='all'||section.key===filter))||[];
 return <section className="project-focus" aria-label={t('title')} aria-busy={loading}>
  <div className="pf-heading"><div><h2>{t('title')}</h2><p>{t('intro')}</p></div><div className="pf-actions"><button type="button" onClick={()=>navigate('requirements')}>{t('plan')}<ArrowRight size={14} aria-hidden="true"/></button><button type="button" className="pf-refresh" disabled={loading} onClick={()=>setTick(n=>n+1)} aria-label={t('refresh')}><RefreshCw size={15} aria-hidden="true"/></button></div></div>
  <div className="pf-filters" role="group" aria-label={t('filter')}>
   <button type="button" className="pf-all" data-focus-filter="all" aria-pressed={filter==='all'} onClick={()=>setFilter('all')}>{t('all')}</button>
   {focusKeys.map(key=><button type="button" data-focus-filter={key} key={key} className={'pf-filter pf-'+key} aria-pressed={filter===key} disabled={!value} onClick={()=>setFilter(key)}><span>{t('section.'+key)}</span><strong>{value?formatNumber(value.sections.find(section=>section.key===key)!.total):'—'}</strong></button>)}
  </div>
  {loading&&<Loading/>}{!!error&&<ErrorNotice error={error} onRetry={()=>setTick(n=>n+1)}/>}
  {value&&<>
   <p className="pf-read-at">{t('observed',{at:formatDateTime(value.observed_at)})}</p>
   <div className="pf-sections">{sections.map(section=><section className="pf-section" key={section.key} aria-label={t('section.'+section.key)}>
    <div className="pf-section-heading"><h3>{t('section.'+section.key)}</h3><span>{t('showing',{loaded:formatNumber(section.items.length),total:formatNumber(section.total)})}</span></div>
    {section.key==='review'&&section.total>0&&<p className="pf-review-note">{t('review_scope')}</p>}
    {section.items.length?<ul className="pf-list">{section.items.map(item=><FocusRow key={item.task_id} item={item} openRecord={openRecord}/>)}</ul>:<p className="pf-section-empty">{t('empty.'+section.key)}</p>}
    {section.truncated&&<button type="button" className="pf-more" onClick={()=>navigate('development','tasks')}>{t('more')}<ArrowRight size={13} aria-hidden="true"/></button>}
   </section>)}</div>
   {empty&&<Empty title={t('empty')} description={t('empty_hint')} action={<button type="button" onClick={()=>navigate('requirements')}>{t('plan')}</button>}/>}
   <div className="pf-footer"><p>{t('scope')}</p><button type="button" onClick={()=>navigate('development','tasks')}>{t('all_tasks')}<ArrowRight size={14} aria-hidden="true"/></button></div>
  </>}
 </section>;
}
function FocusRow({item,openRecord}:{item:ProjectFocusItem;openRecord:(id:string)=>void}){
 const priority=['urgent','high','medium','low'].includes(item.priority)?tr('common.priority.'+item.priority):item.priority||t('priority_unknown');
 return <li><button type="button" className="pf-row" data-focus-task={item.task_id} onClick={()=>openRecord(item.task_id)}>
  <span className="pf-row-main"><strong>{item.title}</strong><span className="pf-row-meta"><span className={'pf-priority priority-'+item.priority}>{priority}</span><span>{t('task_state',{state:stateLabel(item.task_status)})}</span>{item.run_state&&<span>{t('run_state',{state:stateLabel(item.run_state)})}</span>}<span>{t('version',{version:formatNumber(item.task_version)})}</span></span></span>
  <span className="pf-row-context"><span className="pf-reason">{t('reason.'+item.reason)}</span><time dateTime={item.updated_at} title={formatDateTime(item.updated_at)}>{t('updated',{at:time(item.updated_at)})}</time></span><ChevronRight className="pf-chevron" size={16} aria-hidden="true"/>
 </button></li>;
}
