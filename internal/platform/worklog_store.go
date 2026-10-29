package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/ai-mada/suma/internal/store"
)

func (s *Service) InitializeWorklogs(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS suma_worklogs(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,status VARCHAR(16) NOT NULL,task_id VARCHAR(64) NOT NULL,member_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,work_date VARCHAR(10) NOT NULL,minutes INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,id))`,
		`CREATE TABLE IF NOT EXISTS suma_worklog_revisions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worklog_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,snapshot TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,worklog_id,revision))`,
		`CREATE TABLE IF NOT EXISTS suma_worklog_daily_totals(tenant_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,work_date VARCHAR(10) NOT NULL,active_minutes INTEGER NOT NULL,PRIMARY KEY(tenant_id,user_id,work_date))`,
		`CREATE TABLE IF NOT EXISTS suma_worklog_scopes(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,PRIMARY KEY(tenant_id,project_id))`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_worklog_page", "suma_worklogs", "tenant_id", "project_id", "work_date", "id")
}

const worklogSelect = `SELECT r.snapshot FROM suma_worklogs w JOIN suma_worklog_revisions r ON r.tenant_id=w.tenant_id AND r.project_id=w.project_id AND r.worklog_id=w.id AND r.revision=w.version`

func scanWorklog(row scanner) (out Worklog, err error) {
	var snapshot string
	if err = row.Scan(&snapshot); err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(snapshot), &out)
	return out, err
}

func (s *Service) worklogHeadTx(ctx context.Context, auth *authorizedCommand, project, id string) (out Worklog, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	out, err = scanWorklog(auth.tx.QueryRowContext(ctx, s.q(worklogSelect+` WHERE w.tenant_id=? AND w.project_id=? AND w.id=?`), auth.target.tenantID, project, id))
	if errors.Is(err, sql.ErrNoRows) {
		err = missing()
	}
	return out, err
}

func (s *Service) worklogScopeTx(ctx context.Context, auth *authorizedCommand) (version int, err error) {
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_worklog_scopes WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, auth.target.projectID).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

func (s *Service) worklogBumpScopeTx(ctx context.Context, auth *authorizedCommand) error {
	query := `INSERT INTO suma_worklog_scopes(tenant_id,project_id,version) VALUES(?,?,0) ON CONFLICT(tenant_id,project_id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		query = `INSERT IGNORE INTO suma_worklog_scopes(tenant_id,project_id,version) VALUES(?,?,0)`
	}
	if _, err := auth.tx.ExecContext(ctx, s.q(query), auth.target.tenantID, auth.target.projectID); err != nil {
		return err
	}
	_, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_worklog_scopes SET version=version+1 WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, auth.target.projectID)
	return err
}

// Daily reservations are keyed by account across all projects in the tenant.
// Deltas are combined and acquired in date order, with atomic range checks.
// A denied reservation exposes no amounts or other project identities.
func (s *Service) worklogLedgerTx(ctx context.Context, auth *authorizedCommand, old *Worklog, next Worklog) error {
	deltas := map[string]int{}
	if old != nil && old.Status == "active" {
		deltas[old.WorkDate] -= old.Minutes
	}
	if next.Status == "active" {
		deltas[next.WorkDate] += next.Minutes
	}
	dates := make([]string, 0, len(deltas))
	for day, delta := range deltas {
		if delta != 0 {
			dates = append(dates, day)
		}
	}
	sort.Strings(dates)
	for _, day := range dates {
		query := `INSERT INTO suma_worklog_daily_totals(tenant_id,user_id,work_date,active_minutes) VALUES(?,?,?,0) ON CONFLICT(tenant_id,user_id,work_date) DO NOTHING`
		if s.Dialect == "oceanbase" {
			query = `INSERT IGNORE INTO suma_worklog_daily_totals(tenant_id,user_id,work_date,active_minutes) VALUES(?,?,?,0)`
		}
		if _, err := auth.tx.ExecContext(ctx, s.q(query), auth.target.tenantID, next.UserID, day); err != nil {
			return err
		}
		result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_worklog_daily_totals SET active_minutes=active_minutes+? WHERE tenant_id=? AND user_id=? AND work_date=? AND active_minutes+? BETWEEN 0 AND 1440`), deltas[day], auth.target.tenantID, next.UserID, day, deltas[day])
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return conflict("Worklog daily total would exceed its limit")
		}
	}
	return nil
}

func (s *Service) worklogSaveTx(ctx context.Context, auth *authorizedCommand, old *Worklog, out Worklog) error {
	if err := auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return err
	}
	if err := s.worklogLedgerTx(ctx, auth, old, out); err != nil {
		return err
	}
	tenant := auth.target.tenantID
	if old == nil {
		_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_worklogs(tenant_id,project_id,id,version,status,task_id,member_id,user_id,work_date,minutes,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), tenant, out.ProjectID, out.ID, out.Version, out.Status, out.TaskID, out.MemberID, out.UserID, out.WorkDate, out.Minutes, out.CreatedAt, out.UpdatedAt)
		if err != nil {
			return err
		}
	} else {
		result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_worklogs SET version=?,status=?,task_id=?,work_date=?,minutes=?,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, out.Status, out.TaskID, out.WorkDate, out.Minutes, out.UpdatedAt, tenant, out.ProjectID, out.ID, old.Version)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return conflict("Worklog version changed")
		}
	}
	out.CanCorrect, out.CanVoid = false, false
	if _, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_worklog_revisions(tenant_id,project_id,worklog_id,revision,snapshot) VALUES(?,?,?,?,?)`), tenant, out.ProjectID, out.ID, out.Version, stringify(out)); err != nil {
		return err
	}
	return s.worklogBumpScopeTx(ctx, auth)
}
