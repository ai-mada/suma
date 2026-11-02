// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import * as ApiContext from './api-context';
import {api,type Session} from './api';
import {setLocale} from './i18n';
import {ToolWorkspace} from './ToolWorkspace';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import type {ToolAction} from './tool-hooks';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';

const project='a'.repeat(32),pairID='c'.repeat(32),workerID='remote-'+pairID,stamp='2026-11-01T04:00:00Z';
const session:Session={user:{id:'b'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'d'.repeat(32),name:'Organization'},projects:[]};
const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const waiting={id:pairID,project_id:project,version:1,state:'waiting',control_url:'https://control.example.test/suma',created_at:stamp,expires_at:'2026-11-01T04:10:00Z',can_approve:false};
const proposed={...waiting,version:2,state:'proposed',device_name:'Target build machine',credential_sha256:'e'.repeat(64),can_approve:true};
const approved={...proposed,version:3,state:'approved',can_approve:false,worker_id:workerID,credential_expires_at:'2026-11-02T04:00:00Z'};
const inventory=(worker:string)=>({project_id:project,worker_id:worker,state:'fresh',as_of:stamp,observed_at:stamp,expires_at:'2026-11-01T04:01:00Z',items:[]});
const identity=(ids:string[])=>({path:'/release-workers',loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn(async()=>{})}) as unknown as IdentityRead;
let root:Root,host:HTMLDivElement;
async function settle(){await act(async()=>{for(let i=0;i<28;i++)await Promise.resolve();});}
async function click(selector:string){const button=document.querySelector<HTMLElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}
async function change(selector:string,value:string){const select=document.querySelector<HTMLSelectElement>(selector);expect(select,selector).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,value);select!.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
beforeEach(async()=>{await setLocale('en-US',{persist:false});sessionStorage.clear();vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();document.body.innerHTML='';sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});

function Workspace(){const[selected,onSelect]=useState('');return <ApiViewProvider session={session}><ToolWorkspace project={project} session={session} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}
function ContextWorkspace({identity,activeProject}:{identity:Session;activeProject:string}){const[selected,onSelect]=useState('');return <ApiViewProvider session={identity}><ToolWorkspace project={activeProject} session={identity} selected={selected} onSelect={onSelect} refresh={0} panel=""/></ApiViewProvider>;}

it('T205 opens an existing node detection entry and returns to its explicit opt-in guide without credentials or a scan write',async()=>{
 const writes:string[]=[];vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1','');if(init.method&&init.method!=='GET')writes.push(path);if(path==='/tool-profiles')return Response.json({project_id:project,state:'active',query:'',items:[],has_more:false,as_of:stamp});if(path==='/release-workers')return Response.json({items:[{id:'existing-node',project_id:project,worker_id:workerID,status:'active',version:1,expires_at:'2026-11-02T04:00:00Z'}]});if(path==='/project-execution/runners')return Response.json({items:[],as_of:stamp});if(path==='/adapter-catalog')return Response.json({project_id:project,as_of:stamp,items:[adapter]});if(path==='/tool-installation-scans')return Response.json({project_id:project,worker_id:workerID,as_of:stamp,maintenance:{state:'unknown',checked_at:stamp},items:[]});return Response.json({items:[]});}));
 await act(async()=>root.render(<Workspace/>));await settle();await click('[data-tool-worker-scan="'+workerID+'"]');expect(document.querySelector('[data-remote-tool-scan]')).not.toBeNull();await click('[data-remote-tool-scan-guide]');expect(document.querySelector('[role=dialog]')).toBeNull();expect(host.querySelector<HTMLInputElement>('[name=remote-runner-worker]')?.value).toBe(workerID);expect(host.querySelector<HTMLInputElement>('[data-remote-runner-allow-installed-discovery]')?.checked).toBe(false);expect(writes).toHaveLength(0);
});

