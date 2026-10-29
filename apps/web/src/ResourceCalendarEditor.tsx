import {useCallback,useEffect,useRef,useState} from 'react';
import {Plus,Trash2} from 'lucide-react';
import type {ResourceCalendar,ResourceCalendarSchedule} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi,useApiView} from './api-context';
import {tr,useLocale,UiError,formatNumber,renderDiagnostic} from './i18n';
import {Field,ErrorNotice,Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard,useWriteAction,WriteNotice,WriteFields,WriteButton} from './write-action';
import {calendarDraft,calendarDraftError,calendarPath,calendarSchedule,validCalendar,validCalendarReceipt,validCalendarRequest,workweekTemplate,type CalendarDraft} from './resource-calendar-model';
import './ResourceCalendarEditor.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('resource-calendar.'+key,values);
export const calendarWeekdays=['monday','tuesday','wednesday','thursday','friday','saturday','sunday'];
type Props={project:string;member:string;week:string;onSaved:()=>void;onCancel:()=>void};

/** A mounted editor owns exactly one actor, project, resource and displayed week. */
export function ResourceCalendarEditor(props:Props){
 const api=useApi(),view=useApiView(),caller=useRef({api,epoch:0});
 if(caller.current.api!==api)caller.current={api,epoch:caller.current.epoch+1};
 const scope=JSON.stringify([view?.userID??'',view?.organizationID??'',props.project,props.member,props.week]);
 return <CalendarScope key={scope+':'+caller.current.epoch} {...props} scope={scope}/>;
}

