package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// The existing machine observation endpoint dispatches to this branch only for
// the exact Claude SDK source. The original Codex wire and receipt are separate.
func (s *Service) ObserveClaudeToolPermission(ctx context.Context, actor Actor, project string, a dag.Assignment, id, key string, observation cl.PermissionObservation) (pc.Permission, error) {
	return s.observeClaudeToolPermission(ctx, actor, "", project, a, id, key, observation)
}

// A local callback delivery cannot create its own platform approval. Success
// requires every original persisted source and its exact decided write facts.
func (s *Service) validateClaudePermissionResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	var proofs []cl.PermissionEvidence
	if result.ToolEvidence != nil && result.ToolEvidence.ClaudeSDKExecution != nil {
		proofs = result.ToolEvidence.ClaudeSDKExecution.Permissions
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
	if len(ids) > cl.MaxCallbacks || len(proofs) > cl.MaxCallbacks {
		return invalid(toolPermissionInvalid)
	}
	stored := map[string]toolPermissionRow{}
	for _, id := range ids {
		row, e := s.toolPermissionReadTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		if row.value.Request.Claude != nil {
			stored[id] = row
		}
	}
	success := result.ExitCode == 0 && result.Error == "" && !result.Stopped && !result.OutcomeUnknown
	seen := map[string]bool{}
	for _, proof := range proofs {
		if cl.ValidateBasis(proof.Basis) != nil || proof.Observation != nil && cl.ValidatePermissionObservation(*proof.Observation) != nil {
			return invalid(toolPermissionInvalid)
		}
		if proof.PermissionID == "" {
			if success {
				return invalid(toolPermissionInvalid)
			}
			continue
		}
		row, ok := stored[proof.PermissionID]
		if !ok || seen[proof.PermissionID] || proof.Basis != row.value.Request.Claude.Basis || proof.ToolName != row.value.Request.Claude.ToolName || stringify(proof.Observation) != stringify(row.value.ClaudeObservation) || result.ToolEvidence == nil || result.ToolEvidence.ClaudeSDKExecution == nil || !tc.EqualProfile(row.profile, result.ToolEvidence.Profile) {
			return invalid(toolPermissionInvalid)
		}
		configuration := result.ToolEvidence.ClaudeSDKExecution
		if configuration.ConfigurationSHA256 != proof.ConfigurationSHA256 || configuration.SessionID != proof.SessionID {
			return invalid(toolPermissionInvalid)
		}
		seen[proof.PermissionID] = true
		if success && (row.value.Decision == nil || row.value.Decision.Claude == nil || row.value.Decision.Claude.Outcome != "allow_once" || proof.Observation == nil || !proof.Observation.AnswerObserved || !proof.Observation.ReplyWritten || !proof.Observation.ExecutionClosed || proof.Observation.FailureCode != "") {
			return invalid(toolPermissionInvalid)
		}
	}
	if success && len(seen) != len(stored) {
		return invalid(toolPermissionInvalid)
	}
	return nil
}

func (s *Service) ObservePoolClaudeToolPermission(ctx context.Context, token, project string, a dag.Assignment, id, key string, observation cl.PermissionObservation) (pc.Permission, error) {
	return s.observeClaudeToolPermission(ctx, Actor{}, token, project, a, id, key, observation)
}

func (s *Service) observeClaudeToolPermission(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id, key string, observation cl.PermissionObservation) (pc.Permission, error) {
	var empty pc.Permission
	if !handoffID(project, false) || project != a.ProjectID || !handoffID(a.TenantID, false) || !handoffID(id, false) || !planningKey(key) || cl.ValidatePermissionObservation(observation) != nil {
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
	row, err := s.toolPermissionReadTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return empty, err
	}
	if !toolPermissionSameSource(row.value, a) || row.profileID != basis.profile.ID || row.profileVersion != basis.profile.Version || !tc.EqualProfile(row.profile, basis.profile.Profile) || !toolPermissionRequestMatchesBasis(row.value.Request, basis) || row.value.Request.Claude == nil {
		return empty, conflict(toolPermissionChanged)
	}
	body := struct {
		Kind, PermissionID string
		Assignment         dag.Assignment
		Observation        cl.PermissionObservation
	}{"tool_permission_claude_observation", id, a, observation}
	hash, replay, err := s.candidateReceipt(ctx, authority, key, body)
	if err != nil {
		return empty, err
	}
	if replay {
		var original pc.Permission
		if json.Unmarshal([]byte(hash), &original) != nil || original.ID != id || !toolPermissionSameSource(original, a) || original.ClaudeObservation == nil || stringify(*original.ClaudeObservation) != stringify(observation) {
			return empty, conflict(toolPermissionChanged)
		}
		return original, nil
	}
	// An existing persisted receipt acknowledges only the original write. A new
	// callback observation still requires the live attempt and current authority.
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return empty, err
	}
	if basis.state != "running" || basis.runState != "running" && basis.runState != "paused" || basis.attempt != a.Attempt || basis.fence != a.Fence || basis.worker != a.WorkerID || basis.lease <= now.Unix() || basis.deadline <= now.Unix() {
		return empty, conflict(toolPermissionChanged)
	}
	if observation.ParamsSHA256 != row.value.Request.Claude.ParamsSHA256 || !toolInputObservationAdvances(row.value.ClaudeObservation, observation) {
		return empty, conflict(toolPermissionChanged)
	}
	if observation.AnswerObserved {
		if row.value.Decision == nil || row.value.Decision.Claude == nil {
			return empty, conflict(toolPermissionChanged)
		}
		digest, e := cl.PermissionDecisionDigest(*row.value.Request.Claude, *row.value.Decision.Claude)
		if e != nil || digest != observation.AnswerSHA256 {
			return empty, conflict(toolPermissionChanged)
		}
	}
	if observation.ReplyStarted && (row.value.ClaudeObservation == nil || !row.value.ClaudeObservation.ReplyStarted) {
		if toolPermissionHasEnded(row.value) || toolPermissionEffective(row, basis, now).State != "decided" || row.value.Decision == nil {
			return empty, conflict(toolPermissionChanged)
		}
		if pc.IsGrant(row.value.Decision.Choice) {
			allowed, e := s.toolPermissionWorkAllowedTx(ctx, tx, row.tenant, project, basis)
			if e != nil {
				return empty, e
			}
			if !allowed {
				return empty, forbidden()
			}
		}
	}
	before := row.value.Version
	row.value.Version++
	row.value.ClaudeObservation = &observation
	if err = s.toolPermissionSaveTx(ctx, tx, row, before); err != nil {
		return empty, err
	}
	if err = s.saveCandidateReceipt(ctx, authority, key, hash, row.value); err != nil {
		return empty, err
	}
	return row.value, tx.Commit()
}
