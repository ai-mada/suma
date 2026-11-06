import {useCallback,useEffect,useId,useRef,useState,type Ref} from 'react';
import {ChevronDown,MessagesSquare,RefreshCw,Send,ShieldCheck,Square} from 'lucide-react';
import type {CloudCollaborationSource,CloudTurn,CloudTurnCommand,CloudTurnPage,CloudTurnPermission,CloudTurnPermissionOption,CloudTurnView} from '../../../packages/contracts/src';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Loading} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {cloudTurnControlPath,cloudTurnPagePath,cloudTurnPermissionPath,cloudTurnPromptPath,cloudTurnViewPath,cloudTurnsIdentity,readCloudTurnPage,readCloudTurnView,validCloudTurnReceipt,validCloudTurnRecovery,validCloudTurnsScope,type CloudTurnsScope} from './workbuddy-cloud-turns-model';
import {wellFormed} from './i18n/catalog';
import './WorkBuddyCloudTurns.css';

export type WorkBuddyCloudTurnsProps={source:CloudCollaborationSource;revision?:number;disabled?:boolean;runVersion?:number;disclosureRef?:Ref<HTMLButtonElement>;onChanged?:()=>void|Promise<void>};
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-cloud-turns.'+key,values);
const encoder=new TextEncoder(),maximum=2147483647;
const active=(turn?:CloudTurn)=>!!turn&&['prepared','running','waiting_permission'].includes(turn.state);
const refusal=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const time=(value:string)=>formatDateTime(value,{dateStyle:'short',timeStyle:'short'});

/** Observation versions refresh reads. Only the stable private source, actor
 * or caller retires this controller and its original command ownership. */
