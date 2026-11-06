import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import {Archive,ArchiveRestore,ArrowUpRight,Bell,Check,CircleAlert,Mail,MessageSquare,RefreshCw} from 'lucide-react';
import type {NotificationInboxBatchTarget,NotificationInboxItem} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {Badge,Empty,ErrorNotice,Loading} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {ProjectNotificationPreferencesControl} from './ProjectNotificationPreferences';
import {
 appendNotificationInboxPage,appendNotificationOrganizationInboxPage,inboxRows,notificationInboxBatchPath,notificationInboxCommandPath,
 notificationInboxFilters,notificationInboxPath,notificationInboxReceiptIsStale,notificationInboxScope,notificationInboxSelectionLimit,notifyOrganizationInboxChanged,
 notificationOrganizationInboxPath,notificationOrganizationInboxScope,validNotificationInboxFilter,
 validNotificationInboxPage,validNotificationInboxRecovery,validNotificationInboxResult,validNotificationOrganizationInboxPage,
 type InboxReadPage,type InboxWriteReceipt,type NotificationInboxAction,type NotificationInboxFilter,type NotificationInboxScopeMode,type NotificationToolInputTarget,
} from './notification-inbox-model';
import './NotificationInbox.css';

export type NotificationInboxProps={
 project:string;userID:string;organizationID:string;filter:NotificationInboxFilter;refresh:number;invalid?:boolean;
 scopeMode?:NotificationInboxScopeMode;
 onScopeMode?:(scope:NotificationInboxScopeMode)=>void;
 onFilter:(filter:NotificationInboxFilter)=>void;
 onTask:(taskID:string,projectID?:string)=>void;
 onMessage?:(taskID:string,messageID:string,projectID?:string)=>void;
 onReviewRequest?:(taskID:string,requestID:string,projectID?:string)=>void;
 onHandoff?:(taskID:string,handoffID:string,projectID?:string)=>void;
 onRun?:(taskID:string,runID:string,projectID?:string)=>void;
 onToolInput?:(taskID:string,target:NotificationToolInputTarget,projectID?:string)=>void;
};
const t=(key:string,values?:Record<string,unknown>)=>tr('notification-inbox.'+key,values);
const mt=(key:string,values?:Record<string,unknown>)=>tr('task-conversation-mentions.'+key,values);
const rt=(key:string,values?:Record<string,unknown>)=>tr('task-review-requests.'+key,values);
const st=(key:string,values?:Record<string,unknown>)=>tr('task-subscriptions.'+key,values);
const messageSource=(item:NotificationInboxItem)=>item.kind==='task_mentioned'||item.kind==='task_followed';
const accessLost=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type ReadState={key:string;api:ApiCaller;page?:InboxReadPage;loading:boolean;error?:unknown};
type InboxSelection={key:string;api:ApiCaller;targets:NotificationInboxBatchTarget[]};
const scopeKey=({scopeMode,userID,organizationID,project}:NotificationInboxProps)=>scopeMode==='organization'
 ?notificationOrganizationInboxScope(userID,organizationID)
 :notificationInboxScope(userID,organizationID,project);

/** Only an identity/project change remounts the writer. Filter and refresh
 * changes cannot discard or retarget an unacknowledged personal command. */
export function NotificationInbox(props:NotificationInboxProps){const scope=scopeKey(props);return <InboxScope key={scope} {...props} scope={scope}/>;}

