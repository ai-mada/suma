// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {parseWorkbenchRoute} from './navigation';
import {RunTeamOrigin} from './RunTeamOrigin';
import {taskDeliveryNavigateEvent} from './task-delivery-navigation-model';

const project='project-a';
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
function linkedPlan(run='run-a',name='Delivery team'){
 return {items:[{
  id:'plan-a',project_id:project,team_id:'team-a',team_name:name,revision:4,
  items:[{run_id:run,task_id:'task-a',dispatch_plan_id:'dispatch-a',role_title:'Developer',
   plan:{id:'dispatch-a',project_id:project,task_id:'task-a',run_id:run}}],
 }],total:1};
}
let host:HTMLDivElement,root:Root,requests:{url:URL;init?:RequestInit}[];
let reply:()=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');
 document.body.append(host);
 root=createRoot(host);
 requests=[];
 reply=async()=>Response.json(linkedPlan());
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  requests.push({url:new URL(input,location.href),init});
  return reply();
 }));
});
afterEach(async()=>{
 await act(async()=>root.unmount());
 host.remove();
 vi.unstubAllGlobals();
 await setLocale('zh-CN',{persist:false});
});
async function settle(){
 for(let i=0;i<3;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
}
async function render(run='run-a',identity=session){
 await act(async()=>root.render(<ApiViewProvider session={identity}><RunTeamOrigin project={project} run={run}/></ApiViewProvider>));
 await settle();
}
async function click(selector:string){
 const control=host.querySelector<HTMLElement>(selector);
 expect(control,selector).not.toBeNull();
 await act(async()=>control!.click());
 await settle();
}
const state=()=>host.querySelector('[data-run-team-origin-state]')?.getAttribute('data-run-team-origin-state');
const link=()=>host.querySelector<HTMLAnchorElement>('[data-run-team-origin-plan]');

it('looks up only the selected run on demand, retries an invalid binding and opens the verified team plan',async()=>{
 await render();
 expect(requests).toHaveLength(0);
 const mismatched=linkedPlan();
 mismatched.items[0].items[0].plan.run_id='another-run';
 reply=async()=>Response.json(mismatched);
 await click('[data-run-team-origin-toggle]');
 expect(requests).toHaveLength(1);
 const [{url,init}]=requests;
 expect(url.pathname).toBe('/api/v1/expert-team-plans');
 expect(Object.fromEntries(url.searchParams)).toEqual({project_id:project,run_id:'run-a',limit:'1'});
 expect(init?.method).toBe('GET');
 expect(new Headers(init?.headers).get('X-Suma-Expected-Organization')).toBe(session.organization.id);
 expect(new Headers(init?.headers).get('X-Suma-Expected-User')).toBe(session.user.id);
 expect(state()).toBe('error');
 expect(link()).toBeNull();
 expect(host.textContent).not.toContain('Delivery team');

 reply=async()=>Response.json(linkedPlan());
 await click('[data-run-team-origin-retry]');
 expect(state()).toBe('found');
 expect(document.activeElement).toBe(host.querySelector('[data-run-team-origin-toggle]'));
 expect(host.textContent).toContain('Developer · Current plan revision 4');
 expect(parseWorkbenchRoute(new URL(link()!.href).search)).toMatchObject({project,view:'development',sub:'runs',layout:'team',panel:'expert-team:plan-a',run:''});
 const guard=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener('suma:navigate',guard,{once:true});
 await click('[data-run-team-origin-plan]');
 expect(guard).toHaveBeenCalledOnce();
 const navigate=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener(taskDeliveryNavigateEvent,navigate,{once:true});
 await click('[data-run-team-origin-plan]');
 expect(navigate.mock.calls[0][0]).toHaveProperty('detail',link()!.getAttribute('href'));
 expect(requests).toHaveLength(2);
});

it('discards an old read when the account or run changes and separates denial from a verified absence',async()=>{
 let finishOld!:(response:Response)=>void;
 reply=()=>new Promise(resolve=>{finishOld=resolve;});
 await render();
 await click('[data-run-team-origin-toggle]');
 expect(state()).toBe('loading');
 const oldSignal=requests[0].init?.signal;
 const newSession={...session,organization:{id:'org-b',name:'Other organization'}};
 await render('run-b',newSession);
 expect(oldSignal?.aborted).toBe(true);
 expect(state()).toBeUndefined();
 await act(async()=>finishOld(Response.json(linkedPlan('run-a','Old team'))));
 await settle();
 expect(host.textContent).not.toContain('Old team');

 reply=async()=>Response.json({error:{code:'forbidden',message:'Forbidden'}},{status:403});
 await click('[data-run-team-origin-toggle]');
 expect(state()).toBe('denied');
 expect(link()).toBeNull();
 expect(host.querySelector('[data-run-team-origin-retry]')).toBeNull();
 expect(requests.at(-1)?.url.searchParams.get('run_id')).toBe('run-b');

 await click('[data-run-team-origin-toggle]');
 reply=async()=>Response.json({items:[],total:0});
 await click('[data-run-team-origin-toggle]');
 expect(state()).toBe('none');
 expect(link()).toBeNull();
});
