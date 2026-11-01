// @vitest-environment jsdom
import {act,useMemo} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider,useApi} from './api-context';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import type {OpenCodeLocalConnectorProps} from './OpenCodeLocalConnector';
import type {DesktopOpenCodeConnectedProof} from './desktop-opencode-connector-model';
import type {AdapterDescriptor} from './AdapterConnections';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolObservation,ToolProfile} from './tool-model';
import {ToolWriteRecovery} from './tool-write';
import {useToolAction} from './tool-hooks';
import {setLocale} from './i18n';
const leaf=vi.hoisted(()=>({current:undefined as OpenCodeLocalConnectorProps|undefined,proof:undefined as DesktopOpenCodeConnectedProof|undefined}));
vi.mock('./OpenCodeLocalConnector',()=>({OpenCodeLocalConnector:(props:OpenCodeLocalConnectorProps)=>{leaf.current=props;return <div><button type="button" data-opencode-connected onClick={()=>props.onConnected(leaf.proof!)}>Fixture connected</button><button type="button" data-opencode-dirty onClick={()=>props.onRegistrationReadyChange?.(false)}>Fixture draft changed</button><button type="button" data-opencode-ready onClick={()=>props.onRegistrationReadyChange?.(true)}>Fixture exact draft</button></div>;}}));
const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),oldWorker='desktop-'+'5'.repeat(32);
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const adapter:AdapterDescriptor={id:'opencode-sdk',product:'OpenCode',entrypoint_kind:'sdk',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'opencode',protocol:'opencode-sdk-v1',capabilities:[],source_refs:[],limitations:[]};
const proof:DesktopOpenCodeConnectedProof={worker_id:worker,model:'vendor/fixed-model',provider_id:'approved',endpoint_origin:'https://models.example.test',tool_version:'1.19.4',executable_sha256:'a'.repeat(64),configuration_sha256:'b'.repeat(64)};
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
function observation(id:string):ToolObservation{return {profile:{command_id:id,adapter:'opencode',protocol:'opencode-sdk-v1',profile_sha256:'c'.repeat(64),executable_sha256:'d'.repeat(64),tool_version:'suma-opencode-runtime 1.0.0',read_only:false,workspace_tools_version:'attempt-workspace-v1',opencode_execution:{sdk_version:'1.19.4',server_version:'1.19.4',provider_id:proof.provider_id,model:proof.model,endpoint_origin:proof.endpoint_origin,configuration_sha256:proof.configuration_sha256,budget_mode:'observe_only',identity_evidence:'configuration_ack_only'}},available:true,reason:''};}
let root:Root,host:HTMLDivElement,props:Omit<ToolQuickConnectProps,'action'>,inventory:ToolObservation[],posts:{body:Record<string,unknown>;headers:Headers}[];
function Consumer(){const api=useApi(),recovery=useMemo(()=>new ToolWriteRecovery('opencode-quick-parent',sessionStorage,api),[api]),action=useToolAction(recovery);return <ToolQuickConnect {...props} action={action}/>;}
async function settle(){await act(async()=>{for(let i=0;i<22;i++)await Promise.resolve();});}
async function render(){await act(async()=>root.render(<ApiViewProvider session={session}><Consumer/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);leaf.proof=proof;posts=[];
 inventory=[observation('correct'),...['configuration_sha256','provider_id','model','endpoint_origin'].map(key=>{const row=observation('wrong-'+key);row.profile.opencode_execution={...row.profile.opencode_execution!,[key]:key==='configuration_sha256'?'f'.repeat(64):key==='endpoint_origin'?'https://other.example.test':'other'};return row;})];
 props={project,workers:identity([oldWorker]),sharedWorkers:identity([]) as unknown as SharedIdentityRead,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const url=new URL(String(input),'https://example.test');if(init?.method==='POST'){const body=JSON.parse(String(init.body));if(url.pathname.endsWith('/tool-profiles/matches'))return Response.json({...body,items:[],total:0});posts.push({body,headers:new Headers(init.headers)});return Response.json({...body,id:'saved',version:1,state:'paused',available:false,reason:'',enabled_by:'',created_at:'',updated_at:''} satisfies ToolProfile);}
  if(url.pathname.endsWith('/adapter-catalog'))return Response.json({project_id:project,as_of:'2026-10-31T08:00:00Z',items:[adapter]});
  const actualWorker=url.searchParams.get('worker_id');return Response.json({project_id:project,worker_id:actualWorker,state:'fresh',as_of:'2026-10-31T08:00:00Z',observed_at:'2026-10-31T08:00:00Z',expires_at:'2026-10-31T08:01:00Z',items:actualWorker===worker?inventory:[observation('old-worker')]});}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});
it('registers only the exact OpenCode runtime inventory while preserving Node identity, draft readiness and the connected worker',async()=>{
 await render();const select=host.querySelector<HTMLSelectElement>('[data-quick-tool]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,adapter.id);select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 await click('[data-opencode-connected]');expect(host.querySelector('[data-quick-register]')).toBeNull();expect(posts).toHaveLength(0);
 props.workers=identity([oldWorker,worker]);await render();expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('correct');expect(host.querySelector('[data-quick-configuration]')).toBeNull();
 await click('[data-opencode-dirty]');expect(host.querySelector('[data-quick-register]')?.hasAttribute('disabled')).toBe(true);
 await click('[data-opencode-ready]');await click('[data-quick-register]');
 expect(posts).toHaveLength(1);expect(posts[0].body).toMatchObject({project_id:project,worker_id:worker,name:'correct',profile:{executable_sha256:'d'.repeat(64),tool_version:'suma-opencode-runtime 1.0.0',opencode_execution:{provider_id:proof.provider_id,model:proof.model,endpoint_origin:proof.endpoint_origin,configuration_sha256:proof.configuration_sha256}}});
 expect(posts[0].body.profile).not.toHaveProperty('executable');expect(posts[0].headers.get('X-Suma-Expected-User')).toBe(session.user.id);expect(props.onRegistered).toHaveBeenCalledOnce();expect(props.onEnable).not.toHaveBeenCalled();
 expect(host.querySelector('[data-quick-enable]')).not.toBeNull();
});
