package platform

import (
	"context"
	"fmt"
)

// InitializeProjections installs delivery ledgers at service startup. Projection
// loops perform DML only and do not need schema creation permissions at runtime.
func (s *Service) InitializeProjections(ctx context.Context) error {
	if err := s.validateKnowledgeProjectionInitialSchema(ctx); err != nil {
		return err
	}
	for _, query := range []string{
		indexFailuresDDL,
		`CREATE TABLE IF NOT EXISTS suma_failure_projection(sequence BIGINT PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,projected_at VARCHAR(40) NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(query)); err != nil {
			return fmt.Errorf("projection migration: %w", err)
		}
	}
	return s.initializeKnowledgeProjectionIndexes(ctx)
}
