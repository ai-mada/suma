import {expect,it} from 'vitest';
import {octopHarnessConfiguration,octopRunFacts,validOctopCapability,type OctopHarnessInputs} from './octop-execution-model';
import type {ToolEvidence} from './tool-model';
export const capability={harness_version:'1.0.5',acp_sdk_version:'0.10.1',provider_id:'openai',model:'configured-model',endpoint_origin:'https://provider.example.test',configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'} as const;
const profile={command_id:'octop-code',adapter:'octop',protocol:'octop-harness-acp-v1',profile_sha256:'b'.repeat(64),executable_sha256:'c'.repeat(64),tool_version:'1.0.5',read_only:false,workspace_tools_version:'attempt-workspace-v1',octop_execution:capability};
export const proof={configuration_sha256:capability.configuration_sha256,session_id:'session-a',requested_provider:'openai',requested_model:'configured-model',acknowledged_provider:'openai',acknowledged_model:'configured-model',stop_reason:'end_turn',completed_at:'2026-10-28T00:00:00Z',process_stopped:true,cancel_requested:false};
const evidence=()=>({profile,terminal:'completed',usage_state:'unreported',event_count:2,octop_execution:proof}) as ToolEvidence;
it('accepts only explicit Octop configuration acknowledgments without stronger budget or provider claims',()=>{
 expect(validOctopCapability(capability)).toBe(true);
 for(const bad of [{...capability,budget_mode:'hard'},{...capability,identity_evidence:'provider_verified'},{...capability,endpoint_origin:'https://user:secret@provider.example.test'},{...capability,endpoint_origin:'https://provider.example.test/v1'},{...capability,api_key:'private'},{...capability,configuration_sha256:'bad'}])expect(validOctopCapability(bad)).toBe(false);
});
it('requires explicit Python, bundle, checks and fixed model inputs before creating a secret-free administrator configuration',()=>{
 const input:OctopHarnessInputs={python:'C:/Python313/python.exe',bundle:'D:/tools/suma/octop-runtime.pyz',manifest:'D:/tools/suma/octop-runtime.manifest.json',checks:'formal-check',provider:'openai',model:'configured-model',endpoint:'https://provider.example.test/v1',keyEnvironment:'OCTOP_MODEL_API_KEY',maxSteps:'8',maxOutputTokens:'4096'};
 const config=octopHarnessConfiguration(input);expect(config).toBeDefined();const command=config!['octop-harness-develop'] as any;expect(command).toMatchObject({adapter:'octop',protocol:'octop-harness-acp-v1',executable:input.python,args:[input.bundle],input:'octop-acp-jsonl',read_only:false,native_workspace:{checks:['formal-check'],runtime_manifest:input.manifest},octop:{provider_id:'openai',model:'configured-model',base_url:input.endpoint,api_key_env_ref:'OCTOP_MODEL_API_KEY',max_steps:8,max_output_tokens:4096}});expect(command).not.toHaveProperty('env_from');expect(command).not.toHaveProperty('env');
 for(const patch of [{python:'python'},{bundle:'relative.pyz'},{manifest:'D:/different/runtime.json'},{checks:''},{checks:'check,check'},{maxSteps:'33'},{maxOutputTokens:'32769'},{endpoint:'https://provider.example.test?api_key=secret'},{keyEnvironment:'SECRET=value'}])expect(octopHarnessConfiguration({...input,...patch})).toBeUndefined();
});
it('separates protocol completion, platform outcome, process stop proof and cancellation intent',()=>{
 expect(octopRunFacts('succeeded',evidence()).phase).toBe('completed');
 expect(octopRunFacts('reconciling',{...evidence(),octop_execution:{...proof,process_stopped:false,cancel_requested:true}}).phase).toBe('unknown');
 expect(octopRunFacts('cancelled',{...evidence(),octop_execution:{...proof,stop_reason:'cancelled',cancel_requested:true}}).phase).toBe('stopped');
 expect(octopRunFacts('failed',evidence()).phase).toBe('failed');
 expect(octopRunFacts('succeeded',{...evidence(),octop_execution:{...proof,configuration_sha256:'d'.repeat(64)}}).phase).toBe('mismatch');
 expect(octopRunFacts('succeeded',{...evidence(),octop_execution:undefined}).phase).toBe('unrecorded');
});
