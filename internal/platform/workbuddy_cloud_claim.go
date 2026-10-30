package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type poolClaimParticipants struct{ users, organizations, keys []string }
type cloudClaimAuthority struct {
	s       *Service
	w       *WorkBuddyConnections
	tx      *sql.Tx
	private *authorizedCommand
	pool    *poolMachineProof
	owners  map[string]bool
	pinned  map[string]wc.CloudAssignmentBinding
	replays map[string]dag.Assignment
}

func (s *Service) newCloudClaimAuthority(w *WorkBuddyConnections) *cloudClaimAuthority {
	return &cloudClaimAuthority{s: s, w: w, owners: map[string]bool{}, pinned: map[string]wc.CloudAssignmentBinding{}, replays: map[string]dag.Assignment{}}
}
func cloudClaimKey(tenant, project, run, node string) string {
	return strings.Join([]string{tenant, project, run, node}, "\x00")
}

func (c *cloudClaimAuthority) discover(ctx context.Context, tx *sql.Tx, candidates []dag.ClaimCandidateRef) (poolClaimParticipants, error) {
	out := poolClaimParticipants{}
	if len(candidates) > 64 {
		return out, cloudConflict()
	}
	if len(candidates) == 0 {
		return out, nil
	}
	clauses := []string{}
	args := []any{}
	for _, r := range candidates {
		clauses = append(clauses, `(tenant_id=? AND project_id=? AND run_id=? AND node_id=?)`)
		args = append(args, r.TenantID, r.ProjectID, r.RunID, r.NodeID)
	}
	rows, err := tx.QueryContext(ctx, c.s.q(`SELECT id,owner_id FROM suma_workbuddy_cloud_delegations WHERE `+strings.Join(clauses, " OR ")), args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, owner string
		if err = rows.Scan(&id, &owner); err != nil {
			rows.Close()
			return out, err
		}
		if !c.owners[owner] {
			c.owners[owner] = true
			out.users = append(out.users, owner)
		}
		out.keys = append(out.keys, "workbuddy-cloud:"+id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, owner := range sortedAuthorityIDs(out.users) {
		orgs, e := c.s.authorityOrganizations(ctx, tx, owner)
		if e != nil {
			return out, e
		}
		out.organizations = append(out.organizations, orgs...)
	}
	return out, nil
}

func (c *cloudClaimAuthority) beforePrivate(a *engineCommandAuthority, tx *sql.Tx, worker, operation string, caps []string) error {
	if err := c.s.authorityWriter(a.ctx, tx); err != nil {
		return err
	}
	refs, err := dag.DiscoverClaimCandidatesTx(a.ctx, tx, c.s.Dialect, a.target.tenantID, a.target.projectID, worker, operation, caps)
	if err != nil {
		return err
	}
	participants, err := c.discover(a.ctx, tx, refs)
	if err != nil {
		return err
	}
	a.tx = tx
	a.proof, err = c.s.authorizeTxWithParticipants(a.ctx, tx, a.actor, a.target, purposeRunnerWrite, participants.organizations, participants.users...)
	if err != nil {
		return err
	}
	if err = c.s.runnerIdentityBindingTx(a.ctx, tx, a.proof, worker); err != nil {
		return err
	}
	c.tx, c.private = tx, a.proof
	return nil
}

func (c *cloudClaimAuthority) beforePool(ctx context.Context, tx *sql.Tx, token string, req dag.PoolClaimRequest) (*poolMachineProof, dag.PoolClaimAuthority, error) {
	if err := c.s.authorityWriter(ctx, tx); err != nil {
		return nil, dag.PoolClaimAuthority{}, err
	}
	identity, err := c.s.poolMachineIdentityTx(ctx, tx, token, req.RunnerID, false)
	if err != nil {
		return nil, dag.PoolClaimAuthority{}, err
	}
	if identity.runner.PoolID != req.PoolID || identity.runner.WorkerID != req.WorkerID {
		return nil, dag.PoolClaimAuthority{}, forbidden()
	}
	if len(req.Scopes) > 256 {
		return nil, dag.PoolClaimAuthority{}, cloudInvalid()
	}
	for _, local := range req.Scopes {
		if _, err = c.s.poolActiveScopeTx(ctx, tx, req.PoolID, local.TenantID, local.ProjectID); err != nil {
			return nil, dag.PoolClaimAuthority{}, err
		}
	}
	refs, err := dag.DiscoverPoolClaimCandidatesTx(ctx, tx, c.s.Dialect, req)
	if err != nil {
		return nil, dag.PoolClaimAuthority{}, err
	}
	participants, err := c.discover(ctx, tx, refs)
	if err != nil {
		return nil, dag.PoolClaimAuthority{}, err
	}
	proof, auth, err := c.s.authorizePoolRunnerTx(ctx, tx, token, req.RunnerID, req.Scopes, &participants)
	if err == nil {
		c.tx, c.pool = tx, proof
	}
	return proof, auth, err
}

// eligibleTx is the only exception to ordinary external/tool admission. The
// complete owner set has already been locked; no late identity locks are taken.
func (c *cloudClaimAuthority) eligibleTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (handled, allowed bool, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, c.s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, run, node).Scan(&raw)
	if err != nil {
		return false, false, err
	}
	var spec dag.Node
	if json.Unmarshal([]byte(raw), &spec) != nil {
		return false, false, cloudConflict()
	}
	if !dag.WorkBuddyCloudNode(spec) {
		return false, false, nil
	}
	if c.tx != tx || c.w == nil {
		return true, false, nil
	}
	key := cloudClaimKey(tenant, project, run, node)
	if a, ok := c.replays[key]; ok {
		err = c.validateAssignmentTx(ctx, tx, a, false)
		return true, err == nil, err
	}
	d, err := c.validateNodeTx(ctx, tx, tenant, project, run, node, nil, true)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Code != "authority_retry" && api.Code != "workbuddy_cloud_budget_unsupported" && api.Status >= 400 && api.Status < 500 {
			return true, false, nil
		}
		return true, false, err
	}
	b, err := d.AssignmentBinding()
	if err != nil {
		return true, false, cloudConflict()
	}
	c.pinned[key] = b
	return true, true, nil
}

