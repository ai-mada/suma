package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

func (s *Service) OpenToolPermission(ctx context.Context, actor Actor, project string, a dag.Assignment, request pc.Request) (pc.Permission, error) {
	return s.toolPermissionMachine(ctx, actor, "", project, a, "", &request)
}
func (s *Service) PollToolPermission(ctx context.Context, actor Actor, project string, a dag.Assignment, id string) (pc.Permission, error) {
	return s.toolPermissionMachine(ctx, actor, "", project, a, id, nil)
}
func (s *Service) OpenPoolToolPermission(ctx context.Context, token, project string, a dag.Assignment, request pc.Request) (pc.Permission, error) {
	return s.toolPermissionMachine(ctx, Actor{}, token, project, a, "", &request)
}
func (s *Service) PollPoolToolPermission(ctx context.Context, token, project string, a dag.Assignment, id string) (pc.Permission, error) {
	return s.toolPermissionMachine(ctx, Actor{}, token, project, a, id, nil)
}

func (s *Service) toolPermissionMachine(ctx context.Context, actor Actor, token, project string, a dag.Assignment, id string, request *pc.Request) (pc.Permission, error) {
	var empty pc.Permission
	if !handoffID(project, false) || a.ProjectID != project || !handoffID(a.TenantID, false) || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || a.DispatchID == "" || request != nil && pc.ValidateRequest(*request) != nil || request == nil && !handoffID(id, false) {
		return empty, invalid(toolPermissionInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return empty, err
	}
	defer cancel()
	defer tx.Rollback()
	if _, err = s.toolPermissionMachineAuthorityTx(ctx, tx, actor, token, project, a); err != nil {
		return empty, err
	}
	b, err := s.toolPermissionBasisTx(ctx, tx, a.TenantID, project, a)
	if err != nil {
		return empty, err
	}
	if err = s.toolPermissionDispatchTx(ctx, tx, a.TenantID, project, a); err != nil {
		return empty, err
	}
	if err = s.toolPermissionCurrentProfileTx(ctx, tx, a.TenantID, project, b, a); err != nil {
		return empty, err
	}
	if request != nil && !toolPermissionRequestMatchesBasis(*request, b) {
		return empty, conflict(toolPermissionChanged)
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return empty, err
	}
	if request == nil {
		r, e := s.toolPermissionReadTx(ctx, tx, a.TenantID, project, id)
		if e != nil {
			return empty, e
		}
		if !toolPermissionSameSource(r.value, a) || r.profileID != b.profile.ID || r.profileVersion != b.profile.Version || !tc.EqualProfile(r.profile, b.profile.Profile) || !toolPermissionRequestMatchesBasis(r.value.Request, b) {
			return empty, conflict(toolPermissionChanged)
		}
		return s.toolPermissionDeliveryTx(ctx, tx, r, b, now)
	}
	var existing string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND worker_id=? AND operation_id=?`), a.TenantID, project, a.WorkerID, request.IdempotencyKey).Scan(&existing)
	if err == nil {
		r, e := s.toolPermissionReadTx(ctx, tx, a.TenantID, project, existing)
		if e != nil {
			return empty, e
		}
		if r.requestHash != toolPermissionRequestHash(a, *request) || r.profileID != b.profile.ID || r.profileVersion != b.profile.Version || !tc.EqualProfile(r.profile, b.profile.Profile) {
			return empty, &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
		}
		return s.toolPermissionDeliveryTx(ctx, tx, r, b, now)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return empty, err
	}
	// Recover the original operation before new-work admission. Delivery still
	// applies current lease/pause policy and cannot renew a previously decided grant.
	probe := toolPermissionRow{value: pc.Permission{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, State: "pending"}}
	if toolPermissionEffective(probe, b, now).State != "pending" || b.runState != "running" {
		return empty, conflict(toolPermissionChanged)
	}
	sessionSHA, requestSHA, err := pc.RequestSourceDigests(*request)
	if err != nil {
		return empty, invalid(toolPermissionInvalid)
	}
	if err = s.toolInteractionAdmitTx(ctx, tx, a, sessionSHA, requestSHA); err != nil {
		return empty, err
	}
	r := toolPermissionRow{tenant: a.TenantID, profileID: b.profile.ID, profileVersion: b.profile.Version, profile: b.profile.Profile, assignment: a, requestHash: toolPermissionRequestHash(a, *request)}
	r.value = pc.Permission{ID: newID(), ProjectID: project, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: b.profile.Profile.ProfileSHA256, Version: 1, State: "pending", Request: *request, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: time.Unix(b.deadline, 0).UTC().Format(time.RFC3339Nano), AllowedActions: []string{}}
	if err = s.toolPermissionSaveTx(ctx, tx, r, 0); err != nil {
		return empty, err
	}
	if err = tx.Commit(); err != nil {
		return empty, err
	}
	return r.value, nil
}

func (s *Service) toolPermissionHumanTx(ctx context.Context, actor Actor, project, id string, purpose commandPurpose) (context.Context, *authorizedCommand, toolPermissionRow, func(), error) {
	var empty toolPermissionRow
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ctx, nil, empty, func() {}, err
	}
	close := func() { tx.Rollback(); cancel() }
	r, err := s.toolPermissionReadTx(ctx, tx, actor.credential.tenantID, project, id)
	if err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	var organizations []string
	if r.assignment.PoolID != "" {
		var owner string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT owner_tenant_id FROM suma_execution_pools WHERE id=?`), r.assignment.PoolID).Scan(&owner); err != nil {
			close()
			return ctx, nil, empty, func() {}, forbidden()
		}
		organizations = append(organizations, owner)
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: r.tenant, projectID: project, resourceID: id}, purpose, organizations)
	if err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	if err = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, r.tenant, project, r.value.RunID); err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	r, err = s.toolPermissionReadTx(ctx, tx, r.tenant, project, id)
	if err != nil {
		close()
		return ctx, nil, empty, func() {}, err
	}
	return ctx, auth, r, close, nil
}

