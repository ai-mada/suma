import {expect,it} from 'vitest';
import {ToolWriteRecovery,type ToolPendingRequest} from './tool-write';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import type {ApiCaller} from './api';

it('retains the original operation for a forged Octop producer receipt and refuses caller-supplied execution configuration',async()=>{
 const capability={harness_version:'1.0.5',acp_sdk_version:'0.10.1',provider_id:'openai',model:'configured-model',endpoint_origin:'https://provider.example.test',configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'};
 const body={project_id:'project-a',task_id:'task-a',expected_task_version:2,workflow_id:'source-flow',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',expected_tool_version:3};
 const plan={id:'plan-a',project_id:'project-a',task_id:'task-a',task_version:2,workflow_id:'source-flow',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',tool_profile_version:3,tool_profile_sha256:'c'.repeat(64),run_id:'run-a',octop_execution:capability,workflow:{id:'derived-flow',version:1,nodes:[{id:'develop',type:'agent',max_retries:0,config:{tool_profile_id:'tool-a',tool_profile_sha256:'c'.repeat(64),octop_execution:capability}}]}};
 const malformed={...plan,workflow:{...plan.workflow,nodes:[{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,octop_execution:{...capability,configuration_sha256:'b'.repeat(64)}}}]}};
 let response:unknown=malformed;const calls:string[]=[],values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>void values.set(key,value),removeItem:(key:string)=>void values.delete(key)};
 const send:ApiCaller=async<T>(_path:string,payload?:unknown)=>{calls.push(String((payload as Record<string,unknown>)?.idempotency_key));return response as T;};
 const recovery=new ToolWriteRecovery('octop-current',storage,send,(value,request)=>validateTaskDispatchResponse(value,request,send,'project-a'));
 await expect(recovery.run('/task-dispatch-plans',body)).rejects.toThrow();const original=recovery.pending?.idempotency_key;expect(original).toBeTruthy();expect(values.size).toBe(1);
 response=plan;await expect(recovery.restore()).resolves.toEqual(plan);expect(calls).toEqual([original,original]);expect(recovery.pending).toBeUndefined();
 await expect(recovery.run('/task-dispatch-plans',{...body,octop_execution:capability})).rejects.toThrow();expect(values.size).toBe(0);expect(calls).toHaveLength(2);
 const start:ToolPendingRequest={path:'/task-dispatch-plans/plan-a/start',body:{project_id:'project-a',expected_version:1},method:'POST',idempotency_key:'start-original'},run={id:'run-a',project_id:'project-a',workflow_id:'derived-flow',workflow_version:1};response=malformed;expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(false);response=plan;expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(true);
 for(const node of [{...plan.workflow.nodes[0],type:'test'},{...plan.workflow.nodes[0],max_retries:1},{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,tool_profile_sha256:'d'.repeat(64)}}]){response={...plan,workflow:{...plan.workflow,nodes:[node]}};expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(false);}
});
