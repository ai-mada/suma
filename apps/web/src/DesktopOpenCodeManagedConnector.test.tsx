// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import type {DesktopOpenCodeSetup} from './desktop-opencode-connector-model';
import {setLocale} from './i18n';
const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),base={project_id:project,user_id:'2'.repeat(32),tenant_id:'3'.repeat(32),adapter_id:'opencode-sdk',locale:'en-US'};
const session={user:{id:base.user_id,name:'Owner',email:'owner@example.test'},organization:{id:base.tenant_id,name:'Team'}};
const setup:DesktopOpenCodeSetup={executable_sha256:'a'.repeat(64),provider_id:'approved',model:'fixed-model',base_url:'https://Models.Example.Test:443/v1',api_key_env_ref:'MODEL_PROVIDER_KEY'};
const facts={worker_id:worker,model:setup.model,provider_id:setup.provider_id,endpoint_origin:'https://Models.Example.Test:443',tool_version:'1.19.4',executable_sha256:setup.executable_sha256,configuration_sha256:'b'.repeat(64)};
const response=(phase:string,patch:Record<string,unknown>={})=>({status:'ok',connection:{project_id:project,adapter_id:base.adapter_id,phase,...(phase==='unconfigured'?{}:facts),...patch}});
let root:Root,host:HTMLDivElement,manage:ReturnType<typeof vi.fn>,connected:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);manage=vi.fn(async()=>response('unconfigured'));connected=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected HTTP');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(draft=setup,expected=facts.configuration_sha256){await act(async()=>root.render(<ApiViewProvider session={session}><DesktopToolConnector project={project} adapterID={base.adapter_id} opencodeSetup={draft} expectedConfigurationSHA={expected} onConnected={connected}/></ApiViewProvider>));}
async function click(action:string){const button=host.querySelector<HTMLButtonElement>('[data-local-tool-'+action+']');expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());}
it('submits only the exact OpenCode setup and keeps status, stop and a later explicit setup separate',async()=>{
 await render();expect(manage.mock.calls[0][0]).toEqual({...base,action:'status'});
 manage.mockImplementation(async()=>response('connected'));await click('connect');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect',opencode_setup:setup});
 expect(connected).toHaveBeenCalledExactlyOnceWith(worker);
 const changed={...setup,base_url:'https://Models.Example.Test:443/other',api_key_env_ref:'OTHER_PROVIDER_KEY'};await render(changed,'c'.repeat(64));await click('status');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});expect(connected).toHaveBeenCalledOnce();
 manage.mockImplementation(async()=>response('stopped'));await click('stop');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'stop'});
 manage.mockImplementation(async()=>response('connected',{configuration_sha256:'c'.repeat(64)}));await click('connect');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect',opencode_setup:changed});expect(connected).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
});
it('reconciles the original uncertain connection without attaching an edited setup',async()=>{
 await render();manage.mockImplementationOnce(async()=>{throw Error('lost response');});await click('connect');
 await render({...setup,model:'edited-model'});manage.mockImplementation(async()=>response('prepared'));await click('status');await click('connect');
 expect(manage.mock.calls.slice(-2).map(call=>call[0])).toEqual([{...base,action:'status'},{...base,action:'connect'}]);
 expect(connected).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});
