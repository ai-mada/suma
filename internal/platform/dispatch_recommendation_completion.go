package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This admits only completion of the already live attempt. Source freshness is
// evaluated by the outcome hook after the original bytes/usage can be recorded;
// drift never authorizes another model request or a development dispatch.
func (s *Service) checkDispatchRecommendationCompletionTx(ctx context.Context, tx *sql.Tx, scope dag.LiveReadOnlyCompletion, previous, final dag.Result) (bool, error) {
	if scope.NodeID != "recommend" || scope.Attempt != 1 || scope.Spec.Type != "agent" || scope.Spec.Config["purpose"] != "dispatch_recommendation" || stringify(scope.Spec.Config["max_steps"]) != "1" || final.ToolEvidence == nil {
		return false, nil
	}
	record, found, err := s.dispatchRecommendationForRunTx(ctx, tx, scope.TenantID, scope.ProjectID, scope.RunID)
	if err != nil || !found {
		return false, err
	}
	a := record.Recommendation
	if a.State != "reconciling" || record.WorkerID != scope.WorkerID || a.AnalysisModelExecution == nil || a.AnalysisModelExecution.Parameters.MaxSteps != 1 || a.AnalysisModelExecution.Parameters.MaxOutputTokens > 8192 || scope.Spec.Config["tool_profile_id"] != a.AnalysisToolProfileID || scope.Spec.Config["tool_profile_sha256"] != a.AnalysisToolProfileSHA256 || scope.Spec.Config["recommendation_source_sha256"] != a.Source.SHA256 || !dispatchRecommendationModelCompleted(final, a) {
		return false, nil
	}
	bound, err := dag.ModelBinding(scope.Spec)
	if err != nil || stringify(bound) != stringify(a.AnalysisModelExecution) {
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
	auth, err := s.dispatchRecommendationCreatorTx(ctx, tx, scope.TenantID, scope.ProjectID, record)
	if err != nil {
		return false, err
	}
	profile, err := s.toolProfileTx(ctx, tx, auth.currentPrincipal, scope.ProjectID, a.AnalysisToolProfileID)
	if err != nil {
		return false, err
	}
	if profile.Version != a.AnalysisToolProfileVersion || profile.WorkerID != scope.WorkerID || !dispatchRecommendationProfile(profile) || !tc.EqualProfile(profile.Profile, final.ToolEvidence.Profile) {
		return false, nil
	}
	profile, err = s.toolProfileObservationLockedTx(ctx, tx, scope.TenantID, scope.ProjectID, profile)
	if err != nil {
		return false, err
	}
	if !profile.Available {
		return false, nil
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, scope.TenantID, scope.ProjectID)
	if err != nil {
		return false, err
	}
	original, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, scope.TenantID, scope.ProjectID, scope.RunID)
	if err != nil {
		return false, err
	}
	if route != a.Source.ExecutionRoute || original != route {
		return false, nil
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, scope.TenantID, scope.ProjectID, profile, route); err != nil {
		return false, err
	}
	return true, nil
}

func dispatchRecommendationGatewayAdvances(binding mc.Binding, previous, final mc.Evidence) bool {
	if !mc.EvidenceMatchesBinding(binding, previous) || !mc.EvidenceMatchesBinding(binding, final) || len(final.Requests) != 1 {
		return false
	}
	if previous.GatewayClosed {
		return stringify(previous) == stringify(final)
	}
	if len(previous.Requests) == 0 {
		return true
	}
	old, next := previous.Requests[0], final.Requests[0]
	if old.State == "rejected" || old.RequestSHA256 != next.RequestSHA256 || old.OutputAllowance != next.OutputAllowance || old.ResponseSHA256 != "" && old.ResponseSHA256 != next.ResponseSHA256 || old.ProviderModel != "" && old.ProviderModel != next.ProviderModel || old.ProviderRequestID != "" && old.ProviderRequestID != next.ProviderRequestID {
		return false
	}
	if old.Usage != nil {
		if next.Usage == nil {
			return false
		}
		before := []*int64{old.Usage.InputTokens, old.Usage.OutputTokens, old.Usage.CachedInputTokens, old.Usage.CacheWriteInputTokens, old.Usage.ReasoningOutputTokens}
		after := []*int64{next.Usage.InputTokens, next.Usage.OutputTokens, next.Usage.CachedInputTokens, next.Usage.CacheWriteInputTokens, next.Usage.ReasoningOutputTokens}
		for i, n := range before {
			if n != nil && (after[i] == nil || *after[i] < *n) {
				return false
			}
		}
	}
	return true
}
