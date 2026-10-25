package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ai-mada/suma/internal/store"
)

var errTaskDesignSource = errors.New("task design source is unavailable or inconsistent")

type taskDesignAdoptionPosition struct {
	tenant, project, requirement string
	version                      int
}

type taskDesignIndexedAdoption struct {
	id, key, action      string
	version, taskVersion int
}

func validateTaskDesignAdoption(a RequirementDesignAdoption, project, requirement, id string, version int) error {
	if project == "" || requirement == "" || a.ID == "" || a.ID != id || a.ProjectID != project || a.RequirementID != requirement || a.Version != version || version < 1 || a.Revision < 1 || a.RequirementVersion < 1 || len(a.SHA256) != 64 || len(a.PlanSHA256) != 64 || a.CreatedBy == "" || a.CreatedAt == "" || len(a.Tasks) < 1 || len(a.Tasks) > 100 {
		return errTaskDesignSource
	}
	keys, ids := map[string]bool{}, map[string]bool{}
	for _, task := range a.Tasks {
		if !designKey(task.Key) || task.TaskID == "" || task.Version < 1 || keys[task.Key] || ids[task.TaskID] {
			return errTaskDesignSource
		}
		switch task.Action {
		case "create", "update", "keep", "exclude":
		default:
			return errTaskDesignSource
		}
		keys[task.Key], ids[task.TaskID] = true, true
	}
	return nil
}

