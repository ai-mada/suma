import {useEffect,useId,useRef,useState} from 'react';
import {Check,ChevronRight,Copy,ExternalLink,RefreshCw,ShieldCheck} from 'lucide-react';
import type {WorkBuddyAuthorization,WorkBuddyConnection,WorkBuddyConnectionPage} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,tr,useLocale,UiError} from './i18n';
import {Empty,ErrorNotice,Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {desktopWorkspaceBridge} from './desktop-workspace-model';
import {useLeaveGuard,useWriteAction,WriteNotice} from './write-action';
import {readWorkBuddyAuthorization,readWorkBuddyConnection,readWorkBuddyPage,validWorkBuddyReceipt,validWorkBuddyRecovery,workBuddyPageSize} from './workbuddy-connections-model';
import './WorkBuddyConnections.css';

const t=(key:string,params?:Record<string,unknown>)=>tr('workbuddy-connections.'+key,params);
const scopeLabel=(scope:string)=>{const key='workbuddy-connections.scope.'+scope;return tr(key)===key?t('scope_unknown',{scope}):tr(key);};
const time=(value:string)=>value?formatDateTime(value):t('no_expiry');
const canLeave=()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));

/** A clean callback locator is a connection ID only, never OAuth callback data. */
export function WorkBuddyConnections({project,panel}:{project:string;panel?:string}){
 useLocale();const view=useApiView(),context=JSON.stringify([view?.organizationID,view?.userID,project]),focusedID=panel?.match(/^workbuddy:([A-Za-z0-9][A-Za-z0-9_.-]{0,199})$/)?.[1]||'',disclosureKey=JSON.stringify([context,focusedID]),[disclosure,setDisclosure]=useState<{key:string;open:boolean}>(),id=useId(),open=disclosure?.key===disclosureKey?disclosure.open:!!focusedID;
 if(!project)return null;
 return <section className="panel workbuddy-connections" aria-label={t('title')}>
  <button className="workbuddy-disclosure" type="button" aria-expanded={open} aria-controls={id} onClick={()=>{if(!canLeave())return;setDisclosure({key:disclosureKey,open:!open});}}><ChevronRight size={20} aria-hidden="true"/><span><strong>{t('title')}</strong><small>{t('intro')}</small></span><span className="workbuddy-disclosure-action">{t(open?'collapse':'expand')}</span></button>
  {open&&<div id={id}><ConnectionManager key={disclosureKey} project={project} context={context} focusedID={focusedID}/></div>}
 </section>;
}
type Snapshot={caller:ApiCaller;revision:number;cursor:string;page?:WorkBuddyConnectionPage;focused?:WorkBuddyConnection;error?:unknown;loading:boolean};
type Form={kind:'authorize';connection?:WorkBuddyConnection}|{kind:'disconnect';connection:WorkBuddyConnection};
type NativeAuthorizationBridge={openWorkBuddyAuthorization:(request:{project_id:string;authorization_id:string;user_id:string;tenant_id:string})=>Promise<unknown>};
function ConnectionManager({project,context,focusedID}:{project:string;context:string;focusedID:string}){
 useLocale();const caller=useApi(),view=useApiView(),owner=view?.userID,scope='workbuddy-connections:'+context,desktop=desktopWorkspaceBridge(),native=desktop&&typeof(desktop as unknown as Partial<NativeAuthorizationBridge>).openWorkBuddyAuthorization==='function'?desktop as unknown as NativeAuthorizationBridge:undefined,desktopUnavailable=!!desktop&&!native;
 const [snapshot,setSnapshot]=useState<Snapshot>(),[cursor,setCursor]=useState(''),[history,setHistory]=useState<string[]>([]),[revision,setRevision]=useState(0),[form,setForm]=useState<Form>(),[selectedScopes,setSelectedScopes]=useState<string[]>([]),[confirmed,setConfirmed]=useState(false),[authorization,setAuthorization]=useState<WorkBuddyAuthorization>(),[notice,setNotice]=useState('');
 const [nativeBusy,setNativeBusy]=useState(false),nativeAttempt=useRef(0),nativePending=useRef(false),checkingCurrent=useRef(false);
 const epoch=useRef(0),alive=useRef(true),activeCaller=useRef(caller);activeCaller.current=caller;
 const action=useWriteAction({scope,storage:toolSessionStorage(),validateRequest:request=>validWorkBuddyRecovery(request,project),validateResponse:(value,request)=>validWorkBuddyReceipt(value,request,project,owner)});
 const locked=action.busy||!!action.pending||action.persistenceBlocked||nativeBusy;
 const current=snapshot?.caller===caller&&snapshot.revision===revision&&snapshot.cursor===cursor?snapshot:undefined;
 const denied=action.errorStatus===403||action.errorStatus===404||current?.error instanceof ApiError&&[403,404].includes(current.error.status);
 const page=!denied&&!current?.loading?current?.page:undefined;
 const rows=page?(current?.focused?[current.focused,...page.items.filter(row=>row.id!==current.focused!.id)]:page.items):[];
 const mayRecover=!!page&&(page.can_authorize||rows.some(row=>row.allowed_actions.length>0));
 const formAllowed=!!page&&!!form&&(form.kind==='authorize'?!desktopUnavailable&&(form.connection?!!rows.find(row=>row.id===form.connection?.id)?.allowed_actions.includes('authorize'):page.can_authorize):!!rows.find(row=>row.id===form.connection.id)?.allowed_actions.includes('disconnect'));
 useLeaveGuard(!!form&&(selectedScopes.length>0||confirmed),action.setError);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;++nativeAttempt.current;};},[]);
 useEffect(()=>{++nativeAttempt.current;nativePending.current=false;setNativeBusy(false);},[caller,authorization?.id,authorization?.version]);
 useEffect(()=>{if(denied){setAuthorization(undefined);setForm(undefined);setSelectedScopes([]);setConfirmed(false);setNotice('');setSnapshot(before=>before?.page?{...before,page:undefined,focused:undefined}:before);}},[denied]);
 useEffect(()=>{if(page&&form&&!formAllowed){setForm(undefined);setSelectedScopes([]);setConfirmed(false);}},[page,form,formAllowed]);
 useEffect(()=>{
  const generation=++epoch.current,controller=new AbortController();setSnapshot({caller,revision,cursor,loading:true});
  let refusal:ApiError|undefined;
  const read=(path:string)=>caller<unknown>(path,undefined,'GET',undefined,controller.signal).catch(error=>{
   // Each request owns its refusal: an earlier network failure from the other
   // read must never conceal a later authorization loss or preserve its URL.
   if(generation===epoch.current&&alive.current&&activeCaller.current===caller&&!controller.signal.aborted&&error instanceof ApiError&&[403,404].includes(error.status)){
    refusal=error;setSnapshot({caller,revision,cursor,loading:false,error});setAuthorization(undefined);setForm(undefined);setSelectedScopes([]);setConfirmed(false);
   }
   throw error;
  });
  const list=read('/workbuddy-connections?project_id='+encodeURIComponent(project)+'&limit='+workBuddyPageSize+(cursor?'&cursor='+encodeURIComponent(cursor):''));
  const detail=focusedID?read('/workbuddy-connections/'+encodeURIComponent(focusedID)+'?project_id='+encodeURIComponent(project)):Promise.resolve(undefined);
  void Promise.all([list,detail]).then(([value,focusedValue])=>{
   if(generation!==epoch.current||!alive.current||activeCaller.current!==caller)return;
   const result=readWorkBuddyPage(value,project,owner),focused=focusedID?readWorkBuddyConnection(focusedValue,project,owner):undefined;
   if(!result||focusedID&&(!focused||focused.id!==focusedID))throw new UiError('workbuddy-connections.invalid_response');
   setSnapshot({caller,revision,cursor,loading:false,page:result,focused});
  }).catch(error=>{if(generation!==epoch.current||controller.signal.aborted||!alive.current)return;const failure=refusal??error;setSnapshot({caller,revision,cursor,loading:false,error:failure});if(failure instanceof ApiError&&[403,404].includes(failure.status)){setAuthorization(undefined);setForm(undefined);setSelectedScopes([]);setConfirmed(false);}});
  return()=>{++epoch.current;controller.abort();};
 },[caller,project,owner,cursor,revision,focusedID]);
 useEffect(()=>{if(!authorization)return;const remaining=Date.parse(authorization.expires_at)-Date.now();if(remaining<=0){setAuthorization(undefined);setNotice('link_expired');return;}const timer=setTimeout(()=>{setAuthorization(undefined);setNotice('link_expired');},Math.min(remaining,2147483647));return()=>clearTimeout(timer);},[authorization]);
 useEffect(()=>{if(!authorization||!page)return;const row=current?.focused?.id===authorization.connection.id?current.focused:page.items.find(item=>item.id===authorization.connection.id);if(!page.can_authorize||row&&(row.state!=='authorizing'||row.version!==authorization.connection.version))setAuthorization(undefined);},[page,current?.focused,authorization]);
 const refresh=()=>{action.setError('');action.clearConflict();setRevision(value=>value+1);};
 function openForm(next:Form){if(locked||desktopUnavailable&&next.kind==='authorize'||!canLeave())return;setForm(next);setSelectedScopes([]);setConfirmed(false);setAuthorization(undefined);setNotice('');action.clearConflict();}
 function cancelForm(){if(locked)return;setForm(undefined);setSelectedScopes([]);setConfirmed(false);action.clearConflict();}
 async function write(path:string,body:Record<string,unknown>,method='POST'){
  if(desktopUnavailable&&path==='/workbuddy-authorizations')return;
  const result=await action.run<unknown>(path,body,method);if(!alive.current||activeCaller.current!==caller||!result)return;
  setForm(undefined);setSelectedScopes([]);setConfirmed(false);
  if(path==='/workbuddy-authorizations'){
   const receipt=readWorkBuddyAuthorization(result,project,owner);setAuthorization(receipt);setNotice(receipt?.authorization_url?'':'authorization_recorded');
   // Preserve the returned URL in memory while the bounded list is rechecked.
   setRevision(value=>value+1);
  }else{setAuthorization(undefined);setNotice(path.endsWith('/disconnect')?'disconnected_notice':'refreshed_notice');setRevision(value=>value+1);}
 }
 async function checkCurrent(){
  if(checkingCurrent.current)return;
  const request=action.pending,originalAuthorization=authorization,id=request?request.path==='/workbuddy-authorizations'?request.body.connection_id:request.path.split('/')[2]:originalAuthorization?.connection.id;
  if(typeof id!=='string'){refresh();return;}
  checkingCurrent.current=true;
  const generation=++epoch.current;setSnapshot({caller,revision,cursor,loading:true});
  try{
   const connectionID=decodeURIComponent(id),value=await caller<unknown>('/workbuddy-connections/'+encodeURIComponent(connectionID)+'?project_id='+encodeURIComponent(project));
   if(!alive.current||generation!==epoch.current||activeCaller.current!==caller)return;
   const row=readWorkBuddyConnection(value,project,owner);if(!row||row.id!==connectionID)throw new UiError('workbuddy-connections.invalid_response');
   if(originalAuthorization){
    if(row.state!=='authorizing'||row.version!==originalAuthorization.connection.version)setAuthorization(undefined);
    setNotice(row.state==='connected'?'authorization_complete':row.state==='authorizing'?'authorization_pending':'authorization_changed');
   }else setNotice('status_checked');
   setRevision(value=>value+1);
  }catch(error){if(alive.current&&generation===epoch.current&&activeCaller.current===caller){setSnapshot({caller,revision,cursor,loading:false,error});if(error instanceof ApiError&&[403,404].includes(error.status))setAuthorization(undefined);}}
  finally{checkingCurrent.current=false;}
 }
 useEffect(()=>{
  if(!native||!authorization||!page||denied||locked||form)return;
  const onFocus=()=>{if(!nativePending.current)void checkCurrent();};
  window.addEventListener('focus',onFocus);return()=>window.removeEventListener('focus',onFocus);
 });
 async function openNativeAuthorization(){
  if(!native||!authorization||!page?.can_authorize||denied||locked||form||nativePending.current||!view?.userID||!view.organizationID)return;
  const sequence=++nativeAttempt.current;nativePending.current=true;setNativeBusy(true);setNotice('');
  try{
   const result=await native.openWorkBuddyAuthorization({project_id:project,authorization_id:authorization.id,user_id:view.userID,tenant_id:view.organizationID});
   if(!alive.current||activeCaller.current!==caller||sequence!==nativeAttempt.current)return;
   if(result&&typeof result==='object'&&!Array.isArray(result)&&'status' in result&&result.status==='opened'&&Object.keys(result).length===1)setNotice('desktop_opened');
   else setNotice(result&&typeof result==='object'&&'code' in result&&result.code==='oauth_window_closed'?'desktop_closed':'desktop_failed');
  }catch{if(alive.current&&activeCaller.current===caller&&sequence===nativeAttempt.current)setNotice('desktop_failed');}
  finally{if(sequence===nativeAttempt.current){nativePending.current=false;if(alive.current&&activeCaller.current===caller)setNativeBusy(false);}}
 }
 const authorizationVisible=!desktopUnavailable&&page?.can_authorize&&authorization?.authorization_url&&!denied;
 return <div className="workbuddy-manager">
  <div className="workbuddy-toolbar"><p className="workbuddy-boundary"><ShieldCheck size={17} aria-hidden="true"/>{t('boundary')}</p><button type="button" disabled={current?.loading||action.busy||nativeBusy||!!form} onClick={refresh}><RefreshCw size={16} aria-hidden="true"/>{t('refresh_list')}</button></div>
  {desktopUnavailable&&<DesktopAuthorizationGuide project={project}/>}
  <WriteNotice action={action} onRebase={async()=>{cancelForm();refresh();}}/>
  {action.pending&&<div className="workbuddy-recovery" role="status"><p>{t(action.unknown?'unknown':'pending')}</p><div className="workbuddy-actions"><button type="button" disabled={action.busy||denied} onClick={()=>void checkCurrent()}>{t('check_current')}</button>{mayRecover&&(!desktopUnavailable||action.pending.path!=='/workbuddy-authorizations')&&<button type="button" disabled={action.busy} onClick={()=>{const request=action.pending!;void write(request.path,request.body,request.method);}}>{t('recover')}</button>}</div></div>}
  {notice&&!denied&&<p className="workbuddy-notice" role="status">{t(notice)}</p>}
  {authorizationVisible&&<div className="workbuddy-authorization" role="status"><h4>{t('ready')}</h4><p>{t('ready_hint')}</p>{native?<button type="button" className="primary workbuddy-official-link" disabled={locked} onClick={()=>void openNativeAuthorization()}>{t(nativeBusy?'desktop_opening':'desktop_open')}<ExternalLink size={16} aria-hidden="true"/></button>:<a className="primary workbuddy-official-link" href={authorization.authorization_url} target="_blank" rel="noopener noreferrer">{t('official_link')}<ExternalLink size={16} aria-hidden="true"/></a>}<small>{t('link_expiry',{time:time(authorization.expires_at)})}</small><button type="button" disabled={nativeBusy||current?.loading} onClick={()=>void checkCurrent()}>{t('check_authorization')}</button></div>}
  {denied?<><p className="muted">{t('access_lost')}</p>{current?.error&&<ErrorNotice error={current.error}/>}</>:!current||current.loading?<Loading/>:current.error?<ErrorNotice error={current.error} onRetry={refresh}/>:page&&<>
   {!page.configured&&<p className="workbuddy-config" role="status">{t('not_configured')}</p>}
   {!desktopUnavailable&&page.can_authorize&&<button type="button" className="primary" disabled={locked||!!form} onClick={()=>openForm({kind:'authorize'})}>{t('connect')}</button>}
   {form&&formAllowed&&<form className="workbuddy-form" onSubmit={event=>{event.preventDefault();if(locked||action.conflict)return;const connection=form.connection;if(form.kind==='authorize'){if(!selectedScopes.length)return;void write('/workbuddy-authorizations',{project_id:project,scopes:selectedScopes,...(connection?{connection_id:connection.id,expected_version:connection.version}:{})});}else{if(!confirmed)return;void write('/workbuddy-connections/'+encodeURIComponent(connection!.id)+'/disconnect',{project_id:project,expected_version:connection!.version,reason:'user_requested_local_disconnect'});}}}>
    <h4>{t(form.kind==='authorize'?'choose_scopes':'disconnect_title')}</h4>
    {form.connection&&<p>{t('account')}: <code>{form.connection.open_id||t('account_pending')}</code></p>}
    <fieldset disabled={locked||action.conflict}><legend>{t(form.kind==='authorize'?'permissions':'confirmation')}</legend>{form.kind==='authorize'?<><p className="muted">{t('scopes_hint')}</p>{page.available_scopes.map(item=><label className="workbuddy-check" key={item}><input type="checkbox" value={item} checked={selectedScopes.includes(item)} onChange={event=>setSelectedScopes(values=>event.target.checked?[...values,item]:values.filter(value=>value!==item))}/><span>{scopeLabel(item)}</span></label>)}</>:<><p className="workbuddy-local-warning">{t('disconnect_warning')}</p><label className="workbuddy-check"><input name="confirm-disconnect" type="checkbox" checked={confirmed} onChange={event=>setConfirmed(event.target.checked)}/><span>{t('disconnect_confirm')}</span></label></>}</fieldset>
    <div className="workbuddy-actions"><button type="button" disabled={locked} onClick={cancelForm}>{t('cancel')}</button><button className="primary" type="submit" disabled={locked||action.conflict||(form.kind==='authorize'?!selectedScopes.length:!confirmed)}>{t(form.kind==='authorize'?'prepare':'disconnect_submit')}</button></div>
   </form>}
   <div className="workbuddy-list" aria-label={t('accounts')}>{rows.map(row=><article key={row.id} data-workbuddy-connection-id={row.id} className={"workbuddy-card"+(row.id===focusedID?" workbuddy-card-target":"")}><header><div><span className="workbuddy-fact-label">{t('account')}</span><h4>{row.open_id||t('account_pending')}</h4></div><span className={'workbuddy-state workbuddy-state-'+row.state}>{t('state.'+row.state)}</span></header><dl className="workbuddy-facts"><div><dt>{t('permissions')}</dt><dd>{row.scopes.length?row.scopes.map(scopeLabel).join(' · '):t('permissions_pending')}</dd></div><div><dt>{t('expires')}</dt><dd>{time(row.expires_at)}</dd></div></dl><div className="workbuddy-actions">{!desktopUnavailable&&row.allowed_actions.includes('authorize')&&<button type="button" disabled={locked||!!form} onClick={()=>openForm({kind:'authorize',connection:row})}>{t('reauthorize')}</button>}{row.allowed_actions.includes('refresh')&&<button type="button" disabled={locked||!!form} onClick={()=>{setAuthorization(undefined);void write('/workbuddy-connections/'+encodeURIComponent(row.id)+'/refresh',{project_id:project,expected_version:row.version});}}>{t('refresh_authorization')}</button>}{row.allowed_actions.includes('disconnect')&&<button type="button" disabled={locked||!!form} onClick={()=>openForm({kind:'disconnect',connection:row})}>{t('disconnect')}</button>}</div><details><summary>{t('technical')}</summary><dl className="workbuddy-technical"><div><dt>{t('connection_id')}</dt><dd><code>{row.id}</code></dd></div><div><dt>{t('version')}</dt><dd>{row.version}</dd></div><div><dt>{t('grant_generation')}</dt><dd>{row.grant_generation}</dd></div><div><dt>{t('token_generation')}</dt><dd>{row.token_generation}</dd></div></dl></details></article>)}</div>
   {!rows.length&&<Empty title={t('empty')} description={t('empty_hint')}/>}
   {(history.length>0||page.next_cursor)&&<nav className="workbuddy-pagination" aria-label={t('pagination')}><button type="button" disabled={!history.length||locked||!!form} onClick={()=>{setAuthorization(undefined);setCursor(history[history.length-1]);setHistory(values=>values.slice(0,-1));}}>{t('previous')}</button><span>{t('page',{page:history.length+1})}</span><button type="button" disabled={!page.next_cursor||locked||!!form} onClick={()=>{setAuthorization(undefined);setHistory(values=>[...values,cursor]);setCursor(page.next_cursor);}}>{t('next')}</button></nav>}
  </>}
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
