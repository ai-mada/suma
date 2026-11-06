// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import type {IssueRepair,IssueRepairView,Record as DomainRecord,TestPlanView} from '../../../packages/contracts/src';
import {setLocale} from './i18n';
import {IssueRepairPanel} from './IssueRepairPanel';
const project='project-a',issue='issue-a',stamp='2026-10-29T01:00:00Z',digest='a'.repeat(64);
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
function record(id:string,title:string,status:string,version=3):DomainRecord{return {id,title,status,version,tenant_id:'tenant-a',project_id:project,description:'Original <script>source</script> {{literal}}',priority:'high',owner_id:'',requirement_id:'',run_id:'',depends_on:[],acceptance_criteria:[],created_at:stamp,updated_at:stamp,data:{}};}
function cycle():IssueRepair{return {id:'repair-a',tenant_id:'tenant-a',project_id:project,issue_id:issue,cycle:1,version:1,requirement_id:'requirement-a',task_id:'task-a',prepared_by:'user-a',created_at:stamp,updated_at:stamp};}
function view(prepared=false):IssueRepairView{return {issue:record(issue,'Original incident',prepared?'in_progress':'open'),cycle:1,...(prepared?{current:cycle(),task:{...record('task-a','Repair task','todo',4),requirement_id:'requirement-a'}}:{}),history:[],history_total:0,history_complete:true,can_prepare:!prepared,can_bind:prepared,can_close:false,can_reopen:false,blockers:[{code:prepared?'regression_not_bound':'repair_not_prepared'}]};}
function bound():IssueRepairView{const value=view(true);value.issue.version=4;value.current={...cycle(),version:2,regression:{plan_id:'plan-a',plan_revision:2,plan_sha256:digest,case_keys:['case-A'],bound_by:'user-a',bound_at:stamp}};value.task={...value.task!,status:'accepted',run_id:'run-a'};value.can_close=true;value.blockers=[];return value;}
function closed():IssueRepairView{const value=bound();value.issue={...value.issue,status:'closed',version:5};value.current={...value.current!,version:3,closure:{run_id:'run-a',task_version:4,review_id:'review-a',plan_id:'plan-a',plan_revision:2,plan_sha256:digest,case_keys:['case-A'],source_commits:['b'.repeat(40)],closed_by:'user-a',closed_at:stamp,reason:'Verified the exact fix'}};value.can_close=false;value.can_bind=false;value.can_reopen=true;value.blockers=[{code:'issue_closed',ref_id:issue}];return value;}
function plan(version=9):TestPlanView{const revision={revision:version===9?2:3,state:'published',title:'Regression plan',description:'Exact selectors',source:{task:{id:'task-a',version:4,title:'Repair task',description:'Repair',acceptance_criteria:['No regression']},task_basis_sha256:digest,sha256:digest},cases:[{key:'case-A',title:'Original regression',description:'',preconditions:'Fixed commit',steps:['Run assertion'],expected_result:'No failure',priority:'high',covers:[1],selector:{class_name:'suite',name:'original failure'}}],sha256:version===9?digest:'c'.repeat(64),created_by:'user-a',created_at:stamp};return {id:'plan-a',project_id:project,task_id:'task-a',version,state:'published',latest_revision:revision.revision,published_revision:revision.revision,has_draft:false,basis_current:true,eligibility:'current',allowed_actions:['save','publish','retire'],created_at:stamp,updated_at:stamp,latest:revision,published:revision} as TestPlanView;}
let root:Root|undefined,host:HTMLDivElement,reads:{path:string;signal?:AbortSignal|null}[],writes:{path:string;body:any;headers:Headers}[];
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();host=document.createElement('div');document.body.append(host);reads=[];writes=[];vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(s=session,p=project,i=issue,onChanged:()=>void|Promise<void>=()=>{},panel=''){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={s}><IssueRepairPanel session={s} project={p} issueId={i} issueVersion={3} panel={panel} refresh={0} onChanged={onChanged}/></ApiViewProvider>);});await settle();}
function transport(reply:(path:string,options?:RequestInit)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(options?.body)writes.push({path,body:JSON.parse(String(options.body)),headers:new Headers(options.headers)});else reads.push({path,signal:options?.signal});return reply(path,options);}));}
async function click(selector:string){const element=host.querySelector<HTMLElement>(selector);expect(element,selector).not.toBeNull();await act(async()=>element!.click());await settle();}
async function change(selector:string,value:string){const element=host.querySelector<HTMLInputElement|HTMLTextAreaElement|HTMLSelectElement>(selector);expect(element,selector).not.toBeNull();const proto=element instanceof HTMLSelectElement?HTMLSelectElement.prototype:element instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(element,value);element!.dispatchEvent(new Event(element instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});}
async function submit(){await act(async()=>host.querySelector('[data-ir-form]')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true})));await settle();}
const submitButton=()=>host.querySelector<HTMLButtonElement>('[data-ir-form] button.primary')!;

