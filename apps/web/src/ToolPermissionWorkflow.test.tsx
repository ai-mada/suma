// @vitest-environment jsdom
import {webcrypto} from 'node:crypto';
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import type {Session,Workflow} from './api';
import {setLocale} from './i18n';
import {WorkflowView} from './Workflow';

// Keep real workflow/run reads and immutable-definition selection. The panel's
// decision and recovery behavior is covered by its own changed-wire tests.
vi.mock('@xyflow/react',()=>({ReactFlow:({children}:{children:ReactNode})=><div>{children}</div>,Background:()=>null,Controls:()=>null,MiniMap:()=>null,Handle:()=>null,Position:{Left:'left',Right:'right'},MarkerType:{ArrowClosed:'arrowclosed'},applyNodeChanges:(_:unknown,nodes:unknown)=>nodes,applyEdgeChanges:(_:unknown,edges:unknown)=>edges}));
vi.mock('elkjs/lib/elk.bundled.js',()=>({default:class{async layout(graph:{children:{id:string}[]}){return {...graph,children:graph.children.map((node,index)=>({...node,x:index*240,y:0}))};}}}));
vi.mock('./run-events',async load=>({...await load<typeof import('./run-events')>(),connectRunEvents:vi.fn(async()=>{})}));
vi.mock('./NodeSchedulingFacts',()=>({NodeSchedulingFacts:()=>null}));
vi.mock('./ToolBindingStatus',()=>({ToolBindingStatus:()=>null}));
vi.mock('./execution-pools/RunRouteFacts',()=>({RunRouteFacts:()=>null}));
vi.mock('./ToolFacts',async load=>({...await load<typeof import('./ToolFacts')>(),ToolExecutionFacts:()=>null}));
vi.mock('./ToolExecutionPermissionPanel',()=>({ToolExecutionPermissionPanel:({protocol,attempt,fence,readOnly}:{protocol:string;attempt:number;fence:number;readOnly?:boolean})=><div data-permission-protocol={protocol} data-attempt={attempt} data-fence={fence} data-readonly={String(!!readOnly)}/> }));

type Protocol='acp-stdio-v1'|'codex-app-server-v1';
const profileHash='a'.repeat(64),stamp='2026-10-31T00:00:00Z';
const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
function definition(protocol:Protocol,version=1):Workflow{
 const binding=protocol==='acp-stdio-v1'
  ?{acp_execution:{runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:profileHash,model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'}}
  :{codex_execution:{protocol,configuration_sha256:profileHash,catalog_sha256:profileHash,model:'fixed-model',reasoning_effort:'low',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',binding_sha256:profileHash}};
 return {id:'workflow-a',name:'Original task execution',version,nodes:[{id:'develop',name:'Develop',type:'agent',command:['tool-command'],config:{tool_profile_id:'profile-a',tool_profile_sha256:profileHash,...binding}}],edges:[]};
}
function execution(){return {id:'run-a',workflow_id:'workflow-a',workflow_name:'Original task execution',workflow_version:1,version:3,state:'running',created_at:stamp,updated_at:stamp,nodes:[{id:'develop',name:'Develop',type:'agent',state:'running',attempt:2,fence:3,worker_id:'worker-a',tool_profile_id:'profile-a',tool_profile_sha256:profileHash,command:['tool-command']}]};}
let host:HTMLDivElement,root:Root|undefined,currentDefinition:Workflow,currentRun:ReturnType<typeof execution>,reads:string[],writes:number;
beforeEach(async()=>{
 vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener(){},removeEventListener(){},addListener(){},removeListener(){}}));
 await setLocale('en-US',{persist:false});sessionStorage.clear();localStorage.clear();reads=[];writes=0;
 host=document.createElement('div');document.body.append(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');reads.push(path+url.search);
  if(options?.body){writes++;throw Error('Opening permissions must not mutate execution');}
  if(path==='/workflows')return Response.json({items:[{...currentDefinition,version:2}],total:1});
  if(path==='/workflows/workflow-a')return Response.json(currentDefinition);
  if(path==='/runs')return Response.json({items:[currentRun],total:1});
  if(path==='/runs/run-a')return Response.json(currentRun);
  return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function mount(attemptId?:string){
 if(root)await act(async()=>root?.unmount());
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><WorkflowView session={session} project="project-a" runId="run-a" nodeId="develop" attemptId={attemptId} onNode={()=>{}} onRun={()=>{}} onRefresh={()=>{}} refresh={0}/></ApiViewProvider>);});
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,150));});
 for(let n=0;n<5;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
}
it.each<Protocol>(['acp-stdio-v1','codex-app-server-v1'])('connects the original %s run to the shared permission panel and retires mismatched profile facts',async protocol=>{
 currentDefinition=definition(protocol);currentRun=execution();await mount();
 const panel=host.querySelector('[data-permission-protocol]');
 expect(panel?.getAttribute('data-permission-protocol')).toBe(protocol);
 expect(panel?.getAttribute('data-attempt')).toBe('2');expect(panel?.getAttribute('data-fence')).toBe('3');
 expect(panel?.getAttribute('data-readonly')).toBe('false');
 expect(reads).toContain('/workflows/workflow-a?project_id=project-a&version=1');expect(writes).toBe(0);
 await mount('1');expect(host.querySelector('[data-permission-protocol]')?.getAttribute('data-readonly')).toBe('true');expect(writes).toBe(0);
 currentRun=execution();currentRun.nodes[0].tool_profile_sha256='b'.repeat(64);await mount();
 expect(host.querySelector('[data-permission-protocol]')).toBeNull();expect(writes).toBe(0);
});
