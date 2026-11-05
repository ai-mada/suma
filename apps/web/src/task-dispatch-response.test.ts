// @vitest-environment jsdom
import {expect,it} from 'vitest';
import type {ApiCaller} from './api';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import {ToolWriteRecovery,type ToolPendingRequest} from './tool-write';

const project='project159', organization='org159', stamp='2026-11-05T12:00:00Z';
const request:ToolPendingRequest={path:'/task-dispatch-plans/plan159/start',method:'POST',body:{project_id:project,expected_version:1},idempotency_key:'original-start-operation159'};
const fixed=()=>({model:'claude-sonnet-5',model_source:'administrator_cli_argument',configuration_sha256:'c'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login'});
const envelope=(basis:{id:string;task_id:string;task_version:number;workflow_id:string;workflow_version:number;agent_node_id:string;tool_profile_id:string;tool_profile_version:number})=>({
 ...basis,project_id:project,state:'ready',version:1,valid:true,reason:'',run_id:'run159',created_at:stamp,expires_at:stamp,
 tool_profile_sha256:'b'.repeat(64),
 context:{task:{id:basis.task_id,version:basis.task_version,title:'Task',description:'Description',acceptance_criteria:[]}},
 workflow:{id:basis.workflow_id,version:basis.workflow_version,nodes:[{id:basis.agent_node_id,name:'Develop',type:'agent',max_retries:0,command:['code'],config:{tool_profile_id:basis.tool_profile_id,tool_profile_sha256:'b'.repeat(64)}}],edges:[]},
 execution_route:{tenant_id:organization,project_id:project,version:0,pool_id:'',grant_id:'',grant_version:0},
});
const plan=()=>{
 const base=envelope({id:'plan159',task_id:'task159',task_version:3,workflow_id:'published159',workflow_version:7,agent_node_id:'develop',tool_profile_id:'claude-profile',tool_profile_version:2});
 const [node]=base.workflow.nodes;
 return {...base,state:'started',version:2,valid:false,reason:'Already started',claude_execution:fixed(),workflow:{...base.workflow,nodes:[{...node,config:{...node.config,claude_execution:fixed()}}]}};
};
const run=()=>({id:'run159',tenant_id:organization,project_id:project,workflow_id:'managed-run159',workflow_version:1});

it('keeps an ordinary preview pending until the exact requested task, workflow and tool are returned',async()=>{
 const body={project_id:project,task_id:'task-a',expected_task_version:4,workflow_id:'flow-a',workflow_version:2,agent_node_id:'develop',tool_profile_id:'tool-b',expected_tool_version:3};
 const receipt=envelope({id:'plan-a',task_id:'task-a',task_version:4,workflow_id:'flow-a',workflow_version:2,agent_node_id:'develop',tool_profile_id:'tool-b',tool_profile_version:3});
 const preview:ToolPendingRequest={path:'/task-dispatch-plans',method:'POST',body,idempotency_key:'original-preview'};
 let response:unknown={...receipt,tool_profile_id:'other-tool'};
 const writes:unknown[]=[];
 const send:ApiCaller=async(path,input,method)=>{expect(path).toBe(preview.path);expect(method).toBe('POST');writes.push(structuredClone(input));return response as never;};
 const action=new ToolWriteRecovery('dispatch-basis235',sessionStorage,send,(value,pending)=>validateTaskDispatchResponse(value,pending,send,project,organization));
 await expect(action.run(preview.path,body)).rejects.toThrow();
 expect(action.pending?.body).toEqual(body);
 for(const mismatch of [{}, {...receipt,id:''}, {...receipt,project_id:'other-project'}, {...receipt,task_id:'other-task'}, {...receipt,task_version:5}, {...receipt,workflow_id:'other-flow'}, {...receipt,workflow_version:3}, {...receipt,agent_node_id:'other-node'}, {...receipt,tool_profile_version:2},
  {...receipt,context:null}, {...receipt,workflow:{...receipt.workflow,nodes:[null]}}, {...receipt,execution_route:{...receipt.execution_route,tenant_id:'other-org'}},
  {...receipt,context:{...receipt.context,knowledge:[{id:'unexpected',revision:1,sha256:'a'.repeat(64),kind:'spec',title:'Other',summary:'',content:'Other content',sources:[]}]}}]){
  expect(await validateTaskDispatchResponse(mismatch,preview,send,project,organization)).toBe(false);
 }
 expect(await validateTaskDispatchResponse({...receipt,state:'started',version:2,valid:false},preview,send,project,organization)).toBe(true);
 response=receipt;
 await expect(action.restore()).resolves.toEqual(receipt);
 expect(writes).toHaveLength(2);
 expect(writes[1]).toEqual(writes[0]);
 expect(action.pending).toBeUndefined();
});

it('T192 validates the SDK source before acknowledging the original managed Start receipt',async()=>{
 const sdk={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:'a'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
 const {claude_execution:_,...base}=plan();
 const original={...base,tool_profile_id:'sdk-profile',tool_profile_sha256:'b'.repeat(64),claude_sdk_execution:sdk,workflow:{...base.workflow,nodes:[{id:'develop',name:'Develop',type:'agent',max_retries:0,command:['sdk-command'],config:{tool_profile_id:'sdk-profile',tool_profile_sha256:'b'.repeat(64),claude_sdk_execution:sdk}}]}};
 const read:ApiCaller=async()=>original as never;expect(await validateTaskDispatchResponse(run(),request,read,project,organization)).toBe(true);
 const drifted:ApiCaller=async()=>({...original,claude_sdk_execution:{...sdk,configuration_sha256:'f'.repeat(64)}}) as never;
 expect(await validateTaskDispatchResponse(run(),request,drifted,project,organization)).toBe(false);
});

it('acknowledges the reserved managed run instead of the original published workflow',async()=>{
 const calls:string[]=[];const read:ApiCaller=async path=>{calls.push(path);return plan() as never;};
 expect(await validateTaskDispatchResponse(run(),request,read,project,organization)).toBe(true);
 expect(calls).toEqual(['/task-dispatch-plans/plan159?project_id='+project]);
});

it('rejects original or foreign workflow identities, versions and fixed-plan bindings',async()=>{
 const read:ApiCaller=async()=>plan() as never;
 for(const value of [{...run(),workflow_id:'published159',workflow_version:7},{...run(),workflow_id:'managed-foreign159'},{...run(),workflow_version:7},{...run(),id:'foreign159'},{...run(),project_id:'foreign159'},{...run(),tenant_id:'foreign159'}])expect(await validateTaskDispatchResponse(value,request,read,project,organization)).toBe(false);
 for(const original of [{...plan(),id:'foreign159'},{...plan(),run_id:'foreign159'},{...plan(),project_id:'foreign159'},{...plan(),state:'ready'},{...plan(),claude_execution:{...fixed(),model:'claude-other-5'}}]){
  const wrong:ApiCaller=async()=>original as never;
  expect(await validateTaskDispatchResponse(run(),request,wrong,project,organization)).toBe(false);
 }
});

it('keeps the same Start body and key when the authorized plan read fails, then recovers that run',async()=>{
 const values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};
 let readable=false;const writes:unknown[]=[];
 const send:ApiCaller=async(path,body,method)=>{if(method==='POST'){writes.push(structuredClone(body));return run() as never;}expect(path).toBe('/task-dispatch-plans/plan159?project_id='+project);if(!readable)throw Error('original current-authorized read unavailable');return plan() as never;};
 const action=new ToolWriteRecovery('managed159',storage,send,(value,pending)=>validateTaskDispatchResponse(value,pending,send,project,organization));
 await expect(action.run(request.path,request.body)).rejects.toThrow();
 const pending=action.pending;expect(pending?.path).toBe(request.path);expect(pending?.body).toEqual(request.body);expect(pending?.idempotency_key).toBeTruthy();expect(values.size).toBe(1);
 readable=true;await expect(action.restore()).resolves.toEqual(run());
 expect(writes).toHaveLength(2);expect(writes[1]).toEqual(writes[0]);expect(action.pending).toBeUndefined();expect(values.size).toBe(0);
});
