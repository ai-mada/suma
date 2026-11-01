import {expect,it} from 'vitest';
import type {ModelExecutionOptions} from '../../../packages/contracts/src';
import {validModelOptions,modelSelection} from './model-execution-model';

it('T192 reads only the exact fixed SDK model with no gateway options, hard budget or mixed runtime',()=>{
 const scope={project:'project',profile:'tool',version:4,sha256:'a'.repeat(64)};
 const capability={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:'c'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
 const options:ModelExecutionOptions={project_id:'project',tool_profile_id:'tool',tool_profile_version:4,tool_profile_sha256:scope.sha256,observed_at:'2026-11-01T00:00:00Z',supported:false,codex_supported:false,codex_reason:'',reason:'fixed_by_tool',items:[],fixed_claude_sdk:capability};
 expect(validModelOptions(options,scope)).toBe(true);expect(modelSelection(options,{deployment:'',steps:'1',output:'1',tokens:'',cost:''})).toBeUndefined();
 for(const patch of [{supported:true},{codex_supported:true},{reason:'gateway_ready'},{fixed_claude:{model:'wrong'}},{fixed_acp:{}},{parameter_limits:{max_steps:1,max_output_tokens:1}},{fixed_claude_sdk:{...capability,sdk_version:'0.3.315'}}])expect(validModelOptions({...options,...patch} as ModelExecutionOptions,scope)).toBe(false);
 expect(validModelOptions(options,{...scope,project:'another'})).toBe(false);
});
