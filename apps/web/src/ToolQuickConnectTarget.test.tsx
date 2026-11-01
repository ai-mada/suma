// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ToolQuickConnect,type ToolQuickConnectProps,type ToolConnectDraft} from './ToolQuickConnect';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import type {AdapterDescriptor} from './AdapterConnections';
import type {IdentityRead,SharedIdentityRead} from './execution-pools/ToolIdentity';
import type {ToolAction} from './tool-hooks';

const native=vi.hoisted(()=>({props:undefined as undefined|{onConnected:(value:any)=>void;onBlockedChange?:(value:boolean)=>void;onRegistrationReadyChange?:(value:boolean)=>void;onGuide?:()=>void},kind:''}));
vi.mock('./DesktopToolConnector',()=>({DesktopToolConnector:(props:any)=>{native.props=props;native.kind='generic';return <div data-native-fixture/>;}}));
vi.mock('./CodexLocalConnector',()=>({CodexLocalConnector:(props:any)=>{native.props=props;native.kind='codex';return <div data-native-fixture/>;}}));
vi.mock('./OpenCodeLocalConnector',()=>({OpenCodeLocalConnector:(props:any)=>{native.props=props;native.kind='opencode';return <div data-native-fixture/>;}}));
vi.mock('./QoderLocalConnector',()=>({QoderLocalConnector:(props:any)=>{native.props=props;native.kind='acp';return <div data-native-fixture/>;}}));
const project='1'.repeat(32),workerA='desktop-'+'a'.repeat(32),workerB='desktop-'+'b'.repeat(32),session={user:{id:'2'.repeat(32),name:'Owner',email:'owner@example.test'},organization:{id:'3'.repeat(32),name:'Team'}};
const hash=(value:string)=>value.repeat(64),proof={configuration_sha256:hash('c'),executable_sha256:hash('d'),tool_version:'reviewed-cli',catalog_sha256:hash('e')};
const identity=(ids:string[])=>({loading:false,error:'',data:{items:ids.map(worker_id=>({worker_id,status:'active'}))},read:vi.fn()}) as unknown as IdentityRead;
function descriptor(kind:string):AdapterDescriptor{return {id:kind==='generic'?'codex-jsonl':kind==='codex'?'codex-app-server':kind==='opencode'?'opencode-sdk':'qoder-acp',product:kind==='opencode'?'OpenCode':kind==='acp'?'Qoder':'Codex',entrypoint_kind:kind==='codex'?'app_server':kind==='opencode'?'sdk':'cli',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile',adapter:kind==='opencode'?'opencode':kind==='acp'?'qoder':'codex',protocol:kind==='codex'?'codex-app-server-v1':kind==='opencode'?'opencode-sdk-v1':kind==='acp'?'acp-stdio-v1':'codex-jsonl-v1',capabilities:[],source_refs:[],limitations:[]};}
function observation(adapter:AdapterDescriptor,worker:string){
 const profile={command_id:worker===workerA?'command-a':'command-b',adapter:adapter.adapter,protocol:adapter.protocol,profile_sha256:worker===workerA?hash('a'):hash('b'),executable_sha256:proof.executable_sha256,tool_version:adapter.id==='opencode-sdk'?'suma-opencode-runtime 1.0.0':proof.tool_version,read_only:false};
 if(adapter.id==='codex-app-server')return {profile,available:true,reason:'',codex_catalog:{protocol:'codex-app-server-v1',configuration_sha256:proof.configuration_sha256,catalog_sha256:proof.catalog_sha256,models:[{model:'synthetic-model',reasoning_efforts:['high'],default_reasoning_effort:'high'}]}};
 if(adapter.id==='opencode-sdk')return {profile:{...profile,workspace_tools_version:'attempt-workspace-v1',opencode_execution:{sdk_version:'1.19.4',server_version:'1.19.4',configuration_sha256:proof.configuration_sha256,provider_id:'synthetic',model:'vendor/model',endpoint_origin:'https://models.example.test',budget_mode:'observe_only',identity_evidence:'configuration_ack_only'}},available:true,reason:''};
 if(adapter.id==='qoder-acp')return {profile:{...profile,acp_execution:{runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:proof.configuration_sha256,model:'synthetic-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'}},available:true,reason:''};
 return {profile,available:true,reason:''};
}
const unavailableReason='Workspace configuration missing: <script>not executable</script>';
let root:Root,host:HTMLDivElement,adapter:AdapterDescriptor,props:ToolQuickConnectProps,requests:{path:string;body?:Record<string,unknown>}[],drafts:ToolConnectDraft[],includeUnavailable:boolean;
beforeEach(async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);requests=[];drafts=[];includeUnavailable=false;
 props={project,workers:identity([workerA,workerB]),sharedWorkers:identity([]) as unknown as SharedIdentityRead,action:{run:vi.fn(async(_path:string,body:Record<string,unknown>)=>({...body,id:'saved',version:1,state:'paused',available:false})),clearError:vi.fn(),busy:false,pending:undefined,conflict:false} as unknown as ToolAction,onRegistered:vi.fn(),onEnable:vi.fn(),onAdvanced:vi.fn(),onWorker:vi.fn(),onDraftChange:value=>drafts.push(value)};
 vi.stubGlobal('fetch',vi.fn(async(input:string,init:RequestInit={})=>{const path=String(input).replace('/api/v1',''),body=init.body?JSON.parse(String(init.body)):undefined;requests.push({path,body});if(path.startsWith('/adapter-catalog'))return Response.json({project_id:project,as_of:'2026-11-01T08:00:20Z',items:[adapter]});if(path==='/tool-profiles/matches')return Response.json({...body,items:[],total:0});if(path.startsWith('/tool-discovery')){const worker=new URL(path,'https://example.test').searchParams.get('worker_id')!,actual=observation(adapter,worker),items=includeUnavailable?[actual,{...actual,profile:{...actual.profile,command_id:'blocked-known',profile_sha256:hash('e')},available:false,reason:unavailableReason}]:[actual];return Response.json({project_id:project,worker_id:worker,state:'fresh',as_of:'2026-11-01T08:00:20Z',observed_at:'2026-11-01T08:00:00Z',expires_at:'2026-11-01T08:01:00Z',items});}throw Error('Unexpected path '+path);}));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){await act(async()=>{for(let i=0;i<24;i++)await Promise.resolve();});}
async function render(key:string){await act(async()=>root.render(<ApiViewProvider session={session}><ToolQuickConnect key={key} {...props} adapter={adapter}/></ApiViewProvider>));await settle();}
async function selectWorker(worker:string){const field=host.querySelector<HTMLSelectElement>('[data-quick-worker]')!;expect(field).not.toBeNull();expect(field.disabled).toBe(false);await act(async()=>{Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype,'value')!.set!.call(field,worker);field.dispatchEvent(new Event('change',{bubbles:true}));});await settle();}
async function advanced(worker:string,selector='[data-quick-advanced]'){const button=host.querySelector<HTMLButtonElement>(selector);expect(button).not.toBeNull();expect(button!.disabled).toBe(false);await act(async()=>button!.click());await settle();expect(props.onAdvanced).toHaveBeenLastCalledWith(adapter,worker);}
async function nativeStatus(worker:string){const current=native.props!,value=native.kind==='codex'?{worker_id:worker,...proof}:native.kind==='opencode'?{worker_id:worker,configuration_sha256:proof.configuration_sha256,executable_sha256:proof.executable_sha256,tool_version:'1.19.4',provider_id:'synthetic',model:'vendor/model',endpoint_origin:'https://models.example.test'}:worker;await act(async()=>{current.onRegistrationReadyChange?.(true);current.onConnected(value);});await settle();}
const reads=()=>requests.filter(item=>item.path.startsWith('/tool-discovery'));

