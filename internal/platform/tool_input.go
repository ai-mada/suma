package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func (s *Service) OpenToolInput(ctx context.Context, actor Actor, project string, a dag.Assignment, request ic.ToolInputRequest) (ic.ToolInput, error) {
	return s.toolInputMachine(ctx, actor, "", project, a, "", &request)
}
func (s *Service) OpenPoolToolInput(ctx context.Context, token, project string, a dag.Assignment, request ic.ToolInputRequest) (ic.ToolInput, error) {
	return s.toolInputMachine(ctx, Actor{}, token, project, a, "", &request)
}
func (s *Service) PollToolInput(ctx context.Context, actor Actor, project string, a dag.Assignment, id string) (ic.ToolInput, error) {
	return s.toolInputMachine(ctx, actor, "", project, a, id, nil)
}
func (s *Service) PollPoolToolInput(ctx context.Context, token, project string, a dag.Assignment, id string) (ic.ToolInput, error) {
	return s.toolInputMachine(ctx, Actor{}, token, project, a, id, nil)
}

func (s *Service) toolInputBasisTx(ctx context.Context, tx *sql.Tx, actor Actor, token, project string, a dag.Assignment) (*candidateAuthority, toolPermissionBasis, error) {
	authority, err := s.toolPermissionMachineAuthorityTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return nil, toolPermissionBasis{}, err
	}
	b, err := s.toolPermissionBasisTx(ctx, tx, a.TenantID, project, a)
	if err != nil {
		return nil, b, err
	}
	if toolInputProfileProtocol(b.node, b.profile) == "" {
		return nil, b, conflict(toolInputChanged)
	}
	if err = s.toolPermissionDispatchTx(ctx, tx, a.TenantID, project, a); err == nil {
		err = s.toolPermissionCurrentProfileTx(ctx, tx, a.TenantID, project, b, a)
	}
	return authority, b, err
}

func toolInputProfileProtocol(node dag.Node, profile ToolProfile) string {
	if profile.Profile.UserInputProtocol == cc.UserInputProtocol && supportsCodexExecution(profile) {
		binding, err := dag.CodexBinding(node)
		if err == nil && binding != nil {
			return cc.UserInputProtocol
		}
	}
	if profile.Profile.Adapter == "cursor" && profile.Profile.UserInputProtocol == cu.InputProtocol && acpNodeMatchesProfile(node, profile) {
		return cu.InputProtocol
	}
	return ""
}

func toolInputRowMatches(r toolInputRow, b toolPermissionBasis, a dag.Assignment) bool {
	return toolInputSameSource(r.value, a) && r.profileID == b.profile.ID && r.profileVersion == b.profile.Version && tc.EqualProfile(r.profile, b.profile.Profile) && toolInputMatchesBasis(r.value.Request, b)
}

func (s *Service) toolInputDeliveryTx(ctx context.Context, tx *sql.Tx, r toolInputRow, b toolPermissionBasis, now time.Time) (ic.ToolInput, error) {
	v := toolInputEffective(r, b, now)
	if v.State == "answered" {
		allowed, err := s.toolPermissionWorkAllowedTx(ctx, tx, r.tenant, v.ProjectID, b)
		if err != nil {
			return ic.ToolInput{}, err
		}
		if !allowed {
			v.State = "closed"
		}
	}
	return v, nil
}

