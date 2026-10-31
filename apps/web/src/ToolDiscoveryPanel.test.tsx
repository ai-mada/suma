// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {ApiViewProvider,useApi} from './api-context';
import {setLocale,formatDateTime} from './i18n';
import {ToolDiscoveryPanel, type ToolDiscoveryPanelProps} from './ToolDiscoveryPanel';
import {parseToolDiscovery} from './tool-discovery-model';

const project='project-a',worker='worker-a';
const item={profile:{command_id:'check',adapter:'command',protocol:'text-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},available:true,reason:''};
const session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
function report(currentWorker=worker,currentProject=project){return {project_id:currentProject,worker_id:currentWorker,state:'fresh',as_of:'2026-10-29T08:00:20Z',observed_at:'2026-10-29T08:00:00Z',expires_at:'2026-10-29T08:01:00Z',items:[structuredClone(item)]};}
const adoptedSource=(currentWorker=worker,currentProject=project)=>expect.objectContaining({project:currentProject,worker:currentWorker,apiCaller:expect.any(Function),observedAt:report().observed_at,expiresAt:report().expires_at,deadline:expect.any(Number)});
let root:Root,host:HTMLDivElement,adopt:ReturnType<typeof vi.fn>,requests:{path:string;init:RequestInit}[],reply:(path:string,init:RequestInit)=>Promise<Response>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);adopt=vi.fn();requests=[];
 reply=async()=>Response.json(report());
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit)=>{const path=String(input).replace('/api/v1','');requests.push({path,init});return reply(path,init);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<16;i++)await Promise.resolve();});}
async function render(overrides:Partial<ToolDiscoveryPanelProps>={},identity=session){await act(async()=>root.render(<ApiViewProvider session={identity}><ToolDiscoveryPanel project={project} worker={worker} authorized disabled={false} autoAdopt onAdopt={adopt} {...overrides}/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button,selector).not.toBeNull();await act(async()=>button!.click());await settle();}

it('parses only exact scoped, self-consistent current reports and reuses the inventory validator',()=>{
 const parsed=parseToolDiscovery(report(),project,worker);
 expect(parsed.state).toBe('fresh');expect(parsed.items).toEqual([item]);expect(parsed.remainingMs).toBe(40000);
 expect(parseToolDiscovery({...report(),state:'stale',as_of:'2026-10-29T08:01:00Z',items:[]},project,worker).state).toBe('stale');
 expect(parseToolDiscovery({project_id:project,worker_id:worker,state:'unreported',as_of:'2026-10-29T08:00:20Z',items:[]},project,worker).state).toBe('unreported');
 const corruptions=[
  {project_id:'other'}, {worker_id:'other'}, {state:'healthy'}, {as_of:'2026-02-30T08:00:20Z'}, {as_of:null},
  {observed_at:'2026-10-29T08:00:21Z'}, {expires_at:'2026-10-29T08:00:20Z'}, {expires_at:'2026-10-29T08:01:01Z'},
  {token:'must-not-leak'}, {items:null}, {items:[{...item,raw_args:'must-not-leak'}]},
  {items:[{...item,profile:{...item.profile,profile_sha256:'missing'}}]}, {items:[item,item]},
 ];
 for(const change of corruptions)expect(()=>parseToolDiscovery({...report(),...change},project,worker)).toThrow();
 expect(()=>parseToolDiscovery({...report(),state:'stale'},project,worker)).toThrow();
 expect(()=>parseToolDiscovery({...report(),state:'unreported',items:[]},project,worker)).toThrow();
});

it('automatically reads the selected authorized identity and adopts each fresh report only once without writing',async()=>{
 await render();expect(requests).toHaveLength(1);expect(requests[0].path).toBe('/tool-discovery?project_id=project-a&worker_id=worker-a');expect(requests[0].init.method).toBe('GET');expect(requests[0].init.body).toBeUndefined();
 expect(adopt).toHaveBeenCalledExactlyOnceWith([item],adoptedSource());expect(host.querySelector('[data-tool-discovery-state="fresh"]')).not.toBeNull();
 await render({onAdopt:items=>adopt(items)});await act(async()=>setLocale('zh-CN',{persist:false}));expect(requests).toHaveLength(1);expect(adopt).toHaveBeenCalledTimes(1);
 await click('[data-tool-discovery-refresh]');expect(requests).toHaveLength(2);expect(adopt).toHaveBeenCalledTimes(1);
});

it('renders actual tool counts and report time in both languages',async()=>{
 await render();expect(host.textContent).toContain('Tools reported: 1 · available to register: 1');
 const time=()=>formatDateTime(report().observed_at,{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit'});
 expect(host.querySelector('.tool-discovery-observed')?.textContent).toContain(time());
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('已检测到 1 项工具，1 项可用于登记');expect(host.querySelector('.tool-discovery-observed')?.textContent).toContain(time());expect(host.textContent).not.toMatch(/\{(?:count|available|time)\}/);
});

it('does not read or adopt until both a selected identity and its current authorization are present',async()=>{
 await render({worker:''});await render({authorized:false});expect(requests).toHaveLength(0);expect(adopt).not.toHaveBeenCalled();expect(host.querySelector('[data-tool-discovery-adopt]')).toBeNull();
 await render();expect(requests).toHaveLength(1);expect(adopt).toHaveBeenCalledTimes(1);
});

it('preserves manual drafts until explicit adoption and never submits a surrounding form',async()=>{
 let submits=0;
 await act(async()=>root.render(<form onSubmit={event=>{event.preventDefault();submits++;}}><ToolDiscoveryPanel project={project} worker={worker} authorized disabled={false} autoAdopt={false} onAdopt={adopt}/></form>));await settle();
 expect(adopt).not.toHaveBeenCalled();expect(host.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')?.type).toBe('button');
 await click('[data-tool-discovery-adopt]');expect(adopt).toHaveBeenCalledExactlyOnceWith([item],adoptedSource());expect(submits).toBe(0);
 await click('[data-tool-discovery-adopt]');expect(adopt).toHaveBeenCalledTimes(1);
});

it('allows explicit re-adoption of the same report after a manual draft change without resetting automatic deduplication',async()=>{
 await render({manualRevision:0});expect(adopt).toHaveBeenCalledExactlyOnceWith([item],adoptedSource());
 await render({manualRevision:1});expect(adopt).toHaveBeenCalledTimes(1);expect(host.querySelector('.tool-discovery-adopted')).toBeNull();
 await render({autoAdopt:false,manualRevision:1});expect(requests).toHaveLength(1);expect(adopt).toHaveBeenCalledTimes(1);
 expect(host.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')?.disabled).toBe(false);
 await click('[data-tool-discovery-adopt]');expect(adopt).toHaveBeenNthCalledWith(2,[item],adoptedSource());expect(adopt).toHaveBeenCalledTimes(2);
 expect(host.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')?.disabled).toBe(true);
 await click('[data-tool-discovery-adopt]');expect(adopt).toHaveBeenCalledTimes(2);
 await render({autoAdopt:true,manualRevision:1});await click('[data-tool-discovery-refresh]');expect(adopt).toHaveBeenCalledTimes(2);
});

it('uses the current manual-draft and disabled flags when a response arrives',async()=>{
 let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await render();await render({autoAdopt:false,disabled:true});await act(async()=>finish(Response.json(report())));await settle();
 expect(adopt).not.toHaveBeenCalled();expect(host.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')?.disabled).toBe(true);
 await render({autoAdopt:false,disabled:false});expect(adopt).not.toHaveBeenCalled();await click('[data-tool-discovery-adopt]');expect(adopt).toHaveBeenCalledTimes(1);
});

it('aborts a previous identity read and ignores its late response after a worker switch',async()=>{
 const finishes=new Map<string,(value:Response)=>void>();reply=path=>new Promise(resolve=>{finishes.set(new URL(path,'https://example.test').searchParams.get('worker_id')!,resolve);});
 await render();await render({worker:'worker-b'});expect(requests[0].init.signal?.aborted).toBe(true);expect(host.querySelector('[data-tool-discovery-state="fresh"]')).toBeNull();
 await act(async()=>finishes.get(worker)!(Response.json(report())));await settle();expect(adopt).not.toHaveBeenCalled();
 await act(async()=>finishes.get('worker-b')!(Response.json(report('worker-b'))));await settle();expect(adopt).toHaveBeenCalledExactlyOnceWith([item],adoptedSource('worker-b'));
});

it('isolates responses across project, user and organization changes',async()=>{
 reply=async path=>{const params=new URL(path,'https://example.test').searchParams;return Response.json(report(params.get('worker_id')!,params.get('project_id')!));};
 await render({autoAdopt:false});await render({project:'project-b',autoAdopt:false});expect(requests).toHaveLength(2);expect(adopt).not.toHaveBeenCalled();
 await render({project:'project-b',autoAdopt:false},{...session,user:{...session.user,id:'user-b'}});expect(requests).toHaveLength(3);
 await render({project:'project-b',autoAdopt:false},{...session,organization:{...session.organization,id:'org-b'}});expect(requests).toHaveLength(4);expect(requests.every(request=>request.init.method==='GET')).toBe(true);
});

it('clears observations immediately when authorization is lost and ignores the old response',async()=>{
 let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await render();await render({authorized:false});expect(requests[0].init.signal?.aborted).toBe(true);
 await act(async()=>finish(Response.json(report())));await settle();expect(adopt).not.toHaveBeenCalled();expect(host.querySelector('[data-tool-discovery-state="fresh"]')).toBeNull();
});

it('clears previous results during refresh and allows a failed read to be retried explicitly',async()=>{
 await render({autoAdopt:false});let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await click('[data-tool-discovery-refresh]');expect(host.querySelector('[data-tool-discovery-state="fresh"]')).toBeNull();expect(host.querySelector('[data-tool-discovery-adopt]')).toBeNull();
 await act(async()=>finish(Response.json({error:{code:'forbidden',message:'Current access denied'}},{status:403})));await settle();
 expect(host.querySelector('[role="alert"]')?.textContent).toContain('Current access denied');expect(adopt).not.toHaveBeenCalled();expect(requests).toHaveLength(2);
 reply=async()=>Response.json(report());await click('[data-tool-discovery-retry]');expect(requests).toHaveLength(3);expect(host.querySelector('[data-tool-discovery-state="fresh"]')).not.toBeNull();
});

it('rejects malformed and cross-scope responses without rendering or adopting their tools',async()=>{
 reply=async()=>Response.json({...report(),worker_id:'other-worker'});await render();expect(host.querySelector('[role="alert"]')).not.toBeNull();expect(adopt).not.toHaveBeenCalled();
 reply=async()=>Response.json({...report(),items:[{...item,raw_args:'must-not-render'}]});await click('[data-tool-discovery-retry]');expect(host.textContent).not.toContain('must-not-render');expect(adopt).not.toHaveBeenCalled();
});

it('shows stale and unreported next steps without adopting historical results or retrying in a loop',async()=>{
 reply=async()=>Response.json({...report(),state:'stale',as_of:'2026-10-29T08:01:00Z',items:[]});await render();expect(host.querySelector('[data-tool-discovery-state="stale"]')).not.toBeNull();expect(host.querySelector('code')?.textContent).toContain('suma-runner --describe-tools');expect(adopt).not.toHaveBeenCalled();
 reply=async()=>Response.json({project_id:project,worker_id:worker,state:'unreported',as_of:'2026-10-29T08:01:00Z',items:[]});await click('[data-tool-discovery-refresh]');expect(host.querySelector('[data-tool-discovery-state="unreported"]')).not.toBeNull();expect(requests).toHaveLength(2);expect(adopt).not.toHaveBeenCalled();
});

it('separates a fresh empty inventory from an unreported node',async()=>{
 reply=async()=>Response.json({...report(),items:[]});await render({autoAdopt:false});expect(host.querySelector('[data-tool-discovery-state="fresh"]')).not.toBeNull();expect(host.querySelector('[data-tool-discovery-empty]')).not.toBeNull();expect(host.querySelector('[data-tool-discovery-adopt]')).toBeNull();expect(adopt).not.toHaveBeenCalled();
});

it('expires fresh observations using the server-relative lifetime without scheduling another read',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});await render({autoAdopt:false});expect(host.querySelector<HTMLButtonElement>('[data-tool-discovery-adopt]')?.disabled).toBe(false);
 await act(async()=>vi.advanceTimersByTimeAsync(40001));expect(host.querySelector('[data-tool-discovery-state="expired"]')).not.toBeNull();
 await click('[data-tool-discovery-adopt]');expect(adopt).not.toHaveBeenCalled();expect(requests).toHaveLength(1);
});

it('subtracts request time from the remaining database freshness interval',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});let finish!:(value:Response)=>void;reply=()=>new Promise(resolve=>{finish=resolve;});
 await render({autoAdopt:false});await act(async()=>vi.advanceTimersByTimeAsync(2000));await act(async()=>finish(Response.json({...report(),as_of:'2026-10-29T08:00:59Z'})));await settle();
 expect(host.querySelector('[data-tool-discovery-state="expired"]')).not.toBeNull();await click('[data-tool-discovery-adopt]');expect(adopt).not.toHaveBeenCalled();
});

it('adopts the original scoped caller and conservative deadline without renewing freshness on later adoption',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});
 let currentCaller!:ReturnType<typeof useApi>,finish!:(value:Response)=>void;
 function Caller(){currentCaller=useApi();return null;}
 reply=()=>new Promise(resolve=>{finish=resolve;});
 async function draw(manualRevision=0,identity=session,readRevision=0){await act(async()=>root.render(<ApiViewProvider session={identity}><Caller/><ToolDiscoveryPanel project={project} worker={worker} authorized disabled={false} autoAdopt={false} manualRevision={manualRevision} readRevision={readRevision} onAdopt={adopt}/></ApiViewProvider>));await settle();}
 const started=performance.now();await draw();const originalCaller=currentCaller;
 await act(async()=>vi.advanceTimersByTimeAsync(2000));await act(async()=>finish(Response.json(report())));await settle();
 await act(async()=>vi.advanceTimersByTimeAsync(3000));await click('[data-tool-discovery-adopt]');
 expect(adopt).toHaveBeenCalledExactlyOnceWith([item],{project,worker,apiCaller:originalCaller,observedAt:report().observed_at,expiresAt:report().expires_at,deadline:started+40000});
 const source=adopt.mock.calls[0][1];expect(source.deadline-performance.now()).toBe(35000);
 await draw(1);await act(async()=>vi.advanceTimersByTimeAsync(1000));await click('[data-tool-discovery-adopt]');
 expect(adopt).toHaveBeenNthCalledWith(2,[item],source);expect(requests).toHaveLength(1);
 await draw(1,session,1);expect(requests).toHaveLength(2);await draw(1,session,1);expect(requests).toHaveLength(2);
 await act(async()=>finish(Response.json({...report(),as_of:'2026-10-29T08:00:26Z'})));await settle();expect(adopt).toHaveBeenCalledTimes(2);
 await draw(1,{...session,user:{...session.user,id:'user-b'}},1);expect(currentCaller).not.toBe(originalCaller);expect(source.apiCaller).toBe(originalCaller);expect(adopt).toHaveBeenCalledTimes(2);expect(requests).toHaveLength(3);
 await act(async()=>finish(Response.json({...report(),as_of:'2026-10-29T08:00:26Z'})));await settle();await click('[data-tool-discovery-adopt]');
 expect(adopt).toHaveBeenNthCalledWith(3,[item],{...source,apiCaller:currentCaller});expect(requests.every(request=>request.init.method==='GET')).toBe(true);
});
