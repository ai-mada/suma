import {useCallback,useEffect,useLayoutEffect,useRef,useState} from 'react';
import {Archive,ArchiveRestore,ArrowUpRight,Bell,Check,Mail,RefreshCw} from 'lucide-react';
import type {NotificationInboxCommandReceipt,NotificationInboxItem,NotificationInboxPage} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {Badge,Empty,ErrorNotice,Loading} from './ui';
import {useWriteAction,WriteNotice} from './write-action';
import {appendNotificationInboxPage,notificationInboxCommandPath,notificationInboxFilters,notificationInboxPath,notificationInboxReceiptIsStale,notificationInboxScope,validNotificationInboxFilter,validNotificationInboxPage,validNotificationInboxReceipt,validNotificationInboxRecovery,type NotificationInboxAction,type NotificationInboxFilter} from './notification-inbox-model';
import './NotificationInbox.css';

export type NotificationInboxProps={project:string;userID:string;organizationID:string;filter:NotificationInboxFilter;refresh:number;onFilter:(filter:NotificationInboxFilter)=>void;onTask:(taskID:string)=>void;onMessage?:(taskID:string,messageID:string)=>void;onReviewRequest?:(taskID:string,requestID:string)=>void;onHandoff?:(taskID:string,handoffID:string)=>void;invalid?:boolean};
const t=(key:string,values?:Record<string,unknown>)=>tr('notification-inbox.'+key,values);
const mt=(key:string,values?:Record<string,unknown>)=>tr('task-conversation-mentions.'+key,values);
const rt=(key:string,values?:Record<string,unknown>)=>tr('task-review-requests.'+key,values);
const accessLost=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type ReadState={key:string;api:ApiCaller;page?:NotificationInboxPage;loading:boolean;error?:unknown};

/** Only an identity/project change remounts the writer. Filter and refresh
 * changes cannot discard or retarget an unacknowledged personal command. */
export function NotificationInbox(props:NotificationInboxProps){return <InboxScope key={notificationInboxScope(props.userID,props.organizationID,props.project)} {...props}/>;}

