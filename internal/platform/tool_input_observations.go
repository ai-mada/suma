package platform

import (
	"context"
	"database/sql"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/dag"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func toolInputObservationAdvances(old *delivery.Observation, next delivery.Observation) bool {
	if delivery.Validate(next) != nil {
		return false
	}
	if old == nil {
		return true
	}
	return next.Sequence > old.Sequence && next.ParamsSHA256 == old.ParamsSHA256 && (!old.AnswerObserved || next.AnswerObserved) && (old.AnswerSHA256 == "" || old.AnswerSHA256 == next.AnswerSHA256) && (!old.ReplyStarted || next.ReplyStarted) && (!old.ReplyWritten || next.ReplyWritten) && (!old.RequestResolved || next.RequestResolved) && (!old.ExecutionClosed || next.ExecutionClosed) && (old.FailureCode == "" || old.FailureCode == next.FailureCode) && (!toolInputEnded(old) || old.ReplyStarted || !next.ReplyStarted)
}

func (s *Service) ObserveToolInput(ctx context.Context, actor Actor, project string, a dag.Assignment, id, key string, o delivery.Observation) (ic.ToolInput, error) {
	return s.observeToolInput(ctx, actor, "", project, a, id, key, o)
}
func (s *Service) ObservePoolToolInput(ctx context.Context, token, project string, a dag.Assignment, id, key string, o delivery.Observation) (ic.ToolInput, error) {
	return s.observeToolInput(ctx, Actor{}, token, project, a, id, key, o)
}
func (s *Service) observeToolInput(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id, key string, o delivery.Observation) (out ic.ToolInput, err error) {
	if !handoffID(project, false) || a.ProjectID != project || !handoffID(a.TenantID, false) || !handoffID(id, false) || !planningKey(key) || delivery.Validate(o) != nil {
		return out, invalid(toolInputInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	authority, b, err := s.toolInputBasisTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return out, err
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	r, err := s.toolInputReadTx(ctx, tx, a.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if !toolInputRowMatches(r, b, a) {
		return out, conflict(toolInputChanged)
	}
	hash, replay, err := s.candidateReceipt(ctx, authority, key, struct {
		Kind, InputID string
		Assignment    dag.Assignment
		Observation   any
	}{"tool_input_observation", id, a, toolInputNativeObservation(r.value.Request, &o)})
	if err != nil {
		return out, err
	}
	if replay {
		if decodeToolInputStorage(hash, &out) != nil || out.ID != id || !toolInputSameSource(out, a) || out.Observation == nil || stringify(*out.Observation) != stringify(o) {
			return ic.ToolInput{}, conflict(toolInputChanged)
		}
		return out, nil
	}
	// Current machine identity, scope and the exact original request remain
	// mandatory above. An immutable receipt acknowledges an existing operation;
	// only a new observation requires the execution to still be live.
	if b.state != "running" || b.runState != "running" && b.runState != "paused" || b.attempt != a.Attempt || b.fence != a.Fence || b.worker != a.WorkerID || b.lease <= now.Unix() || b.deadline <= now.Unix() {
		return out, conflict(toolInputChanged)
	}
	if o.ParamsSHA256 != ic.ParamsSHA256(r.value.Request) || r.value.Request.Cursor != nil && o.RequestResolved || !toolInputObservationAdvances(r.value.Observation, o) || o.AnswerObserved && (r.value.Answer == nil || r.value.Answer.AnswerSHA256 != o.AnswerSHA256) {
		return out, conflict(toolInputChanged)
	}
	// Native closure can beat the human answer. Once observed, a later packet
	// may report an already-started write, but cannot start a new one.
	if o.ReplyStarted && (r.value.Observation == nil || !r.value.Observation.ReplyStarted) {
		if toolInputEnded(r.value.Observation) || toolInputEffective(r, b, now).State != "answered" {
			return out, conflict(toolInputChanged)
		}
		allowed, e := s.toolPermissionWorkAllowedTx(ctx, tx, r.tenant, project, b)
		if e != nil {
			return out, e
		}
		if !allowed {
			return out, forbidden()
		}
	}
	before := r.value.Version
	r.value.Version++
	r.value.Observation = &o
	if toolInputEnded(&o) {
		r.value.State = "closed"
	}
	if err = s.toolInputSaveTx(ctx, tx, r, before); err != nil {
		return out, err
	}
	if err = s.saveCandidateReceipt(ctx, authority, key, hash, toolInputStorageValue(r.value)); err != nil {
		return out, err
	}
	return r.value, tx.Commit()
}

func toolInputEvidenceMatches(r ic.ToolInput, e cc.CodexUserInputEvidence) bool {
	s := r.Request.Codex
	var observation *delivery.Observation
	if e.Observation != nil {
		value := ic.FromCodexObservation(*e.Observation)
		observation = &value
	}
	return s != nil && cc.ValidateUserInputEvidence(e) == nil && e.InputID == r.ID && stringify(e.RequestID) == stringify(s.RequestID) && e.Method == s.Method && e.ThreadID == s.ThreadID && e.TurnID == s.TurnID && e.ItemID == s.ItemID && e.BindingSHA256 == s.BindingSHA256 && e.ParamsSHA256 == s.ParamsSHA256 && e.QuestionCount == len(s.Questions) && e.IsBlocking == s.IsBlocking && stringify(observation) == stringify(r.Observation)
}

func cursorToolInputEvidenceMatches(r ic.ToolInput, e cu.CursorInputEvidence) bool {
	s := r.Request.Cursor
	if s == nil || cu.ValidateEvidence(e) != nil {
		return false
	}
	kind, count := "plan", 0
	if s.Questions != nil {
		kind, count = "questions", len(s.Questions.Questions)
	}
	return e.InputID == r.ID && e.RequestID == s.RequestID && e.PromptID == s.PromptID && e.SessionID == s.SessionID && e.Method == s.Method && e.ConfigurationSHA256 == s.ConfigurationSHA256 && e.ParamsSHA256 == s.ParamsSHA256 && e.Kind == kind && e.QuestionCount == count && stringify(e.Observation) == stringify(r.Observation)
}

func (s *Service) validateToolInputResultTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	var proofs []cc.CodexUserInputEvidence
	var cursorProofs []cu.CursorInputEvidence
	if result.ToolEvidence != nil && result.ToolEvidence.CodexExecution != nil {
		proofs = result.ToolEvidence.CodexExecution.UserInputs
	}
	if result.ToolEvidence != nil && result.ToolEvidence.ACPExecution != nil {
		cursorProofs = result.ToolEvidence.ACPExecution.UserInputs
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inputs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=? ORDER BY id LIMIT 33`), tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence, result.WorkerID)
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
	if len(ids) > 32 || len(proofs)+len(cursorProofs) > 32 {
		return invalid(toolInputInvalid)
	}
	stored := map[string]toolInputRow{}
	for _, id := range ids {
		r, e := s.toolInputReadTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		stored[id] = r
	}
	success := result.ExitCode == 0 && result.Error == "" && !result.Stopped && !result.OutcomeUnknown
	seen := map[string]bool{}
	for _, e := range proofs {
		if cc.ValidateUserInputEvidence(e) != nil {
			return invalid(toolInputInvalid)
		}
		if e.InputID == "" {
			if success {
				return invalid(toolInputInvalid)
			}
			continue
		}
		r, ok := stored[e.InputID]
		if !ok || seen[e.InputID] || !toolInputEvidenceMatches(r.value, e) || result.ToolEvidence == nil || result.ToolEvidence.CodexExecution == nil || !tc.EqualProfile(r.profile, result.ToolEvidence.Profile) {
			return invalid(toolInputInvalid)
		}
		configuration := result.ToolEvidence.CodexExecution
		if configuration.BindingSHA256 != e.BindingSHA256 || configuration.ThreadID != "" && configuration.ThreadID != e.ThreadID || configuration.TurnID != "" && configuration.TurnID != e.TurnID {
			return invalid(toolInputInvalid)
		}
		seen[e.InputID] = true
		o := r.value.Observation
		if success && (o == nil || !toolInputEnded(o) || o.ReplyStarted && (!o.ReplyWritten || o.FailureCode != "")) {
			return invalid(toolInputInvalid)
		}
	}
	for _, evidence := range cursorProofs {
		r, ok := stored[evidence.InputID]
		if !ok || seen[evidence.InputID] || !cursorToolInputEvidenceMatches(r.value, evidence) || result.ToolEvidence == nil || result.ToolEvidence.ACPExecution == nil || !tc.EqualProfile(r.profile, result.ToolEvidence.Profile) {
			return invalid(toolInputInvalid)
		}
		configuration := result.ToolEvidence.ACPExecution
		if configuration.ConfigurationSHA256 != evidence.ConfigurationSHA256 || configuration.SessionID != evidence.SessionID {
			return invalid(toolInputInvalid)
		}
		seen[evidence.InputID] = true
		o := r.value.Observation
		if success && (o == nil || !toolInputEnded(o) || o.ReplyStarted && (!o.ReplyWritten || o.FailureCode != "")) {
			return invalid(toolInputInvalid)
		}
	}
	if success && len(seen) != len(stored) {
		return invalid(toolInputInvalid)
	}
	return nil
}
