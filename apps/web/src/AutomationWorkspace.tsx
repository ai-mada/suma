import {useApi,useApiView} from './api-context';
import {tr,useLocale,UiError,renderDiagnostic,formatNumber,formatDateTime,getLocale} from './i18n';
import {useCallback,useEffect,useLayoutEffect,useMemo,useRef,useState,type FormEvent,type ReactNode} from 'react';
import {Archive,ArrowUpRight,CalendarClock,ChevronRight,Clock3,Pause,Play,Plus,RefreshCw,Settings2,TriangleAlert,X} from 'lucide-react';
import {api,ApiError,query,stateLabel,type Session,type Workflow} from './api';
import {Empty,ErrorNotice,Field,Loading,Modal} from './ui';
import {EntityPager,type PageSnapshot} from './pagination';
import {useLeaveGuard} from './write-action';
import {AutomationIngressWorkspace} from './AutomationIngress';
import {ingressReason} from './ingress-model';
import {automationTriggerPanelID} from './navigation';
import {AutomationRecovery,AutomationRevisionPager,automationDetailPath,automationEnabled,automationError,automationFailurePaused,automationFailurePauseReason,automationMoment,automationPermissions,automationReason,automationSpec,automationStates,automationWorkflowOptions,misfireNames,newAutomationSpec,occurrenceStates,overlapNames,type AutomationOccurrence,type AutomationPreview,type AutomationResult,type AutomationRevision,type AutomationRule,type AutomationSpec} from './automation-model';
import './automation.css';

type Props={project:string;session:Session;selected:string;search:string;onSearch:(value:string)=>void;refresh:number;onSelect:(id:string)=>void;panel?:string;onIngressSelect?:(id:string)=>void;onOccurrenceSelect?:(id:string)=>void};
type Dialog={kind:'create'}|{kind:'edit'|'enable'|'archive';rule:AutomationRule};
function canLeave(){return window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));}
function sessionStore(){try{return typeof sessionStorage==='undefined'?undefined:sessionStorage;}catch{return undefined;}}
type OccurrenceFilter='all'|'attention';
function useAutomationPage<T extends {id:string}>(project:string,kind:string,refresh:number,filter?:OccurrenceFilter){
 const api=useApi();
 const view=useApiView(),userID=view?.userID,organizationID=view?.organizationID;
 const pager=useMemo(()=>new EntityPager<T>(project,kind,40,{
  request:api,
  parameters:filter?{filter}:undefined,
  discardOnAccessLoss:!!filter,
  headers:filter&&userID&&organizationID?{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID}:undefined,
 }),[project,kind,api,filter,userID,organizationID]);
 const [state,setState]=useState<{pager:EntityPager<T>;page:PageSnapshot<T>}>();
 const read=useCallback(async(more=false)=>{
  const request=more?pager.more():pager.first();
  setState({pager,page:pager.snapshot()});
  await request;
  setState(current=>current?.pager===pager?{pager,page:pager.snapshot()}:current);
 },[pager]);
 useEffect(()=>{void read();},[read,refresh]);
 // A changed filter, project, or API identity must not render the old window.
 const page=state?.pager===pager?state.page:{...pager.snapshot(),loading:true};
 return {...page,read};
}
function useAutomationDetail(project:string,id:string,refresh:number){ const api=useApi();
 const path=id?automationDetailPath(project,id):'',epoch=useRef(0),[state,setState]=useState<{path:string;data?:AutomationRule;loading:boolean;error:string|Error}>({path,loading:!!id,error:''});
 const read=useCallback(async()=>{const request=++epoch.current;if(!path){setState({path,loading:false,error:''});return undefined;}setState(before=>({path,data:before.path===path?before.data:undefined,loading:true,error:''}));try{const data=await api<AutomationRule>(path);if(request!==epoch.current)return undefined;setState({path,data,loading:false,error:''});return data;}catch(error){if(request===epoch.current)setState(before=>({...before,loading:false,error:automationError(error)}));return undefined;}},[path]);
 useEffect(()=>{void read();return()=>{epoch.current++;};},[read,refresh]);return {...(state.path===path?state:{path,data:undefined,loading:!!id,error:''}),read};
}
function useAutomationWrite(recovery:AutomationRecovery){
 const[busy,setBusy]=useState(false),[error,setError]=useState<string|Error>(''),[conflict,setConflict]=useState(false),[deferred,setDeferred]=useState(false),[,render]=useState(0);const pending=recovery.pending;
 useLeaveGuard(busy||!!pending&&!deferred,setError);
 async function execute<T,>(send:()=>Promise<T>){if(busy)return undefined;setBusy(true);setError('');setConflict(false);setDeferred(false);try{return await send();}catch(error){setError(automationError(error));setConflict(error instanceof ApiError&&error.status===409&&!recovery.pending);return undefined;}finally{setBusy(false);render(value=>value+1);}}
 return {busy,pending,error:recovery.error||error,conflict,deferred,setError,clear:()=>{setError('');setConflict(false);},defer:()=>setDeferred(true),run:<T,>(path:string,body:Record<string,unknown>)=>execute<T>(()=>recovery.run<T>(path,body)),restore:()=>execute<AutomationRule|AutomationResult>(()=>recovery.restore<AutomationRule|AutomationResult>())};
}
type WriteAction=ReturnType<typeof useAutomationWrite>;

