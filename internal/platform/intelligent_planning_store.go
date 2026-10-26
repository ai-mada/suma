package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
)

const (
	planningInvalidRequest     = "Invalid intelligent planning request"
	planningInvalidOutput      = "The model planning output does not satisfy the fixed proposal contract"
	planningSourceChanged      = "Planning source has changed; create a new analysis"
	planningProfileInvalid     = "Select a current enabled read-only Suma planning profile"
	planningPromptLimit        = "Planning context exceeds 32000 UTF-8 bytes; select a smaller task scope"
	planningVersionConflict    = "The planning analysis has changed; reload before deciding"
	planningConfirmation       = "Confirm that the selected task content will be sent to the configured model"
	planningApplyConfirmation  = "Confirm the proposed schedule changes before applying them"
	planningExecutionFailed    = "The planning execution did not complete successfully"
	planningExecutionUnknown   = "The planning execution outcome is unknown; inspect the original run"
	planningSourceUnauthorized = "The planning creator no longer has current execution authority"
	planningAlertInvalid       = "Invalid planning alert decision or version"
)

type planningRecord struct {
	Analysis              PlanningAnalysis          `json:"analysis"`
	CreatorCredentialHash string                    `json:"creator_credential_hash"`
	OriginalOutput        string                    `json:"original_output"`
	WorkerID              string                    `json:"worker_id"`
	Route                 dag.ProjectExecutionRoute `json:"route"`
}

func (s *Service) MigrateIntelligentPlanning(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_planning_analyses(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_planning_outcomes(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,analysis_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,result_sha256 VARCHAR(64) NOT NULL,PRIMARY KEY(analysis_id,attempt,fence))`,
		`CREATE TABLE IF NOT EXISTS suma_planning_alerts(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,analysis_id VARCHAR(64) NOT NULL,risk_key VARCHAR(64) NOT NULL,severity VARCHAR(20) NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(analysis_id,risk_key))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	for _, index := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_planning_analysis_page", "suma_planning_analyses", []string{"tenant_id", "project_id", "created_at", "id"}},
		{"suma_planning_alert_page", "suma_planning_alerts", []string{"tenant_id", "project_id", "state", "created_at", "id"}},
		{"suma_planning_alert_severity", "suma_planning_alerts", []string{"tenant_id", "project_id", "state", "severity"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, index.table, index.columns...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) planningRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out planningRecord, err error) {
	var raw, state string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,state,payload FROM suma_planning_analyses WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&version, &state, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.Analysis.ID != id || out.Analysis.ProjectID != project || out.Analysis.Version != version || out.Analysis.State != state {
		return out, conflict(planningVersionConflict)
	}
	return out, nil
}

func (s *Service) savePlanningRecordTx(ctx context.Context, tx *sql.Tx, tenant, project string, record planningRecord, old int) error {
	a := record.Analysis
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_planning_analyses SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(record), tenant, project, a.ID, old)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return conflict(planningVersionConflict)
	}
	return nil
}

func (s *Service) planningAlertTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out PlanningAlert, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_planning_alerts WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	return
}