it('locks the original workspace while approving and continues with the exact paired node without a token dialog or registration',async()=>{
 let pair:typeof waiting|typeof proposed|typeof approved=waiting,releaseApproval!:(response:Response)=>void;const writes:{path:string;body:Record<string,unknown>}[]=[],discoveryWorkers:string[]=[];
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1',''),method=init.method||'GET',body=init.body?JSON.parse(String(init.body)):undefined;
  if(method==='GET'){
   if(path==='/tool-profiles')return Response.json({items:[],total:0});
   if(path==='/release-workers')return Response.json({items:['other-node',...(pair.state==='approved'?[workerID]:[])].map((worker_id,i)=>({id:'worker-'+i,project_id:project,worker_id,status:'active',version:1,expires_at:'2026-11-02T04:00:00Z'}))});
   if(path==='/project-execution/runners')return Response.json({items:[],as_of:stamp});
   if(path==='/adapter-catalog')return Response.json({project_id:project,as_of:stamp,items:[adapter]});
   if(path==='/runner-pairings/'+pairID)return Response.json(pair);
   if(path==='/tool-discovery'){const target=url.searchParams.get('worker_id')!;discoveryWorkers.push(target);return Response.json(inventory(target));}
   return Response.json({items:[]});
  }
  writes.push({path,body});
  if(path==='/runner-pairings')return Response.json(waiting,{status:201});
  if(path==='/runner-pairings/'+pairID+'/commands')return new Promise<Response>(resolve=>{releaseApproval=resolve;});
  return Response.json({error:{code:'unexpected_write',message:'Unexpected fixture write'}},{status:400});
 }));
 await act(async()=>root.render(<Workspace/>));await settle();
 await click('.tool-workers .tool-section-heading button');await click('[data-worker-mode="pairing"]');await click('[data-runner-pairing-create]');expect(writes).toHaveLength(1);
 pair=proposed;await click('[data-runner-pairing-refresh]');await click('[data-runner-pairing-confirm]');await click('[data-runner-pairing-approve]');
 expect(writes).toHaveLength(2);expect(writes[1].body).toMatchObject({project_id:project,expected_version:2,action:'approve'});
 expect(document.querySelector<HTMLButtonElement>('[data-worker-mode="manual"]')?.disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('.tool-workers .tool-section-heading button')?.disabled).toBe(true);
 await click('[role=dialog] .dialog-heading .icon-button');expect(document.querySelector('[data-runner-pairing-card]')).not.toBeNull();
 pair=approved;await act(async()=>releaseApproval(Response.json(approved)));await settle();await click('[data-runner-pairing-continue]');
 expect(document.querySelector('.tool-token')).toBeNull();await change('[data-quick-tool]',adapter.id);expect(discoveryWorkers).toEqual([workerID]);
 expect(writes.map(write=>write.path)).toEqual(['/runner-pairings','/runner-pairings/'+pairID+'/commands']);expect(host.querySelector('[data-tool-receipt]')).toBeNull();
});

