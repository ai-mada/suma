import {useEffect,useId,useRef,useState,type FormEvent} from 'react';
import {Share2,RefreshCw} from 'lucide-react';
import type {CloudHistoryLoad,CloudHistoryEntry,CloudHistorySharePreview,CloudHistorySharePreviewRequest,TaskConversationMessage} from '../../../packages/contracts/src';
import {ApiError,type RecordItem} from './api';
import {useApi,useApiView} from './api-context';
import {formatNumber,tr,useLocale} from './i18n';
import {Modal} from './ui';
import {useWriteAction} from './write-action';
import {toolSessionStorage} from './tool-hooks';
import {defaultTaskView,taskFilterText,taskReadDenied,useTaskWorkspacePage} from './task-views-model';
import {validConversationMessageDetail} from './task-conversation-model';
import {eligibleHistoryEntry,historySharePath,historySharePreviewPath,readHistorySharePreview,readHistoryShareReceipt,validHistoryShareRequest,validHistoryShareSequences} from './cloud-history-sharing-model';
import './CloudHistorySharing.css';

export type CloudHistoryShareSelection={load:CloudHistoryLoad;entries:CloudHistoryEntry[];sequences:number[]};
type TaskChoice={id:string;title:string;version:number};
type Props={project:string;runID:string;nodeID:string;selection?:CloudHistoryShareSelection;onClearSelection:()=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('workbuddy-history-sharing.'+key,values);
const denied=(error:unknown)=>error instanceof ApiError&&[401,403,404].includes(error.status);
const choice=(row:RecordItem):TaskChoice|undefined=>typeof row.id==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]{1,64}$/.test(row.id)&&Number.isSafeInteger(row.version)&&row.version>0&&(row.title===undefined||typeof row.title==='string')?{id:row.id,title:row.title||row.id,version:row.version}:undefined;

/** Target selection is the existing authorized task query, mounted on demand. */
function SharingTaskPicker({project,value,onChange,onReady,disabled}:{project:string;value?:TaskChoice;onChange:(value?:TaskChoice)=>void;onReady:(ready:boolean)=>void;disabled:boolean}){
 const api=useApi(),view=useApiView(),id=useId(),[draft,setDraft]=useState(''),[query,setQuery]=useState(''),[refresh,setRefresh]=useState(0),[badSearch,setBadSearch]=useState(false);
 const page=useTaskWorkspacePage(api,[view?.userID||'',view?.organizationID||'',project],{...defaultTaskView,q:query},refresh,!view);
 const callbacks=useRef({onChange,onReady});callbacks.current={onChange,onReady};
 const rows=page.items||[],options=rows.map(choice),invalid=options.some(item=>!item),selected=options.find(item=>item?.id===value?.id);
 useEffect(()=>{callbacks.current.onReady(!!selected&&!page.loading&&!page.error&&!invalid);},[selected?.id,selected?.version,page.loading,!!page.error,invalid]);
 useEffect(()=>{if(taskReadDenied(page.error))callbacks.current.onChange(undefined);else if(!page.loading&&!page.error&&value){if(!selected)callbacks.current.onChange(undefined);else if(selected.version!==value.version||selected.title!==value.title)callbacks.current.onChange(selected);}},[page.loading,!!page.error,selected?.id,selected?.version,selected?.title]);
 const search=(event:FormEvent)=>{event.preventDefault();if(draft!==''&&!taskFilterText(draft)){setBadSearch(true);return;}setBadSearch(false);setQuery(draft);setRefresh(value=>value+1);};
 return <div className="history-sharing-task-picker">
  <form onSubmit={search}><label htmlFor={id+'-search'}>{t('search_tasks')}</label><div className="history-sharing-search"><input id={id+'-search'} value={draft} maxLength={400} disabled={disabled} onChange={event=>setDraft(event.target.value)}/><button type="submit" disabled={disabled||page.loading} data-history-share-action="search">{t('search')}</button></div></form>
  {badSearch&&<p role="alert">{t('search_invalid')}</p>}
  <label htmlFor={id+'-task'}>{t('target_task')}</label><select id={id+'-task'} data-history-share-task value={value?.id||''} disabled={disabled||page.loading||!!page.error||invalid} onChange={event=>onChange(options.find(item=>item?.id===event.target.value))}><option value="">{t('choose_task')}</option>{options.map(item=>item&&<option key={item.id} value={item.id}>{item.title}</option>)}</select>
  {page.loading&&<p role="status">{t('reading_tasks')}</p>}{(page.error||invalid)&&<p role="alert">{t('tasks_failed')}</p>}
  {!page.loading&&!page.error&&rows.length===0&&<p>{t('tasks_empty')}</p>}
  <div className="history-sharing-actions"><button type="button" disabled={disabled||page.loading} onClick={()=>setRefresh(value=>value+1)} data-history-share-action="tasks-read"><RefreshCw size={15} aria-hidden="true"/>{t('read_tasks')}</button>{page.next_cursor&&<button type="button" disabled={disabled||page.loading} onClick={()=>void page.more()} data-history-share-action="tasks-more">{t('more_tasks')}</button>}</div>
 </div>;
}

