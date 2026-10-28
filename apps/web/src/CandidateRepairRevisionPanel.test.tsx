// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {CandidateCompositionPanel} from './CandidateCompositionPanel';
import {CandidateRepairPlanFacts} from './CandidateRepairRevisionPanel';
import {setLocale} from './i18n';
import captured from './test-fixtures/candidate-conflict-repair-plan.json';
import type {Session} from './api';

// The complete captured032 wire is retained; only the new033 fields/scenarios
// below are added from the frozen revision contract, not a clean-merge mock.
const project=captured.project_id,task=captured.task_id,hash='a'.repeat(64),stamp='2026-10-27T01:00:00Z';
const session:Session={user:{id:'revision-user',name:'Current viewer',email:'viewer@example.test'},organization:{id:'revision-org',name:'Org'},projects:[]};
const lineage=(id=captured.id,parent='',ordinal=1)=>({root_plan_id:captured.id,parent_plan_id:parent,ordinal,current_head_plan_id:id,is_current:true,reason:parent?'Use the healthy native tool':'Initial repair plan observed before explicit revision',created_at:stamp,definition_sha256:hash,creator_kind:'browser',creator_principal_id:'original-author',observed:!parent});
const parentPlan=()=>({...structuredClone(captured),repair_revision:lineage(),allowed_actions:['start','refresh','revise']});
const workflow={id:'revision-workflow',name:'Healthy repair workflow',version:3,nodes:[{id:'code',name:'Native repair',type:'agent'},{id:'test',name:'Independent tests',type:'test',command:['formal-check'],config:{candidate_from:'code'}}],edges:[{from:'code',to:'test'}]};
const tool={id:'healthy-tool',name:'Healthy native tool',project_id:project,worker_id:'healthy-worker',version:4,state:'enabled',available:true,reason:'',profile:{command_id:'native-repair',adapter:'suma',protocol:'suma-jsonl-v1',workspace_tools_version:'attempt-workspace-v1',conflict_repair_protocol:'suma.candidate-conflict-repair.v1',read_only:false,profile_sha256:hash,executable_sha256:hash,tool_version:'1.0.0'}};
let root:Root|undefined,host:HTMLDivElement,current:any,context:any,writes:{path:string;body:any}[],reads:string[],changed:string[],reply:(path:string,init?:RequestInit)=>Promise<Response>;
const revised=()=>({...parentPlan(),id:'new-repair-plan',version:1,workflow_id:workflow.id,workflow_version:workflow.version,workflow,producer_node_id:'code',tool_profile_id:tool.id,tool_profile_version:tool.version,tool_profile_sha256:hash,operation_id:'revision-operation',repair_revision:lineage('new-repair-plan',captured.id,2),allowed_actions:['start','refresh','revise']});
function defaults(path:string){if(path.startsWith('/model-execution/options?'))return Response.json({project_id:project,tool_profile_id:tool.id,tool_profile_version:4,tool_profile_sha256:hash,observed_at:stamp,supported:false,codex_supported:false,items:[]});if(path.includes('/repair-revision-context?'))return Response.json(context);if(path.startsWith('/workflows?'))return Response.json({items:[workflow,{...workflow,id:'command-only',nodes:[{...workflow.nodes[0],type:'command'},workflow.nodes[1]]}]});if(path.startsWith('/tool-profiles?'))return Response.json({items:[tool,{...tool,id:'ordinary',profile:{...tool.profile,conflict_repair_protocol:undefined}}],total:2});if(path.startsWith('/project-execution/route?'))return Response.json({...captured.execution_route,mode:'private',allowed_actions:[],as_of:stamp});if(path.startsWith('/candidate-compositions/'))return Response.json(current);throw Error('Unexpected read '+path);}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();current=parentPlan();context={project_id:project,plan_id:current.id,plan_version:current.version,task_version:current.task_version,conflict_basis:current.conflict_basis,eligibility_mode:'ready',can_revise:true,blockers:[]};writes=[];reads=[];changed=[];host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);reply=async(path,init)=>init?.body?Response.json(current=revised()):defaults(path);vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(init?.body)writes.push({path,body:JSON.parse(String(init.body))});else reads.push(path);return reply(path,init);}));});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><CandidateCompositionPanel session={session} project={project} taskID={task} planID={captured.id} onPlan={id=>changed.push(id)}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const node=host.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}
async function change(selector:string,value:string){const node=host.querySelector<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement>(selector)!;expect(node,selector).not.toBeNull();const proto=node instanceof HTMLSelectElement?HTMLSelectElement.prototype:node instanceof HTMLTextAreaElement?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;await act(async()=>{Object.getOwnPropertyDescriptor(proto,'value')!.set!.call(node,value);node.dispatchEvent(new Event(node instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function choose(){await change('[name=repair_revision_workflow]',workflow.id+'@3');await change('[name=repair_revision_tool]',tool.id);await change('[name=repair_revision_producer]','code');await change('[name=repair_revision_reason]','Use the healthy native tool');await click('[name=repair_revision_ack]');}

it('localizes an observed system reason in both languages while preserving the original and user reasons',async()=>{
 const originalReason=current.repair_revision.reason;
 await act(async()=>{root=createRoot(host);root.render(<CandidateRepairPlanFacts project={project} plan={current}/>);});
 expect(host.textContent).not.toContain(originalReason);
 expect(host.textContent).toContain('This earlier definition');
 await act(async()=>{await setLocale('zh-CN',{persist:false});});
 expect(host.textContent).toContain('这份早期定义在开始记录修订历史时留存');
 expect(host.textContent).not.toContain(originalReason);
 expect(current.repair_revision.reason).toBe(originalReason);
 const userReason='保留原件 / retain literal reason';
 const child={...current,repair_revision:{...lineage('new-repair-plan',captured.id,2),reason:userReason}};
 await act(async()=>root!.render(<CandidateRepairPlanFacts project={project} plan={child}/>));
 expect(host.textContent).toContain(userReason);
 await act(async()=>{await setLocale('en-US',{persist:false});});
 expect(host.textContent).toContain(userReason);
});

it('loads revision configuration only when expanded and opens the exact same-task new preview without starting it',async()=>{
 await render();expect(reads.some(path=>path.includes('repair-revision-context')||path.startsWith('/workflows')||path.startsWith('/tool-profiles'))).toBe(false);
 await click('[data-repair-revision-toggle]');await choose();await click('[data-repair-revision-create]');
 expect(writes).toHaveLength(1);expect(writes[0]).toMatchObject({path:'/candidate-compositions/'+captured.id+'/repair-revisions',body:{project_id:project,expected_version:captured.version,expected_task_version:captured.task_version,expected_conflict_sha256:captured.conflict_basis.sha256,workflow_id:'revision-workflow',workflow_version:3,producer_node_id:'code',tool_profile_id:'healthy-tool',expected_tool_version:4,acknowledge:true,reason:'Use the healthy native tool'}});
 expect(changed).toEqual(['new-repair-plan']);expect(current.task_id).toBe(task);expect(current.run_id).toBe('');expect(host.querySelector<HTMLInputElement>('[name=composition_start_ack]')?.checked).toBe(false);expect(host.querySelector('[data-repair-revision-parent]')?.getAttribute('href')).toContain('composition%3A'+captured.id);
});
it('fixes native revision model parameters and retains the original operation after the wrong binding is returned',async()=>{
 const modelSha='1'.repeat(64),selection={deployment_id:'repair-model',deployment_revision:2,deployment_sha256:modelSha,parameters:{max_steps:2,max_output_tokens:100}};
 const binding={protocol:'suma-model-execution-v1',deployment:{deployment_id:'repair-model',revision:2,deployment_sha256:modelSha,model:'Revision model'},parameters:selection.parameters,parameters_sha256:hash,binding_sha256:hash,input_token_reservation:1000,output_token_reservation:100};
 let bad=true;reply=async(path,init)=>{
  if(path.startsWith('/model-execution/options?'))return Response.json({project_id:project,tool_profile_id:tool.id,tool_profile_version:4,tool_profile_sha256:hash,observed_at:stamp,supported:true,parameter_limits:{max_steps:128,max_output_tokens:131072},codex_supported:false,items:[{deployment:{...binding.deployment,protocol:'openai-chat-completions-v1',endpoint_origin:'http://127.0.0.1:1234',endpoint_config_sha256:hash,context_window:500},cost_state:'unpriced',default_parameters:selection.parameters}]});
  if(init?.body){const actual={...binding,parameters:bad?{...selection.parameters,max_steps:3}:selection.parameters},result={...revised(),model_execution:actual,workflow:{...workflow,nodes:[{...workflow.nodes[0],config:{model_execution:actual}},workflow.nodes[1]]}};if(!bad)current=result;return Response.json(result);}return defaults(path);
 };
 await render();await click('[data-repair-revision-toggle]');await choose();expect(host.querySelector<HTMLButtonElement>('[data-repair-revision-create]')?.disabled).toBe(true);
 await change('[data-model-select]',modelSha);await click('[name=repair_revision_ack]');await click('[data-repair-revision-create]');expect(writes[0].body.model_execution_selection).toEqual(selection);expect(changed).toHaveLength(0);expect(host.querySelector('[data-repair-revision-restore]')).not.toBeNull();
 await act(async()=>root!.unmount());root=undefined;bad=false;await render();await click('[data-repair-revision-restore]');expect(writes[1]).toEqual(writes[0]);expect(changed).toEqual(['new-repair-plan']);expect(host.textContent).toContain('Revision model');
});

it('keeps selected inputs on 409 and requires fresh facts and acknowledgement before a new revision',async()=>{
 let conflict=true;reply=async(path,init)=>{if(!init?.body)return defaults(path);if(conflict){conflict=false;current={...parentPlan(),version:captured.version+1};context={...context,plan_version:current.version,conflict_basis:{...context.conflict_basis,sha256:'f'.repeat(64)}};return Response.json({error:{code:'conflict',message:'Repair plan changed'}},{status:409});}return Response.json(current=revised());};
 await render();await click('[data-repair-revision-toggle]');await choose();await click('[data-repair-revision-create]');await click('.conflict-banner button');
 expect(host.querySelector<HTMLTextAreaElement>('[name=repair_revision_reason]')?.value).toBe('Use the healthy native tool');expect(host.querySelector<HTMLInputElement>('[name=repair_revision_ack]')?.checked).toBe(false);expect(host.querySelector('.conflict-banner')).toBeNull();await click('[name=repair_revision_ack]');await click('[data-repair-revision-create]');expect(writes).toHaveLength(2);expect(writes[1].body.expected_version).toBe(captured.version+1);expect(writes[1].body.expected_conflict_sha256).toBe('f'.repeat(64));expect(writes[1].body.idempotency_key).not.toBe(writes[0].body.idempotency_key);
});

it('recovers a malformed success after reload even when the old plan has become historical',async()=>{
 let malformed=true;reply=async(path,init)=>{if(!init?.body)return defaults(path);if(malformed){malformed=false;current={...parentPlan(),repair_revision:{...lineage(),is_current:false,current_head_plan_id:'new-repair-plan'},allowed_actions:[]};context={...context,can_revise:false,eligibility_mode:'',blockers:['A newer repair plan exists']};return Response.json({id:'new-repair-plan'});}return Response.json(current=revised());};
 await render();await click('[data-repair-revision-toggle]');await choose();await click('[data-repair-revision-create]');expect(changed).toHaveLength(0);const original=structuredClone(writes[0]);await act(async()=>root!.unmount());root=undefined;await setLocale('zh-CN',{persist:false});await render();await click('[data-repair-revision-restore]');expect(writes).toHaveLength(2);expect(writes[1]).toEqual(original);expect(changed).toEqual(['new-repair-plan']);
});

it('hides the protected child plan immediately when revision or Start loses write authority',async()=>{
 await render();await click('[data-repair-revision-toggle]');await choose();reply=async(path,init)=>init?.body?Response.json({error:{code:'forbidden',message:'Revision authority revoked'}},{status:403}):defaults(path);await click('[data-repair-revision-create]');expect(host.querySelector('[data-composition-plan]')).toBeNull();expect(host.textContent).toContain('Revision authority revoked');expect(changed).toHaveLength(0);
});

it('requires server-certified startup evidence before replacing a failed plan and sends its exact recovery basis',async()=>{
 current={...parentPlan(),state:'failed',run_id:'failed-repair-run',run_state:'failed',task_state:'in_progress',allowed_actions:[]};context={...context,can_revise:false,eligibility_mode:'',blockers:['Startup proof is missing']};await render();await click('[data-repair-revision-toggle]');expect(host.querySelector('[data-repair-revision-form]')).toBeNull();expect(host.textContent).toContain('Startup proof is missing');
 context={...context,can_revise:true,eligibility_mode:'certified_startup_failure',blockers:[],recovery_basis:{plan_id:current.id,plan_version:current.version,task_id:task,task_version:current.task_version,run_id:current.run_id,run_version:3,attempts:[{assignment:{run_id:current.run_id,node_id:current.producer_node_id,attempt:1,fence:2,worker_id:'failed-worker'},result_sha256:hash,startup_failure_sha256:'b'.repeat(64)}],sha256:'c'.repeat(64)}};await click('[data-repair-revision-refresh]');await choose();await click('[data-repair-revision-create]');expect(writes).toHaveLength(1);expect(writes[0].body.expected_recovery_sha256).toBe('c'.repeat(64));expect(current.run_id).toBe('');expect(changed).toEqual(['new-repair-plan']);
});
