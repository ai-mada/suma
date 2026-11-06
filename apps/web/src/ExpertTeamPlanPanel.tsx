import {ExecutionModelFacts} from './ExecutionModelFacts';
import {CodexNativeSkillFacts} from './CodexNativeSkillFacts';
import {ExpertTeamExecutionFacts} from './ExpertTeamExecutionFacts';
import {expertPlanResponse} from './expert-plan-response';
import {ExpertModelSelections,expertModelScope,expertModelPayload,type ExpertModelChoices,type ExpertModelTask} from './ExpertModelSelections';
import {ExpertTestBindings} from './ExpertTestBindings';
import {expertTestScope,expertTestPayload,type ExpertTestChoices,type ExpertTestTask} from './expert-test-bindings-model';
import {FrozenTestBindingFacts} from './ToolFacts';
import {useCallback,useEffect,useRef,useState} from 'react';
import type {ExpertTeamConfiguration,ExpertTeamPlan,ExpertTeamPlanItem} from '../../../packages/contracts/src';
import {ApiError,query,stateLabel,type RecordItem,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {KnowledgeContextFacts} from './KnowledgeFacts';
import {DispatchRouteFacts} from './execution-pools/DispatchRouteFacts';
import {FailureDiagnosisLauncher} from './FailureDiagnosisLauncher';
import {compositionCodeBaseline} from './candidate-composition-model';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useWriteAction,WriteFields,WriteNotice} from './write-action';
import {expertID,expertInteger,expertText,validExpertConfiguration} from './expert-team-model';
import {expertPlanCommandRecovery,expertPlanCursor,expertPlanReason,expertTopology,expertUnstarted,validOrchestratedPlan} from './expert-team-plan-model';
import {useExpertPage,useExpertRead,type ExpertPage} from './expert-team-hooks';
import {followTaskDeliveryHref,taskDeliveryHref} from './task-delivery-navigation-model';
import {ArrowRight} from 'lucide-react';
import './ExpertTeamPlanPanel.css';

type Command='pause'|'resume'|'stop'|'revise';
type RevisionRow={taskID:string;selected:boolean;version:number;role:string};
type RevisionDraft={configuration:ExpertTeamConfiguration;rows:RevisionRow[]};
type Decision={action:Command;version:number;reason:string};
type Props={project:string;session:Session;plan:ExpertTeamPlan;loading:boolean;read:()=>Promise<ExpertTeamPlan|undefined>;onChanged:(value:ExpertTeamPlan)=>Promise<void>;onLock:(value:boolean)=>void;onForbidden:(error:unknown)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('expert-team-orchestration.'+key,values);
const old=(key:string,values?:Record<string,unknown>)=>tr('expert-team.'+key,values);
const time=(value:string)=>formatDateTime(value,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});
const taskURL=(project:string,id:string)=>'?'+new URLSearchParams({project,view:'development',sub:'tasks',record:id});
const runURL=(project:string,id:string)=>'?'+new URLSearchParams({project,view:'development',sub:'runs',run:id});
const revisionKey=(value:ExpertTeamPlan)=>String(value.revision);

