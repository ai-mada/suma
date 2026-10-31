import {expect,it} from 'vitest';
import {parseToolManifest,taskToolProtocol,toolBinding,type ToolProfile} from './tool-model';
import {localDiscoveryAdapters,matchingQuickTools,quickConnectAdapters} from './tool-quick-connect-model';
import type {AdapterDescriptor} from './AdapterConnections';
const cap={runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:'a'.repeat(64),model:'explicit-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'};
const definition={command_id:'qoder-develop',adapter:'qoder',protocol:'acp-stdio-v1',profile_sha256:'b'.repeat(64),executable_sha256:'c'.repeat(64),tool_version:'reviewed full version',read_only:false,acp_execution:cap};
const parse=(patch:Record<string,unknown>={},available=true)=>parseToolManifest(JSON.stringify([{profile:{...definition,...patch},available,reason:available?'observed':'unavailable'}]));
it('imports actual ACP inventory without granting gateway or unobserved execution capability',()=>{
 expect(taskToolProtocol(parse()[0].profile)).toBe(true);
 expect(taskToolProtocol(parse({acp_execution:undefined},false)[0].profile)).toBe(false);
 for(const patch of [{acp_execution:undefined},{read_only:true},{model_execution_protocol:'suma-model-execution-v1'},{workspace_tools_version:'attempt-workspace-v1'},{adapter:'octop'},{acp_execution:{...cap,budget_mode:'hard'}},{acp_execution:{...cap,credential_env_ref:'PRIVATE_TOKEN'}}])expect(()=>parse(patch)).toThrow();
});
it('freezes ACP capability with zero retries and removes stale binding when changing tools',()=>{
 const profile={id:'profile',profile:definition} as ToolProfile;
 const bound=toolBinding({config:{acp_execution:{model:'stale'},claude_execution:{model:'stale'},other:'kept'}},profile);
 expect(bound).toMatchObject({command:['qoder-develop'],max_retries:0,config:{acp_execution:cap,other:'kept'}});
 expect((bound.config as Record<string,unknown>).acp_execution).not.toBe(cap);
 expect(bound.config).not.toHaveProperty('claude_execution');
 expect(toolBinding(bound).config).toEqual({other:'kept'});
});
it('keeps Qoder reverse MCP and unscanned installations separate from observed Runner profiles',()=>{
 const adapter={id:'qoder-acp',adapter:'qoder',protocol:'acp-stdio-v1',direction:'suma_to_tool',implementation:'implemented',setup_kind:'runner_profile'} as AdapterDescriptor;
 expect(quickConnectAdapters([adapter,{...adapter,id:'qoder-client',direction:'tool_to_suma'}])).toEqual([adapter]);
 expect(localDiscoveryAdapters.has('qoder-acp')).toBe(false);
 expect(matchingQuickTools(parse(),adapter)).toHaveLength(1);
 expect(matchingQuickTools(parse(),{...adapter,direction:'tool_to_suma'})).toEqual([]);
});
