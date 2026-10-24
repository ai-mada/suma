package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func decode(v any, out any) error { return json.Unmarshal([]byte(encoded(v)), out) }
func mapItems(n Node) ([]any, error) {
	var items []any
	raw, ok := n.Config["items"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("%w: map config.items required", ErrInvalid)
	}
	if err := decode(raw, &items); err != nil {
		return nil, fmt.Errorf("%w: map config.items must be an array", ErrInvalid)
	}
	return items, nil
}
func mapTemplate(n Node) (Node, error) {
	template := Node{Type: "task", Command: n.Command, Idempotent: n.Idempotent, Resources: n.Resources, TimeoutSeconds: n.TimeoutSeconds, MaxRetries: n.MaxRetries, Join: "all_success"}
	if raw, ok := n.Config["template"]; ok {
		if err := decode(raw, &template); err != nil {
			return template, fmt.Errorf("%w: invalid map template", ErrInvalid)
		}
	}
	return template, nil
}

// SetResourceLimits configures per-project concurrent resources. The reserved
// name budget is a cumulative admission budget, charged once per attempt.
func (e *Engine) SetResourceLimits(ctx context.Context, tenant, project string, limits map[string]int64) error {
	if err := scopeValid(tenant, project); err != nil {
		return err
	}
	return e.write(ctx, func(t *transaction) error {
		for name, capacity := range limits {
			if !identifier.MatchString(name) || capacity < 0 {
				return fmt.Errorf("%w: resource limit", ErrInvalid)
			}
			var old int64
			err := t.row("SELECT capacity FROM dag_resource_limits WHERE tenant_id=? AND project_id=? AND name=?", tenant, project, name).Scan(&old)
			if err == sql.ErrNoRows {
				err = t.exec("INSERT INTO dag_resource_limits (tenant_id,project_id,name,capacity,consumed) VALUES (?,?,?,?,0)", tenant, project, name, capacity)
			} else if err == nil {
				err = t.exec("UPDATE dag_resource_limits SET capacity=? WHERE tenant_id=? AND project_id=? AND name=?", capacity, tenant, project, name)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}
func (t *transaction) reserve(r *runState, n *nodeState) (bool, error) {
	for name, amount := range n.spec.Resources {
		var cap, consumed int64
		err := t.row("SELECT capacity,consumed FROM dag_resource_limits WHERE tenant_id=? AND project_id=? AND name=?", r.TenantID, r.ProjectID, name).Scan(&cap, &consumed)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return false, err
		}
		var used int64
		if name == "budget" {
			used = consumed
		} else {
			if err = t.row("SELECT COALESCE(SUM(x.amount),0) FROM dag_reservations x JOIN dag_runs r ON r.id=x.run_id WHERE r.tenant_id=? AND r.project_id=? AND x.name=?", r.TenantID, r.ProjectID, name).Scan(&used); err != nil {
				return false, err
			}
		}
		if amount > cap-used {
			return false, nil
		}
	}
	for name, amount := range n.spec.Resources {
		if name == "budget" {
			if err := t.exec("UPDATE dag_resource_limits SET consumed=consumed+? WHERE tenant_id=? AND project_id=? AND name=?", amount, r.TenantID, r.ProjectID, name); err != nil {
				return false, err
			}
		} else {
			if err := t.exec("INSERT INTO dag_reservations (run_id,node_id,name,amount) VALUES (?,?,?,?)", r.ID, n.ID, name, amount); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}
func (t *transaction) release(r *runState, n *nodeState) error {
	return t.exec("DELETE FROM dag_reservations WHERE run_id=? AND node_id=?", r.ID, n.ID)
}
func capable(node *nodeState, capabilities []string) bool {
	if len(capabilities) == 0 {
		return false
	}
	required, _ := node.spec.Config["capability"].(string)
	commandRequired := ""
	if node.Type == "command" || node.Type == "agent" || node.Type == "test" {
		if len(node.spec.Command) != 1 {
			return false
		}
		commandRequired = "command:" + node.spec.Command[0]
	}
	if node.Type == "external_workflow" {
		plugin, _ := node.spec.Config["plugin"].(string)
		required = "external_workflow:" + plugin
	}
	if required == "" && commandRequired == "" {
		required = node.Type
	}
	has := func(value string) bool {
		if value == "" {
			return true
		}
		for _, s := range capabilities {
			if s == value {
				return true
			}
		}
		return false
	}
	return has(required) && has(commandRequired)
}
func (e *Engine) Claim(ctx context.Context, tenant, project, worker string, capabilities []string) (*Assignment, error) {
	return e.ClaimBound(ctx, tenant, project, worker, capabilities, nil)
}
func (e *Engine) ClaimBound(ctx context.Context, tenant, project, worker string, capabilities []string, before func(*sql.Tx) error) (*Assignment, error) {
	return e.ClaimEligible(ctx, tenant, project, worker, capabilities, before, nil)
}

// Service-owned eligibility is evaluated before acquiring an attempt/resource.
// Self-reported worker capabilities never grant managed deployment authority.
func (e *Engine) ClaimEligible(ctx context.Context, tenant, project, worker string, capabilities []string, before func(*sql.Tx) error, eligible func(*sql.Tx, string, string) (bool, error)) (*Assignment, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, err
	}
	if worker == "" || len(worker) > 100 {
		return nil, fmt.Errorf("%w: worker_id required", ErrInvalid)
	}
	var out *Assignment
	err := e.writeBefore(ctx, before, func(t *transaction) error {
		rows, err := t.rows("SELECT q.run_id,q.node_id FROM dag_ready q JOIN dag_runs r ON r.id=q.run_id LEFT JOIN dag_run_fairness f ON f.run_id=q.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.state='running' ORDER BY COALESCE(f.last_claim_sequence,0),q.sequence,q.run_id,q.node_id", tenant, project)
		if err != nil {
			return err
		}
		type candidate struct{ run, node string }
		candidates := []candidate{}
		for rows.Next() {
			var c candidate
			if err = rows.Scan(&c.run, &c.node); err != nil {
				rows.Close()
				return err
			}
			candidates = append(candidates, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, c := range candidates {
			if eligible != nil {
				ok, err := eligible(t.tx, c.run, c.node)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
			}
			r, err := load(ctx, t.tx, e, tenant, project, c.run)
			if err != nil {
				return err
			}
			n := r.find(c.node)
			if n == nil || n.State != "ready" {
				continue
			}
			if !capable(n, capabilities) {
				continue
			}
			pin, err := candidateFor(r, n)
			if err != nil {
				if errors.Is(err, errCandidatePending) {
					if n.Reason != "等待候选源完成并固定提交" {
						n.Reason = "等待候选源完成并固定提交"
						if err = t.event(r, n.ID, "candidate_waiting", map[string]any{"source_node": n.spec.Config["candidate_from"]}); err != nil {
							return err
						}
						if err = t.save(r); err != nil {
							return err
						}
					}
					continue
				}
				if err = t.transition(r, n, "failed", "候选代码缺少当前尝试的干净提交证据，未派发测试"); err != nil {
					return err
				}
				if err = t.advance(r); err != nil {
					return err
				}
				if err = t.save(r); err != nil {
					return err
				}
				continue
			}
			reserved, err := t.reserve(r, n)
			if err != nil {
				return err
			}
			if !reserved {
				continue
			}
			n.Attempt++
			n.Fence++
			n.WorkerID = worker
			n.StartedAt = stamp(t.now)
			n.CompletedAt = ""
			n.lease = t.now + e.LeaseSeconds
			if err = t.freezeCandidate(r, n, pin); err != nil {
				return err
			}
			if err = t.transition(r, n, "running", ""); err != nil {
				return err
			}
			var claimSequence int64
			if err = t.row("SELECT seq FROM dag_lock WHERE id=1").Scan(&claimSequence); err != nil {
				return err
			}
			if err = t.exec("DELETE FROM dag_run_fairness WHERE run_id=?", r.ID); err != nil {
				return err
			}
			if err = t.exec("INSERT INTO dag_run_fairness (run_id,last_claim_sequence) VALUES (?,?)", r.ID, claimSequence); err != nil {
				return err
			}
			if err = t.exec("DELETE FROM dag_ready WHERE run_id=? AND node_id=?", r.ID, n.ID); err != nil {
				return err
			}
			if err = t.exec("INSERT INTO dag_attempts (run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result) VALUES (?,?,?,?,?,?,?,?,?)", r.ID, n.ID, n.Attempt, n.Fence, worker, "running", n.StartedAt, "", ""); err != nil {
				return err
			}
			if err = t.save(r); err != nil {
				return err
			}
			out = &Assignment{RunID: r.ID, NodeID: n.ID, Attempt: n.Attempt, Fence: n.Fence, WorkerID: worker, Command: n.spec.Command, Type: n.Type, TimeoutSeconds: n.spec.TimeoutSeconds, Idempotent: n.spec.Idempotent, Config: n.spec.Config}
			if pin != nil {
				out.CandidateCommit = pin.CandidateCommit
				out.CandidateFrom = pin.CandidateFrom
				out.CandidateAttempt = pin.CandidateAttempt
				out.CandidateFence = pin.CandidateFence
				out.CandidateEvidenceSHA256 = pin.CandidateEvidenceSHA256
			}
			return nil
		}
		return nil
	})
	return out, err
}
func (t *transaction) assigned(tenant, project string, a Assignment) (*runState, *nodeState, error) {
	r, err := load(t.ctx, t.tx, t.e, tenant, project, a.RunID)
	if err != nil {
		return nil, nil, err
	}
	n := r.find(a.NodeID)
	if n == nil || n.Attempt != a.Attempt || n.Fence != a.Fence || n.WorkerID != a.WorkerID || n.State != "running" || n.lease <= t.now {
		return nil, nil, ErrStale
	}
	return r, n, nil
}
func (e *Engine) Heartbeat(ctx context.Context, tenant, project string, a Assignment) error {
	return e.HeartbeatWithAuthority(ctx, tenant, project, a, nil)
}

// HeartbeatWithAuthority binds a lease extension to current service identity
// under the same transaction and organization-before-scheduler lock order.
func (e *Engine) HeartbeatWithAuthority(ctx context.Context, tenant, project string, a Assignment, before func(*sql.Tx) error) error {
	return e.writeBefore(ctx, before, func(t *transaction) error {
		r, n, err := t.assigned(tenant, project, a)
		if err != nil {
			return err
		}
		if r.State == "cancelling" {
			return ErrCancelled
		}
		n.lease = t.now + e.LeaseSeconds
		return t.exec("UPDATE dag_nodes SET lease_until=? WHERE run_id=? AND id=?", n.lease, r.ID, n.ID)
	})
}
func (e *Engine) Complete(ctx context.Context, tenant, project string, result Result) error {
	return e.CompleteBound(ctx, tenant, project, result, nil, nil)
}

// CompleteBound commits trusted service proofs with the final attempt. before
// runs before dag_lock (organization authority lock order); commit runs after
// the persisted final attempt, before activation and the operation receipt.
// Both hooks must use tx only and must not call Engine or perform network IO.
func (e *Engine) CompleteBound(ctx context.Context, tenant, project string, result Result, before func(*sql.Tx) error, commit func(*sql.Tx, string) error) error {
	return e.completeBound(ctx, tenant, project, result, before, commit, false)
}

// CompleteManagedObservedBound resolves an expired managed attempt using a
// service-verified immutable provider receipt. It is never a worker endpoint.
// The authority and observation hooks are mandatory; the old attempt result is
// retained separately instead of pretending a lost worker sent a stop receipt.
func (e *Engine) CompleteManagedObservedBound(ctx context.Context, tenant, project string, result Result, before func(*sql.Tx) error, commit func(*sql.Tx, string) error) error {
	if before == nil || commit == nil || result.ReleaseEvidence == nil || result.ArtifactEvidence != nil || result.OutcomeUnknown {
		return fmt.Errorf("%w: managed resolution requires authority and terminal provider evidence", ErrInvalid)
	}
	return e.completeBound(ctx, tenant, project, result, before, commit, true)
}

func (e *Engine) completeBound(ctx context.Context, tenant, project string, result Result, before func(*sql.Tx) error, commit func(*sql.Tx, string) error, managedResolution bool) error {
	if len(result.Output) > 4*1024*1024 || len(result.Error) > 64*1024 {
		return fmt.Errorf("%w: result too large", ErrInvalid)
	}
	return e.writeBefore(ctx, before, func(t *transaction) error {
		if managedResolution {
			var count int
			if err := t.row("SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND run_id=?", tenant, project, result.RunID).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("%w: observation resolution is limited to service-managed runs", ErrInvalid)
			}
		}
		hash := fingerprint(result)
		scope := "result:" + result.RunID
		previous, err := t.operation(tenant, project, scope, result.OperationID, hash)
		if err != nil {
			return err
		}
		if previous != "" {
			return nil
		}
		r, err := load(ctx, t.tx, e, tenant, project, result.RunID)
		if err != nil {
			return err
		}
		n := r.find(result.NodeID)
		if n == nil || n.Attempt != result.Attempt || n.Fence != result.Fence || n.WorkerID != result.WorkerID {
			return ErrStale
		}
		if managedResolution {
			if n.State == "running" && n.lease > t.now {
				return fmt.Errorf("%w: worker still has a current lease", ErrConflict)
			}
		}
		if n.State != "running" && n.State != "reconciling" {
			return ErrStale
		}
		if n.State == "reconciling" && !result.Stopped && !managedResolution {
			return ErrStale
		}
		if n.State == "running" && n.lease <= t.now && !result.Stopped && !managedResolution {
			return ErrStale
		}
		if err = t.validateCandidateResult(r, n, result); err != nil {
			return err
		}
		if err = validateToolResult(n.spec, result); err != nil {
			return err
		}
		if managedResolution {
			var original string
			if err = t.row("SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?", r.ID, n.ID, n.Attempt).Scan(&original); err != nil {
				return err
			}
			if err = t.exec("INSERT INTO dag_managed_resolutions(run_id,node_id,attempt,operation_id,original_result,resolution_result,resolved_at)VALUES(?,?,?,?,?,?,?)", r.ID, n.ID, n.Attempt, result.OperationID, original, encoded(result), stamp(t.now)); err != nil {
				return err
			}
			if err = t.event(r, n.ID, "managed_observation_reconciled", map[string]any{"attempt": n.Attempt, "previous_result_sha256": hashText(original), "provider_evidence_sha256": result.ReleaseEvidence.EvidenceSHA256, "resolution_operation_id": result.OperationID}); err != nil {
				return err
			}
		}
		n.Output = result.Output
		n.Error = result.Error
		state := "succeeded"
		if result.ExitCode != 0 || result.Error != "" {
			state = "failed"
		}
		if result.Stopped && !result.OutcomeUnknown && r.State != "cancelling" {
			// A confirmed stop cannot prove successful execution, regardless
			// of OS exit or absent/failed/completed protocol evidence. A
			// pending cancellation is resolved separately below.
			state = "failed"
			if n.Error == "" {
				n.Error = "execution process stopped before verified completion"
			}
		}
		if result.OutcomeUnknown {
			state = "reconciling"
		}
		if r.State == "cancelling" && result.Stopped && !result.OutcomeUnknown {
			state = "cancelled"
		}
		if err = t.transition(r, n, state, n.Error); err != nil {
			return err
		}
		if terminal(state) || result.Stopped {
			if err = t.release(r, n); err != nil {
				return err
			}
		}
		if err = t.exec("UPDATE dag_attempts SET state=?,completed_at=?,result=? WHERE run_id=? AND node_id=? AND attempt=?", state, stamp(t.now), encoded(result), r.ID, n.ID, n.Attempt); err != nil {
			return err
		}
		if commit != nil {
			if err = commit(t.tx, state); err != nil {
				return err
			}
		}
		if state == "failed" && r.State != "cancelling" && n.spec.Idempotent && n.Attempt <= n.spec.MaxRetries {
			n.State = "pending"
			n.CompletedAt = ""
			n.WorkerID = ""
			if err = t.event(r, n.ID, "retry_scheduled", map[string]any{"attempt": n.Attempt + 1}); err != nil {
				return err
			}
		}
		if err = t.advance(r); err != nil {
			return err
		}
		if err = t.save(r); err != nil {
			return err
		}
		return t.remember(tenant, project, scope, result.OperationID, hash, r.ID)
	})
}
func (e *Engine) Tick(ctx context.Context) error {
	return e.write(ctx, func(t *transaction) error {
		rows, err := t.rows("SELECT tenant_id,project_id,id FROM dag_runs WHERE state IN ('running','paused','cancelling')")
		if err != nil {
			return err
		}
		type key struct{ tenant, project, id string }
		keys := []key{}
		for rows.Next() {
			var k key
			if err = rows.Scan(&k.tenant, &k.project, &k.id); err != nil {
				rows.Close()
				return err
			}
			keys = append(keys, k)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, k := range keys {
			r, err := load(ctx, t.tx, e, k.tenant, k.project, k.id)
			if err != nil {
				return err
			}
			changed := false
			for _, n := range r.nodes {
				if n.State == "running" && n.lease <= t.now {
					if err = t.transition(r, n, "reconciling", "lease expired; external outcome unknown"); err != nil {
						return err
					}
					if err = t.exec("UPDATE dag_attempts SET state='reconciling' WHERE run_id=? AND node_id=? AND attempt=?", r.ID, n.ID, n.Attempt); err != nil {
						return err
					}
					changed = true
				}
				if n.State == "waiting" && n.Type == "timer" && n.wake <= t.now {
					changed = true
				}
				if n.State == "pending" && n.wake > 0 && n.wake <= t.now {
					changed = true
				}
				if n.State == "waiting" && n.Type == "subworkflow" && n.ChildRunID != "" {
					var childState string
					if err = t.row("SELECT state FROM dag_runs WHERE id=?", n.ChildRunID).Scan(&childState); err != nil {
						return err
					}
					changed = changed || terminal(childState) || n.wake > 0 && n.wake <= t.now
				}
			}
			if changed {
				if err = t.advance(r); err != nil {
					return err
				}
				if err = t.save(r); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
