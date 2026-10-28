import {useEffect,useRef,useState,useSyncExternalStore} from 'react';
import {FolderOpen,Link,RefreshCw} from 'lucide-react';
import type {WorkspaceMetadata} from '../../../packages/contracts/src';
import {useApi,useApiView} from './api-context';
import {ApiError} from './api';
import {tr as t,useLocale} from './i18n';
import {useLeaveGuard} from './write-action';
import {beginDesktopBinding,consumeDesktopBinding,desktopBindingSlot,desktopBindingsRevision,desktopScopeKey,desktopWorkspaceBridge,runDesktopBinding,selectedDesktopDirectory,subscribeDesktopBindings,validDesktopScope,validWorkspaceMetadata,type DesktopBindingSlot} from './desktop-workspace-model';
import './DesktopWorkspacePicker.css';

export type DesktopWorkspacePickerProps={project:string;metadata:WorkspaceMetadata;active:boolean;disabled:boolean;onBound:(metadata:WorkspaceMetadata)=>void;onRefresh:()=>Promise<WorkspaceMetadata|undefined>;onBlockedChange?:(blocked:boolean)=>void};
export function DesktopWorkspacePicker({project,metadata,active,disabled,onBound,onRefresh,onBlockedChange}:DesktopWorkspacePickerProps){
 const api=useApi(),view=useApiView(),locale=useLocale(),bridge=desktopWorkspaceBridge();
 const scope={userID:view?.userID??'',organizationID:view?.organizationID??'',project},key=desktopScopeKey(scope);
 const [selection,setSelection]=useState<{key:string;api:typeof api;expected:string;path:string;grant:NonNullable<ReturnType<typeof selectedDesktopDirectory>>['grant']}|undefined>();
 const [status,setStatus]=useState<{key:string;message:string;conflict?:boolean}>(),[choosing,setChoosing]=useState(false),[refreshing,setRefreshing]=useState(false);
 const intent=useRef({api,key,active,epoch:0});
 if(intent.current.api!==api||intent.current.key!==key||intent.current.active!==active)intent.current={api,key,active,epoch:intent.current.epoch+1};
 const mounted=useRef(true);useEffect(()=>{mounted.current=true;return()=>{mounted.current=false;intent.current.epoch++;};},[]);
 useSyncExternalStore(subscribeDesktopBindings,desktopBindingsRevision,desktopBindingsRevision);
 const slot=desktopBindingSlot(scope),choice=selection?.key===key&&selection.api===api?selection:undefined,currentStatus=status?.key===key?status:undefined;
 const operationConfirmed=slot?.outcome?.kind==='acknowledged';
 const available=!!bridge&&validDesktopScope(scope)&&metadata.project_id===project&&metadata.desktop_binding_available===true;
 const blocked=!!slot||refreshing,visible=available||!!slot;
 useLeaveGuard(blocked,()=>setStatus({key,message:'preferences.write.guard'}));
 useEffect(()=>{onBlockedChange?.(blocked);return()=>onBlockedChange?.(false);},[blocked,onBlockedChange]);
 useEffect(()=>{setSelection(undefined);setStatus(undefined);setChoosing(false);setRefreshing(false);},[api,key]);
 useEffect(()=>{if(!active)setChoosing(false);},[active]);
 const live=(epoch:number)=>mounted.current&&intent.current.epoch===epoch&&intent.current.active;
 async function choose(){
  if(!bridge||!active||disabled||slot||choosing||refreshing||!validWorkspaceMetadata(metadata,project))return;
  const epoch=intent.current.epoch,expected=metadata.binding_sha256;setChoosing(true);setSelection(undefined);setStatus(undefined);
  try{
   const result=await bridge.chooseProjectDirectory({project_id:project,user_id:scope.userID,tenant_id:scope.organizationID,locale});
   if(!live(epoch))return;
   if(result&&typeof result==='object'&&Object.keys(result).length===1&&(result as {status?:string}).status==='cancelled')return;
   const next=selectedDesktopDirectory(result,scope);
   if(!next){setStatus({key,message:'desktop-workspace.selection_failed'});return;}
   setSelection({key,api,expected,...next});
  }catch{if(live(epoch))setStatus({key,message:'desktop-workspace.selection_failed'});}
  finally{if(live(epoch))setChoosing(false);}
 }
 async function submit(original?:DesktopBindingSlot){
  if(!active||disabled||refreshing||choosing||slot?.busy)return;
  const epoch=intent.current.epoch;let pending=original;
  if(!pending){
   if(!choice||currentStatus?.conflict)return;
   if(!selectedDesktopDirectory({status:'selected',path:choice.path,grant:choice.grant},scope)){setSelection(undefined);setStatus({key,message:'desktop-workspace.expired'});return;}
   try{pending=beginDesktopBinding(scope,api,choice.grant,choice.expected);}catch{setStatus({key,message:'desktop-workspace.selection_unavailable'});return;}
  }
  setStatus(undefined);const result=await runDesktopBinding(pending);
  if(!live(epoch))return;
  if(result.kind==='acknowledged'){
   // An immutable operation receipt describes its original commit. Another
   // authorized binding may already be current; only a fresh read owns the IDE.
   setSelection(undefined);setRefreshing(true);
   try{
    const current=await onRefresh();if(!live(epoch))return;
    if(!validWorkspaceMetadata(current,project))throw Error('unconfirmed current binding');
    consumeDesktopBinding(pending);
    setStatus({key,message:current.configured&&current.binding_sha256===result.metadata.binding_sha256?'desktop-workspace.bound':'desktop-workspace.current_changed'});
    onBound(current);
   }catch{if(live(epoch))setStatus({key,message:'desktop-workspace.refresh_failed'});}
   finally{if(live(epoch))setRefreshing(false);}
  }
  else if(result.kind==='failed'){
   consumeDesktopBinding(pending);
   const conflict=result.error instanceof ApiError&&result.error.status===409;
   const expired=result.error instanceof ApiError&&result.error.status===410;
   if(!conflict)setSelection(undefined);
   setStatus({key,conflict,message:conflict?'desktop-workspace.conflict':expired?'desktop-workspace.expired':'desktop-workspace.binding_failed'});
  }
 }
 async function refresh(){
  if(!active||disabled||slot||refreshing)return;const epoch=intent.current.epoch;setRefreshing(true);
  try{const next=await onRefresh();if(live(epoch)){if(!validWorkspaceMetadata(next,project))throw Error('unconfirmed');setSelection(undefined);setStatus({key,message:'desktop-workspace.refreshed'});}}
  catch{if(live(epoch))setStatus({key,message:'desktop-workspace.refresh_failed',conflict:true});}
  finally{if(live(epoch))setRefreshing(false);}
 }
 if(!visible)return null;
 return <section className="desktop-workspace" aria-label={t('desktop-workspace.title')} aria-busy={!!slot?.busy||choosing||refreshing}>
  <div className="desktop-workspace-heading"><FolderOpen size={18}/><div><strong>{t('desktop-workspace.title')}</strong><p>{t(metadata.configured?'desktop-workspace.replace_help':'desktop-workspace.help')}</p></div></div>
  {choice&&!slot&&<div className="desktop-workspace-selection"><span>{t('desktop-workspace.selected_folder')}</span><code>{choice.path}</code><small>{t('desktop-workspace.project')}</small></div>}
  {slot&&<p role="status" className="warning">{t(slot.busy?'desktop-workspace.submitting':operationConfirmed?'desktop-workspace.operation_confirmed':'desktop-workspace.unknown')}</p>}
  {currentStatus&&<p role="status">{t(currentStatus.message)}</p>}
  {disabled&&<p className="desktop-workspace-hint">{t('desktop-workspace.disabled')}</p>}
  <div className="desktop-workspace-actions">
   {slot?<button type="button" className="primary" disabled={!active||disabled||slot.busy||refreshing} onClick={()=>void submit(slot)}><RefreshCw size={14}/>{t(slot.busy?'desktop-workspace.submitting':operationConfirmed?'desktop-workspace.refresh_current':'desktop-workspace.reconcile')}</button>:<>
    <button type="button" disabled={!active||disabled||choosing||refreshing||!!currentStatus?.conflict||!available} onClick={()=>void choose()}><FolderOpen size={14}/>{t(choosing?'desktop-workspace.choosing':'desktop-workspace.choose')}</button>
    {choice&&<button type="button" className="primary" disabled={!active||disabled||choosing||refreshing||!!currentStatus?.conflict} onClick={()=>void submit()}><Link size={14}/>{t('desktop-workspace.bind')}</button>}
    {currentStatus?.conflict&&<button type="button" disabled={!active||disabled||refreshing} onClick={()=>void refresh()}><RefreshCw size={14}/>{t('desktop-workspace.refresh')}</button>}
   </>}
  </div>
 </section>;
}
