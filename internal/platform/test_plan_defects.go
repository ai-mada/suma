package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// The protected run/case slot is authoritative. Editable issue metadata and a
// caller-selected operation key cannot create or replace that relationship.
func (s *Service) CreateTestPlanDefect(ctx context.Context, actor Actor, project, id, run string, request TestPlanDefectRequest) (out Record, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if request.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || !testPlanRunID(run) || !designKey(request.CaseKey) || !designVersion(request.ExpectedVersion) || request.ExpectedVersion < 1 || !testPlanText(request.Reason, 2000, true) || !planningKey(request.IdempotencyKey) {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"test_plan_defect": id, "run": run, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if current.Version != request.ExpectedVersion {
		return out, conflict(testPlanVersion)
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, auth.target.tenantID, project, run)
	if err != nil {
		return out, err
	}
	if !found || fixed.PlanID != id || fixed.TaskID != current.TaskID {
		return out, missing()
	}
	execution, err := s.testPlanExecutionTx(ctx, auth.tx, auth.target.tenantID, project, current, fixed, run)
	if err != nil {
		return out, err
	}
	if !execution.BasisCurrent {
		return out, conflict(testPlanSource)
	}
	var selected *TestCaseResult
	for i := range execution.Cases {
		if execution.Cases[i].CaseKey == request.CaseKey {
			selected = &execution.Cases[i]
			break
		}
	}
	if selected == nil || selected.Status != "failed" && selected.Status != "error" || len(selected.Evidence) == 0 {
		return out, invalid(testPlanDefect)
	}
	var issueID string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT issue_id FROM suma_test_plan_defects WHERE tenant_id=? AND project_id=? AND plan_id=? AND run_id=? AND case_key=?`), auth.target.tenantID, project, id, run, request.CaseKey).Scan(&issueID)
	if err == nil {
		out, err = s.getRecordTx(ctx, auth, project, "issues", issueID)
		if err != nil {
			return out, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	} else {
		priority := "high"
		for _, definition := range fixed.Revision.Cases {
			if definition.Key == request.CaseKey {
				priority = definition.Priority
				break
			}
		}
		// Use the existing record validation/storage helpers without a second
		// generic operation slot. Only this domain writes the protected relation.
		data := map[string]any{"origin": "test_plan_case", "plan_id": id, "plan_revision": fixed.Revision.Revision, "plan_sha256": fixed.Revision.SHA256, "task_id": current.TaskID, "run_id": run, "case_key": request.CaseKey, "report_ids": []string{}}
		ids := []string{}
		seen := map[string]bool{}
		for _, e := range selected.Evidence {
			if !seen[e.ReportID] {
				seen[e.ReportID] = true
				ids = append(ids, e.ReportID)
			}
		}
		data["report_ids"] = ids
		out = recordFrom(auth.currentPrincipal, project, map[string]any{"title": selected.Title, "description": request.Reason, "status": "open", "priority": priority, "data": data})
		if err = s.validateRecord(ctx, auth.tx, auth, out, "issues"); err != nil {
			return out, err
		}
		if err = s.insertCommandRecord(ctx, auth.tx, auth, out, "issues"); err != nil {
			return out, err
		}
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plan_defects(tenant_id,project_id,plan_id,run_id,case_key,issue_id,created_at)VALUES(?,?,?,?,?,?,?)`), auth.target.tenantID, project, id, run, request.CaseKey, out.ID, now()); err != nil {
			return out, err
		}
		if err = s.commandActivity(ctx, auth, "test_plan.defect_created", out.ID, request.Reason); err != nil {
			return out, err
		}
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return Record{}, err
	}
	return out, auth.tx.Commit()
}
