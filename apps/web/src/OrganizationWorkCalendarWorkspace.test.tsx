// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {OrganizationWorkCalendar,OrganizationWorkCalendarSaveRequest,ResourceCalendarSchedule} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {OrganizationWorkCalendarWorkspace} from './OrganizationWorkCalendarWorkspace';

const owner:Session={user:{id:'owner-a',name:'Ari',email:'ari@example.test',role:'owner'},organization:{id:'org-a',name:'Studio'},projects:[]};
const member:Session={...owner,user:{id:'member-a',name:'Taylor',email:'taylor@example.test',role:'member'}};
const members={items:[{user_id:owner.user.id,...owner.user},{user_id:member.user.id,...member.user}]};
const week='2026-11-02';
const schedule:ResourceCalendarSchedule={weekdays:[8,0,null,6,6,0,0],exceptions:[{date:week,available_hours:2.5,note:'Availability override'}]};
const head=(user=member.user.id,can_edit=true):OrganizationWorkCalendar=>({organization_id:'org-a',user_id:user,version:2,schedule,updated_at:'2026-11-01T08:00:00Z',can_edit});
type Exchange={path:string;headers:Headers;body?:OrganizationWorkCalendarSaveRequest;signal?:AbortSignal|null};
let root:Root,host:HTMLDivElement,exchanges:Exchange[];

beforeEach(async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();
 host=document.createElement('div');document.body.append(host);root=createRoot(host);exchanges=[];
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();sessionStorage.clear();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
function transport(reply:(request:Exchange)=>Promise<Response>){
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const request:Exchange={path:String(input).replace(/^\/api\/v1/,''),headers:new Headers(init?.headers),body:init?.body?JSON.parse(String(init.body)):undefined,signal:init?.signal};
  exchanges.push(request);return reply(request);
 }));
}
function Page({session,selectedUser,refresh=0}:{session:Session;selectedUser:string;refresh?:number}){
 const [value,setWeek]=useState(week);
 return <ApiViewProvider session={session}><OrganizationWorkCalendarWorkspace session={session} week={value} selectedUser={selectedUser}
  refresh={refresh} onWeek={setWeek} onSelect={()=>{}}/></ApiViewProvider>;
}
async function render(session=owner,selectedUser=member.user.id,refresh=0){
 await act(async()=>root.render(<Page session={session} selectedUser={selectedUser} refresh={refresh}/>));await settle();
}
async function click(selector:string){
 const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();
}
async function change(name:string,value:string){
 const input=document.querySelector<HTMLInputElement>('input[name="'+name+'"]')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));});
}
async function submit(){
 await act(async()=>{document.querySelector('[data-resource-calendar-form]')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();
}
const day=(value:string)=>host.querySelector<HTMLElement>('[data-org-calendar-day="'+value+'"]');

it('uses a date exception for the saved week, distinguishes zero from unknown, and recomputes on week change without a write',async()=>{
 transport(async req=>Response.json(req.path==='/organization/members'?members:head()));
 await render();
 expect(exchanges.map(req=>req.path)).toEqual(['/organization/members','/organization/work-calendars/member-a']);
 expect(exchanges[1].headers.get('X-Suma-Expected-User')).toBe('owner-a');
 expect(exchanges[1].headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(day(week)?.dataset.exception).toBe('true');expect(day(week)?.textContent).toContain('2.5 h');
 expect(day('2026-11-03')?.dataset.availability).toBe('off');expect(day('2026-11-03')?.textContent).toContain('0 h');
 expect(day('2026-11-04')?.dataset.availability).toBe('unknown');
 expect(host.querySelector('[data-org-calendar-total]')?.textContent).toBe('Unknown');
 await click('[data-org-calendar-next]');expect(day('2026-11-09')?.textContent).toContain('8 h');expect(day(week)).toBeNull();
 expect(exchanges).toHaveLength(2);
});

it('loads only the member’s own read-only calendar even with another member in the route',async()=>{
 transport(async()=>Response.json(head(member.user.id,false)));
 await render(member,owner.user.id);
 expect(exchanges.map(req=>req.path)).toEqual(['/organization/work-calendars/member-a']);
 expect(host.querySelector('[data-org-calendar-member]')).toBeNull();
 expect(host.querySelector('[data-org-calendar-edit]')).toBeNull();
 expect(host.textContent).toContain('This calendar is read-only.');
});

it('saves the organization-scoped draft with its exact version and stable recovery, then reloads saved availability',async()=>{
 let current=head(),lost=true;
 transport(async req=>{
  if(req.path==='/organization/members')return Response.json(members);
  if(!req.body)return Response.json(current);
  if(lost)throw new TypeError('response lost');
  current={...current,version:3,schedule:req.body.schedule};
  return Response.json(current);
 });
 await render();await click('[data-org-calendar-edit]');await change('weekday-2','3.25');await submit();
 const original=exchanges.find(req=>req.body)!;
 expect(original.path).toBe('/organization/work-calendars/member-a');
 expect(original.body).toMatchObject({expected_version:2,schedule:{weekdays:[8,0,3.25,6,6,0,0]}});
 expect(Object.keys(original.body).sort()).toEqual(['expected_version','idempotency_key','schedule']);
 expect(host.querySelector('[data-org-calendar-total]')?.textContent).toBe('Unknown');
 let navigated=true;
 await act(async()=>{navigated=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});
 expect(navigated).toBe(false);
 lost=false;await submit();expect(exchanges.filter(req=>req.body).map(req=>req.body)).toEqual([original.body,original.body]);
 expect(original.headers.get('X-Suma-Expected-User')).toBe('owner-a');
 expect(original.headers.get('X-Suma-Expected-Organization')).toBe('org-a');
 expect(document.querySelector('[data-resource-calendar-form]')).toBeNull();
 expect(host.querySelector('[data-org-calendar-total]')?.textContent).toBe('17.75 h');
});

it('aborts old view requests and clears saved facts on a refresh that loses access',async()=>{
 let release:(response:Response)=>void=()=>{},delay=true,deny=false;
 transport(async req=>{
  if(req.path==='/organization/members')return Response.json(members);
  if(delay&&req.path.endsWith('member-a'))return new Promise<Response>(resolve=>{release=resolve;});
  if(deny)return Response.json({error:{code:'forbidden',message:'Forbidden'}},{status:403});
  return Response.json({...head(owner.user.id),schedule:{weekdays:[1,1,1,1,1,0,0],exceptions:[]}});
 });
 await render();const older=exchanges[1];delay=false;
 await render(owner,owner.user.id);
 expect(older.signal?.aborted).toBe(true);
 await act(async()=>release(Response.json(head())));await settle();
 expect(host.querySelector('[data-org-calendar-total]')?.textContent).toBe('5 h');
 deny=true;await render(owner,owner.user.id,1);
 expect(host.querySelector('[data-org-calendar-total]')).toBeNull();
 expect(host.querySelector('[data-org-calendar-edit]')).toBeNull();
});