function CalendarScope({project,member,onSaved,onCancel,scope}:Props&{scope:string}){
 useLocale();const api=useApi(),owner=useRef(api),alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);owner.current=api;
 const [basis,setBasis]=useState<ResourceCalendar>(),[draft,setDraft]=useState<CalendarDraft>(),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>(''),[denied,setDenied]=useState(false);
 const action=useWriteAction({scope:'resource-calendar:'+scope,storage:toolSessionStorage(),validateRequest:request=>validCalendarRequest(request,project,member),validateResponse:(value,request)=>validCalendarReceipt(value,request,project,member)});
 const pendingSchedule=action.pending?.body.schedule as ResourceCalendarSchedule|undefined;
 const visible=pendingSchedule?calendarDraft(pendingSchedule):draft;
 const dirty=!!draft&&!!basis&&JSON.stringify(calendarSchedule(draft))!==JSON.stringify(calendarSchedule(calendarDraft(basis.schedule)));
 const locked=loading||action.busy||!!action.pending||denied||!basis?.can_edit;
 useLeaveGuard(dirty,action.setError);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;controller.current?.abort();};},[]);
 const read=useCallback(async(rebase=false)=>{
  const current=++epoch.current;controller.current?.abort();const cancel=new AbortController();controller.current=cancel;const caller=api;setLoading(true);setError('');
  try{const value=await api<unknown>(calendarPath(member)+query(project),undefined,undefined,undefined,cancel.signal);
   if(!alive.current||owner.current!==caller||current!==epoch.current)return;
   if(!validCalendar(value,project,member))throw new UiError('common.responseUnknown');
   setBasis(value);setDenied(false);if(!rebase)setDraft(calendarDraft(value.schedule));return value;
  }catch(failure){if(!alive.current||current!==epoch.current||cancel.signal.aborted)return;setError(failure);if(failure instanceof ApiError&&(failure.status===401||failure.status===403)){setDenied(true);setBasis(undefined);setDraft(undefined);}throw failure;
  }finally{if(alive.current&&current===epoch.current)setLoading(false);}
 },[api,project,member]);
 useEffect(()=>{void read().catch(()=>{});},[read]);
 const writeDenied=action.errorStatus===401||action.errorStatus===403;
 useEffect(()=>{if(writeDenied){setDenied(true);setBasis(undefined);setDraft(undefined);controller.current?.abort();}},[writeDenied]);
 const update=(next:CalendarDraft)=>{setDraft(next);action.setError('');};
 const acknowledge=async(value:ResourceCalendar|undefined,caller:typeof api)=>{if(!value||!alive.current||owner.current!==caller)return;setBasis(value);setDraft(calendarDraft(value.schedule));onSaved();};
 async function submit(event:React.FormEvent){
  event.preventDefault();if(denied||loading||action.busy||!basis?.can_edit)return;const caller=api;
  if(action.pending){const request=action.pending;await acknowledge(await action.run<ResourceCalendar>(request.path,request.body,request.method),caller);return;}
  if(!draft||!basis||action.conflict||!dirty)return;
  const issue=calendarDraftError(draft);if(issue){action.setError(new UiError('resource-calendar.'+issue));return;}
  await acknowledge(await action.run<ResourceCalendar>(calendarPath(member),{project_id:project,expected_version:basis.version,schedule:calendarSchedule(draft)},'PUT'),caller);
 }
 return <form className="resource-calendar-editor" onSubmit={event=>void submit(event)} data-resource-calendar-form>
  <WriteNotice action={action} onRebase={async()=>{if(!await read(true))throw new UiError('common.readIncomplete');}}/>
  {!!error&&<ErrorNotice error={renderDiagnostic(error)} onRetry={()=>void read(!!draft).catch(()=>{})}/>}
  {loading&&<Loading/>}
  {denied?<p className="warning" role="status">{t('access_changed')}</p>:visible&&<>
   <p className="resource-calendar-help">{t('semantics')}</p>
   {!basis?.can_edit&&<p className="resource-calendar-help" role="status">{t('read_only')}</p>}
   {action.pending&&<p className="warning" role="status">{t('original_pending')}</p>}
   <WriteFields action={action}><fieldset disabled={locked||action.conflict} className="resource-calendar-fields">
    <div className="resource-calendar-heading"><div><h3>{t('weekly_title')}</h3><p>{t('weekly_hint')}</p></div><button type="button" data-calendar-template onClick={()=>draft&&update({...draft,...workweekTemplate()})}>{t('apply_template')}</button></div>
    <div className="resource-calendar-weekdays">{calendarWeekdays.map((day,index)=><Field key={day} label={t('day.'+day)} hint={visible.weekdays[index]===''?t('unknown'):visible.weekdays[index]==='0'?t('day_off'):t('hours_unit')}><input type="number" name={'weekday-'+index} min="0" max="24" step="0.25" value={visible.weekdays[index]} placeholder={t('unknown')} onChange={event=>draft&&update({...draft,weekdays:draft.weekdays.map((value,i)=>i===index?event.target.value:value)})}/></Field>)}</div>
    <div className="resource-calendar-heading"><div><h3>{t('exceptions_title')}</h3><p>{t('exceptions_hint')}</p></div><button type="button" data-calendar-add disabled={visible.exceptions.length>=366} onClick={()=>draft&&update({...draft,exceptions:[...draft.exceptions,{date:'',hours:'',note:''}]})}><Plus size={14} aria-hidden="true"/>{t('add_exception')}</button></div>
    {visible.exceptions.length?<div className="resource-calendar-exceptions">{visible.exceptions.map((item,index)=><section key={index} className="resource-calendar-exception">
     <Field label={t('exception_date')}><input name={'exception-date-'+index} type="date" required value={item.date} onInput={event=>draft&&update({...draft,exceptions:draft.exceptions.map((value,i)=>i===index?{...value,date:event.currentTarget.value}:value)})} onChange={event=>draft&&update({...draft,exceptions:draft.exceptions.map((value,i)=>i===index?{...value,date:event.target.value}:value)})}/></Field>
     <Field label={t('exception_hours')} hint={item.hours===''?t('override_unknown'):item.hours==='0'?t('day_off'):t('hours_unit')}><input name={'exception-hours-'+index} type="number" min="0" max="24" step="0.25" placeholder={t('unknown')} value={item.hours} onChange={event=>draft&&update({...draft,exceptions:draft.exceptions.map((value,i)=>i===index?{...value,hours:event.target.value}:value)})}/></Field>
     <Field label={t('exception_note')}><input name={'exception-note-'+index} maxLength={400} value={item.note} onChange={event=>draft&&update({...draft,exceptions:draft.exceptions.map((value,i)=>i===index?{...value,note:event.target.value}:value)})}/></Field>
     <button type="button" className="resource-calendar-remove" aria-label={t('remove_exception',{index:formatNumber(index+1)})} onClick={()=>draft&&update({...draft,exceptions:draft.exceptions.filter((_,i)=>i!==index)})}><Trash2 size={15} aria-hidden="true"/>{t('remove')}</button>
    </section>)}</div>:<p className="resource-calendar-help">{t('inherited')}</p>}
   </fieldset></WriteFields>
  </>}
  <div className="dialog-actions"><button type="button" disabled={action.busy||!!action.pending} onClick={()=>{setDraft(undefined);onCancel();}}>{t('cancel')}</button><small>{basis?t(basis.version===0?'unsaved':'version',{version:formatNumber(basis.version)}):''}</small><WriteButton action={action} disabled={denied||loading||!basis?.can_edit||!action.pending&&!dirty}>{t('save')}</WriteButton></div>
 </form>;
}
