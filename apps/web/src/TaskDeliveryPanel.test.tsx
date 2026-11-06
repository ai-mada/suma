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
beforeEach(async()=>{sessionStorage.clear();await setLocale('en-US',{persist:false});reads=[];writes=0;host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
function transport(reply:(path:string,options?:RequestInit)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(options?.body)writes++;else reads.push({path,signal:options?.signal});return reply(path,options);}));}
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(task='task-a',callbacks:Record<string,any>={},s=session){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={s}><TaskDeliveryPanel session={s} project="project-a" taskId={task} taskVersion={3} refresh={0} {...callbacks}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();if(element instanceof HTMLButtonElement)expect(element.disabled).toBe(false);expect(element!.getAttribute('aria-disabled')).not.toBe('true');await act(async()=>element!.click());await settle();}

it('reads one task only when opened, separates execution/tests/acceptance and drills into exact current references',async()=>{
 transport(async()=>Response.json(delivery()));const run=vi.fn(),issue=vi.fn(),requirement=vi.fn(),plan=vi.fn(),review=vi.fn();await render('task-a',{onOpenRun:run,onOpenReview:review,onOpenIssue:issue,onOpenRequirement:requirement,onTeamPlan:plan});expect(reads).toHaveLength(0);await click('[data-task-delivery-toggle]');expect(reads.map(read=>read.path)).toEqual(['/tasks/task-a/delivery?project_id=project-a']);expect(host.textContent).toContain('Not accepted');expect(host.textContent).toContain('not approval eligibility');
 await click('[data-task-delivery-test="verify"]');expect(run).toHaveBeenCalledWith({run:'run-a',node:'verify',attempt:'1'});await click('[data-task-delivery-issue="issue-a"]');expect(issue).toHaveBeenCalledWith('issue-a');await click('[data-task-delivery-requirement="req-a"]');expect(requirement).toHaveBeenCalledWith('req-a');await click('[data-task-delivery-plan="plan-a"]');expect(plan).toHaveBeenCalledWith('plan-a');await click('[data-task-delivery-review]');expect(review).toHaveBeenCalledExactlyOnceWith();expect(writes).toBe(0);
 const quality=host.querySelector<HTMLAnchorElement>('[data-task-delivery-test-plan]')!;const target=new URL(quality.href);expect(target.searchParams.get('view')).toBe('quality');expect(target.searchParams.get('record')).toBe('quality-plan-a');expect(target.searchParams.get('run')).toBe('run-a');
 const deny=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',deny);const protectedClick=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>{quality.dispatchEvent(protectedClick);});expect(protectedClick.defaultPrevented).toBe(true);window.removeEventListener('suma:navigate',deny);
 await render('task-a',{navigationDisabled:true});const locked=host.querySelector<HTMLAnchorElement>('[data-task-delivery-test-plan]')!;expect(locked.getAttribute('aria-disabled')).toBe('true');expect(locked.tabIndex).toBe(-1);const lockedClick=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>{locked.dispatchEvent(lockedClick);});expect(lockedClick.defaultPrevented).toBe(true);
});

