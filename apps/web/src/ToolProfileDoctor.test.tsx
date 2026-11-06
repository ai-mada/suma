// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ToolProfile,ToolProfileDiagnostics} from '../../../packages/contracts/src';
import {ToolProfileDoctor} from './ToolProfileDoctor';
import {ApiViewProvider} from './api-context';
import type {Session} from './api';
import {setLocale} from './i18n';

const session:Session={user:{id:'u',name:'Owner',email:'owner@example.test'},organization:{id:'o',name:'Organization'},projects:[]};
const tool:ToolProfile={id:'tool',project_id:'p',worker_id:'runner',name:'Team tool',version:1,state:'enabled',profile:{command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false},enabled_by:'u',created_at:'2026-11-01T00:00:00Z',updated_at:'2026-11-01T00:00:00Z',available:false,reason:'Node credential needs review'};
const report=(source=tool):ToolProfileDiagnostics=>({tool:source,checked_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:00:12Z',checks:[{code:'authorization',status:'pass',reason:'',next_action:'none'},{code:'worker',status:'blocked',reason:source.reason,next_action:'open_worker'},{code:'inventory',status:'not_checked',reason:'Waiting for node',next_action:'refresh'},{code:'profile',status:'not_checked',reason:'Waiting for inventory',next_action:'refresh'}],next_action:'open_worker',maintenance_resolution:{profile_id:source.id,profile_version:source.version,worker_id:source.worker_id,...(source.state==='revoked'?{action:'none',reason:'profile_revoked'}:{action:'open_worker',reason:'maintenance_unknown'})}});
type Props=Parameters<typeof ToolProfileDoctor>[0];
let root:Root,host:HTMLDivElement;
async function settle(){await act(async()=>{for(let i=0;i<18;i++)await Promise.resolve();});}
async function render(props:Partial<Props>={},identity:Session=session){await act(async()=>root.render(<ApiViewProvider session={identity}><ToolProfileDoctor project="p" tool={tool} confirmed {...props}/></ApiViewProvider>));await settle();}
function state(){return host.querySelector('[data-tool-doctor-card]')?.getAttribute('data-tool-doctor-state');}
const action=()=>host.querySelector<HTMLButtonElement>('[data-tool-doctor-maintenance-action],[data-tool-doctor-action]');
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
function deferred(){let resolve!:(value:Response)=>void;const promise=new Promise<Response>(done=>{resolve=done;});return {promise,resolve};}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('T195 resolves each blocked check through the exact live destination while preserving the single primary and read-only recovery gates',async()=>{
 let now=0;vi.spyOn(performance,'now').mockImplementation(()=>now);
 const multiple=():ToolProfileDiagnostics=>({...report(),checks:[
  {code:'authorization',status:'blocked',reason:'Review the original approval',next_action:'review_profile'},
  {code:'worker',status:'blocked',reason:'Review this exact node',next_action:'open_worker'},
  {code:'inventory',status:'not_checked',reason:'Read current inventory evidence',next_action:'refresh'},
  {code:'profile',status:'blocked',reason:'The complete configuration differs',next_action:'register_profile'},
 ],next_action:'review_profile'});
 let value=multiple(),status=200;
 const fetcher=vi.fn(async(_input:string,_init:RequestInit)=>Response.json(status===200?value:{error:{code:'forbidden',message:'Current diagnostic access denied'}},{status}));
 vi.stubGlobal('fetch',fetcher);const onReview=vi.fn(),onWorker=vi.fn(),onRegister=vi.fn(),onSharedWorker=vi.fn();
 const destinations={onReview,onWorker,onRegister};
 await render(destinations);
 const details=host.querySelector<HTMLDetailsElement>('.tool-doctor-details');expect(details).not.toBeNull();expect(details!.open).toBe(false);details!.open=true;
 expect(host.querySelectorAll('[data-tool-doctor-check-action]')).toHaveLength(4);
 expect(host.querySelectorAll('[data-tool-doctor-card] .primary')).toHaveLength(1);
 expect(host.querySelector('[data-tool-doctor-action="review_profile"]')).not.toBeNull();
 await click('[data-tool-doctor-action="review_profile"]');await click('[data-tool-doctor-check-action="authorization"]');expect(onReview).toHaveBeenCalledTimes(2);for(const [source] of onReview.mock.calls)expect(source).toEqual(tool);
 await click('[data-tool-doctor-check-action="worker"]');expect(onWorker).toHaveBeenCalledExactlyOnceWith(tool.worker_id);
 await click('[data-tool-doctor-check-action="profile"]');expect(onRegister).toHaveBeenCalledExactlyOnceWith(tool);
 value={...multiple(),checks:multiple().checks.map(check=>check.code==='worker'?{...check,status:'pass',reason:''}:check)};
 await click('[data-tool-doctor-check-action="inventory"]');expect(fetcher).toHaveBeenCalledTimes(2);expect(host.querySelector('[data-tool-doctor-check-action="worker"]')).toBeNull();
 value=multiple();await click('[data-tool-doctor-check-action="inventory"]');
 await render({onReview,onRegister,onSharedWorker});expect(host.textContent).toContain('read-only');await click('[data-tool-doctor-check-action="worker"]');expect(onSharedWorker).toHaveBeenCalledExactlyOnceWith(tool.worker_id);expect(onWorker).toHaveBeenCalledOnce();
 await render({onReview});expect(host.querySelector('[data-tool-doctor-check-action="worker"]')).toBeNull();expect(host.querySelector('[data-tool-doctor-check-action="profile"]')).toBeNull();
 await render(destinations);now=13000;await click('[data-tool-doctor-check-action="worker"]');expect(onWorker).toHaveBeenCalledOnce();expect(state()).toBe('expired');expect(host.querySelector('[data-tool-doctor-check-action]')).toBeNull();
 await click('[data-tool-doctor-refresh]');expect(state()).toBe('ready');
 status=403;await click('[data-tool-doctor-check-action="inventory"]');expect(state()).toBe('error');expect(host.querySelector('[data-tool-doctor-check-action]')).toBeNull();expect(host.querySelector('[data-tool-profile-doctor-target]')).toBeNull();
 const reads=fetcher.mock.calls.length;await render({...destinations,pending:true});expect(state()).toBe('suspended');expect(host.querySelector('[data-tool-doctor-check-action]')).toBeNull();expect(fetcher).toHaveBeenCalledTimes(reads);
 status=200;value={...multiple(),tool:{...tool,worker_id:'different-worker',profile:{...tool.profile,profile_sha256:'c'.repeat(64)}},maintenance_resolution:{...multiple().maintenance_resolution,worker_id:'different-worker'}};
 await render(destinations);expect(state()).toBe('error');await click('[data-tool-doctor-refresh]');expect(state()).toBe('changed');expect(host.querySelector('[data-tool-doctor-check-action]')).toBeNull();expect(onRegister).toHaveBeenCalledOnce();
 expect(fetcher.mock.calls.every(([,init])=>init.method==='GET'&&init.body===undefined)).toBe(true);
});

it('T194 opens a separate explicit inspection for the exact fresh profile and keeps reading, expiry and refusal distinct',async()=>{
 let now=0;vi.spyOn(performance,'now').mockImplementation(()=>now);const onInspect=vi.fn(),onWorker=vi.fn();let status=200;
 let value:ToolProfileDiagnostics={...report(),maintenance_resolution:{profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'inspect',reason:'maintenance_supported'}};
 const fetcher=vi.fn(async(_input:string,_init:RequestInit)=>Response.json(status===200?value:{error:{code:'forbidden',message:'Forbidden'}},{status}));vi.stubGlobal('fetch',fetcher);
 await render({onInspect,onWorker});expect(state()).toBe('ready');expect(onInspect).not.toHaveBeenCalled();
 expect(host.querySelector('[data-tool-doctor-inspect]')).toBeNull();expect(host.querySelectorAll('[data-tool-doctor-card] .primary')).toHaveLength(1);
 await click('[data-tool-doctor-maintenance-action="inspect"]');expect(onInspect).toHaveBeenCalledExactlyOnceWith(tool);expect(fetcher).toHaveBeenCalledOnce();
 const steps:ToolProfileDiagnostics['maintenance_resolution'][]=[
  {profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'view_inspection',reason:'inspection_pending',inspection_id:'inspection:original/A'},
  {profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'view_inspection',reason:'inspection_claimed',inspection_id:'inspection:original/A'},
  {profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'recover_inspection',reason:'inspection_unknown',inspection_id:'inspection:original/A'},
  {profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'recover_inspection',reason:'recovery_expired',inspection_id:'inspection:original/A'},
  {profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'view_recovery',reason:'recovery_requested',inspection_id:'inspection:original/A',recovery_id:'recovery:original/A'},
  {profile_id:tool.id,profile_version:tool.version,worker_id:tool.worker_id,action:'view_recovery',reason:'recovery_proof_stored',inspection_id:'inspection:original/A',recovery_id:'recovery:original/A'},
 ];
 for(const step of steps){
  value={...report(),maintenance_resolution:step};await click('[data-tool-doctor-refresh]');expect(state()).toBe('ready');
  expect(host.querySelectorAll('[data-tool-doctor-card] .primary')).toHaveLength(1);
  await click('[data-tool-doctor-maintenance-action="'+step.action+'"]');
  expect(onInspect).toHaveBeenLastCalledWith(tool,{inspectionID:step.inspection_id,...(step.recovery_id?{recoveryID:step.recovery_id}:{})});
 }
 expect(onInspect).toHaveBeenCalledTimes(7);expect(onWorker).not.toHaveBeenCalled();
 const stale=host.querySelector<HTMLButtonElement>('[data-tool-doctor-maintenance-action]')!;now=13000;await act(async()=>stale.click());await settle();expect(state()).toBe('expired');expect(host.querySelector('[data-tool-doctor-maintenance-action]')).toBeNull();expect(onInspect).toHaveBeenCalledTimes(7);
 status=403;await click('[data-tool-doctor-refresh]');expect(host.querySelector('[data-tool-doctor-maintenance-action]')).toBeNull();expect(host.querySelector('[data-tool-profile-doctor-target]')).toBeNull();
 await render({onInspect,onWorker,pending:true});expect(state()).toBe('suspended');expect(host.querySelector('[data-tool-doctor-maintenance-action]')).toBeNull();expect(onInspect).toHaveBeenCalledTimes(7);
 expect(fetcher.mock.calls.every(([,init])=>init.method==='GET'&&init.body===undefined)).toBe(true);
});
it('T193 carries the exact original profile into configuration and offers only a supplied read-only shared-node destination',async()=>{
 const original={...tool,reason:'The reported configuration differs from this profile'};
 let current:ToolProfileDiagnostics={...report(original),checks:report(original).checks.map(check=>({...check,status:check.code==='profile'?'blocked' as const:'pass' as const,reason:check.code==='profile'?original.reason:'',next_action:check.code==='profile'?'register_profile':'none'})),next_action:'register_profile'};
 const onRegister=vi.fn(),onSharedWorker=vi.fn(),fetcher=vi.fn(async(_input:string,_init:RequestInit)=>Response.json(current));vi.stubGlobal('fetch',fetcher);
 await render({tool:original,onRegister});await click('[data-tool-doctor-action="register_profile"]');expect(onRegister).toHaveBeenCalledExactlyOnceWith(original);
 const basis=host.querySelector<HTMLDetailsElement>('[data-tool-profile-doctor-basis]');expect(basis).not.toBeNull();expect(basis!.open).toBe(false);expect(basis!.textContent).toContain(original.worker_id);expect(host.querySelectorAll('[data-tool-doctor-action]')).toHaveLength(1);
 const shared={...tool,id:'shared-tool',worker_id:'pool-shared-worker',name:'Original shared tool'};current=report(shared);
 await render({tool:shared,onSharedWorker});expect(host.textContent).toContain('read-only');await click('[data-tool-doctor-maintenance-action="open_worker"]');expect(onSharedWorker).toHaveBeenCalledExactlyOnceWith(shared.worker_id);expect(onRegister).toHaveBeenCalledOnce();
 fetcher.mockResolvedValueOnce(Response.json({error:{code:'forbidden',message:'Current diagnostic access denied'}},{status:403}));await click('[data-tool-doctor-refresh]');expect(host.querySelector('[data-tool-profile-doctor-target]')).toBeNull();
 const deniedRetry=deferred();fetcher.mockImplementationOnce(()=>deniedRetry.promise);await click('[data-tool-doctor-refresh]');expect(host.querySelector('[data-tool-profile-doctor-target]')).toBeNull();expect(action()).toBeNull();
 await act(async()=>deniedRetry.resolve(Response.json({error:{code:'unavailable',message:'Temporary read failure'}},{status:503})));await settle();expect(host.querySelector('[data-tool-profile-doctor-target]')).toBeNull();
 await click('[data-tool-doctor-refresh]');expect(host.querySelector('[data-tool-profile-doctor-target]')?.textContent).toContain(shared.name);expect(onSharedWorker).toHaveBeenCalledOnce();
 await render({tool:shared,onSharedWorker,disabled:true});expect(action()).toBeNull();expect(onSharedWorker).toHaveBeenCalledOnce();expect(fetcher.mock.calls.every(call=>call[1].method==='GET')).toBe(true);
});

it('reads exact scoped facts and offers only an explicit supplied destination without writing',async()=>{
 const fetcher=vi.fn(async()=>Response.json(report())),onWorker=vi.fn();vi.stubGlobal('fetch',fetcher);
 await render({onWorker});expect(state()).toBe('ready');expect(onWorker).not.toHaveBeenCalled();expect(fetcher).toHaveBeenCalledTimes(1);
 const [path,init]=fetcher.mock.calls[0] as unknown as [string,RequestInit];
 expect(path).toBe('/api/v1/tool-profiles/tool/diagnostics?project_id=p');expect(init.method).toBe('GET');expect(init.body).toBeUndefined();expect(init.headers).toMatchObject({'X-Suma-Expected-User':'u','X-Suma-Expected-Organization':'o'});
 await click('[data-tool-doctor-maintenance-action="open_worker"]');expect(onWorker).toHaveBeenCalledExactlyOnceWith('runner');
 await render();expect(action()).toBeNull();expect(fetcher).toHaveBeenCalledTimes(1);
});
it('requires refreshing the original detail when the returned version or complete profile changes',async()=>{
 const newer={...tool,version:2},onWorker=vi.fn(),onRefreshTool=vi.fn();let value=report(newer);vi.stubGlobal('fetch',vi.fn(async()=>Response.json(value)));
 await render({onWorker,onRefreshTool});expect(state()).toBe('changed');expect(action()).toBeNull();expect(onRefreshTool).not.toHaveBeenCalled();
 await click('[data-tool-doctor-refresh-tool]');expect(onRefreshTool).toHaveBeenCalledTimes(1);
 await render({tool:newer,onWorker,onRefreshTool});expect(state()).toBe('ready');
 value=report({...newer,profile:{...newer.profile,tool_version:'2.0'}});await click('[data-tool-doctor-refresh]');expect(state()).toBe('changed');expect(action()).toBeNull();expect(onWorker).not.toHaveBeenCalled();
});
it('deducts request latency from the monotonic observation deadline and expires without polling',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});const first=deferred(),fetcher=vi.fn().mockImplementationOnce(()=>first.promise).mockImplementation(async()=>Response.json(report())),onWorker=vi.fn();vi.stubGlobal('fetch',fetcher);
 await render({onWorker});expect(state()).toBe('loading');await act(async()=>vi.advanceTimersByTime(5000));await act(async()=>first.resolve(Response.json(report())));await settle();expect(state()).toBe('ready');
 await act(async()=>vi.advanceTimersByTime(7001));expect(state()).toBe('expired');expect(action()).toBeNull();expect(fetcher).toHaveBeenCalledTimes(1);expect(onWorker).not.toHaveBeenCalled();
 await click('[data-tool-doctor-refresh]');expect(fetcher).toHaveBeenCalledTimes(2);expect(state()).toBe('ready');
});
it('suspends during an original write and ignores a late diagnostic rather than replacing the write',async()=>{
 const first=deferred(),fetcher=vi.fn().mockImplementationOnce(()=>first.promise).mockImplementation(async()=>Response.json(report())),onWorker=vi.fn();vi.stubGlobal('fetch',fetcher);
 await render({pending:true,onWorker});expect(state()).toBe('suspended');expect(fetcher).not.toHaveBeenCalled();
 await render({onWorker});expect(state()).toBe('loading');await render({pending:true,onWorker});expect((fetcher.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true);
 await act(async()=>first.resolve(Response.json(report())));await settle();expect(state()).toBe('suspended');expect(action()).toBeNull();expect(onWorker).not.toHaveBeenCalled();
 await render({onWorker});expect(fetcher).toHaveBeenCalledTimes(2);expect(state()).toBe('ready');
 await render({confirmed:false,onWorker});expect(state()).toBe('suspended');expect(action()).toBeNull();expect(fetcher).toHaveBeenCalledTimes(2);
});
it('rearms an early timer wake so an expired suggestion cannot remain visible',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});let now=0;vi.spyOn(performance,'now').mockImplementation(()=>now);vi.stubGlobal('fetch',vi.fn(async()=>Response.json(report())));
 await render({onWorker:vi.fn()});expect(state()).toBe('ready');now=11999.5;
 await act(async()=>vi.advanceTimersByTime(12000));expect(state()).toBe('ready');now=12000.5;
 await act(async()=>vi.advanceTimersByTime(1));expect(state()).toBe('expired');expect(action()).toBeNull();
});
it('discards old-account and old-project responses while exposing only the new source',async()=>{
 const first=deferred(),second=deferred(),fetcher=vi.fn().mockImplementationOnce(()=>first.promise).mockImplementationOnce(()=>second.promise),onWorker=vi.fn();vi.stubGlobal('fetch',fetcher);
 await render({onWorker});const identity={...session,user:{...session.user,id:'other'},organization:{id:'other-org',name:'Other organization'}},other={...tool,project_id:'q',worker_id:'other-runner'};
 await render({project:'q',tool:other,onWorker},identity);expect(state()).toBe('loading');expect((fetcher.mock.calls[0][1] as RequestInit).signal?.aborted).toBe(true);
 await act(async()=>second.resolve(Response.json(report(other))));await settle();expect(state()).toBe('ready');
 await act(async()=>first.resolve(Response.json(report())));await settle();expect(state()).toBe('ready');await click('[data-tool-doctor-maintenance-action="open_worker"]');expect(onWorker).toHaveBeenCalledExactlyOnceWith('other-runner');
 expect((fetcher.mock.calls[1][1] as RequestInit).headers).toMatchObject({'X-Suma-Expected-User':'other','X-Suma-Expected-Organization':'other-org'});
});


