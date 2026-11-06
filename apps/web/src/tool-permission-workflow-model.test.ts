import {expect,it} from 'vitest';
import type {ACPExecutionCapability,CodexBinding} from '../../../packages/contracts/src';
import type {Run,Workflow} from './api';
import {frozenToolInputProtocolForNode,frozenToolPermissionProtocolForNode} from './tool-permission-workflow-model';

const hash='a'.repeat(64);
const acp:ACPExecutionCapability={runtime:'qoder',protocol_version:1,sdk_version:'0.13.5',configuration_sha256:hash,model:'fixed-model',model_source:'administrator_session_config',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',auth_strategy:'existing_cli_login',permission_mode:'human_once'};
const codex:CodexBinding={protocol:'codex-app-server-v1',configuration_sha256:hash,catalog_sha256:hash,model:'fixed-model',reasoning_effort:'high',budget_mode:'observe_only',identity_evidence:'configuration_ack_only',binding_sha256:hash};
const claudeSDK={sdk_version:'0.3.314',cli_version:'2.1.314',model:'claude-sonnet-5',configuration_sha256:hash,model_source:'administrator_sdk_option',budget_mode:'observe_only',identity_evidence:'sdk_init_model_ack_only',auth_strategy:'api_key_env_ref'};
function fixture(protocol:'acp'|'codex'){
 const config={tool_profile_id:'profile-a',tool_profile_sha256:hash,...(protocol==='acp'?{acp_execution:acp}:{codex_execution:codex})};
 const definition:Workflow={id:'workflow-a',name:'Fixed workflow',version:5,nodes:[{id:'develop',name:'Develop',type:'agent',command:['native-command'],config}],edges:[]};
 const run:Run={id:'run-a',workflow_id:definition.id,workflow_name:definition.name,workflow_version:5,version:8,state:'running',nodes:[{id:'develop',name:'Develop',type:'agent',command:['native-command'],state:'running',attempt:2,fence:3,worker_id:'worker-a',tool_profile_id:'profile-a',tool_profile_sha256:hash}],created_at:'',updated_at:''};
 return {definition,run,node:{...run.nodes[0],command:['native-command']}};
}

it('identifies the exact current ACP or Codex binding without excluding terminal evidence',()=>{
 for(const [driver,expected] of [['acp','acp-stdio-v1'],['codex','codex-app-server-v1']] as const){const {definition,run,node}=fixture(driver);expect(frozenToolPermissionProtocolForNode(definition,run,node)).toBe(expected);expect(frozenToolPermissionProtocolForNode(definition,{...run,state:'succeeded',nodes:[{...run.nodes[0],state:'succeeded'}]},{...node,state:'succeeded'})).toBe(expected);}
});

it('keeps the frozen question protocol across an unclaimed retry and a mapped node',()=>{
 const {definition,run,node}=fixture('codex');
 const retry={...node,state:'ready',worker_id:'',fence:0};
 expect(frozenToolInputProtocolForNode(definition,{...run,nodes:[retry]},retry)).toBe('codex-app-server-v1');
 const child={...retry,id:'mapped[2]'};
 const mapped={...definition,nodes:[{id:'mapped',name:'Mapped tasks',type:'map',config:{template:definition.nodes[0]}}]};
 expect(frozenToolInputProtocolForNode(mapped,{...run,nodes:[child]},child)).toBe('codex-app-server-v1');
 expect(frozenToolInputProtocolForNode(mapped,{...run,nodes:[child]},{...child,tool_profile_sha256:'b'.repeat(64)})).toBeUndefined();
 expect(frozenToolInputProtocolForNode(definition,{...run,nodes:[child]},child)).toBeUndefined();
});

it('T192 exposes Claude SDK decisions only for its exact frozen original attempt and independent capability',()=>{
 const {definition,run,node}=fixture('acp'),declared=definition.nodes[0];
 const config={tool_profile_id:'profile-a',tool_profile_sha256:hash,claude_sdk_execution:claudeSDK};
 const sdkDefinition={...definition,nodes:[{...declared,config}]};
 expect(frozenToolPermissionProtocolForNode(sdkDefinition,run,node)).toBe('claude-sdk-v1');
 expect(frozenToolPermissionProtocolForNode(sdkDefinition,{...run,state:'succeeded'},node)).toBe('claude-sdk-v1');
 for(const patch of [{claude_sdk_execution:null},{claude_sdk_execution:{...claudeSDK,sdk_version:'latest'}},{acp_execution:acp},{codex_execution:codex},{claude_execution:{model:'claude-sonnet-5'}}])expect(frozenToolPermissionProtocolForNode({...sdkDefinition,nodes:[{...declared,config:{...config,...patch}}]},run,node)).toBeUndefined();
 expect(frozenToolPermissionProtocolForNode(sdkDefinition,run,{...node,fence:4})).toBeUndefined();
});

it('rejects ambiguous or foreign workflow revisions and non-agent node sources',()=>{
 const {definition,run,node}=fixture('codex'),declared=definition.nodes[0],actual=run.nodes[0];
 const cases=[
  {name:'different workflow',definition,run:{...run,workflow_id:'other-workflow'},node},
  {name:'different immutable revision',definition,run:{...run,workflow_version:6},node},
  {name:'missing workflow identity',definition:{...definition,id:undefined},run:{...run,workflow_id:undefined},node},
  {name:'duplicate declared node',definition:{...definition,nodes:[declared,{...declared}]},run,node},
  {name:'duplicate actual node',definition,run:{...run,nodes:[actual,{...actual}]},node},
  {name:'foreign selected node',definition,run,node:{...node,id:'other-node'}},
  {name:'declared command node',definition:{...definition,nodes:[{...declared,type:'command'}]},run,node},
  {name:'actual test node',definition,run:{...run,nodes:[{...actual,type:'test'}]},node},
  {name:'selected task node',definition,run,node:{...node,type:'task'}},
 ];
 for(const value of cases)expect(frozenToolPermissionProtocolForNode(value.definition,value.run,value.node),value.name).toBeUndefined();
});

it('rejects drifted execution ownership, commands and competing native capabilities',()=>{
 const {definition,run,node}=fixture('acp'),declared=definition.nodes[0],actual=run.nodes[0],config=declared.config!;
 const cases=[
  {name:'current profile changed',definition,run:{...run,nodes:[{...actual,tool_profile_id:'other-profile'}]},node},
  {name:'selected profile digest changed',definition,run,node:{...node,tool_profile_sha256:'b'.repeat(64)}},
  {name:'invalid fixed profile digest',definition:{...definition,nodes:[{...declared,config:{...config,tool_profile_sha256:'bad'}}]},run:{...run,nodes:[{...actual,tool_profile_sha256:'bad'}]},node:{...node,tool_profile_sha256:'bad'}},
  {name:'changed current command',definition,run:{...run,nodes:[{...actual,command:['other-command']}]},node},
  {name:'extra declared command arguments',definition:{...definition,nodes:[{...declared,command:['native-command','--extra']}]},run,node},
  {name:'previous attempt and fence',definition,run,node:{...node,attempt:1,fence:2}},
  {name:'unsafe current fence',definition,run:{...run,nodes:[{...actual,fence:Number.MAX_SAFE_INTEGER+1}]},node:{...node,fence:Number.MAX_SAFE_INTEGER+1}},
  {name:'not yet attempted',definition,run:{...run,nodes:[{...actual,attempt:0}]},node:{...node,attempt:0}},
  {name:'missing current worker',definition,run:{...run,nodes:[{...actual,worker_id:''}]},node:{...node,worker_id:''}},
  {name:'different selected worker',definition,run,node:{...node,worker_id:'other-worker'}},
  {name:'both native protocols',definition:{...definition,nodes:[{...declared,config:{...config,codex_execution:codex}}]},run,node},
  {name:'foreign native protocol',definition:{...definition,nodes:[{...declared,config:{...config,acp_execution:{...acp,protocol_version:2}}}]},run,node},
  {name:'another tool capability',definition:{...definition,nodes:[{...declared,config:{...config,opencode_execution:{protocol:'opencode-http-v1'}}}]},run,node},
 ];
 for(const value of cases)expect(frozenToolPermissionProtocolForNode(value.definition,value.run,value.node),value.name).toBeUndefined();
});
