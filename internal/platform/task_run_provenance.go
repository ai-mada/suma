package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/ai-mada/suma/internal/store"
)

const taskRunProvenanceInvalid = "Task execution history request is invalid"
const taskRunProvenanceConflict = "Execution run is already associated with another task or scope"

type TaskRunProvenance struct {
	TaskID      string  `json:"task_id"`
	RunID       string  `json:"run_id"`
	TaskVersion *int    `json:"task_version"`
	BoundAt     *string `json:"bound_at"`
	ObservedAt  string  `json:"observed_at"`
	BindingKind string  `json:"binding_kind"`
	PlanID      string  `json:"plan_id"`
	Current     bool    `json:"current"`
}

type TaskRunProvenancePage struct {
	Items      []TaskRunProvenance `json:"items"`
	Total      int                 `json:"total"`
	NextCursor string              `json:"next_cursor"`
}

func (s *Service) InitializeTaskRunProvenance(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_task_run_provenance(run_id VARCHAR(100) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,observed_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`))
	if err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_run_provenance_scope", "suma_task_run_provenance", "tenant_id", "project_id", "task_id", "run_id")
}

// Resolve ownership from protected snapshots as well as the current task
// pointer. A mutable task pointer cannot give another task a managed run.
func (s *Service) taskRunSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, task, run string) (plan string, version *int, err error) {
	var runTenant, runProject string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT tenant_id,project_id FROM dag_runs WHERE id=?`), run).Scan(&runTenant, &runProject); err != nil {
		return
	}
	if runTenant != tenant || runProject != project {
		err = conflict(taskRunProvenanceConflict)
		return
	}
	r, found, e := s.candidateCompositionExecutionRecordTx(ctx, tx, tenant, project, run)
	if e != nil {
		err = e
		return
	}
	if found {
		if r.Plan.TaskID != task || r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r) {
			err = conflict(taskRunProvenanceConflict)
			return
		}
		v := r.Plan.TaskVersion + 1
		return r.Plan.ID, &v, nil
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return
	}
	var dispatch TaskDispatchPlan
	if json.Unmarshal([]byte(raw), &dispatch) != nil || dispatch.TaskID != task || dispatch.RunID != run || dispatch.TaskVersion < 1 {
		err = conflict(taskRunProvenanceConflict)
		return
	}
	v := dispatch.TaskVersion + 1
	return dispatch.ID, &v, nil
}

func (s *Service) recordTaskRunProvenanceTx(ctx context.Context, tx *sql.Tx, tenant, project, task, run, kind, stamp string, boundVersion *int) error {
	plan, sourceVersion, err := s.taskRunSourceTx(ctx, tx, tenant, project, task, run)
	if err != nil {
		return err
	}
	var other int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tasks WHERE run_id=? AND (tenant_id<>? OR project_id<>? OR id<>?)`), run, tenant, project, task).Scan(&other); err != nil {
		return err
	}
	if other != 0 {
		return conflict(taskRunProvenanceConflict)
	}
	var storedTenant, storedProject, storedTask string
	err = tx.QueryRowContext(ctx, s.q(`SELECT tenant_id,project_id,task_id FROM suma_task_run_provenance WHERE run_id=?`), run).Scan(&storedTenant, &storedProject, &storedTask)
	if err == nil {
		if storedTenant != tenant || storedProject != project || storedTask != task {
			return conflict(taskRunProvenanceConflict)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	out := TaskRunProvenance{TaskID: task, RunID: run, ObservedAt: stamp, BindingKind: kind, PlanID: plan, TaskVersion: sourceVersion}
	if kind == "bound" {
		out.BoundAt = &stamp
		out.TaskVersion = boundVersion
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_run_provenance(run_id,tenant_id,project_id,task_id,observed_at,payload)VALUES(?,?,?,?,?,?)`), run, tenant, project, task, stamp, stringify(out))
	return err
}

func (s *Service) bindTaskRunProvenanceTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, task Record, newRun string) error {
	if err := assertProjectCommand(tx, auth, task.ProjectID); err != nil {
		return err
	}
	stamp := automationStamp(auth.checkedAt)
	if task.RunID != "" {
		if err := s.recordTaskRunProvenanceTx(ctx, tx, auth.target.tenantID, task.ProjectID, task.ID, task.RunID, "observed", stamp, nil); err != nil {
			return err
		}
	}
	if task.RunID == newRun {
		return nil
	}
	version := task.Version + 1
	return s.recordTaskRunProvenanceTx(ctx, tx, auth.target.tenantID, task.ProjectID, task.ID, newRun, "bound", stamp, &version)
}

func (s *Service) ListTaskRunProvenance(ctx context.Context, actor Actor, project, task string, limit int, before string) (out TaskRunProvenancePage, err error) {
	out.Items = []TaskRunProvenance{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(task, false) || limit < 1 || limit > 100 || len(before) > 4096 {
		return out, invalid(taskRunProvenanceInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	current, err := s.getRecordTx(ctx, auth, project, "tasks", task)
	if err != nil {
		return out, err
	}
	position, err := automationDecodeCursor(auth.currentPrincipal, project, "task_run_history:"+task, before)
	if err != nil {
		return out, err
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_run_provenance WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, task).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT payload FROM suma_task_run_provenance WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id>? ORDER BY run_id LIMIT ?`), auth.target.tenantID, project, task, position, limit+1)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var item TaskRunProvenance
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			return out, err
		}
		item.Current = item.RunID == current.RunID
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, "task_run_history:"+task, out.Items[len(out.Items)-1].RunID)
	}
	return out, nil
}
