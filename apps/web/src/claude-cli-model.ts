import type {ClaudeExecutionCapability} from '../../../packages/contracts/src';
export const claudeCLIModel='claude-sonnet-5';
const keys=['model','model_source','configuration_sha256','budget_mode','identity_evidence','auth_strategy'] as const;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value);
/** Original fixed CLI model syntax, without implying model availability. */
export function validClaudeCLIModel(value:unknown):value is string{return typeof value==='string'&&value.length<=256&&/^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$/.test(value);}
export function validClaudeCapability(value:unknown):value is ClaudeExecutionCapability{
 return object(value)&&Object.keys(value).length===keys.length&&keys.every(key=>Object.hasOwn(value,key))&&validClaudeCLIModel(value.model)&&value.model_source==='administrator_cli_argument'&&typeof value.configuration_sha256==='string'&&/^[a-f0-9]{64}$/.test(value.configuration_sha256)&&value.budget_mode==='observe_only'&&value.identity_evidence==='configuration_ack_only'&&value.auth_strategy==='existing_cli_login';
}
export function sameClaudeCapability(a:unknown,b:unknown){return validClaudeCapability(a)&&validClaudeCapability(b)&&keys.every(key=>a[key]===b[key]);}
export function validClaudePlan(value:unknown):boolean{
 if(!object(value))return false;
 const nodes=object(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[],node=nodes.find((item:unknown)=>object(item)&&item.id===value.agent_node_id),config=object(node)&&object(node.config)?node.config:undefined;
 if(value.claude_execution===undefined&&config?.claude_execution===undefined)return true;
 if(!object(node)||node.type!=='agent'||node.max_retries!==undefined&&node.max_retries!==0)return false;
 for(const field of ['model_execution','codex_execution','opencode_execution','octop_execution'])if(Object.hasOwn(value,field)||config&&Object.hasOwn(config,field))return false;
 return sameClaudeCapability(value.claude_execution,config?.claude_execution);
}
export function validClaudeDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean{
 if(!validClaudePlan(value)||!object(value))return false;
 if(value.claude_execution===undefined)return true;
 return typeof value.id==='string'&&value.id.length>0&&value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&!Object.hasOwn(body,'model_execution_selection')&&!Object.hasOwn(body,'codex_execution_selection');
}
export function claudeCLIConfiguration(model=claudeCLIModel){
 return {'claude-develop':{adapter:'claude',protocol:'claude-jsonl-v1',executable:'claude',input:'goal',read_only:false,args:['-p','--input-format','text','--output-format','stream-json','--verbose','--no-session-persistence','--safe-mode','--restricted','--settings','{"disableAllHooks":true,"permissions":{"blockReadsOutsideWorkingDirectories":true,"blockWritesOutsideWorkingDirectories":true}}','--strict-mcp-config','--mcp-config','{"mcpServers":{}}','--permission-mode','dontAsk','--permission-prompts','none','--tools','Read,Edit,Write,Glob,Grep,Bash','--allowedTools','Read,Edit,Write,Glob,Grep,Bash(git status *),Bash(git diff *),Bash(git add *),Bash(git commit *)','--model',model]}};
}
