import {useCallback,useEffect,useRef,useState} from 'react';
import type {Worklog,WorklogPage} from '../../../packages/contracts/src';
import {query,label,type RecordItem} from './api';
import {useApi,useApiView} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Field,Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard,useWriteAction,WriteButton,WriteFields,WriteNotice} from './write-action';
import {editWorklogDraft,newWorklogDraft,validWorklog,validWorklogPage,validWorklogRecord,validWorklogRecords,validWorklogRequest,validWorklogReceipt,worklogDraftError,worklogPagePath,worklogPath,worklogScope,type WorklogDraft,type WorklogMode} from './worklogs-model';
import {useWorklogRead,worklogAccessLost} from './worklogs-read';

const t=(key:string,values?:Record<string,unknown>)=>tr('worklogs.'+key,values);
type Authority=Pick<WorklogPage,'can_create'|'can_manage_others'|'self_member_id'>;
type Props={project:string;week:string;mode:WorklogMode;row?:Worklog;authority:Authority;onSaved:()=>void;onCancel:()=>void;onDenied:()=>void};

export function WorklogsEditor(props:Props){
 const api=useApi(),view=useApiView(),caller=useRef({api,epoch:0});if(caller.current.api!==api)caller.current={api,epoch:caller.current.epoch+1};
 const scope=worklogScope(view?.userID??'',view?.organizationID??'',props.project,props.week);
 return <EditorScope key={JSON.stringify([scope,props.mode,props.row?.id,caller.current.epoch])} {...props} scope={scope}/>;
}