export function WorkBuddyCloudTurns(props:WorkBuddyCloudTurnsProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope={source:props.source,userID:view?.userID||'',organizationID:view?.organizationID||''};
 const allowed=validCloudTurnsScope(scope),identity=allowed?cloudTurnsIdentity(scope):JSON.stringify(scope);
 return <TurnsController {...props} key={identity+':'+owner.current.epoch} scope={scope} identity={identity} allowed={allowed}/>;
}
type Snapshot={view?:CloudTurnView;page?:CloudTurnPage;loading:boolean;error?:unknown};
function TurnsController({source,revision,disabled=false,runVersion,disclosureRef,onChanged,scope,identity,allowed}:WorkBuddyCloudTurnsProps&{scope:CloudTurnsScope;identity:string;allowed:boolean}){
 useLocale();const api=useApi(),panelID=useId(),inputID=useId(),hintID=useId();
 const [open,setOpen]=useState(false),[prompt,setPrompt]=useState(''),[snapshot,setSnapshot]=useState<Snapshot>({loading:false}),[denial,setDenial]=useState<unknown>(),[confirmed,setConfirmed]=useState<'prompt'|'command'>();
 const alive=useRef(true),epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),current=useRef(snapshot),pinned=useRef<string|undefined>(undefined),commandBusy=useRef(false),readBusy=useRef(false),notified=useRef(false),deniedNow=useRef(false),changed=useRef(onChanged),scopeNow=useRef(scope);
 changed.current=onChanged;current.current=snapshot;
 if(source.submission_version>scopeNow.current.source.submission_version)scopeNow.current={...scope,source};
 const action=useWriteAction({scope:'workbuddy-cloud-turns:'+identity,storage:toolSessionStorage(),validateRequest:request=>validCloudTurnRecovery(request,scopeNow.current),validateResponse:(value,request)=>validCloudTurnReceipt(value,request,scopeNow.current)});
 const pending=action.pending,originalTurn=pending?.path.match(/^\/workbuddy-cloud-turns\/([^/]+)\//)?.[1],pendingPrompt=!!pending&&pending.path===cloudTurnPromptPath(source.submission_id);
 const originalTurnRef=useRef(originalTurn);originalTurnRef.current=originalTurn;
 const notifyChanged=()=>{try{void Promise.resolve(changed.current?.()).catch(()=>{});}catch{/* A parent refresh cannot undo an acknowledged operation. */}};
 const clearPrivate=useCallback((error:unknown)=>{
  if(!alive.current)return;++epoch.current;abort.current?.abort();deniedNow.current=true;current.current={loading:false};setSnapshot(current.current);setPrompt('');setConfirmed(undefined);pinned.current=undefined;setDenial(error);
  // This is private collaboration authority, not proof that the public node
  // was revoked. Ask its existing reader to evaluate its own authority.
  if(!notified.current){notified.current=true;try{void Promise.resolve(changed.current?.()).catch(()=>{});}catch{}}
 },[]);
 const load=useCallback(async(more=false,target?:string,background=false):Promise<boolean>=>{
  if(!allowed||background&&readBusy.current)return false;
  const before=current.current;if(more&&(!before.page?.has_more||before.loading))return false;
  const generation=++epoch.current;abort.current?.abort();const controller=new AbortController();abort.current=controller;readBusy.current=true;
  // Background reads retain the current rows, controls and focus. Each group
  // has one in-flight GET at a time and never changes the original writer.
  current.current={...before,loading:!background,error:undefined};setSnapshot(current.current);
  try{
   const requestScope=scopeNow.current;
   const headers={'X-Suma-Expected-User':requestScope.userID,'X-Suma-Expected-Organization':requestScope.organizationID};
   let view=before.view;
   if(!more){
    const raw=await api<unknown>(cloudTurnViewPath(requestScope),undefined,'GET',headers,controller.signal);
    if(!alive.current||controller.signal.aborted||generation!==epoch.current)return false;
    view=readCloudTurnView(raw,requestScope);if(!view)throw new UiError('workbuddy-cloud-turns.invalid_read');
    scopeNow.current={...requestScope,source:view.source.collaboration};
   }
   if(!view)throw new UiError('workbuddy-cloud-turns.invalid_read');
   const turnID=originalTurnRef.current||target||pinned.current||view.latest_turn?.id;
   let page:CloudTurnPage|undefined;
   if(turnID){
    const previous=(more||background)&&before.page?.turn.id===turnID?before.page:undefined,after=previous?.next_after||0;
    const raw=await api<unknown>(cloudTurnPagePath(source.project_id,turnID,after),undefined,'GET',headers,controller.signal);
    if(!alive.current||controller.signal.aborted||generation!==epoch.current)return false;
    const value=readCloudTurnPage(raw,scopeNow.current,turnID,after);if(!value)throw new UiError('workbuddy-cloud-turns.invalid_read');
    const items=previous?[...previous.items,...value.items]:value.items;
    if(items.length>1000||items.reduce((sum,item)=>sum+encoder.encode(item.text).length,0)>1048576||new Set(items.map(item=>item.sequence)).size!==items.length)throw new UiError('workbuddy-cloud-turns.limit');
    page={...value,items};pinned.current=turnID;
   }
   deniedNow.current=false;notified.current=false;setDenial(undefined);current.current={view,page,loading:false};setSnapshot(current.current);return true;
  }catch(error){
   if(!alive.current||controller.signal.aborted||generation!==epoch.current)return false;
   if(refusal(error)){clearPrivate(error);return false;}
   current.current={...before,loading:false,error};setSnapshot(current.current);return false;
  }finally{if(generation===epoch.current)readBusy.current=false;}
 },[api,allowed,source.project_id,clearPrivate]);
 const latestLoad=useRef(load);latestLoad.current=load;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;abort.current?.abort();};},[]);
 useEffect(()=>{if(open)void load();return()=>{++epoch.current;abort.current?.abort();};},[open,load,revision,source.submission_version]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useEffect(()=>{if(writeDenied)clearPrivate(new ApiError(action.errorStatus!,'workbuddy_cloud_turn_access_changed',t('access_changed')));},[writeDenied,action.errorStatus,clearPrivate]);
 const denied=!allowed||!!denial||writeDenied,view=!denied?snapshot.view:undefined,page=!denied?snapshot.page:undefined,turn=page?.turn||view?.latest_turn;
 useEffect(()=>{
  if(!open||disabled||denied||snapshot.error||!active(turn)||turn?.overdue)return;
  let disposed=false,timer:ReturnType<typeof setTimeout>|undefined;
  const schedule=()=>{
   if(disposed||document.visibilityState==='hidden')return;
   timer=setTimeout(async()=>{
    if(disposed||document.visibilityState==='hidden')return;
    try{if(!readBusy.current)await latestLoad.current(false,undefined,true);}finally{schedule();}
   },1000);
  };
  const visibility=()=>{if(timer!==undefined)clearTimeout(timer);if(document.visibilityState!=='hidden')schedule();};
  document.addEventListener('visibilitychange',visibility);schedule();
  return()=>{disposed=true;if(timer!==undefined)clearTimeout(timer);document.removeEventListener('visibilitychange',visibility);};
 },[open,disabled,denied,!!snapshot.error,turn?.id,turn?.state,turn?.overdue]);
 const locked=disabled||denied||action.busy||!!pending||snapshot.loading||!!snapshot.error||action.conflict;
 const promptBytes=encoder.encode(prompt).length,validPrompt=!!prompt.trim()&&wellFormed(prompt)&&promptBytes<=32000;
 const canSend=!locked&&!!view?.allowed_actions.includes('prompt')&&!active(view.latest_turn)&&view.latest_turn?.state!=='unknown'&&Number.isSafeInteger(runVersion)&&Number(runVersion)>0&&Number(runVersion)<maximum&&view.version<maximum&&validPrompt;
 const currentTurn=!!turn&&view?.latest_turn?.id===turn.id;
 const permissionLocked=(permission:CloudTurnPermission)=>locked||!active(turn)||!currentTurn||permission.state!=='pending'||permission.version>=maximum||!!page?.commands.some(command=>command.kind==='permission'&&command.permission_id===permission.id);
 const hasCancel=!!page?.commands.some(command=>command.kind==='cancel');
 async function refresh(){if(action.busy||!allowed)return false;action.setError('');if(!await latestLoad.current())return false;action.clearConflict();return true;}
 async function execute(path:string,body:Record<string,unknown>,isPrompt=false,method?:string){
  if(commandBusy.current||action.busy||denied)return;commandBusy.current=true;setConfirmed(undefined);
  const originalText=typeof body.prompt==='string'?body.prompt:undefined;
  try{
   const value=await action.run<CloudTurn|CloudTurnCommand>(path,body,method);
   if(!value||!alive.current||deniedNow.current)return;
   if(isPrompt){pinned.current=(value as CloudTurn).id;if(originalText!==undefined)setPrompt(before=>before===originalText?'':before);}
   setConfirmed(isPrompt?'prompt':'command');await latestLoad.current(false,isPrompt?(value as CloudTurn).id:(value as CloudTurnCommand).turn_id);notifyChanged();
  }finally{commandBusy.current=false;}
 }
 function send(){
  if(!canSend||!view)return;
  void execute(cloudTurnPromptPath(source.submission_id),{project_id:source.project_id,expected_submission_version:view.source.collaboration.submission_version,expected_run_version:runVersion!,expected_version:view.version,prompt},true);
 }
 function reply(permission:CloudTurnPermission,option:CloudTurnPermissionOption){
  if(permissionLocked(permission)||!['allow_once','reject_once'].includes(option.kind)||option.kind==='allow_once'&&!permission.approve_allowed||!permission.options.some(value=>value.id===option.id&&value.kind===option.kind))return;
  void execute(cloudTurnPermissionPath(permission.turn_id,permission.id),{project_id:source.project_id,expected_version:permission.version,option_id:option.id});
 }
 function control(kind:'cancel'|'reconcile'){
  if(locked||!turn||!currentTurn||!view?.allowed_actions.includes(kind)||turn.version>=maximum||kind==='cancel'&&hasCancel)return;
  void execute(cloudTurnControlPath(turn.id,kind),{project_id:source.project_id,expected_version:turn.version});
 }
 function recover(){
  if(!pending||action.busy||denied||originalTurn&&page?.turn.id!==originalTurn)return;
  void execute(pending.path,pending.body,pendingPrompt,pending.method);
 }
 const feedback=<div className="cloud-turns-feedback">
  {denied?<p role="alert">{t('access_changed')}</p>:<><WriteNotice action={action} onRebase={async()=>{if(!await refresh())throw new UiError('workbuddy-cloud-turns.read_failed');}}/>{confirmed&&<p role="status">{t(confirmed==='prompt'?'acknowledged_prompt':'acknowledged_command')}</p>}</>}
  {pending&&<div className="cloud-turns-recovery"><p role="status">{t(pendingPrompt?'unknown_prompt':'unknown_command')}</p><button type="button" data-cloud-turns-recover disabled={action.busy||denied||!!originalTurn&&page?.turn.id!==originalTurn} onClick={recover}>{t('recover')}</button></div>}
 </div>;
 return <section className="workbuddy-cloud-turns" aria-label={t('title')}>
  <button ref={disclosureRef} type="button" className="cloud-turns-disclosure" data-cloud-turns-toggle aria-expanded={open} aria-controls={panelID} onClick={()=>setOpen(value=>!value)}><MessagesSquare size={17} aria-hidden="true"/><span>{t('title')}</span><ChevronDown size={15} aria-hidden="true" className={open?'is-open':''}/></button>
  {!open&&(pending||action.error||denied||confirmed)&&feedback}
  <div id={panelID} data-cloud-turns-panel hidden={!open} className="cloud-turns-panel">
   <p className="cloud-turns-hint">{t('intro')}</p><p className="cloud-turns-hint">{t('read_scope')}</p>{feedback}
   <div className="cloud-turns-toolbar"><span data-cloud-turns-state data-state={turn?.state||'none'} aria-live="polite">{turn?t('state.'+turn.state):t('no_turn')}</span><button type="button" data-cloud-turns-refresh disabled={action.busy||snapshot.loading||!allowed} onClick={()=>void refresh()}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button></div>
   {Boolean(snapshot.error)&&!denied&&<ErrorNotice error={snapshot.error} onRetry={()=>void refresh()}/>}{snapshot.loading&&!view&&<Loading/>}
   {!denied&&turn&&<>
    <div className="cloud-turns-facts"><span>{t('deadline')} <time dateTime={turn.deadline_at}>{time(turn.deadline_at)}</time></span><span>{t('usage.'+turn.usage_state)}</span></div>
    {turn.state==='completed'&&<p className="cloud-turns-boundary">{t('completed_hint')}</p>}{turn.state==='unknown'&&<p className="cloud-turns-boundary">{t('stopped_hint')}</p>}{turn.overdue&&<p role="status">{t('overdue')}</p>}
    {turn.error_code&&<p role="status">{t('error.'+turn.error_code)}</p>}
    {page&&<>
     <p className="cloud-turns-hint">{t('loaded',{value:formatNumber(page.items.length)})} · {t('coverage_hint')}</p>
     {!page.items.length&&!snapshot.loading&&<p>{t('empty')}</p>}
     <ol className="cloud-turns-entries">{page.items.map(entry=>{
      const artifact=entry.kind==='artifact';
      const label=artifact?t('artifact_change',{type:t('artifact_type.'+entry.artifact_type),event:t('artifact_event.'+entry.artifact_event)}):t('entry.'+entry.kind);
      return <li key={entry.sequence} className={artifact?'cloud-turns-artifact':undefined}>
       <div><span>{label}</span><small>{formatNumber(entry.sequence)}</small></div>
       <p className={artifact&&!entry.text?'cloud-turns-artifact-hint':undefined}>{entry.text||t(artifact?'artifact_no_preview':'unsupported')}</p>
       {entry.tool_call_id&&<details><summary>{t('source_reference')}</summary><code>{entry.tool_call_id}</code>{entry.tool_state&&<p>{entry.tool_state}</p>}</details>}
      </li>;
     })}</ol>
     {page.has_more&&<button type="button" data-cloud-turns-more disabled={snapshot.loading||action.busy} onClick={()=>void load(true)}>{t('more')}</button>}
     {!!page.permissions.length&&<section className="cloud-turns-permissions" aria-label={t('permission_title')}><h4><ShieldCheck size={16} aria-hidden="true"/>{t('permission_title')}</h4>{page.permissions.map(permission=><article key={permission.id}>
      <div className="cloud-turns-permission-heading"><strong>{permission.title}</strong><span>{t('permission.'+permission.state)}</span></div><pre>{permission.details}</pre>{!permission.approve_allowed&&<p>{t('details_unavailable')}</p>}
      <div className="cloud-turns-permission-actions">{permission.options.map(option=><button type="button" key={option.id} data-cloud-turns-permission={permission.id} data-cloud-turns-option={option.id} disabled={permissionLocked(permission)||!['allow_once','reject_once'].includes(option.kind)||option.kind==='allow_once'&&!permission.approve_allowed} onClick={()=>reply(permission,option)}>{option.kind==='allow_once'||option.kind==='reject_once'?<><span>{t(option.kind==='allow_once'?'permit_once':'reject_once')}</span><small className="cloud-turns-option-name">{option.name||option.id}</small></>:option.name}</button>)}</div>
      {permission.options.some(option=>!['allow_once','reject_once'].includes(option.kind))&&<p className="cloud-turns-hint">{t('once_only')}</p>}
     </article>)}</section>}
     {!!page.commands.length&&<div className="cloud-turns-command-status" role="status">{page.commands.map(command=><p key={command.id}>{t('command_kind.'+command.kind)} · {t('command.'+command.state)}{command.error_code?' · '+t('error.'+command.error_code):''}</p>)}</div>}
    </>}
    <div className="cloud-turns-controls">{view?.allowed_actions.includes('cancel')&&<button type="button" data-cloud-turns-cancel disabled={locked||hasCancel||!currentTurn} onClick={()=>control('cancel')}><Square size={13} aria-hidden="true"/>{t('cancel')}</button>}{view?.allowed_actions.includes('reconcile')&&<button type="button" data-cloud-turns-reconcile disabled={locked||!currentTurn} onClick={()=>control('reconcile')}>{t('reconcile')}</button>}</div>
    {hasCancel&&<p className="cloud-turns-hint">{t('cancel_hint')}</p>}{view?.allowed_actions.includes('reconcile')&&<p className="cloud-turns-hint">{t('reconcile_hint')}</p>}
    {view?.latest_turn&&page?.turn.id!==view.latest_turn.id&&<button type="button" disabled={locked} onClick={()=>{pinned.current=view.latest_turn!.id;void load(false,pinned.current);}}>{t('select_latest')}</button>}
    <details className="cloud-turns-source"><summary>{t('source_reference')}</summary><dl><dt>{t('turn_reference')}</dt><dd>{turn.id}</dd><dt>{t('source_reference')}</dt><dd>{source.run_id} / {source.node_id}<br/>{turn.source.assignment_sha256}</dd></dl>{turn.stop_reason&&<p>{t('stop_reason')}: {turn.stop_reason}</p>}{turn.usage&&<dl>{Object.entries(turn.usage).map(([name,value])=><div key={name}><dt>{t('tokens.'+name)}</dt><dd>{formatNumber(value)}</dd></div>)}</dl>}</details>
   </>}
   {!denied&&<div className="cloud-turns-compose"><label htmlFor={inputID}>{t('prompt_label')}</label><textarea id={inputID} data-cloud-turns-prompt value={prompt} rows={4} aria-describedby={hintID} disabled={locked||!view?.allowed_actions.includes('prompt')} onChange={event=>setPrompt(event.target.value)}/><p id={hintID} className="cloud-turns-hint">{t('prompt_hint')}</p><div className="cloud-turns-compose-actions"><span>{t('prompt_bytes',{value:formatNumber(promptBytes),limit:formatNumber(32000)})}</span><button type="button" className="primary" data-cloud-turns-send disabled={!canSend} onClick={send}><Send size={14} aria-hidden="true"/>{t(action.busy?'sending':'send')}</button></div>{promptBytes>32000&&<p role="alert">{t('prompt_limit')}</p>}{!runVersion&&<p className="cloud-turns-hint">{t('execution_unavailable')}</p>}</div>}
  </div>
 </section>;
}
