package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Only the same still-leased original read-only attempt can advance an unknown
// receipt. The Core verifies lease, stop and fence; this hook grants no retry.
func (s *Service) checkTaskConversationReplyCompletionTx(ctx context.Context, tx *sql.Tx, scope dag.LiveReadOnlyCompletion, previous, final dag.Result) (bool, error) {
	if scope.NodeID != "reply" || scope.Attempt != 1 || scope.Spec.Type != "agent" || scope.Spec.Config["purpose"] != "task_conversation_reply" || stringify(scope.Spec.Config["max_steps"]) != "1" || final.ToolEvidence == nil {
		return false, nil
	}
	record, found, err := s.taskConversationReplyForRunTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID)
	if err != nil || !found {
		return false, err
	}
	a := record.Reply
	if a.State != "reconciling" || record.WorkerID != scope.WorkerID || !taskConversationReplyModelCompleted(final, a) || scope.Spec.Config["conversation_source_sha256"] != a.Source.SHA256 || scope.Spec.Config["tool_profile_id"] != a.ToolProfileID || scope.Spec.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return false, nil
	}
	bound, err := dag.ModelBinding(scope.Spec)
	if err != nil || stringify(bound) != stringify(a.ModelExecution) {
		return false, nil
	}
	if previous.ToolEvidence != nil {
		if !tc.EqualProfile(previous.ToolEvidence.Profile, final.ToolEvidence.Profile) {
			return false, nil
		}
		if old := previous.ToolEvidence.ModelExecution; old != nil && !dispatchRecommendationGatewayAdvances(*bound, *old, *final.ToolEvidence.ModelExecution) {
			return false, nil
		}
	}
	// Live automatic reconciliation still needs the original current creator,
	// profile and route. A normal terminal result is retained independently by
	// the outcome hook even if those conditions subsequently changed.
	if err = s.checkTaskConversationReplyExecutionTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID, true); err != nil {
		return false, err
	}
	return true, nil
}
