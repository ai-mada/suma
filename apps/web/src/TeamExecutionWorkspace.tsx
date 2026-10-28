import {lazy,Suspense,useCallback,useRef,useState} from 'react';
import {RefreshCw,ArrowRight,Users} from 'lucide-react';
import type {ExpertTeamPlan} from '../../../packages/contracts/src';
import {ApiError,query,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,useLocale,UiError} from './i18n';
import {Empty,ErrorNotice,Loading} from './ui';
import {ErrorBoundary} from './ErrorBoundary';
import {useExpertPage,type ExpertPage} from './expert-team-hooks';
import {expertID,expertInteger} from './expert-team-model';
import {expertPlanCursor,validOrchestratedPlan} from './expert-team-plan-model';
import './TeamExecutionWorkspace.css';

type Props={session:Session;project:string;refresh:number;panel:string;onPlan:(id:string)=>void;onTasks:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('team-execution.'+key,values);
const planKey=(plan:ExpertTeamPlan)=>plan.id;
const counts=['waiting','ready','executing','awaiting_review','blocked','stopped'] as const;
const TeamTaskDispatchDialog=lazy(()=>import('./TeamTaskDispatchDialog').then(module=>({default:module.TeamTaskDispatchDialog})));

/** URL selection reuses the existing plan locator. It never invents a draft,
 * task selection or dispatch when an existing plan is opened or shared. */
export function selectedTeamExecutionPlan(panel:string){
 const id=panel.startsWith('expert-team:')?panel.slice('expert-team:'.length):'';
 return expertID(id)?id:'';
}

export function TeamExecutionWorkspace(props:Props){
 const [retryEpoch,setRetryEpoch]=useState(0);
 // A new owner gets a new read/cache scope before rendering, including when
 // the same project locator appears under a different account or organization.
 // A retry after revoked reads must also discard the shared hook's old value,
 // not merely re-enable it while a replacement request is still pending.
 return <TeamExecutionScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project,retryEpoch])} {...props} onRetryScope={()=>setRetryEpoch(value=>value+1)}/>;
}

function TeamExecutionScope({session,project,refresh,panel,onPlan,onTasks,onRetryScope}:Props&{onRetryScope:()=>void}){
 useLocale();const api=useApi(),[forbidden,setForbidden]=useState<unknown>();
 const selected=selectedTeamExecutionPlan(panel),invalidLink=panel.startsWith('expert-team:')&&!selected;
 const retained=useRef<ExpertPage<ExpertTeamPlan>|undefined>(undefined);
 const valid=useCallback((value:ExpertPage<ExpertTeamPlan>)=>!!value&&Array.isArray(value.items)&&value.items.length<=10&&expertInteger(value.total)&&value.total>=value.items.length&&value.items.every(item=>validOrchestratedPlan(item,project))&&new Set(value.items.map(planKey)).size===value.items.length&&(value.next_cursor===undefined||expertPlanCursor(value.next_cursor)),[project]);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){retained.current=undefined;setForbidden(error);}},[]);
 const page=useExpertPage(api,'/expert-team-plans'+query(project)+'&limit=10',!!project&&!forbidden&&!invalidLink,refresh,valid,failed,planKey,'before');
 if(page.value)retained.current=page.value;
 // The shared hook deliberately clears its authority result on failure. Keep
 // only a visibly stale presentation snapshot for transient failures, never
 // current permission/action facts or the page cursor used by the hook.
 const snapshot=page.value??retained.current,plans=snapshot?.items||[];
 if(forbidden)return <ErrorNotice error={forbidden} onRetry={onRetryScope}/>;
 if(invalidLink)return <section className="team-execution" aria-label={t('title')}><ErrorNotice error={new UiError('team-execution.invalid_link')}/><div><button type="button" onClick={()=>onPlan('')}>{t('back_to_list')}</button></div></section>;
 const open=(id:string)=>{if(window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))onPlan(id);};
 return <section className="team-execution" aria-label={t('title')}>
  <header className="team-execution-toolbar"><div><h2><Users size={18} aria-hidden="true"/>{t('title')}</h2><p>{t('intro')}</p></div><button type="button" disabled={page.loading} onClick={()=>void page.read()}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button></header>
  <p className="team-execution-boundary">{t('acceptance_boundary')}</p>
  {Boolean(page.error)&&<><ErrorNotice error={page.error} onRetry={()=>void page.read()}/>{snapshot&&<p className="team-execution-stale" role="status">{t('stale')}</p>}</>}
  {page.loading&&!snapshot?<Loading/>:snapshot&&<p className="team-execution-scope" aria-live="polite">{t('page_scope',{loaded:formatNumber(plans.length),total:formatNumber(snapshot.total)})}</p>}
  {snapshot&&!plans.length&&!page.error&&!page.loading&&<Empty title={t('empty_title')} description={t('empty_help')} action={<button type="button" className="primary" onClick={onTasks}>{t('open_tasks')}<ArrowRight size={14} aria-hidden="true"/></button>}/>}
  {!!plans.length&&<ul className="team-execution-list" aria-label={t('plans')} aria-busy={page.loading}>{plans.map(plan=><li key={plan.id} className="team-execution-row" data-team-execution-plan={plan.id}>
   <div className="team-execution-plan-heading"><div><button type="button" className="team-execution-open" aria-label={t('open_plan',{name:plan.team_name,id:plan.id.slice(0,8)})} onClick={()=>open(plan.id)}>{plan.team_name}<ArrowRight size={14} aria-hidden="true"/></button><p>{t('plan_reference',{id:plan.id.slice(0,8)})}<span aria-hidden="true"> · </span>{tr('expert-team-orchestration.revision',{value:formatNumber(plan.revision)})}<span aria-hidden="true"> · </span><time dateTime={plan.created_at}>{formatDateTime(plan.created_at,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})}</time></p></div><span className={'team-execution-state team-execution-state-'+plan.state}>{tr('expert-team-orchestration.plan.'+plan.state)}</span></div>
   <div className="team-execution-plan-progress"><span>{t('accepted',{accepted:formatNumber(plan.progress.accepted),total:formatNumber(plan.progress.total)})}</span><progress value={plan.progress.accepted} max={plan.progress.total} aria-label={t('accepted_label',{name:plan.team_name})}/></div>
   {counts.some(key=>plan.progress[key]>0)&&<dl className="team-execution-counts">{counts.filter(key=>plan.progress[key]>0).map(key=><div key={key} className={key==='blocked'||key==='awaiting_review'?'needs-attention':undefined}><dt>{tr('expert-team-orchestration.count.'+key)}</dt><dd>{formatNumber(plan.progress[key])}</dd></div>)}</dl>}
   {!plan.valid&&<p className="team-execution-source-warning">{t('source_changed')}</p>}
  </li>)}</ul>}
  {snapshot?.next_cursor&&<div className="team-execution-more"><button type="button" disabled={page.loading||!!page.error} onClick={()=>void page.read(true)}>{t('more')}</button></div>}
  {selected&&<ErrorBoundary key={selected} inline><Suspense fallback={<Loading/>}><TeamTaskDispatchDialog project={project} session={session} tasks={[]} open planID={selected} onPlan={onPlan} onClose={()=>onPlan('')} onStarted={()=>void page.read()} onForbidden={failed}/></Suspense></ErrorBoundary>}
 </section>;
}
