package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
)

func (s *Service) steeringMachineBasisTx(ctx context.Context, tx *sql.Tx, actor Actor, token, project string, a dag.Assignment) (*candidateAuthority, toolPermissionBasis, error) {
	proof, err := s.toolPermissionMachineAuthorityTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return nil, toolPermissionBasis{}, err
	}
	b, err := s.toolPermissionBasisTx(ctx, tx, a.TenantID, project, a)
	if err != nil {
		return nil, b, err
	}
	binding, e := dag.CodexBinding(b.node)
	if e != nil || binding == nil || b.profile.Profile.Protocol != cc.Protocol || !supportsCodexExecution(b.profile) {
		return nil, b, conflict(steeringChanged)
	}
	if err = s.toolPermissionDispatchTx(ctx, tx, a.TenantID, project, a); err != nil {
		return nil, b, err
	}
	return proof, b, nil
}
func steeringAssignmentValid(project string, a dag.Assignment) bool {
	return handoffID(project, false) && a.ProjectID == project && handoffID(a.TenantID, false) && diagnosisID(a.RunID, 100) && diagnosisID(a.NodeID, 150) && a.Attempt > 0 && a.Fence > 0 && a.WorkerID != "" && a.DispatchID != ""
}

func (s *Service) OpenSteeringSession(ctx context.Context, actor Actor, project string, a dag.Assignment, source cc.CodexSteeringSource, key string) (sc.SteeringOpenResult, error) {
	return s.openSteeringSession(ctx, actor, "", project, a, source, key)
}
func (s *Service) OpenPoolSteeringSession(ctx context.Context, token, project string, a dag.Assignment, source cc.CodexSteeringSource, key string) (sc.SteeringOpenResult, error) {
	return s.openSteeringSession(ctx, Actor{}, token, project, a, source, key)
}
func (s *Service) openSteeringSession(ctx context.Context, actor Actor, token, project string, a dag.Assignment, source cc.CodexSteeringSource, key string) (out sc.SteeringOpenResult, err error) {
	if !steeringAssignmentValid(project, a) || cc.ValidateSteeringSource(source) != nil || !planningKey(key) {
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
	binding, _ := dag.CodexBinding(b.node)
	if source.BindingSHA256 != binding.BindingSHA256 {
		return out, conflict(steeringChanged)
	}
	body := struct {
		Kind       string
		Assignment dag.Assignment
		Source     cc.CodexSteeringSource
	}{"steering_open", a, source}
	hash, replay, err := s.candidateReceipt(ctx, proof, key, body)
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(hash), &out) != nil || out.Session == nil && out.Reason != "not_task_execution" || out.Session != nil && (!steeringSameAssignment(*out.Session, a) || out.Session.Source != source) {
			return sc.SteeringOpenResult{}, conflict(steeringChanged)
		}
		return out, nil
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	if b.state != "running" || b.runState != "running" || b.attempt != a.Attempt || b.fence != a.Fence || b.worker != a.WorkerID || b.lease <= now.Unix() || b.deadline <= now.Unix() {
		return out, conflict(steeringChanged)
	}
	var task string
	err = tx.QueryRowContext(ctx, s.q(`SELECT t.id FROM suma_tasks t JOIN suma_task_dispatch_plans p ON p.tenant_id=t.tenant_id AND p.project_id=t.project_id AND p.run_id=t.run_id AND p.state='started' WHERE t.tenant_id=? AND t.project_id=? AND t.run_id=?`), a.TenantID, project, a.RunID).Scan(&task)
	if errors.Is(err, sql.ErrNoRows) {
		out.Reason = "not_task_execution"
		if err = s.saveCandidateReceipt(ctx, proof, key, hash, out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if err != nil {
		return out, err
	}
	var planRaw string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?`), a.TenantID, project, a.RunID).Scan(&planRaw); err != nil {
		return out, err
	}
	var plan TaskDispatchPlan
	if json.Unmarshal([]byte(planRaw), &plan) != nil || plan.TaskID != task || plan.RunID != a.RunID {
		return out, conflict(steeringChanged)
	}
	if plan.AgentNodeID != a.NodeID {
		out.Reason = "not_task_execution"
		if err = s.saveCandidateReceipt(ctx, proof, key, hash, out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	v := sc.SteeringSession{ID: newID(), ProjectID: project, TaskID: task, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, ProfileSHA256: b.profile.Profile.ProfileSHA256, Source: source, Version: 1, State: "active", OpenedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: time.Unix(b.deadline, 0).UTC().Format(time.RFC3339Nano), AllowedActions: []string{}}
	r := steeringSessionRecord{Session: v, Assignment: a, ProfileID: b.profile.ID, ProfileVersion: b.profile.Version, Profile: b.profile.Profile, OperationID: key, RequestHash: digest(stringify(body))}
	if err = s.steeringFreshTx(ctx, tx, a.TenantID, project, r, b, now); err != nil {
		return out, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_steering_sessions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), a.TenantID, project, a.RunID, a.NodeID, a.Attempt, a.Fence).Scan(&count); err != nil {
		return out, err
	}
	if count != 0 {
		return out, conflict(steeringChanged)
	}
	if err = s.saveSteeringSessionTx(ctx, tx, a.TenantID, r, 0); err != nil {
		return out, err
	}
	out.Session = &v
	if err = s.saveCandidateReceipt(ctx, proof, key, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) steeringMessageTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, task, id string) (sc.SteeringMessage, string, error) {
	m, err := scanTaskConversation(tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, project, task, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sc.SteeringMessage{}, "", missing()
	}
	if err != nil {
		return sc.SteeringMessage{}, "", err
	}
	v := sc.SteeringMessage{ID: m.ID, Sequence: m.Sequence, AuthorID: m.AuthorID, Body: m.Body, BodySHA256: sc.MessageDigest(m.Body)}
	if m.ActorKind != "human" || sc.ValidateMessage(v) != nil {
		return sc.SteeringMessage{}, "", invalid(steeringInvalid)
	}
	return v, m.RunID, nil
}

func (s *Service) CreateSteeringInstruction(ctx context.Context, actor Actor, request sc.SteeringCreateRequest) (out sc.SteeringInstruction, err error) {
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	if !handoffID(request.ProjectID, false) || !handoffID(request.TaskID, false) || !handoffID(request.MessageID, false) || !handoffID(request.SessionID, false) || request.ExpectedTaskVersion < 1 || request.ExpectedTaskVersion > 2147483646 || request.ExpectedSessionVersion < 1 || request.ExpectedSessionVersion >= cc.MaxSafeInteger || !planningKey(request.IdempotencyKey) {
		return out, invalid(steeringInvalid)
	}
	project := request.ProjectID
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, request.TaskID, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, struct {
		Kind    string
		Request sc.SteeringCreateRequest
	}{"steering_create", request})
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(hash), &out) != nil || sc.ValidateInstruction(out) != nil || out.TaskID != request.TaskID || out.ProjectID != project || out.SessionID != request.SessionID || out.Message.ID != request.MessageID || out.ActorID != auth.currentPrincipal.ID {
			return sc.SteeringInstruction{}, conflict(steeringChanged)
		}
		return out, nil
	}
	r, err := s.steeringSessionTx(ctx, auth.tx, auth.target.tenantID, project, request.SessionID)
	if err != nil {
		return out, err
	}
	if r.Session.TaskID != request.TaskID || r.Session.Version != request.ExpectedSessionVersion {
		return out, conflict(steeringChanged)
	}
	if err = dag.GuardExecutionScopeTx(ctx, auth.tx, s.Dialect, auth.target.tenantID, project, r.Session.RunID); err != nil {
		return out, err
	}
	r, err = s.steeringSessionTx(ctx, auth.tx, auth.target.tenantID, project, request.SessionID)
	if err != nil {
		return out, err
	}
	if r.Session.TaskID != request.TaskID || r.Session.Version != request.ExpectedSessionVersion {
		return out, conflict(steeringChanged)
	}
	message, _, err := s.steeringMessageTx(ctx, auth.tx, auth, project, request.TaskID, request.MessageID)
	if err != nil {
		return out, err
	}
	if message.AuthorID != auth.currentPrincipal.ID {
		return out, forbidden()
	}
	b, err := s.toolPermissionBasisTx(ctx, auth.tx, auth.target.tenantID, project, r.Assignment)
	if err != nil {
		return out, err
	}
	now, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	if err = s.steeringFreshTx(ctx, auth.tx, auth.target.tenantID, project, r, b, now); err != nil {
		return out, err
	}
	if err = s.steeringTaskTx(ctx, auth.tx, auth.target.tenantID, project, request.TaskID, r.Session.RunID, int(request.ExpectedTaskVersion)); err != nil {
		return out, err
	}
	var count int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_steering_instructions WHERE tenant_id=? AND project_id=? AND session_id=?`), auth.target.tenantID, project, r.Session.ID).Scan(&count); err != nil {
		return out, err
	}
	if count >= 32 {
		return out, conflict(steeringChanged)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_steering_instructions WHERE session_id=? AND message_id=?`), r.Session.ID, message.ID).Scan(&count); err != nil {
		return out, err
	}
	if count != 0 {
		return out, conflict(steeringChanged)
	}
	out = sc.SteeringInstruction{ID: newID(), SessionID: r.Session.ID, ProjectID: project, TaskID: request.TaskID, RunID: r.Session.RunID, NodeID: r.Session.NodeID, Attempt: r.Session.Attempt, Fence: r.Session.Fence, Version: 1, Message: message, ActorID: auth.currentPrincipal.ID, CreatedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: r.Session.ExpiresAt}
	if _, err = cc.BuildSteeringParams(r.Session.Source, out.ID, message.Body); err != nil {
		return sc.SteeringInstruction{}, invalid(steeringInvalid)
	}
	row := steeringInstructionRecord{Instruction: out, Caller: expertCredentialSnapshot(actor), TaskVersion: int(request.ExpectedTaskVersion)}
	if err = s.saveSteeringInstructionTx(ctx, auth.tx, auth.target.tenantID, row, 0); err != nil {
		return sc.SteeringInstruction{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return sc.SteeringInstruction{}, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) steeringInstructionFreshTx(ctx context.Context, tx *sql.Tx, tenant, project string, session steeringSessionRecord, b toolPermissionBasis, r steeringInstructionRecord, now time.Time) error {
	if err := s.steeringFreshTx(ctx, tx, tenant, project, session, b, now); err != nil {
		return err
	}
	if err := s.steeringTaskTx(ctx, tx, tenant, project, r.Instruction.TaskID, r.Instruction.RunID, r.TaskVersion); err != nil {
		return err
	}
	if _, err := s.compositionCreatorTx(ctx, tx, tenant, project, r.Caller); err != nil {
		return err
	}
	expires, err := time.Parse(time.RFC3339Nano, r.Instruction.ExpiresAt)
	if err != nil || !now.Before(expires) {
		return conflict(steeringChanged)
	}
	var body, author, kind string
	var seq int64
	err = tx.QueryRowContext(ctx, s.q(`SELECT body,author_id,actor_kind,sequence FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), tenant, project, r.Instruction.TaskID, r.Instruction.Message.ID).Scan(&body, &author, &kind, &seq)
	if err != nil {
		return err
	}
	m := r.Instruction.Message
	if body != m.Body || sc.MessageDigest(body) != m.BodySHA256 || author != m.AuthorID || seq != m.Sequence || kind != "human" {
		return conflict(steeringChanged)
	}
	return nil
}