it('T223 automatically rechecks only the visible selected tool after expiry and respects the workspace pause',async()=>{
 vi.useFakeTimers();let now=0;vi.spyOn(performance,'now').mockImplementation(()=>now);let visible='visible';vi.spyOn(document,'visibilityState','get').mockImplementation(()=>visible as DocumentVisibilityState);
 const fetcher=vi.fn(async(_input:string,_init:RequestInit)=>Response.json(report()));vi.stubGlobal('fetch',fetcher);
 const onWorker=vi.fn();await render({autoRefresh:true,onWorker});await act(async()=>vi.advanceTimersByTimeAsync(0));await settle();expect(state()).toBe('ready');expect(fetcher).toHaveBeenCalledTimes(1);
 const oldAction=action()!;await render({autoRefresh:true,onWorker,confirmed:false});await act(async()=>oldAction.click());expect(onWorker).not.toHaveBeenCalled();expect(fetcher).toHaveBeenCalledTimes(1);await render({autoRefresh:true,onWorker});
 visible='hidden';now=16000;await act(async()=>vi.advanceTimersByTime(16000));await settle();expect(fetcher).toHaveBeenCalledTimes(1);
 visible='visible';await act(async()=>document.dispatchEvent(new Event('visibilitychange')));await settle();expect(fetcher).toHaveBeenCalledTimes(2);
 await act(async()=>window.dispatchEvent(new Event('focus')));await settle();expect(fetcher).toHaveBeenCalledTimes(2);
 await render({autoRefresh:false});now=40000;await act(async()=>vi.advanceTimersByTime(24000));await act(async()=>window.dispatchEvent(new Event('focus')));await settle();expect(fetcher).toHaveBeenCalledTimes(2);expect(host.querySelector('[data-tool-doctor-auto="paused"]')).not.toBeNull();
 await click('[data-tool-doctor-refresh]');expect(fetcher).toHaveBeenCalledTimes(3);expect(fetcher.mock.calls.every(call=>{const init=call[1] as RequestInit|undefined;return !init||init.method==='GET';})).toBe(true);
});

