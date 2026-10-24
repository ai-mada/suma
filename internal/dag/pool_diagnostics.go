package dag

import (
	"context"
	"database/sql"
)

// addPoolAdmissionFacts is read-only and projects only resources this node asks
// for. Shared usage is aggregate; no other tenant, run, task or owner is exposed.
func (e *Engine) addPoolAdmissionFacts(ctx context.Context, tx *sql.Tx, out *NodeAdmissionSnapshot, tenant, project string) error {
	route, found, err := e.runRoute(ctx, tx, tenant, project, out.RunID, false)
	if err != nil {
		return err
	}
	if !found || route.PoolID == "" {
		return nil
	}
	out.ExecutionPoolID = route.PoolID
	policy, err := e.ReadPoolPolicyTx(ctx, tx, route.PoolID)
	if err != nil {
		return err
	}
	out.PoolPolicyVersion = policy.Version
	check := !terminal(out.NodeState) && out.NodeState != "running"
	add := func(code, message string) {
		if check {
			out.Blockers = append(out.Blockers, SchedulingBlocker{Code: code, Message: message, Source: "pool", RunID: out.RunID, NodeID: out.NodeID})
		}
	}
	if policy.Version == 0 {
		add("pool_policy_missing", "共享池正式准入策略缺失，不能派发")
		return nil
	}
	var grant, status string
	var version int
	err = tx.QueryRowContext(ctx, e.q(`SELECT grant_id,grant_version,status FROM dag_execution_pool_scopes WHERE pool_id=? AND tenant_id=? AND project_id=?`), route.PoolID, tenant, project).Scan(&grant, &version, &status)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == sql.ErrNoRows || status != "active" {
		add("pool_scope_inactive", "共享池当前双边授权未生效，停止新增派发")
	}
	if err == nil && (grant != route.GrantID || version != route.GrantVersion) {
		add("pool_scope_version_mismatch", "共享池当前授权与运行固定版本不一致，需要重新审阅计划")
	}
	accounts := map[string]ResourceSnapshot{}
	for _, r := range policy.Resources {
		accounts[r.Name] = r
	}
	for _, request := range out.Requests {
		r, ok := accounts[request.Name]
		if !ok {
			r = ResourceSnapshot{Name: request.Name, Unit: request.Unit, Mode: "unlimited", ReleaseClass: request.ReleaseClass}
			r.Capacity = policy.ResourceLimits[request.Name]
			if r.Capacity != nil {
				r.Mode = "limited"
			}
		}
		out.PoolResources = append(out.PoolResources, r)
		used := r.Used
		if request.ReleaseClass == "cumulative" {
			used = r.Consumed
		}
		if check && r.Capacity != nil && request.Amount > *r.Capacity-used {
			required, capacity := request.Amount, *r.Capacity
			available := capacity - used
			if available < 0 {
				available = 0
			}
			code, message := "pool_resource_capacity", "共享池配置准入资源不足；不是已测硬件负载"
			if request.Name == "budget" {
				code, message = "pool_budget_exhausted", "共享池累计准入单位不足；不代表金额或token账单"
			}
			out.Blockers = append(out.Blockers, SchedulingBlocker{Code: code, Message: message, Source: "pool", Resource: request.Name, Unit: request.Unit, Required: &required, Capacity: &capacity, Used: &used, Available: &available, RunID: out.RunID, NodeID: out.NodeID})
		}
	}
	return nil
}