it('does not substitute the only other node when the approved preferred node is absent, but permits an explicit alternate choice',async()=>{
 const discoveryWorkers:string[]=[],run=vi.fn(),props:ToolQuickConnectProps={project,preferredWorkerID:workerID,workers:identity(['other-node']),sharedWorkers:identity([]) as unknown as SharedIdentityRead,action:{run,busy:false,pending:undefined,conflict:false} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1','');if(path==='/adapter-catalog')return Response.json({project_id:project,as_of:stamp,items:[adapter]});if(path==='/tool-discovery'){const worker=url.searchParams.get('worker_id')!;discoveryWorkers.push(worker);return Response.json(inventory(worker));}return Response.json({items:[]});}));
 await act(async()=>root.render(<ApiViewProvider session={session}><ToolQuickConnect {...props}/></ApiViewProvider>));await settle();await change('[data-quick-tool]',adapter.id);
 expect(discoveryWorkers).toEqual([]);expect(host.querySelector('[data-quick-preferred-worker-missing]')).not.toBeNull();expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('');expect(run).not.toHaveBeenCalled();
 await change('[data-quick-worker]','other-node');expect(discoveryWorkers).toEqual(['other-node']);expect(host.querySelector('[data-quick-preferred-worker-missing]')).toBeNull();expect(run).not.toHaveBeenCalled();
});

it('continues an explicitly read approved node into its fixed preparation guide and discards that target on caller, actor or project changes without writing',async()=>{
 let activeSession=session,activeProject=project,caller=ApiContext.bindApiView(activeSession,api);const writes:string[]=[],discoveryWorkers:string[]=[];
 const qoder={...adapter,id:'qoder-acp',product:'Qoder',adapter:'qoder',protocol:'acp-stdio-v1'};
 vi.spyOn(ApiContext,'useApi').mockImplementation(()=>caller);
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1',''),targetProject=url.searchParams.get('project_id')||project;
  if((init.method||'GET')!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'Preparation is explicit and read-only'}},{status:400});}
  if(path==='/tool-profiles')return Response.json({items:[],total:0});
  if(path==='/release-workers')return Response.json({items:['other-node',...(targetProject===project?[workerID]:[])].map((worker_id,i)=>({id:'worker-'+i,project_id:targetProject,worker_id,status:'active',version:1,expires_at:'2026-11-02T04:00:00Z'}))});
  if(path==='/project-execution/runners')return Response.json({items:[],as_of:stamp});
  if(path==='/adapter-catalog')return Response.json({project_id:targetProject,as_of:stamp,items:[adapter,qoder]});
  if(path==='/runner-pairings')return Response.json({project_id:targetProject,status:url.searchParams.get('status')||'all',q:url.searchParams.get('q')||'',items:targetProject===project?[approved]:[],total:targetProject===project?1:0,limit:Number(url.searchParams.get('limit')||20),next_cursor:'',as_of:stamp});
  if(path==='/runner-pairings/'+pairID)return targetProject===project?Response.json(approved):Response.json({error:{code:'not_found',message:'Different project'}},{status:404});
  if(path==='/tool-discovery'){const worker=url.searchParams.get('worker_id')!;discoveryWorkers.push(worker);return Response.json(inventory(worker));}
  return Response.json({items:[]});
 }));
 const render=async()=>{await act(async()=>root.render(<ContextWorkspace identity={activeSession} activeProject={activeProject}/>));await settle();};
 const seed=()=>sessionStorage.setItem('suma-runner-pairing:'+JSON.stringify([activeSession.user.id,activeSession.organization.id,activeProject]),pairID);
 const prepare=async()=>{
  seed();await click('.tool-workers .tool-section-heading button');await click('[data-worker-mode="pairing"]');await click('[data-runner-pairing-continue]');await change('[data-quick-tool]',adapter.id);
  expect(document.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(workerID);
  await change('[data-quick-worker]','other-node');expect(document.querySelector('[data-quick-prepare-runner]')).toBeNull();
  await change('[data-quick-worker]',workerID);await click('[data-quick-prepare-runner]');
  expect(document.querySelector('[role="dialog"]')).toBeNull();expect(host.querySelector<HTMLDetailsElement>('.tool-cli-setup')?.open).toBe(true);expect(host.querySelector<HTMLDetailsElement>('[data-remote-runner-setup]')?.open).toBe(true);
  const fields={control:approved.control_url,project,worker:workerID,pairingID:pairID};
  for(const [key,value] of Object.entries(fields)){const field=host.querySelector<HTMLInputElement>('[name="remote-runner-'+key+'"]');expect(field?.value,key).toBe(value);expect(field?.readOnly,key).toBe(true);}
   expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-mode"]')?.value).toBe('paired');expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-localTool"]')?.value).toBe(adapter.id);expect(writes).toEqual([]);
  await click('[data-tools-return-quick]');await change('[data-quick-tool]',qoder.id);await change('[data-quick-worker]','other-node');await click('[data-quick-advanced]');
  expect(host.querySelector<HTMLInputElement>('[name="remote-runner-worker"]')?.value).toBe('other-node');expect(host.querySelector<HTMLInputElement>('[name="remote-runner-worker"]')?.readOnly).toBe(true);
  expect(host.querySelector<HTMLInputElement>('[name="remote-runner-control"]')?.value).toBe('');expect(host.querySelector('[name="remote-runner-pairingID"]')).toBeNull();
   expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-mode"]')?.value).toBe('identity');expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-localTool"]')?.value).toBe(qoder.id);expect(host.querySelector('[data-remote-runner-command="prepare"]')).toBeNull();expect(writes).toEqual([]);
 };
 const noOldTarget=()=>{for(const key of ['control','worker','pairingID'])expect(host.querySelector<HTMLInputElement>('[name="remote-runner-'+key+'"]')?.value||'').toBe('');expect(document.querySelector('[data-quick-prepare-runner]')).toBeNull();};
 // Returning to Quick reads the retained Codex target before the explicit Qoder/node choice.
 seed();await render();await prepare();expect(discoveryWorkers).toEqual([workerID,'other-node',workerID,workerID,'other-node']);
  const ordinaryGuide=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent?.trim()==='View setup guide');expect(ordinaryGuide).toBeDefined();await act(async()=>ordinaryGuide!.click());await settle();
  await click('[data-tools-return-quick]');await change('[data-quick-tool]',adapter.id);await change('[data-quick-worker]',workerID);await click('[data-quick-prepare-runner]');
  expect(host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')?.value).toBe('codex');expect(host.querySelector<HTMLSelectElement>('[name="remote-runner-localTool"]')?.value).toBe(adapter.id);expect(host.querySelector<HTMLInputElement>('[name="remote-runner-worker"]')?.value).toBe(workerID);expect(writes).toEqual([]);
 caller=ApiContext.bindApiView(activeSession,api);await render();noOldTarget();
 await prepare();activeSession={...session,user:{...session.user,id:'e'.repeat(32)}};caller=ApiContext.bindApiView(activeSession,api);await render();noOldTarget();
 await prepare();activeProject='f'.repeat(32);caller=ApiContext.bindApiView(activeSession,api);await render();noOldTarget();
 expect(writes).toEqual([]);
});