it('T223 stops automatic diagnostic reads after a refusal or drift and never performs a repair',async()=>{
 vi.useFakeTimers();let now=0;vi.spyOn(performance,'now').mockImplementation(()=>now);vi.spyOn(document,'visibilityState','get').mockReturnValue('visible');let denied=false;
 const fetcher=vi.fn(async()=>Response.json(denied?{error:{code:'forbidden',message:'Access denied'}}:report(),{status:denied?403:200}));vi.stubGlobal('fetch',fetcher);const onInspect=vi.fn();
 await render({autoRefresh:true,onInspect});await act(async()=>vi.advanceTimersByTimeAsync(0));await settle();expect(state()).toBe('ready');
 await render({autoRefresh:true,onInspect,confirmed:false});expect(state()).toBe('suspended');await render({autoRefresh:true,onInspect});expect(fetcher).toHaveBeenCalledTimes(1);denied=true;now=15000;await act(async()=>vi.advanceTimersByTimeAsync(15000));await vi.waitFor(()=>expect(state()).toBe('error'));expect(fetcher).toHaveBeenCalledTimes(2);
 now=60000;await act(async()=>vi.advanceTimersByTime(45000));await act(async()=>window.dispatchEvent(new Event('focus')));await settle();expect(fetcher).toHaveBeenCalledTimes(2);expect(onInspect).not.toHaveBeenCalled();
 await render({autoRefresh:true,onInspect,confirmed:false});await render({autoRefresh:true,onInspect});expect(state()).toBe('error');expect(fetcher).toHaveBeenCalledTimes(2);
 await render({autoRefresh:true,onInspect,tool:{...tool,version:2}});await act(async()=>vi.advanceTimersByTimeAsync(0));await vi.waitFor(()=>expect(state()).toBe('error'));expect(fetcher).toHaveBeenCalledTimes(3);await render({autoRefresh:true,onInspect,pending:true});expect(state()).toBe('suspended');now=90000;await act(async()=>vi.advanceTimersByTime(30000));expect(fetcher).toHaveBeenCalledTimes(3);expect(onInspect).not.toHaveBeenCalled();
});