it('shows release review only from a protected closed cycle with the matching sealed regression',async()=>{
 const current=closed();
 current.current!.closure!.package_regression={node_id:'package.test',attempt:2,fence:4,artifact_id:'artifact-a',manifest_sha256:digest,source_commit:'b'.repeat(40),proof_id:'proof-package'};
 current.release_handoff={issue_id:issue,issue_version:5,repair_id:'repair-a',repair_version:3,cycle:1,task_id:'task-a',task_version:4,run_id:'run-a',review_id:'review-a',artifact_id:'artifact-a',build_attestation_id:'proof-build',package_test_attestation_id:'proof-package'};
 transport(async()=>Response.json(current));
 await render();
 await click('[data-ir-toggle]');
 expect(host.querySelector('[data-ir-release]')?.getAttribute('href')).toBe('?project=project-a&view=operations&sub=release-candidates&panel=repair-release%3Aissue-a%3A1');
 expect(writes).toHaveLength(0);
 current.release_handoff.repair_version=4;
 await click('[data-ir-refresh]');
 expect(host.querySelector('[data-ir-release]')).toBeNull();
});

it('T119 prepares the canonical requirement and an ordinary repair task only after explicit criteria, without dispatching',async()=>{
 let current=view();const changed=vi.fn();transport(async(_path,init)=>{if(init?.body){current=view(true);current.issue.version=4;return Response.json(current);}return Response.json(current);});await render(session,project,issue,changed);expect(reads).toHaveLength(0);await click('[data-ir-toggle]');expect(reads).toHaveLength(1);await click('[data-ir-prepare]');expect(submitButton().disabled).toBe(true);await change('[name=ir_criteria]','Reproduce the failure\nVerify independent regression');await change('[name=ir_reason]','Repair the reported behavior');await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.querySelector<HTMLTextAreaElement>('[name=ir_reason]')?.value).toBe('Repair the reported behavior');expect(host.textContent).toContain('准备需求与修复任务');await submit();expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:'/issues/issue-a/repair/prepare',body:{project_id:project,expected_issue_version:3,owner_id:'',acceptance_criteria:['Reproduce the failure','Verify independent regression'],reason:'Repair the reported behavior'}});expect(writes.every(item=>!item.path.includes('dispatch')&&!item.path.includes('/runs'))).toBe(true);expect(host.querySelector('script')).toBeNull();expect(host.querySelector('a[href*="record=task-a"]')).not.toBeNull();expect(changed).toHaveBeenCalledTimes(1);
});
it('T119 freezes the published plan and selected cases, preserving drafts and requiring reselection after a changed-basis conflict',async()=>{
 let current=view(true),version=9,conflict=true;transport(async(path,init)=>{if(path.startsWith('/test-plans?')){expect(path).toContain('task_id=task-a');return Response.json({items:[plan(version)],total:1,next_cursor:''});}if(init?.body){if(conflict){conflict=false;version=10;current={...current,issue:{...current.issue,version:4}};return Response.json({error:{code:'conflict',message:'Plan changed'}},{status:409});}current={...current,issue:{...current.issue,version:5},current:{...current.current!,version:2,regression:{plan_id:'plan-a',plan_revision:3,plan_sha256:'c'.repeat(64),case_keys:['case-A'],bound_by:'user-a',bound_at:stamp}}};return Response.json(current);}return Response.json(current);});await render();await click('[data-ir-toggle]');await click('[data-ir-bind]');await change('[name=ir_plan]','plan-a');await click('[name=ir_case]');await change('[name=ir_reason]','Keep the original failing selector');await submit();expect(writes[0].body).toMatchObject({expected_plan_version:9,expected_repair_version:1,plan_id:'plan-a',case_keys:['case-A']});expect(host.querySelector('.conflict-banner')).not.toBeNull();expect(host.querySelector<HTMLTextAreaElement>('[name=ir_reason]')?.value).toBe('Keep the original failing selector');await click('.conflict-banner button');expect(host.querySelector<HTMLInputElement>('[name=ir_case]')?.checked).toBe(false);expect(submitButton().disabled).toBe(true);await click('[name=ir_case]');await submit();expect(writes[1].body).toMatchObject({expected_issue_version:4,expected_plan_version:10,case_keys:['case-A']});expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);expect(writes.every(item=>!('passed' in item.body)&&!('status' in item.body))).toBe(true);
});
it('T119 closes only server-ready evidence with reason and acknowledgement, then reopens without erasing the previous closure',async()=>{
 let current=bound();transport(async(_path,init)=>{if(init?.body){const body=JSON.parse(String(init.body));if(writes.at(-1)?.path.endsWith('/close'))current=closed();else {const previous={...closed().current!,version:4,reopened_at:stamp,reopened_by:'user-a',reopen_reason:body.reason};current={...view(),cycle:2,issue:{...view().issue,version:6},history:[previous],history_total:1};}return Response.json(current);}return Response.json(current);});await render();await click('[data-ir-toggle]');await click('[data-ir-close]');expect(submitButton().disabled).toBe(true);await change('[name=ir_reason]','Verified the exact fix');expect(submitButton().disabled).toBe(true);await click('[name=ir_ack]');await submit();expect(writes[0].body).toMatchObject({expected_issue_version:4,expected_repair_version:2,run_id:'run-a',reason:'Verified the exact fix'});await click('[data-ir-reopen]');expect(submitButton().disabled).toBe(true);await change('[name=ir_reason]','Reproduced after closure');await submit();expect(writes[1].path).toBe('/issues/issue-a/repair/reopen');expect(writes[1].body).not.toHaveProperty('run_id');expect(host.querySelectorAll('[data-ir-history]')).toHaveLength(1);expect(host.querySelector('[data-ir-history] [data-ir-closure-run]')?.getAttribute('href')).toBe('?project=project-a&view=development&sub=runs&run=run-a');expect(host.querySelector('[data-ir-history] [data-ir-closure-regression]')?.getAttribute('href')).toBe('?project=project-a&view=quality&sub=tests&record=plan-a&run=run-a&panel=test-results');expect(host.textContent).toContain('Verified the exact fix');expect(host.textContent).toContain('Reproduced after closure');expect(host.querySelector('[data-ir-prepare]')).not.toBeNull();
});
it('T119 retains the exact original operation when a successful-looking binding receipt does not match selected cases',async()=>{
 let current=view(true),malformed=true;transport(async(path,init)=>{if(path.startsWith('/test-plans?'))return Response.json({items:[plan()],total:1,next_cursor:''});if(init?.body){const value={...view(true),issue:{...view(true).issue,version:4},current:{...cycle(),version:2,regression:{plan_id:malformed?'forged-plan':'plan-a',plan_revision:2,plan_sha256:digest,case_keys:['case-A'],bound_by:'user-a',bound_at:stamp}}};if(!malformed)current=value;return Response.json(value);}return Response.json(current);});await render();await click('[data-ir-toggle]');await click('[data-ir-bind]');await change('[name=ir_plan]','plan-a');await click('[name=ir_case]');await change('[name=ir_reason]','Original binding operation');await submit();expect(host.querySelector('[data-ir-restore]')).not.toBeNull();expect(host.querySelector('[name=ir_reason]')?.matches(':disabled')).toBe(true);await act(async()=>setLocale('zh-CN',{persist:false}));malformed=false;await click('[data-ir-restore]');expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(writes[0].body);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(host.querySelector('[data-ir-restore]')).toBeNull();
});
it('T119 aborts late reads on scope changes and hides protected repair facts when access is revoked',async()=>{
 let release!:(value:Response)=>void;transport(async()=>new Promise(resolve=>{release=resolve;}));await render();await click('[data-ir-toggle]');const signal=reads[0].signal,old=release;const other={...session,user:{...session.user,id:'other-user'},organization:{id:'other-org',name:'Other'}};await render(other,'other-project','other-issue');expect(signal?.aborted).toBe(true);await act(async()=>old(Response.json(bound())));await settle();expect(host.textContent).not.toContain('Repair task');expect(host.querySelector('[data-ir-close]')).toBeNull();transport(async()=>Response.json({error:{code:'forbidden',message:'Current project access denied'}},{status:403}));await click('[data-ir-toggle]');expect(host.textContent).toContain('Current project access denied');expect(host.querySelector('[data-ir-prepare]')).toBeNull();expect(writes).toHaveLength(0);
});