/** The journal survives private-source invalidation; private preview text does not. */
export function CloudHistorySharing(props:Props){
 const view=useApiView();return <SharingController key={JSON.stringify([view?.userID,view?.organizationID,props.project,props.runID,props.nodeID])} {...props}/>;
}
function SharingController({project,runID,nodeID,selection,onClearSelection}:Props){
 useLocale();const api=useApi(),view=useApiView(),id=useId(),[task,setTask]=useState<TaskChoice>(),[pickerReady,setPickerReady]=useState(false),[preview,setPreview]=useState<CloudHistorySharePreview>(),[consent,setConsent]=useState(false),[reading,setReading]=useState(false),[issue,setIssue]=useState(''),[saved,setSaved]=useState<{message:TaskConversationMessage;taskID:string}>();
 const alive=useRef(true),owner=useRef(api),previewRef=useRef<CloudHistorySharePreview|undefined>(undefined),selectionRef=useRef(selection),generation=useRef(0),abort=useRef<AbortController|undefined>(undefined),busy=useRef(false),callbacks=useRef(onClearSelection);
 owner.current=api;previewRef.current=preview;selectionRef.current=selection;callbacks.current=onClearSelection;
 const selectionKey=selection?JSON.stringify([selection.load.id,selection.load.version,selection.sequences]):'',currentKey=useRef(selectionKey);currentKey.current=selectionKey;
 const clearPreview=()=>{++generation.current;abort.current?.abort();previewRef.current=undefined;setPreview(undefined);setConsent(false);setReading(false);};
 const scope=JSON.stringify([view?.userID,view?.organizationID,project,runID,nodeID]);
 const action=useWriteAction({scope:'workbuddy-history-share:'+scope,storage:toolSessionStorage(),validateRequest:request=>validHistoryShareRequest(request,project),validateResponse:async(value,request)=>{
  const originalPreview=previewRef.current?.preview_sha256===request.body.preview_sha256?previewRef.current:undefined;
  const message=await readHistoryShareReceipt(value,request,view?.userID||'',{runID,nodeID},originalPreview);if(!message)return false;
  try{
   const taskID=String(request.body.task_id),raw=await api<unknown>('/task-conversation-messages/'+encodeURIComponent(message.id)+'?project_id='+encodeURIComponent(project)+'&task_id='+encodeURIComponent(taskID),undefined,'GET',{'X-Suma-Expected-User':view?.userID||'','X-Suma-Expected-Organization':view?.organizationID||''});
   return validConversationMessageDetail(raw,project,taskID,message.id)&&raw.message.sequence===message.sequence&&raw.message.body===message.body&&!!await readHistoryShareReceipt(raw.message,request,view?.userID||'',{runID,nodeID},originalPreview);
  }catch(error){if(alive.current&&owner.current===api&&denied(error)){clearPreview();setIssue('denied');callbacks.current();}return false;}
 }});
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++generation.current;abort.current?.abort();};},[]);
 useEffect(()=>{clearPreview();setIssue('');},[selectionKey]);
 useEffect(()=>{if(action.errorStatus&&[401,403,404,409].includes(action.errorStatus)){clearPreview();setIssue(action.errorStatus===409?'changed':'denied');}},[action.errorStatus]);
 const validSelection=!!selection&&selection.load.source.project_id===project&&selection.load.source.run_id===runID&&selection.load.source.node_id===nodeID&&validHistoryShareSequences(selection.sequences)&&selection.entries.length===selection.sequences.length&&selection.entries.every((entry,index)=>entry.sequence===selection.sequences[index]&&eligibleHistoryEntry(selection.load,entry));
 // Remove private text in this commit, before passive cleanup. The metadata-only
 // journal remains usable when the current source can no longer be displayed.
 const visiblePreview=preview&&selection&&validSelection&&preview.task_id===task?.id&&preview.task_version===task.version&&preview.load_id===selection.load.id&&preview.load_version===selection.load.version&&Object.entries(preview.source).every(([key,value])=>selection.load.source[key as keyof typeof selection.load.source]===value)&&preview.entries.length===selection.entries.length&&preview.entries.every((entry,index)=>entry.sequence===selection.entries[index].sequence&&entry.kind===selection.entries[index].kind&&entry.text===selection.entries[index].text)?preview:undefined;
 previewRef.current=visiblePreview;
 const chooseTask=(value?:TaskChoice)=>{setTask(value);clearPreview();setIssue('');};
 const prepare=async()=>{
  const selected=selectionRef.current;if(busy.current||action.busy||action.pending||!validSelection||!selected||!task||!pickerReady)return;
  clearPreview();setSaved(undefined);setIssue('');const epoch=++generation.current,key=currentKey.current,controller=new AbortController();abort.current=controller;setReading(true);
  const request:CloudHistorySharePreviewRequest={project_id:project,task_id:task.id,expected_task_version:task.version,expected_load_version:selected.load.version,sequences:[...selected.sequences]};
  try{
   const raw=await api<unknown>(historySharePreviewPath(selected.load.id),request,'POST',undefined,controller.signal);
   if(!alive.current||owner.current!==api||epoch!==generation.current||key!==currentKey.current)return;
   const value=await readHistorySharePreview(raw,selected.load,request,selected.entries);
   if(!alive.current||owner.current!==api||epoch!==generation.current||key!==currentKey.current)return;
   if(!value)throw Error();action.clearConflict();previewRef.current=value;setPreview(value);
  }catch(error){if(!alive.current||owner.current!==api||epoch!==generation.current||controller.signal.aborted)return;setIssue(denied(error)?'denied':error instanceof ApiError&&error.status===409?'changed':error instanceof ApiError&&error.status===400?'selection_invalid':'preview_failed');}
  finally{if(alive.current&&owner.current===api&&epoch===generation.current)setReading(false);}
 };
 const share=async(recover=false)=>{
  if(busy.current||action.busy||!view)return;
  const original=recover?action.pending:undefined,pinned=previewRef.current;
  if(recover&&!original||!recover&&(!pinned||!consent||action.pending||action.conflict))return;
  const path=original?.path||historySharePath(pinned!.load_id),body=original?.body||{project_id:project,task_id:pinned!.task_id,expected_task_version:pinned!.task_version,expected_load_version:pinned!.load_version,sequences:pinned!.entries.map(entry=>entry.sequence),preview_sha256:pinned!.preview_sha256};
  busy.current=true;setIssue('');
  try{
   const message=await action.run<TaskConversationMessage>(path,body,original?.method||'POST');
   if(!alive.current||owner.current!==api)return;
   if(message){setSaved({message,taskID:String(body.task_id)});clearPreview();callbacks.current();}
  }finally{busy.current=false;}
 };
 const pendingNotice=action.pending?<div className="history-sharing-notice" role="status"><strong>{t('pending_title')}</strong><p>{t('pending_hint')}</p><button type="button" disabled={action.busy} onClick={()=>void share(true)} data-history-share-action="recover">{t('recover')}</button></div>:null;
 if(!selection&&!action.pending&&!saved&&!issue&&!action.error)return null;
 return <section className="cloud-history-sharing" aria-labelledby={id+'-title'} aria-busy={reading||action.busy} data-history-sharing>
  <header><Share2 size={18} aria-hidden="true"/><h4 id={id+'-title'}>{t('title')}</h4></header>
  {selection&&<><p>{t('selected_count',{count:formatNumber(selection.sequences.length)})}</p><p className="history-sharing-scope" data-history-share-scope>{t('scope')}</p></>}
  {!visiblePreview&&pendingNotice}
  {issue&&<p className="history-sharing-notice" role="alert">{t(issue)}</p>}
  {action.error&&!action.pending&&!issue&&<p role="alert">{t(action.persistenceBlocked?'recovery_blocked':action.conflict?'changed':'share_failed')}</p>}
  {saved&&<div className="history-sharing-saved" role="status"><p>{t('shared')}</p><a href={'?'+new URLSearchParams({project,view:'development',sub:'ide',panel:'workbench-task:'+saved.taskID,conversation_message:saved.message.id})}>{t('open_message')}</a></div>}
  {selection&&!action.pending&&<><SharingTaskPicker project={project} value={task} onChange={chooseTask} onReady={setPickerReady} disabled={reading||action.busy}/>{!validSelection&&<p role="alert">{t('selection_invalid')}</p>}<button type="button" onClick={()=>void prepare()} disabled={!validSelection||!task||!pickerReady||reading||action.busy||action.persistenceBlocked} data-history-share-action="preview">{t(reading?'previewing':'preview')}</button></>}
  <Modal open={!!visiblePreview} onClose={()=>{if(!action.busy)clearPreview();}} title={t('confirm_title')} description={t('scope')}>
   {visiblePreview&&<div className="history-sharing-preview"><strong>{visiblePreview.task_title}</strong><p>{t('preview_basis',{version:formatNumber(visiblePreview.task_version),state:t('load_state_'+visiblePreview.load_state)})}</p><p>{t('coverage')}</p><pre tabIndex={0} data-history-share-preview aria-label={t('preview_body')}>{visiblePreview.body}</pre><p className="history-sharing-scope" data-history-share-scope>{t('scope')}</p>
    {pendingNotice}{issue&&<p role="alert">{t(issue)}</p>}
    <label className="history-sharing-consent"><input type="checkbox" checked={consent} disabled={action.busy||!!action.pending} data-history-share-confirm-scope onChange={event=>setConsent(event.target.checked)}/><span>{t('confirm_scope')}</span></label>
    <button type="button" className="primary" data-history-share-action="confirm" disabled={!consent||action.busy||!!action.pending||action.conflict} onClick={()=>void share()}>{t(action.busy?'sharing':'confirm')}</button>
   </div>}
  </Modal>
 </section>;
}
