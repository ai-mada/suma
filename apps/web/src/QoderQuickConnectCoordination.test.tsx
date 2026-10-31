// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import type {QoderLocalConnectorProps} from './QoderLocalConnector';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolAction} from './tool-hooks';
import {setLocale} from './i18n';
const leaf=vi.hoisted(()=>({current:undefined as QoderLocalConnectorProps|undefined}));
const worker='desktop-'+'4'.repeat(32),oldWorker='desktop-'+'5'.repeat(32);
vi.mock('./QoderLocalConnector',()=>({QoderLocalConnector:(props:QoderLocalConnectorProps)=>{leaf.current=props;return <div><button type="button" data-qoder-connected onClick={()=>props.onConnected(worker)}>Fixture connected</button><button type="button" data-qoder-dirty onClick={()=>props.onRegistrationReadyChange?.(false)}>Fixture edited</button><button type="button" data-qoder-ready onClick={()=>props.onRegistrationReadyChange?.(true)}>Fixture current</button><button type="button" data-qoder-pending onClick={()=>props.onBlockedChange?.(true)}>Fixture unknown</button><button type="button" data-qoder-reconcile disabled={props.disabled} onClick={()=>props.onBlockedChange?.(false)}>Fixture reconcile</button></div>;}}));
const adapter={id:'qoder-acp',product:'Qoder',entrypoint_kind:'acp',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'qoder',protocol:'acp-stdio-v1',capabilities:[],source_refs:[],limitations:[]} as const;
const acp={runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:'c'.repeat(64),model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'};
const project='1'.repeat(32),session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
let root:Root,host:HTMLDivElement,props:ToolQuickConnectProps;
async function settle(){await act(async()=>{for(let i=0;i<18;i++)await Promise.resolve();});}
async function render(){await act(async()=>root.render(<ApiViewProvider session={session}><ToolQuickConnect {...props}/></ApiViewProvider>));await settle();}
async function click(action:string){const button=host.querySelector<HTMLButtonElement>('[data-qoder-'+action+']');expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);
 props={project,workers:identity([oldWorker]),sharedWorkers:identity([]) as unknown as SharedIdentityRead,action:{busy:false,conflict:false,run:vi.fn()} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
 vi.stubGlobal('fetch',vi.fn(async(input:string)=>{const url=new URL(String(input),'https://example.test');if(url.pathname.endsWith('/adapter-catalog'))return Response.json({project_id:project,as_of:'2026-10-31T08:00:00Z',items:[adapter]});const actualWorker=url.searchParams.get('worker_id');return Response.json({project_id:project,worker_id:actualWorker,state:'fresh',as_of:'2026-10-31T08:00:00Z',observed_at:'2026-10-31T08:00:00Z',expires_at:'2026-10-31T08:01:00Z',items:[{profile:{command_id:actualWorker===worker?'exact-qoder':'old-qoder',adapter:'qoder',protocol:'acp-stdio-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'1.0',read_only:false,acp_execution:acp},available:true,reason:''}]});}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('keeps Qoder worker identity, draft readiness and unresolved operation locks aligned with its parent without blocking reconciliation',async()=>{
 await render();const tool=host.querySelector<HTMLSelectElement>('[data-quick-tool]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(tool,adapter.id);tool.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 await click('connected');expect(host.querySelector('[data-quick-register]')).toBeNull();
 props.workers=identity([oldWorker,worker]);await render();expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('exact-qoder');
 await click('dirty');expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);await click('ready');
 await click('pending');expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')!.disabled).toBe(true);expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')!.disabled).toBe(true);expect(leaf.current?.disabled).toBe(false);
 expect(host.querySelector<HTMLButtonElement>('[data-qoder-reconcile]')!.disabled).toBe(false);await click('reconcile');expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')!.disabled).toBe(false);expect(props.action.run).not.toHaveBeenCalled();expect(props.onEnable).not.toHaveBeenCalled();
});