it('T212 opens a bounded Pool locator only through the current exact shared worker and catalog without scanning or registering',async()=>{
 const pool='pool-one',runner='runner-one',sharedWorker='pool-'+runner,writes:string[]=[],discovery:string[]=[];
 let sharedPool=pool,refresh=0;
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{
  const url=new URL(String(input),'http://local.test'),path=url.pathname.replace('/api/v1','');
  if((init.method||'GET')!=='GET'){writes.push(path);return Response.json({error:{code:'unexpected_write',message:'No business write'}},{status:400});}
  if(path==='/tool-profiles')return Response.json({items:[],total:0});
  if(path==='/release-workers')return Response.json({items:[]});
  if(path==='/project-execution/runners')return Response.json({as_of:stamp,items:[{id:runner,pool_id:sharedPool,worker_id:sharedWorker,name:'Shared machine',status:'active',credential_expires_at:'2026-11-02T04:00:00Z'}]});
  if(path==='/adapter-catalog')return Response.json({project_id:project,as_of:stamp,items:[adapter]});
  if(path==='/tool-discovery'){discovery.push(url.searchParams.get('worker_id')!);return Response.json(inventory(url.searchParams.get('worker_id')!));}
  return Response.json({items:[]});
 }));
 const draw=async(panel:string)=>{await act(async()=>root.render(<ApiViewProvider session={session}><ToolWorkspace project={project} session={session} selected="" onSelect={()=>{}} refresh={refresh} panel={panel}/></ApiViewProvider>));await settle();};
 await draw('pool-tool:'+pool+':'+runner);
 expect(document.querySelector('[data-quick-tool]')).not.toBeNull();
 expect(discovery).toEqual([]);expect(writes).toEqual([]);
 await change('[data-quick-tool]',adapter.id);
 expect(document.querySelector('.tool-quick-node')?.textContent).toContain(sharedWorker);
 expect(discovery).toEqual([sharedWorker]);
 await click('[role=dialog] .dialog-heading .icon-button');
 sharedPool='wrong-pool';refresh++;
 await draw('pool-tool:'+pool+':another-runner');
 expect(document.querySelector('[data-quick-tool]')).toBeNull();expect(writes).toEqual([]);
 await draw('pool-tool:'+pool+':'+runner+':extra');
 expect(document.querySelector('[data-quick-tool]')).toBeNull();expect(discovery).toEqual([sharedWorker]);
});
