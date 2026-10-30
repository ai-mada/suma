// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import {setLocale} from './i18n';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),worker='desktop-'+'4'.repeat(32);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const connection=(phase:string,extra={})=>({status:'ok',connection:{project_id:project,adapter_id:'codex-jsonl',phase,...extra}});
let host:HTMLDivElement,root:Root,onConnected:ReturnType<typeof vi.fn>,manage:ReturnType<typeof vi.fn>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);onConnected=vi.fn();
 manage=vi.fn(async()=>connection('unconfigured'));vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage});
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(next=session,secondary=false){await act(async()=>root.render(<ApiViewProvider session={next}><DesktopToolConnector project={project} adapterID="codex-jsonl" onConnected={onConnected} secondary={secondary}/></ApiViewProvider>));}
const button=(action:string)=>host.querySelector<HTMLButtonElement>('[data-local-tool-'+action+']');
async function click(action:string){expect(button(action)).not.toBeNull();await act(async()=>button(action)!.click());}
it('reads status without starting and requires an explicit connection before reporting the exact node',async()=>{
 await render();expect(manage.mock.calls.map(call=>call[0].action)).toEqual(['status']);expect(onConnected).not.toHaveBeenCalled();
 manage.mockImplementation(async()=>connection('starting',{worker_id:worker}));await click('connect');expect(onConnected).not.toHaveBeenCalled();
 expect(manage.mock.calls[1][0]).toEqual({action:'connect',project_id:project,user_id:user,tenant_id:tenant,adapter_id:'codex-jsonl',locale:'en-US'});
 manage.mockImplementation(async()=>connection('connected',{worker_id:worker}));await click('status');expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);await click('status');expect(onConnected).toHaveBeenCalledOnce();
});
it('retains an uncertain connection for original-status recovery and ignores a late response for another account',async()=>{
 await render();let finish!:(value:unknown)=>void;manage.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 await click('connect');expect(button('connect')?.disabled).toBe(true);await act(async()=>finish({status:'ok',connection:{project_id:project,adapter_id:'codex-jsonl',phase:'connected',worker_id:worker,token:'private'}}));
 expect(onConnected).not.toHaveBeenCalled();expect(host.textContent).not.toContain('private');expect(host.textContent).toContain('Check the original connection');
 manage.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));await click('status');await render({...session,user:{...session.user,id:'5'.repeat(32)}});await act(async()=>finish(connection('connected',{worker_id:worker})));
 expect(onConnected).not.toHaveBeenCalled();expect(host.textContent).not.toContain(worker);
});
it('keeps stopping separate from confirmed stop and retains a management entry in the secondary layout',async()=>{
 manage.mockImplementation(async()=>connection('connected',{worker_id:worker}));await render(session,true);expect(host.querySelector('details')).not.toBeNull();
 manage.mockImplementation(async()=>connection('stop_unconfirmed',{worker_id:worker}));await click('stop');expect(host.textContent).toContain('Stop is unconfirmed');expect(button('connect')).toBeNull();expect(button('status')).not.toBeNull();
 manage.mockRejectedValueOnce(new Error('unavailable'));await click('status');expect(button('connect')).toBeNull();expect(host.textContent).toContain('Stop is unconfirmed');
 manage.mockImplementation(async()=>connection('stopped',{worker_id:worker}));await click('status');expect(host.textContent).toContain('Local node stopped');expect(button('connect')).not.toBeNull();
});
it('loads the existing project-folder picker only when the native connection needs a workspace',async()=>{
 manage.mockImplementation(async()=>connection('needs_workspace'));
 const metadata={project_id:project,configured:false,binding_available:false,desktop_binding_available:true,binding_sha256:'',binding_path_sha256:'',binding_operation_id:''};
 vi.stubGlobal('fetch',vi.fn(async()=>Response.json(metadata)));vi.stubGlobal('sumaDesktop',{platform:'linux',version:'1.0.0',manageLocalTool:manage,chooseProjectDirectory:vi.fn()});
 await render();expect(host.textContent).toContain('Choose project folder');expect(host.querySelector('input')).toBeNull();expect(fetch).toHaveBeenCalledOnce();expect(manage.mock.calls.map(call=>call[0].action)).toEqual(['status']);
});
it('hides reconnection after an authorization refusal and translates the management actions without repeating a mutation',async()=>{
 await render();manage.mockImplementation(async()=>({status:'failed',code:'managed_runner_access_changed'}));await click('connect');
 expect(button('connect')).toBeNull();expect(host.textContent).toContain('Check current account permissions and the node credential expiry.');expect(host.textContent).toContain('Expired credentials require new authorization.');expect(onConnected).not.toHaveBeenCalled();
 manage.mockImplementation(async()=>({status:'failed',code:'managed_runner_busy'}));await click('status');expect(button('connect')).toBeNull();
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('本机连接');expect(host.textContent).toContain('当前账号权限及节点凭据有效期');expect(host.textContent).toContain('凭据过期需要重新授权');expect(manage.mock.calls.map(call=>call[0].action)).toEqual(['status','connect','status']);
});
it('refreshes the same original node after a confirmed stop and explicit reconnect, without repeating callbacks for status reads',async()=>{
 manage.mockImplementation(async()=>connection('connected',{worker_id:worker}));await render();expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);
 manage.mockImplementation(async()=>connection('stopped',{worker_id:worker}));await click('stop');expect(onConnected).toHaveBeenCalledOnce();
 manage.mockImplementation(async()=>connection('connected',{worker_id:worker}));await click('connect');expect(onConnected).toHaveBeenNthCalledWith(2,worker);await click('status');expect(onConnected).toHaveBeenCalledTimes(2);
});
it('polls starting status to the real connected node without issuing another connection',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout']});await render();manage.mockImplementation(async()=>connection('starting',{worker_id:worker}));await click('connect');
 manage.mockImplementation(async()=>connection('connected',{worker_id:worker}));await act(async()=>vi.advanceTimersByTimeAsync(2001));
 expect(manage.mock.calls.map(call=>call[0].action)).toEqual(['status','connect','status']);expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);
});
