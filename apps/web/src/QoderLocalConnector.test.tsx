// @vitest-environment jsdom
import {act,useState} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {QoderLocalConnector} from './QoderLocalConnector';
import type {DesktopACPRequest,DesktopACPResult,DesktopACPInstallation,DesktopACPAdapterID} from './desktop-acp-connector-model';
import type {DesktopToolRequest} from './desktop-tool-connector-model';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),installation='4'.repeat(32),probe='5'.repeat(32),worker='desktop-'+'6'.repeat(32);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const modelA='model:alpha',modelB='model:beta',shaA='c'.repeat(64),shaB='d'.repeat(64),executable='a'.repeat(64),stamp='2026-11-01T10:00:00Z';
const settleTimeout=window.setTimeout.bind(window);
const native:DesktopACPInstallation={installation_id:installation,label:'Observed native Qoder',kind:'native',tool_version:'Qoder 1.2.3',executable_sha256:executable};
const base={project_id:project,user_id:user,tenant_id:tenant,adapter_id:'qoder-acp',locale:'en-US'};
function observation(phase:'unconfigured'|'detected'|'unknown'='unconfigured'):DesktopACPResult{return {status:'ok',observation:{project_id:project,user_id:user,tenant_id:tenant,adapter_id:'qoder-acp',phase,installations:phase==='unconfigured'?[]:[native]}};}
function ready(operation:string,owner=user,name='Observed tool model'):DesktopACPResult{const now=Date.now();return {status:'ok',observation:{project_id:project,user_id:owner,tenant_id:tenant,adapter_id:'qoder-acp',phase:'ready',installations:[native],probe:{id:probe,operation_id:operation,installation_id:installation,source_sha256:'b'.repeat(64),created_at:new Date(now-1000).toISOString(),expires_at:new Date(now+59000).toISOString(),discovery:{protocol:'suma-acp-model-discovery-v1',runtime:'qoder',state:'ready',executable_sha256:executable,tool_version:'Qoder 1.2.3',catalogue:{source:'acp_session_config',protocol_version:1,config_id:'model-choice',current_value:modelA,models:[{id:modelA,name,configuration_sha256:shaA},{id:modelB,name,configuration_sha256:shaB}]},evidence:{session_created:true,session_cancel_requested:false,session_cancel_written:false,prompt_requests:0,configuration_writes:0,permission_requests:0,unexpected_activity:0,host_process_stopped:true,host_process_tree_stopped:true,host_process_exit_code:0,host_stopped_after_rpc:false}}}}};}
function managed(phase:string,model=modelA,sha=shaA,withWorker=true){return {status:'ok',connection:{project_id:project,adapter_id:'qoder-acp',phase,...(withWorker?{worker_id:worker,model,configuration_sha256:sha,tool_version:'Qoder 1.2.3',executable_sha256:executable}:{})}};}
let root:Root,host:HTMLDivElement,onConnected:ReturnType<typeof vi.fn>,inspect:ReturnType<typeof vi.fn<(request:DesktopACPRequest)=>Promise<unknown>>>,manage:ReturnType<typeof vi.fn<(request:DesktopToolRequest)=>Promise<unknown>>>,inspectReply:(request:DesktopACPRequest)=>Promise<unknown>,manageReply:(request:DesktopToolRequest)=>Promise<unknown>;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.useFakeTimers({toFake:['Date']});vi.setSystemTime(stamp);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);onConnected=vi.fn();
 inspectReply=async request=>request.action==='detect'?observation('detected'):request.action==='models'?ready(request.operation_id):observation();manageReply=async()=>managed('unconfigured',modelA,shaA,false);
 inspect=vi.fn(request=>inspectReply(request));manage=vi.fn(request=>manageReply(request));vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',inspectLocalACP:inspect,manageLocalTool:manage});vi.stubGlobal('fetch',vi.fn(async()=>{throw new Error('Unexpected HTTP');}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let index=0;index<5;index++)await act(async()=>{await new Promise(resolve=>settleTimeout(resolve,0));});}
