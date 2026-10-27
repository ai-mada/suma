import {expect,it} from 'vitest';
import {repairRecovery} from './candidate-conflict-repair-model';
import {repairRevisionRecovery} from './candidate-repair-revision-model';
const model={deployment_id:'model-a',deployment_revision:1,deployment_sha256:'a'.repeat(64),parameters:{max_steps:2,max_output_tokens:100},budget:{token_limit:20000}};
const fields={project_id:'project-a',expected_conflict_sha256:'b'.repeat(64),workflow_id:'workflow-a',workflow_version:1,producer_node_id:'repair',tool_profile_id:'tool-a',expected_tool_version:2,model_execution_selection:model};
const request=(revision:boolean)=>({path:'/candidate-compositions/original/'+(revision?'repair-revisions':'repairs'),method:'POST',idempotency_key:'original-key',body:revision?{...fields,expected_version:1,expected_task_version:2,reason:'Reviewed native model',acknowledge:true}:{...fields,expected_composition_version:1,repair:{title:'Repair',description:'Preserve each source',priority:'medium',acceptance_criteria:['Independent tests pass']}}});
it('recovers typed native allowances only within the closed repair requests and refuses Codex, null and private fields',()=>{
 for(const revision of [false,true]){
  const original=request(revision),valid=(body:Record<string,unknown>)=>(revision?repairRevisionRecovery:repairRecovery)({...original,body},'project-a','original');
  expect(valid(original.body)).toBe(true);
  for(const body of [{...original.body,model_execution_selection:null},{...original.body,codex_execution_selection:{model:'codex-a'}},{...original.body,model_execution_selection:{...model,api_token:'private'}},{...original.body,model_execution_selection:{...model,budget:{token_limit:'private'}}}])expect(valid(body)).toBe(false);
 }
});
