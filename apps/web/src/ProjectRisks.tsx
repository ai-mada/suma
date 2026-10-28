import {useCallback,useEffect,useLayoutEffect,useRef,useState,type MouseEvent} from 'react';
import {ArrowLeft,ArrowUpRight,Plus,RefreshCw,ShieldAlert} from 'lucide-react';
import type {PlanningAlert,ProjectRisk,ProjectRiskFields,ProjectRiskPage,ProjectRiskReview,ProjectRiskReviewInput,ProjectRiskSummary,ProjectRiskView} from '../../../packages/contracts/src';
import {ApiError,label,query,type RecordItem,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {Empty,ErrorNotice,Field,Loading,Modal} from './ui';
import {useLeaveGuard,useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteAction} from './write-action';
import {blankRiskFields,riskCommands,riskDispositions,riskFields,riskHref,riskID,riskObject,riskPath,riskRecordHref,riskRunHref,riskStates,riskStrategies,riskText,validRiskFields,validRiskPage,validRiskRequest,validRiskResponse,validRiskReviewInput,validRiskSummary,validRiskView,type RiskCommand} from './project-risks-model';
import './ProjectRisks.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('project-risks.'+key,values);
const canNavigate=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));
const accessLost=(value:unknown)=>value instanceof ApiError&&[401,403,404].includes(value.status);
const scopeOf=(session:Session,project:string)=>JSON.stringify([session.user.id,session.organization.id,project]);
type ReadState<T>={value?:T;loading:boolean;error?:unknown};

/** A read holds one scope only. Refresh, locale and account changes cannot
 * promote an old response into another risk; denied reads drop its facts. */
function useRiskRead<T>(path:string|undefined,refresh:number,validate:(value:unknown)=>value is T,onDenied?:()=>void){
 const api=useApi(),[state,setState]=useState<ReadState<T>&{path?:string;api?:typeof api}>({loading:false}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),denied=useRef(onDenied);denied.current=onDenied;
 const read=useCallback(async()=>{
  if(!path)return;const current=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;setState({path,api,loading:true});
  try{const value=await api<unknown>(path,undefined,'GET',undefined,abort.signal);if(abort.signal.aborted||current!==epoch.current)return;if(!validate(value))throw new UiError('project-risks.invalid_read');setState({path,api,value,loading:false});return value;}
  catch(error){if(!abort.signal.aborted&&current===epoch.current){setState({path,api,loading:false,error});if(accessLost(error))denied.current?.();}}
 },[api,path,validate]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();};},[read,refresh]);
 const clear=useCallback(()=>{epoch.current++;controller.current?.abort();setState({path,api,loading:false});},[path,api]);
 const install=(value:T)=>setState({path,api,value,loading:false});
 const visible:ReadState<T>=state.path===path&&state.api===api?state:{loading:!!path};
 return {...visible,read,clear,install};
}
const projectPage=(project:string,state='',before='',limit=20)=>riskPath(project)+'&limit='+limit+(state?'&state='+state:'')+(before?'&before='+encodeURIComponent(before):'');

export function ProjectRiskSummaryCard({project,refresh,onOpen}:{project:string;refresh:number;onOpen:(id?:string)=>void}){
 useLocale();const [tick,setTick]=useState(0),valid=useCallback((value:unknown):value is ProjectRiskSummary=>validRiskSummary(value,project),[project]),read=useRiskRead('/risks/summary'+query(project),refresh+tick,valid);
 return <section className="pr-summary" aria-label={t('summary_title')} aria-busy={read.loading}>
  <div className="pr-heading"><h2><ShieldAlert size={16} aria-hidden="true"/> {t('summary_title')}</h2><button type="button" onClick={()=>onOpen()}>{t('open_register')}<ArrowUpRight size={13}/></button></div>
  {read.loading&&<Loading/>}{!!read.error&&<ErrorNotice error={toolError(read.error)} onRetry={()=>setTick(n=>n+1)}/>}
  {read.value&&<><p className="pr-copy">{t('summary_counts',{active:formatNumber(read.value.active_count),overdue:formatNumber(read.value.overdue_count),unknown:formatNumber(read.value.unassessed_count)})}</p><p className="pr-muted">{t('summary_scope',{at:formatDateTime(read.value.as_of)})}</p></>}
 </section>;
}

