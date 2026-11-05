import {modelBinding,runtimeCapability} from './model-execution.test-support';
import {expect,it} from 'vitest';
import {ToolWriteRecovery,type ToolPendingRequest} from './tool-write';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import type {ApiCaller} from './api';

it('T118 preserves original intent for a forged producer model binding and accepts shared Octop model selection',async()=>{
 const capability=runtimeCapability;
 const body={project_id:'project-a',task_id:'task-a',expected_task_version:2,workflow_id:'source-flow',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',expected_tool_version:3,model_execution_selection:{deployment_id:modelBinding.deployment.deployment_id,deployment_revision:modelBinding.deployment.revision,deployment_sha256:modelBinding.deployment.deployment_sha256,parameters:modelBinding.parameters,budget:modelBinding.budget}};
 const toolDigest='c'.repeat(64);
 const plan={
  id:'plan-a',project_id:'project-a',task_id:'task-a',task_version:2,run_id:'run-a',state:'ready',version:1,
  workflow_id:'source-flow',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',tool_profile_version:3,tool_profile_sha256:toolDigest,
  model_execution:modelBinding,valid:true,reason:'',created_at:'2026-11-01T00:00:00Z',expires_at:'2026-11-01T00:15:00Z',
  context:{task:{id:'task-a',version:2,title:'Implement the task',description:'',acceptance_criteria:[]}},
  workflow:{id:'source-flow',version:1,nodes:[{id:'develop',name:'Develop',type:'agent',command:['octop-code'],max_retries:0,config:{tool_profile_id:'tool-a',tool_profile_sha256:toolDigest,model_execution:modelBinding}}]},
  execution_route:{tenant_id:'org-a',project_id:'project-a',version:0,pool_id:'',grant_id:'',grant_version:0},
 };
 const malformed={...plan,workflow:{...plan.workflow,nodes:[{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,model_execution:{...modelBinding,parameters:{...modelBinding.parameters,max_steps:31}}}}]}};
 let response:unknown=malformed;const calls:string[]=[],values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>void values.set(key,value),removeItem:(key:string)=>void values.delete(key)};
 const send:ApiCaller=async<T>(_path:string,payload?:unknown)=>{calls.push(String((payload as Record<string,unknown>)?.idempotency_key));return response as T;};
 const validate=(value:unknown,request:ToolPendingRequest)=>validateTaskDispatchResponse(value,request,send,'project-a','org-a');
 const recovery=new ToolWriteRecovery('octop-current',storage,send,validate);
 await expect(recovery.run('/task-dispatch-plans',body)).rejects.toThrow();const original=recovery.pending?.idempotency_key;expect(original).toBeTruthy();expect(values.size).toBe(1);
 response=plan;await expect(recovery.restore()).resolves.toEqual(plan);expect(calls).toEqual([original,original]);expect(recovery.pending).toBeUndefined();
 await expect(recovery.run('/task-dispatch-plans',{...body,octop_execution:capability})).rejects.toThrow();expect(values.size).toBe(0);expect(calls).toHaveLength(2);
 const start:ToolPendingRequest={path:'/task-dispatch-plans/plan-a/start',body:{project_id:'project-a',expected_version:1},method:'POST',idempotency_key:'start-original'};
 const run={id:'run-a',tenant_id:'org-a',project_id:'project-a',workflow_id:'managed-run-a',workflow_version:1};
 const started={...plan,state:'started',version:2,valid:false,reason:'already started'};
 response={...started,workflow:malformed.workflow};expect(await validate(run,start)).toBe(false);
 response=started;expect(await validate(run,start)).toBe(true);
 response={...started,octop_execution:capability};expect(await validate(run,start)).toBe(false);
 for(const node of [{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,octop_execution:capability}},{...plan.workflow.nodes[0],type:'test'},{...plan.workflow.nodes[0],max_retries:1},{...plan.workflow.nodes[0],config:{...plan.workflow.nodes[0].config,tool_profile_sha256:'d'.repeat(64)}}]){response={...started,workflow:{...started.workflow,nodes:[node]}};expect(await validate(run,start)).toBe(false);}
});