export function AutomationWorkspace({project,session,selected,search,onSearch,refresh,onSelect,panel,onIngressSelect,onOccurrenceSelect}:Props){ const api=useApi();useLocale();
 const[revision,setRevision]=useState(0),[dialog,setDialog]=useState<Dialog>(),[notice,setNotice]=useState<string|Error>(''),[editFresh,setEditFresh]=useState<AutomationRule>();
 const recovery=useMemo(()=>new AutomationRecovery(session.organization.id+':'+session.user.id+':'+project, sessionStore(), api),[session.organization.id, session.user.id, project, api]),action=useAutomationWrite(recovery),tick=refresh+revision;
 const page=useAutomationPage<AutomationRule>(project,'automations',tick),detail=useAutomationDetail(project,selected,tick),permissions=automationPermissions(session,project),rule=detail.data;
 const selectedOccurrence=automationTriggerPanelID(panel??'');
 const workspace=useRef<HTMLDivElement>(null),returnToOccurrence=useRef<string>(undefined);
 useLayoutEffect(()=>{
  if(selectedOccurrence||!returnToOccurrence.current)return;
  const id=returnToOccurrence.current;
  returnToOccurrence.current=undefined;
  const target=workspace.current?.querySelector<HTMLElement>('[data-automation-open-trigger="'+id+'"]')??workspace.current?.querySelector<HTMLElement>('#automation-occurrence-heading');
  target?.focus();
 },[selectedOccurrence]);
 function selectOccurrence(id:string){
  if(!onOccurrenceSelect||!canLeave())return;
  if(id&&id===selectedOccurrence){workspace.current?.querySelector<HTMLElement>('#automation-trigger-detail-heading')?.focus();return;}
  returnToOccurrence.current=id?undefined:selectedOccurrence;
  onOccurrenceSelect(id);
 }
 const disabled=action.busy||!!action.pending||!!recovery.error,stale=detail.loading||!!detail.error;
 const refreshFailurePause=useCallback(()=>{void detail.read();void page.read();},[detail.read,page.read]);
 const filtered=(page.items||[]).filter(item=>(item.name+' '+item.workflow_id+' '+item.id).toLowerCase().includes(search.toLowerCase()));
 const changed=()=>setRevision(value=>value+1);
 function committed(result:AutomationRule|AutomationResult,message:string|Error){const saved='rule'in result?result.rule:result;setDialog(undefined);setEditFresh(undefined);setNotice(message);changed();onSelect(saved.id);}
 async function command(value:AutomationRule,command:string){const result=await action.run<AutomationResult>('/automations/'+encodeURIComponent(value.id)+'/commands',{project_id:project,expected_version:value.version,action:command});if(result)committed(result,command==='trigger'?tr("automation.manual_trigger_saved_durably_see_trigger_records_for"):command==='pause'?tr("automation.rule_paused_new_manual_scheduled_and_event_dispatches"):tr("automation.rule_updated"));}
 async function reloadConflict(){const fresh=await detail.read();if(fresh){setEditFresh(fresh);action.clear();setNotice(new UiError("automation.latest_rule_loaded_review_its_version_state_and"));}}
 const editPolicy=rule&&<button disabled={disabled||stale||!permissions.write||rule.state==='archived'||action.conflict} onClick={()=>{action.clear();setEditFresh(undefined);setDialog({kind:'edit',rule});}}><Settings2 size={14}/>{tr("automation.edit_policy")}</button>;
 return <div ref={workspace} className="automation-workspace">
  <div className="automation-intro"><span className="row-icon"><CalendarClock size={19}/></span><div><h2>{tr("automation.automation")}</h2><p>{tr("automation.dispatch_a_fixed_workflow_version_on_a_schedule")}</p></div></div>
  {!permissions.write&&<div className="automation-message">{tr("automation.this_project_is_read_only_creating_editing_and")}</div>}
  {action.error&&<><ErrorNotice error={action.error}/>{action.pending&&<p className="automation-hint">{tr("automation.the_original_operation_still_needs_reconciliation_this_refusal")}</p>}</>} {notice&&<div className="automation-message" role="status">{renderDiagnostic(notice)}<button className="icon-button" aria-label={tr("automation.dismiss_automation_notice")} onClick={()=>setNotice('')}><X size={14}/></button></div>}
  {action.pending&&<section className="automation-recovery panel"><div><strong>{tr("automation.automation_operation_result_is_unconfirmed")}</strong><p>{tr("automation.the_original_request_version_and_operation_id_are")}</p><details><summary>{tr("automation.original_operation_id")}</summary><code>{action.pending.idempotency_key}</code></details></div><div className="automation-actions"><button className="primary" disabled={action.busy} onClick={async()=>{const result=await action.restore();if(result)committed(result,tr("automation.original_receipt_recovered_check_the_official_rule_and"));}}>{action.busy?tr("automation.recovering"):tr("automation.recover_original_receipt")}</button>{!action.deferred&&<button disabled={action.busy} onClick={()=>{action.defer();setDialog(undefined);}}>{tr("automation.keep_and_recover_later")}</button>}</div></section>}
  {action.conflict&&!dialog&&<div className="automation-message warning"><div><strong>{tr("automation.rule_version_or_state_changed")}</strong><p>{tr("automation.load_and_review_the_latest_rule_before_acting")}</p></div><button disabled={detail.loading} onClick={()=>void reloadConflict()}>{tr("automation.load_latest_rule")}</button></div>}
  <div className={'automation-columns '+(selected?'with-detail':'')}>
   <section className="panel automation-list"><div className="list-toolbar"><span>{tr("automation.rules")}<b>{page.items?formatNumber(page.total):'—'}</b></span><div className="inline"><button className="icon-button" aria-label={tr("automation.refresh_automation_rules")} disabled={page.loading} onClick={()=>{void page.read();void detail.read();}}><RefreshCw size={15}/></button><button className="primary" disabled={disabled||!permissions.write} onClick={()=>{action.clear();setEditFresh(undefined);setDialog({kind:'create'});}}><Plus size={14}/>{tr("automation.new_rule")}</button></div></div>
    <div className="automation-filter"><input aria-label={tr("automation.search_loaded_automation_rules")} placeholder={tr("automation.search_loaded_rules")} value={search} onChange={event=>onSearch(event.target.value)}/><small>{tr("automation.loaded_range",{loaded:page.items?formatNumber(page.items.length):'—',total:page.items?formatNumber(page.total):'—'})}</small></div>
    {page.error&&<ErrorNotice error={page.failure||page.error} onRetry={()=>void page.read(page.error_action==='more')}/>}
    {!page.items?<AutomationUnread loading={page.loading}/>:filtered.length?<div className="automation-resources">{filtered.map(value=><button key={value.id} className={'automation-resource '+(selected===value.id?'selected':'')} onClick={()=>{if(canLeave()){action.clear();setNotice('');onSelect(value.id);}}}><span className="row-icon">{value.trigger==='cron'?<Clock3 size={16}/>:<Play size={16}/>}</span><span className="automation-resource-text"><strong>{value.name}</strong><small>{value.trigger==='cron'?value.cron+' · '+value.timezone:tr("automation.manual_trigger")} · v{formatNumber(value.workflow_version)}</small><span className={'automation-pill '+value.state}>{automationStates[value.state]||value.state}</span></span><ChevronRight size={15}/></button>)}</div>:<Empty title={search?tr("automation.no_matching_rules_in_the_loaded_range"):tr("automation.no_automation_rules_yet")} description={search?tr("automation.adjust_the_search_or_load_more_rules"):tr("automation.publish_a_workflow_version_then_create_a_scheduled")}/>}
    <div className="list-footer pagination-footer"><span>{page.items?tr("automation.loaded_range",{loaded:formatNumber(page.items.length),total:formatNumber(page.total)}):tr("automation.no_confirmed_rule_range")}</span>{page.next_cursor&&<button disabled={page.loading} onClick={()=>void page.read(true)}>{page.loading?tr("automation.loading"):tr("automation.load_more_rules")}</button>}</div>
   </section>
   {selected?<section className="panel automation-detail"><header className="automation-detail-top"><button className="text-button" onClick={()=>{if(canLeave())onSelect('');}}>{tr("automation.back_to_rule_list")}</button><button className="icon-button" aria-label={tr("automation.close_automation_details")} onClick={()=>{if(canLeave())onSelect('');}}><X size={17}/></button></header>{detail.error&&<ErrorNotice error={detail.error} onRetry={()=>void detail.read()}/>} {detail.loading&&<p className="automation-read-state" role="status">{tr("automation.loading_rule_details")}</p>}{rule&&<><div className="automation-detail-heading"><div><h2>{rule.name}</h2><p>{tr("automation.the_workflow_version_is_fixed_enable_the_rule")}</p></div><button className="icon-button" aria-label={tr("automation.refresh_automation_details")} disabled={detail.loading} onClick={()=>void detail.read()}><RefreshCw size={15}/></button></div>{detail.error&&<p className="automation-message warning">{tr("automation.the_last_loaded_rule_is_retained_writes_are")}</p>}{selectedOccurrence&&rule.id===selected&&rule.project_id===project&&<AutomationTriggerDetail
     key={JSON.stringify([session.user.id,session.organization.id,project,rule.id,selectedOccurrence])}
     project={project} rule={rule} id={selectedOccurrence} refresh={tick} session={session}
     onClose={onOccurrenceSelect?()=>selectOccurrence(''):undefined}/>}
    <AutomationRuleFacts rule={rule} recoveryAction={editPolicy}/><div className="automation-rule-actions">{!automationFailurePaused(rule)&&editPolicy}{automationEnabled(rule)?<><button disabled={disabled||stale||!permissions.write||action.conflict} onClick={()=>void command(rule,'pause')}><Pause size={14}/>{tr("automation.pause")}</button><button className="primary" disabled={disabled||stale||!permissions.write||action.conflict} onClick={()=>void command(rule,'trigger')}><Play size={14}/>{tr("automation.trigger_once")}</button></>:rule.state!=='archived'&&!automationFailurePaused(rule)&&<button className="primary" disabled={disabled||stale||!permissions.approve||!permissions.write||action.conflict} onClick={()=>{action.clear();setEditFresh(undefined);setDialog({kind:'enable',rule});}}><Play size={14}/>{tr("automation.review_and_enable")}</button>}{rule.state!=='archived'&&<button disabled={disabled||stale||!permissions.write||action.conflict} onClick={()=>{action.clear();setEditFresh(undefined);setDialog({kind:'archive',rule});}}><Archive size={14}/>{tr("automation.archive")}</button>}</div>{!permissions.approve&&rule.state!=='archived'&&!automationEnabled(rule)&&!automationFailurePaused(rule)&&<p className="automation-hint">{tr("automation.enabling_requires_project_approval_permission_contact_a_reviewer")}</p>}<AutomationIngressWorkspace key={session.organization.id+session.user.id+project+rule.id} project={project} session={session} rule={rule} refresh={tick} disabled={disabled||stale} panel={panel} onSelect={onIngressSelect} onRefreshRule={detail.read}/><AutomationSchedulePreview project={project} spec={rule} disabled={stale}/><AutomationOccurrences key={rule.id} project={project} rule={rule.id} refresh={tick} timezone={rule.timezone} activeRevision={automationEnabled(rule)?rule.revision:undefined} onFailurePause={refreshFailurePause} selected={selectedOccurrence} onOpen={onOccurrenceSelect?selectOccurrence:undefined}/><AutomationRevisions key={"revisions:"+rule.id} project={project} rule={rule.id} refresh={tick}/></>}{!rule&&!detail.loading&&!detail.error&&<Empty title={tr("automation.this_rule_has_not_been_loaded")} description={tr("automation.reload_the_rule_within_the_current_project")}/>}</section>:<section className="panel automation-selection"><CalendarClock size={27} strokeWidth={1.4}/><h3>{tr("automation.select_a_rule_to_inspect_its_trigger_basis")}</h3><p>{tr("automation.the_fixed_workflow_missed_run_policy_waiting_reasons")}</p><a href={'?'+new URLSearchParams({project,view:'development',sub:'workflows'})}>{tr("automation.view_workflows")}<ArrowUpRight size={13}/></a></section>}
  </div>
  <Modal open={!!dialog} title={dialog?.kind==='create'?tr("automation.new_automation_rule"):dialog?.kind==='edit'?tr("automation.edit_rule_policy"):dialog?.kind==='enable'?tr("automation.review_and_enable_rule"):tr("automation.archive_automation_rule")} description={dialog?.kind==='enable'?tr("automation.while_enabled_authorization_and_dispatch_are_checked_against"):dialog?.kind==='archive'?tr("automation.archiving_retains_the_rule_and_trigger_history_and"):tr("automation.pin_a_published_workflow_version_new_rules_are")} onClose={()=>{if(canLeave()){setDialog(undefined);setEditFresh(undefined);action.clear();}}}>
   {dialog&&(dialog.kind==='create'||dialog.kind==='edit')&&<AutomationForm project={project} rule={dialog.kind==='edit'?dialog.rule:undefined} fresh={editFresh} action={action} onReload={reloadConflict} onSaved={result=>committed(result,tr("automation.rule_saved_for_review_enable_it_manually_after"))}/>}
   {dialog&&(dialog.kind==='enable'||dialog.kind==='archive')&&<AutomationDecision project={project} kind={dialog.kind} rule={dialog.rule} action={action} onReload={reloadConflict} fresh={editFresh} onSaved={result=>committed(result,dialog.kind==='enable'?tr("automation.rule_enabled_actual_triggers_and_run_results_are"):tr("automation.rule_archived_trigger_history_retained"))}/>}
  </Modal>
 </div>;
}