it('keeps preparation-time automation evidence scoped to its repair cycle and ignores malformed locators',async()=>{
 const current=view(true);
 const snapshot={run_id:'original-failed-run',origin:{
  rule_id:'rule-a',rule_name:'Release checks',occurrence_id:'failed-trigger',revision:4,source:'cron',scheduled_at:stamp,
  failure_pause:{occurrence_id:'paused-trigger',observed_at:stamp,threshold:2},
 }};
 current.current={...current.current!,automation_source:snapshot};
 current.issue.data={automation_source:{...snapshot,run_id:'forged-run'}};
 transport(async()=>Response.json(current));
 await render();await click('[data-ir-toggle]');
 expect(host.querySelector('[data-ir-automation-source]')?.textContent).toContain('Release checks');
 expect(host.querySelector('[data-ir-automation-source]')?.textContent).toContain('Rule revision 4');
 const run=host.querySelector<HTMLAnchorElement>('[data-ir-automation-run]');
 const trigger=host.querySelector<HTMLAnchorElement>('[data-ir-automation-trigger]');
 const pause=host.querySelector<HTMLAnchorElement>('[data-ir-automation-pause]');
 expect(run?.getAttribute('href')).toBe('?project=project-a&view=development&sub=runs&run=original-failed-run');
 expect(trigger?.getAttribute('href')).toBe('?project=project-a&view=development&sub=automations&record=rule-a&panel=automation-trigger%3Afailed-trigger');
 expect(pause?.getAttribute('href')).toBe('?project=project-a&view=development&sub=automations&record=rule-a&panel=automation-trigger%3Apaused-trigger');
 const veto=(event:Event)=>event.preventDefault();
 window.addEventListener('suma:navigate',veto);
 try{
  for(const link of [run,trigger,pause]){
   const event=new MouseEvent('click',{bubbles:true,cancelable:true});
   expect(link?.dispatchEvent(event)).toBe(false);
  }
 }finally{window.removeEventListener('suma:navigate',veto);}
 current.current.automation_source={...snapshot,origin:{...snapshot.origin,failure_pause:{...snapshot.origin.failure_pause,occurrence_id:'failed-trigger'}}};
 await click('[data-ir-refresh]');
 expect(host.querySelector('[data-ir-automation-trigger]')).not.toBeNull();
 expect(host.querySelector('[data-ir-automation-pause]')).toBeNull();
 current.current.automation_source={...snapshot,run_id:'../forged-run'};
 await click('[data-ir-refresh]');
 expect(host.querySelector('[data-ir-automation-source]')).toBeNull();
 expect(host.textContent).toContain('Repair task');
 expect(host.querySelector('[data-ir-bind]')).not.toBeNull();
 expect(host.querySelector('[data-ir-close]')).toBeNull();
 expect(writes).toHaveLength(0);
});

