import {useCallback,useEffect,useRef,useState} from 'react';
import type {ExpertTeamConfiguration,ExpertTeamPlan} from '../../../packages/contracts/src';
import {ApiError,query,stateLabel,type RecordItem,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {KnowledgeContextFacts} from './KnowledgeFacts';
import {DispatchRouteFacts} from './execution-pools/DispatchRouteFacts';
import {toolError,toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading,Modal} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {expertBlocker,expertID,expertInteger,expertRecovery,validExpertConfiguration,validExpertPlan,validExpertTasks} from './expert-team-model';
import {useExpertPage,useExpertRead,type ExpertPage} from './expert-team-hooks';
import './TeamTaskDispatchDialog.css';
type Props={project:string;session:Session;tasks:RecordItem[];open:boolean;onClose:()=>void;onStarted:(plan:ExpertTeamPlan)=>void;planID?:string;onPlan:(id:string)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('expert-team.'+key,values);
const teamKey=(value:ExpertTeamConfiguration)=>value.team_id;
export function TeamTaskDispatchDialog(props:Props){return props.open?<DispatchScope key={JSON.stringify([props.session.user.id,props.session.organization.id,props.project])} {...props}/>:null;}
function DispatchScope({project,session,tasks,onClose,onStarted,planID,onPlan}:Props){
 useLocale();const api=useApi(),[located,setLocated]=useState(expertID(planID)?planID:''),[team,setTeam]=useState(''),[roles,setRoles]=useState<Record<string,string>>({}),[fixedTasks,setFixedTasks]=useState(()=>tasks.map(value=>({...value}))),[ack,setAck]=useState(false),[forbidden,setForbidden]=useState<unknown>(),alive=useRef(true),taskRead=useRef<AbortController|undefined>(undefined),callbacks=useRef({onStarted,onPlan});callbacks.current={onStarted,onPlan};
 const action=useWriteAction({scope:JSON.stringify(['expert-team-dispatch',session.user.id,session.organization.id,project]),storage:toolSessionStorage(),validateRequest:request=>expertRecovery(request,project)}),locked=action.busy||!!action.pending||action.persistenceBlocked;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;taskRead.current?.abort();};},[]);
 useEffect(()=>{if(expertID(planID))setLocated(planID);},[planID]);
 const failed=useCallback((error:unknown)=>{if(error instanceof ApiError&&[401,403].includes(error.status))setForbidden(error);},[]);
 const configurationsValid=useCallback((value:ExpertPage<ExpertTeamConfiguration>)=>Array.isArray(value.items)&&value.items.length<=20&&expertInteger(value.total)&&value.total>=value.items.length&&value.items.every(item=>validExpertConfiguration(item,project))&&(value.next_cursor===undefined||typeof value.next_cursor==='string'),[project]);
 const configurations=useExpertPage<ExpertTeamConfiguration>(api,'/expert-teams'+query(project)+'&limit=20',!located&&!forbidden,0,configurationsValid,failed,teamKey,'before');
 const planValid=useCallback((value:ExpertTeamPlan)=>validExpertPlan(value,project,located),[project,located]);
 const detail=useExpertRead<ExpertTeamPlan>(api,'/expert-team-plans/'+encodeURIComponent(located)+query(project),!!located&&!forbidden,0,planValid,failed),plan=detail.value;
 useEffect(()=>setAck(false),[plan?.id,plan?.version,plan?.state,plan?.valid,plan?.can_start]);
 const configuration=configurations.value?.items.find(item=>item.team_id===team),selectionValid=validExpertTasks(fixedTasks,project);
 const canPreview=selectionValid&&configuration?.state==='published'&&configuration.valid&&fixedTasks.every(task=>configuration.roles.some(role=>role.key===roles[task.id]))&&!locked&&!action.conflict;
 const canStart=!!plan&&plan.state==='ready'&&plan.valid&&plan.can_start&&ack&&!detail.loading&&!detail.error&&!locked&&!action.conflict;
 async function confirmed(value:ExpertTeamPlan|undefined){
  if(!value||!alive.current)return;if(!validExpertPlan(value,project,value.id)){action.setError(new UiError('expert-team.invalid_read'));return;}
  setAck(false);setLocated(value.id);callbacks.current.onPlan(value.id);
  if(value.id===located)await detail.read();
  if(value.state==='started')callbacks.current.onStarted(value);
 }
 async function preview(){if(!canPreview||!configuration)return;await confirmed(await action.run<ExpertTeamPlan>('/expert-team-plans',{project_id:project,team_id:configuration.team_id,expected_configuration_version:configuration.version,items:fixedTasks.map(task=>({task_id:task.id,expected_task_version:task.version,role_key:roles[task.id]}))}));}
 async function start(){if(!canStart||!plan)return;await confirmed(await action.run<ExpertTeamPlan>('/expert-team-plans/'+encodeURIComponent(plan.id)+'/start',{project_id:project,expected_version:plan.version,acknowledge_dispatch:true}));}
 async function rebase(){
  setAck(false);if(located){if(!await detail.read())throw new UiError('expert-team.refresh_failed');return;}
  if(!await configurations.read())throw new UiError('expert-team.refresh_failed');
  taskRead.current?.abort();const abort=new AbortController();taskRead.current=abort;
  const current=await Promise.all(fixedTasks.map(task=>api<RecordItem>('/tasks/'+encodeURIComponent(task.id)+query(project),undefined,'GET',undefined,abort.signal)));
  if(abort.signal.aborted||!alive.current)return;if(!validExpertTasks(current,project)||current.some((task,index)=>task.id!==fixedTasks[index].id))throw new UiError('expert-team.selection_invalid');setFixedTasks(current);
 }
 return <Modal open onClose={()=>{if(!locked)onClose();}} title={t('dispatch_title')} description={t('dispatch_intro')}><div className="team-task-dispatch">
  {forbidden?<ErrorNotice error={toolError(forbidden)}/>:<>
   <WriteNotice action={action} onRebase={rebase}/>{action.pending&&<div className="team-dispatch-callout" role="status"><strong>{t('original_operation')}</strong><p>{t('batch_recovery_hint')}</p><details><summary>{t('original_request')}</summary><pre className="team-dispatch-raw">{JSON.stringify(action.pending.body,null,2)}</pre></details><button data-team-restore type="button" disabled={action.busy} onClick={()=>{const request=action.pending;if(request)void action.run<ExpertTeamPlan>(request.path,request.body,request.method).then(confirmed);}}>{t('restore')}</button></div>}
   {!located?<><p className="team-dispatch-muted">{t('selected_tasks',{count:formatNumber(fixedTasks.length)})}</p>{!selectionValid&&<p className="warning" role="alert">{t('selection_invalid')}</p>}{configurations.loading&&<Loading/>}{Boolean(configurations.error)&&<ErrorNotice error={toolError(configurations.error)} onRetry={()=>void configurations.read()}/>}
    <Field label={t('published_team')}><select name="dispatch_team" value={team} disabled={locked} onChange={event=>{const selected=configurations.value?.items.find(item=>item.team_id===event.target.value);setTeam(event.target.value);setRoles(Object.fromEntries(fixedTasks.map(task=>[task.id,selected?.roles.length===1?selected.roles[0].key:''])));}}><option value="">{t('choose_team')}</option>{configurations.value?.items.map(item=><option value={item.team_id} key={item.team_id} disabled={item.state!=='published'||!item.valid}>{item.team_name} · v{formatNumber(item.version)}{item.state!=='published'||!item.valid?' · '+t('unavailable'):''}</option>)}</select></Field>
    {configurations.value&&!configurations.value.items.some(value=>value.state==='published'&&value.valid)&&<div className="team-dispatch-callout"><p>{t('no_configuration')}</p><a href={'?'+new URLSearchParams({project,view:'settings',sub:'teams'})}>{t('open_teams')}</a></div>}{configurations.value?.next_cursor&&<button type="button" disabled={locked||configurations.loading} onClick={()=>void configurations.read(true)}>{t('more_configurations')}</button>}
    {configuration&&<p className="team-dispatch-muted">{t('configuration_version',{value:formatNumber(configuration.version)})}</p>}
    {selectionValid&&<div className="team-dispatch-items">{fixedTasks.map(task=><section className="team-dispatch-task" key={task.id}><strong>{task.title||task.id}</strong><small>v{formatNumber(task.version)}</small><Field label={t('task_role')}><select name={'dispatch_role_'+task.id} value={roles[task.id]||''} disabled={locked||!configuration} onChange={event=>setRoles({...roles,[task.id]:event.target.value})}><option value="">{t('choose_role')}</option>{configuration?.roles.map(role=><option key={role.key} value={role.key}>{role.title} · {role.tool_name}</option>)}</select></Field></section>)}</div>}
    <p className="team-dispatch-muted">{t('preview_notice')}</p><button data-team-preview className="primary" type="button" disabled={!canPreview} onClick={()=>void preview()}>{t('prepare_preview')}</button>
   </>:<>
    {detail.loading&&<Loading/>}{Boolean(detail.error)&&<ErrorNotice error={toolError(detail.error)} onRetry={()=>void detail.read()}/>}
    {plan&&<><div className="team-dispatch-heading"><div><h3>{plan.team_name}</h3><p className="team-dispatch-muted">{t('plan.'+plan.state)} · {t('configuration_version',{value:formatNumber(plan.configuration_version)})}</p></div><button type="button" disabled={locked||detail.loading} onClick={()=>void detail.read()}>{t('refresh')}</button></div>
     {!!plan.blockers.length&&<div className="team-dispatch-callout"><strong>{t('blocked')}</strong><ul>{plan.blockers.map((value,index)=><li key={index}>{expertBlocker(value)}</li>)}</ul></div>}
     <div className="team-dispatch-items">{plan.items.map(item=><article className="team-dispatch-task" key={item.plan.task_id}><div className="team-dispatch-heading"><strong>{item.plan.context.task.title||item.plan.task_id}</strong>{plan.state==='started'&&item.plan.run_id&&<a data-team-run href={'?'+new URLSearchParams({project,view:'development',sub:'runs',run:item.plan.run_id})}>{t('open_run')}</a>}</div><p>{item.role_title} · {item.resource_name}</p><p className="team-dispatch-muted">{plan.state==='started'?item.run_state?stateLabel(item.run_state):t('run_unknown'):t('not_started')}</p><details><summary>{t('fixed_basis')}</summary><p>{t('task_version',{value:formatNumber(item.plan.task_version)})}</p><p>{item.plan.tool_profile_id} · v{formatNumber(item.plan.tool_profile_version)}</p><code>{item.plan.tool_profile_sha256}</code><DispatchRouteFacts route={item.plan.execution_route}/><KnowledgeContextFacts project={project} items={item.plan.context.knowledge||[]}/><pre className="team-dispatch-raw">{JSON.stringify({task:item.plan.context.task,workflow:item.plan.workflow},null,2)}</pre></details></article>)}</div>
     <p className="team-dispatch-muted">{t('independent_notice')}</p>
     {plan.state==='ready'&&<><p className="team-dispatch-muted">{t('expires',{value:formatDateTime(plan.expires_at,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})})}</p><label className="team-dispatch-check"><input name="acknowledge_team_dispatch" type="checkbox" checked={ack} disabled={locked||!plan.can_start||!plan.valid} onChange={event=>setAck(event.target.checked)}/>{t('dispatch_ack')}</label><button data-team-start className="primary" type="button" disabled={!canStart} onClick={()=>void start()}>{t('start_all')}</button>{!plan.valid&&<a aria-disabled={locked} href={'?'+new URLSearchParams({project,view:'development',sub:'tasks'})} onClick={event=>{if(locked)event.preventDefault();}}>{t('reselect_tasks')}</a>}</>}
     {plan.state==='started'&&<p className="team-dispatch-success" role="status">{t('started_notice')}</p>}
    </>}
   </>}
  </>}
 </div></Modal>;
}
