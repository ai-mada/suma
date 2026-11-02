// @vitest-environment jsdom
import {act,useMemo} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider,useApi} from './api-context';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import type {ClaudeSDKLocalConnectorProps} from './ClaudeSDKLocalConnector';
import type {DesktopClaudeSDKConnectedProof} from './desktop-claude-sdk-connector-model';
import type {AdapterDescriptor} from './AdapterConnections';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolObservation,ToolProfile} from './tool-model';
import {ToolWriteRecovery} from './tool-write';
import {useToolAction} from './tool-hooks';
import {setLocale} from './i18n';

const leaf=vi.hoisted(()=>({current:undefined as ClaudeSDKLocalConnectorProps|undefined,proof:undefined as DesktopClaudeSDKConnectedProof|undefined}));
vi.mock('./ClaudeSDKLocalConnector',()=>({ClaudeSDKLocalConnector:(props:ClaudeSDKLocalConnectorProps)=>{leaf.current=props;return <div><button type="button" data-claude-sdk-connected onClick={()=>{props.onRegistrationReadyChange?.(true);props.onConnected(leaf.proof!);}}>Fixture SDK connected</button><button type="button" data-claude-sdk-dirty onClick={()=>props.onRegistrationReadyChange?.(false)}>Fixture draft changed</button><button type="button" data-claude-sdk-ready onClick={()=>props.onRegistrationReadyChange?.(true)}>Fixture exact draft</button></div>;}}));
const project='1'.repeat(32),worker='desktop-'+'4'.repeat(32),oldWorker='desktop-'+'5'.repeat(32);
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const adapter:AdapterDescriptor={id:'claude-code-sdk',product:'Claude Agent SDK',entrypoint_kind:'sdk',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'claude',protocol:'claude-sdk-v1',capabilities:[],source_refs:[],limitations:[]};
const capability={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-4-6',configuration_sha256:'b'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
const proof:DesktopClaudeSDKConnectedProof={worker_id:worker,model:capability.model,credential_env_ref:'TEAM_ANTHROPIC_KEY',tool_version:'2.1.314',executable_sha256:'a'.repeat(64),runtime_executable_sha256:'d'.repeat(64),configuration_sha256:capability.configuration_sha256,claude_sdk_execution:capability};
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
function observation(id:string):ToolObservation{return {profile:{command_id:id,adapter:'claude',protocol:'claude-sdk-v1',profile_sha256:'c'.repeat(64),executable_sha256:proof.runtime_executable_sha256,tool_version:'2.1.314',read_only:false,user_input_protocol:'claude-sdk-input-v1',claude_sdk_execution:{...capability}},available:true,reason:''};}
let root:Root,host:HTMLDivElement,props:Omit<ToolQuickConnectProps,'action'>,inventory:ToolObservation[],posts:{body:Record<string,unknown>;headers:Headers}[],reads:string[];
function Consumer(){const api=useApi(),recovery=useMemo(()=>new ToolWriteRecovery('claude-sdk-quick-parent',sessionStorage,api),[api]),action=useToolAction(recovery);return <ToolQuickConnect {...props} action={action}/>;}
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function render(){await act(async()=>root.render(<ApiViewProvider session={session}><Consumer/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();host=document.createElement('div');document.body.append(host);root=createRoot(host);leaf.proof=proof;posts=[];reads=[];
 const wrongNode=observation('wrong-node');wrongNode.profile.executable_sha256=proof.executable_sha256;
 const wrongConfig=observation('wrong-config');wrongConfig.profile.claude_sdk_execution={...capability,configuration_sha256:'e'.repeat(64)};
 const wrongModel=observation('wrong-model');wrongModel.profile.claude_sdk_execution={...capability,model:'claude-opus-4-6'};
 inventory=[observation('correct-sdk'),wrongNode,wrongConfig,wrongModel];
 props={project,workers:identity([oldWorker]),sharedWorkers:identity([]) as unknown as SharedIdentityRead,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{const url=new URL(String(input),'https://example.test');if(init?.method==='POST'){const body=JSON.parse(String(init.body));if(url.pathname.endsWith('/tool-profiles/matches'))return Response.json({...body,items:[],total:0});posts.push({body,headers:new Headers(init.headers)});return Response.json({...body,id:'saved-sdk',version:1,state:'paused',available:false,reason:'',enabled_by:'',created_at:'',updated_at:''} satisfies ToolProfile);}
  const actualProject=url.searchParams.get('project_id')!;if(url.pathname.endsWith('/adapter-catalog'))return Response.json({project_id:actualProject,as_of:'2026-11-02T08:00:00Z',items:[adapter]});
  const actualWorker=url.searchParams.get('worker_id');reads.push(String(input));return Response.json({project_id:actualProject,worker_id:actualWorker,state:'fresh',as_of:'2026-11-02T08:00:00Z',observed_at:'2026-11-02T08:00:00Z',expires_at:'2026-11-02T08:01:00Z',items:actualWorker===worker?inventory:[observation('old-worker-sdk')]});}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('T201 registers only fresh exact Claude SDK runtime inventory after explicit readiness and ignores old-scope callbacks',async()=>{
 await render();const select=host.querySelector<HTMLSelectElement>('[data-quick-tool]')!;await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,adapter.id);select.dispatchEvent(new Event('change',{bubbles:true}));});await settle();
 const oldLeaf=leaf.current!;expect(oldLeaf).toBeDefined();expect(posts).toHaveLength(0);expect(props.onEnable).not.toHaveBeenCalled();
 await click('[data-claude-sdk-connected]');expect(host.querySelector('[data-quick-register]')).toBeNull();expect(posts).toHaveLength(0);
 props.workers=identity([oldWorker,worker]);await render();expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('correct-sdk');expect(host.querySelector('[data-quick-configuration]')).toBeNull();expect(posts).toHaveLength(0);
 await click('[data-claude-sdk-dirty]');expect(host.querySelector('[data-quick-register]')?.hasAttribute('disabled')).toBe(true);await click('[data-claude-sdk-ready]');await click('[data-quick-register]');
 expect(posts).toHaveLength(1);expect(posts[0].body).toMatchObject({project_id:project,worker_id:worker,name:'correct-sdk',profile:{executable_sha256:proof.runtime_executable_sha256,tool_version:'2.1.314',claude_sdk_execution:capability}});expect(posts[0].body).not.toHaveProperty('credential_env_ref');expect(posts[0].body.profile).not.toHaveProperty('credential_env_ref');expect(posts[0].body.profile).not.toHaveProperty('executable');expect(posts[0].headers.get('X-Suma-Expected-User')).toBe(session.user.id);expect(props.onRegistered).toHaveBeenCalledOnce();expect(props.onEnable).not.toHaveBeenCalled();expect(host.querySelector('[data-quick-enable]')).not.toBeNull();
 await click('[data-quick-advanced]');expect(props.onAdvanced).toHaveBeenLastCalledWith(adapter,worker);
 props.project='6'.repeat(32);await render();const before=reads.length;await act(async()=>{oldLeaf.onRegistrationReadyChange?.(true);oldLeaf.onConnected(proof);});await settle();expect(reads).toHaveLength(before);expect(posts).toHaveLength(1);expect(props.onEnable).not.toHaveBeenCalled();
});
