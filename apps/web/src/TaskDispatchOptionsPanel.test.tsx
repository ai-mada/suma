// @vitest-environment jsdom
import {act,createRef,type Ref} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {TaskDispatchOptions,TaskDispatchToolOption,ToolProfile as ContractToolProfile} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import type {Session,Workflow} from './api';
import {setLocale} from './i18n';
import {TaskDispatchDialog} from './TaskDispatch';
import {TaskDispatchOptionsPanel,type TaskDispatchOptionsHandle} from './TaskDispatchOptionsPanel';
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
async function render(current=scope,disabled=false,panelRef?:Ref<TaskDispatchOptionsHandle>){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><TaskDispatchOptionsPanel session={session} scope={current} disabled={disabled} panelRef={panelRef} onAdopt={adopt} onUnavailable={unavailable}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const node=document.querySelector<HTMLElement>(selector);expect(node,selector).not.toBeNull();await act(async()=>node!.click());await settle();}

const searchField=()=>host.querySelector<HTMLInputElement>('[data-dispatch-options-search]')!;
async function editSearch(value:string){
 const input=searchField();
 await act(async()=>{
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);
  input.dispatchEvent(new Event('input',{bubbles:true}));
 });
}

it('rejects mismatched basis, stale nested facts and fabricated or duplicate page recommendations',()=>{
 expect(validDispatchOptions(page(),scope,session.organization.id)).toBe(true);
 const filtered={...page(),query:'Tool B'};
 expect(validDispatchOptions(filtered,scope,session.organization.id,'Tool B')).toBe(true);
 expect(validDispatchOptions(filtered,scope,session.organization.id,'tool b')).toBe(false);
 expect(validDispatchOptions(filtered,scope,session.organization.id)).toBe(false);
 expect(validDispatchOptions(page(),scope,session.organization.id,'Tool B')).toBe(false);
 const corruptions:((value:TaskDispatchOptions)=>void)[]=[value=>{value.project_id='other';},value=>{value.task_id='other';},value=>{value.task_version++;},value=>{value.workflow_version++;},value=>{value.agent_node_id='other';},value=>{value.context.task.version++;},value=>{value.execution_route.project_id='other';},value=>{value.execution_route.tenant_id='other';},value=>{value.items[1].tool.project_id='other';},value=>{value.items.push(value.items[1]);},value=>{value.total=0;},value=>{value.next_cursor='x'.repeat(4097);},value=>{value.recommended_tool_profile_id='tool-c';},value=>{value.items[1].eligible=false;},value=>{value.items[1].cost_state='free';},value=>{value.items[1].tool.observation!.profile.profile_sha256='d'.repeat(64);}];
 for(const corrupt of corruptions){const changed=structuredClone(page());corrupt(changed);expect(validDispatchOptions(changed,scope,session.organization.id)).toBe(false);}
 expect(dispatchOptionsPath(scope,'page-2')).toContain('expected_task_version=4&workflow_id=flow-a&workflow_version=2&agent_node_id=code&limit=50&before=page-2');
});

it('reads only on demand, adopts without writing, switches CNEN locally and recommends only within the current page',async()=>{
 await render();expect(requests).toHaveLength(0);await click('[data-dispatch-options-toggle]');
 expect(document.body.textContent).toContain('Page 1');expect(document.body.textContent).toContain('other pages have not been compared');
 await click('[data-dispatch-options-adopt]');expect(adopt).toHaveBeenCalledWith(expect.objectContaining({id:'tool-b',version:3}));expect(requests).toHaveLength(1);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(document.querySelector('[data-dispatch-options-adopt]')?.textContent).toBe('采用当前推荐');expect(requests).toHaveLength(1);
 await render(scope,true);await click('[data-dispatch-options-adopt]');expect(adopt).toHaveBeenCalledTimes(1);expect(requests).toHaveLength(1);
 await render();await click('[data-dispatch-page="next"]');expect(document.body.textContent).toContain('第 2 页');expect(document.querySelector('[data-dispatch-option="tool-b"]')).toBeNull();
 await click('[data-dispatch-options-adopt]');expect(adopt).toHaveBeenLastCalledWith(expect.objectContaining({id:'tool-c'}));expect(requests.every(request=>request.method==='GET')).toBe(true);
});

