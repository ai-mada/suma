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
const report=(source=tool):ToolProfileDiagnostics=>({tool:source,checked_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:00:12Z',checks:[{code:'authorization',status:'pass',reason:'',next_action:'none'},{code:'worker',status:'blocked',reason:source.reason,next_action:'open_worker'},{code:'inventory',status:'not_checked',reason:'Waiting for node',next_action:'refresh'},{code:'profile',status:'not_checked',reason:'Waiting for inventory',next_action:'refresh'}],next_action:'open_worker'});
type Props=Parameters<typeof ToolProfileDoctor>[0];
let root:Root,host:HTMLDivElement;
async function settle(){await act(async()=>{for(let i=0;i<18;i++)await Promise.resolve();});}
async function render(props:Partial<Props>={},identity:Session=session){await act(async()=>root.render(<ApiViewProvider session={identity}><ToolProfileDoctor project="p" tool={tool} confirmed {...props}/></ApiViewProvider>));await settle();}
function state(){return host.querySelector('[data-tool-doctor-card]')?.getAttribute('data-tool-doctor-state');}
const action=()=>host.querySelector<HTMLButtonElement>('[data-tool-doctor-action]');
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
function deferred(){let resolve!:(value:Response)=>void;const promise=new Promise<Response>(done=>{resolve=done;});return {promise,resolve};}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('reads exact scoped facts and offers only an explicit supplied destination without writing',async()=>{
 const fetcher=vi.fn(async()=>Response.json(report())),onWorker=vi.fn();vi.stubGlobal('fetch',fetcher);
 await render({onWorker});expect(state()).toBe('ready');expect(onWorker).not.toHaveBeenCalled();expect(fetcher).toHaveBeenCalledTimes(1);
 const [path,init]=fetcher.mock.calls[0] as unknown as [string,RequestInit];
 expect(path).toBe('/api/v1/tool-profiles/tool/diagnostics?project_id=p');expect(init.method).toBe('GET');expect(init.body).toBeUndefined();expect(init.headers).toMatchObject({'X-Suma-Expected-User':'u','X-Suma-Expected-Organization':'o'});
 await click('[data-tool-doctor-action="open_worker"]');expect(onWorker).toHaveBeenCalledExactlyOnceWith('runner');
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
 await act(async()=>first.resolve(Response.json(report())));await settle();expect(state()).toBe('ready');await click('[data-tool-doctor-action="open_worker"]');expect(onWorker).toHaveBeenCalledExactlyOnceWith('other-runner');
 expect((fetcher.mock.calls[1][1] as RequestInit).headers).toMatchObject({'X-Suma-Expected-User':'other','X-Suma-Expected-Organization':'other-org'});
});