it('keeps delivery actions tied to the verified original run until explicitly switching to current delivery',async()=>{
 const original=delivery(),openCurrent=vi.fn(),runID='run-a';
 let reply:()=>Promise<Response>=async()=>Response.json(original);
 transport(()=>reply());
 const origin='[data-task-delivery-origin]',current='[data-task-delivery-current-link]';
 const onOpenRun=vi.fn();
 await render('task-a',{openOnEntry:true,originRunID:runID,onOpenCurrentDelivery:openCurrent,onOpenRun});
 expect(host.querySelector(origin)?.getAttribute('data-relation')).toBe('current');
 expect(host.querySelector('[data-task-delivery-test="verify"]')).not.toBeNull();
 expect(host.querySelector(current)).toBeNull();
 let resolve!:(response:Response)=>void;
 reply=()=>new Promise<Response>(done=>{resolve=done;});
 await click('[data-task-delivery-refresh]');
 expect(host.querySelector(origin)).toBeNull();
 expect(host.querySelector('[data-task-delivery-review]')).toBeNull();
 const rebound={...original,run:{...original.run,run_id:'run-b'}};
 await act(async()=>resolve(Response.json(rebound)));
 expect(host.querySelector(origin)?.getAttribute('data-relation')).toBe('changed');
 for(const action of ['run','test','test-plan','review','repository','issue','plan'])expect(host.querySelector('[data-task-delivery-'+action+']')).toBeNull();
 expect(new URL(host.querySelector<HTMLAnchorElement>(current)!.href).searchParams.get('panel')).toBe('task-delivery');
 const guard=(event:Event)=>event.preventDefault();
 window.addEventListener('suma:navigate',guard);
 try{await click(current);expect(openCurrent).not.toHaveBeenCalled();}
 finally{window.removeEventListener('suma:navigate',guard);}
 await click(current);expect(openCurrent).toHaveBeenCalledOnce();
 for(const basis of ['not_dispatched','unknown']){
  reply=async()=>Response.json({...original,run:null,run_basis_state:basis});
  await click('[data-task-delivery-refresh]');
  expect(host.querySelector(origin)?.getAttribute('data-relation')).toBe(basis==='not_dispatched'?'unbound':'unknown');
  expect(host.querySelector(current)).not.toBeNull();
 }
 reply=async()=>Response.json({...rebound,task_version:4});
 await click('[data-task-delivery-refresh]');
 expect(host.querySelector(origin)).toBeNull();
 expect(host.querySelector(current)).toBeNull();
 reply=async()=>Response.json({error:{code:'forbidden',message:'No access'}},{status:403});
 await click('[data-task-delivery-refresh]');
 expect(host.querySelector(origin)).toBeNull();
 expect(host.querySelector(current)).toBeNull();
 expect(reads.every(read=>read.path==='/tasks/task-a/delivery?project_id=project-a')).toBe(true);
 expect(writes).toBe(0);
});

it('opens repository integration only from current accepted delivery and makes no write',async()=>{
 const openRepository=vi.fn(),selector='[data-task-delivery-repository]';
 let facts=delivery();
 transport(async()=>Response.json(facts));
 await render('task-a',{onOpenRepository:openRepository});
 await click('[data-task-delivery-toggle]');
 expect(host.querySelector(selector)).toBeNull();
 facts={...facts,task_status:'accepted',formal_acceptance:true};
 await click('[data-task-delivery-refresh]');
 const link=host.querySelector<HTMLAnchorElement>(selector)!;
 expect(link.textContent).toBe('Open repository integration');
 const target=new URL(link.href);
 expect(target.searchParams.get('record')).toBe('task-a');
 expect(target.searchParams.get('panel')).toBe('repository-integration');
 await click(selector);
 expect(openRepository).toHaveBeenCalledOnce();
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector(selector)?.textContent).toBe('查看仓库集成');
 facts={...facts,task_version:4,run_basis_state:'changed'};
 await click('[data-task-delivery-refresh]');
 expect(host.querySelector(selector)).toBeNull();
 expect(writes).toBe(0);
});

