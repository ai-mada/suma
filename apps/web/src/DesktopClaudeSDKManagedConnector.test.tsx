// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import {setLocale} from './i18n';
const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),base={project_id:project,user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'claude-code-sdk',locale:'en-US'};
const session={user:{id:base.user_id,name:'Owner',email:'owner@example.test'},organization:{id:base.tenant_id,name:'Team'}};
const setup={executable_sha256:'a'.repeat(64),model:'claude-sonnet-4-6',credential_env_ref:'ANTHROPIC_API_KEY'};
const capability={sdk_version:'0.3.314',cli_version:'2.1.314',model:setup.model,configuration_sha256:'b'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
const facts={worker_id:worker,model:setup.model,credential_env_ref:setup.credential_env_ref,tool_version:'2.1.314',executable_sha256:setup.executable_sha256,runtime_executable_sha256:'c'.repeat(64),configuration_sha256:capability.configuration_sha256,claude_sdk_execution:capability};
const response=(phase:string)=>({status:'ok',connection:{project_id:project,adapter_id:base.adapter_id,phase,...(phase==='unconfigured'?{}:facts)}});
let root:Root,host:HTMLDivElement,manage:ReturnType<typeof vi.fn>,connected:ReturnType<typeof vi.fn>,received:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);manage=vi.fn(async()=>response('unconfigured'));connected=vi.fn();received=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected HTTP');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(draft=setup,observer=received){await act(async()=>root.render(<ApiViewProvider session={session}><DesktopToolConnector project={project} adapterID={base.adapter_id} claudeSDKSetup={draft} onConnected={connected} onConnection={observer}/></ApiViewProvider>));}
async function click(action:string){const button=host.querySelector<HTMLButtonElement>('[data-local-tool-'+action+']');expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());}
it('T201 attests the real current credential name across slow start, status, reload and configuration-free recovery',async()=>{
 await render();expect(manage.mock.calls[0][0]).toEqual({...base,action:'status'});
 const progress={project_id:project,adapter_id:base.adapter_id,phase:'starting',worker_id:worker};
 manage.mockImplementationOnce(async()=>({status:'ok',connection:progress}));await click('connect');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect',claude_sdk_setup:setup});
 expect(received.mock.calls.at(-1)).toEqual([progress]);
 expect(connected).not.toHaveBeenCalled();manage.mockImplementation(async()=>response('connected'));await click('status');
 expect(received.mock.calls.at(-1)).toEqual([response('connected').connection]);expect(connected).toHaveBeenCalledExactlyOnceWith(worker);
 const changed={...setup,credential_env_ref:'OTHER_PROVIDER_KEY'};await render(changed);await click('status');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});expect(received.mock.calls.at(-1)).toEqual([response('connected').connection]);expect(connected).toHaveBeenCalledOnce();
 manage.mockImplementation(async()=>response('stopped'));await click('stop');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'stop'});
 manage.mockImplementationOnce(async()=>{throw Error('lost response');});await click('connect');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect',claude_sdk_setup:changed});
 manage.mockImplementationOnce(async()=>response('prepared'));await click('status');
 const recovered={...response('connected'),connection:{...response('connected').connection,credential_env_ref:changed.credential_env_ref}};manage.mockImplementation(async()=>recovered);await click('connect');
 expect(manage.mock.calls.slice(-2).map(call=>call[0])).toEqual([{...base,action:'status'},{...base,action:'connect'}]);
 expect(received.mock.calls.at(-1)).toEqual([recovered.connection]);expect(connected).toHaveBeenCalledTimes(2);
 await act(async()=>root.unmount());root=createRoot(host);await render(changed);expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});expect(connected).toHaveBeenCalledTimes(3);expect(fetch).not.toHaveBeenCalled();
 // Same-scope A->B->A must not deliver an old request to the latest observer.
 await render(setup);manage.mockImplementation(async()=>response('stopped'));await click('stop');
 const before=received.mock.calls.length,newObserver=vi.fn();let complete!:(value:ReturnType<typeof response>)=>void;
 manage.mockImplementationOnce(()=>new Promise(resolve=>{complete=resolve;}));await click('connect');
 await render(changed,newObserver);await render(setup,newObserver);
 await act(async()=>complete(response('connected')));
 expect(received).toHaveBeenCalledTimes(before+1);expect(newObserver).not.toHaveBeenCalled();
 manage.mockImplementation(async()=>response('connected'));await click('status');expect(newObserver).toHaveBeenCalledExactlyOnceWith(response('connected').connection);
});
