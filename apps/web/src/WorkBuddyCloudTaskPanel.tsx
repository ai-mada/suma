import {useEffect,useId,useRef,useState,type FormEvent} from 'react';
import {CircleAlert,Cloud,FileText,PauseCircle,RefreshCw,ShieldCheck} from 'lucide-react';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {stateLabel} from './api';
import {cloudRunnerPhase,type CloudRunFacts} from './workbuddy-cloud-task-model';
import './WorkBuddyCloudTaskPanel.css';

type CloudAction='delegate'|'revoke'|'refresh';
export type WorkBuddyCloudReason='source_missing'|'node_mismatch'|'connection_unavailable'|'connection_expired'|'budget_enforced'|'permission_changed';
export type WorkBuddyCloudActionResult={kind:'confirmed'|'conflict'|'unknown'|'denied'|'failed';reason?:WorkBuddyCloudReason};
export type WorkBuddyCloudDelegateDraft={connectionID:string;connectionVersion:number;grantGeneration:number;expiresAt:string;sourceVersion:string};

/** This is a view projection, not an alternative API contract. The container
 * supplies current-authorized exact-node facts, original-operation recovery
 * and versioned commands. An eligible connection must match the frozen node. */
export type WorkBuddyCloudTaskModel={
 scopeKey:string;
 sourceVersion:string;
 execution?:CloudRunFacts;
 state:'loading'|'ready'|'unavailable'|'denied';
 unavailableReason?:WorkBuddyCloudReason;
 task:{title:string;runLabel:string;nodeLabel:string;prompt:string;name?:string;requestDigest:string;nodeDigest:string};
 connections:{id:string;label:string;version:number;grantGeneration:number;eligible:boolean;unavailableReason?:WorkBuddyCloudReason}[];
 actions:{delegate:boolean;revoke:boolean;refresh:boolean};
 delegation?:{id:string;version:number;state:'active'|'revoked';expiresAt:string;connectionLabel:string;createsConsumed:0|1};
 submission?:{id:string;version:number;state:'prepared'|'unknown'|'task_known';taskID?:string;providerStatus?:string;observedAt?:string;lastError?:string};
 recovery?:{action:CloudAction;state:'unknown'|'conflict';canReconcile:boolean};
};
export type WorkBuddyCloudTaskPanelProps={
 model:WorkBuddyCloudTaskModel;
 onDelegate:(draft:WorkBuddyCloudDelegateDraft)=>Promise<WorkBuddyCloudActionResult>;
 onRevoke:()=>Promise<WorkBuddyCloudActionResult>;
 onRefresh:()=>Promise<WorkBuddyCloudActionResult>;
 /** Read the current snapshot; this must not replay a create command. */
 onReload:()=>Promise<void>;
 /** Resolve the original persisted operation, never manufacture a new one. */
 onReconcile?:()=>Promise<WorkBuddyCloudActionResult>;
};
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-cloud-task.'+key,values);
const durations=[15,60,240,1440] as const;
const reason=(value?:WorkBuddyCloudReason)=>t('reason_'+(value||'connection_unavailable'));

export function WorkBuddyCloudTaskPanel(props:WorkBuddyCloudTaskPanelProps){
 // User/organization/project/run/node transitions retire all protected drafts
 // and async callbacks from the previous scope, even when the panel stays open.
 return <CloudTaskWorkspace key={props.model.scopeKey} {...props}/>;
}

