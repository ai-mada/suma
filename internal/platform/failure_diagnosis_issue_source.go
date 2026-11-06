package platform

import (
	"context"
	"errors"
	"net/http"
)

// FailureDiagnosisIssueSource locates the original run of a projected issue.
// The run is absent for an ordinary issue. The locator does not select an
// attempt or grant permission to diagnose or execute anything.
type FailureDiagnosisIssueSource struct {
	ProjectID        string               `json:"project_id"`
	IssueID          string               `json:"issue_id"`
	RunID            string               `json:"run_id,omitempty"`
	AutomationOrigin *AutomationRunOrigin `json:"automation_origin,omitempty" wire:"nonnull"`
}

// Select only string IDs from the immutable operation response. The Go
// provenance reader still verifies the full receipt and durable event.
func (s *Service) failureIssueReceiptIDSQL() string {
	switch s.Dialect {
	case "postgres":
		return `CASE WHEN jsonb_typeof(CAST(o.response AS JSONB)->'id')='string' THEN CAST(o.response AS JSONB)->>'id' END`
	case "oceanbase":
		return `CASE WHEN JSON_VALID(o.response) THEN CASE WHEN JSON_TYPE(JSON_EXTRACT(o.response,'$.id'))='STRING' THEN JSON_UNQUOTE(JSON_EXTRACT(o.response,'$.id')) END END`
	default:
		return `CASE WHEN json_valid(o.response) THEN CASE WHEN json_type(o.response,'$.id')='text' THEN json_extract(o.response,'$.id') END END`
	}
}

func (s *Service) GetFailureDiagnosisIssueSource(ctx context.Context, actor Actor, project, issue string) (out FailureDiagnosisIssueSource, err error) {
	if !handoffID(project, false) || !handoffID(issue, false) {
		return out, invalid(diagnosisInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, issue, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	record, err := s.getRecordTx(ctx, auth, project, "issues", issue)
	if err != nil {
		return out, err
	}
	if record.ID != issue || record.TenantID != auth.target.tenantID || record.ProjectID != project {
		return out, missing()
	}
	out = FailureDiagnosisIssueSource{ProjectID: project, IssueID: issue}
	query := `SELECT r.id,o.operation_id FROM suma_operations o
		JOIN dag_runs r ON r.tenant_id=o.tenant_id AND r.project_id=o.project_id AND r.id=SUBSTR(o.operation_id,LENGTH('failure-')+1)
		WHERE o.tenant_id=? AND o.project_id=? AND o.operation_id LIKE 'failure-%' AND (` + s.failureIssueReceiptIDSQL() + `)=?
		ORDER BY r.id LIMIT 2`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), auth.target.tenantID, project, issue)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	if !rows.Next() {
		return out, rows.Err()
	}
	var run, operation string
	if err = rows.Scan(&run, &operation); err != nil {
		return out, err
	}
	if rows.Next() || operation != failureIssueKey(run) {
		return out, conflict(diagnosisIssue)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if err = rows.Close(); err != nil {
		return out, err
	}
	id, found, err := s.failureIssueIDTx(ctx, auth.tx, auth.target.tenantID, project, run)
	if err != nil {
		return out, err
	}
	if !found || id != issue {
		return out, conflict(diagnosisIssue)
	}
	out.RunID = run
	if !auth.actor.IsBrowser() {
		return out, nil
	}
	// The issue's canonical run remains useful even if its optional automation
	// binding cannot be verified. Database and transaction failures still fail
	// the read, and project API credentials do not read this browser provenance.
	origin, err := s.automationRunOriginCurrent(ctx, auth.tx, auth.target.tenantID, project, run)
	if err != nil {
		var invalidOrigin invalidAutomationRunOrigin
		var apiError *APIError
		if errors.As(err, &invalidOrigin) || errors.As(err, &apiError) && apiError.Status == http.StatusNotFound {
			return out, nil
		}
		return out, err
	}
	out.AutomationOrigin = origin
	return out, nil
}
