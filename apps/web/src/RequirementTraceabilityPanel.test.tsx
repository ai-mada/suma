// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {RequirementTraceability,TaskDeliveryNodeSummary} from '../../../packages/contracts/src';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {RequirementTraceabilityPanel,type RequirementTraceabilityPanelProps} from './RequirementTraceabilityPanel';

const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
function summary():TaskDeliveryNodeSummary{return {total:1,waiting:1,running:0,succeeded:0,failed:0,skipped:0,reconciling:0,other:0,limit:20,complete:true,items:[{id:'verify',name:'Independent test',type:'test',state:'pending',attempt:0}]};}
function trace(id='requirement-a'):RequirementTraceability{return {project_id:'project-a',requirement_id:id,requirement_version:2,requirement_status:'draft',design_version:0,published_revision:0,design_sha256:'',eligibility:'none',criteria:[{index:1,text:'原始验收正文',task_keys:[]}],tasks:[]};}
function published():RequirementTraceability{return {...trace(),design_version:3,published_revision:1,design_sha256:'a'.repeat(64),eligibility:'current',criteria:[{index:1,text:'原始验收正文',task_keys:['task-a','task-b']}],tasks:[{key:'task-a',title:'Actual business task',covers:[1],task_id:'formal-a',task_version:4,task_status:'in_progress',binding_state:'current',run_basis_state:'current',formal_acceptance:false,run:{run_id:'run-a',version:5,state:'running',nodes:summary(),tests:summary()}},{key:'task-b',title:'Unadopted proposal',covers:[1],task_id:'',task_version:0,task_status:'',binding_state:'not_adopted',run_basis_state:'not_dispatched',formal_acceptance:false,run:null}]};}
let root:Root|undefined,host:HTMLDivElement,reads:{path:string;signal?:AbortSignal|null}[],writes:number;
beforeEach(async()=>{await setLocale('en-US',{persist:false});reads=[];writes=0;host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
function transport(reply:(path:string)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(options?.body)writes++;else reads.push({path,signal:options?.signal});return reply(path);}));}
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(patch:Partial<RequirementTraceabilityPanelProps>={}){const props={session,project:'project-a',requirementId:'requirement-a',requirementVersion:2,refresh:0,...patch};await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={props.session}><RequirementTraceabilityPanel {...props}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());await settle();}
async function expandCriterion(){const details=host.querySelector<HTMLDetailsElement>('[data-trace-criterion="1"]');expect(details).not.toBeNull();await act(async()=>{details!.open=true;details!.dispatchEvent(new Event('toggle'));});await settle();}

it('reads only when opened, retains unmapped criteria and switches labels without translating business content or rereading',async()=>{
 transport(async()=>Response.json(trace()));await render();expect(reads).toHaveLength(0);await click('[data-trace-toggle]');expect(reads.map(read=>read.path)).toEqual(['/requirements/requirement-a/traceability?project_id=project-a']);expect(host.textContent).toContain('原始验收正文');expect(host.textContent).toContain('No approved task mapping');expect(host.querySelector('[data-trace-task]')).toBeNull();
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('验收追踪');expect(host.textContent).toContain('原始验收正文');expect(reads).toHaveLength(1);expect(writes).toBe(0);
});

it('keeps proposals, execution tests and formal acceptance distinct and navigates to a node without inventing attempt zero',async()=>{
 transport(async()=>Response.json(published()));const onOpenRun=vi.fn(),onOpenTask=vi.fn();await render({onOpenRun,onOpenTask});await click('[data-trace-toggle]');expect(host.textContent).toContain('0 of 2 mapped tasks currently accepted');await expandCriterion();expect(host.textContent).toContain('Proposal not adopted');expect(host.textContent).toContain('Current task not formally accepted');
 await click('[data-trace-task="formal-a"]');expect(onOpenTask).toHaveBeenCalledWith('formal-a');await click('[data-trace-run="run-a"]');expect(onOpenRun).toHaveBeenCalledWith({run:'run-a'});await click('[data-trace-test="verify"]');expect(onOpenRun).toHaveBeenLastCalledWith({run:'run-a',node:'verify'});expect(host.textContent).toContain('Not started');expect(host.textContent).not.toContain('Attempt 0');expect(writes).toBe(0);
});

