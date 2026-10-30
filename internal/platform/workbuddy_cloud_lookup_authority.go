package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// Lookup has its own read authority. Execution authority would require a live
// lease/delegation and decrypt OAuth credentials, none of which a stored-fact
// read may renew or consume. Current identities and grants are still checked.
func (w *WorkBuddyConnections) cloudLookupAuthorityTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, req wc.CloudLookupRequest) (cloudDelegationRow, error) {
	var empty cloudDelegationRow
	if err := w.s.authorityWriter(ctx, tx); err != nil {
		return empty, err
	}
	discovered, err := w.cloudDelegationTx(ctx, tx, subject.tenant, subject.project, req.DelegationID, false)
	if err != nil {
		return empty, err
	}
	users, orgs, keys := []string{discovered.OwnerID}, []string{subject.tenant}, []string{"workbuddy-cloud:" + req.DelegationID}
	poolOwner := ""
	if subject.pool {
		identity, e := w.s.poolMachineIdentityTx(ctx, tx, subject.token, req.Assignment.PoolRunnerID, false)
		if e != nil {
			return empty, e
		}
		poolOwner = identity.owner
		orgs = append(orgs, poolOwner)
	} else {
		if candidateHasPoolFields(req.Assignment) {
			return empty, forbidden()
		}
		users = append(users, subject.actor.credential.subjectID)
		keys = append(keys, credentialLockKeys(subject.actor.credential)...)
	}
	for _, user := range sortedAuthorityIDs(users) {
		set, e := w.s.authorityOrganizations(ctx, tx, user)
		if e != nil {
			return empty, e
		}
		orgs = append(orgs, set...)
	}
	if err = w.s.lockHumanAuthorities(ctx, tx, users, orgs, keys); err != nil {
		return empty, err
	}
	d, err := w.cloudDelegationTx(ctx, tx, subject.tenant, subject.project, req.DelegationID, true)
	if err != nil {
		return empty, err
	}
	if d.OwnerID != discovered.OwnerID {
		return empty, &APIError{409, "authority_retry", "权限集合已改变，请重试", nil}
	}
	if _, err = w.s.currentProjectMembershipLockedTx(ctx, tx, subject.tenant, d.OwnerID, subject.project, true, false); err != nil {
		return empty, err
	}
	if subject.pool {
		a := req.Assignment
		if a.TenantID != subject.tenant || a.ProjectID != subject.project {
			return empty, forbidden()
		}
		identity, e := w.s.poolMachineIdentityTx(ctx, tx, subject.token, a.PoolRunnerID, true)
		if e != nil {
			return empty, e
		}
		if identity.owner != poolOwner || identity.poolStatus != "active" || identity.runner.PoolID != a.PoolID || identity.runner.WorkerID != a.WorkerID || identity.credentialID != a.PoolCredentialID || identity.runner.CredentialVersion != a.PoolCredentialVersion {
			return empty, forbidden()
		}
		scope, e := w.s.currentPoolActiveScopeTx(ctx, tx, a.PoolID, subject.tenant, subject.project)
		if e != nil {
			return empty, e
		}
		if scope.GrantID != a.PoolGrantID || scope.GrantVersion != a.PoolGrantVersion {
			return empty, forbidden()
		}
		if e = dag.ValidatePoolAssignmentTx(ctx, tx, w.s.Dialect, candidateDAGAssignment(a)); e != nil {
			return empty, e
		}
	} else {
		auth, e := w.s.authorizeTx(ctx, tx, subject.actor, commandTarget{tenantID: subject.tenant, projectID: subject.project}, purposeRunnerRead)
		if e != nil {
			return empty, e
		}
		if e = w.s.runnerIdentityBindingTx(ctx, tx, auth, req.Assignment.WorkerID); e != nil {
			return empty, e
		}
		route, e := dag.ReadRunExecutionRouteTx(ctx, tx, w.s.Dialect, subject.tenant, subject.project, req.Assignment.RunID)
		if e != nil {
			return empty, e
		}
		if route.PoolID != "" {
			return empty, forbidden()
		}
	}
	if err = dag.GuardProjectTx(ctx, tx, w.s.Dialect, subject.tenant, subject.project); err != nil {
		return empty, err
	}
	if err = w.validateCloudDispatchBindingTx(ctx, tx, subject.tenant, subject.project, d.CloudDelegation, req.Assignment); err != nil {
		return empty, err
	}
	if d.RunID != req.Assignment.RunID || d.NodeID != req.Assignment.NodeID {
		return empty, cloudConflict()
	}
	n, err := w.cloudNodeTx(ctx, tx, subject.tenant, subject.project, d.RunID, d.NodeID)
	if err != nil {
		return empty, err
	}
	var fence int64
	var worker string
	if err = tx.QueryRowContext(ctx, w.s.q(w.s.executionReadSQL(`SELECT fence,worker_id FROM dag_nodes WHERE run_id=? AND id=?`, true)), d.RunID, d.NodeID).Scan(&fence, &worker); err != nil {
		return empty, err
	}
	if n.attempt != req.Assignment.Attempt || fence != req.Assignment.Fence || worker != req.Assignment.WorkerID || n.specHash != d.NodeSpecSHA256 || n.requestHash != d.RequestSHA256 || n.binding.ConnectionID != d.ConnectionID || n.binding.GrantGeneration != d.GrantGeneration {
		return empty, cloudConflict()
	}
	switch n.nodeState {
	case "running", "reconciling", "waiting", "succeeded", "failed", "cancelled", "skipped":
	default:
		return empty, cloudConflict()
	}
	row, err := w.rowTx(ctx, tx, subject.tenant, subject.project, d.ConnectionID, true)
	if err != nil {
		return empty, err
	}
	if row.OwnerID != d.OwnerID {
		return empty, missing()
	}
	if err = w.workBuddyCommandApp(row); err != nil {
		return empty, err
	}
	if row.State == "disconnected" || row.GrantGeneration != d.GrantGeneration || cloudClientBinding(row) != d.ClientBindingSHA256 {
		return empty, cloudConflict()
	}
	actual, configured := map[string]bool{}, map[string]bool{}
	for _, scope := range row.Scopes {
		actual[scope] = true
	}
	for _, scope := range w.app.Scopes {
		configured[scope] = true
	}
	for _, scope := range []string{"user.task.readable", "user.task.invokable"} {
		if !actual[scope] || !configured[scope] {
			return empty, forbidden()
		}
	}
	// Delegation expiry/revocation and OAuth token expiry do not erase original
	// stored facts. No token is opened, no budget is consumed, no lease renewed.
	return d, nil
}
