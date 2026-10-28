package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	drc "github.com/ai-mada/suma/internal/dispatchrecommendationcontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func dispatchRecommendationAdmission(message string) error {
	return &APIError{Status: 409, Code: "dispatch_recommendation_basis_changed", Message: message}
}

// Recover only the real still-current originating browser credential. This
// never creates authority from the stored user ID or a runner's claimed name.
func (s *Service) dispatchRecommendationCreatorTx(ctx context.Context, tx *sql.Tx, tenant, project string, r dispatchRecommendationRecord) (*authorizedCommand, error) {
	auth, err := s.persistedBrowserProjectWriteLockedTx(ctx, tx, r.CreatorCredentialHash, r.Recommendation.CreatedBy, commandTarget{tenantID: tenant, projectID: project, resourceID: r.Request.TaskID})
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return nil, dispatchRecommendationAdmission(dispatchRecommendationAuthority)
		}
		return nil, err
	}
	return auth, nil
}

func (s *Service) checkDispatchRecommendationExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) error {
	record, found, err := s.dispatchRecommendationForRunTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return err
	}
	a := record.Recommendation
	if a.State != "running" && a.State != "reconciling" {
		return dispatchRecommendationAdmission(dispatchRecommendationVersion)
	}
	var attempt int
	var nodeState string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT attempt,state FROM dag_nodes WHERE run_id=? AND id='recommend'`), run).Scan(&attempt, &nodeState); err != nil {
		return err
	}
	// A manually retried/reconciled DAG must not turn one recommendation into
	// another model call. Recovery inspects the original operation instead.
	if attempt > 1 || attempt > 0 && nodeState != "running" && nodeState != "cancelling" && nodeState != "reconciling" && nodeState != "succeeded" {
		return dispatchRecommendationAdmission(dispatchRecommendationVersion)
	}
	auth, err := s.dispatchRecommendationCreatorTx(ctx, tx, tenant, project, record)
	if err != nil {
		return err
	}
	if err = s.checkDispatchRecommendationBasisTx(ctx, nil, auth, record); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return dispatchRecommendationAdmission(api.Message)
		}
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	if bound != a.Source.ExecutionRoute {
		return dispatchRecommendationAdmission(dispatchRecommendationSourceChanged)
	}
	return nil
}

func dispatchRecommendationModelCompleted(result dag.Result, a DispatchRecommendation) bool {
	tool := result.ToolEvidence
	if tool == nil || a.AnalysisModelExecution == nil || tool.ModelExecution == nil || tool.Terminal != "completed" || tool.Profile.ProfileSHA256 != a.AnalysisToolProfileSHA256 || !dispatchRecommendationProfile(ToolProfile{State: "enabled", Profile: tool.Profile}) {
		return false
	}
	proof, b := tool.ModelExecution, a.AnalysisModelExecution
	if proof.GatewayProtocol != mc.GatewayProtocol || !proof.GatewayClosed || proof.BindingSHA256 != b.BindingSHA256 || proof.DeploymentSHA256 != b.Deployment.SHA256 || proof.ParametersSHA256 != b.ParametersSHA256 || proof.Model != b.Deployment.Model || len(proof.Requests) != 1 {
		return false
	}
	request := proof.Requests[0]
	return request.State == "completed" && request.Step == 0 && request.ProviderModel == b.Deployment.Model && request.ResponseSHA256 != "" && request.UsageState == tc.UsageComplete && tool.UsageState == tc.UsageComplete
}

func dispatchRecommendationBoundedOutput(raw string) string {
	if len(raw) > drc.MaxOutputBytes || !utf8.ValidString(raw) {
		return ""
	}
	return raw
}

// Callback projection is in the original result transaction, after the Go
// scheduler validates the current attempt. It cannot launch or approve work.
func (s *Service) recordDispatchRecommendationOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	record, found, err := s.dispatchRecommendationForRunTx(ctx, tx, tenant, project, result.RunID)
	if err != nil || !found {
		return err
	}
	if authorize == nil {
		return forbidden()
	}
	if err = authorize(tx); err != nil {
		return err
	}
	a := record.Recommendation
	var attempt int
	var fence int64
	var worker, raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &raw)
	if err != nil {
		return err
	}
	if result.NodeID != "recommend" || result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker || worker != record.WorkerID {
		return forbidden()
	}
	var node dag.Node
	if json.Unmarshal([]byte(raw), &node) != nil || node.Config["purpose"] != "dispatch_recommendation" || node.Config["recommendation_source_sha256"] != a.Source.SHA256 || node.Config["tool_profile_id"] != a.AnalysisToolProfileID || node.Config["tool_profile_sha256"] != a.AnalysisToolProfileSHA256 {
		return forbidden()
	}
	bound, err := dag.ModelBinding(node)
	if err != nil || stringify(bound) != stringify(a.AnalysisModelExecution) {
		return forbidden()
	}
	resultSHA := digest(stringify(result))
	var count int
	err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_dispatch_recommendation_outcomes WHERE tenant_id=? AND project_id=? AND recommendation_id=? AND attempt=? AND fence=? AND result_sha256=?`), tenant, project, a.ID, attempt, fence, resultSHA).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if a.State != "running" && a.State != "reconciling" {
		return conflict(dispatchRecommendationVersion)
	}
	previous := a.Version
	a.Version++
	a.UpdatedAt = now()
	a.AllowedActions = []string{}
	a.Proposal = nil
	// An unknown partial observation is preserved in its immutable outcome,
	// not presented as a completed original model answer. Oversized known
	// answers retain their original digest/result without a misleading truncation.
	if !result.OutcomeUnknown {
		a.OriginalOutputSHA256 = digest(result.Output)
		a.OriginalOutput = dispatchRecommendationBoundedOutput(result.Output)
	}
	a.BasisCurrent = true
	switch {
	case result.OutcomeUnknown:
		a.State, a.Reason = "reconciling", dispatchRecommendationUnknown
	case result.Stopped || result.ExitCode != 0 || result.Error != "":
		a.State, a.Reason = "failed", dispatchRecommendationFailed
	case !dispatchRecommendationModelCompleted(result, a):
		a.State, a.Reason = "failed", dispatchRecommendationFailed
	default:
		proposal, e := drc.ParseProposal(result.Output)
		if e != nil || validateDispatchRecommendationProposal(a.Source, proposal) != nil {
			a.State, a.Reason = "failed", dispatchRecommendationOutput
			break
		}
		if e = s.checkDispatchRecommendationExecutionTx(ctx, tx, tenant, project, result.RunID); e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return e
			}
			a.State, a.Reason, a.BasisCurrent = "failed", api.Message, false
			break
		}
		a.State, a.Reason, a.Proposal = "ready", "", &proposal
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_dispatch_recommendation_outcomes(tenant_id,project_id,recommendation_id,attempt,fence,result_sha256,created_at,result)VALUES(?,?,?,?,?,?,?,?)`), tenant, project, a.ID, attempt, fence, resultSHA, a.UpdatedAt, stringify(result)); err != nil {
		return err
	}
	record.Recommendation = a
	return s.saveDispatchRecommendationTx(ctx, tx, tenant, record, previous)
}
