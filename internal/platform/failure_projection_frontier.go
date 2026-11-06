package platform

import (
	"context"
	"fmt"
)

const failureProjectionFrontierDDL = `CREATE TABLE IF NOT EXISTS suma_failure_projection_frontier(id INTEGER PRIMARY KEY,sequence BIGINT NOT NULL)`

func (s *Service) initializeFailureProjectionFrontier(ctx context.Context) error {
	query := `INSERT INTO suma_failure_projection_frontier(id,sequence) VALUES(1,0) ON CONFLICT(id) DO NOTHING`
	if s.Dialect == "oceanbase" {
		query = `INSERT INTO suma_failure_projection_frontier(id,sequence) VALUES(1,0) ON DUPLICATE KEY UPDATE id=id`
	}
	if _, err := s.DB.ExecContext(ctx, query); err != nil {
		return err
	}
	_, err := s.failureProjectionFrontier(ctx)
	return err
}

func (s *Service) failureProjectionFrontier(ctx context.Context) (int64, error) {
	var sequence int64
	if err := s.DB.QueryRowContext(ctx, `SELECT sequence FROM suma_failure_projection_frontier WHERE id=1`).Scan(&sequence); err != nil {
		return 0, err
	}
	if sequence < 0 {
		return 0, fmt.Errorf("invalid failure projection frontier")
	}
	return sequence, nil
}

// Advance only after every fresh failure through this sequence has a durable
// acknowledgement or retry. The DAG commits event sequence ranges in order
// under its global sequence row, so a later commit cannot insert below it.
func (s *Service) advanceFailureProjectionFrontier(ctx context.Context, through int64) error {
	if through < 0 {
		return fmt.Errorf("invalid failure projection frontier")
	}
	result, err := s.DB.ExecContext(ctx, s.q(`UPDATE suma_failure_projection_frontier SET sequence=? WHERE id=1 AND sequence>=0 AND sequence<?`), through, through)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 1 {
		return err
	}
	current, err := s.failureProjectionFrontier(ctx)
	if err != nil {
		return err
	}
	if current < through {
		return fmt.Errorf("failure projection frontier was not advanced")
	}
	return nil
}