it('opens protected package evidence and keeps closure blocked without independent verification',async()=>{
 const original={run_id:'run-failed',node_id:'package.test',attempt:2,fence:8,artifact_id:'failed-package',manifest_sha256:digest,source_commit:'b'.repeat(40),scope_root:'apps/catalog'};
 const current={...bound(),original_package:original,can_close:false,blockers:[{code:'package_regression_missing'}]};
 current.issue.data={run_id:'forged-run',artifact_id:'forged-artifact'};
 transport(async()=>Response.json(current));await render();await click('[data-ir-toggle]');
 const card=host.querySelector('[data-ir-package="origin"]');
 expect(card?.textContent).toContain('Original package failure');
 expect(card?.textContent).toContain('apps/catalog');
 expect(card?.querySelector('[data-ir-package-attempt]')?.getAttribute('href')).toBe('?project=project-a&view=development&sub=runs&run=run-failed&node=package.test&attempt=2');
 expect(card?.querySelector('[data-ir-package-artifact]')?.getAttribute('href')).toBe('?project=project-a&view=operations&sub=artifacts&record=failed-package');
 expect(host.querySelector('[data-ir-close]')).toBeNull();
 expect(host.textContent).not.toContain('forged-run');
 await act(async()=>setLocale('zh-CN',{persist:false}));
 expect(card?.textContent).toContain('原包测试失败');
 expect(host.querySelector('[data-ir-blockers]')?.textContent).toContain('请执行独立包测试');
 expect(writes).toHaveLength(0);
});

