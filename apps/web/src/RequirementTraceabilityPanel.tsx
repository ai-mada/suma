import {useId,useState} from 'react';
import {ChevronDown,RefreshCw} from 'lucide-react';
import type {Session} from './api';
import type {TaskDeliveryRunTarget} from './TaskDependenciesWorkspace';
import {formatNumber,tr,useLocale} from './i18n';
import {Badge,ErrorNotice,Loading} from './ui';
import {useRequirementTraceabilityRead} from './requirement-traceability-hooks';
import {traceCriterionProgress,type RequirementTraceability,type RequirementTraceabilityCriterion,type RequirementTraceabilityTask} from './requirement-traceability-model';
import './RequirementTraceabilityPanel.css';

export type RequirementTraceabilityPanelProps={session:Session;project:string;requirementId:string;requirementVersion:number;refresh:number;onOpenTask?:(id:string)=>void;onOpenRun?:(target:TaskDeliveryRunTarget)=>void;onRefreshRequirement?:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('requirement-traceability.'+key,values);
export function RequirementTraceabilityPanel(props:RequirementTraceabilityPanelProps){return <TraceabilityOwner key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.requirementId])} {...props}/>;}
function TraceabilityOwner(props:RequirementTraceabilityPanelProps){const [retry,setRetry]=useState(0);return <TraceabilityScope key={retry} {...props} initiallyOpen={retry>0} retryRead={()=>setRetry(value=>value+1)}/>;}
function TraceabilityScope({project,requirementId,requirementVersion,refresh,onOpenTask,onOpenRun,onRefreshRequirement,initiallyOpen,retryRead}:RequirementTraceabilityPanelProps&{initiallyOpen:boolean;retryRead:()=>void}){
 useLocale();const [open,setOpen]=useState(initiallyOpen),body=useId(),read=useRequirementTraceabilityRead(project,requirementId,requirementVersion,open,refresh),value=read.value;
 const mismatch=!!value&&value.requirement_version!==requirementVersion;
 return <section className="requirement-traceability" aria-label={t('title')}><button type="button" className="requirement-traceability-toggle" data-trace-toggle aria-expanded={open} aria-controls={body} onClick={()=>setOpen(current=>!current)}><span>{t('title')}</span><ChevronDown size={15}/></button>
  {open&&<div id={body} className="requirement-traceability-body" aria-busy={read.loading}><p className="trace-muted">{t('intro')}</p><div className="trace-actions"><button type="button" data-trace-refresh disabled={read.loading||read.forbidden} onClick={()=>void read.read()}><RefreshCw size={13}/>{t('refresh')}</button></div>
   {read.loading&&<Loading/>}{Boolean(read.error)&&<ErrorNotice error={read.error}/>}
   {read.forbidden?<div className="trace-warning"><p>{t('access_changed')}</p><button type="button" data-trace-retry onClick={retryRead}>{tr('common.retry')}</button></div>:Boolean(read.error)&&!read.loading&&<button type="button" data-trace-retry onClick={()=>void read.read()}>{tr('common.retry')}</button>}
   {value&&<><p className="trace-meta">{t('requirement_version',{version:formatNumber(value.requirement_version)})}{value.published_revision>0&&<> · {t('design_revision',{revision:formatNumber(value.published_revision)})}</>}</p>
    {mismatch&&<div className="trace-warning" role="status"><p>{t('version_changed')}</p>{onRefreshRequirement&&<button type="button" data-trace-refresh-requirement onClick={onRefreshRequirement}>{t('refresh_requirement')}</button>}</div>}
    <p className={value.eligibility==='current'?'trace-muted':'trace-warning'}>{t('eligibility.'+value.eligibility)}</p>
    {value.criteria.length?<ol className="trace-criteria">{value.criteria.map(criterion=><CriterionRow key={JSON.stringify([requirementId,value.requirement_version,criterion.index])} value={value} criterion={criterion} locked={mismatch} onOpenTask={onOpenTask} onOpenRun={onOpenRun}/>)}</ol>:<p className="trace-muted">{t('no_criteria')}</p>}
    <p className="trace-boundary">{t('boundary')}</p>
   </>}
  </div>}
 </section>;
}
type Navigation={locked:boolean;onOpenTask?:RequirementTraceabilityPanelProps['onOpenTask'];onOpenRun?:RequirementTraceabilityPanelProps['onOpenRun']};
function CriterionRow({value,criterion,...navigation}:{value:RequirementTraceability;criterion:RequirementTraceabilityCriterion}&Navigation){
 const [open,setOpen]=useState(false),progress=traceCriterionProgress(value,criterion);
 return <li><details className="trace-criterion" data-trace-criterion={criterion.index} onToggle={event=>setOpen(event.currentTarget.open)}><summary><span className="trace-index">{formatNumber(criterion.index)}</span><span className="trace-criterion-heading"><strong>{criterion.text}</strong><small>{progress.mapped?t('task_progress',{accepted:formatNumber(progress.accepted),mapped:formatNumber(progress.mapped)}):t('unmapped')}</small></span><ChevronDown size={14}/></summary>
  {open&&<div className="trace-criterion-body">{progress.mapped?criterion.task_keys.map(key=>{const task=value.tasks.find(task=>task.key===key)!;return <TraceTask key={key} task={task} {...navigation}/>;}):<p className="trace-muted">{t('unmapped_hint')}</p>}</div>}
 </details></li>;
}
function TraceTask({task,locked,onOpenTask,onOpenRun}:{task:RequirementTraceabilityTask}&Navigation){
 const run=task.run,tests=run?.tests;
 return <article className="trace-task" data-trace-task-key={task.key}><div className="trace-task-heading"><strong>{task.title}</strong><code>{task.key}</code></div>
  <p className={task.binding_state==='current'?'trace-muted':'trace-warning'}>{t('binding.'+task.binding_state)}</p>
  {task.task_id&&<><div className="inline wrap"><Badge state={task.task_status}/><small className="trace-muted">{t('task_version',{version:formatNumber(task.task_version)})}</small></div><button type="button" className="trace-link" data-trace-task={task.task_id} disabled={locked||!onOpenTask} onClick={()=>onOpenTask?.(task.task_id)}>{t('open_task')}</button></>}
  <strong className={task.formal_acceptance?'trace-accepted':'trace-muted'}>{t(task.formal_acceptance?'accepted':'not_accepted')}</strong>
  <p className={task.run_basis_state==='changed'||task.run_basis_state==='unknown'?'trace-warning':'trace-muted'}>{t('basis.'+task.run_basis_state)}</p>
  {run&&<><div className="inline wrap"><Badge state={run.state}/><code className="trace-muted">{run.run_id}</code></div><button type="button" className="trace-link" data-trace-run={run.run_id} disabled={locked||!onOpenRun} onClick={()=>onOpenRun?.({run:run.run_id})}>{t('open_run')}</button>
   {tests&&<details className="trace-tests"><summary>{t('test_counts',{total:formatNumber(tests.total),passed:formatNumber(tests.succeeded),failed:formatNumber(tests.failed),pending:formatNumber(tests.waiting+tests.running+tests.reconciling+tests.other)})}</summary><p className="trace-muted">{t('test_boundary')}</p>
    {!tests.total&&<p className="trace-muted">{t('no_tests')}</p>}
    <ul>{tests.items.map(node=><li key={node.id}><button type="button" className="trace-link" data-trace-test={node.id} disabled={locked||!onOpenRun} onClick={()=>onOpenRun?.({run:run.run_id,node:node.id,...(node.attempt>0?{attempt:String(node.attempt)}:{})})}>{node.name||node.id}</button><div className="inline wrap"><Badge state={node.state}/><small className="trace-muted">{node.attempt>0?t('attempt',{attempt:formatNumber(node.attempt)}):t('unstarted')}</small></div></li>)}</ul>
    {!tests.complete&&<p className="trace-muted">{t('scope',{loaded:formatNumber(tests.items.length),total:formatNumber(tests.total)})} <button type="button" className="trace-link" disabled={locked||!onOpenRun} onClick={()=>onOpenRun?.({run:run.run_id})}>{t('open_complete_run')}</button></p>}
   </details>}
  </>}
 </article>;
}