func (s *Service) toolInputMachine(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id string, request *ic.ToolInputRequest) (out ic.ToolInput, err error) {
	if !handoffID(project, false) || a.ProjectID != project || !handoffID(a.TenantID, false) || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || a.DispatchID == "" || request != nil && ic.ValidateRequest(*request) != nil || request == nil && !handoffID(id, false) {
		return out, invalid(toolInputInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	_, b, err := s.toolInputBasisTx(ctx, tx, actor, token, project, a)
	if err != nil {
		return out, err
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	if request == nil {
		r, e := s.toolInputReadTx(ctx, tx, a.TenantID, project, id)
		if e != nil {
			return out, e
		}
		if !toolInputRowMatches(r, b, a) {
			return out, conflict(toolInputChanged)
		}
		return s.toolInputDeliveryTx(ctx, tx, r, b, now)
	}
	if !toolInputMatchesBasis(*request, b) {
		return out, conflict(toolInputChanged)
	}
	var existing string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_tool_inputs WHERE tenant_id=? AND project_id=? AND worker_id=? AND operation_id=?`), a.TenantID, project, a.WorkerID, request.IdempotencyKey).Scan(&existing)
	if err == nil {
		r, e := s.toolInputReadTx(ctx, tx, a.TenantID, project, existing)
		if e != nil {
			return out, e
		}
		if r.requestHash != toolInputRequestHash(a, *request) || !toolInputRowMatches(r, b, a) {
			return out, conflict(toolInputChanged)
		}
		return s.toolInputDeliveryTx(ctx, tx, r, b, now)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if b.runState != "running" || b.state != "running" || b.attempt != a.Attempt || b.fence != a.Fence || b.worker != a.WorkerID || b.lease <= now.Unix() || b.deadline <= now.Unix() {
		return out, conflict(toolInputChanged)
	}
	allowed, err := s.toolPermissionWorkAllowedTx(ctx, tx, a.TenantID, project, b)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, forbidden()
	}
	sessionSHA, requestSHA := toolInputSourceDigests(*request)
	if err = s.toolInteractionAdmitTx(ctx, tx, a, sessionSHA, requestSHA); err != nil {
		return out, err
	}
	deadline := now.Add(15 * time.Minute)
	if nodeDeadline := time.Unix(b.deadline, 0); nodeDeadline.Before(deadline) {
		deadline = nodeDeadline
	}
	r := toolInputRow{tenant: a.TenantID, profileID: b.profile.ID, profileVersion: b.profile.Version, profile: b.profile.Profile, assignment: a, requestHash: toolInputRequestHash(a, *request)}
	r.value = ic.ToolInput{ID: newID(), ProjectID: project, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: b.profile.Profile.ProfileSHA256, Version: 1, State: "pending", Request: *request, CreatedAt: now.UTC().Format(time.RFC3339Nano), ExpiresAt: deadline.UTC().Format(time.RFC3339Nano), AllowedActions: []string{}}
	if err = s.toolInputSaveTx(ctx, tx, r, 0); err != nil {
		return out, err
	}
	return r.value, tx.Commit()
}

func (s *Service) toolInputHumanTx(ctx context.Context, actor Actor, project, id string, purpose commandPurpose) (context.Context, *authorizedCommand, toolInputRow, func(), error) {
	var empty toolInputRow
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ctx, nil, empty, func() {}, err
	}
	close := func() { tx.Rollback(); cancel() }
	r, err := s.toolInputReadTx(ctx, tx, actor.credential.tenantID, project, id)
	if err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	organizations := []string{}
	if r.assignment.PoolID != "" {
		var owner string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT owner_tenant_id FROM suma_execution_pools WHERE id=?`), r.assignment.PoolID).Scan(&owner); err != nil {
			close()
			return ctx, nil, empty, func() {}, forbidden()
		}
		organizations = append(organizations, owner)
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: r.tenant, projectID: project, resourceID: id}, purpose, organizations)
	if err == nil {
		err = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, r.tenant, project, r.value.RunID)
	}
	if err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	r, err = s.toolInputReadTx(ctx, tx, r.tenant, project, id)
	if err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	return ctx, auth, r, close, nil
}

func (s *Service) AnswerToolInput(ctx context.Context, actor Actor, id string, request ic.ToolInputAnswerRequest) (out ic.ToolInput, err error) {
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	if !handoffID(id, false) || !handoffID(request.ProjectID, false) || request.ExpectedVersion < 1 || request.ExpectedVersion >= cc.MaxSafeInteger || !planningKey(request.IdempotencyKey) {
		return out, invalid(toolInputInvalid)
	}
	ctx, auth, r, close, err := s.toolInputHumanTx(ctx, actor, request.ProjectID, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	answerSHA, err := ic.AnswerDigest(r.value.Request, request.Answers, request.Cursor)
	if err != nil {
		return out, invalid(toolInputInvalid)
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, struct {
		Kind, ID string
		Request  ic.ToolInputAnswerRequest
	}{"tool_input_answer", id, request})
	if err != nil {
		return out, err
	}
	if replay {
		if decodeToolInputStorage(hash, &out) != nil || out.ID != id || out.Answer == nil || out.Answer.ActorID != auth.currentPrincipal.ID || out.Answer.AnswerSHA256 != answerSHA || r.value.Request.Codex != nil && stringify(out.Answer.Answers) != stringify(request.Answers) || stringify(out.Answer.Cursor) != stringify(request.Cursor) {
			return ic.ToolInput{}, conflict(toolInputChanged)
		}
		return out, nil
	}
	b, err := s.toolPermissionBasisTx(ctx, auth.tx, r.tenant, request.ProjectID, r.assignment)
	if err != nil {
		return out, err
	}
	if !toolInputRowMatches(r, b, r.assignment) {
		return out, conflict(toolInputChanged)
	}
	if err = s.toolPermissionCurrentProfileTx(ctx, auth.tx, r.tenant, request.ProjectID, b, r.assignment); err == nil {
		err = s.toolPermissionDispatchTx(ctx, auth.tx, r.tenant, request.ProjectID, r.assignment)
	}
	if err != nil {
		return out, err
	}
	now, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	if toolInputEffective(r, b, now).State != "pending" || r.value.Version != request.ExpectedVersion {
		return out, conflict(toolInputChanged)
	}
	allowed, err := s.toolPermissionWorkAllowedTx(ctx, auth.tx, r.tenant, request.ProjectID, b)
	if err != nil {
		return out, err
	}
	if !allowed {
		return out, forbidden()
	}
	before := r.value.Version
	r.value.Version++
	r.value.State = "answered"
	r.value.Answer = &ic.ToolInputAnswer{Answers: append([]cc.CodexUserInputSelection{}, request.Answers...), Cursor: request.Cursor, AnswerSHA256: answerSHA, ActorID: auth.currentPrincipal.ID, AnsweredAt: now.UTC().Format(time.RFC3339Nano)}
	if err = s.toolInputSaveTx(ctx, auth.tx, r, before); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, toolInputStorageValue(r.value)); err != nil {
		return out, err
	}
	return r.value, auth.tx.Commit()
}