export function AutomationUnread({loading}:{loading:boolean}){useLocale();return loading?<Loading/>:<div className="automation-read-state"><strong>{tr("automation.rules_have_not_been_loaded")}</strong><p>{tr("automation.there_is_no_confirmed_record_range_reload_to")}</p></div>;}
export function AutomationRuleFacts({rule,recoveryAction}:{rule:AutomationRule;recoveryAction?:ReactNode}){useLocale();return <div className="automation-facts">
 <div className="automation-state-row"><span className={'automation-pill '+rule.state}>{automationStates[rule.state]||rule.state}</span><span>{tr("automation.spec_operation_version",{revision:formatNumber(rule.revision),version:formatNumber(rule.version)})}</span></div>
 {automationFailurePaused(rule)?<div className="automation-blocker" data-automation-failure-paused><div role="status">
  <strong className="automation-failure-pause-title"><TriangleAlert size={15} aria-hidden="true"/>{tr("automation.failure_pause_title")}</strong>
  <p>{tr("automation.failure_pause_recovery")}</p>
 </div>{recoveryAction&&<div className="automation-failure-edit">{recoveryAction}</div>}
 </div>:rule.last_error&&<div className="automation-blocker"><strong>{tr("automation.dispatch_notice")}</strong><AutomationReason reason={rule.last_error} timezone={rule.timezone}/></div>}
 <AutomationSpecification spec={rule}/>
 <dl className="automation-next"><div><dt>{tr("automation.next_nominal_time")}</dt><dd>{automationEnabled(rule)&&rule.trigger==='cron'&&rule.next_at?<AutomationTime value={rule.next_at} timezone={rule.timezone}/>:'—'}</dd></div></dl>
 <details className="automation-technical"><summary>{tr("automation.rule_identity_and_audit_information")}</summary><dl><div><dt>{tr("automation.rules")}</dt><dd>{rule.id}</dd></div><div><dt>{tr("automation.last_activator")}</dt><dd>{rule.activated_by||tr("automation.not_enabled_yet")}</dd></div><div><dt>{tr("automation.created")}</dt><dd>{fullTime(rule.created_at)}</dd></div><div><dt>{tr("automation.updated")}</dt><dd>{fullTime(rule.updated_at)}</dd></div></dl></details>
 </div>;}
