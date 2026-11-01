import type {ACPExecutionCapability,ACPExecutionEvidence} from '../../../packages/contracts/src';
import {wellFormed} from './i18n/catalog';
const max=2147483647,encoder=new TextEncoder(),bytes=(v:string)=>encoder.encode(v).length;
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v)&&(Object.getPrototypeOf(v)===Object.prototype||Object.getPrototypeOf(v)===null);
const fields=(v:Record<string,unknown>,required:readonly string[],optional:readonly string[]=[])=>required.every(k=>Object.hasOwn(v,k))&&Reflect.ownKeys(v).every(k=>typeof k==='string'&&(required.includes(k)||optional.includes(k)));
const integer=(v:unknown,min=0,limit=max):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=min&&v<=limit;
const id=(v:unknown,limit=64):v is string=>typeof v==='string'&&/^(?!\.{1,2}$)[A-Za-z0-9_.-]+$/.test(v)&&v.length<=limit;
const text=(v:unknown,limit=32768):v is string=>typeof v==='string'&&wellFormed(v)&&bytes(v)<=limit&&!/[\p{Cc}]/u.test(v.replace(/[\r\n\t]/g,''));
const token=(v:unknown,limit=128):v is string=>text(v,limit)&&!!v&&v.trim()===v&&!/[\p{Cc}]/u.test(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const capabilityKeys=['runtime','protocol_version','sdk_version','configuration_sha256','model','model_source','budget_mode','identity_evidence','auth_strategy','permission_mode'] as const;
export function validACPCapability(value:unknown,expectedRuntime?:'qoder'|'cursor'):value is ACPExecutionCapability{
 return object(value)&&fields(value,capabilityKeys)&&(expectedRuntime===undefined||value.runtime===expectedRuntime)&&(value.runtime==='qoder'&&value.auth_strategy==='existing_cli_login'||value.runtime==='cursor'&&value.auth_strategy==='explicit_api_key_env')&&value.protocol_version===1&&value.sdk_version==='0.13.5'&&hash(value.configuration_sha256)&&token(value.model,256)&&value.model_source==='administrator_session_config'&&value.budget_mode==='observe_only'&&value.identity_evidence==='configuration_ack_only'&&value.permission_mode==='human_once';
}
export function validACPEvidence(value:unknown):value is ACPExecutionEvidence{
 const required=['configuration_sha256','requested_model','model_selection_acknowledged','prompt_started','prompt_completed','tools_complete','permissions_requested','permissions_denied','cancel_requested','host_process_stopped','host_process_tree_stopped'];
 if(!object(value)||!fields(value,required,['selected_model','session_id','protocol_version','agent_name','agent_version','stop_reason'])||!hash(value.configuration_sha256)||!token(value.requested_model,256)||!['model_selection_acknowledged','prompt_started','prompt_completed','tools_complete','cancel_requested','host_process_stopped','host_process_tree_stopped'].every(k=>typeof value[k]==='boolean')||!integer(value.permissions_requested,0,32)||!integer(value.permissions_denied,0,value.permissions_requested))return false;
 if(['selected_model','session_id','agent_name','agent_version'].some(k=>Object.hasOwn(value,k)&&!token(value[k],256))||Object.hasOwn(value,'protocol_version')&&value.protocol_version!==1)return false;
 if(Object.hasOwn(value,'stop_reason')&&(typeof value.stop_reason!=='string'||!['end_turn','max_tokens','max_turn_requests','refusal','cancelled'].includes(value.stop_reason)))return false;
 if(value.model_selection_acknowledged&&value.selected_model!==value.requested_model||value.prompt_started&&(!value.model_selection_acknowledged||!value.session_id||value.protocol_version!==1)||value.prompt_completed&&(!value.prompt_started||!value.stop_reason)||value.host_process_tree_stopped&&!value.host_process_stopped)return false;
 return true;
}
/** The original workflow and preview must freeze the same administrator
 * capability; no fresh inventory is substituted into historical evidence. */
export function validACPPlan(value:unknown):boolean{
 if(!object(value))return false;
 const nodes=object(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[],matching=nodes.filter(n=>object(n)&&n.id===value.agent_node_id),node=matching[0],config=object(node)&&object(node.config)?node.config:undefined;
 if(value.acp_execution===undefined&&config?.acp_execution===undefined)return true;
 const original=value.acp_execution,fixed=config?.acp_execution;
 if(matching.length!==1||!object(node)||node.type!=='agent'||node.max_retries!==undefined&&node.max_retries!==0||!validACPCapability(original)||!validACPCapability(fixed))return false;
 for(const key of ['model_execution','codex_execution','opencode_execution','octop_execution','claude_execution'])if(Object.hasOwn(value,key)||config&&Object.hasOwn(config,key))return false;
 return capabilityKeys.every(k=>original[k]===fixed[k]);
}
export function validACPDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean{
 if(!validACPPlan(value)||!object(value))return false;
 if(value.acp_execution===undefined)return true;
 return id(value.id)&&value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&!Object.hasOwn(body,'model_execution_selection')&&!Object.hasOwn(body,'codex_execution_selection');
}
