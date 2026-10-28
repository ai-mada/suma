import {useId,useLayoutEffect,useRef,useState} from 'react';
import {ChevronDown,FolderGit2} from 'lucide-react';
import type {WorkspaceEvidence} from './api';
import type {AttemptToolScope} from './attempt-tool-history';
import {formatDateTime,formatNumber,tr,useLocale} from './i18n';
import {groupWorkspaceOperations,workspaceOperationFailed,type WorkspaceOperationFact,type WorkspaceOperationGroup,workspaceOperationsForAttempt} from './workspace-operations-model';
import {nativeCheckForOperation,nativeOperationCoverage,NativeWorkspaceCheckOutput} from './NativeWorkspaceFacts';
import './WorkspaceOperations.css';
const t=(key:string,values?:Record<string,unknown>)=>tr('workspace-operations.'+key,values);
type Projection=ReturnType<typeof workspaceOperationsForAttempt>;
type Props={scope:AttemptToolScope;projection:Projection;workspaceEvidence?:WorkspaceEvidence;context:'current'|'history';unconfirmed?:boolean;onSource?:(sequence:number)=>void};
/** Both callers supply already loaded, exactly scoped persisted events. This
 * component never starts a read, executes a tool or synthesizes an ACK. */
export function WorkspaceOperations(props:Props){return <OperationScope key={JSON.stringify(props.scope)} {...props}/>;}
function OperationScope({scope,projection,workspaceEvidence,context,unconfirmed,onSource}:Props){
 useLocale();const id=useId(),[open,setOpen]=useState(false),[filter,setFilter]=useState<'all'|'failed'|'unknown'>('all'),root=useRef<HTMLElement>(null),toggle=useRef<HTMLButtonElement>(null),focused=useRef<HTMLElement|null>(null);
 useLayoutEffect(()=>{if(focused.current&&!focused.current.isConnected){const active=document.activeElement;if(!active||active===document.body||!active.isConnected)toggle.current?.focus({preventScroll:true});focused.current=null;}});
 if(projection.scopeKey!==JSON.stringify(scope))return null;
 const groups=groupWorkspaceOperations(projection.facts),coverage=nativeOperationCoverage(workspaceEvidence),rows=groups.filter(group=>filter==='all'||filter==='failed'&&workspaceOperationFailed(group)||filter==='unknown'&&(group.inconsistent||group.latest.operation.phase==='unknown'));
 if(!groups.length&&!projection.invalid&&!workspaceEvidence?.native_workspace)return null;
 return <section className="workspace-operations" ref={root} aria-label={t('title')} onFocusCapture={event=>{focused.current=(event.target as HTMLElement).closest<HTMLElement>('[data-wo-operation]');}} onBlurCapture={event=>{const next=event.relatedTarget;if(!(next instanceof Node)||!root.current?.contains(next))focused.current=null;}}>
  <button type="button" className="wo-toggle" ref={toggle} data-wo-toggle aria-controls={id} aria-expanded={open} onClick={()=>setOpen(value=>!value)}><FolderGit2 size={15} aria-hidden="true"/><span>{t('title')}</span><ChevronDown size={15} className={open?'is-open':''} aria-hidden="true"/></button>
  {open&&<div className="wo-body" id={id}>
   <p className="wo-scope">{t(context==='current'?'current_scope':'history_scope',{attempt:formatNumber(scope.attempt),worker:scope.worker_id})}</p>
   <p className="wo-muted">{t('loaded_scope',{operations:formatNumber(groups.length),records:formatNumber(projection.facts.length)})}</p>
   <p className="wo-muted">{t('boundary')}</p>{unconfirmed&&<p className="wo-warning" role="status">{t('unconfirmed')}</p>}
   {!!projection.invalid&&<p className="wo-warning" role="status">{t('invalid',{count:formatNumber(projection.invalid)})}</p>}
   <div className="wo-toolbar"><label>{t('filter')}<select data-wo-filter value={filter} onChange={event=>setFilter(event.target.value as typeof filter)}><option value="all">{t('filter_all')}</option><option value="failed">{t('filter_failed')}</option><option value="unknown">{t('filter_unknown')}</option></select></label></div>
   {!rows.length&&<p className="wo-empty">{t(groups.length?'empty_filtered':'empty_loaded')}</p>}
   <div className="wo-list">{rows.map(group=><Operation key={group.id} group={group} workspaceEvidence={workspaceEvidence} onSource={onSource}/>)}</div>
   <details><summary>{t('local_capture')}</summary>{coverage?<><p className="wo-muted">{t('capture_'+coverage.state)}</p><dl className="wo-facts"><dt>{t('invocations')}</dt><dd>{formatNumber(coverage.invocation_count)} / {formatNumber(coverage.max_invocations)}</dd><dt>{t('records')}</dt><dd>{formatNumber(coverage.record_count)} / {formatNumber(coverage.max_records)}</dd><dt>{t('pending')}</dt><dd>{formatNumber(coverage.pending_count)}</dd>{coverage.reason&&<><dt>{t('reason_code')}</dt><dd><code>{coverage.reason}</code></dd></>}</dl></>:<p className="wo-muted">{t('capture_missing')}</p>}<p className="wo-muted">{t('capture_boundary')}</p></details>
  </div>}
 </section>;
}
function Operation({group,workspaceEvidence,onSource}:{group:WorkspaceOperationGroup;workspaceEvidence?:WorkspaceEvidence;onSource?:Props['onSource']}){
 const {operation:op}=group.latest,phase=group.inconsistent?'inconsistent':op.phase,check=!group.inconsistent?nativeCheckForOperation(workspaceEvidence,op):undefined;
 return <details className="wo-row" data-wo-operation={group.id}>
  <summary><ChevronDown size={13} className="wo-disclosure" aria-hidden="true"/><span className="wo-action">{group.inconsistent?t('operation'):t('action_'+op.action)}</span><code className="wo-target">{group.inconsistent?group.id:op.path||op.check_id||t('target_workspace')}</code><span className={'wo-phase phase-'+phase}>{t('phase_'+phase)}</span></summary>
  <div className="wo-row-body">
   {group.inconsistent?<p className="wo-warning">{t('inconsistent')}</p>:<>
    {group.missingStart&&<p className="wo-warning">{t('missing_start')}</p>}{group.pendingEnd&&<p className="wo-muted">{t('pending_end')}</p>}
    {op.replayed&&<p className="wo-muted">{t('replayed')}</p>}
    {op.action==='check'&&<><p className={workspaceOperationFailed(group)?'wo-warning':'wo-muted'}>{t('check_boundary')}</p><dl className="wo-facts"><dt>{t('check_exit')}</dt><dd>{op.exit_code===undefined?t('not_provided'):formatNumber(op.exit_code)}</dd><dt>{t('check_stopped')}</dt><dd>{op.stopped===undefined?t('not_provided'):t(op.stopped?'yes':'no')}</dd></dl>{check?<div className="wo-check"><p className="wo-muted">{t('matching_check',{generation:formatNumber(check.generation)})}</p><dl className="wo-facts"><dt>{t('check_profile')}</dt><dd><code>{check.profile_sha256}</code></dd><dt>{t('workspace_sha')}</dt><dd><code>{check.workspace_sha256}</code></dd></dl><NativeWorkspaceCheckOutput check={check}/></div>:<p className="wo-muted">{t('check_unavailable')}</p>}</>}
    {op.error_code&&<p className="wo-warning">{workspaceOperationError(op.error_code)}</p>}
    <dl className="wo-facts"><dt>{t('operation_id')}</dt><dd><code>{op.operation_id}</code></dd>{op.effect_operation_id&&<><dt>{t('effect_operation')}</dt><dd><code>{op.effect_operation_id}</code></dd></>}<dt>{t('generation')}</dt><dd>{formatNumber(op.generation_before)} → {formatNumber(op.generation_after)}</dd><dt>{t('started_at')}</dt><dd>{formatDateTime(op.started_at,{dateStyle:'short',timeStyle:'medium'})}</dd>{op.completed_at&&<><dt>{t('completed_at')}</dt><dd>{formatDateTime(op.completed_at,{dateStyle:'short',timeStyle:'medium'})}</dd></>}<dt>{t('host_recorded_at')}</dt><dd>{formatDateTime(op.recorded_at,{dateStyle:'short',timeStyle:'medium'})}</dd><dt>{t('request_sha')}</dt><dd><code>{op.request_sha256}</code></dd>{op.result_sha256&&<><dt>{t('result_sha')}</dt><dd><code>{op.result_sha256}</code></dd></>}<dt>{t('parent_profile')}</dt><dd><code>{op.profile_sha256}</code></dd><dt>{t('policy_sha')}</dt><dd><code>{op.policy_sha256}</code></dd>{op.error_code&&<><dt>{t('reason_code')}</dt><dd><code>{op.error_code}</code></dd></>}</dl>
   </>}
   <details><summary>{t('source_records')}</summary><p className="wo-muted">{t('sequence_boundary')}</p><ul className="wo-events">{group.facts.map((fact,index)=><li key={fact.operation.event_id+':'+fact.serverSequence+':'+index}>{group.inconsistent?<><p>{t('local_record',{sequence:formatNumber(fact.operation.sequence),revision:formatNumber(fact.operation.revision)})}</p><p>{t('server_record',{sequence:formatNumber(fact.serverSequence),at:formatDateTime(fact.serverAt,{dateStyle:'short',timeStyle:'medium'})})}</p></>:<WorkspaceOperationRecord fact={fact}/>}<details><summary>{t('contract_record')}</summary><pre className="output" tabIndex={0}>{JSON.stringify(fact.operation,null,2)}</pre></details>{onSource&&<button type="button" onClick={()=>onSource(fact.serverSequence)}>{t('open_source')}</button>}</li>)}</ul></details>
  </div>
 </details>;
}
const knownErrors=new Set(['arguments','scope','closed','unavailable','path','missing','text','output_limit','conflict','operation','operation_conflict','unknown','check','check_changed','checks_stale','cursor','trace_limit','merge_unresolved','merge_incomplete','check_failed','check_stopped']);
function workspaceOperationError(code:string){const key=code.startsWith('native_workspace_')?code.slice('native_workspace_'.length):'';return knownErrors.has(key)?t('error_'+key):t('error_unknown_code');}
export function WorkspaceOperationRecord({fact}:{fact:WorkspaceOperationFact}){const op=fact.operation;return <><strong>{t('record',{action:t('action_'+op.action),phase:t('phase_'+op.phase)})}</strong><p>{t('local_record',{sequence:formatNumber(op.sequence),revision:formatNumber(op.revision)})}</p><p>{t('server_record',{sequence:formatNumber(fact.serverSequence),at:formatDateTime(fact.serverAt,{dateStyle:'short',timeStyle:'medium'})})}</p></>;}
