import type {OpenCodeExecutionCapability,OpenCodeExecutionEvidence,ToolEvent,ToolEvidence} from '../../../packages/contracts/src';
import {sameOpenCodeCapability,validOpenCodeCapability} from './opencode-execution-model';

export type OpenCodeRunTest={id:string;name:string;state:string;onOpen:()=>void};
export type OpenCodeRunPhase='pending'|'running'|'completed'|'failed'|'stopped'|'unknown'|'unrecorded';
export type OpenCodeRunFacts={phase:OpenCodeRunPhase;notice?:'run_progress'|'run_missing'|'run_mismatch'|'run_failed_unreported'|'run_platform_failed'|'run_reconciling';capability?:OpenCodeExecutionCapability;proof?:OpenCodeExecutionEvidence;session?:string;terminal?:string;positiveStop:boolean;identity:string};
const protocol='opencode-sdk-v1';
const object=(v:unknown):v is Record<string,unknown>=>!!v&&typeof v==='object'&&!Array.isArray(v);
const hash=(v:unknown):v is string=>typeof v==='string'&&/^[a-f0-9]{64}$/.test(v);
const command=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$/.test(v);
const nodeID=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$/.test(v);
const positive=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>0;
const counter=(v:unknown):v is number=>typeof v==='number'&&Number.isSafeInteger(v)&&v>=0;
const id=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9_.:/@+\-]{0,127}$/.test(v);
const model=(v:unknown):v is string=>typeof v==='string'&&/^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$/.test(v);
const optional=(v:unknown,valid:(v:unknown)=>boolean)=>v===''||valid(v);
const utcTime=(v:unknown):v is string=>typeof v==='string'&&/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(v)&&Number.isFinite(Date.parse(v))&&new Date(v).toISOString().slice(0,19)===v.slice(0,19);
const proofKeys=['configuration_sha256','session_id','user_message_id','assistant_message_id','requested_provider','requested_model','acknowledged_provider','acknowledged_model','finish','completed_at','server_stopped','abort_acknowledged'];

/** A bounded original wire view may be disclosed even when its correlation
 * fails. It cannot create a positive completion/stop or provider-health fact. */
function boundedProof(value:unknown):value is OpenCodeExecutionEvidence {
 if(!object(value)||Object.keys(value).length!==proofKeys.length||Object.keys(value).some(key=>!proofKeys.includes(key)))return false;
 return hash(value.configuration_sha256)&&optional(value.session_id,id)&&optional(value.user_message_id,id)&&optional(value.assistant_message_id,id)&&command(value.requested_provider)&&model(value.requested_model)&&optional(value.acknowledged_provider,command)&&optional(value.acknowledged_model,model)&&typeof value.finish==='string'&&value.finish.length<=64&&!/[\u0000-\u001f\u007f-\u009f]/.test(value.finish)&&optional(value.completed_at,utcTime)&&typeof value.server_stopped==='boolean'&&typeof value.abort_acknowledged==='boolean';
}

function correlated(proof:OpenCodeExecutionEvidence,capability:OpenCodeExecutionCapability,session:unknown):boolean {
 return proof.configuration_sha256===capability.configuration_sha256&&proof.requested_provider===capability.provider_id&&proof.requested_model===capability.model&&session===proof.session_id&&(!proof.user_message_id||!!proof.session_id)&&(!proof.assistant_message_id||!!proof.user_message_id&&!!proof.session_id&&proof.assistant_message_id!==proof.user_message_id)&&(!proof.abort_acknowledged||!!proof.session_id);
}

/** Frozen definition only: current inventory cannot substitute a model for a
 * running or historical attempt. Callers also exclude dirty/unavailable graphs. */
export function frozenOpenCodeForNode(definition:unknown,run:unknown,node:unknown):OpenCodeExecutionCapability|undefined {
 if(!object(definition)||!object(run)||!object(node)||!command(run.id)||!nodeID(definition.id)||!nodeID(run.workflow_id)||!nodeID(node.id)||!positive(definition.version)||!positive(run.workflow_version)||definition.id!==run.workflow_id||definition.version!==run.workflow_version||!Array.isArray(definition.nodes)||!Array.isArray(run.nodes))return;
 const actual=run.nodes.find(value=>object(value)&&value.id===node.id),declared=definition.nodes.find(value=>object(value)&&value.id===node.id);
 if(!object(actual)||!object(declared)||!object(declared.config)||declared.type!=='agent'||node.type!=='agent'||actual.type!=='agent'||!Array.isArray(declared.command)||declared.command.length!==1||!command(declared.command[0]))return;
 const config=declared.config;
 for(const value of [node,actual])if(!Array.isArray(value.command)||value.command.length!==1||value.command[0]!==declared.command[0]||!command(value.tool_profile_id)||!hash(value.tool_profile_sha256)||config.tool_profile_id!==value.tool_profile_id||config.tool_profile_sha256!==value.tool_profile_sha256)return;
 if(!counter(node.attempt)||!counter(node.fence)||!counter(actual.attempt)||!counter(actual.fence)||node.attempt!==actual.attempt||node.fence!==actual.fence||declared.max_retries!==0||Object.keys(config).some(key=>['model_execution','codex_execution','model','model_id','effort','reasoning_effort','provider','model_provider','base_url','endpoint','api_key','env','env_from','cwd','argv','max_steps','max_output_tokens','max_tokens','max_cost','max_cost_micros','service_tier','config','configuration','sandbox','approval_policy','windows','windows_sandbox','hard_budget','token_budget','budget','resume','session_id','tools','mcp_servers'].includes(key))||!validOpenCodeCapability(config.opencode_execution))return;
 return config.opencode_execution;
}

