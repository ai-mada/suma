import type {ToolExecutionProfile,ToolEvidence} from '../../../packages/contracts/src';
export type OctopExecutionCapability=NonNullable<ToolExecutionProfile['octop_execution']>;
export type OctopExecutionEvidence=NonNullable<ToolEvidence['octop_execution']>;
export type OctopHarnessInputs={python:string;bundle:string;manifest:string;checks:string};
export const octopHarnessPreset={'octop-harness-develop':{adapter:'octop',protocol:'octop-harness-acp-v1',executable:'',args:[] as string[],input:'octop-acp-jsonl',read_only:false,model_execution_protocol:'suma-model-execution-v1'}} as const;
const record=(value:unknown):value is Record<string,unknown>=>value!==null&&typeof value==='object'&&!Array.isArray(value);
const text=(value:unknown,max=256,empty=false):value is string=>typeof value==='string'&&(empty||value.length>0)&&value.trim()===value&&new TextEncoder().encode(value).length<=max&&!/[\u0000-\u001f\u007f-\u009f]/.test(value);
const digest=(value:unknown):value is string=>typeof value==='string'&&/^[a-f0-9]{64}$/.test(value);
const fields=(value:Record<string,unknown>,keys:readonly string[])=>Object.keys(value).every(key=>keys.includes(key));
/** Runtime protocol declarations contain no model settings or credentials.
 * The shared platform model binding is selected separately for every task. */
export function validOctopCapability(value:unknown):value is OctopExecutionCapability{
 return record(value)&&fields(value,['harness_version','acp_sdk_version'])&&value.harness_version==='1.0.5'&&value.acp_sdk_version==='0.12.1';
}
function absolute(value:string){return text(value,2048)&&(/^[a-z]:[\\/]/i.test(value)||/^\/(?!\/)/.test(value)||/^\\\\[^\\]+\\[^\\]+\\/.test(value));}
function directory(value:string){const normalized=value.replaceAll('\\','/'),parent=normalized.slice(0,normalized.lastIndexOf('/'));return /^[a-z]:\//i.test(normalized)||normalized.startsWith('//')?parent.toLowerCase():parent;}
/** Local administrator draft only. Model deployments, credentials, parameters
 * and budgets use the same Runner allowlist and task selection as native Suma. */
export function octopHarnessConfiguration(input:OctopHarnessInputs):Record<string,unknown>|undefined{
 const checks=input.checks.split(',').map(value=>value.trim()).filter(Boolean);
 if(!absolute(input.python)||/\.(?:cmd|bat|ps1|sh)$/i.test(input.python)||!absolute(input.bundle)||!/\.pyz$/i.test(input.bundle)||!absolute(input.manifest)||!/\.json$/i.test(input.manifest)||directory(input.bundle)!==directory(input.manifest)||checks.length<1||checks.length>16||new Set(checks).size!==checks.length||checks.some(value=>!/^[A-Za-z0-9._-]{1,100}$/.test(value)))return;
 return {'octop-harness-develop':{...octopHarnessPreset['octop-harness-develop'],executable:input.python,args:[input.bundle],native_workspace:{checks,runtime_manifest:input.manifest}}};
}
function validProof(value:unknown):value is OctopExecutionEvidence{
 return record(value)&&fields(value,['binding_sha256','session_id','stop_reason','completed_at','process_stopped','cancel_requested'])&&digest(value.binding_sha256)&&text(value.session_id,256,true)&&text(value.stop_reason,100,true)&&text(value.completed_at,100,true)&&(!value.completed_at||Number.isFinite(Date.parse(value.completed_at)))&&typeof value.process_stopped==='boolean'&&typeof value.cancel_requested==='boolean';
}
export type OctopRunFacts={phase:'completed'|'failed'|'stopped'|'unknown'|'unrecorded'|'mismatch';capability?:OctopExecutionCapability;proof?:OctopExecutionEvidence};
/** ACP completion correlates with the Go-owned model gateway binding.
 * Cancellation intent never substitutes for process or gateway closure. */
export function octopRunFacts(state:string,evidence?:ToolEvidence):OctopRunFacts{
 const capability=validOctopCapability(evidence?.profile.octop_execution)?evidence.profile.octop_execution:undefined,proof=validProof(evidence?.octop_execution)?evidence.octop_execution:undefined;
 if(!evidence)return {phase:'unrecorded'};
 if(evidence.profile.adapter!=='octop'||evidence.profile.protocol!=='octop-harness-acp-v1'||evidence.profile.model_execution_protocol!=='suma-model-execution-v1'||!capability)return {phase:'mismatch'};
 if(!proof)return {phase:evidence.octop_execution?'mismatch':'unrecorded',capability};
 const facts={capability,proof},gateway=evidence.model_execution;
 if(!gateway||gateway.gateway_protocol!=='suma-model-gateway-v1'||gateway.binding_sha256!==proof.binding_sha256)return {...facts,phase:'mismatch'};
 if(!gateway.gateway_closed||!proof.process_stopped||!proof.completed_at||!proof.session_id||state==='reconciling'||evidence.terminal==='unknown')return {...facts,phase:'unknown'};
 if(state==='cancelled')return {...facts,phase:'stopped'};
 if(state==='failed'||evidence.terminal!=='completed'||proof.stop_reason!=='end_turn')return {...facts,phase:'failed'};
 return {...facts,phase:state==='succeeded'?'completed':'unknown'};
}
