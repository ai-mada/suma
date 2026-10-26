package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ai-mada/suma/internal/dag"
)

func handoffTerminal(state string, skipped bool) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled" || skipped && state == "skipped"
}

// The project scheduler guard is held while collecting this proof. It hashes
// original rows without publishing private execution output in the handoff DTO.
func (s *Service) handoffBasisTx(ctx context.Context, auth *authorizedCommand, task Record) (TaskHandoffBasis, []string, error) {
	basis := TaskHandoffBasis{TaskVersion: task.Version, TaskBasisSHA256: taskBasis(task), RequirementID: task.RequirementID, FromOwnerID: task.OwnerID, SourceRunID: task.RunID}
	blockers := []string{}
	if task.Status == "archived" || task.Status == "deleted" {
		blockers = append(blockers, handoffBasisChanged)
	}
	if task.RequirementID != "" {
		err := auth.tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, task.ProjectID, task.RequirementID).Scan(&basis.RequirementVersion)
		if errors.Is(err, sql.ErrNoRows) {
			blockers = append(blockers, handoffBasisChanged)
		} else if err != nil {
			return basis, blockers, err
		}
	}
	hash := sha256.New()
	write := func(value any) { fmt.Fprintln(hash, stringify(value)) }
	write(task.Data)
	queue := []string{}
	if task.RunID != "" {
		queue = append(queue, task.RunID)
	}
	seen := map[string]bool{}
	unsafe := false
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(seen) > 256 {
			return basis, blockers, invalid(handoffRunUnsafe)
		}
		var version int
		var state string
		err := auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, task.ProjectID, id).Scan(&version, &state)
		if errors.Is(err, sql.ErrNoRows) {
			unsafe = true
			continue
		}
		if err != nil {
			return basis, blockers, err
		}
		if id == task.RunID {
			basis.SourceRunVersion, basis.SourceRunState = version, state
		}
		write([]any{id, version, state})
		unsafe = unsafe || !handoffTerminal(state, false)
		rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,state,attempt,fence,worker_id,spec,output,error_text FROM dag_nodes WHERE run_id=? ORDER BY id`), id)
		if err != nil {
			return basis, blockers, err
		}
		for rows.Next() {
			var node, nodeState, worker, spec, output, errorText string
			var attempt int
			var fence int64
			if err = rows.Scan(&node, &nodeState, &attempt, &fence, &worker, &spec, &output, &errorText); err != nil {
				rows.Close()
				return basis, blockers, err
			}
			unsafe = unsafe || !handoffTerminal(nodeState, true)
			write([]any{node, nodeState, attempt, fence, worker, spec, output, errorText})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return basis, blockers, err
		}
		rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT node_id,attempt,fence,worker_id,state,result FROM dag_attempts WHERE run_id=? ORDER BY node_id,attempt`), id)
		if err != nil {
			return basis, blockers, err
		}
		for rows.Next() {
			var node, worker, attemptState, raw string
			var attempt int
			var fence int64
			if err = rows.Scan(&node, &attempt, &fence, &worker, &attemptState, &raw); err != nil {
				rows.Close()
				return basis, blockers, err
			}
			unsafe = unsafe || !handoffTerminal(attemptState, false)
			if raw != "" {
				var result dag.Result
				if json.Unmarshal([]byte(raw), &result) != nil || result.OutcomeUnknown {
					unsafe = true
				}
			}
			write([]any{node, attempt, fence, worker, attemptState, raw})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return basis, blockers, err
		}
		rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT child_run_id FROM dag_subruns WHERE parent_run_id=? UNION SELECT compensation_run_id FROM dag_compensations WHERE original_run_id=? ORDER BY 1`), id, id)
		if err != nil {
			return basis, blockers, err
		}
		for rows.Next() {
			var child string
			if err = rows.Scan(&child); err != nil {
				rows.Close()
				return basis, blockers, err
			}
			queue = append(queue, child)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return basis, blockers, err
		}
	}
	for _, kind := range []string{"tests", "evidence"} {
		if task.RunID == "" {
			break
		}
		rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,version,status,run_id,data FROM suma_`+kind+` WHERE tenant_id=? AND project_id=? AND run_id=? ORDER BY id`), auth.target.tenantID, task.ProjectID, task.RunID)
		if err != nil {
			return basis, blockers, err
		}
		for rows.Next() {
			var id, status, run, raw string
			var version int
			if err = rows.Scan(&id, &version, &status, &run, &raw); err != nil {
				rows.Close()
				return basis, blockers, err
			}
			write([]any{kind, id, version, status, run, raw})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return basis, blockers, err
		}
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,task_version,run_id,reviewer_id,decision,reason FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? ORDER BY id`), auth.target.tenantID, task.ProjectID, task.ID)
	if err != nil {
		return basis, blockers, err
	}
	for rows.Next() {
		var id, run, actor, decision, reason string
		var version int
		if err = rows.Scan(&id, &version, &run, &actor, &decision, &reason); err != nil {
			rows.Close()
			return basis, blockers, err
		}
		write([]any{id, version, run, actor, decision, reason})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return basis, blockers, err
	}
	basis.DeliverySHA256 = hex.EncodeToString(hash.Sum(nil))
	if unsafe {
		blockers = append(blockers, handoffRunUnsafe)
	}
	return basis, blockers, nil
}