it('T214 preserves exact native delivery links without callbacks and disables mismatched or locked review navigation',async()=>{
 let current=delivery(),noRun=false;transport(async()=>Response.json(noRun?{...current,run:null,run_basis_state:'not_dispatched'}:current));
 await render();await click('[data-task-delivery-toggle]');
 const target=(selector:string)=>{const anchor=host.querySelector<HTMLAnchorElement>(selector)!;expect(anchor.tagName).toBe('A');expect(anchor.getAttribute('aria-disabled')).not.toBe('true');return new URL(anchor.href);};
 const plan=target('[data-task-delivery-plan="plan-a"]');expect(plan.searchParams.get('project')).toBe('project-a');expect(plan.searchParams.get('view')).toBe('development');expect(plan.searchParams.get('sub')).toBe('runs');expect(plan.searchParams.get('layout')).toBe('team');expect(plan.searchParams.get('panel')).toBe('expert-team:plan-a');
 const issue=target('[data-task-delivery-issue="issue-a"]');expect(issue.searchParams.get('view')).toBe('quality');expect(issue.searchParams.get('sub')).toBe('issues');expect(issue.searchParams.get('record')).toBe('issue-a');
 const requirement=target('[data-task-delivery-requirement="req-a"]');expect(requirement.searchParams.get('view')).toBe('requirements');expect(requirement.searchParams.get('record')).toBe('req-a');
 const review=target('[data-task-delivery-review]');expect(review.searchParams.get('record')).toBe('task-a');expect(review.searchParams.get('panel')).toBe('task-acceptance:3:run-a');
 // The ordinary native link remains usable; prevent its final jsdom navigation
 // only after observing the component's own event decision.
 let nativeAllowed=false;const capture=(event:Event)=>{nativeAllowed=!event.defaultPrevented;event.preventDefault();};host.addEventListener('click',capture);
 const modified=new MouseEvent('click',{bubbles:true,cancelable:true,ctrlKey:true});await act(async()=>host.querySelector('[data-task-delivery-review]')!.dispatchEvent(modified));host.removeEventListener('click',capture);expect(nativeAllowed).toBe(true);
 const onPlan=vi.fn();await render('task-a',{onTeamPlan:onPlan});
 const deny=(event:Event)=>event.preventDefault();window.addEventListener('suma:navigate',deny);try{
  host.addEventListener('click',capture);const native=new MouseEvent('click',{bubbles:true,cancelable:true,metaKey:true});await act(async()=>host.querySelector('[data-task-delivery-plan]')!.dispatchEvent(native));host.removeEventListener('click',capture);expect(nativeAllowed).toBe(true);expect(onPlan).not.toHaveBeenCalled();
  const prevented=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>host.querySelector('[data-task-delivery-plan]')!.dispatchEvent(prevented));expect(prevented.defaultPrevented).toBe(true);expect(onPlan).not.toHaveBeenCalled();
 }finally{window.removeEventListener('suma:navigate',deny);}
 await render('task-a',{navigationDisabled:true});for(const selector of ['[data-task-delivery-plan]','[data-task-delivery-issue]','[data-task-delivery-requirement]','[data-task-delivery-review]']){const anchor=host.querySelector<HTMLAnchorElement>(selector)!;expect(anchor.hasAttribute('href')).toBe(false);expect(anchor.getAttribute('aria-disabled')).toBe('true');expect(anchor.tabIndex).toBe(-1);}
 const callback=vi.fn();await render('task-a',{taskVersion:4,onOpenReview:callback});const changed=new MouseEvent('click',{bubbles:true,cancelable:true});await act(async()=>host.querySelector('[data-task-delivery-review]')!.dispatchEvent(changed));expect(changed.defaultPrevented).toBe(true);expect(callback).not.toHaveBeenCalled();
 await render();current={...current,run_basis_state:'changed'};await click('[data-task-delivery-refresh]');expect(host.querySelector('[data-task-delivery-review]')?.getAttribute('href')).toBeNull();
 noRun=true;await click('[data-task-delivery-refresh]');expect(host.querySelector('[data-task-delivery-review]')?.getAttribute('href')).toBeNull();expect(writes).toBe(0);
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

it('opens the exact WorkBuddy delivery attempt only with current, authorized facts',async()=>{
 const base=delivery(),onOpenRun=vi.fn(),trace={run_id:'run-a',node_id:'develop',attempt:2};
 const nodes={...summary(),waiting:1,succeeded:0,items:[{id:'develop',name:'Develop',type:'external_workflow',state:'waiting',attempt:2}]};
 const tests={...summary(),total:0,succeeded:0,items:[]};
 let target:unknown=trace,denied=false,basis='current';
 transport(async()=>denied?Response.json({error:{code:'forbidden',message:'No access'}},{status:403}):Response.json({...base,run_basis_state:basis,run:{...base.run,state:'running',nodes,tests,workbuddy_delivery:target}}));
 const action='[data-task-delivery-workbuddy]';
 await render('task-a',{onOpenRun});await click('[data-task-delivery-toggle]');
 expect(host.textContent).toContain('WorkBuddy code delivery pending verification');
 expect(host.textContent).toContain('No test nodes are recorded in this run');
 expect(host.textContent).toContain('Not accepted');
 const link=host.querySelector<HTMLAnchorElement>(action)!;link.focus();expect(document.activeElement).toBe(link);
 const href=link.getAttribute('href');
 expect(Object.fromEntries(new URL(link.href).searchParams)).toEqual({project:'project-a',view:'development',sub:'runs',run:'run-a',node:'develop',attempt:'2'});
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.textContent).toContain('WorkBuddy 代码交付待核对');
 expect(link.textContent).toBe('打开 WorkBuddy 原始尝试');
 const beforeNavigation=reads.length;
 await click(action);expect(onOpenRun).toHaveBeenCalledExactlyOnceWith({run:'run-a',node:'develop',attempt:'2'});expect(reads).toHaveLength(beforeNavigation);
 await render();expect(host.querySelector(action)?.getAttribute('href')).toBe(href);
 await render('task-a',{onOpenRun,taskVersion:4});expect(host.querySelector(action)).toBeNull();
 await render('task-a',{onOpenRun,navigationDisabled:true});
 const locked=host.querySelector<HTMLAnchorElement>(action)!;expect(locked.hasAttribute('href')).toBe(false);expect(locked.tabIndex).toBe(-1);
 await render('task-a',{onOpenRun});
 basis='changed';await click('[data-task-delivery-refresh]');expect(host.querySelector(action)).not.toBeNull();
 for(const invalid of [{...trace,run_id:'run-other'},{...trace,node_id:'other'},{...trace,attempt:0},{...trace,attempt:1},{...trace,private_id:'vendor-task'},null]){
  target=invalid;await click('[data-task-delivery-refresh]');
  expect(host.querySelector(action)).toBeNull();expect(host.textContent).toContain('未能核实所选任务的交付记录');
 }
 target=trace;nodes.items[0].type='command';await click('[data-task-delivery-retry]');expect(host.querySelector(action)).toBeNull();
 nodes.items[0].type='external_workflow';nodes.items[0].state='succeeded';await click('[data-task-delivery-retry]');expect(host.querySelector(action)).toBeNull();
 nodes.items[0].state='waiting';nodes.waiting=0;nodes.succeeded=1;await click('[data-task-delivery-retry]');expect(host.querySelector(action)).toBeNull();
 nodes.waiting=1;nodes.succeeded=0;await click('[data-task-delivery-retry]');expect(host.querySelector(action)).not.toBeNull();
 denied=true;await click('[data-task-delivery-refresh]');expect(host.querySelector(action)).toBeNull();
 expect(onOpenRun).toHaveBeenCalledOnce();expect(writes).toBe(0);
 expect(vi.mocked(fetch).mock.calls.every(([,options])=>options?.method==='GET')).toBe(true);
});

