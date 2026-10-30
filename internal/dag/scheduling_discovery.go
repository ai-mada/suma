package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
)

// ClaimCandidateRef is a non-authoritative bounded discovery result. The real
// claim must recheck it after complete participant locks; discovery writes nothing.
type ClaimCandidateRef struct{ TenantID, ProjectID, RunID, NodeID string }

func DiscoverClaimCandidatesTx(ctx context.Context, tx *sql.Tx, dialect, tenant, project, worker, operation string, caps []string) ([]ClaimCandidateRef, error) {
	e := &Engine{dialect: dialect}
	t := &transaction{e: e, tx: tx, ctx: ctx}
	if scopeValid(tenant, project) != nil {
		return nil, ErrInvalid
	}
	caps, err := normalizedCapabilities(caps)
	if err != nil {
		return nil, err
	}
	var raw string
	err = t.row(`SELECT assignment FROM dag_dispatches WHERE tenant_id=? AND project_id=? AND operation_id=?`, tenant, project, operation).Scan(&raw)
	if err == nil {
		return discoveryReceipt(raw)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	items, err := t.claimCandidateWindow(tenant, project, worker, caps, "", 64, true)
	if err != nil {
		return nil, err
	}
	out := []ClaimCandidateRef{}
	for _, c := range items {
		out = append(out, ClaimCandidateRef{tenant, project, c.run, c.node})
	}
	return out, nil
}

func discoveryReceipt(raw string) ([]ClaimCandidateRef, error) {
	if raw == "null" || raw == "" {
		return []ClaimCandidateRef{}, nil
	}
	var a Assignment
	if json.Unmarshal([]byte(raw), &a) != nil {
		return nil, ErrConflict
	}
	return []ClaimCandidateRef{{a.TenantID, a.ProjectID, a.RunID, a.NodeID}}, nil
}

func DiscoverPoolClaimCandidatesTx(ctx context.Context, tx *sql.Tx, dialect string, in PoolClaimRequest) ([]ClaimCandidateRef, error) {
	in, err := normalizePoolRequest(in)
	if err != nil {
		return nil, err
	}
	e := &Engine{dialect: dialect}
	t := &transaction{e: e, tx: tx, ctx: ctx}
	var raw string
	err = t.row(`SELECT assignment FROM dag_execution_pool_dispatches WHERE pool_id=? AND runner_id=? AND operation_id=?`, in.PoolID, in.RunnerID, in.OperationID).Scan(&raw)
	if err == nil {
		return discoveryReceipt(raw)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	active := []PoolScope{}
	caps := map[string][]string{}
	for _, supported := range in.Scopes {
		var s PoolScope
		s.PoolID = in.PoolID
		s.TenantID = supported.TenantID
		s.ProjectID = supported.ProjectID
		err = t.row(`SELECT grant_id,grant_version,project_weight,status FROM dag_execution_pool_scopes WHERE pool_id=? AND tenant_id=? AND project_id=?`, in.PoolID, s.TenantID, s.ProjectID).Scan(&s.GrantID, &s.GrantVersion, &s.ProjectWeight, &s.Status)
		if err != nil {
			return nil, err
		}
		caps[poolScopeKey(s.TenantID, s.ProjectID)] = supported.Capabilities
		if s.Status == "active" && len(supported.Capabilities) > 0 {
			active = append(active, s)
		}
	}
	ranked, err := t.rankPoolScopesReadOnly(in.PoolID, active)
	if err != nil {
		return nil, err
	}
	if len(ranked) == 0 {
		return []ClaimCandidateRef{}, nil
	}
	var last string
	err = t.row(`SELECT last_scope FROM dag_execution_pool_scans WHERE pool_id=? AND runner_id=? AND request_scope_sha256=?`, in.PoolID, in.RunnerID, fingerprint(in.Scopes)).Scan(&last)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	start := 0
	for i, c := range ranked {
		if poolScopeKey(c.scope.TenantID, c.scope.ProjectID) == last {
			start = (i + 1) % len(ranked)
			break
		}
	}
	worker := "poolscan-" + fingerprint([]string{in.PoolID, in.RunnerID, in.WorkerID})
	out := []ClaimCandidateRef{}
	remaining := 64
	for i := 0; i < len(ranked) && i < 16 && remaining > 0; i++ {
		scope := ranked[(start+i)%len(ranked)].scope
		limit := remaining
		if limit > 4 {
			limit = 4
		}
		items, err := t.claimCandidateWindow(scope.TenantID, scope.ProjectID, worker, caps[poolScopeKey(scope.TenantID, scope.ProjectID)], in.PoolID, limit, true)
		if err != nil {
			return nil, err
		}
		remaining -= len(items)
		for _, c := range items {
			out = append(out, ClaimCandidateRef{scope.TenantID, scope.ProjectID, c.run, c.node})
		}
	}
	return out, nil
}

// This simulates exactly the existing fair-epoch normalization without writing
// fairness rows or changing cursors before human authority is coordinated.
func (t *transaction) rankPoolScopesReadOnly(pool string, scopes []PoolScope) ([]poolFairCandidate, error) {
	conditions := []string{`(tenant_id='' AND project_id='')`}
	args := []any{pool}
	seen := map[string]bool{}
	for _, s := range scopes {
		if !seen[s.TenantID] {
			conditions = append(conditions, `(tenant_id=? AND project_id='')`)
			args = append(args, s.TenantID)
			seen[s.TenantID] = true
		}
		conditions = append(conditions, `(tenant_id=? AND project_id=?)`)
		args = append(args, s.TenantID, s.ProjectID)
	}
	rows, err := t.rows(`SELECT tenant_id,project_id,pass,remainder,weight,floor,epoch,last_sequence FROM dag_execution_pool_fairness WHERE pool_id=? AND (`+strings.Join(conditions, " OR ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	states := map[string]poolFairState{}
	for rows.Next() {
		var tenant, project string
		var s poolFairState
		if err = rows.Scan(&tenant, &project, &s.pass, &s.remainder, &s.weight, &s.floor, &s.epoch, &s.last); err != nil {
			rows.Close()
			return nil, err
		}
		states[tenant+"/"+project] = s
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	root := states["/"]
	rebase := int64(0)
	if root.floor >= poolFairRebase {
		rebase = root.floor
		root.floor = 0
	}
	out := []poolFairCandidate{}
	for _, scope := range scopes {
		parent := states[scope.TenantID+"/"]
		if rebase > 0 {
			parent.pass = maxPoolInt(parent.pass-rebase, 0)
		}
		child := states[scope.TenantID+"/"+scope.ProjectID]
		if parent.floor >= poolFairRebase {
			child.pass = maxPoolInt(child.pass-parent.floor, 0)
			parent.floor = 0
			parent.epoch++
		}
		out = append(out, poolFairCandidate{scope: scope, tenant: parent, project: child, tenantEffective: maxPoolInt(parent.pass, root.floor), projectEffective: maxPoolInt(child.pass, parent.floor)})
	}
	sortPoolFairCandidates(out)
	return out, nil
}

func sortPoolFairCandidates(items []poolFairCandidate) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.tenantEffective != b.tenantEffective {
			return a.tenantEffective < b.tenantEffective
		}
		if a.tenant.last != b.tenant.last {
			return a.tenant.last < b.tenant.last
		}
		if a.scope.TenantID != b.scope.TenantID {
			return a.scope.TenantID < b.scope.TenantID
		}
		if a.projectEffective != b.projectEffective {
			return a.projectEffective < b.projectEffective
		}
		if a.project.last != b.project.last {
			return a.project.last < b.project.last
		}
		return a.scope.ProjectID < b.scope.ProjectID
	})
}
