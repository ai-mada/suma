package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type workspaceObservationBasis struct {
	Scope                        woc.Scope
	Assignment                   dag.Assignment
	Profile                      ToolProfile
	PoolOwner, RunName, NodeName string
}
type workspaceObservationSubject struct {
	Actor                          Actor
	Pool                           bool
	Token, Tenant, Project, Runner string
	Assignment                     dag.Assignment
}

func workspaceObservationSameScope(a dag.Assignment, s woc.Scope) bool {
	return a.RunID == s.RunID && a.NodeID == s.NodeID && a.Attempt == s.Attempt && a.Fence == s.Fence && a.WorkerID == s.WorkerID
}

// Discovery reads only scope metadata before taking globally ordered current
// authority locks. It grants no file access and returns no workspace roots.
func (s *Service) workspaceObservationBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, scope woc.Scope) (workspaceObservationBasis, error) {
	out := workspaceObservationBasis{Scope: scope}
	if woc.ValidateScope(scope) != nil {
		return out, invalid(workspaceObservationInvalid)
	}
	var nodeState, runState, worker, raw string
	var attempt int
	var fence, lease int64
	err := tx.QueryRowContext(ctx, s.q(`SELECT n.state,r.state,n.worker_id,n.attempt,n.fence,n.lease_until,n.spec,r.workflow_name FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, scope.RunID, scope.NodeID).Scan(&nodeState, &runState, &worker, &attempt, &fence, &lease, &raw, &out.RunName)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	if nodeState != "running" || (runState != "running" && runState != "paused") || worker != scope.WorkerID || attempt != scope.Attempt || fence != scope.Fence || lease <= clock {
		return out, conflict(workspaceObservationUnavailable)
	}
	var node dag.Node
	if err = json.Unmarshal([]byte(raw), &node); err != nil {
		return out, err
	}
	out.NodeName = node.Name
	id, sha, bound, err := dag.ToolBinding(node)
	if err != nil || !bound || id != scope.ToolProfileID || sha != scope.ProfileSHA256 || node.Type != "agent" {
		return out, conflict(workspaceObservationUnavailable)
	}
	out.Profile, err = s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, id)
	if err != nil {
		return out, err
	}
	p := out.Profile
	if p.WorkerID != worker || p.Profile.ProfileSHA256 != sha || p.Profile.CommandID != node.Command[0] || p.Profile.Adapter != "suma" || p.Profile.Protocol != "suma-jsonl-v1" || p.Profile.WorkspaceToolsVersion != "attempt-workspace-v1" || p.Profile.ReadOnly {
		return out, conflict(workspaceObservationUnavailable)
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, scope.RunID)
	if err != nil {
		return out, err
	}
	out.Assignment = dag.Assignment{RunID: scope.RunID, NodeID: scope.NodeID, Attempt: scope.Attempt, Fence: scope.Fence, WorkerID: scope.WorkerID}
	if route.PoolID == "" {
		return out, nil
	}
	var runnerID, slotID string
	err = tx.QueryRowContext(ctx, s.q(`SELECT runner_id,slot_id FROM dag_execution_pool_slots WHERE pool_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), route.PoolID, scope.RunID, scope.NodeID, scope.Attempt, scope.Fence).Scan(&runnerID, &slotID)
	if err != nil {
		return out, conflict(workspaceObservationUnavailable)
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT assignment FROM dag_execution_pool_dispatches WHERE pool_id=? AND runner_id=? AND assignment LIKE ? ORDER BY created_at DESC LIMIT 1001`), route.PoolID, runnerID, `%"run_id":"`+scope.RunID+`"%`)
	if err != nil {
		return out, err
	}
	count := 0
	found := false
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return out, err
		}
		count++
		var a dag.Assignment
		if json.Unmarshal([]byte(raw), &a) == nil && workspaceObservationSameScope(a, scope) && a.TenantID == tenant && a.ProjectID == project && a.PoolID == route.PoolID && a.PoolRunnerID == runnerID && a.PoolSlotID == slotID {
			if found {
				rows.Close()
				return out, conflict(workspaceObservationUnavailable)
			}
			out.Assignment = a
			found = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if count > 1000 || !found {
		return out, conflict(workspaceObservationUnavailable)
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT owner_tenant_id FROM suma_execution_pools WHERE id=?`), route.PoolID).Scan(&out.PoolOwner); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) workspaceObservationLockActorsTx(ctx context.Context, tx *sql.Tx, actors []Actor, users, organizations []string) error {
	var keys []string
	for _, a := range actors {
		users = append(users, a.credential.subjectID)
		keys = append(keys, credentialLockKeys(a.credential)...)
	}
	users = sortedAuthorityIDs(users)
	for _, u := range users {
		orgs, err := s.authorityOrganizations(ctx, tx, u)
		if err != nil {
			return err
		}
		organizations = append(organizations, orgs...)
	}
	return s.lockHumanAuthorities(ctx, tx, users, organizations, keys)
}
func (s *Service) workspaceObservationBasisUsersTx(ctx context.Context, tx *sql.Tx, tenant, project string, b workspaceObservationBasis) ([]string, error) {
	users := []string{b.Profile.EnabledBy}
	if b.Assignment.PoolID == "" {
		var creator string
		err := tx.QueryRowContext(ctx, s.q(`SELECT created_by FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=?`), tenant, project, b.Scope.WorkerID).Scan(&creator)
		if err != nil {
			return nil, forbidden()
		}
		users = append(users, creator)
	}
	return users, nil
}
func parseWorkspaceObservationTime(value string) (int64, error) {
	at, err := time.Parse(time.RFC3339Nano, value)
	return at.Unix(), err
}

func (s *Service) validateWorkspaceObservationBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, b workspaceObservationBasis) error {
	if err := dag.GuardProjectTx(ctx, tx, s.Dialect, tenant, project); err != nil {
		return err
	}
	current, err := s.workspaceObservationBasisTx(ctx, tx, tenant, project, b.Scope)
	if err != nil {
		return err
	}
	if current.Assignment.PoolID != b.Assignment.PoolID || current.PoolOwner != b.PoolOwner {
		return conflict(workspaceObservationUnavailable)
	}
	if b.Assignment.PoolID != "" {
		if err = dag.ValidatePoolAssignmentTx(ctx, tx, s.Dialect, b.Assignment); err != nil {
			return conflict(workspaceObservationUnavailable)
		}
		scope, e := s.currentPoolActiveScopeTx(ctx, tx, b.Assignment.PoolID, tenant, project)
		if e != nil {
			return e
		}
		if scope.GrantID != b.Assignment.PoolGrantID || scope.GrantVersion != b.Assignment.PoolGrantVersion {
			return forbidden()
		}
		var poolStatus, runnerStatus, credentialStatus, credentialID, expires string
		var version, currentVersion int
		err = tx.QueryRowContext(ctx, s.q(`SELECT p.status,r.status,c.status,c.id,c.version,r.credential_version,c.expires_at FROM suma_execution_pools p JOIN suma_execution_pool_runners r ON r.pool_id=p.id JOIN suma_execution_pool_credentials c ON c.runner_id=r.id WHERE p.id=? AND r.id=? AND r.worker_id=?`), b.Assignment.PoolID, b.Assignment.PoolRunnerID, b.Scope.WorkerID).Scan(&poolStatus, &runnerStatus, &credentialStatus, &credentialID, &version, &currentVersion, &expires)
		if err != nil {
			return forbidden()
		}
		clock, e := s.automationClock(ctx, tx)
		if e != nil {
			return e
		}
		until, e := parseWorkspaceObservationTime(expires)
		if e != nil || poolStatus != "active" || runnerStatus != "active" || credentialStatus != "active" || credentialID != b.Assignment.PoolCredentialID || version != currentVersion || version != b.Assignment.PoolCredentialVersion || until <= clock.Unix() {
			return forbidden()
		}
	}
	profile, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, current.Profile)
	if err != nil {
		return err
	}
	if !profile.Available {
		return conflict(workspaceObservationUnavailable)
	}
	return nil
}

func (s *Service) workspaceObservationCallerActorTx(ctx context.Context, tx *sql.Tx, r workspaceObservationRecord) (Actor, error) {
	c, err := s.readCredential(ctx, tx, r.Caller.Digest, false)
	if err != nil {
		return Actor{}, err
	}
	if c.kind != r.Caller.Kind || c.ref != r.Caller.ref() || (c.kind != actorBrowser && c.kind != actorProjectAPI) {
		return Actor{}, forbidden()
	}
	return Actor{kind: c.kind, credential: c.ref, projection: c.p}, nil
}

func (s *Service) workspaceObservationMachineAuthorityTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, b workspaceObservationBasis, r *workspaceObservationRecord) error {
	actors := []Actor{}
	var caller Actor
	var err error
	if r != nil {
		caller, err = s.workspaceObservationCallerActorTx(ctx, tx, *r)
		if err != nil {
			return err
		}
		actors = append(actors, caller)
	}
	if !subject.Pool {
		actors = append(actors, subject.Actor)
	}
	users, err := s.workspaceObservationBasisUsersTx(ctx, tx, subject.Tenant, subject.Project, b)
	if err != nil {
		return err
	}
	if err = s.workspaceObservationLockActorsTx(ctx, tx, actors, users, []string{subject.Tenant, b.PoolOwner}); err != nil {
		return err
	}
	if r != nil {
		if _, err = s.authorizeTxWithParticipants(ctx, tx, caller, commandTarget{tenantID: subject.Tenant, projectID: subject.Project}, purposeProjectRead, []string{b.PoolOwner}); err != nil {
			return err
		}
	}
	if subject.Pool {
		if subject.Assignment.PoolID == "" || subject.Assignment.PoolRunnerID != subject.Runner || subject.Assignment.DispatchID != b.Assignment.DispatchID || subject.Assignment.PoolGrantID != b.Assignment.PoolGrantID || subject.Assignment.PoolGrantVersion != b.Assignment.PoolGrantVersion || !workspaceObservationSameScope(subject.Assignment, b.Scope) {
			return forbidden()
		}
		proof, e := s.PoolRunnerAssignmentAuthorityTx(ctx, tx, subject.Token, subject.Assignment)
		if e != nil {
			return e
		}
		if e = proof.assertAssignment(tx, subject.Assignment); e != nil {
			return e
		}
	} else {
		if b.Assignment.PoolID != "" {
			return forbidden()
		}
		auth, e := s.runnerIdentityTx(ctx, tx, subject.Actor, subject.Project, b.Scope.WorkerID)
		if e != nil {
			return e
		}
		if e = s.projectRunnerExecutionAuthorityTx(ctx, tx, auth, b.Scope.WorkerID, b.Scope.RunID); e != nil {
			return e
		}
	}
	return s.validateWorkspaceObservationBasisTx(ctx, tx, subject.Tenant, subject.Project, b)
}
