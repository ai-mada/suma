package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type cloudNode struct {
	spec                                       dag.Node
	binding                                    wc.NodeBinding
	specHash, requestHash, runState, nodeState string
	runVersion, attempt                        int
}

func (w *WorkBuddyConnections) cloudNodeTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (out cloudNode, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, w.s.q(w.s.executionReadSQL(`SELECT n.spec,r.version,r.state,n.state,n.attempt FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`, true)), tenant, project, run, node).Scan(&raw, &out.runVersion, &out.runState, &out.nodeState, &out.attempt)
	if err == sql.ErrNoRows {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out.spec) != nil || out.spec.Type != wc.NodeType || out.spec.Config["plugin"] != wc.Plugin || out.spec.Config["workflow"] != wc.Protocol || len(out.spec.Command) != 0 {
		return out, cloudInvalid()
	}
	out.binding, err = wc.ParseNodeBinding(out.spec.Config["workbuddy_cloud"])
	if err != nil {
		return out, cloudInvalid()
	}
	out.requestHash, err = out.binding.RequestDigest()
	out.specHash = digest(stringify(out.spec))
	return
}
func cloudClientBinding(row workBuddyConnectionRow) string {
	return digest(stringify([]any{row.ID, row.OwnerID, row.ClientID, row.OpenID, row.AppDigest, row.GrantGeneration, row.Scopes}))
}
func (w *WorkBuddyConnections) cloudGrantTx(ctx context.Context, tx *sql.Tx, tenant, project, owner, connection string, generation int, binding string) (workBuddyConnectionRow, *workbuddy.Client, error) {
	row, err := w.rowTx(ctx, tx, tenant, project, connection, true)
	if err != nil {
		return row, nil, err
	}
	if row.OwnerID != owner {
		return row, nil, missing()
	}
	if err = w.workBuddyCommandApp(row); err != nil {
		return row, nil, err
	}
	clock, err := w.s.authorityNow(ctx, tx)
	if err != nil {
		return row, nil, err
	}
	expiry, e := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if row.State != "connected" || row.ActiveOperationID != "" || row.GrantGeneration != generation || e != nil || !clock.Before(expiry) || binding != "" && cloudClientBinding(row) != binding {
		return row, nil, cloudConflict()
	}
	scopes := map[string]bool{}
	for _, s := range row.Scopes {
		scopes[s] = true
	}
	allowed := map[string]bool{}
	for _, s := range w.app.Scopes {
		allowed[s] = true
	}
	for _, s := range []string{"user.task.readable", "user.task.invokable"} {
		if !scopes[s] || !allowed[s] {
			return row, nil, forbidden()
		}
	}
	grant, err := w.open(row)
	if err != nil {
		return row, nil, conflict(workBuddyUnavailable)
	}
	client, err := workbuddy.NewClient(workbuddy.Config{BaseURL: w.app.BaseURL, Token: grant.AccessToken, HTTPClient: w.app.HTTPClient})
	if err != nil {
		return row, nil, conflict(workBuddyUnavailable)
	}
	return row, client, nil
}
func (w *WorkBuddyConnections) cloudBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string) error {
	if err := w.s.usageLockTx(ctx, tx, tenant); err != nil {
		return err
	}
	for _, scope := range []string{"organization", "project"} {
		policy, err := w.s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if policy.Mode == "enforce" {
			return &APIError{409, "workbuddy_cloud_budget_unsupported", "workbuddy_cloud_budget_unsupported", nil}
		}
	}
	return nil
}

type cloudMachineProof struct {
	candidate  *candidateAuthority
	delegation cloudDelegationRow
	node       cloudNode
	connection workBuddyConnectionRow
	client     *workbuddy.Client
}

