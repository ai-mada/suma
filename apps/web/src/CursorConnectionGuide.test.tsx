// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi,type Mock} from 'vitest';
import {CursorConnectionGuide} from './CursorConnectionGuide';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import {ApiViewProvider} from './api-context';
import type {AdapterDescriptor} from './AdapterConnections';
import {setLocale} from './i18n';

const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const officialURL='https://cursor.com/docs/cli/installation';
const candidate='C:/tools/agent.exe';
const receivedAt='2026-10-31T06:07:08.000Z';
const sourceScope={project_id:project,user_id:user,tenant_id:tenant,adapter_id:'cursor-cli',locale:'en-US'};
const row=(reason_code='cursor_agent_unverified')=>({adapter_id:'cursor-cli',product:'Cursor',installed:true,launch_supported:false,reason_code,executable:candidate});
const report=(item:unknown)=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[item]}});
const absent=(reason_code:string)=>({adapter_id:'cursor-cli',product:'Cursor',installed:false,launch_supported:false,reason_code});
type Discovery=(request:Record<string,unknown>)=>Promise<unknown>;
let root:Root,host:HTMLDivElement,fetchMock:Mock<typeof fetch>;

beforeEach(async()=>{
 await setLocale('en-US',{persist:false});
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('sumaDesktop',undefined);
 vi.useFakeTimers({toFake:['Date']});vi.setSystemTime('2026-10-31T06:00:00.000Z');
 fetchMock=vi.fn<typeof fetch>(async()=>{throw new Error('Unexpected HTTP request');});vi.stubGlobal('fetch',fetchMock);
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});

async function settle(){await act(async()=>{for(let index=0;index<12;index++)await Promise.resolve();});}
async function render(props:{project:string;disabled?:boolean}={project},identity=session){
 await act(async()=>root.render(<ApiViewProvider session={identity}><CursorConnectionGuide {...props}/></ApiViewProvider>));await settle();
}
function bridge(discoverInstalledTools:Discovery){vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',discoverInstalledTools});}
function scan(){const button=host.querySelector<HTMLButtonElement>('[data-cursor-scan]');expect(button).not.toBeNull();return button!;}
async function clickScan(){await act(async()=>scan().click());await settle();}
function observedTime(){return host.querySelector<HTMLTimeElement>('[data-cursor-observed-at]');}
function noExecutionActions(){expect(host.querySelector('[data-local-tool-connect],[data-quick-register],[data-quick-enable]')).toBeNull();}

it('provides only the official browser guide without a desktop scan or HTTP request',async()=>{
 await render();
 expect(host.querySelector('[data-cursor-guide]')).not.toBeNull();
 const link=host.querySelector<HTMLAnchorElement>('[data-cursor-browser-guide]');
 expect(link?.href).toBe(officialURL);expect(host.querySelectorAll('a')).toHaveLength(1);
 expect(host.querySelector('[data-cursor-scan]')).toBeNull();expect(host.querySelector('[data-cursor-reason]')).toBeNull();
 expect(observedTime()).toBeNull();noExecutionActions();expect(fetchMock).not.toHaveBeenCalled();
});

it('requires an explicit desktop scan and sends only the original project and actor scope',async()=>{
 const discover=vi.fn<Discovery>(async()=>report(absent('cursor_editor_only')));bridge(discover);
 await render();expect(discover).not.toHaveBeenCalled();expect(observedTime()).toBeNull();
 expect(host.querySelector<HTMLAnchorElement>('[data-cursor-official-guide]')?.href).toBe(officialURL);
 await clickScan();expect(discover).toHaveBeenCalledExactlyOnceWith(sourceScope);
 expect(host.querySelector('[data-cursor-reason="cursor_editor_only"]')).not.toBeNull();
 expect(host.querySelectorAll('button')).toHaveLength(1);noExecutionActions();expect(fetchMock).not.toHaveBeenCalled();
});

it.each([
 ['not_installed',absent('not_installed')],
 ['cursor_editor_only',absent('cursor_editor_only')],
 ['cursor_agent_unverified',row()],
 ['cursor_launcher_unsupported',row('cursor_launcher_unsupported')],
 ['cursor_scan_incomplete',absent('cursor_scan_incomplete')],
] as const)('presents the %s observation with its client receive time and no execution action',async(reason,item)=>{
 let finish!:(value:unknown)=>void;const discover=vi.fn<Discovery>(()=>new Promise(resolve=>{finish=resolve;}));bridge(discover);
 await render();await clickScan();expect(scan().disabled).toBe(true);expect(observedTime()).toBeNull();
 vi.setSystemTime(receivedAt);await act(async()=>finish(report(item)));await settle();
 expect(host.querySelector('[data-cursor-reason="'+reason+'"]')).not.toBeNull();
 expect(observedTime()?.dateTime).toBe(receivedAt);expect(observedTime()?.textContent?.trim()).not.toBe('');
 if(item.installed)expect(host.textContent).toContain(candidate);else expect(host.textContent).not.toContain(candidate);
 expect(scan().disabled).toBe(false);noExecutionActions();expect(fetchMock).not.toHaveBeenCalled();
});

it.each([
 ['launch permission',{...row(),launch_supported:true}],
 ['unverified version',{...row(),version:'2026.10.22-27dcf5f'}],
 ['another adapter',{...row(),adapter_id:'codex-jsonl'}],
] as const)('rejects an IPC result containing %s instead of presenting a trusted observation',async(_name,item)=>{
 bridge(vi.fn<Discovery>(async()=>report(item)));await render();await clickScan();
 expect(host.querySelector('[data-cursor-reason]')).toBeNull();expect(observedTime()).toBeNull();
 expect(host.textContent).not.toContain(candidate);expect(host.querySelector('[role="alert"]')).not.toBeNull();
 expect(scan().disabled).toBe(false);noExecutionActions();expect(fetchMock).not.toHaveBeenCalled();
});

it('clears old path and time before rechecking and prevents duplicate scans in the same tick',async()=>{
 let finish!:(value:unknown)=>void;
 const discover=vi.fn<Discovery>().mockResolvedValueOnce(report(row())).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));bridge(discover);
 await render();await clickScan();expect(host.textContent).toContain(candidate);expect(observedTime()).not.toBeNull();
 const button=scan();await act(async()=>{button.click();button.click();});await settle();
 expect(discover).toHaveBeenCalledTimes(2);expect(scan().disabled).toBe(true);
 expect(host.textContent).not.toContain(candidate);expect(observedTime()).toBeNull();expect(host.querySelector('[data-cursor-reason]')).toBeNull();
 vi.setSystemTime(receivedAt);await act(async()=>finish(report(absent('not_installed'))));await settle();
 expect(host.querySelector('[data-cursor-reason="not_installed"]')).not.toBeNull();expect(observedTime()?.dateTime).toBe(receivedAt);
});

