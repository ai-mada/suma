import {tr,UiError} from './i18n';
import type {RecordItem} from './api';
import type {ApplicationReleaseSetRef,IssueRepairReleaseHandoff} from '../../../packages/contracts/src';

export type ReleaseBlocker={code:string;message:string};
export type AssignmentRef={run_id:string;node_id:string;attempt:number;fence:number;worker_id:string};
export type ArtifactFile={path:string;size_bytes:number;sha256:string};
export type ArtifactManifest={schema_version:number;kind:string;files:ArtifactFile[]};
export type ArtifactProof={id:string;project_id:string;assignment:AssignmentRef;kind:string;artifact_id:string;manifest_sha256:string;source_commit:string;profile_id:string;profile_sha256:string;workspace_sha256:string;created_at:string};
export type ReleaseArtifact={id:string;project_id:string;manifest_sha256:string;manifest:ArtifactManifest;size_bytes:number;state:string;created_at:string;proofs:ArtifactProof[]};
export type ReleaseTaskBasis={id:string;version:number;run_id:string;review_id:string;sha256:string};
export type ReleaseCandidate={id:string;project_id:string;name:string;artifact_id:string;artifact_sha256:string;source_commit:string;build_attestation_id:string;package_test_attestation_id:string;accepted_task_basis_sha256:string;build_proof:ArtifactProof;package_test_proof:ArtifactProof;tasks:ReleaseTaskBasis[];created_at:string;repair_handoff?:IssueRepairReleaseHandoff};
export type CandidateDetail={candidate:ReleaseCandidate;eligibility:boolean;blockers:ReleaseBlocker[]};
export type HealthCheck={path:string;expected_status:number;expected_file:string;expected_sha256?:string};
export type HealthPolicy={checks:HealthCheck[];timeout_ms:number;attempts:number};
export type ReleaseProvider={id:string;site_ids:string[];kind:string;capability_state:string};
export type ReleaseEnvironment={id:string;project_id:string;version:number;name:string;status:string;provider_id:string;site_id:string;policy_version:number;health:HealthPolicy;generation:number;active_generation:number;active_artifact_id:string;active_artifact_sha256:string;barrier_operation_id:string;capability_state:string;created_at:string;updated_at:string;allowed_actions?:string[]};
export type ReleasePlanContent={application_release_set?:ApplicationReleaseSetRef|null;candidate_id:string;action:string;environment_id:string;provider_id:string;site_id:string;environment_policy_version:number;expected_generation:number;generation:number;artifact_id:string;artifact_sha256:string;source_commit:string;build_attestation_id:string;package_test_attestation_id:string;accepted_task_basis_sha256:string;reserved_execution:{run_id:string;node_id:string};health:HealthPolicy;rollback_source_operation_id?:string};
export type ReleaseApproval={actor_id:string;decision:string;reason:string;at:string;plan_sha256:string};
export type ReleaseRepairSource={issue_id:string;cycle:number;candidate_id:string};
export const releaseRunnerObservationStates=['not_registered','unobserved','stale','capability_missing','capability_observed'] as const;
export type ReleaseRunnerReadiness={state:(typeof releaseRunnerObservationStates)[number];registered_count:number;checked_at:string;observed_at?:string};
export type ReleasePlan={id:string;project_id:string;version:number;state:string;revision:number;sha256:string;content:ReleasePlanContent;created_by:string;created_at:string;approval:ReleaseApproval|null;execution_operation_id:string;eligibility:boolean;blockers:ReleaseBlocker[];allowed_actions:string[];repair_source?:ReleaseRepairSource|null;execution_lane?:string;execution_readiness?:ReleaseRunnerReadiness|null};
export type PlanInput={project_id:string;candidate_id:string;environment_id:string;action:'deploy'|'rollback';expected_environment_generation:number;rollback_source_operation_id?:string};
export type PlanPreview={preview_sha256:string;environment:ReleaseEnvironment;candidate_id:string;action:string;health:HealthPolicy;eligibility:boolean;blockers:ReleaseBlocker[];allowed_actions:string[];execution_lane?:string;execution_readiness?:ReleaseRunnerReadiness|null};
export type HealthCheckResult={path:string;status_code:number;body_sha256:string;observed_at:string};
export type DeploymentObservation={protocol:string;operation_id:string;request_sha256:string;provider_id:string;plan_id:string;plan_revision:number;plan_sha256:string;environment_id:string;generation:number;fence_generation:number;state:string;effect:{state:string;previous_active_generation:number;active_generation:number;active_artifact_sha256:string;observed_at:string};health:{state:string;generation:number;artifact_sha256:string;checks:HealthCheckResult[]};evidence_sha256:string;error:{code:string;message:string}|null;version:number};
export type ReleaseDeployment={operation_id:string;project_id:string;version:number;plan_id:string;plan_revision:number;plan_sha256:string;environment_id:string;provider_id:string;generation:number;artifact_id:string;artifact_sha256:string;run_id:string;node_id:string;state:string;effect_state:string;health_state:string;assignment?:AssignmentRef;request_sha256:string;observation:DeploymentObservation|null;observations?:DeploymentObservation[];issue_id:string;created_at:string;updated_at:string;allowed_actions:string[];blockers:ReleaseBlocker[];repair_source?:ReleaseRepairSource|null};
export type ReleaseListKind='artifacts'|'release-candidates'|'environments'|'release-plans'|'deployments';
export type ReleaseListItem=ReleaseArtifact|CandidateDetail|ReleaseEnvironment|ReleasePlan|ReleaseDeployment;
export type ReleasePage<T>={items:T[];total:number;next_cursor?:string;allowed_actions?:string[]};
export const releaseTabs:ReleaseListKind[]=['artifacts','release-candidates','environments','release-plans','deployments'];
export const releaseNames:Record<ReleaseListKind,string>={get artifacts(){return tr('release.names.artifacts');},get 'release-candidates'(){return tr('release.names.candidates');},get environments(){return tr('release.names.environments');},get 'release-plans'(){return tr('release.names.plans');},get deployments(){return tr('release.names.deployments');}};
export const defaultHealth:HealthPolicy={checks:[{path:'/',expected_status:200,expected_file:'index.html'}],timeout_ms:2000,attempts:3};

