// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import type {ClaudeSDKExecutionCapability,ClaudeSDKExecutionEvidence,ToolEvidence} from '../../../packages/contracts/src';
import {ClaudeSDKExecutionFacts} from './ClaudeSDKExecutionFacts';
import {claudeSDKInputEvidence,claudeSDKPermissionEvidence} from './claude-sdk.test-support';
import {setLocale} from './i18n';

it('T192 displays only original SDK evidence and rejects cross-session, incomplete callbacks, mixed authority and mismatched totals',async()=>{
 await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 const capability:ClaudeSDKExecutionCapability={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:'a'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
 const permission={...structuredClone(claudeSDKPermissionEvidence),observation:{...claudeSDKPermissionEvidence.observation!,reply_written:true,execution_closed:true}},input={...structuredClone(claudeSDKInputEvidence),observation:{...claudeSDKInputEvidence.observation!,reply_written:true,execution_closed:true}};
 const proof:ClaudeSDKExecutionEvidence={sdk_version:'0.3.314',cli_version:'2.1.314',configuration_sha256:capability.configuration_sha256,session_id:input.session_id,requested_model:capability.model,acknowledged_model:capability.model,result_subtype:'success',result_is_error:false,turn_ended:true,tools_completed:true,cancel_requested:false,host_process_stopped:true,host_process_tree_stopped:true,permission_requests:1,denied_callbacks:0,native_permission_denials:0,permissions:[permission],user_inputs:[input]};
 const evidence:ToolEvidence={profile:{command_id:'claude-sdk-develop',adapter:'claude',protocol:'claude-sdk-v1',profile_sha256:'e'.repeat(64),executable_sha256:'f'.repeat(64),tool_version:'2.1.314',read_only:false,user_input_protocol:'claude-sdk-input-v1',claude_sdk_execution:capability},session_id:proof.session_id,terminal:'completed',event_count:6,usage_state:'partial',claude_sdk_execution:proof};
 const host=document.createElement('div');document.body.append(host);const root=createRoot(host);
 const render=async(value:ToolEvidence|undefined=evidence)=>act(async()=>root.render(<ClaudeSDKExecutionFacts capability={capability} evidence={value}/>));
 try{
  await render();expect(host.querySelector('[data-claude-sdk-evidence="matched"]')).not.toBeNull();expect(host.querySelector('[data-claude-sdk-session]')?.textContent).toBe(proof.session_id);expect(host.textContent).toContain('claude-sonnet-5');expect(host.textContent).toContain('local SDK host');expect(host.textContent).toContain('0.3.314');expect(host.querySelector('select,input')).toBeNull();
  const reject=async(value:ToolEvidence)=>{await render(value);expect(host.querySelector('[data-claude-sdk-evidence="invalid"]')).not.toBeNull();expect(host.querySelector('[data-claude-sdk-session]')).toBeNull();};
  await reject({...evidence,session_id:'another-session'});await reject({...evidence,usage_state:'complete'});
  for(const patch of [{configuration_sha256:'b'.repeat(64)},{requested_model:'claude-other-5'},{acknowledged_model:'claude-other-5'},{permission_requests:2},{host_process_tree_stopped:false},{cancel_requested:true},{tools_completed:false},{denied_callbacks:1},{native_permission_denials:1}])await reject({...evidence,claude_sdk_execution:{...proof,...patch}});
  const missingNative={...proof};delete missingNative.native_permission_denials;await reject({...evidence,claude_sdk_execution:missingNative});
  await reject({...evidence,claude_sdk_execution:{...proof,user_inputs:[{...input,observation:{...input.observation!,reply_written:false}}]}});
  await reject({...evidence,claude_sdk_execution:{...proof,user_inputs:[{...input,request_id:permission.request_id}]}});
  await reject({...evidence,claude_sdk_execution:{...proof,user_inputs:[{...input,callback_id:permission.callback_id}]}});
  await reject({...evidence,claude_sdk_execution:{...proof,user_inputs:[{...input,observation:{...input.observation!,request_resolved:true}}]}});
  await reject({...evidence,profile:{...evidence.profile,protocol:'claude-jsonl-v1'}});
  await reject({...evidence,claude_sdk_execution:{...proof,user_inputs:Array.from({length:32},(_,index)=>({...input,input_id:'input-'+index,request_id:'request-'+index,callback_id:'callback-'+index}))}});
  const unknown={...evidence,terminal:'unknown',claude_sdk_execution:{...proof,turn_ended:false,tools_completed:false,host_process_stopped:false,host_process_tree_stopped:false,user_inputs:[{...input,observation:{...input.observation!,reply_written:false,execution_closed:false}}]}};
  await render(unknown);expect(host.querySelector('[data-claude-sdk-evidence="matched"]')).not.toBeNull();expect(host.textContent).toContain('Not confirmed');
  const partial={...unknown.claude_sdk_execution,permissions:[]};delete partial.native_permission_denials;
  await render({...unknown,claude_sdk_execution:partial});expect(host.querySelector('[data-claude-sdk-evidence="matched"]')).not.toBeNull();expect(host.querySelector('[data-claude-sdk-session]')?.textContent).toBe(proof.session_id);expect(host.querySelector('[data-claude-sdk-native-denials]')?.textContent).toContain('Not recorded');
  const {permissions:_omittedPermissions,...omitted}=partial;await render({...unknown,claude_sdk_execution:omitted});expect(host.querySelector('[data-claude-sdk-evidence="matched"]')).not.toBeNull();
  await reject({...evidence,claude_sdk_execution:{...proof,denied_permissions:0}} as unknown as ToolEvidence);
  await act(async()=>root.render(<ClaudeSDKExecutionFacts capability={capability}/>));expect(host.querySelector('[data-claude-sdk-evidence="unrecorded"]')).not.toBeNull();expect(host.querySelector('[data-claude-sdk-session]')).toBeNull();expect(fetch).not.toHaveBeenCalled();
 }finally{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();}
});
