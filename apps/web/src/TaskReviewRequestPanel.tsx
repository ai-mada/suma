import {useCallback,useEffect,useId,useRef,useState} from 'react';
import {ChevronDown,ClipboardCheck,RefreshCw,Search} from 'lucide-react';
import type {TaskReviewRequest,TaskReviewRequestDetail,TaskReviewRequestPage,TaskReviewTargetPage} from '../../../packages/contracts/src';
import {ApiError,query,type ApiCaller,type Session} from './api';
import {useApi} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard,useWriteAction,WriteNotice} from './write-action';
import {appendReviewRequests,emptyReviewDraft,readReviewDraft,reviewDraftKey,reviewRequestPath,reviewRequestsPath,reviewTargetsPath,reviewVersion,saveReviewDraft,validReviewQuery,validReviewRequestDetail,validReviewRequestPage,validReviewRequestReceipt,validReviewRequestRecovery,validReviewTargets,validReviewText,type ReviewDraft} from './task-review-requests-model';
import './TaskReviewRequestPanel.css';

export type TaskReviewRequestPanelProps={session:Session;project:string;taskID:string;taskVersion:number;runID:string;refresh:number;selectedRequest?:string;onChanged:()=>void|Promise<void>;onOpenReview:()=>void;onOpenRun:(id:string)=>void;onForbidden?:(error:unknown)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('task-review-requests.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
type RunBasis={id:string;project_id:string;version:number;state:string};

/** Each caller is keyed by the human/project/task AND API owner. Filter and
 * refresh changes never remount the writer or retarget its original JSON. */
export function TaskReviewRequestPanel(props:TaskReviewRequestPanelProps){
 const api=useApi(),owner=useRef({api,epoch:0});if(owner.current.api!==api)owner.current={api,epoch:owner.current.epoch+1};
 const scope=JSON.stringify([props.session.user.id,props.session.organization.id,props.project,props.taskID]);
 return <ReviewScope key={scope+':'+owner.current.epoch} {...props} scope={scope}/>;
}

type ReadSnapshot<T>={api:ApiCaller;key:string;value?:T;loading:boolean;error?:unknown};
/** A single bounded read, optionally followed by explicit cursor pages. Any
 * current-owner refusal revokes sibling facts too, even after a path changed. */
function useReviewRead<T>(path:string|undefined,refresh:number,valid:(value:unknown)=>value is T,onDenied:(error:unknown)=>void,merge?:(previous:T,next:T)=>T|undefined,nextCursor?:(value:T)=>string){
 const api=useApi(),key=JSON.stringify([path,refresh]),[snapshot,setSnapshot]=useState<ReadSnapshot<T>>();
 const current=useRef({api,key,path}),alive=useRef(true),epoch=useRef(0),abort=useRef<AbortController|undefined>(undefined),cached=useRef<ReadSnapshot<T>|undefined>(undefined),denied=useRef(onDenied);denied.current=onDenied;current.current={api,key,path};
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;abort.current?.abort();};},[]);
 const read=useCallback(async(more=false):Promise<T|undefined>=>{
  if(!path)return;const previous=cached.current?.key===key&&cached.current.api===api?cached.current.value:undefined,cursor=more&&previous&&nextCursor?nextCursor(previous):'';if(more&&!cursor)return previous;
  const generation=++epoch.current,controller=new AbortController();abort.current?.abort();abort.current=controller;
  const live=()=>alive.current&&!controller.signal.aborted&&epoch.current===generation&&current.current.api===api&&current.current.key===key;
  const pending={api,key,value:more?previous:undefined,loading:true};cached.current=pending;setSnapshot(pending);
  try{const value=await api<unknown>(path+(cursor?'&cursor='+encodeURIComponent(cursor):''),undefined,'GET',undefined,controller.signal);if(!live())return;
   if(!valid(value))throw new UiError('task-review-requests.invalid_read');const merged=more&&previous&&merge?merge(previous,value):value;
   if(!merged||more&&nextCursor&&cursor===nextCursor(value))throw new UiError('task-review-requests.page_changed');
   const state={api,key,value:merged,loading:false};cached.current=state;setSnapshot(state);return merged;
  }catch(error){if(!alive.current||current.current.api!==api)return;if(refused(error)){denied.current(error);return;}if(!live())return;cached.current=undefined;setSnapshot({api,key,error,loading:false});}
 },[api,key,path,valid,merge,nextCursor]);
 useEffect(()=>{cached.current=undefined;setSnapshot(undefined);if(path)void read();return()=>{epoch.current++;abort.current?.abort();};},[read]);
 const visible=path&&snapshot?.api===api&&snapshot.key===key?snapshot:undefined;return {...visible,read};
}
const requestCursor=(page:TaskReviewRequestPage)=>page.next_cursor;
const targetCursor=(page:TaskReviewTargetPage)=>page.next_cursor;
function mergeTargets(previous:TaskReviewTargetPage,next:TaskReviewTargetPage){const items=[...previous.items,...next.items];return new Set(items.map(item=>item.resource_id)).size===items.length&&new Set(items.map(item=>item.user_id)).size===items.length&&items.length<=next.total?{...next,items}:undefined;}

