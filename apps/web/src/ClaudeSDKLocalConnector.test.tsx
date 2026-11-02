// @vitest-environment jsdom
import {act,StrictMode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {DesktopToolConnectorProps} from './DesktopToolConnector';
import {ClaudeSDKLocalConnector} from './ClaudeSDKLocalConnector';
const child=vi.hoisted(()=>({props:undefined as DesktopToolConnectorProps|undefined}));
vi.mock('./DesktopToolConnector',()=>({DesktopToolConnector:(props:DesktopToolConnectorProps)=>{child.props=props;return <div data-managed-connector-seam/>;}}));
const project='1'.repeat(32),user='2'.repeat(32),tenant='3'.repeat(32),worker='desktop-'+'4'.repeat(32),sha='a'.repeat(64),configSHA='b'.repeat(64);
const session={user:{id:user,name:'Owner',email:'owner@example.test'},organization:{id:tenant,name:'Team'}};
const report=()=>({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'claude-code-sdk',product:'Claude Code',installed:true,launch_supported:false,executable:'D:/tools/claude.exe',executable_sha256:sha,version:'2.1.314',reason_code:'configuration_required'}]}});
const capability={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-4-6',configuration_sha256:configSHA,model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'} as const;
const connection={project_id:project,adapter_id:'claude-code-sdk',phase:'connected' as const,worker_id:worker,model:capability.model,credential_env_ref:'TEAM_ANTHROPIC_KEY',tool_version:'2.1.314',executable_sha256:sha,runtime_executable_sha256:'c'.repeat(64),configuration_sha256:configSHA,claude_sdk_execution:capability,credential_version:1};
let host:HTMLDivElement,root:Root,detect:ReturnType<typeof vi.fn>,connected:ReturnType<typeof vi.fn>,ready:ReturnType<typeof vi.fn>,blocked:ReturnType<typeof vi.fn>;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);detect=vi.fn(async()=>report());connected=vi.fn();ready=vi.fn();blocked=vi.fn();vi.stubGlobal('sumaDesktop',{platform:'win32',version:'1.0.0',discoverInstalledTools:detect,manageLocalTool:vi.fn()});vi.stubGlobal('fetch',vi.fn(()=>{throw Error('No HTTP in local setup');}));});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function render(p=project,identity=session){await act(async()=>root.render(<StrictMode><ApiViewProvider session={identity}><ClaudeSDKLocalConnector project={p} onConnected={connected} onRegistrationReadyChange={ready} onBlockedChange={blocked}/></ApiViewProvider></StrictMode>));}
async function input(name:string,value:string){const field=host.querySelector<HTMLInputElement>('[name='+name+']')!;expect(field).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value')!.set!.call(field,value);field.dispatchEvent(new Event('input',{bubbles:true}));});}
async function configure(){await input('model','claude-sonnet-4-6');await input('credential_env_ref','TEAM_ANTHROPIC_KEY');}
async function initial(){await render();await act(async()=>child.props!.onConnection?.({project_id:project,adapter_id:'claude-code-sdk',phase:'unconfigured'}));}

it('T201 adopts only the real current Claude credential name and isolates stale callbacks across draft ABA',async()=>{
 await initial();await configure();const setup=child.props!.claudeSDKSetup!;expect(setup).toEqual({model:'claude-sonnet-4-6',credential_env_ref:'TEAM_ANTHROPIC_KEY',executable_sha256:sha});expect(child.props!.expectedConfigurationSHA).toBeUndefined();
 await act(async()=>child.props!.onConnection?.({...connection,credential_env_ref:'OTHER_KEY'}));expect(connected).not.toHaveBeenCalled();expect(ready).toHaveBeenLastCalledWith(false);
 await act(async()=>child.props!.onConnection?.({project_id:project,adapter_id:'claude-code-sdk',phase:'starting',worker_id:worker}));expect(ready).toHaveBeenLastCalledWith(false);expect(connected).not.toHaveBeenCalled();
 await act(async()=>child.props!.onConnection?.(connection));expect(ready).toHaveBeenLastCalledWith(true);expect(child.props!.expectedConfigurationSHA).toBeUndefined();expect(connected).toHaveBeenCalledExactlyOnceWith({worker_id:worker,model:connection.model,credential_env_ref:connection.credential_env_ref,tool_version:'2.1.314',executable_sha256:sha,runtime_executable_sha256:connection.runtime_executable_sha256,configuration_sha256:configSHA,claude_sdk_execution:capability});
 await act(async()=>child.props!.onConnection?.({...connection,credential_version:2}));expect(connected).toHaveBeenCalledTimes(2);
 const original=child.props!.onConnection;await input('credential_env_ref','OTHER_KEY');expect(ready).toHaveBeenLastCalledWith(false);expect(child.props!.expectedConfigurationSHA).toBeUndefined();await act(async()=>original?.(connection));expect(ready).toHaveBeenLastCalledWith(false);expect(connected).toHaveBeenCalledTimes(2);
 await act(async()=>child.props!.onConnection?.(connection));expect(connected).toHaveBeenCalledTimes(2);expect(fetch).not.toHaveBeenCalled();
 await input('credential_env_ref','TEAM_ANTHROPIC_KEY');await act(async()=>original?.(connection));expect(ready).toHaveBeenLastCalledWith(false);expect(connected).toHaveBeenCalledTimes(2);
 await act(async()=>child.props!.onConnection?.(connection));expect(ready).toHaveBeenLastCalledWith(true);expect(connected).toHaveBeenCalledTimes(3);
});

it('T201 retains only scoped public drafts, leaves original recovery usable and rejects late/unsupported discovery',async()=>{
 let finish!:(value:unknown)=>void;detect.mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));await initial();await render(project,{...session,user:{...session.user,id:'8'.repeat(32)}});await act(async()=>finish(report()));expect(host.textContent).not.toContain('2.1.314');expect(child.props!.claudeSDKSetup).toBeUndefined();
 await act(async()=>child.props!.onConnection?.({project_id:project,adapter_id:'claude-code-sdk',phase:'unconfigured'}));await configure();await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.querySelector<HTMLInputElement>('[name=model]')!.value).toBe('claude-sonnet-4-6');await act(async()=>child.props!.onBlockedChange?.(true));expect(host.querySelector<HTMLInputElement>('[name=model]')!.matches(':disabled')).toBe(true);expect(child.props!.claudeSDKSetup).toBeUndefined();expect(child.props!.disabled).toBe(false);expect(blocked).toHaveBeenLastCalledWith(true);
 await act(async()=>root.unmount());root=createRoot(host);await render(project,{...session,user:{...session.user,id:'8'.repeat(32)}});expect(host.querySelector<HTMLInputElement>('[name=credential_env_ref]')!.value).toBe('TEAM_ANTHROPIC_KEY');expect(child.props!.expectedConfigurationSHA).toBeUndefined();expect(connected).not.toHaveBeenCalled();
 detect.mockResolvedValueOnce({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{...report().report.items[0],version:'2.1.315',reason_code:'version_unsupported'}]}});await act(async()=>child.props!.onConnection?.({project_id:project,adapter_id:'claude-code-sdk',phase:'unconfigured'}));expect(child.props!.claudeSDKSetup).toBeUndefined();expect(ready).toHaveBeenLastCalledWith(false);await render('9'.repeat(32));expect(host.querySelector<HTMLInputElement>('[name=model]')!.value).toBe('');expect(fetch).not.toHaveBeenCalled();
});
