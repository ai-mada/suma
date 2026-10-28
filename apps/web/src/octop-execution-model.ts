import type {ToolExecutionProfile,ToolEvidence} from '../../../packages/contracts/src';
export type OctopExecutionCapability=NonNullable<ToolExecutionProfile['octop_execution']>;
export type OctopExecutionEvidence=NonNullable<ToolEvidence['octop_execution']>;
export type OctopHarnessInputs={python:string;bundle:string;manifest:string;checks:string;provider:string;model:string;endpoint:string;keyEnvironment:string;maxSteps:string;maxOutputTokens:string};
export const octopHarnessPreset={'octop-harness-develop':{adapter:'octop',protocol:'octop-harness-acp-v1',executable:'',args:[] as string[],input:'octop-acp-jsonl',read_only:false}} as const;
const record=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown,max=256,empty=false):value is string=>typeof value==='string'&&(empty||value.length>0)&&value.trim()===value&&new TextEncoder().encode(value).length<=max&&!/[\u0000-\u001f\u007f-\u009f]/.test(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const fields=(value:Record<string,unknown>,keys:readonly string[])=>Object.keys(value).every(key=>keys.includes(key));
function endpoint(value:string,origin=false){
 if(!text(value,2048)||origin&&!/^(https?):\/\/[^/?#@\s]+$/.test(value))return false;
 try{const url=new URL(value);return !url.username&&!url.password&&!url.search&&!url.hash&&(url.protocol==='https:'||url.protocol==='http:'&&['localhost','127.0.0.1','[::1]'].includes(url.hostname));}catch{return false;}
}
/** Public inventory contains a fixed configuration acknowledgment, never the
 * private provider URL, key, full environment or a hard-budget assertion. */
export function validOctopCapability(value:unknown):value is OctopExecutionCapability{
 return record(value)&&fields(value,['harness_version','acp_sdk_version','provider_id','model','endpoint_origin','configuration_sha256','budget_mode','identity_evidence'])&&text(value.harness_version)&&text(value.acp_sdk_version)&&text(value.provider_id)&&text(value.model)&&typeof value.endpoint_origin==='string'&&endpoint(value.endpoint_origin,true)&&digest(value.configuration_sha256)&&value.budget_mode==='observe_only'&&value.identity_evidence==='configuration_ack_only';
}
function absolute(value:string){return text(value,2048)&&(/^[a-z]:[\\/]/i.test(value)||/^\/(?!\/)/.test(value)||/^\\\\[^\\]+\\[^\\]+\\/.test(value));}
function directory(value:string){const normalized=value.replaceAll('\\','/'),parent=normalized.slice(0,normalized.lastIndexOf('/'));return /^[a-z]:\//i.test(normalized)||normalized.startsWith('//')?parent.toLowerCase():parent;}
/** Builds a local administrator draft only. Go still checks actual files,
 * runtime content, native tools and current authority before any execution. */
export function octopHarnessConfiguration(input:OctopHarnessInputs):Record<string,unknown>|undefined{
 const checks=input.checks.split(',').map(value=>value.trim()).filter(Boolean),key=input.keyEnvironment.trim();
 if(!absolute(input.python)||/\.(?:cmd|bat|ps1|sh)$/i.test(input.python)||!absolute(input.bundle)||!/\.pyz$/i.test(input.bundle)||!absolute(input.manifest)||!/\.json$/i.test(input.manifest)||directory(input.bundle)!==directory(input.manifest)||checks.length<1||checks.length>16||new Set(checks).size!==checks.length||checks.some(value=>!/^[A-Za-z0-9._-]{1,100}$/.test(value))||!/^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$/.test(input.provider)||!text(input.model,200)||!endpoint(input.endpoint)||key&&(!/^[A-Z_][A-Z0-9_]{0,99}$/.test(key)||/^SUMA_/i.test(key))||!/^[1-9][0-9]*$/.test(input.maxSteps)||Number(input.maxSteps)>32||!/^[1-9][0-9]*$/.test(input.maxOutputTokens)||Number(input.maxOutputTokens)>32768)return;
 return {'octop-harness-develop':{...octopHarnessPreset['octop-harness-develop'],executable:input.python,args:[input.bundle],native_workspace:{checks,runtime_manifest:input.manifest},octop:{provider_id:input.provider,model:input.model,base_url:input.endpoint,...(key?{api_key_env_ref:key}:{}),max_steps:Number(input.maxSteps),max_output_tokens:Number(input.maxOutputTokens)}}};
}
function validProof(value:unknown):value is OctopExecutionEvidence{
 return record(value)&&fields(value,['configuration_sha256','session_id','requested_provider','requested_model','acknowledged_provider','acknowledged_model','stop_reason','completed_at','process_stopped','cancel_requested'])&&digest(value.configuration_sha256)&&text(value.session_id,256,true)&&text(value.requested_provider)&&text(value.requested_model)&&text(value.acknowledged_provider,256,true)&&text(value.acknowledged_model,256,true)&&text(value.stop_reason,100,true)&&text(value.completed_at,100,true)&&(!value.completed_at||Number.isFinite(Date.parse(value.completed_at)))&&typeof value.process_stopped==='boolean'&&typeof value.cancel_requested==='boolean';
}
export type OctopRunFacts={phase:'completed'|'failed'|'stopped'|'unknown'|'unrecorded'|'mismatch';capability?:OctopExecutionCapability;proof?:OctopExecutionEvidence};
/** Protocol stop reasons, actual process receipts and cancellation intent are
 * independent facts. A configured or acknowledged model is not vendor proof. */
export function octopRunFacts(state:string,evidence?:ToolEvidence):OctopRunFacts{
 const capability=validOctopCapability(evidence?.profile.octop_execution)?evidence.profile.octop_execution:undefined,proof=validProof(evidence?.octop_execution)?evidence.octop_execution:undefined;
 if(!evidence)return {phase:'unrecorded'};
 if(evidence.profile.adapter!=='octop'||evidence.profile.protocol!=='octop-harness-acp-v1'||!capability)return {phase:'mismatch'};
 if(!proof)return {phase:evidence.octop_execution?'mismatch':'unrecorded',capability};
 const facts={capability,proof};
 if(proof.configuration_sha256!==capability.configuration_sha256||proof.requested_provider!==capability.provider_id||proof.requested_model!==capability.model||proof.acknowledged_provider&&proof.acknowledged_provider!==capability.provider_id||proof.acknowledged_model&&proof.acknowledged_model!==capability.model)return {...facts,phase:'mismatch'};
 if(!proof.process_stopped||!proof.completed_at||!proof.session_id||!proof.acknowledged_provider||!proof.acknowledged_model||state==='reconciling'||evidence.terminal==='unknown')return {...facts,phase:'unknown'};
 if(state==='cancelled'&&proof.stop_reason==='cancelled')return {...facts,phase:'stopped'};
 if(state==='failed'||evidence.terminal!=='completed'||proof.stop_reason!=='end_turn')return {...facts,phase:'failed'};
 return {...facts,phase:state==='succeeded'?'completed':'unknown'};
}

const capabilityKeys=['harness_version','acp_sdk_version','provider_id','model','endpoint_origin','configuration_sha256','budget_mode','identity_evidence'] as const;
export function sameOctopCapability(a:unknown,b:unknown):boolean{
 return validOctopCapability(a)&&validOctopCapability(b)&&capabilityKeys.every(key=>a[key]===b[key]);
}
/** The public plan and its actual producer must freeze the same capability.
 * A mixed native/Codex/OpenCode plan cannot hide behind an Octop display card. */
export function validOctopPlan(value:unknown):boolean{
 if(!record(value))return false;
 const nodes=record(value.workflow)&&Array.isArray(value.workflow.nodes)?value.workflow.nodes:[],matches=nodes.filter(item=>record(item)&&item.id===value.agent_node_id),node=matches[0],config=record(node)&&record(node.config)?node.config:undefined;
 if(value.octop_execution===undefined&&!matches.some(item=>record(item)&&record(item.config)&&item.config.octop_execution!==undefined))return true;
 return matches.length===1&&record(node)&&node.type==='agent'&&!!config&&digest(value.tool_profile_sha256)&&config.tool_profile_id===value.tool_profile_id&&config.tool_profile_sha256===value.tool_profile_sha256&&(node.max_retries===undefined||node.max_retries===0)&&!value.model_execution&&!value.codex_execution&&!value.opencode_execution&&!config.model_execution&&!config.codex_execution&&!config.opencode_execution&&sameOctopCapability(value.octop_execution,config.octop_execution);
}
/** Scope and fixed producer checks precede clearing the original operation.
 * This preserves the received facts; it does not re-preview a new plan. */
export function validOctopDispatchReceipt(value:unknown,body:Record<string,unknown>):boolean{
 if(!validOctopPlan(value)||!record(value))return false;
 if(value.octop_execution===undefined)return true;
 return typeof value.id==='string'&&value.id.length>0&&value.project_id===body.project_id&&value.task_id===body.task_id&&value.task_version===body.expected_task_version&&value.workflow_id===body.workflow_id&&value.workflow_version===body.workflow_version&&value.agent_node_id===body.agent_node_id&&value.tool_profile_id===body.tool_profile_id&&value.tool_profile_version===body.expected_tool_version&&!['model_execution_selection','codex_execution_selection','octop_execution','octop_execution_selection','octop'].some(key=>Object.hasOwn(body,key));
}