export function candidateRequest(project:string,name:string,artifact:ReleaseArtifact,build:ArtifactProof,test:ArtifactProof,tasks:Pick<RecordItem,'id'|'version'|'status'>[]){
 if(artifact.project_id!==project||artifact.state!=='sealed')throw new UiError('release.validation.artifact');
 if(build.kind!=='build'||test.kind!=='package_test'||[build,test].some(proof=>proof.project_id!==project||proof.artifact_id!==artifact.id||proof.manifest_sha256!==artifact.manifest_sha256)||build.source_commit!==test.source_commit)throw new UiError('release.validation.proofs');
 if(!name.trim()||!tasks.length||tasks.some(task=>task.status!=='accepted'||!Number.isSafeInteger(task.version)||task.version<1)||new Set(tasks.map(task=>task.id)).size!==tasks.length)throw new UiError('release.validation.tasks');
 return {project_id:project,name:name.trim(),artifact_id:artifact.id,build_attestation_id:build.id,package_test_attestation_id:test.id,tasks:tasks.map(task=>({id:task.id,expected_version:task.version}))};
}
export function planInput(project:string,candidate:string,environment:ReleaseEnvironment,rollbackSource?:string):PlanInput{
 if(!candidate||environment.project_id!==project||!Number.isSafeInteger(environment.generation)||environment.generation<0)throw new UiError('release.validation.plan');
 return {project_id:project,candidate_id:candidate,environment_id:environment.id,action:rollbackSource?'rollback':'deploy',expected_environment_generation:environment.generation,...(rollbackSource?{rollback_source_operation_id:rollbackSource}:{})};
}
function planInputKey(input:PlanInput){return JSON.stringify([input.project_id,input.candidate_id,input.environment_id,input.action,input.expected_environment_generation,input.rollback_source_operation_id||'']);}
export function planCreateRequest(input:PlanInput,previewInput:PlanInput,preview:PlanPreview){
 if(planInputKey(input)!==planInputKey(previewInput)||preview.candidate_id!==input.candidate_id||preview.environment.id!==input.environment_id||preview.environment.generation!==input.expected_environment_generation||preview.action!==input.action)throw new UiError('release.validation.preview_stale');
 if(!preview.eligibility||!preview.preview_sha256)throw new UiError('release.validation.blocked');
 if(!hasReleaseAction(preview,'create'))throw new UiError('release.validation.create');
 return {...input,preview_sha256:preview.preview_sha256};
}
export function planDecisionRequest(plan:ReleasePlan,decision:string,reason:string){return {...planVersionRequest(plan),decision,reason};}
export function planVersionRequest(plan:ReleasePlan){return {project_id:plan.project_id,expected_version:plan.version,plan_revision:plan.revision,plan_sha256:plan.sha256};}
export function hasReleaseAction(object:{allowed_actions?:string[];eligibility?:boolean},action:string){return Array.isArray(object.allowed_actions)&&object.allowed_actions.includes(action)&&(!['execute','approve','create'].includes(action)||object.eligibility!==false);}
const operationNames:Record<string,string>={get queued(){return tr('release.operation.queued');},get received(){return tr('release.operation.received');},get authorized(){return tr('release.operation.authorized');},get staging(){return tr('release.operation.staging');},get applying(){return tr('release.operation.applying');},get checking_health(){return tr('release.operation.checking_health');},get succeeded(){return tr('release.operation.succeeded');},get deployed_healthy(){return tr('release.operation.deployed_healthy');},get deployed_unhealthy(){return tr('release.operation.deployed_unhealthy');},get failed(){return tr('release.operation.failed');},get cancelled(){return tr('release.operation.cancelled');},get reconciling(){return tr('release.operation.reconciling');}};
const effectNames:Record<string,string>={get not_started(){return tr('release.effect.not_started');},get applied(){return tr('release.effect.applied');},get unknown(){return tr('release.effect.unknown');}};
const healthNames:Record<string,string>={get not_checked(){return tr('release.health.not_checked');},get passed(){return tr('release.health.passed');},get failed(){return tr('release.health.failed');},get unknown(){return tr('release.health.unknown');}};
export const planStateNames:Record<string,string>={get pending_approval(){return tr('release.state.pending_approval');},get approved(){return tr('release.state.approved');},get rejected(){return tr('release.state.rejected');},get revoked(){return tr('release.state.revoked');},get executing(){return tr('release.state.executing');},get completed(){return tr('release.state.completed');}};
export function deploymentFacts(deployment:Pick<ReleaseDeployment,'state'|'effect_state'|'health_state'>){return {state:(Object.hasOwn(operationNames,deployment.state)?operationNames[deployment.state]:undefined)||tr('release.operation.unknown',{state:deployment.state}),effect:(Object.hasOwn(effectNames,deployment.effect_state)?effectNames[deployment.effect_state]:undefined)||tr('release.effect.unrecognized',{state:deployment.effect_state}),health:(Object.hasOwn(healthNames,deployment.health_state)?healthNames[deployment.health_state]:undefined)||tr('release.health.unrecognized',{state:deployment.health_state})};}
export function releaseItemID(item:ReleaseListItem){return 'candidate'in item?item.candidate.id:'operation_id'in item?item.operation_id:item.id;}
export function releaseItemName(item:ReleaseListItem){return 'candidate'in item?item.candidate.name:'name'in item?item.name:'operation_id'in item?item.operation_id:'id'in item?item.id:'';}
export function releaseListPath(project:string,kind:ReleaseListKind,cursor=''){const params=new URLSearchParams({project_id:project});if(kind==='artifacts'||kind==='release-candidates'){params.set('limit','100');if(cursor)params.set('cursor',cursor);}return '/'+kind+'?'+params;}
export type ReleaseReferenceKind='environments'|'release-candidates';
export function releaseReferencePath(project:string,kind:ReleaseReferenceKind,id:string){return '/'+kind+'/'+encodeURIComponent(id)+'?'+new URLSearchParams({project_id:project});}
export function releaseReferenceName(project:string,kind:ReleaseReferenceKind,id:string,value:ReleaseEnvironment|CandidateDetail|undefined){const record=kind==='release-candidates'&&value&&'candidate'in value?value.candidate:value&&'id'in value?value:undefined;return record?.id===id&&record.project_id===project&&typeof record.name==='string'&&record.name.trim()?record.name:undefined;}
