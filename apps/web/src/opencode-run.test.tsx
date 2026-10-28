// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {OpenCodeExecutionCapability,ToolEvidence} from '../../../packages/contracts/src';
import type {ToolEvent} from './tool-model';
import {ToolExecutionFacts} from './ToolFacts';
import {OpenCodeExecutionFacts} from './OpenCodeExecutionFacts';
import {setLocale} from './i18n';
import {frozenOpenCodeForNode,openCodeRunFacts} from './opencode-run-model';

const capability:OpenCodeExecutionCapability={sdk_version:'1.19.4',server_version:'1.19.4',provider_id:'owned',model:'team/model-a',endpoint_origin:'https://models.example',configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'};
const profile={command_id:'opencode',adapter:'opencode',protocol:'opencode-sdk-v1',profile_sha256:'b'.repeat(64),executable_sha256:'c'.repeat(64),tool_version:'1.19.4',read_only:false,workspace_tools_version:'attempt-workspace-v1',opencode_execution:capability};
const event:ToolEvent={schema_version:1,command_id:'opencode',profile_sha256:profile.profile_sha256,protocol:'opencode-sdk-v1',kind:'turn_completed',session_id:'ses_original',message:'Original progress'};
const evidence=():ToolEvidence=>({profile,session_id:'ses_original',terminal:'completed',event_count:4,usage_state:'partial',usage:{input_tokens:0,output_tokens:5},opencode_execution:{configuration_sha256:capability.configuration_sha256,session_id:'ses_original',user_message_id:'msg_request',assistant_message_id:'msg_response',requested_provider:'owned',requested_model:'team/model-a',acknowledged_provider:'owned',acknowledged_model:'team/model-a',finish:'stop',completed_at:'2026-10-28T01:00:00Z',server_stopped:true,abort_acknowledged:false}});
let host:HTMLDivElement,root:Root;
beforeEach(async()=>{vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);root=createRoot(host);await setLocale('en-US',{persist:false});});
afterEach(async()=>{await act(async()=>root.unmount());host.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});

it('shows a current SDK progress stream without turning turn_completed into successful execution',async()=>{
 await act(async()=>root.render(<ToolExecutionFacts state="running" events={[event]}/>));
 const card=host.querySelector('[data-opencode-run-state="running"]');
 expect(card).not.toBeNull();expect(card?.textContent).toContain('Running');expect(card?.textContent).not.toContain('Tool turn completed');
 expect(card?.textContent).toContain('final execution receipt has not been recorded');
 expect(host.querySelectorAll('select,input')).toHaveLength(0);
});

it('discloses original model and message provenance only as tool facts, with bounded current-run test actions',async()=>{
 const opened=vi.fn(),tests=Array.from({length:23},(_,index)=>({id:'test-'+index,name:'Independent test '+index,state:index?'pending':'succeeded',onOpen:()=>opened(index)}));
 await act(async()=>root.render(<ToolExecutionFacts state="succeeded" evidence={evidence()} tests={tests}/>));
 const card=host.querySelector('[data-opencode-run-state="completed"]')!;
 expect(card.textContent).toContain('Tool turn completed');expect(card.textContent).toContain('msg_request');expect(card.textContent).toContain('msg_response');
 expect(card.textContent).toContain('Tool-acknowledged model configuration');expect(card.textContent).toContain('Not acknowledged');
 const disclosures=card.querySelectorAll('details');expect(disclosures.length).toBeGreaterThan(0);expect([...disclosures].every(item=>!item.open)).toBe(true);
 const followUp=[...disclosures].find(item=>item.querySelector('summary')?.textContent==='Continue verification')!;
 expect(followUp.textContent).toContain('Showing the first 20 of 23');expect(followUp.querySelectorAll('button')).toHaveLength(20);
 expect(followUp.textContent).toContain('authorized reviewer');
 await act(async()=>followUp.querySelector('summary')!.click());expect(followUp.open).toBe(true);
 await act(async()=>followUp.querySelectorAll('button')[3].click());expect(opened).toHaveBeenCalledExactlyOnceWith(3);
});

it('keeps unknown or mismatched execution evidence from becoming a positive completion or stop claim',async()=>{
 const bad=evidence();bad.opencode_execution={...bad.opencode_execution!,session_id:'ses_foreign'};
 await act(async()=>root.render(<ToolExecutionFacts state="succeeded" evidence={bad}/>));
 const card=host.querySelector('[data-opencode-run-state="unknown"]')!;
 expect(card).not.toBeNull();expect(card.textContent).toContain('does not match');expect(card.textContent).not.toContain('The private tool service has stopped');
 expect(card.textContent).not.toContain('Tool turn completed');
});