function AutomationSpecification({spec}:{spec:AutomationSpec}){useLocale();return <dl><div><dt>{tr("automation.fixed_workflow")}</dt><dd>{spec.workflow_id}<span className="automation-version">v{formatNumber(spec.workflow_version)}</span></dd></div><div><dt>{tr("automation.trigger_method")}</dt><dd>{spec.trigger==='cron'?tr("automation.scheduled_expression",{cron:spec.cron}):tr("automation.manual_triggers_only")}</dd></div><div><dt>{tr("automation.time_zone")}</dt><dd>{spec.timezone}</dd></div><div><dt>{tr("automation.missed_runs")}</dt><dd>{misfireNames[spec.misfire]}{spec.misfire==='catch_up'?tr("automation.catch_up_window",{limit:formatNumber(spec.catch_up_limit)}):''}</dd></div><div><dt>{tr("automation.overlapping_runs")}</dt><dd>{overlapNames[spec.overlap]}</dd></div>
 <div><dt>{tr("automation.failure_pause_policy")}</dt><dd>{spec.failure_pause_threshold?tr("automation.failure_pause_threshold",{countText:formatNumber(spec.failure_pause_threshold)}):tr("automation.failure_pause_off")}</dd></div></dl>;}
function AutomationTime({value,timezone}:{value:string;timezone:string}){useLocale();const moment=automationMoment(value,timezone);return <span className="automation-time"><span>{tr("automation.zoned_time",{time:moment.local,timezone:moment.timezone})}</span><small>UTC {moment.utc}</small></span>;}
function AutomationReason({reason,scheduledAt='',timezone='UTC',source}:{reason:string;scheduledAt?:string;timezone?:string;source?:string}){useLocale();const value=automationReason(reason,scheduledAt,timezone),eventReason=source==='webhook'?ingressReason(reason):undefined;return <div className="automation-reason"><p>{eventReason?.known?eventReason.message:value.message}</p>{value.range&&<p className="automation-reason-window">{tr("automation.window_display",{range:value.range})}</p>}<details><summary>{tr("automation.original_reason")}</summary><code>{value.raw}</code></details></div>;}
function fullTime(value:string){return value?tr("automation.device_time",{time:formatDateTime(value,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false})}):'—';}

function AutomationOccurrences({project,rule,refresh,timezone,activeRevision,onFailurePause,selected,onOpen}:{project:string;rule:string;refresh:number;timezone:string;activeRevision?:number;onFailurePause:()=>void;selected?:string;onOpen?:(id:string)=>void}){
 useLocale();
 const [filter,setFilter]=useState<OccurrenceFilter>('all'),[history,setHistory]=useState(false);
 const page=useAutomationPage<AutomationOccurrence>(project,'automations/'+encodeURIComponent(rule)+'/occurrences',refresh,filter);
 const reportedPause=useRef<number|undefined>(undefined);
 const failurePauseObserved=activeRevision!==undefined&&!page.error&&page.items?.some(occurrence=>occurrence.rule_id===rule&&occurrence.revision===activeRevision&&occurrence.state==='blocked'&&occurrence.reason===automationFailurePauseReason);
 // A durable pause can be observed in history before the rule detail is refreshed.
 useEffect(()=>{
  if(!failurePauseObserved||reportedPause.current===activeRevision)return;
  reportedPause.current=activeRevision;
  onFailurePause();
 },[activeRevision,failurePauseObserved,onFailurePause]);
 useEffect(()=>setHistory(false),[refresh]);
 useEffect(()=>{
  if(history||page.loading)return;
  const timer=setInterval(()=>{if(document.visibilityState!=='hidden')void page.read();},8000);
  return()=>clearInterval(timer);
 },[page.read,page.loading,history]);
 return <section className="automation-occurrences" aria-labelledby="automation-occurrence-heading">
  <div className="automation-section-heading">
   <div><h3 id="automation-occurrence-heading" tabIndex={-1}>{tr("automation.trigger_records")}</h3><p>{history?tr("automation.the_current_range_stays_while_browsing_history_refresh"):tr("automation.trigger_facts_and_run_results_are_retained_separately")}{tr("automation.display_timezone",{timezone})}</p></div>
   <button className="icon-button" aria-label={tr("automation.refresh_trigger_records")} disabled={page.loading} onClick={()=>{setHistory(false);void page.read();}}><RefreshCw size={14}/></button>
  </div>
  <div className="automation-occurrence-filters" role="group" aria-label={tr("automation.occurrence_filter_label")}>
   <button type="button" data-automation-occurrence-filter="all" aria-pressed={filter==='all'} onClick={()=>{if(filter!=='all'){setHistory(false);setFilter('all');}}}>{tr("automation.occurrence_filter_all")}</button>
   <button type="button" data-automation-occurrence-filter="attention" aria-pressed={filter==='attention'} onClick={()=>{if(filter!=='attention'){setHistory(false);setFilter('attention');}}}>{tr("automation.occurrence_filter_attention")}</button>
  </div>
  {page.error&&<ErrorNotice error={page.failure||page.error} onRetry={()=>void page.read(page.error_action==='more')}/>}
  {!page.items?(page.loading?<Loading/>:<p className="automation-read-state">{tr("automation.no_confirmed_trigger_records_have_been_loaded")}</p>):page.items.length?
   <div className="automation-occurrence-list">{page.items.map(occurrence=><AutomationOccurrenceRow key={occurrence.id} occurrence={occurrence} project={project} timezone={timezone} selected={occurrence.id===selected} onOpen={onOpen}/>)}</div>:
   <Empty title={tr(filter==='attention'?"automation.no_attention_records":"automation.no_trigger_records_yet")} description={tr(filter==='attention'?"automation.no_attention_records_hint":"automation.eligible_scheduled_or_manual_triggers_are_retained_here")}/>}
  <div className="automation-occurrence-footer">
   <small aria-live="polite">{tr("automation.loaded_range",{loaded:page.items?formatNumber(page.items.length):'—',total:page.items?formatNumber(page.total):'—'})}</small>
   {page.next_cursor&&<button type="button" disabled={page.loading} onClick={()=>{setHistory(true);void page.read(true);}}>{tr("automation.load_more_triggers")}</button>}
  </div>
 </section>;
}
type TriggerDetailRead={loading:true}|{loading:false;record:AutomationOccurrence}|{loading:false;unavailable:true}|{loading:false;error:string|Error};