it('T194 keeps explicit and original targets across native status callbacks, permits an explicit switch, and preserves unpinned and unknown-proof flows',async()=>{
 for(const [index,kind] of ['generic','codex','opencode','acp'].entries()){
  adapter=descriptor(kind);includeUnavailable=kind==='generic';props={...props,workers:identity([workerA,workerB]),preferredWorkerID:index%2===0?workerA:undefined,draft:{adapterID:adapter.id,workerID:index%2===0?'':workerA,commandID:index%2===0?'':'command-a',name:'Retained authored name',nameEdited:true}};requests=[];drafts=[];await render(kind);
  expect(reads()).toHaveLength(1);expect(drafts.at(-1)?.workerID).toBe(workerA);const before=reads().length;
  await advanced(workerA);if(includeUnavailable){expect(host.querySelector('[data-quick-unavailable="blocked-known"]')).not.toBeNull();expect(host.querySelector('[data-quick-unavailable-reason="blocked-known"]')?.textContent).toBe(unavailableReason);expect(host.querySelector('script')).toBeNull();await advanced(workerA,'[data-quick-configure="blocked-known"]');expect(props.action.run).not.toHaveBeenCalled();}
  await nativeStatus(workerB);expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(workerA);expect(reads()).toHaveLength(before);expect(drafts.at(-1)).toMatchObject({workerID:workerA,commandID:'command-a',name:'Retained authored name'});expect(props.workers.read).not.toHaveBeenCalled();expect(host.querySelector('[data-quick-native-target-mismatch]')?.textContent).toContain(workerB);
  await advanced(workerA);
  const locate=host.querySelector<HTMLButtonElement>('[data-quick-target-picker]')!;await act(async()=>locate.click());expect(document.activeElement).toBe(host.querySelector('[data-quick-worker]'));
  await selectWorker(workerB);await nativeStatus(workerB);expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(workerB);expect(host.querySelector('[data-quick-native-target-mismatch]')).toBeNull();expect(drafts.at(-1)).toMatchObject({workerID:workerB,commandID:'command-b',name:'Retained authored name'});const switchedReads=reads().length;
  await advanced(workerB);if(kind==='generic'||kind==='opencode'){expect(native.props!.onGuide).toBeDefined();await act(async()=>native.props!.onGuide!());expect(props.onAdvanced).toHaveBeenLastCalledWith(adapter,workerB);}if(includeUnavailable)await advanced(workerB,'[data-quick-configure="blocked-known"]');
  await nativeStatus(workerA);expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(workerB);expect(reads()).toHaveLength(switchedReads);expect(drafts.at(-1)?.commandID).toBe('command-b');expect(props.action.run).not.toHaveBeenCalled();
  await advanced(workerB);
  await act(async()=>native.props!.onBlockedChange?.(true));await settle();expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(true);await act(async()=>native.props!.onBlockedChange?.(false));await settle();await nativeStatus(workerB);expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(false);
 }
 adapter=descriptor('generic');props={...props,workers:identity([workerA]),preferredWorkerID:undefined,draft:undefined};requests=[];await render('unpinned');await nativeStatus(workerB);
 expect(reads()).toHaveLength(1);expect(host.querySelector('[data-quick-register]')).toBeNull();expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe('');
 await act(async()=>native.props!.onBlockedChange?.(true));await settle();expect(host.querySelector<HTMLSelectElement>('[data-quick-tool]')?.disabled).toBe(true);await act(async()=>native.props!.onBlockedChange?.(false));await settle();
 props.workers=identity([workerA,workerB]);await render('unpinned');expect(host.querySelector<HTMLSelectElement>('[data-quick-worker]')?.value).toBe(workerB);expect(reads().at(-1)?.path).toContain('worker_id='+workerB);
 const register=host.querySelector<HTMLButtonElement>('[data-quick-register]')!;expect(register.disabled).toBe(false);await act(async()=>register.click());await settle();expect(props.action.run).toHaveBeenCalledOnce();expect(props.onRegistered).toHaveBeenCalledOnce();const originalReads=reads().length;await nativeStatus(workerA);expect(reads()).toHaveLength(originalReads);expect(host.querySelector('[data-quick-enable]')).not.toBeNull();
});