func (c *cloudClaimAuthority) validateNodeTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string, binding *wc.CloudAssignmentBinding, fresh bool) (out cloudDelegationRow, err error) {
	if c.tx != tx || c.w == nil {
		return out, forbidden()
	}
	if c.private != nil {
		if c.private.target.tenantID != tenant || c.private.target.projectID != project {
			return out, forbidden()
		}
		if err = c.private.assertTx(tx, c.private.target, purposeRunnerWrite); err != nil {
			return out, err
		}
	} else if c.pool != nil {
		var found bool
		for _, scope := range c.pool.scopes {
			if scope.TenantID == tenant && scope.ProjectID == project {
				found = true
				err = c.pool.assertScope(tx, scope)
				break
			}
		}
		if !found || err != nil {
			if err != nil {
				return out, err
			}
			return out, forbidden()
		}
	} else {
		return out, forbidden()
	}
	var id, owner string
	query := `SELECT id,owner_id FROM suma_workbuddy_cloud_delegations WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`
	err = tx.QueryRowContext(ctx, c.s.q(c.s.currentSQL(query)), tenant, project, run, node).Scan(&id, &owner)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if !c.owners[owner] {
		return out, &APIError{409, "authority_retry", "权限集合已改变，请重试", nil}
	}
	out, err = c.w.cloudDelegationTx(ctx, tx, tenant, project, id, true)
	if err != nil {
		return out, err
	}
	if out.OwnerID != owner {
		return out, cloudConflict()
	}
	if _, err = c.s.currentProjectMembershipLockedTx(ctx, tx, tenant, owner, project, true, false); err != nil {
		return out, err
	}
	clock, err := c.s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	expiry, e := time.Parse(time.RFC3339Nano, out.ExpiresAt)
	if out.State != "active" || e != nil || !clock.Before(expiry) || out.MaxCreates != 1 {
		return out, cloudConflict()
	}
	if fresh && (out.CreatesConsumed != 0 || out.Version >= wc.MaxVersion) {
		return out, cloudConflict()
	}
	if binding != nil {
		current, e := out.AssignmentBinding()
		if e != nil || current.DelegationID != binding.DelegationID || current.DelegationSHA256 != binding.DelegationSHA256 {
			return out, cloudConflict()
		}
	}
	n, err := c.w.cloudNodeTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return out, err
	}
	if n.specHash != out.NodeSpecSHA256 || n.requestHash != out.RequestSHA256 || n.binding.ConnectionID != out.ConnectionID || n.binding.GrantGeneration != out.GrantGeneration || n.runState != "running" {
		return out, cloudConflict()
	}
	if _, err = c.w.cloudGrantMetadataTx(ctx, tx, tenant, project, owner, out.ConnectionID, out.GrantGeneration, out.ClientBindingSHA256, true); err != nil {
		return out, err
	}
	err = c.w.cloudBudgetTx(ctx, tx, tenant, project)
	return out, err
}