it('opens the original WorkBuddy handoff while the current candidate remains the delivery basis',async()=>{
 const original=delivery(),onOpenRun=vi.fn();
 let kind='local_candidate',denied=false;
 const handoff={run_id:'cloud-run',node_id:'workbuddy-cloud',attempt:2};
 transport(async()=>denied?Response.json({error:{code:'forbidden',message:'No access'}},{status:403}):
  Response.json({...original,run:{...original.run,workbuddy_origin:{...handoff,kind}}}));
 const card='[data-task-delivery-workbuddy-origin]',action='[data-task-delivery-workbuddy-origin-link]';
 await render('task-a',{openOnEntry:true,originRunID:'run-a',onOpenRun});
 expect(host.querySelector(card)?.getAttribute('data-task-delivery-workbuddy-origin')).toBe('local_candidate');
 expect(host.querySelector(card)?.textContent).toContain('Local candidate');
 expect(host.querySelector('[data-task-delivery-workbuddy]')).toBeNull();
 expect(host.querySelector('[data-repository-source-toggle]')).toBeNull();
 const history=host.querySelector<HTMLAnchorElement>('[data-task-delivery-workbuddy-history]')!;
 expect(Object.fromEntries(new URL(history.href).searchParams)).toEqual({project:'project-a',view:'development',sub:'runs',layout:'workbuddy',workbuddy_task:'task-a'});
 const link=host.querySelector<HTMLAnchorElement>(action)!;
 link.focus();expect(document.activeElement).toBe(link);
 expect(Object.fromEntries(new URL(link.href).searchParams)).toEqual({project:'project-a',view:'development',sub:'runs',run:'cloud-run',node:'workbuddy-cloud',attempt:'2'});
 const denyNavigation=(event:Event)=>event.preventDefault();
 window.addEventListener('suma:navigate',denyNavigation);
 try{await click(action);expect(onOpenRun).not.toHaveBeenCalled();}
 finally{window.removeEventListener('suma:navigate',denyNavigation);}
 const beforeNavigation=reads.length;
 await click(action);expect(onOpenRun).toHaveBeenCalledExactlyOnceWith({run:'cloud-run',node:'workbuddy-cloud',attempt:'2'});
 await click('[data-task-delivery-run]');expect(onOpenRun).toHaveBeenLastCalledWith({run:'run-a'});expect(reads).toHaveLength(beforeNavigation);
 expect(host.querySelector('[data-task-delivery-test="verify"]')).not.toBeNull();
 await render('task-a',{openOnEntry:true,originRunID:'run-a',onOpenRun,navigationDisabled:true});
 expect(host.querySelector('[data-task-delivery-workbuddy-history]')).toBeNull();
 expect(host.querySelector(action)?.hasAttribute('href')).toBe(false);
 expect(host.querySelector(action)?.getAttribute('tabindex')).toBe('-1');
 await render('task-a',{openOnEntry:true,originRunID:'run-a',onOpenRun,taskVersion:4});
 expect(host.querySelector(card)).toBeNull();
 await render('task-a',{openOnEntry:true,originRunID:'cloud-run',onOpenRun});
 expect(host.querySelector('[data-task-delivery-origin]')?.getAttribute('data-relation')).toBe('changed');
 expect(host.querySelector(card)).toBeNull();
 await render('task-a',{openOnEntry:true,onOpenRun});
 kind='repository_source';await click('[data-task-delivery-refresh]');
 expect(host.querySelector(card)?.textContent).toContain('Repository source');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector(card)?.textContent).toContain('仓库来源');
 expect(host.querySelector(action)?.textContent).toBe('打开 WorkBuddy 原始尝试');
 denied=true;await click('[data-task-delivery-refresh]');expect(host.querySelector(card)).toBeNull();
 expect(writes).toBe(0);
});