function EditorScope({project,week,mode,row,authority,onSaved,onCancel,onDenied,scope}:Props&{scope:string}){
 useLocale();const api=useApi(),alive=useRef(true),owner=useRef(api);owner.current=api;
 const [basis,setBasis]=useState(row),[capabilities,setCapabilities]=useState(authority),[draft,setDraft]=useState<WorklogDraft>(()=>row?editWorklogDraft(row):newWorklogDraft(week,authority.self_member_id)),[dirty,setDirty]=useState(false),[denied,setDenied]=useState(false),[basisReady,setBasisReady]=useState(mode==='create'),[readError,setReadError]=useState<unknown>('');
 const currentTask=useRef<RecordItem|undefined>(undefined),basisEpoch=useRef(0),[taskLoading,setTaskLoading]=useState(false);
 const action=useWriteAction({scope:'worklogs:'+scope,storage:toolSessionStorage(),validateRequest:request=>validWorklogRequest(request,project,mode,row?.id),validateResponse:(value,request)=>validWorklogReceipt(value,request,project,mode,row?.id,row?.member_id)});
 const loseAccess=useCallback(()=>{setDenied(true);setBasis(undefined);setReadError('');onDenied();},[onDenied]);
 const detailValidator=useCallback((value:unknown):value is Worklog=>validWorklog(value,project,row?.id),[project,row?.id]);
 const detail=useWorklogRead(mode==='create'?undefined:worklogPath(row?.id)+query(project),detailValidator,0,loseAccess);
 useEffect(()=>{if(detail.value){setBasis(detail.value);setBasisReady(true);}},[detail.value]);
 useEffect(()=>{if(detail.error)setReadError(detail.error);},[detail.error]);
 useEffect(()=>{if(action.errorStatus&&[401,403,404].includes(action.errorStatus))loseAccess();},[action.errorStatus,loseAccess]);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;basisEpoch.current++;};},[]);
 useLeaveGuard(dirty,action.setError);
 const update=(patch:Partial<WorklogDraft>)=>{setDraft(before=>({...before,...patch}));setDirty(true);action.setError('');};
 const readTask=useCallback(async(id:string,adopt=true)=>{
  const epoch=++basisEpoch.current,caller=api;if(!id){currentTask.current=undefined;return;}
  setTaskLoading(true);setReadError('');
  try{const value=await api<unknown>('/tasks/'+encodeURIComponent(id)+query(project));if(!alive.current||epoch!==basisEpoch.current||owner.current!==caller)return;if(!validWorklogRecord(value,project)||value.id!==id)throw new UiError('common.responseUnknown');currentTask.current=value;if(adopt)setDraft(before=>before.task_id===id?{...before,expected_task_version:value.version}:before);return value;}
  catch(error){if(alive.current&&epoch===basisEpoch.current){setReadError(error);if(worklogAccessLost(error))loseAccess();}throw error;}
  finally{if(alive.current&&epoch===basisEpoch.current)setTaskLoading(false);}
 },[api,project,loseAccess]);
 useEffect(()=>{if(mode==='correct'&&row?.task_id)void readTask(row.task_id).catch(()=>{});},[mode,row?.task_id,readTask]);
 const pending=action.pending?.body;
 const visible:WorklogDraft=pending?{...draft,member_id:typeof pending.member_id==='string'?pending.member_id:draft.member_id,task_id:typeof pending.task_id==='string'?pending.task_id:draft.task_id,expected_task_version:typeof pending.expected_task_version==='number'?pending.expected_task_version:draft.expected_task_version,work_date:typeof pending.work_date==='string'?pending.work_date:draft.work_date,minutes:typeof pending.minutes==='number'?String(pending.minutes):draft.minutes,note:typeof pending.note==='string'?pending.note:draft.note,reason:typeof pending.reason==='string'?pending.reason:draft.reason,on_behalf_ack:pending.on_behalf_ack===true}:draft;
 const onBehalf=visible.member_id!==capabilities.self_member_id;
 const authorized=mode==='create'?capabilities.can_create:!!basis&&(mode==='correct'?basis.can_correct:basis.can_void)&&basis.status==='active';
 // A stored original operation can be reconciled after its head was voided or
 // moved. Current edit flags only govern new writes; the server reauthorizes
 // the exact replay, and any refusal clears the protected view.
 const locked=denied||!basisReady||detail.loading||taskLoading||!action.pending&&(!authorized||onBehalf&&!capabilities.can_manage_others);
 async function refreshBasis(){
  if(mode==='create'){
   const value=await api<unknown>(worklogPagePath(project,week));if(!validWorklogPage(value,project))throw new UiError('common.responseUnknown');setCapabilities(value);
   if(!value.can_create)throw new UiError('worklogs.read_only');
  }else{const value=await detail.read();if(!value)throw new UiError('common.readIncomplete');if(value.member_id!==row?.member_id)throw new UiError('common.responseUnknown');setBasis(value);}
  if(mode!=='void'&&draft.task_id&&!await readTask(draft.task_id))throw new UiError('common.readIncomplete');
 }
 async function submit(event:React.FormEvent){
  event.preventDefault();if(locked||action.busy)return;const caller=api;
  if(action.pending){const original=action.pending,value=await action.run(original.path,original.body,original.method);if(value&&alive.current&&owner.current===caller){setDirty(false);onSaved();}return;}
  if(action.conflict)return;
  const issue=worklogDraftError(draft,mode,onBehalf,capabilities.can_manage_others);if(issue){action.setError(new UiError('worklogs.'+issue));return;}
  const fields=mode==='void'?{}:{task_id:draft.task_id,expected_task_version:draft.expected_task_version,work_date:draft.work_date,minutes:Number(draft.minutes),note:draft.note};
  const body={project_id:project,...fields,...(mode==='create'?{member_id:draft.member_id}:{expected_version:basis?.version,reason:draft.reason.trim()}),on_behalf_ack:onBehalf&&draft.on_behalf_ack};
  const value=await action.run(worklogPath(row?.id,mode==='correct'?'corrections':mode==='void'?'void':''),body,'POST');
  if(value&&alive.current&&owner.current===caller){setDirty(false);onSaved();}
 }
 return <form className="worklogs-editor" data-worklog-editor onSubmit={event=>void submit(event)}>
  <WriteNotice action={action} onRebase={refreshBasis}/>
  {!!readError&&<ErrorNotice error={readError} onRetry={()=>void refreshBasis().catch(action.setError)}/>}
  {detail.loading&&<Loading/>}
  {denied?<p role="status" className="warning">{t('access_changed')}</p>:<>
   <p className="worklogs-help">{t(mode==='create'?'manual_help':mode==='correct'?'correction_help':'void_help')}</p>
   {!authorized&&basisReady&&<p role="status" className="warning">{t('read_only')}</p>}
   {action.pending&&<p role="status" className="warning">{t('original_pending')}</p>}
   <WriteFields action={action}><fieldset className="worklogs-fields" disabled={locked||action.conflict}>
    {mode==='create'?<RecordPicker project={project} kind="members" value={visible.member_id} fallback={capabilities.self_member_id?{id:capabilities.self_member_id,name:t('your_resource')}:undefined} onlySelf={!capabilities.can_manage_others} self={capabilities.self_member_id} onDenied={loseAccess} onSelect={id=>update({member_id:id,on_behalf_ack:false})}/>:<Field label={t('member')} hint={t('immutable_resource')}><input name="worklog-member" value={basis?.member_name||row?.member_name||''} readOnly/></Field>}
    {mode!=='void'&&<>
     <RecordPicker project={project} kind="tasks" value={visible.task_id} fallback={row?{id:row.task_id,name:row.task_title}:undefined} onDenied={loseAccess} onSelect={(id,item)=>{currentTask.current=item;update({task_id:id,expected_task_version:0});if(id)void readTask(id).catch(()=>{});}}/>
     <p className="worklogs-help" role="status">{visible.expected_task_version?t('task_version',{version:formatNumber(visible.expected_task_version)}):t('choose_task_help')}</p>
     <div className="form-grid"><Field label={t('date')}><input name="worklog-date" type="date" required value={visible.work_date} onInput={event=>update({work_date:event.currentTarget.value})} onChange={event=>update({work_date:event.target.value})}/></Field><Field label={t('minutes')} hint={t('minutes_hint')}><input name="worklog-minutes" type="number" required min="1" max="1440" step="1" value={visible.minutes} onChange={event=>update({minutes:event.target.value})}/></Field></div>
     <Field label={t('note')} hint={t('note_hint')}><textarea name="worklog-note" rows={3} maxLength={400} value={visible.note} onChange={event=>update({note:event.target.value})}/></Field>
    </>}
    {mode!=='create'&&<Field label={t('reason')} hint={t('reason_hint')}><textarea name="worklog-reason" required rows={2} maxLength={400} value={visible.reason} onChange={event=>update({reason:event.target.value})}/></Field>}
    {onBehalf&&<label className="worklogs-ack"><input name="worklog-ack" type="checkbox" checked={visible.on_behalf_ack} onChange={event=>update({on_behalf_ack:event.target.checked})}/><span>{t('acknowledge')}</span></label>}
   </fieldset></WriteFields>
  </>}
  <div className="dialog-actions"><button type="button" disabled={action.busy||!!action.pending} onClick={()=>{setDirty(false);onCancel();}}>{t('cancel')}</button>{basis&&<small>{t('head_version',{version:formatNumber(basis.version)})}</small>}<WriteButton action={action} disabled={locked}>{t(mode==='create'?'save':mode==='correct'?'save_correction':'confirm_void')}</WriteButton></div>
 </form>;
}