// Collect every real participant before taking account/credential/project locks.
// No owner ID is ever converted into an Actor or machine credential.
func (w *WorkBuddyConnections) cloudMachineAuthorityTx(ctx context.Context, tx *sql.Tx, subject candidateSubject, id string, a cc.AssignmentRef, newExternal bool) (out cloudMachineProof, err error) {
	if err = w.s.authorityWriter(ctx, tx); err != nil {
		return out, err
	}
	discovered, err := w.cloudDelegationTx(ctx, tx, subject.tenant, subject.project, id, false)
	if err != nil {
		return out, err
	}
	users := []string{discovered.OwnerID}
	orgs := []string{subject.tenant}
	keys := []string{"workbuddy-cloud:" + id}
	poolOwner := ""
	if subject.pool {
		identity, e := w.s.poolMachineIdentityTx(ctx, tx, subject.token, a.PoolRunnerID, false)
		if e != nil {
			return out, e
		}
		poolOwner = identity.owner
		orgs = append(orgs, poolOwner)
	} else {
		users = append(users, subject.actor.credential.subjectID)
		keys = append(keys, credentialLockKeys(subject.actor.credential)...)
	}
	for _, user := range sortedAuthorityIDs(users) {
		set, e := w.s.authorityOrganizations(ctx, tx, user)
		if e != nil {
			return out, e
		}
		orgs = append(orgs, set...)
	}
	if err = w.s.lockHumanAuthorities(ctx, tx, users, orgs, keys); err != nil {
		return out, err
	}
	out.delegation, err = w.cloudDelegationTx(ctx, tx, subject.tenant, subject.project, id, true)
	if err != nil {
		return out, err
	}
	d := out.delegation
	if d.OwnerID != discovered.OwnerID {
		return out, &APIError{409, "authority_retry", "权限集合已改变，请重试", nil}
	}
	if _, err = w.s.currentProjectMembershipLockedTx(ctx, tx, subject.tenant, d.OwnerID, subject.project, true, false); err != nil {
		return out, err
	}
	out.candidate, err = w.s.candidateAuthorityTx(ctx, tx, subject, a)
	if err != nil {
		return out, err
	}
	if subject.pool {
		identity, e := w.s.poolMachineIdentityTx(ctx, tx, subject.token, a.PoolRunnerID, true)
		if e != nil {
			return out, e
		}
		scope, e := w.s.currentPoolActiveScopeTx(ctx, tx, a.PoolID, subject.tenant, subject.project)
		if e != nil {
			return out, e
		}
		if identity.owner != poolOwner || scope.GrantID != a.PoolGrantID || scope.GrantVersion != a.PoolGrantVersion || identity.credentialID != a.PoolCredentialID || identity.runner.CredentialVersion != a.PoolCredentialVersion {
			return out, forbidden()
		}
	}
	clock, err := w.s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	expiry, e := time.Parse(time.RFC3339Nano, d.ExpiresAt)
	if d.State != "active" || e != nil || !clock.Before(expiry) || a.RunID != d.RunID || a.NodeID != d.NodeID {
		return out, cloudConflict()
	}
	out.node, err = w.cloudNodeTx(ctx, tx, subject.tenant, subject.project, d.RunID, d.NodeID)
	if err != nil {
		return out, err
	}
	n := out.node
	if n.specHash != d.NodeSpecSHA256 || n.requestHash != d.RequestSHA256 || n.binding.ConnectionID != d.ConnectionID || n.binding.GrantGeneration != d.GrantGeneration || newExternal && n.runState != "running" {
		return out, cloudConflict()
	}
	out.connection, out.client, err = w.cloudGrantTx(ctx, tx, subject.tenant, subject.project, d.OwnerID, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256)
	if err != nil {
		return out, err
	}
	if newExternal {
		err = w.cloudBudgetTx(ctx, tx, subject.tenant, subject.project)
	}
	return out, err
}

func (w *WorkBuddyConnections) cloudMachineActivity(ctx context.Context, proof cloudMachineProof, kind, id string) error {
	if err := proof.candidate.assert(); err != nil {
		return err
	}
	c := proof.candidate
	_, err := c.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_activity(id,tenant_id,project_id,actor_id,kind,object_id,title,created_at)VALUES(?,?,?,?,?,?,?,?)`), newID(), c.subject.tenant, c.subject.project, "runner:"+c.assignment.WorkerID, kind, id, kind, now())
	return err
}
