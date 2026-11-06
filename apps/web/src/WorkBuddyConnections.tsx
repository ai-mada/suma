import {useEffect,useId,useRef,useState} from 'react';
import {Check,ChevronRight,Copy,ExternalLink,Link2,Plus,RefreshCw,Search,ShieldCheck} from 'lucide-react';
import type {WorkBuddyAuthorization,WorkBuddyConnection,WorkBuddyConnectionPage} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,tr,useLocale,UiError} from './i18n';
import {Empty,ErrorNotice,Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {desktopWorkspaceBridge} from './desktop-workspace-model';
import {useLeaveGuard,useWriteAction,WriteNotice} from './write-action';
import {readWorkBuddyAuthorization,readWorkBuddyConnection,readWorkBuddyPage,validWorkBuddyReceipt,validWorkBuddyRecovery,workBuddyPageSize} from './workbuddy-connections-model';
import {workBuddyConnectionReason,workBuddyReusableScopes,workBuddyScopeDifference} from './workbuddy-connection-presentation';
import {reserveWorkBuddyAuthorizationTab,type WorkBuddyAuthorizationTab} from './workbuddy-authorization-tab';
import './WorkBuddyConnections.css';

const t=(key:string,params?:Record<string,unknown>)=>tr('workbuddy-connections.'+key,params);
const scopeLabel=(scope:string)=>{const key='workbuddy-connections.scope.'+scope;return tr(key)===key?t('scope_unknown',{scope}):tr(key);};
const time=(value:string)=>value?formatDateTime(value):t('no_expiry');
const canLeave=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));

