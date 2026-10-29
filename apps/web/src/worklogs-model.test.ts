import {expect,it} from 'vitest';
import {worklogDraftError,worklogPath,worklogWeekEnd,validWorklog,validWorklogPage,validWorklogRequest,validWorklogReceipt,worklogRecovery,worklogRecoveryKey,worklogScope} from './worklogs-model';

export const worklogFixture=(patch:Record<string,unknown>={})=>({id:'log-a',project_id:'project-a',version:1,status:'active',source:'human_manual',task_id:'task-a',task_version:2,task_title:'Delivery task',member_id:'member-a',member_version:1,member_name:'Human resource',user_id:'owner-a',user_name:'Owner',work_date:'2026-10-28',minutes:45,note:'Explicit work',reason:'',actor_id:'owner-a',actor_name:'Owner',on_behalf_ack:false,created_at:'2026-10-28T10:00:00Z',updated_at:'2026-10-28T10:00:00Z',can_correct:true,can_void:true,...patch});
export const worklogPageFixture=(patch:Record<string,unknown>={})=>({project_id:'project-a',items:[worklogFixture()],total:1,limit:20,next_cursor:'',scope_version:1,summary:{active_count:1,voided_count:0,total_minutes:45},can_create:true,can_manage_others:false,self_member_id:'member-a',...patch});

it('requires explicit valid calendar dates and integer minutes without converting blank or decimal effort',()=>{
 const draft={member_id:'member-a',task_id:'task-a',expected_task_version:2,work_date:'2026-10-28',minutes:'45',note:'',reason:'',on_behalf_ack:false};
 expect(worklogDraftError(draft,'create',false,false)).toBe('');
 for(const minutes of ['', '0','1.5','1441','NaN'])expect(worklogDraftError({...draft,minutes},'create',false,false)).toBe('minutes_invalid');
 expect(worklogDraftError({...draft,work_date:'2026-02-30'},'create',false,false)).toBe('date_invalid');
 expect(worklogDraftError({...draft,note:'😀'.repeat(201)},'create',false,false)).toBe('note_invalid');
 expect(worklogDraftError(draft,'correct',false,false)).toBe('reason_invalid');
 expect(worklogDraftError(draft,'create',true,false)).toBe('on_behalf_denied');
 expect(worklogDraftError(draft,'create',true,true)).toBe('ack_required');
 expect(worklogWeekEnd('2026-10-26')).toBe('2026-11-01');
});

it('validates current head facts and exact aggregate instead of deriving totals from a page',()=>{
 expect(validWorklog(worklogFixture(),'project-a')).toBe(true);
 expect(validWorklog(worklogFixture({project_id:'other'}),'project-a')).toBe(false);
 expect(validWorklog(worklogFixture({minutes:1.5}),'project-a')).toBe(false);
 expect(validWorklogPage(worklogPageFixture({total:31,next_cursor:'next',summary:{active_count:30,voided_count:1,total_minutes:999}}),'project-a')).toBe(true);
 expect(validWorklogPage(worklogPageFixture({summary:{active_count:2,voided_count:0,total_minutes:45}}),'project-a')).toBe(false);
 expect(validWorklogPage(worklogPageFixture({items:[worklogFixture({can_void:'yes'})]}),'project-a')).toBe(false);
});

it('binds recovery to original project, path, task version and immutable member and validates the exact receipt',()=>{
 const body={project_id:'project-a',task_id:'task-a',expected_task_version:2,member_id:'member-a',work_date:'2026-10-28',minutes:45,note:'Explicit work',on_behalf_ack:false,idempotency_key:'operation-a'};
 const request={path:worklogPath(),method:'POST',idempotency_key:'operation-a',body};
 expect(validWorklogRequest(request,'project-a','create')).toBe(true);
 expect(validWorklogReceipt(worklogFixture(),request,'project-a','create')).toBe(true);
 expect(validWorklogReceipt(worklogFixture({task_version:3}),request,'project-a','create')).toBe(false);
 expect(validWorklogRequest({...request,body:{...body,expected_task_version:0}},'project-a','create')).toBe(false);
 const correct={...request,path:worklogPath('log-a','corrections'),body:{...body,expected_version:1,reason:'Correct note'}};
 delete (correct.body as Partial<typeof body>).member_id;
 expect(validWorklogRequest(correct,'project-a','correct','log-a')).toBe(true);
 expect(validWorklogRequest({...correct,body:{...correct.body,member_id:'other'}},'project-a','correct','log-a')).toBe(false);
 expect(validWorklogReceipt(worklogFixture({version:2,reason:'Correct note'}),correct,'project-a','correct','log-a','member-a')).toBe(true);
 expect(validWorklogReceipt(worklogFixture({version:2,member_id:'other',reason:'Correct note'}),correct,'project-a','correct','log-a','member-a')).toBe(false);
});

it('locates an original revision operation in its exact actor/project/week slot even when its head leaves the list',()=>{
 const scope=worklogScope('owner-a','org-a','project-a','2026-10-26'),request={path:'/worklogs/log-a/void',method:'POST',idempotency_key:'operation-a',body:{project_id:'project-a',expected_version:2,reason:'Wrong resource',on_behalf_ack:false,idempotency_key:'operation-a'}};
 const storage={getItem:(key:string)=>key===worklogRecoveryKey(scope)?JSON.stringify(request):null};
 expect(worklogRecovery(storage,scope,'project-a')).toMatchObject({mode:'void',log:'log-a',request});
 expect(worklogRecovery(storage,worklogScope('other','org-a','project-a','2026-10-26'),'project-a')).toBeUndefined();
 expect(worklogRecovery(storage,scope,'project-b')).toEqual({invalid:true});
});
