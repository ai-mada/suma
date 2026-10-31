// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {QoderLocalConnector} from './QoderLocalConnector';
import type {DesktopACPRequest,DesktopACPResult,DesktopACPInstallation} from './desktop-acp-connector-model';
import type {DesktopToolRequest} from './desktop-tool-connector-model';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),installation='4'.repeat(32),probe='5'.repeat(32),worker='desktop-'+'6'.repeat(32);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const modelA='model:alpha',modelB='model:beta',shaA='c'.repeat(64),shaB='d'.repeat(64),executable='a'.repeat(64),stamp='2026-11-01T10:00:00Z';
const settleTimeout=window.setTimeout.bind(window);
const native:DesktopACPInstallation={installation_id:installation,label:'Observed native Qoder',kind:'native',tool_version:'Qoder 1.2.3',executable_sha256:executable};
const base={project_id:project,user_id:user,tenant_id:tenant,adapter_id:'qoder-acp',locale:'en-US'};
function observation(phase:'unconfigured'|'detected'|'unknown'='unconfigured'):DesktopACPResult{return {status:'ok',observation:{project_id:project,user_id:user,tenant_id:tenant,adapter_id:'qoder-acp',phase,installations:phase==='unconfigured'?[]:[native]}};}
function ready(operation:string,owner=user,name='Observed tool model'):DesktopACPResult{const now=Date.now();return {status:'ok',observation:{project_id:project,user_id:owner,tenant_id:tenant,adapter_id:'qoder-acp',phase:'ready',installations:[native],probe:{id:probe,operation_id:operation,installation_id:installation,source_sha256:'b'.repeat(64),created_at:new Date(now-1000).toISOString(),expires_at:new Date(now+59000).toISOString(),discovery:{protocol:'suma-acp-model-discovery-v1',runtime:'qoder',state:'ready',executable_sha256:executable,tool_version:'Qoder 1.2.3',catalogue:{source:'acp_session_config',protocol_version:1,config_id:'model-choice',current_value:modelA,models:[{id:modelA,name,configuration_sha256:shaA},{id:modelB,name,configuration_sha256:shaB}]},evidence:{session_created:true,session_close_requested:false,session_close_acknowledged:false,prompt_requests:0,configuration_writes:0,permission_requests:0,unexpected_activity:0,host_process_stopped:true,host_process_tree_stopped:true}}}}};}
function managed(phase:string,model=modelA,sha=shaA,withWorker=true){return {status:'ok',connection:{project_id:project,adapter_id:'qoder-acp',phase,...(withWorker?{worker_id:worker,model,configuration_sha256:sha,tool_version:'Qoder 1.2.3',executable_sha256:executable}:{})}};}
let root:Root,host:HTMLDivElement,onConnected:ReturnType<typeof vi.fn>,inspect:ReturnType<typeof vi.fn<(request:DesktopACPRequest)=>Promise<unknown>>>,manage:ReturnType<typeof vi.fn<(request:DesktopToolRequest)=>Promise<unknown>>>,inspectReply:(request:DesktopACPRequest)=>Promise<unknown>,manageReply:(request:DesktopToolRequest)=>Promise<unknown>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(stamp);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);onConnected=vi.fn();
 inspectReply=async request=>request.action==='detect'?observation('detected'):request.action==='models'?ready(request.operation_id):observation();manageReply=async()=>managed('unconfigured',modelA,shaA,false);
 inspect=vi.fn(request=>inspectReply(request));manage=vi.fn(request=>manageReply(request));vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',inspectLocalACP:inspect,manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(async()=>{throw new Error('Unexpected HTTP');}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let index=0;index<5;index++)await act(async()=>{await new Promise(resolve=>settleTimeout(resolve,0));});}
async function render(identity=session){await act(async()=>root.render(<ApiViewProvider session={identity}><QoderLocalConnector project={project} onConnected={onConnected}/></ApiViewProvider>));await settle();}
function element<T extends HTMLElement=HTMLButtonElement>(selector:string){const value=host.querySelector<T>(selector);expect(value,selector).not.toBeNull();return value!;}
async function click(selector:string){await act(async()=>element(selector).click());await settle();}
async function select(selector:string,value:string){const input=element<HTMLSelectElement>(selector);await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function choose(model=modelA){await click('[data-acp-installations-inspect]');if(element<HTMLSelectElement>('[data-acp-installation-select]').value!==installation)await select('[data-acp-installation-select]',installation);await click('[data-acp-models-discover]');await select('[data-acp-model-select]',model);}

it('disables installation detection until the initial status observation has settled',async()=>{
 let finishStatus!:(value:unknown)=>void;inspectReply=request=>request.action==='status'?new Promise(resolve=>{finishStatus=resolve;}):Promise.resolve(observation('detected'));
 await render();expect(inspect.mock.calls.map(([request])=>request.action)).toEqual(['status']);expect(element<HTMLButtonElement>('[data-acp-installations-inspect]').disabled).toBe(true);expect(element('section.desktop-tool-connector').getAttribute('aria-busy')).toBe('true');
 await click('[data-acp-installations-inspect]');expect(inspect.mock.calls.filter(([request])=>request.action==='detect')).toHaveLength(0);
 await act(async()=>finishStatus(observation()));await settle();expect(element<HTMLButtonElement>('[data-acp-installations-inspect]').disabled).toBe(false);expect(element('section.desktop-tool-connector').getAttribute('aria-busy')).toBe('false');
 await click('[data-acp-installations-inspect]');expect(inspect.mock.calls.filter(([request])=>request.action==='detect')).toHaveLength(1);expect(inspect.mock.calls.at(-1)![0]).toEqual({...base,action:'detect'});expect(fetch).not.toHaveBeenCalled();
});

it('observes only status on mount and explicitly connects the selected real model only after an exact Worker proof',async()=>{
 await render();expect(inspect.mock.calls.map(([request])=>request.action)).toEqual(['status']);expect(manage.mock.calls.map(([request])=>request.action)).toEqual(['status']);expect(inspect.mock.calls[0][0]).toEqual({...base,action:'status'});expect(onConnected).not.toHaveBeenCalled();
 await choose(modelB);const models=inspect.mock.calls.find(([request])=>request.action==='models')![0];expect(models).toMatchObject({...base,action:'models',installation_id:installation,operation_id:expect.stringMatching(/^[a-f0-9]{32}$/)});expect(manage.mock.calls.filter(([request])=>request.action==='connect')).toHaveLength(0);
 manageReply=async()=>managed('starting',modelB,shaB);await click('[data-local-tool-connect]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect',acp_selection:{installation_id:installation,probe_id:probe,model_id:modelB},expected_configuration_sha256:shaB});expect(onConnected).not.toHaveBeenCalled();
 manageReply=async()=>managed('connected',modelB,shaA);await click('[data-local-tool-status]');expect(onConnected).not.toHaveBeenCalled();manageReply=async()=>managed('connected',modelB,shaB);await click('[data-local-tool-status]');expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);expect(fetch).not.toHaveBeenCalled();
});

