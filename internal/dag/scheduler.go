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
		return nil, dagDiagnostic(fmt.Errorf("%w: map config.items required", ErrInvalid), "dag.map.items_required", nil)
	}
	if err := decode(raw, &items); err != nil {
		return nil, dagDiagnostic(fmt.Errorf("%w: map config.items must be an array", ErrInvalid), "dag.map.items_array", nil)
	}
	return items, nil
}
func mapTemplate(n Node) (Node, error) {
	template := Node{Type: "task", Command: n.Command, Idempotent: n.Idempotent, Resources: n.Resources, TimeoutSeconds: n.TimeoutSeconds, MaxRetries: n.MaxRetries, Join: "all_success", Priority: n.Priority}
	if raw, ok := n.Config["template"]; ok {
		if err := decode(raw, &template); err != nil {
			return template, dagDiagnostic(fmt.Errorf("%w: invalid map template", ErrInvalid), "dag.map.template", nil)
		}
	}
	return template, nil
}

// SetResourceLimits configures per-project concurrent resources. The reserved
// name budget is a cumulative admission budget, charged once per attempt.
func (e *Engine) SetResourceLimits(ctx context.Context, tenant, project string, limits map[string]int64) error {
	return e.write(ctx, func(t *transaction) error { return t.mergeResourceLimits(tenant, project, limits) })
}
func (t *transaction) reserve(r *runState, n *nodeState) (bool, error) {
	return t.reservePolicyResources(r, n)
}
func (t *transaction) release(r *runState, n *nodeState) error {
	return t.releasePolicyResources(r, n, false)
}
func capable(node *nodeState, capabilities []string) bool {
	if len(capabilities) == 0 {
		return false
	}
	required, _ := node.spec.Config["capability"].(string)
	commandRequired := ""
	if node.Type == "command" || node.Type == "agent" || node.Type == "test" || node.Type == "terminal_session" {
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
	return e.ClaimWithOperation(ctx, tenant, project, worker, uid(), capabilities, before, eligible)
}

func (e *Engine) ClaimWithOperation(ctx context.Context, tenant, project, worker, operationID string, capabilities []string, before func(*sql.Tx) error, eligible func(*sql.Tx, string, string) (bool, error)) (*Assignment, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, err
	}
	if worker == "" || len(worker) > 100 {
		return nil, dagDiagnostic(fmt.Errorf("%w: worker_id required", ErrInvalid), "dag.claim.worker", nil)
	}
	if len(operationID) < 8 || len(operationID) > 128 {
		return nil, dagDiagnostic(fmt.Errorf("%w: claim operation requires 8..128 characters", ErrInvalid), "dag.claim.operation", nil)
	}
	capabilities, err := normalizedCapabilities(capabilities)
	if err != nil {
		return nil, err
	}
	requestHash := fingerprint(map[string]any{"worker_id": worker, "capabilities": capabilities})
	var out *Assignment
	err = e.writeBefore(ctx, before, func(t *transaction) error {
		if err := t.guardProject(tenant, project); err != nil {
			return err
		}
		previous, found, err := t.recoverDispatch(tenant, project, worker, operationID, requestHash)
		if err != nil {
			return err
		}
		if found {
			if previous != nil && previous.DispatchState == "active" && eligible != nil {
				allowed, err := eligible(t.tx, previous.RunID, previous.NodeID)
				if err != nil {
					return err
				}
				if !allowed {
					return dagDiagnostic(fmt.Errorf("%w: original dispatch is currently blocked; inspect its existing attempt", ErrConflict), "dag.claim.dispatch_blocked", nil)
				}
			}
			out = previous
			return nil
		}
		candidates, err := t.claimCandidateBatch(tenant, project, worker, capabilities)
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
			r, err := t.loadHot(tenant, project, c.run)
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
			seconds, err := t.attemptWindow(r, n, false)
			if err != nil {
				return err
			}
			if seconds <= 0 {
				if err = t.transition(r, n, "failed", "automatic retry deadline exhausted before dispatch"); err != nil {
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
			allowed, err := t.checkAttemptAccounting(r, n, worker)
			if err != nil {
				return err
			}
			if !allowed {
				continue
			}
			reserved, err := t.reserve(r, n)
			if err != nil {
				return err
			}
			if !reserved {
				continue
			}
			if err = t.reserveAttemptAccounting(r, n, worker); err != nil {
				return err
			}
			if seconds, err = t.attemptWindow(r, n, true); err != nil {
				return err
			}
			n.Attempt++
			n.Fence++
			n.WorkerID = worker
			n.Output, n.Error = "", ""
			n.WorkspaceEvidence, n.ArtifactEvidence, n.ToolEvidence = nil, nil, nil
			n.StartedAt = stamp(t.now)
			n.CompletedAt = ""
			n.lease = t.now + e.LeaseSeconds
			if seconds > 0 && n.lease > t.now+int64(seconds) {
				n.lease = t.now + int64(seconds)
			}
			if err = t.freezeCandidate(r, n, pin); err != nil {
				return err
			}
			if err = t.transition(r, n, "running", ""); err != nil {
				return err
			}
			claimSequence, err := t.nextQueueSequence(tenant, project)
			if err != nil {
				return err
			}
			if err = t.exec("DELETE FROM dag_run_fairness WHERE run_id=?", r.ID); err != nil {
				return err
			}
			if err = t.exec("INSERT INTO dag_run_fairness (run_id,last_claim_sequence) VALUES (?,?)", r.ID, claimSequence); err != nil {
				return err
			}
			if err = t.exec(`DELETE FROM dag_ready WHERE run_id=? AND node_id=?`, r.ID, n.ID); err != nil {
				return err
			}
			if err = t.exec("INSERT INTO dag_attempts (run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result) VALUES (?,?,?,?,?,?,?,?,?)", r.ID, n.ID, n.Attempt, n.Fence, worker, "running", n.StartedAt, "", ""); err != nil {
				return err
			}
			if err = t.save(r); err != nil {
				return err
			}
			if err = t.resetClaimScan(tenant, project, worker, capabilities); err != nil {
				return err
			}
			out = &Assignment{RunID: r.ID, NodeID: n.ID, Attempt: n.Attempt, Fence: n.Fence, WorkerID: worker, Command: n.spec.Command, Type: n.Type, TimeoutSeconds: seconds, Idempotent: n.spec.Idempotent, Config: n.spec.Config}
			if pin != nil {
				out.CandidateCommit = pin.CandidateCommit
				out.CandidateFrom = pin.CandidateFrom
				out.CandidateAttempt = pin.CandidateAttempt
				out.CandidateFence = pin.CandidateFence
				out.CandidateEvidenceSHA256 = pin.CandidateEvidenceSHA256
			}
			return t.recordDispatch(tenant, project, worker, operationID, requestHash, out, n.lease)
		}
		return t.recordDispatch(tenant, project, worker, operationID, requestHash, nil, 0)
	})
	return out, err
}
func (t *transaction) assigned(tenant, project string, a Assignment) (*runState, *nodeState, error) {
	if err := t.guardExecution(tenant, project, a.RunID); err != nil {
		return nil, nil, err
	}
	r, err := t.loadHot(tenant, project, a.RunID)
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
		remaining, err := t.attemptWindow(r, n, false)
		if err != nil {
			return err
		}
		if remaining <= 0 {
			return ErrStale
		}
		if n.lease > t.now+int64(remaining) {
			n.lease = t.now + int64(remaining)
		}
		if err = t.syncWake(r, n); err != nil {
			return err
		}
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
	if before == nil || commit == nil || (result.ReleaseEvidence == nil) == (result.IntegrationEvidence == nil) || result.ArtifactEvidence != nil || result.OutcomeUnknown || result.IntegrationEvidence != nil && (result.WorkspaceEvidence != nil || result.ToolEvidence != nil) {
		return dagDiagnostic(fmt.Errorf("%w: managed resolution requires authority and terminal provider evidence", ErrInvalid), "dag.result.managed_resolution", nil)
	}
	return e.completeBound(ctx, tenant, project, result, before, commit, true)
}

func (e *Engine) completeBound(ctx context.Context, tenant, project string, result Result, before func(*sql.Tx) error, commit func(*sql.Tx, string) error, managedResolution bool) error {
	if err := validateFailureClass(result.FailureClass); err != nil {
		return err
	}
	if len(result.Output) > 4*1024*1024 || len(result.Error) > 64*1024 {
		return dagDiagnostic(fmt.Errorf("%w: result too large", ErrInvalid), "dag.result.size", nil)
	}
	return e.writeBefore(ctx, before, func(t *transaction) error {
		if managedResolution {
			var count int
			if err := t.row("SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND run_id=?", tenant, project, result.RunID).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return dagDiagnostic(fmt.Errorf("%w: observation resolution is limited to service-managed runs", ErrInvalid), "dag.result.observation_scope", nil)
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
		r, err := t.loadHot(tenant, project, result.RunID)
		if err != nil {
			return err
		}
		n := r.find(result.NodeID)
		if n == nil || n.Attempt != result.Attempt || n.Fence != result.Fence || n.WorkerID != result.WorkerID {
			return ErrStale
		}
		if managedResolution {
			if n.State == "running" && n.lease > t.now {
				return dagDiagnostic(fmt.Errorf("%w: worker still has a current lease", ErrConflict), "dag.result.active_lease", nil)
			}
		}
		if n.State != "running" && n.State != "reconciling" {
			return ErrStale
		}
		liveResolution := n.State == "reconciling" && !result.Stopped && !managedResolution
		if liveResolution && !t.liveReadOnlyCompletionCandidate(r, n, result, before != nil && commit != nil) {
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
		if liveResolution {
			if err = t.reconcileLiveReadOnlyCompletion(r, n, result); err != nil {
				return err
			}
		}
		if managedResolution {
			var original string
			if err = t.row("SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?", r.ID, n.ID, n.Attempt).Scan(&original); err != nil {
				return err
			}
			if err = t.exec("INSERT INTO dag_managed_resolutions(run_id,node_id,attempt,operation_id,original_result,resolution_result,resolved_at)VALUES(?,?,?,?,?,?,?)", r.ID, n.ID, n.Attempt, result.OperationID, original, encoded(result), stamp(t.now)); err != nil {
				return err
			}
			providerSHA := ""
			if result.ReleaseEvidence != nil {
				providerSHA = result.ReleaseEvidence.EvidenceSHA256
			} else {
				providerSHA = result.IntegrationEvidence.ObservationSHA256
			}
			if err = t.event(r, n.ID, "managed_observation_reconciled", map[string]any{"attempt": n.Attempt, "previous_result_sha256": hashText(original), "provider_evidence_sha256": providerSHA, "resolution_operation_id": result.OperationID}); err != nil {
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
			if err = t.releasePolicyResources(r, n, result.Stopped && result.OutcomeUnknown); err != nil {
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
		if err = t.recordAttemptAccounting(r, n, result); err != nil {
			return err
		}
		if state == "failed" && r.State != "cancelling" {
			if _, err = t.scheduleTransientRetry(r, n, result); err != nil {
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
	report, err := e.TickBatch(ctx, 100)
	if err != nil {
		return err
	}
	if report.Failed > 0 {
		return dagDiagnostic(fmt.Errorf("scheduler wake processing failed for %d isolated runs", report.Failed), "dag.wake.isolated_failures", diagnosticParams{"count": report.Failed})
	}
	return nil
}
