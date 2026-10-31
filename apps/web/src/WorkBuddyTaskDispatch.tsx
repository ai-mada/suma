import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import {ArrowUpRight,Cloud,Play,RefreshCw} from 'lucide-react';
import type {WorkBuddyConnection,WorkBuddyTaskDispatchPlan,Run} from '../../../packages/contracts/src';
import {ApiError,type RecordItem,type Session} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Loading,Modal} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useWriteAction,WriteNotice} from './write-action';
import {readWorkBuddyPage,workBuddyPageSize} from './workbuddy-connections-model';
import {cloudDispatchPlanPath,cloudDispatchPreviewPath,cloudDispatchRecoveryKind,cloudDispatchRecoveryPlanID,cloudDispatchStartPath,readWorkBuddyTaskDispatchPlan,validCloudDispatchScope,validWorkBuddyTaskDispatchReceipt,validWorkBuddyTaskDispatchRecovery,type CloudDispatchScope} from './workbuddy-task-dispatch-model';
import {useKnowledgeEntries} from './knowledge-hooks';
import {pinnedKnowledgeReferences,publishedKnowledgeReference,type KnowledgeSummary} from './knowledge-model';
import {KnowledgeContextFacts} from './KnowledgeFacts';
import {RequirementDesignFacts} from './RequirementDesignFacts';
import './WorkBuddyTaskDispatch.css';

