import {modelBinding,runtimeCapability} from './model-execution.test-support';
import {expect,it} from 'vitest';
import {ToolWriteRecovery,type ToolPendingRequest} from './tool-write';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import type {ApiCaller} from './api';

it('T118 preserves original intent for a forged producer model binding and accepts shared Octop model selection',async()=>{
 const capability=runtimeCapability;
 const body={project_id:'project-a',task_id:'task-a',expected_task_version:2,workflow_id:'source-flow',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',expected_tool_version:3,model_execution_selection:{deployment_id:modelBinding.deployment.deployment_id,deployment_revision:modelBinding.deployment.revision,deployment_sha256:modelBinding.deployment.deployment_sha256,parameters:modelBinding.parameters,budget:modelBinding.budget}};
 const plan={id:'plan-a',project_id:'project-a',task_id:'task-a',task_version:2,workflow_id:'source-flow',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',tool_profile_version:3,tool_profile_sha256:'c'.repeat(64),run_id:'run-a',model_execution:modelBinding,workflow:{id:'derived-flow',version:1,nodes:[{id:'develop',type:'agent',max_retries:0,config:{tool_profile_id:'tool-a',tool_profile_sha256:'c'.repeat(64),model_execution:modelBinding}}]}};
 const malformed={...plan,workflow:{...plan.workflow,nodes:[{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,model_execution:{...modelBinding,parameters:{...modelBinding.parameters,max_steps:31}}}}]}};
 let response:unknown=malformed;const calls:string[]=[],values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>void values.set(key,value),removeItem:(key:string)=>void values.delete(key)};
 const send:ApiCaller=async<T>(_path:string,payload?:unknown)=>{calls.push(String((payload as Record<string,unknown>)?.idempotency_key));return response as T;};
 const recovery=new ToolWriteRecovery('octop-current',storage,send,(value,request)=>validateTaskDispatchResponse(value,request,send,'project-a'));
 await expect(recovery.run('/task-dispatch-plans',body)).rejects.toThrow();const original=recovery.pending?.idempotency_key;expect(original).toBeTruthy();expect(values.size).toBe(1);
 response=plan;await expect(recovery.restore()).resolves.toEqual(plan);expect(calls).toEqual([original,original]);expect(recovery.pending).toBeUndefined();
 await expect(recovery.run('/task-dispatch-plans',{...body,octop_execution:capability})).rejects.toThrow();expect(values.size).toBe(0);expect(calls).toHaveLength(2);
 const start:ToolPendingRequest={path:'/task-dispatch-plans/plan-a/start',body:{project_id:'project-a',expected_version:1},method:'POST',idempotency_key:'start-original'},run={id:'run-a',project_id:'project-a',workflow_id:'derived-flow',workflow_version:1};response=malformed;expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(false);response=plan;expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(true);
 response={...plan,octop_execution:capability};expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(false);
 for(const node of [{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,octop_execution:capability}},{...plan.workflow.nodes[0],type:'test'},{...plan.workflow.nodes[0],max_retries:1},{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,tool_profile_sha256:'d'.repeat(64)}}]){response={...plan,workflow:{...plan.workflow,nodes:[node]}};expect(await validateTaskDispatchResponse(run,start,send,'project-a')).toBe(false);}
});
