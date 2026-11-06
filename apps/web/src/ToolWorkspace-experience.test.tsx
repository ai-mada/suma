// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {ToolWorkspace} from './ToolWorkspace';
import {setLocale} from './i18n';
import type {Session} from './api';

const session:Session={user:{id:'user-one',name:'User',email:'user@example.test'},organization:{id:'team-one',name:'Team'},projects:[]},project='project-one',worker='worker-one',at='2026-11-02T00:00:00Z';
const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
let root:Root,host:HTMLDivElement,reads:string[],writes:string[];
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');host.id='main';host.tabIndex=-1;document.body.append(host);root=createRoot(host);reads=[];writes=[];
 vi.stubGlobal('requestAnimationFrame',(callback:FrameRequestCallback)=>setTimeout(()=>callback(performance.now()),0));vi.stubGlobal('cancelAnimationFrame',clearTimeout);
 vi.spyOn(HTMLElement.prototype,'getClientRects').mockImplementation(()=>[{width:100,height:44}] as unknown as DOMRectList);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1','');if(init.method&&init.method!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'Unexpected write'}},{status:409});}reads.push(path);if(path==='/tool-profiles')return Response.json({items:[],total:0});if(path==='/release-workers')return Response.json({items:[{id:'identity-one',project_id:project,worker_id:worker,status:'active',version:1,expires_at:'2026-11-03T00:00:00Z'}]});if(path==='/project-execution/runners')return Response.json({items:[],as_of:at});if(path==='/adapter-catalog')return Response.json({project_id:project,as_of:at,items:[adapter]});if(path==='/tool-discovery')return Response.json({project_id:project,worker_id:worker,state:'unreported',as_of:at,items:[]});return Response.json({error:{code:'unavailable',message:'No fixture'}},{status:503});}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<12;i++)await Promise.resolve();await new Promise(resolve=>setTimeout(resolve,25));});}
function Workspace({panel='',initialSelected=''}:{panel?:string;initialSelected?:string}){const[selected,onSelect]=useState(initialSelected);return <ApiViewProvider session={session}><ToolWorkspace project={project} session={session} selected={selected} onSelect={onSelect} refresh={0} panel={panel}/></ApiViewProvider>;}
async function draw(panel?:string,initialSelected?:string){await act(async()=>root.render(<Workspace panel={panel} initialSelected={initialSelected}/>));await settle();}
async function click(selector:string){const button=document.querySelector<HTMLElement>(selector)!;expect(button,selector).not.toBeNull();await act(async()=>{button.focus();button.click();});await settle();}
async function choose(selector:string,value:string){const select=document.querySelector<HTMLSelectElement>(selector)!;expect(select,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,value);select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}

