import {CalendarDays,ChevronLeft,ChevronRight,PencilLine,RefreshCw,UserRound} from 'lucide-react';
import {useCallback,useEffect,useId,useState} from 'react';
import type {OrganizationWorkCalendar} from '../../../packages/contracts/src';
import {ApiError,type ApiCaller,type Session} from './api';
import {useApi,useApiView} from './api-context';
import {formatDateTime,formatNumber,tr,UiError,useLocale} from './i18n';
import {isCapacityWeek,mondayOf} from './planning-model';
import {OrganizationWorkCalendarEditor} from './ResourceCalendarEditor';
import {organizationCalendarWeek,shiftCalendarDate,targetCalendarPath,validOrganizationCalendar} from './resource-calendar-model';
import {ErrorNotice,Field,Loading,Modal} from './ui';
import './OrganizationWorkCalendarWorkspace.css';

type Props={
 session:Session;
 week:string;
 selectedUser:string;
 refresh:number;
 onWeek:(value:string)=>void;
 onSelect:(id:string)=>void;
};
type Member={user_id:string;name:string;email:string;role:'owner'|'admin'|'member'};
type Snapshot<T>={api:ApiCaller;key:string;value?:T;error?:unknown};

const t=(key:string,params?:Record<string,unknown>)=>tr('organization-work-calendar.'+key,params);
const dateOnly=(date:string)=>formatDateTime(date,{dateStyle:'medium',timeZone:'UTC'});

function memberRows(value:unknown):Member[]|undefined {
 if(!value||typeof value!=='object'||!('items' in value)||!Array.isArray(value.items))return;
 const members:Member[]=value.items;
 if(members.every(row=>row&&typeof row.user_id==='string'&&!!row.user_id&&typeof row.name==='string'&&
  typeof row.email==='string'&&['owner','admin','member'].includes(row.role))&&new Set(members.map(row=>row.user_id)).size===members.length)return members;
}

function useViewRead<T>(path:string|undefined,user:string,organization:string,revision:string,parse:(raw:unknown)=>T|undefined){
 const api=useApi();
 const [snapshot,setSnapshot]=useState<Snapshot<T>>();
 const key=JSON.stringify([path,user,organization,revision]);
 const active=snapshot?.api===api&&snapshot.key===key?snapshot:undefined;
 useEffect(()=>{
  if(!path)return;
  const controller=new AbortController();
  void api<unknown>(path,undefined,'GET',{'X-Suma-Expected-User':user,'X-Suma-Expected-Organization':organization},controller.signal)
   .then(raw=>{
    if(controller.signal.aborted)return;
    const value=parse(raw);
    if(value===undefined)throw new UiError('common.responseUnknown');
    setSnapshot({api,key,value});
   }).catch(error=>{if(!controller.signal.aborted)setSnapshot({api,key,error});});
  return ()=>controller.abort();
 },[api,path,key,user,organization,parse]);
 return {value:active?.value,error:active?.error,loading:!!path&&!active};
}

export function OrganizationWorkCalendarWorkspace(props:Props){
 const {user,organization}=props.session;
 return <CalendarWorkspace key={JSON.stringify([user.id,organization.id,user.role])} {...props}/>;
}

