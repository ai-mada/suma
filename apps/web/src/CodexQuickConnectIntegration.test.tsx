// @vitest-environment jsdom
import {act,useMemo} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider,useApi} from './api-context';
import {ToolQuickConnect,type ToolQuickConnectProps} from './ToolQuickConnect';
import type {CodexLocalConnectorProps} from './CodexLocalConnector';
import type {DesktopCodexConnectedProof} from './desktop-codex-connector-model';
import type {AdapterDescriptor} from './AdapterConnections';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolObservation,ToolProfile} from './tool-model';
import {ToolWriteRecovery} from './tool-write';
import {useToolAction} from './tool-hooks';
import {setLocale} from './i18n';

const leaf=vi.hoisted(()=>({latest:undefined as CodexLocalConnectorProps|undefined,proof:undefined as DesktopCodexConnectedProof|undefined}));
vi.mock('./CodexLocalConnector',()=>({CodexLocalConnector:(props:CodexLocalConnectorProps)=>{
 leaf.latest=props;
 return <div data-native-connector><button data-native-connected type="button" disabled={props.disabled} onClick={()=>props.onConnected(leaf.proof!)}>Connected fixture</button><button data-native-block type="button" onClick={()=>props.onBlockedChange?.(true)}>Pending fixture</button><button data-native-reconcile type="button" disabled={props.disabled} onClick={()=>props.onBlockedChange?.(false)}>Reconcile fixture</button><button data-native-registration-stale type="button" onClick={()=>props.onRegistrationReadyChange?.(false)}>Draft changed fixture</button><button data-native-registration-ready type="button" onClick={()=>props.onRegistrationReadyChange?.(true)}>Current configuration fixture</button></div>;
}}));

const project='1'.repeat(32),oldWorker='desktop-'+'a'.repeat(32),newWorker='desktop-'+'b'.repeat(32);
const session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const native:AdapterDescriptor={id:'codex-app-server',product:'Codex',entrypoint_kind:'app_server',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:'codex',protocol:'codex-app-server-v1',capabilities:[],source_refs:[],limitations:[]};
const proof:DesktopCodexConnectedProof={worker_id:newWorker,tool_version:'codex-cli fixture175',configuration_sha256:'c'.repeat(64),executable_sha256:'d'.repeat(64),catalog_sha256:'e'.repeat(64)};
function observation(command='native-correct'):ToolObservation{return {profile:{command_id:command,adapter:'codex',protocol:'codex-app-server-v1',profile_sha256:'f'.repeat(64),executable_sha256:proof.executable_sha256,tool_version:proof.tool_version,read_only:false},available:true,reason:'',codex_catalog:{protocol:'codex-app-server-v1',configuration_sha256:proof.configuration_sha256,catalog_sha256:proof.catalog_sha256,models:[{model:'model-a',reasoning_efforts:['high'],default_reasoning_effort:'high'}]}};}
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
let root:Root,host:HTMLDivElement,inputs:Omit<ToolQuickConnectProps,'action'>,paths:string[],inventory:ToolObservation[],posts:{path:string;body:Record<string,unknown>;headers:Headers}[];
function Consumer(){const api=useApi(),recovery=useMemo(()=>new ToolWriteRecovery('codex-quick-parent',sessionStorage,api),[api]),action=useToolAction(recovery);return <ToolQuickConnect {...inputs} action={action}/>;}
async function settle(){await act(async()=>{for(let index=0;index<24;index++)await Promise.resolve();});}
async function render(){await act(async()=>root.render(<ApiViewProvider session={session}><Consumer/></ApiViewProvider>));await settle();}
async function click(selector:string){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();await act(async()=>button!.click());await settle();}
async function selectNative(){const select=host.querySelector<HTMLSelectElement>('[data-quick-tool]');expect(select).not.toBeNull();await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(select,native.id);select!.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}

beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);sessionStorage.clear();leaf.latest=undefined;leaf.proof=proof;paths=[];posts=[];inventory=[observation()];host=document.createElement('div');document.body.append(host);root=createRoot(host);
 inputs={project,workers:identity([oldWorker]),sharedWorkers:identity([]) as unknown as SharedIdentityRead,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn()};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init?:RequestInit)=>{
  const path=String(input).replace('/api/v1','');paths.push(path);
  if(init?.method==='POST'){
   const body=JSON.parse(String(init.body)) as Record<string,unknown>;posts.push({path,body,headers:new Headers(init.headers)});
   const receipt:ToolProfile={id:'native-profile',project_id:project,worker_id:String(body.worker_id),name:String(body.name),profile:body.profile as ToolProfile['profile'],version:1,state:'paused',enabled_by:'',created_at:'2026-10-31T08:00:00Z',updated_at:'2026-10-31T08:00:00Z',available:false,reason:''};
   return Response.json(receipt);
  }
  if(path.startsWith('/adapter-catalog?'))return Response.json({project_id:project,as_of:'2026-10-31T08:00:20Z',items:[native]});
  if(path.startsWith('/tool-discovery?')){const worker=new URL(path,'https://example.test').searchParams.get('worker_id');return Response.json({project_id:project,worker_id:worker,state:'fresh',as_of:'2026-10-31T08:00:20Z',observed_at:'2026-10-31T08:00:00Z',expires_at:'2026-10-31T08:01:00Z',items:worker===newWorker?inventory:[observation('old-worker-command')]});}
  throw Error('Unexpected integration transport path');
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();sessionStorage.clear();vi.unstubAllGlobals();vi.restoreAllMocks();});

it('keeps an unresolved native worker proof from falling back to the old sole worker and lets the blocked child reconcile',async()=>{
 await render();await selectNative();expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe('old-worker-command');
 await click('[data-native-block]');expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(true);expect(leaf.latest?.disabled).toBe(false);expect(host.querySelector<HTMLButtonElement>('[data-native-reconcile]')?.disabled).toBe(false);
 await click('[data-native-reconcile]');expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(false);
 const oldReads=paths.filter(path=>path.includes('/tool-discovery?')).length;
 await click('[data-native-connected]');
 expect(inputs.workers.read).toHaveBeenCalledOnce();expect(inputs.sharedWorkers.read).toHaveBeenCalledOnce();
 expect(host.querySelector('[data-quick-register]')).toBeNull();expect(host.querySelector('[data-quick-name]')).toBeNull();expect(paths.filter(path=>path.includes('/tool-discovery?'))).toHaveLength(oldReads);expect(posts).toEqual([]);expect(inputs.onRegistered).not.toHaveBeenCalled();
});

it('registers paused only from fresh native inventory matching all four connected facts',async()=>{
 const correct=observation(),config=observation('wrong-config'),executable=observation('wrong-executable'),version=observation('wrong-version'),catalog=observation('wrong-catalog');
 config.codex_catalog={...config.codex_catalog!,configuration_sha256:'0'.repeat(64)};executable.profile={...executable.profile,executable_sha256:'0'.repeat(64)};version.profile={...version.profile,tool_version:'another-reviewed-version'};catalog.codex_catalog={...catalog.codex_catalog!,catalog_sha256:'0'.repeat(64)};
 inventory=[config,executable,version,catalog];await render();await selectNative();await click('[data-native-connected]');inputs.workers=identity([oldWorker,newWorker]);await render();
 expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(newWorker);expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);expect(host.querySelector('[data-quick-name]')).toBeNull();expect(posts).toEqual([]);
 inventory=[...inventory,correct];await click('.tool-quick-inventory-heading button');expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe(correct.profile.command_id);expect(host.querySelector('[data-quick-configuration]')).toBeNull();
 await click('[data-native-registration-stale]');expect(host.querySelector('[data-quick-name]')).toBeNull();expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(true);expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(false);expect(leaf.latest?.disabled).toBe(false);await click('[data-quick-register]');expect(posts).toEqual([]);
 await click('[data-native-registration-ready]');expect(host.querySelector<HTMLInputElement>('[data-quick-name]')?.value).toBe(correct.profile.command_id);expect(host.querySelector<HTMLButtonElement>('[data-quick-register]')?.disabled).toBe(false);
 await click('[data-quick-register]');expect(posts).toHaveLength(1);expect(posts[0].path).toBe('/tool-profiles');expect(posts[0].body).toMatchObject({project_id:project,worker_id:newWorker,name:correct.profile.command_id,profile:correct.profile});expect(typeof posts[0].body.idempotency_key).toBe('string');expect(posts[0].headers.get('X-Suma-Expected-User')).toBe(session.user.id);expect(posts[0].headers.get('X-Suma-Expected-Organization')).toBe(session.organization.id);
 expect(inputs.onRegistered).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({worker_id:newWorker,state:'paused',profile:correct.profile}));expect(inputs.onEnable).not.toHaveBeenCalled();expect(host.querySelector('[data-quick-enable]')).not.toBeNull();
});