func (s *Service) DecideToolPermission(ctx context.Context, actor Actor, id string, request pc.DecisionRequest) (pc.Permission, error) {
	var empty pc.Permission
	if !actor.IsBrowser() {
		return empty, forbidden()
	}
	if !handoffID(id, false) || pc.ValidateDecisionRequest(request) != nil {
		return empty, invalid(toolPermissionInvalid)
	}
	ctx, auth, r, close, err := s.toolPermissionHumanTx(ctx, actor, request.ProjectID, id, purposeBrowserProjectApprove)
	if err != nil {
		return empty, err
	}
	defer close()
	b, err := s.toolPermissionBasisTx(ctx, auth.tx, r.tenant, request.ProjectID, r.assignment)
	if err != nil {
		return empty, err
	}
	if r.profileID != b.profile.ID || r.profileVersion != b.profile.Version || !tc.EqualProfile(r.profile, b.profile.Profile) || !toolPermissionRequestMatchesBasis(r.value.Request, b) {
		return empty, conflict(toolPermissionChanged)
	}
	if err = s.toolPermissionCurrentProfileTx(ctx, auth.tx, r.tenant, request.ProjectID, b, r.assignment); err != nil {
		return empty, err
	}
	if err = s.toolPermissionDispatchTx(ctx, auth.tx, r.tenant, request.ProjectID, r.assignment); err != nil {
		return empty, err
	}
	// A current approver can recover its original receipt after the process ends.
	// This does not recreate a permission, renew a lease, or authorize its delivery.
	body := struct {
		Kind, ID string
		Request  pc.DecisionRequest
	}{"tool_permission_decision", id, request}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, body)
	if err != nil {
		return empty, err
	}
	if replay {
		var original pc.Permission
		if json.Unmarshal([]byte(hash), &original) != nil || original.ID != id || original.Decision == nil || original.Decision.ActorID != auth.currentPrincipal.ID || stringify(original.Decision.Choice) != stringify(request.Choice) || !pc.DecisionMatchesRequest(original.Request, *original.Decision) {
			return empty, conflict(toolPermissionChanged)
		}
		return original, nil
	}
	now, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return empty, err
	}
	effective := toolPermissionEffective(r, b, now)
	if effective.State != "pending" || r.value.Version != request.ExpectedVersion {
		return empty, conflict(toolPermissionChanged)
	}
	if !pc.ChoiceMatchesRequest(r.value.Request, request.Choice) {
		return empty, invalid(toolPermissionInvalid)
	}
	kind := toolPermissionChoiceAction(request.Choice)
	actions, err := s.toolPermissionAllowsTx(ctx, auth.tx, r.tenant, request.ProjectID, effective, b)
	if err != nil {
		return empty, err
	}
	allowed := false
	for _, action := range actions {
		allowed = allowed || action == kind
	}
	if !allowed {
		return empty, forbidden()
	}
	before := r.value.Version
	r.value.Version++
	r.value.State = "decided"
	r.value.Decision = &pc.Decision{Choice: request.Choice, ActorID: auth.currentPrincipal.ID, DecidedAt: now.Format(time.RFC3339Nano)}
	if err = s.toolPermissionSaveTx(ctx, auth.tx, r, before); err != nil {
		return empty, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, r.value); err != nil {
		return empty, err
	}
	if err = auth.tx.Commit(); err != nil {
		return empty, err
	}
	return r.value, nil
}

