import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {describe,expect,it} from 'vitest';
import {ArtifactDetails,CandidateDetails,DeploymentDetails,PlanDetails,ReleaseUnreadList} from './ReleaseWorkspace';
import {ReleaseNamedReferenceView} from './ReleaseFacts';
import {parseWorkbenchRoute} from './navigation';
import {releaseItemID,releaseListPath,releaseReferenceName,releaseReferencePath,type ReleaseArtifact,type ReleaseDeployment,type ReleasePlan} from './release-model';

const noop=()=>{};
describe('release rendered consumers and deep links',()=>{
 it('reads human names only from the exact scoped environment/candidate and keeps full identifiers in technical details',()=>{
  const id='3bd116fa-1de2-4592-9b17-abcdef123456',environment={id,project_id:'p',name:'内部预览站点'} as any,candidate={candidate:{id,project_id:'p',name:'本周正式交付'}} as any;
  expect(releaseReferencePath('project/a','environments','env a')).toBe('/environments/env%20a?project_id=project%2Fa');
  expect(releaseReferenceName('p','environments',id,environment)).toBe('内部预览站点');expect(releaseReferenceName('p','release-candidates',id,candidate)).toBe('本周正式交付');
  expect(releaseReferenceName('other','environments',id,environment)).toBeUndefined();expect(releaseReferenceName('p','release-candidates','different',candidate)).toBeUndefined();
  const html=renderToStaticMarkup(createElement(ReleaseNamedReferenceView,{id,name:environment.name,label:'目标环境',loading:false,error:'',onRetry:noop}));
  expect(html).toContain('<strong>内部预览站点</strong>');expect(html).toContain('<details><summary>技术标识</summary>');expect(html).toContain(id);
  const fallback=renderToStaticMarkup(createElement(ReleaseNamedReferenceView,{id,label:'目标环境',loading:false,error:'无读取权限',onRetry:noop}));
  expect(fallback).toContain('<strong>3bd116fa-1de…</strong>');expect(fallback).toContain('名称暂未读取');expect(fallback).toContain('重新读取名称');expect(fallback).not.toContain('内部预览站点');
 });
 it('stops announcing loading when the first collection read fails without inventing an empty count',()=>{
  const html=renderToStaticMarkup(createElement(ReleaseUnreadList,{loading:false}));
  expect(html).toContain('列表尚未读取');expect(html).toContain('没有可确认的记录范围');expect(html).not.toContain('加载中');expect(html).not.toContain('尚无真实部署操作');
 });
 it('keeps sealed bytes distinct from proof and prevents fixing an unproved artifact as a candidate',()=>{
  const artifact:ReleaseArtifact={id:'artifact',project_id:'p',state:'sealed',manifest_sha256:'real-manifest',size_bytes:4,created_at:'2026-10-24T00:00:00Z',manifest:{schema_version:1,kind:'static_site_v1',files:[{path:'index.html',size_bytes:4,sha256:'real-file'}]},proofs:[]};
  const html=renderToStaticMarkup(createElement(ArtifactDetails,{artifact,onCandidate:noop,disabled:false}));
  for(const text of ['字节已封存','尚无真实构建成功证明','尚无对本制品字节的包测试成功证明','real-manifest','index.html','real-file'])expect(html).toContain(text);
  expect(html).toMatch(/<button[^>]*disabled=""[^>]*>/);expect(html).not.toContain('上传制品');
 });
 it('shows only the recorded build scope in artifact and candidate evidence',()=>{
  const applicationBuild={application_id:'application-a',version_id:'version-a',version_sha256:'a'.repeat(64),scope_id:'scope-a',scope_root:'apps/storefront',scope_tree_oid:'b'.repeat(40),source_commit:'c'.repeat(40),receipt_sha256:'d'.repeat(64)};
  const build={id:'proof-build',project_id:'p',assignment:{run_id:'run',node_id:'build',worker_id:'builder',attempt:1,fence:1},kind:'build',artifact_id:'artifact',manifest_sha256:'manifest',source_commit:applicationBuild.source_commit,profile_id:'command',profile_sha256:'profile',workspace_sha256:'worktree',created_at:'2026-10-24T00:00:00Z',application_build:applicationBuild};
  const {application_build:_,...projectBuild}=build;
  const test={...projectBuild,id:'proof-test',kind:'package_test',assignment:{...build.assignment,node_id:'package-test',worker_id:'tester'}};
  const artifact:ReleaseArtifact={id:'artifact',project_id:'p',state:'sealed',manifest_sha256:'manifest',size_bytes:4,created_at:build.created_at,manifest:{schema_version:1,kind:'static_site_v1',files:[]},proofs:[build,test]};
  const candidate={id:'candidate',project_id:'p',name:'Storefront',artifact_id:artifact.id,artifact_sha256:artifact.manifest_sha256,source_commit:build.source_commit,build_attestation_id:build.id,package_test_attestation_id:test.id,accepted_task_basis_sha256:'accepted',build_proof:build,package_test_proof:test,tasks:[],created_at:build.created_at};
  const artifactHTML=renderToStaticMarkup(createElement(ArtifactDetails,{artifact,onCandidate:noop,disabled:false}));
  const candidateHTML=renderToStaticMarkup(createElement(CandidateDetails,{detail:{candidate,eligibility:false,blockers:[]},onPrepare:noop,disabled:false}));
  for(const html of [artifactHTML,candidateHTML]){
   expect(html.match(/应用范围构建/g)).toHaveLength(1);
   for(const value of [applicationBuild.application_id,applicationBuild.version_id,applicationBuild.scope_root,applicationBuild.scope_tree_oid])expect(html).toContain('<code>'+value+'</code>');
   expect(html).not.toContain('未记录应用代码范围');
  }
  const ordinary=renderToStaticMarkup(createElement(ArtifactDetails,{artifact:{...artifact,proofs:[projectBuild,test]},onCandidate:noop,disabled:false}));
  expect(ordinary).toContain('未记录应用代码范围');
  expect(ordinary).not.toContain(applicationBuild.version_id);
 });
 it('renders immutable plan scope and current blockers together without presenting historical approval as executable',()=>{
  const plan:ReleasePlan={created_by:'author',created_at:'2026-10-24T00:00:00Z',id:'plan',project_id:'p',version:9,revision:1,sha256:'plan-hash',state:'approved',eligibility:false,blockers:[{code:'basis_changed',message:'任务验收依据已变化'}],allowed_actions:[],execution_operation_id:'',content:{action:'deploy',artifact_id:'artifact-real',build_attestation_id:'build-proof',package_test_attestation_id:'test-proof',reserved_execution:{run_id:'managed-run',node_id:'deploy'},candidate_id:'candidate-real',environment_id:'env-real',provider_id:'provider-real',site_id:'site-real',environment_policy_version:3,expected_generation:8,generation:9,artifact_sha256:'artifact-hash',source_commit:'source-commit',accepted_task_basis_sha256:'accepted-basis',health:{checks:[{path:'/',expected_status:200,expected_file:'index.html',expected_sha256:'expected-body'}],attempts:2,timeout_ms:2000}},approval:{actor_id:'approver',decision:'approved',reason:'历史批准依据',at:'2026-10-24T00:00:00Z',plan_sha256:'plan-hash'}};
  const html=renderToStaticMarkup(createElement(PlanDetails,{plan,disabled:false,onDecision:noop,onExecute:noop,onCandidate:noop,onEnvironment:noop,onOperation:noop}));
  for(const text of ['r1','v9','plan-hash','basis_changed','任务验收依据已变化','历史批准依据','artifact-hash','accepted-basis','expected-body','env-real','site-real'])expect(html).toContain(text);
  expect(html).not.toContain('执行这份已批准计划');
 });
 it('shows applied content and failed health together and gives only server permitted recovery actions',()=>{
  const deployment={operation_id:'op-real',project_id:'p',version:3,plan_id:'plan-real',environment_id:'env-real',artifact_sha256:'artifact-real',run_id:'managed-run',node_id:'deploy',state:'deployed_unhealthy',effect_state:'applied',health_state:'failed',allowed_actions:['reconcile','rollback_preview'],blockers:[],observation:null,observations:[],issue_id:'issue-real',created_at:'2026-10-24T00:00:00Z'} as unknown as ReleaseDeployment;
  const html=renderToStaticMarkup(createElement(DeploymentDetails,{deployment,disabled:false,onPlan:noop,onEnvironment:noop,onReconcile:noop,onRollback:noop}));
  for(const text of ['内容切换已确认','健康检查失败','核对原操作','准备独立回退计划','尚无已认证的提供方观察','managed-run','issue-real'])expect(html).toContain(text);
  expect(html).not.toContain('>已回退<');expect(html).not.toContain('触发健康检查');expect(html).not.toContain('取消部署');
 });
 it('restores exact project resource and evidence section independently of a loaded list page',()=>{
  const route=parseWorkbenchRoute('?project=project-real&view=operations&sub=release-plans&record=plan-real&panel=review');
  expect(route).toMatchObject({project:'project-real',view:'operations',sub:'release-plans',record:'plan-real',panel:'review'});
  expect(releaseListPath('p','release-candidates','opaque-cursor')).toBe('/release-candidates?project_id=p&limit=100&cursor=opaque-cursor');
  expect(releaseListPath('p','deployments','irrelevant')).toBe('/deployments?project_id=p');
  expect(releaseItemID({candidate:{id:'candidate-off-page'}} as any)).toBe('candidate-off-page');
 });
});
