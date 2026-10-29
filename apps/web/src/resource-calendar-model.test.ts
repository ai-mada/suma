import {describe,expect,it} from 'vitest';
import {calendarDraft,calendarSchedule,calendarDraftError,validCalendarRequest,validCalendarReceipt} from './resource-calendar-model';

const schedule={weekdays:[8,8,8,8,8,0,0],exceptions:[{date:'2026-10-28',available_hours:null,note:'Unknown leave approval'}]};
describe('resource calendar draft and exact operation',()=>{
 it('preserves unknown, explicit zero and removal of a date override',()=>{
  const draft=calendarDraft(schedule);expect(draft.weekdays[5]).toBe('0');expect(draft.exceptions[0].hours).toBe('');
  draft.weekdays[0]='';draft.exceptions=[];
  expect(calendarSchedule(draft)).toEqual({weekdays:[null,8,8,8,8,0,0],exceptions:[]});
 });
 it('rejects duplicate dates, invalid dates, fractional and unbounded hours',()=>{
  for(const value of ['-1','24.25','0.1','Infinity']){const draft=calendarDraft(schedule);draft.weekdays[0]=value;expect(calendarDraftError(draft)).toBe('hours_invalid');}
  const draft=calendarDraft(schedule);draft.exceptions.push({...draft.exceptions[0]});expect(calendarDraftError(draft)).toBe('date_duplicate');
  draft.exceptions=[{date:'2026-02-30',hours:'0',note:''}];expect(calendarDraftError(draft)).toBe('date_invalid');
  draft.exceptions=[{date:'2026-10-28',hours:'8',note:'假'.repeat(201)}];expect(calendarDraftError(draft)).toBe('note_invalid');
 });
 it('binds recovery to the original member, project, version and saved schedule',()=>{
  const request={path:'/resource-calendars/member-a',method:'PUT',idempotency_key:'operation-a',body:{project_id:'project-a',expected_version:2,schedule,idempotency_key:'operation-a'}};
  const receipt={project_id:'project-a',member_id:'member-a',version:3,schedule,can_edit:true,updated_at:''};
  expect(validCalendarRequest(request,'project-a','member-a')).toBe(true);expect(validCalendarRequest(request,'project-b','member-a')).toBe(false);
  expect(validCalendarReceipt(receipt,request,'project-a','member-a')).toBe(true);
  expect(validCalendarReceipt({...receipt,version:4},request,'project-a','member-a')).toBe(false);
  expect(validCalendarReceipt({...receipt,schedule:{...schedule,exceptions:[]}},request,'project-a','member-a')).toBe(false);
 });
});
