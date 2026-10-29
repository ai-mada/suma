import type {ResourceCalendar,ResourceCalendarSchedule} from '../../../packages/contracts/src';
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
export function validCalendar(value:unknown,project:string,member:string):value is ResourceCalendar{return object(value)&&value.project_id===project&&value.member_id===member&&typeof value.version==='number'&&Number.isSafeInteger(value.version)&&value.version>=0&&typeof value.can_edit==='boolean'&&typeof value.updated_at==='string'&&validCalendarSchedule(value.schedule)&&(value.version!==0||value.schedule.weekdays.every(day=>day===null)&&value.schedule.exceptions.length===0);}
export function validCalendarRequest(request:WriteRecoveryRequest,project:string,member:string){const body=request.body;return request.path===calendarPath(member)&&request.method==='PUT'&&Object.keys(body).sort().join(',')==='expected_version,idempotency_key,project_id,schedule'&&body.project_id===project&&typeof body.expected_version==='number'&&Number.isSafeInteger(body.expected_version)&&body.expected_version>=0&&body.idempotency_key===request.idempotency_key&&validCalendarSchedule(body.schedule);}
export function validCalendarReceipt(value:unknown,request:WriteRecoveryRequest,project:string,member:string){return validCalendarRequest(request,project,member)&&validCalendar(value,project,member)&&value.version===(request.body.expected_version as number)+1&&JSON.stringify(calendarSchedule(calendarDraft(value.schedule)))===JSON.stringify(calendarSchedule(calendarDraft(request.body.schedule as ResourceCalendarSchedule)));}