it('checks the current registered repository only on demand and hides stale, denied or late source facts',async()=>{
 const value=delivery(),trace={run_id:'run-a',node_id:'develop',attempt:2};
 const nodes={...summary(),waiting:1,succeeded:0,items:[{id:trace.node_id,name:'Develop',type:'external_workflow',state:'waiting',attempt:trace.attempt}]};
 const tests={...summary(),total:0,succeeded:0,items:[]};
 const target={id:'target-a',project_id:'project-a',version:7,name:'Code delivery',ref:'refs/heads/delivery',status:'active',active_operation_id:'',blockers:[]};
 const observed={...trace,task_id:'task-a',task_version:3,target_id:target.id,target_version:target.version,ref:target.ref,head_commit:'b'.repeat(40),object_format:'sha1',observed_at:'2026-11-05T12:00:00Z'};
 let reply='success',release!:(response:Response)=>void;
 transport(async path=>{
  if(path.startsWith('/repository-integration-targets?'))return Response.json({items:[target,{...target,id:'target-b',status:'paused',name:'Paused target'}],total:2});
  if(!path.includes('/repository-source?'))return Response.json({...value,run:{...value.run,state:'running',nodes,tests,workbuddy_delivery:trace}});
  if(reply==='late')return new Promise<Response>(resolve=>{release=resolve;});
  if(reply==='denied')return Response.json({error:{code:'forbidden',message:'No access'}},{status:403});
  return Response.json(reply==='stale'?{...observed,attempt:1}:observed);
 });
 const result='[data-repository-source-result]',action='[data-repository-source-check]';
 await render();await click('[data-task-delivery-toggle]');expect(reads).toHaveLength(1);
 await click('[data-repository-source-toggle]');
 expect(reads.at(-1)?.path).toBe('/repository-integration-targets?project_id=project-a');
 expect(reads).toHaveLength(2);
 const select=host.querySelector<HTMLSelectElement>('[data-repository-source-target]')!;
 expect(select.value).toBe(target.id);expect(select.options.length).toBe(2);
 await click(action);
 expect(reads.at(-1)?.path).toBe('/tasks/task-a/delivery/repository-source?project_id=project-a&task_version=3&run_id=run-a&target_id=target-a&target_version=7');
 expect(host.querySelector(result)?.textContent).toContain(observed.head_commit);
 expect(host.querySelector(result)?.textContent).toContain('WorkBuddy authorship');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.querySelector(result)?.textContent).toContain('候选导入、测试和验收需分别提供证据');
 reply='stale';await click(action);expect(host.querySelector(result)).toBeNull();expect(host.textContent).toContain('与本次交付尝试或目标不符');
 reply='denied';await click(action);expect(host.querySelector(result)).toBeNull();expect(host.querySelector(action)).toBeNull();expect(host.textContent).toContain('当前权限无法核对来源');
 await click('[data-repository-source-reload]');
 reply='late';await click(action);const signal=reads.at(-1)?.signal;
 await render('task-a',{taskVersion:4});expect(signal?.aborted).toBe(true);
 await act(async()=>release(Response.json(observed)));await settle();
 expect(host.querySelector('[data-repository-source-toggle]')).toBeNull();expect(host.querySelector(result)).toBeNull();expect(writes).toBe(0);
 expect(vi.mocked(fetch).mock.calls.every(([,options])=>options?.method==='GET')).toBe(true);
});