type Props={session:Session;project:string;selected:string;onSelect:(id:string)=>void;refresh:number;onChanged:()=>void;onOpenTask?:(id:string)=>void};
export function ProjectRisks(props:Props){return <RiskWorkspace key={scopeOf(props.session,props.project)} {...props}/>;}
function RiskWorkspace({session,project,selected,onSelect,refresh,onChanged,onOpenTask}:Props){
 useLocale();const [state,setState]=useState(''),[before,setBefore]=useState(''),[pageNumber,setPageNumber]=useState(1),[creating,setCreating]=useState(false),[denied,setDenied]=useState(false),[tick,setTick]=useState(0),[notice,setNotice]=useState('');
 const fail=useCallback(()=>setDenied(true),[]),valid=useCallback((value:unknown):value is ProjectRiskPage=>validRiskPage(value,project),[project]),page=useRiskRead(denied?undefined:projectPage(project,state,before),refresh+tick,valid,fail);
 useLayoutEffect(()=>{if(denied)page.clear();},[denied,page.clear]);
 function saved(value:ProjectRiskView){setCreating(false);setNotice(value.created===false?'existing':'saved');setBefore('');setPageNumber(1);onSelect(value.risk.id);onChanged();}
 return <section className="project-risks" aria-label={t('title')}>
  <div className="pr-heading"><div><h2>{t('title')}</h2><p>{t('intro')}</p></div>{!denied&&page.value?.can_create&&<button type="button" className="primary" data-risk-new onClick={()=>setCreating(true)}><Plus size={14}/>{t('create')}</button>}</div>
  {notice&&<p className="pr-success" role="status">{t(notice)}</p>}
  {denied?<ErrorNotice error={new UiError('project-risks.access_changed')} onRetry={()=>{setDenied(false);setTick(n=>n+1);}}/>:<>
   <div className={'pr-layout'+(selected?' has-detail':'')}>
    <div className="pr-list-region"><div className="pr-toolbar"><Field label={t('filter')}><select data-risk-filter value={state} onChange={event=>{setState(event.target.value);setBefore('');setPageNumber(1);}}><option value="">{t('all_states')}</option>{riskStates.map(value=><option value={value} key={value}>{t('state.'+value)}</option>)}</select></Field><button type="button" disabled={page.loading} aria-label={t('refresh')} onClick={()=>{setBefore('');setPageNumber(1);setTick(n=>n+1);}}><RefreshCw size={14}/></button></div>
     {page.loading&&<Loading/>}{!!page.error&&<ErrorNotice error={toolError(page.error)} onRetry={()=>void page.read()}/>}
     {page.value&&<><p className="pr-muted">{t('page_scope',{page:formatNumber(pageNumber),loaded:formatNumber(page.value.items.length),total:formatNumber(page.value.total)})}</p><div className="pr-list">{page.value.items.map(risk=><button type="button" className="pr-item" key={risk.id} data-risk-select={risk.id} aria-pressed={selected===risk.id} onClick={()=>{if(canNavigate()){setNotice('');onSelect(risk.id);}}}><strong>{risk.title}</strong><span className={'pr-state state-'+risk.state}>{t('state.'+risk.state)}</span><small>{t('list_meta',{score:risk.score===null?t('unknown'):formatNumber(risk.score),date:risk.review_date||t('not_scheduled')})}</small></button>)}</div>{!page.value.items.length&&<Empty title={t('empty')} description={t('empty_hint')}/>}{page.value.next_cursor&&<button type="button" disabled={page.loading} onClick={()=>{setBefore(page.value!.next_cursor);setPageNumber(n=>n+1);}}>{t('next')}</button>}</>}
    </div>
    {selected&&<RiskDetail key={selected} session={session} project={project} id={selected} refresh={refresh+tick} onBack={()=>{if(canNavigate())onSelect('');}} onChanged={onChanged} onOpenTask={onOpenTask} onDenied={fail}/>}
   </div>
   {creating&&<RiskCreateEditor session={session} project={project} onSaved={saved} onClose={()=>setCreating(false)} onDenied={fail}/>}
  </>}
 </section>;
}

