// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {Planning} from './Planning';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {Session} from './api';
import type {PlanningData} from './planning-model';

const session:Session={user:{id:'owner-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const planning=(total=9,title='Private planning task'):PlanningData=>({tasks:[{id:'task-a',project_id:'project-a',version:2,title,status:'open',plan:{version:1,iteration_id:'',milestone_id:'',start_date:'',due_date:'',estimated_hours:17}}],iterations:[],milestones:[],baselines:[],capacity:[],metrics:{total_tasks:total,accepted_tasks:2,blocked_tasks:0,scheduled_tasks:1,estimated_tasks:1,estimated_hours:17,overdue_tasks:0,acceptance_percent:null},as_of:'2026-10-29T10:00:00Z',total});
const worklogs=()=>({project_id:'project-a',items:[],total:0,limit:20,next_cursor:'',scope_version:0,summary:{active_count:0,voided_count:0,total_minutes:0},can_create:true,can_manage_others:false,self_member_id:'member-a'});
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);await setLocale('en-US',{persist:false});sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let index=0;index<4;index++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(refresh=0,layout='worklogs',onSelect=vi.fn()){await act(async()=>root.render(<ApiViewProvider session={session}><Planning session={session} project="project-a" week="2026-10-26" onWeek={()=>true} refresh={refresh} mode="planning" layout={layout} onLayout={()=>{}} selected="old-selection" onSelect={onSelect} onOpenRisk={()=>{}}/></ApiViewProvider>));await settle();}
async function retry(){await act(async()=>host.querySelector<HTMLButtonElement>('.error button')!.click());await settle();}

it('clears parent planning facts and selection on a child denial and prevents an older pending parent response from reviving them',async()=>{
 let planningReads=0,worklogReads=0,retryAuthorized=false,release:(value:Response)=>void=()=>{};
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const path=String(input).replace(/^\/api\/v1/,'');
  if(path.startsWith('/planning?')){planningReads++;if(planningReads===1)return Response.json(planning());if(!retryAuthorized)return new Promise<Response>(resolve=>{release=resolve;});return Response.json(planning(10,'Fresh current task'));}
  if(path.startsWith('/worklogs?')){worklogReads++;if(worklogReads>1&&!retryAuthorized)return Response.json({error:{code:'forbidden',message:'Access revoked'}},{status:403});return Response.json(worklogs());}
  throw Error('Unexpected request '+path);
 }));
 const onSelect=vi.fn();await render(0,'worklogs',onSelect);expect(host.querySelector('.planning-summary')).not.toBeNull();expect(host.querySelector('.planning-coverage')).not.toBeNull();
 await render(1,'worklogs',onSelect);expect(host.querySelector('.planning-summary')).toBeNull();expect(host.querySelector('.planning-coverage')).toBeNull();expect(host.querySelector('.worklogs-view')).toBeNull();expect(onSelect).toHaveBeenCalledWith('');expect(host.textContent).toContain('Protected work records have been cleared');
 await act(async()=>release(Response.json(planning(777,'Late private task'))));await settle();expect(host.querySelector('.planning-summary')).toBeNull();expect(host.textContent).not.toContain('777');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('访问权限已变化');retryAuthorized=true;await retry();expect(host.querySelector('.planning-summary')).not.toBeNull();expect(host.querySelector('.plan-stats')?.textContent).toContain('2 / 10');
});

it('keeps planning facts on a transient refresh failure, but clears them on a parent authorization refusal and retries from current scope',async()=>{
 let status=200;vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const path=String(input).replace(/^\/api\/v1/,'');if(!path.startsWith('/planning?'))throw Error('Unexpected request '+path);return status===200?Response.json(planning()):Response.json({error:{code:status===503?'unavailable':'forbidden',message:status===503?'Temporarily unavailable':'Access revoked'}},{status});}));
 const onSelect=vi.fn();await render(0,'timeline',onSelect);expect(host.textContent).toContain('Private planning task');status=503;await render(1,'timeline',onSelect);expect(host.textContent).toContain('Temporarily unavailable');expect(host.textContent).toContain('Private planning task');expect(host.querySelector('.planning-summary')).not.toBeNull();expect(onSelect).not.toHaveBeenCalled();
 status=401;await render(2,'timeline',onSelect);expect(host.querySelector('.planning-summary')).toBeNull();expect(host.textContent).not.toContain('Private planning task');expect(onSelect).toHaveBeenCalledWith('');expect(host.textContent).toContain('Access has changed');status=200;await retry();expect(host.textContent).toContain('Private planning task');expect(host.querySelector('.planning-summary')).not.toBeNull();
});