/** A clean callback locator is a connection ID only, never OAuth callback data. */
export function WorkBuddyConnections({project,panel,openRequest}:{project:string;panel?:string;openRequest?:number}){
 useLocale();const view=useApiView(),context=JSON.stringify([view?.organizationID,view?.userID,project]),focusedID=panel?.match(/^workbuddy:([A-Za-z0-9][A-Za-z0-9_.-]{0,199})$/)?.[1]||'',disclosureKey=JSON.stringify([context,focusedID]),[disclosure,setDisclosure]=useState<{key:string;open:boolean}>(),id=useId(),open=disclosure?.key===disclosureKey?disclosure.open:!!focusedID||panel==='workbuddy',section=useRef<HTMLElement|null>(null);
 useEffect(()=>{if(!openRequest&&panel!=='workbuddy')return;setDisclosure({key:disclosureKey,open:true});const frame=requestAnimationFrame(()=>{section.current?.scrollIntoView?.({block:'start',behavior:'auto'});section.current?.querySelector<HTMLButtonElement>('.workbuddy-disclosure')?.focus();});return()=>cancelAnimationFrame(frame);},[disclosureKey,openRequest,panel]);
 if(!project)return null;
 return <section ref={section} className="panel workbuddy-connections" aria-label={t('title')}>
  <button className="workbuddy-disclosure" type="button" aria-expanded={open} aria-controls={id} onClick={()=>{if(!canLeave())return;setDisclosure({key:disclosureKey,open:!open});}}><ChevronRight size={20} aria-hidden="true"/><span><strong>{t('title')}</strong><small>{t('intro')}</small></span><span className="workbuddy-disclosure-action">{t(open?'collapse':'expand')}</span></button>
  {open&&<div id={id}><ConnectionManager key={disclosureKey} project={project} context={context} focusedID={focusedID}/></div>}
 </section>;
}
type ConnectionCheck={id:string;kind:'authorization'|'connection'|'operation'};
type Snapshot={caller:ApiCaller;revision:number;cursor:string;page?:WorkBuddyConnectionPage;focused?:WorkBuddyConnection;error?:unknown;connectionCheck?:ConnectionCheck;loading:boolean};
const accessDenied=(status:number|undefined)=>status===401||status===403||status===404;
type Form={kind:'authorize';connection?:WorkBuddyConnection}|{kind:'disconnect';connection:WorkBuddyConnection};
const firstAuthorization:Form={kind:'authorize'};
type NativeAuthorizationBridge={openWorkBuddyAuthorization:(request:{project_id:string;authorization_id:string;user_id:string;tenant_id:string})=>Promise<unknown>};
type AuthorizationLaunch=({kind:'browser';tab?:WorkBuddyAuthorizationTab}|{kind:'desktop'})&{authorizationID?:string};
function ConnectionManager({project,context,focusedID}:{project:string;context:string;focusedID:string}){
 useLocale();const caller=useApi(),view=useApiView(),owner=view?.userID,scope='workbuddy-connections:'+context,desktop=desktopWorkspaceBridge(),native=desktop&&typeof(desktop as unknown as Partial<NativeAuthorizationBridge>).openWorkBuddyAuthorization==='function'?desktop as unknown as NativeAuthorizationBridge:undefined,desktopUnavailable=!!desktop&&!native;
 const [snapshot,setSnapshot]=useState<Snapshot>(),[cursor,setCursor]=useState(''),[history,setHistory]=useState<string[]>([]),[revision,setRevision]=useState(0),[form,setForm]=useState<Form>(),[selectedScopes,setSelectedScopes]=useState<string[]>([]),[confirmed,setConfirmed]=useState(false),[authorization,setAuthorization]=useState<WorkBuddyAuthorization>(),[notice,setNotice]=useState('');
 const [nativeBusy,setNativeBusy]=useState(false),nativeAttempt=useRef(0),nativePending=useRef(false),checkingCurrent=useRef(false);
 const [editingScopes,setEditingScopes]=useState(false);
 const [returnConnectionID,setReturnConnectionID]=useState('');
 const detailID=returnConnectionID||focusedID;
 const openedBrowserAuthorization=useRef('');
 const launch=useRef<AuthorizationLaunch|undefined>(undefined);
 const formHeading=useRef<HTMLHeadingElement>(null),focusForm=useRef(false);
 const authorizationActions=useRef<HTMLDivElement>(null),focusAuthorization=useRef('');
 const refreshButton=useRef<HTMLButtonElement>(null),focusAfterCheck=useRef(false);
 const scopeOptions=useRef<HTMLDivElement>(null);
 const formID=useId();
 const historyID=useId(),[historyDisclosure,setHistoryDisclosure]=useState<boolean>();
 const epoch=useRef(0),alive=useRef(true),activeCaller=useRef(caller);activeCaller.current=caller;
 const action=useWriteAction({scope,storage:toolSessionStorage(),validateRequest:request=>validWorkBuddyRecovery(request,project),validateResponse:(value,request)=>validWorkBuddyReceipt(value,request,project,owner)});
 const locked=action.busy||!!action.pending||action.persistenceBlocked||nativeBusy;
 const current=snapshot?.caller===caller&&snapshot.revision===revision&&snapshot.cursor===cursor?snapshot:undefined;
 const denied=accessDenied(action.errorStatus)||current?.error instanceof ApiError&&accessDenied(current.error.status);
 const page=!denied&&!current?.loading?current?.page:undefined;
 const rows=page?(current?.focused?[current.focused,...page.items.filter(row=>row.id!==current.focused!.id)]:page.items):[];
 const authorizationRow=authorization&&rows.find(row=>row.id===authorization.connection.id);
 const authorizationCurrent=!!page?.can_authorize&&(!authorizationRow||
  authorizationRow.state==='authorizing'&&authorizationRow.version===authorization?.connection.version&&
  authorizationRow.grant_generation===authorization?.connection.grant_generation);
 const authorizationVisible=!desktopUnavailable&&authorizationCurrent&&authorization?.state==='pending'&&
  !!authorization.authorization_url&&Date.parse(authorization.expires_at)>Date.now();
 const awaitingConfirmation=notice==='browser_opened'||notice==='desktop_opened'||notice==='authorization_pending';
 const officialActionClass='workbuddy-official-link'+(awaitingConfirmation?' workbuddy-reopen':' primary');
 const flowConnection=authorizationVisible?authorization?.connection:form?.kind==='authorize'?form.connection:undefined;
 const listedRows=rows.filter(row=>row.id!==flowConnection?.id||!authorizationVisible&&row.state==='disconnected');
 const currentRows=listedRows.filter(row=>row.state!=='disconnected'),disconnectedRows=listedRows.filter(row=>row.state==='disconnected');
 const historyExpanded=historyDisclosure??disconnectedRows.some(row=>row.id===detailID);
 const mayRecover=!!page&&(page.can_authorize||rows.some(row=>row.allowed_actions.length>0));
 const firstConnection=!!page?.can_authorize&&!desktopUnavailable&&!cursor&&!page.next_cursor&&!rows.length&&!authorization;
 const visibleForm=form??(firstConnection&&!action.pending?firstAuthorization:undefined);
 const formConnection=visibleForm?.connection;
 const reusableScopes=formConnection&&page?workBuddyReusableScopes(formConnection.scopes,page.available_scopes):[];
 const unavailableScopes=formConnection?.scopes.filter(scope=>!reusableScopes.includes(scope))??[];
 const showScopeEditor=editingScopes||reusableScopes.length===0;
 const allowedActions=formConnection?rows.find(row=>row.id===formConnection.id)?.allowed_actions:undefined;
 const formAllowed=visibleForm?.kind==='authorize'
  ? !desktopUnavailable&&(formConnection?!!allowedActions?.includes('authorize'):!!page?.can_authorize)
  : !!allowedActions?.includes('disconnect');
 useLeaveGuard(!!form&&(selectedScopes.length>0||confirmed),action.setError);
 useEffect(()=>{
  if(!formAllowed||!focusForm.current)return;
  focusForm.current=false;
  formHeading.current?.focus();
 },[form,formAllowed]);
 useEffect(()=>{
  const button=refreshButton.current;
  if(!focusAfterCheck.current||!current||current.loading||!button||button.disabled)return;
  focusAfterCheck.current=false;
  if(document.activeElement===document.body)button.focus();
 },[current,action.busy,nativeBusy,form]);
 useEffect(()=>{
  if(editingScopes)scopeOptions.current?.querySelector('input')?.focus();
 },[editingScopes]);
 useEffect(()=>{
  alive.current=true;
  return()=>{alive.current=false;++epoch.current;++nativeAttempt.current;closeLaunch();};
 },[]);
 useEffect(()=>{++nativeAttempt.current;nativePending.current=false;setNativeBusy(false);},[caller,authorization?.id,authorization?.version]);
 useEffect(()=>{if(denied){setAuthorization(undefined);setForm(undefined);setSelectedScopes([]);setConfirmed(false);setNotice('');setSnapshot(before=>before?.page?{...before,page:undefined,focused:undefined}:before);}},[denied]);
 useEffect(()=>{if(page&&form&&!formAllowed){setForm(undefined);setSelectedScopes([]);setConfirmed(false);}},[page,form,formAllowed]);
 useEffect(()=>{
  const generation=++epoch.current,controller=new AbortController();setSnapshot({caller,revision,cursor,loading:true});
  let refusal:ApiError|undefined;
  const read=(path:string)=>caller<unknown>(path,undefined,'GET',undefined,controller.signal).catch(error=>{
   // Each request owns its refusal: an earlier network failure from the other
   // read must never conceal a later authorization loss or preserve its URL.
   if(generation===epoch.current&&alive.current&&activeCaller.current===caller&&!controller.signal.aborted&&error instanceof ApiError&&accessDenied(error.status)){
    refusal=error;setSnapshot({caller,revision,cursor,loading:false,error});setAuthorization(undefined);setForm(undefined);setSelectedScopes([]);setConfirmed(false);
   }
   throw error;
  });
  const list=read('/workbuddy-connections?project_id='+encodeURIComponent(project)+'&limit='+workBuddyPageSize+(cursor?'&cursor='+encodeURIComponent(cursor):''));
  const detail=detailID?read('/workbuddy-connections/'+encodeURIComponent(detailID)+'?project_id='+encodeURIComponent(project)):Promise.resolve(undefined);
  void Promise.all([list,detail]).then(([value,focusedValue])=>{
   if(generation!==epoch.current||!alive.current||activeCaller.current!==caller)return;
   const result=readWorkBuddyPage(value,project,owner),focused=detailID?readWorkBuddyConnection(focusedValue,project,owner):undefined;
   if(!result||detailID&&(!focused||focused.id!==detailID))throw new UiError('workbuddy-connections.invalid_response');
   setSnapshot({caller,revision,cursor,loading:false,page:result,focused});
  }).catch(error=>{if(generation!==epoch.current||controller.signal.aborted||!alive.current)return;const failure=refusal??error;setSnapshot({caller,revision,cursor,loading:false,error:failure});if(failure instanceof ApiError&&accessDenied(failure.status)){setAuthorization(undefined);setForm(undefined);setSelectedScopes([]);setConfirmed(false);}});
  return()=>{++epoch.current;controller.abort();};
 },[caller,project,owner,cursor,revision,detailID]);
 useEffect(()=>{
  if(!authorization)return;
  const expire=()=>{
   if(!authorizationRow)setReturnConnectionID(authorization.connection.id);
   setAuthorization(undefined);
   setNotice('link_expired');
  };
  const remaining=Date.parse(authorization.expires_at)-Date.now();
  if(remaining<=0){expire();return;}
  const timer=setTimeout(expire,Math.min(remaining,2147483647));
  return()=>clearTimeout(timer);
 },[authorization,authorizationRow]);
 useEffect(()=>{
  if(authorization&&page&&!authorizationCurrent)setAuthorization(undefined);
 },[page,authorizationCurrent,authorization]);
 const refresh=()=>{action.setError('');action.clearConflict();setRevision(value=>value+1);};
 function openForm(next:Form){if(locked||desktopUnavailable&&next.kind==='authorize'||!canLeave())return;focusForm.current=true;setForm(next);setEditingScopes(false);setSelectedScopes([]);setConfirmed(false);setAuthorization(undefined);setNotice('');action.clearConflict();}
 function cancelForm(){if(locked)return;setForm(undefined);setEditingScopes(false);setSelectedScopes([]);setConfirmed(false);action.clearConflict();}
 function selectScope(scope:string,checked:boolean){
  if(!form)setForm(firstAuthorization);
  setSelectedScopes(values=>checked?[...values,scope]:values.filter(value=>value!==scope));
 }
 function closeLaunch(){
  const attempt=launch.current;
  launch.current=undefined;
  if(attempt?.kind==='browser')attempt.tab?.close();
 }
 function prepareAuthorization(scopes:string[],openOfficial:boolean){
  if(!formAllowed||locked||action.conflict||launch.current||!scopes.length)return;
  const attempt:AuthorizationLaunch|undefined=openOfficial
   ? native?{kind:'desktop'}:{kind:'browser',tab:reserveWorkBuddyAuthorizationTab(t('preparing'))}
   : undefined;
  launch.current=attempt;
  void write('/workbuddy-authorizations',{
   project_id:project,scopes,...(formConnection?{connection_id:formConnection.id,expected_version:formConnection.version}:{}),
  },'POST',attempt);
 }
 async function write(path:string,body:Record<string,unknown>,method='POST',attempt?:AuthorizationLaunch){
  if(desktopUnavailable&&path==='/workbuddy-authorizations')return;
  const result=await action.run<unknown>(path,body,method);
  if(!alive.current||activeCaller.current!==caller||!result){
   if(attempt&&launch.current===attempt)closeLaunch();
   return;
  }
  setForm(undefined);setEditingScopes(false);setSelectedScopes([]);setConfirmed(false);
  if(path==='/workbuddy-authorizations'){
   const receipt=readWorkBuddyAuthorization(result,project,owner);
   if(attempt&&launch.current===attempt){
    if(receipt?.authorization_url)attempt.authorizationID=receipt.id;
    else closeLaunch();
   }
   focusAuthorization.current=receipt?.authorization_url?receipt.id:'';
   setAuthorization(receipt);setNotice(receipt?.authorization_url?'':'authorization_recorded');
   // Preserve the returned URL in memory while the bounded list is rechecked.
   setRevision(value=>value+1);
  }else{
   const row=readWorkBuddyConnection(result,project,owner);
   setAuthorization(undefined);
   setNotice(connectionWriteNotice(row?.state,path.endsWith('/disconnect')));
   setRevision(value=>value+1);
  }
 }
 function checkCurrent(connection?:WorkBuddyConnection,moveFocus=false){
  const request=connection?undefined:action.pending,originalAuthorization=connection?undefined:authorization;
  const id=connection?.id??(request?request.path==='/workbuddy-authorizations'?request.body.connection_id:request.path.split('/')[2]:originalAuthorization?.connection.id);
  if(typeof id!=='string'||!id){refresh();return;}
  const connectionID=request&&request.path!=='/workbuddy-authorizations'?decodeURIComponent(id):id;
  void checkConnection({id:connectionID,kind:connection?'connection':request?'operation':'authorization'},moveFocus);
 }
 function refreshCurrent(){
  // A page without the checked connection cannot resolve a failed exact read.
  if(current?.connectionCheck){void checkConnection(current.connectionCheck,true);return;}
  refresh();
 }
 async function checkConnection(target:ConnectionCheck,moveFocus:boolean){
  if(checkingCurrent.current||!alive.current||activeCaller.current!==caller)return;
  const originalAuthorization=target.kind==='authorization'&&authorization?.connection.id===target.id?authorization:undefined;
  focusAfterCheck.current=moveFocus;
  checkingCurrent.current=true;
  const generation=++epoch.current;setSnapshot({caller,revision,cursor,loading:true,connectionCheck:target});
  try{
   const value=await caller<unknown>('/workbuddy-connections/'+encodeURIComponent(target.id)+'?project_id='+encodeURIComponent(project));
   if(!alive.current||generation!==epoch.current||activeCaller.current!==caller)return;
   const row=readWorkBuddyConnection(value,project,owner);if(!row||row.id!==target.id)throw new UiError('workbuddy-connections.invalid_response');
   if(originalAuthorization){
    const sameGrant=row.grant_generation===originalAuthorization.connection.grant_generation;
    const stillPending=sameGrant&&row.state==='authorizing'&&row.version===originalAuthorization.connection.version;
    if(!stillPending){
     setAuthorization(undefined);
     if(!rows.some(item=>item.id===row.id))setReturnConnectionID(row.id);
    }
    if(row.state==='disconnected'){
     setHistoryDisclosure(true);
     setNotice('authorization_disconnected');
    }else setNotice(sameGrant&&row.state==='connected'?'authorization_complete':stillPending?'authorization_pending':'authorization_changed');
   }else setNotice(target.kind==='connection'?'connection_checked':'status_checked');
   setRevision(value=>value+1);
  }catch(error){
   if(!alive.current||generation!==epoch.current||activeCaller.current!==caller)return;
   const refused=error instanceof ApiError&&accessDenied(error.status);
   setSnapshot({caller,revision,cursor,loading:false,error,connectionCheck:refused?undefined:target});
   if(refused)setAuthorization(undefined);
  }
  finally{checkingCurrent.current=false;}
 }
 useEffect(()=>{
  if(desktopUnavailable||!authorization||!page||denied||locked||form)return;
  const onFocus=()=>{if(!nativePending.current&&(native||openedBrowserAuthorization.current===authorization.id))void checkCurrent();};
  window.addEventListener('focus',onFocus);return()=>window.removeEventListener('focus',onFocus);
 });
 async function openNativeAuthorization(){
  if(!native||!authorization||!authorizationVisible||Date.parse(authorization.expires_at)<=Date.now()||
   locked||form||nativePending.current||!view?.userID||!view.organizationID)return;
  const sequence=++nativeAttempt.current;nativePending.current=true;setNativeBusy(true);setNotice('');
  try{
   const result=await native.openWorkBuddyAuthorization({project_id:project,authorization_id:authorization.id,user_id:view.userID,tenant_id:view.organizationID});
   if(!alive.current||activeCaller.current!==caller||sequence!==nativeAttempt.current)return;
   if(result&&typeof result==='object'&&!Array.isArray(result)&&'status' in result&&result.status==='opened'&&Object.keys(result).length===1)setNotice('desktop_opened');
   else setNotice(result&&typeof result==='object'&&'code' in result&&result.code==='oauth_window_closed'?'desktop_closed':'desktop_failed');
  }catch{if(alive.current&&activeCaller.current===caller&&sequence===nativeAttempt.current)setNotice('desktop_failed');}
  finally{if(sequence===nativeAttempt.current){nativePending.current=false;if(alive.current&&activeCaller.current===caller)setNativeBusy(false);}}
 }
 useEffect(()=>{
  const attempt=launch.current;
  if(attempt?.authorizationID&&(denied||current?.error||page&&!authorizationVisible))closeLaunch();
  if(!authorizationVisible||!authorization||authorization.id!==focusAuthorization.current||locked||form)return;
  if(Date.parse(authorization.expires_at)<=Date.now()){closeLaunch();return;}
  focusAuthorization.current='';
  if(attempt?.authorizationID===authorization.id&&launch.current===attempt){
   launch.current=undefined;
   if(attempt.kind==='desktop')void openNativeAuthorization();
   else if(authorization.authorization_url&&attempt.tab?.navigate(authorization.authorization_url)){
    openedBrowserAuthorization.current=authorization.id;
    setNotice('browser_opened');
    return;
   }else{
    attempt.tab?.close();
    setNotice('browser_manual');
   }
  }
  authorizationActions.current?.querySelector<HTMLElement>('.workbuddy-official-link')?.focus();
 });
 return <div className="workbuddy-manager">
  <div className="workbuddy-toolbar"><p className="workbuddy-boundary"><ShieldCheck size={17} aria-hidden="true"/>{t('boundary')}</p><button type="button" ref={refreshButton} disabled={current?.loading||action.busy||nativeBusy||!!form} onClick={refreshCurrent}><RefreshCw size={16} aria-hidden="true"/>{t(current?.connectionCheck?(current.loading?'check_current':'retry_check'):'refresh_list')}</button></div>
  {desktopUnavailable&&page?.can_authorize&&<DesktopAuthorizationGuide project={project}/>}
  <WriteNotice action={action} onRebase={async()=>{cancelForm();refresh();}}/>
  {action.pending&&<div className="workbuddy-recovery" role="status"><p>{t(action.unknown?'unknown':'pending')}</p><div className="workbuddy-actions">{!current?.connectionCheck&&<button type="button" disabled={action.busy||denied} onClick={()=>checkCurrent(undefined,true)}>{t('check_current')}</button>}{mayRecover&&(!desktopUnavailable||action.pending.path!=='/workbuddy-authorizations')&&<button type="button" disabled={action.busy} onClick={()=>{const request=action.pending!;void write(request.path,request.body,request.method);}}>{t('recover')}</button>}</div></div>}
  {notice&&!denied&&!current?.connectionCheck&&<p className="workbuddy-notice" role="status" data-success={notice==='authorization_complete'} data-attention={notice==='refresh_unknown_notice'||notice==='reason.reauthorization_required'||notice==='link_expired'||notice==='browser_manual'}>{notice==='authorization_complete'&&<Check size={18} aria-hidden="true"/>}{t(notice)}</p>}
  {authorizationVisible&&authorization&&<section className="workbuddy-authorization" aria-labelledby={formID+'-ready'}>
   <div className="workbuddy-flow-heading">
    <span className="workbuddy-step" aria-hidden="true">2</span>
    <div><h3 id={formID+'-ready'}>{t(awaitingConfirmation?'awaiting_confirmation':'ready')}</h3><p>{t('ready_hint')}</p></div>
   </div>
   <div className="workbuddy-actions workbuddy-flow-actions" ref={authorizationActions}>
    {native?<button type="button" className={officialActionClass} disabled={locked} onClick={()=>void openNativeAuthorization()}>{t(nativeBusy?'desktop_opening':'desktop_open')}<ExternalLink size={16} aria-hidden="true"/></button>:
     <a className={officialActionClass} href={authorization.authorization_url} target="_blank" rel="noopener noreferrer" onClick={event=>{
      if(Date.parse(authorization.expires_at)<=Date.now()){event.preventDefault();return;}
      openedBrowserAuthorization.current=authorization.id;
      setNotice('browser_opened');
     }}>{t(awaitingConfirmation?'official_reopen':'official_link')}<ExternalLink size={16} aria-hidden="true"/></a>}
    <button type="button" className={awaitingConfirmation?'primary':undefined} disabled={nativeBusy||current?.loading} onClick={()=>checkCurrent(undefined,true)}>{t('check_authorization')}</button>
   </div>
   <div className="workbuddy-flow-footer">
    <small>{t('link_expiry',{time:time(authorization.expires_at)})}</small>
    <button type="button" disabled={locked} onClick={()=>{if(!authorizationRow)setReturnConnectionID(authorization.connection.id);setAuthorization(undefined);setNotice('');refreshButton.current?.focus();}}>{t('return_to_connections')}</button>
   </div>
  </section>}
  {denied?<><p className="muted">{t('access_lost')}</p>{current?.error&&<ErrorNotice error={current.error}/>}</>:!current||current.loading?<Loading/>:current.error?<ErrorNotice error={current.error} onRetry={current.connectionCheck?undefined:refresh}/>:page&&<>
   {!page.configured&&<div className="workbuddy-config">
    <div className="workbuddy-flow-heading">
     <span className="workbuddy-step" aria-hidden="true"><Link2 size={18}/></span>
     <div><h3>{t('not_configured')}</h3><p>{t('setup_contact')}</p></div>
    </div>
    <details><summary>{t('setup_details')}</summary><p>{t('setup_requirements')}</p><code>/api/v1/workbuddy-oauth/callback</code></details>
   </div>}
   {!desktopUnavailable&&page.can_authorize&&!firstConnection&&!authorizationVisible&&!form&&<button type="button" className="workbuddy-add" disabled={locked} onClick={()=>openForm(firstAuthorization)}><Plus size={16} aria-hidden="true"/>{t('connect')}</button>}
   {visibleForm&&formAllowed&&<form className="workbuddy-form" aria-labelledby={formID} onSubmit={event=>{
    event.preventDefault();if(locked||action.conflict)return;
    if(visibleForm.kind==='authorize'){
     prepareAuthorization(selectedScopes,!!(event.nativeEvent as SubmitEvent).submitter);
    }else if(confirmed){
     void write('/workbuddy-connections/'+encodeURIComponent(visibleForm.connection.id)+'/disconnect',{project_id:project,expected_version:visibleForm.connection.version,reason:'user_requested_local_disconnect'});
    }
   }}>
    <div className="workbuddy-flow-heading">
     {visibleForm.kind==='authorize'&&<span className="workbuddy-step" aria-hidden="true">1</span>}
     <div>
      <h3 id={formID} ref={formHeading} tabIndex={-1}>{t(firstConnection?'connect':visibleForm.kind==='authorize'?'choose_scopes':'disconnect_title')}</h3>
      {visibleForm.kind==='authorize'&&<p>{t(visibleForm.connection?.state==='authorizing'?'replace_link_hint':'scopes_hint')}</p>}
     </div>
    </div>
    {visibleForm.connection&&<p className="workbuddy-form-account"><span>{t('account')}</span> <code>{visibleForm.connection.open_id||t('account_pending')}</code></p>}
    <fieldset disabled={locked||action.conflict}><legend>{t(visibleForm.kind==='authorize'?'permissions':'confirmation')}</legend>{visibleForm.kind==='authorize'?<>
     {!showScopeEditor&&<div className="workbuddy-current-scopes workbuddy-reuse">
      <h5>{t('reusable_permissions')}</h5>
      <PermissionList scopes={reusableScopes}/>
      {unavailableScopes.length>0&&<p className="workbuddy-unavailable">{t('reuse_unavailable',{scopes:unavailableScopes.map(scopeLabel).join(' · ')})}</p>}
      <div className="workbuddy-actions workbuddy-flow-actions">
       <button className="primary" type="button" onClick={()=>prepareAuthorization(reusableScopes,true)}>{t(action.busy?'preparing':'reuse_continue')}<ExternalLink size={16} aria-hidden="true"/></button>
       <button type="button" onClick={()=>{setEditingScopes(true);setSelectedScopes(reusableScopes);}}>{t('reuse_permissions')}</button>
      </div>
     </div>}
     {showScopeEditor&&<div ref={scopeOptions} className="workbuddy-scope-options">{page.available_scopes.map(item=>{
      const hintID=formID+'-'+item;
      return <label className="workbuddy-check workbuddy-scope-option" key={item}>
       <input type="checkbox" value={item} aria-label={scopeLabel(item)} aria-describedby={hintID} checked={selectedScopes.includes(item)} onChange={event=>selectScope(item,event.target.checked)}/>
       <span><strong>{scopeLabel(item)}</strong><small id={hintID}>{t('scope_help.'+item)}</small></span>
      </label>;
     })}</div>}
     {formConnection&&selectedScopes.length>0&&<PermissionChanges current={formConnection.scopes} requested={selectedScopes} available={page.available_scopes}/>}
    </>:<><p className="workbuddy-local-warning">{t('disconnect_warning')}</p><label className="workbuddy-check"><input name="confirm-disconnect" type="checkbox" checked={confirmed} onChange={event=>setConfirmed(event.target.checked)}/><span>{t('disconnect_confirm')}</span></label></>}</fieldset>
    <div className="workbuddy-actions workbuddy-flow-actions">
     {(visibleForm.kind==='disconnect'||showScopeEditor)&&<button className="primary" type="submit" disabled={locked||action.conflict||(visibleForm.kind==='authorize'?!selectedScopes.length:!confirmed)}>{t(visibleForm.kind==='authorize'?action.busy?'preparing':'prepare':'disconnect_submit')} {visibleForm.kind==='authorize'&&<ExternalLink size={16} aria-hidden="true"/>}</button>}
     {form&&<button type="button" disabled={locked} onClick={cancelForm}>{t('cancel')}</button>}
    </div>
   </form>}
   {!!listedRows.length&&<div className="workbuddy-account-groups" aria-label={t('accounts')}>
    <p className="workbuddy-page-scope">{t('records_scope')}</p>
    {currentRows.length>0&&<section className="workbuddy-account-group" aria-label={t('current_accounts')}><h3>{t('current_accounts')}</h3><div className="workbuddy-list">{currentRows.map(renderConnection)}</div></section>}
    {disconnectedRows.length>0&&<section className="workbuddy-account-group workbuddy-history" aria-label={t('disconnected_accounts')}>
     <h3><button type="button" className="workbuddy-history-toggle" aria-expanded={historyExpanded} aria-controls={historyID} onClick={()=>setHistoryDisclosure(!historyExpanded)}><ChevronRight size={18} aria-hidden="true"/><span>{t('disconnected_read_count',{count:disconnectedRows.length})}</span><span className="workbuddy-history-action">{t(historyExpanded?'collapse_history':'expand_history')}</span></button></h3>
     <div id={historyID} className="workbuddy-history-content" hidden={!historyExpanded}><p>{t('retained_history')}</p><div className="workbuddy-list">{disconnectedRows.map(renderConnection)}</div></div>
    </section>}
   </div>}
   {!rows.length&&page.configured&&!firstConnection&&!authorizationVisible&&!desktopUnavailable&&<Empty title={t('empty')} description={t(page.can_authorize?'empty_hint':'read_only_hint')}/>}
   {(history.length>0||page.next_cursor)&&<nav className="workbuddy-pagination" aria-label={t('pagination')}><button type="button" disabled={!history.length||locked||!!form} onClick={()=>{setAuthorization(undefined);setReturnConnectionID('');setCursor(history[history.length-1]);setHistory(values=>values.slice(0,-1));}}>{t('previous')}</button><span>{t('page',{page:history.length+1})}</span><button type="button" disabled={!page.next_cursor||locked||!!form} onClick={()=>{setAuthorization(undefined);setReturnConnectionID('');setHistory(values=>[...values,cursor]);setCursor(page.next_cursor);}}>{t('next')}</button></nav>}
  </>}
 </div>;

 function renderConnection(row:WorkBuddyConnection){
  const reason=workBuddyConnectionReason(row);
  const disabled=locked||!!form;
  const canCheck=['authorizing','refreshing','authorization_unknown','refresh_unknown'].includes(row.state);
  const expired=row.state==='connected'&&Date.parse(row.expires_at)<=Date.now();
  const authorize=!desktopUnavailable&&row.allowed_actions.includes('authorize')&&
   <button key="authorize" type="button" disabled={disabled} onClick={()=>openForm({kind:'authorize',connection:row})}>{t(row.state==='authorizing'?'replace_link':'reauthorize')}</button>;
  const [primary,...additional]=[
   row.state==='authorizing'&&authorize,
   canCheck&&<button key="check" type="button" disabled={disabled} onClick={()=>checkCurrent(row,true)}><Search size={16} aria-hidden="true"/>{t('check_connection')}</button>,
   row.allowed_actions.includes('refresh')&&<button key="refresh" type="button" disabled={disabled} onClick={()=>{setAuthorization(undefined);void write('/workbuddy-connections/'+encodeURIComponent(row.id)+'/refresh',{project_id:project,expected_version:row.version});}}>{t('refresh_authorization')}</button>,
   row.state!=='authorizing'&&authorize,
   row.allowed_actions.includes('disconnect')&&<button key="disconnect" type="button" disabled={disabled} onClick={()=>openForm({kind:'disconnect',connection:row})}>{t('disconnect')}</button>,
  ].filter(Boolean);
  return <article key={row.id} data-workbuddy-connection-id={row.id} className={"workbuddy-card"+(row.id===detailID?" workbuddy-card-target":"")}>
   <header><div><span className="workbuddy-fact-label">{t('account')}</span><h4>{row.open_id||t('account_pending')}</h4></div><span className={'workbuddy-state workbuddy-state-'+row.state}>{t('state.'+row.state)}</span></header>
   <dl className="workbuddy-facts"><div><dt>{t('permissions')}</dt><dd>{row.scopes.length?row.scopes.map(scopeLabel).join(' · '):t('permissions_pending')}</dd></div><div><dt>{t('expires')}</dt><dd>{row.expires_at?<time dateTime={row.expires_at}>{time(row.expires_at)}</time>:time('')}{expired&&<span className="workbuddy-expired">{t('access_expired')}</span>}</dd></div><div><dt>{t('updated')}</dt><dd><time dateTime={row.updated_at}>{time(row.updated_at)}</time></dd></div></dl>
   {reason&&<p className="workbuddy-connection-reason">{t('reason.'+reason)}</p>}
   {primary&&<div className="workbuddy-card-primary">{primary}</div>}
   {additional.length>0&&<details className="workbuddy-card-management"><summary>{t('manage_connection')}</summary><div className="workbuddy-actions">{additional}</div></details>}
   <details><summary>{t('technical')}</summary><dl className="workbuddy-technical"><div><dt>{t('connection_id')}</dt><dd><code>{row.id}</code></dd></div><div><dt>{t('version')}</dt><dd>{row.version}</dd></div><div><dt>{t('grant_generation')}</dt><dd>{row.grant_generation}</dd></div><div><dt>{t('token_generation')}</dt><dd>{row.token_generation}</dd></div></dl></details>
  </article>;
 }
}

