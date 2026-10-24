import type {RecordItem} from './api';

export type ReleaseBlocker={code:string;message:string};
export type AssignmentRef={run_id:string;node_id:string;attempt:number;fence:number;worker_id:string};
export type ArtifactFile={path:string;size_bytes:number;sha256:string};
export type ArtifactManifest={schema_version:number;kind:string;files:ArtifactFile[]};
export type ArtifactProof={id:string;project_id:string;assignment:AssignmentRef;kind:string;artifact_id:string;manifest_sha256:string;source_commit:string;profile_id:string;profile_sha256:string;workspace_sha256:string;created_at:string};
export type ReleaseArtifact={id:string;project_id:string;manifest_sha256:string;manifest:ArtifactManifest;size_bytes:number;state:string;created_at:string;proofs:ArtifactProof[]};
export type ReleaseTaskBasis={id:string;version:number;run_id:string;review_id:string;sha256:string};
export type ReleaseCandidate={id:string;project_id:string;name:string;artifact_id:string;artifact_sha256:string;source_commit:string;build_attestation_id:string;package_test_attestation_id:string;accepted_task_basis_sha256:string;build_proof:ArtifactProof;package_test_proof:ArtifactProof;tasks:ReleaseTaskBasis[];created_at:string};
export type CandidateDetail={candidate:ReleaseCandidate;eligibility:boolean;blockers:ReleaseBlocker[]};
export type HealthCheck={path:string;expected_status:number;expected_file:string;expected_sha256?:string};
export type HealthPolicy={checks:HealthCheck[];timeout_ms:number;attempts:number};
export type ReleaseProvider={id:string;site_ids:string[];kind:string;capability_state:string};
export type ReleaseEnvironment={id:string;project_id:string;version:number;name:string;status:string;provider_id:string;site_id:string;policy_version:number;health:HealthPolicy;generation:number;active_generation:number;active_artifact_id:string;active_artifact_sha256:string;barrier_operation_id:string;capability_state:string;created_at:string;updated_at:string;allowed_actions?:string[]};
export type ReleasePlanContent={candidate_id:string;action:string;environment_id:string;provider_id:string;site_id:string;environment_policy_version:number;expected_generation:number;generation:number;artifact_id:string;artifact_sha256:string;source_commit:string;build_attestation_id:string;package_test_attestation_id:string;accepted_task_basis_sha256:string;reserved_execution:{run_id:string;node_id:string};health:HealthPolicy;rollback_source_operation_id?:string};
export type ReleaseApproval={actor_id:string;decision:string;reason:string;at:string;plan_sha256:string};
export type ReleasePlan={id:string;project_id:string;version:number;state:string;revision:number;sha256:string;content:ReleasePlanContent;created_by:string;created_at:string;approval:ReleaseApproval|null;execution_operation_id:string;eligibility:boolean;blockers:ReleaseBlocker[];allowed_actions:string[]};
export type PlanInput={project_id:string;candidate_id:string;environment_id:string;action:'deploy'|'rollback';expected_environment_generation:number;rollback_source_operation_id?:string};
export type PlanPreview={preview_sha256:string;environment:ReleaseEnvironment;candidate_id:string;action:string;health:HealthPolicy;eligibility:boolean;blockers:ReleaseBlocker[];allowed_actions:string[]};
export type HealthCheckResult={path:string;status_code:number;body_sha256:string;observed_at:string};
export type DeploymentObservation={protocol:string;operation_id:string;request_sha256:string;provider_id:string;plan_id:string;plan_revision:number;plan_sha256:string;environment_id:string;generation:number;fence_generation:number;state:string;effect:{state:string;previous_active_generation:number;active_generation:number;active_artifact_sha256:string;observed_at:string};health:{state:string;generation:number;artifact_sha256:string;checks:HealthCheckResult[]};evidence_sha256:string;error:{code:string;message:string}|null;version:number};
export type ReleaseDeployment={operation_id:string;project_id:string;version:number;plan_id:string;plan_revision:number;plan_sha256:string;environment_id:string;provider_id:string;generation:number;artifact_id:string;artifact_sha256:string;run_id:string;node_id:string;state:string;effect_state:string;health_state:string;assignment?:AssignmentRef;request_sha256:string;observation:DeploymentObservation|null;observations?:DeploymentObservation[];issue_id:string;created_at:string;updated_at:string;allowed_actions:string[];blockers:ReleaseBlocker[]};
export type ReleaseListKind='artifacts'|'release-candidates'|'environments'|'release-plans'|'deployments';
export type ReleaseListItem=ReleaseArtifact|CandidateDetail|ReleaseEnvironment|ReleasePlan|ReleaseDeployment;
export type ReleasePage<T>={items:T[];total:number;next_cursor?:string;allowed_actions?:string[]};
export const releaseTabs:ReleaseListKind[]=['artifacts','release-candidates','environments','release-plans','deployments'];
export const releaseNames:Record<ReleaseListKind,string>={artifacts:'制品文件','release-candidates':'发布候选',environments:'发布环境','release-plans':'部署计划',deployments:'部署记录'};
export const defaultHealth:HealthPolicy={checks:[{path:'/',expected_status:200,expected_file:'index.html'}],timeout_ms:2000,attempts:3};

