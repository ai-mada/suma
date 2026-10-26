package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

func poolScopeKey(tenant, project string) string { return fingerprint([]string{tenant, project}) }
func normalizePoolRequest(in PoolClaimRequest) (PoolClaimRequest, error) {
	if !identifier.MatchString(in.PoolID) || !identifier.MatchString(in.RunnerID) || !identifier.MatchString(in.WorkerID) || len(in.OperationID) < 8 || len(in.OperationID) > 128 || len(in.Scopes) == 0 || len(in.Scopes) > 256 {
		return in, ErrInvalid
	}
	slot, err := strconv.Atoi(in.SlotID)
	if err != nil || slot < 0 || strconv.Itoa(slot) != in.SlotID {
		return in, dagDiagnostic(fmt.Errorf("%w: slot_id must be canonical zero-based decimal", ErrInvalid), "dag.pool.slot_id", nil)
	}
	in.Scopes = append([]PoolCapabilityScope(nil), in.Scopes...)
	seen := map[string]bool{}
	for i := range in.Scopes {
		s := &in.Scopes[i]
		if scopeValid(s.TenantID, s.ProjectID) != nil {
			return in, ErrInvalid
		}
		key := poolScopeKey(s.TenantID, s.ProjectID)
		if seen[key] {
			return in, dagDiagnostic(fmt.Errorf("%w: duplicate capability scope", ErrInvalid), "dag.pool.duplicate_scope", nil)
		}
		seen[key] = true
		s.Capabilities, err = normalizedCapabilities(s.Capabilities)
		if err != nil {
			return in, err
		}
	}
	sort.Slice(in.Scopes, func(i, j int) bool {
		a, b := in.Scopes[i], in.Scopes[j]
		if a.TenantID != b.TenantID {
			return a.TenantID < b.TenantID
		}
		return a.ProjectID < b.ProjectID
	})
	return in, nil
}
func (t *transaction) currentPoolScope(pool, tenant, project string) (PoolScope, error) {
	out := PoolScope{PoolID: pool, TenantID: tenant, ProjectID: project}
	err := t.row(`SELECT grant_id,grant_version,project_weight,status FROM dag_execution_pool_scopes WHERE pool_id=? AND tenant_id=? AND project_id=?`+t.e.currentReadSuffix(), pool, tenant, project).Scan(&out.GrantID, &out.GrantVersion, &out.ProjectWeight, &out.Status)
	return out, err
}