func (s *Service) ListToolPermissions(ctx context.Context, actor Actor, project, run, node string, locator ...string) (pc.List, error) {
	out := pc.List{Items: []pc.Permission{}}
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	if !handoffID(project, false) || run == "" || node == "" || len(locator) > 1 || len(locator) == 1 && !handoffID(locator[0], false) {
		return out, invalid(toolPermissionInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, actor.credential.tenantID, project, run)
	if err != nil {
		return out, err
	}
	organizations := []string{}
	if route.PoolID != "" {
		var owner string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT owner_tenant_id FROM suma_execution_pools WHERE id=?`), route.PoolID).Scan(&owner); err != nil {
			return out, forbidden()
		}
		organizations = append(organizations, owner)
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: run}, purposeBrowserProjectRead, organizations)
	if err != nil {
		return out, err
	}
	if err = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, auth.target.tenantID, project, run); err != nil {
		return out, err
	}
	var attempt int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), auth.target.tenantID, project, run, node).Scan(&attempt); err != nil {
		return out, missing()
	}
	query := `SELECT id FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? ORDER BY id LIMIT 33`
	args := []any{auth.target.tenantID, project, run, node, attempt}
	if len(locator) == 1 {
		query = `SELECT id FROM suma_tool_permissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND id=?`
		args = []any{auth.target.tenantID, project, run, node, locator[0]}
	}
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
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
		return out, err
	}
	if len(ids) > 32 {
		return out, conflict(toolPermissionChanged)
	}
	if len(locator) == 1 && len(ids) == 0 {
		return out, missing()
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	_, canDecide := s.currentProjectMembershipLockedTx(ctx, tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, true)
	if canDecide != nil && !automationAuthorityFailure(canDecide) {
		return out, canDecide
	}
	for _, id := range ids {
		r, e := s.toolPermissionReadTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		b, e := s.toolPermissionBasisTx(ctx, tx, r.tenant, project, r.assignment)
		if e != nil {
			if !automationAuthorityFailure(e) {
				return out, e
			}
			p := r.value
			if p.State == "pending" {
				p.State = "superseded"
			}
			p.AllowedActions = []string{}
			out.Items = append(out.Items, p)
			continue
		}
		// A stored human decision is an audit fact, never a current send grant.
		// Only still-pending requests acquire a derived terminal read state.
		p := r.value
		p.AllowedActions = []string{}
		if p.State == "pending" {
			p = toolPermissionEffective(r, b, now)
		}
		if canDecide == nil && r.profileID == b.profile.ID && r.profileVersion == b.profile.Version && tc.EqualProfile(r.profile, b.profile.Profile) {
			e = s.toolPermissionCurrentProfileTx(ctx, tx, r.tenant, project, b, r.assignment)
			if e == nil {
				e = s.toolPermissionDispatchTx(ctx, tx, r.tenant, project, r.assignment)
			}
			if e != nil && !automationAuthorityFailure(e) {
				return out, e
			}
			if e == nil {
				p.AllowedActions, e = s.toolPermissionAllowsTx(ctx, tx, r.tenant, project, p, b)
				if e != nil {
					return out, e
				}
			}
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}
