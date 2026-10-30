// @vitest-environment jsdom
import {expect,it} from 'vitest';
import {parseToolManifest,taskToolProtocol,toolBinding,type ToolProfile} from './tool-model';
import {localDiscoveryAdapters,readInstalledToolDiscovery} from './tool-quick-connect-model';

const capability=()=>({model:'claude-sonnet-5',model_source:'administrator_cli_argument',configuration_sha256:'c'.repeat(64),budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login'});
const observation=()=>({available:true,reason:'',profile:{command_id:'claude-develop',adapter:'claude',protocol:'claude-jsonl-v1',profile_sha256:'a'.repeat(64),executable_sha256:'b'.repeat(64),tool_version:'2.1.314 (Claude Code)',read_only:false,claude_execution:capability()}});
it('accepts only the fixed Claude CLI inventory and preserves its model in task binding',()=>{
 const [item]=parseToolManifest(JSON.stringify([observation()]));expect(taskToolProtocol(item.profile)).toBe(true);
 const tool={id:'tool',profile:item.profile} as ToolProfile;
 const node=toolBinding({config:{goal:'Approved original goal'}},tool);
 expect(node.config).toMatchObject({goal:'Approved original goal',claude_execution:capability()});
 expect(node.max_retries).toBe(0);
 expect(localDiscoveryAdapters.has('claude-code-cli')).toBe(true);
 expect(readInstalledToolDiscovery({status:'observed',report:{protocol:'suma-installed-tools-v1',items:[{adapter_id:'claude-code-cli',product:'Claude Code',installed:false,launch_supported:false,reason_code:'not_installed'}]}},'claude-code-cli')?.product).toBe('Claude Code');
 for(const mutation of [
  {tool_version:'2.1.315'},
  {model_execution_protocol:'suma-model-execution-v1'},
  {workspace_tools_version:'attempt-workspace-v1'},
  {claude_execution:{...capability(),budget_mode:'enforce'}},
  {claude_execution:{...capability(),model:'sonnet'}},
 ])expect(()=>parseToolManifest(JSON.stringify([{...observation(),profile:{...observation().profile,...mutation}}]))).toThrow();
 expect(()=>parseToolManifest(JSON.stringify([{...observation(),available:false,profile:{...observation().profile,tool_version:'2.1.315'}}]))).toThrow();
});
