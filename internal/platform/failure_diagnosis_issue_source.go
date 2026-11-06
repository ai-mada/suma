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
	out.RunID, err = s.failureDiagnosisIssueRunTx(ctx, auth)
	if err != nil || out.RunID == "" || !auth.actor.IsBrowser() {
		return out, err
	}
	out.AutomationOrigin, err = s.verifiedIssueAutomationOriginTx(ctx, auth, out.RunID)
	return out, err
}

// A caller checks the current issue in this authority transaction before
// resolving its original run. Editable issue fields do not establish lineage.
func (s *Service) failureDiagnosisIssueRunTx(ctx context.Context, auth *authorizedCommand) (string, error) {
	if auth == nil {
		return "", forbidden()
	}
	tenant, project, issue := auth.target.tenantID, auth.target.projectID, auth.target.resourceID
	if err := assertProjectRead(auth, project); err != nil {
		return "", err
	}
	query := `SELECT r.id,o.operation_id FROM suma_operations o
		JOIN dag_runs r ON r.tenant_id=o.tenant_id AND r.project_id=o.project_id AND r.id=SUBSTR(o.operation_id,LENGTH('failure-')+1)
		WHERE o.tenant_id=? AND o.project_id=? AND o.operation_id LIKE 'failure-%' AND (` + s.failureIssueReceiptIDSQL() + `)=?
		ORDER BY r.id LIMIT 2`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), tenant, project, issue)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var run, operation string
	if err = rows.Scan(&run, &operation); err != nil {
		return "", err
	}
	if rows.Next() {
		return "", conflict(diagnosisIssue)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	if err = rows.Close(); err != nil {
		return "", err
	}
	if operation != failureIssueKey(run) {
		return "", conflict(diagnosisIssue)
	}
	id, found, err := s.failureIssueIDTx(ctx, auth.tx, tenant, project, run)
	if err != nil {
		return "", err
	}
	if !found || id != issue {
		return "", conflict(diagnosisIssue)
	}
	return run, nil
}

// An unverified automation binding does not invalidate a canonical failed run.
// Database and transaction failures still fail the enclosing operation.
func (s *Service) verifiedIssueAutomationOriginTx(ctx context.Context, auth *authorizedCommand, run string) (*AutomationRunOrigin, error) {
	origin, err := s.automationRunOriginCurrent(ctx, auth.tx, auth.target.tenantID, auth.target.projectID, run)
	if err != nil {
		var invalidOrigin invalidAutomationRunOrigin
		var apiError *APIError
		if errors.As(err, &invalidOrigin) || errors.As(err, &apiError) && apiError.Status == http.StatusNotFound {
			return nil, nil
		}
	}
	return origin, err
}
