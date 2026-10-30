package platform

import (
	"context"
	"database/sql"
	"encoding/json"
)

const expertTeamExecutionNodeLimit = 24

// Execution facts are a current authorized read model, never a plan revision or
// a new source of execution authority. Only explicitly selected fields escape.
type ExpertTeamExecutionFact struct {
	RunID      string                        `json:"run_id"`
	ObservedAt string                        `json:"observed_at"`
	NodesTotal int                           `json:"nodes_total"`
	Nodes      []ExpertTeamExecutionNodeFact `json:"nodes"`
	Truncated  bool                          `json:"truncated"`
}
type ExpertTeamExecutionNodeFact struct {
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	State    string `json:"state"`
	Reason   string `json:"reason"`
	Attempt  int    `json:"attempt"`
	WorkerID string `json:"worker_id"`
	ReadyAt  string `json:"ready_at"`
}

type expertTeamExecutionSummary struct {
	total, running, waiting, blocked, unknown int
}

// This full aggregate is deliberately independent of the bounded display page.
func (s *Service) expertTeamExecutionSummaryTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (out expertTeamExecutionSummary, err error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT n.state,COUNT(*),SUM(CASE WHEN n.attempt>0 OR COALESCE(n.reason,'')<>'' THEN 1 ELSE 0 END) FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? GROUP BY n.state`), tenant, project, run)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var count, priorOrReason int
		if err = rows.Scan(&state, &count, &priorOrReason); err != nil {
			return out, err
		}
		out.total += count
		switch state {
		case "running":
			out.running += count
		case "waiting":
			out.waiting += count
		case "pending", "ready":
			out.waiting += priorOrReason
		case "reconciling", "cancelling", "blocked", "paused":
			out.blocked += count
		case "succeeded", "skipped", "failed", "cancelled":
		default:
			out.unknown += count
		}
	}
	return out, rows.Err()
}
func expertTeamKnownRunState(state string) bool {
	switch state {
	case "running", "paused", "cancelling", "reconciling", "succeeded", "failed", "cancelled", "skipped":
		return true
	}
	return false
}
func clearExpertTeamExecution(plan *ExpertTeamPlan) {
	for i := range plan.Items {
		plan.Items[i].Execution = nil
	}
}
func (s *Service) expertTeamExecutionFactTx(ctx context.Context, tx *sql.Tx, tenant, project, run, observedAt string) (*ExpertTeamExecutionFact, error) {
	out := &ExpertTeamExecutionFact{RunID: run, ObservedAt: observedAt, Nodes: []ExpertTeamExecutionNodeFact{}}
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), tenant, project, run).Scan(&out.NodesTotal); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT n.id,n.state,COALESCE(n.reason,''),n.attempt,COALESCE(n.worker_id,''),COALESCE(q.ready_at,''),n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id LEFT JOIN dag_ready q ON q.run_id=n.run_id AND q.node_id=n.id AND q.tenant_id=r.tenant_id AND q.project_id=r.project_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? ORDER BY CASE n.state WHEN 'reconciling' THEN 0 WHEN 'cancelling' THEN 1 WHEN 'failed' THEN 2 WHEN 'blocked' THEN 3 WHEN 'waiting' THEN 4 WHEN 'running' THEN 5 WHEN 'ready' THEN 6 WHEN 'pending' THEN 7 WHEN 'succeeded' THEN 8 WHEN 'skipped' THEN 8 WHEN 'cancelled' THEN 8 WHEN 'paused' THEN 3 ELSE 0 END,n.id LIMIT ?`), tenant, project, run, expertTeamExecutionNodeLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var node ExpertTeamExecutionNodeFact
		var raw string
		if err = rows.Scan(&node.NodeID, &node.State, &node.Reason, &node.Attempt, &node.WorkerID, &node.ReadyAt, &raw); err != nil {
			return nil, err
		}
		var spec struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		if err = json.Unmarshal([]byte(raw), &spec); err != nil {
			return nil, err
		}
		node.Name, node.Type = spec.Name, spec.Type
		out.Nodes = append(out.Nodes, node)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out.Truncated = out.NodesTotal > len(out.Nodes)
	return out, nil
}
