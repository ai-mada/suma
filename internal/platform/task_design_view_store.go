package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

var errTaskDesignSource = errors.New("task design source is unavailable or inconsistent")

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

// Initialize the index used by the current atomic adoption transaction.
func (s *Service) InitializeTaskDesignView(ctx context.Context) error {
	ddl := `CREATE TABLE IF NOT EXISTS suma_requirement_design_adoption_tasks(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,adoption_id VARCHAR(64) NOT NULL,adoption_version INTEGER NOT NULL,task_id VARCHAR(64) NOT NULL,task_key VARCHAR(64) NOT NULL,action VARCHAR(20) NOT NULL,task_version INTEGER NOT NULL,PRIMARY KEY(tenant_id,project_id,requirement_id,adoption_id,task_id),UNIQUE(tenant_id,project_id,requirement_id,adoption_id,task_key))`
	if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_design_adoption_task_version", "suma_requirement_design_adoption_tasks", "tenant_id", "project_id", "requirement_id", "task_id", "task_key", "adoption_version"); err != nil {
		return err
	}
	return nil
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