async function render(identity=session,adapterID:DesktopACPAdapterID='qoder-acp'){await act(async()=>root.render(<ApiViewProvider session={identity}><QoderLocalConnector project={project} adapterID={adapterID} onConnected={onConnected}/></ApiViewProvider>));await settle();}
function element<T extends HTMLElement=HTMLButtonElement>(selector:string){const value=host.querySelector<T>(selector);expect(value,selector).not.toBeNull();return value!;}
async function click(selector:string){await act(async()=>element(selector).click());await settle();}
async function select(selector:string,value:string){const input=element<HTMLSelectElement>(selector);await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function choose(model=modelA){await click('[data-acp-installations-inspect]');if(element<HTMLSelectElement>('[data-acp-installation-select]').value!==installation)await select('[data-acp-installation-select]',installation);await click('[data-acp-models-discover]');await select('[data-acp-model-select]',model);}

// Synthetic authorized native proof exercises the shared UI; it is not a
// statement that an installed Cursor host or vendor account was verified.
function cursorObservation(result:DesktopACPResult):DesktopACPResult{
 if(result.status!=='ok')return result;
 const value=result.observation,discovery=value.probe?.discovery;
 return {status:'ok',observation:{...value,adapter_id:'cursor-cli',installations:value.installations.map(item=>({...item,label:'Observed Cursor Agent',tool_version:item.tool_version?'Cursor Agent 1.2.3':undefined})),...(value.probe?{probe:{...value.probe,...(discovery?{discovery:{...discovery,runtime:'cursor',tool_version:'Cursor Agent 1.2.3'}}:{})}}:{})}};
}
async function credential(value:string){const input=element<HTMLInputElement>('[data-acp-credential-reference]');await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));});await settle();}

it('connects only the explicitly selected Cursor source and model using its own credential reference and exact Worker proof',async()=>{
 const onReady=vi.fn(),cursorBase={...base,adapter_id:'cursor-cli'};
 inspectReply=async request=>cursorObservation(request.action==='detect'?observation('detected'):request.action==='models'?ready(request.operation_id):observation());
 manageReply=async()=>({status:'ok',connection:{project_id:project,adapter_id:'cursor-cli',phase:'unconfigured'}});
 await act(async()=>root.render(<ApiViewProvider session={session}><QoderLocalConnector project={project} adapterID="cursor-cli" onConnected={onConnected} onRegistrationReadyChange={onReady}/></ApiViewProvider>));await settle();
 expect(inspect.mock.calls.map(([request])=>request)).toEqual([{...cursorBase,action:'status'}]);expect(host.textContent).toContain('Cursor Agent quick connection');expect(host.textContent).not.toContain('local Qoder');expect(host.querySelector('a[href*="docs.qoder.com"]')).toBeNull();
 await click('[data-acp-installations-inspect]');expect(element<HTMLButtonElement>('[data-acp-models-discover]').disabled).toBe(true);expect(host.textContent).not.toContain('leave blank to use the existing CLI login');await credential('PATH');expect(element<HTMLButtonElement>('[data-acp-models-discover]').disabled).toBe(true);expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(0);
 await credential('MY_CURSOR_TOKEN');await click('[data-acp-models-discover]');expect(inspect.mock.calls.at(-1)![0]).toMatchObject({...cursorBase,action:'models',installation_id:installation,credential_env_ref:'MY_CURSOR_TOKEN'});expect(element<HTMLSelectElement>('[data-acp-model-select]').value).toBe('');expect(onConnected).not.toHaveBeenCalled();
 await select('[data-acp-model-select]',modelB);manageReply=async()=>managed('connected',modelB,shaB);await click('[data-local-tool-connect]');expect(onConnected).not.toHaveBeenCalled();expect(onReady).not.toHaveBeenLastCalledWith(true);
 manageReply=async()=>({status:'ok',connection:{project_id:project,adapter_id:'cursor-cli',phase:'connected',worker_id:worker,model:modelB,configuration_sha256:shaB,tool_version:'Cursor Agent 1.2.3',executable_sha256:executable}});await click('[data-local-tool-status]');expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);expect(onReady).toHaveBeenLastCalledWith(true);
 expect(manage.mock.calls.find(([request])=>request.action==='connect')![0]).toEqual({...cursorBase,action:'connect',acp_selection:{installation_id:installation,probe_id:probe,model_id:modelB},expected_configuration_sha256:shaB});
 await credential('MY_OTHER_CURSOR_TOKEN');expect(onReady).toHaveBeenLastCalledWith(false);expect(element('[data-acp-models-state]').getAttribute('data-acp-models-state')).toBe('source_changed');expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);expect(fetch).not.toHaveBeenCalled();
});