const blockers=new Set(['risk_owner_required','risk_owner_unavailable','risk_assessment_incomplete','risk_links_changed','risk_task_not_accepted','risk_task_evidence_stale','risk_execution_source_unsupported','risk_review_missing','risk_review_changed','risk_closed','risk_wrong_state']);
function RiskBlockers({view}:{view:ProjectRiskView}){return view.readiness.blockers.length?<ul className="pr-blockers" data-risk-blockers>{view.readiness.blockers.map((item,index)=><li key={item.code+':'+index}>{blockers.has(item.code)?t('blocker.'+item.code):t('unknown_blocker')}{item.ref_id&&<> · <code>{item.ref_id}</code></>}<details><summary>{t('original_code')}</summary><code>{item.code}</code></details></li>)}</ul>:null;}
function RiskDetail({session,project,id,refresh,onBack,onChanged,onOpenTask,onDenied}:{session:Session;project:string;id:string;refresh:number;onBack:()=>void;onChanged:()=>void;onOpenTask?:Props['onOpenTask'];onDenied:()=>void}){
 useLocale();const valid=useCallback((value:unknown):value is ProjectRiskView=>validRiskView(value,project,id),[project,id]),read=useRiskRead(riskPath(project,id),refresh,valid,onDenied),[editing,setEditing]=useState<ProjectRisk>(),[command,setCommand]=useState<{action:RiskCommand;view:ProjectRiskView}>(),[notice,setNotice]=useState('');
 const action=useWriteAction({scope:'project-risk:'+scopeOf(session,project)+':'+id,storage:toolSessionStorage(),validateRequest:request=>validRiskRequest(request,project,id),validateResponse:(value,request)=>validRiskResponse(value,request,project,id)}),lost=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(lost)onDenied();},[lost,onDenied]);
 const view=read.value,risk=view?.risk;
 function saved(value:ProjectRiskView|undefined){if(!value)return;read.install(value);setEditing(undefined);setCommand(undefined);setNotice('saved');onChanged();}
 function follow(event:MouseEvent<HTMLAnchorElement>,kind:string,record:string){if(event.defaultPrevented||event.button!==0||event.ctrlKey||event.metaKey||event.altKey||event.shiftKey)return;if(!canNavigate()){event.preventDefault();return;}if(kind==='task'&&onOpenTask){event.preventDefault();onOpenTask(record);}}
 const allowed=(action:string)=>!!view?.allowed_actions.includes(action),ready=(action:RiskCommand)=>action==='close'?!!view?.readiness.close_ready:action==='start'||action==='submit_review'?!!view?.readiness.assessment_ready:true;
 return <aside className="pr-detail" aria-label={t('details')} aria-busy={read.loading}>
  <div className="pr-actions"><button type="button" className="pr-back" onClick={onBack}><ArrowLeft size={14}/>{t('back')}</button><button type="button" disabled={read.loading} onClick={()=>void read.read()}><RefreshCw size={13}/>{t('refresh')}</button></div>
  {read.loading&&<Loading/>}{!!read.error&&<ErrorNotice error={toolError(read.error)} onRetry={()=>void read.read()}/>}{notice&&<p className="pr-success" role="status">{t(notice)}</p>}
  {!editing&&!command&&<><WriteNotice action={action}/><Recovery action={action} disabled={lost} onCompleted={async value=>{if(value){saved(value);await read.read();}}}/></>}
  {view&&risk&&<>
   <div className="pr-heading"><h3>{risk.title}</h3><span className={'pr-state state-'+risk.state}>{t('state.'+risk.state)}</span></div><p className="pr-copy">{risk.detail}</p>
   <dl className="pr-facts"><dt>{t('owner')}</dt><dd><RiskOwnerLabel project={project} id={risk.owner_id} refresh={refresh}/></dd><dt>{t('review_date')}</dt><dd>{risk.review_date||t('not_scheduled')}</dd><dt>{t('strategy')}</dt><dd>{t('strategy.'+(risk.strategy||'unknown'))}</dd><dt>{t('score')}</dt><dd>{risk.score===null?t('unknown'):formatNumber(risk.score)} <span className="pr-muted">{t('score_hint')}</span></dd></dl>
   {risk.plan&&<section className="pr-section"><h4>{t('plan')}</h4><p className="pr-copy">{risk.plan}</p></section>}
   <RiskBlockers view={view}/>
   <div className="pr-actions">{allowed('save')&&<button type="button" data-risk-edit disabled={action.busy||!!action.pending} onClick={()=>setEditing(risk)}>{t('edit')}</button>}{riskCommands.filter(command=>allowed(command)).map(command=><button type="button" key={command} data-risk-command={command} disabled={!ready(command)||action.busy||!!action.pending} className={command==='close'?'primary':undefined} onClick={()=>setCommand({action:command,view})}>{t('action.'+command)}</button>)}</div>
   <section className="pr-section"><h4>{t('links')}</h4><p className="pr-muted">{t('links_hint')}</p><div className="pr-links">{risk.links.map(link=><div className="pr-link" key={link.kind+':'+link.id}><a href={riskRecordHref(project,link.kind,link.id)} onClick={event=>follow(event,link.kind,link.id)}>{t('open_'+link.kind)}<ArrowUpRight size={12}/></a><small><code>{link.id}</code></small></div>)}</div>{!risk.links.length&&<p className="pr-muted">{t('no_links')}</p>}</section>
   {view.current_review&&<ReviewFacts review={view.current_review} project={project}/>}
   <details className="pr-section"><summary>{t('basis')}</summary><dl className="pr-facts"><dt>{t('risk_id')}</dt><dd><code>{risk.id}</code></dd><dt>{t('version')}</dt><dd>v{formatNumber(risk.version)} · {t('assessment_revision',{value:formatNumber(risk.assessment_revision)})}</dd><dt>{t('cycle')}</dt><dd>{formatNumber(risk.cycle)}</dd><dt>{t('probability')}</dt><dd>{risk.probability===null?t('unknown'):formatNumber(risk.probability)}</dd><dt>{t('impact')}</dt><dd>{risk.impact===null?t('unknown'):formatNumber(risk.impact)}</dd><dt>{t('updated_at')}</dt><dd>{formatDateTime(risk.updated_at)}</dd></dl>{risk.source&&<><p className="pr-muted">{t('source_boundary')}</p><a href={riskRunHref(project,risk.source.run_id)} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_source_run')}<ArrowUpRight size={12}/></a><dl className="pr-facts"><dt>{t('source_alert')}</dt><dd><code>{risk.source.alert_id}</code> · v{formatNumber(risk.source.alert_version)}</dd><dt>{t('source_sha')}</dt><dd><code>{risk.source.source_sha256}</code></dd><dt>{t('output_sha')}</dt><dd><code>{risk.source.original_output_sha256}</code></dd></dl></>}</details>
   {!!view.history.length&&<details className="pr-section"><summary>{t('history',{count:formatNumber(view.history_total)})}</summary><div className="pr-history">{view.history.map(review=><ReviewFacts key={review.id} review={review} project={project}/>)}</div>{!view.history_complete&&<p className="pr-muted">{t('history_limited')}</p>}</details>}
  </>}
  {editing&&<RiskEditor key={'edit:'+editing.id} action={action} session={session} project={project} risk={editing} onSaved={saved} onClose={()=>setEditing(undefined)} onDenied={onDenied} readLatest={read.read}/>}
  {command&&<RiskCommandDialog key={command.action} action={action} project={project} view={command.view} command={command.action} onSaved={saved} onClose={()=>setCommand(undefined)} onDenied={onDenied} readLatest={read.read}/>}
 </aside>;
}

