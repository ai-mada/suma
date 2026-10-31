// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {ToolAction} from './tool-hooks';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';

const adapter={id:'codex-jsonl',product:'Codex',entrypoint_kind:'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};
const profile={command_id:'codex-team',adapter:'codex',protocol:'codex-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false};
const observation={profile,available:true,reason:''};
const session={user:{id:'u',name:'Owner',email:'u@example.test'},organization:{id:'o',name:'Org'}};
const report=(worker='one')=>({project_id:'p',worker_id:worker,state:'fresh',as_of:'2026-10-30T08:00:20Z',observed_at:'2026-10-30T08:00:00Z',expires_at:'2026-10-30T08:01:00Z',items:[observation]});
let root:Root,host:HTMLDivElement,paths:string[],reply:(path:string)=>Promise<Response>,props:ToolQuickConnectProps;
const identity=(ids:string[],loading=false)=>({loading,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);paths=[];
 reply=async path=>Response.json(path.startsWith('/adapter-catalog')?{project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[adapter]}:report(new URL(path,'https://example.test').searchParams.get('worker_id')!));
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const path=String(input).replace('/api/v1','');paths.push(path);return reply(path);}));
 const run=vi.fn(async(_path:string,body:Record<string,unknown>)=>({...body,id:'registered',version:1,state:'paused',available:false}));
 props={project:'p',workers:identity(['one']),sharedWorkers:identity([]) as unknown as SharedIdentityRead,action:{run,busy:false,pending:undefined,conflict:false} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.useRealTimers();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<18;i++)await Promise.resolve();});}
