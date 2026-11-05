import type {OrganizationWorkCalendar,ResourceCalendar,ResourceCalendarSchedule} from '../../../packages/contracts/src';
import type {WriteRecoveryRequest} from './write-action';

export type CalendarDraft={weekdays:string[];exceptions:{date:string;hours:string;note:string}[]};
export const workweekTemplate=()=>({weekdays:['8','8','8','8','8','0','0']});
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const hours=(value:unknown)=>value===null||typeof value==='number'&&Number.isFinite(value)&&value>=0&&value<=24&&Number.isInteger(value*4);
const date=(value:unknown)=>typeof value==='string'&&/^\d{4}-\d{2}-\d{2}$/.test(value)&&value>='0001-01-01'&&Number.isFinite(Date.parse(value+'T00:00:00Z'))&&new Date(value+'T00:00:00Z').toISOString().slice(0,10)===value;
const note=(value:unknown)=>typeof value==='string'&&Array.from(value).length<=200&&!/[\uD800-\uDFFF]/u.test(value);
export function validCalendarSchedule(value:unknown):value is ResourceCalendarSchedule{
 if(!object(value)||Object.keys(value).sort().join(',')!=='exceptions,weekdays'||!Array.isArray(value.weekdays)||value.weekdays.length!==7||!value.weekdays.every(hours)||!Array.isArray(value.exceptions)||value.exceptions.length>366)return false;
 const seen=new Set<string>();return value.exceptions.every(item=>{if(!object(item)||Object.keys(item).sort().join(',')!=='available_hours,date,note'||!date(item.date)||!hours(item.available_hours)||!note(item.note)||seen.has(item.date as string))return false;seen.add(item.date as string);return true;});
}
export function calendarDraft(schedule:ResourceCalendarSchedule):CalendarDraft{return {weekdays:(schedule.weekdays??Array(7).fill(null)).map(value=>value===null?'':String(value)),exceptions:(schedule.exceptions??[]).map(item=>({date:item.date,hours:item.available_hours===null?'':String(item.available_hours),note:item.note}))};}
export function calendarDraftError(draft:CalendarDraft){
 if(draft.weekdays.length!==7||[...draft.weekdays,...draft.exceptions.map(item=>item.hours)].some(value=>value.trim()!==''&&!hours(Number(value))))return 'hours_invalid';
 if(draft.exceptions.length>366)return 'exceptions_limit';
 const seen=new Set<string>();for(const item of draft.exceptions){if(!date(item.date))return 'date_invalid';if(seen.has(item.date))return 'date_duplicate';seen.add(item.date);if(!note(item.note))return 'note_invalid';}return '';
}
export function calendarSchedule(draft:CalendarDraft):ResourceCalendarSchedule{const value=(text:string)=>text.trim()===''?null:Number(text);return {weekdays:draft.weekdays.map(value),exceptions:draft.exceptions.map(item=>({date:item.date,available_hours:value(item.hours),note:item.note})).sort((a,b)=>a.date.localeCompare(b.date))};}
export function calendarPath(member:string){return '/resource-calendars/'+encodeURIComponent(member);}

type CalendarData = Pick<ResourceCalendar,'version'|'schedule'|'updated_at'|'can_edit'>;
export type CalendarTarget = {kind:'project'|'organization';scopeID:string;member:string};
export function targetCalendarPath(target:CalendarTarget){
 return target.kind==='project'?calendarPath(target.member):'/organization/work-calendars/'+encodeURIComponent(target.member);
}
export function calendarRequestScope(target:CalendarTarget){
 return target.kind==='project'?{project_id:target.scopeID}:{};
}
function validCalendarData(value:unknown):value is Record<string,unknown> & CalendarData{
 return object(value)&&typeof value.version==='number'&&Number.isSafeInteger(value.version)&&value.version>=0&&
  typeof value.can_edit==='boolean'&&typeof value.updated_at==='string'&&validCalendarSchedule(value.schedule)&&
  (value.version!==0||value.schedule.weekdays.every(day=>day===null)&&value.schedule.exceptions.length===0);
}
export function validCalendar(value:unknown,project:string,member:string):value is ResourceCalendar{
 return validCalendarData(value)&&value.project_id===project&&value.member_id===member;
}
export function validOrganizationCalendar(value:unknown,organization:string,user:string):value is OrganizationWorkCalendar{
 return validCalendarData(value)&&value.organization_id===organization&&value.user_id===user;
}
export function validTargetCalendar(value:unknown,target:CalendarTarget):value is CalendarData{
 return target.kind==='project'?validCalendar(value,target.scopeID,target.member):validOrganizationCalendar(value,target.scopeID,target.member);
}
export function validTargetCalendarRequest(request:WriteRecoveryRequest,target:CalendarTarget){
 const body=request.body;
 const fields=target.kind==='project'?'expected_version,idempotency_key,project_id,schedule':'expected_version,idempotency_key,schedule';
 return request.path===targetCalendarPath(target)&&request.method==='PUT'&&Object.keys(body).sort().join(',')===fields&&
  (target.kind==='organization'||body.project_id===target.scopeID)&&typeof body.expected_version==='number'&&
  Number.isSafeInteger(body.expected_version)&&body.expected_version>=0&&body.idempotency_key===request.idempotency_key&&
  validCalendarSchedule(body.schedule);
}
export function validTargetCalendarReceipt(value:unknown,request:WriteRecoveryRequest,target:CalendarTarget){
 return validTargetCalendarRequest(request,target)&&validTargetCalendar(value,target)&&
  value.version===(request.body.expected_version as number)+1&&
  JSON.stringify(calendarSchedule(calendarDraft(value.schedule)))===JSON.stringify(calendarSchedule(calendarDraft(request.body.schedule as ResourceCalendarSchedule)));
}
export function validCalendarRequest(request:WriteRecoveryRequest,project:string,member:string){
 return validTargetCalendarRequest(request,{kind:'project',scopeID:project,member});
}
export function validCalendarReceipt(value:unknown,request:WriteRecoveryRequest,project:string,member:string){
 return validTargetCalendarReceipt(value,request,{kind:'project',scopeID:project,member});
}

export function shiftCalendarDate(value:string,offset:number){
 if(!date(value))return '';
 const next=new Date(value+'T00:00:00Z');
 next.setUTCDate(next.getUTCDate()+offset);
 const shifted=next.toISOString().slice(0,10);
 return date(shifted)?shifted:'';
}

export function organizationCalendarWeek(schedule:ResourceCalendarSchedule,dates:string[]){
 const exceptions=new Map(schedule.exceptions.map(exception=>[exception.date,exception]));
 return dates.map((date,index)=>{
  const exception=exceptions.get(date);
  return {date,hours:exception?exception.available_hours:schedule.weekdays[index],exception:!!exception};
 });
}
