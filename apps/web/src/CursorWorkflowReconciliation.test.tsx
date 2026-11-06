// @vitest-environment jsdom
import {webcrypto} from 'node:crypto';
import {act,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {ACPExecutionCapability,ExecutionAttempt,ToolEvidence} from '../../../packages/contracts/src';
import {ApiViewProvider} from './api-context';
import type {Run,Session,Workflow} from './api';
import {setLocale} from './i18n';
import {WorkflowView} from './Workflow';

vi.mock('@xyflow/react',()=>({ReactFlow:({children}:{children:ReactNode})=><div>{children}</div>,Background:()=>null,Controls:()=>null,MiniMap:()=>null,Handle:()=>null,Position:{Left:'left',Right:'right'},MarkerType:{ArrowClosed:'arrowclosed'},applyNodeChanges:(_:unknown,nodes:unknown)=>nodes,applyEdgeChanges:(_:unknown,edges:unknown)=>edges}));
vi.mock('elkjs/lib/elk.bundled.js',()=>({default:class{async layout(graph:{children:{id:string}[]}){return {...graph,children:graph.children.map((node,index)=>({...node,x:index*240,y:0}))};}}}));
vi.mock('./run-events',async load=>({...await load<typeof import('./run-events')>(),connectRunEvents:vi.fn(async()=>{})}));
const historyFixture=vi.hoisted(()=>({attempt:undefined as ExecutionAttempt|undefined}));
vi.mock('./NodeSchedulingFacts',()=>({NodeSchedulingFacts:({renderAttemptContext}:{renderAttemptContext?:(attempt:ExecutionAttempt)=>ReactNode})=>
 historyFixture.attempt?renderAttemptContext?.(historyFixture.attempt):null}));
vi.mock('./ToolInputPanel',()=>({ToolInputPanel:({requestedInputID,attempt,fence,readOnly}:{requestedInputID?:string;attempt:number;fence:number;readOnly?:boolean})=>
 <output data-question={requestedInputID||'current'} data-attempt={attempt} data-fence={fence} data-readonly={readOnly}/>}));
vi.mock('./ToolBindingStatus',()=>({ToolBindingStatus:()=>null}));
vi.mock('./execution-pools/RunRouteFacts',()=>({RunRouteFacts:()=>null}));

const stamp='2026-11-05T00:00:00Z',profileHash='a'.repeat(64),configurationHash='b'.repeat(64);
const capability:ACPExecutionCapability={runtime:'cursor',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:configurationHash,model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'explicit_api_key_env',permission_mode:'human_once'};
const session:Session={user:{id:'user-a',name:'A',email:'a@example.test'},organization:{id:'org-a',name:'Organization'},projects:[]};
const node={id:'develop',name:'Develop',type:'agent',command:['cursor-command']};
const definition:Workflow={id:'workflow-a',name:'Original workflow',version:1,nodes:[{...node,config:{tool_profile_id:'profile-a',tool_profile_sha256:profileHash,acp_execution:capability}}],edges:[]};
const evidence:ToolEvidence={
 profile:{command_id:'cursor-command',adapter:'cursor',protocol:'acp-stdio-v1',profile_sha256:profileHash,executable_sha256:'c'.repeat(64),tool_version:'Cursor fixture',read_only:false,acp_execution:capability},
 terminal:'unknown',session_id:'cursor-session-a',event_count:1,usage_state:'unreported',
 acp_execution:{configuration_sha256:configurationHash,requested_model:capability.model,selected_model:capability.model,model_selection_acknowledged:true,session_id:'cursor-session-a',protocol_version:1,prompt_started:true,prompt_completed:false,tools_complete:false,permissions_requested:0,permissions_denied:0,cancel_requested:true,host_process_stopped:true,host_process_tree_stopped:false,host_stopped_after_rpc:false},
};
const originalRun:Run={id:'run-a',workflow_id:'workflow-a',workflow_name:definition.name,workflow_version:1,version:3,state:'cancelling',created_at:stamp,updated_at:stamp,nodes:[{...node,state:'reconciling',attempt:2,fence:3,worker_id:'worker-a',tool_profile_id:'profile-a',tool_profile_sha256:profileHash,tool_evidence:evidence}]};
let host:HTMLDivElement,root:Root,currentRun:Run,writes:{path:string;body:Record<string,unknown>}[];
beforeEach(async()=>{
 vi.stubGlobal('crypto',webcrypto);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener(){},removeEventListener(){},addListener(){},removeListener(){}}));
 await setLocale('en-US',{persist:false});
 sessionStorage.clear();localStorage.clear();currentRun=structuredClone(originalRun);writes=[];historyFixture.attempt=undefined;
 host=document.createElement('div');document.body.append(host);root=createRoot(host);
 vi.stubGlobal('fetch',vi.fn(async(input:string,options?:RequestInit)=>{
  const url=new URL(String(input),location.href),path=url.pathname.replace(/^\/api\/v1/,'');
  if(options?.body){
   writes.push({path,body:JSON.parse(String(options.body))});
   currentRun={...originalRun,version:4,state:'cancelled',nodes:[{...originalRun.nodes[0],state:'failed'}]};
   if(writes.length===1)throw Error('Original reconciliation reply lost');
   return Response.json(currentRun);
  }
  if(path==='/workflows')return Response.json({items:[{...definition,version:2}],total:1});
  if(path==='/workflows/workflow-a')return Response.json(definition);
  if(path==='/runs')return Response.json({items:[currentRun],total:1});
  if(path==='/runs/run-a')return Response.json(currentRun);
  return Response.json({items:[],total:0});
 }));
});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();await setLocale('zh-CN',{persist:false});vi.unstubAllGlobals();});
async function render(attemptId?:string,refresh=0,requestedInputID?:string,nodeId='develop'){
 await act(async()=>root.render(<ApiViewProvider session={session}><WorkflowView project="project-a" runId="run-a" nodeId={nodeId} attemptId={attemptId} requestedInputID={requestedInputID} onNode={()=>{}} onRun={()=>{}} onRefresh={()=>{}} refresh={refresh}/></ApiViewProvider>));
 await act(async()=>{await new Promise(resolve=>setTimeout(resolve,150));});
 for(let i=0;i<5;i++)await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});
}
it('binds a requested historical question to the durable fence even before the retry has a worker',async()=>{
 currentRun={...originalRun,nodes:[{...originalRun.nodes[0],state:'ready',worker_id:'',tool_evidence:undefined}]};
 historyFixture.attempt={run_id:'run-a',node_id:'develop',attempt:1,fence:2,worker_id:'worker-old',state:'failed',started_at:stamp,completed_at:stamp,result_source:'unreported'};
 const input='b'.repeat(32);
 await render('1',0,input);
 const scope=host.querySelector<HTMLOutputElement>('[data-question]')?.dataset;
 expect(scope).toMatchObject({question:input,attempt:'1',fence:'2',readonly:'true'});
 expect(host.querySelector('[data-question="current"]')).toBeNull();
 historyFixture.attempt={...historyFixture.attempt,run_id:'another-run'};
 await render('1',0,input);
 expect(host.querySelector('[data-question]')).toBeNull();
 await render('1',0,input,'missing-node');
 expect(host.textContent).toContain('The linked input is unavailable in this execution attempt.');
 expect(host.querySelector('[data-question]')).toBeNull();
 expect(writes).toHaveLength(0);
});
function button(label:string,scope:ParentNode=host){return [...scope.querySelectorAll('button')].find(item=>item.textContent===label);}
async function value(field:HTMLSelectElement|HTMLTextAreaElement,text:string){
 const select=field instanceof HTMLSelectElement;
 await act(async()=>{Object.getOwnPropertyDescriptor(select?HTMLSelectElement.prototype:HTMLTextAreaElement.prototype,'value')!.set!.call(field,text);field.dispatchEvent(new Event(select?'change':'input',{bubbles:true}));});
}

