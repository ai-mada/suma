package dag

import (
	"database/sql"
	"fmt"
	"strings"
)

type claimCandidate struct{ run, node string }
type claimScan struct {
	turn, head, nodeSequence, nodeRank int64
	run, node                          string
}
type claimTurn struct {
	run        string
	turn, head int64
}

func (t *transaction) refreshRunTurn(r *runState) error {
	var first sql.NullInt64
	if err := t.row(`SELECT MIN(sequence) FROM dag_ready_v2 WHERE run_id=?`, r.ID).Scan(&first); err != nil {
		return err
	}
	if !first.Valid {
		return t.exec(`DELETE FROM dag_scheduler_run_turns WHERE run_id=?`, r.ID)
	}
	var last int64
	err := t.row(`SELECT last_claim_sequence FROM dag_run_fairness WHERE run_id=?`, r.ID).Scan(&last)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	enabled := 0
	if r.State == "running" {
		enabled = 1
	}
	if err = t.insertIgnore(`INSERT INTO dag_scheduler_run_turns(run_id,tenant_id,project_id,enabled,last_sequence,first_ready_sequence)VALUES(?,?,?,?,?,?)`, "run_id", r.ID, r.TenantID, r.ProjectID, enabled, last, first.Int64); err != nil {
		return err
	}
	return t.exec(`UPDATE dag_scheduler_run_turns SET enabled=?,last_sequence=?,first_ready_sequence=? WHERE run_id=?`, enabled, last, first.Int64, r.ID)
}

func (t *transaction) storeClaimScan(tenant, project, worker, hash string, c claimScan) error {
	if err := t.insertIgnore(`INSERT INTO dag_claim_ready_scans(tenant_id,project_id,worker_id,capability_sha256,last_turn,last_head,last_run_id,last_node_rank,last_node_sequence,last_node_id)VALUES(?,?,?,?,0,0,'',0,0,'')`, "tenant_id,project_id,worker_id,capability_sha256", tenant, project, worker, hash); err != nil {
		return err
	}
	return t.exec(`UPDATE dag_claim_ready_scans SET last_turn=?,last_head=?,last_run_id=?,last_node_rank=?,last_node_sequence=?,last_node_id=? WHERE tenant_id=? AND project_id=? AND worker_id=? AND capability_sha256=?`, c.turn, c.head, c.run, c.nodeRank, c.nodeSequence, c.node, tenant, project, worker, hash)
}
func (t *transaction) resetClaimScan(tenant, project, worker string, caps []string) error {
	return t.storeClaimScan(tenant, project, worker, fingerprint(caps), claimScan{})
}