export function ExpertTeamPlanPanel({project,session,plan,loading,read,onChanged,onLock,onForbidden}:Props){
 useLocale();const [models,setModels]=useState<ExpertModelChoices>(),[tests,setTests]=useState<ExpertTestChoices>();const api=useApi(),alive=useRef(true),lookup=useRef<AbortController|undefined>(undefined),callbacks=useRef({read,onChanged,onLock});callbacks.current={read,onChanged,onLock};
 const action=useWriteAction({scope:JSON.stringify(['expert-team-plan-command',session.user.id,session.organization.id,project,plan.id]),storage:toolSessionStorage(),validateRequest:request=>expertPlanCommandRecovery(request,project,plan.id),validateResponse:(value,request)=>expertPlanResponse(value,request,project)}),pending=action.pending?.body,locked=action.busy||!!action.pending||action.persistenceBlocked;
 const [decision,setDecision]=useState<Decision|undefined>(pending?{action:pending.action as Command,version:Number(pending.expected_version),reason:String(pending.reason)}:undefined),[ack,setAck]=useState(false),[editor,setEditor]=useState<RevisionDraft>(),[lookupBusy,setLookupBusy]=useState(false),[lookupError,setLookupError]=useState<unknown>(),[historyOpen,setHistoryOpen]=useState(false),[revision,setRevision]=useState(0),[historyTick,setHistoryTick]=useState(0),[forbidden,setForbidden]=useState<unknown>();
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;lookup.current?.abort();};},[]);
 useEffect(()=>{onLock(locked);return()=>onLock(false);},[locked,onLock]);
 useEffect(()=>setAck(false),[plan.version,plan.revision,plan.activation_generation,plan.state]);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setForbidden(error);setEditor(undefined);setDecision(undefined);onForbidden(error);}},[onForbidden]);
 const pageValid=useCallback((value:ExpertPage<ExpertTeamPlan>)=>!!value&&Array.isArray(value.items)&&value.items.length<=20&&expertInteger(value.total)&&value.total>=value.items.length&&(value.next_cursor===undefined||expertPlanCursor(value.next_cursor))&&value.items.every(item=>validOrchestratedPlan(item,project,plan.id)&&!item.can_start&&!item.allowed_actions.length),[project,plan.id]);
 const histories=useExpertPage(api,'/expert-team-plans/'+encodeURIComponent(plan.id)+'/revisions'+query(project)+'&limit=20',historyOpen&&!forbidden,historyTick,pageValid,failed,revisionKey,'before');
 const revisionValid=useCallback((value:ExpertTeamPlan)=>validOrchestratedPlan(value,project,plan.id)&&value.revision===revision&&!value.can_start&&!value.allowed_actions.length,[project,plan.id,revision]);
 const historical=useExpertRead(api,'/expert-team-plans/'+encodeURIComponent(plan.id)+'/revisions/'+revision+query(project),historyOpen&&revision>0&&!forbidden,historyTick,revisionValid,failed);
 const allowed=(name:Command)=>!loading&&!forbidden&&plan.allowed_actions.includes(name),unstarted=plan.items.filter(expertUnstarted),configurationChanged=!!editor&&editor.configuration.version!==plan.configuration_version;
 const selected=editor?.rows.filter(row=>row.selected)||[];
 const modelTasks:ExpertModelTask[]=selected.flatMap(row=>{const role=editor?.configuration.roles.find(role=>role.key===row.role);const item=plan.items.find(item=>item.task_id===row.taskID);return role&&item?[{taskID:row.taskID,title:item.plan.context.task.title||row.taskID,role}]:[];});
 const modelsReady=models?.scope===expertModelScope(project,modelTasks)&&models.ready;
 const testTasks:ExpertTestTask[]=modelTasks.map(task=>({...task,version:selected.find(row=>row.taskID===task.taskID)!.version,item:plan.items.find(item=>item.task_id===task.taskID)})),testBasis=expertTestScope(project,testTasks),testOwner=useRef({api,basis:testBasis,epoch:0});
 if(testOwner.current.api!==api||testOwner.current.basis!==testBasis)testOwner.current={api,basis:testBasis,epoch:testOwner.current.epoch+1};
 const testEpoch=testOwner.current.epoch,testScope=JSON.stringify([testBasis,testEpoch]),testsReady=tests?.scope===testScope&&tests.ready;
 const updateTests=useCallback((value:ExpertTestChoices)=>{if(testOwner.current.api===api&&testOwner.current.basis===testBasis&&testOwner.current.epoch===testEpoch&&value.scope===testBasis){setTests({...value,scope:testScope});setAck(false);}},[api,testBasis,testEpoch,testScope]);
 const canRevise=!!editor&&selected.length>0&&modelsReady&&testsReady&&selected.every(row=>{const item=plan.items.find(item=>item.task_id===row.taskID);return !!item&&expertUnstarted(item)&&expertInteger(row.version)&&row.version>0&&editor.configuration.roles.some(role=>role.key===row.role);})&&(!configurationChanged||unstarted.every(item=>selected.some(row=>row.taskID===item.task_id)));
 async function inputs(current:ExpertTeamPlan,previous?:RevisionDraft){
  lookup.current?.abort();const abort=new AbortController();lookup.current=abort;setLookupBusy(true);setLookupError(undefined);
  try{const pendingItems=current.items.filter(expertUnstarted),[configuration,tasks]=await Promise.all([api<ExpertTeamConfiguration>('/expert-teams/'+encodeURIComponent(current.team_id)+query(project),undefined,'GET',undefined,abort.signal),Promise.all(pendingItems.map(item=>api<RecordItem>('/tasks/'+encodeURIComponent(item.task_id)+query(project),undefined,'GET',undefined,abort.signal)))]);if(abort.signal.aborted||!alive.current)return;
   if(!validExpertConfiguration(configuration,project,current.team_id)||configuration.state!=='published'||!configuration.valid||tasks.some((task,index)=>task.id!==pendingItems[index].task_id||task.project_id!==project||!expertInteger(task.version)||task.version<1))throw new UiError('expert-team-orchestration.revision_inputs_failed');
   const rows=pendingItems.map((item,index)=>{const saved=previous?.rows.find(row=>row.taskID===item.task_id);return {taskID:item.task_id,selected:saved?.selected||false,version:tasks[index].version,role:saved?.role||item.role_key};});
   for(const saved of previous?.rows||[])if(saved.selected&&!pendingItems.some(item=>item.task_id===saved.taskID))rows.push(saved);
   setEditor({configuration,rows});return true;
  }catch(error){if(abort.signal.aborted||!alive.current)return;setLookupError(error);failed(error);}finally{if(!abort.signal.aborted&&alive.current)setLookupBusy(false);}
 }
 async function choose(name:Command){if(locked||!allowed(name))return;lookup.current?.abort();setLookupBusy(false);setLookupError(undefined);setDecision({action:name,version:plan.version,reason:''});setAck(false);setEditor(undefined);if(name==='revise')await inputs(plan);}
 async function completed(value:ExpertTeamPlan|undefined){if(!value||!alive.current)return;if(!validOrchestratedPlan(value,project,plan.id)){action.setError(new UiError('expert-team.invalid_read'));return;}setDecision(undefined);setEditor(undefined);setAck(false);setHistoryTick(value=>value+1);await callbacks.current.onChanged(value);}
 async function submit(){if(!decision||!allowed(decision.action)||!ack||!expertText(decision.reason)||locked||action.conflict||lookupBusy||decision.action==='revise'&&!canRevise)return;await completed(await action.run<ExpertTeamPlan>('/expert-team-plans/'+encodeURIComponent(plan.id)+'/commands',{project_id:project,expected_version:decision.version,action:decision.action,reason:decision.reason,acknowledge:true,...(decision.action==='revise'&&editor?{expected_configuration_version:editor.configuration.version,items:selected.map(row=>({task_id:row.taskID,expected_task_version:row.version,role_key:row.role,...expertModelPayload(models,row.taskID),...expertTestPayload(tests,row.taskID)}))}:{})}));}
 async function rebase(){setAck(false);const latest=await callbacks.current.read();if(!latest)throw new UiError('expert-team.refresh_failed');if(decision?.action==='revise'&&!await inputs(latest,editor))throw new UiError('expert-team-orchestration.revision_inputs_failed');setDecision(previous=>previous?{...previous,version:latest.version}:previous);}
 const ready=!!decision&&allowed(decision.action)&&ack&&expertText(decision.reason)&&!locked&&!action.conflict&&!lookupBusy&&(decision.action!=='revise'||canRevise);
 if(forbidden)return <ErrorNotice error={toolError(forbidden)}/>;
 return <div className="expert-plan-panel">
  <div className="expert-plan-summary"><div><h3>{plan.team_name}</h3><p>{t('plan.'+plan.state)} · {t('revision',{value:formatNumber(plan.revision)})}</p></div><button data-team-refresh type="button" aria-label={t('refresh')} disabled={action.busy||loading} onClick={()=>void read()}>{t('refresh')}</button></div>
  <dl className="expert-plan-progress" aria-label={t('progress')}>{(['total','waiting','ready','executing','awaiting_review','accepted','blocked','stopped'] as const).map(key=><div key={key}><dt>{t('count.'+key)}</dt><dd>{formatNumber(plan.progress[key])}</dd></div>)}</dl>
  {!!plan.blockers.length&&<div className="team-dispatch-callout"><strong>{old('blocked')}</strong><Reasons values={plan.blockers}/></div>}
  <PlanItems plan={plan} project={project} session={session} deliveryAvailable={!locked&&!loading&&!action.conflict} onChanged={async()=>{await read();}}/><p className="team-dispatch-muted">{t('acceptance_boundary')}</p>
  <WriteNotice action={action} onRebase={rebase}/>{action.pending&&<div className="team-dispatch-callout" role="status"><strong>{old('original_operation')}</strong><p>{t('command_recovery')}</p><details><summary>{old('original_request')}</summary><pre className="team-dispatch-raw">{JSON.stringify(action.pending,null,2)}</pre></details><button data-team-command-restore type="button" disabled={action.busy||loading} onClick={()=>{const request=action.pending;if(request)void action.run<ExpertTeamPlan>(request.path,request.body,request.method).then(completed);}}>{old('restore')}</button></div>}
  <div className="expert-plan-actions">{(['pause','resume','stop','revise'] as const).filter(name=>plan.allowed_actions.includes(name)).map(name=><button type="button" data-team-command={name} key={name} disabled={locked||loading} onClick={()=>void choose(name)}>{t('action.'+name)}</button>)}</div>
  {decision&&<form data-team-decision className="team-dispatch-callout" onSubmit={event=>{event.preventDefault();void submit();}}><h4>{t('action.'+decision.action)}</h4><p>{t('help.'+decision.action)}</p>{lookupBusy&&<Loading/>}{Boolean(lookupError)&&<ErrorNotice error={toolError(lookupError)} onRetry={()=>void inputs(plan,editor)}/>}
   {decision.action==='revise'&&editor&&<div data-team-revision><p>{t('revision_configuration',{value:formatNumber(editor.configuration.version)})}</p>{configurationChanged&&<p className="warning">{t('configuration_changed')}</p>}<button type="button" disabled={locked||lookupBusy} onClick={()=>setEditor({...editor,rows:editor.rows.map(row=>({...row,selected:plan.items.some(item=>item.task_id===row.taskID&&expertUnstarted(item))}))})}>{t('select_pending')}</button>
    {editor.rows.map(row=>{const item=plan.items.find(item=>item.task_id===row.taskID),started=!item||!expertUnstarted(item),title=item?.plan.context.task.title||row.taskID;return <section className="expert-plan-revision-row" key={row.taskID}><label className="team-dispatch-check"><input name={'team_revision_select_'+row.taskID} aria-label={t('select_revision_task',{title})} type="checkbox" checked={row.selected} disabled={locked||started&&!row.selected} onChange={event=>setEditor({...editor,rows:editor.rows.map(value=>value.taskID===row.taskID?{...value,selected:!started&&event.target.checked}:value)})}/><span>{title}<small>{t(started?'started_immutable':'current_task_version',{value:formatNumber(row.version)})}</small></span></label><Field label={t('revision_role',{title})}><select name={'team_revision_role_'+row.taskID} aria-label={t('revision_role',{title})} value={row.role} disabled={locked||started||!row.selected} onChange={event=>setEditor({...editor,rows:editor.rows.map(value=>value.taskID===row.taskID?{...value,role:event.target.value}:value)})}>{editor.configuration.roles.map(role=><option key={role.key} value={role.key}>{role.title}</option>)}{!editor.configuration.roles.some(role=>role.key===row.role)&&<option value={row.role} disabled>{row.role} · {t('role_changed')}</option>}</select></Field></section>;})}
    {!!modelTasks.length&&<><ExpertModelSelections project={project} tasks={modelTasks} disabled={locked} onChange={setModels}/><ExpertTestBindings project={project} tasks={testTasks} disabled={locked} onChange={updateTests}/></>}
    <p className="team-dispatch-muted">{t('started_preserved',{count:formatNumber(plan.items.filter(item=>!expertUnstarted(item)).length)})}</p>
   </div>}
   <WriteFields action={action}><Field label={old('reason')}><textarea name="team_plan_reason" aria-label={old('reason')} rows={3} maxLength={2000} value={decision.reason} onChange={event=>setDecision({...decision,reason:event.target.value})}/></Field><label className="team-dispatch-check"><input name="team_plan_ack" aria-label={t('ack.'+decision.action)} type="checkbox" checked={ack} onChange={event=>setAck(event.target.checked)}/><span>{t('ack.'+decision.action)}</span></label></WriteFields><div className="expert-plan-actions"><button data-team-confirm-command type="submit" className="primary" disabled={!ready}>{t('confirm.'+decision.action)}</button><button type="button" disabled={locked} onClick={()=>{setDecision(undefined);setEditor(undefined);setLookupError(undefined);}}>{old('cancel')}</button></div>
  </form>}
  <details className="expert-plan-source"><summary>{t('source_facts')}</summary><dl><dt>{t('graph_hash')}</dt><dd><code>{plan.graph_sha256}</code></dd><dt>{t('activation')}</dt><dd>{formatNumber(plan.activation_generation)}</dd><dt>{old('configuration_version',{value:''})}</dt><dd>{formatNumber(plan.configuration_version)}</dd></dl><pre className="team-dispatch-raw">{JSON.stringify(plan.configuration,null,2)}</pre></details>
  <details className="expert-plan-history" onToggle={event=>setHistoryOpen(event.currentTarget.open)}><summary>{t('history')}</summary>{histories.loading&&<Loading/>}{Boolean(histories.error)&&<ErrorNotice error={toolError(histories.error)} onRetry={()=>void histories.read()}/>}{histories.value&&!histories.value.items.length&&<p className="team-dispatch-muted">{t('history_empty')}</p>}<div className="expert-plan-history-list">{histories.value?.items.map(value=><button type="button" aria-label={t('read_revision',{value:formatNumber(value.revision)})} aria-pressed={revision===value.revision} key={value.revision} disabled={locked} onClick={()=>setRevision(value.revision)}>{t('revision',{value:formatNumber(value.revision)})} · {time(value.created_at)}</button>)}</div>{histories.value?.next_cursor&&<button type="button" disabled={locked||histories.loading} onClick={()=>void histories.read(true)}>{t('more_revisions')}</button>}{historical.loading&&<Loading/>}{Boolean(historical.error)&&<ErrorNotice error={toolError(historical.error)} onRetry={()=>void historical.read()}/>}{historical.value&&<section className="team-dispatch-callout"><strong>{t('historical_readonly',{value:formatNumber(historical.value.revision)})}</strong><PlanItems plan={historical.value} project={project}/><pre className="team-dispatch-raw">{JSON.stringify({graph_sha256:historical.value.graph_sha256,configuration:historical.value.configuration},null,2)}</pre></section>}</details>
 </div>;
}
function Reasons({values}:{values:string[]}){return values.length?<ul className="expert-plan-reasons">{values.map((value,index)=><li key={index}>{expertPlanReason(value)}</li>)}</ul>:null;}
function PlanItems({plan,project,session,deliveryAvailable=false,onChanged}:{plan:ExpertTeamPlan;project:string;session?:Session;deliveryAvailable?:boolean;onChanged?:()=>void|Promise<void>}){
 const items=expertTopology(plan.items)||[],titles=new Map(plan.items.map(item=>[item.task_id,item.plan.context.task.title||item.task_id]));
 return <ol className="expert-plan-items">{items.map(item=>{
  const review=item.state==='awaiting_review';
  const dispatch=item.plan;
  const deliveryHref=session&&plan.project_id===project&&expertID(item.dispatch_plan_id)&&item.run_id&&
   dispatch.state==='started'&&dispatch.project_id===project&&dispatch.task_id===item.task_id&&
   dispatch.id===item.dispatch_plan_id&&dispatch.run_id===item.run_id
   ?taskDeliveryHref(project,{kind:'delivery',id:item.task_id,runID:item.run_id}):undefined;
  return <li className="team-dispatch-task" data-team-item={item.task_id} key={item.task_id}><div className="team-dispatch-heading"><strong>{titles.get(item.task_id)}</strong><span className={'expert-plan-state expert-plan-state-'+item.state}>{['waiting_execution','execution_unknown'].includes(item.state)?tr('team-execution-facts.item.'+item.state):t('item.'+item.state)}</span></div><p>{item.role_title} · {item.resource_name}</p><FrozenTestBindingFacts bindings={item.plan.test_bindings}/>
  {!!item.dependencies.length&&<div className="expert-plan-dependencies" aria-label={t('dependencies')}>{item.dependencies.map(fact=><a key={fact.task_id} className={'expert-plan-dependency '+(fact.accepted?'is-accepted':'')} href={taskURL(project,fact.task_id)}>{titles.get(fact.task_id)||fact.task_id} · {t(fact.accepted?'dependency_accepted':'dependency_waiting')}{!fact.selected?' · '+t('external_dependency'):''}</a>)}</div>}
  {!!item.blockers.length&&<Reasons values={item.blockers}/>}{item.state==='awaiting_review'&&<p className="team-dispatch-muted">{t('review_required')}</p>}
  <div className="expert-plan-links">
   {deliveryHref&&<a className="expert-plan-delivery-link" data-team-task-delivery data-review={review||undefined} href={deliveryAvailable?deliveryHref:undefined}
    role={deliveryAvailable?undefined:'link'} aria-disabled={!deliveryAvailable||undefined}
    onClick={event=>{if(deliveryAvailable)followTaskDeliveryHref(event,deliveryHref);else event.preventDefault();}}>
    {t(review?'review_delivery':'open_delivery')}<ArrowRight size={15} aria-hidden="true"/>
   </a>}
   {(!deliveryHref||!review)&&<a href={taskURL(project,item.task_id)}>{t('open_task')}</a>}
   {item.run_id&&item.dispatch_plan_id&&<a data-team-run href={runURL(project,item.run_id)}>{old('open_run')}</a>}
  </div>
  {session&&expertUnstarted(item)&&<div className="expert-plan-baseline-link"><a href={taskURL(project,item.task_id)+'&panel=code-baseline'}>{tr('candidate-composition.development_baseline_title')}</a><p className="team-dispatch-muted">{tr('candidate-composition.pending_baseline_help')}</p></div>}
  {compositionCodeBaseline(item.plan.code_baseline,project,item.task_id)&&<p className="team-dispatch-muted">{tr('candidate-composition.selected_base')} <code>{item.plan.code_baseline.source.head_commit}</code></p>}
  {item.run_id&&<p className="team-dispatch-muted">{t('actual_run_state',{state:item.run_state?stateLabel(item.run_state):old('run_unknown')})}</p>}
  {session&&item.run_id&&<ExpertTeamExecutionFacts project={project} runID={item.run_id} execution={item.execution}/>}
  {session&&item.state==='blocked'&&item.run_id&&<FailureDiagnosisLauncher session={session} project={project} runID={item.run_id} refresh={plan.version} onChanged={onChanged}/>}
  <ExecutionModelFacts binding={item.plan.model_execution} codex={item.plan.codex_execution} openCode={item.plan.opencode_execution} claudeSDK={item.plan.claude_sdk_execution}/>
  {item.plan.codex_execution?<CodexNativeSkillFacts mode="plan" bundle={item.plan.execution_skills}/>:item.plan.context.knowledge?.some(source=>source.kind==='skill')&&<CodexNativeSkillFacts mode="text"/>}
  <details><summary>{old('fixed_basis')}</summary><p>{old('task_version',{value:formatNumber(item.task_version)})} · {t('revision',{value:formatNumber(item.source_revision)})}</p><code>{item.task_basis_sha256}</code><p>{item.plan.tool_profile_id} · v{formatNumber(item.plan.tool_profile_version)}</p><code>{item.plan.tool_profile_sha256}</code><DispatchRouteFacts route={item.plan.execution_route}/><KnowledgeContextFacts project={project} items={item.plan.context.knowledge||[]}/><pre className="team-dispatch-raw">{JSON.stringify({task:item.plan.context.task,workflow:item.plan.workflow,code_baseline:item.plan.code_baseline,dependencies:item.dependencies,dispatch_plan_id:item.dispatch_plan_id,run_id:item.run_id},null,2)}</pre></details>
 </li>;})}</ol>;
}
