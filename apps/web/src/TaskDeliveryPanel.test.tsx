// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {TaskDeliveryPanel} from './TaskDeliveryPanel';
const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const summary=()=>({total:1,waiting:0,running:0,succeeded:1,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[{id:'verify',name:'Independent checks',type:'test',state:'succeeded',attempt:1}]});
function delivery(task='task-a'){return {project_id:'project-a',task_id:task,task_version:3,test_plan_id:'quality-plan-a',task_status:'awaiting_review',run_basis_state:'current',formal_acceptance:false,run:{run_id:'run-a',version:4,state:'succeeded',nodes:summary(),tests:summary()},expert_plans:{scope:'current_run',total:1,limit:20,complete:true,items:[{plan_id:'plan-a',version:2,revision:1,team_id:'team-a',team_name:'Engineering',state:'active',item_state:'awaiting_review'}]},issues:{scope:'current_run',total:1,limit:20,complete:true,items:[{issue_id:'issue-a',title:'Review finding',issue_version:1,issue_status:'open',source:'review_rejected',source_review_id:'review-a',requirement_id:'req-a',requirement_version:2}]}};}
let root:Root|undefined,host:HTMLDivElement,reads:{path:string;signal?:AbortSignal|null}[],writes:number;
beforeEach(async()=>{await setLocale('en-US',{persist:false});reads=[];writes=0;host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
function transport(reply:(path:string)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(options?.body)writes++;else reads.push({path,signal:options?.signal});return reply(path);}));}
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(task='task-a',callbacks:Record<string,any>={},s=session){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={s}><TaskDeliveryPanel session={s} project="project-a" taskId={task} taskVersion={3} refresh={0} {...callbacks}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const element=host.querySelector<HTMLButtonElement>(selector);expect(element,selector).not.toBeNull();expect(element!.disabled).toBe(false);await act(async()=>element!.click());await settle();}

it('reads one task only when opened, separates execution/tests/acceptance and drills into exact current references',async()=>{
 transport(async()=>Response.json(delivery()));const run=vi.fn(),issue=vi.fn(),requirement=vi.fn(),plan=vi.fn();await render('task-a',{onOpenRun:run,onOpenIssue:issue,onOpenRequirement:requirement,onTeamPlan:plan});expect(reads).toHaveLength(0);await click('[data-task-delivery-toggle]');expect(reads.map(read=>read.path)).toEqual(['/tasks/task-a/delivery?project_id=project-a']);expect(host.textContent).toContain('Not accepted');expect(host.textContent).toContain('not approval eligibility');
 await click('[data-task-delivery-test="verify"]');expect(run).toHaveBeenCalledWith({run:'run-a',node:'verify',attempt:'1'});await click('[data-task-delivery-issue="issue-a"]');expect(issue).toHaveBeenCalledWith('issue-a');await click('[data-task-delivery-requirement="req-a"]');expect(requirement).toHaveBeenCalledWith('req-a');await click('[data-task-delivery-plan="plan-a"]');expect(plan).toHaveBeenCalledWith('plan-a');expect(writes).toBe(0);
 const quality=host.querySelector<HTMLAnchorElement>('[data-task-delivery-test-plan]')!;const target=new URL(quality.href);expect(target.searchParams.get('view')).toBe('quality');expect(target.searchParams.get('record')).toBe('quality-plan-a');expect(target.searchParams.get('run')).toBe('run-a');
 const deny=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',deny);const protectedClick=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>{quality.dispatchEvent(protectedClick);});expect(protectedClick.defaultPrevented).toBe(true);window.removeEventListener('suma:navigate',deny);
 await render('task-a',{navigationDisabled:true});const locked=host.querySelector<HTMLAnchorElement>('[data-task-delivery-test-plan]')!;expect(locked.getAttribute('aria-disabled')).toBe('true');expect(locked.tabIndex).toBe(-1);const lockedClick=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>{locked.dispatchEvent(lockedClick);});expect(lockedClick.defaultPrevented).toBe(true);
});
it('aborts a previous task read and never renders its late delivery under another task or owner',async()=>{
 let release!:(value:Response)=>void;transport(async path=>path.includes('/task-a/')?new Promise<Response>(resolve=>{release=resolve;}):Response.json(delivery('task-b')));await render();await click('[data-task-delivery-toggle]');const signal=reads[0].signal;await render('task-b',{}, {...session,user:{...session.user,id:'user-b'}});expect(signal?.aborted).toBe(true);await act(async()=>release(Response.json(delivery())));await settle();expect(host.querySelector('[data-task-delivery-run]')).toBeNull();await click('[data-task-delivery-toggle]');expect(reads[1].path).toContain('/task-b/delivery');expect(host.querySelector('[data-task-delivery-run]')).not.toBeNull();
});
it('hides revoked facts and retries with a new read scope, without creating work',async()=>{
 let denied=false;transport(async()=>denied?Response.json({error:{code:'forbidden',message:'No access'}},{status:403}):Response.json(delivery()));await render();await click('[data-task-delivery-toggle]');denied=true;await click('[data-task-delivery-refresh]');expect(host.querySelector('[data-task-delivery-run]')).toBeNull();denied=false;await click('[data-task-delivery-retry]');expect(host.querySelector('[data-task-delivery-run]')).not.toBeNull();expect(reads).toHaveLength(3);expect(writes).toBe(0);
});
it('rejects malformed or foreign projections before exposing navigation and localizes its own controls',async()=>{
 transport(async()=>Response.json({...delivery(),task_id:'other'}));await render();await click('[data-task-delivery-toggle]');expect(host.querySelector('[data-task-delivery-run]')).toBeNull();expect(host.textContent).toContain('could not be verified');await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('交付与质量');expect(host.querySelector('[data-task-delivery-test]')).toBeNull();
});

