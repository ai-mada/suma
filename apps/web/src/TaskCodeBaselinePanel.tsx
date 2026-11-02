import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,GitBranch} from 'lucide-react';
import {ApiError,query,type RecordItem,type Session} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Field,Loading} from './ui';
import {toolError,toolSessionStorage} from './tool-hooks';
import {useWriteAction,WriteFields,WriteNotice} from './write-action';
import {compositionCursor,compositionID,compositionInteger,compositionReason,compositionRecovery,compositionSourceKey,compositionText,validCodeBaseline,validCompositionOptions,validCompositionPlan,validCompositionTask,type CandidateCompositionBaseline,type CandidateCompositionPlan,type CandidateCompositionOptions,type CandidateCompositionSourceChoice} from './candidate-composition-model';
import {useCompositionPage,useCompositionRead,type CompositionPage} from './candidate-composition-hooks';
import './CandidateCompositionPanel.css';

export type TaskCodeBaselinePanelProps={session:Session;project:string;taskID?:string;composition?:CandidateCompositionPlan;compositionLoading?:boolean;readComposition?:()=>Promise<CandidateCompositionPlan|undefined>;refresh?:number;initiallyOpen?:boolean;onChanged?:()=>void|Promise<void>;onLock?:(locked:boolean)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('candidate-composition.'+key,values);
const sourceText=(key:string,values?:Record<string,unknown>)=>tr('task-code-baseline-source.'+key,values);
const sourceBasis=(value:CandidateCompositionSourceChoice)=>JSON.stringify([value.task_id,value.task_version,value.source_node_id,value.source]);
const taskURL=(project:string,id:string)=>'?'+new URLSearchParams({project,view:'development',sub:'tasks',record:id});
const runURL=(project:string,id:string)=>'?'+new URLSearchParams({project,view:'development',sub:'runs',run:id});
const compositionURL=(project:string,plan:CandidateCompositionPlan)=>taskURL(project,plan.task_id)+'&panel='+encodeURIComponent('composition:'+plan.id);
const planInProject=(value:unknown,project:string,id?:string):value is CandidateCompositionPlan=>!!value&&typeof value==='object'&&'task_id' in value&&typeof value.task_id==='string'&&validCompositionPlan(value,project,value.task_id,id);
export function TaskCodeBaselinePanel(props:TaskCodeBaselinePanelProps){
 const api=useApi(),basis=JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.taskID||props.composition?.id]),owner=useRef({api,basis,epoch:0});
 if(owner.current.api!==api||owner.current.basis!==basis)owner.current={api,basis,epoch:owner.current.epoch+1};
 return <BaselineScope key={basis+':'+owner.current.epoch} {...props}/>;
}
function BaselineScope({session,project,taskID,composition,compositionLoading=false,readComposition,refresh=0,initiallyOpen=false,onChanged,onLock}:TaskCodeBaselinePanelProps){
 useLocale();const api=useApi(),bodyID=useId(),[open,setOpen]=useState(initiallyOpen),[targetID,setTargetID]=useState(taskID||''),[selectedID,setSelectedID]=useState(''),[tick,setTick]=useState(0),[reason,setReason]=useState(''),[ack,setAck]=useState(''),[bound,setBound]=useState(false),[boundRequest,setBoundRequest]=useState<Record<string,unknown>>(),[forbidden,setForbidden]=useState<unknown>(),alive=useRef(true),callbacks=useRef({onChanged,readComposition,onLock});callbacks.current={onChanged,readComposition,onLock};
 const [origin,setOrigin]=useState<'accepted_task'|'composition'>(composition?'composition':'accepted_task'),[taskSource,setTaskSource]=useState<CandidateCompositionSourceChoice>();
 const fixedID=composition?.id,scope=JSON.stringify(['task-code-baseline',session.user.id,session.organization.id,project,taskID?'task:'+taskID:'composition:'+fixedID]);
 const action=useWriteAction({scope,storage:toolSessionStorage(),validateRequest:request=>compositionRecovery(request,project,taskID||composition?.task_id||'')&&!!request.path.match(/^\/tasks\/[A-Za-z0-9_-]+\/code-baseline$/)&&(!taskID||request.path==='/tasks/'+encodeURIComponent(taskID)+'/code-baseline')&&(!fixedID||request.body.composition_id===fixedID)}),pending=action.pending,locked=action.busy||!!pending||action.persistenceBlocked;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{callbacks.current.onLock?.(locked);return()=>callbacks.current.onLock?.(false);},[locked]);
 useEffect(()=>{if(initiallyOpen)setOpen(true);},[initiallyOpen]);
 useEffect(()=>{if(pending){const target=pending.path.match(/^\/tasks\/([A-Za-z0-9_-]+)\/code-baseline$/)?.[1];if(target){setOpen(true);setTargetID(target);setOrigin(typeof pending.body.source_task_id==='string'?'accepted_task':'composition');setSelectedID(typeof pending.body.composition_id==='string'?pending.body.composition_id:'');setReason(typeof pending.body.reason==='string'?pending.body.reason:'');setAck('');}}},[!!pending]);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status)){setForbidden(error);setAck('');}},[]);
 const visible=open&&!forbidden&&(!!taskID||composition?.state==='accepted'||!!pending);
 const targetValid=useCallback((value:unknown):value is RecordItem=>validCompositionTask(value,targetID,project),[project,targetID]);
 const target=useCompositionRead(api,'/tasks/'+encodeURIComponent(targetID)+query(project),visible&&!!targetID,refresh+tick,targetValid,failed);
 const baselineValid=useCallback((value:unknown):value is CandidateCompositionBaseline=>validCodeBaseline(value,project,targetID),[project,targetID]);
 const baseline=useCompositionRead(api,'/tasks/'+encodeURIComponent(targetID)+'/code-baseline'+query(project),visible&&!!targetID,refresh+tick,baselineValid,failed);
 const taskPageValid=useCallback((value:unknown):value is CompositionPage<RecordItem>=>{const page=value as CompositionPage<RecordItem>|undefined;return !!page&&Array.isArray(page.items)&&page.items.length<=100&&compositionInteger(page.total)&&page.total>=page.items.length&&compositionCursor(page.next_cursor)&&page.items.every(item=>validCompositionTask(item,undefined,project));},[project]);
 const tasks=useCompositionPage(api,'/tasks'+query(project)+'&limit=100',visible&&!taskID,refresh+tick,taskPageValid,failed,'cursor');
 const pageValid=useCallback((value:unknown):value is CompositionPage<CandidateCompositionPlan>=>{const page=value as CompositionPage<CandidateCompositionPlan>|undefined;return !!page&&Array.isArray(page.items)&&page.items.length<=20&&compositionInteger(page.total)&&page.total>=page.items.length&&compositionCursor(page.next_cursor)&&page.items.every(item=>planInProject(item,project));},[project]);
 const mode=fixedID?'composition':origin;
 const choices=useCompositionPage(api,'/candidate-compositions'+query(project)+'&limit=20',visible&&!fixedID&&mode==='composition'&&!!baseline.value?.can_bind&&!pending,refresh+tick,pageValid,failed);
 const sourceOptionsValid=useCallback((value:unknown):value is CandidateCompositionOptions=>validCompositionOptions(value,project,targetID),[project,targetID]);
 const sources=useCompositionRead(api,'/candidate-compositions/options'+query(project)+'&task_id='+encodeURIComponent(targetID),visible&&!fixedID&&mode==='accepted_task'&&!!baseline.value?.can_bind&&!pending,refresh+tick,sourceOptionsValid,failed);
 const selectedValid=useCallback((value:unknown):value is CandidateCompositionPlan=>planInProject(value,project,selectedID),[project,selectedID]);
 const selected=useCompositionRead(api,'/candidate-compositions/'+encodeURIComponent(selectedID)+query(project),visible&&!fixedID&&mode==='composition'&&!!selectedID&&!pending,refresh+tick,selectedValid,failed),plan=composition||selected.value;
 const current=!!target.value&&!target.loading&&!target.error&&!!baseline.value&&!baseline.loading&&!baseline.error&&!forbidden;
 const sourceChoice=taskSource&&sources.value?.sources.find(value=>compositionSourceKey(value)===compositionSourceKey(taskSource));
 const taskSourceCurrent=!!taskSource&&!!sourceChoice&&sourceChoice.can_bind_baseline&&sourceChoice.task_id!==targetID&&sourceBasis(sourceChoice)===sourceBasis(taskSource)&&!sources.loading&&!sources.error&&sources.value?.task_version===target.value?.version;
 const sourceCurrent=mode==='accepted_task'?taskSourceCurrent:!!plan&&!compositionLoading&&(!!fixedID||!selected.loading&&!selected.error)&&plan.state==='accepted'&&plan.valid&&plan.task_id!==targetID;
 const canBind=current&&sourceCurrent&&!!baseline.value?.can_bind&&!pending;
 const selectionBasis=JSON.stringify([targetID,target.value?.version,baseline.value?.version,mode,mode==='accepted_task'&&taskSource?sourceBasis(taskSource):[plan?.id,plan?.version,plan?.state,plan?.candidate],canBind]);
 const confirmation=useRef({basis:selectionBasis,epoch:0});if(confirmation.current.basis!==selectionBasis)confirmation.current={basis:selectionBasis,epoch:confirmation.current.epoch+1};
 const confirmationKey=JSON.stringify([selectionBasis,confirmation.current.epoch]),acknowledged=canBind&&ack===confirmationKey;
 const actualBinding=baseline.value?.binding,boundCurrent=current&&!!actualBinding&&!!boundRequest&&(typeof boundRequest.source_task_id==='string'?actualBinding.source_task_id===boundRequest.source_task_id&&actualBinding.source_task_version===boundRequest.expected_source_task_version&&actualBinding.source.source.node_id===boundRequest.source_node_id:typeof boundRequest.composition_id==='string'?actualBinding.composition_id===boundRequest.composition_id&&actualBinding.composition_version===boundRequest.expected_composition_version:actualBinding.terminal_candidate_plan_id===boundRequest.terminal_candidate_plan_id&&actualBinding.terminal_candidate_plan_version===boundRequest.expected_terminal_candidate_plan_version);
 const nextHead=mode==='accepted_task'?taskSource?.source.head_commit:plan?.candidate?.head_commit;
 async function completed(value:unknown,request:Record<string,unknown>){if(!alive.current||value===undefined)return;if(!validCompositionTask(value,targetID,project)){action.setError(new UiError('candidate-composition.invalid_read'));return;}setBoundRequest(request);setBound(true);setAck('');await Promise.allSettled([target.read(),baseline.read()]);if(alive.current)try{await callbacks.current.onChanged?.();}catch(error){action.setError(error);}}
 async function bind(){
  if(!canBind||!acknowledged||!compositionText(reason)||locked||action.conflict||!target.value||!baseline.value)return;
  const source=mode==='accepted_task'&&taskSource?{source_task_id:taskSource.task_id,expected_source_task_version:taskSource.task_version,source_node_id:taskSource.source_node_id}:plan?{composition_id:plan.id,expected_composition_version:plan.version}:undefined;if(!source)return;
  const body={project_id:project,expected_task_version:target.value.version,expected_binding_version:baseline.value.version,...source,acknowledge:true,reason};
  await completed(await action.run('/tasks/'+encodeURIComponent(targetID)+'/code-baseline',body),body);
 }
 async function rebase(){setAck('');const source=mode==='accepted_task'?await sources.read():fixedID?await callbacks.current.readComposition?.():await selected.read();if(!source)throw new UiError('candidate-composition.refresh_failed');const values=await Promise.all([target.read(),baseline.read()]);if(values.some(value=>!value))throw new UiError('candidate-composition.refresh_failed');}
 // Original uncertain requests retain their exact body/key even when the
 // source disappeared or no longer qualifies for a new binding. The server
 // authorizes the original receipt; the current options page is not authority.
 const restoreReady=!!pending&&current&&pending.path==='/tasks/'+encodeURIComponent(targetID)+'/code-baseline';
 async function restore(){if(!pending||!restoreReady)return;await completed(await action.run(pending.path,pending.body,pending.method),pending.body);}
 if(!taskID&&composition?.state!=='accepted'&&!pending)return null;
 const errors=[target.error,baseline.error,tasks.error,choices.error,selected.error,sources.error].filter(Boolean),acceptedChoices=choices.value?.items.filter(value=>value.state==='accepted'&&value.valid&&value.task_id!==taskID)||[];
 return <section className="candidate-composition composition-task-baseline">
  <button data-composition-binding-toggle type="button" className="composition-toggle" aria-expanded={open} aria-controls={bodyID} disabled={action.busy} onClick={()=>setOpen(value=>!value)}><GitBranch size={16}/><span>{t(taskID?'development_baseline_title':'baseline_title')}</span><ChevronDown size={16}/></button>
  {open&&<div id={bodyID} className="composition-body">
   <p className="composition-muted">{taskID?sourceText('help'):t('baseline_help')}</p>
   {Boolean(forbidden)&&<ErrorNotice error={toolError(forbidden)} onRetry={()=>{setForbidden(undefined);setTick(value=>value+1);}}/>}
   <WriteNotice action={action} onRebase={rebase}/>
   {pending&&<div className="warning"><p>{t('recovery')}</p>{typeof pending.body.source_task_id==='string'&&<p data-task-baseline-pending-source>{sourceText('pending_source',{task:pending.body.source_task_id,version:formatNumber(Number(pending.body.expected_source_task_version)),node:pending.body.source_node_id})}</p>}<button data-composition-restore type="button" disabled={!restoreReady||action.busy} onClick={()=>void restore()}>{t('restore')}</button></div>}
   {errors.map((error,index)=><ErrorNotice key={index} error={toolError(error)}/>)}
   {!taskID&&<><Field label={t('target_task')}><select name="composition_target" aria-label={t('target_task')} value={targetID} disabled={locked||tasks.loading||!!forbidden} onChange={event=>{setTargetID(event.target.value);setAck('');setBound(false);setTaskSource(undefined);}}><option value="">{t('choose_task')}</option>{tasks.value?.items.filter(value=>value.id!==plan?.task_id).map(value=><option key={value.id} value={value.id}>{value.title} · v{formatNumber(value.version)}</option>)}</select></Field>{tasks.value?.next_cursor&&<button type="button" disabled={locked||tasks.loading} onClick={()=>void tasks.read(true)}>{t('more_tasks')}</button>}</>}
   {(target.loading||baseline.loading)&&<Loading/>}
   {target.value&&<p>{target.value.title} · v{formatNumber(target.value.version)}</p>}
   {baseline.value&&<>
    <Reasons values={baseline.value.blockers}/>
    <dl className="composition-facts"><dt>{t('previous_baseline')}</dt><dd><code>{actualBinding?.source.head_commit||t('admin_default')}</code></dd>{nextHead&&<><dt>{t('next_baseline')}</dt><dd><code>{nextHead}</code></dd></>}</dl>
    {actualBinding&&<details><summary>{t('baseline_facts')}</summary><a href={runURL(project,actualBinding.source.source.run_id)}>{t('source_run')}</a><pre className="composition-raw">{JSON.stringify(actualBinding,null,2)}</pre></details>}
    {!baseline.value.can_bind&&<p className="composition-muted">{t('binding_unavailable')}</p>}
    {baseline.value.can_bind&&<>
     {!fixedID&&<Field label={sourceText('origin')}><select name="task_baseline_origin" data-task-baseline-origin value={mode} disabled={locked||!!forbidden} onChange={event=>{setOrigin(event.target.value as typeof origin);setSelectedID('');setTaskSource(undefined);setAck('');setBound(false);}}><option value="accepted_task">{sourceText('accepted_task')}</option><option value="composition">{sourceText('composition')}</option></select></Field>}
     {!fixedID&&mode==='accepted_task'&&<>
      <Field label={sourceText('source')}><select name="task_baseline_source" data-task-baseline-source value={taskSourceCurrent&&taskSource?compositionSourceKey(taskSource):''} disabled={locked||sources.loading||!!forbidden} onChange={event=>{const value=sources.value?.sources.find(item=>compositionSourceKey(item)===event.target.value);setTaskSource(value?.can_bind_baseline&&value.task_id!==targetID?value:undefined);setAck('');setBound(false);}}><option value="">{sourceText('choose')}</option>{sources.value?.sources.map(value=><option key={compositionSourceKey(value)} value={compositionSourceKey(value)} disabled={!value.can_bind_baseline||value.task_id===targetID}>{sourceText('option',{title:value.title||value.task_id,version:formatNumber(value.task_version),node:value.source_node_id})}</option>)}</select></Field>
      {sources.loading&&<Loading/>}
      {sources.value&&<small className="composition-muted">{sourceText('bounded')}</small>}
      {sources.value&&!sources.value.sources.some(value=>value.can_bind_baseline&&value.task_id!==targetID)&&<p className="composition-muted">{sourceText('empty')}</p>}
      {taskSource&&!taskSourceCurrent&&!sources.loading&&<p role="status" className="warning" data-task-baseline-source-changed>{sourceText('changed')}</p>}
      {taskSourceCurrent&&taskSource&&<div data-task-baseline-candidate><a href={taskURL(project,taskSource.task_id)}>{taskSource.title||taskSource.task_id} · v{formatNumber(taskSource.task_version)}</a><dl className="composition-facts"><dt>{sourceText('node')}</dt><dd>{taskSource.source_node_id}</dd><dt>{sourceText('commit')}</dt><dd><code>{taskSource.source.head_commit}</code></dd></dl><a href={runURL(project,taskSource.source.source.run_id)+'&node='+encodeURIComponent(taskSource.source_node_id)+'&attempt='+taskSource.source.source.attempt}>{sourceText('open_source')}</a></div>}
      <button type="button" data-task-baseline-source-refresh disabled={locked||sources.loading||target.loading||baseline.loading||!!forbidden} onClick={()=>void rebase().catch(error=>action.setError(error))}>{sourceText('refresh')}</button>
     </>}
     {!fixedID&&mode==='composition'&&<><Field label={t('accepted_composition')}><select name="composition_baseline_source" aria-label={t('accepted_composition')} value={selectedID} disabled={locked||choices.loading||!!forbidden} onChange={event=>{setSelectedID(event.target.value);setAck('');setBound(false);}}><option value="">{t('choose_composition')}</option>{acceptedChoices.map(value=><option value={value.id} key={value.id}>{value.context.task.title||value.task_id} · {value.candidate?.head_commit.slice(0,12)}</option>)}{selectedID&&!acceptedChoices.some(value=>value.id===selectedID)&&<option value={selectedID} disabled>{selectedID}</option>}</select></Field>{choices.loading&&<Loading/>}{choices.value&&!acceptedChoices.length&&<p className="composition-muted">{t('no_accepted_compositions')}</p>}{choices.value?.next_cursor&&<button type="button" disabled={locked||choices.loading} onClick={()=>void choices.read(true)}>{t('more_compositions')}</button>}</>}
     {mode==='composition'&&plan&&<><Reasons values={plan.blockers}/><a href={compositionURL(project,plan)}>{t('open_composition')}</a></>}
     <form data-composition-bind onSubmit={event=>{event.preventDefault();void bind();}}><WriteFields action={action}><Field label={t('bind_reason')}><textarea name="composition_bind_reason" aria-label={t('bind_reason')} rows={3} maxLength={2000} value={reason} onChange={event=>{setReason(event.target.value);setBound(false);}}/></Field><label className="composition-check"><input name="composition_bind_ack" aria-label={t('bind_ack')} type="checkbox" checked={acknowledged} disabled={!canBind||!!forbidden} onChange={event=>setAck(event.target.checked?confirmationKey:'')}/><span>{t('bind_ack')}</span></label></WriteFields><button data-composition-bind-submit type="submit" className="primary" disabled={!canBind||!acknowledged||!compositionText(reason)||locked||action.conflict}>{t('bind')}</button></form>
    </>}
    {bound&&boundCurrent&&<p className="composition-success" role="status">{t('bound')} <a href={taskURL(project,targetID)}>{t('open_target')}</a></p>}
   </>}
  </div>}
 </section>;
}
function Reasons({values}:{values:string[]}){return values.length?<ul className="composition-reasons">{values.map((value,index)=><li key={index}>{compositionReason(value)}</li>)}</ul>:null;}