it('restores an unknown model observation by its saved status operation and retires denied or late private scope responses',async()=>{
 let operation='',recover=false;inspectReply=async request=>{if(request.action==='detect')return observation('detected');if(request.action==='models'){operation=request.operation_id;throw new Error('IPC response lost after the original observation');}if(request.action==='status'&&request.operation_id)return recover?ready(request.operation_id):observation('unknown');return observation();};
 await render();await click('[data-acp-installations-inspect]');await click('[data-acp-models-discover]');const original=inspect.mock.calls.find(([request])=>request.action==='models')![0],recordKey=Array.from({length:sessionStorage.length},(_,index)=>sessionStorage.key(index)!).find(key=>{try{return JSON.parse(sessionStorage.getItem(key)!).operation_id===operation;}catch{return false;}})!;expect(recordKey).toBeDefined();const stored=sessionStorage.getItem(recordKey);expect(JSON.parse(stored!)).toEqual(original);
 await act(async()=>root.unmount());root=createRoot(host);await render();expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);recover=true;await click('[data-acp-observation-recover]');expect(inspect.mock.calls.at(-1)![0]).toEqual({...base,action:'status',operation_id:operation});expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);expect(sessionStorage.getItem(recordKey)).toBe(stored);expect(host.querySelector('[data-acp-model-select]')).not.toBeNull();
 let finish!:(value:unknown)=>void;const next={...session,user:{...session.user,id:'7'.repeat(32)}};inspectReply=()=>new Promise(resolve=>{finish=resolve;});await render(next);const stale=finish;
 const denied={...session,user:{...session.user,id:'8'.repeat(32)}};inspectReply=async()=>({status:'failed',code:'acp_discovery_authorization_changed'});await render(denied);await act(async()=>stale(ready(operation,next.user.id,'Old private catalogue')));await settle();expect(host.querySelector('[data-acp-model-select]')).toBeNull();expect(host.querySelector('[data-acp-selected-model]')).toBeNull();expect(host.querySelector('[data-local-tool-connect],[data-local-tool-stop]')).toBeNull();expect(host.textContent).not.toContain('Old private catalogue');expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);expect(manage.mock.calls.filter(([request])=>request.action==='connect')).toHaveLength(0);expect(onConnected).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});

it('invalidates changed and expired drafts while preserving original status, stop and prepared recovery without reprobes',async()=>{
 vi.useRealTimers();vi.useFakeTimers({toFake:['Date','setTimeout','clearTimeout']});vi.setSystemTime(stamp);
 await render();await choose(modelA);manageReply=async()=>managed('connected');await click('[data-local-tool-connect]');expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);
 await select('[data-acp-model-select]',modelB);const credential=element<HTMLInputElement>('[data-acp-credential-reference]');await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(credential,'QODER_API_KEY');credential.dispatchEvent(new Event('input',{bubbles:true}));});await settle();expect(element('[data-acp-models-state]').getAttribute('data-acp-models-state')).toBe('source_changed');
 await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(credential,'');credential.dispatchEvent(new Event('input',{bubbles:true}));});await settle();await select('[data-acp-model-select]',modelB);await act(async()=>vi.advanceTimersByTimeAsync(61000));await settle();expect(element('[data-acp-selection-ready]').getAttribute('data-acp-selection-ready')).toBe('false');
 await click('[data-local-tool-status]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});expect(onConnected).toHaveBeenCalledOnce();manageReply=async()=>managed('stopped');await click('[data-local-tool-stop]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'stop'});const fresh=host.querySelector<HTMLButtonElement>('[data-local-tool-connect]');expect(!fresh||fresh.disabled).toBe(true);
 manageReply=async()=>managed('prepared');await click('[data-local-tool-status]');manageReply=async()=>managed('connected');await click('[data-local-tool-connect]');expect(manage.mock.calls.at(-1)![0]).toEqual({...base,action:'connect'});expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);expect(onConnected).toHaveBeenCalledOnce();expect(fetch).not.toHaveBeenCalled();
});