const repositoryAction=(name:string)=>'[data-repository-source-'+name+']';
function repositoryWorkbenchFixture(){
 const trace={run_id:'run-a',node_id:'develop',attempt:2};
 const nodes={...summary(),waiting:1,succeeded:0,items:[{id:trace.node_id,name:'Develop',type:'external_workflow',state:'waiting',attempt:trace.attempt}]};
 const tests={...summary(),total:0,succeeded:0,items:[]};
 const target={id:'target-a',project_id:'project-a',version:7,name:'Code delivery',ref:'refs/heads/delivery',status:'active',active_operation_id:'',blockers:[]};
 const scope={...trace,project_id:'project-a',task_id:'task-a',task_version:3,target_id:target.id,target_version:target.version};
 const head='b'.repeat(40),base='a'.repeat(40),digest='c'.repeat(64),profileSHA='d'.repeat(64),testSHA='e'.repeat(64),at='2026-11-05T12:00:00Z';
 const intake={...scope,id:'intake-a',head_commit:head,base_commit:base,bundle_sha256:digest,size_bytes:1248,approved_at:at};
 const producer={id:'producer-a',version:4,name:'Pinned importer',worker_id:'worker-a',command_id:'git-import'};
 const tester={id:'tester-b',version:6,name:'Independent JUnit',worker_id:'worker-b',command_id:'junit'};
 const testBinding={node_id:'repository-test',tool_profile_id:tester.id,tool_profile_version:tester.version,tool_profile_sha256:testSHA,worker_id:tester.worker_id,command_id:tester.command_id};
 const runID='run-new',planID='plan-a',workflowID='managed-'+runID;
 const plan={id:planID,project_id:'project-a',version:1,state:'ready',task_id:'task-a',task_version:3,workflow_id:workflowID,workflow_version:1,agent_node_id:'repository-source',tool_profile_id:producer.id,
  tool_profile_version:producer.version,tool_profile_sha256:profileSHA,run_id:runID,created_at:at,expires_at:'2026-11-05T13:00:00Z',context:{task:{id:'task-a',version:3},test_plan:{case_count:1}},
  workflow:{id:workflowID,version:1,nodes:[{id:'repository-source',type:'command',command:[producer.command_id],config:{tool_profile_id:producer.id,tool_profile_sha256:profileSHA,
   repository_source_intake:{intake_id:intake.id,plan_id:planID,task_id:'task-a',task_version:3,head_commit:head,base_commit:base,bundle_sha256:digest,size_bytes:intake.size_bytes}}},
   {id:'repository-test',type:'test',command:[tester.command_id],config:{tool_profile_id:tester.id,tool_profile_sha256:testSHA,candidate_from:'repository-source'}}],edges:[{from:'repository-source',to:'repository-test'}]},
  test_bindings:[testBinding],valid:true,reason:''};
 return {target,intake,plan,runID,source:{...trace,task_id:'task-a',task_version:3,target_id:target.id,target_version:target.version,ref:target.ref,head_commit:head,object_format:'sha1',observed_at:at},
  delivery:{...delivery(),run:{...delivery().run,state:'running',nodes,tests,workbuddy_delivery:trace}},
  options:{...scope,head_commit:head,base_commit:base,baseline_ref:'refs/heads/main',live_state:'ready',published_case_count:1,active_terminal:false,profiles_truncated:false,retained_intakes:[],producer_profiles:[producer],test_profiles:[{...tester,id:'same-worker',worker_id:producer.worker_id},tester]}};
}

