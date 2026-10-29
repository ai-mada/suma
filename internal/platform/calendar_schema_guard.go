package platform

import (
	"context"
	"fmt"

	"github.com/ai-mada/suma/internal/store"
)

// A new independent calendar can be installed in a declared current database.
// An existing malformed calendar must fail before initialization writes; no old
// weekly capacity is interpreted, imported or deleted as part of startup.
func (s *Service) validateResourceCalendarInitialSchema(ctx context.Context) error {
	var query string
	switch s.Dialect {
	case "sqlite":
		query = `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=? AND table_type='BASE TABLE'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=? AND table_type='BASE TABLE'`
	default:
		return fmt.Errorf("unsupported initial schema dialect %q", s.Dialect)
	}
	var present int
	if err := s.DB.QueryRowContext(ctx, s.q(query), "suma_resource_calendars").Scan(&present); err != nil {
		return err
	}
	if present == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT tenant_id,project_id,member_id,version,schedule,updated_at FROM suma_resource_calendars WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: current resource calendar schema is required", store.ErrInitialSchema)
	}
	return rows.Close()
}