it('opens and focuses the existing panel once without replacing the current page',async()=>{
 const panelRef=createRef<TaskDispatchOptionsHandle>();
 const toggle=()=>host.querySelector<HTMLButtonElement>('[data-dispatch-options-toggle]')!;
 await render(scope,true,panelRef);
 await act(async()=>panelRef.current!.open());
 expect(requests).toHaveLength(0);
 expect(toggle().getAttribute('aria-expanded')).toBe('false');
 await render(scope,false,panelRef);
 await act(async()=>{panelRef.current!.open();panelRef.current!.open();});
 await settle();
 expect(document.activeElement).toBe(toggle());
 expect(toggle().getAttribute('aria-expanded')).toBe('true');
 expect(requests).toHaveLength(1);
 await click('[data-dispatch-page="next"]');
 expect(host.querySelector('[data-dispatch-option="tool-c"]')).not.toBeNull();
 await act(async()=>panelRef.current!.open());
 expect(document.activeElement).toBe(toggle());
 expect(host.querySelector('[data-dispatch-option="tool-c"]')).not.toBeNull();
 expect(requests.map(request=>request.method)).toEqual(['GET','GET']);
 expect(adopt).not.toHaveBeenCalled();
});

it('lists compatible tools first and lets operators expand current-page diagnostics locally',async()=>{
 await render();
 await click('[data-dispatch-options-toggle]');
 const details=host.querySelector<HTMLDetailsElement>('[data-dispatch-options-exclusions]')!;
 const compatible=host.querySelector('[aria-label="Compatible tools on the current page"]')!;
 expect(compatible.querySelector('[data-dispatch-option="tool-b"]')).not.toBeNull();
 expect(compatible.querySelector('[data-dispatch-option="tool-a"]')).toBeNull();
 expect(compatible.compareDocumentPosition(details)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
 expect(details.open).toBe(false);
 expect(details.querySelector('summary')?.textContent).toContain('1 on this page');
 await click('[data-dispatch-options-exclusions] summary');
 expect(details.open).toBe(true);
 expect(details.querySelector('[data-dispatch-option="tool-a"]')?.textContent).toContain('Tool unavailable');
 expect(details.querySelector('button')).toBeNull();
 expect(requests).toHaveLength(1);
 reply=async()=>Response.json(page([option('tool-c',false)],''));
 await click('[data-dispatch-page="next"]');
 expect(host.textContent).toContain('No compatible tool on this page');
 expect(host.textContent).not.toContain('try the next page');
 expect(host.querySelector<HTMLDetailsElement>('[data-dispatch-options-exclusions]')?.open).toBe(false);
 expect(host.querySelector('[data-dispatch-options-adopt]')).toBeNull();
});

it('clears protected observations after denial and rejects cross-scope or late responses before rendering',async()=>{
 await render();await click('[data-dispatch-options-toggle]');expect(document.querySelectorAll('[data-dispatch-option]')).toHaveLength(2);
 reply=async()=>Response.json({error:{code:'forbidden',message:'Access denied'}},{status:403});await click('[data-dispatch-page="refresh"]');expect(document.querySelector('[data-dispatch-option]')).toBeNull();expect(document.querySelector('[data-dispatch-options-adopt]')).toBeNull();expect(unavailable).toHaveBeenCalledOnce();
 reply=async()=>Response.json({...page(),workflow_version:9});await click('[data-dispatch-page="refresh"]');expect(document.querySelector('[data-dispatch-option]')).toBeNull();expect(document.body.textContent).toContain('does not match this task');
 let finish:((value:Response)=>void)|undefined;reply=()=>new Promise(resolve=>{finish=resolve;});
 const refresh=document.querySelector<HTMLElement>('[data-dispatch-page="refresh"]')!;await act(async()=>{refresh.click();});
 await render({...scope,expected_task_version:5});expect(document.querySelector('[data-dispatch-options-toggle]')?.getAttribute('aria-expanded')).toBe('false');
 await act(async()=>finish!(Response.json(page())));await settle();expect(document.querySelector('[data-dispatch-option]')).toBeNull();expect(adopt).not.toHaveBeenCalled();
});

it('retries the original tool page, keeps keyboard focus and rejects cursor loops',async()=>{
 const next='[data-dispatch-page="next"]';
 const refresh='[data-dispatch-page="refresh"]';
 let failed=false;
 reply=async path=>{
  if(path.includes('before=page-3'))return Response.json(page([option('tool-d')],'page-2'));
  if(path.includes('before=page-2')){
   if(!failed){failed=true;return Response.json({error:{code:'unavailable',message:'Temporarily unavailable'}},{status:503});}
   return Response.json(page([option('tool-c')],'page-3'));
  }
  return Response.json(page());
 };
 await render();await click('[data-dispatch-options-toggle]');
 const forward=host.querySelector<HTMLButtonElement>(next)!;forward.focus();await click(next);
 expect(host.querySelector('[data-dispatch-option]')).toBeNull();expect(document.activeElement).toBe(forward);
 const retry=host.querySelector<HTMLButtonElement>(refresh)!;
 retry.focus();await act(async()=>retry.click());await settle();
 expect(requests.map(request=>new URLSearchParams(request.path.split('?')[1]).get('before'))).toEqual([null,'page-2','page-2']);
 expect(document.activeElement).toBe(retry);
 expect(host.textContent).toContain('Page 2');
 expect(host.querySelector('[data-dispatch-option="tool-c"]')).not.toBeNull();
 await click(next);
 expect(host.querySelector('[data-dispatch-option]')).toBeNull();
 expect(host.textContent).toContain('does not match this task');
 await click('[data-dispatch-page="previous"]');
 expect(host.querySelector('[data-dispatch-option="tool-c"]')).not.toBeNull();
 await click(refresh);
 expect(requests.at(-1)?.path).toContain('before=page-2');
 await click('[data-dispatch-page="first"]');
 expect(host.querySelector('[data-dispatch-option="tool-b"]')).not.toBeNull();
 expect(requests.every(request=>request.method==='GET')).toBe(true);expect(adopt).not.toHaveBeenCalled();
});

it('applies search on Enter and keeps the committed filter through paging and retry',async()=>{
 const submitted='Worker-A';
 let failed=false;
 reply=async path=>{
  const params=new URL(path,'http://localhost').searchParams;
  if(!params.has('query'))return Response.json(page());
  expect(params.get('query')).toBe(submitted);
  if(params.has('before')){
   if(!failed){failed=true;return Response.json({error:{code:'unavailable',message:'Temporarily unavailable'}},{status:503});}
   return Response.json({...page([option('tool-d')],''),query:submitted,total:2});
  }
  return Response.json({...page([option('tool-c')],'filtered-next'),query:submitted,total:2});
 };
 await render();
 await click('[data-dispatch-options-toggle]');
 await click('[data-dispatch-options-adopt]');
 await editSearch('  '+submitted+'  ');
 expect(requests).toHaveLength(1);
 searchField().focus();
 const enter=new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true});
 await act(async()=>{searchField().dispatchEvent(enter);});await settle();
 expect(enter.defaultPrevented).toBe(true);
 expect(document.activeElement).toBe(searchField());
 expect(host.textContent).toContain('“'+submitted+'” · 1 on this page · 2 matching');
 expect(host.querySelector('[data-dispatch-option="tool-b"]')).toBeNull();
 await editSearch('different unsent draft');
 await click('[data-dispatch-page="next"]');
 expect(host.querySelector('[data-dispatch-option]')).toBeNull();
 await click('[data-dispatch-page="refresh"]');
 expect(host.textContent).toContain('Page 2');
 expect(host.querySelector('[data-dispatch-option="tool-d"]')).not.toBeNull();
 expect(adopt).toHaveBeenCalledTimes(1);
 expect(unavailable).not.toHaveBeenCalled();
 const filters=requests.map(request=>{
  const params=new URL(request.path,'http://localhost').searchParams;
  return [request.method,params.get('query'),params.get('before')];
 });
 expect(filters).toEqual([
  ['GET',null,null],
  ['GET',submitted,null],
  ['GET',submitted,'filtered-next'],
  ['GET',submitted,'filtered-next'],
 ]);
});

