// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ResourceCalendarEditor} from './ResourceCalendarEditor';
import {CapacityView} from './Planning';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import type {Capacity} from './planning-model';
import type {ResourceCalendarSchedule} from '../../../packages/contracts/src';
import {setLocale} from './i18n';

const session:Session={user:{id:'owner-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const unknown:ResourceCalendarSchedule={weekdays:[null,null,null,null,null,null,null],exceptions:[]};
const standard:ResourceCalendarSchedule={weekdays:[8,8,8,8,8,0,0],exceptions:[]};
const calendar=(version=0,schedule:ResourceCalendarSchedule=unknown,member='member-a',project='project-a',can_edit=true)=>({project_id:project,member_id:member,version,schedule,updated_at:'',can_edit});
let host:HTMLDivElement,root:Root,writes:{path:string;body:any;method:string;headers:Headers}[],reads:{path:string;signal?:AbortSignal|null}[];
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});localStorage.clear();sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);writes=[];reads=[];});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
function transport(reply:(path:string,init?:RequestInit)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(init?.body)writes.push({path,body:JSON.parse(String(init.body)),method:String(init.method),headers:new Headers(init.headers)});else reads.push({path,signal:init?.signal});return reply(path,init);}));}
async function render(member='member-a',project='project-a',onSaved=vi.fn(),view=session,week='2026-10-26'){await act(async()=>root.render(<ApiViewProvider session={view}><ResourceCalendarEditor project={project} member={member} week={week} onSaved={onSaved} onCancel={()=>{}}/></ApiViewProvider>));await settle();return onSaved;}
async function click(selector:string){const node=host.querySelector<HTMLButtonElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}
async function change(name:string,value:string){const input=host.querySelector<HTMLInputElement>('[name="'+name+'"]');expect(input,name).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input!.dispatchEvent(new Event('input',{bubbles:true}));input!.dispatchEvent(new Event('change',{bubbles:true}));});}
async function submit(){await act(async()=>{host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();}

it('starts unknown and applies the explicit weekly template only to a guarded draft',async()=>{
 transport(async()=>Response.json(calendar()));await render();expect(host.querySelector<HTMLInputElement>('[name=weekday-0]')!.value).toBe('');expect(host.textContent).toContain('No calendar saved');
 await click('[data-calendar-template]');expect(writes).toHaveLength(0);expect(host.querySelector<HTMLInputElement>('[name=weekday-0]')!.value).toBe('8');expect(host.querySelector<HTMLInputElement>('[name=weekday-6]')!.value).toBe('0');
 let navigated=true;await act(async()=>{navigated=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});expect(navigated).toBe(false);
 await click('[data-calendar-add]');await change('exception-date-0','2026-10-28');await change('exception-hours-0','0');await change('exception-note-0','Leave <script>raw</script> {{date}}');
 transport(async(_path,init)=>{const body=JSON.parse(String(init?.body));return Response.json(calendar(1,body.schedule));});await submit();
 expect(writes[0]).toMatchObject({path:'/resource-calendars/member-a',method:'PUT',body:{project_id:'project-a',expected_version:0,schedule:{weekdays:[8,8,8,8,8,0,0],exceptions:[{date:'2026-10-28',available_hours:0,note:'Leave <script>raw</script> {{date}}'}]}}});expect(writes[0].body).not.toHaveProperty('week_start');expect(host.querySelector('script')).toBeNull();
});

it('preserves the weekly and exception draft through conflict, then explicitly adopts only the current version',async()=>{
 let fresh=false,conflict=true;transport(async(_path,init)=>{if(init?.body){const body=JSON.parse(String(init.body));if(conflict)return Response.json({error:{code:'version_conflict',message:'Changed elsewhere'}},{status:409});return Response.json(calendar(body.expected_version+1,body.schedule));}return Response.json(calendar(fresh?4:2,standard));});await render();
 await change('weekday-0','4');await click('[data-calendar-add]');await change('exception-date-0','2026-10-28');await submit();expect(writes).toHaveLength(1);expect(host.querySelector<HTMLInputElement>('[name=weekday-0]')!.value).toBe('4');expect(host.querySelector<HTMLInputElement>('[name=exception-date-0]')!.value).toBe('2026-10-28');
 fresh=true;await click('.conflict-banner button');expect(host.querySelector<HTMLInputElement>('[name=weekday-0]')!.value).toBe('4');expect(host.querySelector<HTMLInputElement>('[name=exception-hours-0]')!.value).toBe('');
 conflict=false;await submit();expect(writes[1].body.expected_version).toBe(4);expect(writes[1].body.schedule.exceptions[0].available_hours).toBeNull();expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
});

it('recovers a lost save after remount with the exact original body, operation and actor headers',async()=>{
 let lost=true;transport(async(_path,init)=>{if(init?.body){if(lost)throw TypeError('response lost');const body=JSON.parse(String(init.body));return Response.json(calendar(body.expected_version+1,body.schedule));}return Response.json(calendar(2,standard));});await render();await change('weekday-1','0');await submit();const original=writes[0];expect(host.querySelector('[name=weekday-1]')?.matches(':disabled')).toBe(true);
 await act(async()=>root.render(<div/>));await render();expect(host.querySelector<HTMLInputElement>('[name=weekday-1]')!.value).toBe('0');await act(async()=>{await setLocale('zh-CN',{persist:false});});lost=false;await submit();expect(writes[1]).toMatchObject({path:original.path,method:'PUT',body:original.body});expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('owner-a');expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(sessionStorage.length).toBe(0);
});

it('ignores a late read after resource/project/account scope changes and freezes current read-only controls',async()=>{
 let release:(value:Response)=>void=()=>{};transport(async(path)=>path.includes('member-a')?new Promise<Response>(resolve=>{release=resolve;}):Response.json(calendar(0,unknown,'member-b','project-b',false)));
 await render();const pendingRead=reads[0];await render('member-b','project-b',vi.fn(),{...session,user:{...session.user,id:'owner-b'}});expect(pendingRead.signal?.aborted).toBe(true);await act(async()=>release(Response.json(calendar(9,standard))));await settle();expect(host.textContent).not.toContain('version 9');expect(host.querySelector<HTMLInputElement>('[name=weekday-0]')!.value).toBe('');expect(host.querySelector('[name=weekday-0]')?.matches(':disabled')).toBe(true);await submit();expect(writes).toHaveLength(0);
});

it('never consumes the original save receipt in a newly selected week',async()=>{
 let release:(value:Response)=>void=()=>{};transport(async(_path,init)=>init?.body?new Promise<Response>(resolve=>{release=resolve;}):Response.json(calendar(0)));
 const originalSaved=vi.fn(),nextSaved=vi.fn();await render('member-a','project-a',originalSaved);await click('[data-calendar-template]');
 await act(async()=>{host.querySelector('form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();expect(writes).toHaveLength(1);
 await render('member-a','project-a',nextSaved,session,'2026-11-02');await act(async()=>release(Response.json(calendar(1,standard))));await settle();expect(originalSaved).not.toHaveBeenCalled();expect(nextSaved).not.toHaveBeenCalled();expect(host.querySelector<HTMLInputElement>('[name=weekday-0]')!.value).toBe('');expect(sessionStorage.length).toBe(1);
});

it('does not send duplicate date exceptions and distinguishes removal from an unknown override',async()=>{
 transport(async()=>Response.json(calendar(1,standard)));await render();for(let i=0;i<2;i++){await click('[data-calendar-add]');await change('exception-date-'+i,'2026-10-28');}await submit();expect(writes).toHaveLength(0);expect(host.textContent).toContain('only one exception per date');
 await click('[aria-label="Remove date exception 2"]');expect(host.querySelector('[name=exception-hours-0]')).not.toBeNull();await click('[aria-label="Remove date exception 1"]');expect(host.textContent).toContain('Each date inherits');
});

it('renders server daily and total facts without calculating distribution, and reads a calendar only on demand',async()=>{
 transport(async()=>Response.json(calendar()));const row:Capacity={member_id:'member-a',name:'Resource',kind:'human',week_start:'2026-10-26',calendar_version:0,available_hours:19,allocated_hours:7,unestimated_tasks:1,unscheduled_tasks:2,distribution_unknown_tasks:1,distribution_unknown_hours:11,days:Array.from({length:7},(_,index)=>({date:'2026-10-'+(26+index),available_hours:index===0?null:index===1?0:4,source:index===1?'exception':index===0?'unknown':'base'}))};
 row.days[5].date='2026-10-31';row.days[6].date='2026-11-01';await act(async()=>root.render(<ApiViewProvider session={session}><CapacityView project="project-a" week="2026-10-26" onWeek={()=>true} rows={[row]} ready pending={false} routeError="" onRefresh={()=>{}}/></ApiViewProvider>));expect(reads).toHaveLength(0);expect(host.querySelectorAll('.capacity-calendar-day')).toHaveLength(7);expect(host.textContent).toContain('19 h');expect(host.textContent).toContain('7 h');expect(host.textContent).toContain('11 estimated hours could not be distributed');expect(host.textContent).not.toContain('12 h');
 await click('td:last-child button');expect(reads[0].path).toBe('/resource-calendars/member-a?project_id=project-a');expect(writes).toHaveLength(0);
});
