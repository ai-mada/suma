package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func steeringObservationAdvances(old *cc.CodexSteeringObservation, next cc.CodexSteeringObservation) bool {
	if cc.ValidateSteeringObservation(next) != nil {
		return false
	}
	if old == nil {
		return true
	}
	return next.Sequence > old.Sequence && next.RequestID == old.RequestID && next.ParamsSHA256 == old.ParamsSHA256 && (!old.Observed || next.Observed) && (!old.WriteStarted || next.WriteStarted) && (!old.WriteWritten || next.WriteWritten) && (!old.NativeAccepted || next.NativeAccepted) && (!old.NativeRejected || next.NativeRejected) && (!old.TurnClosed || next.TurnClosed) && (old.FailureCode == "" || old.FailureCode == next.FailureCode) && (!old.TurnClosed || old.WriteStarted || !next.WriteStarted)
}
func (s *Service) ObserveSteering(ctx context.Context, actor Actor, project string, a dag.Assignment, session, id, key string, o cc.CodexSteeringObservation) (sc.SteeringInstruction, error) {
	return s.observeSteering(ctx, actor, "", project, a, session, id, key, o)
}
func (s *Service) ObservePoolSteering(ctx context.Context, token, project string, a dag.Assignment, session, id, key string, o cc.CodexSteeringObservation) (sc.SteeringInstruction, error) {
	return s.observeSteering(ctx, Actor{}, token, project, a, session, id, key, o)
}
func (s *Service) observeSteering(ctx context.Context, actor Actor, token, project string, a dag.Assignment, session, id, key string, o cc.CodexSteeringObservation) (out sc.SteeringInstruction, err error) {
	if !steeringAssignmentValid(project, a) || !handoffID(session, false) || !handoffID(id, false) || !planningKey(key) || cc.ValidateSteeringObservation(o) != nil {
		return out, invalid(steeringInvalid)
	}
	located, err := s.steeringSessionTx(ctx, s.DB, a.TenantID, project, session)
	if err != nil {
		return out, err
	}
	caller, err := s.steeringInstructionTx(ctx, s.DB, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if !steeringSameAssignment(located.Session, a) || caller.Instruction.SessionID != session {
		return out, conflict(steeringChanged)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.steeringLockCreatorsTx(ctx, tx, actor, located, []steeringInstructionRecord{caller}); err != nil {
		return out, err
	}
	proof, b, err := s.steeringMachineBasisTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return out, err
	}
	r, err := s.steeringSessionTx(ctx, tx, a.TenantID, project, session)
	if err != nil {
		return out, err
	}
	row, err := s.steeringInstructionTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if !steeringSameAssignment(r.Session, a) || !steeringSourceMatches(r.Session, b) || row.Instruction.SessionID != session || row.Instruction.RunID != a.RunID || row.Instruction.NodeID != a.NodeID || row.Instruction.Attempt != a.Attempt || row.Instruction.Fence != a.Fence || row.Caller != caller.Caller {
		return out, conflict(steeringChanged)
	}
	hash, replay, err := s.candidateReceipt(ctx, proof, key, struct {
		Kind, Session, Instruction string
		Assignment                 dag.Assignment
		Observation                cc.CodexSteeringObservation
	}{"steering_observation", session, id, a, o})
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(hash), &out) != nil || sc.ValidateInstruction(out) != nil || out.ID != id || out.SessionID != session || out.Observation == nil || *out.Observation != o {
			return sc.SteeringInstruction{}, conflict(steeringChanged)
		}
		return out, nil
	}
	paramsSHA, err := cc.SteeringParamsDigest(r.Session.Source, id, row.Instruction.Message.Body)
	if err != nil || paramsSHA != o.ParamsSHA256 || !steeringObservationAdvances(row.Instruction.Observation, o) {
		return out, conflict(steeringChanged)
	}
	started := row.Instruction.Observation != nil && row.Instruction.Observation.WriteStarted
	if !started {
		if r.Session.State != "active" && o.WriteStarted {
			return out, conflict(steeringChanged)
		}
		if o.WriteStarted || !o.TurnClosed {
			now, e := s.authorityNow(ctx, tx)
			if e != nil {
				return out, e
			}
			if err = s.steeringInstructionFreshTx(ctx, tx, a.TenantID, project, r, b, row, now); err != nil {
				return out, err
			}
		}
	}
	// A session cannot reuse one host RPC ID for another steer. Client-origin
	// IDs are independent of native server approval/question request IDs.
	others, err := s.steeringInstructionsTx(ctx, tx, a.TenantID, project, session)
	if err != nil {
		return out, err
	}
	for _, other := range others {
		if other.Instruction.ID != id && other.Instruction.Observation != nil && other.Instruction.Observation.RequestID == o.RequestID {
			return out, conflict(steeringChanged)
		}
	}
	before := row.Instruction.Version
	row.Instruction.Version++
	row.Instruction.Observation = &o
	if err = s.saveSteeringInstructionTx(ctx, tx, a.TenantID, row, before); err != nil {
		return out, err
	}
	if err = s.saveCandidateReceipt(ctx, proof, key, hash, row.Instruction); err != nil {
		return out, err
	}
	return row.Instruction, tx.Commit()
}