it('validates the byte limit, replaces stale searches and clears an empty result to page one',async()=>{
 let completeSlow:((value:Response)=>void)|undefined;
 reply=async path=>{
  const query=new URL(path,'http://localhost').searchParams.get('query');
  if(query==='slow')return new Promise(resolve=>{completeSlow=resolve;});
  if(query==='absent')return Response.json({...page([],''),query,total:0});
  return Response.json(page());
 };
 await render();
 await click('[data-dispatch-options-toggle]');
 await editSearch('中'.repeat(41));
 await click('[data-dispatch-options-search-submit]');
 expect(searchField().getAttribute('aria-invalid')).toBe('true');
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('The query is too long');
 expect(requests).toHaveLength(1);
 await editSearch('slow');
 await click('[data-dispatch-options-search-submit]');
 expect(host.querySelector('[data-dispatch-option]')).toBeNull();
 await editSearch('absent');
 await click('[data-dispatch-options-search-submit]');
 expect(host.textContent).toContain('No tool configurations match this search');
 await act(async()=>completeSlow!(Response.json({error:{code:'forbidden',message:'Stale denial'}},{status:403})));
 await settle();
 expect(host.textContent).toContain('No tool configurations match this search');
 expect(unavailable).not.toHaveBeenCalled();
 await click('[data-dispatch-options-search-clear]');
 expect(searchField().value).toBe('');
 expect(document.activeElement).toBe(searchField());
 expect(host.querySelector('[data-dispatch-option="tool-b"]')).not.toBeNull();
 expect(new URL(requests.at(-1)!.path,'http://localhost').searchParams.has('query')).toBe(false);
 expect(adopt).not.toHaveBeenCalled();
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

it('selects a nonrecommended compatible tool on a later page for the existing explicit preview',async()=>{
 const alternate=option('tool-d');
 alternate.tool={...alternate.tool,version:7,worker_id:'worker-d'};
 const denied:TaskDispatchToolOption={...option('tool-e'),eligible:false,reason_code:'route_incompatible',reason:'Project execution route differs'};
 reply=async(path,init)=>{
  if(init?.body)throw new Error('receipt lost');
  if(path.startsWith('/workflows?'))return Response.json({items:[workflow]});
  if(path.startsWith('/tool-profiles?'))return Response.json({items:[profile('tool-b')],total:5,next_cursor:'inventory-next'});
  if(path.includes('before=page-2'))return Response.json({...page([option('tool-c'),alternate,denied],''),total:5});
  if(path.includes('/dispatch-options?'))return Response.json({...page(),total:5});
  return Response.json({items:[],total:0,next_cursor:''});
 };
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><TaskDispatchDialog project={project} session={session} task={{id:'task-a',project_id:project,version:4,title:'Develop task'}} initialWorkflow={workflow} onPlan={vi.fn()} onClose={vi.fn()} onRun={vi.fn()}/></ApiViewProvider>);});
 await settle();
 await click('[data-dispatch-options-toggle]');
 await click('[data-dispatch-page="next"]');
 const action='[data-dispatch-option="tool-d"] button';
 expect(document.querySelector(action)).not.toBeNull();
 expect(document.querySelector(action)?.getAttribute('aria-label')).toContain('Tool tool-d');
 expect(document.querySelector('[data-dispatch-option="tool-c"] [data-dispatch-options-adopt]')).not.toBeNull();
 expect(document.querySelector('[data-dispatch-option="tool-e"] button')).toBeNull();
 await click(action);
 const selector=document.querySelector<HTMLSelectElement>('[data-dispatch-manual-tool]');
 expect(selector?.value).toBe('tool-d');
 expect(selector?.selectedOptions[0]?.textContent).toContain('worker-d');
 expect(requests.some(request=>request.method==='POST')).toBe(false);
 await click('[data-dispatch-prepare]');
 const writes=requests.filter(request=>request.method==='POST');
 expect(writes).toHaveLength(1);
 expect(writes[0]).toMatchObject({path:'/task-dispatch-plans',body:{expected_task_version:4,tool_profile_id:'tool-d',expected_tool_version:7}});
});