func capabilityFilter(caps []string) (string, []any) {
	if len(caps) == 0 {
		return "1=0", nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(caps)), ",")
	commands := []string{}
	params := []any{}
	for _, c := range caps {
		params = append(params, c)
		if strings.HasPrefix(c, "command:") {
			commands = append(commands, strings.TrimPrefix(c, "command:"))
		}
	}
	extra := `(q.required_capability='' OR q.required_capability IN (` + placeholders + `))`
	command := `1=0`
	if len(commands) > 0 {
		command = `q.command_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(commands)), ",") + `)`
		for _, c := range commands {
			params = append(params, c)
		}
	}
	other := `((q.required_capability<>'' AND q.required_capability IN (` + placeholders + `)) OR (q.required_capability='' AND q.node_type IN (` + placeholders + `)))`
	for _, c := range caps {
		params = append(params, c)
	}
	for _, c := range caps {
		params = append(params, c)
	}
	return extra + ` AND ((q.node_type IN ('command','agent','test') AND ` + command + `) OR (q.node_type NOT IN ('command','agent','test') AND ` + other + `))`, params
}

// At most 16 indexed run heads and 64 matching node candidates per request.
// Unsuccessful scans persist their position; successful admissions reset it so
// every new turn again considers the least-served runnable run.
func (t *transaction) claimCandidateBatch(tenant, project, worker string, caps []string) ([]claimCandidate, error) {
	return t.claimCandidateBatchPool(tenant, project, worker, caps, "", 64)
}
func (t *transaction) claimCandidateBatchPool(tenant, project, worker string, caps []string, pool string, limit int) ([]claimCandidate, error) {
	out := []claimCandidate{}
	if len(caps) == 0 {
		return out, nil
	}
	hash := fingerprint(caps)
	scan := claimScan{}
	err := t.row(`SELECT last_turn,last_head,last_run_id,last_node_rank,last_node_sequence,last_node_id FROM dag_claim_ready_scans WHERE tenant_id=? AND project_id=? AND worker_id=? AND capability_sha256=?`, tenant, project, worker, hash).Scan(&scan.turn, &scan.head, &scan.run, &scan.nodeRank, &scan.nodeSequence, &scan.node)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	filter, filterParams := capabilityFilter(caps)
	visited := map[string]bool{}
	wrapped := false
	poolFilter := ` AND NOT EXISTS(SELECT 1 FROM dag_run_execution_routes rr WHERE rr.run_id=dag_scheduler_run_turns.run_id AND rr.pool_id<>'')`
	if pool != "" {
		poolFilter = ` AND EXISTS(SELECT 1 FROM dag_run_execution_routes rr WHERE rr.run_id=dag_scheduler_run_turns.run_id AND rr.pool_id=?)`
	}
	for turns := 0; turns < 16 && len(out) < limit; turns++ {
		var turn claimTurn
		resume := false
		if scan.run != "" && scan.nodeSequence < maxSchedulingInteger {
			args := []any{tenant, project, scan.run, scan.turn, scan.head}
			if pool != "" {
				args = append(args, pool)
			}
			err = t.row(`SELECT run_id,last_sequence,first_ready_sequence FROM dag_scheduler_run_turns WHERE tenant_id=? AND project_id=? AND enabled=1 AND run_id=? AND last_sequence=? AND first_ready_sequence=?`+poolFilter, args...).Scan(&turn.run, &turn.turn, &turn.head)
			if err == nil {
				resume = true
			} else if err != sql.ErrNoRows {
				return nil, err
			}
		}
		if !resume {
			query := `SELECT run_id,last_sequence,first_ready_sequence FROM dag_scheduler_run_turns WHERE tenant_id=? AND project_id=? AND enabled=1`
			args := []any{tenant, project}
			query += poolFilter
			if pool != "" {
				args = append(args, pool)
			}
			if scan.run != "" {
				query += ` AND (last_sequence>? OR (last_sequence=? AND (first_ready_sequence>? OR (first_ready_sequence=? AND run_id>?))))`
				args = append(args, scan.turn, scan.turn, scan.head, scan.head, scan.run)
			}
			query += ` ORDER BY last_sequence,first_ready_sequence,run_id LIMIT 1`
			err = t.row(query, args...).Scan(&turn.run, &turn.turn, &turn.head)
			if err == sql.ErrNoRows {
				if wrapped || scan.run == "" {
					break
				}
				scan = claimScan{}
				wrapped = true
				turns--
				continue
			}
			if err != nil {
				return nil, err
			}
			scan = claimScan{turn: turn.turn, head: turn.head, run: turn.run}
		}
		if visited[turn.run] {
			break
		}
		visited[turn.run] = true
		remaining := limit - len(out)
		query, args := readyCandidateStatement(turn.run, filter, filterParams, scan, resume, remaining)
		rows, err := t.rows(query, args...)
		if err != nil {
			return nil, err
		}
		count := 0
		for rows.Next() {
			var node string
			var seq, nodeRank int64
			if err = rows.Scan(&node, &seq, &nodeRank); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, claimCandidate{run: turn.run, node: node})
			scan = claimScan{turn: turn.turn, head: turn.head, run: turn.run, nodeSequence: seq, nodeRank: nodeRank, node: node}
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if count < remaining {
			scan.nodeSequence = maxSchedulingInteger
			scan.node = ""
		}
		if len(out) > limit {
			return nil, fmt.Errorf("%w: claim scan bound exceeded", ErrInvalid)
		}
	}
	if err = t.storeClaimScan(tenant, project, worker, hash, scan); err != nil {
		return nil, err
	}
	return out, nil
}

func readyCandidateStatement(run, filter string, filterParams []any, scan claimScan, resume bool, remaining int) (string, []any) {
	query := `SELECT k.node_id,k.ready_sequence,k.rank_at FROM dag_ready_admission_order k JOIN dag_ready_v2 q ON q.run_id=k.run_id AND q.node_id=k.node_id WHERE k.run_id=? AND ` + filter
	args := append([]any{run}, filterParams...)
	if resume {
		query += ` AND (k.rank_at,k.ready_sequence,k.node_id)>(?,?,?)`
		args = append(args, scan.nodeRank, scan.nodeSequence, scan.node)
	}
	query += ` ORDER BY k.rank_at,k.ready_sequence,k.node_id LIMIT ?`
	return query, append(args, remaining)
}
