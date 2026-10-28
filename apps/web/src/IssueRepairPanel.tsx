import {useCallback,useEffect,useId,useRef,useState,type MouseEvent} from 'react';
import {ArrowUpRight,ChevronDown,RefreshCw,Wrench} from 'lucide-react';
import type {IssueRepair,IssueRepairView,TestPlanView} from '../../../packages/contracts/src';
import {ApiError,query,stateLabel,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useLeaveGuard,useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteRecoveryRequest} from './write-action';
import {useTestPlanPage,useTestPlanRead} from './test-plans-hooks';
import {validPlanPage,validTestPlan} from './test-plans-model';
import './IssueRepairPanel.css';

type Props={session:Session;project:string;issueId:string;issueVersion:number;refresh:number;onChanged:()=>void|Promise<void>;onOpenTask?:(id:string)=>void;onOpenRequirement?:(id:string)=>void};
type Command='prepare'|'regression'|'close'|'reopen';
const t=(key:string,values?:Record<string,unknown>)=>tr('issue-repair.'+key,values);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const id=(value:unknown):value is string=>typeof value==='string'&&/^[A-Za-z0-9_-]{1,100}$/.test(value);
const integer=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0;
const positive=(value:unknown):value is number=>integer(value)&&value>0;
const hash=(value:unknown)=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const text=(value:unknown,max=2000,empty=false):value is string=>typeof value==='string'&&(empty||!!value.trim())&&new TextEncoder().encode(value).length<=max;
const keys=(value:Record<string,unknown>,allowed:string[])=>Object.keys(value).every(key=>allowed.includes(key));
const caseKeys=(value:unknown):value is string[]=>Array.isArray(value)&&value.length>0&&value.length<=100&&value.every(item=>typeof item==='string'&&/^[A-Za-z0-9_-]{1,64}$/.test(item))&&new Set(value).size===value.length;
const equalKeys=(a:unknown,b:unknown)=>caseKeys(a)&&caseKeys(b)&&JSON.stringify([...a].sort())===JSON.stringify([...b].sort());
const base=(issue:string)=>'/issues/'+encodeURIComponent(issue)+'/repair';
const canNavigate=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));
const blockerCodes=new Set(['repair_not_prepared','issue_closed','regression_not_bound','repair_task_not_accepted','repair_run_changed','repair_evidence_stale','regression_plan_changed','regression_case_failed','original_case_required','formal_review_missing']);
function record(value:unknown,project:string,expected?:string):value is IssueRepairView['issue']{
 return object(value)&&id(value.id)&&(expected===undefined||value.id===expected)&&value.project_id===project&&positive(value.version)&&typeof value.title==='string'&&typeof value.description==='string'&&typeof value.status==='string';
}
function validCycle(value:unknown,project:string,issue:string):value is IssueRepair{
 if(!object(value)||!id(value.id)||value.project_id!==project||value.issue_id!==issue||!positive(value.cycle)||!positive(value.version)||!id(value.requirement_id)||!id(value.task_id)||typeof value.prepared_by!=='string'||typeof value.created_at!=='string'||typeof value.updated_at!=='string')return false;
 if(value.regression!=null){const r=value.regression;if(!object(r)||!id(r.plan_id)||!positive(r.plan_revision)||!hash(r.plan_sha256)||!caseKeys(r.case_keys)||typeof r.bound_by!=='string'||typeof r.bound_at!=='string')return false;}
 if(value.closure!=null){const c=value.closure;if(!object(c)||!id(c.run_id)||!positive(c.task_version)||!id(c.review_id)||!id(c.plan_id)||!positive(c.plan_revision)||!hash(c.plan_sha256)||!caseKeys(c.case_keys)||!Array.isArray(c.source_commits)||c.source_commits.some(item=>typeof item!=='string'||!/^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(item))||typeof c.closed_by!=='string'||typeof c.closed_at!=='string'||!text(c.reason))return false;}
 return ['reopened_at','reopened_by','reopen_reason'].every(key=>value[key]===undefined||typeof value[key]==='string');
}
/** The view is tied to one original issue. Editable issue.data never supplies
 * repair lineage, regression approval or a candidate for closing the issue. */