it('preserves the fixed preview treatment and updates CNEN run copy without carrying disclosures to another session',async()=>{
 await act(async()=>root.render(<OpenCodeExecutionFacts capability={capability}/>));
 expect(host.textContent).toContain('Fixed OpenCode execution');expect(host.querySelector('[data-opencode-run-state]')).toBeNull();
 await act(async()=>root.render(<ToolExecutionFacts state="succeeded" evidence={evidence()}/>));
 const details=host.querySelector('[data-opencode-run-state] details')! as HTMLDetailsElement;
 await act(async()=>details.querySelector('summary')!.click());expect(details.open).toBe(true);
 await act(async()=>setLocale('zh-CN',{persist:false}));expect(host.textContent).toContain('工具回合已完成');expect(details.open).toBe(true);
 const next=evidence();next.session_id='ses_next';next.opencode_execution={...next.opencode_execution!,session_id:'ses_next',user_message_id:'msg_next_request',assistant_message_id:'msg_next_response'};
 await act(async()=>root.render(<ToolExecutionFacts state="succeeded" evidence={next}/>));
 expect((host.querySelector('[data-opencode-run-state] details') as HTMLDetailsElement).open).toBe(false);expect(host.textContent).not.toContain('ses_original');
});

it('uses only the exact original frozen workflow and current node binding for a live model fact',()=>{
 const declared={id:'develop',type:'agent',command:['opencode'],max_retries:0,config:{tool_profile_id:'profile-original',tool_profile_sha256:profile.profile_sha256,opencode_execution:capability}};
 const node={id:'develop',type:'agent',command:['opencode'],tool_profile_id:'profile-original',tool_profile_sha256:profile.profile_sha256,attempt:1,fence:2,state:'running'};
 const definition={id:'workflow-original',version:3,nodes:[declared]},run={id:'run-original',workflow_id:'workflow-original',workflow_version:3,nodes:[node]};
 expect(frozenOpenCodeForNode(definition,run,node)).toEqual(capability);
 expect(frozenOpenCodeForNode({...definition,version:4},run,node)).toBeUndefined();
 expect(frozenOpenCodeForNode({...definition,id:'other-workflow'},run,node)).toBeUndefined();
 expect(frozenOpenCodeForNode({...definition,id:undefined,version:undefined},{...run,workflow_id:undefined,workflow_version:undefined},node)).toBeUndefined();
 for(const changed of [{...node,command:['other']},{...node,tool_profile_sha256:'d'.repeat(64)},{...node,tool_profile_id:'other-profile'},{...node,fence:3}])expect(frozenOpenCodeForNode(definition,run,changed)).toBeUndefined();
 for(const field of ['model_execution','codex_execution','provider','mcp_servers','max_output_tokens'])expect(frozenOpenCodeForNode({...definition,nodes:[{...declared,config:{...declared.config,[field]:null}}]},run,node)).toBeUndefined();
});

it('separates platform failure or reconciliation from a genuinely completed tool turn and a known stopped server',()=>{
 expect(openCodeRunFacts({state:'failed',evidence:evidence()})).toMatchObject({phase:'completed',notice:'run_platform_failed',positiveStop:true});
 expect(openCodeRunFacts({state:'reconciling',evidence:evidence()})).toMatchObject({phase:'unknown',notice:'run_reconciling',positiveStop:true});
 const unknown=evidence();unknown.terminal='unknown';
 expect(openCodeRunFacts({state:'reconciling',evidence:unknown})).toMatchObject({phase:'unknown',positiveStop:true});
 const step=evidence();step.opencode_execution={...step.opencode_execution!,finish:'step_finish'};
 expect(openCodeRunFacts({state:'succeeded',evidence:step})).toMatchObject({phase:'unknown',notice:'run_mismatch',positiveStop:false});
});

it('does not invent a queued or executed tool for skipped and pre-attempt cancelled nodes',()=>{
 for(const state of ['skipped','cancelled'])expect(openCodeRunFacts({state,fixedOpenCode:capability})).toMatchObject({phase:'unrecorded',positiveStop:false});
 expect(openCodeRunFacts({state:'ready',fixedOpenCode:capability})).toMatchObject({phase:'pending',positiveStop:false});
 expect(openCodeRunFacts({state:'failed',fixedOpenCode:capability})).toMatchObject({phase:'failed',notice:'run_failed_unreported',positiveStop:false});
 const stopped=evidence();stopped.terminal='unknown';stopped.opencode_execution={...stopped.opencode_execution!,abort_acknowledged:true};
 expect(openCodeRunFacts({state:'cancelled',evidence:stopped})).toMatchObject({phase:'stopped',positiveStop:true});
});