it('retains the exact verified package links after reopening',async()=>{
 const finished=closed();
 const previous={...finished.current!,closure:{...finished.current!.closure!,package_regression:{node_id:'verify-package',attempt:4,fence:11,artifact_id:'repaired-package',manifest_sha256:digest,source_commit:'b'.repeat(40),proof_id:'proof-a'}},reopened_at:stamp,reopened_by:'user-a',reopen_reason:'Regression reported'};
 transport(async()=>Response.json({...view(),cycle:2,history:[previous],history_total:1}));
 await render();await click('[data-ir-toggle]');
 const card=host.querySelector('[data-ir-history] [data-ir-package="regression"]');
 expect(card?.querySelector('[data-ir-package-attempt]')?.getAttribute('href')).toBe('?project=project-a&view=development&sub=runs&run=run-a&node=verify-package&attempt=4');
 expect(card?.querySelector('[data-ir-package-artifact]')?.getAttribute('href')).toBe('?project=project-a&view=operations&sub=artifacts&record=repaired-package');
});

it('reveals the exact linked historical closure and discloses unavailable cycles',async()=>{
 const finished=closed(),previous={...finished.current!,reopened_at:stamp,reopened_by:'user-a',reopen_reason:'Regression reported'};
 transport(async()=>Response.json({...view(),cycle:8,history:[previous],history_total:4,history_complete:false}));
 await render(session,project,issue,undefined,'repair-cycle:1');
 const original=host.querySelector('[data-ir-cycle="1"]');
 expect(original?.closest('details')?.open).toBe(true);
 expect(original?.querySelector('details')?.open).toBe(true);
 expect(original?.textContent).toContain('Verified the exact fix');
 await render(session,project,issue,undefined,'repair-cycle:2');
 expect(host.textContent).toContain('linked repair cycle 2');
 expect(host.querySelector('[data-ir-cycle="2"]')).toBeNull();
 expect(writes).toHaveLength(0);
});

it('links the protected original failure to its fixed test run and ignores editable origin fields',async()=>{
 const original_case={plan_id:'plan-source',run_id:'run-source',case_key:'case-source',plan_revision:2,plan_sha256:digest};
 const item=view();
 item.issue.data={origin:'test_plan_case',...original_case,case_key:'forged'};
 let response:IssueRepairView&{original_case?:typeof original_case}={...item,original_case};
 transport(async()=>Response.json(response));
 await render();await click('[data-ir-toggle]');
 const href=host.querySelector('[data-ir-case-origin-link]')?.getAttribute('href');
 expect(Object.fromEntries(new URLSearchParams(href??''))).toEqual({project,view:'quality',sub:'tests',record:'plan-source',run:'run-source',panel:['test-results','2',digest,'case-source',issue].join(':')});
 expect(host.querySelector('[data-ir-case-origin]')?.textContent).not.toContain('forged');
 response={...item,original_case:undefined};
 await click('[data-ir-refresh]');
 expect(host.querySelector('[data-ir-case-origin]')).toBeNull();
 expect(writes).toHaveLength(0);
});
