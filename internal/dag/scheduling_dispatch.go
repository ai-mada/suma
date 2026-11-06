package dag

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxClaimCapabilities = 256

func normalizedCapabilities(values []string) ([]string, error) {
	if len(values) > MaxClaimCapabilities {
		return nil, dagDiagnostic(fmt.Errorf("%w: too many capabilities", ErrInvalid), "dag.claim.capability_count", nil)
	}
	set := map[string]bool{}
	for _, v := range values {
		if v == "" || len(v) > 256 || !utf8.ValidString(v) || strings.IndexFunc(v, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return nil, dagDiagnostic(fmt.Errorf("%w: invalid capability", ErrInvalid), "dag.claim.capability", nil)
		}
		set[v] = true
	}
	out := []string{}
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out, nil
}

func (t *transaction) recoverDispatch(tenant, project, worker, operationID, hash string) (*Assignment, bool, error) {
	var id, stored, owner, raw string
	err := t.row(`SELECT id,request_sha256,worker_id,assignment FROM dag_dispatches WHERE tenant_id=? AND project_id=? AND operation_id=?`, tenant, project, operationID).Scan(&id, &stored, &owner, &raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if stored != hash || owner != worker {
		return nil, false, dagDiagnostic(fmt.Errorf("%w: claim identity or capabilities changed", ErrConflict), "dag.claim.changed", nil)
	}
	if raw == "null" || raw == "" {
		return nil, true, nil
	}
	var a Assignment
	if err = json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, false, err
	}
	a.DispatchID = id
	a.DispatchState = "closed"
	// Preserve the original dispatch lease as historical observation when the
	// current node/attempt no longer exists. Closed never grants execution.
	var attempt int
	var fence, lease int64
	var currentWorker, nodeState, runState string
	err = t.row(`SELECT n.attempt,n.fence,n.worker_id,n.state,n.lease_until,r.state FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`, tenant, project, a.RunID, a.NodeID).Scan(&attempt, &fence, &currentWorker, &nodeState, &lease, &runState)
	if err != nil && err != sql.ErrNoRows {
		return nil, false, err
	}
	if err == nil && attempt == a.Attempt && fence == a.Fence && currentWorker == worker {
		if nodeState == "reconciling" || nodeState == "running" && (lease <= t.now || runState == "cancelling") {
			a.DispatchState = "reconciling"
		} else if nodeState == "running" {
			a.DispatchState = "active"
			var deadline int64
			err = t.row(`SELECT w.deadline_at FROM dag_retry_windows w JOIN dag_node_dependencies d ON d.run_id=w.run_id AND d.node_id=w.node_id AND d.generation=w.generation WHERE w.run_id=? AND w.node_id=?`, a.RunID, a.NodeID).Scan(&deadline)
			if err != nil && err != sql.ErrNoRows {
				return nil, false, err
			}
			if err == nil {
				if deadline <= t.now {
					a.DispatchState = "reconciling"
				} else {
					a.TimeoutSeconds = int(deadline - t.now)
				}
			}
		}
		if lease > 0 {
			a.LeaseUntil = stamp(lease)
		}
	}
	return &a, true, nil
}

func (t *transaction) recordDispatch(tenant, project, worker, operationID, hash string, a *Assignment, lease int64, hook *RunnableHook) error {
	id := uid()
	run, node := "", ""
	attempt := 0
	var fence int64
	if a != nil {
		a.TenantID, a.ProjectID = tenant, project
		a.DispatchID = id
		a.DispatchState = "active"
		a.LeaseUntil = stamp(lease)
		run, node, attempt, fence = a.RunID, a.NodeID, a.Attempt, a.Fence
	}
	if err := t.bindRunnable(a, hook); err != nil {
		return err
	}
	var version int
	err := t.row(`SELECT version FROM dag_scheduling_policies WHERE tenant_id=? AND project_id=?`, tenant, project).Scan(&version)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err = t.exec(`INSERT INTO dag_dispatches(id,tenant_id,project_id,operation_id,request_sha256,worker_id,run_id,node_id,attempt,fence,policy_version,assignment,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, id, tenant, project, operationID, hash, worker, run, node, attempt, fence, version, encoded(a), stamp(t.now)); err != nil {
		return err
	}
	if hook != nil && hook.RecordPrivateClaimTx != nil {
		if err = hook.RecordPrivateClaimTx(t.ctx, t.tx); err != nil {
			return err
		}
	}
	return t.recordExternalAssignment(a)
}
