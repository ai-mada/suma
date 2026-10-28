import {useCallback,useEffect,useId,useMemo,useRef,useState} from 'react';
import {ChevronDown,Sparkles} from 'lucide-react';
import type {PlanningAnalysis,PlanningAlert,PlanningOutput,ToolProfile} from '../../../packages/contracts/src';
import {query,type ApiCaller,type Session} from './api';
import {useApi} from './api-context';
import type {PlannedTask} from './planning-model';
import {automationPermissions} from './automation-model';
import {connectRunEvents,runConnectionMessage,SnapshotRefreshQueue,type RunConnectionState} from './run-events';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading} from './ui';
import {useWriteAction,WriteButton,WriteFields,WriteNotice,type WriteRecoveryRequest} from './write-action';
import './PlanningIntelligence.css';
import {ProjectRiskRegistration} from './ProjectRisks';

type Props={session:Session;project:string;tasks:PlannedTask[];refresh:number;onChanged:()=>void|Promise<void>;onOpenRisk?:(id:string)=>void};
type Analysis=PlanningAnalysis&{original_output?:string};
type Planner=ToolProfile&{profile:ToolProfile['profile']&{planning?:{protocol:string;model:string;endpoint_origin:string}|null}};
type Page<T>={items:T[];total:number;next_cursor?:string;open_count?:number;critical_count?:number};
type Editor={id:string;version:number;raw:string};
const text=(key:string,values?:Record<string,unknown>)=>tr('intelligent-planning.'+key,values);
const positive=(value:unknown)=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const boundedText=(value:unknown,max=4000)=>typeof value==='string'&&!!value.trim()&&new TextEncoder().encode(value).length<=max;
const id=(value:unknown):value is string=>typeof value==='string'&&value.length>0&&value.length<=64&&!/[\x00-\x1f\x7f]/.test(value);
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const analysisStates=new Set(['running','ready','failed','reconciling','applied','dismissed']);
const reasonKeys:Record<string,string>={
 'Invalid intelligent planning request':'invalid_request',
 'The model planning output does not satisfy the fixed proposal contract':'invalid_output',
 'Planning source has changed; create a new analysis':'source_changed',
 'Select a current enabled read-only Suma planning profile':'profile_invalid',
 'Planning context exceeds 32000 UTF-8 bytes; select a smaller task scope':'prompt_limit',
 'The planning analysis has changed; reload before deciding':'version_conflict',
 'Confirm that the selected task content will be sent to the configured model':'model_confirmation',
 'Confirm the proposed schedule changes before applying them':'apply_confirmation',
 'The planning execution did not complete successfully':'execution_failed',
 'The planning execution outcome is unknown; inspect the original run':'execution_unknown',
 'The planning creator no longer has current execution authority':'source_unauthorized',
 'Invalid planning alert decision or version':'alert_invalid'
};
const hostReason=(value:string)=>Object.hasOwn(reasonKeys,value)?text('reason.'+reasonKeys[value]):value;
const stringify=(value:unknown)=>value?JSON.stringify(value,null,2):'';
const plannerAvailable=(item:Planner)=>item.state==='enabled'&&item.available&&item.profile.adapter==='suma'&&item.profile.read_only&&item.profile.planning?.protocol==='suma-schedule-plan-v1'&&!!item.profile.planning.model&&!!item.profile.planning.endpoint_origin;