function InboxScope({project,userID,organizationID,filter,refresh,scope,scopeMode='project',onScopeMode,onFilter,onTask,onMessage,onReviewRequest,onHandoff,onRun,onToolInput,invalid=false}:NotificationInboxProps&{scope:string}){
 useLocale();
 const api=useApi(),organization=scopeMode==='organization',projectScope=organization?undefined:project,key=JSON.stringify([filter,refresh,invalid]);
 const action=useWriteAction({
  scope,storage:toolSessionStorage(),allowLeaveWithPending:organization,
  validateRequest:request=>validNotificationInboxRecovery(request,projectScope),validateResponse:validNotificationInboxResult,
  definitiveRefusalOnRestore:(error,request)=>request.path===notificationInboxBatchPath&&
   (error.code==='version_conflict'||error.code==='idempotency_conflict'),
 });
 const [state,setState]=useState<ReadState>({key,api,loading:true}),[denial,setDenial]=useState<unknown>(),[notice,setNotice]=useState('');
 const [selection,setSelection]=useState<InboxSelection>();
 const batchWrite=useRef(false);
 const alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),currentKey=useRef(key),currentApi=useRef(api),denied=useRef(false),cached=useRef<{key:string;api:ApiCaller;page:InboxReadPage}|undefined>(undefined);
 currentKey.current=key;currentApi.current=api;
 const clearPrivate=useCallback((error:unknown)=>{
  if(!alive.current)return;denied.current=true;epoch.current++;controller.current?.abort();cached.current=undefined;setSelection(undefined);setNotice('');setDenial(error);setState({key:currentKey.current,api:currentApi.current,loading:false,error});
  notifyOrganizationInboxChanged(userID,organizationID);
 },[userID,organizationID]);
 const read=useCallback(async(more=false):Promise<InboxReadPage|undefined>=>{
  const previous=more&&cached.current?.key===key&&cached.current.api===api?cached.current.page:undefined,cursor=previous?.next_cursor;
  if(more&&!cursor)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  if(!more){cached.current=undefined;setSelection(undefined);}
  setState({key,api,page:previous,loading:true});
  try{
   if(invalid||!validNotificationInboxFilter(filter))throw new UiError('notification-inbox.invalid_filter');
   const path=projectScope===undefined?notificationOrganizationInboxPath(filter,cursor):notificationInboxPath(projectScope,filter,cursor);
   const value=await api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);
   if(!alive.current||abort.signal.aborted||generation!==epoch.current||currentKey.current!==key||currentApi.current!==api)return;
   let page:InboxReadPage|undefined;
   if(projectScope===undefined){
    if(!validNotificationOrganizationInboxPage(value,organizationID,userID,filter)||cursor&&value.next_cursor===cursor)throw new UiError('notification-inbox.invalid_read');
    if(previous&&'organization_id' in previous&&previous.scope_revision!==value.scope_revision)throw new UiError('notification-inbox.organization_scope_changed');
    page=previous&&'organization_id' in previous?appendNotificationOrganizationInboxPage(previous,value):value;
   }else{
    if(!validNotificationInboxPage(value,projectScope,userID,filter)||cursor&&value.next_cursor===cursor)throw new UiError('notification-inbox.invalid_read');
    page=previous&&'project_id' in previous?appendNotificationInboxPage(previous,value):value;
   }
   if(!page||previous&&page.items.length===previous.items.length&&page.items.length<page.total)throw new UiError('notification-inbox.page_changed');
   cached.current={key,api,page};denied.current=false;setDenial(undefined);setState({key,api,page,loading:false});return page;
  }catch(error){
   if(!alive.current)return;
   // An explicit refusal belongs to this mounted human/project, even if a
   // different filter started meanwhile. An old identity has already unmounted.
   if(accessLost(error)){clearPrivate(error);return;}
   if(abort.signal.aborted||generation!==epoch.current||currentKey.current!==key||currentApi.current!==api)return;
   const failure=projectScope===undefined&&cursor&&error instanceof ApiError&&error.status===409
    ?new UiError('notification-inbox.organization_scope_changed'):error;
   cached.current=undefined;setSelection(undefined);setState({key,api,loading:false,error:failure});
  }finally{
   if(controller.current===abort)controller.current=undefined;
  }
 },[api,key,projectScope,userID,organizationID,filter,invalid,clearPrivate]);
 const latestRead=useRef(read);latestRead.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;controller.current?.abort();cached.current=undefined;};},[]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();};},[read]);
 useEffect(()=>{
  if(!organization||invalid)return;
  let lastCheck=-Infinity;
  const recheck=()=>{
   if(document.visibilityState==='hidden'||navigator.onLine===false||controller.current||Date.now()-lastCheck<3000)return;
   lastCheck=Date.now();
   void latestRead.current();
  };
  window.addEventListener('focus',recheck);
  window.addEventListener('online',recheck);
  document.addEventListener('visibilitychange',recheck);
  return ()=>{
   window.removeEventListener('focus',recheck);
   window.removeEventListener('online',recheck);
   document.removeEventListener('visibilitychange',recheck);
  };
 },[organization,invalid]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 const {setError:setActionError}=action;
 useLayoutEffect(()=>{
  if(!writeDenied)return;
  clearPrivate(new ApiError(action.errorStatus!,'notification_access_changed',t('access_changed')));
  if(organization){
   // A revoked source project does not establish loss of organization access.
   // Keep the original journal if unresolved, but re-read authorized rows.
   setActionError('');setNotice('organization_write_denied');void latestRead.current();
  }
 },[writeDenied,action.errorStatus,clearPrivate,organization,setActionError]);
 const visible:ReadState=state.key===key&&state.api===api?state:{key,api,loading:true};
 const page=denial||writeDenied?undefined:visible.page;
 const locked=action.busy||!!action.pending, navigationLocked=action.busy||!organization&&!!action.pending;
 const commandsLocked=locked||!!denial||writeDenied||action.conflict||action.persistenceBlocked;
 const rows=page?inboxRows(page):[];
 const selected=page&&selection?.key===key&&selection.api===api?selection.targets:undefined;
 const selectionLocked=commandsLocked||visible.loading;
 const bulkActions:NotificationInboxAction[]=filter==='archived'?['read','unread','restore']:filter==='unread'?['read','archive']:['read','unread','archive'];
 async function confirmed(receipt:InboxWriteReceipt|undefined){
  if(!receipt||!alive.current)return;
  notifyOrganizationInboxChanged(userID,organizationID);
  if('results' in receipt){
   setSelection(undefined);setNotice('batch_confirmed');
   if(!denied.current)await latestRead.current();
   return;
  }
  if(denied.current){setNotice('update_confirmed');return;}
  const current=cached.current?.key===currentKey.current&&cached.current.api===currentApi.current?cached.current.page:undefined;
  if(notificationInboxReceiptIsStale(receipt,current)){setNotice('update_confirmed_newer');return;}
  // Receipts carry no task/source snapshot. Re-read the current filter rather
  // than guessing counts or applying an old receipt over a newer server row.
  setNotice('update_confirmed');await latestRead.current();
 }
 async function command(item:NotificationInboxItem,value:NotificationInboxAction,itemProject:string){
  if(commandsLocked||item.version>=2147483647)return;
  batchWrite.current=false;setNotice('');await confirmed(await action.run<InboxWriteReceipt>(notificationInboxCommandPath(item.id),{project_id:itemProject,expected_version:item.version,action:value}));
 }
 function toggleSelection(item:NotificationInboxItem,itemProject:string){
  if(selectionLocked||item.version>=2147483647)return;
  setSelection(current=>{
   if(!current||current.key!==key||current.api!==api)return current;
   const target=current.targets.find(candidate=>candidate.notification_id===item.id);
   if(target)return {...current,targets:current.targets.filter(candidate=>candidate!==target)};
   if(current.targets.length>=notificationInboxSelectionLimit)return current;
   return {...current,targets:[...current.targets,{project_id:itemProject,notification_id:item.id,expected_version:item.version}]};
  });
 }
 async function commandBatch(value:NotificationInboxAction){
  if(selectionLocked||!selected?.length)return;
  batchWrite.current=true;setNotice('');await confirmed(await action.run<InboxWriteReceipt>(notificationInboxBatchPath,{items:selected,action:value}));
 }
 async function recover(){
  const pending=action.pending;if(!pending||action.busy||denial)return;
  batchWrite.current=pending.path===notificationInboxBatchPath;
  await confirmed(await action.run<InboxWriteReceipt>(pending.path,pending.body,pending.method));
 }
 async function retry(){
  action.setError('');denied.current=false;setDenial(undefined);
  if(await latestRead.current()){
   notifyOrganizationInboxChanged(userID,organizationID);
   if(batchWrite.current)action.clearConflict();
  }
 }
 function openSource(item:NotificationInboxItem,itemProject:string){
  const origin:[]|[string]=organization?[itemProject]:[];
  if(item.kind==='task_run_failed'){if(item.run_id)onRun?.(item.task_id,item.run_id,...origin);}
  else if(item.kind==='task_tool_input_requested'){if(item.tool_input_target)onToolInput?.(item.task_id,item.tool_input_target,...origin);}
  else if(messageSource(item)){if(item.message_id)onMessage?.(item.task_id,item.message_id,...origin);}
  else if(item.kind==='task_handoff_requested'&&item.handoff_id)onHandoff?.(item.task_id,item.handoff_id,...origin);
  else if(item.kind==='task_review_requested'&&item.review_request_id)onReviewRequest?.(item.task_id,item.review_request_id,...origin);
  else onTask(item.task_id,...origin);
 }
 const pendingAction=action.pending?.body.action,pendingItems=action.pending?.body.items;
 const batchConflict=action.conflict&&batchWrite.current;
 return <section className="notification-inbox" aria-label={t('title')}>
  <header className="notification-inbox-heading">
   <div><h2><Bell size={19} aria-hidden="true"/>{t('title')}</h2><p>{t(organization?'organization_intro':'intro')}</p></div>
   {!organization&&<ProjectNotificationPreferencesControl project={project} userID={userID} organizationID={organizationID} disabled={locked} accessBlocked={!!denial||writeDenied} onAccessDenied={clearPrivate}/>}
   <div className="notification-inbox-heading-actions">
    {onScopeMode&&<label className="notification-inbox-mode">
     <span>{t('scope_label')}</span>
     <select value={scopeMode} disabled={navigationLocked} data-notification-scope-mode onChange={event=>onScopeMode(event.target.value==='organization'?'organization':'project')}>
      <option value="project">{t('scope_project')}</option>
      <option value="organization">{t('scope_organization')}</option>
     </select>
    </label>}
    <button type="button" disabled={visible.loading||action.busy} onClick={()=>void retry()} data-notification-refresh><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button>
   </div>
  </header>
  <div className="notification-inbox-filters" role="group" aria-label={mt('inbox_filter_label')}>
   {notificationInboxFilters.map(value=><button type="button" key={value} aria-pressed={filter===value&&!invalid} disabled={navigationLocked} onClick={()=>{action.setError('');onFilter(value);}} data-notification-filter={value}>{t('filter.'+value)}<span className="notification-inbox-count">{page?formatNumber(page.counts[value]):'—'}</span></button>)}
  </div>
  {!writeDenied&&!batchConflict&&<WriteNotice action={action} onRebase={async()=>{if(!await latestRead.current())throw new UiError('notification-inbox.refresh_failed');}}/>}
  {batchConflict&&!writeDenied&&<div className="conflict-banner notification-inbox-batch-conflict">
   <div><strong>{t('batch_changed')}</strong><p>{t('batch_reselect')}</p></div>
   <button type="button" disabled={visible.loading||action.busy} onClick={()=>void retry()}>{t('refresh')}</button>
  </div>}
  {action.pending&&<div className="notification-inbox-recovery" role="status"><strong>{t('original_update')}</strong><p>{t(organization?'organization_original_update_hint':'original_update_hint')}</p>{typeof pendingAction==='string'&&<small>{t('action.'+pendingAction)}{Array.isArray(pendingItems)&&' · '+t('batch_target_count',{count:formatNumber(pendingItems.length)})}</small>}<button type="button" disabled={action.busy||!!denial||organization&&!page} onClick={()=>void recover()} data-notification-recover>{t('restore_original')}</button></div>}
  {notice&&<p className={notice==='organization_write_denied'?'notification-inbox-note':'notification-inbox-notice'} role="status">{t(notice)}</p>}
  {Boolean(visible.error)&&<ErrorNotice error={visible.error} onRetry={()=>void retry()}/>}
  {Boolean(denial)&&<p className="notification-inbox-note" role="status">{t('private_cleared')}</p>}
  {visible.loading&&!page&&<Loading/>}
  {page&&<>
   <div className="notification-inbox-scope">
    <div><p aria-live="polite">{t('showing',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}</p><small>{t('as_of',{at:observed(page.as_of)})}</small></div>
    {!!rows.length&&<button type="button" disabled={selected?action.busy:selectionLocked||rows.every(({notification})=>notification.version>=2147483647)}
     data-notification-select-mode={selected?undefined:''} data-notification-select-cancel={selected?'':undefined}
     onClick={()=>setSelection(selected?undefined:{key,api,targets:[]})}>{t(selected?'batch_cancel':'batch_select')}</button>}
   </div>
   {selected&&<div className="notification-inbox-batch" role="group" aria-label={t('batch_operations')}>
    <div className="notification-inbox-batch-summary">
     <strong role="status" data-notification-selection-count>{t('batch_selection',{count:formatNumber(selected.length),limit:formatNumber(notificationInboxSelectionLimit)})}</strong>
     <small>{t('batch_scope')}</small>
    </div>
    <div className="notification-inbox-batch-actions">
     {bulkActions.map(value=><button key={value} type="button" disabled={selectionLocked||!selected.length}
      data-notification-batch-action={value} onClick={()=>void commandBatch(value)}>{t('action.'+value)}</button>)}
     {!!selected.length&&<button type="button" disabled={action.busy} onClick={()=>setSelection({key,api,targets:[]})}>{t('batch_clear')}</button>}
    </div>
   </div>}
   {!rows.length?<Empty title={mt('inbox_empty_'+filter)} description={t('empty_hint')}/>:<ul className="notification-inbox-list" aria-label={mt('inbox_list')} aria-busy={visible.loading}>{rows.map(({notification:item,project_id:itemProject,project_title:projectTitle})=>{
    const runFailed=item.kind==='task_run_failed',toolInput=item.kind==='task_tool_input_requested';
    const name=item.current_task?.title||item.task_id,runAction=toolInput?'open_tool_input':runFailed?'open_run':'';
    const sourceDisabled=navigationLocked||messageSource(item)&&!onMessage||item.kind==='task_review_requested'&&!onReviewRequest||item.kind==='task_handoff_requested'&&!onHandoff||runFailed&&!onRun||toolInput&&!onToolInput;
    const sourceLabel=runAction?t(runAction+'_named',{name}):item.kind==='task_followed'?st('inbox_open_message',{title:name}):item.kind==='task_handoff_requested'?t('open_handoff',{name}):item.kind==='task_review_requested'?rt('open_notification',{name}):item.kind==='task_mentioned'?mt('open_discussion',{name}):t('open_task',{name});
    const eventLabel=toolInput?t('tool_input_requested'):runFailed?t('run_failed'):item.kind==='task_followed'?st('inbox_label'):item.kind==='task_handoff_requested'?t('handoff_requested'):item.kind==='task_review_requested'?rt('notification'):item.kind==='task_mentioned'?mt('mentioned'):t('assigned');
    const at=observed(item.created_at),checked=!!selected?.some(target=>target.notification_id===item.id);
    const selectionLabel=[eventLabel,item.current_task?.title,organization?projectTitle||itemProject:undefined,at].filter(Boolean).join(' · ');
    return <li key={item.id} className={'notification-inbox-row'+(!item.read?' is-unread':'')+(selected?' is-selecting':'')+(checked?' is-selected':'')} data-notification-id={item.id}>
    {selected&&<label className="notification-inbox-select-row"><input type="checkbox" data-notification-select-row checked={checked}
     disabled={selectionLocked||item.version>=2147483647||!checked&&selected.length>=notificationInboxSelectionLimit}
     onChange={()=>toggleSelection(item,itemProject)} aria-label={t('batch_select_named',{name:selectionLabel})}/></label>}
    <div className="notification-inbox-main"><div className="notification-inbox-event">
     <span className={runFailed?'notification-inbox-run-failed':toolInput?'notification-inbox-tool-input':undefined}>{runFailed&&<CircleAlert size={15} aria-hidden="true"/>}{toolInput&&<MessageSquare size={15} aria-hidden="true"/>}{eventLabel}</span>
     <span className="notification-inbox-state">{t(item.read?'read':'unread')}</span>{item.archived&&<span className="notification-inbox-state">{t('archived')}</span>}
    </div>
     {organization&&<span className="notification-inbox-project">{t('source_project',{name:projectTitle||itemProject})}</span>}
     {item.source_available&&item.current_task?<><button type="button" className={'notification-inbox-source'+(runAction?' is-run-source':'')} disabled={sourceDisabled} onClick={()=>openSource(item,itemProject)} aria-label={sourceLabel}>{name}{runAction?<span className="notification-inbox-run-action">{t(runAction)}<ArrowUpRight size={15} aria-hidden="true"/></span>:<ArrowUpRight size={15} aria-hidden="true"/>}</button><div className="notification-inbox-source-facts"><span>{t('current_task_status')} <Badge state={item.current_task.status}/></span>{item.kind==='task_assigned'&&!item.assignment_current&&<span>{t('assignment_changed')}</span>}</div></>:<strong className="notification-inbox-unavailable">{t('source_unavailable')}</strong>}
     <time dateTime={item.created_at}>{toolInput?t('tool_input_requested_at',{at}):runFailed?t('run_failed_at',{at}):item.kind==='task_followed'?st('inbox_created_at',{at}):item.kind==='task_handoff_requested'?t('handoff_requested_at',{at}):item.kind==='task_review_requested'?rt('requested_at',{at}):item.kind==='task_mentioned'?mt('mentioned_at',{at}):t('assigned_at',{at})}</time>
    </div>
    {!selected&&<div className="notification-inbox-actions"><button type="button" disabled={commandsLocked||item.version>=2147483647} onClick={()=>void command(item,item.read?'unread':'read',itemProject)} data-notification-action={item.read?'unread':'read'} aria-label={t(item.read?'unread_named':'read_named',{name:item.current_task?.title||t('source_unavailable')})}>{item.read?<Mail size={15} aria-hidden="true"/>:<Check size={15} aria-hidden="true"/>}{t('action.'+(item.read?'unread':'read'))}</button><button type="button" disabled={commandsLocked||item.version>=2147483647} onClick={()=>void command(item,item.archived?'restore':'archive',itemProject)} data-notification-action={item.archived?'restore':'archive'} aria-label={t(item.archived?'restore_named':'archive_named',{name:item.current_task?.title||t('source_unavailable')})}>{item.archived?<ArchiveRestore size={15} aria-hidden="true"/>:<Archive size={15} aria-hidden="true"/>}{t('action.'+(item.archived?'restore':'archive'))}</button></div>}
   </li>;})}</ul>}
   {page.next_cursor&&<div className="notification-inbox-more"><button type="button" disabled={visible.loading||navigationLocked} onClick={()=>void read(true)} data-notification-more>{visible.loading?t('loading_more'):t('more')}</button></div>}
   <p className="notification-inbox-note">{organization?t('organization_scope_hint'):mt('inbox_scope')}</p>
  </>}
 </section>;
}
function observed(value:string){return formatDateTime(value,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});}
