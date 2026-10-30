// @vitest-environment jsdom
import {act} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setLocale} from './i18n';
import {ModelExecutionPanel} from './ModelExecutionPanel';
import {ToolSetupGuide} from './ToolSetupGuide';
import {validClaudeDispatchReceipt,validClaudePlan} from './claude-cli-model';
import {validModelOptions} from './model-execution-model';
const fixed=()=>({model:'claude-sonnet-5',model_source:'administrator_cli_argument',configuration_sha256:'c'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login'});
let host:HTMLDivElement,root:Root|undefined;
beforeEach(async()=>{await setLocale('en-US',{persist:false});vi.stubGlobal('IS_REACT_ACT_ENVIRONMENT',true);host=document.createElement('div');document.body.append(host);});
afterEach(async()=>{if(root)await act(async()=>root?.unmount());root=undefined;host.remove();vi.unstubAllGlobals();});
it('shows the fixed CLI model without gateway selection, a hard cap or a model probe',async()=>{
 const fetcher=vi.fn();vi.stubGlobal('fetch',fetcher);const changed=vi.fn();
 await act(async()=>{root=createRoot(host);root.render(<ModelExecutionPanel scope={{project:'project',profile:'tool',version:1,sha256:'a'.repeat(64)}} fixedClaude={fixed()} supported={false} disabled={false} onChange={changed}/>);});
 expect(host.textContent).toContain('claude-sonnet-5');expect(host.textContent).toContain('platform-enforced budgets are unavailable');expect(host.querySelector('select')).toBeNull();expect(host.querySelector('input')).toBeNull();expect(fetcher).not.toHaveBeenCalled();expect(changed.mock.lastCall?.[0]).toEqual({selection:undefined,ready:true});
});
it('offers a native foreground preset with explicit file and scoped Git permissions',async()=>{
 await act(async()=>{root=createRoot(host);root.render(<ToolSetupGuide expanded/>);});
 const select=host.querySelector<HTMLSelectElement>('.tool-cli-setup-select select')!;await act(async()=>{select.value='claude';select.dispatchEvent(new Event('change',{bubbles:true}));});
 const config=JSON.parse(host.querySelector('pre')!.textContent!)['claude-develop'];
 expect(config.executable).toBe('claude');expect(config.args).toContain('--restricted');expect(config.args).toContain('--safe-mode');expect(config.args).toContain('--no-session-persistence');expect(config.args).not.toContain('--dangerously-skip-permissions');expect(config.args).not.toContain('--resume');expect(config.args).toContain('Read,Edit,Write,Glob,Grep,Bash(git status *),Bash(git diff *),Bash(git add *),Bash(git commit *)');expect(host.textContent).toContain('Managed policies still apply');expect(host.querySelectorAll('select')).toHaveLength(1);
});
it('retains original-operation uncertainty when the saved plan changes its fixed CLI model',()=>{
 const body={project_id:'project',task_id:'task',expected_task_version:2,workflow_id:'workflow',workflow_version:3,agent_node_id:'develop',tool_profile_id:'tool',expected_tool_version:4};
 const plan={id:'plan',project_id:'project',task_id:'task',task_version:2,workflow_id:'workflow',workflow_version:3,agent_node_id:'develop',tool_profile_id:'tool',tool_profile_version:4,claude_execution:fixed(),workflow:{nodes:[{id:'develop',type:'agent',max_retries:0,config:{claude_execution:fixed()}}]}};
 expect(validClaudeDispatchReceipt(plan,body)).toBe(true);
 expect(validClaudePlan({...plan,claude_execution:{...fixed(),model:'claude-other-5'}})).toBe(false);
 expect(validClaudeDispatchReceipt(plan,{...body,expected_tool_version:5})).toBe(false);
 expect(validClaudeDispatchReceipt(plan,{...body,model_execution_selection:null})).toBe(false);
 expect(validClaudePlan({...plan,model_execution:null})).toBe(false);
 const scope={project:'project',profile:'tool',version:4,sha256:'a'.repeat(64)},options={project_id:'project',tool_profile_id:'tool',tool_profile_version:4,tool_profile_sha256:scope.sha256,observed_at:'2026-10-30T00:00:00Z',supported:false,codex_supported:false,codex_reason:'',reason:'fixed_by_tool',items:[],fixed_claude:fixed()};
 expect(validModelOptions(options,scope)).toBe(true);
 expect(validModelOptions({...options,supported:true},scope)).toBe(false);
 expect(validModelOptions({...options,fixed_opencode:null},scope)).toBe(false);
});
