import {expect,it} from 'vitest';
import {compositionCodeBaseline,compositionRecovery,validCompositionOptions} from './candidate-composition-model';
import type {WriteRecoveryRequest} from './write-action';

it('T222 accepts exactly one baseline origin, requires the explicit source qualification and keeps the two-source composition creation boundary',()=>{
 const project='project-a',task='target-task',sha='a'.repeat(64),source={schema_version:1,transfer_id:'transfer-a',project_id:project,state:'sealed',source:{run_id:'source-run',node_id:'agent.one',attempt:1,fence:2,worker_id:'worker-a'},base_commit:'b'.repeat(40),head_commit:'c'.repeat(40),evidence_sha256:sha,bundle_sha256:sha,size_bytes:32};
 const base={protocol:'suma.code-baseline.v1',id:'baseline-a',project_id:project,task_id:task,task_version:2,accepted_basis_sha256:sha,source,binding_sha256:sha};
 const ordinary={...base,source_task_id:'accepted-task',source_task_version:4},composition={...base,composition_id:'composition-a',composition_version:3},terminal={...base,terminal_candidate_plan_id:'terminal-a',terminal_candidate_plan_version:2};
 for(const value of [ordinary,composition,terminal])expect(compositionCodeBaseline(value,project,task)).toBe(true);
 for(const value of [{...ordinary,source_task_id:task},{...ordinary,source_task_version:0},{...ordinary,source_task_version:null},{...ordinary,composition_id:''},{...ordinary,terminal_candidate_plan_id:null},{...composition,source_task_id:''},{...terminal,source_task_version:0},base])expect(compositionCodeBaseline(value,project,task)).toBe(false);
 const body={project_id:project,expected_task_version:2,expected_binding_version:1,source_task_id:'accepted-task',expected_source_task_version:4,source_node_id:'agent.one',acknowledge:true,reason:'Reviewed candidate',idempotency_key:'operation-123'};
 const request=(value:Record<string,unknown>):WriteRecoveryRequest=>({method:'POST',path:'/tasks/'+task+'/code-baseline',body:value,idempotency_key:'operation-123'});
 expect(compositionRecovery(request(body),project,task)).toBe(true);
 for(const value of [{...body,source_task_id:task},{...body,source_node_id:''},{...body,source_node_id:'x'.repeat(151)},{...body,expected_source_task_version:0},{...body,composition_id:null},{...body,expected_composition_version:0},{...body,terminal_candidate_plan_id:''},{...body,expected_terminal_candidate_plan_version:0},{...body,source:source}])expect(compositionRecovery(request(value),project,task)).toBe(false);
 const {source_task_id:_task,expected_source_task_version:_version,source_node_id:_node,...common}=body;
 expect(compositionRecovery(request({...common,composition_id:'composition-a',expected_composition_version:3}),project,task)).toBe(true);
 expect(compositionRecovery(request({...common,terminal_candidate_plan_id:'terminal-a',expected_terminal_candidate_plan_version:2}),project,task)).toBe(true);
 const choice={task_id:'accepted-task',task_version:4,title:'Accepted task',source_node_id:'agent.one',source,eligible:false,can_bind_baseline:true,blockers:[]};
 const options={project_id:project,task_id:task,task_version:2,can_create:false,blockers:[],sources:[choice]};
 expect(validCompositionOptions(options,project,task)).toBe(true);
 const {can_bind_baseline:_qualification,...missing}=choice;
 for(const value of [missing,{...choice,can_bind_baseline:null},{...choice,can_bind_baseline:'true'},{...choice,source_node_id:'other-node'}])expect(validCompositionOptions({...options,sources:[value]},project,task)).toBe(false);
 const create={project_id:project,task_id:task,expected_task_version:2,workflow_id:'flow-a',workflow_version:1,producer_node_id:'compose',sources:[{task_id:'a',expected_task_version:4,source_node_id:'agent.one'}],idempotency_key:'operation-123'};
 expect(compositionRecovery({...request(create),path:'/candidate-compositions'},project,task)).toBe(false);
 expect(compositionRecovery({...request({...create,sources:[...create.sources,{task_id:'b',expected_task_version:4,source_node_id:'agent.two'}]}),path:'/candidate-compositions'},project,task)).toBe(true);
});