// ClaimPoolWithOperation performs selection, two-level resource admission,
// fencing, machine-slot binding, fair service and receipt in one transaction.
// before owns live machine identity/organization authorization before pool locks;
// eligible must not acquire new organization locks or perform external I/O.
func (e *Engine) ClaimPoolWithOperation(ctx context.Context, request PoolClaimRequest, before func(*sql.Tx) (PoolClaimAuthority, error), eligible func(*sql.Tx, PoolScope, string, string) (bool, error)) (*Assignment, error) {
	request, err := normalizePoolRequest(request)
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, dagDiagnostic(fmt.Errorf("%w: current pool authority required", ErrInvalid), "dag.pool.authority", nil)
	}
	var authority PoolClaimAuthority
	var out *Assignment
	err = e.writeBefore(ctx, func(tx *sql.Tx) error { var err error; authority, err = before(tx); return err }, func(t *transaction) error {
		if !identifier.MatchString(authority.CredentialID) || authority.CredentialVersion < 1 || authority.MaxSlots < 1 || authority.MaxSlots > 128 {
			return ErrInvalid
		}
		slot, _ := strconv.Atoi(request.SlotID)
		if slot >= authority.MaxSlots {
			return dagDiagnostic(fmt.Errorf("%w: slot outside registered capacity", ErrInvalid), "dag.pool.slot_capacity", nil)
		}
		if err := e.guardPoolTx(ctx, t.tx, request.PoolID); err != nil {
			return err
		}
		policy, err := e.readPoolPolicy(ctx, t.tx, request.PoolID, true)
		if err != nil {
			return err
		}
		if policy.Version == 0 {
			return ErrNotFound
		}
		allowed := map[string]PoolScope{}
		for _, s := range authority.Scopes {
			if s.PoolID != request.PoolID {
				return ErrInvalid
			}
			allowed[poolScopeKey(s.TenantID, s.ProjectID)] = s
		}
		active := []PoolScope{}
		caps := map[string][]string{}
		for _, supported := range request.Scopes {
			key := poolScopeKey(supported.TenantID, supported.ProjectID)
			given, ok := allowed[key]
			if !ok {
				return dagDiagnostic(fmt.Errorf("%w: local capability scope is not currently authorized", ErrInvalid), "dag.pool.capability_authority", nil)
			}
			actual, err := t.currentPoolScope(request.PoolID, supported.TenantID, supported.ProjectID)
			if err != nil {
				return err
			}
			if actual.GrantID != given.GrantID || actual.GrantVersion != given.GrantVersion || actual.ProjectWeight != given.ProjectWeight || actual.Status != given.Status {
				return dagDiagnostic(fmt.Errorf("%w: pool grant changed", ErrConflict), "dag.pool.grant_changed", nil)
			}
			caps[key] = supported.Capabilities
			if actual.Status == "active" && len(supported.Capabilities) > 0 {
				active = append(active, actual)
			}
		}
		hash := fingerprint(request)
		previous, found, err := t.recoverPoolDispatch(request, authority, hash, allowed, eligible)
		if err != nil {
			return err
		}
		if found {
			out = previous
			return nil
		}
		if !authority.PoolActive {
			return t.recordPoolDispatch(request, authority, hash, nil)
		}
		var occupied int
		if err = t.row(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id=? AND runner_id=? AND slot_id=?`, request.PoolID, request.RunnerID, request.SlotID).Scan(&occupied); err != nil {
			return err
		}
		if occupied > 0 {
			return t.recordPoolDispatch(request, authority, hash, nil)
		}
		ranked, err := t.rankPoolScopes(request.PoolID, active)
		if err != nil {
			return err
		}
		scanHash := fingerprint(request.Scopes)
		var last string
		err = t.row(`SELECT last_scope FROM dag_execution_pool_scans WHERE pool_id=? AND runner_id=? AND request_scope_sha256=?`, request.PoolID, request.RunnerID, scanHash).Scan(&last)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		start := 0
		if last != "" {
			for i, c := range ranked {
				if poolScopeKey(c.scope.TenantID, c.scope.ProjectID) == last {
					start = (i + 1) % len(ranked)
					break
				}
			}
		}
		remaining := 64
		scanWorker := "poolscan-" + fingerprint([]string{request.PoolID, request.RunnerID, request.WorkerID})
		selected := []poolFairCandidate{}
		for i := 0; i < len(ranked) && i < 16; i++ {
			selected = append(selected, ranked[(start+i)%len(ranked)])
		}
		// Different pools may retain immutable runs for the same projects after
		// a route switch. Lock the bounded project batch lexically, never in its
		// changing fair-service order, to avoid cross-pool project lock cycles.
		locks := append([]poolFairCandidate(nil), selected...)
		sort.Slice(locks, func(i, j int) bool {
			a, b := locks[i].scope, locks[j].scope
			if a.TenantID != b.TenantID {
				return a.TenantID < b.TenantID
			}
			return a.ProjectID < b.ProjectID
		})
		for _, c := range locks {
			if err = t.guardProject(c.scope.TenantID, c.scope.ProjectID); err != nil {
				return err
			}
		}
		for _, fair := range selected {
			if remaining <= 0 {
				break
			}
			scope := fair.scope
			key := poolScopeKey(scope.TenantID, scope.ProjectID)
			last = key
			perScope := remaining
			if perScope > 4 {
				perScope = 4
			}
			candidates, err := t.claimCandidateBatchPool(scope.TenantID, scope.ProjectID, scanWorker, caps[key], request.PoolID, perScope)
			if err != nil {
				return err
			}
			remaining -= len(candidates)
			for _, candidate := range candidates {
				r, err := t.loadHot(scope.TenantID, scope.ProjectID, candidate.run)
				if err != nil {
					return err
				}
				n := r.find(candidate.node)
				if n == nil || n.State != "ready" || r.State != "running" || r.ExecutionPoolID != request.PoolID || r.ExecutionPoolGrantID != scope.GrantID || r.ExecutionPoolGrantVersion != scope.GrantVersion || !capable(n, caps[key]) {
					continue
				}
				if eligible != nil {
					ok, err := eligible(t.tx, scope, r.ID, n.ID)
					if err != nil {
						return err
					}
					if !ok {
						continue
					}
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
				if r.kernelGeneration >= 2 && seconds <= 0 {
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
				allowed, err := t.checkAttemptAccounting(r, n, request.WorkerID)
				if err != nil {
					return err
				}
				if !allowed {
					continue
				}
				poolChecks, ok, err := t.checkPoolResources(r, n)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				reserved, err := t.reservePolicyResources(r, n)
				if err != nil {
					return err
				}
				if !reserved {
					continue
				}
				if err = t.reservePoolResources(r, n, poolChecks); err != nil {
					return err
				}
				if err = t.reserveAttemptAccounting(r, n, request.WorkerID); err != nil {
					return err
				}
				if seconds, err = t.attemptWindow(r, n, true); err != nil {
					return err
				}
				n.Attempt++
				n.Fence++
				n.WorkerID = request.WorkerID
				n.StartedAt = stamp(t.now)
				n.CompletedAt = ""
				n.Output = ""
				n.Error = ""
				n.WorkspaceEvidence, n.ArtifactEvidence, n.ToolEvidence = nil, nil, nil
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
				seq, err := t.nextQueueSequence(r.TenantID, r.ProjectID)
				if err != nil {
					return err
				}
				if err = t.exec(`DELETE FROM dag_run_fairness WHERE run_id=?`, r.ID); err != nil {
					return err
				}
				if err = t.exec(`INSERT INTO dag_run_fairness(run_id,last_claim_sequence)VALUES(?,?)`, r.ID, seq); err != nil {
					return err
				}
				if err = t.exec(`DELETE FROM dag_ready WHERE run_id=? AND node_id=?`, r.ID, n.ID); err != nil {
					return err
				}
				if err = t.exec(`DELETE FROM dag_ready_v2 WHERE run_id=? AND node_id=?`, r.ID, n.ID); err != nil {
					return err
				}
				if err = t.exec(`INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result)VALUES(?,?,?,?,?,'running',?,'','')`, r.ID, n.ID, n.Attempt, n.Fence, n.WorkerID, n.StartedAt); err != nil {
					return err
				}
				if err = t.exec(`INSERT INTO dag_execution_pool_slots(pool_id,runner_id,slot_id,run_id,node_id,attempt,fence)VALUES(?,?,?,?,?,?,?)`, request.PoolID, request.RunnerID, request.SlotID, r.ID, n.ID, n.Attempt, n.Fence); err != nil {
					return err
				}
				weight := policy.TenantWeights[scope.TenantID]
				if weight == 0 {
					weight = 1
				}
				if err = t.chargePoolFairness(request.PoolID, fair, weight); err != nil {
					return err
				}
				if err = t.save(r); err != nil {
					return err
				}
				if err = t.resetClaimScan(r.TenantID, r.ProjectID, scanWorker, caps[key]); err != nil {
					return err
				}
				out = &Assignment{TenantID: r.TenantID, ProjectID: r.ProjectID, PoolID: request.PoolID, PoolRunnerID: request.RunnerID, PoolSlotID: request.SlotID, PoolGrantID: scope.GrantID, PoolGrantVersion: scope.GrantVersion, PoolCredentialID: authority.CredentialID, PoolCredentialVersion: authority.CredentialVersion, RunID: r.ID, NodeID: n.ID, Attempt: n.Attempt, Fence: n.Fence, WorkerID: n.WorkerID, Command: n.spec.Command, Type: n.Type, TimeoutSeconds: seconds, Idempotent: n.spec.Idempotent, Config: n.spec.Config, DispatchID: uid(), DispatchState: "active", LeaseUntil: stamp(n.lease)}
				if pin != nil {
					out.CandidateCommit = pin.CandidateCommit
					out.CandidateFrom = pin.CandidateFrom
					out.CandidateAttempt = pin.CandidateAttempt
					out.CandidateFence = pin.CandidateFence
					out.CandidateEvidenceSHA256 = pin.CandidateEvidenceSHA256
				}
				if err = t.storePoolScan(request.PoolID, request.RunnerID, scanHash, ""); err != nil {
					return err
				}
				return t.recordPoolDispatch(request, authority, hash, out)
			}
		}
		if err = t.storePoolScan(request.PoolID, request.RunnerID, scanHash, last); err != nil {
			return err
		}
		return t.recordPoolDispatch(request, authority, hash, nil)
	})
	return out, err
}

func (t *transaction) storePoolScan(pool, runner, hash, last string) error {
	if err := t.insertIgnore(`INSERT INTO dag_execution_pool_scans(pool_id,runner_id,request_scope_sha256,last_scope)VALUES(?,?,?,'')`, "pool_id,runner_id,request_scope_sha256", pool, runner, hash); err != nil {
		return err
	}
	return t.exec(`UPDATE dag_execution_pool_scans SET last_scope=? WHERE pool_id=? AND runner_id=? AND request_scope_sha256=?`, last, pool, runner, hash)
}
func (t *transaction) recordPoolDispatch(in PoolClaimRequest, auth PoolClaimAuthority, hash string, a *Assignment) error {
	id := uid()
	if a != nil {
		id = a.DispatchID
	}
	return t.exec(`INSERT INTO dag_execution_pool_dispatches(pool_id,runner_id,operation_id,dispatch_id,request_sha256,credential_id,credential_version,assignment,created_at)VALUES(?,?,?,?,?,?,?,?,?)`, in.PoolID, in.RunnerID, in.OperationID, id, hash, auth.CredentialID, auth.CredentialVersion, encoded(a), stamp(t.now))
}
func (t *transaction) recoverPoolDispatch(in PoolClaimRequest, auth PoolClaimAuthority, hash string, allowed map[string]PoolScope, eligible func(*sql.Tx, PoolScope, string, string) (bool, error)) (*Assignment, bool, error) {
	var raw, stored, credential string
	var version int
	err := t.row(`SELECT assignment,request_sha256,credential_id,credential_version FROM dag_execution_pool_dispatches WHERE pool_id=? AND runner_id=? AND operation_id=?`, in.PoolID, in.RunnerID, in.OperationID).Scan(&raw, &stored, &credential, &version)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if hash != stored || credential != auth.CredentialID || version != auth.CredentialVersion {
		return nil, false, ErrConflict
	}
	if raw == "null" {
		return nil, true, nil
	}
	var a Assignment
	if err = json.Unmarshal([]byte(raw), &a); err != nil {
		return nil, false, err
	}
	scope, ok := allowed[poolScopeKey(a.TenantID, a.ProjectID)]
	if !ok || scope.GrantID != a.PoolGrantID || scope.GrantVersion != a.PoolGrantVersion {
		return nil, false, ErrConflict
	}
	r, err := t.loadHot(a.TenantID, a.ProjectID, a.RunID)
	if err != nil {
		return nil, false, err
	}
	n := r.find(a.NodeID)
	a.DispatchState = "closed"
	if n != nil && n.Attempt == a.Attempt && n.Fence == a.Fence && n.WorkerID == a.WorkerID {
		if n.State == "reconciling" || n.State == "running" && (n.lease <= t.now || r.State == "cancelling") {
			a.DispatchState = "reconciling"
		} else if n.State == "running" {
			a.DispatchState = "active"
		}
		if n.lease > 0 {
			a.LeaseUntil = stamp(n.lease)
		}
	}
	if a.DispatchState == "active" {
		if !auth.PoolActive || scope.Status != "active" {
			return nil, false, dagDiagnostic(fmt.Errorf("%w: original pool dispatch no longer eligible", ErrConflict), "dag.pool.dispatch_eligibility", nil)
		}
		if eligible != nil {
			ok, err := eligible(t.tx, scope, a.RunID, a.NodeID)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, ErrConflict
			}
		}
		remaining, err := t.attemptWindow(r, n, false)
		if err != nil {
			return nil, false, err
		}
		if remaining <= 0 {
			a.DispatchState = "reconciling"
		} else {
			a.TimeoutSeconds = remaining
		}
	}
	return &a, true, nil
}

// ValidatePoolAssignmentTx reads immutable dispatch identity only. It never
// acquires authority/pool/project locks and never authorizes a state transition.
func ValidatePoolAssignmentTx(ctx context.Context, tx *sql.Tx, dialect string, a Assignment) error {
	e := &Engine{dialect: dialect}
	if a.DispatchID == "" || a.PoolID == "" || a.TenantID == "" || a.ProjectID == "" || a.PoolCredentialVersion < 1 {
		return ErrStale
	}
	var raw string
	err := tx.QueryRowContext(ctx, e.q(`SELECT assignment FROM dag_execution_pool_dispatches WHERE dispatch_id=? AND pool_id=? AND runner_id=?`), a.DispatchID, a.PoolID, a.PoolRunnerID).Scan(&raw)
	if err != nil {
		return ErrStale
	}
	var stored Assignment
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return ErrStale
	}
	identity := func(x Assignment) any {
		return []any{x.DispatchID, x.TenantID, x.ProjectID, x.PoolID, x.PoolRunnerID, x.PoolSlotID, x.PoolGrantID, x.PoolGrantVersion, x.PoolCredentialID, x.PoolCredentialVersion, x.RunID, x.NodeID, x.WorkerID, x.Attempt, x.Fence}
	}
	if fingerprint(identity(a)) != fingerprint(identity(stored)) {
		return ErrStale
	}
	return nil
}
