import type {ClaudeSDKExecutionCapability,ClaudeSDKExecutionEvidence,ToolEvidence} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
import {validClaudeSDKPermissionEvidence,validClaudeSDKInputEvidence} from './claude-sdk-interaction-model';

const keys=['sdk_version','cli_version','model','configuration_sha256','model_source','budget_mode','identity_evidence','auth_strategy'] as const;
const object=(value:unknown):value is Record<string,unknown>=>!!value&&typeof value==='object'&&!Array.isArray(value)&&[Object.prototype,null].includes(Object.getPrototypeOf(value));
const hash=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const model=(value:unknown):value is string=>typeof value==='string'&&value.length<=256&&/^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$/.test(value);
const own=(value:Record<string,unknown>,key:string)=>Object.hasOwn(value,key);
const mixed=['model_execution','codex_execution','opencode_execution','octop_execution','claude_execution','acp_execution'];
const overrides=[...mixed,'model','model_id','effort','model_provider','provider','base_url','endpoint','api_key','cwd','argv','service_tier','config','configuration','sandbox','approval_policy','windows','windows_sandbox','max_steps','temperature','top_p','reasoning_effort','max_tokens','max_output_tokens','max_cost','max_cost_micros','hard_budget','token_budget','budget','resume','continue','session_id','tools','mcp_servers','mcp_config','permissions','permission_mode','permission_prompts','settings','settings_sources','allowed_tools','disallowed_tools','agent','agents','home','executable','args','env','env_from','api_key_env_ref','credential_env_ref','parameters','model_parameters','claude_sdk'];
export function validClaudeSDKCapability(value:unknown):value is ClaudeSDKExecutionCapability{
 return object(value)&&Reflect.ownKeys(value).length===keys.length&&keys.every(key=>own(value,key))&&value.sdk_version==='0.3.314'&&value.cli_version==='2.1.314'&&model(value.model)&&hash(value.configuration_sha256)&&value.model_source==='administrator_sdk_option'&&value.budget_mode==='observe_only'&&value.identity_evidence==='sdk_init_model_ack_only'&&value.auth_strategy==='api_key_env_ref';
}
export function sameClaudeSDKCapability(left:unknown,right:unknown):boolean{return validClaudeSDKCapability(left)&&validClaudeSDKCapability(right)&&keys.every(key=>left[key]===right[key]);}
/** Only the fixed original SDK profile may supply execution configuration. */
export function validClaudeSDKPlan(value:unknown):boolean{
 if(!object(value))return false;
 const nodes=object(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[],matches=nodes.filter(node=>object(node)&&node.id===value.agent_node_id),node=matches[0],config=object(node)&&object(node.config)?node.config:undefined;
 if(!own(value,'claude_sdk_execution')&&!matches.some(node=>object(node)&&object(node.config)&&own(node.config,'claude_sdk_execution')))return true;
 return matches.length===1&&object(node)&&node.type==='agent'&&(node.max_retries===undefined||node.max_retries===0)&&Array.isArray(node.command)&&node.command.length===1&&typeof node.command[0]==='string'&&!!node.command[0]&&!!config&&typeof value.tool_profile_id==='string'&&!!value.tool_profile_id&&hash(value.tool_profile_sha256)&&config.tool_profile_id===value.tool_profile_id&&config.tool_profile_sha256===value.tool_profile_sha256&&!mixed.some(key=>own(value,key))&&!overrides.some(key=>own(config,key))&&sameClaudeSDKCapability(value.claude_sdk_execution,config.claude_sdk_execution);
}
export function validClaudeSDKDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean{
 if(!object(value)||!validClaudeSDKPlan(value))return false;
 if(value.claude_sdk_execution===undefined)return true;
 return typeof value.id==='string'&&!!value.id&&value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&!own(body,'model_execution_selection')&&!own(body,'codex_execution_selection');
}
const evidenceKeys=['sdk_version','cli_version','configuration_sha256','session_id','requested_model','acknowledged_model','result_subtype','result_is_error','turn_ended','tools_completed','cancel_requested','host_process_stopped','host_process_tree_stopped','permission_requests','denied_callbacks'];
const integer=(value:unknown,max=32):value is number=>typeof value==='number'&&Number.isSafeInteger(value)&&value>=0&&value<=max;
const text=(value:unknown,max:number):value is string=>typeof value==='string'&&wellFormed(value)&&new TextEncoder().encode(value).length<=max&&value.trim()===value&&!/\p{Cc}/u.test(value);
const sessionID=(value:unknown):value is string=>text(value,150)&&!!value&&!/\p{White_Space}/u.test(value);
/** Source matching validates the original published evidence. It grants no
 * provider, current access, dispatch, quality or acceptance authority. */
export function validClaudeSDKEvidence(value:unknown,fixed:ClaudeSDKExecutionCapability,source:ToolEvidence):value is ClaudeSDKExecutionEvidence{
 if(!validClaudeSDKCapability(fixed)||!object(value)||!evidenceKeys.every(key=>own(value,key))||Reflect.ownKeys(value).some(key=>typeof key!=='string'||!evidenceKeys.includes(key)&&!['permissions','user_inputs','native_permission_denials'].includes(key))||!object(source)||!object(source.profile))return false;
 const profile=source.profile;
 if(profile.adapter!=='claude'||profile.protocol!=='claude-sdk-v1'||profile.read_only!==false||profile.tool_version!=='2.1.314'||profile.user_input_protocol!=='claude-sdk-input-v1'||!sameClaudeSDKCapability(profile.claude_sdk_execution,fixed)||['workspace_tools_version','model_execution_protocol','terminal_session_protocol','test_report_protocol','conflict_repair_protocol','planning','knowledge_extraction','failure_diagnosis','intelligent_operations','dispatch_recommendation','task_conversation_reply','claude_execution','acp_execution','opencode_execution','octop_execution'].some(key=>!!profile[key])||mixed.some(key=>own(source,key))||!['partial','unreported'].includes(source.usage_state)||!['completed','failed','unknown'].includes(source.terminal))return false;
 if(value.sdk_version!==fixed.sdk_version||value.cli_version!==fixed.cli_version||value.configuration_sha256!==fixed.configuration_sha256||value.requested_model!==fixed.model||typeof value.session_id!=='string'||value.session_id!==''&&!sessionID(value.session_id)||(source.session_id??'')!==value.session_id||typeof value.acknowledged_model!=='string'||value.acknowledged_model!==''&&!model(value.acknowledged_model)||!text(value.result_subtype,80)||!['result_is_error','turn_ended','tools_completed','cancel_requested','host_process_stopped','host_process_tree_stopped'].every(key=>typeof value[key]==='boolean')||!integer(value.permission_requests)||!integer(value.denied_callbacks)||own(value,'native_permission_denials')&&!integer(value.native_permission_denials,1000000)||value.host_process_tree_stopped&&!value.host_process_stopped)return false;
 const permissions=value.permissions===undefined?[]:value.permissions,inputs=value.user_inputs===undefined?[]:value.user_inputs;
 if(!Array.isArray(permissions)||!Array.isArray(inputs)||permissions.length>value.permission_requests||permissions.length+inputs.length>32)return false;
 const ids=new Set<string>(),requests=new Set<string>(),callbacks=new Set<string>(),complete=source.terminal==='completed';
 for(const [rows,validate,identity] of [[permissions,validClaudeSDKPermissionEvidence,'permission_id'],[inputs,validClaudeSDKInputEvidence,'input_id']] as const){
  for(const row of rows){
   if(!validate(row)||!object(row)||row.session_id!==value.session_id||row.configuration_sha256!==fixed.configuration_sha256)return false;
   const id=row[identity] as string,request=row.request_id as string,callback=row.callback_id as string;
   if(ids.has(id)||requests.has(request)||callbacks.has(callback))return false;
   ids.add(id);requests.add(request);callbacks.add(callback);
   const observation=row.observation;
   if(complete&&(!object(observation)||!observation.answer_observed||!observation.reply_written||!observation.execution_closed||!!observation.failure_code))return false;
  }
 }
 return !complete||!!value.session_id&&value.acknowledged_model===fixed.model&&value.result_subtype==='success'&&!value.result_is_error&&value.turn_ended===true&&value.tools_completed===true&&!value.cancel_requested&&value.host_process_stopped===true&&value.host_process_tree_stopped===true&&value.denied_callbacks===0&&value.native_permission_denials===0&&permissions.length===value.permission_requests;
}
