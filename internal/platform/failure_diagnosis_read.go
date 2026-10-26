package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) failureDiagnosisProjectionTx(ctx context.Context, auth *authorizedCommand, out FailureDiagnosisAnalysis) (FailureDiagnosisAnalysis, error) {
	if err := assertProjectRead(auth, out.ProjectID); err != nil {
		return out, err
	}
	out.AllowedActions = []string{}
	out.BasisCurrent = true
	if e := s.checkFailureDiagnosisSourceTx(ctx, auth.tx, auth.target.tenantID, out.ProjectID, out.Source); e != nil {
		var api *APIError
		if !errors.As(e, &api) {
			return out, e
		}
		out.BasisCurrent = false
	}
	existing, found, err := s.lookupIssueRequirementTx(ctx, auth, out.ProjectID, out.Source.IssueID)
	if err != nil {
		return out, err
	}
	if found {
		out.AdoptedRequirementID = existing.ID
	}
	var version int
	var state string
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM suma_failure_diagnoses WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, out.ProjectID, out.ID).Scan(&version, &state); err != nil {
		return out, err
	}
	if version != out.Version || state != out.State || !auth.actor.IsBrowser() {
		return out, nil
	}
	_, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, out.ProjectID, true, false)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return out, nil
		}
		return out, err
	}
	if out.State == "ready" {
		out.AllowedActions = []string{"revise", "dismiss"}
		if out.BasisCurrent && !found {
			out.AllowedActions = append(out.AllowedActions, "adopt")
		}
	} else if out.State == "failed" {
		out.AllowedActions = []string{"dismiss"}
	}
	return out, nil
}
func (s *Service) GetFailureDiagnosis(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out FailureDiagnosisAnalysis, err error) {
	if engine == nil || !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(diagnosisInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.failureDiagnosisTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.failureDiagnosisProjectionTx(ctx, auth, r.Analysis)
}
func (s *Service) ListFailureDiagnoses(ctx context.Context, engine *dag.Engine, actor Actor, project, run, node string, attempt, limit int, before string) (out FailureDiagnosisPage, err error) {
	out.Items = []FailureDiagnosisAnalysis{}
	if limit == 0 {
		limit = 20
	}
	if engine == nil || !handoffID(project, false) || !diagnosisID(run, 100) || !diagnosisID(node, 150) || attempt < 1 || limit < 1 || limit > 100 {
		return out, invalid(diagnosisInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	collection := "failure-diagnoses:" + run + ":" + node + ":" + strconv.Itoa(attempt)
	last, err := automationDecodeCursor(auth.currentPrincipal, project, collection, before)
	if err != nil {
		return out, err
	}
	where := ` WHERE tenant_id=? AND project_id=? AND source_run_id=? AND source_node_id=? AND source_attempt=?`
	args := []any{auth.target.tenantID, project, run, node, attempt}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_failure_diagnoses`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if last != "" {
		var at string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM suma_failure_diagnoses`+where+` AND id=?`), append(args, last)...).Scan(&at)
		if errors.Is(err, sql.ErrNoRows) {
			return out, invalid(diagnosisInvalid)
		}
		if err != nil {
			return out, err
		}
		where += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, at, at, last)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT payload FROM suma_failure_diagnoses`+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), append(args, limit+1)...)
	if err != nil {
		return out, err
	}
	items := []failureDiagnosisRecord{}
	for rows.Next() {
		var raw string
		var r failureDiagnosisRecord
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &r); err != nil {
			break
		}
		items = append(items, r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(items) > limit {
		items = items[:limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, collection, items[len(items)-1].Analysis.ID)
	}
	for _, r := range items {
		p, e := s.failureDiagnosisProjectionTx(ctx, auth, r.Analysis)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, p)
	}
	return out, nil
}
