import {expect,it} from 'vitest';
import captured from './test-fixtures/candidate-conflict-repair-plan.json';
import {compositionConflictBasis,validCompositionPlan} from './candidate-composition-model';
import {validRepairPlan} from './candidate-conflict-repair-model';

// Derived from the actual public create/readback wire, SHA-256
// 9ecc24adebd935c4d376197b4c03c5ee0a714ae92936332db40c545b4f866e1a.
// IDs, hashes, worker names and human text are deterministically sanitized;
// field presence, strategy and all identity/order equality relationships remain.
it('reads the actual repair strategy only with its complete fixed basis and rejects mismatched strategy or source lineage',()=>{
 const original=captured.conflict_basis.composition_id,project=captured.project_id,task=captured.task_id;
 expect(captured.strategy).toBe('conflict_repair');
 expect(compositionConflictBasis(captured.conflict_basis,project)).toBe(true);
 expect(validCompositionPlan(captured,project,task)).toBe(true);
 expect(validRepairPlan(captured,project,original)).toBe(true);
 for(const invalid of [
  {...captured,strategy:'merge_commit'},
  {...captured,conflict_basis:undefined},
  {...captured,tool_profile_sha256:''},
  {...captured,conflict_basis:{...captured.conflict_basis,evidence:{...captured.conflict_basis.evidence,source_heads:[...captured.conflict_basis.evidence.source_heads].reverse()}}},
  {...captured,conflict_basis:{...captured.conflict_basis,assignment:{...captured.conflict_basis.assignment,fence:captured.conflict_basis.fence+1}}},
 ])expect(validRepairPlan(invalid,project,original)).toBe(false);
 expect(validRepairPlan(captured,project,'different-original')).toBe(false);
});