it('aborts an old owner or requirement read and never exposes its late references',async()=>{
 let release!:(value:Response)=>void;transport(async path=>path.includes('/requirement-a/')?new Promise<Response>(resolve=>{release=resolve;}):Response.json(trace('requirement-b')));await render();await click('[data-trace-toggle]');const signal=reads[0].signal;await render({session:{...session,user:{...session.user,id:'user-b'}},requirementId:'requirement-b'});expect(signal?.aborted).toBe(true);await act(async()=>release(Response.json(published())));await settle();expect(host.querySelector('[data-trace-task]')).toBeNull();expect(host.querySelector('[data-trace-criterion]')).toBeNull();await click('[data-trace-toggle]');expect(reads[1].path).toContain('/requirement-b/traceability');expect(host.textContent).toContain('No approved task mapping');
});

it('clears previous facts on read errors and permission loss, then explicitly retries with a fresh scope',async()=>{
 let mode=200;transport(async()=>mode===200?Response.json(published()):Response.json({error:{code:mode===403?'forbidden':'unavailable',message:'Read unavailable'}},{status:mode}));await render({onOpenTask:vi.fn()});await click('[data-trace-toggle]');await expandCriterion();expect(host.querySelector('[data-trace-task]')).not.toBeNull();mode=503;await click('[data-trace-refresh]');expect(host.querySelector('[data-trace-task]')).toBeNull();expect(host.querySelector('[data-trace-criterion]')).toBeNull();mode=403;await click('[data-trace-retry]');expect(host.textContent).toContain('Protected facts are hidden');await settle();expect(reads).toHaveLength(3);mode=200;await click('[data-trace-retry]');expect(host.querySelector('[data-trace-criterion]')).not.toBeNull();expect(reads).toHaveLength(4);expect(writes).toBe(0);
});

it('refreshes a mismatched requirement only on explicit request and locks evidence until the detail adopts that version',async()=>{
 const value=published();value.requirement_version=3;transport(async()=>Response.json(value));const onRefreshRequirement=vi.fn(),onOpenRun=vi.fn();await render({onRefreshRequirement,onOpenRun,onOpenTask:vi.fn()});await click('[data-trace-toggle]');await expandCriterion();expect(onRefreshRequirement).not.toHaveBeenCalled();expect(host.textContent).toContain('Refresh the detail');expect(host.querySelector<HTMLButtonElement>('[data-trace-task]')!.disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-trace-test]')!.disabled).toBe(true);await click('[data-trace-refresh]');expect(onRefreshRequirement).not.toHaveBeenCalled();expect(reads).toHaveLength(2);await click('[data-trace-refresh-requirement]');expect(onRefreshRequirement).toHaveBeenCalledTimes(1);await render({requirementVersion:3,onRefreshRequirement,onOpenRun,onOpenTask:vi.fn()});await expandCriterion();expect(host.querySelector<HTMLButtonElement>('[data-trace-test]')!.disabled).toBe(false);expect(host.querySelector('[data-trace-refresh-requirement]')).toBeNull();expect(reads).toHaveLength(3);
});

it('rejects a foreign or incomplete mapping before showing evidence navigation',async()=>{
 const value=published();value.tasks[0].covers=[];transport(async()=>Response.json(value));await render({onOpenTask:vi.fn(),onOpenRun:vi.fn()});await click('[data-trace-toggle]');expect(host.textContent).toContain('could not be verified');expect(host.querySelector('[data-trace-criterion]')).toBeNull();expect(host.querySelector('[data-trace-task]')).toBeNull();
});
