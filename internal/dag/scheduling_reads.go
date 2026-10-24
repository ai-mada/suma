package dag

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func workerNodeType(kind string) bool {
	switch kind {
	case "task", "command", "agent", "script", "test", "external_workflow":
		return true
	}
	return false
}
func nodeResourceRequests(n Node) []ResourceRequest {
	if !workerNodeType(n.Type) {
		return []ResourceRequest{}
	}
	amounts := map[string]int64{}
	for name, amount := range n.Resources {
		amounts[name] = amount
	}
	if amounts["execution_slots"] < 1 {
		amounts["execution_slots"] = 1
	}
	if amounts["budget"] < 1 {
		amounts["budget"] = 1
	}
	names := []string{}
	for name := range amounts {
		names = append(names, name)
	}
	sort.Strings(names)
	out := []ResourceRequest{}
	for _, name := range names {
		out = append(out, ResourceRequest{Name: name, Amount: amounts[name], Unit: resourceUnit(name), ReleaseClass: resourceClass(name)})
	}
	return out
}

func (e *Engine) AdmissionSnapshotTx(ctx context.Context, tx *sql.Tx, tenant, project, runID, nodeID string) (NodeAdmissionSnapshot, error) {
	out := NodeAdmissionSnapshot{RunID: runID, NodeID: nodeID, Blockers: []SchedulingBlocker{}, Requests: []ResourceRequest{}, Reservations: []ResourceReservationSnapshot{}}
	var runState, kind, reason, raw string
	var wake int64
	err := tx.QueryRowContext(ctx, e.q(`SELECT r.version,r.state,n.state,n.attempt,n.fence,n.spec,n.reason,n.wake_at FROM dag_runs r JOIN dag_nodes n ON n.run_id=r.id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), tenant, project, runID, nodeID).Scan(&out.RunVersion, &runState, &out.NodeState, &out.Attempt, &out.Fence, &raw, &reason, &wake)
	if err == sql.ErrNoRows {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	var spec Node
	if err = json.Unmarshal([]byte(raw), &spec); err != nil {
		return out, err
	}
	kind = spec.Type
	policy, err := e.ReadSchedulingPolicyTx(ctx, tx, tenant, project)
	if err != nil {
		return out, err
	}
	out.PolicyVersion = policy.Version
	out.EvaluatedAt = policy.AsOf
	out.Requests = nodeResourceRequests(spec)
	reserved, err := tx.QueryContext(ctx, e.q(`SELECT name,amount,release_class,attempt,fence FROM dag_resource_ledger WHERE run_id=? AND node_id=? AND state='active' ORDER BY name,attempt`), runID, nodeID)
	if err != nil {
		return out, err
	}
	for reserved.Next() {
		var r ResourceReservationSnapshot
		if err = reserved.Scan(&r.Name, &r.Amount, &r.ReleaseClass, &r.Attempt, &r.Fence); err != nil {
			reserved.Close()
			return out, err
		}
		r.Unit = resourceUnit(r.Name)
		out.Reservations = append(out.Reservations, r)
	}
	err = reserved.Err()
	reserved.Close()
	if err != nil {
		return out, err
	}
	if err = e.addPoolAdmissionFacts(ctx, tx, &out, tenant, project); err != nil {
		return out, err
	}
	if terminal(out.NodeState) || out.NodeState == "running" {
		out.Status = "not_pending"
		return out, nil
	}
	add := func(code, message, source string) {
		out.Blockers = append(out.Blockers, SchedulingBlocker{Code: code, Message: message, Source: source, RunID: runID, NodeID: nodeID})
	}
	if runState == "paused" {
		add("run_paused", "运行已暂停，不再派发新尝试", "run")
	}
	if runState == "cancelling" {
		add("run_cancelling", "正在取消，等待实际停止或核对", "run")
	}
	switch out.NodeState {
	case "reconciling":
		add("outcome_unknown", "旧尝试的外部效果未知，需要核对，不能重新执行", "attempt")
	case "pending":
		now, _ := time.Parse(time.RFC3339, policy.AsOf)
		if wake > now.Unix() && out.Attempt > 0 {
			add("retry_backoff", "已知瞬时失败正在持久退避，尚未创建新尝试", "retry")
		} else {
			add("dependencies_pending", "等待依赖或条件结算", "dependencies")
		}
	case "waiting":
		switch kind {
		case "approval":
			add("approval_required", "等待当前版本的人工审批", "approval")
		case "timer":
			add("timer_waiting", "等待数据库定时唤醒", "timer")
		case "map":
			add("expansion_waiting", "等待展开封闭及子节点终结", "expansion")
		case "subworkflow":
			add("child_waiting", "等待固定子流程终结", "subworkflow")
		default:
			add("node_waiting", "节点仍在等待", "node")
		}
	}
	if wake > 0 && !terminal(out.NodeState) {
		for i := range out.Blockers {
			if out.Blockers[i].Code == "dependencies_pending" || out.Blockers[i].Code == "timer_waiting" || out.Blockers[i].Code == "retry_backoff" {
				out.Blockers[i].RetryAt = stamp(wake)
			}
		}
	}
	accounts := map[string]ResourceSnapshot{}
	for _, r := range policy.Resources {
		accounts[r.Name] = r
	}
	for _, request := range out.Requests {
		account, ok := accounts[request.Name]
		if !ok {
			account = ResourceSnapshot{Name: request.Name, Unit: request.Unit, Mode: "unlimited", Capacity: policy.ResourceLimits[request.Name]}
		}
		used := account.Used
		if request.ReleaseClass == "cumulative" {
			used = account.Consumed
		}
		if account.Capacity != nil && request.Amount > *account.Capacity-used {
			cap := *account.Capacity
			required := request.Amount
			available := cap - used
			if available < 0 {
				available = 0
			}
			code := "resource_capacity"
			message := "项目准入资源不足；这是配置额度，不是已测硬件负载"
			if request.Name == "budget" {
				code = "budget_exhausted"
				message = "累计准入单位不足，不代表金额或token账单"
			}
			out.Blockers = append(out.Blockers, SchedulingBlocker{Code: code, Message: message, Source: "policy", Resource: request.Name, Unit: request.Unit, Required: &required, Capacity: &cap, Used: &used, Available: &available, RunID: runID, NodeID: nodeID})
		}
	}
	if out.NodeState == "ready" {
		add("worker_capability_unknown", "当前只读快照没有指定执行节点，能力、在线及工具资格仍需领取时核验", "worker")
	}
	out.Status = "eligible"
	if len(out.Blockers) > 0 {
		out.Status = "unknown"
		for _, b := range out.Blockers {
			if b.Code != "worker_capability_unknown" {
				out.Status = "blocked"
				break
			}
		}
	}
	return out, nil
}

type schedulingCursor struct {
	Tenant   string `json:"tenant"`
	Project  string `json:"project"`
	Run      string `json:"run,omitempty"`
	Node     string `json:"node,omitempty"`
	Sequence int64  `json:"sequence,omitempty"`
	Attempt  int    `json:"attempt,omitempty"`
	Kind     string `json:"kind"`
}

func decodeSchedulingCursor(raw, tenant, project, kind string) (schedulingCursor, error) {
	c := schedulingCursor{Tenant: tenant, Project: project, Kind: kind}
	if raw == "" {
		return c, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(b) > 2048 || json.Unmarshal(b, &c) != nil || c.Tenant != tenant || c.Project != project || c.Kind != kind {
		return c, dagDiagnostic(fmt.Errorf("%w: scheduling cursor scope", ErrInvalid), "dag.scheduling.cursor", nil)
	}
	return c, nil
}
func encodeSchedulingCursor(c schedulingCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(encoded(c)))
}

func (e *Engine) QueuePage(ctx context.Context, tenant, project string, limit int, cursor string) (ReadyPage, error) {
	out := ReadyPage{Items: []ReadyEntry{}}
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	c, err := decodeSchedulingCursor(cursor, tenant, project, "ready")
	if err != nil {
		return out, err
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*) FROM dag_ready_v2 q JOIN dag_nodes n ON n.run_id=q.run_id AND n.id=q.node_id WHERE q.tenant_id=? AND q.project_id=? AND n.state='ready'`), tenant, project).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT q.run_id,q.node_id,r.version,n.state,n.attempt,n.fence,q.ready_at,q.sequence,q.command_id,q.node_type,r.workflow_name,n.spec FROM dag_ready_v2 q JOIN dag_runs r ON r.id=q.run_id JOIN dag_nodes n ON n.run_id=q.run_id AND n.id=q.node_id WHERE q.tenant_id=? AND q.project_id=? AND n.state='ready'`
	params := []any{tenant, project}
	if cursor != "" {
		query += ` AND (q.sequence>? OR (q.sequence=? AND (q.run_id>? OR (q.run_id=? AND q.node_id>?))))`
		params = append(params, c.Sequence, c.Sequence, c.Run, c.Run, c.Node)
	}
	query += ` ORDER BY q.sequence,q.run_id,q.node_id LIMIT ?`
	params = append(params, limit+1)
	rows, err := tx.QueryContext(ctx, e.q(query), params...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item ReadyEntry
		var raw string
		if err = rows.Scan(&item.RunID, &item.NodeID, &item.RunVersion, &item.NodeState, &item.Attempt, &item.Fence, &item.ReadyAt, &item.Sequence, &item.CommandID, &item.Type, &item.WorkflowName, &raw); err != nil {
			rows.Close()
			return out, err
		}
		var n Node
		if err = json.Unmarshal([]byte(raw), &n); err != nil {
			rows.Close()
			return out, err
		}
		item.NodeName = n.Name
		if item.NodeName == "" {
			item.NodeName = item.NodeID
		}
		item.Requests = nodeResourceRequests(n)
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor = encodeSchedulingCursor(schedulingCursor{Tenant: tenant, Project: project, Kind: "ready", Run: last.RunID, Node: last.NodeID, Sequence: last.Sequence})
	}
	for i := range out.Items {
		snapshot, err := e.AdmissionSnapshotTx(ctx, tx, tenant, project, out.Items[i].RunID, out.Items[i].NodeID)
		if err != nil {
			return out, err
		}
		out.Items[i].Admission = &snapshot
	}
	return out, tx.Commit()
}

func (e *Engine) GetAttempt(ctx context.Context, tenant, project, runID, nodeID string, attempt int) (ExecutionAttempt, error) {
	var out ExecutionAttempt
	if attempt < 1 {
		return out, dagDiagnostic(fmt.Errorf("%w: attempt must be positive", ErrInvalid), "dag.attempt.positive", nil)
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var raw string
	err = tx.QueryRowContext(ctx, e.q(`SELECT a.run_id,a.node_id,a.attempt,a.fence,a.worker_id,a.state,a.started_at,a.completed_at,a.result FROM dag_attempts a JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.run_id=? AND a.node_id=? AND a.attempt=?`), tenant, project, runID, nodeID, attempt).Scan(&out.RunID, &out.NodeID, &out.Attempt, &out.Fence, &out.WorkerID, &out.State, &out.StartedAt, &out.CompletedAt, &raw)
	if err == sql.ErrNoRows {
		return out, ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if err = attemptResultSource(&out, raw); err != nil {
		return out, err
	}
	var observations int
	if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*) FROM dag_managed_resolutions WHERE run_id=? AND node_id=? AND attempt=?`), runID, nodeID, attempt).Scan(&observations); err != nil {
		return out, err
	}
	if observations > 0 {
		out.ResultSource = "managed_observation"
	}
	return out, tx.Commit()
}