function CalendarWorkspace({session,week,selectedUser,refresh,onWeek,onSelect}:Props){
 useLocale();
 const id=useId(),view=useApiView();
 const {user,organization}=session;
 const admin=user.role==='owner'||user.role==='admin';
 const currentView=view?.userID===user.id&&view.organizationID===organization.id;
 const member=admin&&selectedUser?selectedUser:user.id;
 const [revision,setRevision]=useState(0),[saveRevision,setSaveRevision]=useState(0);
 const [editing,setEditing]=useState<{member:string;week:string}>();
 const members=useViewRead(currentView&&admin?'/organization/members':undefined,user.id,organization.id,JSON.stringify([refresh,revision]),memberRows);
 const selected=members.value?.find(row=>row.user_id===member);
 const eligible=currentView&&(member===user.id||!!selected);
 const path=eligible?targetCalendarPath({kind:'organization',scopeID:organization.id,member}):undefined;
 const parseCalendar=useCallback((value:unknown)=>validOrganizationCalendar(value,organization.id,member)?value:undefined,[organization.id,member]);
 const read=useViewRead(path,user.id,organization.id,JSON.stringify([refresh,revision,saveRevision]),parseCalendar);
 const dates=isCapacityWeek(week)?Array.from({length:7},(_,index)=>shiftCalendarDate(week,index)):[];
 const validWeek=dates.length===7&&dates.every(Boolean);
 const previous=validWeek?shiftCalendarDate(week,-7):'';
 const next=validWeek?shiftCalendarDate(week,7):'';
 const thisWeek=mondayOf();
 const calendar=read.value;
 const missingMember=admin&&member!==user.id&&!!members.value&&!selected;
 const changedAccess=missingMember||read.error instanceof ApiError&&[401,403,404].includes(read.error.status);
 const editOpen=editing?.member===member&&editing.week===week&&admin&&currentView&&!changedAccess;
 const name=member===user.id?user.name:selected?.name;
 const reload=useCallback(()=>setRevision(value=>value+1),[]);
 const accessChanged=useCallback(()=>{setEditing(undefined);reload();},[reload]);
 useEffect(()=>{if(editing&&!editOpen)setEditing(undefined);},[editing,editOpen]);
 const closeEdit=()=>{
  if(window.dispatchEvent(new Event('suma:navigate',{cancelable:true})))setEditing(undefined);
 };
 const saved=()=>{
  setEditing(undefined);
  setSaveRevision(value=>value+1);
 };

 return <section className="organization-work-calendar" data-organization-work-calendar aria-labelledby={id+'-title'}>
  <header className="org-calendar-heading">
   <h2 id={id+'-title'}>{t('title')}</h2>
   <p>{t('description')}</p>
  </header>
  <section className="panel org-calendar-panel" aria-labelledby={id+'-availability'}>
   <div className="org-calendar-toolbar">
    {admin?<Field label={t('member')}><select data-org-calendar-member value={member} disabled={!currentView||members.loading||!members.value}
      onChange={event=>onSelect(event.target.value)}>
      {!members.value?.some(row=>row.user_id===user.id)&&<option value={user.id}>{user.name}</option>}
      {member!==user.id&&!selected&&<option value={member} disabled>{t('member_unavailable')}</option>}
      {members.value?.map(row=><option key={row.user_id} value={row.user_id}>{row.name} · {row.email}</option>)}
    </select></Field>:<div className="org-calendar-person"><UserRound size={18} aria-hidden="true"/><div><span>{t('member')}</span><strong>{user.name}</strong></div></div>}
    <div className="org-calendar-week">
     <div className="org-calendar-week-label"><CalendarDays size={18} aria-hidden="true"/><div><span>{t('selected_week')}</span>
      {validWeek&&<strong>{t('week_range',{start:dateOnly(week),end:dateOnly(dates[6])})}</strong>}
     </div></div>
     <div className="org-calendar-controls" role="group" aria-label={t('week_controls')}>
      <button type="button" data-org-calendar-previous disabled={!previous} aria-label={t('previous')} title={t('previous')} onClick={()=>onWeek(previous)}><ChevronLeft size={17} aria-hidden="true"/></button>
      <button type="button" data-org-calendar-current disabled={week===thisWeek} onClick={()=>onWeek(thisWeek)}>{t('this_week')}</button>
      <button type="button" data-org-calendar-next disabled={!next} aria-label={t('next')} title={t('next')} onClick={()=>onWeek(next)}><ChevronRight size={17} aria-hidden="true"/></button>
      <button type="button" data-org-calendar-refresh disabled={members.loading||read.loading||!currentView} aria-label={t('refresh')} title={t('refresh')} onClick={reload}><RefreshCw size={16} aria-hidden="true"/></button>
     </div>
    </div>
   </div>
   <div className="org-calendar-body">
    <div className="org-calendar-section-heading">
     <div><h3 id={id+'-availability'}>{t('saved_availability')}</h3>{name&&<p>{name}</p>}</div>
     {calendar?.can_edit&&admin&&validWeek&&<button type="button" className="primary" data-org-calendar-edit onClick={()=>setEditing({member,week})}>
      <PencilLine size={15} aria-hidden="true"/>{t('edit')}
     </button>}
    </div>
    {!currentView&&<ErrorNotice error={new UiError('common.viewContextConflict')}/>}
    {currentView&&Boolean(members.error)&&<ErrorNotice error={members.error} onRetry={reload}/>}
    {missingMember&&<ErrorNotice error={new UiError('organization-work-calendar.member_unavailable')}/>}
    {Boolean(read.error)&&<ErrorNotice error={read.error} onRetry={reload}/>}
    {(members.loading&&!eligible||read.loading)&&<Loading/>}
    {!validWeek&&<ErrorNotice error={new UiError('organization-work-calendar.invalid_week')}/>}
    {calendar&&validWeek&&<SavedWeek dates={dates} calendar={calendar}/>}
   </div>
  </section>
  <Modal open={editOpen} onClose={closeEdit} title={t('edit_title',{name:name||''})} description={t('edit_description')}>
   {editOpen&&<OrganizationWorkCalendarEditor organization={organization.id} member={member} week={week} onSaved={saved} onCancel={()=>setEditing(undefined)} onAccessChanged={accessChanged}/>}
  </Modal>
 </section>;
}

function SavedWeek({dates,calendar}:{dates:string[];calendar:OrganizationWorkCalendar}){
 const days=organizationCalendarWeek(calendar.schedule,dates);
 const unknown=days.filter(day=>day.hours===null).length;
 const total=days.reduce((sum,day)=>sum+(day.hours??0),0);
 return <>
  <div className="org-calendar-summary">
   <div role="status"><span>{t('weekly_total')}</span><strong data-org-calendar-total>{unknown?t('unknown'):t('hours',{hours:formatNumber(total)})}</strong>
    {unknown>0&&<small>{t('unknown_days',{count:formatNumber(unknown)})}</small>}
   </div>
   <p>{calendar.version===0?t('not_configured'):t('version',{version:formatNumber(calendar.version)})}</p>
  </div>
  <ol className="org-calendar-days" aria-label={t('daily_availability')}>
   {days.map(day=><li key={day.date} data-org-calendar-day={day.date} data-availability={day.hours===null?'unknown':day.hours===0?'off':'known'} data-exception={day.exception}>
    <time dateTime={day.date}><span>{formatDateTime(day.date,{weekday:'short',timeZone:'UTC'})}</span><span>{formatDateTime(day.date,{month:'short',day:'numeric',timeZone:'UTC'})}</span></time>
    <strong>{day.hours===null?t('unknown'):t('hours',{hours:formatNumber(day.hours)})}</strong>
    {day.exception&&<small>{t('exception')}</small>}
   </li>)}
  </ol>
  {!calendar.can_edit&&<p className="org-calendar-note">{t('read_only')}</p>}
 </>;
}