function ReviewFacts({review,project}:{review:ProjectRiskReview;project:string}){
 const verified=review.disposition==='mitigated';
 return <section className="pr-section" data-risk-review={review.id}><h4>{t('review_title')} · {t('decision.'+review.decision)}</h4><p className="pr-copy">{t('disposition.'+review.disposition)}</p><p className="pr-muted">{t(verified?'mitigation_boundary':'decision_boundary')}</p><p className="pr-copy">{review.rationale}</p><details><summary>{t('review_evidence')}</summary><p className="pr-copy">{review.evidence}</p><dl className="pr-facts"><dt>{t('residual_score')}</dt><dd>{review.residual_score===null?t('unknown'):formatNumber(review.residual_score)}</dd><dt>{t('submitted_at')}</dt><dd>{formatDateTime(review.submitted_at)}</dd><dt>{t('submitted_by')}</dt><dd>{review.submitted_by}</dd><dt>{t('assessment_sha')}</dt><dd><code>{review.assessment_sha256}</code></dd><dt>{t('review_id')}</dt><dd><code>{review.id}</code></dd>{review.decided_at&&<><dt>{t('decision_at')}</dt><dd>{formatDateTime(review.decided_at)}</dd><dt>{t('decided_by')}</dt><dd>{review.decided_by}</dd><dt>{t('reason')}</dt><dd className="pr-copy">{review.decision_reason}</dd></>}</dl>{review.links.map(link=><div className="pr-link" key={link.kind+':'+link.id}><a href={riskRecordHref(project,link.kind,link.id)} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_'+link.kind)} · v{formatNumber(link.version)}<ArrowUpRight size={12}/></a>{link.verified_run_id&&<a href={riskRunHref(project,link.verified_run_id)} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_verified_run')}<ArrowUpRight size={12}/></a>}{link.review_id&&<small>{t('formal_review')}: <code>{link.review_id}</code></small>}</div>)}</details></section>;
}

