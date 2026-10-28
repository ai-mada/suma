package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// A late completed receipt resolves only the original live attempt. The Core
// proves its lease and fence; this hook rechecks current creator/profile/route
// authority. It neither retries a model call nor adopts any proposed draft.
func (s *Service) checkIntelligentOperationCompletionTx(ctx context.Context, tx *sql.Tx, scope dag.LiveReadOnlyCompletion, previous, final dag.Result) (bool, error) {
	if scope.NodeID != "analyze" || scope.Spec.Type != "agent" || stringify(scope.Spec.Config["max_steps"]) != "1" || final.ToolEvidence == nil {
		return false, nil
	}
	p := final.ToolEvidence.Profile
	if final.ToolEvidence.Terminal != "completed" || !intelligentOperationProfile(ToolProfile{State: "enabled", Profile: p}) {
		return false, nil
	}
	if previous.ToolEvidence != nil && !toolcontract.EqualProfile(previous.ToolEvidence.Profile, p) {
		return false, nil
	}
	record, found, err := s.intelligentOperationExecutionRecordTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID)
	if err != nil || !found {
		return false, err
	}
	a := record.Analysis
	if a.State != "reconciling" || a.MaxSteps != 1 || record.WorkerID != scope.WorkerID || a.ToolProfileSHA256 != p.ProfileSHA256 || scope.Spec.Config["tool_profile_id"] != a.ToolProfileID || scope.Spec.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return false, nil
	}
	if err = s.checkIntelligentOperationsExecutionTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID, false); err != nil {
		return false, err
	}
	// Source drift and malformed proposals are domain outcomes, not missing
	// execution facts. The outcome hook preserves bytes/usage and blocks adoption.
	return true, nil
}
