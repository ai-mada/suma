package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func (s *Service) ListToolInputs(ctx context.Context, actor Actor, project, run, node string, attempt int, fence int64) (out ic.ToolInputList, err error) {
	out.Items = []ic.ToolInput{}
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	if !handoffID(project, false) || !diagnosisID(run, 100) || !diagnosisID(node, 150) || attempt < 1 || fence < 1 {
		return out, invalid(toolInputInvalid)
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
	var storedTenant, storedProject, storedRun, storedNode, nodeRaw string
	// A valid empty list must refer to a durable attempt. Historical attempts
	// retain their own fence even after the current node advances.
	err = tx.QueryRowContext(ctx, s.q(`
		SELECT r.tenant_id,r.project_id,r.id,n.id,n.spec
		FROM dag_runs r
		JOIN dag_nodes n ON n.run_id=r.id
		JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id
		WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=? AND a.attempt=? AND a.fence=?
	`), auth.target.tenantID, project, run, node, attempt, fence).Scan(&storedTenant, &storedProject, &storedRun, &storedNode, &nodeRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	// SQL text equality can ignore case on some deployments.
	if storedTenant != auth.target.tenantID || storedProject != project || storedRun != run || storedNode != node {
		return out, missing()
	}
	var spec dag.Node
	if json.Unmarshal([]byte(nodeRaw), &spec) != nil {
		return out, conflict(toolInputChanged)
	}
	profileID, profileSHA, bound, e := dag.ToolBinding(spec)
	if e == nil && bound && spec.Type == "agent" {
		profile, e := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, profileID)
		if e != nil {
			if !automationAuthorityFailure(e) {
				return out, e
			}
		} else if profile.Profile.ProfileSHA256 == profileSHA {
			out.Protocol = toolInputProfileProtocol(spec, profile)
		}
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inputs WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? ORDER BY id LIMIT 33`), auth.target.tenantID, project, run, node, attempt, fence)
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
		return out, conflict(toolInputChanged)
	}
	now, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	_, canWrite := s.currentProjectMembershipLockedTx(ctx, tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false)
	if canWrite != nil && !automationAuthorityFailure(canWrite) {
		return out, canWrite
	}
	for _, id := range ids {
		r, e := s.toolInputReadTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		v := r.value
		v.AllowedActions = []string{}
		b, e := s.toolPermissionBasisTx(ctx, tx, r.tenant, project, r.assignment)
		if e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return out, e
			}
			if v.State == "pending" {
				v.State = "closed"
			}
			out.Items = append(out.Items, v)
			continue
		}
		if v.State == "pending" {
			v = toolInputEffective(r, b, now)
		}
		if out.Protocol != "" && v.State == "pending" && canWrite == nil && toolInputRowMatches(r, b, r.assignment) {
			e = s.toolPermissionCurrentProfileTx(ctx, tx, r.tenant, project, b, r.assignment)
			if e == nil {
				e = s.toolPermissionDispatchTx(ctx, tx, r.tenant, project, r.assignment)
			}
			if e == nil {
				var allowed bool
				allowed, e = s.toolPermissionWorkAllowedTx(ctx, tx, r.tenant, project, b)
				if allowed && e == nil {
					v.AllowedActions = []string{"answer"}
				}
			}
			if e != nil && !automationAuthorityFailure(e) {
				return out, e
			}
		}
		out.Items = append(out.Items, v)
	}
	return out, nil
}