function AutomationTriggerDetail({project,rule,id,refresh,session,onClose}:{project:string;rule:AutomationRule;id:string;refresh:number;session:Session;onClose?:()=>void}){
 const api=useApi();
 useLocale();
 const heading=useRef<HTMLHeadingElement>(null);
 const [reload,setReload]=useState(0),[read,setRead]=useState<TriggerDetailRead>({loading:true});
 const userID=session.user.id,organizationID=session.organization.id,ruleID=rule.id;
 // Focus the landing once, while it is stable, so a slow response never
 // steals focus from a user who has continued to another control.
 useLayoutEffect(()=>{heading.current?.focus();},[]);
 useEffect(()=>{
  const controller=new AbortController();
  const path='/automations/'+encodeURIComponent(ruleID)+'/occurrences/'+encodeURIComponent(id)+query(project);
  const headers={'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID};
  setRead({loading:true});
  void (async()=>{
   try{
    const record=await api<AutomationOccurrence>(path,undefined,'GET',headers,controller.signal);
    if(controller.signal.aborted)return;
    if(!record||record.id!==id||record.rule_id!==ruleID||!Number.isSafeInteger(record.revision)||record.revision<1||
       typeof record.source!=='string'||typeof record.state!=='string'||typeof record.scheduled_at!=='string'||
       typeof record.reason!=='string'||typeof record.run_id!=='string'||typeof record.updated_at!=='string'){
     throw new UiError('automation.trigger_detail_unconfirmed');
    }
    setRead({loading:false,record});
   }catch(error){
    if(controller.signal.aborted)return;
    if(error instanceof ApiError&&[401,403,404].includes(error.status)){setRead({loading:false,unavailable:true});return;}
    setRead({loading:false,error:automationError(error)});
   }
  })();
  return ()=>controller.abort();
 },[api,project,ruleID,id,userID,organizationID,refresh,reload]);
 const retry=()=>setReload(value=>value+1);
 return <section id="automation-trigger-detail" className="automation-trigger-detail" aria-labelledby="automation-trigger-detail-heading" aria-busy={read.loading} data-automation-trigger-detail={id}>
  <header className="automation-trigger-detail-header">
   <h3 id="automation-trigger-detail-heading" tabIndex={-1} ref={heading}>{tr('automation.trigger_detail_title')}</h3>
   <div className="inline">
    <button type="button" className="icon-button" aria-label={tr('automation.refresh_trigger_detail')} disabled={read.loading} onClick={retry}><RefreshCw size={15}/></button>
    {onClose&&<button type="button" className="icon-button" aria-label={tr('automation.close_trigger_detail')} onClick={onClose}><X size={16}/></button>}
   </div>
  </header>
  {read.loading?<p className="automation-read-state" role="status">{tr('automation.loading_trigger_detail')}</p>:
   'unavailable' in read?<p className="automation-read-state" role="status">{tr('automation.trigger_detail_unavailable')}</p>:
   'error' in read?<ErrorNotice error={read.error} onRetry={retry}/>:
   <>
    {read.record.revision!==rule.revision&&<p className="automation-trigger-history-hint">{tr('automation.historical_trigger_revision',{trigger:formatNumber(read.record.revision),current:formatNumber(rule.revision)})}</p>}
    <AutomationOccurrenceRow occurrence={read.record} project={project} timezone={rule.timezone} detail/>
   </>}
 </section>;
}

function runHref(project:string,run:string){return '?'+new URLSearchParams({project,view:'development',sub:'runs',run});}

function validPauseEvidence(occurrence:AutomationOccurrence){
 const evidence=occurrence.failure_pause_evidence;
 if(!evidence||occurrence.run_id||!Number.isInteger(evidence.threshold)||evidence.threshold<2||evidence.threshold>10||
    typeof evidence.observed_at!=='string'||evidence.observed_at.length!==20||evidence.observed_at!==occurrence.updated_at||
    !Array.isArray(evidence.failed_runs)||evidence.failed_runs.length!==evidence.threshold)return undefined;
 const observedAt=Date.parse(evidence.observed_at);
 if(!Number.isFinite(observedAt)||new Date(observedAt).toISOString().replace('.000Z','Z')!==evidence.observed_at)return undefined;
 const validID=(id:unknown):id is string=>typeof id==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id);
 const runs=new Set<string>(),triggers=new Set<string>();
 for(const entry of evidence.failed_runs){
  if(!entry||!validID(entry.run_id)||!validID(entry.trigger_id)||entry.trigger_id===occurrence.id||
     runs.has(entry.run_id)||triggers.has(entry.trigger_id))return undefined;
  runs.add(entry.run_id);triggers.add(entry.trigger_id);
 }
 return evidence;
}

