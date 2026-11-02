// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {Session} from './api';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {ToolProfile} from './tool-model';
import {ToolTaskHandoffDialog} from './ToolTaskHandoffDialog';

const project='project-a',stamp='2026-11-02T13:30:00Z';
const session:Session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
function profile():ToolProfile{
 const definition={command_id:'codex-task',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'codex-cli 0.1',read_only:false};
 return {id:'exact-tool-outside-first-page',project_id:project,worker_id:'worker-a',name:'Exact development tool',version:4,state:'enabled',profile:definition,enabled_by:'user-a',created_at:stamp,updated_at:stamp,available:true,reason:'',last_seen_at:stamp,observation:{profile:{...definition},available:true,reason:''}};
}
function task(id='target-task',version=3){return {id,project_id:project,version,title:'Task '+id,status:'todo'};}
function page(second=false){const rows=[task(second?'target-task':'first-task')];return {items:rows,task_plans:rows.map(row=>({task_id:row.id,iteration_id:'',plan_version:0})),total:2,next_cursor:second?'':'page-2',as_of:stamp};}
let root:Root|undefined,host:HTMLDivElement,chosen:ReturnType<typeof vi.fn>,closed:ReturnType<typeof vi.fn>;
let calls:{path:string;method:string;headers:Headers}[],reply:(path:string,options?:RequestInit)=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);chosen=vi.fn();closed=vi.fn();calls=[];
 reply=async path=>Response.json(path.startsWith('/task-workspace?')?page(path.includes('cursor=page-2')):path.startsWith('/tasks/')?task('target-task',7):profile());
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{const path=String(input).replace('/api/v1','');calls.push({path,method:options?.method||'GET',headers:new Headers(options?.headers)});return reply(path,options);}));
});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.restoreAllMocks();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});
async function settle(){for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(account=session,tool=profile(),disabled=false){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={account}><ToolTaskHandoffDialog session={account} project={project} tool={tool} disabled={disabled} onChoose={chosen} onClose={closed}/></ApiViewProvider>);});await settle();}
async function click(selector:string){const button=document.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function choose(value='target-task'){const select=document.querySelector<HTMLSelectElement>('[data-handoff-task]')!;await act(async()=>{select.value=value;select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
const continuation='[data-handoff-action="continue"]';

it('T213 explicitly picks a paged task and rereads its current version and exact out-of-page tool using only scoped GETs',async()=>{
 await render();expect((document.querySelector(continuation) as HTMLButtonElement).disabled).toBe(true);expect(chosen).not.toHaveBeenCalled();
 expect(document.body.textContent).toContain('Exact development tool');expect(document.body.textContent).toContain('Version 4 · Worker worker-a');expect(document.body.textContent).toContain('Loaded 1 of 2 tasks');
 const search=document.querySelector<HTMLInputElement>('[data-handoff-search]')!;
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(search,'target query');search.dispatchEvent(new Event('input',{bubbles:true}));});
 await click('[data-handoff-action="search"]');await act(async()=>{await new Promise(resolve=>setTimeout(resolve,200));});await settle();
 expect(calls.some(call=>call.path.includes('q=target+query'))).toBe(true);
 await click('[data-handoff-action="more"]');await choose();await click(continuation);
 expect(document.body.textContent).toContain('Loaded 2 of 2 tasks');expect(document.body.textContent).not.toContain('{version}');expect(document.body.textContent).not.toContain('{loaded}');
 expect(chosen).toHaveBeenCalledOnce();expect(chosen.mock.calls[0][0]).toMatchObject({id:'target-task',project_id:project,version:7});
 expect(chosen.mock.calls[0][1]).toEqual({project_id:project,id:profile().id,version:4,worker_id:'worker-a',profile_sha256:'a'.repeat(64),name:'Exact development tool'});
 expect(calls.some(call=>call.path.includes('cursor=page-2'))).toBe(true);
 expect(calls.filter(call=>call.path.startsWith('/tool-profiles/')).map(call=>call.path)).toEqual(['/tool-profiles/'+profile().id+'?project_id='+project]);
 expect(calls.some(call=>call.path.startsWith('/tool-profiles?'))).toBe(false);
 for(const call of calls){expect(call.method).toBe('GET');expect(call.headers.get('X-Suma-Expected-User')).toBe('user-a');expect(call.headers.get('X-Suma-Expected-Organization')).toBe('org-a');}
 expect(calls.some(call=>/task-dispatch-plans|\/start|\/commands/.test(call.path))).toBe(false);
});

it('T213 blocks changed tools and revoked reads without upgrading a pin, and does not read for an ineligible source',async()=>{
 reply=async path=>Response.json(path.startsWith('/task-workspace?')?{...page(true),total:1}:path.startsWith('/tasks/')?task():{...profile(),version:5});
 await render();await choose();await click(continuation);expect(chosen).not.toHaveBeenCalled();expect(document.body.textContent).toContain('basis changed');expect((document.querySelector(continuation) as HTMLButtonElement).disabled).toBe(true);
 const newer={...profile(),version:5};
 reply=async path=>path.startsWith('/task-workspace?')?Response.json({...page(true),total:1}):Response.json({error:{code:'forbidden',message:'Current access refused'}},{status:403});
 await render(session,newer);await choose();await click(continuation);expect(chosen).not.toHaveBeenCalled();expect((document.querySelector(continuation) as HTMLButtonElement).disabled).toBe(true);
 expect(document.body.textContent).not.toContain('Exact development tool');expect(document.querySelector('[data-handoff-task] option[value="target-task"]')).toBeNull();
 const before=calls.length;await render(session,{...newer,state:'paused'});expect(document.body.textContent).toContain('cannot be used for a task');expect(calls).toHaveLength(before);
});

it('T213 rejects late exact reads after actor ABA and close without changing the current scope',async()=>{
 let pending:{path:string;resolve:(value:Response)=>void}[]=[];
 reply=async path=>path.startsWith('/task-workspace?')?Response.json({...page(true),total:1}):new Promise<Response>(resolve=>pending.push({path,resolve}));
 await render();await choose();await click(continuation);expect(pending).toHaveLength(2);
 const old=pending;pending=[];await render({...session,user:{...session.user,id:'user-b'}});await render();
 await act(async()=>{for(const item of old)item.resolve(Response.json(item.path.startsWith('/tasks/')?task():profile()));});await settle();expect(chosen).not.toHaveBeenCalled();
 await choose();await click(continuation);expect(pending).toHaveLength(2);await click('[data-handoff-close]');expect(closed).toHaveBeenCalledOnce();
 await act(async()=>{for(const item of pending)item.resolve(Response.json(item.path.startsWith('/tasks/')?task():profile()));});await settle();expect(chosen).not.toHaveBeenCalled();
 expect(calls.every(call=>call.method==='GET')).toBe(true);
});
