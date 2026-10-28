package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

const (
	dispatchRecommendationInvalid        = "Invalid dispatch recommendation request"
	dispatchRecommendationProfileMessage = "Select a current enabled read-only Suma dispatch recommendation profile"
	dispatchRecommendationSourceChanged  = "The dispatch recommendation source changed; review current candidates again"
	dispatchRecommendationEmpty          = "No eligible execution candidate satisfies the current constraints"
	dispatchRecommendationConsent        = "Confirm that the current task and candidate context will be sent to the selected model"
	dispatchRecommendationOutput         = "The model recommendation does not satisfy the fixed candidate contract"
	dispatchRecommendationVersion        = "The dispatch recommendation changed; reload before deciding"
	dispatchRecommendationUnknown        = "The dispatch recommendation outcome is unknown; inspect the original run"
	dispatchRecommendationFailed         = "The dispatch recommendation did not complete successfully"
	dispatchRecommendationAuthority      = "The dispatch recommendation creator no longer has current execution authority"
	dispatchRecommendationPrompt         = "The recommendation context exceeds 64000 UTF-8 bytes; select a smaller context"
)

type dispatchRecommendationRecord struct {
	Recommendation        DispatchRecommendation               `json:"recommendation"`
	Request               DispatchRecommendationOptionsRequest `json:"request"`
	CreatorCredentialHash string                               `json:"creator_credential_hash"`
	WorkerID              string                               `json:"worker_id"`
}

// This is a new current domain, not a historical recommendation importer.
func (s *Service) InitializeDispatchRecommendations(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_dispatch_recommendations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_dispatch_recommendation_outcomes(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,recommendation_id VARCHAR(64) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,result_sha256 VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,result TEXT NOT NULL,PRIMARY KEY(recommendation_id,attempt,fence,result_sha256))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_dispatch_recommendation_page", "suma_dispatch_recommendations", "tenant_id", "project_id", "task_id", "created_at", "id")
}

func (s *Service) dispatchRecommendationTx(ctx context.Context, tx governanceQuery, tenant, project, id string) (out dispatchRecommendationRecord, err error) {
	var raw, state, run, task, expires string
	var version int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,state,run_id,task_id,expires_at,payload FROM suma_dispatch_recommendations WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&version, &state, &run, &task, &expires, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	a := out.Recommendation
	if a.ID != id || a.ProjectID != project || a.Version != version || a.State != state || a.RunID != run || a.Source.Context.Task.ID != task || a.ExpiresAt != expires {
		return out, conflict(dispatchRecommendationVersion)
	}
	return out, nil
}

func (s *Service) saveDispatchRecommendationTx(ctx context.Context, tx *sql.Tx, tenant string, record dispatchRecommendationRecord, previous int) error {
	a := record.Recommendation
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_dispatch_recommendations SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), a.Version, a.State, stringify(record), tenant, a.ProjectID, a.ID, previous)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return conflict(dispatchRecommendationVersion)
	}
	return err
}

func (s *Service) dispatchRecommendationForRunTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (out dispatchRecommendationRecord, found bool, err error) {
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_dispatch_recommendations WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out, err = s.dispatchRecommendationTx(ctx, tx, tenant, project, id)
	return out, true, err
}