function proposalValue(raw:string,taskIDs?:Set<string>):PlanningOutput|undefined{
 try{
  const value:unknown=JSON.parse(raw);
  if(!object(value)||value.schema_version!==1||!boundedText(value.summary,2000)||!boundedText(value.rationale,8000)||!Array.isArray(value.actions)||value.actions.length<1||value.actions.length>20||!Array.isArray(value.risks)||value.risks.length>20||Object.keys(value).some(key=>!['schema_version','summary','rationale','actions','risks'].includes(key)))return;
  const selected=new Set<string>(),risks=new Set<string>();
  for(const action of value.actions){
   if(!object(action)||!id(action.task_id)||selected.has(action.task_id)||taskIDs&&!taskIDs.has(action.task_id)||Object.keys(action).some(key=>!['task_id','iteration_id','milestone_id','start_date','due_date','estimated_hours'].includes(key))||['iteration_id','milestone_id','start_date','due_date'].some(key=>typeof action[key]!=='string')||action.estimated_hours!==null&&(typeof action.estimated_hours!=='number'||!Number.isFinite(action.estimated_hours)||action.estimated_hours<0))return;
   selected.add(action.task_id);
  }
  for(const risk of value.risks){
   if(!object(risk)||!id(risk.key)||risks.has(risk.key)||!['info','warning','critical'].includes(String(risk.severity))||!boundedText(risk.title,200)||!boundedText(risk.detail)||!Array.isArray(risk.task_ids)||risk.task_ids.some(value=>!id(value)||taskIDs&&!taskIDs.has(value))||Object.keys(risk).some(key=>!['key','severity','title','detail','task_ids'].includes(key)))return;
   risks.add(risk.key);
  }
  return value as unknown as PlanningOutput;
 }catch{return;}
}

function recoveryValid(request:WriteRecoveryRequest,project:string){
 const body=request.body;if(request.method!=='POST'||body.project_id!==project)return false;
 if(request.path==='/planning/analyses')return Object.keys(body).every(key=>['project_id','goal','locale','task_ids','tool_profile_id','expected_tool_version','max_steps','max_output_tokens','acknowledge_model_use','idempotency_key'].includes(key))&&boundedText(body.goal)&&['en-US','zh-CN'].includes(String(body.locale))&&Array.isArray(body.task_ids)&&body.task_ids.length>0&&body.task_ids.length<=20&&body.task_ids.every(id)&&new Set(body.task_ids).size===body.task_ids.length&&id(body.tool_profile_id)&&positive(body.expected_tool_version)&&positive(body.max_steps)&&Number(body.max_steps)<=4&&positive(body.max_output_tokens)&&Number(body.max_output_tokens)<=8192&&body.acknowledge_model_use===true;
 const match=request.path.match(/^\/planning\/(analyses|alerts)\/([^/]+)\/commands$/);
 try{if(!match||!id(decodeURIComponent(match[2]))||!positive(body.expected_version)||!boundedText(body.reason))return false;}catch{return false;}
 if(match[1]==='alerts')return Object.keys(body).every(key=>['project_id','expected_version','action','reason','idempotency_key'].includes(key))&&['acknowledge','resolve'].includes(String(body.action));
 return Object.keys(body).every(key=>['project_id','expected_version','action','proposal','reason','acknowledge_schedule_changes','idempotency_key'].includes(key))&&['revise','apply','dismiss'].includes(String(body.action))&&typeof body.acknowledge_schedule_changes==='boolean'&&(body.action!=='apply'||body.acknowledge_schedule_changes===true)&&(body.action!=='revise'||!!proposalValue(JSON.stringify(body.proposal)));
}

function useCollection<T extends{id:string;project_id:string}>(api:ApiCaller,project:string,path:string,enabled:boolean,refresh:number,limit=20,cursorName='before'){
 const [state,setState]=useState<{path:string;page?:Page<T>;loading:boolean;error?:unknown}>({path,loading:false}),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),current=useRef<Page<T>|undefined>(undefined);
 const read=useCallback(async(more=false)=>{
  if(!enabled)return;
  const previous=current.current,cursor=more?previous?.next_cursor:'';if(more&&!cursor)return previous;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;if(!more)current.current=undefined;setState({path,page:more?previous:undefined,loading:true});
  try{
   const page=await api<Page<T>>(path+(cursor?'&'+cursorName+'='+encodeURIComponent(cursor):''),undefined,'GET',undefined,abort.signal);
   if(abort.signal.aborted||generation!==epoch.current)return;
   if(!Array.isArray(page.items)||page.items.length>limit||!Number.isSafeInteger(page.total)||page.total<page.items.length||page.items.some(item=>!id(item.id)||item.project_id!==project)||page.next_cursor!==undefined&&typeof page.next_cursor!=='string'||path.startsWith('/planning/alerts?')&&(!Number.isSafeInteger(page.open_count)||Number(page.open_count)<0||!Number.isSafeInteger(page.critical_count)||Number(page.critical_count)<0))throw new UiError('intelligent-planning.invalid_read');
   const value={...page,items:more?[...new Map([...(previous?.items||[]),...page.items].map(item=>[item.id,item])).values()]:page.items};
   if(value.items.length>value.total)throw new UiError('intelligent-planning.invalid_read');current.current=value;setState({path,page:value,loading:false});return value;
  }catch(error){if(abort.signal.aborted||generation!==epoch.current)return;current.current=undefined;setState({path,loading:false,error});}
 },[api,project,path,enabled,limit,cursorName]);
 useEffect(()=>{current.current=undefined;if(enabled)void read();return()=>{epoch.current++;controller.current?.abort();};},[read,enabled,refresh]);
 const clear=useCallback((error?:unknown)=>{epoch.current++;controller.current?.abort();current.current=undefined;setState({path,loading:false,error});},[path]);
 return {...(state.path===path?state:{path,loading:enabled}),read,clear};
}