type EditorProps={session:Session;project:string;risk?:ProjectRisk;sourceAlert?:PlanningAlert;onSaved:(value:ProjectRiskView)=>void;onClose:()=>void;onDenied:()=>void;readLatest?:()=>Promise<ProjectRiskView|undefined>};
function RiskCreateEditor(props:EditorProps){const {session,project,sourceAlert}=props,action=useWriteAction({scope:'project-risk:'+scopeOf(session,project)+':create:'+(sourceAlert?.id||'manual'),storage:toolSessionStorage(),validateRequest:request=>validRiskRequest(request,project,undefined,sourceAlert?.id),validateResponse:(value,request)=>validRiskResponse(value,request,project,undefined,sourceAlert?.id)});return <RiskEditor {...props} action={action}/>;}
function RiskEditor({project,risk,sourceAlert,onSaved,onClose,onDenied,readLatest,action}:EditorProps&{action:WriteAction}){
 useLocale();const pending=action.pending?.body;
 const [draft,setDraft]=useState<ProjectRiskFields>(()=>validRiskFields(pending)?pending:risk?riskFields(risk):{...blankRiskFields(),title:sourceAlert?.title||'',detail:sourceAlert?.detail||'',links:sourceAlert?.task_ids.map(id=>({kind:'task',id}))||[]}),[version,setVersion]=useState(Number(pending?.expected_version||risk?.version||0)),[reason,setReason]=useState(typeof pending?.reason==='string'?pending.reason:''),[dirty,setDirty]=useState(false);
 const denied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus),locked=action.busy||!!action.pending||denied;useLayoutEffect(()=>{if(denied)onDenied();},[denied,onDenied]);useLeaveGuard(dirty,action.setError);
 function update(patch:Partial<ProjectRiskFields>){setDirty(true);setDraft(previous=>({...previous,...patch}));}
 async function finish(value:ProjectRiskView|undefined){if(value){setDirty(false);onSaved(value);}}
 async function save(){if(locked||action.conflict||action.persistenceBlocked||!validRiskFields(draft)||!riskText(reason))return;await finish(await action.run<ProjectRiskView>('/risks'+(risk?'/'+encodeURIComponent(risk.id):''),{project_id:project,...draft,reason,...(risk?{expected_version:version}:sourceAlert?{source_alert_id:sourceAlert.id,expected_alert_version:sourceAlert.version}:{})},risk?'PATCH':'POST'));}
 return <Modal open onClose={()=>{if(!dirty&&!locked)onClose();else action.setError(new UiError('project-risks.draft_guard'));}} title={t(risk?'edit':'create')} description={t(sourceAlert?'register_hint':'edit_hint')}>
  <form className="pr-modal" data-risk-editor onSubmit={event=>{event.preventDefault();void save();}}>
   <WriteNotice action={action} onRebase={async()=>{const value=await readLatest?.();if(!value)throw new UiError('project-risks.refresh_failed');setVersion(value.risk.version);}}/>
   <Recovery action={action} disabled={denied} onCompleted={finish}/>
   {!denied&&<><WriteFields action={action}>
    <Field label={t('field_title')}><input name="risk_title" value={draft.title} required maxLength={500} onChange={event=>update({title:event.target.value})}/></Field><Field label={t('detail')}><textarea name="risk_detail" rows={3} required maxLength={8000} value={draft.detail} onChange={event=>update({detail:event.target.value})}/></Field>
    <div className="pr-fields-grid"><RiskOwner project={project} value={draft.owner_id} onChange={owner_id=>update({owner_id})}/><Field label={t('review_date')}><input name="risk_review_date" type="date" value={draft.review_date} onChange={event=>update({review_date:event.target.value})}/></Field></div>
    <Field label={t('strategy')}><select name="risk_strategy" value={draft.strategy} onChange={event=>update({strategy:event.target.value})}>{riskStrategies.map(value=><option key={value||'unknown'} value={value}>{t('strategy.'+(value||'unknown'))}</option>)}</select></Field><Field label={t('plan')}><textarea name="risk_plan" rows={3} maxLength={8000} value={draft.plan} onChange={event=>update({plan:event.target.value})}/></Field>
    <details><summary>{t('assessment_optional')}</summary><p className="pr-muted">{t('ordinal_hint')}</p><div className="pr-fields-grid"><Rating label={t('probability')} name="risk_probability" value={draft.probability} onChange={probability=>update({probability})}/><Rating label={t('impact')} name="risk_impact" value={draft.impact} onChange={impact=>update({impact})}/></div></details>
    <RiskLinksEditor project={project} links={draft.links} onChange={links=>update({links})}/>
    <Field label={t('reason')}><textarea name="risk_reason" required rows={2} maxLength={2000} value={reason} onChange={event=>{setReason(event.target.value);setDirty(true);}}/></Field>
   </WriteFields><div className="pr-actions"><WriteButton action={action} disabled={locked||action.persistenceBlocked||!validRiskFields(draft)||!riskText(reason)}>{t('save')}</WriteButton><button type="button" disabled={locked} onClick={()=>{setDirty(false);onClose();}}>{t('cancel_draft')}</button>{risk&&<small className="pr-muted">{t('based_version',{version:formatNumber(version)})}</small>}</div></>}
  </form>
 </Modal>;
}

