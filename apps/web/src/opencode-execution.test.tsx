// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {ModelExecutionPanel} from './ModelExecutionPanel';
import {validModelOptions} from './model-execution-model';
import {validOpenCodePlan} from './opencode-execution-model';
import {validateTaskDispatchResponse} from './task-dispatch-response';
import {ToolWriteRecovery} from './tool-write';
import {parseToolManifest} from './tool-model';
import type {ApiCaller} from './api';

const capability={sdk_version:'1.19.4',server_version:'1.19.4',provider_id:'owned',model:'team/model-a',endpoint_origin:'https://models.example',configuration_sha256:'a'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only'};
const profile={command_id:'opencode',adapter:'opencode',protocol:'opencode-sdk-v1',profile_sha256:'b'.repeat(64),executable_sha256:'c'.repeat(64),tool_version:'1.19.4',read_only:false,workspace_tools_version:'attempt-workspace-v1',opencode_execution:capability};
const body={project_id:'project-a',task_id:'task-a',expected_task_version:2,workflow_id:'flow-a',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',expected_tool_version:3};
const plan={id:'plan-a',project_id:'project-a',task_id:'task-a',task_version:2,workflow_id:'flow-a',workflow_version:1,agent_node_id:'develop',tool_profile_id:'tool-a',tool_profile_version:3,opencode_execution:capability,workflow:{nodes:[{id:'develop',config:{opencode_execution:capability}}]}};
let root:Root|undefined,host:HTMLDivElement|undefined;
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host?.remove();vi.unstubAllGlobals();await setLocale('zh-CN',{persist:false});});

it('accepts only the current SDK contract and actual host capabilities, with no private configuration or old protocol alias',()=>{
 const observation={profile,available:true,reason:'',workspace_observation_protocol:'workspace-observation-v1',workspace_write_protocol:'workspace-write-v1'};
 expect(parseToolManifest(JSON.stringify([observation]))[0].profile.opencode_execution).toEqual(capability);
 for(const bad of [
  {...profile,protocol:'opencode-jsonl-v1'},
  {...profile,opencode_execution:{...capability,api_key:'private'}},
  {...profile,opencode_execution:{...capability,budget_mode:'enforce'}},
  {...profile,conflict_repair_protocol:'suma.candidate-conflict-repair.v1'},
  {...profile,adapter:'suma',protocol:'suma-jsonl-v1'},
 ])expect(()=>parseToolManifest(JSON.stringify([{...observation,profile:bad}]))).toThrow();
});

it('keeps an original dispatch operation after mismatched fixed producer facts and recovers the same key',async()=>{
 const values=new Map<string,string>(),storage={getItem:(key:string)=>values.get(key)??null,setItem:(key:string,value:string)=>{values.set(key,value);},removeItem:(key:string)=>{values.delete(key);}};
 let wrong=true;const calls:unknown[][]=[];
 const api:ApiCaller=async(...args)=>{calls.push(args);return (wrong?{...plan,opencode_execution:{...capability,model:'different'}}:plan) as never;};
 const validate=(value:unknown,request:Parameters<typeof validateTaskDispatchResponse>[1])=>validateTaskDispatchResponse(value,request,api,'project-a');
 const first=new ToolWriteRecovery('opencode-original',storage,api,validate);
 await expect(first.run('/task-dispatch-plans',body)).rejects.toThrow();
 expect(first.pending?.body).toEqual(body);wrong=false;
 const recovered=new ToolWriteRecovery('opencode-original',storage,api,validate);await recovered.restore();
 expect(calls[1]).toEqual(calls[0]);expect(recovered.pending).toBeUndefined();
 expect(validOpenCodePlan({...plan,opencode_execution:undefined})).toBe(false);
 expect(validOpenCodePlan({...plan,model_execution:{model:'foreign'}})).toBe(false);
});

it('rejects a fixed SDK options response that also claims a selectable model lane',()=>{
 const scope={project:'project-a',profile:'tool-a',version:3,sha256:profile.profile_sha256};
 const page={project_id:scope.project,tool_profile_id:scope.profile,tool_profile_version:3,tool_profile_sha256:scope.sha256,observed_at:'2026-10-28T00:00:00Z',supported:false,codex_supported:false,reason:'fixed_by_tool',codex_reason:'',items:[],fixed_opencode:capability};
 expect(validModelOptions(page,scope)).toBe(true);
 expect(validModelOptions({...page,supported:true},scope)).toBe(false);
 expect(validModelOptions({...page,codex_supported:true},scope)).toBe(false);
 expect(validModelOptions({...page,tool_profile_id:'foreign'},scope)).toBe(false);
});

it('shows compact fixed administrator facts in both languages without a selector or another model request',async()=>{
 vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);const fetch=vi.fn();vi.stubGlobal('fetch',fetch);const changed=vi.fn();
 host=document.createElement('div');document.body.append(host);
 await setLocale('en-US',{persist:false});
 await act(async()=>{root=createRoot(host!);root.render(<ModelExecutionPanel scope={{project:'project-a',profile:'tool-a',version:3,sha256:profile.profile_sha256}} fixedOpenCode={capability} supported={false} disabled={false} onChange={changed}/>);});
 expect(host.textContent).toContain('team/model-a');expect(host.textContent).toContain('cost is unpriced');expect(host.querySelector('select')).toBeNull();expect(changed.mock.lastCall?.[0]).toEqual({selection:undefined,ready:true});
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('OpenCode 固定执行配置');expect(host.textContent).toContain('费用未定价');expect(fetch).not.toHaveBeenCalled();
});
