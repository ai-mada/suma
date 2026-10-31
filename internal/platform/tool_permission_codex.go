package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// This is the same machine authority used for Open/Poll, including the real
// Pool proof. Its existing receipt adapter does not create transfer authority.
func (s *Service) toolPermissionMachineAuthorityTx(ctx context.Context, tx *sql.Tx, actor Actor, token, project string, a dag.Assignment) (*candidateAuthority, error) {
	proof := &candidateAuthority{tx: tx, assignment: codeInputAssignmentRef(a), subject: projectCandidateSubject(actor, project)}
	var err error
	if token != "" {
		if a.PoolID == "" {
			return nil, forbidden()
		}
		proof.subject = candidateSubject{pool: true, token: token, tenant: a.TenantID, project: project}
		proof.machine, err = s.PoolRunnerAssignmentAuthorityTx(ctx, tx, token, candidateDAGAssignment(proof.assignment))
	} else {
		if a.PoolID != "" || actor.kind != actorProjectRunner || actor.credential.tenantID != a.TenantID {
			return nil, forbidden()
		}
		proof.human, err = s.runnerIdentityTx(ctx, tx, actor, project, a.WorkerID)
		if err == nil {
			err = s.projectRunnerExecutionAuthorityTx(ctx, tx, proof.human, a.WorkerID, a.RunID)
		}
	}
	if err != nil {
		return nil, err
	}
	if err = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, a.TenantID, project, a.RunID); err != nil {
		return nil, err
	}
	return proof, nil
}

func toolPermissionObservationMonotonic(previous *cc.CodexApprovalObservation, next cc.CodexApprovalObservation) bool {
	if cc.ValidateApprovalObservation(next) != nil {
		return false
	}
	if previous == nil {
		return true
	}
	if next.Sequence <= previous.Sequence || next.ParamsSHA256 != previous.ParamsSHA256 || previous.DecisionObserved && !next.DecisionObserved || previous.Decision != "" && previous.Decision != next.Decision || previous.ReplyStarted && !next.ReplyStarted || previous.ReplyWritten && !next.ReplyWritten || previous.RequestResolved && !next.RequestResolved || previous.FailureCode != "" && previous.FailureCode != next.FailureCode {
		return false
	}
	if previous.ItemStatus != "" && previous.ItemStatus != "inProgress" && next.ItemStatus != previous.ItemStatus || previous.ItemStatus == "inProgress" && next.ItemStatus == "" {
		return false
	}
	return true
}

func (s *Service) ObserveToolPermission(ctx context.Context, actor Actor, project string, a dag.Assignment, id, key string, observation cc.CodexApprovalObservation) (pc.Permission, error) {
	return s.observeToolPermission(ctx, actor, "", project, a, id, key, observation)
}

func (s *Service) ObservePoolToolPermission(ctx context.Context, token, project string, a dag.Assignment, id, key string, observation cc.CodexApprovalObservation) (pc.Permission, error) {
	return s.observeToolPermission(ctx, Actor{}, token, project, a, id, key, observation)
}

