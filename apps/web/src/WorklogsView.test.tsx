// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {WorklogsView} from './WorklogsView';
import {WorklogsEditor} from './WorklogsEditor';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';
import {worklogRecoveryKey,worklogScope} from './worklogs-model';

const session:Session={user:{id:'owner-a',name:'Owner',email:'owner@example.test',role:'admin'},organization:{id:'org-a',name:'Org'},projects:[]};
const log=(patch:Record<string,unknown>={})=>({id:'log-a',project_id:'project-a',version:1,status:'active',source:'human_manual',task_id:'task-a',task_version:2,task_title:'Delivery task',member_id:'member-a',member_version:1,member_name:'Human resource',user_id:'owner-a',user_name:'Owner',work_date:'2026-10-28',minutes:45,note:'Explicit work',reason:'',actor_id:'owner-a',actor_name:'Owner',on_behalf_ack:false,created_at:'2026-10-28T10:00:00Z',updated_at:'2026-10-28T10:00:00Z',can_correct:true,can_void:true,...patch});
const page=(patch:Record<string,unknown>={})=>({project_id:'project-a',items:[log()],total:1,limit:20,next_cursor:'',scope_version:1,summary:{active_count:1,voided_count:0,total_minutes:45},can_create:true,can_manage_others:false,self_member_id:'member-a',...patch});
const task=(version=2,project='project-a')=>({id:'task-a',project_id:project,version,title:'Delivery task',status:'open'});
const members={items:[{id:'member-a',project_id:'project-a',version:1,name:'My human resource',kind:'human',status:'active',user_id:'owner-a'},{id:'member-b',project_id:'project-a',version:1,name:'Another person',kind:'human',status:'active',user_id:'other-user'},{id:'machine',project_id:'project-a',version:1,name:'Machine',kind:'agent',status:'active'}],total:3};
let host:HTMLDivElement,root:Root,writes:{path:string;body:any;method:string;headers:Headers}[],reads:{path:string;signal?:AbortSignal|null}[];
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);writes=[];reads=[];});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
function transport(reply?:(path:string,init?:RequestInit)=>Promise<Response|undefined>){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(init?.body)writes.push({path,body:JSON.parse(String(init.body)),method:String(init.method),headers:new Headers(init.headers)});else reads.push({path,signal:init?.signal});const response=await reply?.(path,init);if(response)return response;if(path.startsWith('/members?'))return Response.json(members);if(path.startsWith('/tasks?'))return Response.json({items:[task()],total:1});if(path.startsWith('/tasks/'))return Response.json(task());if(path.startsWith('/worklogs/log-a?'))return Response.json(log());if(init?.body){const body=JSON.parse(String(init.body));return Response.json(log({...body,task_version:body.expected_task_version??2,version:body.expected_version?body.expected_version+1:1,status:path.endsWith('/void')?'voided':'active'}));}return Response.json(page());}));}
async function render(view=session,project='project-a',week='2026-10-26',onWeek:((value:string)=>boolean)=()=>true){await act(async()=>root.render(<ApiViewProvider session={view}><WorklogsView project={project} week={week} onWeek={onWeek} refresh={0}/></ApiViewProvider>));await settle();}
const find=<T extends Element=HTMLElement>(selector:string)=>document.querySelector<T>(selector)!;
async function click(selector:string){expect(find(selector),selector).not.toBeNull();await act(async()=>find<HTMLButtonElement>(selector).click());await settle();}
async function change(name:string,value:string){const input=find<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement>('[name="'+name+'"]');expect(input,name).not.toBeNull();await act(async()=>{const prototype=input instanceof HTMLSelectElement?HTMLSelectElement.prototype:input instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function submit(){await act(async()=>{find('form[data-worklog-editor]').dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}

it('shows server totals across the whole week, retains voided heads, and loads immutable history only on demand in both languages',async()=>{
 transport(async path=>path.includes('/revisions?')?Response.json({project_id:'project-a',worklog_id:'log-a',items:[log({version:2,status:'voided',reason:'Incorrect entry',can_correct:false,can_void:false}),log({can_correct:false,can_void:false})],total:2,limit:20,next_cursor:'',scope_version:2}):path.startsWith('/worklogs?')?Response.json(page({items:[log(),log({id:'log-b',status:'voided',can_correct:false,can_void:false})],total:8,summary:{active_count:6,voided_count:2,total_minutes:999}})):undefined);
 await render();expect(reads[0].path).toContain('from_date=2026-10-26&to_date=2026-11-01&status=all&limit=20');expect(host.querySelector('.worklogs-totals')?.textContent).toContain('999');expect(host.textContent).toContain('Voided');expect(reads.some(item=>item.path.includes('/revisions?'))).toBe(false);expect(writes).toHaveLength(0);
 await click('[data-worklog-history="log-a"]');expect(document.body.textContent).toContain('Incorrect entry');expect(document.querySelectorAll('.worklogs-revision')).toHaveLength(2);await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(document.body.textContent).toContain('工时修订历史');expect(host.textContent).toContain('已登记工时');expect(writes).toHaveLength(0);
});

it('defaults to the server-confirmed self resource, uses an explicit current task version, and never infers management from the session role',async()=>{
 transport();await render();await click('[data-worklog-new]');expect(find<HTMLSelectElement>('[name=worklog-members]').value).toBe('member-a');expect(find('[name=worklog-members]').textContent).not.toContain('Another person');expect(find('[name=worklog-members]').textContent).not.toContain('Machine');expect(writes).toHaveLength(0);
 await change('worklog-tasks','task-a');await change('worklog-minutes','45');await change('worklog-note','Saved manually');await submit();expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:'/worklogs',method:'POST',body:{project_id:'project-a',member_id:'member-a',task_id:'task-a',expected_task_version:2,work_date:'2026-10-26',minutes:45,note:'Saved manually',on_behalf_ack:false}});expect(writes[0].headers.get('X-Suma-Expected-User')).toBe('owner-a');
});

it('requires current management and explicit acknowledgement before an on-behalf registration',async()=>{
 transport(async path=>path.startsWith('/worklogs?')?Response.json(page({can_manage_others:true})):undefined);await render();await click('[data-worklog-new]');await change('worklog-members','member-b');await change('worklog-tasks','task-a');await change('worklog-minutes','60');await submit();expect(writes).toHaveLength(0);expect(document.body.textContent).toContain('Confirm that you are acting');await click('[name=worklog-ack]');await submit();expect(writes[0].body).toMatchObject({member_id:'member-b',on_behalf_ack:true,minutes:60});
});

it('keeps correction fields through a conflict, then adopts only refreshed head/task versions with an explicit reason',async()=>{
 let version=3,taskVersion=5,conflict=true;transport(async(path,init)=>{if(path==='/worklogs/log-a/corrections'&&init?.body){if(conflict)return Response.json({error:{code:'version_conflict',message:'Changed elsewhere'}},{status:409});const body=JSON.parse(String(init.body));return Response.json(log({...body,version:body.expected_version+1,task_version:body.expected_task_version}));}if(path.startsWith('/worklogs/log-a?'))return Response.json(log({version}));if(path.startsWith('/tasks/task-a?'))return Response.json(task(taskVersion));return undefined;});
 await render();await click('[data-worklog-correct="log-a"]');expect(find<HTMLInputElement>('[name=worklog-member]').readOnly).toBe(true);expect(document.querySelector('[name=worklog-members]')).toBeNull();await change('worklog-minutes','70');await change('worklog-reason','Correct duration');await submit();expect(writes[0].body).toMatchObject({expected_version:3,expected_task_version:5,minutes:70,reason:'Correct duration'});expect(writes[0].body).not.toHaveProperty('member_id');
 version=6;taskVersion=7;await click('.conflict-banner button');expect(find<HTMLInputElement>('[name=worklog-minutes]').value).toBe('70');expect(find<HTMLTextAreaElement>('[name=worklog-reason]').value).toBe('Correct duration');conflict=false;await submit();expect(writes[1].body).toMatchObject({expected_version:6,expected_task_version:7,minutes:70});expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
});

it('voids only the current authorized active head with a reason and keeps the original immutable facts',async()=>{
 transport();await render();await click('[data-worklog-void="log-a"]');await submit();expect(writes).toHaveLength(0);await change('worklog-reason','Wrong resource, register again');await submit();expect(writes[0]).toMatchObject({path:'/worklogs/log-a/void',body:{project_id:'project-a',expected_version:1,reason:'Wrong resource, register again',on_behalf_ack:false}});expect(writes[0].body).not.toHaveProperty('minutes');expect(writes[0].body).not.toHaveProperty('member_id');
});

it('restores a lost save after remount with the exact body and operation despite a locale change',async()=>{
 let lost=true;transport(async(_path,init)=>{if(init?.body&&lost)throw TypeError('lost response');return undefined;});const saved=vi.fn(),authority={can_create:true,can_manage_others:false,self_member_id:'member-a'};
 const editor=()=>root.render(<ApiViewProvider session={session}><WorklogsEditor project="project-a" week="2026-10-26" mode="create" authority={authority} onSaved={saved} onCancel={()=>{}} onDenied={()=>{}}/></ApiViewProvider>);
 await act(async()=>editor());await settle();await change('worklog-tasks','task-a');await change('worklog-minutes','90');await submit();const original=writes[0];expect(saved).not.toHaveBeenCalled();expect(sessionStorage.length).toBe(1);expect(find('[name=worklog-minutes]').matches(':disabled')).toBe(true);
 await act(async()=>root.render(<div/>));await act(async()=>{await setLocale('zh-CN',{persist:false});editor();});await settle();expect(find<HTMLInputElement>('[name=worklog-minutes]').value).toBe('90');lost=false;await submit();expect(writes[1]).toMatchObject({path:original.path,method:original.method,body:original.body});expect(writes[1].headers.get('X-Suma-Expected-User')).toBe(original.headers.get('X-Suma-Expected-User'));expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe(original.headers.get('X-Suma-Expected-Organization'));expect(saved).toHaveBeenCalledTimes(1);expect(sessionStorage.length).toBe(0);
});

it('exposes original-operation recovery after a completed void leaves no actionable visible head',async()=>{
 const original={path:'/worklogs/log-a/void',method:'POST',idempotency_key:'original-void-a',body:{project_id:'project-a',expected_version:1,reason:'Wrong resource',on_behalf_ack:false,idempotency_key:'original-void-a'}};
 sessionStorage.setItem(worklogRecoveryKey(worklogScope('owner-a','org-a','project-a','2026-10-26')),JSON.stringify(original));
 transport(async(path,init)=>init?.body?Response.json(log({version:2,status:'voided',reason:'Wrong resource',can_correct:false,can_void:false})):path.startsWith('/worklogs/log-a?')?Response.json(log({version:2,status:'voided',can_correct:false,can_void:false})):path.startsWith('/worklogs?')?Response.json(page({items:[],total:0,summary:{active_count:0,voided_count:0,total_minutes:0},can_create:false})):undefined);
 await render();expect(host.querySelector('[data-worklog-new]')).toBeNull();expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(false);await click('[data-worklog-restore]');await submit();expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:original.path,method:original.method,body:original.body});expect(writes[0].headers.get('X-Suma-Expected-User')).toBe('owner-a');expect(writes[0].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(sessionStorage.length).toBe(0);expect(document.querySelector('[data-worklog-editor]')).toBeNull();
});

it('blocks leaving a dirty draft and retains the natural week when route navigation refuses it',async()=>{
 transport();const onWeek=vi.fn(()=>window.dispatchEvent(new Event('suma:navigate',{cancelable:true})));await render(session,'project-a','2026-10-26',onWeek);await click('[data-worklog-new]');await change('worklog-minutes','30');expect(window.dispatchEvent(new Event('suma:navigate',{cancelable:true}))).toBe(false);await change('worklog-week','2026-11-02');expect(onWeek).toHaveBeenCalledWith('2026-11-02');expect(find<HTMLInputElement>('[name=worklog-week]').value).toBe('2026-10-26');expect(find<HTMLInputElement>('[name=worklog-minutes]').value).toBe('30');expect(writes).toHaveLength(0);
});

it('clears stale pages on a cursor conflict and explicitly restarts from the first page',async()=>{
 transport(async path=>path.startsWith('/worklogs?')?path.includes('cursor=')?Response.json({error:{code:'scope_conflict',message:'Scope changed'}},{status:409}):Response.json(page({next_cursor:'next-a'})):undefined);await render();await click('[data-worklog-next]');expect(host.querySelector('tbody')).toBeNull();expect(host.textContent).toContain('Refresh from the first page');await click('[data-worklog-reset]');expect(host.textContent).toContain('Explicit work');expect(reads.filter(item=>item.path.startsWith('/worklogs?')).at(-1)?.path).not.toContain('cursor=');
});

it('hides all protected list and editor data immediately when a write loses authorization',async()=>{
 transport(async(_path,init)=>init?.body?Response.json({error:{code:'forbidden',message:'Access revoked'}},{status:403}):undefined);await render();await click('[data-worklog-void="log-a"]');await change('worklog-reason','Remove duplicate');await submit();expect(host.textContent).not.toContain('Explicit work');expect(host.textContent).not.toContain('Human resource');expect(document.querySelector('[data-worklog-editor]')).toBeNull();expect(host.textContent).toContain('Protected work records have been cleared');
});

it('ignores a late project response and displays only fresh server read-only capabilities in the new scope',async()=>{
 let release:(value:Response)=>void=()=>{};transport(async path=>path.includes('project_id=project-a')?new Promise<Response>(resolve=>{release=resolve;}):Response.json(page({project_id:'project-b',items:[log({project_id:'project-b',task_title:'Private B task',can_correct:false,can_void:false})],can_create:false,can_manage_others:false,self_member_id:''})));
 await render();const original=reads[0];await render({...session,user:{...session.user,id:'other-user'}},'project-b');expect(original.signal?.aborted).toBe(true);await act(async()=>release(Response.json(page())));await settle();expect(host.textContent).toContain('Private B task');expect(host.textContent).not.toContain('Delivery task');expect(host.querySelector('[data-worklog-new]')).toBeNull();expect(host.querySelector('[data-worklog-correct]')).toBeNull();expect(host.querySelector('[data-worklog-void]')).toBeNull();expect(writes).toHaveLength(0);
});
