import {useCallback,useEffect,useMemo,useRef,useState} from 'react';
import {Plus,Trash2} from 'lucide-react';
import type {ResourceCalendar,ResourceCalendarSchedule} from '../../../packages/contracts/src';
import {ApiError,query} from './api';
import {useApi,useApiView} from './api-context';
import {tr,useLocale,UiError,formatNumber,renderDiagnostic} from './i18n';
import {Field,ErrorNotice,Loading} from './ui';
import {toolSessionStorage} from './tool-hooks';
import {useLeaveGuard,useWriteAction,WriteNotice,WriteFields,WriteButton} from './write-action';
import {calendarDraft,calendarDraftError,calendarRequestScope,calendarSchedule,targetCalendarPath,validTargetCalendar,validTargetCalendarReceipt,validTargetCalendarRequest,workweekTemplate,type CalendarDraft} from './resource-calendar-model';
import './ResourceCalendarEditor.css';

const t=(key:string,values?:Record<string,unknown>)=>tr('resource-calendar.'+key,values);
export const calendarWeekdays=['monday','tuesday','wednesday','thursday','friday','saturday','sunday'];
type EditorProps={member:string;week:string;onSaved:()=>void;onCancel:()=>void};
type Props=EditorProps&{project:string};
type OrganizationProps=EditorProps&{organization:string;onAccessChanged:()=>void};
type ScopedEditorProps=EditorProps&{kind:'project'|'organization';scopeID:string;onAccessChanged?:()=>void};
type CalendarSnapshot=Pick<ResourceCalendar,'version'|'schedule'|'updated_at'|'can_edit'>;

export function ResourceCalendarEditor({project,...props}:Props){
 return <CalendarOwner {...props} kind="project" scopeID={project}/>;
}

export function OrganizationWorkCalendarEditor({organization,...props}:OrganizationProps){
 return <CalendarOwner {...props} kind="organization" scopeID={organization}/>;
}

/** Each mounted editor owns one actor, calendar target and displayed week. */
function CalendarOwner(props:ScopedEditorProps){
 const api=useApi(),view=useApiView(),caller=useRef({api,epoch:0});
 if(caller.current.api!==api)caller.current={api,epoch:caller.current.epoch+1};
 const target=props.kind==='project'?[props.scopeID]:['organization',props.scopeID];
 const scope=JSON.stringify([view?.userID??'',view?.organizationID??'',...target,props.member,props.week]);
 return <CalendarScope key={scope+':'+caller.current.epoch} {...props} scope={scope}/>;
}

function CalendarScope({kind,scopeID,member,onSaved,onCancel,onAccessChanged,scope}:ScopedEditorProps&{scope:string}){
 useLocale();const api=useApi(),view=useApiView(),owner=useRef(api),alive=useRef(true),epoch=useRef(0),controller=useRef<AbortController|undefined>(undefined);owner.current=api;
 const viewerID=view?.userID,organizationID=view?.organizationID;
 const target=useMemo(()=>({kind,scopeID,member}),[kind,scopeID,member]);
 const path=targetCalendarPath(target);
 const [basis,setBasis]=useState<CalendarSnapshot>(),[draft,setDraft]=useState<CalendarDraft>(),[loading,setLoading]=useState(true),[error,setError]=useState<unknown>(''),[denied,setDenied]=useState(false);
 const action=useWriteAction({scope:'resource-calendar:'+scope,storage:toolSessionStorage(),validateRequest:request=>validTargetCalendarRequest(request,target),validateResponse:(value,request)=>validTargetCalendarReceipt(value,request,target)});
 const pendingSchedule=action.pending?.body.schedule as ResourceCalendarSchedule|undefined;
 const visible=pendingSchedule?calendarDraft(pendingSchedule):draft;
 const dirty=!!draft&&!!basis&&JSON.stringify(calendarSchedule(draft))!==JSON.stringify(calendarSchedule(calendarDraft(basis.schedule)));
 const locked=loading||action.busy||!!action.pending||denied||!basis?.can_edit;
 useLeaveGuard(dirty,action.setError);
 useEffect(()=>{alive.current=true;return()=>{alive.current=false;epoch.current++;controller.current?.abort();};},[]);
 const read=useCallback(async(rebase=false)=>{
  const current=++epoch.current;controller.current?.abort();const cancel=new AbortController();controller.current=cancel;const caller=api;setLoading(true);setError('');
  try{
   if(kind==='organization'&&(!viewerID||organizationID!==scopeID))throw new UiError('common.viewContextConflict');
   const headers=kind==='organization'?{'X-Suma-Expected-User':viewerID!,'X-Suma-Expected-Organization':scopeID}:undefined;
   const value=await api<unknown>(path+(kind==='project'?query(scopeID):''),undefined,'GET',headers,cancel.signal);
   if(!alive.current||owner.current!==caller||current!==epoch.current)return;
   if(!validTargetCalendar(value,target))throw new UiError('common.responseUnknown');
   if(kind==='organization'&&!value.can_edit){onAccessChanged?.();return;}
   setBasis(value);setDenied(false);if(!rebase)setDraft(calendarDraft(value.schedule));return value;
  }catch(failure){if(!alive.current||current!==epoch.current||cancel.signal.aborted)return;setError(failure);if(failure instanceof ApiError&&(failure.status===401||failure.status===403||kind==='organization'&&failure.status===404)){setDenied(true);setBasis(undefined);setDraft(undefined);onAccessChanged?.();}throw failure;
  }finally{if(alive.current&&current===epoch.current)setLoading(false);}
 },[api,kind,scopeID,path,target,viewerID,organizationID,onAccessChanged]);
 useEffect(()=>{void read().catch(()=>{});},[read]);
 const writeDenied=action.errorStatus===401||action.errorStatus===403||kind==='organization'&&action.errorStatus===404;
 useEffect(()=>{if(writeDenied){setDenied(true);setBasis(undefined);setDraft(undefined);controller.current?.abort();onAccessChanged?.();}},[writeDenied,onAccessChanged]);
 const update=(next:CalendarDraft)=>{setDraft(next);action.setError('');};
 const acknowledge=async(value:CalendarSnapshot|undefined,caller:typeof api)=>{if(!value||!alive.current||owner.current!==caller)return;setBasis(value);setDraft(calendarDraft(value.schedule));onSaved();};
 async function submit(event:React.FormEvent){
  event.preventDefault();if(denied||loading||action.busy||!basis?.can_edit)return;const caller=api;
  if(action.pending){const request=action.pending;await acknowledge(await action.run<CalendarSnapshot>(request.path,request.body,request.method),caller);return;}
  if(!draft||!basis||action.conflict||!dirty)return;
  const issue=calendarDraftError(draft);if(issue){action.setError(new UiError('resource-calendar.'+issue));return;}
  await acknowledge(await action.run<CalendarSnapshot>(path,{...calendarRequestScope(target),expected_version:basis.version,schedule:calendarSchedule(draft)},'PUT'),caller);
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
    <div className="resource-calendar-heading"><div><h3>{t('weekly_title')}</h3><p>{kind==='organization'?tr('organization-work-calendar.weekly_hint'):t('weekly_hint')}</p></div><button type="button" data-calendar-template onClick={()=>draft&&update({...draft,...workweekTemplate()})}>{t('apply_template')}</button></div>
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
