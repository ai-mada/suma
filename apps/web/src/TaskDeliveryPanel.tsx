import {useEffect,useId,useRef,useState} from 'react';
import {ArrowUpRight,ChevronDown,CircleAlert,Cloud,GitPullRequest,Link2,RefreshCw} from 'lucide-react';
import {type Session} from './api';
import {tr,useLocale,formatNumber} from './i18n';
import {Badge,ErrorNotice,Loading} from './ui';
import {useTaskDeliveryRead} from './task-delivery-hooks';
import {FailureDiagnosisLauncher} from './FailureDiagnosisLauncher';
import {TaskDeliveryRepositorySource} from './TaskDeliveryRepositorySource';
import type {TaskDeliveryRunTarget} from './TaskDependenciesWorkspace';
import {followTaskDeliveryHref,taskDeliveryHref,workBuddyProgressTaskHref} from './task-delivery-navigation-model';
import {TaskRepairContext} from './TaskRepairContext';
import {canLeave,DeliveryReference} from './TaskDeliveryReference';
import {executionHref} from './scheduling-model';
import './TaskDeliveryPanel.css';
export type TaskDeliveryPanelProps={session:Session;project:string;taskId:string;taskVersion:number;refresh:number;openOnEntry?:boolean;originRunID?:string;navigationDisabled?:boolean;onOpenCurrentDelivery?:()=>void;onOpenRun?:(target:TaskDeliveryRunTarget)=>void;onOpenReview?:()=>void;onOpenRepository?:()=>void;onOpenIssue?:(id:string)=>void;onOpenRequirement?:(id:string)=>void;onTeamPlan?:(id:string)=>void;onExtractAcceptedDelivery?:(taskID:string)=>void;onVersionMismatch?:()=>void;onForbidden?:()=>void};
export function TaskDeliveryPanel(props:TaskDeliveryPanelProps){return <DeliveryOwner key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.taskId,props.originRunID])} {...props}/>;}
function DeliveryOwner(props:TaskDeliveryPanelProps){const [retry,setRetry]=useState(0);return <DeliveryScope key={retry} {...props} initiallyOpen={retry>0} retryRead={()=>setRetry(value=>value+1)}/>;}
const t=(key:string,values?:Record<string,unknown>)=>tr('task-delivery.'+key,values);
function DeliveryScope({session,project,taskId,taskVersion,refresh,openOnEntry,originRunID,navigationDisabled,onOpenCurrentDelivery,onOpenRun,onOpenReview,onOpenRepository,onOpenIssue,onOpenRequirement,onTeamPlan,onExtractAcceptedDelivery,onVersionMismatch,onForbidden,initiallyOpen,retryRead}:TaskDeliveryPanelProps&{initiallyOpen:boolean;retryRead:()=>void}){
 useLocale();const [open,setOpen]=useState(initiallyOpen||!!openOnEntry),[repositoryLocked,setRepositoryLocked]=useState(false),body=useId(),read=useTaskDeliveryRead(project,taskId,open,refresh),snapshot=read.value;
 const toggle=useRef<HTMLButtonElement>(null);
 useEffect(()=>{
  if(!openOnEntry)return;
  setOpen(true);
  toggle.current?.focus({preventScroll:true});
  toggle.current?.scrollIntoView?.({block:'nearest'});
 },[openOnEntry]);
 const callbacks=useRef({onVersionMismatch,onForbidden});callbacks.current={onVersionMismatch,onForbidden};
 useEffect(()=>{if(snapshot&&snapshot.task_version!==taskVersion)callbacks.current.onVersionMismatch?.();},[snapshot,taskVersion]);
 useEffect(()=>{if(read.forbidden)callbacks.current.onForbidden?.();},[read.forbidden]);
 const locked=!!navigationDisabled||read.loading||!!read.error;
 const originRelation=originRunID&&!read.loading&&!read.error&&!read.forbidden&&snapshot?.task_version===taskVersion
  ?snapshot.run?snapshot.run.run_id===originRunID?'current':'changed':snapshot.run_basis_state==='not_dispatched'?'unbound':'unknown'
  :undefined;
 const value=!originRunID||originRelation==='current'?snapshot:undefined,run=value?.run;
 const workbuddy=value?.task_version===taskVersion?run?.workbuddy_delivery:undefined;
 const workbuddyOrigin=!read.loading&&!read.error&&!read.forbidden&&value?.task_version===taskVersion
  ?run?.workbuddy_origin:undefined;
 const reviewHref=value&&value.task_version===taskVersion&&value.run_basis_state==='current'&&run
  ?taskDeliveryHref(project,{kind:'review',id:taskId,taskVersion:value.task_version,runID:run.run_id}):undefined;
 const repositoryHref=value?.formal_acceptance&&value.task_version===taskVersion&&value.run_basis_state==='current'
  ?taskDeliveryHref(project,{kind:'repository',id:taskId}):undefined;
 const workbuddyHistoryHref=(workbuddyOrigin||workbuddy)&&snapshot?.task_version===taskVersion?workBuddyProgressTaskHref(project,taskId):undefined;
 const repair=!read.loading&&!read.error&&!read.forbidden&&value?.task_version===taskVersion?value.repair:undefined;
 const scope=(total:number,loaded:number)=>t('scope',{loaded:formatNumber(loaded),total:formatNumber(total)});
 return <section className="task-delivery" aria-label={t('title')}><button type="button" className="task-delivery-toggle" ref={toggle} data-task-delivery-toggle aria-expanded={open} aria-controls={body} onClick={()=>{if(!open||canLeave())setOpen(current=>!current);}}><span>{t('title')}</span><ChevronDown size={15}/></button>
  {open&&<div id={body} className="task-delivery-body"><p className="task-delivery-muted">{t('intro')}</p><div className="task-delivery-actions"><button type="button" data-task-delivery-refresh disabled={repositoryLocked||read.loading||read.forbidden} onClick={()=>void read.read()}><RefreshCw size={13}/>{t('refresh')}</button></div>{read.loading&&<Loading/>}{Boolean(read.error)&&<ErrorNotice error={read.error}/>}
   {read.forbidden&&<div className="task-delivery-warning"><p>{t('access_changed')}</p><button type="button" data-task-delivery-retry onClick={retryRead}>{tr('common.retry')}</button></div>}
   {!read.forbidden&&Boolean(read.error)&&!read.loading&&<button type="button" data-task-delivery-retry onClick={()=>void read.read()}>{tr('common.retry')}</button>}
   {originRunID&&snapshot&&snapshot.task_version!==taskVersion&&<p role="status" className="task-delivery-muted">{t('origin.version_pending')}</p>}
   {originRelation&&<div className="task-delivery-origin" data-task-delivery-origin data-relation={originRelation}>
    {originRelation==='current'?<Link2 size={16} aria-hidden="true"/>:<CircleAlert size={16} aria-hidden="true"/>}
    <div><div role="status"><strong>{t('origin.'+originRelation+'.title')}</strong><p>{t('origin.'+originRelation+'.detail')}</p></div>
     {originRelation!=='current'&&<DeliveryReference kind="current-link" id={taskId} href={taskDeliveryHref(project,{kind:'delivery',id:taskId})} locked={locked||repositoryLocked} canLeave={canLeave} onNavigate={onOpenCurrentDelivery}>{t('origin.open_current')}</DeliveryReference>}
    </div>
   </div>}
   {value&&<><section className="task-delivery-section"><h4>{t('business')}</h4><div className="inline wrap"><Badge state={value.task_status}/><span className="task-delivery-muted">{t('version',{version:formatNumber(value.task_version)})}</span></div>{value.task_version!==taskVersion&&<p className="task-delivery-warning">{t('graph_changed')}</p>}</section>
    {repair&&<TaskRepairContext project={project} repair={repair} locked={locked||repositoryLocked} onOpenIssue={onOpenIssue} onOpenRun={onOpenRun}/>}
    <section className="task-delivery-section"><h4>{t('execution')}</h4><p className={value.run_basis_state==='changed'||value.run_basis_state==='unknown'?'task-delivery-warning':'task-delivery-muted'}>{t('basis.'+value.run_basis_state)}</p>{run&&<><div className="inline wrap"><Badge state={run.state}/><code>{run.run_id.slice(0,8)}</code></div><p className="task-delivery-muted">{t('node_counts',{total:formatNumber(run.nodes.total),running:formatNumber(run.nodes.running),failed:formatNumber(run.nodes.failed),unknown:formatNumber(run.nodes.reconciling)})}</p><button type="button" data-task-delivery-run disabled={locked||!onOpenRun} onClick={()=>onOpenRun?.({run:run.run_id})}>{t('open_run')}</button></>}
     {workbuddyOrigin&&<div className="task-delivery-workbuddy-origin" data-task-delivery-workbuddy-origin={workbuddyOrigin.kind}>
      <Cloud size={16} aria-hidden="true"/>
      <div>
       <strong>{t('workbuddy.origin.title')}</strong>
       <p>{t('workbuddy.origin.'+workbuddyOrigin.kind)} · {t('attempt',{attempt:formatNumber(workbuddyOrigin.attempt)})}</p>
       <DeliveryReference kind="workbuddy-origin-link" id={workbuddyOrigin.node_id} href={executionHref(project,workbuddyOrigin.run_id,workbuddyOrigin.node_id,workbuddyOrigin.attempt)}
        locked={locked||repositoryLocked} canLeave={canLeave}
        onNavigate={onOpenRun?()=>onOpenRun({run:workbuddyOrigin.run_id,node:workbuddyOrigin.node_id,attempt:String(workbuddyOrigin.attempt)}):undefined}>
        {t('workbuddy.open_attempt')}<ArrowUpRight size={13} aria-hidden="true"/>
       </DeliveryReference>
      </div>
     </div>}
     {workbuddyHistoryHref&&!locked&&!repositoryLocked&&<a className="task-delivery-reference" data-task-delivery-workbuddy-history href={workbuddyHistoryHref}
      onClick={event=>followTaskDeliveryHref(event,workbuddyHistoryHref)}>{tr('workbuddy-progress.task_history')}<ArrowUpRight size={13} aria-hidden="true"/></a>}
     {workbuddy&&<div className="inline wrap">
      <p className="task-delivery-warning">{t('workbuddy.pending')}</p>
      <DeliveryReference kind="workbuddy" id={workbuddy.node_id} href={executionHref(project,workbuddy.run_id,workbuddy.node_id,workbuddy.attempt)}
       locked={locked} canLeave={canLeave}
       onNavigate={onOpenRun?()=>onOpenRun({run:workbuddy.run_id,node:workbuddy.node_id,attempt:String(workbuddy.attempt)}):undefined}>
       {t('workbuddy.open_attempt')}
      </DeliveryReference>
     </div>}
    </section></>}
    {(!originRunID||originRelation==='current'||repositoryLocked&&!read.forbidden)&&<TaskDeliveryRepositorySource project={project} taskId={taskId} taskVersion={taskVersion} organization={session.organization.id} owner={JSON.stringify([session.user.id,session.organization.id,project,taskId])}
      attempt={!locked&&value?.run_basis_state==='current'?workbuddy:undefined} disabled={locked} onLockChange={setRepositoryLocked} onOpenRun={onOpenRun?runID=>onOpenRun({run:runID}):undefined}/>}
    {value&&<>
    <section className="task-delivery-section"><h4>{t('tests')}</h4>{value.test_plan_id&&<a data-task-delivery-test-plan aria-disabled={locked||undefined} tabIndex={locked?-1:undefined} href={'?'+new URLSearchParams({project,view:'quality',sub:'tests',record:value.test_plan_id,...(run?{run:run.run_id,panel:'test-results'}:{})})} onClick={event=>{if(locked||!canLeave())event.preventDefault();}}>{tr('test-report-setup.open_plan')}</a>}<p className="task-delivery-muted">{t('test_boundary')}</p>{run?<><p>{t('test_counts',{total:formatNumber(run.tests.total),passed:formatNumber(run.tests.succeeded),failed:formatNumber(run.tests.failed),pending:formatNumber(run.tests.waiting+run.tests.running+run.tests.reconciling+run.tests.other)})}</p>{!!run.tests.items.length&&<ul className="task-delivery-list">{run.tests.items.map(node=><li key={node.id}><button type="button" data-task-delivery-test={node.id} disabled={locked||!onOpenRun} onClick={()=>onOpenRun?.({run:run.run_id,node:node.id,...(node.attempt>0?{attempt:String(node.attempt)}:{})})}>{node.name||node.id}</button><Badge state={node.state}/><small>{node.attempt>0?t('attempt',{attempt:formatNumber(node.attempt)}):tr('development.workflow.not_yet_executed')}</small></li>)}</ul>}{!run.tests.complete&&<p className="task-delivery-muted">{scope(run.tests.total,run.tests.items.length)} <button type="button" disabled={locked||!onOpenRun} onClick={()=>onOpenRun?.({run:run.run_id})}>{t('open_all_nodes')}</button></p>}{!run.tests.total&&<p className="task-delivery-muted">{t('no_tests')}</p>}</>:<p className="task-delivery-muted">{t('no_run')}</p>}</section>
    <section className="task-delivery-section"><h4>{t('acceptance')}</h4><strong className={value.formal_acceptance?'task-delivery-accepted':'task-delivery-muted'}>{t(value.formal_acceptance?'accepted':'not_accepted')}</strong><p className="task-delivery-muted">{t('acceptance_boundary')}</p><DeliveryReference kind="review" id={taskId} href={reviewHref} locked={locked} canLeave={canLeave} onNavigate={onOpenReview}>{t('open_review')}</DeliveryReference>
     {repositoryHref&&<DeliveryReference kind="repository" id={taskId} href={repositoryHref} locked={locked||repositoryLocked} canLeave={canLeave} onNavigate={onOpenRepository}><GitPullRequest size={15} aria-hidden="true"/>{t('open_repository')}</DeliveryReference>}
     {value.formal_acceptance&&value.task_version===taskVersion&&<button type="button" data-task-delivery-extract disabled={locked||!onExtractAcceptedDelivery} onClick={()=>{if(canLeave())onExtractAcceptedDelivery?.(taskId);}}>{tr('delivery-knowledge.extract_action')}</button>}</section>
    <section className="task-delivery-section"><h4>{t('team_plans')}</h4><p className="task-delivery-muted">{t('current_run_scope')}</p>{value.expert_plans.items.length?<ul className="task-delivery-list">{value.expert_plans.items.map(plan=><li key={plan.plan_id}><DeliveryReference kind="plan" id={plan.plan_id} href={taskDeliveryHref(project,{kind:'team',id:plan.plan_id})} locked={locked} canLeave={canLeave} onNavigate={onTeamPlan?()=>onTeamPlan(plan.plan_id):undefined}>{plan.team_name}</DeliveryReference><small>{tr('expert-team-orchestration.revision',{value:formatNumber(plan.revision)})} · {tr('expert-team-orchestration.item.'+plan.item_state)}</small></li>)}</ul>:<p className="task-delivery-muted">{t('no_plans')}</p>}{!value.expert_plans.complete&&<p className="task-delivery-muted">{scope(value.expert_plans.total,value.expert_plans.items.length)}</p>}</section>
    <section className="task-delivery-section"><h4>{t('issues')}</h4><p className="task-delivery-muted">{t('current_run_scope')}</p>{value.issues.items.length?<ul className="task-delivery-list">{value.issues.items.map(issue=><li key={issue.issue_id}><DeliveryReference kind="issue" id={issue.issue_id} href={taskDeliveryHref(project,{kind:'issue',id:issue.issue_id})} locked={locked} canLeave={canLeave} onNavigate={onOpenIssue?()=>onOpenIssue(issue.issue_id):undefined}>{issue.title||issue.issue_id}</DeliveryReference><div className="inline wrap"><Badge state={issue.issue_status}/><small>{t('source.'+issue.source)}</small></div>{issue.requirement_id&&<DeliveryReference kind="requirement" id={issue.requirement_id} href={taskDeliveryHref(project,{kind:'requirement',id:issue.requirement_id})} locked={locked} canLeave={canLeave} onNavigate={onOpenRequirement?()=>onOpenRequirement(issue.requirement_id):undefined}>{t('open_requirement')} · {t('version',{version:formatNumber(issue.requirement_version)})}</DeliveryReference>}</li>)}</ul>:<p className="task-delivery-muted">{t('no_issues')}</p>}{!value.issues.complete&&<p className="task-delivery-muted">{scope(value.issues.total,value.issues.items.length)}</p>}</section>
    {run&&(run.nodes.failed>0||run.nodes.reconciling>0)&&<FailureDiagnosisLauncher session={session} project={project} runID={run.run_id} refresh={refresh}/>}
   </>}
  </div>}
 </section>;
}
