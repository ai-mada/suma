package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

const cloudDelegationColumns = "id,tenant_id,project_id,owner_id,connection_id,run_id,node_id,version,state,payload"
const cloudSubmissionColumns = "id,tenant_id,project_id,delegation_id,run_id,node_id,version,state,assignment_sha256,worker_credential_hash,token_generation,prepared_nonce,payload"

var cloudSchemaTables = []struct {
	name, columns, ddl string
	unique             []string
}{
	{"suma_workbuddy_cloud_delegations", cloudDelegationColumns, `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_delegations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,connection_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,version INTEGER NOT NULL,state VARCHAR(32) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,run_id,node_id))`, []string{"tenant_id,project_id,run_id,node_id"}},
	{"suma_workbuddy_cloud_submissions", cloudSubmissionColumns, `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_submissions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,delegation_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,version INTEGER NOT NULL,state VARCHAR(32) NOT NULL,assignment_sha256 VARCHAR(64) NOT NULL,worker_credential_hash VARCHAR(64) NOT NULL,token_generation INTEGER NOT NULL,prepared_nonce VARCHAR(64) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,run_id,node_id),UNIQUE(tenant_id,project_id,delegation_id))`, []string{"tenant_id,project_id,run_id,node_id", "tenant_id,project_id,delegation_id"}},
}

