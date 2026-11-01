// @vitest-environment jsdom
import {act} from 'react';
import {createRoot, type Root} from 'react-dom/client';
import {afterEach, beforeEach, expect, it, vi} from 'vitest';
import {ApiViewProvider} from './api-context';
import {setLocale} from './i18n';
import {ModelExecutionPanel} from './ModelExecutionPanel';
import {modelMoneyMicros, modelSelection, validModelOptions} from './model-execution-model';
import type {ModelExecutionOptions} from '../../../packages/contracts/src';

const scope={project:'project-a',profile:'tool-a',version:2,sha256:'a'.repeat(64)};
const session={user:{id:'user-a',name:'Owner',email:'owner@example.test'},organization:{id:'org-a',name:'Org'},projects:[]};
function options():ModelExecutionOptions{return {codex_supported:false,codex_reason:'',project_id:scope.project,tool_profile_id:scope.profile,tool_profile_version:2,tool_profile_sha256:scope.sha256,observed_at:'2026-10-27T01:00:00Z',supported:true,parameter_limits:{max_steps:128,max_output_tokens:131072},reason:'',items:[{deployment:{deployment_id:'model-a',revision:3,deployment_sha256:'b'.repeat(64),protocol:'openai-chat-completions-v1',model:'actual-a',endpoint_origin:'http://127.0.0.1:1234',endpoint_config_sha256:'c'.repeat(64),context_window:8192},cost_state:'unpriced',default_parameters:{max_steps:8,max_output_tokens:4096},input_token_reservation:65536,output_token_reservation:4096}]};}
let root:Root|undefined,host:HTMLDivElement;
beforeEach(async()=>{await setLocale('en-US',{persist:false});host=document.createElement('div');document.body.append(host);vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);});
afterEach(async()=>{if(root)await act(async()=>root!.unmount());root=undefined;host.remove();vi.unstubAllGlobals();vi.restoreAllMocks();await setLocale('zh-CN',{persist:false});});
async function settle(){await act(async()=>{await new Promise(resolve=>setTimeout(resolve,0));});}
it('T192 shows the fixed SDK configuration without fetching models or offering a gateway budget',async()=>{
 const changed=vi.fn(),fetch=vi.fn();vi.stubGlobal('fetch',fetch);
 const sdk={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:'b'.repeat(64),model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported={false} fixedClaudeSDK={sdk} disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();
 expect(host.querySelector('[data-claude-sdk-execution-facts]')).not.toBeNull();expect(host.textContent).toContain(sdk.model);
 expect(host.querySelector('[data-model-select],[data-model-token-budget],[data-model-cost-budget]')).toBeNull();expect(fetch).not.toHaveBeenCalled();expect(changed.mock.lastCall?.[0]).toEqual({ready:true,selection:undefined});
});
it('reads options with GET only, explicitly selects the exact revision and explains unpriced execution in both languages',async()=>{
 const changed=vi.fn();let method='';vi.stubGlobal('fetch',vi.fn(async(_url:string,init?:RequestInit)=>{method=init?.method||'GET';return method==='GET'?Response.json(options()):Response.json({error:{code:'invalid',message:'GET required'}},{status:405});}));
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();
 expect(method).toBe('GET');expect(changed.mock.lastCall?.[0].ready).toBe(false);
 const select=host.querySelector<HTMLSelectElement>('[data-model-select]')!;expect(select).not.toBeNull();
 await act(async()=>{select.value='b'.repeat(64);select.dispatchEvent(new Event('change',{bubbles:true}));});
 expect(changed.mock.lastCall?.[0]).toMatchObject({ready:true,selection:{deployment_id:'model-a',deployment_revision:3,deployment_sha256:'b'.repeat(64),parameters:{max_steps:8,max_output_tokens:4096}}});
 expect(host.textContent).toContain('unpriced');expect(host.querySelector<HTMLInputElement>('[data-model-cost-budget]')?.disabled).toBe(true);
 await act(async()=>{await setLocale('zh-CN',{persist:false});});expect(host.textContent).toContain('费用未定价');
});
it('rejects a response for another tool scope without exposing or adopting its models',async()=>{
 const changed=vi.fn();vi.stubGlobal('fetch',vi.fn(async()=>Response.json({...options(),tool_profile_id:'another-tool'})));
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();
 expect(host.querySelector('[data-model-select]')).toBeNull();expect(host.textContent).not.toContain('actual-a');expect(changed.mock.lastCall?.[0].ready).toBe(false);
});
it('preserves exact decimal budgets and blocks unaffordable or unpriced selections',()=>{
 expect(modelMoneyMicros('0.000057')).toBe(57);expect(modelMoneyMicros('0.0000001')).toBeUndefined();expect(modelMoneyMicros('1e3')).toBeUndefined();
 const page=options(),draft={deployment:'b'.repeat(64),steps:'2',output:'100',tokens:'16484',cost:''};
 expect(validModelOptions(page,scope)).toBe(true);expect(modelSelection(page,draft)?.budget?.token_limit).toBe(16484);
 expect(modelSelection(page,{...draft,tokens:'16483'})).toBeUndefined();expect(modelSelection(page,{...draft,cost:'0.01'})).toBeUndefined();expect(modelSelection(page,{...draft,steps:'129'})).toBeUndefined();
});

it('discovers fixed tools only from an exact scoped response without weakening required native selection',async()=>{
 const changed=vi.fn();vi.stubGlobal('fetch',vi.fn(async()=>Response.json({...options(),supported:false,items:[]})));
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported={false} discover disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();
 expect(changed.mock.lastCall?.[0]).toEqual({ready:true,selection:undefined});
 await act(async()=>root!.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported discover disabled={false} onChange={changed}/></ApiViewProvider>));await settle();
 expect(changed.mock.lastCall?.[0].ready).toBe(false);
});
it('T118 uses shared SDK model selection and current dynamic bounds without a fixed-model shortcut',async()=>{
 const page=options();page.parameter_limits={max_steps:64,max_output_tokens:65536};
 expect(validModelOptions(page,scope)).toBe(true);expect(validModelOptions({...page,parameter_limits:undefined},scope)).toBe(false);
 expect(validModelOptions({...page,fixed_octop:{harness_version:'1.0.5',acp_sdk_version:'0.12.1'}} as ModelExecutionOptions,scope)).toBe(false);
 const changed=vi.fn(),fetcher=vi.fn(async()=>Response.json(page));vi.stubGlobal('fetch',fetcher);
 await act(async()=>{root=createRoot(host);root.render(<ApiViewProvider session={session}><ModelExecutionPanel scope={scope} supported disabled={false} onChange={changed}/></ApiViewProvider>);});await settle();
 const select=host.querySelector<HTMLSelectElement>('[data-model-select]')!;await act(async()=>{select.value='b'.repeat(64);select.dispatchEvent(new Event('change',{bubbles:true}));});
 expect(host.querySelector<HTMLInputElement>('[data-model-steps]')?.max).toBe('64');expect(host.querySelector<HTMLInputElement>('[data-model-output]')?.max).toBe('65536');expect(changed.mock.lastCall?.[0].ready).toBe(true);expect(fetcher).toHaveBeenCalledTimes(1);
 expect(modelSelection(page,{deployment:'b'.repeat(64),steps:'64',output:'65536',tokens:'',cost:''})?.parameters).toEqual({max_steps:64,max_output_tokens:65536});expect(modelSelection(page,{deployment:'b'.repeat(64),steps:'65',output:'65536',tokens:'',cost:''})).toBeUndefined();
});