// The relation is a rebuildable seek index. It never substitutes for the
// original adoption JSON, immutable revision hash, or current read authority.
func (s *Service) insertTaskDesignAdoptionIndexTx(ctx context.Context, tx *sql.Tx, tenant, project, requirement string, a RequirementDesignAdoption) error {
	if err := validateTaskDesignAdoption(a, project, requirement, a.ID, a.Version); err != nil {
		return err
	}
	query := `INSERT INTO suma_requirement_design_adoption_tasks(tenant_id,project_id,requirement_id,adoption_id,adoption_version,task_id,task_key,action,task_version)VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(tenant_id,project_id,requirement_id,adoption_id,task_id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		query = `INSERT IGNORE INTO suma_requirement_design_adoption_tasks(tenant_id,project_id,requirement_id,adoption_id,adoption_version,task_id,task_key,action,task_version)VALUES(?,?,?,?,?,?,?,?,?)`
	}
	for _, task := range a.Tasks {
		if _, err := tx.ExecContext(ctx, s.q(query), tenant, project, requirement, a.ID, a.Version, task.TaskID, task.Key, task.Action, task.Version); err != nil {
			return err
		}
		var existing taskDesignIndexedAdoption
		if err := tx.QueryRowContext(ctx, s.q(`SELECT adoption_version,task_key,action,task_version FROM suma_requirement_design_adoption_tasks WHERE tenant_id=? AND project_id=? AND requirement_id=? AND adoption_id=? AND task_id=?`), tenant, project, requirement, a.ID, task.TaskID).Scan(&existing.version, &existing.key, &existing.action, &existing.taskVersion); err != nil {
			return err
		}
		if existing.version != a.Version || existing.key != task.Key || existing.action != task.Action || existing.taskVersion != task.Version {
			return errTaskDesignSource
		}
	}
	return nil
}

// Called only after the formal append-only adoption was inserted in this same
// approved transaction. Replays and keep-only recovery create no new relation.
func (s *Service) indexRequirementDesignAdoptionTx(ctx context.Context, auth *authorizedCommand, project, requirement string, a RequirementDesignAdoption) error {
	if err := assertDesignWrite(auth, project); err != nil {
		return err
	}
	if auth.purpose != purposeBrowserProjectApprove {
		return forbidden()
	}
	stored, err := s.taskDesignAdoptionTx(ctx, auth, project, requirement, a.ID)
	if err != nil {
		return err
	}
	if stringify(stored) != stringify(a) {
		return errTaskDesignSource
	}
	return s.insertTaskDesignAdoptionIndexTx(ctx, auth.tx, auth.target.tenantID, project, requirement, stored)
}

// Migration reads immutable sources in stable composite-key pages. Each page
// commits only derived relations; interruptions safely resume on the next run.
func (s *Service) MigrateTaskDesignView(ctx context.Context) error {
	ddl := `CREATE TABLE IF NOT EXISTS suma_requirement_design_adoption_tasks(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,adoption_id VARCHAR(64) NOT NULL,adoption_version INTEGER NOT NULL,task_id VARCHAR(64) NOT NULL,task_key VARCHAR(64) NOT NULL,action VARCHAR(20) NOT NULL,task_version INTEGER NOT NULL,PRIMARY KEY(tenant_id,project_id,requirement_id,adoption_id,task_id),UNIQUE(tenant_id,project_id,requirement_id,adoption_id,task_key))`
	if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_design_adoption_task_version", "suma_requirement_design_adoption_tasks", "tenant_id", "project_id", "requirement_id", "task_id", "task_key", "adoption_version"); err != nil {
		return err
	}
	position := taskDesignAdoptionPosition{}
	for {
		next, count, err := s.backfillTaskDesignAdoptionPage(ctx, position)
		if err != nil {
			return fmt.Errorf("task design index migration: %w", err)
		}
		if count == 0 {
			return nil
		}
		position = next
	}
}

func (s *Service) backfillTaskDesignAdoptionPage(ctx context.Context, position taskDesignAdoptionPosition) (taskDesignAdoptionPosition, int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return position, 0, err
	}
	defer tx.Rollback()
	query := `SELECT tenant_id,project_id,requirement_id,id,version,snapshot FROM suma_requirement_design_adoptions`
	args := []any{}
	if position.tenant != "" {
		query += ` WHERE tenant_id>? OR (tenant_id=? AND project_id>?) OR (tenant_id=? AND project_id=? AND requirement_id>?) OR (tenant_id=? AND project_id=? AND requirement_id=? AND version>?)`
		args = append(args, position.tenant, position.tenant, position.project, position.tenant, position.project, position.requirement, position.tenant, position.project, position.requirement, position.version)
	}
	query += ` ORDER BY tenant_id,project_id,requirement_id,version LIMIT 32`
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return position, 0, err
	}
	type source struct {
		position taskDesignAdoptionPosition
		adoption RequirementDesignAdoption
	}
	sources := []source{}
	for rows.Next() {
		var item source
		var id, raw string
		if err = rows.Scan(&item.position.tenant, &item.position.project, &item.position.requirement, &id, &item.position.version, &raw); err != nil {
			break
		}
		if item.position.tenant == "" {
			err = errTaskDesignSource
			break
		}
		if err = json.Unmarshal([]byte(raw), &item.adoption); err != nil {
			break
		}
		if err = validateTaskDesignAdoption(item.adoption, item.position.project, item.position.requirement, id, item.position.version); err != nil {
			break
		}
		sources = append(sources, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return position, 0, err
	}
	for _, item := range sources {
		if err = s.insertTaskDesignAdoptionIndexTx(ctx, tx, item.position.tenant, item.position.project, item.position.requirement, item.adoption); err != nil {
			return position, 0, err
		}
		position = item.position
	}
	return position, len(sources), tx.Commit()
}

func (s *Service) taskDesignAdoptionTx(ctx context.Context, auth *authorizedCommand, project, requirement, id string) (out RequirementDesignAdoption, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	var raw string
	var version int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,snapshot FROM suma_requirement_design_adoptions WHERE tenant_id=? AND project_id=? AND requirement_id=? AND id=?`), auth.target.tenantID, project, requirement, id).Scan(&version, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, errTaskDesignSource
	}
	if err != nil {
		return
	}
	if json.Unmarshal([]byte(raw), &out) != nil {
		return out, errTaskDesignSource
	}
	return out, validateTaskDesignAdoption(out, project, requirement, id, version)
}

func (s *Service) taskDesignRevisionTx(ctx context.Context, auth *authorizedCommand, project, requirement string, a RequirementDesignAdoption) (out RequirementDesignRevision, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT snapshot FROM suma_requirement_design_revisions WHERE tenant_id=? AND project_id=? AND requirement_id=? AND revision=?`), auth.target.tenantID, project, requirement, a.Revision).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, errTaskDesignSource
	}
	if err != nil {
		return
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.Revision != a.Revision || out.State != "published" || out.Requirement.ID != requirement || out.Requirement.Version != a.RequirementVersion || out.SHA256 != a.SHA256 || out.SHA256 != designRevisionHash(out) {
		return out, errTaskDesignSource
	}
	return
}
