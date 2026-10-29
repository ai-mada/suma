package dag

import (
	"context"
	"database/sql"
	"fmt"
)

// CreateManagedRunBound atomically freezes a service-generated workflow and
// reserved run ID, its domain binding and ready queue. The two callbacks obey
// organization -> scheduler -> domain lock ordering and use tx exclusively.
func (e *Engine) CreateManagedRunBound(ctx context.Context, tenant, project, runID, operationID string, w Workflow, before func(*sql.Tx) error, bind func(*sql.Tx, string) error) (Run, error) {
	var out Run
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	if !identifier.MatchString(runID) || len(runID) > 92 || before == nil || bind == nil {
		return out, dagDiagnostic(fmt.Errorf("%w: managed run requires reserved ID and authority/binding hooks", ErrInvalid), "dag.run.managed_authority", nil)
	}
	w.ID = "managed-" + runID
	w.Version = 1
	var err error
	w, err = normalize(w)
	if err != nil {
		return out, err
	}
	hash := fingerprint(map[string]any{"run_id": runID, "workflow": w})
	err = e.writeBefore(ctx, before, func(t *transaction) error {
		previous, err := t.operation(tenant, project, "managed_run", operationID, hash)
		if err != nil {
			return err
		}
		if previous != "" {
			r, err := load(ctx, t.tx, e, tenant, project, previous)
			if err == nil {
				out = r.public()
			}
			return err
		}
		t.allowBoundCreation = true
		if err = t.validateReferences(tenant, project, w); err != nil {
			return err
		}
		t.allowBoundCreation = false
		if err = t.exec("INSERT INTO dag_workflows(tenant_id,project_id,id,version,name,spec)VALUES(?,?,?,?,?,?)", tenant, project, w.ID, 1, w.Name, encoded(w)); err != nil {
			return err
		}
		if err = t.exec("INSERT INTO dag_managed_workflows(tenant_id,project_id,workflow_id,run_id)VALUES(?,?,?,?)", tenant, project, w.ID, runID); err != nil {
			return err
		}
		r, err := t.createRunID(tenant, project, runID, w, bind)
		if err != nil {
			return err
		}
		if err = t.remember(tenant, project, "managed_run", operationID, hash, r.ID); err != nil {
			return err
		}
		out = r.public()
		return nil
	})
	return out, err
}
func (e *Engine) ManagedRun(ctx context.Context, tenant, project, runID string) (bool, error) {
	var n int
	err := e.db.QueryRowContext(ctx, e.q("SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND run_id=?"), tenant, project, runID).Scan(&n)
	return n > 0, err
}