it('T206 puts one Add action outside the list and keeps configuration order and optional setup lazy in both languages',async()=>{
 await draw();const add=host.querySelector<HTMLButtonElement>('[data-tools-add]')!;expect(add).not.toBeNull();expect(add.textContent).toContain('Add tool');expect(add.closest('.tool-list')).toBeNull();expect(host.querySelectorAll('button.primary')).toHaveLength(1);expect(host.querySelector('[data-tool-workspace-more]')?.getAttribute('open')).toBeNull();expect(reads.filter(path=>path==='/adapter-catalog')).toHaveLength(0);
 const list=host.querySelector('[data-tool-workspace-configurations]')!,workbuddy=host.querySelector('.workbuddy-connections')!,more=host.querySelector('[data-tool-workspace-more]')!;expect(list.compareDocumentPosition(workbuddy)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();expect(list.compareDocumentPosition(more)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();expect(workbuddy.closest('[data-tool-workspace-more]')).toBeNull();await act(async()=>setLocale('zh-CN',{persist:false}));expect(add.textContent).toContain('添加工具');await click('[data-tools-add]');expect(document.querySelector('[data-quick-tool]')).not.toBeNull();expect(writes).toHaveLength(0);
});

it('T206 expands the original guide from Quick and keeps its exact node focus after the dialog closes',async()=>{
 await draw();await click('[data-tools-add]');await choose('[data-quick-tool]',adapter.id);await click('[data-quick-advanced]');expect(document.querySelector('[role=dialog]')).toBeNull();const more=host.querySelector<HTMLDetailsElement>('[data-tool-workspace-more]')!;expect(more.open).toBe(true);const remote=host.querySelector<HTMLDetailsElement>('[data-remote-runner-setup]')!;expect(remote.open).toBe(true);expect(host.querySelector<HTMLInputElement>('[name=remote-runner-worker]')?.value).toBe(worker);const guideSummary=host.querySelector<HTMLElement>('.tool-cli-setup>summary')!;expect(document.activeElement).toBe(guideSummary);expect(document.activeElement).not.toBe(host.querySelector('[data-tools-add]'));expect(writes).toHaveLength(0);
});

it('T206 leaves the original uncertain operation visible while advanced setup is collapsed and blocks new configuration identities',async()=>{
 sessionStorage.setItem('suma-tool-recovery-v1:team-one:user-one:project-one:profiles',JSON.stringify({path:'/release-workers',method:'POST',body:{project_id:project,worker_id:worker,idempotency_key:'original-operation'},idempotency_key:'original-operation'}));await draw();const notice=host.querySelector('.tool-recovery')!;expect(notice).not.toBeNull();expect(notice.closest('details')).toBeNull();expect(notice.textContent).toContain('original-operation');expect(host.querySelector<HTMLButtonElement>('[data-tools-add]')?.disabled).toBe(true);expect(host.querySelector<HTMLDetailsElement>('[data-tool-workspace-more]')?.open).toBe(false);expect(writes).toHaveLength(0);expect(sessionStorage.getItem('suma-tool-recovery-v1:team-one:user-one:project-one:profiles')).toContain('original-operation');
});

it('returns to the exact historical attempt through the existing navigation guard',async()=>{
 await draw('return-run:run-one/map%2Fchild/2');
 const back=host.querySelector<HTMLAnchorElement>('[data-tool-return-run]')!;
 expect(back).not.toBeNull();
 expect(back.textContent).toContain('attempt 2');
 expect(Object.fromEntries(new URL(back.href).searchParams)).toEqual({project,view:'development',sub:'runs',run:'run-one',node:'map/child',attempt:'2'});
 const guard=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener('suma:navigate',guard,{once:true});
 expect(back.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}))).toBe(false);
 expect(guard).toHaveBeenCalledOnce();
 expect(writes).toHaveLength(0);
 await draw('return-run:run-one/map%2Fchild/02');
 expect(host.querySelector('[data-tool-return-run]')).toBeNull();
});

it.each(['exact','foreign project','foreign profile'] as const)('opens the run diagnostic handoff with an %s profile response',async(scope)=>{
 const fetchMock=vi.mocked(fetch),previous=fetchMock.getMockImplementation()!;
 fetchMock.mockImplementation((input,init)=>{
  if(new URL(String(input),'http://local.test').pathname!=='/api/v1/tool-profiles/profile-one')return previous(input,init);
  return Promise.resolve(Response.json({id:scope==='foreign profile'?'other-profile':'profile-one',project_id:scope==='foreign project'?'other-project':project,
   worker_id:worker,name:'Independent test tool',state:'paused',version:1,available:false,reason:'profile_paused',enabled_by:'',created_at:at,updated_at:at,
   profile:{adapter:'command',protocol:'text-v1',command_id:'verify-tool',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},
  }));
 });
 await draw('return-run:run-one/check/2','profile-one');
 const diagnostics=host.querySelector<HTMLElement>('[data-tool-run-diagnostics]')!;
 expect(document.activeElement).toBe(diagnostics);
 expect(diagnostics.querySelector('[data-tool-run-handoff] [data-tool-return-run]')?.textContent).toContain('attempt 2');
 expect(host.querySelectorAll('[data-tool-return-run]')).toHaveLength(1);
 if(scope==='exact'){
  expect(diagnostics.querySelector('[data-tool-doctor-card]')).not.toBeNull();
  const actions=host.querySelector('[data-tool-profile-actions]')!;
  expect(diagnostics.compareDocumentPosition(actions)&Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
 }else{
  expect(diagnostics.querySelector('[role="alert"] button')).not.toBeNull();
  expect(host.querySelector('[data-tool-profile-actions]')).toBeNull();
  expect(diagnostics.textContent).not.toContain('Independent test tool');
 }
 expect(writes).toHaveLength(0);
});
