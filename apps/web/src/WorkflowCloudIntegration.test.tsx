// @vitest-environment jsdom
import {webcrypto} from 'node:crypto';
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CloudNodeView,WorkBuddyConnection} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import type {Run,Session,Workflow,WNode} from './api';
import {setLocale} from './i18n';
import {cloudRequestDigest} from './workbuddy-cloud-task-model';
import {WorkflowView} from './Workflow';

// Keep the real run loading, frozen definition selection, node inspector and
// WorkBuddy workspace. Canvas geometry and unrelated fact panels have their
// own tests; the live notification adapter is not an SSE claim in this fixture.
vi.mock('@xyflow/react',()=>({ReactFlow:({children}:{children:ReactNode})=><div>{children}</div>,Background:()=>null,Controls:()=>null,MiniMap:()=>null,Handle:()=>null,Position:{Left:'left',Right:'right'},MarkerType:{ArrowClosed:'arrowclosed'},applyNodeChanges:(_:unknown,nodes:unknown)=>nodes,applyEdgeChanges:(_:unknown,edges:unknown)=>edges}));
vi.mock('elkjs/lib/elk.bundled.js',()=>({default:class{async layout(graph:{children:{id:string}[]}){return {...graph,children:graph.children.map((node,index)=>({...node,x:index*240,y:0}))};}}}));
vi.mock('./run-events',async importOriginal=>{const actual=await importOriginal<typeof import('./run-events')>();return {...actual,connectRunEvents:vi.fn(async()=>{})};});
vi.mock('./NodeSchedulingFacts',()=>({NodeSchedulingFacts:()=>null}));
vi.mock('./ToolBindingStatus',()=>({ToolBindingStatus:()=>null}));
vi.mock('./execution-pools/RunRouteFacts',()=>({RunRouteFacts:()=>null}));
vi.mock('./ToolFacts',async importOriginal=>({...await importOriginal<typeof import('./ToolFacts')>(),ToolExecutionFacts:()=>null}));

const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]},stamp='2026-10-30T00:00:00Z';
const cloudNode=():WNode=>({id:'cloud',name:'Original cloud implementation',type:'external_workflow',config:{plugin:'workbuddy-cloud',workflow:'workbuddy-cloud/v1',workbuddy_cloud:{connection_id:'connection-a',grant_generation:1,request:{prompt:'Exact original cloud request',name:'Approved task'}}}});
const other:WNode={id:'other',name:'Another task',type:'command',command:['true']};
const workflow=(version=1,cloud=true):Workflow=>({id:'flow-a',name:'Frozen workflow',version,nodes:[cloud?cloudNode():{...cloudNode(),name:'WorkBuddy misleading name',config:{plugin:'another-provider',workflow:'workbuddy-cloud/v1'}},other],edges:[]});
const makeRun=(id='run-a'):Run=>({id,workflow_id:'flow-a',workflow_name:'Frozen workflow',workflow_version:1,version:3,state:'running',nodes:[cloudNode(),other].map(({id,name,type})=>({id,name,type,state:'pending',attempt:0})),created_at:stamp,updated_at:stamp});
let host:HTMLDivElement,root:Root|undefined,latest:Workflow,historical:Workflow,currentRun:Run,source:CloudNodeView,reads:string[];
async function sourceView(project='project-a',run='run-a'):Promise<CloudNodeView>{const binding={connection_id:'connection-a',grant_generation:1,request:{prompt:'Exact original cloud request',name:'Approved task'}};return {project_id:project,run_id:run,node_id:'cloud',run_version:3,node_spec_sha256:'a'.repeat(64),request_sha256:await cloudRequestDigest(binding.request),binding,allowed_actions:['delegate']};}
function account(project:string):WorkBuddyConnection{return {id:'connection-a',project_id:project,owner_id:'user-a',version:3,client_id:'client-a',open_id:'Private owned account',scopes:['user.task.invokable','user.task.readable'],state:'connected',grant_generation:1,token_generation:1,expires_at:'2030-01-01T00:00:00Z',created_at:stamp,updated_at:stamp,last_error:'',allowed_actions:['refresh','disconnect']};}
beforeEach(async()=>{
 vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener(){},removeEventListener(){},addListener(){},removeListener(){}}));
 await setLocale('en-US',{persist:false});sessionStorage.clear();latest=workflow();historical=workflow();currentRun=makeRun();source=await sourceView();reads=[];host=document.createElement('div');document.body.append(host);
});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();vi.restoreAllMocks();});
async function settle(){for(let n=0;n<5;n++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
async function render(project='project-a',run='run-a',node='cloud'){await act(async()=>{root??=createRoot(host);root.render(<ApiViewProvider session={session}><WorkflowView session={session} project={project} runId={run} nodeId={node} onNode={()=>{}} onRun={()=>{}} onRefresh={()=>{}} refresh={0}/></ApiViewProvider>);});await act(async()=>{await new Promise(resolve=>setTimeout(resolve,150));});await settle();}
function transport(override?:(url:URL)=>Promise<Response|undefined>){vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
 const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');reads.push(path+url.search);
 if(options?.body)throw Error('Run mounting must not send mutations');
 const replaced=await override?.(url);if(replaced)return replaced;
 if(path==='/workflows')return Response.json({items:[latest],total:1});
 if(path==='/workflows/flow-a')return Response.json(historical);
 if(path==='/runs')return Response.json({items:[currentRun],total:1});
 if(path==='/runs/run-a'||path==='/runs/run-b')return Response.json({...currentRun,id:path.split('/')[2]});
 if(path==='/tasks')return Response.json({items:[],total:0});
 if(path==='/workbuddy-cloud-nodes')return Response.json(source);
 if(path==='/workbuddy-connections/connection-a')return Response.json(account(url.searchParams.get('project_id')||''));
 if(path.startsWith('/workbuddy'))throw Error('Unexpected cloud endpoint '+path);
 return Response.json({items:[],total:0});
 }));}

