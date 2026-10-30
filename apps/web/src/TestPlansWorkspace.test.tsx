// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale,tr} from './i18n';
import {TestPlansWorkspace} from './TestPlansWorkspace';

const sha='a'.repeat(64),project='project-a',task={id:'task-a',project_id:project,version:3,title:'Current task',description:'Task body',acceptance_criteria:['Access is authorized'],requirement_id:''};
const source={task:{id:task.id,version:task.version,title:task.title,description:task.description,acceptance_criteria:task.acceptance_criteria},task_basis_sha256:sha,sha256:sha};
const revision={revision:1,state:'draft',title:'Existing plan',description:'Draft',source,cases:[],sha256:sha,created_by:'user-a',created_at:'2026-10-28T00:00:00Z'};
const plan={id:'plan-a',project_id:project,task_id:task.id,version:1,state:'draft',latest_revision:1,published_revision:0,has_draft:true,basis_current:true,eligibility:'none',latest:revision,allowed_actions:['save','publish'],created_at:revision.created_at,updated_at:revision.created_at};
const execution={plan_id:'plan-a',task_id:'task-a',revision:1,plan_sha256:sha,run_id:'run-a',run_state:'succeeded',created_at:revision.created_at,state:'incomplete',basis_current:true,source_commits:[],total:1,passed:0,failed:0,errors:0,skipped:0,not_run:1,cases:[{case_key:'authorize',title:'Authorization case',status:'not_run',covers:[1],evidence:[],issue_id:''}],reports:[]};
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test',role:'owner'},organization:{id:'org-a',name:'Org'},projects:[{id:project,version:1,title:'Project',role:'admin'}]};
let root:Root|undefined,host:HTMLDivElement,writes:{path:string;body:Record<string,unknown>}[],reads:string[],reply:(path:string,init?:RequestInit)=>Promise<Response>,changed:ReturnType<typeof vi.fn>,select:ReturnType<typeof vi.fn>;
function defaults(path:string){if(path.startsWith('/tasks?'))return Response.json({items:[task],total:1,next_cursor:''});if(path.startsWith('/tasks/task-a?'))return Response.json(task);if(path.includes('/executions/run-a?'))return Response.json(execution);if(path.includes('/executions?'))return Response.json({items:[execution],total:1,next_cursor:''});if(path.startsWith('/test-plans?'))return Response.json({items:[plan],total:1,next_cursor:''});return Response.json(plan);}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();writes=[];reads=[];changed=vi.fn();select=vi.fn();host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);reply=async path=>defaults(path);vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(init?.body)writes.push({path,body:JSON.parse(String(init.body))});else reads.push(path);return reply(path,init);}));});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(planID='',tab:'cases'|'results'='cases',runID='',actor=session){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={actor}><TestPlansWorkspace session={actor} project={project} refresh={0} planID={planID} runID={runID} tab={tab} onSelectPlan={select} onSelectExecution={()=>{}} onTab={()=>{}} onOpenTask={()=>{}} onOpenRun={()=>{}} onOpenIssue={()=>{}} onChanged={changed}/></ApiViewProvider>);});await settle();}
async function change(selector:string,value:string){const element=(host.querySelector<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement>(selector)||document.querySelector<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement>('[role=dialog] '+selector))!;expect(element,selector).not.toBeNull();const prototype=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:element instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function click(selector:string){const element=(host.querySelector<HTMLElement>(selector)||document.querySelector<HTMLElement>('[role=dialog] '+selector))!;expect(element,selector).not.toBeNull();await act(async()=>element.click());await settle();}

it('creates only a draft using the actually loaded task and requirement versions',async()=>{
 reply=async(path,init)=>init?.body?Response.json(plan):defaults(path);await render();await change('[name=test_plan_task]','task-a');await change('[name=test_plan_title]','Reviewed automatic checks');await click('[data-test-plan-save]');expect(writes[0].body).toMatchObject({project_id:project,task_id:'task-a',expected_task_version:3,expected_requirement_version:0,title:'Reviewed automatic checks',cases:[]});expect(writes[0].body).not.toHaveProperty('passed');expect(select).toHaveBeenCalledWith('plan-a');
});

it('uses server save actions for developer and reviewer sessions without projected project roles',async()=>{
 for(const role of ['developer','reviewer']){
  const allowed_actions=role==='reviewer'?['save','publish']:['save'];
  reply=async(path,init)=>init?.body?Response.json({...plan,allowed_actions}):path.startsWith('/test-plans/plan-a?')?Response.json({...plan,allowed_actions}):defaults(path);
  const actor:Session={...session,user:{...session.user,id:'user-'+role,role:'member'},projects:[{id:project,version:1,title:'Project'}]};
  await render('plan-a','cases','',actor);
  expect(host.querySelector<HTMLInputElement>('[name=test_plan_title]')?.readOnly).toBe(false);
  await change('[name=test_plan_title]',role+' reviewed draft');await click('[data-test-plan-save]');expect(writes.at(-1)?.body).toMatchObject({title:role+' reviewed draft',expected_version:1});
  expect(select).not.toHaveBeenCalled();
  expect(!!host.querySelector('[data-test-plan-publish]')).toBe(role==='reviewer');expect(writes.every(write=>write.path==='/test-plans/plan-a')).toBe(true);
 }
});

it('selects a newly created plan only after refreshing the list and releasing its own dirty guard',async()=>{
 let created=false,finish!:(response:Response)=>void;let navigationAllowed=false;
 select.mockImplementation(()=>{navigationAllowed=window.dispatchEvent(new Event('suma:navigate',{cancelable:true}));});
 reply=async(path,init)=>{if(init?.body){created=true;return Response.json(plan);}if(path.startsWith('/test-plans?'))return created?new Promise<Response>(resolve=>{finish=resolve;}):Response.json({items:[],total:0,next_cursor:''});return defaults(path);};
 await render();await change('[name=test_plan_task]','task-a');await change('[name=test_plan_title]','Created after current source read');await click('[data-test-plan-save]');
 expect(typeof finish).toBe('function');expect(select).not.toHaveBeenCalled();expect(writes).toHaveLength(1);
 await act(async()=>{finish(Response.json({items:[plan],total:1,next_cursor:''}));});await settle();
 expect(select).toHaveBeenCalledExactlyOnceWith('plan-a');expect(navigationAllowed).toBe(true);expect(writes).toHaveLength(1);
});

it('does not navigate a new actor when an earlier create finishes refreshing its list',async()=>{
 let created=false,finish!:(response:Response)=>void;
 reply=async(path,init)=>{if(init?.body){created=true;return Response.json(plan);}if(path.startsWith('/test-plans?'))return created?new Promise<Response>(resolve=>{finish=resolve;}):Response.json({items:[],total:0,next_cursor:''});return defaults(path);};
 await render();await change('[name=test_plan_task]','task-a');await change('[name=test_plan_title]','Only the original actor owns this acknowledgement');await click('[data-test-plan-save]');const original=finish;expect(typeof original).toBe('function');
 reply=async path=>defaults(path);const other:Session={...session,user:{...session.user,id:'other-user'},organization:{...session.organization,id:'other-org'}};await render('plan-a','cases','',other);
 await act(async()=>{original(Response.json({items:[plan],total:1,next_cursor:''}));});await settle();expect(select).not.toHaveBeenCalled();expect(changed).not.toHaveBeenCalled();
});

it('keeps a selected plan read-only when the server withholds save even if the session has project write roles',async()=>{
 reply=async path=>path.startsWith('/test-plans/plan-a?')?Response.json({...plan,allowed_actions:[]}):defaults(path);
 for(const role of ['viewer','developer']){
  const actor:Session={...session,user:{...session.user,id:'user-'+role,role:'member'},projects:[{...session.projects[0],role}]};await render('plan-a','cases','',actor);
  expect(host.querySelector<HTMLInputElement>('[name=test_plan_title]')?.readOnly).toBe(true);expect(host.querySelector('[data-test-plan-save]')).toBeNull();expect(host.querySelector('[data-test-plan-publish]')).toBeNull();expect(writes).toHaveLength(0);
 }
});

it('waits for the current task source before checking coverage or enabling a save',async()=>{
 const testCase={key:'current-access',title:'Current task access',description:'',preconditions:'',steps:[],expected_result:'',priority:'medium',covers:[1],selector:{class_name:'Access',name:'current_task'}};
 const withCase={...plan,allowed_actions:['save'],latest:{...revision,cases:[testCase]}};
 let release!:(response:Response)=>void;
 reply=async(path,init)=>init?.body?Response.json(withCase):path.startsWith('/tasks/task-a?')?new Promise<Response>(resolve=>{release=resolve;}):path.startsWith('/test-plans/plan-a?')?Response.json(withCase):defaults(path);
 const actor:Session={...session,user:{...session.user,role:'member'},projects:[{id:project,version:1,title:'Project'}]};await render('plan-a','cases','',actor);
 expect(typeof release).toBe('function');expect(host.textContent).not.toContain(tr('test-plans.validation.covers_invalid'));expect(host.querySelector<HTMLButtonElement>('[data-test-plan-save]')?.disabled).toBe(true);
 await change('[name=test_plan_title]','Source-confirmed draft');await click('[data-test-plan-save]');expect(writes).toHaveLength(0);
 // Generic Record normalization converts this array to a multiline string;
 // the immutable plan source remains an array, and both must resolve to one criterion.
 await act(async()=>{release(Response.json({...task,data:{}}));});await settle();
 expect(host.textContent).not.toContain(tr('test-plans.validation.covers_invalid'));expect(host.querySelector<HTMLButtonElement>('[data-test-plan-save]')?.disabled).toBe(false);
 await click('[data-test-plan-save]');expect(writes.at(-1)?.body).toMatchObject({expected_task_version:3,title:'Source-confirmed draft',cases:[{...testCase,covers:[1]}]});
});

it('does not revive an earlier caller’s source refusal or save action after switching to a read-only plan',async()=>{
 let oldSource=true,finish!:(response:Response)=>void;
 reply=async path=>path.startsWith('/tasks/task-a?')&&oldSource?new Promise<Response>(resolve=>{finish=resolve;}):path.startsWith('/test-plans/plan-a?')?Response.json({...plan,allowed_actions:oldSource?['save']:[]}):defaults(path);
 const writer:Session={...session,user:{...session.user,id:'writer',role:'member'},projects:[{id:project,version:1}]};await render('plan-a','cases','',writer);const original=finish;expect(typeof original).toBe('function');
 oldSource=false;const viewer:Session={...writer,user:{...writer.user,id:'viewer'}};await render('plan-a','cases','',viewer);await act(async()=>{original(Response.json({error:{code:'forbidden',message:'Earlier caller denied'}},{status:403}));});await settle();
 expect(host.querySelector('[data-test-plan-editor]')).not.toBeNull();expect(host.querySelector<HTMLInputElement>('[name=test_plan_title]')?.readOnly).toBe(true);expect(host.querySelector('[data-test-plan-save]')).toBeNull();expect(host.querySelector('[data-test-plan-retry]')).toBeNull();expect(host.textContent).not.toContain('Earlier caller denied');expect(writes).toHaveLength(0);
});
it('shows missing case evidence without a pass editor or implicit test execution',async()=>{
 await render('plan-a','results','run-a');expect(host.querySelector('[data-test-case-result=authorize]')?.textContent).toContain('Not reported');expect(host.querySelector('[name=result]')).toBeNull();expect(writes).toHaveLength(0);expect(reads.some(path=>path.includes('/reports/'))).toBe(false);
});
it('clears protected plan facts on denial and waits for an explicit retry',async()=>{
 reply=async path=>path.startsWith('/test-plans/plan-a?')?Response.json({error:{code:'forbidden',message:'Access changed'}},{status:403}):defaults(path);await render('plan-a');const count=reads.length;await settle();expect(reads).toHaveLength(count);expect(changed).not.toHaveBeenCalled();expect(host.querySelector('[data-test-plan-editor]')).toBeNull();expect(host.querySelector('[data-test-plan-retry]')).not.toBeNull();
});
it('recovers the original save after a lost reply without adopting a later plan version',async()=>{
 let lost=true;reply=async(path,init)=>init?.body?(lost?Promise.reject(TypeError('lost')):Response.json({...plan,version:2,latest_revision:2,latest:{...revision,revision:2,title:'Original submitted draft'}})):path.startsWith('/tasks/task-a?')?Response.json({...task,version:4}):defaults(path);await render('plan-a');await change('[name=test_plan_title]','Original submitted draft');await click('[data-test-plan-save]');const original=writes.at(-1)!.body;await act(async()=>root!.unmount());root=undefined;reply=async(path,init)=>init?.body?(lost?Promise.reject(TypeError('lost')):Response.json({...plan,version:2,latest_revision:2,latest:{...revision,revision:2,title:'Original submitted draft'}})):path.startsWith('/test-plans/plan-a?')?Response.json({...plan,version:3,latest_revision:3,latest:{...revision,revision:3,title:'Later plan'}}):defaults(path);await render('plan-a');lost=false;await click('[data-test-plan-restore]');expect(writes.at(-1)?.body).toEqual(original);expect(writes.at(-1)?.body.expected_version).toBe(1);expect(original.expected_task_version).toBe(4);
});

it('registers an issue from the exact failed case and refreshes its server-owned issue link',async()=>{
 let issued=false;const commit='c'.repeat(40),report={id:'report-a',run_id:'run-a',node_id:'test',attempt:1,fence:1,worker_id:'worker-a',source_commit:commit,sha256:sha,protocol:'suma-junit-report-v1',eligibility:'current',process_exit_code:1,outcome_unknown:false,reported_at:revision.created_at,total:1,passed:0,failed:1,errors:0,skipped:0,unmapped:0};
 const failed=()=>({...execution,state:'failed',run_state:'failed',source_commits:[commit],failed:1,not_run:0,cases:[{...execution.cases[0],status:'failed',issue_id:issued?'issue-a':'',evidence:[{report_id:'report-a',node_id:'test',attempt:1,fence:1,worker_id:'worker-a',source_commit:commit,status:'failed',message:'Expected authorization denial',duration_ms:8}]}],reports:[report]});
 reply=async(path,init)=>{if(init?.body){issued=true;return Response.json({id:'issue-a',project_id:project,version:1,title:'Observed failing case'});}if(path.includes('/executions/run-a?'))return Response.json(failed());if(path.includes('/executions?'))return Response.json({items:[failed()],total:1,next_cursor:''});return defaults(path);};await render('plan-a','results','run-a');await click('[data-test-case-result=authorize]>button');await change('[name=test_plan_defect_reason]','The current project authorization check failed.');await click('[data-test-plan-defect]');expect(writes.at(-1)?.path).toBe('/test-plans/plan-a/executions/run-a/defects');expect(writes.at(-1)?.body).toMatchObject({project_id:project,expected_version:1,case_key:'authorize',reason:'The current project authorization check failed.'});expect(host.querySelector('[data-test-case-result=authorize]')?.textContent).toContain('Open issue and feedback');
});
