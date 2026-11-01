// @vitest-environment jsdom
import {expect,it} from 'vitest';
import type {ClaudeSDKExecutionCapability} from '../../../packages/contracts/src';
import {parseToolManifest,taskToolProtocol,toolBinding,type ToolProfile} from './tool-model';

const capability=():ClaudeSDKExecutionCapability=>({sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:'c'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'});
const observation=()=>({available:true,reason:'',profile:{command_id:'claude-sdk-develop',adapter:'claude',protocol:'claude-sdk-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'2.1.314',read_only:false,user_input_protocol:'claude-sdk-input-v1',claude_sdk_execution:capability()}});
it('T192 accepts a complete SDK profile and freezes its exact SDK model without borrowing CLI or gateway authority',()=>{
 const [item]=parseToolManifest(JSON.stringify([observation()]));expect(taskToolProtocol(item.profile)).toBe(true);
 const node=toolBinding({config:{goal:'Original approved goal',claude_execution:{wrong:true},model_execution:{wrong:true},codex_execution:{wrong:true}}},{id:'tool',profile:item.profile} as ToolProfile);
 expect(node).toMatchObject({max_retries:0,command:['claude-sdk-develop'],config:{goal:'Original approved goal',claude_sdk_execution:capability()}});expect(node.config).not.toHaveProperty('claude_execution');expect(node.config).not.toHaveProperty('model_execution');expect(node.config).not.toHaveProperty('codex_execution');
 for(const mutation of [{tool_version:'0.3.314'},{tool_version:'2.1.314 (Claude Code)'},{user_input_protocol:''},{read_only:true},{claude_execution:{}},{acp_execution:{}},{model_execution_protocol:'suma-model-execution-v1'},{claude_sdk_execution:{...capability(),auth_strategy:'existing_cli_login'}}])expect(()=>parseToolManifest(JSON.stringify([{...observation(),profile:{...observation().profile,...mutation}}]))).toThrow();
 expect(()=>parseToolManifest(JSON.stringify([{...observation(),profile:{...observation().profile,claude_sdk_execution:undefined}}]))).toThrow();
 const unavailable={...observation(),available:false,reason:'SDK host unavailable',profile:{...observation().profile,claude_sdk_execution:undefined}};expect(parseToolManifest(JSON.stringify([unavailable]))).toHaveLength(1);
 expect(taskToolProtocol(unavailable.profile)).toBe(false);
});
