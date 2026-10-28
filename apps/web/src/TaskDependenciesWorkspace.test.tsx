// @vitest-environment jsdom
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {beforeEach,afterEach,it,expect,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';
import {TaskDependenciesWorkspace} from './TaskDependenciesWorkspace';
import type {TaskDependencyGraph} from './task-dependencies-model';

const flowProbe=vi.hoisted(()=>({fit:vi.fn(async()=>true)}));
// This adapter deliberately reports selection again when the callback identity
// changes, as React Flow does. It tests our synchronization boundary, not the
// actual canvas engine; production rendering is verified separately in-browser.
vi.mock('@xyflow/react',async()=>{const {useEffect}=await import('react');return {ReactFlow:({children,nodes,onSelectionChange,onInit}:{children:ReactNode;nodes:{id:string;selected?:boolean}[];onSelectionChange:(value:{nodes:{id:string;selected?:boolean}[];edges:unknown[]})=>void;onInit:(instance:unknown)=>void})=>{useEffect(()=>{onInit({fitView:flowProbe.fit});},[onInit]);useEffect(()=>{onSelectionChange({nodes:nodes.filter(node=>node.selected),edges:[]});},[onSelectionChange,nodes]);return <div data-flow-snapshot>{nodes.filter(node=>node.selected).map(node=>node.id).join(',')}{children}</div>;},Background:()=>null,Controls:()=>null,Handle:()=>null,Position:{Left:'left',Right:'right'},MarkerType:{ArrowClosed:'arrowclosed'},applyNodeChanges:(_changes:unknown,nodes:unknown)=>nodes};});
vi.mock('elkjs/lib/elk.bundled.js',()=>({default:class {async layout(value:unknown){return value;}}}));
const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
const snapshot=():TaskDependencyGraph=>({project_id:'project-a',total:3,limit:1000,complete:true,tasks:[{task_id:'a',version:2,title:'Architecture',status:'ready',depends_on:[]},{task_id:'b',version:4,title:'Develop',status:'in_progress',depends_on:['a']},{task_id:'c',version:1,title:'Verify',status:'draft',depends_on:['b']}]});
let root:Root|undefined,host:HTMLDivElement,writes:{body:any;headers:Headers}[],reads:{path:string;signal?:AbortSignal|null}[];
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();localStorage.clear();writes=[];reads=[];flowProbe.fit.mockClear();host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('matchMedia',()=>({matches:true}));vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>setTimeout(()=>callback(performance.now()),0));vi.stubGlobal('cancelAnimationFrame',(id:number)=>clearTimeout(id));});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.restoreAllMocks();vi.unstubAllGlobals();});
function transport(reply:(path:string,options?:RequestInit)=>Promise<Response>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace(/^\/api\/v1/,'');if(options?.body)writes.push({body:JSON.parse(String(options.body)),headers:new Headers(options.headers)});else reads.push({path,signal:options?.signal});return reply(path,options);}));}
async function settle(){for(let n=0;n<3;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(s=session,project='project-a',onTasks=()=>{}){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={s}><TaskDependenciesWorkspace session={s} project={project} refresh={0} onTasks={onTasks}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const element=host.querySelector<HTMLButtonElement>(selector);expect(element,selector).not.toBeNull();expect(element!.disabled,selector).toBe(false);await act(async()=>element!.click());await settle();}
async function select(name:string,value:string){const element=host.querySelector<HTMLSelectElement>(`[name=${name}]`)!;expect(element,name).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(element,value);element.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
function receipt(body:any){return {project_id:body.project_id,operation_id:body.idempotency_key,changes:body.changes.map((change:any)=>({task_id:change.task_id,previous_version:change.expected_version,version:change.expected_version+1,depends_on:change.depends_on}))};}

it('saves the keyboard-editable draft as one minimal CAS batch and independently confirms the resulting graph',async()=>{
 let graph=snapshot();transport(async(_path,options)=>{if(!options?.body)return Response.json(graph);const body=writes.at(-1)!.body;for(const change of body.changes){const task=graph.tasks.find(task=>task.task_id===change.task_id)!;task.version++;task.depends_on=change.depends_on;}return Response.json(receipt(body));});
 await render();expect(reads).toHaveLength(1);expect(reads[0].path).toBe('/task-dependencies?project_id=project-a');await select('dependency_task','c');await click('[data-dependency-remove=b]');await select('dependency_source','a');await click('[data-dependency-add]');await click('[data-dependencies-save] button');
 expect(writes).toHaveLength(1);expect(writes[0].body.changes).toEqual([{task_id:'c',expected_version:1,depends_on:['a']}]);expect(writes[0].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(reads).toHaveLength(2);expect(host.textContent).toContain('A new read confirmed');expect(host.querySelector<HTMLButtonElement>('[data-dependencies-save] button')!.disabled).toBe(true);
});

it('preserves a conflicting draft until an explicit comparison and rebase, without overwriting another task',async()=>{
 let graph=snapshot(),conflict=true;
 transport(async(_path,options)=>{if(!options?.body)return Response.json(graph);const body=writes.at(-1)!.body;if(conflict){conflict=false;graph=snapshot();graph.tasks[1]={...graph.tasks[1],version:5,depends_on:[]};graph.tasks[2]={...graph.tasks[2],version:2,depends_on:['a']};return Response.json({error:{code:'conflict',message:'Task changed'}},{status:409});}for(const change of body.changes){const task=graph.tasks.find(task=>task.task_id===change.task_id)!;task.version++;task.depends_on=change.depends_on;}return Response.json(receipt(body));});
 await render();await select('dependency_task','c');await click('[data-dependency-remove=b]');await click('[data-dependencies-save] button');expect(host.querySelector('[data-dependencies-review]')).not.toBeNull();expect(host.querySelector('[data-dependency-remove]')).toBeNull();
 await click('[data-dependencies-read-latest]');expect(writes).toHaveLength(1);expect(host.querySelector<HTMLButtonElement>('[data-dependencies-save] button')!.disabled).toBe(true);await click('[data-dependencies-keep]');await click('[data-dependencies-save] button');
 expect(writes[1].body.changes).toEqual([{task_id:'c',expected_version:2,depends_on:[]}]);expect(graph.tasks[1]).toMatchObject({version:5,depends_on:[]});expect(host.textContent).toContain('A new read confirmed');
});

it('restores an exact unknown save across language changes and never certifies an old receipt as the current graph',async()=>{
 let graph=snapshot(),invalid=true;transport(async(_path,options)=>{if(!options?.body)return Response.json(graph);return Response.json(invalid?{...receipt(writes.at(-1)!.body),operation_id:'wrong-operation'}:receipt(writes.at(-1)!.body));});
 await render();await select('dependency_task','c');await click('[data-dependency-remove=b]');await click('[data-dependencies-save] button');expect(writes).toHaveLength(1);expect(host.querySelector<HTMLSelectElement>('[name=dependency_source]')!.disabled).toBe(true);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('原保存操作尚未确认');invalid=false;graph=snapshot();graph.tasks[2]={...graph.tasks[2],version:3,depends_on:['a']};await click('[data-dependencies-restore] button');
 expect(writes).toHaveLength(2);expect(writes[1].body).toEqual(writes[0].body);expect(writes[1].headers.get('X-Suma-Expected-User')).toBe('user-a');expect(writes[1].headers.get('X-Suma-Expected-Organization')).toBe('org-a');expect(host.textContent).toContain('保存已记录，当前关系仍需确认');expect(host.textContent).not.toContain('已保存，并通过重新读取');expect(host.querySelector<HTMLButtonElement>('[data-dependencies-save] button')!.disabled).toBe(true);expect(reads).toHaveLength(2);
 await click('[data-dependencies-keep]');expect(host.querySelector('[data-dependencies-review]')).toBeNull();expect(host.querySelector<HTMLSelectElement>('[name=dependency_source]')!.disabled).toBe(false);expect(writes).toHaveLength(2);
});

it('aborts and ignores a previous project read, and offers no partial editor over the task limit',async()=>{
 let release!:(value:Response)=>void;const onTasks=vi.fn();transport(async path=>path.includes('project-a')?new Promise<Response>(resolve=>{release=resolve;}):Response.json({project_id:'project-b',total:1001,limit:1000,complete:false,tasks:[]}));
 await render();const signal=reads[0].signal;await render({...session,user:{...session.user,id:'user-b'},organization:{id:'org-b',name:'Other'}},'project-b',onTasks);expect(signal?.aborted).toBe(true);await act(async()=>release(Response.json(snapshot())));await settle();
 expect(host.textContent).toContain('1,001');expect(host.textContent).not.toContain('Architecture');expect(host.querySelector('[name=dependency_task]')).toBeNull();expect(host.querySelector('[data-dependencies-save]')).toBeNull();expect(writes).toHaveLength(0);await click('.task-dependencies-heading button');expect(onTasks).toHaveBeenCalledOnce();
});

it('hides protected facts immediately after access is revoked and retries with a fresh read scope',async()=>{
 let denied=false;transport(async()=>denied?Response.json({error:{code:'forbidden',message:'Access denied'}},{status:403}):Response.json(snapshot()));await render();expect(host.textContent).toContain('Architecture');denied=true;await click('[data-dependencies-refresh]');expect(host.textContent).not.toContain('Architecture');expect(host.querySelector('[name=dependency_task]')).toBeNull();denied=false;await click('.task-dependencies button');expect(host.textContent).toContain('Architecture');expect(reads).toHaveLength(3);
});

it('keeps canvas selection callbacks stable across inspector changes and fits only after layout',async()=>{
 vi.stubGlobal('matchMedia',()=>({matches:false}));transport(async()=>Response.json(snapshot()));await render();expect(flowProbe.fit).toHaveBeenCalledTimes(1);
 await select('dependency_task','c');expect(host.querySelector('[data-flow-snapshot]')!.textContent).toBe('c');await select('dependency_task','a');await select('dependency_task','b');expect(host.querySelector('[data-flow-snapshot]')!.textContent).toBe('b');expect(flowProbe.fit).toHaveBeenCalledTimes(1);
 await click('.task-dependencies-canvas-toolbar button');expect(flowProbe.fit).toHaveBeenCalledTimes(2);expect(writes).toHaveLength(0);
});
