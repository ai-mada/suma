package platform

import (
	"context"
	"fmt"
	"slices"

	"github.com/ai-mada/suma/internal/store"
)

const organizationWorkCalendarTable = "suma_organization_work_calendars"
const organizationWorkCalendarDDL = `CREATE TABLE IF NOT EXISTS suma_organization_work_calendars(tenant_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,schedule TEXT NOT NULL,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,user_id))`

func (s *Service) validateOrganizationWorkCalendarInitialSchema(ctx context.Context) error {
	var query string
	switch s.Dialect {
	case "sqlite":
		query = `SELECT type FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		query = `SELECT table_type FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT table_type FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: unsupported organization work calendar schema dialect", store.ErrInitialSchema)
	}
	var kind string
	var exists int
	rows, err := s.DB.QueryContext(ctx, s.q(query), organizationWorkCalendarTable)
	if err != nil {
		return err
	}
	for rows.Next() {
		if err = rows.Scan(&kind); err != nil {
			rows.Close()
			return err
		}
		exists++
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	if exists != 1 || kind != "table" && kind != "BASE TABLE" {
		return fmt.Errorf("%w: organization work calendar table required", store.ErrInitialSchema)
	}
	var baseline string
	if err = s.DB.QueryRowContext(ctx, `SELECT version FROM suma_schema_baselines WHERE component='platform'`).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered organization work calendar schema required", store.ErrInitialSchema)
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT * FROM suma_organization_work_calendars WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: organization work calendar columns required", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	slices.Sort(columns)
	if !slices.Equal(columns, []string{"schedule", "tenant_id", "updated_at", "user_id", "version"}) {
		return fmt.Errorf("%w: exact organization work calendar columns required", store.ErrInitialSchema)
	}
	primary, _, err := s.notificationInboxSchemaKeys(ctx, organizationWorkCalendarTable)
	if err != nil {
		return err
	}
	if primary != "tenant_id,user_id" {
		return fmt.Errorf("%w: organization work calendar primary key required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) InitializeOrganizationWorkCalendars(ctx context.Context) error {
	if err := s.validateOrganizationWorkCalendarInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.ddl(organizationWorkCalendarDDL))
	return err
}