func attemptResultSource(out *ExecutionAttempt, raw string) error {
	out.ResultSource = "unreported"
	if raw == "" {
		return nil
	}
	var value struct {
		RunID      string `json:"run_id"`
		NodeID     string `json:"node_id"`
		Outcome    string `json:"outcome"`
		Reason     string `json:"reason"`
		ChildRunID string `json:"child_run_id"`
	}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return err
	}
	object, err := rawObject([]byte(raw))
	if err != nil {
		return err
	}
	runnerShaped := false
	hasRunKey := false
	hasIdentityAlias := false
	for key := range object {
		if strings.EqualFold(key, "run_id") {
			hasRunKey = true
		}
		for _, name := range []string{"run_id", "node_id", "attempt", "fence", "worker_id", "exit_code"} {
			if strings.EqualFold(key, name) {
				runnerShaped = true
				if key != name && name != "exit_code" {
					hasIdentityAlias = true
				}
			}
		}
	}
	// Manual reconciliation stores its command payload, including node_id.
	// Child workflow observations also have a separate persisted source shape.
	if !hasRunKey {
		if value.Outcome != "" {
			out.ResultSource = "manual_reconciliation"
			out.Reconciliation = &AttemptReconciliation{Outcome: value.Outcome, Reason: value.Reason}
			return nil
		}
		if value.ChildRunID != "" {
			out.ResultSource = "child_workflow"
			out.ChildRunID = value.ChildRunID
			return nil
		}
	}
	if runnerShaped {
		var result Result
		if err = json.Unmarshal([]byte(raw), &result); err != nil {
			return err
		}
		page := AttemptToolEventPage{Attempt: out.Attempt, Fence: out.Fence, WorkerID: out.WorkerID}
		if hasIdentityAlias || !exactJSONText(object["run_id"], out.RunID) || !exactJSONText(object["node_id"], out.NodeID) || !provesAttempt([]byte(raw), page) ||
			result.RunID != out.RunID || result.NodeID != out.NodeID || result.Attempt != out.Attempt || result.Fence != out.Fence || result.WorkerID != out.WorkerID {
			return attemptReadError("receipt", fmt.Errorf("saved runner receipt does not match durable attempt identity"))
		}
		out.Result = &result
		out.ResultSource = "runner_result"
		return nil
	}
	return nil
}