/** Platform state, tool terminal proof and progress are deliberately separate.
 * A progress turn_completed is never the final whole-turn result. */
export function openCodeRunFacts({state,evidence,events=[],fixedOpenCode}:{state:string;evidence?:ToolEvidence;events?:ToolEvent[];fixedOpenCode?:OpenCodeExecutionCapability|null}):OpenCodeRunFacts|undefined {
 const sdkEvents=events.filter(event=>event.protocol===protocol),profile=evidence?.profile;
 const configured=validOpenCodeCapability(fixedOpenCode)?fixedOpenCode:undefined;
 const reported=validOpenCodeCapability(profile?.opencode_execution)?profile.opencode_execution:undefined;
 if(!configured&&!reported&&profile?.protocol!==protocol&&!evidence?.opencode_execution&&!sdkEvents.length)return;
 const capability=configured??reported,proof=boundedProof(evidence?.opencode_execution)?evidence.opencode_execution:undefined;
 const sessions=[...new Set(sdkEvents.map(event=>event.session_id).filter((value):value is string=>!!value&&id(value)))];
 const session=proof?.session_id||sessions.length===1&&sessions[0]||undefined;
 const identity=JSON.stringify([capability?.configuration_sha256,profile?.profile_sha256,sdkEvents[0]?.profile_sha256,session,proof?.user_message_id,proof?.assistant_message_id]);
 const result:OpenCodeRunFacts={phase:'unknown',capability,proof,session,terminal:evidence?.terminal,positiveStop:false,identity};
 const first=sdkEvents[0];
 const eventsMatch=sessions.length<=1&&sdkEvents.every(event=>command(event.command_id)&&hash(event.profile_sha256)&&event.command_id===(profile?.command_id??first?.command_id)&&event.profile_sha256===(profile?.profile_sha256??first?.profile_sha256)&&(!event.session_id||id(event.session_id))&&(!proof?.session_id||!event.session_id||event.session_id===proof.session_id))&&(!sdkEvents.length||events.every(event=>event.protocol===protocol));
 const badFixed=fixedOpenCode!=null&&!configured;
 const sdkProfile=profile?.adapter==='opencode'&&profile.protocol===protocol&&!profile.read_only&&profile.workspace_tools_version==='attempt-workspace-v1'&&!profile.model_execution_protocol&&!profile.conflict_repair_protocol&&!profile.planning&&!profile.knowledge_extraction&&!profile.failure_diagnosis;
 const valid=!!evidence&&sdkProfile&&!!reported&&!!capability&&!!proof&&sameOpenCodeCapability(capability,reported)&&!evidence.model_execution&&!evidence.codex_execution&&evidence.usage_state!=='complete'&&correlated(proof,capability,evidence.session_id??'');
 if(badFixed||!eventsMatch||evidence&&!valid){result.notice='run_mismatch';return result;}
 if(!evidence){
  if(state==='running'){result.phase='running';result.notice='run_progress';}
  else if(['ready','pending'].includes(state)&&!sdkEvents.length){result.phase='pending';}
  else if(state==='failed'){result.phase='failed';result.notice='run_failed_unreported';}
  else if(['skipped','cancelled'].includes(state)&&!sdkEvents.length){result.phase='unrecorded';}
  else result.notice='run_missing';
  return result;
 }
 if(!proof)return result;
 const completed=!!proof.session_id&&!!proof.user_message_id&&!!proof.assistant_message_id&&proof.acknowledged_provider===proof.requested_provider&&proof.acknowledged_model===proof.requested_model&&proof.finish==='stop'&&!!proof.completed_at&&proof.server_stopped&&!proof.abort_acknowledged&&Number.isSafeInteger(evidence.event_count)&&evidence.event_count>0;
 const stopped=proof.server_stopped&&(!proof.session_id||proof.abort_acknowledged);
 if(!['completed','failed','unknown'].includes(evidence.terminal)||evidence.terminal==='completed'&&!completed){result.notice='run_mismatch';return result;}
 result.positiveStop=proof.server_stopped;
 if(state==='reconciling'){result.notice=evidence.terminal==='completed'&&completed?'run_reconciling':'run_missing';return result;}
 if(state==='cancelled'&&stopped){result.phase='stopped';result.positiveStop=true;return result;}
 if(evidence.terminal==='completed'&&completed&&['succeeded','failed'].includes(state)){result.phase='completed';result.positiveStop=true;if(state==='failed')result.notice='run_platform_failed';return result;}
 if(evidence.terminal==='failed'&&state==='failed'&&proof.server_stopped){result.phase='failed';result.positiveStop=true;return result;}
 result.notice=evidence.terminal==='unknown'?'run_missing':'run_mismatch';
 return result;
}