it('restores the original Cursor observation without reprobing and isolates Qoder recovery and late adapter responses',async()=>{
 const cursorBase={...base,adapter_id:'cursor-cli'};let original='';
 inspectReply=async request=>{if(request.action==='models'){original=request.operation_id;throw Error('Original Cursor observation response lost');}return cursorObservation(observation(request.action==='detect'?'detected':request.operation_id?'unknown':'unconfigured'));};
 manageReply=async request=>({status:'ok',connection:{project_id:project,adapter_id:request.adapter_id,phase:'unconfigured'}});
 await render(session,'cursor-cli');await click('[data-acp-installations-inspect]');await credential('MY_CURSOR_TOKEN');await click('[data-acp-models-discover]');
 const key=Array.from({length:sessionStorage.length},(_,index)=>sessionStorage.key(index)!).find(value=>value.includes('desktop-acp:')&&value.includes('cursor-cli'))!,saved=sessionStorage.getItem(key);expect(JSON.parse(saved!)).toMatchObject({...cursorBase,action:'models',operation_id:original,credential_env_ref:'MY_CURSOR_TOKEN'});
 await act(async()=>root.unmount());root=createRoot(host);await render(session,'cursor-cli');expect(inspect.mock.calls.at(-1)![0]).toEqual({...cursorBase,action:'status',operation_id:original});expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);
 let finish!:(value:unknown)=>void;inspectReply=()=>new Promise(resolve=>{finish=resolve;});await click('[data-acp-observation-recover]');const late=finish;
 inspectReply=async()=>observation();await render(session,'qoder-acp');expect(inspect.mock.calls.at(-1)![0]).toEqual({...base,action:'status'});await act(async()=>late(cursorObservation(ready(original,user,'Private Cursor model'))));await settle();expect(host.textContent).not.toContain('Private Cursor model');expect(host.querySelector('[data-acp-model-select]')).toBeNull();expect(sessionStorage.getItem(key)).toBe(saved);
 inspectReply=async()=>({status:'failed',code:'acp_discovery_authorization_changed'});await render(session,'cursor-cli');expect(host.querySelector('[data-acp-model-select],[data-local-tool-connect]')).toBeNull();expect(sessionStorage.getItem(key)).toBe(saved);expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);expect(onConnected).not.toHaveBeenCalled();expect(fetch).not.toHaveBeenCalled();
});

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

it('reports model and managed uncertainty to the parent without disabling original recovery, and unlocks after confirmation',async()=>{
 const onBlocked=vi.fn(),onReady=vi.fn();
 function Parent(){const [blocked,setBlocked]=useState(false);return <ApiViewProvider session={session}><button data-parent-tool-picker disabled={blocked}>Choose tool</button><QoderLocalConnector project={project} adapterID="qoder-acp" onConnected={onConnected} onBlockedChange={value=>{onBlocked(value);setBlocked(value);}} onRegistrationReadyChange={onReady}/></ApiViewProvider>;}
 await act(async()=>root.render(<Parent/>));await settle();expect(element<HTMLButtonElement>('[data-parent-tool-picker]').disabled).toBe(false);
 await click('[data-acp-installations-inspect]');let finishModel!:(value:unknown)=>void,original='';inspectReply=request=>request.action==='models'?(original=request.operation_id,new Promise(resolve=>{finishModel=resolve;})):Promise.resolve(observation());
 await click('[data-acp-models-discover]');expect(onBlocked).toHaveBeenLastCalledWith(true);expect(element<HTMLButtonElement>('[data-parent-tool-picker]').disabled).toBe(true);
 await act(async()=>finishModel({status:'failed',code:'acp_discovery_unknown'}));await settle();expect(onBlocked).toHaveBeenLastCalledWith(true);expect(element<HTMLButtonElement>('[data-acp-observation-status]').disabled).toBe(false);
 inspectReply=async request=>ready(request.action==='status'?request.operation_id||original:original);await click('[data-acp-observation-status]');expect(onBlocked).toHaveBeenLastCalledWith(false);await select('[data-acp-model-select]',modelA);
 let finishConnect!:(value:unknown)=>void;manageReply=()=>new Promise(resolve=>{finishConnect=resolve;});await click('[data-local-tool-connect]');expect(onBlocked).toHaveBeenLastCalledWith(true);expect(element<HTMLSelectElement>('[data-acp-model-select]').disabled).toBe(true);expect(element<HTMLInputElement>('[data-acp-credential-reference]').disabled).toBe(true);
 await act(async()=>finishConnect({status:'failed',code:'managed_runner_busy'}));await settle();expect(onBlocked).toHaveBeenLastCalledWith(true);expect(element<HTMLButtonElement>('[data-local-tool-status]').disabled).toBe(false);expect(element<HTMLButtonElement>('[data-acp-observation-status]').disabled).toBe(false);
 manageReply=async()=>managed('connected');await click('[data-local-tool-status]');expect(onBlocked).toHaveBeenLastCalledWith(false);expect(onReady).toHaveBeenLastCalledWith(true);expect(element<HTMLButtonElement>('[data-parent-tool-picker]').disabled).toBe(false);expect(onConnected).toHaveBeenCalledExactlyOnceWith(worker);expect(inspect.mock.calls.filter(([request])=>request.action==='models')).toHaveLength(1);
 vi.stubGlobal('sumaDesktop',undefined);await act(async()=>root.render(<Parent/>));await settle();expect(onBlocked).toHaveBeenLastCalledWith(false);expect(onReady).toHaveBeenLastCalledWith(false);expect(fetch).not.toHaveBeenCalled();
});
