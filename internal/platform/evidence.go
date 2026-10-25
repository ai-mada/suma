package platform

import (
	"context"
	"database/sql"
)

// Strong gates call this only after organization/credential authorization and
// the shared scheduler project guard. Display readers use snapshot queries.
func (s *Service) executionReadSQL(query string, current bool) string {
	if current && (s.Dialect == "oceanbase" || s.Dialect == "postgres") {
		return query + " FOR UPDATE"
	}
	return query
}

func (s *Service) evidenceSnapshotOptions() *sql.TxOptions {
	options := &sql.TxOptions{ReadOnly: true}
	if s.Dialect == "oceanbase" || s.Dialect == "postgres" {
		options.Isolation = sql.LevelRepeatableRead
	}
	return options
}

// Mutation gates may perform authorization reads before taking the project
// guard. Read committed prevents a pre-guard repeatable-read snapshot from
// hiding evidence committed while the gate was waiting for that guard.
func (s *Service) executionWriteOptions() *sql.TxOptions {
	if s.Dialect == "oceanbase" || s.Dialect == "postgres" {
		return &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	return nil
}

// checkTaskEvidenceTx validates the complete business dependency chain against
// canonical versions, independently of the latest displayed acceptance status.
func (s *Service) checkTaskEvidenceTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, visiting, valid map[string]bool) error {
	return s.checkTaskEvidenceReadTx(ctx, tx, p, project, id, visiting, valid, false)
}

func (s *Service) checkCurrentTaskEvidenceTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, visiting, valid map[string]bool) error {
	return s.checkTaskEvidenceReadTx(ctx, tx, p, project, id, visiting, valid, true)
}

func (s *Service) checkTaskEvidenceReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, visiting, valid map[string]bool, current bool) error {
	if valid[id] {
		return nil
	}
	if visiting[id] || len(visiting) > 10000 {
		return conflict("任务证据依赖链无效")
	}
	visiting[id] = true
	defer delete(visiting, id)
	task, err := scanRecord(tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, current)), p.TenantID, project, id))
	if err != nil || task.Status != "accepted" {
		return conflict("前置任务验收已失效")
	}
	runID, _ := task.Data["verified_run_id"].(string)
	if runID != task.RunID || task.Data["run_task_basis"] != taskBasis(task) {
		return conflict("前置任务内容与验收依据不一致")
	}
	if err = s.checkEvidenceReadTx(ctx, tx, p.TenantID, project, runID, 0, map[string]bool{}, current); err != nil {
		return err
	}
	if task.RequirementID != "" {
		var v int
		if err = tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT version FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`, current)), p.TenantID, project, task.RequirementID).Scan(&v); err != nil {
			return err
		}
		stored, _ := task.Data["requirement_version"].(float64)
		if int(stored) != v {
			return conflict("前置需求版本已变化")
		}
	}
	basis, _ := task.Data["dependency_basis"].(map[string]any)
	for _, dep := range task.DependsOn {
		if err = s.checkTaskEvidenceReadTx(ctx, tx, p, project, dep, visiting, valid, current); err != nil {
			return err
		}
		upstream, e := scanRecord(tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, current)), p.TenantID, project, dep))
		if e != nil {
			return e
		}
		if basis[dep] != digest(stringify(map[string]any{"version": upstream.Version, "verified_run_id": upstream.Data["verified_run_id"]})) {
			return conflict("前置任务依赖版本已变化")
		}
	}
	valid[id] = true
	return nil
}

// InvalidateDependentsTx propagates invalidation through the business task graph.
// It runs in the transaction that changed the original evidence.
func (s *Service) InvalidateDependentsTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project string, roots []string, includeRoots bool) error {
	if err := assertProjectCommand(tx, auth, project); err != nil {
		return err
	}
	p := auth.currentPrincipal
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=?`), p.TenantID, project)
	if err != nil {
		return err
	}
	tasks := map[string]Record{}
	children := map[string][]string{}
	for rows.Next() {
		task, e := scanRecord(rows)
		if e != nil {
			rows.Close()
			return e
		}
		tasks[task.ID] = task
		for _, dep := range task.DependsOn {
			children[dep] = append(children[dep], task.ID)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	queue := append([]string{}, roots...)
	seen := map[string]bool{}
	rootSet := map[string]bool{}
	for _, id := range roots {
		rootSet[id] = true
	}
	for pos := 0; pos < len(queue); pos++ {
		id := queue[pos]
		if seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, children[id]...)
		task, ok := tasks[id]
		if !ok || task.Status != "accepted" || rootSet[id] && !includeRoots {
			continue
		}
		delete(task.Data, "verified_run_id")
		task.Data["acceptance_invalidated_reason"] = "需求、前置任务或关联执行依据已变化"
		res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET status='in_progress',version=version+1,updated_at=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), now(), stringify(task.Data), p.TenantID, project, id, task.Version)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return conflict("任务验收并发变化")
		}
		if e = s.commandActivity(ctx, auth, "task.acceptance_invalidated", id, task.Title); e != nil {
			return e
		}
	}
	return s.ReopenPlanningTx(ctx, tx, auth, project)
}

