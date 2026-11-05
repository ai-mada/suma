package dag

import (
	"context"
	"database/sql"
	"strings"
)

type PoolRunnerSlotCounts struct {
	Held        int
	Running     int
	Reconciling int
	Unknown     int
}

// ReadPoolRunnerSlotCountsTx reports held slots for one already-authorized page
// of registered runners. The caller owns tx and provides each runner's current
// worker identity. No underlying assignment identifiers or liveness claims
// leave this read.
func ReadPoolRunnerSlotCountsTx(ctx context.Context, tx *sql.Tx, dialect, pool string, runnerWorkers map[string]string) (map[string]PoolRunnerSlotCounts, error) {
	counts := make(map[string]PoolRunnerSlotCounts, len(runnerWorkers))
	if len(runnerWorkers) == 0 {
		return counts, nil
	}
	args := make([]any, 1, len(runnerWorkers)+1)
	args[0] = pool
	for id := range runnerWorkers {
		args = append(args, id)
	}
	placeholders := strings.Repeat("?,", len(runnerWorkers)-1) + "?"
	// Keep exact state and machine identifiers separate even on databases
	// whose default text collation is case-insensitive.
	runner, worker, state := "s.runner_id", "a.worker_id", "a.state"
	if dialect == "oceanbase" {
		runner, worker, state = "BINARY "+runner, "BINARY "+worker, "BINARY "+state
	}
	group := runner + "," + worker + "," + state
	query := `SELECT ` + group + `,COUNT(*)
		FROM dag_execution_pool_slots s
		LEFT JOIN dag_attempts a ON a.run_id=s.run_id AND a.node_id=s.node_id AND a.attempt=s.attempt AND a.fence=s.fence
		WHERE s.pool_id=? AND s.runner_id IN (` + placeholders + `)
		GROUP BY ` + group
	e := &Engine{dialect: dialect}
	rows, err := tx.QueryContext(ctx, e.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var worker, state sql.NullString
		var n int
		if err = rows.Scan(&id, &worker, &state, &n); err != nil {
			return nil, err
		}
		expectedWorker, ok := runnerWorkers[id]
		if !ok {
			continue
		}
		value := counts[id]
		value.Held += n
		switch {
		case worker.String == expectedWorker && state.String == "running":
			value.Running += n
		case worker.String == expectedWorker && state.String == "reconciling":
			value.Reconciling += n
		default:
			value.Unknown += n
		}
		counts[id] = value
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return counts, nil
}
