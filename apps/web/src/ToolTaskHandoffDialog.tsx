import {useEffect,useId,useRef,useState,type FormEvent} from 'react';
import type {RecordItem,Session} from './api';
import {useApi,useApiView} from './api-context';
import {formatNumber,tr,UiError,useLocale} from './i18n';
import {ErrorNotice,Field,Modal} from './ui';
import {defaultTaskView,taskFilterText,taskReadDenied,useTaskWorkspacePage} from './task-views-model';
import type {ToolProfile} from './tool-model';
import {createToolTaskHandoffPin,readToolTaskHandoffProfile,readToolTaskHandoffTask,type ToolTaskHandoffPin} from './tool-task-handoff-model';
import './ToolTaskHandoffDialog.css';

type Props={session:Session;project:string;tool:ToolProfile;disabled?:boolean;onClose:()=>void;onChoose:(task:RecordItem,pin:ToolTaskHandoffPin)=>void};
const t=(key:string,values?:Record<string,unknown>)=>tr('tool-task-handoff-picker.'+key,values);

/** Navigation intent stays in memory. The original preview and Start commands
 * remain the only way to create and execute a formally bound dispatch plan. */
export function ToolTaskHandoffDialog(props:Props){
 const api=useApi(),view=useApiView(),pin=createToolTaskHandoffPin(props.tool,props.project);
 const allowed=!!view&&view.userID===props.session.user.id&&view.organizationID===props.session.organization.id&&!props.disabled&&!!pin;
 const scope=JSON.stringify([props.session.user.id,props.session.organization.id,view?.userID,view?.organizationID,props.project,pin,allowed]);
 // A monotonically changing owner prevents A→B→A from adopting an older
 // caller's response even when its final public identifiers look identical.
 const owner=useRef({api,scope,epoch:0});
 if(owner.current.api!==api||owner.current.scope!==scope)owner.current={api,scope,epoch:owner.current.epoch+1};
 const capturedOwner=owner.current;
 return <TaskHandoffScope key={capturedOwner.epoch} {...props} pin={pin} allowed={allowed} owns={()=>owner.current===capturedOwner}/>;
}