func (s *Service) CloseSteeringSession(ctx context.Context, actor Actor, project string, a dag.Assignment, id, key string) (sc.SteeringSession, error) {
	return s.closeSteeringSession(ctx, actor, "", project, a, id, key)
}
func (s *Service) ClosePoolSteeringSession(ctx context.Context, token, project string, a dag.Assignment, id, key string) (sc.SteeringSession, error) {
	return s.closeSteeringSession(ctx, Actor{}, token, project, a, id, key)
}
func (s *Service) closeSteeringSession(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id, key string) (out sc.SteeringSession, err error) {
	if !steeringAssignmentValid(project, a) || !handoffID(id, false) || !planningKey(key) {
		return out, invalid(steeringInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	proof, b, err := s.steeringMachineBasisTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return out, err
	}
	r, err := s.steeringSessionTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if !steeringSameAssignment(r.Session, a) || !steeringSourceMatches(r.Session, b) {
		return out, conflict(steeringChanged)
	}
	hash, replay, err := s.candidateReceipt(ctx, proof, key, struct {
		Kind, Session string
		Assignment    dag.Assignment
	}{"steering_close", id, a})
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(hash), &out) != nil || sc.ValidateSession(out) != nil || out.ID != id || !steeringSameAssignment(out, a) || out.State != "closed" {
			return sc.SteeringSession{}, conflict(steeringChanged)
		}
		return out, nil
	}
	if r.Session.State != "closed" {
		before := r.Session.Version
		r.Session.Version++
		r.Session.State = "closed"
		if err = s.saveSteeringSessionTx(ctx, tx, a.TenantID, r, before); err != nil {
			return out, err
		}
	}
	if err = s.saveCandidateReceipt(ctx, proof, key, hash, r.Session); err != nil {
		return out, err
	}
	return r.Session, tx.Commit()
}

func (s *Service) validateCodexSteeringResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	var proof *cc.CodexSteeringEvidence
	if result.ToolEvidence != nil && result.ToolEvidence.CodexExecution != nil {
		proof = result.ToolEvidence.CodexExecution.Steering
	}
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_steering_sessions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID).Scan(&id)
	if err == sql.ErrNoRows {
		if proof != nil {
			return invalid(steeringInvalid)
		}
		return nil
	}
	if err != nil {
		return err
	}
	r, err := s.steeringSessionTx(ctx, tx, tenant, project, id)
	if err != nil {
		return err
	}
	rows, err := s.steeringInstructionsTx(ctx, tx, tenant, project, id)
	if err != nil {
		return err
	}
	success := result.ExitCode == 0 && result.Error == "" && !result.Stopped && !result.OutcomeUnknown
	if proof == nil {
		if success {
			return invalid(steeringInvalid)
		}
		return nil
	}
	if cc.ValidateSteeringEvidence(*proof) != nil || proof.SessionID != id || proof.Source != r.Session.Source || proof.Closed != (r.Session.State == "closed") || !tc.EqualProfile(r.Profile, result.ToolEvidence.Profile) {
		return invalid(steeringInvalid)
	}
	configuration := result.ToolEvidence.CodexExecution
	if configuration.BindingSHA256 != proof.Source.BindingSHA256 || configuration.ThreadID != proof.Source.ThreadID || configuration.TurnID != proof.Source.TurnID {
		return invalid(steeringInvalid)
	}
	known := map[string]sc.SteeringInstruction{}
	for _, row := range rows {
		known[row.Instruction.ID] = row.Instruction
	}
	for _, item := range proof.Instructions {
		v, ok := known[item.InstructionID]
		if !ok || v.Message.BodySHA256 != item.MessageSHA256 || stringify(v.Observation) != stringify(item.Observation) {
			return invalid(steeringInvalid)
		}
		delete(known, item.InstructionID)
		if success && item.Observation != nil {
			o := item.Observation
			if o.WriteStarted && (!o.WriteWritten || !o.NativeAccepted && !o.NativeRejected || o.FailureCode != "" && !o.NativeRejected) {
				return invalid(steeringInvalid)
			}
		}
	}
	if success && (!proof.Closed || len(known) != 0) {
		return invalid(steeringInvalid)
	}
	return nil
}
