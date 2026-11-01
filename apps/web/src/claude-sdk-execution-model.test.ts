import {expect,it} from 'vitest';
import type {ClaudeSDKExecutionCapability} from '../../../packages/contracts/src';
import {validClaudeSDKCapability,sameClaudeSDKCapability,validClaudeSDKPlan,validClaudeSDKDispatchReceipt} from './claude-sdk-execution-model';

const fixed=():ClaudeSDKExecutionCapability=>({sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:'c'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'});
it('T192 keeps the pinned SDK capability and exact formal plan separate from CLI, gateway and changed receipts',()=>{
 expect(validClaudeSDKCapability(fixed())).toBe(true);expect(sameClaudeSDKCapability(fixed(),{...fixed()})).toBe(true);
 for(const patch of [{sdk_version:'0.3.315'},{cli_version:'2.1.315'},{model:'sonnet'},{model_source:'administrator_cli_argument'},{identity_evidence:'configuration_ack_only'},{auth_strategy:'existing_cli_login'},{budget_mode:'enforce'},{extra:true}])expect(validClaudeSDKCapability({...fixed(),...patch})).toBe(false);
 const body={project_id:'project',task_id:'task',expected_task_version:2,workflow_id:'workflow',workflow_version:3,agent_node_id:'develop',tool_profile_id:'tool',expected_tool_version:4};
 const config={tool_profile_id:'tool',tool_profile_sha256:'a'.repeat(64),claude_sdk_execution:fixed()};
 const node={id:'develop',type:'agent',max_retries:0,command:['claude-sdk-develop'],config};
 const plan={id:'plan',project_id:'project',task_id:'task',task_version:2,workflow_id:'workflow',workflow_version:3,agent_node_id:'develop',tool_profile_id:'tool',tool_profile_version:4,tool_profile_sha256:'a'.repeat(64),claude_sdk_execution:fixed(),workflow:{nodes:[node]}};
 expect(validClaudeSDKPlan(plan)).toBe(true);expect(validClaudeSDKDispatchReceipt(plan,body)).toBe(true);
 expect(validClaudeSDKPlan({...plan,claude_sdk_execution:{...fixed(),model:'claude-other-5'}})).toBe(false);
 expect(validClaudeSDKPlan({...plan,workflow:{nodes:[node,node]}})).toBe(false);
 for(const patch of [{claude_execution:null},{model_execution:null},{api_key_env_ref:'SECRET_REF'},{resume:null}])expect(validClaudeSDKPlan({...plan,workflow:{nodes:[{...node,config:{...config,...patch}}]}})).toBe(false);
 expect(validClaudeSDKPlan({...plan,workflow:{nodes:[{...node,max_retries:1}]}})).toBe(false);
 expect(validClaudeSDKDispatchReceipt(plan,{...body,expected_tool_version:5})).toBe(false);
 expect(validClaudeSDKDispatchReceipt(plan,{...body,model_execution_selection:null})).toBe(false);
 expect(validClaudeSDKPlan({workflow:{nodes:[]}})).toBe(true);
});