it('T223 preserves an in-flight scoped read during background detail refresh and cancels it for a real pending write',async()=>{
 const first=deferred(),second=deferred();const fetcher=vi.fn().mockImplementationOnce(()=>first.promise).mockImplementationOnce(()=>second.promise);vi.stubGlobal('fetch',fetcher);
 await render({autoRefresh:true});expect(state()).toBe('loading');expect(fetcher).toHaveBeenCalledTimes(1);
 const signal=(fetcher.mock.calls[0][1] as RequestInit).signal;
 await render({autoRefresh:true,confirmed:false});expect(state()).toBe('suspended');expect(signal?.aborted).toBe(false);await render({autoRefresh:true});expect(fetcher).toHaveBeenCalledTimes(1);
 await act(async()=>first.resolve(Response.json(report())));await vi.waitFor(()=>expect(state()).toBe('ready'));
 await click('[data-tool-doctor-refresh]');expect(fetcher).toHaveBeenCalledTimes(2);const next=(fetcher.mock.calls[1][1] as RequestInit).signal;
 await render({autoRefresh:true,pending:true});expect(state()).toBe('suspended');expect(next?.aborted).toBe(true);await act(async()=>second.resolve(Response.json(report())));await settle();expect(state()).toBe('suspended');expect(fetcher).toHaveBeenCalledTimes(2);
});