func (s *Service) validateWorkBuddyCloudInitialSchema(ctx context.Context) error {
	var exists string
	switch s.Dialect {
	case "sqlite":
		exists = `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		exists = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		exists = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: cloud schema dialect", store.ErrInitialSchema)
	}
	present := 0
	for _, t := range cloudSchemaTables {
		var n int
		if err := s.DB.QueryRowContext(ctx, s.q(exists), t.name).Scan(&n); err != nil {
			return err
		}
		present += n
	}
	if present == 0 {
		return nil
	}
	if present != len(cloudSchemaTables) {
		return fmt.Errorf("%w: complete WorkBuddy cloud domain required", store.ErrInitialSchema)
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component=?`), "platform").Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: current cloud baseline required", store.ErrInitialSchema)
	}
	for _, t := range cloudSchemaTables {
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+t.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: cloud columns unavailable", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		wanted := strings.Split(t.columns, ",")
		seen := map[string]bool{}
		for _, c := range columns {
			seen[c] = true
		}
		if len(wanted) != len(columns) {
			return fmt.Errorf("%w: exact cloud columns required", store.ErrInitialSchema)
		}
		for _, c := range wanted {
			if !seen[c] {
				return fmt.Errorf("%w: exact cloud columns required", store.ErrInitialSchema)
			}
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, t.name)
		if err != nil {
			return err
		}
		if primary != "id" {
			return fmt.Errorf("%w: cloud primary key required", store.ErrInitialSchema)
		}
		for _, u := range t.unique {
			if !unique[u] {
				return fmt.Errorf("%w: cloud uniqueness required", store.ErrInitialSchema)
			}
		}
	}
	return nil
}
func (s *Service) InitializeWorkBuddyCloud(ctx context.Context) error {
	if err := s.validateWorkBuddyCloudInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, t := range cloudSchemaTables {
		if _, err := s.DB.ExecContext(ctx, s.ddl(t.ddl)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workbuddy_cloud_progress_page", "suma_workbuddy_cloud_submissions", "tenant_id", "project_id", "id")
}

type cloudDelegationRow struct {
	wc.CloudDelegation
	tenant string
}
type cloudSubmissionRow struct {
	wc.CloudSubmission
	tenant, assignmentHash, workerHash, nonce string
	tokenGeneration                           int
}

func cloudInvalid() error  { return invalid("workbuddy_cloud_invalid") }
func cloudConflict() error { return conflict("workbuddy_cloud_changed") }
func cloudPending(id, state string) error {
	return &APIError{409, "workbuddy_cloud_reconciliation_required", "workbuddy_cloud_reconciliation_required", map[string]any{"submission_id": id, "state": state}}
}

func (w *WorkBuddyConnections) cloudDelegationTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, current bool) (out cloudDelegationRow, err error) {
	query := `SELECT id,tenant_id,project_id,owner_id,connection_id,run_id,node_id,version,state,payload FROM suma_workbuddy_cloud_delegations WHERE tenant_id=? AND project_id=? AND id=?`
	if current {
		query = w.s.currentSQL(query)
	}
	var raw string
	var d wc.CloudDelegation
	err = tx.QueryRowContext(ctx, w.s.q(query), tenant, project, id).Scan(&d.ID, &out.tenant, &d.ProjectID, &d.OwnerID, &d.ConnectionID, &d.RunID, &d.NodeID, &d.Version, &d.State, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out.CloudDelegation) != nil || out.Validate() != nil || out.ID != d.ID || out.ProjectID != d.ProjectID || out.OwnerID != d.OwnerID || out.ConnectionID != d.ConnectionID || out.RunID != d.RunID || out.NodeID != d.NodeID || out.Version != d.Version || out.State != d.State {
		return out, cloudConflict()
	}
	return out, nil
}
func (w *WorkBuddyConnections) cloudSubmissionTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, current bool) (out cloudSubmissionRow, err error) {
	query := `SELECT id,tenant_id,project_id,delegation_id,run_id,node_id,version,state,assignment_sha256,worker_credential_hash,token_generation,prepared_nonce,payload FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND id=?`
	if current {
		query = w.s.currentSQL(query)
	}
	var raw string
	var d wc.CloudSubmission
	err = tx.QueryRowContext(ctx, w.s.q(query), tenant, project, id).Scan(&d.ID, &out.tenant, &d.ProjectID, &d.DelegationID, &d.RunID, &d.NodeID, &d.Version, &d.State, &out.assignmentHash, &out.workerHash, &out.tokenGeneration, &out.nonce, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out.CloudSubmission) != nil || out.Validate() != nil || out.ID != d.ID || out.ProjectID != d.ProjectID || out.DelegationID != d.DelegationID || out.RunID != d.RunID || out.NodeID != d.NodeID || out.Version != d.Version || out.State != d.State || digest(stringify(out.Assignment)) != out.assignmentHash {
		return out, cloudConflict()
	}
	return out, nil
}
func (w *WorkBuddyConnections) saveCloudDelegationTx(ctx context.Context, tx *sql.Tx, d cloudDelegationRow, before int) error {
	if d.Validate() != nil || d.Version != before+1 {
		return cloudInvalid()
	}
	if before == 0 {
		_, err := tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_delegations(`+cloudDelegationColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?)`), d.ID, d.tenant, d.ProjectID, d.OwnerID, d.ConnectionID, d.RunID, d.NodeID, d.Version, d.State, stringify(d.CloudDelegation))
		return err
	}
	result, err := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_delegations SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND owner_id=? AND version=?`), d.Version, d.State, stringify(d.CloudDelegation), d.tenant, d.ProjectID, d.ID, d.OwnerID, before)
	return cloudCAS(result, err)
}
func (w *WorkBuddyConnections) saveCloudSubmissionTx(ctx context.Context, tx *sql.Tx, d cloudSubmissionRow, before int) error {
	if d.Validate() != nil || d.Version != before+1 {
		return cloudInvalid()
	}
	if before == 0 {
		_, err := tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_submissions(`+cloudSubmissionColumns+`)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), d.ID, d.tenant, d.ProjectID, d.DelegationID, d.RunID, d.NodeID, d.Version, d.State, d.assignmentHash, d.workerHash, d.tokenGeneration, d.nonce, stringify(d.CloudSubmission))
		return err
	}
	result, err := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_submissions SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND prepared_nonce=? AND assignment_sha256=?`), d.Version, d.State, stringify(d.CloudSubmission), d.tenant, d.ProjectID, d.ID, before, d.nonce, d.assignmentHash)
	return cloudCAS(result, err)
}
func cloudCAS(r sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return cloudConflict()
	}
	return nil
}