it.each(['actor','project'] as const)('discards a late desktop result after the %s changes',async(changed)=>{
 let finish!:(value:unknown)=>void;const discover=vi.fn<Discovery>(()=>new Promise(resolve=>{finish=resolve;}));bridge(discover);
 await render();await clickScan();
 await render({project:changed==='project'?'4'.repeat(32):project},changed==='actor'?{...session,user:{...session.user,id:'5'.repeat(32)}}:session);
 await act(async()=>finish(report(row())));await settle();
 expect(host.textContent).not.toContain(candidate);expect(host.querySelector('[data-cursor-reason]')).toBeNull();expect(observedTime()).toBeNull();
 expect(scan().disabled).toBe(false);expect(discover).toHaveBeenCalledExactlyOnceWith(sourceScope);expect(fetchMock).not.toHaveBeenCalled();
});

it('does not scan while the owning workflow disables the guide',async()=>{
 const discover=vi.fn<Discovery>(async()=>report(row()));bridge(discover);await render({project,disabled:true});
 expect(scan().disabled).toBe(true);await clickScan();expect(discover).not.toHaveBeenCalled();
 expect(observedTime()).toBeNull();expect(fetchMock).not.toHaveBeenCalled();
});

it('reports an IPC failure without exposing its raw error or presenting stale facts',async()=>{
 const privateError='private-provider-token-and-local-stack';
 const discover=vi.fn<Discovery>().mockResolvedValueOnce(report(row())).mockRejectedValueOnce(new Error(privateError));bridge(discover);
 await render();await clickScan();expect(host.textContent).toContain(candidate);await clickScan();
 expect(host.querySelector('[role="alert"]')).not.toBeNull();expect(host.textContent).not.toContain(privateError);
 expect(host.textContent).not.toContain(candidate);expect(observedTime()).toBeNull();expect(host.querySelector('[data-cursor-reason]')).toBeNull();
 expect(scan().disabled).toBe(false);expect(fetchMock).not.toHaveBeenCalled();
});

it('routes a planned Cursor catalog entry only to the guide without node discovery or advanced registration',async()=>{
 const cursor:AdapterDescriptor={id:'cursor-cli',product:'Cursor',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'planned',setup_kind:'manual_guide',capabilities:[],source_refs:[],limitations:['no_execution_adapter']};
 fetchMock.mockImplementation(async input=>{
  expect(String(input)).toBe('/api/v1/adapter-catalog?project_id='+project);
  return Response.json({project_id:project,as_of:'2026-10-31T06:00:00Z',items:[cursor]});
 });
 const run=vi.fn(async()=>undefined),onRegistered=vi.fn(),onEnable=vi.fn(),onAdvanced=vi.fn(),onWorker=vi.fn(),read=vi.fn(async()=>true);
 const props:ToolQuickConnectProps={project,adapter:cursor,workers:{path:'local',loading:false,error:'',read},sharedWorkers:{path:'shared',loading:false,error:'',read},action:{busy:false,error:'',conflict:false,pending:undefined,deferred:false,setError:vi.fn(),clearError:vi.fn(),defer:vi.fn(),run,restore:vi.fn(async()=>undefined)},onRegistered,onEnable,onAdvanced,onWorker};
 await act(async()=>root.render(<ApiViewProvider session={session}><ToolQuickConnect {...props}/></ApiViewProvider>));await settle();
 expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.value).toBe('cursor-cli');
 expect(host.querySelector('[data-cursor-guide]')).not.toBeNull();expect(host.querySelector('[data-quick-worker],[data-quick-connect-worker],[data-quick-advanced]')).toBeNull();
 noExecutionActions();expect(fetchMock).toHaveBeenCalledTimes(1);expect(run).not.toHaveBeenCalled();expect(read).not.toHaveBeenCalled();
 expect(onRegistered).not.toHaveBeenCalled();expect(onEnable).not.toHaveBeenCalled();expect(onAdvanced).not.toHaveBeenCalled();expect(onWorker).not.toHaveBeenCalled();
});
