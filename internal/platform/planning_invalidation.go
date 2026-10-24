package platform

import (
	"context"
	"database/sql"
)

// ReopenPlanningTx keeps a completed period from displaying completion after
// any of its formal task evidence becomes invalid. It shares the source commit.
func (s *Service) ReopenPlanningTx(ctx context.Context, tx *sql.Tx, p Principal, project string) error {
	rows, e := tx.QueryContext(ctx, s.q(`SELECT p.id,p.version,p.title FROM suma_planning_periods p WHERE p.tenant_id=? AND p.project_id=? AND p.status='completed' AND EXISTS(SELECT 1 FROM suma_task_plans tp JOIN suma_tasks t ON t.id=tp.task_id AND t.tenant_id=tp.tenant_id AND t.project_id=tp.project_id WHERE tp.tenant_id=p.tenant_id AND tp.project_id=p.project_id AND ((p.kind='iterations' AND tp.iteration_id=p.id) OR (p.kind='milestones' AND tp.milestone_id=p.id)) AND t.status<>'accepted')`), p.TenantID, project)
	if e != nil {
		return e
	}
	type changed struct {
		id, title string
		version   int
	}
	items := []changed{}
	for rows.Next() {
		var item changed
		if e = rows.Scan(&item.id, &item.version, &item.title); e != nil {
			rows.Close()
			return e
		}
		items = append(items, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, item := range items {
		res, er := tx.ExecContext(ctx, s.q(`UPDATE suma_planning_periods SET status='active',version=version+1,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND status='completed'`), now(), p.TenantID, project, item.id, item.version)
		if er != nil {
			return er
		}
		n, _ := res.RowsAffected()
		if n != 1 {
			return conflict("计划完成依据已变化，请重新核对")
		}
		if er = s.activity(ctx, tx, p, project, "planning.completion_invalidated", item.id, item.title); er != nil {
			return er
		}
	}
	return nil
}