func (s *Service) observeToolPermission(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id, key string, observation cc.CodexApprovalObservation) (pc.Permission, error) {
	var empty pc.Permission
	if !handoffID(project, false) || project != a.ProjectID || !handoffID(a.TenantID, false) || !handoffID(id, false) || !planningKey(key) || cc.ValidateApprovalObservation(observation) != nil {
		return empty, invalid(toolPermissionInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return empty, err
	}
	defer cancel()
	defer tx.Rollback()
	authority, err := s.toolPermissionMachineAuthorityTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return empty, err
	}
	basis, err := s.toolPermissionBasisTx(ctx, tx, a.TenantID, project, a)
	if err != nil {
		return empty, err
	}
	if err = s.toolPermissionDispatchTx(ctx, tx, a.TenantID, project, a); err != nil {
		return empty, err
	}
	if err = s.toolPermissionCurrentProfileTx(ctx, tx, a.TenantID, project, basis, a); err != nil {
		return empty, err
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return empty, err
	}
	if basis.state != "running" || basis.runState != "running" && basis.runState != "paused" || basis.attempt != a.Attempt || basis.fence != a.Fence || basis.worker != a.WorkerID || basis.lease <= now.Unix() || basis.deadline <= now.Unix() {
		return empty, conflict(toolPermissionChanged)
	}
	row, err := s.toolPermissionReadTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return empty, err
	}
	if !toolPermissionSameSource(row.value, a) || row.profileID != basis.profile.ID || row.profileVersion != basis.profile.Version || !tc.EqualProfile(row.profile, basis.profile.Profile) || !toolPermissionRequestMatchesBasis(row.value.Request, basis) || row.value.Request.Protocol != cc.Protocol {
		return empty, conflict(toolPermissionChanged)
	}
	body := struct {
		Kind, PermissionID string
		Assignment         dag.Assignment
		Observation        cc.CodexApprovalObservation
	}{"tool_permission_observation", id, a, observation}
	hash, replay, err := s.candidateReceipt(ctx, authority, key, body)
	if err != nil {
		return empty, err
	}
	if replay {
		var original pc.Permission
		if json.Unmarshal([]byte(hash), &original) != nil || original.ID != id || !toolPermissionSameSource(original, a) || original.Observation == nil || stringify(*original.Observation) != stringify(observation) {
			return empty, conflict(toolPermissionChanged)
		}
		return original, nil
	}
	if observation.ParamsSHA256 != row.value.Request.Codex.ParamsSHA256 || !toolPermissionObservationMonotonic(row.value.Observation, observation) || observation.DecisionObserved && (row.value.Decision == nil || row.value.Decision.Codex == nil || row.value.Decision.Codex.Decision != observation.Decision) {
		return empty, conflict(toolPermissionChanged)
	}
	before := row.value.Version
	row.value.Version++
	row.value.Observation = &observation
	if err = s.toolPermissionSaveTx(ctx, tx, row, before); err != nil {
		return empty, err
	}
	if err = s.saveCandidateReceipt(ctx, authority, key, hash, row.value); err != nil {
		return empty, err
	}
	if err = tx.Commit(); err != nil {
		return empty, err
	}
	return row.value, nil
}

func toolPermissionApprovalEvidenceMatches(request cc.CodexApprovalRequest, proof cc.CodexApprovalEvidence) bool {
	return cc.ValidateApprovalEvidence(proof) == nil && stringify(request.RequestID) == stringify(proof.RequestID) && request.Method == proof.Method && request.ThreadID == proof.ThreadID && request.TurnID == proof.TurnID && request.ItemID == proof.ItemID && request.ApprovalID == proof.ApprovalID && request.ParamsSHA256 == proof.ParamsSHA256
}

// A machine result cannot turn its own local flags into a platform approval
// fact. Original result replay is handled before this hook by accounting.
func (s *Service) validateCodexApprovalResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	var proofs []cc.CodexApprovalEvidence
	if result.ToolEvidence != nil && result.ToolEvidence.CodexExecution != nil {
		proofs = result.ToolEvidence.CodexExecution.Approvals
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? ORDER BY id LIMIT 33`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > 32 || len(proofs) > 32 {
		return invalid(toolPermissionInvalid)
	}
	stored := map[string]toolPermissionRow{}
	for _, id := range ids {
		row, e := s.toolPermissionReadTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		if row.value.Request.Protocol == cc.Protocol {
			stored[id] = row
		}
	}
	success := result.ExitCode == 0 && result.Error == "" && !result.Stopped && !result.OutcomeUnknown
	seen := map[string]bool{}
	for _, proof := range proofs {
		if cc.ValidateApprovalEvidence(proof) != nil {
			return invalid(toolPermissionInvalid)
		}
		if proof.PermissionID == "" {
			if success {
				return invalid(toolPermissionInvalid)
			}
			continue // explicitly unregistered local failure diagnosis, never a grant
		}
		row, ok := stored[proof.PermissionID]
		if !ok || seen[proof.PermissionID] || !toolPermissionApprovalEvidenceMatches(*row.value.Request.Codex, proof) || stringify(row.value.Observation) != stringify(proof.Observation) || result.ToolEvidence == nil || result.ToolEvidence.CodexExecution == nil || result.ToolEvidence.CodexExecution.BindingSHA256 != row.value.Request.Codex.BindingSHA256 || !tc.EqualProfile(row.profile, result.ToolEvidence.Profile) {
			return invalid(toolPermissionInvalid)
		}
		configuration := result.ToolEvidence.CodexExecution
		if configuration.ThreadID != "" && configuration.ThreadID != proof.ThreadID || configuration.TurnID != "" && configuration.TurnID != proof.TurnID {
			return invalid(toolPermissionInvalid)
		}
		seen[proof.PermissionID] = true
		if success && (!cc.ApprovalEvidenceClosed(proof) || row.value.Decision == nil || row.value.Decision.Codex == nil || row.value.Observation.Decision != row.value.Decision.Codex.Decision) {
			return invalid(toolPermissionInvalid)
		}
	}
	if success && len(seen) != len(stored) {
		return invalid(toolPermissionInvalid)
	}
	return nil
}