async function render(identitySession=session){await act(async()=>root.render(<ApiViewProvider session={identitySession}><ToolQuickConnect {...props}/></ApiViewProvider>));await settle();}
async function change(selector:string,value:string){const field=host.querySelector<HTMLInputElement|HTMLSelectElement>(selector)!;expect(field).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(field instanceof HTMLSelectElement?HTMLSelectElement.prototype:HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event(field instanceof HTMLSelectElement?'change':'input',{bubbles:true}));});await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector)!;expect(button).not.toBeNull();await act(async()=>button.click());await settle();}
it('automatically uses the sole authorized node and saves paused, requiring a separate explicit enable',async()=>{
 await render();await change('[data-quick-tool]','codex-jsonl');
 expect(paths).toEqual(['/adapter-catalog?project_id=p','/tool-discovery?project_id=p&worker_id=one']);
 expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('codex-team');expect(props.action.run).not.toHaveBeenCalled();
 await change('[data-quick-name]','Team Codex');await click('[data-quick-register]');
 expect(props.action.run).toHaveBeenCalledExactlyOnceWith('/tool-profiles',{project_id:'p',worker_id:'one',name:'Team Codex',profile});
 expect(props.onRegistered).toHaveBeenCalledOnce();expect(props.onEnable).not.toHaveBeenCalled();expect(host.textContent).toContain('paused');
 await click('[data-quick-enable]');expect(props.onEnable).toHaveBeenCalledOnce();
});
it('Qoder uses actual ACP inventory for paused registration without claiming local discovery or model health',async()=>{
 const qoder={...adapter,id:'qoder-acp',product:'Qoder',adapter:'qoder',protocol:'acp-stdio-v1'};
 const capability={runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:'c'.repeat(64),model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'};
 const fixed={...profile,command_id:'qoder-team',adapter:'qoder',protocol:'acp-stdio-v1',acp_execution:capability};
 const discoverInstalledTools=vi.fn(),manageLocalTool=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',discoverInstalledTools,manageLocalTool});
 reply=async path=>Response.json(path.startsWith('/adapter-catalog')?{project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[qoder]}:{...report(),items:[{profile:fixed,available:true,reason:'actual protocol observation'}]});
 await render();await change('[data-quick-tool]','qoder-acp');expect(props.action.run).not.toHaveBeenCalled();expect(discoverInstalledTools).not.toHaveBeenCalled();expect(manageLocalTool).not.toHaveBeenCalled();
 await click('[data-quick-advanced]');expect(props.onAdvanced).toHaveBeenCalledExactlyOnceWith(qoder);
 await click('[data-quick-register]');expect(props.action.run).toHaveBeenCalledExactlyOnceWith('/tool-profiles',{project_id:'p',worker_id:'one',name:'qoder-team',profile:fixed});expect(props.onEnable).not.toHaveBeenCalled();
 expect(host.querySelector('[data-quick-register]')).toBeNull();await click('[data-quick-enable]');expect(props.onEnable).toHaveBeenCalledOnce();
});
it('requires a node choice when several are authorized and never fans out reads',async()=>{
 props.workers=identity(['one','two']);await render();await change('[data-quick-tool]','codex-jsonl');expect(paths).toHaveLength(1);
 await change('[data-quick-worker]','two');expect(paths).toHaveLength(2);expect(paths[1]).toContain('worker_id=two');
});
it('expires inventory and refuses registration without silently reading again',async()=>{
 vi.useFakeTimers({toFake:['setTimeout','clearTimeout','performance']});await render();await change('[data-quick-tool]','codex-jsonl');
 expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(false);
 await act(async()=>vi.advanceTimersByTimeAsync(40001));expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);expect(paths).toHaveLength(2);
});
it('hides old facts on authorization loss and ignores a late response after a scope change',async()=>{
 let finish!:(value:Response)=>void;const base=reply;reply=path=>path.startsWith('/tool-discovery')?new Promise(resolve=>{finish=resolve;}):base(path);
 await render();await change('[data-quick-tool]','codex-jsonl');props.workers=identity([],true);await render();await act(async()=>finish(Response.json(report())));await settle();
 expect(host.textContent).not.toContain('codex-team');expect(host.querySelector('[data-quick-register]')).toBeNull();
 props.workers=identity(['one']);await render({...session,user:{...session.user,id:'other'}});expect(host.querySelector('[data-quick-name]')).toBeNull();expect(props.action.run).not.toHaveBeenCalled();
});
it('offers a real node connection action and advanced setup when no node is connected',async()=>{
 props.workers=identity([]);await render();await change('[data-quick-tool]','codex-jsonl');await click('[data-quick-connect-worker]');expect(props.onWorker).toHaveBeenCalledOnce();expect(paths).toHaveLength(1);
 await click('[data-quick-advanced]');expect(props.onAdvanced).toHaveBeenCalledOnce();await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('连接执行节点');
});
it('requires a configuration choice and retains an authored name across a fresh inventory read',async()=>{
 const base=reply;reply=async path=>path.startsWith('/tool-discovery')?Response.json({...report(),items:[observation,{...observation,profile:{...profile,command_id:'codex-other'}},{...observation,available:false,reason:'not available',profile:{...profile,command_id:'other-protocol',protocol:'codex-app-server-v1'}}]}):base(path);
 await render();await change('[data-quick-tool]','codex-jsonl');expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);expect(host.textContent).not.toContain('other-protocol');
 await change('[data-quick-configuration]','codex-team');await change('[data-quick-name]','My team tool');
 const refresh=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent==='Read again')!;await act(async()=>refresh.click());await settle();await change('[data-quick-configuration]','codex-other');expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('My team tool');expect(props.action.run).not.toHaveBeenCalled();
});
it('shows a scoped desktop installation fact without creating a profile or node',async()=>{
 props.workers=identity([]);const discoverInstalledTools=vi.fn(async()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'codex-jsonl',product:'Codex',installed:true,launch_supported:true,executable:'C:/tools/codex.exe',version:'1.0',reason_code:'workspace_required'}]}}));
 vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',discoverInstalledTools});await render();await change('[data-quick-tool]','codex-jsonl');
 const scan=[...host.querySelectorAll<HTMLButtonElement>('button')].find(button=>button.textContent==='Check local installation')!;expect(scan).toBeDefined();await act(async()=>scan.click());await settle();
 expect(discoverInstalledTools).toHaveBeenCalledExactlyOnceWith({project_id:'p',user_id:'u',tenant_id:'o',adapter_id:'codex-jsonl',locale:'en-US'});expect(host.textContent).toContain('Local installation detected');expect(host.querySelector('[data-quick-register]')).toBeNull();expect(props.action.run).not.toHaveBeenCalled();
 await render({...session,user:{...session.user,id:'other'}});expect(host.textContent).not.toContain('Local installation detected');
});
it('carries the currently chosen tool into advanced setup after changing the initial catalog selection',async()=>{
 props.adapter=adapter as ToolQuickConnectProps['adapter'];const other={...adapter,id:'codebuddy-cli',product:'CodeBuddy Code',adapter:'codebuddy',protocol:'codebuddy-jsonl-v1'};
 const base=reply;reply=async path=>path.startsWith('/adapter-catalog')?Response.json({project_id:'p',as_of:'2026-10-30T08:00:20Z',items:[adapter,other]}):base(path);
 await render();await change('[data-quick-tool]','codebuddy-cli');await click('[data-quick-advanced]');expect(props.onAdvanced).toHaveBeenCalledExactlyOnceWith(other);expect(props.action.run).not.toHaveBeenCalled();
});
it('retains a matched paused POST receipt during same-scope identity loading without exposing it or offering another registration',async()=>{
 let finish!:(value:unknown)=>void;const run=vi.fn(()=>new Promise(resolve=>{finish=resolve;}));props.action={...props.action,run} as unknown as ToolAction;
 await render();await change('[data-quick-tool]','codex-jsonl');await click('[data-quick-register]');expect(run).toHaveBeenCalledOnce();
 props.workers=identity(['one'],true);await render();await act(async()=>finish({project_id:'p',worker_id:'one',name:'codex-team',profile,id:'registered',version:1,state:'paused',available:false}));await settle();
 expect(host.querySelector('[data-quick-enable]')).toBeNull();expect(host.textContent).not.toContain('Registered “codex-team”');expect(props.onRegistered).not.toHaveBeenCalled();
 props.workers=identity(['one']);await render();expect(host.querySelector('[data-quick-enable]')).not.toBeNull();expect(host.querySelector('[data-quick-register]')).toBeNull();expect(props.onRegistered).toHaveBeenCalledOnce();expect(run).toHaveBeenCalledOnce();
 await render({...session,user:{...session.user,id:'other'}});expect(host.querySelector('[data-quick-enable]')).toBeNull();expect(host.textContent).not.toContain('Registered “codex-team”');expect(run).toHaveBeenCalledOnce();
});
it('manages a local connection beside an old unreported identity, refreshes both node sources and selects the exact connected node',async()=>{
 const project='1'.repeat(32),local='desktop-'+'4'.repeat(32),currentSession={...session,user:{...session.user,id:'2'.repeat(32)},organization:{...session.organization,id:'3'.repeat(32)}};
 props.project=project;props.workers=identity(['old']);const oldRead=props.workers.read,sharedRead=props.sharedWorkers.read;
 const manageLocalTool=vi.fn(async(request:{action:string})=>({status:'ok',connection:{project_id:project,adapter_id:'codex-jsonl',phase:request.action==='connect'?'connected':'unconfigured',...(request.action==='connect'?{worker_id:local}:{})}}));
 vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',manageLocalTool});
 reply=async path=>Response.json(path.startsWith('/adapter-catalog')?{project_id:project,as_of:'2026-10-30T08:00:20Z',items:[adapter]}:{...report(new URL(path,'https://example.test').searchParams.get('worker_id')!),project_id:project,state:new URL(path,'https://example.test').searchParams.get('worker_id')==='old'?'unreported':'fresh',...(new URL(path,'https://example.test').searchParams.get('worker_id')==='old'?{items:[],observed_at:'',expires_at:''}:{})});
 await render(currentSession);await change('[data-quick-tool]','codex-jsonl');expect(host.querySelector('[data-local-tool-connect]')).not.toBeNull();await click('[data-local-tool-connect]');expect(oldRead).toHaveBeenCalledOnce();expect(sharedRead).toHaveBeenCalledOnce();
 props.workers=identity(['old',local]);await render(currentSession);expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(local);expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('codex-team');
 await click('[data-quick-register]');expect(host.querySelector('[data-quick-enable]')).not.toBeNull();expect(host.querySelector('[data-local-tool-stop]')).not.toBeNull();expect(manageLocalTool.mock.calls.filter(call=>call[0].action==='connect')).toHaveLength(1);
});
