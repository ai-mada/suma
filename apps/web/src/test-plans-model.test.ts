import {describe,it,expect} from 'vitest';
import {newQualityCase,planDraftErrors,validTestPlan,validTestPlanExecution,testPlanRecovery} from './test-plans-model';
import type {QualityTestCase,TestPlanView,TestPlanExecution} from '../../../packages/contracts/src';

const sha='a'.repeat(64);
const source={task:{id:'task-a',version:3,title:'Task',description:'Current task',acceptance_criteria:['Authorized access']},task_basis_sha256:sha,sha256:sha};
const testCase:QualityTestCase={key:'authorize',title:'Authorization',description:'Current authorization',preconditions:'An authenticated caller',steps:['Request protected data'],expected_result:'Only the current project is accessible',priority:'high',covers:[1],selector:{class_name:'AccessTests',name:'rejects_other_project'}};
const revision={revision:1,state:'draft',title:'Access plan',description:'Review the current scope',source,cases:[testCase],sha256:sha,created_by:'user-a',created_at:'2026-10-28T00:00:00Z'};
export const plan:TestPlanView={id:'plan-a',project_id:'project-a',task_id:'task-a',version:1,state:'draft',latest_revision:1,published_revision:0,has_draft:true,basis_current:true,eligibility:'none',latest:revision,allowed_actions:['save','publish'],created_at:revision.created_at,updated_at:revision.created_at};

describe('test plans domain boundaries',()=>{
 it('accepts only the current project plan with the exact task snapshot and known state',()=>{
  expect(validTestPlan(plan,'project-a')).toBe(true);
  expect(validTestPlan({...plan,project_id:'foreign'},'project-a')).toBe(false);
  expect(validTestPlan({...plan,latest:{...revision,source:{...source,task:{...source.task,id:'other'}}}},'project-a')).toBe(false);
  expect(validTestPlan({...plan,state:'accepted'},'project-a')).toBe(false);
 });
 it('keeps incomplete automatic cases editable but rejects duplicate selectors and invalid task coverage',()=>{
  const empty=newQualityCase();
  expect(planDraftErrors('A draft','',[empty],source.task.acceptance_criteria)).toEqual([]);
  expect(planDraftErrors('A draft','',[testCase,{...testCase,key:'another'}],source.task.acceptance_criteria)).toContain('selector_duplicate');
  expect(planDraftErrors('A draft','',[{...testCase,covers:[2]}],source.task.acceptance_criteria)).toContain('covers_invalid');
  expect(planDraftErrors('A draft','',[{...testCase,selector:{class_name:'x'.repeat(501),name:'valid'}}],source.task.acceptance_criteria)).toContain('text_limit');
 });
 it('does not treat absent, skipped or ambiguous case reports as passing execution',()=>{
  const value:TestPlanExecution={plan_id:'plan-a',task_id:'task-a',revision:1,plan_sha256:sha,run_id:'run-a',run_state:'succeeded',created_at:revision.created_at,state:'incomplete',basis_current:true,source_commits:[],total:1,passed:0,failed:0,errors:0,skipped:0,not_run:1,cases:[{case_key:'authorize',title:'Authorization',status:'not_run',covers:[1],evidence:[],issue_id:''}],reports:[],package_evidence:[]};
  expect(validTestPlanExecution(value,'plan-a','task-a')).toBe(true);
  expect(validTestPlanExecution({...value,plan_id:'other'},'plan-a','task-a')).toBe(false);
  expect(validTestPlanExecution({...value,cases:[{...value.cases[0],status:'approved'}]},'plan-a','task-a')).toBe(false);
  expect(validTestPlanExecution({...value,total:2},'plan-a','task-a')).toBe(false);
  expect(validTestPlanExecution({...value,revision:0,plan_sha256:'',state:'pending',total:0,not_run:0,cases:[]},'plan-a','task-a')).toBe(true);
  expect(validTestPlanExecution({...value,reports:[{id:'report-missing',run_id:'run-a',node_id:'test',attempt:1,fence:1,worker_id:'worker-a',source_commit:'',sha256:'',protocol:'suma-junit-report-v1',eligibility:'invalid',process_exit_code:0,outcome_unknown:false,reported_at:revision.created_at,total:0,passed:0,failed:0,errors:0,skipped:0,unmapped:0}]},'plan-a','task-a')).toBe(true);
 });
 it('checks the package proof independently of a passing JUnit case',()=>{
  const commit='b'.repeat(40),report={id:'report-a',run_id:'run-a',node_id:'test',attempt:1,fence:1,worker_id:'worker-a',source_commit:commit,sha256:sha,protocol:'suma-junit-report-v1',eligibility:'current',process_exit_code:0,outcome_unknown:false,reported_at:revision.created_at,total:1,passed:1,failed:0,errors:0,skipped:0,unmapped:0};
  const verified={node_id:'package-test',source_node_id:'build',state:'verified',attempt:2,worker_id:'worker-b',source_worker_id:'worker-a',artifact_id:'artifact-a',manifest_sha256:sha,source_commit:commit};
  const unverified={node_id:verified.node_id,source_node_id:'build',state:'unverified',attempt:2};
  const value={plan_id:'plan-a',task_id:'task-a',revision:1,plan_sha256:sha,run_id:'run-a',run_state:'succeeded',created_at:revision.created_at,state:'passed',basis_current:true,source_commits:[commit],total:1,passed:1,failed:0,errors:0,skipped:0,not_run:0,cases:[{case_key:'authorize',title:'Authorization',status:'passed',covers:[1],evidence:[{report_id:report.id,node_id:'test',attempt:1,fence:1,worker_id:'worker-a',source_commit:commit,status:'passed',message:'',duration_ms:8}],issue_id:''}],reports:[report]};
  const valid=(overrides:Record<string,unknown>)=>validTestPlanExecution({...value,package_evidence:[verified],...overrides},'plan-a','task-a');
  expect(valid({})).toBe(true);
  expect(valid({package_evidence:[{...unverified,state:'skipped'}]})).toBe(true);
  expect(valid({package_evidence:[unverified]})).toBe(false);
  expect(valid({state:'incomplete',package_evidence:[unverified]})).toBe(true);
  expect(valid({state:'incomplete',package_evidence:[{...unverified,artifact_id:'artifact-a'}]})).toBe(false);
  expect(valid({state:'run_changed',basis_current:false})).toBe(false);
  expect(valid({package_evidence:[verified,verified]})).toBe(false);
  expect(valid({package_evidence:undefined})).toBe(false);
  expect(valid({package_evidence:null})).toBe(false);
 });
 it('restores only an original plan or defect command in its exact project scope',()=>{
  const request={path:'/test-plans/plan-a/commands',method:'POST',idempotency_key:'original-operation',body:{project_id:'project-a',expected_version:1,action:'publish',reason:'Reviewed',idempotency_key:'original-operation'}};
  expect(testPlanRecovery(request,'project-a')).toBe(true);
  expect(testPlanRecovery({...request,body:{...request.body,project_id:'foreign'}},'project-a')).toBe(false);
  expect(testPlanRecovery({...request,path:'/runner/results'},'project-a')).toBe(false);
  expect(testPlanRecovery({...request,body:{...request.body,passed:true}},'project-a')).toBe(false);
 });
});