function connectionWriteNotice(state:WorkBuddyConnection['state']|undefined,disconnect:boolean){
 if(disconnect)return state==='disconnected'?'disconnected_notice':'status_checked';
 switch(state){
  case 'connected':return 'refreshed_notice';
  case 'refreshing':return 'refresh_pending_notice';
  case 'refresh_unknown':return 'refresh_unknown_notice';
  case 'reauthorization_required':return 'reason.reauthorization_required';
  default:return 'status_checked';
 }
}

function PermissionList({scopes}:{scopes:readonly string[]}){
 return scopes.length?<ul className="workbuddy-scope-list">{[...new Set(scopes)].map(scope=><li key={scope}>{scopeLabel(scope)}</li>)}</ul>:<p className="muted">{t('no_permissions')}</p>;
}

function PermissionChanges({current,requested,available}:{current:readonly string[];requested:readonly string[];available:readonly string[]}){
 const changes=workBuddyScopeDifference(current,requested,available);
 return <div className="workbuddy-scope-review" role="group" aria-label={t('permission_changes')}>
  <h5>{t('permission_changes')}</h5><p className="muted">{t('permission_changes_hint')}</p>
  <div className="workbuddy-scope-diff" aria-live="polite">{(['added','removed','retained'] as const).map(kind=><section key={kind} aria-label={t('permissions_'+kind)}><h6>{t('permissions_'+kind)}</h6><PermissionList scopes={changes[kind]}/></section>)}</div>
  {changes.unavailable.length>0&&<div className="workbuddy-scope-unavailable"><h6>{t('permissions_unavailable')}</h6><PermissionList scopes={changes.unavailable}/><p>{t('permissions_unavailable_hint')}</p></div>}
 </div>;
}