export function validIssueRepairView(value:unknown,project:string,issue:string):value is IssueRepairView{
 if(!object(value)||!record(value.issue,project,issue)||!['open','in_progress','closed'].includes(value.issue.status)||!integer(value.cycle)||!Array.isArray(value.history)||value.history.length>10||!integer(value.history_total)||value.history_total<value.history.length||typeof value.history_complete!=='boolean'||value.history_complete&&value.history_total!==value.history.length||!['can_prepare','can_bind','can_close','can_reopen'].every(key=>typeof value[key]==='boolean')||!Array.isArray(value.blockers)||value.blockers.length>100||!value.blockers.every(item=>object(item)&&typeof item.code==='string'&&item.code.length>0&&item.code.length<=150&&(item.ref_id===undefined||id(item.ref_id))))return false;
 if(value.current!=null&&(!validCycle(value.current,project,issue)||value.current.cycle!==value.cycle))return false;
 if(value.task!=null&&(!object(value.current)||!record(value.task,project,String(value.current.task_id))))return false;
 return value.history.every(item=>validCycle(item,project,issue))&&new Set(value.history.map(item=>item.id)).size===value.history.length;
}
function recovery(request:WriteRecoveryRequest,project:string,issue:string){
 const body=request.body,action=request.path.slice(base(issue).length+1);
 if(request.method!=='POST'||request.path!==base(issue)+'/'+action||!['prepare','regression','close','reopen'].includes(action)||body.project_id!==project||!positive(body.expected_issue_version)||!text(body.reason))return false;
 const common=['project_id','expected_issue_version','reason','idempotency_key'];
 if(action==='prepare')return keys(body,[...common,'title','description','owner_id','acceptance_criteria'])&&text(body.title,500)&&text(body.description,200000,true)&&(body.owner_id===''||id(body.owner_id))&&Array.isArray(body.acceptance_criteria)&&body.acceptance_criteria.length>0&&body.acceptance_criteria.length<=100&&body.acceptance_criteria.every(item=>text(item));
 if(!positive(body.expected_repair_version))return false;
 if(action==='regression')return keys(body,[...common,'expected_repair_version','expected_plan_version','plan_id','case_keys'])&&positive(body.expected_plan_version)&&id(body.plan_id)&&caseKeys(body.case_keys);
 return keys(body,[...common,'expected_repair_version',...(action==='close'?['run_id']:[])])&&(action!=='close'||id(body.run_id));
}
function originalResponse(value:unknown,request:WriteRecoveryRequest,project:string,issue:string){
 if(!validIssueRepairView(value,project,issue)||value.issue.version<Number(request.body.expected_issue_version))return false;
 const action=request.path.slice(base(issue).length+1),current=value.current,body=request.body;
 if(action==='prepare')return !!current&&!!value.task&&value.issue.status==='in_progress'&&value.task.id===current.task_id;
 if(action==='regression')return !!current&&!!current.regression&&current.version>Number(body.expected_repair_version)&&current.regression.plan_id===body.plan_id&&equalKeys(current.regression.case_keys,body.case_keys);
 if(action==='close')return value.issue.status==='closed'&&!!current?.closure&&current.closure.run_id===body.run_id&&current.closure.reason===body.reason;
 return action==='reopen'&&value.issue.status==='open'&&[...(value.history||[]),...(current?[current]:[])].some(item=>item.reopen_reason===body.reason&&!!item.reopened_at);
}

/** Account/project/issue changes discard the previous visible scope and abort
 * pending reads. Durable writes remain recoverable only under their own key. */