function AutomationPauseEvidence({occurrence,project,timezone}:{occurrence:AutomationOccurrence;project:string;timezone:string}){
 const evidence=validPauseEvidence(occurrence);
 if(!evidence)return <p className="automation-pause-evidence-unavailable" data-automation-pause-evidence-unavailable>{tr('automation.pause_evidence_unavailable')}</p>;
 const observed=automationMoment(evidence.observed_at,timezone);
 return <section className="automation-pause-evidence" aria-labelledby="automation-pause-evidence-heading" data-automation-pause-evidence>
  <h4 id="automation-pause-evidence-heading">{tr('automation.pause_evidence_title')}</h4>
  <p>{tr('automation.pause_evidence_threshold',{threshold:formatNumber(evidence.threshold)})}</p>
  <time dateTime={evidence.observed_at}>{tr('automation.pause_evidence_observed',{time:observed.local,timezone:observed.timezone})}</time>
  <ol aria-label={tr('automation.pause_evidence_failed_runs')}>
   {evidence.failed_runs.map(entry=><li key={entry.run_id}>
    <a href={runHref(project,entry.run_id)} data-automation-pause-evidence-run={entry.run_id}
       aria-label={tr('automation.pause_evidence_open_run_named',{id:entry.run_id})}
       onClick={event=>{if(!canLeave())event.preventDefault();}}>
     {tr('automation.pause_evidence_open_run')}<code title={entry.run_id}>{entry.run_id.slice(0,16)}</code><ArrowUpRight size={13} aria-hidden="true"/>
    </a>
   </li>)}
  </ol>
 </section>;
}

export function AutomationOccurrenceRow({occurrence,project,timezone='UTC',selected,onOpen,detail=false}:{occurrence:AutomationOccurrence;project:string;timezone?:string;selected?:boolean;onOpen?:(id:string)=>void;detail?:boolean}){
 useLocale();
 return <article className="automation-occurrence" data-automation-trigger-record={detail?occurrence.id:undefined}>
  <div className="automation-occurrence-title">
   <strong>{occurrence.source==='manual'?tr("automation.manual_trigger"):occurrence.source==='webhook'?tr("automation.signed_event_trigger"):tr("automation.scheduled_trigger")} <small>r{formatNumber(occurrence.revision)}</small></strong>
   <span className={'automation-pill '+occurrence.state}>{occurrenceStates[occurrence.state]||occurrence.state}</span>
  </div>
  <time dateTime={occurrence.scheduled_at}><AutomationTime value={occurrence.scheduled_at} timezone={timezone}/></time>
  {occurrence.reason&&<AutomationReason reason={occurrence.reason} scheduledAt={occurrence.scheduled_at} timezone={timezone} source={occurrence.source}/>}
  {detail&&occurrence.state==='blocked'&&occurrence.reason===automationFailurePauseReason&&<AutomationPauseEvidence occurrence={occurrence} project={project} timezone={timezone}/>}
  {occurrence.run_id?<><a href={runHref(project,occurrence.run_id)} onClick={event=>{if(!canLeave())event.preventDefault();}}>{tr("automation.view_actual_run")}<ArrowUpRight size={13}/><span>{occurrence.run_id.slice(0,16)}</span></a><small className="automation-run-state">{occurrence.run_state?tr("automation.run_state",{state:stateLabel(occurrence.run_state)}):tr("automation.run_state_has_not_been_loaded")}</small>{occurrence.run_needs_reconciliation&&<small className="automation-run-reconciliation"><TriangleAlert size={14} aria-hidden="true"/>{tr("automation.run_needs_reconciliation")}</small>}</>:<small className="muted">{tr("automation.no_run_linked_yet")}</small>}
  {detail&&<small className="automation-trigger-updated">{tr('automation.trigger_detail_updated',{time:automationMoment(occurrence.updated_at,timezone).local,timezone})}</small>}
  <div className="automation-trigger-row-actions">
   <details><summary>{tr("automation.trigger_id")}</summary><code>{occurrence.id}</code></details>
   {onOpen&&<button type="button" className="automation-open-trigger" data-automation-open-trigger={occurrence.id}
   aria-label={tr('automation.open_trigger_detail_named',{id:occurrence.id})} aria-expanded={!!selected} aria-controls={selected?'automation-trigger-detail':undefined}
   onClick={()=>onOpen(occurrence.id)}>{tr('automation.open_trigger_detail')}<ArrowUpRight size={13} aria-hidden="true"/></button>}
  </div>
 </article>;
}

function AutomationRevisions({project,rule,refresh}:{project:string;rule:string;refresh:number}){useLocale();
 const pager=useMemo(()=>new AutomationRevisionPager(project,rule),[project,rule]),[page,setPage]=useState(()=>pager.snapshot()),[open,setOpen]=useState(false);
 const read=useCallback(async(more=false)=>{const request=more?pager.more():pager.first();setPage(pager.snapshot());await request;setPage(pager.snapshot());},[pager]);
 useEffect(()=>{if(open)void read();},[read,refresh,open]);
 return <details className="automation-history" open={open} onToggle={event=>setOpen(event.currentTarget.open)}><summary>{tr("automation.specification_revision_history")}</summary>{open&&<><p className="automation-hint">{tr("automation.each_edit_retains_an_independent_read_only_specification")}</p>{page.error&&<ErrorNotice error={page.error} onRetry={()=>void read(page.more_error)}/>} {!page.items?(page.loading?<Loading/>:<p className="automation-read-state">{tr("automation.no_confirmed_revision_range_has_been_loaded")}</p>):page.items.length?<div className="automation-history-list">{page.items.map(revision=><AutomationRevisionFacts key={revision.revision} revision={revision}/>)}</div>:<Empty title={tr("automation.no_specification_revisions_loaded")} description={tr("automation.reload_the_server_revision_history")}/>}<div className="automation-occurrence-footer"><small>{tr("automation.loaded_immutable_revisions",{count:page.total,loaded:page.items?formatNumber(page.items.length):'—',total:page.items?formatNumber(page.total):'—'})}</small>{page.next_cursor&&<button type="button" disabled={page.loading} onClick={()=>void read(true)}>{tr("automation.load_earlier_revisions")}</button>}</div></>}</details>;
}
export function AutomationRevisionFacts({revision}:{revision:AutomationRevision}){useLocale();return <article className="automation-history-item automation-facts"><div className="automation-history-title"><strong>r{formatNumber(revision.revision)} · {revision.spec.name}</strong><time dateTime={revision.created_at}><AutomationTime value={revision.created_at} timezone="UTC"/></time></div><AutomationSpecification spec={revision.spec}/></article>;}