function ReviewScope({session,project,taskID,taskVersion,runID,refresh,selectedRequest='',onChanged,onOpenReview,onOpenRun,onForbidden,scope}:TaskReviewRequestPanelProps&{scope:string}){
 useLocale();const ids=useId(),storage=toolSessionStorage(),draftKey=reviewDraftKey(session.user.id,session.organization.id,project,taskID),initial=useRef(readReviewDraft(storage,draftKey));
 const [draft,setDraft]=useState<ReviewDraft>(initial.current.draft),[invalidDraft,setInvalidDraft]=useState(!!initial.current.invalid),[storageFailed,setStorageFailed]=useState(false);
 const [open,setOpen]=useState(!!selectedRequest||!!initial.current.draft.note||!!initial.current.draft.reviewer_resource_id||!!initial.current.draft.cancel||!!initial.current.invalid),[composing,setComposing]=useState(!!initial.current.draft.note||!!initial.current.draft.reviewer_resource_id);
 const [denial,setDenial]=useState<unknown>(),[tick,setTick]=useState(0),[notice,setNotice]=useState(false),[selectedValid,setSelectedValid]=useState('');
 const alive=useRef(true),callbacks=useRef({onChanged,onForbidden});callbacks.current={onChanged,onForbidden};
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;};},[]);
 const deny=useCallback((error:unknown)=>{if(!alive.current)return;setDenial(error);setSelectedValid('');callbacks.current.onForbidden?.(error);},[]);
 const action=useWriteAction({scope:'task-review-requests:'+scope,storage,validateRequest:request=>validReviewRequestRecovery(request,project,taskID),validateResponse:(value,request)=>validReviewRequestReceipt(value,request,project,taskID,session.user.id)});
 const writeDenied=!!action.errorStatus&&[401,403,404].includes(action.errorStatus),blocked=!!denial||writeDenied,locked=action.busy||!!action.pending;
 useEffect(()=>{if(writeDenied)deny(new ApiError(action.errorStatus!,'review_access_changed',t('access_changed')));},[writeDenied,action.errorStatus,deny]);
 const pageValid=useCallback((value:unknown):value is TaskReviewRequestPage=>validReviewRequestPage(value,project,taskID),[project,taskID]);
 const history=useReviewRead(!blocked?reviewRequestsPath(project,taskID):undefined,refresh+tick+taskVersion,pageValid,deny,appendReviewRequests,requestCursor),page=history.value;
 const detailValid=useCallback((value:unknown):value is TaskReviewRequestDetail=>validReviewRequestDetail(value,project,taskID,selectedRequest),[project,taskID,selectedRequest]);
 const located=useReviewRead(!blocked&&selectedRequest?reviewRequestPath(project,selectedRequest):undefined,refresh+tick+taskVersion,detailValid,deny);
 const pendingID=page?.pending_request_id||'',loadedPending=page?.items.find(item=>item.id===pendingID);
 const pendingValid=useCallback((value:unknown):value is TaskReviewRequestDetail=>validReviewRequestDetail(value,project,taskID,pendingID),[project,taskID,pendingID]);
 const pendingRead=useReviewRead(!blocked&&pendingID&&!loadedPending&&pendingID!==selectedRequest?reviewRequestPath(project,pendingID):undefined,refresh+tick+taskVersion,pendingValid,deny);
 const exactPending=pendingID===selectedRequest?located.value?.request:pendingRead.value?.request;
 const pending=exactPending&&(!loadedPending||exactPending.version>=loadedPending.version)?exactPending:loadedPending;
 // The formal DAG Run response has no task_id. Match the exact project/run;
 // the task/page version below and Create's server-side task/run CAS retain
 // the task binding rather than inventing a field in the shared Run schema.
 const runValid=useCallback((value:unknown):value is RunBasis=>!!value&&typeof value==='object'&&'id' in value&&value.id===runID&&'project_id' in value&&value.project_id===project&&'version' in value&&reviewVersion(value.version)&&'state' in value&&typeof value.state==='string',[project,runID]);
 const run=useReviewRead(!blocked&&composing&&page?.can_request&&runID?'/runs/'+encodeURIComponent(runID)+query(project):undefined,refresh+tick+taskVersion,runValid,deny);
 const dirty=!!draft.note||!!draft.reviewer_resource_id||!!draft.cancel||invalidDraft;
 useLeaveGuard((dirty||storageFailed)&&!action.pending,action.setError);
 useEffect(()=>{if(invalidDraft)return;try{if(storage||dirty)saveReviewDraft(storage,draftKey,draft);setStorageFailed(false);}catch{setStorageFailed(true);}},[storage,draftKey,draft,invalidDraft]);
 useEffect(()=>{if(selectedRequest||action.pending)setOpen(true);},[selectedRequest,!!action.pending]);
 const focused=useRef<HTMLElement|undefined>(undefined);
 useEffect(()=>{if(!selectedRequest){focused.current=undefined;return;}const element=document.getElementById(ids+'-'+selectedRequest);if(!blocked&&open&&element&&element!==focused.current){element.focus();element.scrollIntoView?.({block:'nearest'});focused.current=element;}},[selectedRequest,open,blocked,page,located.value,pendingRead.value,ids]);
 const basis=run.value&&page&&page.task_version===taskVersion&&run.value.state==='succeeded'&&page.task_version<2147483647&&run.value.version<2147483647?{task_version:page.task_version,run_id:run.value.id,run_version:run.value.version}:undefined;
 const basisCurrent=!!basis&&JSON.stringify(draft.basis)===JSON.stringify(basis),recipientKey=JSON.stringify([refresh+tick+taskVersion,draft.reviewer_resource_id]);
 const ready=!!page?.can_request&&!pendingID&&!blocked&&!history.loading&&!history.error&&!invalidDraft&&!action.conflict&&!action.persistenceBlocked&&basisCurrent&&selectedValid===recipientKey&&validReviewText(draft.note);
 function clearDraft(){setDraft(emptyReviewDraft());setComposing(false);setNotice(false);}
 function saveLocal(clear=false){try{saveReviewDraft(storage,draftKey,clear?emptyReviewDraft():draft);setStorageFailed(false);if(clear){setInvalidDraft(false);clearDraft();}}catch{setStorageFailed(true);}}
 async function confirmed(value:TaskReviewRequest|undefined){if(!value||!alive.current)return;clearDraft();setNotice(true);await Promise.allSettled([history.read(),Promise.resolve().then(()=>callbacks.current.onChanged())]);}
 async function create(){if(!ready||locked||!draft.basis)return;await confirmed(await action.run<TaskReviewRequest>('/task-review-requests',{project_id:project,task_id:taskID,expected_task_version:draft.basis.task_version,run_id:draft.basis.run_id,expected_run_version:draft.basis.run_version,reviewer_resource_id:draft.reviewer_resource_id,note:draft.note}));}
 async function cancel(){const original=draft.cancel;if(!original||locked||blocked||!pending||pending.id!==original.id||pending.version!==original.version||!pending.can_cancel||!validReviewText(original.reason))return;await confirmed(await action.run<TaskReviewRequest>('/task-review-requests/'+encodeURIComponent(original.id)+'/commands',{project_id:project,expected_version:original.version,action:'cancel',reason:original.reason}));}
 async function recover(){const original=action.pending;if(!original||action.busy||blocked)return;await confirmed(await action.run<TaskReviewRequest>(original.path,original.body,original.method));}
 async function rebase(){const current=await history.read();if(!current)throw new UiError('task-review-requests.refresh_failed');if(composing&&runID)await run.read();}
 const onValidity=useCallback((key:string,valid:boolean)=>setSelectedValid(valid?key:''),[]);

 function requestCard(item:TaskReviewRequest,current=false){return <article key={item.id} id={ids+'-'+item.id} tabIndex={-1} className={'task-review-card'+(item.id===selectedRequest?' is-located':'')} data-review-request={item.id}>
  <header><strong>{t('state.'+item.state)}</strong><time dateTime={item.created_at}>{formatDateTime(item.created_at)}</time></header>
  <dl><dt>{t('reviewer')}</dt><dd>{item.reviewer_name_at_request||item.reviewer_resource_id}{item.is_current_assignee&&<small> · {t('you')}</small>}</dd><dt>{t('requester')}</dt><dd>{item.requester_name_at_request||item.requester_user_id}</dd><dt>{t('run')}</dt><dd><code>{item.run_id}</code> · v{formatNumber(item.run_version)}</dd></dl>
  <p className="task-review-note">{item.note}</p>{item.state==='pending'&&!item.binding_current&&<p className="task-review-warning">{t('binding_changed')}</p>}{item.state==='pending'&&!item.reviewer_available&&<p className="task-review-warning">{t('reviewer_unavailable')}</p>}
  {item.state==='closed'&&(item.decision?<div className="task-review-decision"><strong>{t('decision.'+item.decision.decision)}</strong><p>{t('actual_reviewer',{id:item.decision.reviewer_user_id})}</p><p className="task-review-note">{item.decision.reason}</p></div>:<p className="task-review-warning">{t('decision_unavailable')}</p>)}
  {item.state==='cancelled'&&<p className="task-review-note">{t('cancelled_reason',{reason:item.cancel_reason})}</p>}
  {item.source_available?<div className="task-review-actions"><button type="button" disabled={locked} onClick={()=>onOpenRun(item.run_id)}>{t('open_run')}</button>{current&&<button type="button" disabled={locked} onClick={onOpenReview}>{t('open_review')}</button>}{current&&item.can_cancel&&<button type="button" disabled={locked||invalidDraft} data-review-cancel={item.id} onClick={()=>{setOpen(true);setDraft(value=>({...value,cancel:{id:item.id,version:item.version,reason:''}}));}}>{t('cancel')}</button>}</div>:<p className="task-review-warning">{t('source_unavailable')}</p>}
 </article>;}

 return <section className="task-review-requests" aria-label={t('title')}>
  <h3><button type="button" className="task-review-toggle" data-review-toggle aria-expanded={open} aria-controls={ids+'-body'} disabled={locked&&open} onClick={()=>{if(open&&(dirty||locked||storageFailed)){action.setError(new UiError('task-review-requests.draft_guard'));return;}setOpen(value=>!value);}}><ClipboardCheck size={15} aria-hidden="true"/><span>{t('title')}</span><ChevronDown size={15} aria-hidden="true"/></button></h3>
  <p className="task-review-muted">{t('boundary')}</p>
  {blocked?<ErrorNotice error={denial||new UiError('task-review-requests.access_changed')} onRetry={()=>{action.setError('');setDenial(undefined);setTick(value=>value+1);}}/>:<>
   {history.loading&&!page&&<Loading/>}{!!history.error&&<ErrorNotice error={history.error} onRetry={()=>void history.read()}/>}
   {page&&(pending?requestCard(pending,true):pendingID?<><Loading/>{!!pendingRead.error&&<ErrorNotice error={pendingRead.error} onRetry={()=>void pendingRead.read()}/>}</>:<p className="task-review-muted">{t('no_pending')}</p>)}
  </>}
  {storageFailed&&<div role="alert"><p>{t('draft_storage')}</p><button type="button" disabled={locked} onClick={()=>saveLocal()}>{t('retry_draft')}</button></div>}
  {invalidDraft&&<div role="alert"><p>{t('invalid_draft')}</p><button type="button" disabled={locked} onClick={()=>saveLocal(true)}>{t('clear_draft')}</button></div>}
  {blocked&&dirty&&!invalidDraft&&!action.pending&&<button type="button" data-review-clear-draft disabled={locked} onClick={clearDraft}>{t('clear_draft')}</button>}
  {open&&<div id={ids+'-body'} className="task-review-body">
   <WriteNotice action={action} onRebase={rebase}/>{action.pending&&<div className="task-review-recovery" role="status"><strong>{t('original_pending')}</strong><p className="task-review-note">{String(action.pending.body.note||action.pending.body.reason||'')}</p><button type="button" data-review-restore disabled={action.busy||blocked} onClick={()=>void recover()}>{t('restore')}</button></div>}
   {notice&&<p role="status">{t('confirmed')}</p>}
   {!blocked&&<>
    {selectedRequest&&located.loading&&<Loading/>}{!!located.error&&<ErrorNotice error={located.error} onRetry={()=>void located.read()}/>}
    {located.value&&located.value.request.id!==pending?.id&&!page?.items.some(item=>item.id===selectedRequest)&&<section aria-label={t('original_request')}><h4>{t('original_request')}</h4>{requestCard(located.value.request)}</section>}
    {page?.can_request&&!pendingID&&<button type="button" disabled={locked||invalidDraft} data-review-compose onClick={()=>setComposing(true)}>{t('request')}</button>}
    {composing&&<div className="task-review-compose"><h4>{t('request')}</h4>
     {run.loading&&<Loading/>}{!!run.error&&<ErrorNotice error={run.error} onRetry={()=>void run.read()}/>}
     {!basis&&<p className="task-review-warning">{t('run_required')}</p>}
     {basis&&!basisCurrent&&<div className="task-review-warning"><p>{t(draft.basis?'draft_basis_changed':'adopt_basis')}</p><button type="button" disabled={locked} data-review-adopt onClick={()=>setDraft(value=>({...value,basis}))}>{t('use_current_basis')}</button></div>}
     {draft.basis&&<small className="task-review-muted">{t('draft_basis',{task:formatNumber(draft.basis.task_version),run:draft.basis.run_id,version:formatNumber(draft.basis.run_version)})}</small>}
     <ReviewTargetPicker project={project} taskID={taskID} selectedID={draft.reviewer_resource_id} validityKey={recipientKey} disabled={locked||invalidDraft||!page?.can_request} refresh={refresh+tick+taskVersion} onChange={id=>setDraft(value=>({...value,reviewer_resource_id:id}))} onValidity={onValidity} onDenied={deny}/>
     <label htmlFor={ids+'-note'}>{t('note')}</label><textarea id={ids+'-note'} name="review_request_note" rows={3} maxLength={4000} value={typeof action.pending?.body.note==='string'?action.pending.body.note:draft.note} disabled={locked||invalidDraft||!page?.can_request} onChange={event=>setDraft(value=>({...value,note:event.target.value}))}/>
     <div className="task-review-actions"><button type="button" disabled={locked} onClick={clearDraft}>{t('clear_draft')}</button><button type="button" className="primary" data-review-create disabled={!ready||locked} onClick={()=>void create()}>{t('send_request')}</button></div>
    </div>}
    {draft.cancel&&<div className="task-review-cancel"><label htmlFor={ids+'-cancel'}>{t('cancel_reason')}</label><textarea id={ids+'-cancel'} name="review_cancel_reason" rows={2} maxLength={4000} value={typeof action.pending?.body.reason==='string'?action.pending.body.reason:draft.cancel.reason} disabled={locked} onChange={event=>setDraft(value=>({...value,cancel:value.cancel?{...value.cancel,reason:event.target.value}:undefined}))}/>{pending?.version!==draft.cancel.version&&<p className="task-review-warning">{t('cancel_changed')}</p>}<div className="task-review-actions"><button type="button" disabled={locked} onClick={()=>setDraft(value=>({...value,cancel:undefined}))}>{t('back')}</button><button type="button" data-review-confirm-cancel disabled={locked||!pending?.can_cancel||pending.id!==draft.cancel.id||pending.version!==draft.cancel.version||!validReviewText(draft.cancel.reason)} onClick={()=>void cancel()}>{t('confirm_cancel')}</button></div></div>}
    {page&&<section className="task-review-history"><header><h4>{t('history',{loaded:formatNumber(page.items.length),total:formatNumber(page.total)})}</h4><button type="button" disabled={locked||history.loading} onClick={()=>void history.read()} aria-label={t('refresh')}><RefreshCw size={14} aria-hidden="true"/></button></header>{page.items.filter(item=>item.id!==pending?.id).map(item=>requestCard(item.id===selectedRequest&&located.value&&located.value.request.version>=item.version?located.value.request:item))}{page.next_cursor&&<button type="button" disabled={locked||history.loading} data-review-more onClick={()=>void history.read(true)}>{t('more')}</button>}</section>}
   </>}
  </div>}
 </section>;
}

