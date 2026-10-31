import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ChevronDown,RefreshCw,ShieldCheck} from 'lucide-react';
import type {ToolPermission} from '../../../packages/contracts/src';
import {ApiError,operation} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {canChoosePermission,permissionListPath,permissionRecoveryScope,permissionMatchesIntent,validPermission,validPermissionIntent,validPermissionRecovery,validPermissionReceipt,type ACPPermissionIntent,type ACPPermissionScope} from './acp-execution-model';
import './ACPExecutionPermissionPanel.css';

export type ACPExecutionPermissionPanelProps={project:string;runID:string;nodeID:string;attempt:number;fence:number;state?:string;revision?:number;onChanged?:()=>void|Promise<void>};
type Snapshot={items:ToolPermission[];loading:boolean;error?:unknown;denied?:boolean;loaded?:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('acp-runtime.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const activeStates=new Set(['ready','running','waiting','reconciling']);

/** A permission belongs to one original attempt. Opening, refreshing and
 * collapsing this view never starts a process or repeats a tool call. */
export function ACPExecutionPermissionPanel(props:ACPExecutionPermissionPanelProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope:ACPPermissionScope={project:props.project,runID:props.runID,nodeID:props.nodeID,attempt:props.attempt,fence:props.fence,userID:view?.userID||'',organizationID:view?.organizationID||''};
 return <PermissionController key={JSON.stringify([permissionRecoveryScope(scope),owner.current.epoch])} {...props} scope={scope}/>;
}

type IntentState={intent?:ACPPermissionIntent;error?:unknown};
function readIntent(storage:ReturnType<typeof toolSessionStorage>,key:string,scope:ACPPermissionScope):IntentState{
 try{
  if(!storage)throw new UiError('acp-runtime.intent_unavailable');
  const raw=storage.getItem(key);if(!raw)return {};
  if(raw.length>16384)throw new UiError('acp-runtime.intent_unavailable');
  const value:unknown=JSON.parse(raw);if(!validPermissionIntent(value,scope))throw new UiError('acp-runtime.intent_unavailable');
  return {intent:value};
 }catch{return {error:new UiError('acp-runtime.intent_unavailable')};}
}
function PermissionController({scope,state,revision,onChanged}:ACPExecutionPermissionPanelProps&{scope:ACPPermissionScope}){
 useLocale();const api=useApi(),panelID=useId(),storage=toolSessionStorage(),recoveryScope=permissionRecoveryScope(scope),intentKey='suma-acp-permission-intent:'+recoveryScope;
 const [open,setOpen]=useState(false),[snapshot,setSnapshot]=useState<Snapshot>({items:[],loading:!!scope.userID&&!!scope.organizationID,denied:!scope.userID||!scope.organizationID}),[backgroundPaused,setBackgroundPaused]=useState(false),[confirmed,setConfirmed]=useState<string>();
 const [intentState,setIntentState]=useState<IntentState>(()=>readIntent(storage,intentKey,scope));
 const intent=useRef(intentState.intent),history=useRef(intentState.intent),current=useRef(snapshot),alive=useRef(true),generation=useRef(0),controller=useRef<AbortController|undefined>(undefined),flight=useRef<Promise<boolean>|undefined>(undefined),flightScope=useRef(''),commandBusy=useRef(false),polls=useRef(0),notify=useRef(onChanged);
 const readScope=JSON.stringify([recoveryScope,scope.attempt,scope.fence]),activeReadScope=useRef(readScope);activeReadScope.current=readScope;notify.current=onChanged;
 const setIntent=(next:IntentState)=>{intent.current=next.intent;if(next.intent)history.current=next.intent;setIntentState(next);};
 const action=useWriteAction({scope:'acp-tool-permissions:'+recoveryScope,storage,validateRequest:request=>validPermissionRecovery(request,scope),validateResponse:(raw,request)=>!!intent.current&&validPermissionReceipt(raw,request,intent.current.scope)&&permissionMatchesIntent(raw,intent.current)});
 const store=(value:Snapshot)=>{current.current=value;setSnapshot(value);};
 const deny=useCallback(()=>{
  ++generation.current;controller.current?.abort();current.current={items:[],loading:false,denied:true};setSnapshot(current.current);setConfirmed(undefined);
 },[]);
 const read=useCallback(():Promise<boolean>=>{
  if(activeReadScope.current!==readScope)return Promise.resolve(false);
  if(flight.current)return flightScope.current===readScope?flight.current:flight.current.then(()=>read());
  if(!scope.userID||!scope.organizationID)return Promise.resolve(false);
  const epoch=++generation.current,abort=new AbortController();controller.current=abort;flightScope.current=readScope;
  const previous=current.current.denied?[]:current.current.items;
  store({items:previous,loading:true,loaded:current.current.loaded});
  const live=()=>alive.current&&!abort.signal.aborted&&generation.current===epoch&&activeReadScope.current===readScope;
  const headers={'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID};
  const list=(value:unknown,limit:number):ToolPermission[]=>{
   const items=value&&typeof value==='object'&&!Array.isArray(value)?(value as {items?:unknown}).items:undefined;
   if(!Array.isArray(items)||items.length>limit||!items.every(item=>validPermission(item,scope))||new Set(items.map(item=>item.id)).size!==items.length)throw new UiError('acp-runtime.invalid_read');
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
     if(!row||!permissionMatchesIntent(row,original))throw new UiError('acp-runtime.intent_unavailable');
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
   ++polls.current;void read();
  },2000);
  return()=>window.clearInterval(timer);
 },[read,state,snapshot.denied,!!snapshot.error,backgroundPaused]);
 const pendingCount=snapshot.items.filter(item=>item.state==='pending'&&item.attempt===scope.attempt&&item.fence===scope.fence).length;
 const savedIntent=intentState.intent;
 const requestMatches=!action.pending||!!savedIntent&&action.pending.path===savedIntent.request.path&&action.pending.method===savedIntent.request.method&&action.pending.idempotency_key===savedIntent.request.idempotency_key&&['project_id','expected_version','idempotency_key','option_id'].every(key=>action.pending!.body[key]===savedIntent.request.body[key]);
 const intentUnavailable=!!intentState.error||!!action.pending&&(!savedIntent||!requestMatches);
 const hasRecovery=!!savedIntent||!!action.pending;
 const locked=action.busy||hasRecovery||intentUnavailable||action.conflict||snapshot.denied||writeDenied||snapshot.loading||!!snapshot.error;
 const recoveryPermission=savedIntent?snapshot.items.find(item=>permissionMatchesIntent(item,savedIntent)):undefined;
 async function refresh(){
  polls.current=0;setBackgroundPaused(false);setIntent(readIntent(storage,intentKey,scope));
  const ready=await readNow.current();if(ready&&writeDenied)action.setError('');return ready;
 }
 async function decide(permission?:ToolPermission,optionID?:string){
  if(commandBusy.current||snapshot.denied||writeDenied||intentUnavailable)return;
  let original=intent.current;
  if(original&&(!recoveryPermission||snapshot.loading||snapshot.error||!requestMatches))return;
  if(!original){
   const option=permission?.request.options.find(item=>item.id===optionID);
   if(!permission||!option||locked||permission.attempt!==scope.attempt||permission.fence!==scope.fence||!canChoosePermission(permission,option)||option.kind!=='allow_once'&&option.kind!=='reject_once')return;
   const key=operation();
   original={scope:{...scope},permission_id:permission.id,profile_sha256:permission.profile_sha256,dispatch_id:permission.dispatch_id,worker_id:permission.worker_id,option_kind:option.kind,request:{path:'/tool-permissions/'+encodeURIComponent(permission.id)+'/decision',method:'POST',idempotency_key:key,body:{project_id:scope.project,expected_version:permission.version,option_id:option.id,idempotency_key:key}}};
   if(!validPermissionIntent(original,scope))return;
   try{
    if(!storage)throw new UiError('acp-runtime.intent_unavailable');
    // Both journals precede the HTTP mutation. This source snapshot survives
    // a new attempt without substituting its fence, version, option or key.
    storage.setItem(intentKey,JSON.stringify(original));setIntent({intent:original});
   }catch{action.setError(new UiError('acp-runtime.intent_unavailable'));return;}
  }
  commandBusy.current=true;
  try{
   const value=await action.run<ToolPermission>(original.request.path,original.request.body,original.request.method);
   if(!value||!alive.current)return;
   setConfirmed(value.id);
   try{if(!storage)throw new Error();storage.removeItem(intentKey);setIntent({});}catch{setIntent({intent:original,error:new UiError('acp-runtime.intent_unavailable')});}
   store({items:[...current.current.items.filter(item=>item.id!==value.id),value],loading:false,loaded:true});
   ++generation.current;controller.current?.abort();flight.current=undefined;await readNow.current();
   try{await notify.current?.();}catch{/* The saved decision is not undone by a parent refresh failure. */}
  }finally{commandBusy.current=false;}
 }
 return <section className="acp-permissions" aria-label={t('permissions_title')}>
  <div className="acp-permissions-heading">
   <button type="button" className="acp-permissions-toggle" aria-expanded={open} aria-controls={panelID} onClick={()=>setOpen(value=>!value)}><ShieldCheck size={16}/><span>{t('permissions_title')}</span>{pendingCount>0&&<span className="acp-permissions-count">{formatNumber(pendingCount)}</span>}<ChevronDown size={15} className={open?'is-open':''}/></button>
   <button type="button" className="acp-permissions-refresh" aria-label={t('refresh')} disabled={snapshot.loading||action.busy} onClick={()=>void refresh()}><RefreshCw size={15}/></button>
  </div>
  {!open&&pendingCount>0&&!snapshot.denied&&<p className="acp-permissions-summary" role="status">{t('pending_summary',{count:formatNumber(pendingCount)})}</p>}
  <WriteNotice action={action} onRebase={async()=>{if(!await refresh())throw new UiError('acp-runtime.refresh_failed');}}/>
  {intentUnavailable&&!snapshot.denied&&!writeDenied&&<p className="warning" role="status">{t('intent_unavailable')}</p>}
  {hasRecovery&&!snapshot.denied&&!writeDenied&&<div className="acp-permissions-recovery"><p role="status">{t('original_pending')}</p><button type="button" data-acp-permission-recover disabled={action.busy||intentUnavailable||!recoveryPermission||snapshot.loading||!!snapshot.error} onClick={()=>void decide()}>{t('recover')}</button></div>}
  {snapshot.denied||writeDenied?<p role="status">{t('access_denied')}</p>:<>
   {snapshot.error&&<ErrorNotice error={snapshot.error}/>}
   {backgroundPaused&&<p className="acp-permissions-summary" role="status">{t('background_paused')}</p>}
   <div id={panelID} hidden={!open} className="acp-permissions-body" aria-busy={snapshot.loading}>
    <p className="acp-permissions-hint">{t('permission_scope')}</p>
    {snapshot.loading&&!snapshot.loaded&&<p role="status">{t('loading')}</p>}
    {snapshot.loaded&&!snapshot.items.length&&<p className="muted">{t('no_permissions')}</p>}
    {snapshot.items.map(permission=><article className="acp-permission" key={permission.id} data-acp-permission={permission.id}>
     <header><h4>{permission.request.title}</h4><span className="acp-permission-state" data-state={permission.state}>{t('state_'+permission.state)}</span></header>
     <dl className="acp-permission-meta"><dt>{t('tool_call')}</dt><dd><code>{permission.request.tool_call_id}</code></dd><dt>{t('request_kind')}</dt><dd><code>{permission.request.kind}</code></dd><dt>{t('attempt')}</dt><dd>{formatNumber(permission.attempt)}</dd><dt>{t('expires')}</dt><dd><time dateTime={permission.expires_at}>{formatDateTime(permission.expires_at,{dateStyle:'short',timeStyle:'short'})}</time></dd></dl>
     {permission.request.details&&<pre className="acp-permission-details" tabIndex={0} aria-label={t('details')}>{permission.request.details}</pre>}
     {!permission.request.details_complete&&<p className="warning">{t('incomplete_details')}</p>}
     {permission.state==='pending'&&<div className="acp-permission-options">{permission.request.options.map(option=><button type="button" key={option.id} data-acp-option={option.id} disabled={!!locked||permission.attempt!==scope.attempt||permission.fence!==scope.fence||!canChoosePermission(permission,option)} onClick={()=>void decide(permission,option.id)}><span>{option.name}</span><small>{t('option_'+option.kind)}</small></button>)}</div>}
     {permission.decision&&<p className="acp-permission-decision"><strong>{t('decision_saved')}</strong> {permission.request.options.find(option=>option.id===permission.decision?.option_id)?.name} <time dateTime={permission.decision.decided_at}>{formatDateTime(permission.decision.decided_at,{dateStyle:'short',timeStyle:'short'})}</time></p>}
     {(permission.decision||confirmed===permission.id)&&<p className="acp-permissions-hint">{t('decision_boundary')}</p>}
    </article>)}
   </div>
  </>}
 </section>;
}
