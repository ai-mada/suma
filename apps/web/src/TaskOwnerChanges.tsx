import {useEffect,useRef,useState,type FormEvent} from 'react';
import {ArrowRight,UserRoundCheck} from 'lucide-react';
import type {TaskOwnerChangePreview,TaskOwnerChangePreviewRequest,TaskOwnerChangeResult,TaskOwnerSelection,TaskOwnerTarget,TaskOwnerTargetPage} from '../../../packages/contracts/src';
import {ApiError,label,query,stateLabel,type RecordItem} from './api';
import {useApi} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {toolSessionStorage} from './tool-hooks';
import {ErrorNotice,Field,Loading,Modal} from './ui';
import {useLeaveGuard,useWriteAction,WriteNotice} from './write-action';
import {appendOwnerTargets,ownerTargetsPath,ownerVersion,validOwnerPreview,validOwnerQuery,validOwnerRecovery,validOwnerResult,validOwnerSelection,validOwnerTargets} from './task-owner-changes-model';
import './TaskOwnerChanges.css';

export type TaskOwnerChangesProps={project:string;userID:string;organizationID:string;tasks:RecordItem[];onChanged:()=>Promise<void>|void;onForbidden:(error:unknown)=>void;disabled?:boolean;authorityError?:unknown;onAuthorityRestored?:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('task-owner-changes.'+key,values);
const refused=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const selection=(tasks:RecordItem[])=>tasks.map(task=>({task_id:task.id,expected_version:task.version}));

/** Keep this wrapper mounted even when selection is empty: recovery is scoped
 * to the original person and project, independently of a page's checkbox state. */
export function TaskOwnerChanges(props:TaskOwnerChangesProps){
 return <OwnerChangeScope key={JSON.stringify([props.userID,props.organizationID,props.project])} {...props}/>;
}

function OwnerChangeScope({project,userID,organizationID,tasks,onChanged,onForbidden,disabled,authorityError,onAuthorityRestored}:TaskOwnerChangesProps){
 useLocale();const api=useApi(),currentApi=useRef(api);currentApi.current=api;
 const [denied,setDenied]=useState(false),blockedRef=useRef(false),writeRetired=useRef(false),parentDenied=useRef(false);blockedRef.current=!!authorityError||denied;const authorityBlocked=blockedRef.current;
 if(authorityError)writeRetired.current=true;
 const action=useWriteAction({scope:JSON.stringify(['task-owner-changes',userID,organizationID,project]),storage:toolSessionStorage(),validateRequest:request=>validOwnerRecovery(request,project),validateResponse:(value,request)=>!blockedRef.current&&!writeRetired.current&&validOwnerResult(value,request,project)});
 const [open,setOpen]=useState(false),[basis,setBasis]=useState<TaskOwnerSelection[]>([]),[names,setNames]=useState<Record<string,string>>({});
 const [owner,setOwner]=useState<TaskOwnerTarget>(),[targets,setTargets]=useState<TaskOwnerTargetPage>(),[search,setSearch]=useState(''),[pageQuery,setPageQuery]=useState(''),[directoryBusy,setDirectoryBusy]=useState(false);
 const [preview,setPreview]=useState<{value:TaskOwnerChangePreview;request:TaskOwnerChangePreviewRequest}>(),[previewBusy,setPreviewBusy]=useState(false),[ack,setAck]=useState(false),[needsRefresh,setNeedsRefresh]=useState(false),[stale,setStale]=useState(false);
 const [readError,setReadError]=useState<unknown>(),[receipt,setReceipt]=useState<TaskOwnerChangeResult>(),[refreshError,setRefreshError]=useState<unknown>();
 const [accessBusy,setAccessBusy]=useState(false),[accessError,setAccessError]=useState<unknown>(),accessEpoch=useRef(0),accessController=useRef<AbortController|undefined>(undefined);
 const alive=useRef(true),directoryEpoch=useRef(0),previewEpoch=useRef(0),directoryController=useRef<AbortController|undefined>(undefined),previewController=useRef<AbortController|undefined>(undefined),basisSource=useRef(''),forbidden=useRef(onForbidden);
 forbidden.current=onForbidden;
 const signature=JSON.stringify(selection(tasks)),signatureRef=useRef(signature);signatureRef.current=signature;
 const pending=action.pending,locked=action.busy||!!pending||action.persistenceBlocked;
 useLeaveGuard(previewBusy,action.setError);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;directoryEpoch.current++;previewEpoch.current++;accessEpoch.current++;directoryController.current?.abort();previewController.current?.abort();accessController.current?.abort();};},[]);
 const clearProtected=()=>{
  directoryEpoch.current++;previewEpoch.current++;directoryController.current?.abort();previewController.current?.abort();
  accessEpoch.current++;accessController.current?.abort();setAccessBusy(false);setAccessError(undefined);
  setTargets(undefined);setOwner(undefined);setPreview(undefined);setReceipt(undefined);setNames({});setBasis([]);setSearch('');setPageQuery('');setAck(false);setDirectoryBusy(false);setPreviewBusy(false);setReadError(undefined);setRefreshError(undefined);
 };
 const deny=(error:unknown)=>{
  if(blockedRef.current)return;blockedRef.current=true;writeRetired.current=true;clearProtected();setDenied(true);forbidden.current(error);
 };
 useEffect(()=>{if(authorityError){parentDenied.current=true;clearProtected();}else if(parentDenied.current){parentDenied.current=false;setDenied(false);}},[authorityError]);
 useEffect(()=>{if(!blockedRef.current&&action.errorStatus&&[401,403,404].includes(action.errorStatus))deny(new ApiError(action.errorStatus,'access_changed',t('access_changed')));},[action.errorStatus]);
 useEffect(()=>{if(open&&!pending&&!receipt&&basisSource.current!==signature){previewEpoch.current++;previewController.current?.abort();setPreviewBusy(false);setStale(true);setPreview(undefined);setAck(false);}},[signature,open,!!pending,!!receipt]);

 const checkAccess=async()=>{
  if(!blockedRef.current||accessBusy||action.busy)return;
  const epoch=++accessEpoch.current;accessController.current?.abort();const abort=new AbortController();accessController.current=abort;setAccessBusy(true);setAccessError(undefined);
  const live=()=>alive.current&&currentApi.current===api&&epoch===accessEpoch.current;
  try{
   const value=await api<unknown>('/task-owner-targets?'+new URLSearchParams({project_id:project,limit:'1'}),undefined,'GET',{'X-Suma-Expected-User':userID,'X-Suma-Expected-Organization':organizationID},abort.signal);
   if(!live())return;
   if(!validOwnerTargets(value,project,1))throw new UiError('task-owner-changes.access_check_invalid');
   // This read proves current write eligibility only. Do not retain its people
   // or release the retired original write without the explicit Restore action.
   setDenied(false);onAuthorityRestored?.();
  }catch(error){if(live())setAccessError(error);}
  finally{if(live())setAccessBusy(false);}
 };

 const readTargets=async(q:string,next='',previous?:TaskOwnerTargetPage)=>{
  if(locked||blockedRef.current)return;
  const epoch=++directoryEpoch.current;directoryController.current?.abort();const abort=new AbortController();directoryController.current=abort;setDirectoryBusy(true);setReadError(undefined);if(!next)setTargets(undefined);
  try{
   const value=await api<unknown>(ownerTargetsPath(project,q,next),undefined,'GET',undefined,abort.signal);
   if(!alive.current||blockedRef.current||currentApi.current!==api||epoch!==directoryEpoch.current)return;
   if(!validOwnerTargets(value,project))throw new UiError('task-owner-changes.invalid_read');
   const page=previous?appendOwnerTargets(previous,value):value;if(!page)throw new UiError('task-owner-changes.invalid_read');
   setTargets(page);setPageQuery(q);
  }catch(error){if(alive.current&&!blockedRef.current&&currentApi.current===api&&epoch===directoryEpoch.current){if(refused(error))deny(error);else setReadError(error);}}
  finally{if(alive.current&&!blockedRef.current&&currentApi.current===api&&epoch===directoryEpoch.current)setDirectoryBusy(false);}
 };
 const launch=()=>{
  if(authorityError){setOpen(true);return;}
  if(!pending&&!action.persistenceBlocked){const selected=selection(tasks);setBasis(selected);setNames(Object.fromEntries(tasks.map(task=>[task.id,label(task)])));basisSource.current=signature;setStale(false);setPreview(undefined);setAck(false);setReceipt(undefined);setRefreshError(undefined);setNeedsRefresh(false);setDenied(false);setOwner(undefined);setSearch('');action.clearConflict();}
  setOpen(true);
 };
 useEffect(()=>{if(open&&!locked&&!blockedRef.current&&!receipt)void readTargets('');},[open]);
 const submitSearch=(event:FormEvent)=>{event.preventDefault();if(validOwnerQuery(search))void readTargets(search);};
 const showPreview=async(refresh=false)=>{
  if(disabled||locked||blockedRef.current||previewBusy||!owner||!validOwnerSelection(basis)||!refresh&&(needsRefresh||stale||action.conflict))return;
  const epoch=++previewEpoch.current;previewController.current?.abort();const abort=new AbortController();previewController.current=abort;setPreviewBusy(true);setPreview(undefined);setAck(false);setReadError(undefined);
  try{
   let selected=basis,target=owner;
   if(refresh){
    const [current,directory]=await Promise.all([
     Promise.all(basis.map(async item=>{
      const row=await api<RecordItem>('/tasks/'+encodeURIComponent(item.task_id)+query(project),undefined,'GET',undefined,abort.signal);
      if(!row||row.id!==item.task_id||row.project_id!==project||!ownerVersion(row.version))throw new UiError('task-owner-changes.invalid_read');return {task_id:row.id,expected_version:row.version};
     })),
     api<unknown>(ownerTargetsPath(project,Array.from(owner.name).slice(0,200).join('')),undefined,'GET',undefined,abort.signal)
    ]);
    if(!alive.current||blockedRef.current||currentApi.current!==api||epoch!==previewEpoch.current)return;
    if(!validOwnerTargets(directory,project))throw new UiError('task-owner-changes.invalid_read');
    setTargets(directory);setPageQuery(Array.from(owner.name).slice(0,200).join(''));const fresh=directory.items.find(item=>item.resource_id===owner.resource_id);
    if(!fresh){setOwner(undefined);throw new UiError('task-owner-changes.target_unavailable');}
    selected=current;target=fresh;setBasis(selected);setOwner(target);basisSource.current=signatureRef.current;setStale(false);
   }
   const request:TaskOwnerChangePreviewRequest={project_id:project,owner_id:target.resource_id,expected_owner_version:target.resource_version,tasks:selected};
   const value=await api<unknown>('/task-owner-changes/preview',request,'POST',undefined,abort.signal);
   if(!alive.current||blockedRef.current||currentApi.current!==api||epoch!==previewEpoch.current)return;
   if(!validOwnerPreview(value,request))throw new UiError('task-owner-changes.invalid_preview');
   setPreview({value,request});setNeedsRefresh(false);action.clearConflict();
  }catch(error){if(alive.current&&!blockedRef.current&&currentApi.current===api&&epoch===previewEpoch.current){if(refused(error))deny(error);else{setReadError(error);if(error instanceof ApiError&&error.status===409)setNeedsRefresh(true);}}}
  finally{if(alive.current&&!blockedRef.current&&currentApi.current===api&&epoch===previewEpoch.current)setPreviewBusy(false);}
 };
 const confirmed=async(value:TaskOwnerChangeResult|undefined)=>{
  if(!value||!alive.current||blockedRef.current||currentApi.current!==api)return;
  setReceipt(value);setPreview(undefined);setAck(false);setNeedsRefresh(false);setStale(false);setReadError(undefined);setRefreshError(undefined);
  try{await onChanged();}catch(error){if(alive.current&&!blockedRef.current&&currentApi.current===api){if(refused(error))deny(error);else setRefreshError(new UiError('task-owner-changes.refresh_failed'));}}
 };
 const apply=async()=>{
  if(disabled||locked||blockedRef.current||needsRefresh||stale||action.conflict||!preview?.value.can_apply||!ack)return;
  writeRetired.current=false;
  await confirmed(await action.run<TaskOwnerChangeResult>('/task-owner-changes',{...preview.request,preview_sha256:preview.value.preview_sha256,acknowledge_invalidation:true}));
 };
 const restore=async()=>{const original=action.pending;if(original&&!blockedRef.current&&!action.busy){writeRetired.current=false;await confirmed(await action.run<TaskOwnerChangeResult>(original.path,original.body,original.method));}};
 const targetSelected=targets?.items.some(item=>item.resource_id===owner?.resource_id);
 const canPreview=validOwnerSelection(basis)&&!!owner&&!disabled&&!locked&&!authorityBlocked&&!directoryBusy&&!previewBusy;
 const mustRefresh=needsRefresh||stale||action.conflict;
 const launchVisible=tasks.length>0||!!pending||action.persistenceBlocked;
 return <>
  {launchVisible&&<button type="button" data-owner-change-launch disabled={!pending&&!action.persistenceBlocked&&disabled} onClick={launch}><UserRoundCheck size={16} aria-hidden="true"/>{pending||action.persistenceBlocked?t('recover'):t('launch',{count:formatNumber(tasks.length)})}</button>}
  <Modal open={open} title={t('title')} description={t('intro')} onClose={()=>{if(!locked&&!previewBusy&&!accessBusy&&window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))setOpen(false);}}>
   <div className="task-owner-changes" data-task-owner-changes>
    {!authorityBlocked&&<><WriteNotice action={action}/>{Boolean(readError)&&<ErrorNotice error={readError}/>}</>}
    {authorityBlocked?<section className="task-owner-callout"><p role="status">{t('access_changed')}</p>{Boolean(accessError)&&<ErrorNotice error={accessError}/>}<button type="button" data-owner-check-access disabled={accessBusy||action.busy} onClick={()=>void checkAccess()}>{t(accessBusy?'checking_access':'check_access')}</button></section>:pending?<section className="task-owner-callout" role="status"><strong>{t('original_operation')}</strong><p>{t('recovery_help')}</p><dl><dt>{t('operation')}</dt><dd><code>{pending.idempotency_key}</code></dd><dt>{t('selected')}</dt><dd>{formatNumber((pending.body.tasks as TaskOwnerSelection[]).length)}</dd></dl><details><summary>{t('original_request')}</summary><pre>{JSON.stringify(pending.body,null,2)}</pre></details><button type="button" data-owner-restore disabled={action.busy} onClick={()=>void restore()}>{t('restore')}</button></section>:action.persistenceBlocked?<p role="status">{t('storage_help')}</p>:receipt?<section data-owner-receipt className="task-owner-receipt" aria-live="polite"><strong>{t('saved')}</strong><p>{t('saved_help')}</p>{Boolean(refreshError)&&<ErrorNotice error={refreshError}/>}<details><summary>{t('receipt')}</summary><dl><dt>{t('operation')}</dt><dd><code>{receipt.operation_id}</code></dd></dl>{receipt.changes.map(item=><article className="task-owner-card" key={item.task_id}><strong>{names[item.task_id]||item.task_id}</strong><span>{t(item.owner_changed?'owner_changed':'owner_unchanged')}</span><span>{t('versions',{before:formatNumber(item.previous_version),after:formatNumber(item.version)})} · {stateLabel(item.status)}</span></article>)}</details></section>:<>
     <p className="task-owner-scope">{t('selection',{count:formatNumber(basis.length)})}</p>
     {!validOwnerSelection(basis)&&<p role="status">{t('invalid_selection')}</p>}
     <form className="task-owner-search" onSubmit={submitSearch}><Field label={t('search_label')}><input name="task_owner_search" value={search} onChange={event=>setSearch(event.target.value)} disabled={previewBusy||directoryBusy} aria-invalid={!validOwnerQuery(search)}/></Field><button type="submit" disabled={previewBusy||directoryBusy||!validOwnerQuery(search)}>{t('search')}</button></form>
     {!validOwnerQuery(search)&&<p className="task-owner-hint" role="status">{t('search_invalid')}</p>}
     <Field label={t('owner')}><select name="task_owner_target" value={owner?.resource_id||''} disabled={directoryBusy||previewBusy} onChange={event=>{setOwner(targets?.items.find(item=>item.resource_id===event.target.value));setPreview(undefined);setAck(false);}}><option value="">{t('choose_owner')}</option>{owner&&!targetSelected&&<option value={owner.resource_id}>{owner.name}</option>}{targets?.items.map(item=><option key={item.resource_id} value={item.resource_id}>{item.name}</option>)}</select></Field>
     {directoryBusy&&<Loading/>}{targets&&<div className="task-owner-directory"><span>{t('loaded',{loaded:formatNumber(targets.items.length),total:formatNumber(targets.total)})}</span>{targets.next_cursor&&<button type="button" disabled={directoryBusy||previewBusy} onClick={()=>void readTargets(pageQuery,targets.next_cursor,targets)}>{t('more_people')}</button>}{!targets.items.length&&<p>{t('no_people')}</p>}</div>}
     {mustRefresh&&<p className="task-owner-callout" role="status">{t('changed_facts')}</p>}
     <button type="button" data-owner-preview={mustRefresh?undefined:''} data-owner-refresh-preview={mustRefresh?'':undefined} disabled={!canPreview} onClick={()=>void showPreview(mustRefresh)}>{previewBusy?t('previewing'):mustRefresh?t('refresh_preview'):t('preview')}</button>
     {preview&&<section className="task-owner-preview" aria-live="polite"><h3>{t('preview_title',{changed:formatNumber(preview.value.changed_count),unchanged:formatNumber(preview.value.unchanged_count)})}</h3><p className="task-owner-impact">{t('impact')}</p><p className="task-owner-hint">{t('unchanged_impact')}</p>{preview.value.tasks.map(item=><article className="task-owner-card" data-owner-preview-task={item.task_id} key={item.task_id}><div className="task-owner-heading"><strong>{item.title||item.task_id}</strong><span>{t(item.will_change?'will_change':'owner_unchanged')}</span></div><div className="task-owner-transfer"><span>{item.owner_name||item.owner_id||t('unassigned')}</span><ArrowRight size={14} aria-hidden="true"/><strong>{preview.value.owner.name}</strong></div><details><summary>{t('task_facts')}</summary><dl><dt>{t('task')}</dt><dd><code>{item.task_id}</code></dd><dt>{t('version')}</dt><dd>{formatNumber(item.version)}</dd><dt>{t('status')}</dt><dd>{stateLabel(item.status)}</dd>{item.run_id&&<><dt>{t('run')}</dt><dd><code>{item.run_id}</code></dd></>}</dl><p>{t(item.will_change?'version_effect':'unchanged_version_effect')}</p></details></article>)}{!preview.value.can_apply&&<p role="status">{t('no_changes')}</p>}<label className="task-owner-ack"><input type="checkbox" name="acknowledge_owner_change" checked={ack} disabled={disabled||!preview.value.can_apply} onChange={event=>setAck(event.target.checked)}/><span>{t('acknowledge')}</span></label><button type="button" className="primary" data-owner-apply disabled={disabled||!preview.value.can_apply||!ack||locked||mustRefresh} onClick={()=>void apply()}>{t('apply',{count:formatNumber(preview.value.changed_count)})}</button></section>}
    </>}
   </div>
  </Modal>
 </>;
}
