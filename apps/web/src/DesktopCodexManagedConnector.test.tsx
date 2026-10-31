// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {DesktopToolConnector} from './DesktopToolConnector';
import {setLocale} from './i18n';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),worker='desktop-'+'4'.repeat(32);
const sha='a'.repeat(64),otherSHA='d'.repeat(64),selection={installation_id:'5'.repeat(32),probe_id:'6'.repeat(32)};
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const base={project_id:project,user_id:user,tenant_id:tenant,adapter_id:'codex-app-server',locale:'en-US'};
const facts={worker_id:worker,tool_version:'codex-cli 1.0.0',executable_sha256:'b'.repeat(64),configuration_sha256:sha,catalog_sha256:'c'.repeat(64)};
const response=(phase:string,extra:Record<string,unknown>={})=>({status:'ok',connection:{project_id:project,adapter_id:base.adapter_id,phase,...(phase==='unconfigured'?{}:facts),...extra}});
let host:HTMLDivElement,root:Root,manage:ReturnType<typeof vi.fn>,onConnected:ReturnType<typeof vi.fn>,onConnection:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);manage=vi.fn(async()=>response('unconfigured'));onConnected=vi.fn();onConnection=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('Unexpected HTTP request');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
const button=(action:string)=>host.querySelector<HTMLButtonElement>('[data-local-tool-'+action+']');
async function click(action:string){expect(button(action)).not.toBeNull();expect(button(action)!.disabled).toBe(false);await act(async()=>button(action)!.click());}
async function render(expected=sha){await act(async()=>root.render(<ApiViewProvider session={session}><DesktopToolConnector project={project} adapterID={base.adapter_id} codexSelection={selection} expectedConfigurationSHA={expected} onConnection={onConnection} onConnected={onConnected}/></ApiViewProvider>));}

it('adopts only the exact Codex selection and configuration, reports complete facts first, and keeps status and stop on the original connection',async()=>{
 const order:string[]=[];onConnection.mockImplementation(()=>order.push('connection'));onConnected.mockImplementation(()=>order.push('connected'));
 await render();expect(manage.mock.calls.map(call=>call[0])).toEqual([{...base,action:'status'}]);order.length=0;
 manage.mockImplementation(async()=>response('connected'));await click('connect');
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect',codex_selection:selection,expected_configuration_sha256:sha});
 expect(order).toEqual(['connection','connected']);expect(onConnection).toHaveBeenLastCalledWith(response('connected').connection);expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);
 await render(otherSHA);await click('status');expect(onConnected).toHaveBeenCalledOnce();expect(button('stop')!.disabled).toBe(false);
 expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});
 manage.mockImplementation(async()=>response('stopped'));await click('stop');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'stop'});
 expect(onConnected).toHaveBeenCalledOnce();expect(fetch).not.toHaveBeenCalled();
});

it('blocks the parent during deferred or unknown connect while keeping original status and confirmed-stop recovery operable',async()=>{
 const blocked=vi.fn();function Parent(){const [locked,setLocked]=useState(false);return <ApiViewProvider session={session}><button data-parent-selector disabled={locked}>Select tool</button><DesktopToolConnector project={project} adapterID={base.adapter_id} codexSelection={selection} expectedConfigurationSHA={sha} onConnection={onConnection} onConnected={onConnected} onBlockedChange={value=>{blocked(value);setLocked(value);}}/></ApiViewProvider>;}
 await act(async()=>root.render(<Parent/>));const selector=()=>host.querySelector<HTMLButtonElement>('[data-parent-selector]')!;expect(selector().disabled).toBe(false);
 let finish!:(value:unknown)=>void;manage.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));await click('connect');expect(selector().disabled).toBe(true);expect(button('status')!.disabled).toBe(true);
 await act(async()=>finish(response('connected',{token:'not-public'})));expect(onConnected).not.toHaveBeenCalled();expect(host.textContent).not.toContain('not-public');expect(selector().disabled).toBe(true);expect(button('status')!.disabled).toBe(false);
 manage.mockImplementation(async()=>response('prepared'));await click('status');expect(selector().disabled).toBe(true);expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});
 manage.mockImplementation(async()=>response('connected'));await click('status');expect(selector().disabled).toBe(false);expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);
 manage.mockImplementation(async()=>response('stop_unconfirmed'));await click('stop');expect(selector().disabled).toBe(true);expect(button('connect')).toBeNull();expect(button('status')!.disabled).toBe(false);
 manage.mockImplementation(async()=>response('stopped'));await click('status');expect(selector().disabled).toBe(false);expect(blocked).toHaveBeenLastCalledWith(false);
 expect(manage.mock.calls.map(call=>call[0])).toEqual([{...base,action:'status'},{...base,action:'connect',codex_selection:selection,expected_configuration_sha256:sha},{...base,action:'status'},{...base,action:'status'},{...base,action:'stop'},{...base,action:'status'}]);expect(fetch).not.toHaveBeenCalled();
});
