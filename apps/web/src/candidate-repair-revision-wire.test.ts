import {expect,it} from 'vitest';
import captured from './test-fixtures/candidate-repair-revision.json';
import {validCompositionPlan} from './candidate-composition-model';
import {repairRevisionRecovery,validRepairRevisionContext,validRepairRevisionResponse} from './candidate-repair-revision-model';
import type {CandidateCompositionPlan} from '../../../packages/contracts/src';

// Actual public Service create/context/revise DTOs, combined raw SHA-256
// 57bb3b7a52fdef29144ea1fa0d0eff551d6529c3b67c5036c179b4872c7d18f3.
// Only identifiers and64-byte digests are deterministically sanitized. Presence,
// strategy, versions, lineage, source order and cross-envelope equality are kept.
it('consumes the actual revision envelopes with same-task new-ID lineage and rejects cross-task or selected-definition mismatches',()=>{
 const {context,parent,request,plan}=captured,p=parent as unknown as CandidateCompositionPlan;
 const recovery={path:'/candidate-compositions/'+parent.id+'/repair-revisions',method:'POST',body:request,idempotency_key:request.idempotency_key};
 expect(validCompositionPlan(parent,parent.project_id,parent.task_id)).toBe(true);
 expect(validRepairRevisionContext(context,parent.project_id,p)).toBe(true);
 expect(repairRevisionRecovery(recovery,parent.project_id,parent.id)).toBe(true);
 expect(validRepairRevisionResponse(plan,parent.project_id,p,recovery)).toBe(true);
 for(const invalid of [{...plan,task_id:'other-task'},{...plan,id:parent.id},{...plan,workflow_version:plan.workflow_version+1},{...plan,tool_profile_version:plan.tool_profile_version+1},{...plan,sources:[...plan.sources].reverse()},{...plan,repair_revision:{...plan.repair_revision,parent_plan_id:'another-parent'}},{...plan,repair_revision:{...plan.repair_revision,creator_principal_id:''}}])expect(validRepairRevisionResponse(invalid,parent.project_id,p,recovery)).toBe(false);
 expect(validRepairRevisionContext({...context,can_revise:true,eligibility_mode:''},parent.project_id,p)).toBe(false);
 expect(repairRevisionRecovery({...recovery,body:{...request,forged_route:'client-route'}},parent.project_id,parent.id)).toBe(false);
});
