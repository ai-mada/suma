import {validACPCapability} from './acp-execution-model';
import {codexBindingPresent} from './codex-execution-model';
import {validClaudeSDKCapability} from './claude-sdk-execution-model';

const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
const positive=(value:unknown):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>0;
const identifier=(value:unknown,max=128):value is string=>typeof value==='string'&&value.length>0&&value.length<=max&&/^[A-Za-z0-9]/.test(value)&&!value.match(/[^A-Za-z0-9_.:/@+\-]/);
const nodeID=(value:unknown):value is string=>typeof value==='string'&&value.length>0&&value.length<=100&&!/[^A-Za-z0-9_.-]/.test(value)&&value!=='.'&&value!=='..';
const hash=(value:unknown):value is string=>typeof value==='string'&&value.length===64&&/^[a-f0-9]+$/.test(value);
function toolProtocol(config:Record<string,unknown>){
 if(['model_execution','opencode_execution','claude_execution','octop_execution'].some(key=>Object.hasOwn(config,key)))return;
 const acp=Object.hasOwn(config,'acp_execution'),codex=Object.hasOwn(config,'codex_execution'),claudeSDK=Object.hasOwn(config,'claude_sdk_execution');
 if(Number(acp)+Number(codex)+Number(claudeSDK)!==1)return;
 if(acp&&validACPCapability(config.acp_execution))return 'acp-stdio-v1';
 if(codex&&codexBindingPresent(config.codex_execution))return 'codex-app-server-v1';
 if(claudeSDK&&validClaudeSDKCapability(config.claude_sdk_execution))return 'claude-sdk-v1';
}

/** Select only the protocol frozen for this exact current attempt. Terminal
 * attempts remain readable; a protocol match never authorizes a decision. */
export function frozenToolPermissionProtocolForNode(definition:unknown,run:unknown,node:unknown):'acp-stdio-v1'|'codex-app-server-v1'|'claude-sdk-v1'|undefined{
 if(!object(definition)||!object(run)||!object(node)||!identifier(definition.id)||definition.id!==run.workflow_id||!positive(definition.version)||definition.version!==run.workflow_version||!Array.isArray(definition.nodes)||!Array.isArray(run.nodes)||!nodeID(node.id)||!positive(node.attempt)||!positive(node.fence))return;
 const declarations=definition.nodes.filter(item=>object(item)&&item.id===node.id),actuals=run.nodes.filter(item=>object(item)&&item.id===node.id),declared=declarations[0],actual=actuals[0];
 if(declarations.length!==1||actuals.length!==1||!object(declared)||!object(actual)||declared.type!=='agent'||actual.type!=='agent'||node.type!=='agent'||!object(declared.config))return;
 const config=declared.config,command=declared.command;
 if(!identifier(config.tool_profile_id)||!hash(config.tool_profile_sha256)||!Array.isArray(command)||command.length!==1||!identifier(command[0])||!identifier(actual.worker_id,150))return;
 for(const current of [actual,node])if(current.attempt!==node.attempt||current.fence!==node.fence||current.worker_id!==actual.worker_id||current.tool_profile_id!==config.tool_profile_id||current.tool_profile_sha256!==config.tool_profile_sha256||!Array.isArray(current.command)||current.command.length!==1||current.command[0]!==command[0])return;
 return toolProtocol(config);
}

/** Match the frozen profile for an exact-question link even when a retry
 * has no current worker. The caller must still read the durable attempt. */
export function frozenToolInputProtocolForNode(definition:unknown,run:unknown,node:unknown){
 if(!object(definition)||!object(run)||!object(node)||!identifier(definition.id)||definition.id!==run.workflow_id||!positive(definition.version)||definition.version!==run.workflow_version||!Array.isArray(definition.nodes)||!Array.isArray(run.nodes)||typeof node.id!=='string'||node.id.length>150)return;
 const actuals=run.nodes.filter(item=>object(item)&&item.id===node.id),actual=actuals[0];
 if(actuals.length!==1||!object(actual)||actual.type!=='agent'||node.type!=='agent')return;
 const mapped=/^([A-Za-z0-9][A-Za-z0-9_.-]{0,99})\[(0|[1-9][0-9]*)\]$/.exec(node.id);
 if(!mapped&&!nodeID(node.id))return;
 const declarations=definition.nodes.filter(item=>object(item)&&item.id===(mapped?.[1]??node.id)),source=declarations[0];
 if(declarations.length!==1||!object(source))return;
 const declared=mapped?(source.type==='map'&&object(source.config)?source.config.template:undefined):source;
 if(!object(declared)||declared.type!=='agent'||!object(declared.config))return;
 const config=declared.config,command=declared.command;
 if(!identifier(config.tool_profile_id)||!hash(config.tool_profile_sha256)||!Array.isArray(command)||command.length!==1||!identifier(command[0]))return;
 for(const row of [actual,node])if(row.tool_profile_id!==config.tool_profile_id||row.tool_profile_sha256!==config.tool_profile_sha256||!Array.isArray(row.command)||row.command.length!==1||row.command[0]!==command[0])return;
 return toolProtocol(config);
}