function CloudTaskWorkspace({model,onDelegate,onRevoke,onRefresh,onReload,onReconcile}:WorkBuddyCloudTaskPanelProps){
 useLocale();const id=useId();
 const [connectionID,setConnectionID]=useState(''),[duration,setDuration]=useState(''),[ack,setAck]=useState(false);
 const [pending,setPending]=useState<CloudAction|'reload'|'reconcile'>(),[outcome,setOutcome]=useState<WorkBuddyCloudActionResult['kind']>();
 const [confirmedAction,setConfirmedAction]=useState<CloudAction>(),[uncertainAction,setUncertainAction]=useState<CloudAction>();
 const [resultReason,setResultReason]=useState<WorkBuddyCloudReason>();
 const [basisChanged,setBasisChanged]=useState(false),[localDenied,setLocalDenied]=useState(false),[reloadFailed,setReloadFailed]=useState(false);
 const alive=useRef(true),busy=useRef(false),initialSource=useRef(model.sourceVersion),errorSummary=useRef<HTMLDivElement>(null);
 const denied=model.state==='denied'||localDenied;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{
  if(initialSource.current===model.sourceVersion)return;
  initialSource.current=model.sourceVersion;setAck(false);setBasisChanged(true);setConfirmedAction(undefined);setReloadFailed(false);
  if(model.state==='ready')setLocalDenied(false);
  // A new snapshot resolves stale versions, not an uncertain external effect.
  setOutcome(value=>value==='conflict'||value==='failed'||value==='confirmed'?undefined:value);
 },[model.sourceVersion]);
 useEffect(()=>{if(denied){setConnectionID('');setDuration('');setAck(false);}},[denied]);
 useEffect(()=>{if(outcome==='failed'||outcome==='conflict')errorSummary.current?.focus({preventScroll:true});},[outcome]);
 const unknown=outcome==='unknown'||!!uncertainAction||model.recovery?.state==='unknown';
 const conflict=outcome==='conflict'||model.recovery?.state==='conflict';
 const locked=!!pending||unknown||conflict||denied||model.state!=='ready';
 const selected=model.connections.find(item=>item.id===connectionID&&item.eligible);
 const hasSubmission=!!model.submission;
 const canDelegate=model.actions.delegate&&!model.delegation&&!hasSubmission&&confirmedAction!=='delegate';
 const canRevoke=model.actions.revoke&&model.delegation?.state==='active'&&confirmedAction!=='revoke';
 const canRefresh=model.actions.refresh&&model.submission?.state==='task_known'&&!!model.submission.taskID;
 const canReconcile=!!onReconcile&&(model.recovery?model.recovery.canReconcile:!!uncertainAction);
 const ready=canDelegate&&!!selected&&durations.includes(Number(duration) as typeof durations[number])&&ack&&!locked;
 const execute=async(action:CloudAction|'reconcile',command:()=>Promise<WorkBuddyCloudActionResult>)=>{
  if(busy.current||denied||model.state!=='ready'||action!=='reconcile'&&locked)return;
  busy.current=true;setPending(action);setReloadFailed(false);setResultReason(undefined);
  if(action!=='reconcile')setOutcome(undefined);
  try{
   const result=await command();
   if(!alive.current)return;
   setOutcome(action==='reconcile'&&result.kind!=='confirmed'&&result.kind!=='denied'?'unknown':result.kind);setResultReason(result.reason);
   if(result.kind==='denied'){setLocalDenied(true);return;}
   if(result.kind==='unknown'){if(action!=='reconcile')setUncertainAction(action);return;}
   if(result.kind==='confirmed'){
    const original=action==='reconcile'?(uncertainAction||model.recovery?.action):action;
    setConfirmedAction(original);setAck(false);
    setUncertainAction(undefined);
   }
  }catch{if(alive.current){setOutcome('unknown');if(action!=='reconcile')setUncertainAction(action);}}
  finally{if(alive.current){busy.current=false;setPending(undefined);}}
 };
 const reload=async()=>{
  if(busy.current)return;busy.current=true;setPending('reload');setReloadFailed(false);
  try{await onReload();if(alive.current){setLocalDenied(false);if(outcome==='conflict'){setOutcome(undefined);setAck(false);setBasisChanged(true);}}}catch{if(alive.current)setReloadFailed(true);}
  finally{if(alive.current){busy.current=false;setPending(undefined);}}
 };
 const delegate=(event:FormEvent)=>{
  event.preventDefault();if(!ready||!selected)return;
  const expiresAt=new Date(Date.now()+Number(duration)*60_000).toISOString();
  void execute('delegate',()=>onDelegate({connectionID:selected.id,connectionVersion:selected.version,grantGeneration:selected.grantGeneration,expiresAt,sourceVersion:model.sourceVersion}));
 };
 const phase=cloudRunnerPhase(model),basic=['not_delegated','delegated','revoked','unknown','known'].includes(phase);
 const status=basic?'status_'+phase:'runner_status_'+phase;
 const hint=phase==='unknown'?'submission_unknown_detail':phase==='known'?'completion_boundary':phase==='revoked'?'revoke_boundary':!basic?'runner_hint_'+phase:undefined;
 const reloadButton=<button type="button" data-cloud-action="reload" onClick={()=>void reload()} disabled={!!pending}><RefreshCw size={16} aria-hidden="true"/>{t(pending==='reload'?'reloading':'reload')}</button>;

 return <section className="workbuddy-cloud-task" aria-labelledby={id+'-title'} aria-busy={!!pending}>
  <header className="cloud-task-header"><Cloud size={21} aria-hidden="true"/><div><h3 id={id+'-title'}>{t('title')}</h3><p>{t('intro')}</p></div></header>
  {model.state==='loading'?<p className="cloud-task-notice" role="status">{t('loading')}</p>:denied?<div className="cloud-task-notice" role="alert"><CircleAlert size={19} aria-hidden="true"/><div><strong>{t('denied_title')}</strong><p>{t('denied_detail')}</p>{reloadButton}</div></div>:model.state==='unavailable'?<div className="cloud-task-notice"><FileText size={19} aria-hidden="true"/><div><strong>{t('unavailable_title')}</strong><p>{reason(model.unavailableReason)}</p>{reloadButton}</div></div>:<>
   <div className="cloud-task-context"><div><strong>{model.task.title}</strong><p title={model.task.runLabel+' / '+model.task.nodeLabel}>{model.task.runLabel}<span aria-hidden="true"> / </span>{model.task.nodeLabel}</p></div><span className={'cloud-task-status '+(phase==='unknown'||phase==='stop_unconfirmed'||phase==='runner_disconnected'?'is-unknown':'')} data-cloud-runner-phase={phase} role="status"><span aria-hidden="true"/>{t(status)}</span></div>
   {hint&&<p className="cloud-task-hint">{t(hint)}</p>}
   {/* Platform pause and provider observations are independent facts; neither replaces the original critical phase. */}
   {model.submission&&model.execution?.runState==='paused'&&<div className="cloud-task-platform-paused" data-cloud-platform-paused><PauseCircle size={18} aria-hidden="true"/><div><strong>{t('runner_status_run_paused')}</strong><p>{t('platform_paused_boundary')}</p></div></div>}
   {model.submission&&(model.submission.state==='task_known'||model.submission.lastError)&&<div className="cloud-task-observation" data-cloud-observation>
    {model.submission.state==='task_known'&&<dl className="cloud-task-facts"><div><dt>{t('provider_status')}</dt><dd className="cloud-task-provider-state">{model.submission.providerStatus||t('not_reported')}</dd></div><div><dt>{t('last_observed')}</dt><dd>{model.submission.observedAt?<time dateTime={model.submission.observedAt}>{formatDateTime(model.submission.observedAt,{dateStyle:'medium',timeStyle:'short'})}</time>:t('not_verified')}</dd></div></dl>}
    {model.submission.lastError&&<p className="cloud-task-inline-error">{t('last_read_failed')}</p>}
   </div>}
   {!unknown&&!conflict&&outcome!=='confirmed'&&<div className="cloud-task-record-refresh">{canRefresh?<button type="button" className="primary" data-cloud-action="refresh" disabled={locked} onClick={()=>void execute('refresh',onRefresh)}><RefreshCw size={16} aria-hidden="true"/>{t(pending==='refresh'?'refreshing':'refresh')}</button>:<><p className="cloud-task-hint">{t('record_refresh_hint')}</p>{reloadButton}</>}</div>}
   {(unknown||conflict)&&<div ref={errorSummary} tabIndex={-1} className="cloud-task-notice is-warning" role="alert"><CircleAlert size={19} aria-hidden="true"/><div><strong>{t(unknown?'operation_unknown':'source_conflict')}</strong><p>{t(unknown?'do_not_repeat':'review_current')}</p>{resultReason&&<p>{reason(resultReason)}</p>}<div className="cloud-task-actions">{reloadButton}{unknown&&onReconcile&&canReconcile&&<button type="button" data-cloud-action="reconcile" className="primary" disabled={!!pending} onClick={()=>void execute('reconcile',onReconcile)}>{t(pending==='reconcile'?'reconciling':'reconcile')}</button>}</div></div></div>}
   {outcome==='failed'&&<div ref={errorSummary} className="cloud-task-notice is-warning" role="alert" tabIndex={-1}><CircleAlert size={19} aria-hidden="true"/><div><strong>{t('command_failed')}</strong><p>{resultReason?reason(resultReason):t('draft_retained')}</p></div></div>}
   {reloadFailed&&<p className="cloud-task-inline-error" role="alert">{t('reload_failed')}</p>}
   {outcome==='confirmed'&&<div className="cloud-task-confirmed"><p className="cloud-task-confirmation" role="status"><ShieldCheck size={17} aria-hidden="true"/>{t(confirmedAction==='delegate'?'delegate_confirmed':confirmedAction==='revoke'?'revoke_confirmed':confirmedAction==='refresh'?'refresh_confirmed':'operation_confirmed')}</p>{reloadButton}</div>}
   <details className="cloud-task-request" data-cloud-execution-details><summary><FileText size={17} aria-hidden="true"/>{t('execution_details')}</summary><div className="cloud-task-request-body">
    {model.execution&&<dl className="cloud-task-facts"><div><dt>{t('platform_run_state')}</dt><dd>{model.execution.runState==='waiting'?t('runner_waiting_state'):stateLabel(model.execution.runState)}</dd></div><div><dt>{t('platform_node_state')}</dt><dd>{model.execution.nodeState==='waiting'?t('runner_waiting_state'):stateLabel(model.execution.nodeState)}</dd></div><div><dt>{t('original_attempt')}</dt><dd>{formatNumber(model.execution.attempt)}</dd></div><div><dt>{t('original_fence')}</dt><dd>{formatNumber(model.execution.fence)}</dd></div><div><dt>{t('original_worker')}</dt><dd><code>{model.execution.workerID||t('not_verified')}</code></dd></div><div><dt>{t('platform_reason')}</dt><dd><code>{model.execution.reason||t('not_reported')}</code></dd></div></dl>}
    {model.task.name&&<strong>{model.task.name}</strong>}<p className="cloud-task-prompt" tabIndex={0} role="region" aria-label={t('request_text')}>{model.task.prompt}</p><dl><div><dt>{t('request_digest')}</dt><dd><code>{model.task.requestDigest}</code></dd></div><div><dt>{t('node_digest')}</dt><dd><code>{model.task.nodeDigest}</code></dd></div></dl>
    {model.delegation&&<div className="cloud-task-delegation"><h4>{t('delegation_title')}</h4><dl className="cloud-task-facts"><div><dt>{t('connection')}</dt><dd>{model.delegation.connectionLabel}</dd></div><div><dt>{t('expires_at')}</dt><dd><time dateTime={model.delegation.expiresAt}>{formatDateTime(model.delegation.expiresAt,{dateStyle:'medium',timeStyle:'short'})}</time></dd></div><div><dt>{t('create_limit')}</dt><dd>{t(model.delegation.createsConsumed===1?'create_consumed':'create_available')}</dd></div></dl><p className="cloud-task-hint">{t('revoke_boundary')}</p>{canRevoke&&<button type="button" data-cloud-action="revoke" disabled={locked} onClick={()=>void execute('revoke',onRevoke)}>{t(pending==='revoke'?'revoking':'revoke')}</button>}</div>}
    {model.submission&&<div className="cloud-task-submission"><h4>{t('submission_title')}</h4>{model.submission.state==='task_known'&&<dl className="cloud-task-facts"><div><dt>{t('task_id')}</dt><dd><code>{model.submission.taskID||t('not_verified')}</code></dd></div><div><dt>{t('usage')}</dt><dd>{t('not_reported')}</dd></div></dl>}{canRefresh&&!unknown&&!conflict&&outcome!=='confirmed'&&reloadButton}</div>}
   </div></details>
   {!model.delegation&&!hasSubmission&&confirmedAction!=='delegate'&&<form onSubmit={delegate} className="cloud-task-form">
    {basisChanged&&<p className="cloud-task-hint" role="status">{t('basis_changed')}</p>}
    <div className="cloud-task-fields"><div><label htmlFor={id+'-connection'}>{t('connection')}</label><select required id={id+'-connection'} data-cloud-field="connection" value={connectionID} disabled={locked||!canDelegate} onChange={event=>{setConnectionID(event.target.value);setAck(false);}} aria-describedby={id+'-connection-hint'}><option value="">{t('choose_connection')}</option>{model.connections.map(item=><option key={item.id} value={item.id} disabled={!item.eligible}>{item.label}{!item.eligible?' — '+reason(item.unavailableReason):''}</option>)}</select><p id={id+'-connection-hint'} className="cloud-task-hint">{model.connections.some(item=>item.eligible)?t('connection_hint'):t('no_eligible_connection')}</p></div>
    <div><label htmlFor={id+'-duration'}>{t('duration')}</label><select required id={id+'-duration'} data-cloud-field="duration" value={duration} disabled={locked||!canDelegate} onChange={event=>{setDuration(event.target.value);setAck(false);}} aria-describedby={id+'-duration-hint'}><option value="">{t('choose_duration')}</option>{durations.map(value=><option key={value} value={value}>{t('duration_'+value)}</option>)}</select><p id={id+'-duration-hint'} className="cloud-task-hint">{t('duration_hint')}</p></div></div>
    <label className="cloud-task-ack"><input required type="checkbox" data-cloud-field="acknowledge" checked={ack} disabled={locked||!canDelegate||!selected||!duration} onChange={event=>setAck(event.target.checked)}/><span>{t('acknowledge')}</span></label>
    <div className="cloud-task-form-footer"><p className="cloud-task-hint">{t('delegation_boundary')}</p><button type="submit" className={unknown||conflict?undefined:'primary'} data-cloud-action="delegate" disabled={!ready}><ShieldCheck size={17} aria-hidden="true"/>{t(pending==='delegate'?'delegating':'delegate')}</button></div>
   </form>}
   {!model.actions.delegate&&!model.delegation&&!hasSubmission&&<p className="cloud-task-hint">{t('delegate_not_allowed')}</p>}
  </>}
 </section>;
}