it('links a confirmed removed configuration to its exact ready queue and guards stale navigation',async()=>{
 let now=0;
 vi.spyOn(performance,'now').mockImplementation(()=>now);
 const removed:ToolProfile={...tool,id:'Tool.Removed',state:'revoked'};
 const fetcher=vi.fn(async(_input:string,_init:RequestInit)=>Response.json(report(removed)));
 vi.stubGlobal('fetch',fetcher);
 await render({tool:removed});
 const link=host.querySelector<HTMLAnchorElement>('[data-tool-queue-focus]')!;
 expect(state()).toBe('ready');
 expect(link.textContent).toContain('View ready queue for this configuration');
 expect(Object.fromEntries(new URL(link.href).searchParams)).toEqual({
  project:'p',view:'development',sub:'runs',layout:'queue',queue_tool:'Tool.Removed',
 });
 const block=vi.fn((event:Event)=>event.preventDefault());
 window.addEventListener('suma:navigate',block);
 try{
  await act(async()=>expect(link.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}))).toBe(false));
  expect(block).toHaveBeenCalledOnce();
  now=13000;
  await act(async()=>expect(link.dispatchEvent(new MouseEvent('click',{bubbles:true,cancelable:true}))).toBe(false));
  expect(block).toHaveBeenCalledOnce();
 }finally{window.removeEventListener('suma:navigate',block);}
 expect(state()).toBe('expired');
 expect(host.querySelector('[data-tool-queue-focus]')).toBeNull();
 await render({tool:removed,pending:true});
 expect(state()).toBe('suspended');
 expect(host.querySelector('[data-tool-queue-focus]')).toBeNull();
 expect(fetcher).toHaveBeenCalledOnce();
 expect(fetcher.mock.calls[0][1].method).toBe('GET');
});
