import {describe,expect,it} from 'vitest';
import {candidateRequest,planInput,planCreateRequest,planDecisionRequest,deploymentFacts,hasReleaseAction,type ReleaseArtifact,type ReleaseCandidate,type ReleaseEnvironment,type ReleasePlan} from './release-model';

const proof=(kind:string)=>({id:kind,project_id:'p',assignment:{run_id:'build-run',node_id:kind,attempt:2,fence:9,worker_id:'trusted-worker'},kind,artifact_id:'artifact-a',manifest_sha256:'a'.repeat(64),source_commit:'c'.repeat(40),profile_id:kind,profile_sha256:'d'.repeat(64),workspace_sha256:'e'.repeat(64),created_at:'2026-10-24T00:00:00Z'});
const artifact:ReleaseArtifact={id:'artifact-a',project_id:'p',manifest_sha256:'a'.repeat(64),manifest:{schema_version:1,kind:'static_site_v1',files:[{path:'index.html',size_bytes:4,sha256:'f'.repeat(64)}]},size_bytes:4,state:'sealed',created_at:'2026-10-24T00:00:00Z',proofs:[proof('build'),proof('package_test')]};
const environment={id:'env-a',project_id:'p',version:3,status:'active',generation:8,active_generation:5,barrier_operation_id:''} as ReleaseEnvironment;

describe('release request consumers',()=>{
 it('freezes real successful proof references and task versions without a user supplied passed conclusion',()=>{
  const request=candidateRequest('p','可审阅候选',artifact,artifact.proofs[0],artifact.proofs[1],[{id:'task-a',version:7,status:'accepted'}]);
  expect(JSON.parse(JSON.stringify(request))).toEqual({project_id:'p',name:'可审阅候选',artifact_id:'artifact-a',build_attestation_id:'build',package_test_attestation_id:'package_test',tasks:[{id:'task-a',expected_version:7}]});
  expect(()=>candidateRequest('p','draft',artifact,artifact.proofs[0],{...artifact.proofs[1],manifest_sha256:'b'.repeat(64)},[])).toThrow('同一');
  expect(()=>candidateRequest('p','draft',{...artifact,state:'receiving'},artifact.proofs[0],artifact.proofs[1],[])).toThrow('封存');
 });
 it('uses highest reserved generation rather than the active generation in the actual plan body',()=>{
  expect(planInput('p','candidate-a',environment)).toEqual({project_id:'p',candidate_id:'candidate-a',environment_id:'env-a',action:'deploy',expected_environment_generation:8});
  expect(planInput('p','candidate-a',environment,'prior-operation')).toMatchObject({action:'rollback',expected_environment_generation:8,rollback_source_operation_id:'prior-operation'});
 });
 it('rejects a stale plan preview when any prepared input changes and preserves exact preview digest',()=>{
  const input=planInput('p','candidate-a',environment),preview={preview_sha256:'preview-real',environment,candidate_id:'candidate-a',action:'deploy',health:environment.health,eligibility:true,blockers:[],allowed_actions:['create']};
  expect(planCreateRequest(input,input,preview)).toEqual({...input,preview_sha256:'preview-real'});
  expect(()=>planCreateRequest({...input,candidate_id:'another'},input,preview)).toThrow('重新预览');
  expect(()=>planCreateRequest({...input,expected_environment_generation:9},input,preview)).toThrow('重新预览');
  expect(()=>planCreateRequest(input,input,{...preview,eligibility:false})).toThrow('条件');
  expect(()=>planCreateRequest(input,input,{...preview,allowed_actions:[]})).toThrow('权限');
 });
 it('submits independent decision version and fixed content revision/hash',()=>{
  const plan={id:'plan-a',project_id:'p',version:6,revision:1,sha256:'fixed-content'} as ReleasePlan;
  expect(planDecisionRequest(plan,'approved','已逐项核对')).toEqual({project_id:'p',expected_version:6,plan_revision:1,plan_sha256:'fixed-content',decision:'approved',reason:'已逐项核对'});
 });
 it('uses server action grants and eligibility instead of interpreting a historical approved label as permission',()=>{
  expect(hasReleaseAction({allowed_actions:['execute'],eligibility:true},'execute')).toBe(true);
  expect(hasReleaseAction({allowed_actions:['execute'],eligibility:false},'execute')).toBe(false);
  expect(hasReleaseAction({allowed_actions:[],eligibility:true},'execute')).toBe(false);
  expect(hasReleaseAction({allowed_actions:['revoke'],eligibility:false},'revoke')).toBe(true);
 });
 it('keeps applied effect and failed health visible together, never infers rollback, and keeps future states unknown',()=>{
  const facts=deploymentFacts({state:'deployed_unhealthy',effect_state:'applied',health_state:'failed'});
  expect(facts).toMatchObject({state:'已切换，健康异常',effect:'内容切换已确认',health:'健康检查失败'});
  expect(JSON.stringify(facts)).not.toContain('已回退');
  expect(deploymentFacts({state:'queued',effect_state:'not_started',health_state:'not_checked'})).toMatchObject({state:'已排队',effect:'尚未开始切换',health:'尚未检查'});
  expect(deploymentFacts({state:'new-provider-state',effect_state:'unknown',health_state:'unknown'})).toMatchObject({state:'未识别状态：new-provider-state',effect:'切换结果待核对',health:'健康结果未确认'});
 });
});