function RiskCommandDialog({project,view,command,onSaved,onClose,onDenied,readLatest,action}:{project:string;view:ProjectRiskView;command:RiskCommand;onSaved:(value:ProjectRiskView)=>void;onClose:()=>void;onDenied:()=>void;readLatest:()=>Promise<ProjectRiskView|undefined>;action:WriteAction}){
 useLocale();const id=view.risk.id,pending=action.pending?.body;
 const [basis,setBasis]=useState(view),[reason,setReason]=useState(typeof pending?.reason==='string'?pending.reason:''),[review,setReview]=useState<ProjectRiskReviewInput>(()=>validRiskReviewInput(pending?.review)?pending.review:{disposition:view.risk.strategy==='accept'?'accepted':view.risk.strategy==='avoid'?'avoided':view.risk.strategy==='transfer'?'transferred':'mitigated',residual_probability:null,residual_impact:null,rationale:'',evidence:''}),[ack,setAck]=useState(false),[dirty,setDirty]=useState(false);
 const denied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus),locked=action.busy||!!action.pending||denied,mitigated=command==='submit_review'?review.disposition==='mitigated':basis.current_review?.disposition==='mitigated',reviewInput={...review,rationale:reason};
 useLayoutEffect(()=>{if(denied)onDenied();},[denied,onDenied]);useLeaveGuard(dirty,action.setError);
 const ready=basis.allowed_actions.includes(command)&&(command==='start'?basis.readiness.assessment_ready:command==='submit_review'?basis.readiness.assessment_ready&&(!mitigated||basis.readiness.mitigation_ready):command==='close'?basis.readiness.close_ready&&!!basis.current_review:true),valid=ready&&riskText(reason)&&(command!=='submit_review'||validRiskReviewInput(reviewInput))&&(command!=='close'||ack);
 async function finish(value:ProjectRiskView|undefined){if(value){setDirty(false);onSaved(value);}}
 async function send(){if(!valid||locked||action.conflict||action.persistenceBlocked)return;await finish(await action.run<ProjectRiskView>('/risks/'+encodeURIComponent(id)+'/commands',{project_id:project,expected_version:basis.risk.version,action:command,reason,...(command==='submit_review'?{review:reviewInput}:command==='close'?{expected_review_id:basis.current_review!.id}:{})}));}
 return <Modal open onClose={()=>{if(!dirty&&!locked)onClose();else action.setError(new UiError('project-risks.draft_guard'));}} title={t('action.'+command)} description={t('command_hint.'+command)}><form className="pr-modal" data-risk-command-form={command} onSubmit={event=>{event.preventDefault();void send();}}>
  <WriteNotice action={action} onRebase={async()=>{const value=await readLatest();if(!value)throw new UiError('project-risks.refresh_failed');setBasis(value);setAck(false);}}/><Recovery action={action} disabled={denied} onCompleted={finish}/>
  {!denied&&<><p className="pr-muted">{t('based_version',{version:formatNumber(basis.risk.version)})}</p><RiskBlockers view={basis}/><WriteFields action={action}>
   {command==='submit_review'&&<><Field label={t('disposition')}><select name="risk_disposition" value={review.disposition} onChange={event=>{setReview({...review,disposition:event.target.value});setDirty(true);}}>{riskDispositions.map(value=><option value={value} key={value}>{t('disposition.'+value)}</option>)}</select></Field><p className="pr-muted">{t(mitigated?'mitigation_boundary':'decision_boundary')}</p><Field label={t('review_evidence')}><textarea name="risk_review_evidence" rows={3} value={review.evidence} required maxLength={8000} onChange={event=>{setReview({...review,evidence:event.target.value});setDirty(true);}}/></Field><details><summary>{t('residual_assessment')}</summary><div className="pr-fields-grid"><Rating label={t('probability')} name="risk_residual_probability" value={review.residual_probability} onChange={residual_probability=>{setReview({...review,residual_probability});setDirty(true);}}/><Rating label={t('impact')} name="risk_residual_impact" value={review.residual_impact} onChange={residual_impact=>{setReview({...review,residual_impact});setDirty(true);}}/></div><p className="pr-muted">{t('ordinal_hint')}</p></details></>}
   {command==='close'&&<><p className="pr-copy">{basis.current_review?t('disposition.'+basis.current_review.disposition):t('unknown')}</p><p className="pr-muted">{t(mitigated?'mitigation_boundary':'decision_boundary')}</p><code>{basis.current_review?.id}</code></>}
   <Field label={t(command==='submit_review'?'review_rationale':'reason')}><textarea name="risk_command_reason" required rows={3} maxLength={2000} value={reason} onChange={event=>{setReason(event.target.value);setAck(false);setDirty(true);}}/></Field>
   {command==='close'&&<label className="pr-check"><input name="risk_close_ack" type="checkbox" checked={ack} onChange={event=>setAck(event.target.checked)}/><span>{t(mitigated?'close_ack_mitigated':'close_ack_decision')}</span></label>}
  </WriteFields><div className="pr-actions"><WriteButton action={action} disabled={!valid||locked||action.persistenceBlocked}>{t('confirm')}</WriteButton><button type="button" disabled={locked} onClick={()=>{setDirty(false);onClose();}}>{t('cancel_draft')}</button></div></>}
 </form></Modal>;
}
function Recovery({action,disabled,onCompleted}:{action:ReturnType<typeof useWriteAction>;disabled:boolean;onCompleted:(value:ProjectRiskView|undefined)=>void|Promise<void>}){const pending=action.pending;return pending&&!disabled?<div className="pr-recovery" role="status"><strong>{t('original_operation')}</strong><p className="pr-copy">{String(pending.body.reason||'')}</p><button type="button" data-risk-restore disabled={action.busy} onClick={()=>void action.run<ProjectRiskView>(pending.path,pending.body,pending.method).then(onCompleted)}>{t('restore')}</button></div>:null;}
function Rating({label,name,value,onChange}:{label:string;name:string;value:number|null;onChange:(value:number|null)=>void}){return <Field label={label}><select name={name} value={value===null?'':String(value)} onChange={event=>onChange(event.target.value===''?null:Number(event.target.value))}><option value="">{t('unknown')}</option>{[1,2,3,4,5].map(number=><option key={number} value={number}>{formatNumber(number)}</option>)}</select></Field>;}

