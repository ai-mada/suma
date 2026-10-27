// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {TaskDispatchOptions,TaskDispatchToolOption,ToolProfile as ContractToolProfile} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import type {Session,Workflow} from './api';
import {setLocale} from './i18n';
import {TaskDispatchDialog} from './TaskDispatch';
import {TaskDispatchOptionsPanel} from './TaskDispatchOptionsPanel';
import {dispatchOptionsPath,validDispatchOptions,type DispatchOptionsScope} from './task-dispatch-options-model';

const stamp='2026-10-27T08:00:00Z',project='project-a',session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const scope:DispatchOptionsScope={project_id:project,task_id:'task-a',expected_task_version:4,workflow_id:'flow-a',workflow_version:2,agent_node_id:'code'};
const workflow:Workflow={id:'flow-a',name:'Code and test',version:2,nodes:[{id:'code',name:'Develop',type:'agent'},{id:'verify',name:'Independent test',type:'test',command:['verify'],config:{candidate_from:'code'}}],edges:[{from:'code',to:'verify'}]};
function profile(id:string,version=3):ContractToolProfile{
 const definition={command_id:'coder',adapter:'suma',protocol:'suma-jsonl-v1',profile_sha256:'c'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0.0',read_only:false};
 return {id,project_id:project,worker_id:'worker-a',name:'Tool '+id,version,state:'enabled',profile:definition,enabled_by:'user-a',created_at:stamp,updated_at:stamp,available:true,reason:'',last_seen_at:stamp,observation:{profile:{...definition},available:true,reason:''}};
}
function option(id:string,eligible=true):TaskDispatchToolOption{
 return {tool:{...profile(id),available:eligible},eligible,reason_code:eligible?'eligible':'tool_unavailable',reason:eligible?'':'inventory_stale',model_state:'fixed_by_tool',cost_state:'unknown',eligible_basis:'current_observation_and_compatibility'};
}
function page(items=[option('tool-a',false),option('tool-b')],next='page-2'):TaskDispatchOptions{
 return {project_id:project,task_id:scope.task_id,task_version:scope.expected_task_version,workflow_id:scope.workflow_id,workflow_version:scope.workflow_version,agent_node_id:scope.agent_node_id,observed_at:stamp,context:{task:{id:scope.task_id,version:scope.expected_task_version,title:'Develop task',description:'Implement the requirement',acceptance_criteria:['Independent tests pass']}},execution_route:{tenant_id:session.organization.id,project_id:project,version:0,pool_id:'',grant_id:'',grant_version:0},items,total:3,next_cursor:next,recommended_tool_profile_id:items.find(item=>item.eligible)?.tool.id||''};
}
let root:Root|undefined,host:HTMLDivElement,requests:{path:string;method:string;body?:Record<string,unknown>}[],reply:(path:string,init?:RequestInit)=>Promise<Response>,adopt:ReturnType<typeof vi.fn>,unavailable:ReturnType<typeof vi.fn>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});requests=[];adopt=vi.fn();unavailable=vi.fn();sessionStorage.clear();host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 reply=async path=>Response.json(path.includes('before=page-2')?page([option('tool-c')],''):page());
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const path=String(input).replace('/api/v1','');requests.push({path,method:init?.method||'GET',body:init?.body?JSON.parse(String(init.body)):undefined});return reply(path,init);}));
});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
async function settle(){for(let i=0;i<4;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(current=scope,disabled=false){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><TaskDispatchOptionsPanel session={session} scope={current} disabled={disabled} onAdopt={adopt} onUnavailable={unavailable}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const node=document.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}

it('rejects mismatched basis, stale nested facts and fabricated or duplicate page recommendations',()=>{
 expect(validDispatchOptions(page(),scope)).toBe(true);
 const corruptions:((value:TaskDispatchOptions)=>void)[]=[value=>{value.project_id='other';},value=>{value.task_id='other';},value=>{value.task_version++;},value=>{value.workflow_version++;},value=>{value.agent_node_id='other';},value=>{value.context.task.version++;},value=>{value.execution_route.project_id='other';},value=>{value.items[1].tool.project_id='other';},value=>{value.items.push(value.items[1]);},value=>{value.total=0;},value=>{value.next_cursor='x'.repeat(4097);},value=>{value.recommended_tool_profile_id='tool-c';},value=>{value.items[1].eligible=false;},value=>{value.items[1].cost_state='free';},value=>{value.items[1].tool.observation!.profile.profile_sha256='d'.repeat(64);}];
 for(const corrupt of corruptions){const changed=structuredClone(page());corrupt(changed);expect(validDispatchOptions(changed,scope)).toBe(false);}
 expect(dispatchOptionsPath(scope,'page-2')).toContain('expected_task_version=4&workflow_id=flow-a&workflow_version=2&agent_node_id=code&limit=50&before=page-2');
});

it('reads only on demand, adopts without writing, switches CNEN locally and recommends only within the current page',async()=>{
 await render();expect(requests).toHaveLength(0);await click('[data-dispatch-options-toggle]');
 expect(document.body.textContent).toContain('Page 1');expect(document.body.textContent).toContain('other pages have not been compared');
 await click('[data-dispatch-options-adopt]');expect(adopt).toHaveBeenCalledWith(expect.objectContaining({id:'tool-b',version:3}));expect(requests).toHaveLength(1);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(document.querySelector('[data-dispatch-options-adopt]')?.textContent).toBe('采用当前推荐');expect(requests).toHaveLength(1);
 await render(scope,true);await click('[data-dispatch-options-adopt]');expect(adopt).toHaveBeenCalledTimes(1);expect(requests).toHaveLength(1);
 await render();await click('[data-dispatch-options-more]');expect(document.body.textContent).toContain('第 2 页');expect(document.querySelector('[data-dispatch-option="tool-b"]')).toBeNull();
 await click('[data-dispatch-options-adopt]');expect(adopt).toHaveBeenLastCalledWith(expect.objectContaining({id:'tool-c'}));expect(requests.every(request=>request.method==='GET')).toBe(true);
});

it('clears protected observations after denial and rejects cross-scope or late responses before rendering',async()=>{
 await render();await click('[data-dispatch-options-toggle]');expect(document.querySelectorAll('[data-dispatch-option]')).toHaveLength(2);
 reply=async()=>Response.json({error:{code:'forbidden',message:'Access denied'}},{status:403});await click('[data-dispatch-options-refresh]');expect(document.querySelector('[data-dispatch-option]')).toBeNull();expect(document.querySelector('[data-dispatch-options-adopt]')).toBeNull();expect(unavailable).toHaveBeenCalledOnce();
 reply=async()=>Response.json({...page(),workflow_version:9});await click('[data-dispatch-options-refresh]');expect(document.querySelector('[data-dispatch-option]')).toBeNull();expect(document.body.textContent).toContain('does not match this task');
 let finish:((value:Response)=>void)|undefined;reply=()=>new Promise(resolve=>{finish=resolve;});
 const refresh=document.querySelector<HTMLElement>('[data-dispatch-options-refresh]')!;await act(async()=>{refresh.click();});
 await render({...scope,expected_task_version:5});expect(document.querySelector('[data-dispatch-options-toggle]')?.getAttribute('aria-expanded')).toBe('false');
 await act(async()=>finish!(Response.json(page())));await settle();expect(document.querySelector('[data-dispatch-option]')).toBeNull();expect(adopt).not.toHaveBeenCalled();
});

it('uses the newly observed exact profile version in the existing explicit preview, preserving its pending request',async()=>{
 const old=profile('tool-b',2);old.profile={...old.profile,profile_sha256:'a'.repeat(64)};
 reply=async(path,init)=>{
  if(init?.body)throw new Error('receipt lost');
  if(path.startsWith('/workflows?'))return Response.json({items:[workflow]});
  if(path.startsWith('/tool-profiles?'))return Response.json({items:[old],total:1,next_cursor:''});
  if(path.includes('/dispatch-options?'))return Response.json(page());
  return Response.json({items:[],total:0,next_cursor:''});
 };
 const renderDialog=async(version=4)=>{await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={{id:'task-a',project_id:project,version,title:'Develop task'}} initialWorkflow={workflow} onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()}/></ApiViewProvider>);});await settle();};
 await renderDialog();await click('[data-dispatch-options-toggle]');await click('[data-dispatch-options-adopt]');
 expect(document.querySelector('.tool-context code')?.textContent).toBe('c'.repeat(64));expect(requests.some(request=>request.method==='POST')).toBe(false);
 await act(async()=>{document.querySelector('form.tool-form')!.dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));});await settle();
 const writes=requests.filter(request=>request.method==='POST');expect(writes).toHaveLength(1);expect(writes[0].body).toMatchObject({expected_task_version:4,tool_profile_id:'tool-b',expected_tool_version:3});
 const original=JSON.parse(sessionStorage.getItem('suma-tool-recovery-v1:org-a:user-a:project-a:dispatch')!);expect({...original.body,idempotency_key:original.idempotency_key}).toEqual(writes[0].body);
 const readCount=requests.filter(request=>request.path.includes('/dispatch-options?')).length;await renderDialog(5);expect(document.querySelector<HTMLButtonElement>('[data-dispatch-options-toggle]')?.disabled).toBe(true);expect(requests.filter(request=>request.path.includes('/dispatch-options?'))).toHaveLength(readCount);expect(JSON.parse(sessionStorage.getItem('suma-tool-recovery-v1:org-a:user-a:project-a:dispatch')!)).toEqual(original);
});