func (c *cloudClaimAuthority) validateAssignmentTx(ctx context.Context, tx *sql.Tx, a dag.Assignment, fresh bool) error {
	if a.WorkBuddyCloud == nil {
		return cloudConflict()
	}
	d, err := c.validateNodeTx(ctx, tx, a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.WorkBuddyCloud, fresh)
	if err != nil {
		return err
	}
	b := a.WorkBuddyCloud
	if d.CreatesConsumed == 0 {
		if d.Version != b.DelegationVersion {
			return cloudConflict()
		}
		return nil
	}
	if fresh || d.CreatesConsumed != 1 || d.Version != b.DelegationVersion+1 {
		return cloudConflict()
	}
	var id string
	err = tx.QueryRowContext(ctx, c.s.q(c.s.currentSQL(`SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND delegation_id=?`)), a.TenantID, a.ProjectID, d.ID).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cloudConflict()
		}
		return err
	}
	sub, err := c.w.cloudSubmissionTx(ctx, tx, a.TenantID, a.ProjectID, id, true)
	if err != nil {
		return err
	}
	snapshot, err := dag.ReadExternalAssignmentTx(ctx, tx, c.s.Dialect, a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt)
	if err != nil {
		return err
	}
	if !wc.SameAssignmentIdentity(sub.Assignment, snapshot.Assignment) || sub.DelegationID != d.ID || sub.RequestSHA256 != d.RequestSHA256 {
		return cloudConflict()
	}
	return nil
}

func (c *cloudClaimAuthority) runnableHook() *dag.RunnableHook {
	return &dag.RunnableHook{
		BindAssignmentTx: func(ctx context.Context, tx *sql.Tx, a *dag.Assignment) error {
			if !dag.WorkBuddyCloudNode(dag.Node{Type: a.Type, Config: a.Config, Command: a.Command}) {
				return nil
			}
			key := cloudClaimKey(a.TenantID, a.ProjectID, a.RunID, a.NodeID)
			b, ok := c.pinned[key]
			if !ok {
				return cloudConflict()
			}
			a.WorkBuddyCloud = &b
			return c.validateAssignmentTx(ctx, tx, *a, true)
		},
		ValidateReplayTx: func(ctx context.Context, tx *sql.Tx, a dag.Assignment) error {
			if a.DispatchState == "active" {
				if err := c.validateAssignmentTx(ctx, tx, a, false); err != nil {
					return err
				}
			}
			c.replays[cloudClaimKey(a.TenantID, a.ProjectID, a.RunID, a.NodeID)] = a
			return nil
		},
	}
}
