package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

var _ dag.LiveReadOnlyCompletionReconciler = (*Service)(nil)

// CheckLiveReadOnlyCompletionTx grants no new execution. The Core first proves
// an unchanged, still-leased, not-stopped unknown attempt, validates its final
// tool receipt, and requires the authenticated transport/commit hooks. Only a
// platform-created single-call knowledge extraction may resolve that uncertainty
// automatically. Ordinary CLIs and planning retain their existing reconciliation.
func (s *Service) CheckLiveReadOnlyCompletionTx(ctx context.Context, tx *sql.Tx, scope dag.LiveReadOnlyCompletion, previous, final dag.Result) (bool, error) {
	if scope.NodeID != "extract" || scope.Spec.Type != "agent" || scope.Spec.Config["purpose"] != "knowledge_extraction" || stringify(scope.Spec.Config["max_steps"]) != "1" || final.ToolEvidence == nil {
		return false, nil
	}
	p := final.ToolEvidence.Profile
	if final.ToolEvidence.Terminal != "completed" || p.Adapter != "suma" || !p.ReadOnly || p.WorkspaceToolsVersion != "" || p.Planning != nil || p.KnowledgeExtraction == nil || p.KnowledgeExtraction.Protocol != toolcontract.KnowledgeExtractionProtocol {
		return false, nil
	}
	if previous.ToolEvidence != nil && !toolcontract.EqualProfile(previous.ToolEvidence.Profile, p) {
		return false, nil
	}
	record, found, err := s.knowledgeIntelligenceExecutionRecordTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID)
	if err != nil || !found {
		return false, err
	}
	a := record.Analysis
	if a.State != "reconciling" || a.MaxSteps != 1 || record.WorkerID != scope.WorkerID || a.ToolProfileSHA256 != p.ProfileSHA256 || scope.Spec.Config["tool_profile_id"] != a.ToolProfileID || scope.Spec.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return false, nil
	}
	// Reconciliation is a fresh authority decision. Source-version changes do
	// not destroy received facts, but a revoked creator/profile/route cannot use
	// this automatic path. Explicit operator reconciliation remains available.
	if err = s.checkKnowledgeIntelligenceExecutionTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID, false); err != nil {
		return false, err
	}
	// Proposal validity is deliberately evaluated by the domain commit hook,
	// after this execution receipt is accepted. A malformed model response still
	// has real usage and an immutable original; it becomes a failed proposal.
	return true, nil
}