export function candidateRequest(project:string,name:string,artifact:ReleaseArtifact,build:ArtifactProof,test:ArtifactProof,tasks:Pick<RecordItem,'id'|'version'|'status'>[]){
 if(artifact.project_id!==project||artifact.state!=='sealed')throw new Error('请先选择当前项目已封存的真实制品。');
 if(build.kind!=='build'||test.kind!=='package_test'||[build,test].some(proof=>proof.project_id!==project||proof.artifact_id!==artifact.id||proof.manifest_sha256!==artifact.manifest_sha256)||build.source_commit!==test.source_commit)throw new Error('构建与包测试证明必须绑定同一制品指纹和源提交。');
 if(!name.trim()||!tasks.length||tasks.some(task=>task.status!=='accepted'||!Number.isSafeInteger(task.version)||task.version<1)||new Set(tasks.map(task=>task.id)).size!==tasks.length)throw new Error('请填写候选名称并选择已正式验收的真实任务。');
 return {project_id:project,name:name.trim(),artifact_id:artifact.id,build_attestation_id:build.id,package_test_attestation_id:test.id,tasks:tasks.map(task=>({id:task.id,expected_version:task.version}))};
}
export function planInput(project:string,candidate:string,environment:ReleaseEnvironment,rollbackSource?:string):PlanInput{
 if(!candidate||environment.project_id!==project||!Number.isSafeInteger(environment.generation)||environment.generation<0)throw new Error('请读取当前项目的候选和环境再准备计划。');
 return {project_id:project,candidate_id:candidate,environment_id:environment.id,action:rollbackSource?'rollback':'deploy',expected_environment_generation:environment.generation,...(rollbackSource?{rollback_source_operation_id:rollbackSource}:{})};
}
function planInputKey(input:PlanInput){return JSON.stringify([input.project_id,input.candidate_id,input.environment_id,input.action,input.expected_environment_generation,input.rollback_source_operation_id||'']);}
export function planCreateRequest(input:PlanInput,previewInput:PlanInput,preview:PlanPreview){
 if(planInputKey(input)!==planInputKey(previewInput)||preview.candidate_id!==input.candidate_id||preview.environment.id!==input.environment_id||preview.environment.generation!==input.expected_environment_generation||preview.action!==input.action)throw new Error('准备内容或环境已变化，请重新预览计划。');
 if(!preview.eligibility||!preview.preview_sha256)throw new Error('部署条件尚未满足，请先查看阻断原因。');
 if(!hasReleaseAction(preview,'create'))throw new Error('当前预览未授予创建计划权限，请刷新权限与预览。');
 return {...input,preview_sha256:preview.preview_sha256};
}
export function planDecisionRequest(plan:ReleasePlan,decision:string,reason:string){return {...planVersionRequest(plan),decision,reason};}
export function planVersionRequest(plan:ReleasePlan){return {project_id:plan.project_id,expected_version:plan.version,plan_revision:plan.revision,plan_sha256:plan.sha256};}
export function hasReleaseAction(object:{allowed_actions?:string[];eligibility?:boolean},action:string){return Array.isArray(object.allowed_actions)&&object.allowed_actions.includes(action)&&(!['execute','approve','create'].includes(action)||object.eligibility!==false);}
const operationNames:Record<string,string>={queued:'已排队',received:'已受理',authorized:'授权已确认',staging:'准备内容',applying:'正在切换',checking_health:'正在检查健康',succeeded:'发布与本代健康已确认',deployed_healthy:'已部署且本代健康通过',deployed_unhealthy:'已切换，健康异常',failed:'本次发布失败',cancelled:'已确认取消',reconciling:'结果待核对'};
const effectNames:Record<string,string>={not_started:'尚未开始切换',applied:'内容切换已确认',unknown:'切换结果待核对'};
const healthNames:Record<string,string>={not_checked:'尚未检查',passed:'本代健康检查通过',failed:'健康检查失败',unknown:'健康结果未确认'};
export const planStateNames:Record<string,string>={pending_approval:'待审阅',approved:'已批准',rejected:'已退回',revoked:'批准已撤销',executing:'执行中',completed:'执行已结束'};
export function deploymentFacts(deployment:Pick<ReleaseDeployment,'state'|'effect_state'|'health_state'>){return {state:operationNames[deployment.state]||'未识别状态：'+deployment.state,effect:effectNames[deployment.effect_state]||'切换结果待核对',health:healthNames[deployment.health_state]||'健康结果未确认'};}
export function releaseItemID(item:ReleaseListItem){return 'candidate'in item?item.candidate.id:'operation_id'in item?item.operation_id:item.id;}
export function releaseItemName(item:ReleaseListItem){return 'candidate'in item?item.candidate.name:'name'in item?item.name:'operation_id'in item?item.operation_id:'id'in item?item.id:'';}
export function releaseListPath(project:string,kind:ReleaseListKind,cursor=''){const params=new URLSearchParams({project_id:project});if(kind==='artifacts'||kind==='release-candidates'){params.set('limit','100');if(cursor)params.set('cursor',cursor);}return '/'+kind+'?'+params;}
export type ReleaseReferenceKind='environments'|'release-candidates';
export function releaseReferencePath(project:string,kind:ReleaseReferenceKind,id:string){return '/'+kind+'/'+encodeURIComponent(id)+'?'+new URLSearchParams({project_id:project});}
export function releaseReferenceName(project:string,kind:ReleaseReferenceKind,id:string,value:ReleaseEnvironment|CandidateDetail|undefined){const record=kind==='release-candidates'&&value&&'candidate'in value?value.candidate:value&&'id'in value?value:undefined;return record?.id===id&&record.project_id===project&&typeof record.name==='string'&&record.name.trim()?record.name:undefined;}