it('reviews exact repository bytes, enforces a different test Worker, and recovers a lost start on the new bound run',async()=>{
 const fixture=repositoryWorkbenchFixture(),openRun=vi.fn(),posts:{path:string;body:Record<string,unknown>}[]=[];
 let launched=false,failStart=true;
 transport(async(path,options)=>{
  if(options?.body)posts.push({path,body:JSON.parse(String(options.body))});
  if(path.startsWith('/repository-integration-targets?'))return Response.json({items:[fixture.target],total:1});
  if(path.includes('/repository-source/options?'))return Response.json(fixture.options);
  if(path.includes('/repository-source/intakes/')&&path.endsWith('/dispatch-plans'))return Response.json(fixture.plan,{status:201});
  if(path.endsWith('/repository-source/intakes'))return Response.json(fixture.intake,{status:201});
  if(path==='/task-dispatch-plans/plan-a/start'){
   launched=true;
   if(failStart){failStart=false;return Response.json({error:{code:'unavailable',message:'Lost response'}},{status:502});}
   return Response.json({id:fixture.runID,tenant_id:'org-a',project_id:'project-a',workflow_id:'managed-'+fixture.runID,workflow_version:1},{status:201});
  }
  if(path.startsWith('/task-dispatch-plans/plan-a?'))return Response.json(launched?{...fixture.plan,state:'started',version:2,valid:false}:fixture.plan);
  if(path.includes('/repository-source?'))return Response.json(fixture.source);
  return Response.json(launched?{...delivery(),task_version:4,run:{...delivery().run,run_id:fixture.runID}}:fixture.delivery);
 });
 await render('task-a',{onOpenRun:openRun});await click('[data-task-delivery-toggle]');await click(repositoryAction('toggle'));
 expect(reads.some(read=>read.path.includes('/options?'))).toBe(false);
 await click(repositoryAction('prepare'));
 expect(host.textContent).toContain(fixture.intake.base_commit);expect(host.textContent).toContain(fixture.intake.head_commit);
 expect(host.querySelector<HTMLButtonElement>(repositoryAction('approve'))?.disabled).toBe(true);
 await click(repositoryAction('approve-consent'));await click(repositoryAction('approve'));
 expect(posts[0].body).toMatchObject({expected_head:fixture.intake.head_commit,expected_base:fixture.intake.base_commit,run_id:'run-a',target_version:7});
 const tester=host.querySelector<HTMLSelectElement>(repositoryAction('tester'))!;expect(tester.value).toBe('tester-b');expect([...tester.options].map(option=>option.value)).toEqual(['','tester-b']);
 await click(repositoryAction('preview'));expect(posts[1].body).toMatchObject({producer_profile_id:'producer-a',expected_producer_profile_version:4,test_profile_id:'tester-b',expected_test_profile_version:6});
 expect(host.querySelector<HTMLButtonElement>(repositoryAction('start'))?.disabled).toBe(true);
 await click(repositoryAction('start-consent'));await click(repositoryAction('start'));
 expect(host.querySelector(repositoryAction('recover'))).not.toBeNull();
 await act(async()=>root!.unmount());root=undefined;
 await render('task-a',{taskVersion:4,onOpenRun:openRun});await click('[data-task-delivery-toggle]');
 expect(host.querySelector(repositoryAction('toggle'))).toBeNull();
 await click(repositoryAction('resume'));await click(repositoryAction('recover'));
 expect(posts[3]).toEqual(posts[2]);
 await click(repositoryAction('open-run'));expect(openRun).toHaveBeenCalledExactlyOnceWith({run:fixture.runID});
});

it('keeps retained sources usable when the provider is unavailable',async()=>{
 const fixture=repositoryWorkbenchFixture();
 transport(async path=>{
  if(path.startsWith('/repository-integration-targets?'))return Response.json({items:[fixture.target],total:1});
  if(path.includes('/repository-source/options?'))return Response.json({...fixture.options,head_commit:'',base_commit:'',baseline_ref:'',live_state:'unavailable',retained_intakes:[fixture.intake]});
  if(path.includes('/repository-source?'))return Response.json({error:{code:'unavailable',message:'Provider unavailable'}},{status:503});
  return Response.json(fixture.delivery);
 });
 await render();await click('[data-task-delivery-toggle]');await click(repositoryAction('toggle'));await click(repositoryAction('check'));
 expect(host.querySelector(repositoryAction('result'))).toBeNull();
 await click(repositoryAction('prepare'));
 expect(host.querySelector(repositoryAction('approve'))).toBeNull();
 expect(host.querySelector<HTMLSelectElement>(repositoryAction('intake'))?.value).toBe(fixture.intake.id);
 expect(host.querySelector<HTMLButtonElement>(repositoryAction('preview'))?.disabled).toBe(false);
 expect(writes).toBe(0);
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