function RecordPicker({project,kind,value,fallback,onlySelf=false,self='',onSelect,onDenied}:{project:string;kind:'members'|'tasks';value:string;fallback?:{id:string;name:string};onlySelf?:boolean;self?:string;onSelect:(id:string,item?:RecordItem)=>void;onDenied:()=>void}){
 const [cursor,setCursor]=useState('');const validate=useCallback((value:unknown):value is import('./worklogs-model').WorklogRecordPage=>validWorklogRecords(value,project),[project]);
 const read=useWorklogRead('/'+kind+query(project)+'&limit=100'+(cursor?'&cursor='+encodeURIComponent(cursor):''),validate,0,onDenied);
 const options=(read.value?.items||[]).filter(item=>kind==='tasks'||item.kind==='human'&&item.status==='active'&&typeof(item.user_id||item.data?.user_id)==='string'&&(!onlySelf||item.id===self));
 return <div className="worklogs-picker"><Field label={t(kind==='members'?'member':'task')}><select name={'worklog-'+kind} required value={value} disabled={read.loading||!!read.error} onChange={event=>onSelect(event.target.value,options.find(item=>item.id===event.target.value))}>
  <option value="">{t(kind==='members'?'choose_member':'choose_task')}</option>{fallback&&!options.some(item=>item.id===fallback.id)&&<option value={fallback.id}>{fallback.name}</option>}{value&&value!==fallback?.id&&!options.some(item=>item.id===value)&&<option value={value}>{t('selected_record',{id:value})}</option>}{options.map(item=><option key={item.id} value={item.id}>{label(item)}</option>)}
 </select></Field>{read.loading&&<Loading/>}{!!read.error&&<ErrorNotice error={read.error} onRetry={()=>void read.read()}/>}{read.value?.next_cursor&&<button type="button" onClick={()=>setCursor(read.value!.next_cursor!)}>{t(kind==='members'?'next_members':'next_tasks')}</button>}</div>;
}