type RecordPage={items:RecordItem[];total:number;next_cursor?:string};
function RiskOwnerLabel({project,id,refresh}:{project:string;id:string;refresh:number}){const valid=useCallback((value:unknown):value is RecordItem=>riskObject(value)&&value.project_id===project&&value.id===id&&Number.isSafeInteger(value.version),[project,id]),read=useRiskRead(id?'/members/'+encodeURIComponent(id)+query(project):undefined,refresh,valid);return <>{id?read.value?label(read.value):<code>{id}</code>:t('unassigned')}{id&&read.error&&<small className="pr-muted"> · {t('owner_unconfirmed')}</small>}</>;}
function validRecords(value:unknown,project:string):value is RecordPage{return riskObject(value)&&Array.isArray(value.items)&&value.items.length<=100&&Number.isSafeInteger(value.total)&&Number(value.total)>=value.items.length&&value.items.every(item=>riskObject(item)&&riskID(item.id)&&item.project_id===project&&Number.isSafeInteger(item.version))&&(value.next_cursor===undefined||typeof value.next_cursor==='string');}
function RiskOwner({project,value,onChange}:{project:string;value:string;onChange:(value:string)=>void}){
 const [cursor,setCursor]=useState(''),valid=useCallback((value:unknown):value is RecordPage=>validRecords(value,project),[project]),read=useRiskRead('/members'+query(project)+'&limit=100'+(cursor?'&cursor='+encodeURIComponent(cursor):''),0,valid),eligible=read.value?.items.filter(item=>item.kind==='human'&&item.status==='active'&&riskID(item.user_id||item.data?.user_id))||[];
 return <div className="pr-picker"><Field label={t('owner')} hint={t('owner_hint')}><select name="risk_owner" value={value} onChange={event=>onChange(event.target.value)}><option value="">{t('unassigned')}</option>{value&&!eligible.some(item=>item.id===value)&&<option value={value}>{t('current_owner',{id:value})}</option>}{eligible.map(item=><option key={item.id} value={item.id}>{label(item)}</option>)}</select></Field>{read.loading&&<small className="pr-muted">{t('reading_members')}</small>}{!!read.error&&<ErrorNotice error={toolError(read.error)} onRetry={()=>void read.read()}/>}{read.value?.next_cursor&&<button type="button" onClick={()=>setCursor(read.value!.next_cursor!)}>{t('next_members')}</button>}</div>;
}
function RiskLinksEditor({project,links,onChange}:{project:string;links:ProjectRiskFields['links'];onChange:(value:ProjectRiskFields['links'])=>void}){
 const [open,setOpen]=useState(false),[kind,setKind]=useState('task'),[cursor,setCursor]=useState(''),[selected,setSelected]=useState(''),valid=useCallback((value:unknown):value is RecordPage=>validRecords(value,project),[project]),collection=kind==='requirement'?'requirements':kind==='issue'?'issues':'tasks',read=useRiskRead(open?'/'+collection+query(project)+'&limit=100'+(cursor?'&cursor='+encodeURIComponent(cursor):''):undefined,0,valid);
 const canAdd=!!selected&&links.length<32&&!!read.value?.items.some(item=>item.id===selected)&&!links.some(item=>item.kind===kind&&item.id===selected);
 return <section className="pr-picker"><strong>{t('links')}</strong><div className="pr-links">{links.map(link=><div className="pr-link" key={link.kind+':'+link.id}><a href={riskRecordHref(project,link.kind,link.id)} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_'+link.kind)} · <code>{link.id}</code></a><button type="button" onClick={()=>onChange(links.filter(item=>item!==link))}>{t('remove_link')}</button></div>)}</div><button type="button" aria-expanded={open} onClick={()=>setOpen(value=>!value)}>{t('add_link')}</button>{open&&<><Field label={t('record_kind')}><select value={kind} onChange={event=>{setKind(event.target.value);setCursor('');setSelected('');}}>{['task','requirement','issue'].map(value=><option value={value} key={value}>{t('kind.'+value)}</option>)}</select></Field>{read.loading&&<Loading/>}{!!read.error&&<ErrorNotice error={toolError(read.error)} onRetry={()=>void read.read()}/>}<Field label={t('select_record')}><select data-risk-link-record value={selected} disabled={read.loading||!!read.error} onChange={event=>setSelected(event.target.value)}><option value="">{t('choose_record')}</option>{read.value?.items.map(item=><option key={item.id} value={item.id} disabled={links.some(link=>link.kind===kind&&link.id===item.id)}>{label(item)}</option>)}</select></Field>{read.value&&<p className="pr-muted">{t('record_scope',{loaded:formatNumber(read.value.items.length),total:formatNumber(read.value.total)})}</p>}{read.value?.next_cursor&&<button type="button" onClick={()=>{setCursor(read.value!.next_cursor!);setSelected('');}}>{t('next')}</button>}<button type="button" disabled={!canAdd} onClick={()=>{if(canAdd){onChange([...links,{kind,id:selected}]);setSelected('');}}}>{t('confirm_link')}</button></>}<p className="pr-muted">{t('links_hint')}</p></section>;
}

/** A planning suggestion is adopted deliberately into the risk domain. This
 * creates no execution, incident, test pass or acceptance decision. */
export function ProjectRiskRegistration({session,project,alert,onOpenRisk,onChanged}:{session:Session;project:string;alert:PlanningAlert;onOpenRisk?:(id:string)=>void;onChanged:()=>void|Promise<void>}){
 const [open,setOpen]=useState(false),[result,setResult]=useState<ProjectRiskView>(),[denied,setDenied]=useState(false),pendingOpen=useRef('');useLocale();
 // Navigate only after the successful editor unmounted and released its draft
 // guard. Other unsaved planning work keeps its own navigation protection.
 useEffect(()=>{if(!open&&result&&pendingOpen.current===result.risk.id){pendingOpen.current='';onOpenRisk?.(result.risk.id);}},[open,result,onOpenRisk]);
 return <>
  {!denied&&<button type="button" data-risk-register onClick={()=>setOpen(true)}>{t('register_alert')}</button>}
  {result&&<p className="pr-muted">{t(result.created===false?'existing':'registered')} <a href={riskHref(project,result.risk.id)} onClick={event=>{if(!canNavigate())event.preventDefault();else if(onOpenRisk){event.preventDefault();onOpenRisk(result.risk.id);}}}>{t('open_registered')}</a></p>}
  {denied&&<p className="pr-warning">{t('access_changed')}</p>}
  {open&&!denied&&<RiskCreateEditor key={scopeOf(session,project)+':'+alert.id} session={session} project={project} sourceAlert={alert} onClose={()=>setOpen(false)} onDenied={()=>{setDenied(true);setResult(undefined);setOpen(false);}} onSaved={value=>{pendingOpen.current=value.risk.id;setOpen(false);setResult(value);void onChanged();}}/>}
 </>;
}