func (s *Service) checkEvidenceTx(ctx context.Context, tx *sql.Tx, tenant, project, runID string, depth int, seen map[string]bool) error {
	return s.checkEvidenceReadTx(ctx, tx, tenant, project, runID, depth, seen, false)
}

func (s *Service) checkCurrentEvidenceTx(ctx context.Context, tx *sql.Tx, tenant, project, runID string, depth int, seen map[string]bool) error {
	return s.checkEvidenceReadTx(ctx, tx, tenant, project, runID, depth, seen, true)
}

func (s *Service) evidenceCompensatedTx(ctx context.Context, tx *sql.Tx, runID string, current bool) (bool, error) {
	if current {
		var id string
		err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT compensation_run_id FROM dag_compensations WHERE original_run_id=?`, true)), runID).Scan(&id)
		if err == sql.ErrNoRows {
			return false, nil
		}
		return err == nil, err
	}
	var count int
	err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_compensations WHERE original_run_id=?`), runID).Scan(&count)
	return count != 0, err
}

func (s *Service) checkEvidenceReadTx(ctx context.Context, tx *sql.Tx, tenant, project, runID string, depth int, seen map[string]bool, current bool) error {
	if runID == "" || depth > 32 || seen[runID] {
		return conflict("前置验收缺少有效执行依据")
	}
	seen[runID] = true
	var state string
	if err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`, current)), tenant, project, runID).Scan(&state); err != nil || state != "succeeded" {
		return conflict("执行证据不再有效")
	}
	compensated, err := s.evidenceCompensatedTx(ctx, tx, runID, current)
	if err != nil {
		return err
	}
	if compensated {
		return conflict("原执行已进入补偿，需要重新交付")
	}
	rows, err := tx.QueryContext(ctx, s.q(s.executionReadSQL(`SELECT x.child_run_id FROM dag_subruns x JOIN dag_nodes n ON n.run_id=x.parent_run_id AND n.id=x.node_id AND n.attempt=x.attempt WHERE x.parent_run_id=? ORDER BY x.child_run_id`, current)), runID)
	if err != nil {
		return err
	}
	children := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		children = append(children, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = s.checkEvidenceReadTx(ctx, tx, tenant, project, child, depth+1, seen, current); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) InvalidateCompensationTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, runID string) error {
	if err := assertProjectCommand(tx, auth, project); err != nil {
		return err
	}
	p := auth.currentPrincipal
	ids := []string{runID}
	seen := map[string]bool{runID: true}
	for pos := 0; pos < len(ids); pos++ {
		if len(ids) > 10000 {
			return invalid("执行证据引用超出限制")
		}
		rows, err := tx.QueryContext(ctx, s.q(`SELECT x.parent_run_id FROM dag_subruns x JOIN dag_runs r ON r.id=x.parent_run_id WHERE x.child_run_id=? AND r.tenant_id=? AND r.project_id=?`), ids[pos], p.TenantID, project)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	for _, id := range ids {
		rows, err := tx.QueryContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND run_id=? AND status='accepted'`), p.TenantID, project, id)
		if err != nil {
			return err
		}
		tasks := []Record{}
		for rows.Next() {
			r, e := scanRecord(rows)
			if e != nil {
				rows.Close()
				return e
			}
			tasks = append(tasks, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, task := range tasks {
			delete(task.Data, "verified_run_id")
			task.Data["acceptance_invalidated_reason"] = "关联运行或子运行已开始补偿"
			res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET status='in_progress',version=version+1,updated_at=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), now(), stringify(task.Data), p.TenantID, project, task.ID, task.Version)
			if e != nil {
				return e
			}
			n, _ := res.RowsAffected()
			if n != 1 {
				return conflict("任务验收并发变化")
			}
			if e = s.commandActivity(ctx, auth, "task.acceptance_invalidated", task.ID, task.Title); e != nil {
				return e
			}
			if e = s.InvalidateDependentsTx(ctx, tx, auth, project, []string{task.ID}, false); e != nil {
				return e
			}
		}
	}
	return nil
}