function InboxScope({project,userID,organizationID,filter,refresh,onFilter,onTask,onMessage,onReviewRequest,onHandoff,invalid=false}:NotificationInboxProps){
 useLocale();const api=useApi(),scope=notificationInboxScope(userID,organizationID,project),key=JSON.stringify([filter,refresh,invalid]);
 const action=useWriteAction({scope,storage:toolSessionStorage(),validateRequest:request=>validNotificationInboxRecovery(request,project),validateResponse:validNotificationInboxReceipt});
 const [state,setState]=useState<ReadState>({key,api,loading:true}),[denial,setDenial]=useState<unknown>(),[notice,setNotice]=useState('');
 const alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined),currentKey=useRef(key),currentApi=useRef(api),denied=useRef(false),cached=useRef<{key:string;api:ApiCaller;page:NotificationInboxPage}|undefined>(undefined);
 currentKey.current=key;currentApi.current=api;
 const clearPrivate=useCallback((error:unknown)=>{
  if(!alive.current)return;denied.current=true;epoch.current++;controller.current?.abort();cached.current=undefined;setNotice('');setDenial(error);setState({key:currentKey.current,api:currentApi.current,loading:false,error});
 },[]);
 const read=useCallback(async(more=false):Promise<NotificationInboxPage|undefined>=>{
  const previous=more&&cached.current?.key===key&&cached.current.api===api?cached.current.page:undefined,cursor=previous?.next_cursor;
  if(more&&!cursor)return;
  const generation=++epoch.current;controller.current?.abort();const abort=new AbortController();controller.current=abort;
  if(!more)cached.current=undefined;
  setState({key,api,page:previous,loading:true});
  try{
   if(invalid||!validNotificationInboxFilter(filter))throw new UiError('notification-inbox.invalid_filter');
   const value=await api<unknown>(notificationInboxPath(project,filter,cursor),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);
   if(!alive.current||abort.signal.aborted||generation!==epoch.current||currentKey.current!==key||currentApi.current!==api)return;
   if(!validNotificationInboxPage(value,project,userID,filter)||cursor&&value.next_cursor===cursor)throw new UiError('notification-inbox.invalid_read');
   const page=previous?appendNotificationInboxPage(previous,value):value;
   if(!page||previous&&!value.items.length&&page.items.length<page.total)throw new UiError('notification-inbox.page_changed');
   cached.current={key,api,page};denied.current=false;setDenial(undefined);setState({key,api,page,loading:false});return page;
  }catch(error){
   if(!alive.current)return;
   // An explicit refusal belongs to this mounted human/project, even if a
   // different filter started meanwhile. An old identity has already unmounted.
   if(accessLost(error)){clearPrivate(error);return;}
   if(abort.signal.aborted||generation!==epoch.current||currentKey.current!==key||currentApi.current!==api)return;
   cached.current=undefined;setState({key,api,loading:false,error});
  }
 },[api,key,project,userID,organizationID,filter,invalid,clearPrivate]);
 const latestRead=useRef(read);latestRead.current=read;
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;controller.current?.abort();cached.current=undefined;};},[]);
 useEffect(()=>{void read();return()=>{epoch.current++;controller.current?.abort();};},[read]);
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus);
 useLayoutEffect(()=>{if(writeDenied)clearPrivate(new ApiError(action.errorStatus!,'notification_access_changed',t('access_changed')));},[writeDenied,action.errorStatus,clearPrivate]);
 const visible:ReadState=state.key===key&&state.api===api?state:{key,api,loading:true},page=denial||writeDenied?undefined:visible.page,locked=action.busy||!!action.pending,commandsLocked=locked||!!denial||writeDenied||action.conflict||action.persistenceBlocked;
 async function confirmed(receipt:NotificationInboxCommandReceipt|undefined){
  if(!receipt||!alive.current)return;
  if(denied.current){setNotice('update_confirmed');return;}
  const current=cached.current?.key===currentKey.current&&cached.current.api===currentApi.current?cached.current.page:undefined;
  if(notificationInboxReceiptIsStale(receipt,current)){setNotice('update_confirmed_newer');return;}
  // Receipts carry no task/source snapshot. Re-read the current filter rather
  // than guessing counts or applying an old receipt over a newer server row.
  setNotice('update_confirmed');await latestRead.current();
 }
 async function command(item:NotificationInboxItem,value:NotificationInboxAction){
  if(commandsLocked||item.version>=2147483647)return;
  setNotice('');await confirmed(await action.run<NotificationInboxCommandReceipt>(notificationInboxCommandPath(item.id),{project_id:project,expected_version:item.version,action:value}));
 }
 async function recover(){const pending=action.pending;if(!pending||action.busy||denial)return;await confirmed(await action.run<NotificationInboxCommandReceipt>(pending.path,pending.body,pending.method));}
 async function retry(){action.setError('');denied.current=false;setDenial(undefined);await latestRead.current();}
 const pendingAction=action.pending?.body.action;
 return <section className="notification-inbox" aria-label={t('title')}>
  <header className="notification-inbox-heading"><div><h2><Bell size={19} aria-hidden="true"/>{t('title')}</h2><p>{t('intro')}</p></div><button type="button" disabled={visible.loading||action.busy} onClick={()=>void retry()} data-notification-refresh><RefreshCw size={15} aria-hidden="true"/>{t('refresh')}</button></header>
  <div className="notification-inbox-filters" role="group" aria-label={mt('inbox_filter_label')}>
   {notificationInboxFilters.map(value=><button type="button" key={value} aria-pressed={filter===value&&!invalid} disabled={locked} onClick={()=>{action.setError('');onFilter(value);}} data-notification-filter={value}>{t('filter.'+value)}<span className="notification-inbox-count">{page?formatNumber(page.counts[value]):'—'}</span></button>)}
  </div>
  {!writeDenied&&<WriteNotice action={action} onRebase={async()=>{if(!await latestRead.current())throw new UiError('notification-inbox.refresh_failed');}}/>}
  {action.pending&&<div className="notification-inbox-recovery" role="status"><strong>{t('original_update')}</strong><p>{t('original_update_hint')}</p>{typeof pendingAction==='string'&&<small>{t('action.'+pendingAction)}</small>}<button type="button" disabled={action.busy||!!denial} onClick={()=>void recover()} data-notification-recover>{t('restore_original')}</button></div>}
  {notice&&<p className="notification-inbox-notice" role="status">{t(notice)}</p>}
  {Boolean(visible.error)&&<ErrorNotice error={visible.error} onRetry={()=>void retry()}/>}
  {Boolean(denial)&&<p className="notification-inbox-note" role="status">{t('private_cleared')}</p>}
  {visible.loading&&!page&&<Loading/>}
  {page&&<>
   <div className="notification-inbox-scope"><p aria-live="polite">{t('showing',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}</p><small>{t('as_of',{at:observed(page.as_of)})}</small></div>
   {!page.items.length?<Empty title={mt('inbox_empty_'+filter)} description={t('empty_hint')}/>:<ul className="notification-inbox-list" aria-label={mt('inbox_list')} aria-busy={visible.loading}>{page.items.map(item=><li key={item.id} className={'notification-inbox-row'+(!item.read?' is-unread':'')} data-notification-id={item.id}>
    <div className="notification-inbox-main"><div className="notification-inbox-event"><span>{item.kind==='task_handoff_requested'?t('handoff_requested'):item.kind==='task_review_requested'?rt('notification'):item.kind==='task_mentioned'?mt('mentioned'):t('assigned')}</span><span className="notification-inbox-state">{t(item.read?'read':'unread')}</span>{item.archived&&<span className="notification-inbox-state">{t('archived')}</span>}</div>
     {item.source_available&&item.current_task?<><button type="button" className="notification-inbox-source" disabled={locked||item.kind==='task_mentioned'&&!onMessage||item.kind==='task_review_requested'&&!onReviewRequest||item.kind==='task_handoff_requested'&&!onHandoff} onClick={()=>{if(item.kind==='task_handoff_requested'&&item.handoff_id)onHandoff?.(item.task_id,item.handoff_id);else if(item.kind==='task_review_requested'&&item.review_request_id)onReviewRequest?.(item.task_id,item.review_request_id);else if(item.kind==='task_mentioned'&&item.message_id)onMessage?.(item.task_id,item.message_id);else onTask(item.task_id);}} aria-label={item.kind==='task_handoff_requested'?t('open_handoff',{name:item.current_task.title||item.task_id}):item.kind==='task_review_requested'?rt('open_notification',{name:item.current_task.title||item.task_id}):item.kind==='task_mentioned'?mt('open_discussion',{name:item.current_task.title||item.task_id}):t('open_task',{name:item.current_task.title||item.task_id})}>{item.current_task.title||item.task_id}<ArrowUpRight size={15} aria-hidden="true"/></button><div className="notification-inbox-source-facts"><span>{t('current_task_status')} <Badge state={item.current_task.status}/></span>{item.kind==='task_assigned'&&!item.assignment_current&&<span>{t('assignment_changed')}</span>}</div></>:<strong className="notification-inbox-unavailable">{t('source_unavailable')}</strong>}
     <time dateTime={item.created_at}>{item.kind==='task_handoff_requested'?t('handoff_requested_at',{at:observed(item.created_at)}):item.kind==='task_review_requested'?rt('requested_at',{at:observed(item.created_at)}):item.kind==='task_mentioned'?mt('mentioned_at',{at:observed(item.created_at)}):t('assigned_at',{at:observed(item.created_at)})}</time>
    </div>
    <div className="notification-inbox-actions"><button type="button" disabled={commandsLocked||item.version>=2147483647} onClick={()=>void command(item,item.read?'unread':'read')} data-notification-action={item.read?'unread':'read'} aria-label={t(item.read?'unread_named':'read_named',{name:item.current_task?.title||t('source_unavailable')})}>{item.read?<Mail size={15} aria-hidden="true"/>:<Check size={15} aria-hidden="true"/>}{t('action.'+(item.read?'unread':'read'))}</button><button type="button" disabled={commandsLocked||item.version>=2147483647} onClick={()=>void command(item,item.archived?'restore':'archive')} data-notification-action={item.archived?'restore':'archive'} aria-label={t(item.archived?'restore_named':'archive_named',{name:item.current_task?.title||t('source_unavailable')})}>{item.archived?<ArchiveRestore size={15} aria-hidden="true"/>:<Archive size={15} aria-hidden="true"/>}{t('action.'+(item.archived?'restore':'archive'))}</button></div>
   </li>)}</ul>}
   {page.next_cursor&&<div className="notification-inbox-more"><button type="button" disabled={visible.loading||locked} onClick={()=>void read(true)} data-notification-more>{visible.loading?t('loading_more'):t('more')}</button></div>}
   <p className="notification-inbox-note">{mt('inbox_scope')}</p>
  </>}
 </section>;
}
function observed(value:string){return formatDateTime(value,{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'});}