export type WorkBuddyTaskDispatchProps={project:string;session:Session;task:RecordItem;planID?:string;onPlan:(id?:string)=>void;onRun:(runID:string,nodeID:string)=>void;onAccessDenied?:(error:unknown)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-task-dispatch.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const maximum=2147483647;
const eligible=(connection:WorkBuddyConnection)=>connection.state==='connected'&&connection.version<maximum&&connection.grant_generation>0&&['user.task.readable','user.task.invokable'].every(value=>connection.scopes.includes(value))&&(!connection.expires_at||Date.parse(connection.expires_at)>Date.now());

/** The controller outlives its modal and route locator. Its personal journal
 * always retains the exact original preview or start until acknowledged. */
export function WorkBuddyTaskDispatch(props:WorkBuddyTaskDispatchProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope={project:props.project,userID:props.session.user.id,organizationID:props.session.organization.id,taskID:props.task.id};
 const allowed=!!view&&view.userID===scope.userID&&view.organizationID===scope.organizationID&&validCloudDispatchScope(scope);
 return <DispatchController {...props} key={JSON.stringify([scope,owner.current.epoch,allowed])} scope={scope} allowed={allowed}/>;
}
type Accounts={items:WorkBuddyConnection[];cursor:string;loading:boolean;error?:unknown};
type PlanRead={id:string;value?:WorkBuddyTaskDispatchPlan;loading:boolean;error?:unknown};
function DispatchController({project,task,planID,onPlan,onRun,onAccessDenied,scope,allowed}:WorkBuddyTaskDispatchProps&{scope:CloudDispatchScope;allowed:boolean}){
 useLocale();const api=useApi(),open=planID!==undefined;
 const [accounts,setAccounts]=useState<Accounts>({items:[],cursor:'',loading:false}),[selected,setSelected]=useState(''),[snapshot,setSnapshot]=useState<PlanRead>(),[localID,setLocalID]=useState<string>(),[denial,setDenial]=useState<unknown>(),[confirmed,setConfirmed]=useState(false),[showKnowledge,setShowKnowledge]=useState(false),[selection,setSelection]=useState<KnowledgeSummary[]>([]),[freshTask,setFreshTask]=useState<{propVersion:number;value:RecordItem}>();
 const alive=useRef(true),deniedNow=useRef(false),epoch=useRef(0),accountEpoch=useRef(0),planAbort=useRef<AbortController|undefined>(undefined),accountAbort=useRef<AbortController|undefined>(undefined),planNow=useRef<WorkBuddyTaskDispatchPlan|undefined>(undefined),notify=useRef(onAccessDenied),notified=useRef(false),commandBusy=useRef(false),accountsNow=useRef(accounts),taskProp=useRef(task.version);
 notify.current=onAccessDenied;accountsNow.current=accounts;taskProp.current=task.version;
 const currentTask=freshTask?.propVersion===task.version?freshTask.value:task;
 const action=useWriteAction({scope:'workbuddy-task-dispatch:'+JSON.stringify([scope.userID,scope.organizationID,project,task.id]),storage:toolSessionStorage(),validateRequest:request=>validWorkBuddyTaskDispatchRecovery(request,scope),validateResponse:(value,request)=>validWorkBuddyTaskDispatchReceipt(value,request,scope,planNow.current)});
 const pendingKind=cloudDispatchRecoveryKind(action.pending,scope),originalID=cloudDispatchRecoveryPlanID(action.pending,scope),activeID=originalID||(planID&&planID!=='new'?planID:localID),form=!activeID;
 const knowledge=useKnowledgeEntries(project,{kind:'all',state:'published',q:''},0,allowed&&open&&form&&showKnowledge&&!denial);
 const knowledgeCurrent=selection.every(entry=>knowledge.items?.some(current=>current.id===entry.id&&current.published_eligible&&current.eligibility==='current'&&current.published_revision===entry.published_revision&&current.published_sha256===entry.published_sha256));
 const clearPrivate=useCallback((error:unknown)=>{
  if(!alive.current)return;deniedNow.current=true;++epoch.current;++accountEpoch.current;planAbort.current?.abort();accountAbort.current?.abort();planNow.current=undefined;setSnapshot(undefined);setAccounts({items:[],cursor:'',loading:false});setSelected('');setSelection([]);setShowKnowledge(false);setFreshTask(undefined);setConfirmed(false);setDenial(error);
  if(!notified.current){notified.current=true;notify.current?.(error);}
 },[]);
 const readPlan=useCallback(async(id:string):Promise<WorkBuddyTaskDispatchPlan|undefined>=>{
  if(!allowed)return;
  const generation=++epoch.current;planAbort.current?.abort();const abort=new AbortController();planAbort.current=abort;planNow.current=undefined;setSnapshot({id,loading:true});
  try{
   if(!/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(id))throw new UiError('workbuddy-task-dispatch.invalid_read');
   const raw=await api<unknown>(cloudDispatchPlanPath(scope,id),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
   if(!alive.current||abort.signal.aborted||generation!==epoch.current)return;
   const value=readWorkBuddyTaskDispatchPlan(raw,scope);if(!value||value.id!==id)throw new UiError('workbuddy-task-dispatch.invalid_read');
   deniedNow.current=false;notified.current=false;setDenial(undefined);planNow.current=value;setSnapshot({id,value,loading:false});return value;
  }catch(error){
   if(!alive.current||abort.signal.aborted||generation!==epoch.current)return;
   if(refused(error)){clearPrivate(error);return;}
   setSnapshot({id,loading:false,error});
  }
 },[api,allowed,scope.project,scope.userID,scope.organizationID,scope.taskID,clearPrivate]);
 const readAccounts=useCallback(async(more=false):Promise<boolean>=>{
  if(!allowed)return false;
  const before=accountsNow.current,cursor=more?before.cursor:'';if(more&&!cursor)return false;
  const generation=++accountEpoch.current;accountAbort.current?.abort();const abort=new AbortController();accountAbort.current=abort;
  setAccounts({items:more?before.items:[],cursor:'',loading:true});
  try{
   const raw=await api<unknown>('/workbuddy-connections?'+new URLSearchParams({project_id:project,limit:String(workBuddyPageSize),...(cursor?{cursor}:{})}),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
   if(!alive.current||abort.signal.aborted||generation!==accountEpoch.current)return false;
   const page=readWorkBuddyPage(raw,project,scope.userID);if(!page||cursor&&page.next_cursor===cursor)throw new UiError('workbuddy-task-dispatch.invalid_read');
   const items=more?[...before.items,...page.items]:page.items;
   if(new Set(items.map(item=>item.id)).size!==items.length)throw new UiError('workbuddy-task-dispatch.invalid_read');
   deniedNow.current=false;notified.current=false;setDenial(undefined);setAccounts({items,cursor:page.next_cursor,loading:false});
   const choices=items.filter(eligible);setSelected(previous=>items.some(item=>item.id===previous&&eligible(item))?previous:!page.next_cursor&&choices.length===1?choices[0].id:'');return true;
  }catch(error){
   if(!alive.current||abort.signal.aborted||generation!==accountEpoch.current)return false;
   if(refused(error)){clearPrivate(error);return false;}
   setAccounts({items:more?before.items:[],cursor:more?cursor:'',loading:false,error});return false;
  }
 },[api,allowed,project,scope.userID,scope.organizationID,clearPrivate]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;++accountEpoch.current;planAbort.current?.abort();accountAbort.current?.abort();};},[]);
 useEffect(()=>{if(activeID)void readPlan(activeID);return()=>{++epoch.current;planAbort.current?.abort();};},[activeID,readPlan]);
 useEffect(()=>{if(open&&form)void readAccounts();return()=>{++accountEpoch.current;accountAbort.current?.abort();};},[open,form,readAccounts]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(writeDenied)clearPrivate(new ApiError(action.errorStatus!,'workbuddy_task_dispatch_access_changed',t('access_changed')));},[writeDenied,action.errorStatus,clearPrivate]);
 useLayoutEffect(()=>{if(refused(knowledge.error))clearPrivate(knowledge.error);},[knowledge.error,clearPrivate]);
 const denied=!allowed||!!denial||writeDenied,plan=!denied&&snapshot&&snapshot.id===activeID?snapshot.value:undefined;
 const locked=action.busy||!!action.pending||denied,connection=!denied?accounts.items.find(item=>item.id===selected&&eligible(item)):undefined;
 const changed=!!plan&&plan.task_version!==currentTask.version,expired=!!plan&&Date.parse(plan.expires_at)<=Date.now();
 const canPreview=!locked&&!action.conflict&&!accounts.loading&&!accounts.error&&!!connection&&currentTask.status!=='accepted'&&currentTask.version>0&&currentTask.version<maximum&&knowledgeCurrent;
 const canStart=!locked&&!action.conflict&&!!plan&&plan.state==='ready'&&plan.valid&&!changed&&!expired&&plan.version<maximum&&!snapshot?.loading;
 async function refresh():Promise<boolean>{
  if(action.busy||!allowed)return false;action.setError('');
  if(activeID){if(!await readPlan(activeID))return false;action.clearConflict();return true;}
  if(action.conflict){
   const version=taskProp.current;
   try{
    const value=await api<RecordItem>('/tasks/'+encodeURIComponent(task.id)+'?project_id='+encodeURIComponent(project),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID});
    if(!alive.current||version!==taskProp.current)return false;
    if(value.id!==task.id||value.project_id!==project||!Number.isSafeInteger(value.version)||value.version<1)throw new UiError('workbuddy-task-dispatch.invalid_read');
    setFreshTask({propVersion:version,value});
   }catch(error){if(!alive.current)return false;if(refused(error))clearPrivate(error);else action.setError(error);return false;}
  }
  if(!await readAccounts())return false;action.clearConflict();return true;
 }
 async function consumePreview(value:WorkBuddyTaskDispatchPlan){
  if(!alive.current||deniedNow.current)return;setConfirmed(true);setLocalID(value.id);onPlan(value.id);
 }
 async function submit(kind:'preview'|'start'|'recover'){
  if(commandBusy.current||action.busy||denied||kind==='preview'&&!canPreview||kind==='start'&&!canStart||kind==='recover'&&(!action.pending||pendingKind==='start'&&!plan))return;
  commandBusy.current=true;setConfirmed(false);
  try{
   if(kind==='recover'){
    const original=action.pending!;
    const value=await action.run<WorkBuddyTaskDispatchPlan|Run>(original.path,original.body,original.method);
    if(!value||!alive.current||deniedNow.current)return;
    if(pendingKind==='preview')await consumePreview(value as WorkBuddyTaskDispatchPlan);
    else{setConfirmed(true);if(originalID)await readPlan(originalID);}
   }else if(kind==='preview'){
    const references=pinnedKnowledgeReferences(selection,project);
    const value=await action.run<WorkBuddyTaskDispatchPlan>(cloudDispatchPreviewPath,{project_id:project,task_id:task.id,expected_task_version:currentTask.version,connection_id:connection!.id,expected_connection_version:connection!.version,grant_generation:connection!.grant_generation,...(references.length?{knowledge:references}:{})});
    if(value)await consumePreview(value);
   }else{
    const value=await action.run<Run>(cloudDispatchStartPath(plan!.id),{project_id:project,expected_version:plan!.version});
    if(value&&alive.current&&!deniedNow.current){setConfirmed(true);await readPlan(plan!.id);}
   }
  }catch(error){if(alive.current)action.setError(error);}finally{commandBusy.current=false;}
 }
 function newPreview(){if(locked)return;setLocalID(undefined);planNow.current=undefined;setSnapshot(undefined);setConfirmed(false);action.clearConflict();onPlan('new');}
 function chooseKnowledge(entry:KnowledgeSummary){
  if(locked)return;try{publishedKnowledgeReference(entry,project);setSelection(before=>before.some(value=>value.id===entry.id)?before.filter(value=>value.id!==entry.id):[...before,entry]);}catch(error){action.setError(error);}
 }
 const accountLink='?'+new URLSearchParams({project,view:'settings',sub:'tools',panel:'workbuddy'});
 const feedback=<div className="workbuddy-dispatch-feedback">
  {denied?<p role="alert">{t('access_changed')}</p>:<><WriteNotice action={action} onRebase={async()=>{if(!await refresh())throw new UiError('workbuddy-task-dispatch.refresh_failed');}}/>{confirmed&&<p role="status">{t('original_confirmed')}</p>}</>}
  {action.pending&&<div className="workbuddy-dispatch-recovery"><p role="status">{t(pendingKind==='start'?'original_start_pending':'original_preview_pending')}</p><button type="button" data-workbuddy-dispatch-recover disabled={action.busy||denied||pendingKind==='start'&&!plan} onClick={()=>void submit('recover')}>{t('recover')}</button></div>}
 </div>;
 return <>
  <button className="full workbuddy-dispatch-entry" type="button" data-workbuddy-dispatch-entry disabled={!allowed} onClick={()=>onPlan(activeID||'new')}><Cloud size={16} aria-hidden="true"/>{t('entry')}</button>
  {!open&&(action.pending||action.error||denied)&&feedback}
  <Modal open={open} onClose={()=>onPlan(undefined)} title={t('title')} description={t('intro')}>
   <div className="workbuddy-task-dispatch" aria-busy={action.busy||!!snapshot?.loading||accounts.loading}>
    <ol className="workbuddy-dispatch-steps"><li aria-current={form?'step':undefined}>{t('account_step')}</li><li aria-current={plan?.state==='ready'?'step':undefined}>{t('preview_step')}</li><li aria-current={plan?.state==='started'?'step':undefined}>{t('dispatch_step')}</li></ol>
    <p className="workbuddy-dispatch-boundary">{t('model_boundary')}</p>
    {feedback}
    {!denied&&form&&<>
     <div className="workbuddy-dispatch-task"><strong>{currentTask.title||currentTask.id}</strong><span>{t('task_version',{value:formatNumber(currentTask.version)})}</span></div>
     <label className="workbuddy-dispatch-field"><span>{t('connection_label')}</span><select data-workbuddy-dispatch-connection value={selected} disabled={locked||accounts.loading} onChange={event=>setSelected(event.target.value)}><option value="">{t('choose_connection')}</option>{accounts.items.map(account=><option key={account.id} value={account.id} disabled={!eligible(account)}>{account.open_id||account.id}{!eligible(account)?' · '+tr('workbuddy-connections.state.'+account.state):''}</option>)}</select></label>
     <p className="workbuddy-dispatch-hint">{t('scope_hint')}</p>
     {accounts.loading&&<Loading/>}{Boolean(accounts.error)&&<ErrorNotice error={accounts.error} onRetry={()=>void refresh()}/>}
     {!accounts.loading&&!accounts.error&&(!accounts.items.length?<p>{t('no_accounts')}</p>:!accounts.items.some(eligible)&&<p>{t('no_eligible_accounts')}</p>)}
     <div className="workbuddy-dispatch-account-actions"><button type="button" data-workbuddy-dispatch-refresh disabled={action.busy||accounts.loading} onClick={()=>void refresh()}><RefreshCw size={14} aria-hidden="true"/>{t('read_current')}</button><a data-workbuddy-dispatch-accounts href={accountLink} aria-disabled={locked} onClick={event=>{if(locked||!window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))event.preventDefault();}}>{t('accounts')}<ArrowUpRight size={14} aria-hidden="true"/></a>{accounts.cursor&&<button type="button" data-workbuddy-dispatch-more-connections disabled={locked||accounts.loading} onClick={()=>void readAccounts(true)}>{t('load_more_connections')}</button>}</div>
     {accounts.cursor&&<p className="workbuddy-dispatch-hint">{t('accounts_loaded',{value:formatNumber(accounts.items.length)})}</p>}
     <details onToggle={event=>setShowKnowledge(event.currentTarget.open)}><summary>{t('knowledge')}</summary><p>{t('knowledge_intro')}</p>{showKnowledge&&<>
      {knowledge.loading&&!knowledge.items&&<Loading/>}{Boolean(knowledge.error)&&<ErrorNotice error={knowledge.error} onRetry={()=>void knowledge.read()}/>}
      <div className="knowledge-dispatch-list">{knowledge.items?.map(entry=><label className="knowledge-dispatch-choice" key={entry.id}><input type="checkbox" checked={selection.some(value=>value.id===entry.id)} disabled={locked||!entry.published_eligible||entry.eligibility!=='current'} onChange={()=>chooseKnowledge(entry)}/><span><strong>{entry.published_title||entry.id}</strong><small>{tr('knowledge.revision',{revision:formatNumber(entry.published_revision)})}</small></span></label>)}</div>
      {knowledge.items?.length===0&&!knowledge.loading&&<p>{t('knowledge_empty')}</p>}{knowledge.cursor&&<button type="button" disabled={locked||knowledge.loading} onClick={()=>void knowledge.read(true)}>{tr('knowledge.more')}</button>}
      <p>{t('knowledge_selected',{value:formatNumber(selection.length)})}</p>{!knowledgeCurrent&&<p role="alert">{t('knowledge_changed')}</p>}
     </>}</details>
     <p className="workbuddy-dispatch-hint">{t('baseline_hint')}</p>
     {currentTask.version>=maximum&&<p role="status">{t('max_version')}</p>}
     <div className="workbuddy-dispatch-actions"><button type="button" className="primary" data-workbuddy-dispatch-preview disabled={!canPreview} onClick={()=>void submit('preview')}>{t(action.busy?'previewing':'preview')}</button></div>
    </>}
    {!denied&&activeID&&<>
     {snapshot?.loading&&<Loading/>}{Boolean(snapshot?.error)&&<ErrorNotice error={snapshot?.error} onRetry={()=>void refresh()}/>}
     {plan&&<section data-workbuddy-dispatch-plan data-state={plan.state} className="workbuddy-dispatch-plan">
      <div className="workbuddy-dispatch-plan-heading"><h3>{plan.context.task.title||plan.task_id}</h3><span>{t(plan.state==='started'?'plan_started':'plan_ready')}</span></div>
      <p className="workbuddy-dispatch-hint">{t('task_version',{value:formatNumber(plan.task_version)})} · {plan.execution_route.pool_id?t('route_pool',{value:plan.execution_route.pool_id}):t('route_private')}</p>
      {plan.state==='ready'&&<p className="workbuddy-dispatch-hint"><time dateTime={plan.expires_at}>{t('expires',{at:formatDateTime(plan.expires_at,{dateStyle:'short',timeStyle:'short'})})}</time></p>}
      {plan.state!=='started'&&(!plan.valid||changed||expired)&&<p role="alert">{t(changed?'source_changed':expired?'plan_expired':'plan_invalid')}</p>}
      <details open><summary>{t('context')}</summary><article><h4>{t('task')}</h4><p className="workbuddy-dispatch-text">{plan.context.task.description}</p><h4>{t('acceptance')}</h4><ul>{plan.context.task.acceptance_criteria.map((line,index)=><li key={index}>{line}</li>)}</ul></article>{plan.context.requirement&&<article><h4>{t('requirement')}: {plan.context.requirement.title}</h4><p className="workbuddy-dispatch-text">{plan.context.requirement.description}</p><ul>{plan.context.requirement.acceptance_criteria.map((line,index)=><li key={index}>{line}</li>)}</ul></article>}
       <RequirementDesignFacts project={project} value={plan.context.design??undefined}/>
       {plan.context.test_plan?<article><h4>{t('test_plan')}: {plan.context.test_plan.title}</h4><p>{t('test_cases',{value:formatNumber(plan.context.test_plan.case_count)})}</p></article>:<p className="workbuddy-dispatch-hint">{t('no_test_plan')}</p>}
       <KnowledgeContextFacts project={project} items={plan.context.knowledge}/>
      </details>
      <details><summary>{t('fixed_ids')}</summary><dl><dt>{t('task')}</dt><dd>{plan.task_id}</dd><dt>{t('connection_label')}</dt><dd>{plan.connection_id}</dd><dt>{t('details')}</dt><dd>{plan.node_id}<br/>{plan.node_spec_sha256}<br/>{plan.request_sha256}</dd></dl></details>
      {plan.reason&&plan.state!=='started'&&<details><summary>{t('raw_reason')}</summary><pre>{plan.reason}</pre></details>}
      <p className="workbuddy-dispatch-boundary">{t('delivery_boundary')}</p>
      <div className="workbuddy-dispatch-actions">{plan.state==='ready'?<button type="button" className="primary" data-workbuddy-dispatch-start disabled={!canStart} onClick={()=>void submit('start')}><Play size={15} aria-hidden="true"/>{t(action.busy?'dispatching':'dispatch')}</button>:!action.pending&&<button type="button" className="primary" data-workbuddy-dispatch-open-run disabled={action.busy} onClick={()=>onRun(plan.run_id,plan.node_id)}>{t('open_run')}<ArrowUpRight size={15} aria-hidden="true"/></button>}<button type="button" data-workbuddy-dispatch-refresh disabled={action.busy||snapshot?.loading} onClick={()=>void refresh()}><RefreshCw size={14} aria-hidden="true"/>{t('read_current')}</button><button type="button" data-workbuddy-dispatch-new disabled={locked} onClick={newPreview}>{t('new_preview')}</button></div>
     </section>}
    </>}
    {denied&&<button type="button" data-workbuddy-dispatch-refresh disabled={!allowed||action.busy} onClick={()=>void refresh()}>{t('read_current')}</button>}
   </div>
  </Modal>
 </>;
}
