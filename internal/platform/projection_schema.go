package platform

import (
	"context"
	"fmt"
)

// MigrateProjections installs delivery ledgers at service startup. Projection
// loops perform DML only and do not need schema creation permissions at runtime.
func (s *Service) MigrateProjections(ctx context.Context) error {
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS suma_index_failures(event_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,source_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,http_status INTEGER NOT NULL,reason VARCHAR(200) NOT NULL,failed_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_failure_projection(sequence BIGINT PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,projected_at VARCHAR(40) NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(query)); err != nil {
			return fmt.Errorf("projection migration: %w", err)
		}
	}
	return nil
}
