import {Fragment,useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ChevronDown,RefreshCw,ShieldCheck} from 'lucide-react';
import type {ToolPermission} from '../../../packages/contracts/src';
import {ApiError,operation} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {buildPermissionIntent,canChoosePermission,permissionListPath,permissionRecoveryScope,permissionMatchesIntent,validPermission,validPermissionIntent,validPermissionRecovery,validPermissionReceipt,type ToolPermissionIntent,type ToolPermissionScope,type ToolPermissionChoice} from './tool-permission-model';
import './ToolExecutionPermissionPanel.css';

export type ToolExecutionPermissionPanelProps={project:string;runID:string;nodeID:string;attempt:number;fence:number;protocol:'acp-stdio-v1'|'codex-app-server-v1';readOnly?:boolean;state?:string;revision?:number;onChanged?:()=>void|Promise<void>};
type Snapshot={items:ToolPermission[];loading:boolean;error?:unknown;denied?:boolean;loaded?:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-permissions.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const activeStates=new Set(['ready','running','waiting','reconciling']);
const observationUnknown=(permission:ToolPermission)=>!!permission.observation&&(!!permission.observation.failure_code||permission.observation.reply_started&&!permission.observation.reply_written);

function PermissionSource({permission}:{permission:ToolPermission}){
 const acp=permission.request.acp,codex=permission.request.codex,source=acp??codex!;
 const facts:[string,string|number|undefined][]=[['permission_id',permission.id],['attempt',permission.attempt],['fence',permission.fence],['dispatch_id',permission.dispatch_id],['worker_id',permission.worker_id],['profile_sha256',permission.profile_sha256],...(acp?[['session_id',acp.session_id],['tool_call',acp.tool_call_id],['request_kind',acp.kind]] as [string,string][]:[['request_id',codex!.request_id.kind+': '+codex!.request_id.value],['request_kind',codex!.method],['thread_id',codex!.thread_id],['turn_id',codex!.turn_id],['item_id',codex!.item_id],['approval_id',codex!.approval_id],['started_at_ms',codex!.started_at_ms],['params_sha256',codex!.params_sha256],['binding_sha256',codex!.binding_sha256],['item_sha256',codex!.item_sha256]] as [string,string|number|undefined][])];
 return <>
  <div className="tool-permission-impact">{codex?.command&&<code>{codex.command}</code>}{codex?.cwd&&<p>{t('cwd')}: <code>{codex.cwd}</code></p>}{codex?.changes&&<ul>{codex.changes.map((change,index)=><li key={index}><code>{change.path}</code>{change.move_path&&<> → <code>{change.move_path}</code></>}</li>)}</ul>}</div>
  <p className="tool-permissions-hint">{t('expires')}: <time dateTime={permission.expires_at}>{formatDateTime(permission.expires_at,{dateStyle:'short',timeStyle:'short'})}</time></p>
  {!source.details_complete&&<p className="warning">{t('incomplete_details')}</p>}
  {codex&&!codex.scope_supported&&<p className="warning">{t('unsupported_scope')}</p>}
  <details className="tool-permission-source" data-tool-source><summary>{t('source_details')}</summary>
   {source.details&&<pre className="tool-permission-details" tabIndex={0} aria-label={t('details')}>{source.details}</pre>}
   {codex?.reason&&<p>{codex.reason}</p>}
   {codex?.changes?.map((change,index)=><section key={index}><h5><code>{change.path}</code> · {t('change_'+change.kind)}</h5><pre className="tool-permission-details" tabIndex={0} aria-label={t('file_diff',{path:change.path})}>{change.diff}</pre></section>)}
   <dl className="tool-permission-meta">{facts.filter(([,value])=>value!==undefined).map(([label,value])=><Fragment key={label}><dt>{t(label)}</dt><dd><code>{value}</code></dd></Fragment>)}{codex?.unsupported_reason&&<><dt>{t('unsupported_reason')}</dt><dd><code>{codex.unsupported_reason}</code></dd></>}</dl>
  </details>
 </>;
}
function PermissionObservation({permission}:{permission:ToolPermission}){
 const observed=permission.observation;if(!observed)return null;
 return <section className="tool-permission-observation" data-tool-observation aria-label={t('observations')}>
  <dl className="tool-permission-meta">{(['decision_observed','reply_started','reply_written','request_resolved'] as const).map(key=><Fragment key={key}><dt>{t(key)}</dt><dd data-fact={key} data-confirmed={observed[key]}>{t(observed[key]?'observed':'not_observed')}</dd></Fragment>)}<dt>{t('item_status')}</dt><dd>{observed.item_status?t('item_'+observed.item_status):t('not_observed')}</dd></dl>
  <p className="tool-permissions-hint">{t('observation_boundary')}</p>
  {observed.failure_code&&<details><summary>{t('failure_details')}</summary><code>{observed.failure_code}</code></details>}
 </section>;
}

/** A permission belongs to one original attempt. Opening, refreshing and
 * collapsing this view never starts a process or repeats a tool call. */
export function ToolExecutionPermissionPanel(props:ToolExecutionPermissionPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope:ToolPermissionScope={project:props.project,runID:props.runID,nodeID:props.nodeID,attempt:props.attempt,fence:props.fence,protocol:props.protocol,userID:view?.userID||'',organizationID:view?.organizationID||''};
 return <PermissionController key={JSON.stringify([permissionRecoveryScope(scope),owner.current.epoch])} {...props} scope={scope}/>;
}

type IntentState={intent?:ToolPermissionIntent;error?:unknown};
function readIntent(storage:ReturnType<typeof toolSessionStorage>,key:string,scope:ToolPermissionScope):IntentState{
 try{
  if(!storage)throw new UiError('tool-permissions.intent_unavailable');
  const raw=storage.getItem(key);if(!raw)return {};
  if(raw.length>16384)throw new UiError('tool-permissions.intent_unavailable');
  const value:unknown=JSON.parse(raw);if(!validPermissionIntent(value,scope))throw new UiError('tool-permissions.intent_unavailable');
  return {intent:value};
 }catch{return {error:new UiError('tool-permissions.intent_unavailable')};}
}
function PermissionController({scope,state,revision,onChanged,readOnly=false}:ToolExecutionPermissionPanelProps&{scope:ToolPermissionScope}){
 useLocale();const api=useApi(),panelID=useId(),storage=toolSessionStorage(),recoveryScope=permissionRecoveryScope(scope),intentKey='suma-tool-permission-intent:'+recoveryScope;
 const [open,setOpen]=useState(false),[snapshot,setSnapshot]=useState<Snapshot>({items:[],loading:!!scope.userID&&!!scope.organizationID,denied:!scope.userID||!scope.organizationID}),[backgroundPaused,setBackgroundPaused]=useState(false),[confirmed,setConfirmed]=useState<string>();
 const toggleRef=useRef<HTMLButtonElement>(null),lastFocused=useRef<HTMLElement|null>(null),ownedCaller=useRef(api),retiredHistory=useRef<ToolPermissionIntent|undefined>(undefined);
 useLayoutEffect(()=>{const prior=lastFocused.current;if(prior&&!prior.isConnected&&document.activeElement===document.body){toggleRef.current?.focus();lastFocused.current=null;}});
 const [intentState,setIntentState]=useState<IntentState>(()=>readIntent(storage,intentKey,scope));
 const intent=useRef(intentState.intent),history=useRef(intentState.intent),current=useRef(snapshot),alive=useRef(true),generation=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef<Promise<boolean>|undefined>(undefined),flightScope=useRef(''),commandBusy=useRef(false),polls=useRef(0),notify=useRef(onChanged);
 const readScope=JSON.stringify([recoveryScope,scope.attempt,scope.fence]),activeReadScope=useRef(readScope);activeReadScope.current=readScope;notify.current=onChanged;
 const setIntent=(next:IntentState)=>{intent.current=next.intent;if(next.intent)history.current=next.intent;setIntentState(next);};
 const action=useWriteAction({scope:'tool-permissions:'+recoveryScope,storage,validateRequest:request=>validPermissionRecovery(request,scope),validateResponse:(raw,request)=>!!intent.current&&validPermissionReceipt(raw,request,intent.current.scope)&&permissionMatchesIntent(raw,intent.current)});
 // Retire only a definitively rejected write, after the shared journal has
 // cleared its pending body. Unknown or restored operations keep both journals.
 useLayoutEffect(()=>{
  const original=intent.current;
  if(!alive.current||ownedCaller.current!==api||!action.conflict||action.errorStatus!==409||action.pending||!original||!validPermissionIntent(original,scope))return;
  try{
   if(!storage)throw new Error();const raw=storage.getItem(intentKey);
   if(raw!==null&&raw!==JSON.stringify(original))throw new Error();
   storage.removeItem(intentKey);retiredHistory.current=original;setIntent({});
  }catch{setIntent({intent:original,error:new UiError('tool-permissions.intent_unavailable')});}
 },[action.conflict,action.errorStatus,action.pending,api,intentKey,recoveryScope]);
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};
 const deny=useCallback(()=>{
  ++generation.current;controller.current?.abort();current.current={items:[],loading:false,denied:true};setSnapshot(current.current);setConfirmed(undefined);
 },[]);
 const read=useCallback((background=false):Promise<boolean>=>{
  if(activeReadScope.current!==readScope)return Promise.resolve(false);
  if(flight.current){if(flightScope.current!==readScope)return flight.current.then(()=>read(background));if(!background)store({...current.current,loading:true});return flight.current;}
  if(!scope.userID||!scope.organizationID)return Promise.resolve(false);
  const epoch=++generation.current,abort=new AbortController();controller.current=abort;flightScope.current=readScope;
  const previous=current.current.denied?[]:current.current.items;
  store({items:previous,loading:!background||!current.current.loaded,loaded:current.current.loaded});
  const live=()=>alive.current&&!abort.signal.aborted&&generation.current===epoch&&activeReadScope.current===readScope;
  const headers={'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID};
  const list=(value:unknown,limit:number):ToolPermission[]=>{
   const items=value&&typeof value==='object'&&!Array.isArray(value)?(value as {items?:unknown}).items:undefined;
   if(!Array.isArray(items)||items.length>limit||!items.every(item=>validPermission(item,scope))||new Set(items.map(item=>item.id)).size!==items.length)throw new UiError('tool-permissions.invalid_read');
   return items;
  };
  const job=(async()=>{
   try{
    const value=await api<unknown>(permissionListPath(scope),undefined,'GET',headers,abort.signal);if(!live())return false;
    const items=list(value,32),original=intent.current??history.current;
    if(original){
     let row=items.find(item=>item.id===original.permission_id);
     if(!row){
      const raw=await api<unknown>(permissionListPath(scope)+'&permission_id='+encodeURIComponent(original.permission_id),undefined,'GET',headers,abort.signal);if(!live())return false;
      const saved=list(raw,1);row=saved[0];if(row)items.push(row);
     }
     if((!row||!permissionMatchesIntent(row,original))&&(!retiredHistory.current||!!intent.current))throw new UiError('tool-permissions.intent_unavailable');
    }
    store({items,loading:false,loaded:true});return true;
   }catch(error){
    if(!live())return false;
    if(refused(error))deny();else store({items:previous,loading:false,loaded:current.current.loaded,error});
    return false;
   }
  })();
  flight.current=job;void job.finally(()=>{if(flight.current===job)flight.current=undefined;});return job;
 },[api,recoveryScope,readScope,scope.project,scope.runID,scope.nodeID,scope.attempt,scope.fence,scope.userID,scope.organizationID,deny]);
 const readNow=useRef(read);readNow.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++generation.current;controller.current?.abort();flight.current=undefined;};},[]);
 useEffect(()=>{void read();},[read,revision]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(writeDenied)deny();},[writeDenied,deny]);
 // One serial bounded read loop. Historical lookups are exact and never grant
 // delivery or confirm the original write operation.
 useEffect(()=>{
  if(!activeStates.has(state||'')||snapshot.denied||snapshot.error||backgroundPaused)return;
  const timer=window.setInterval(()=>{
   if(document.visibilityState==='hidden'||flight.current||commandBusy.current)return;
   if(polls.current>=120){setBackgroundPaused(true);return;}
   ++polls.current;void read(true);
  },2000);
  return()=>window.clearInterval(timer);
 },[read,state,snapshot.denied,!!snapshot.error,backgroundPaused]);
 const pendingCount=snapshot.items.filter(item=>item.state==='pending'&&item.attempt===scope.attempt&&item.fence===scope.fence).length;
 const savedIntent=intentState.intent;
 const requestMatches=!action.pending||!!savedIntent&&action.pending.path===savedIntent.request.path&&action.pending.method===savedIntent.request.method&&action.pending.idempotency_key===savedIntent.request.idempotency_key&&['project_id','expected_version','idempotency_key','protocol','acp','codex'].every(key=>JSON.stringify(action.pending!.body[key])===JSON.stringify(savedIntent.request.body[key]));
 const intentUnavailable=!!intentState.error||!!action.pending&&(!savedIntent||!requestMatches);
 const hasRecovery=!!savedIntent||!!action.pending;
 const locked=readOnly||action.busy||hasRecovery||intentUnavailable||action.conflict||snapshot.denied||writeDenied||snapshot.loading||!!snapshot.error;
 const recoveryPermission=savedIntent?snapshot.items.find(item=>permissionMatchesIntent(item,savedIntent)):undefined;
 async function refresh(){
  polls.current=0;setBackgroundPaused(false);setIntent(readIntent(storage,intentKey,scope));
  const ready=await readNow.current();if(ready&&writeDenied)action.setError('');return ready;
 }
 async function decide(permission?:ToolPermission,choice?:ToolPermissionChoice){
  if(commandBusy.current||snapshot.denied||writeDenied||intentUnavailable)return;
  let original=intent.current;
  if(original&&(!recoveryPermission||snapshot.loading||snapshot.error||!requestMatches))return;
  if(!original){
   if(!permission||!choice||locked||permission.id===retiredHistory.current?.permission_id||!canChoosePermission(permission,choice,scope))return;
   const key=operation();
   original=buildPermissionIntent(permission,scope,{path:'/tool-permissions/'+encodeURIComponent(permission.id)+'/decision',method:'POST',idempotency_key:key,body:{project_id:scope.project,expected_version:permission.version,idempotency_key:key,protocol:scope.protocol,...(scope.protocol==='acp-stdio-v1'?{acp:choice}:{codex:choice})}});
   if(!original)return;
   try{
    if(!storage)throw new UiError('tool-permissions.intent_unavailable');
    // Both journals precede the HTTP mutation. This source snapshot survives
    // a new attempt without substituting its fence, version, option or key.
    storage.setItem(intentKey,JSON.stringify(original));setIntent({intent:original});
   }catch{action.setError(new UiError('tool-permissions.intent_unavailable'));return;}
  }
  commandBusy.current=true;
  try{
   const value=await action.run<ToolPermission>(original.request.path,original.request.body,original.request.method);
   if(!value||!alive.current)return;
   setConfirmed(value.id);
   try{if(!storage)throw new Error();storage.removeItem(intentKey);setIntent({});}catch{setIntent({intent:original,error:new UiError('tool-permissions.intent_unavailable')});}
   store({items:[...current.current.items.filter(item=>item.id!==value.id),value],loading:false,loaded:true});
   ++generation.current;controller.current?.abort();flight.current=undefined;await readNow.current();
   try{await notify.current?.();}catch{/* The saved decision is not undone by a parent refresh failure. */}
  }finally{commandBusy.current=false;}
 }
 return <section className="tool-permissions" aria-label={t('permissions_title')} onFocusCapture={event=>{lastFocused.current=event.target as HTMLElement;}}>
  <div className="tool-permissions-heading">
   <button ref={toggleRef} type="button" className="tool-permissions-toggle" aria-expanded={open} aria-controls={panelID} onClick={()=>{if(open)toggleRef.current?.focus();setOpen(value=>!value);}}><ShieldCheck size={16}/><span>{t('permissions_title')}</span>{pendingCount>0&&<span className="tool-permissions-count">{formatNumber(pendingCount)}</span>}<ChevronDown size={15} className={open?'is-open':''}/></button>
   <button type="button" className="tool-permissions-refresh" aria-label={t('refresh')} disabled={snapshot.loading||action.busy} onClick={()=>void refresh()}><RefreshCw size={15}/></button>
  </div>
  {!open&&pendingCount>0&&!snapshot.denied&&<p className="tool-permissions-summary" role="status">{t('pending_summary',{count:formatNumber(pendingCount)})}</p>}
  {readOnly&&!snapshot.denied&&!writeDenied&&<p className="tool-permissions-hint">{t('historical_readonly')}</p>}
  {!snapshot.denied&&!writeDenied&&snapshot.items.some(observationUnknown)&&<p className="tool-permissions-summary" role="status" data-tool-observation-unknown>{t('observation_unknown')}</p>}
  <WriteNotice action={action} onRebase={async()=>{if(!await refresh())throw new UiError('tool-permissions.refresh_failed');}}/>
  {intentUnavailable&&!snapshot.denied&&!writeDenied&&<p className="warning" role="status">{t('intent_unavailable')}</p>}
  {hasRecovery&&!snapshot.denied&&!writeDenied&&<div className="tool-permissions-recovery"><p role="status">{t('original_pending')}</p><button type="button" data-tool-permission-recover disabled={action.busy||intentUnavailable||!recoveryPermission||snapshot.loading||!!snapshot.error} onClick={()=>void decide()}>{t('recover')}</button></div>}
  {snapshot.denied||writeDenied?<p role="status">{t('access_denied')}</p>:<>
   {snapshot.error&&<ErrorNotice error={snapshot.error}/>}
   {backgroundPaused&&<p className="tool-permissions-summary" role="status">{t('background_paused')}</p>}
   <div id={panelID} hidden={!open} className="tool-permissions-body" aria-busy={snapshot.loading}>
    <p className="tool-permissions-hint">{t('permission_scope')}</p>
    {snapshot.loading&&!snapshot.loaded&&<p role="status">{t('loading')}</p>}
    {snapshot.loaded&&!snapshot.items.length&&<p className="muted">{t('no_permissions')}</p>}
    {snapshot.items.map(permission=><article className="tool-permission" key={permission.id} data-tool-permission={permission.id}>
     <header><h4>{permission.request.protocol==='acp-stdio-v1'?permission.request.acp!.title:t(permission.request.codex!.method==='item/fileChange/requestApproval'?'file_approval_title':'command_approval_title')}</h4><span className="tool-permission-state" data-state={permission.state}>{t('state_'+permission.state)}</span></header>
     <PermissionSource permission={permission}/>
     {permission.attempt!==scope.attempt||permission.fence!==scope.fence||permission.id===retiredHistory.current?.permission_id?<p className="tool-permissions-hint">{t('historical_readonly')}</p>:null}
     {permission.state==='pending'&&<div className="tool-permission-options">{permission.request.acp?permission.request.acp.options.map(option=>{
      const choice={option_id:option.id,kind:option.kind} as ToolPermissionChoice;
      return <button type="button" key={option.id} data-tool-option={option.id} disabled={!!locked||permission.id===retiredHistory.current?.permission_id||!canChoosePermission(permission,choice,scope)} onClick={()=>void decide(permission,choice)}><span>{option.name}</span><small>{t('option_'+option.kind)}</small></button>;
     }):(['accept','decline','cancel'] as const).map(decision=><button type="button" key={decision} data-tool-option={decision} disabled={!!locked||permission.id===retiredHistory.current?.permission_id||!canChoosePermission(permission,{decision},scope)} onClick={()=>void decide(permission,{decision})}>{t('codex_'+decision)}</button>)}</div>}
     {permission.decision&&<p className="tool-permission-decision"><strong>{t('decision_saved')}</strong> {permission.decision.acp?permission.request.acp?.options.find(option=>option.id===permission.decision?.acp?.option_id)?.name:t('codex_'+permission.decision.codex!.decision)} <time dateTime={permission.decision.decided_at}>{formatDateTime(permission.decision.decided_at,{dateStyle:'short',timeStyle:'short'})}</time></p>}
     <PermissionObservation permission={permission}/>
     {(permission.decision||confirmed===permission.id)&&<p className="tool-permissions-hint">{t('decision_boundary')}</p>}
    </article>)}
   </div>
  </>}
 </section>;
}