func (e *Engine) AttemptsPage(ctx context.Context, tenant, project, runID, nodeID string, limit int, cursor string) (AttemptPage, error) {
	out := AttemptPage{Items: []ExecutionAttempt{}}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	c, err := decodeSchedulingCursor(cursor, tenant, project, "attempts:"+runID+":"+nodeID)
	if err != nil {
		return out, err
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*) FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, runID).Scan(&exists); err != nil {
		return out, err
	}
	if exists != 1 {
		return out, ErrNotFound
	}
	if nodeID != "" {
		if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND id=?`), runID, nodeID).Scan(&exists); err != nil {
			return out, err
		}
		if exists != 1 {
			return out, ErrNotFound
		}
	}
	where := ` FROM dag_attempts WHERE run_id=?`
	params := []any{runID}
	if nodeID != "" {
		where += ` AND node_id=?`
		params = append(params, nodeID)
	}
	if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*)`+where), params...).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at` + where
	if cursor != "" {
		query += ` AND (node_id>? OR (node_id=? AND attempt<?))`
		params = append(params, c.Node, c.Node, c.Attempt)
	}
	query += ` ORDER BY node_id,attempt DESC LIMIT ?`
	params = append(params, limit+1)
	rows, err := tx.QueryContext(ctx, e.q(query), params...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var item ExecutionAttempt
		if err = rows.Scan(&item.RunID, &item.NodeID, &item.Attempt, &item.Fence, &item.WorkerID, &item.State, &item.StartedAt, &item.CompletedAt); err != nil {
			rows.Close()
			return out, err
		}
		item.ResultSource = "not_loaded"
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor = encodeSchedulingCursor(schedulingCursor{Tenant: tenant, Project: project, Kind: "attempts:" + runID + ":" + nodeID, Node: last.NodeID, Attempt: last.Attempt})
	}
	return out, tx.Commit()
}
