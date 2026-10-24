import {useCallback,useEffect,useRef,useState} from 'react';
import {ArrowRight,GitBranch} from 'lucide-react';
import {time,type Run} from '../api';
import {Badge,Empty,ErrorNotice,Loading} from '../ui';
import {EntityPager} from '../pagination';
import {formatNumber,tr,useLocale} from '../i18n';

export function Runs({project,refresh,onRun,onWorkflow}:{project:string;refresh:number;onRun:(id:string)=>void;onWorkflow:()=>void}){
 useLocale();const pager=useRef(new EntityPager<Run>(project,'runs'));
 const [page,setPage]=useState(()=>pager.current.snapshot()),rows=page.items;
 const read=useCallback(async(more=false)=>{pager.current.setScope(project,'runs');const request=more?pager.current.more():pager.current.first();setPage(pager.current.snapshot());await request;setPage(pager.current.snapshot());},[project]);
 useEffect(()=>{void read();},[read,refresh]);
 if(page.error&&!rows)return <ErrorNotice error={page.failure||page.error} onRetry={()=>void read()}/>;
 if(!rows)return <Loading/>;
 const loaded=tr('workbench.runs.loaded',{loaded:formatNumber(rows.length),total:formatNumber(page.total)});
 return <section className="panel"><div className="list-toolbar"><span>{loaded}</span><button className="primary" onClick={onWorkflow}><GitBranch size={15}/>{tr('workbench.runs.compose')}</button></div>
  {page.error&&<ErrorNotice error={page.failure||page.error} onRetry={()=>void read(page.error_action==='more')}/>}
  <div className="loaded-scope">{tr('workbench.runs.scope')}</div>
  {rows.length?<div className="table-scroll"><table><thead><tr>{['run','status','version','nodes','started'].map(field=><th key={field}>{tr('workbench.runs.column.'+field)}</th>)}<th/></tr></thead><tbody>{rows.map(run=><tr key={run.id}><td><button className="row-title" onClick={()=>onRun(run.id)}><span className="row-icon"><GitBranch size={16}/></span><span>{run.workflow_name}<small>{run.id.slice(0,16)}</small></span></button></td><td><Badge state={run.state}/></td><td>v{formatNumber(run.workflow_version)}</td><td>{tr('workbench.runs.in_details')}</td><td className="muted">{time(run.created_at)}</td><td><button className="icon-button" aria-label={tr('workbench.runs.open')} onClick={()=>onRun(run.id)}><ArrowRight size={16}/></button></td></tr>)}</tbody></table></div>:<Empty title={tr('workbench.runs.empty')} description={tr('workbench.runs.empty_hint')} action={<button onClick={onWorkflow}>{tr('workbench.runs.create')}</button>}/>}
  <div className="list-footer pagination-footer"><span>{loaded}</span>{page.next_cursor?<button disabled={page.loading} onClick={()=>void read(true)}>{tr(page.loading?'common.loading':'workbench.runs.more')}</button>:<span>{tr('workbench.runs.end')}</span>}</div>
 </section>;
}
