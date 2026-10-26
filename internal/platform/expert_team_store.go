package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/ai-mada/suma/internal/store"
)

const (
	expertInvalid     = "Invalid expert team request"
	expertChanged     = "Expert team configuration changed; refresh the preview"
	expertRetired     = "Expert team configuration is retired"
	expertInactive    = "Expert team is not active"
	expertResource    = "Expert team resource or membership changed"
	expertBinding     = "Expert team workflow, tool or knowledge is no longer eligible"
	expertUnpublished = "Expert team configuration must be published"
	expertExpired     = "Expert team dispatch preview expired"
	expertPlanChanged = "Expert team dispatch plan changed"
	expertOwned       = "This task dispatch plan belongs to an expert team batch"
	expertAck         = "Explicit expert team dispatch acknowledgement required"
	expertTask        = "Expert team task is active or its dependencies are not accepted"
)

var expertRoleKey = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,39}$`)

func expertError(code, message string) error {
	status := 409
	if code == "expert_team_invalid" || code == "expert_team_ack_required" {
		status = 400
	}
	return &APIError{status, code, message, nil}
}

func (s *Service) MigrateExpertTeams(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_expert_team_configurations(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,team_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,team_id))`,
		`CREATE TABLE IF NOT EXISTS suma_expert_team_revisions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,team_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,team_id,version))`,
		`CREATE TABLE IF NOT EXISTS suma_expert_team_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,team_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_expert_team_plan_items(task_plan_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,expert_plan_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL UNIQUE)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_expert_team_plan_scope", "suma_expert_team_plans", "tenant_id", "project_id", "id"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_expert_team_item_scope", "suma_expert_team_plan_items", "tenant_id", "project_id", "expert_plan_id")
}

func (s *Service) expertTeamHeadTx(ctx context.Context, auth *authorizedCommand, team string, revision int) (out ExpertTeamConfiguration, err error) {
	var raw string
	query := `SELECT payload FROM suma_expert_team_configurations WHERE tenant_id=? AND project_id=? AND team_id=?`
	args := []any{auth.target.tenantID, auth.target.projectID, team}
	if revision > 0 {
		query = `SELECT payload FROM suma_expert_team_revisions WHERE tenant_id=? AND project_id=? AND team_id=? AND version=?`
		args = append(args, revision)
	}
	err = auth.tx.QueryRowContext(ctx, s.q(query), args...).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) && revision == 0 {
		out.ProjectID, out.TeamID, out.Roles, out.Blockers = auth.target.projectID, team, []ExpertTeamRole{}, []string{}
		var state string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT name,version,status FROM suma_teams WHERE tenant_id=? AND id=?`), auth.target.tenantID, team).Scan(&out.TeamName, &out.TeamVersion, &state)
		if errors.Is(err, sql.ErrNoRows) {
			return out, missing()
		}
		return out, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	return out, err
}

func (s *Service) saveExpertTeamTx(ctx context.Context, auth *authorizedCommand, old ExpertTeamConfiguration, out ExpertTeamConfiguration) error {
	args := []any{out.Version, out.State, stringify(out), auth.target.tenantID, out.ProjectID, out.TeamID, old.Version}
	if old.Version == 0 {
		if _, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_configurations(tenant_id,project_id,team_id,version,state,payload)VALUES(?,?,?,?,?,?)`), auth.target.tenantID, out.ProjectID, out.TeamID, out.Version, out.State, stringify(out)); err != nil {
			return err
		}
	} else {
		res, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_configurations SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND team_id=? AND version=?`), args...)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return expertError("expert_team_changed", expertChanged)
		}
	}
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_revisions(tenant_id,project_id,team_id,version,payload)VALUES(?,?,?,?,?)`), auth.target.tenantID, out.ProjectID, out.TeamID, out.Version, stringify(out))
	return err
}

func (s *Service) expertTeamPlanTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out ExpertTeamPlan, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_expert_team_plans WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out)
	return out, err
}

func (s *Service) rejectExpertTeamOwnedPlanTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) error {
	var count int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_expert_team_plan_items WHERE tenant_id=? AND project_id=? AND task_plan_id=?`), tenant, project, id).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return expertError("expert_team_batch_owned", expertOwned)
	}
	return nil
}