function AutomationSchedulePreview({project,spec,disabled=false}:{project:string;spec:AutomationSpec;disabled?:boolean}){ const api=useApi();useLocale();
 const[preview,setPreview]=useState<AutomationPreview>(),[error,setError]=useState<string|Error>(''),[busy,setBusy]=useState(false),epoch=useRef(0);const signature=JSON.stringify(spec);
 useEffect(()=>{epoch.current++;setPreview(undefined);setError('');setBusy(false);},[signature,project]);
 async function load(){const generation=++epoch.current;setBusy(true);setError('');try{const body=automationSpec(spec),result=await api<AutomationPreview>('/automations/preview',{project_id:project,spec:body});if(generation===epoch.current)setPreview(result);}catch(error){if(generation===epoch.current)setError(automationError(error));}finally{if(generation===epoch.current)setBusy(false);}}
 return <section className="automation-preview"><div className="automation-section-heading"><div><h3>{tr("automation.upcoming_trigger_preview")}</h3><p>{tr("automation.calculated_from_the_server_s_current_time_no")}</p></div><button type="button" disabled={disabled||busy||spec.trigger==='manual'} onClick={()=>void load()}>{busy?tr("automation.calculating"):tr("automation.preview_next_5_triggers")}</button></div>{spec.trigger==='manual'?<p className="automation-hint">{tr("automation.manual_rules_have_no_scheduled_times_trigger_once")}</p>:<><p className="automation-hint">{tr("automation.preview_timezone",{timezone:spec.timezone})}</p>{error&&<ErrorNotice error={error}/>} {preview&&<ol>{preview.items.map((item,index)=><li key={item.utc+'-'+index}><AutomationTime value={item.utc} timezone={spec.timezone}/></li>)}</ol>}</>}</section>;
}