function TaskHandoffScope({session,project,pin,allowed,owns,onClose,onChoose}:Props&{pin?:ToolTaskHandoffPin;allowed:boolean;owns:()=>boolean}){
 useLocale();const api=useApi(),id=useId();
 const [draft,setDraft]=useState(''),[query,setQuery]=useState(''),[refresh,setRefresh]=useState(0),[selected,setSelected]=useState('');
 const [badSearch,setBadSearch]=useState(false),[reading,setReading]=useState(false),[error,setError]=useState<unknown>(),[sourceBlocked,setSourceBlocked]=useState(false);
 const page=useTaskWorkspacePage(api,[session.user.id,session.organization.id,project],{...defaultTaskView,q:query},refresh,!allowed);
 const rows=page.items||[],invalid=rows.some(row=>!readToolTaskHandoffTask(row,project,row.id));
 const choice=!page.error&&!invalid?rows.find(row=>row.id===selected):undefined;
 const alive=useRef(true),closed=useRef(false),intent=useRef(0),request=useRef<AbortController|undefined>(undefined),busy=useRef(false);
 const callbacks=useRef({onClose,onChoose});callbacks.current={onClose,onChoose};
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;++intent.current;request.current?.abort();};},[]);
 const invalidate=()=>{++intent.current;request.current?.abort();busy.current=false;setReading(false);setError(undefined);};
 const close=()=>{if(!owns())return;closed.current=true;invalidate();callbacks.current.onClose();};
 const choose=(taskID:string)=>{if(!owns())return;invalidate();setSelected(taskID);};
 const reread=()=>{invalidate();setSelected('');setRefresh(value=>value+1);};
 const search=(event:FormEvent)=>{event.preventDefault();if(draft!==''&&!taskFilterText(draft)){setBadSearch(true);return;}invalidate();setSelected('');setBadSearch(false);setQuery(draft);setRefresh(value=>value+1);};
 const continueWithTask=async()=>{
  if(!owns()||!allowed||!pin||!choice||page.loading||page.error||invalid||sourceBlocked||busy.current||closed.current)return;
  const taskID=choice.id,originalPin={...pin},sequence=++intent.current,controller=new AbortController();
  request.current?.abort();request.current=controller;busy.current=true;setReading(true);setError(undefined);
  const current=()=>owns()&&alive.current&&!closed.current&&sequence===intent.current&&!controller.signal.aborted;
  try{
   const headers={'X-Suma-Expected-User':session.user.id,'X-Suma-Expected-Organization':session.organization.id};
   const [taskRaw,toolRaw]=await Promise.all([
    api<unknown>('/tasks/'+encodeURIComponent(taskID)+'?project_id='+encodeURIComponent(project),undefined,'GET',headers,controller.signal),
    api<unknown>('/tool-profiles/'+encodeURIComponent(originalPin.id)+'?project_id='+encodeURIComponent(project),undefined,'GET',headers,controller.signal),
   ]);
   if(!current())return;
   const task=readToolTaskHandoffTask(taskRaw,project,taskID),tool=readToolTaskHandoffProfile(toolRaw,originalPin);
   if(!task||!tool){setSourceBlocked(true);setSelected('');throw new UiError('tool-task-handoff-picker.changed');}
   // The current exact task version replaces a list observation, while the
   // reviewed tool pin is never silently upgraded or substituted by name.
   callbacks.current.onChoose(task,originalPin);
  }catch(failure){if(!current())return;if(taskReadDenied(failure)){setSourceBlocked(true);setSelected('');page.invalidate(failure);}setError(failure);}
  finally{if(current()){busy.current=false;setReading(false);}}
 };
 const pageError=invalid?new UiError('tool-task-handoff-picker.invalid_page'):page.error;
 const accessDenied=taskReadDenied(error)||taskReadDenied(page.error);
 return <Modal open onClose={close} title={t('title')} description={t('description')}>
  <div className="tool-task-handoff-picker" data-tool-task-handoff-picker aria-busy={reading}>
   {pin&&!accessDenied&&<div className="tool-task-handoff-tool"><strong>{pin.name}</strong><span>{t('tool_basis',{version:formatNumber(pin.version),worker:pin.worker_id})}</span></div>}
   {!allowed?<ErrorNotice error={new UiError('tool-task-handoff-picker.unavailable')}/>:accessDenied?<ErrorNotice error={error||page.error}/>:<>
    <form onSubmit={search} className="tool-task-handoff-search">
     <Field label={t('search_label')}><input id={id+'-search'} data-handoff-search value={draft} maxLength={400} disabled={reading} onChange={event=>{invalidate();setSelected('');setDraft(event.target.value);}}/></Field>
     <button type="submit" disabled={reading||page.loading} data-handoff-action="search">{t('search')}</button>
    </form>
    {badSearch&&<ErrorNotice error={new UiError('tool-task-handoff-picker.invalid_search')}/>}
    {!!pageError&&<ErrorNotice error={pageError} onRetry={reread}/>}
    <Field label={t('task_label')} hint={t('task_hint')}><select data-handoff-task value={choice?.id||''} disabled={reading||page.loading||!!pageError||sourceBlocked} onChange={event=>choose(event.target.value)}>
     <option value="">{t('choose_task')}</option>{!invalid&&!page.error&&rows.map(row=><option key={row.id} value={row.id}>{row.title||row.name||row.id} · v{formatNumber(row.version)}</option>)}
    </select></Field>
    {page.loading&&<p role="status">{t('loading')}</p>}
    {!page.loading&&!pageError&&rows.length===0&&<p>{t('empty')}</p>}
    {!pageError&&page.items&&<small className="tool-task-handoff-count">{t('loaded',{loaded:formatNumber(rows.length),total:formatNumber(page.total)})}</small>}
    <div className="tool-task-handoff-list-actions"><button type="button" disabled={reading||page.loading} onClick={reread} data-handoff-action="refresh">{t('refresh')}</button>{page.next_cursor&&<button type="button" disabled={reading||page.loading||sourceBlocked} onClick={()=>{invalidate();void page.more();}} data-handoff-action="more">{t('more')}</button>}</div>
    {!!error&&<ErrorNotice error={error} onRetry={!sourceBlocked&&choice?()=>void continueWithTask():undefined}/>}
   </>}
   <p className="tool-task-handoff-boundary">{t('boundary')}</p>
   <div className="dialog-actions"><button type="button" data-handoff-close onClick={close}>{t('close')}</button><button type="button" className="primary" data-handoff-action="continue" disabled={!allowed||!choice||reading||page.loading||!!pageError||sourceBlocked} onClick={()=>void continueWithTask()}>{t(reading?'checking':'continue')}</button></div>
  </div>
 </Modal>;
}
