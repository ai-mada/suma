package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
)

func operationAdmission(message string) error {
	return &APIError{Status: 409, Code: "intelligent_operation_basis_changed", Message: message}
}

func (s *Service) checkIntelligentOperationsExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, requireCurrentSource bool) error {
	record, found, err := s.intelligentOperationExecutionRecordTx(ctx, tx, tenant, project, run)
	if err != nil || !found {
		return err
	}
	a := record.Analysis
	if requireCurrentSource && a.State != "running" && a.State != "reconciling" {
		return operationAdmission(operationVersion)
	}
	credential, err := s.readCredential(ctx, tx, record.CreatorCredentialHash, true)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return operationAdmission(operationAuthority)
		}
		return err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return err
	}
	if credential.kind != actorBrowser && credential.kind != actorProjectAPI || credential.kind == actorProjectAPI && credential.ref.projectID != project || credential.ref.tenantID != tenant || credential.ref.subjectID != a.CreatedBy || !credential.expires.After(clock) {
		return operationAdmission(operationAuthority)
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, tenant, a.CreatedBy, project, true, false); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return operationAdmission(operationAuthority)
		}
		return err
	}
	if requireCurrentSource {
		if err = s.checkIntelligentOperationSourceTx(ctx, tx, tenant, project, a.Source); err != nil {
			var api *APIError
			if errors.As(err, &api) {
				return operationAdmission(operationSource)
			}
			return err
		}
	}
	profile, err := s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, a.ToolProfileID)
	if err != nil {
		return err
	}
	if !intelligentOperationProfile(profile) || profile.Version != a.ToolProfileVersion || profile.Profile.ProfileSHA256 != a.ToolProfileSHA256 || profile.WorkerID != record.WorkerID {
		return operationAdmission(operationProfile)
	}
	profile, err = s.toolProfileObservationLockedTx(ctx, tx, tenant, project, profile)
	if err != nil {
		return err
	}
	if !profile.Available {
		return operationAdmission(operationProfile)
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, run)
	if err != nil {
		return err
	}
	if route != record.Route || bound != record.Route {
		return operationAdmission(operationSource)
	}
	return s.checkDispatchProfileRouteTx(ctx, tx, tenant, project, profile, route)
}

// Called only after the existing DAG result/attempt and current Runner or pool
// authority have been validated. Source drift never erases real process facts.
func (s *Service) recordIntelligentOperationsOutcomeTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result, authorize func(*sql.Tx) error) error {
	record, found, err := s.intelligentOperationExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if err != nil || !found {
		return err
	}
	if authorize == nil {
		return forbidden()
	}
	if err = authorize(tx); err != nil {
		return err
	}
	a := record.Analysis
	var attempt int
	var fence int64
	var worker, raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &raw)
	if err != nil {
		return err
	}
	if result.NodeID != "analyze" || result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker || worker != record.WorkerID {
		return forbidden()
	}
	var node dag.Node
	if json.Unmarshal([]byte(raw), &node) != nil || node.Config["purpose"] != "intelligent_operations" || stringify(node.Config["max_steps"]) != "1" || node.Config["tool_profile_id"] != a.ToolProfileID || node.Config["tool_profile_sha256"] != a.ToolProfileSHA256 {
		return forbidden()
	}
	sha := digest(stringify(result))
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_intelligent_operation_executions WHERE analysis_id=? AND attempt=? AND fence=? AND result_sha256=?`), a.ID, attempt, fence, sha).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if a.State != "running" && a.State != "reconciling" {
		return conflict(operationVersion)
	}
	old := a.Version
	a.Version++
	a.UpdatedAt = now()
	a.AllowedActions = []string{}
	a.BasisCurrent = true
	record.SystemReason = true
	if !result.OutcomeUnknown && a.OriginalOutputSHA256 == "" {
		a.OriginalOutputSHA256 = digest(result.Output)
		// Preserve accepted original bytes exactly. Oversized/invalid output is
		// rejected with its SHA; never truncate it into an adoptable proposal.
		if len(result.Output) <= operationLimit && utf8.ValidString(result.Output) {
			a.OriginalOutput = result.Output
		}
	}
	if err = s.checkIntelligentOperationSourceTx(ctx, tx, tenant, project, a.Source); err != nil {
		var api *APIError
		if !errors.As(err, &api) {
			return err
		}
		a.BasisCurrent = false
	}
	switch {
	case result.OutcomeUnknown:
		a.State, a.Reason = "reconciling", operationUnknown
	case result.Stopped || result.ExitCode != 0 || result.Error != "":
		a.State, a.Reason = "failed", operationFailed
	default:
		proof := result.ToolEvidence
		if proof == nil || proof.Terminal != "completed" || proof.Profile.ProfileSHA256 != a.ToolProfileSHA256 || !intelligentOperationProfile(ToolProfile{State: "enabled", Profile: proof.Profile}) {
			a.State, a.Reason = "failed", operationFailed
			break
		}
		proposal, e := parseIntelligentOperationOutput(result.Output, a.Source)
		if e != nil {
			a.State, a.Reason = "failed", operationOutput
			break
		}
		a.State, a.Reason, a.Proposal = "ready", "", &proposal
		if !a.BasisCurrent {
			a.Reason = operationSource
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_intelligent_operation_executions(tenant_id,project_id,analysis_id,attempt,fence,result_sha256,created_at,result)VALUES(?,?,?,?,?,?,?,?)`), tenant, project, a.ID, attempt, fence, sha, a.UpdatedAt, stringify(result)); err != nil {
		return err
	}
	record.Analysis = a
	return s.saveIntelligentOperationTx(ctx, tx, tenant, record, old)
}
