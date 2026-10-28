package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/ai-mada/suma/internal/dag"
)

type testPlanNodeFact struct {
	ID, State, Worker string
	Attempt           int
	Fence             int64
	Spec              dag.Node
	Workspace         map[string]any
}

func (s *Service) testPlanNodesTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (map[string]testPlanNodeFact, error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT n.id,n.state,n.attempt,n.fence,n.worker_id,n.spec,COALESCE(a.result,'') FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id LEFT JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? ORDER BY n.id`), tenant, project, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]testPlanNodeFact{}
	for rows.Next() {
		var f testPlanNodeFact
		var spec, workspace string
		if err = rows.Scan(&f.ID, &f.State, &f.Attempt, &f.Fence, &f.Worker, &spec, &workspace); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(spec), &f.Spec); err != nil {
			return nil, err
		}
		if workspace != "" {
			// Workspace proof belongs to the exact persisted attempt result,
			// not a mutable node column. Decode only the required proof field.
			var result struct {
				WorkspaceEvidence map[string]any `json:"workspace_evidence"`
			}
			if err = json.Unmarshal([]byte(workspace), &result); err != nil {
				return nil, err
			}
			f.Workspace = result.WorkspaceEvidence
		}
		out[f.ID] = f
	}
	return out, rows.Err()
}
func testPlanEvidenceSeverity(status string) int {
	switch status {
	case "error":
		return 7
	case "failed":
		return 6
	case "ambiguous":
		return 5
	case "unknown":
		return 4
	case "not_run":
		return 3
	case "skipped":
		return 2
	case "passed":
		return 1
	}
	return 4
}

// Ordinary summaries do not return XML. Persisted report projections exclude
// it, while the current workspace-proof helper still decodes the private
// attempt result. GetTestPlanReport explicitly exposes an authorized original.
func (s *Service) testPlanExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project string, current testPlanAggregate, fixed testPlanRunSnapshot, run string) (out TestPlanExecution, err error) {
	out = TestPlanExecution{PlanID: current.ID, TaskID: fixed.TaskID, RunID: run, CreatedAt: fixed.CreatedAt, State: "pending", BasisCurrent: true, SourceCommits: []string{}, Cases: []TestCaseResult{}, Reports: []TestPlanReportRef{}}
	err = tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, run).Scan(&out.RunState)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if fixed.Revision == nil {
		if current.Published > 0 || current.Governance != fixed.GovernanceRevision {
			out.State = "run_changed"
			out.BasisCurrent = false
		}
		return out, nil
	}
	out.Revision = fixed.Revision.Revision
	out.PlanSHA256 = fixed.Revision.SHA256
	valid, err := s.testPlanSourceCurrentTx(ctx, tx, tenant, project, fixed.Revision.Source)
	if err != nil {
		return out, err
	}
	basisState := ""
	if !valid {
		basisState = "source_changed"
	} else if current.State == "retired" || current.Published == 0 {
		basisState = "retired"
	} else if current.ID != fixed.PlanID || current.Published != fixed.Revision.Revision || current.Governance != fixed.GovernanceRevision {
		basisState = "run_changed"
	}
	var currentRun string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, fixed.TaskID).Scan(&currentRun); err != nil {
		return out, err
	}
	if basisState == "" && currentRun != run {
		basisState = "run_changed"
	}
	compensated, err := s.evidenceCompensatedTx(ctx, tx, run, false)
	if err != nil {
		return out, err
	}
	if compensated && basisState == "" {
		basisState = "run_changed"
	}
	out.BasisCurrent = basisState == ""
	nodes, err := s.testPlanNodesTx(ctx, tx, tenant, project, run)
	if err != nil {
		return out, err
	}
	reports, err := s.testPlanReportsTx(ctx, tx, tenant, project, run)
	if err != nil {
		return out, err
	}
	heads := map[string]bool{}
	invalidCode := false
	for _, n := range nodes {
		if n.State == "skipped" {
			continue
		}
		if n.Spec.Type == "command" || n.Spec.Type == "agent" || n.Spec.Type == "task" || n.Spec.Type == "script" {
			head, e := workspaceRevision(n.Workspace)
			if e != nil {
				invalidCode = true
				continue
			}
			heads[head] = true
		}
	}
	allKnownReports := true
	for _, binding := range fixed.PackageTests {
		node, exists := nodes[binding.NodeID]
		if !exists {
			allKnownReports = false
			continue
		}
		current, e := s.testPlanPackageProofCurrentTx(ctx, tx, tenant, project, run, binding, node)
		if e != nil {
			return out, e
		}
		if !current {
			allKnownReports = false
		}
	}
	extraFailure := false
	unknownReport := false
	for i := range reports {
		report := &reports[i]
		node, exists := nodes[report.NodeID]
		if !exists || node.Attempt != report.Attempt || node.Fence != report.Fence || node.Worker != report.WorkerID {
			report.Eligibility = "stale_attempt"
		} else if node.State == "reconciling" || node.State == "running" || node.State == "ready" || report.OutcomeUnknown {
			report.Eligibility = "unknown"
		} else if report.Eligibility == "current" && basisState != "" {
			report.Eligibility = "source_changed"
		}
		out.Reports = append(out.Reports, report.TestPlanReportRef)
		if report.Eligibility == "stale_attempt" {
			continue
		}
		if report.Eligibility != "current" && report.Eligibility != "source_changed" {
			allKnownReports = false
			if report.Eligibility == "unknown" {
				unknownReport = true
			}
			continue
		}
		if report.Failed > 0 || report.Errors > 0 || report.ProcessExitCode != 0 {
			extraFailure = true
		}
	}
	// Test-only workflows have a fixed source reported by their administrator's
	// command. Code-producing workflows must cover every selected code head.
	if len(heads) == 0 && !invalidCode {
		for _, report := range reports {
			if (report.Eligibility == "current" || report.Eligibility == "source_changed") && gitRevision.MatchString(report.SourceCommit) {
				heads[report.SourceCommit] = true
			}
		}
	}
	for head := range heads {
		out.SourceCommits = append(out.SourceCommits, head)
	}
	sort.Strings(out.SourceCommits)
	issues := map[string]string{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT case_key,issue_id FROM suma_test_plan_defects WHERE tenant_id=? AND project_id=? AND plan_id=? AND run_id=?`), tenant, project, current.ID, run)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var key, id string
		if err = rows.Scan(&key, &id); err != nil {
			break
		}
		issues[key] = id
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, definition := range fixed.Revision.Cases {
		item := TestCaseResult{CaseKey: definition.Key, Title: definition.Title, Status: "passed", Covers: append([]int{}, definition.Covers...), Evidence: []TestCaseEvidence{}, IssueID: issues[definition.Key]}
		if len(out.SourceCommits) == 0 {
			item.Status = "not_run"
		}
		for _, head := range out.SourceCommits {
			matches := []TestCaseEvidence{}
			for _, report := range reports {
				if report.Eligibility != "current" && report.Eligibility != "source_changed" || report.SourceCommit != head {
					continue
				}
				for _, c := range report.Cases {
					if c.Selector == definition.Selector {
						matches = append(matches, TestCaseEvidence{ReportID: report.ID, NodeID: report.NodeID, Attempt: report.Attempt, Fence: report.Fence, WorkerID: report.WorkerID, SourceCommit: head, Status: c.Status, Message: c.Message, DurationMS: c.DurationMS})
					}
				}
			}
			status := "not_run"
			if len(matches) == 1 {
				status = matches[0].Status
			} else if len(matches) > 1 {
				status = "ambiguous"
			}
			item.Evidence = append(item.Evidence, matches...)
			if testPlanEvidenceSeverity(status) > testPlanEvidenceSeverity(item.Status) {
				item.Status = status
			}
		}
		if unknownReport && len(item.Evidence) == 0 {
			item.Status = "unknown"
		}
		out.Total++
		switch item.Status {
		case "passed":
			out.Passed++
		case "failed":
			out.Failed++
		case "error":
			out.Errors++
		case "skipped":
			out.Skipped++
		default:
			out.NotRun++
		}
		out.Cases = append(out.Cases, item)
	}
	switch {
	case basisState != "":
		out.State = basisState
	case out.RunState == "reconciling" || unknownReport:
		out.State = "unknown"
	case out.RunState == "failed" || out.RunState == "cancelled" || out.Failed > 0 || out.Errors > 0 || extraFailure:
		out.State = "failed"
	case out.RunState != "succeeded":
		out.State = "pending"
	case invalidCode || !allKnownReports || out.Total == 0 || out.Passed != out.Total || len(out.Reports) == 0:
		out.State = "incomplete"
	default:
		out.State = "passed"
	}
	return out, nil
}