export function PlanningIntelligence(props:Props){
 const scope=JSON.stringify([props.session.user.id,props.session.organization.id,props.project]);
 return <IntelligenceScope key={scope} {...props} scope={scope}/>;
}

function IntelligenceScope({session,project,tasks,refresh,onChanged,scope,onOpenRisk}:Props&{scope:string}){
 const locale=useLocale(),api=useApi(),bodyID=useId(),[open,setOpen]=useState(false),[alertState,setAlertState]=useState('open');
 const action=useWriteAction({scope:'intelligent-planning:'+scope,storage:toolSessionStorage(),validateRequest:request=>recoveryValid(request,project)}),pending=action.pending?.body;
 const [goal,setGoal]=useState(typeof pending?.goal==='string'?pending.goal:''),[taskIDs,setTaskIDs]=useState<string[]>(Array.isArray(pending?.task_ids)?pending.task_ids as string[]:[]),[profileID,setProfileID]=useState(typeof pending?.tool_profile_id==='string'?pending.tool_profile_id:'');
 const [modelAck,setModelAck]=useState(false),[steps,setSteps]=useState(3),[tokens,setTokens]=useState(4096),[selectedID,setSelectedID]=useState(''),[detail,setDetail]=useState<{data?:Analysis;loading:boolean;error?:unknown}>({loading:false}),[editor,setEditor]=useState<Editor>(),[editing,setEditing]=useState(false),[decisionReason,setDecisionReason]=useState(''),[applyAck,setApplyAck]=useState(false),[originalOpen,setOriginalOpen]=useState(false),[alertDecision,setAlertDecision]=useState<{item:PlanningAlert;action:'acknowledge'|'resolve';reason:string}>(),[connection,setConnection]=useState<RunConnectionState>(),[notice,setNotice]=useState(''),[refreshError,setRefreshError]=useState<unknown>();
 const alive=useRef(true),detailEpoch=useRef(0),detailController=useRef<AbortController|undefined>(undefined),confirmedSelection=useRef(''),onChangedRef=useRef(onChanged);onChangedRef.current=onChanged;
 const profiles=useCollection<Planner>(api,project,'/tool-profiles'+query(project)+'&limit=100',open,refresh,100,'cursor');
 const analyses=useCollection<Analysis>(api,project,'/planning/analyses'+query(project)+'&limit=20',open,refresh);
 const alerts=useCollection<PlanningAlert>(api,project,'/planning/alerts'+query(project)+(alertState?'&state='+alertState:'')+'&limit=20',open,refresh);
 const planners=(profiles.page?.items||[]).filter(plannerAvailable),planner=planners.find(value=>value.id===profileID),analysis=detail.data;
 useEffect(()=>{setModelAck(false);},[planner?.id,planner?.version,planner?.profile.planning?.model,planner?.profile.planning?.endpoint_origin]);
 const locked=action.busy||!!action.pending,canWrite=automationPermissions(session,project).write;
 const readDetail=useCallback(async(analysisID=selectedID)=>{
  if(!open||!analysisID)return;
  const generation=++detailEpoch.current;detailController.current?.abort();const abort=new AbortController();detailController.current=abort;setDetail(before=>({data:before.data?.id===analysisID?before.data:undefined,loading:true}));
  try{
   const value=await api<Analysis>('/planning/analyses/'+encodeURIComponent(analysisID)+query(project),undefined,'GET',undefined,abort.signal);
   if(abort.signal.aborted||generation!==detailEpoch.current)return;
   if(value.project_id!==project||value.id!==analysisID||!positive(value.version)||!analysisStates.has(value.state)||!Array.isArray(value.allowed_actions)||!value.source||!Array.isArray(value.source.tasks))throw new UiError('intelligent-planning.invalid_read');
   setDetail({data:value,loading:false});setEditor(before=>before?.id===value.id&&before.raw?before:{id:value.id,version:value.version,raw:stringify(value.proposal)});return value;
  }catch(error){if(abort.signal.aborted||generation!==detailEpoch.current)return;setDetail({loading:false,error});}
 },[api,project,open,selectedID]);
 useEffect(()=>{if(confirmedSelection.current===selectedID&&selectedID){confirmedSelection.current='';return()=>{detailEpoch.current++;detailController.current?.abort();};}setDetail({loading:false});setEditor(undefined);setEditing(false);setApplyAck(false);setDecisionReason('');setOriginalOpen(false);if(open&&selectedID)void readDetail();return()=>{detailEpoch.current++;detailController.current?.abort();};},[selectedID,open,readDetail]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 useEffect(()=>{if(open&&selectedID)void readDetail();},[refresh]);
 const run=analysis?.run_id||'',streaming=!!analysis&&['running','reconciling'].includes(analysis.state);
 useEffect(()=>{
  if(!open||!run||!streaming)return;const abort=new AbortController();const queue=new SnapshotRefreshQueue(async()=>{await Promise.allSettled([readDetail(),alerts.read()]);});
  void connectRunEvents({project,run,signal:abort.signal,onEvent:()=>queue.request(),onState:state=>{if(abort.signal.aborted)return;setConnection(state);if(state.state==='forbidden'){const error=new UiError('intelligent-planning.access_changed');profiles.clear(error);analyses.clear(error);alerts.clear(error);setDetail({loading:false,error});}},fetcher:(input,init)=>fetch(input,{...init,headers:{...Object.fromEntries(new Headers(init?.headers)),'X-Suma-Expected-User':session.user.id,'X-Suma-Expected-Organization':session.organization.id}})});
  return()=>{abort.abort();queue.close();setConnection(undefined);};
 },[open,run,streaming,project,session.user.id,session.organization.id,readDetail,alerts.read,alerts.clear,analyses.clear,profiles.clear]);
 const frozenIDs=useMemo(()=>new Set(analysis?.source.tasks.map(value=>value.task.id)||[]),[analysis?.source]);
 const parsedProposal=useMemo(()=>editor?proposalValue(editor.raw,frozenIDs):undefined,[editor,frozenIDs]);
 const dirty=!!analysis&&editor?.raw!==stringify(analysis.proposal),editorCurrent=!!analysis&&editor?.id===analysis.id&&editor.version===analysis.version;
 const selectedCurrent=taskIDs.length>0&&taskIDs.length<=20&&taskIDs.every(value=>tasks.some(task=>task.id===value));
 const canCreate=canWrite&&boundedText(goal)&&selectedCurrent&&!!planner&&modelAck&&positive(steps)&&steps<=4&&positive(tokens)&&tokens<=8192&&!profiles.loading;
 async function confirmed(value:Analysis|PlanningAlert|undefined,path:string){
  if(!value||!alive.current)return;if(value.project_id!==project||!id(value.id)){action.setError(new UiError('intelligent-planning.invalid_read'));return;}
  setRefreshError(undefined);
  if(path.includes('/alerts/')){setAlertDecision(undefined);setNotice('alert_saved');}
  else{const result=value as Analysis;if(result.id!==selectedID)confirmedSelection.current=result.id;setSelectedID(result.id);setDetail({data:result,loading:false});setEditor({id:result.id,version:result.version,raw:stringify(result.proposal)});setApplyAck(false);setDecisionReason('');setModelAck(false);setEditing(false);setOriginalOpen(false);setNotice('analysis_saved');}
  const results=await Promise.allSettled([analyses.read(),alerts.read(),Promise.resolve().then(()=>onChangedRef.current())]);if(alive.current&&results.some(value=>value.status==='rejected'))setRefreshError(new UiError('intelligent-planning.refresh_failed'));
 }
 async function perform(path:string,body:Record<string,unknown>){setNotice('');await confirmed(await action.run<Analysis|PlanningAlert>(path,body),path);}
 async function create(){if(!canCreate||!planner)return;await perform('/planning/analyses',{project_id:project,goal,locale,task_ids:taskIDs,tool_profile_id:planner.id,expected_tool_version:planner.version,max_steps:steps,max_output_tokens:tokens,acknowledge_model_use:true});}
 async function command(kind:'revise'|'apply'|'dismiss'){
  if(!analysis||!analysis.allowed_actions.includes(kind)||!editorCurrent||(kind==='revise'||kind==='apply')&&analysis.state!=='ready'||kind==='revise'&&(!parsedProposal||!boundedText(decisionReason))||kind==='apply'&&(!analysis.basis_current||dirty||!applyAck)||kind==='dismiss'&&!boundedText(decisionReason))return;
  await perform('/planning/analyses/'+encodeURIComponent(analysis.id)+'/commands',{project_id:project,expected_version:analysis.version,action:kind,...(kind==='revise'?{proposal:parsedProposal}:{}),reason:decisionReason||text('apply_default_reason'),acknowledge_schedule_changes:kind==='apply'&&applyAck});
 }
 async function rebase(){const value=await readDetail();const results=await Promise.allSettled([profiles.read(),analyses.read(),alerts.read()]);if(selectedID&&!value)throw new UiError('intelligent-planning.refresh_failed');if(value){setEditor(before=>before?{...before,version:value.version}:before);setApplyAck(false);}const alertsResult=results[2];if(alertDecision&&alertsResult.status==='fulfilled'){const latest=alertsResult.value?.items.find(item=>item.id===alertDecision.item.id);if(latest)setAlertDecision({...alertDecision,item:latest});}}
 const runHref=run?'?'+new URLSearchParams({project,view:'development',sub:'runs',run}):'';
 return <section className="panel planning-intelligence" aria-label={text('title')}>
  <h2><button className="pi-toggle" type="button" data-planning-toggle aria-expanded={open} aria-controls={bodyID} disabled={action.busy||open&&!!action.pending} onClick={()=>setOpen(value=>!value)}><Sparkles size={18} aria-hidden="true"/><span>{text('title')}</span><ChevronDown size={17} className={open?'is-open':''} aria-hidden="true"/></button></h2>
  {open&&<div id={bodyID} className="pi-body">
   <p className="pi-muted">{text('intro')}</p><WriteNotice action={action} onRebase={rebase}/>
   {action.pending&&<div className="pi-recovery" role="status"><strong>{text('original_operation')}</strong><p className="pi-raw">{String(action.pending.body.goal||action.pending.body.reason||'')}</p><button type="button" data-planning-restore disabled={action.busy} onClick={()=>{const pending=action.pending;if(pending)void action.run<Analysis|PlanningAlert>(pending.path,pending.body,pending.method).then(value=>confirmed(value,pending.path));}}>{text('restore')}</button></div>}
   {notice&&<p className="pi-success" role="status">{text(notice)}</p>}{Boolean(refreshError)&&<ErrorNotice error={refreshError}/>}
   <div className="pi-columns"><div className="pi-section">
    <h3>{text('create_title')}</h3>{profiles.loading&&<Loading/>}{Boolean(profiles.error)&&<ErrorNotice error={toolError(profiles.error)} onRetry={()=>void profiles.read()}/>}
    {!profiles.loading&&profiles.page&&!planners.length&&<div className="pi-empty"><p>{text('no_planner')}</p><a href={'?'+new URLSearchParams({project,view:'settings',sub:'tools'})}>{text('open_tools')}</a></div>}
    {profiles.page?.next_cursor&&<button type="button" disabled={profiles.loading||locked} onClick={()=>void profiles.read(true)}>{text('more_profiles')}</button>}
    <form data-planning-create onSubmit={event=>{event.preventDefault();void create();}}><WriteFields action={action}>
     <Field label={text('goal')}><textarea name="goal" rows={3} value={goal} maxLength={4000} onChange={event=>setGoal(event.target.value)} required/></Field>{goal&&!boundedText(goal)&&<p className="pi-notice">{text('goal_limit')}</p>}
     <Field label={text('planner')}><select name="planner" value={profileID} onChange={event=>{setProfileID(event.target.value);setModelAck(false);}} required><option value="">{text('choose_planner')}</option>{planners.map(value=><option key={value.id} value={value.id}>{value.name}</option>)}</select></Field>
     {planner&&<div className="pi-planner"><strong>{planner.profile.planning?.model}</strong><code>{planner.profile.planning?.endpoint_origin}</code><p>{text('not_health')}</p></div>}
     <div className="pi-task-heading"><strong>{text('tasks',{count:formatNumber(taskIDs.length)})}</strong><button type="button" disabled={!taskIDs.length} onClick={()=>setTaskIDs([])}>{text('clear_selection')}</button></div>
     <p className="pi-muted">{text('task_scope')}</p><div className="pi-task-options">{tasks.map(task=><label key={task.id}><input type="checkbox" name="planning-task" value={task.id} checked={taskIDs.includes(task.id)} disabled={taskIDs.length>=20&&!taskIDs.includes(task.id)} onChange={event=>{setTaskIDs(values=>event.target.checked?[...values,task.id]:values.filter(value=>value!==task.id));setModelAck(false);}}/><span>{task.title||task.id}<small>v{formatNumber(task.version)}</small></span></label>)}</div>
     {!tasks.length&&<p className="pi-muted">{text('no_tasks')}</p>}{taskIDs.length>0&&!selectedCurrent&&<p className="pi-muted">{text('selection_changed')}</p>}
     <details className="pi-disclosure"><summary>{text('limits')}</summary><div className="pi-limits"><Field label={text('steps')}><input type="number" min={1} max={4} value={steps} onChange={event=>setSteps(Number(event.target.value))}/></Field><Field label={text('tokens')}><input type="number" min={1} max={8192} value={tokens} onChange={event=>setTokens(Number(event.target.value))}/></Field></div></details>
     <label className="pi-check"><input type="checkbox" name="acknowledge_model_use" checked={modelAck} onChange={event=>setModelAck(event.target.checked)}/><span>{text('model_ack')}</span></label>
    </WriteFields><WriteButton action={action} disabled={!canCreate||action.persistenceBlocked}>{text('start')}</WriteButton></form>
   </div><div className="pi-section">
    <div className="pi-heading"><h3>{text('analyses')}</h3><button type="button" disabled={analyses.loading||locked} onClick={()=>void Promise.allSettled([analyses.read(),readDetail()])}>{text('refresh')}</button></div>
    {analyses.loading&&<Loading/>}{Boolean(analyses.error)&&<ErrorNotice error={toolError(analyses.error)} onRetry={()=>void analyses.read()}/>}
    {analyses.page&&!analyses.page.items.length&&<p className="pi-muted">{text('no_analyses')}</p>}
    <div className="pi-analysis-list">{analyses.page?.items.map(value=><button type="button" key={value.id} data-analysis-select aria-pressed={selectedID===value.id} disabled={locked} onClick={()=>{if(selectedID===value.id)void readDetail(value.id);else setSelectedID(value.id);}}><span>{value.goal}</span><small>{text('state.'+value.state)} · {formatDateTime(value.created_at)}</small></button>)}</div>
    {analyses.page?.next_cursor&&<button type="button" disabled={analyses.loading||locked} onClick={()=>void analyses.read(true)}>{text('more_analyses')}</button>}
    {detail.loading&&<Loading/>}{Boolean(detail.error)&&<ErrorNotice error={toolError(detail.error)} onRetry={()=>void readDetail()}/>}
   </div></div>
   {analysis&&<section className="pi-result" aria-label={text('analysis_detail')}>
    <div className="pi-heading"><h3>{text('state.'+analysis.state)}</h3>{runHref&&<a data-planning-run href={runHref}>{text('open_run')}</a>}</div>
    {connection&&<p className="pi-muted" role="status">{text('events.'+connection.state)}{runConnectionMessage(connection)&&<> · {runConnectionMessage(connection)}</>}</p>}
    <p className="pi-raw">{analysis.goal}</p>{analysis.reason&&<p className="pi-notice">{hostReason(analysis.reason)}</p>}{!analysis.basis_current&&['ready','running','reconciling'].includes(analysis.state)&&<p className="pi-notice">{text('basis_changed')}</p>}
    <div className="pi-result-meta"><span>v{formatNumber(analysis.version)}</span><code>{analysis.tool_profile_id} · v{formatNumber(analysis.tool_profile_version)}</code></div>
    {analysis.proposal&&<><h4>{text('proposal')}</h4><p className="pi-raw">{analysis.proposal.summary}</p><p className="pi-raw pi-muted">{analysis.proposal.rationale}</p><div className="pi-schedule">{analysis.proposal.actions.map(value=><div key={value.task_id}><strong>{analysis.source.tasks.find(task=>task.task.id===value.task_id)?.task.title||value.task_id}</strong><span>{value.start_date||'—'} → {value.due_date||'—'}</span><span>{value.estimated_hours===null?text('unknown_estimate'):text('hours',{hours:formatNumber(value.estimated_hours)})}</span></div>)}</div>
     {!!analysis.proposal.risks.length&&<div className="pi-risks"><h4>{text('risks')}</h4>{analysis.proposal.risks.map(risk=><div key={risk.key}><strong>{text('severity.'+risk.severity)} · {risk.title}</strong><p className="pi-raw">{risk.detail}</p></div>)}</div>}
    </>}
    <details className="pi-disclosure"><summary>{text('source')}</summary><pre>{JSON.stringify(analysis.source,null,2)}</pre><code>{analysis.tool_profile_sha256}</code></details>
    <div className="pi-disclosure"><button type="button" className="pi-disclosure-button" data-original-toggle aria-expanded={originalOpen} onClick={()=>setOriginalOpen(value=>!value)}>{text('original')}</button>{originalOpen&&<>{analysis.original_output?<pre data-original-output>{analysis.original_output}</pre>:<p className="pi-muted">{text('original_unavailable')}</p>}<code>{analysis.original_output_sha256}</code></>}</div>
    {!!analysis.allowed_actions.length&&<div className="pi-decisions"><WriteFields action={action}>
     {analysis.allowed_actions.includes('revise')&&<button type="button" data-planning-edit aria-expanded={editing} onClick={()=>setEditing(value=>!value)}>{text('edit')}</button>}
     {editing&&<Field label={text('json_label')}><textarea className="pi-json" name="proposal" rows={12} maxLength={200000} value={editor?.raw||''} onChange={event=>setEditor({id:analysis.id,version:editor?.version||analysis.version,raw:event.target.value})}/></Field>}
     {editing&&!parsedProposal&&<p className="pi-notice">{text('invalid_json')}</p>}{dirty&&<p className="pi-muted">{text('save_first')}</p>}{!editorCurrent&&<p className="pi-notice">{text('version_changed')}</p>}
     <Field label={text('decision_reason')}><textarea name="analysis_reason" rows={2} maxLength={4000} value={decisionReason} onChange={event=>setDecisionReason(event.target.value)}/></Field>
     {analysis.allowed_actions.includes('apply')&&<label className="pi-check"><input type="checkbox" name="acknowledge_schedule_changes" checked={applyAck} onChange={event=>setApplyAck(event.target.checked)}/><span>{text('apply_ack')}</span></label>}
    </WriteFields><div className="pi-actions">
     {analysis.allowed_actions.includes('revise')&&<button type="button" data-analysis-revise disabled={locked||!editorCurrent||!parsedProposal||!boundedText(decisionReason)||action.conflict} onClick={()=>void command('revise')}>{text('save_revision')}</button>}
     {analysis.allowed_actions.includes('apply')&&<button type="button" className="primary" data-analysis-apply disabled={locked||!editorCurrent||dirty||!analysis.basis_current||!applyAck||action.conflict} onClick={()=>void command('apply')}>{text('apply')}</button>}
     {analysis.allowed_actions.includes('dismiss')&&<button type="button" disabled={locked||!editorCurrent||!boundedText(decisionReason)||action.conflict} onClick={()=>void command('dismiss')}>{text('dismiss')}</button>}
    </div></div>}
   </section>}
   <section className="pi-alerts" aria-label={text('alerts')}>
    <div className="pi-heading"><h3>{text('alerts')}</h3><label><span className="sr-only">{text('alert_filter')}</span><select aria-label={text('alert_filter')} value={alertState} disabled={locked} onChange={event=>setAlertState(event.target.value)}><option value="open">{text('alert_state.open')}</option><option value="acknowledged">{text('alert_state.acknowledged')}</option><option value="resolved">{text('alert_state.resolved')}</option><option value="">{text('all')}</option></select></label></div>
    <p className="pi-muted">{text('risk_boundary')}</p>{alerts.loading&&<Loading/>}{Boolean(alerts.error)&&<ErrorNotice error={toolError(alerts.error)} onRetry={()=>void alerts.read()}/>}
    {alerts.page&&<p className="pi-muted">{text('alert_counts',{open:formatNumber(alerts.page.open_count as number),critical:formatNumber(alerts.page.critical_count as number)})}</p>}
    {alerts.page&&!alerts.page.items.length&&<p className="pi-muted">{text('no_alerts')}</p>}
    {alerts.page?.items.map(item=><article key={item.id} className="pi-alert"><div className="pi-heading"><strong>{item.title}</strong><span>{text('severity.'+item.severity)} · {text('alert_state.'+item.state)}</span></div><p className="pi-raw">{item.detail}</p><small>{formatDateTime(item.created_at)}</small><div className="pi-actions">{canWrite&&<ProjectRiskRegistration session={session} project={project} alert={item} onOpenRisk={onOpenRisk} onChanged={onChanged}/>}<button type="button" disabled={locked} onClick={()=>setSelectedID(item.analysis_id)}>{text('open_analysis')}</button>{canWrite&&item.state==='open'&&<button type="button" data-alert-action="acknowledge" disabled={locked} onClick={()=>setAlertDecision({item,action:'acknowledge',reason:''})}>{text('acknowledge')}</button>}{canWrite&&item.state!=='resolved'&&<button type="button" data-alert-action="resolve" disabled={locked} onClick={()=>setAlertDecision({item,action:'resolve',reason:''})}>{text('resolve')}</button>}</div></article>)}
    {alerts.page?.next_cursor&&<button type="button" disabled={locked||alerts.loading} onClick={()=>void alerts.read(true)}>{text('more_alerts')}</button>}
    {alertDecision&&<form className="pi-alert-decision" data-alert-decision onSubmit={event=>{event.preventDefault();if(!boundedText(alertDecision.reason))return;void perform('/planning/alerts/'+encodeURIComponent(alertDecision.item.id)+'/commands',{project_id:project,expected_version:alertDecision.item.version,action:alertDecision.action,reason:alertDecision.reason});}}><h4>{text(alertDecision.action)}</h4><WriteFields action={action}><Field label={text('alert_reason')}><textarea name="alert_reason" value={alertDecision.reason} rows={2} maxLength={4000} required onChange={event=>setAlertDecision({...alertDecision,reason:event.target.value})}/></Field></WriteFields><div className="pi-actions"><WriteButton action={action} disabled={!boundedText(alertDecision.reason)||action.persistenceBlocked}>{text('confirm')}</WriteButton><button type="button" disabled={locked} onClick={()=>setAlertDecision(undefined)}>{text('cancel')}</button></div></form>}
   </section>
  </div>}
 </section>;
}
