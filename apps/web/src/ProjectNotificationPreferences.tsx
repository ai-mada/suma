import {useCallback,useEffect,useId,useLayoutEffect,useRef,useState} from 'react';
import {ChevronDown,RefreshCw,Settings2} from 'lucide-react';
import {ApiError} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {preferencesCommandPath,preferencesPath,readProjectNotificationPreferences,validPreferencesReceipt,validPreferencesRequest,validPreferencesScope,type PreferencesScope,type PreferencesValue,type PreferencesReceipt} from './project-notification-preferences-model';
import './ProjectNotificationPreferences.css';

export type ProjectNotificationPreferencesControlProps={project:string;userID:string;organizationID:string;disabled?:boolean;accessBlocked?:boolean;onAccessDenied?:(error:unknown)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('project-notification-preferences.'+key,values);
const refusal=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const maximumVersion=2147483647;

/** A new caller retires both reads and private drafts synchronously. Collapsing
 * the panel never remounts its writer or discards its durable original command. */
export function ProjectNotificationPreferencesControl(props:ProjectNotificationPreferencesControlProps){
 const api=useApi(),view=useApiView(),owner=useRef({api,epoch:0});
 if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope={project:props.project,userID:props.userID,organizationID:props.organizationID};
 const allowed=!!view&&view.userID===scope.userID&&view.organizationID===scope.organizationID&&validPreferencesScope(scope);
 return <PreferencesController {...props} key={JSON.stringify([scope,owner.current.epoch,allowed])} scope={scope} blocked={!allowed||!!props.accessBlocked}/>;
}

type Snapshot={value?:PreferencesValue;loading:boolean;error?:unknown;denied?:boolean};
function PreferencesController({scope,blocked,disabled=false,onAccessDenied}:ProjectNotificationPreferencesControlProps&{scope:PreferencesScope;blocked:boolean}){
 useLocale();const api=useApi(),panelID=useId(),hintID=useId();
 const [open,setOpen]=useState(false),[snapshot,setSnapshot]=useState<Snapshot>({loading:!blocked}),[draft,setDraft]=useState<boolean>(),[confirmedVersion,setConfirmedVersion]=useState<number>();
 const current=useRef<Snapshot>(snapshot),alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),blockedNow=useRef(blocked),notified=useRef(false),commandBusy=useRef(false),checkbox=useRef<HTMLInputElement>(null),notify=useRef(onAccessDenied);
 blockedNow.current=blocked;notify.current=onAccessDenied;
 const action=useWriteAction({scope:'project-notification-preferences:'+JSON.stringify([scope.userID,scope.organizationID,scope.project]),storage:toolSessionStorage(),validateRequest:request=>validPreferencesRequest(request,scope),validateResponse:(raw,request)=>validPreferencesReceipt(raw,request,scope)});
 const actionNow=useRef(action);actionNow.current=action;
 const store=(next:Snapshot)=>{current.current=next;setSnapshot(next);};
 const clearPrivate=useCallback((error?:unknown)=>{
  if(!alive.current)return;
  ++epoch.current;controller.current?.abort();current.current={loading:false,denied:true};setSnapshot(current.current);setDraft(undefined);setConfirmedVersion(undefined);
  if(error&&!notified.current){notified.current=true;notify.current?.(error);}
 },[]);
 const read=useCallback(async(clearPrevious=false):Promise<PreferencesValue|undefined>=>{
  if(blockedNow.current||!validPreferencesScope(scope))return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  const previous=clearPrevious||current.current.denied?undefined:current.current.value;
  current.current={value:previous,loading:true};setSnapshot(current.current);
  if(clearPrevious)setDraft(undefined);
  try{
   const raw=await api<unknown>(preferencesPath(scope),undefined,'GET',{'X-Suma-Expected-User':scope.userID,'X-Suma-Expected-Organization':scope.organizationID},abort.signal);
   if(!alive.current||blockedNow.current||abort.signal.aborted||generation!==epoch.current)return;
   const value=readProjectNotificationPreferences(raw,scope);if(!value)throw new UiError('project-notification-preferences.invalid_read');
   store({value,loading:false});notified.current=false;
   if(actionNow.current.errorStatus&&[401,403,404].includes(actionNow.current.errorStatus))actionNow.current.setError('');
   return value;
  }catch(error){
   // Even a refusal from a retired read must never clear a later scope.
   if(!alive.current||blockedNow.current||abort.signal.aborted||generation!==epoch.current)return;
   if(refusal(error)){clearPrivate(error);return;}
   store({value:previous,loading:false,error});
  }
 },[api,scope.project,scope.userID,scope.organizationID,clearPrivate]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++epoch.current;controller.current?.abort();};},[]);
 useLayoutEffect(()=>{if(blocked)clearPrivate();},[blocked,clearPrivate]);
 useEffect(()=>{if(!blocked)void read();return()=>{++epoch.current;controller.current?.abort();};},[blocked,read]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(writeDenied)clearPrivate(new ApiError(action.errorStatus!,'notification_preferences_access_changed',t('access_changed')));},[writeDenied,action.errorStatus,clearPrivate]);
 const denied=blocked||writeDenied||!!snapshot.denied,value=denied?undefined:snapshot.value,selected=!denied?(draft??value?.followed_discussions_muted):undefined;
 const dirty=!!value&&draft!==undefined&&draft!==value.followed_discussions_muted,locked=disabled||action.busy||!!action.pending||action.conflict||denied||!value||snapshot.loading||!!snapshot.error||value.version>=maximumVersion;
 useLayoutEffect(()=>{if(checkbox.current)checkbox.current.indeterminate=!value;},[value]);
 async function refresh(){
  if(blockedNow.current||action.busy)return;
  action.setError('');if(await read())action.clearConflict();
 }
 async function submit(recover=false){
  const original=action.pending;
  if(commandBusy.current||action.busy||disabled||blockedNow.current||denied||recover&&!original||!recover&&(locked||!dirty||!value||draft===undefined))return;
  commandBusy.current=true;setConfirmedVersion(undefined);
  try{
   const receipt=recover&&original
    ?await action.run<PreferencesReceipt>(original.path,original.body,original.method)
    :await action.run<PreferencesReceipt>(preferencesCommandPath(scope),{project_id:scope.project,expected_version:value!.version,followed_discussions_muted:draft!});
   if(!receipt||!alive.current||blockedNow.current)return;
   setConfirmedVersion(receipt.preferences.version);
   // This may be a historical replay: its preference is never the current read.
   await read(true);
  }finally{commandBusy.current=false;}
 }
 const state=!value?'unknown':value.followed_discussions_muted?'muted':'enabled';
 return <div className="project-notification-preferences">
  <div className="notification-preferences-entry"><button type="button" data-notification-preferences-toggle aria-expanded={open} aria-controls={panelID} onClick={()=>setOpen(value=>!value)}><Settings2 size={16} aria-hidden="true"/>{t('title')}<ChevronDown size={14} aria-hidden="true" className={open?'is-open':''}/></button><span data-notification-preferences-state data-state={state} aria-live="polite">{!denied&&snapshot.loading?t('loading'):t(state)}</span></div>
  <div id={panelID} className="notification-preferences-panel" data-notification-preferences-panel hidden={!open}>
   <div className="notification-preferences-description"><h3>{t('panel_title')}</h3><p>{t('scope_hint')}</p></div>
   <div className="notification-preferences-editor">
    <label className="notification-preferences-option"><input ref={checkbox} type="checkbox" data-notification-preferences-muted checked={selected??false} aria-checked={!value?'mixed':selected} aria-describedby={hintID} disabled={locked} onChange={event=>setDraft(event.target.checked)}/><span>{t('mute_label')}</span></label>
    <p id={hintID} className="notification-preferences-hint">{t('retained_hint')}</p>
    <div className="notification-preferences-actions"><button type="button" className="primary" data-notification-preferences-save disabled={locked||!dirty} onClick={()=>void submit()}>{t(action.busy?'saving':'save')}</button><button type="button" data-notification-preferences-refresh disabled={blocked||action.busy||snapshot.loading} onClick={()=>void refresh()}><RefreshCw size={14} aria-hidden="true"/>{t('refresh')}</button>{dirty&&!denied&&<span role="status">{t('draft')}</span>}</div>
    {value&&<p className="notification-preferences-hint">{value.version===0?t('default'):t('updated',{at:formatDateTime(value.updated_at,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})})}</p>}
    {value?.version===maximumVersion&&<p role="status">{t('version_limit')}</p>}
   </div>
  </div>
  {Boolean(denied||action.error||action.pending||snapshot.error||confirmedVersion!==undefined)&&<div className="notification-preferences-feedback">
   {denied?<p role="alert">{t('access_changed')}</p>:<>
    <WriteNotice action={action} onRebase={async()=>{if(!await read())throw new UiError('project-notification-preferences.refresh_failed');}}/>
    {Boolean(snapshot.error)&&<ErrorNotice error={snapshot.error}/>}
    {confirmedVersion!==undefined&&<p role="status">{t('saved',{version:confirmedVersion})}</p>}
   </>}
   {action.pending&&<div className="notification-preferences-recovery"><p role="status">{t('original_pending')}</p><button type="button" data-notification-preferences-recover disabled={disabled||action.busy||denied} onClick={()=>void submit(true)}>{t('recover')}</button></div>}
  </div>}
 </div>;
}
