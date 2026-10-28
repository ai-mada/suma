// @vitest-environment jsdom
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {afterEach,expect,it} from 'vitest';
import {OctopExecutionFacts} from './OctopExecutionFacts';
import {setLocale} from './i18n';
import type {ToolEvidence} from './tool-model';
const capability={harness_version:'1.0.5',acp_sdk_version:'0.10.1',provider_id:'openai',model:'configured-model',endpoint_origin:'https://provider.example.test',configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'} as const;
const proof={configuration_sha256:capability.configuration_sha256,session_id:'original-session',requested_provider:'openai',requested_model:'configured-model',acknowledged_provider:'openai',acknowledged_model:'configured-model',stop_reason:'end_turn',completed_at:'2026-10-28T00:00:00Z',process_stopped:false,cancel_requested:true};
const evidence={profile:{command_id:'octop-code',adapter:'octop',protocol:'octop-harness-acp-v1',profile_sha256:'b'.repeat(64),executable_sha256:'c'.repeat(64),tool_version:'1.0.5',read_only:false,workspace_tools_version:'attempt-workspace-v1',octop_execution:capability},terminal:'unknown',usage_state:'unreported',event_count:2,octop_execution:proof} as ToolEvidence;
afterEach(async()=>{await setLocale('zh-CN',{persist:false});});
it('keeps a protocol end, an unconfirmed process stop and a cancel request separate in both languages',async()=>{
 await setLocale('en-US',{persist:false});const html=renderToStaticMarkup(createElement(OctopExecutionFacts,{state:'reconciling',evidence}));expect(html).toContain('data-octop-run-state="unknown"');expect(html).toContain('end_turn');expect(html).toContain('Stop not confirmed');expect(html).toContain('Cancellation requested');expect(html).toContain('observe-only');expect(html).toContain('configuration acknowledgment');const content=document.createElement('div');content.innerHTML=html;expect(content.querySelector('details')?.open).toBe(false);expect(content.querySelector('details')?.textContent).toContain('original-session');
 await setLocale('zh-CN',{persist:false});const chinese=renderToStaticMarkup(createElement(OctopExecutionFacts,{state:'reconciling',evidence}));expect(chinese).toContain('停止尚未确认');expect(chinese).toContain('已请求取消');expect(chinese).toContain('预算仅观测');
});
it('does not infer an execution result from a configured profile or an absent execution receipt',async()=>{
 await setLocale('en-US',{persist:false});
 const configured=renderToStaticMarkup(createElement(OctopExecutionFacts,{capability}));expect(configured).not.toContain('data-octop-run-state');expect(configured).toContain('configuration acknowledgment');
 const missing=renderToStaticMarkup(createElement(OctopExecutionFacts,{state:'succeeded',evidence:{...evidence,octop_execution:undefined}}));expect(missing).toContain('data-octop-run-state="unrecorded"');expect(missing).not.toContain('data-octop-run-state="completed"');
});
