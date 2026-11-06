package platform

import (
	"context"
	"database/sql"
	"errors"
)

const automationManualInvocationsDDL = `CREATE TABLE IF NOT EXISTS suma_automation_manual_invocations(
	tenant_id VARCHAR(64) NOT NULL,
	project_id VARCHAR(64) NOT NULL,
	occurrence_id VARCHAR(64) NOT NULL,
	rule_id VARCHAR(64) NOT NULL,
	invoker_user_id VARCHAR(64) NOT NULL,
	PRIMARY KEY(tenant_id,project_id,occurrence_id)
)`

func (s *Service) manualAutomationInvoker(ctx context.Context, q authorityQuery, ref automationRef, occurrence string) (string, error) {
	var invoker string
	err := q.QueryRowContext(ctx, s.q(`SELECT invoker_user_id FROM suma_automation_manual_invocations WHERE tenant_id=? AND project_id=? AND rule_id=? AND occurrence_id=?`), ref.tenant, ref.project, ref.id, occurrence).Scan(&invoker)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return invoker, err
}
