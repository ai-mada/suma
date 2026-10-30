package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

type ExternalAssignment struct {
	RunID, NodeID                  string
	Attempt                        int
	Fence                          int64
	WorkerID, DispatchID, Protocol string
	Binding                        wc.CloudAssignmentBinding
	Assignment                     cc.AssignmentRef
	AssignmentSHA256               string
}

func externalAssignmentRef(a Assignment) cc.AssignmentRef {
	if a.PoolID == "" {
		return cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	}
	return cc.AssignmentRef{TenantID: a.TenantID, ProjectID: a.ProjectID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
}

func (t *transaction) recordExternalAssignment(a *Assignment) error {
	if a == nil || a.WorkBuddyCloud == nil {
		return nil
	}
	ref := externalAssignmentRef(*a)
	if !ref.Valid() || a.TenantID == "" || a.ProjectID == "" || a.DispatchID == "" || a.DispatchState != "active" {
		return ErrConflict
	}
	b := a.WorkBuddyCloud
	return t.exec(`INSERT INTO dag_external_assignments(tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,dispatch_id,protocol,delegation_id,delegation_version,delegation_sha256,assignment,assignment_sha256)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, a.TenantID, a.ProjectID, a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID, a.DispatchID, wc.Protocol, b.DelegationID, b.DelegationVersion, b.DelegationSHA256, encoded(ref), fingerprint(ref))
}

// ReadExternalAssignmentTx reads the exact original carrier, including its
// original lease observation. It grants no execution authority and takes no locks.
func ReadExternalAssignmentTx(ctx context.Context, tx *sql.Tx, dialect, tenant, project, run, node string, attempt int) (out ExternalAssignment, err error) {
	if tx == nil || scopeValid(tenant, project) != nil || attempt < 1 {
		return out, ErrInvalid
	}
	e := &Engine{dialect: dialect}
	var raw string
	err = tx.QueryRowContext(ctx, e.q(`SELECT run_id,node_id,attempt,fence,worker_id,dispatch_id,protocol,delegation_id,delegation_version,delegation_sha256,assignment,assignment_sha256 FROM dag_external_assignments WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=? AND attempt=?`), tenant, project, run, node, attempt).Scan(&out.RunID, &out.NodeID, &out.Attempt, &out.Fence, &out.WorkerID, &out.DispatchID, &out.Protocol, &out.Binding.DelegationID, &out.Binding.DelegationVersion, &out.Binding.DelegationSHA256, &raw, &out.AssignmentSHA256)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out.Assignment) != nil || !out.Assignment.Valid() || raw != encoded(out.Assignment) || fingerprint(out.Assignment) != out.AssignmentSHA256 || out.Binding.Validate() != nil || out.Protocol != wc.Protocol {
		return ExternalAssignment{}, ErrConflict
	}
	a := out.Assignment
	if a.RunID != run || a.NodeID != node || a.Attempt != attempt || out.RunID != run || out.NodeID != node || out.Attempt != attempt || a.Fence != out.Fence || a.WorkerID != out.WorkerID {
		return ExternalAssignment{}, ErrConflict
	}
	if a.PoolID != "" && (a.TenantID != tenant || a.ProjectID != project || a.DispatchID != out.DispatchID || a.DispatchState != "active" || a.LeaseUntil == "") {
		return ExternalAssignment{}, ErrConflict
	}
	query := `SELECT assignment FROM dag_dispatches WHERE id=? AND tenant_id=? AND project_id=?`
	args := []any{out.DispatchID, tenant, project}
	if a.PoolID != "" {
		query = `SELECT assignment FROM dag_execution_pool_dispatches WHERE dispatch_id=? AND pool_id=? AND runner_id=?`
		args = []any{out.DispatchID, a.PoolID, a.PoolRunnerID}
	}
	var backingRaw string
	if err = tx.QueryRowContext(ctx, e.q(query), args...).Scan(&backingRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ExternalAssignment{}, ErrConflict
		}
		return ExternalAssignment{}, err
	}
	var backing Assignment
	if json.Unmarshal([]byte(backingRaw), &backing) != nil || backing.WorkBuddyCloud == nil || *backing.WorkBuddyCloud != out.Binding || externalAssignmentRef(backing) != a || !cloudAssignment(backing) || backing.TenantID != tenant || backing.ProjectID != project || backing.DispatchID != out.DispatchID {
		return ExternalAssignment{}, ErrConflict
	}
	return out, nil
}
