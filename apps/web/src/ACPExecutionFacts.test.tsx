// @vitest-environment jsdom
import {act} from 'react';
import {createRoot} from 'react-dom/client';
import {expect,it,vi} from 'vitest';
import {ACPExecutionFacts} from './ACPExecutionFacts';
import {setLocale,tr} from './i18n';
import type {ACPExecutionCapability,ToolEvidence} from '../../../packages/contracts/src';

it('T191 displays original Cursor input evidence and the actual nonzero stopped-host exit after a completed RPC',async()=>{
 await setLocale('zh-CN',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);
 const hash='a'.repeat(64),params='b'.repeat(64),answer='c'.repeat(64);
 const capability:ACPExecutionCapability={runtime:'cursor',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:hash,model:'reviewed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'explicit_api_key_env',permission_mode:'human_once'};
 const evidence:ToolEvidence={profile:{command_id:'cursor-develop',adapter:'cursor',protocol:'acp-stdio-v1',profile_sha256:'d'.repeat(64),executable_sha256:'e'.repeat(64),tool_version:'Cursor exact 191',read_only:false,user_input_protocol:'cursor-acp-input-v1',acp_execution:capability},session_id:'original-session',terminal:'completed',event_count:4,usage_state:'unknown',acp_execution:{configuration_sha256:hash,requested_model:'reviewed-model',selected_model:'reviewed-model',model_selection_acknowledged:true,session_id:'original-session',protocol_version:1,prompt_started:true,prompt_completed:true,stop_reason:'end_turn',tools_complete:true,permissions_requested:0,permissions_denied:0,cancel_requested:false,host_process_stopped:true,host_process_tree_stopped:true,host_process_exit_code:137,host_stopped_after_rpc:true,user_inputs:[{input_id:'original-question',request_id:{kind:'number',value:'9007199254740993'},prompt_id:{kind:'string',value:'original-prompt'},session_id:'original-session',method:'cursor/ask_question',configuration_sha256:hash,params_sha256:params,kind:'questions',question_count:1,observation:{sequence:1,params_sha256:params,answer_sha256:answer,answer_observed:true,reply_started:true,reply_written:true,request_resolved:false,execution_closed:true}}]}};
 const host=document.createElement('div');document.body.append(host);const root=createRoot(host);
 try{
  await act(async()=>root.render(<ACPExecutionFacts capability={capability} evidence={evidence}/>));
  expect(host.textContent).not.toContain(tr('acp-runtime.facts.invalid_evidence'));expect(host.textContent).toContain(tr('acp-runtime.facts.host_stopped_after_rpc'));
  expect(host.querySelector('[data-acp-host-exit]')?.textContent).toBe('137');
  await act(async()=>root.render(<ACPExecutionFacts capability={capability} evidence={{...evidence,session_id:'another-session'}}/>));expect(host.textContent).toContain(tr('acp-runtime.facts.invalid_evidence'));
  const incomplete=structuredClone(evidence);incomplete.acp_execution!.host_stopped_after_rpc=false;incomplete.acp_execution!.host_process_exit_code=0;incomplete.acp_execution!.user_inputs![0].observation!.reply_written=false;incomplete.acp_execution!.user_inputs![0].observation!.execution_closed=false;
  await act(async()=>root.render(<ACPExecutionFacts capability={capability} evidence={incomplete}/>));expect(host.textContent).toContain(tr('acp-runtime.facts.invalid_evidence'));
  const invalid=structuredClone(evidence);invalid.acp_execution!.user_inputs![0].observation!.request_resolved=true;
  await act(async()=>root.render(<ACPExecutionFacts capability={capability} evidence={invalid}/>));expect(host.textContent).toContain(tr('acp-runtime.facts.invalid_evidence'));expect(host.querySelector('[data-acp-host-exit]')).toBeNull();
  invalid.acp_execution!.user_inputs![0].observation!.request_resolved=false;invalid.acp_execution!.user_inputs![0].configuration_sha256='f'.repeat(64);
  await act(async()=>root.render(<ACPExecutionFacts capability={capability} evidence={invalid}/>));expect(host.textContent).toContain(tr('acp-runtime.facts.invalid_evidence'));
  const qoder={...capability,runtime:'qoder',auth_strategy:'existing_cli_login'};await act(async()=>root.render(<ACPExecutionFacts capability={qoder} evidence={evidence}/>));expect(host.textContent).toContain(tr('acp-runtime.facts.invalid_evidence'));
 }finally{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();}
});