function FormNotice({action,onReload,onRecovered}:{action:WriteAction;onReload:()=>Promise<void>;onRecovered:(result:AutomationRule|AutomationResult)=>void}){useLocale();return <>{action.error&&<><ErrorNotice error={action.error}/>{action.pending&&<p className="automation-hint">{tr("automation.the_original_operation_still_needs_reconciliation_this_refusal")}</p>}</>} {action.pending&&<div className="automation-message warning"><span>{action.deferred?tr("automation.the_original_operation_is_retained_close_this_dialog"):tr("automation.the_request_result_is_unconfirmed_the_complete_draft")}</span><button type="button" disabled={action.busy} onClick={async()=>{const result=await action.restore();if(result)onRecovered(result);}}>{action.busy?tr("automation.recovering"):tr("automation.recover_original_receipt")}</button>{!action.deferred&&<button type="button" disabled={action.busy} onClick={action.defer}>{tr("automation.keep_and_recover_later")}</button>}</div>}{action.conflict&&<div className="automation-message warning"><div><strong>{tr("automation.the_server_rule_changed_your_draft_is_retained")}</strong><p>{tr("automation.load_and_review_the_latest_rule_then_save")}</p></div><button type="button" disabled={action.busy} onClick={()=>void onReload()}>{tr("automation.load_latest_rule_and_keep_draft")}</button></div>}</>;}
function AutomationForm({project,rule,fresh,action,onReload,onSaved}:{project:string;rule?:AutomationRule;fresh?:AutomationRule;action:WriteAction;onReload:()=>Promise<void>;onSaved:(result:AutomationRule|AutomationResult)=>void}){ const api=useApi();useLocale();
 const[spec,setSpec]=useState<AutomationSpec>(()=>rule?automationSpec(rule):newAutomationSpec()),[version,setVersion]=useState(rule?.version||0),[workflows,setWorkflows]=useState<Workflow[]>(),[workflowError,setWorkflowError]=useState<string|Error>(''),[lookupBusy,setLookupBusy]=useState(false),[formError,setFormError]=useState<string|Error>(''),[dirty,setDirty]=useState(false);useLeaveGuard(dirty&&!action.pending&&!action.busy,action.setError);
 const locked=action.busy||!!action.pending,archived=fresh?.state==='archived';
 const loadWorkflows=useCallback(async()=>{setLookupBusy(true);try{const result=await api<{items:Workflow[]}>('/workflows'+query(project));if(!Array.isArray(result.items))throw new UiError("automation.the_workflow_list_has_not_returned_a_confirmed");setWorkflows(automationWorkflowOptions(result.items));setWorkflowError('');}catch(error){setWorkflowError(automationError(error));}finally{setLookupBusy(false);}},[project]);
 useEffect(()=>{void loadWorkflows();},[loadWorkflows]);useEffect(()=>{if(fresh)setVersion(fresh.version);},[fresh]);
 function update<K extends keyof AutomationSpec>(key:K,value:AutomationSpec[K]){setSpec(before=>({...before,[key]:value}));setDirty(true);setFormError('');}
 async function save(event:FormEvent){event.preventDefault();if(locked||action.conflict)return;setFormError('');let value:AutomationSpec;try{value=automationSpec(spec);}catch(error){setFormError(automationError(error));return;}const body=rule?{project_id:project,action:'revise',expected_version:version,spec:value}:{project_id:project,spec:value};const result=await action.run<AutomationRule|AutomationResult>(rule?'/automations/'+encodeURIComponent(rule.id)+'/commands':'/automations',body);if(result){setDirty(false);onSaved(result);}}
 const selectedWorkflow=workflows?.find(item=>item.id===spec.workflow_id),options=workflows?.filter((item,index,rows)=>rows.findIndex(other=>other.id===item.id)===index);
 return <form className="automation-form" onSubmit={event=>void save(event)}><FormNotice action={action} onReload={onReload} onRecovered={onSaved}/>{formError&&<ErrorNotice error={formError}/>} {archived&&<p className="automation-message warning">{tr("automation.the_current_rule_is_archived_its_policy_cannot")}</p>}{fresh&&<div className="automation-current-basis"><strong>{tr("automation.latest_rule_name",{name:fresh.name})}</strong><span>{automationStates[fresh.state]||fresh.state} · r{formatNumber(fresh.revision)} / v{formatNumber(fresh.version)}</span><p>{tr("automation.latest_rule_spec",{workflow:fresh.workflow_id,version:formatNumber(fresh.workflow_version),trigger:fresh.trigger==='cron'?fresh.cron:tr("automation.manual_trigger"),misfire:misfireNames[fresh.misfire],overlap:overlapNames[fresh.overlap]})}</p></div>}
  <fieldset className="form-fields" disabled={locked||archived}>
   <Field label={tr("automation.rule_name")} hint={tr("automation.keep_the_name_short_at_most_200_utf")}><input value={spec.name} required maxLength={200} onChange={event=>update('name',event.target.value)} placeholder={tr("automation.e_g_weekday_code_checks")}/></Field>
   <div className="automation-form-grid"><Field label={tr("automation.published_workflow")} hint={tr("automation.protected_service_release_workflows_cannot_be_used_for")}><select value={spec.workflow_id} required onChange={event=>{const value=workflows?.find(item=>item.id===event.target.value);setSpec(before=>({...before,workflow_id:event.target.value,workflow_version:value?.version||0}));setDirty(true);}}><option value="">{lookupBusy?tr("automation.loading_workflows"):tr("automation.select_an_actual_workflow")}</option>{spec.workflow_id&&!options?.some(item=>item.id===spec.workflow_id)&&<option value={spec.workflow_id}>{tr("automation.fixed_workflow_reference",{id:spec.workflow_id})}</option>}{options?.map(value=><option key={value.id} value={value.id}>{value.name}</option>)}</select></Field><Field label={tr("automation.fixed_workflow_version")} hint={selectedWorkflow?tr("automation.latest_workflow_version",{version:formatNumber(selectedWorkflow.version)}):tr("automation.the_server_verifies_that_this_exact_version_exists")}><input type="number" min={1} step={1} required value={spec.workflow_version||''} onChange={event=>update('workflow_version',Number(event.target.value))}/></Field></div>
   {workflowError&&<ErrorNotice error={workflowError} onRetry={()=>void loadWorkflows()}/>} {workflows?.length===0&&<p className="automation-hint">{tr("automation.no_workflows_are_available_publish_a_version_on")}</p>}
   <div className="automation-form-grid"><Field label={tr("automation.trigger_method")}><select value={spec.trigger} onChange={event=>update('trigger',event.target.value as AutomationSpec['trigger'])}><option value="cron">{tr("automation.scheduled_trigger")}</option><option value="manual">{tr("automation.manual_triggers_only")}</option></select></Field><Field label={tr("automation.iana_time_zone")}><input value={spec.timezone} list="automation-timezones" required onChange={event=>update('timezone',event.target.value)}/><datalist id="automation-timezones"><option value="Asia/Shanghai"/><option value="UTC"/><option value="America/New_York"/><option value="Europe/London"/><option value="Asia/Tokyo"/></datalist></Field></div>
   {spec.trigger==='cron'&&<Field label={tr("automation.cron_expression")} hint={tr("automation.minute_hour_day_month_weekday_use_numbers_asterisks")}><input value={spec.cron} required maxLength={160} spellCheck={false} onChange={event=>update('cron',event.target.value)} placeholder="0 9 * * 1-5"/></Field>}
   <Field label={tr("automation.failure_pause_policy")} hint={tr("automation.failure_pause_hint")}>
    <select name="failure_pause_threshold" value={spec.failure_pause_threshold??0} onChange={event=>update('failure_pause_threshold',Number(event.target.value))}>
     <option value={0}>{tr("automation.failure_pause_off")}</option>
     {Array.from({length:9},(_,index)=>index+2).map(threshold=><option value={threshold} key={threshold}>{tr("automation.failure_pause_threshold",{countText:formatNumber(threshold)})}</option>)}
    </select>
   </Field>
   <div className="automation-form-grid"><Field label={tr("automation.missed_run_policy")}><select value={spec.misfire} onChange={event=>update('misfire',event.target.value as AutomationSpec['misfire'])}>{Object.entries(misfireNames).map(([key,value])=><option value={key} key={key}>{value}</option>)}</select></Field><Field label={tr("automation.catch_up_limit")} hint={tr("automation.catch_up_1_100_times_at_most_per")}><input type="number" min={1} max={100} step={1} required disabled={spec.misfire!=='catch_up'} value={spec.catch_up_limit} onChange={event=>update('catch_up_limit',Number(event.target.value))}/></Field></div>
   <Field label={tr("automation.when_the_previous_run_is_unfinished")}><select value={spec.overlap} onChange={event=>update('overlap',event.target.value as AutomationSpec['overlap'])}>{Object.entries(overlapNames).map(([key,value])=><option value={key} key={key}>{value}</option>)}</select></Field>
  </fieldset>
  <AutomationSchedulePreview project={project} spec={spec} disabled={locked}/>
  <p className="automation-hint">{rule&&automationEnabled(rule)?tr("automation.saving_pauses_the_current_rule_review_the_new"):tr("automation.rules_are_saved_as_drafts_or_paused_enabling")} {tr("automation.this_rule_does_not_replace_node_approval_task")}</p>
  <div className="dialog-actions">{dirty&&!locked&&<button type="button" onClick={()=>{setDirty(false);setSpec(rule?automationSpec(rule):newAutomationSpec());setFormError('');}}>{tr("automation.discard_current_input")}</button>}<button className="primary" disabled={locked||action.conflict||archived||!spec.workflow_id}>{action.busy?tr("automation.saving"):rule?tr("automation.save_and_pause_rule"):tr("automation.save_draft")}</button></div>
 </form>;
}
function AutomationDecision({project,kind,rule,fresh,action,onReload,onSaved}:{project:string;kind:'enable'|'archive';rule:AutomationRule;fresh?:AutomationRule;action:WriteAction;onReload:()=>Promise<void>;onSaved:(result:AutomationResult)=>void}){useLocale();
 const[checked,setChecked]=useState(false);const value=fresh||rule,disabled=action.busy||!!action.pending||action.conflict||value.state==='archived'||(kind==='enable'&&automationFailurePaused(value));
 useEffect(()=>setChecked(false),[fresh]);
 return <form className="automation-form" onSubmit={async event=>{event.preventDefault();if(disabled||!checked)return;const result=await action.run<AutomationResult>('/automations/'+encodeURIComponent(value.id)+'/commands',{project_id:project,action:kind,expected_version:value.version});if(result)onSaved(result);}}><FormNotice action={action} onReload={onReload} onRecovered={result=>onSaved('rule'in result?result:{rule:result})}/><AutomationRuleFacts rule={value}/><label className="automation-check"><input type="checkbox" disabled={disabled} checked={checked} onChange={event=>setChecked(event.target.checked)}/><span>{kind==='enable'?tr("automation.i_have_checked_the_fixed_workflow_version_time"):tr("automation.archive_this_rule_stop_new_dispatches_and_retain")}</span></label><div className="dialog-actions"><button className="primary" disabled={disabled||!checked}>{action.busy?tr("automation.submitting"):kind==='enable'?tr("automation.enable_this_rule"):tr("automation.confirm_archive")}</button></div></form>;
}