func (s *Service) GetTestPlanExecution(ctx context.Context, actor Actor, project, id, run string) (out TestPlanExecution, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) || !testPlanRunID(run) {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, auth.target.tenantID, project, run)
	if err != nil {
		return out, err
	}
	if !found || fixed.TaskID != current.TaskID {
		return out, missing()
	}
	return s.testPlanExecutionTx(ctx, auth.tx, auth.target.tenantID, project, current, fixed, run)
}
func (s *Service) ListTestPlanExecutions(ctx context.Context, actor Actor, project, id string, limit int, before string) (out TestPlanExecutionList, err error) {
	out.Items = []TestPlanExecution{}
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(id, false) || before != "" && !testPlanRunID(before) || limit < 1 || limit > 100 {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	tenant := auth.target.tenantID
	current, err := s.testPlanTx(ctx, auth.tx, tenant, project, id)
	if err != nil {
		return out, err
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_test_plan_runs WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, current.TaskID).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT run_id FROM suma_test_plan_runs WHERE tenant_id=? AND project_id=? AND task_id=?`
	args := []any{tenant, project, current.TaskID}
	if before != "" {
		fixed, found, e := s.testPlanRunTx(ctx, auth.tx, tenant, project, before)
		if e != nil {
			return out, e
		}
		if !found || fixed.TaskID != current.TaskID {
			return out, missing()
		}
		query += ` AND (created_at<? OR (created_at=? AND run_id<?))`
		args = append(args, fixed.CreatedAt, fixed.CreatedAt, before)
	}
	query += ` ORDER BY created_at DESC,run_id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, e := auth.tx.QueryContext(ctx, s.q(query), args...)
	if e != nil {
		return out, e
	}
	runs := []string{}
	for rows.Next() {
		var run string
		if e = rows.Scan(&run); e != nil {
			break
		}
		runs = append(runs, run)
	}
	if e == nil {
		e = rows.Err()
	}
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(runs) > limit {
		runs = runs[:limit]
		out.NextCursor = runs[len(runs)-1]
	}
	for _, run := range runs {
		fixed, _, e := s.testPlanRunTx(ctx, auth.tx, tenant, project, run)
		if e != nil {
			return out, e
		}
		execution, e := s.testPlanExecutionTx(ctx, auth.tx, tenant, project, current, fixed, run)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, execution)
	}
	return out, nil
}
func (s *Service) GetTestPlanReport(ctx context.Context, actor Actor, project, id, reportID string) (out TestPlanReport, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) || !handoffID(reportID, false) {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_test_plan_reports WHERE tenant_id=? AND project_id=? AND plan_id=? AND id=?`), auth.target.tenantID, project, id, reportID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, auth.target.tenantID, project, out.RunID)
	if err != nil {
		return out, err
	}
	if !found {
		return out, missing()
	}
	execution, err := s.testPlanExecutionTx(ctx, auth.tx, auth.target.tenantID, project, current, fixed, out.RunID)
	if err != nil {
		return out, err
	}
	for _, ref := range execution.Reports {
		if ref.ID == reportID {
			out.Eligibility = ref.Eligibility
			break
		}
	}
	return out, nil
}