export function IssueRepairPanel(props:Props){
 const scope=JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.issueId]);
 return <RepairScope key={scope} {...props} scope={scope}/>;
}
function RepairScope({project,issueId,issueVersion,refresh,onChanged,onOpenTask,onOpenRequirement,scope}:Props&{scope:string}){
 useLocale();const api=useApi(),bodyID=useId(),callbacks=useRef({onChanged,onOpenTask,onOpenRequirement}),alive=useRef(true),panelRoot=useRef<HTMLElement|null>(null),commandForm=useRef<HTMLFormElement|null>(null),previousMode=useRef<Command|undefined>(undefined);callbacks.current={onChanged,onOpenTask,onOpenRequirement};
 const validateRequest=useCallback((request:WriteRecoveryRequest)=>recovery(request,project,issueId),[project,issueId]);
 const validateResponse=useCallback((value:unknown,request:WriteRecoveryRequest)=>originalResponse(value,request,project,issueId),[project,issueId]);
 const action=useWriteAction({scope:'issue-repair:'+scope,storage:toolSessionStorage(),validateRequest,validateResponse});
 const [open,setOpen]=useState(!!action.pending),[access,setAccess]=useState<unknown>(),[tick,setTick]=useState(0),[mode,setMode]=useState<Command>(),[reason,setReason]=useState(''),[ack,setAck]=useState(false),[title,setTitle]=useState(''),[description,setDescription]=useState(''),[criteria,setCriteria]=useState(''),[selectedPlan,setSelectedPlan]=useState(''),[selectedCases,setSelectedCases]=useState<string[]>([]),[notice,setNotice]=useState('');
 // Opening a command moves keyboard focus into its first meaningful field;
 // closing it returns to its action or the persistent panel toggle.
 useEffect(()=>{const before=previousMode.current;previousMode.current=mode;if(mode){commandForm.current?.querySelector<HTMLElement>(mode==='prepare'?'[name=ir_title]':mode==='regression'?'[name=ir_plan]':'[name=ir_reason]')?.focus();}else if(before){(panelRoot.current?.querySelector<HTMLElement>('[data-ir-'+(before==='regression'?'bind':before)+']')??panelRoot.current?.querySelector<HTMLElement>('[data-ir-toggle]'))?.focus();}},[mode]);
 const valid=useCallback((value:unknown):value is IssueRepairView=>validIssueRepairView(value,project,issueId),[project,issueId]);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setAccess(error);setMode(undefined);setReason('');setAck(false);setSelectedPlan('');setSelectedCases([]);}},[]);
 const detail=useTestPlanRead(api,open&&!access?base(issueId)+query(project):undefined,refresh+tick,valid,failed),view=detail.value,current=view?.current;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{if(action.errorStatus===401||action.errorStatus===403)failed(new ApiError(action.errorStatus,'forbidden',t('access_changed')));},[action.errorStatus,failed]);
 const validPlan=useCallback((value:unknown):value is TestPlanView=>validTestPlan(value,project)&&value.task_id===current?.task_id,[project,current?.task_id]);
 const validPlans=useCallback((value:unknown):value is {items:TestPlanView[];total:number;next_cursor:string}=>validPlanPage(value,validPlan),[validPlan]);
 const plans=useTestPlanPage(api,mode==='regression'&&current&&!access?'/test-plans'+query(project)+'&task_id='+encodeURIComponent(current.task_id)+'&limit=20':undefined,refresh+tick,validPlans,failed);
 const chosen=plans.value?.items.find(item=>item.id===selectedPlan),published=chosen?.published;
 const locked=action.busy||!!action.pending||!!access||detail.loading||!!detail.error,dirty=!!mode&&(!!reason||mode==='prepare'&&(!!title||!!description||!!criteria)||mode==='regression'&&(!!selectedPlan||selectedCases.length>0));
 useLeaveGuard(dirty,action.setError);
 const lines=criteria.split('\n').map(value=>value.trim()).filter(Boolean),prepareValid=text(title,500)&&text(description,200000,true)&&lines.length>0&&lines.length<=100&&lines.every(value=>text(value));
 const canSubmit=!!view&&!!mode&&!locked&&!action.conflict&&!action.persistenceBlocked&&text(reason)&&(mode==='prepare'?view.can_prepare&&prepareValid:mode==='regression'?view.can_bind&&!plans.loading&&!plans.error&&!!current&&!!chosen&&chosen.basis_current&&chosen.eligibility==='current'&&!!published&&caseKeys(selectedCases)&&selectedCases.every(key=>published.cases.some(item=>item.key===key)):mode==='close'?view.can_close&&view.blockers.length===0&&!!current&&id(view.task?.run_id)&&ack:view.can_reopen&&!!current);
 function begin(command:Command){if(locked)return;setMode(command);setReason('');setAck(false);setNotice('');if(command==='prepare'){setTitle(t('repair_title',{title:view?.issue.title||''}));setDescription(view?.issue.description||'');setCriteria('');}if(command==='regression'){setSelectedPlan('');setSelectedCases([]);}}
 async function completed(result:IssueRepairView|undefined){
  if(!result||!alive.current)return;setMode(undefined);setReason('');setAck(false);setTitle('');setDescription('');setCriteria('');setSelectedPlan('');setSelectedCases([]);setNotice('saved');
  const results=await Promise.allSettled([detail.read(),Promise.resolve().then(()=>callbacks.current.onChanged())]);if(alive.current&&results.some(item=>item.status==='rejected'))setNotice('refresh_failed');
 }
 async function submit(){
  if(!canSubmit||!view||!mode)return;
  const body:Record<string,unknown>={project_id:project,expected_issue_version:view.issue.version,reason};
  if(mode==='prepare')Object.assign(body,{title,description,owner_id:'',acceptance_criteria:lines});
  else {body.expected_repair_version=current!.version;if(mode==='regression')Object.assign(body,{expected_plan_version:chosen!.version,plan_id:chosen!.id,case_keys:selectedCases});if(mode==='close')body.run_id=view.task!.run_id;}
  await completed(await action.run<IssueRepairView>(base(issueId)+'/'+mode,body));
 }
 async function rebase(){const latest=await detail.read();if(!latest)throw new UiError('issue-repair.refresh_failed');setAck(false);if(mode==='regression'){const updated=await plans.read();if(!updated)throw new UiError('issue-repair.refresh_failed');const next=updated.items.find(item=>item.id===selectedPlan);setSelectedCases(previous=>next?.version!==chosen?.version||next?.published?.sha256!==chosen?.published?.sha256?[]:previous.filter(key=>next?.published?.cases.some(item=>item.key===key)));}}
 const link=(kind:'task'|'requirement'|'plan'|'run',value:string,run='')=>'?'+new URLSearchParams({project,view:kind==='requirement'?'requirements':kind==='plan'?'quality':'development',...(kind==='requirement'?{}:{sub:kind==='plan'?'tests':kind==='run'?'runs':'tasks'}),...(kind==='run'?{run:value}:{record:value}),...(kind==='plan'&&run?{run,panel:'test-results'}:{})});
 function follow(event:MouseEvent<HTMLAnchorElement>,kind:'task'|'requirement',value:string){if(event.defaultPrevented||event.button!==0||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey)return;const handler=kind==='task'?callbacks.current.onOpenTask:callbacks.current.onOpenRequirement;if(!canNavigate()){event.preventDefault();return;}if(handler){event.preventDefault();handler(value);}}
 return <section ref={panelRoot} className="detail-section issue-repair" aria-label={t('title')}>
  <h3><button type="button" className="ir-toggle" data-ir-toggle aria-expanded={open} aria-controls={bodyID} disabled={action.busy||open&&!!action.pending} onClick={()=>{if(!open||!dirty)setOpen(value=>!value);else action.setError(new UiError('issue-repair.draft_open'));}}><Wrench size={16} aria-hidden="true"/><span>{t('title')}</span><ChevronDown size={16} className={open?'is-open':''} aria-hidden="true"/></button></h3>
  {open&&<div id={bodyID} className="ir-body" aria-busy={detail.loading}>
   <p className="ir-muted">{t('intro')}</p><WriteNotice action={action} onRebase={rebase}/>
   {action.pending&&!access&&<div className="ir-recovery" role="status"><strong>{t('original_operation')}</strong><p className="ir-copy">{String(action.pending.body.reason||'')}</p><button type="button" data-ir-restore disabled={action.busy||!!access} onClick={()=>{const pending=action.pending;if(pending)void action.run<IssueRepairView>(pending.path,pending.body,pending.method).then(completed);}}>{t('restore')}</button></div>}
   {notice&&<p className={notice==='saved'?'ir-success':'ir-warning'} role="status">{t(notice)}</p>}
   {access?<ErrorNotice error={toolError(access)} onRetry={()=>{setAccess(undefined);setTick(value=>value+1);}}/>:<>
    <div className="ir-actions"><button type="button" data-ir-refresh disabled={locked} onClick={()=>{setAck(false);setTick(value=>value+1);}}><RefreshCw size={13}/>{t('refresh')}</button></div>{detail.loading&&<Loading/>}{!!detail.error&&<ErrorNotice error={toolError(detail.error)} onRetry={()=>setTick(value=>value+1)}/>}
    {view&&<>
     <div className="ir-state"><strong>{t('cycle',{cycle:formatNumber(view.cycle)})}</strong><span className={'ir-status state-'+view.issue.status}>{t('state.'+view.issue.status)}</span></div>
     {view.issue.version!==issueVersion&&<p className="ir-muted">{t('current_version',{version:formatNumber(view.issue.version)})}</p>}
     {!!view.blockers.length&&view.issue.status!=='closed'&&<div className="ir-warning" data-ir-blockers><p>{t('blocked')}</p><ul>{view.blockers.map((item,index)=><li key={item.code+':'+index}>{blockerCodes.has(item.code)?t('blocker.'+item.code):<>{t('unknown_blocker')} <code>{item.code}</code></>}{item.ref_id&&<small> · <code>{item.ref_id}</code></small>}</li>)}</ul></div>}
     {current?<section className="ir-cycle"><div className="ir-actions"><a href={link('task',current.task_id)} onClick={event=>follow(event,'task',current.task_id)}>{t('open_task')}<ArrowUpRight size={13}/></a><a href={link('requirement',current.requirement_id)} onClick={event=>follow(event,'requirement',current.requirement_id)}>{t('open_requirement')}<ArrowUpRight size={13}/></a></div>
      {view.task&&<p className="ir-copy">{view.task.title} · {stateLabel(view.task.status)}</p>}
      {current.regression?<><p>{t('bound_revision',{revision:formatNumber(current.regression.plan_revision),count:formatNumber(current.regression.case_keys.length)})}</p><a href={link('plan',current.regression.plan_id,view.task?.run_id||'')} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_regression')}<ArrowUpRight size={13}/></a><details><summary>{t('binding_details')}</summary><dl className="ir-facts"><dt>{t('plan_sha')}</dt><dd><code>{current.regression.plan_sha256}</code></dd><dt>{t('case_keys')}</dt><dd>{current.regression.case_keys.join(', ')}</dd></dl></details></>:<p className="ir-muted">{t('no_regression')}</p>}
      {view.task?.run_id&&<a href={link('run',view.task.run_id)} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_run')}<ArrowUpRight size={13}/></a>}
      {view.issue.status!=='closed'&&<p className={view.can_close?'ir-success':'ir-muted'}>{t(view.can_close?'ready':'readiness_boundary')}</p>}
      {current.closure&&<Closure item={current} project={project}/>}<details><summary>{t('cycle_basis')}</summary><dl className="ir-facts"><dt>{t('issue')}</dt><dd><code>{issueId}</code> · v{formatNumber(view.issue.version)}</dd><dt>{t('repair_version')}</dt><dd>v{formatNumber(current.version)}</dd><dt>{t('prepared_at')}</dt><dd>{formatDateTime(current.created_at)}</dd></dl></details>
     </section>:<p className="ir-muted">{t('no_repair')}</p>}
     {!mode&&<div className="ir-actions">{view.can_prepare&&<button type="button" className="primary" data-ir-prepare onClick={()=>begin('prepare')}>{t('prepare')}</button>}{view.can_bind&&<button type="button" data-ir-bind onClick={()=>begin('regression')}>{t(current?.regression?'update_regression':'bind')}</button>}{view.can_close&&<button type="button" className="primary" data-ir-close onClick={()=>begin('close')}>{t('close')}</button>}{view.can_reopen&&<button type="button" data-ir-reopen onClick={()=>begin('reopen')}>{t('reopen')}</button>}</div>}
     {mode&&<form ref={commandForm} className="ir-command" data-ir-form onSubmit={event=>{event.preventDefault();void submit();}}><h4>{t('action.'+mode)}</h4><WriteFields action={action}><fieldset disabled={locked}>
      {mode==='prepare'&&<><p className="ir-muted">{t('prepare_hint')}</p><Field label={t('task_title')}><input name="ir_title" value={title} maxLength={500} required onChange={event=>setTitle(event.target.value)}/></Field><Field label={t('criteria')} hint={t('criteria_hint')}><textarea name="ir_criteria" value={criteria} required rows={3} maxLength={200000} onChange={event=>setCriteria(event.target.value)}/></Field><details><summary>{t('description')}</summary><Field label={t('description')}><textarea name="ir_description" value={description} rows={4} maxLength={200000} onChange={event=>setDescription(event.target.value)}/></Field></details>{!prepareValid&&<p className="ir-muted">{t('prepare_required')}</p>}</>}
      {mode==='regression'&&<><p className="ir-muted">{t('bind_hint')}</p>{plans.loading&&<Loading/>}{!!plans.error&&<ErrorNotice error={toolError(plans.error)} onRetry={()=>void plans.read()}/>}<Field label={t('plan')}><select name="ir_plan" value={selectedPlan} disabled={plans.loading||!!plans.error} onChange={event=>{setSelectedPlan(event.target.value);setSelectedCases([]);}}><option value="">{t('choose_plan')}</option>{plans.value?.items.map(item=><option value={item.id} key={item.id} disabled={!item.published||!item.basis_current||item.eligibility!=='current'}>{item.published?.title||item.id} · {t('published_revision',{revision:formatNumber(item.published_revision)})}</option>)}</select></Field>{plans.value&&!plans.value.items.length&&<p className="ir-muted">{t('no_plan')}</p>}{plans.value?.next_cursor&&<button type="button" disabled={plans.loading} onClick={()=>void plans.read(true)}>{t('more_plans')}</button>}{current&&<a href={link('plan','')} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('manage_plans')}<ArrowUpRight size={13}/></a>}{published&&<fieldset className="ir-cases"><legend>{t('select_cases')}</legend>{published.cases.map(item=><div className="ir-case" key={item.key}><label className="ir-check"><input type="checkbox" name="ir_case" value={item.key} checked={selectedCases.includes(item.key)} onChange={event=>setSelectedCases(previous=>event.target.checked?[...previous,item.key]:previous.filter(key=>key!==item.key))}/><span>{item.title} <code>{item.key}</code></span></label><small className="ir-muted">{item.selector.class_name} · {item.selector.name}</small></div>)}</fieldset>}</>}
      {mode==='close'&&<><p className="ir-muted">{t('close_hint')}</p><p className="ir-copy">{t('run')}: <code>{view.task?.run_id}</code></p><label className="ir-check"><input name="ir_ack" type="checkbox" checked={ack} onChange={event=>setAck(event.target.checked)}/><span>{t('close_ack')}</span></label></>}
      {mode==='reopen'&&<p className="ir-muted">{t('reopen_hint')}</p>}
      <Field label={t('reason')}><textarea name="ir_reason" required rows={3} maxLength={2000} value={reason} onChange={event=>{setReason(event.target.value);setAck(false);}}/></Field>
     </fieldset></WriteFields><div className="ir-actions"><WriteButton action={action} disabled={!canSubmit}>{t('confirm.'+mode)}</WriteButton><button type="button" disabled={locked} onClick={()=>{setMode(undefined);setReason('');setAck(false);}}>{t('cancel')}</button></div></form>}
     {!!view.history.length&&<details><summary>{t('history',{count:formatNumber(view.history_total)})}</summary><div className="ir-history">{view.history.map(item=><article className="ir-history-entry" key={item.id} data-ir-history><strong>{t('cycle',{cycle:formatNumber(item.cycle)})}</strong><a href={link('task',item.task_id)} onClick={event=>follow(event,'task',item.task_id)}>{t('open_task')}<ArrowUpRight size={13}/></a>{item.closure&&<Closure item={item} project={project}/>} {!!item.reopened_at&&<><p className="ir-muted">{t('reopened_at',{at:formatDateTime(item.reopened_at)})}</p><p className="ir-copy">{item.reopen_reason}</p></>}</article>)}</div>{!view.history_complete&&<p className="ir-muted">{t('history_limited')}</p>}</details>}
    </>}
   </>}
  </div>}
 </section>;
}
function Closure({item,project}:{item:IssueRepair;project:string}){const closure=item.closure;if(!closure)return null;const run='?'+new URLSearchParams({project,view:'development',sub:'runs',run:closure.run_id}),regression='?'+new URLSearchParams({project,view:'quality',sub:'tests',record:closure.plan_id,run:closure.run_id,panel:'test-results'});return <details><summary>{t('closure_details')}</summary><div className="ir-actions"><a data-ir-closure-run href={run} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_original_run')}<ArrowUpRight size={13}/></a><a data-ir-closure-regression href={regression} onClick={event=>{if(!canNavigate())event.preventDefault();}}>{t('open_original_regression')}<ArrowUpRight size={13}/></a></div><dl className="ir-facts"><dt>{t('closed_at')}</dt><dd>{formatDateTime(closure.closed_at)}</dd><dt>{t('closed_by')}</dt><dd>{closure.closed_by}</dd><dt>{t('reason')}</dt><dd className="ir-copy">{closure.reason}</dd><dt>{t('run')}</dt><dd><code>{closure.run_id}</code></dd><dt>{t('review')}</dt><dd><code>{closure.review_id}</code></dd><dt>{t('plan_sha')}</dt><dd><code>{closure.plan_sha256}</code></dd><dt>{t('case_keys')}</dt><dd>{closure.case_keys.join(', ')}</dd><dt>{t('commits')}</dt><dd>{closure.source_commits.map(value=><code key={value}>{value}<br/></code>)}</dd></dl></details>;}