/** When a native authorization capability is unavailable, share only the platform
 * address; the browser starts with its own sign-in, never a provider URL/token. */
function DesktopAuthorizationGuide({project}:{project:string}){
 const id=useId(),address=['http:','https:'].includes(location.protocol)?location.origin+'/?'+new URLSearchParams({project,view:'settings',sub:'tools'}):'',fallback=useRef<HTMLTextAreaElement>(null),alive=useRef(true),[feedback,setFeedback]=useState<'copied'|'manual'>();
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 async function copy(){
  if(!address)return;
  try{await navigator.clipboard.writeText(address);if(alive.current)setFeedback('copied');}
  catch{if(alive.current){setFeedback('manual');fallback.current?.focus();fallback.current?.select();}}
 }
 return <div className="workbuddy-desktop-guide"><h4>{t('desktop_title')}</h4><p>{t('desktop_hint')}</p>{address?<><label htmlFor={id}>{t('desktop_address')}<textarea id={id} ref={fallback} data-workbuddy-browser-address readOnly value={address} rows={3} spellCheck={false} onFocus={event=>event.currentTarget.select()}/></label><button type="button" data-workbuddy-browser-copy onClick={()=>void copy()}>{feedback==='copied'?<Check size={16} aria-hidden="true"/>:<Copy size={16} aria-hidden="true"/>}{t(feedback==='copied'?'desktop_copied':'desktop_copy')}</button>{feedback==='manual'&&<p role="alert">{t('desktop_manual')}</p>}</>:<p>{t('desktop_unavailable')}</p>}</div>;
}