it('opens an unstarted test node without inventing an attempt zero',async()=>{
 const value=delivery(),run=vi.fn();value.run.state='running';
 for(const part of [value.run.nodes,value.run.tests]){part.waiting=1;part.succeeded=0;part.items[0].state='pending';part.items[0].attempt=0;}
 transport(async()=>Response.json(value));await render('task-a',{onOpenRun:run});await click('[data-task-delivery-toggle]');await click('[data-task-delivery-test="verify"]');
 expect(run).toHaveBeenCalledWith({run:'run-a',node:'verify'});expect(host.textContent).not.toContain('Attempt 0');
});

it('extracts accepted delivery only from a current formal acceptance and honors navigation and callback locks',async()=>{
 const value={...delivery(),task_status:'accepted'},extract=vi.fn();transport(async()=>Response.json(value));
 await render('task-a',{onExtractAcceptedDelivery:extract});await click('[data-task-delivery-toggle]');expect(host.querySelector('[data-task-delivery-extract]')).toBeNull();
 value.formal_acceptance=true;await click('[data-task-delivery-refresh]');const action=host.querySelector<HTMLButtonElement>('[data-task-delivery-extract]');expect(action).not.toBeNull();expect(action!.disabled).toBe(false);const count=reads.length;
 const prevent=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',prevent);try{await click('[data-task-delivery-extract]');expect(extract).not.toHaveBeenCalled();}finally{window.removeEventListener('suma:navigate',prevent);}
 await click('[data-task-delivery-extract]');expect(extract).toHaveBeenCalledExactlyOnceWith('task-a');expect(reads).toHaveLength(count);expect(writes).toBe(0);
 await render('task-a',{onExtractAcceptedDelivery:extract,navigationDisabled:true});expect(host.querySelector<HTMLButtonElement>('[data-task-delivery-extract]')!.disabled).toBe(true);
 await render();expect(host.querySelector<HTMLButtonElement>('[data-task-delivery-extract]')!.disabled).toBe(true);
 await render('task-a',{onExtractAcceptedDelivery:extract,taskVersion:4});expect(host.querySelector('[data-task-delivery-extract]')).toBeNull();expect(extract).toHaveBeenCalledOnce();expect(writes).toBe(0);
});

it('retires delivery extraction on revoked access and ignores late accepted facts after a task owner change',async()=>{
 let mode:'accepted'|'denied'|'pending'='accepted',release!:(value:Response)=>void;const extract=vi.fn(),forbidden=vi.fn();
 transport(async path=>mode==='denied'?Response.json({error:{code:'forbidden',message:'No access'}},{status:403}):mode==='pending'&&path.includes('/task-a/')?new Promise<Response>(resolve=>{release=resolve;}):Response.json({...delivery(path.includes('/task-b/')?'task-b':'task-a'),task_status:'accepted',formal_acceptance:mode==='accepted'}));
 await render('task-a',{onExtractAcceptedDelivery:extract,onForbidden:forbidden});await click('[data-task-delivery-toggle]');expect(host.querySelector('[data-task-delivery-extract]')).not.toBeNull();
 mode='denied';await click('[data-task-delivery-refresh]');expect(host.querySelector('[data-task-delivery-extract]')).toBeNull();expect(forbidden).toHaveBeenCalledOnce();
 mode='pending';await click('[data-task-delivery-retry]');const signal=reads.at(-1)!.signal;await render('task-b',{onExtractAcceptedDelivery:extract},{...session,user:{...session.user,id:'user-b'}});expect(signal?.aborted).toBe(true);
 await act(async()=>release(Response.json({...delivery(),task_status:'accepted',formal_acceptance:true})));await settle();expect(host.querySelector('[data-task-delivery-extract]')).toBeNull();await click('[data-task-delivery-toggle]');expect(reads.at(-1)!.path).toBe('/tasks/task-b/delivery?project_id=project-a');expect(host.querySelector('[data-task-delivery-extract]')).toBeNull();expect(extract).not.toHaveBeenCalled();expect(writes).toBe(0);
});