it('records the explicitly verified original Cursor result and recovers the same receipt after a lost response',async()=>{
 await render();
 expect(host.textContent).toContain('Cursor process tree stop is unconfirmed');
 expect(writes).toHaveLength(0);
 const open=button('Record original result');expect(open).toBeDefined();
 await act(async()=>open!.click());
 const dialog=document.querySelector('[role=dialog]')!;
 const select=dialog.querySelector('select[required]') as HTMLSelectElement;
 const save=button('Record original result',dialog)!;
 expect(dialog.textContent).toContain('worker-a');
 expect(select.value).toBe('');
 expect([...select.options].map(option=>option.value)).not.toContain('not_started');
 await value(dialog.querySelector('textarea')!,'Original worker process tree stopped; repository has no changes.');
 expect(save.disabled).toBe(true);
 await value(select,'failed');
 expect(save.disabled).toBe(false);
 await act(async()=>save.click());
 expect(writes).toEqual([{path:'/runs/run-a/commands',body:{project_id:'project-a',action:'reconcile',expected_version:3,payload:{node_id:'develop',reason:'Original worker process tree stopped; repository has no changes.',outcome:'failed'},idempotency_key:expect.any(String)}}]);

 await render(undefined,1);
 const recover=button("Recover this operation's receipt",dialog);
 expect(recover?.disabled).toBe(false);
 await act(async()=>recover!.click());
 expect(writes).toEqual([writes[0],writes[0]]);
 expect(document.querySelector('[role=dialog]')).toBeNull();
 expect(button('Record original result')).toBeUndefined();
});

it('keeps a historical attempt read-only and permits an explicit never-started outcome when proof was lost',async()=>{
 await setLocale('zh-CN',{persist:false});
 currentRun={...originalRun,nodes:[{...originalRun.nodes[0],tool_evidence:undefined}]};
 await render('1');
 expect(host.textContent).toContain('正在查看历史尝试');
 expect(button('记录原尝试结果')).toBeUndefined();
 await render('2');
 const open=button('记录原尝试结果');expect(open).toBeDefined();
 await act(async()=>open!.click());
 const select=document.querySelector<HTMLSelectElement>('[role=dialog] select[required]')!;
 expect(select.value).toBe('');
 expect([...select.options].map(option=>option.value)).toContain('not_started');
 expect(writes).toHaveLength(0);
});
