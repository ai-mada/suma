package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/strictjson"
)

// The run scope/state index finds current work without scanning historical
// tasks. Node inspection is then bounded by each selected run's primary key.
// A bound is an honest partial observation, never an inferred zero total.
func (s *Service) toolProfileImpactTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, observed string) (ToolProfileImpact, error) {
	out := ToolProfileImpact{Scope: "active_project_runs", ObservedAt: observed, Coverage: "complete", Items: []ToolProfileImpactItem{}}
	runs := map[string]string{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND state IN ('running','paused','cancelling') ORDER BY state,id LIMIT 201`), tenant, project)
	if err != nil {
		return out, err
	}
	ids := []any{}
	for rows.Next() {
		var id, state string
		if err = rows.Scan(&id, &state); err != nil {
			break
		}
		if len(ids) == 200 {
			out.Coverage, out.Reason = "partial", "run_limit"
			break
		}
		ids = append(ids, id)
		runs[id] = state
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	out.RunsExamined = len(ids)
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	// Completed nodes need no source parsing. A very large or malformed live
	// source is explicitly unavailable; it cannot turn coverage into complete.
	query := `SELECT run_id,id,state,worker_id,attempt,fence,CASE WHEN state IN ('pending','ready','running','waiting','reconciling') THEN SUBSTR(spec,1,262145) ELSE '' END FROM dag_nodes WHERE run_id IN (` + placeholders + `) ORDER BY run_id,id LIMIT 2001`
	rows, err = tx.QueryContext(ctx, s.q(query), ids...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		if out.NodesExamined == 2000 {
			if out.Coverage != "unknown" {
				out.Coverage, out.Reason = "partial", "node_limit"
			}
			break
		}
		var item ToolProfileImpactItem
		var raw string
		if err = rows.Scan(&item.RunID, &item.NodeID, &item.NodeState, &item.WorkerID, &item.Attempt, &item.Fence, &raw); err != nil {
			break
		}
		out.NodesExamined++
		switch item.NodeState {
		case "succeeded", "failed", "skipped", "cancelled":
			continue
		case "pending", "ready", "running", "waiting", "reconciling":
		default:
			out.Coverage, out.Reason = "unknown", "source_unavailable"
			continue
		}
		var node dag.Node
		// This is the current stored Go Node representation, whose nil dependency
		// slice is valid JSON null. Validate bytes/duplicates first, then the
		// actual Node fields; a public wire presence checker would reject the
		// scheduler's own valid representation and fabricate unknown coverage.
		var document map[string]json.RawMessage
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if strictjson.Decode([]byte(raw), &document, 256<<10) != nil || document == nil || decoder.Decode(&node) != nil || node.ID != item.NodeID {
			out.Coverage, out.Reason = "unknown", "source_unavailable"
			continue
		}
		id, sha, bound, e := dag.ToolBinding(node)
		if e != nil {
			out.Coverage, out.Reason = "unknown", "source_unavailable"
			continue
		}
		if !bound || id != profile.ID || sha != profile.Profile.ProfileSHA256 {
			continue
		}
		out.ObservedReferenceCount++
		if len(out.Items) == 5 {
			out.ItemsTruncated = true
			continue
		}
		item.RunState = runs[item.RunID]
		out.Items = append(out.Items, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	return out, err
}