it('mounts the real cloud workspace in the selected original run node without a mutation',async()=>{
 transport();await render();expect(host.querySelector('.workbuddy-cloud-task')).not.toBeNull();expect(host.textContent).toContain('Exact original cloud request');
 expect(Object.hasOwn(currentRun.nodes[0],'config')).toBe(false);expect(reads).toContain('/workbuddy-cloud-nodes?project_id=project-a&run_id=run-a&node_id=cloud');
});

it('keeps another external provider unchanged even when its node is named WorkBuddy',async()=>{
 latest=workflow(1,false);historical=latest;transport();await render();expect(host.querySelector('.workbuddy-cloud-task')).toBeNull();expect(reads.some(path=>path.startsWith('/workbuddy'))).toBe(false);
});

it('fetches the exact historical definition instead of replacing an old run with the latest version',async()=>{
 latest=workflow(2,false);historical=workflow(1,true);transport();await render();
 expect(reads).toContain('/workflows/flow-a?project_id=project-a&version=1');expect(host.querySelector('.workbuddy-cloud-task')).not.toBeNull();expect(host.textContent).toContain('Exact original cloud request');
});

it('retires late definition and cloud reads after selected-node or project context changes',async()=>{
 latest=workflow(2,false);let releaseDefinition!:(response:Response)=>void,releaseSource!:(response:Response)=>void;
 const definitionReply=new Promise<Response>(resolve=>{releaseDefinition=resolve;}),sourceReply=new Promise<Response>(resolve=>{releaseSource=resolve;});let definitionReleased=false;
 transport(async url=>{
  if(url.pathname.endsWith('/workflows/flow-a')&&!definitionReleased)return definitionReply;
  if(url.pathname.endsWith('/workbuddy-cloud-nodes')&&url.searchParams.get('project_id')==='project-a')return sourceReply;
 });
 await render();await render('project-a','run-a','other');definitionReleased=true;await act(async()=>releaseDefinition(Response.json(historical)));await settle();
 expect(host.querySelector('.workbuddy-cloud-task')).toBeNull();expect(reads.some(path=>path.startsWith('/workbuddy-cloud-nodes'))).toBe(false);
 await render('project-a','run-a','cloud');expect(reads).toContain('/workbuddy-cloud-nodes?project_id=project-a&run_id=run-a&node_id=cloud');
 currentRun=makeRun('run-b');await render('project-b','run-b','other');await act(async()=>releaseSource(Response.json(source)));await settle();
 expect(host.querySelector('.workbuddy-cloud-task')).toBeNull();expect(reads.some(path=>path==='/workbuddy-connections/connection-a?project_id=project-a')).toBe(false);
});

it('does not borrow a previous projects same-ID workflow revision while the new project list is still loading',async()=>{
 let releaseList!:(response:Response)=>void;const listReply=new Promise<Response>(resolve=>{releaseList=resolve;});
 transport(async url=>{
  if(url.searchParams.get('project_id')!=='project-b')return;
  if(url.pathname.endsWith('/workflows'))return listReply;
  if(url.pathname.endsWith('/workflows/flow-a'))return Response.json(workflow(1,false));
 });
 await render();expect(host.querySelector('.workbuddy-cloud-task')).not.toBeNull();
 currentRun=makeRun('run-b');await render('project-b','run-b','cloud');
 await act(async()=>releaseList(Response.json({items:[workflow(1,false)],total:1})));await settle();
 expect(host.querySelector('.workbuddy-cloud-task')).toBeNull();
 expect(reads).toContain('/workflows/flow-a?project_id=project-b&version=1');
 expect(reads.some(path=>path.startsWith('/workbuddy-cloud-nodes?project_id=project-b'))).toBe(false);
});