function ReviewTargetPicker({project,taskID,selectedID,validityKey,disabled,refresh,onChange,onValidity,onDenied}:{project:string;taskID:string;selectedID:string;validityKey:string;disabled:boolean;refresh:number;onChange:(id:string)=>void;onValidity:(key:string,valid:boolean)=>void;onDenied:(error:unknown)=>void}){
 const id=useId(),[search,setSearch]=useState(''),[q,setQ]=useState(''),[error,setError]=useState(false),[expanded,setExpanded]=useState(false);
 const valid=useCallback((value:unknown):value is TaskReviewTargetPage=>validReviewTargets(value,project,taskID),[project,taskID]);
 const targets=useReviewRead(expanded?reviewTargetsPath(project,taskID,q):undefined,refresh,valid,onDenied,mergeTargets,targetCursor);
 const selectedValidator=useCallback((value:unknown):value is TaskReviewTargetPage=>validReviewTargets(value,project,taskID,selectedID),[project,taskID,selectedID]);
 const selected=useReviewRead(selectedID?reviewTargetsPath(project,taskID,'',selectedID):undefined,refresh,selectedValidator,onDenied),person=selected.value?.items[0];
 useEffect(()=>{onValidity(validityKey,!!selectedID&&!!person);},[validityKey,selectedID,person,onValidity]);
 return <div className="task-review-targets"><label>{t('reviewer')}</label>{selectedID&&<div className="task-review-selected" data-review-selected={selectedID}><span>{person?.name||(selected.loading?t('checking'):t('unavailable'))} · <code>{selectedID}</code></span><button type="button" disabled={disabled} onClick={()=>onChange('')}>{t('remove')}</button></div>}{!!selected.error&&<ErrorNotice error={selected.error} onRetry={()=>void selected.read()}/>}
  <button type="button" disabled={disabled} aria-expanded={expanded} aria-controls={id+'-list'} data-review-target-toggle onClick={()=>setExpanded(value=>!value)}>{t('choose_reviewer')}</button>
  {expanded&&<div id={id+'-list'}><label htmlFor={id+'-search'}>{t('search_label')}</label><div className="task-review-search"><input id={id+'-search'} type="search" value={search} maxLength={400} disabled={disabled} onChange={event=>setSearch(event.target.value)} onKeyDown={event=>{if(event.key==='Enter'){event.preventDefault();if(validReviewQuery(search)){setQ(search);setError(false);if(q===search)void targets.read();}else setError(true);}}}/><button type="button" disabled={disabled||targets.loading} onClick={()=>{if(!validReviewQuery(search)){setError(true);return;}setQ(search);setError(false);if(q===search)void targets.read();}}><Search size={13} aria-hidden="true"/>{t('search')}</button></div>
   {error&&<p role="alert">{t('invalid_query')}</p>}{targets.loading&&<Loading/>}{!!targets.error&&<ErrorNotice error={targets.error} onRetry={()=>void targets.read()}/>}{targets.value&&<><p className="task-review-muted">{t('target_count',{loaded:formatNumber(targets.value.items.length),total:formatNumber(targets.value.total)})}</p><ul>{targets.value.items.map(item=><li key={item.resource_id}><button type="button" disabled={disabled} aria-pressed={item.resource_id===selectedID} data-review-target={item.resource_id} onClick={()=>onChange(item.resource_id)}>{item.name||item.resource_id}<small>{item.resource_id}</small></button></li>)}</ul>{!targets.value.items.length&&<p className="task-review-muted">{t('no_targets')}</p>}{targets.value.next_cursor&&<button type="button" disabled={disabled||targets.loading} data-review-target-more onClick={()=>void targets.read(true)}>{t('more_targets')}</button>}</>}
  </div>}
 </div>;
}
