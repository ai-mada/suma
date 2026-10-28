package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

const (
	projectRiskInvalid      = "Invalid project risk request"
	projectRiskVersion      = "The risk or its review basis has changed; reload before deciding"
	projectRiskState        = "The current risk state does not allow this action"
	projectRiskOwner        = "The risk owner must be a current authorized human project resource"
	projectRiskBasis        = "Complete the risk assessment and current linked evidence before review"
	projectRiskMitigation   = "Mitigated risks require current formally accepted task evidence"
	projectRiskSource       = "The selected planning suggestion is unavailable or has changed"
	projectRiskHistoryLimit = 10
)

// InitializeProjectRisks is part of the current platform initialization. It
// declares only current domain tables, without importing or backfilling alerts.
func (s *Service) InitializeProjectRisks(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_risks(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,cycle INTEGER NOT NULL,assessment_revision INTEGER NOT NULL,owner_id VARCHAR(64) NOT NULL,review_date VARCHAR(10) NOT NULL,probability INTEGER NULL,impact INTEGER NULL,score INTEGER NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_risk_sources(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,alert_id VARCHAR(64) NOT NULL,risk_id VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,alert_id),UNIQUE(tenant_id,project_id,risk_id))`,
		`CREATE TABLE IF NOT EXISTS suma_risk_reviews(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,risk_id VARCHAR(64) NOT NULL,cycle INTEGER NOT NULL,decision VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,snapshot TEXT NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_risk_page", "suma_risks", "tenant_id", "project_id", "state", "created_at", "id"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_risk_review_page", "suma_risk_reviews", "tenant_id", "project_id", "risk_id", "created_at", "id")
}

func (s *Service) projectRiskTx(ctx context.Context, auth *authorizedCommand, project, id string) (out ProjectRisk, err error) {
	var raw, state, owner string
	var version, cycle, assessment int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state,cycle,assessment_revision,owner_id,payload FROM suma_risks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, id).Scan(&version, &state, &cycle, &assessment, &owner, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.ID != id || out.TenantID != auth.target.tenantID || out.ProjectID != project || out.Version != version || out.State != state || out.Cycle != cycle || out.AssessmentRevision != assessment || out.OwnerID != owner {
		return out, conflict(projectRiskVersion)
	}
	return out, nil
}

func projectRiskScore(p, i *int) *int {
	if p == nil || i == nil {
		return nil
	}
	score := *p * *i
	return &score
}

func (s *Service) saveProjectRiskTx(ctx context.Context, auth *authorizedCommand, out ProjectRisk, previous int) error {
	if err := assertProjectCommand(auth.tx, auth, out.ProjectID); err != nil {
		return err
	}
	if previous == 0 {
		_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_risks(id,tenant_id,project_id,version,state,cycle,assessment_revision,owner_id,review_date,probability,impact,score,created_at,updated_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), out.ID, out.TenantID, out.ProjectID, out.Version, out.State, out.Cycle, out.AssessmentRevision, out.OwnerID, out.ReviewDate, out.Probability, out.Impact, out.Score, out.CreatedAt, out.UpdatedAt, stringify(out))
		return err
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_risks SET version=?,state=?,cycle=?,assessment_revision=?,owner_id=?,review_date=?,probability=?,impact=?,score=?,updated_at=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, out.State, out.Cycle, out.AssessmentRevision, out.OwnerID, out.ReviewDate, out.Probability, out.Impact, out.Score, out.UpdatedAt, stringify(out), out.TenantID, out.ProjectID, out.ID, previous)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return conflict(projectRiskVersion)
	}
	return err
}

func (s *Service) projectRiskReviewTx(ctx context.Context, auth *authorizedCommand, risk ProjectRisk, id string) (out ProjectRiskReview, err error) {
	var raw, decision string
	var cycle int
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT cycle,decision,snapshot FROM suma_risk_reviews WHERE tenant_id=? AND project_id=? AND risk_id=? AND id=?`), risk.TenantID, risk.ProjectID, risk.ID, id).Scan(&cycle, &decision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.ID != id || out.Cycle != cycle || out.Decision != decision {
		return out, conflict(projectRiskVersion)
	}
	return out, nil
}

func (s *Service) saveProjectRiskReviewTx(ctx context.Context, auth *authorizedCommand, risk ProjectRisk, review ProjectRiskReview, insert bool) error {
	if insert {
		_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_risk_reviews(id,tenant_id,project_id,risk_id,cycle,decision,created_at,snapshot)VALUES(?,?,?,?,?,?,?,?)`), review.ID, risk.TenantID, risk.ProjectID, risk.ID, review.Cycle, review.Decision, review.SubmittedAt, stringify(review))
		return err
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_risk_reviews SET decision=?,snapshot=? WHERE tenant_id=? AND project_id=? AND risk_id=? AND id=? AND decision='pending'`), review.Decision, stringify(review), risk.TenantID, risk.ProjectID, risk.ID, review.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return conflict(projectRiskVersion)
	}
	return err
}
