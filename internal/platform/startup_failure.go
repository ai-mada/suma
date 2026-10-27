package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ec "github.com/ai-mada/suma/internal/executioncontract"
)

const startupFailureInvalid = "Host startup failure proof does not match the fixed failed repair attempt"

func (s *Service) validateStartupFailureResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	if result.StartupFailure == nil {
		return nil
	}
	_, err := s.CertifiedCandidateRepairStartupFailureTx(ctx, tx, tenant, project, result)
	return err
}

// Certification reads fixed execution/assignment/profile facts only. A newer
// plan, disabled profile or revoked creator cannot erase a genuine old fact.
func (s *Service) CertifiedCandidateRepairStartupFailureTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) (bool, error) {
	proof := result.StartupFailure
	if proof == nil {
		return false, nil
	}
	invalid := func() (bool, error) { return false, invalid(startupFailureInvalid) }
	if result.ExitCode == 0 || result.Error == "" || result.Stopped || result.OutcomeUnknown || result.ToolEvidence != nil || result.WorkspaceEvidence != nil || result.ArtifactEvidence != nil || result.ReleaseEvidence != nil || result.IntegrationEvidence != nil {
		return invalid()
	}
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, result.RunID)
	if err != nil {
		return false, err
	}
	if !found || r.RepairExecution == nil || r.Plan.ProducerNodeID != result.NodeID || r.Plan.RunID != result.RunID || !validCandidateCompositionExecutionRecord(r) {
		return invalid()
	}
	node, profile, err := s.toolBoundNodeTx(ctx, tx, Principal{TenantID: tenant}, project, result.RunID, result.NodeID)
	if err != nil {
		return false, err
	}
	profileID, sha, bound, err := dag.ToolBinding(node)
	if err != nil {
		return false, err
	}
	if !bound || profileID != r.Plan.ToolProfileID || sha != r.Plan.ToolProfileSHA256 || profile.ID != profileID || profile.WorkerID != result.WorkerID || profile.Profile.ProfileSHA256 != sha || !candidateConflictRepairProfile(profile.Profile) || node.Type != "agent" || node.Config["purpose"] != "candidate_conflict_repair" || node.Config["candidate_from"] != nil || node.Config["candidate_composition"] != nil || node.Config["code_baseline"] != nil || len(node.Command) != 1 || node.Command[0] != profile.Profile.CommandID {
		return invalid()
	}
	assignment := cc.AssignmentRef{RunID: result.RunID, NodeID: result.NodeID, Attempt: result.Attempt, Fence: result.Fence, WorkerID: result.WorkerID}
	if r.Plan.ExecutionRoute.PoolID != "" {
		var raw string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT assignment FROM dag_execution_pool_dispatches WHERE pool_id=? AND runner_id=? AND dispatch_id=?`), r.Plan.ExecutionRoute.PoolID, proof.Assignment.PoolRunnerID, proof.Assignment.DispatchID).Scan(&raw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return invalid()
			}
			return false, err
		}
		var stored dag.Assignment
		if json.Unmarshal([]byte(raw), &stored) != nil || stored.TenantID != tenant || stored.ProjectID != project || stored.RunID != result.RunID || stored.NodeID != result.NodeID || stored.Attempt != result.Attempt || stored.Fence != result.Fence || stored.WorkerID != result.WorkerID || stored.PoolID != r.Plan.ExecutionRoute.PoolID || stored.PoolGrantID != r.Plan.ExecutionRoute.GrantID || stored.PoolGrantVersion != r.Plan.ExecutionRoute.GrantVersion {
			return invalid()
		}
		assignment = cc.AssignmentRef{RunID: stored.RunID, NodeID: stored.NodeID, Attempt: stored.Attempt, Fence: stored.Fence, WorkerID: stored.WorkerID, TenantID: stored.TenantID, ProjectID: stored.ProjectID, PoolID: stored.PoolID, PoolRunnerID: stored.PoolRunnerID, PoolSlotID: stored.PoolSlotID, PoolGrantID: stored.PoolGrantID, PoolGrantVersion: stored.PoolGrantVersion, PoolCredentialID: stored.PoolCredentialID, PoolCredentialVersion: stored.PoolCredentialVersion, DispatchID: stored.DispatchID, DispatchState: stored.DispatchState, LeaseUntil: stored.LeaseUntil}
	}
	if ec.ValidateStartupFailureProof(*proof, assignment, profile.Profile, "candidate_conflict_repair") != nil {
		return invalid()
	}
	var fence int64
	var worker string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT fence,worker_id FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`), result.RunID, result.NodeID, result.Attempt).Scan(&fence, &worker); err != nil {
		return false, err
	}
	if fence != result.Fence || worker != result.WorkerID {
		return invalid()
	}
	var effects int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence).Scan(&effects); err != nil {
		return false, err
	}
	if effects != 0 {
		return invalid()
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT data FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND event_type='tool_progress' ORDER BY sequence DESC LIMIT 4097`), tenant, project, result.RunID, result.NodeID)
	if err != nil {
		return false, err
	}
	seen := 0
	for rows.Next() {
		var raw string
		var event struct {
			Attempt  int    `json:"attempt"`
			Fence    int64  `json:"fence"`
			WorkerID string `json:"worker_id"`
		}
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return false, err
		}
		seen++
		if seen > 4096 || json.Unmarshal([]byte(raw), &event) != nil || event.Attempt == result.Attempt && event.Fence == result.Fence && event.WorkerID == result.WorkerID {
			rows.Close()
			return invalid()
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	rows.Close()
	var usageRaw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_usage_attempts WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence).Scan(&usageRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if err == nil {
		var usage usageRecord
		if json.Unmarshal([]byte(usageRaw), &usage) != nil {
			return invalid()
		}
		// A later assertion cannot refund an older evidence-free result.
		if usage.Attempt.SourceResultSHA256 != "" && usage.Attempt.SourceResultSHA256 != digest(stringify(result)) {
			return invalid()
		}
		if usage.Attempt.Usage != nil || usage.Attempt.UsageSource != "none" {
			if usage.Attempt.UsageSource != "host_startup_proof" || usage.Attempt.SourceResultSHA256 != digest(stringify(result)) || usage.Attempt.Usage == nil || usage.Attempt.Usage.InputTokens == nil || usage.Attempt.Usage.OutputTokens == nil || *usage.Attempt.Usage.InputTokens != 0 || *usage.Attempt.Usage.OutputTokens != 0 {
				return invalid()
			}
		}
	}
	return true, nil
}
