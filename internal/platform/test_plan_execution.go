package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

func (s *Service) testPlanRunTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (out testPlanRunSnapshot, found bool, err error) {
	var raw, task, plan string
	err = tx.QueryRowContext(ctx, s.q(`SELECT task_id,plan_id,snapshot FROM suma_test_plan_runs WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&task, &plan, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, true, err
	}
	if out.TaskID != task || out.PlanID != plan || out.GovernanceRevision < 0 || out.Revision != nil && (out.GovernanceRevision != out.Revision.Revision) || out.Revision != nil && (out.Revision.SHA256 != testPlanRevisionHash(*out.Revision) || out.Revision.State != "published") {
		return out, true, fmt.Errorf("test plan run snapshot is inconsistent")
	}
	if err = validateTestPlanPackageBindings(out.PackageTests); err != nil {
		return out, true, err
	}
	return out, true, nil
}

// Even an absent publication is frozen explicitly. Publishing later cannot
// certify an older run, and retiring a plan cannot erase an older run's gate.
func (s *Service) freezeTaskTestPlanRunTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, taskID, runID string) error {
	if err := assertProjectCommand(tx, auth, project); err != nil {
		return err
	}
	tenant := auth.target.tenantID
	if fixed, found, err := s.testPlanRunTx(ctx, tx, tenant, project, runID); err != nil {
		return err
	} else if found {
		if fixed.TaskID != taskID {
			return conflict(testPlanGate)
		}
		return nil
	}
	revision, err := s.taskTestPlanContextTx(ctx, tx, tenant, project, taskID)
	if err != nil {
		return err
	}
	current, found, err := s.testPlanByTaskTx(ctx, tx, tenant, project, taskID)
	if err != nil {
		return err
	}
	snapshot := testPlanRunSnapshot{TaskID: taskID, Revision: revision, CreatedAt: now(), PackageTests: []testPlanPackageBinding{}}
	if found {
		snapshot.PlanID = current.ID
		snapshot.GovernanceRevision = current.Governance
	}
	cloud, cloudFound, cloudErr := s.workBuddyTaskDispatchRunSourceTx(ctx, tx, tenant, project, runID)
	if cloudErr != nil {
		return cloudErr
	}
	if cloudFound {
		// Snapshot-only: a cloud dispatch freezes the same protected published
		// quality basis, but supplies no local test mapping or passing evidence.
		var current *TaskTestPlanContext
		if revision != nil {
			current = &TaskTestPlanContext{Revision: revision.Revision, SHA256: revision.SHA256, SourceSHA256: revision.Source.SHA256, CaseCount: len(revision.Cases), Title: revision.Title}
		}
		if cloud.TaskID != taskID || cloud.State != "ready" && cloud.State != "started" || stringify(cloud.Context.TestPlan) != stringify(current) {
			return conflict(testPlanGate)
		}
	}
	if revision != nil && !cloudFound {
		rows, e := tx.QueryContext(ctx, s.q(`SELECT spec FROM dag_nodes WHERE run_id=?`), runID)
		if e != nil {
			return e
		}
		specs := []dag.Node{}
		for rows.Next() {
			var raw string
			var node dag.Node
			if e = rows.Scan(&raw); e != nil {
				break
			}
			if e = json.Unmarshal([]byte(raw), &node); e != nil {
				break
			}
			specs = append(specs, node)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		count := 0
		for _, node := range specs {
			if node.Type == "map" || node.Type == "subworkflow" || node.Type == "external_workflow" {
				return invalid(testPlanBinding)
			}
			if node.Type != "test" {
				continue
			}
			id, _ := node.Config["tool_profile_id"].(string)
			sha, _ := node.Config["tool_profile_sha256"].(string)
			if !handoffID(id, false) || !contentDigest.MatchString(sha) || len(node.Command) != 1 {
				return invalid(testPlanBinding)
			}
			profile, e := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, id)
			if e != nil {
				return e
			}
			if profile.Profile.CommandID != node.Command[0] || profile.Profile.ProfileSHA256 != sha {
				return invalid(testPlanBinding)
			}
			if _, e = s.validateToolDispatchTx(ctx, tx, auth, project, id, profile.Version, sha); e != nil {
				return e
			}
			if profile.Profile.TestReportProtocol == tr.Protocol {
				// A report node cannot also consume an artifact. Runner contracts
				// deliberately keep these two independent evidence kinds apart.
				if _, exists := node.Config["artifact_from"]; exists {
					return invalid(testPlanBinding)
				}
				count++
				continue
			}
			binding, e := s.freezeTestPlanPackageBindingTx(ctx, tx, auth, project, runID, node, profile, specs)
			if e != nil {
				return e
			}
			snapshot.PackageTests = append(snapshot.PackageTests, binding)
		}
		if count == 0 {
			return invalid(testPlanBinding)
		}
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plan_runs(tenant_id,project_id,run_id,task_id,plan_id,created_at,snapshot)VALUES(?,?,?,?,?,?,?)`), tenant, project, runID, taskID, snapshot.PlanID, snapshot.CreatedAt, stringify(snapshot))
	return err
}

func testPlanReportWorkspace(result dag.Result) (string, bool) {
	p := result.WorkspaceEvidence
	head, _ := p["head_commit"].(string)
	base, _ := p["base_commit"].(string)
	sha, _ := p["evidence_sha256"].(string)
	if !gitRevision.MatchString(head) || base != head || !contentDigest.MatchString(sha) || p["test_command"] != true || p["clean"] != true || p["state"] != "completed" || p["outcome_unknown"] != false {
		return "", false
	}
	exit := stringify(p["exit_code"])
	if exit != stringify(result.ExitCode) {
		return "", false
	}
	return head, true
}

// The transport and Core have already validated current Runner/Pool authority
// and the exact assignment. Domain parsing is deliberately a separate outcome:
// a successful process with no/invalid report cannot become a passing plan.
func (s *Service) recordTaskTestPlanReportTx(ctx context.Context, tx *sql.Tx, tenant, project string, result dag.Result) error {
	fixed, found, err := s.testPlanRunTx(ctx, tx, tenant, project, result.RunID)
	if err != nil || !found || fixed.Revision == nil {
		return err
	}
	var attempt int
	var fence int64
	var worker, raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, result.RunID, result.NodeID).Scan(&attempt, &fence, &worker, &raw)
	if err != nil {
		return err
	}
	var node dag.Node
	if err = json.Unmarshal([]byte(raw), &node); err != nil {
		return err
	}
	if node.Type != "test" {
		return nil
	}
	if result.Attempt != attempt || result.Fence != fence || result.WorkerID != worker {
		return forbidden()
	}
	if binding, packageTest := testPlanPackageBindingFor(fixed, node.ID); packageTest {
		// No synthetic missing-JUnit report is created for an artifact test.
		// Its actual sealed input and current artifact proof are checked by the
		// release authority and by the plan projection, not counted as a case.
		return validateTestPlanPackageResult(binding, node, result)
	}
	id := digest(stringify([]any{tenant, project, result.RunID, result.NodeID, result.Attempt, result.Fence}))[:32]
	var existing string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_test_plan_reports WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&existing)
	if err == nil {
		var old TestPlanReport
		if json.Unmarshal([]byte(existing), &old) != nil {
			return fmt.Errorf("test plan report is inconsistent")
		}
		if old.OutcomeUnknown && !result.OutcomeUnknown {
			return conflict(testPlanGate)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	report := TestPlanReport{TestPlanReportRef: TestPlanReportRef{ID: id, RunID: result.RunID, NodeID: result.NodeID, Attempt: result.Attempt, Fence: result.Fence, WorkerID: result.WorkerID, Protocol: tr.Protocol, Eligibility: "invalid", ProcessExitCode: result.ExitCode, OutcomeUnknown: result.OutcomeUnknown, ReportedAt: now()}, Cases: []tr.TestReportCase{}}
	head, workspace := testPlanReportWorkspace(result)
	report.SourceCommit = head
	if result.TestReport != nil {
		report.Protocol = result.TestReport.Protocol
		report.SHA256 = result.TestReport.SHA256
		if len(result.TestReport.XML) <= tr.MaxXMLBytes && utf8.ValidString(result.TestReport.XML) {
			report.XML = result.TestReport.XML
		}
	}
	if result.OutcomeUnknown || result.Stopped {
		report.Eligibility = "unknown"
	} else if workspace && result.TestReport != nil && result.ToolEvidence != nil && result.ToolEvidence.Profile.TestReportProtocol == tr.Protocol && result.ToolEvidence.Profile.ProfileSHA256 == node.Config["tool_profile_sha256"] && len(node.Command) == 1 && result.ToolEvidence.Profile.CommandID == node.Command[0] {
		parsed, e := tr.Validate(*result.TestReport)
		if e == nil {
			report.Eligibility = "current"
			report.Cases = parsed.Cases
			report.Total, report.Passed, report.Failed, report.Errors, report.Skipped = parsed.Total, parsed.Passed, parsed.Failed, parsed.Errors, parsed.Skipped
			selectors := map[string]bool{}
			for _, c := range fixed.Revision.Cases {
				selectors[testPlanSelectorKey(c.Selector)] = true
			}
			for _, c := range report.Cases {
				if !selectors[testPlanSelectorKey(c.Selector)] {
					report.Unmapped++
				}
			}
		}
	}
	projection := report
	projection.XML = ""
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plan_reports(id,tenant_id,project_id,plan_id,run_id,node_id,attempt,fence,created_at,projection,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), id, tenant, project, fixed.PlanID, result.RunID, result.NodeID, result.Attempt, result.Fence, report.ReportedAt, stringify(projection), stringify(report))
	return err
}

func (s *Service) testPlanReportsTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (out []TestPlanReport, err error) {
	out = []TestPlanReport{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT projection FROM suma_test_plan_reports WHERE tenant_id=? AND project_id=? AND run_id=? ORDER BY node_id,attempt,fence,id`), tenant, project, run)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var report TestPlanReport
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &report); err != nil {
			return out, err
		}
		out = append(out, report)
	}
	return out, rows.Err()
}

func (s *Service) checkCurrentTaskTestPlanRunTx(ctx context.Context, tx *sql.Tx, tenant, project, task, run string) error {
	current, found, err := s.testPlanByTaskTx(ctx, tx, tenant, project, task)
	if err != nil {
		return err
	}
	fixed, hasSnapshot, err := s.testPlanRunTx(ctx, tx, tenant, project, run)
	if err != nil {
		return err
	}
	if !hasSnapshot {
		if found && (current.Published > 0 || current.Governance > 0) {
			return conflict(testPlanGate)
		}
		return nil
	}
	if fixed.TaskID != task {
		return conflict(testPlanGate)
	}
	if fixed.Revision == nil {
		// A publish/retire round trip must never restore an old no-plan run.
		// A genuinely new run after retirement freezes the new epoch instead.
		if found && (current.Published > 0 || current.Governance != fixed.GovernanceRevision) || !found && fixed.GovernanceRevision != 0 {
			return conflict(testPlanGate)
		}
		return nil
	}
	if !found || fixed.PlanID != current.ID || current.Governance != fixed.GovernanceRevision {
		return conflict(testPlanGate)
	}
	execution, err := s.testPlanExecutionTx(ctx, tx, tenant, project, current, fixed, run)
	if err != nil {
		return err
	}
	if execution.State != "passed" || !execution.BasisCurrent {
		return conflict(testPlanGate)
	}
	return nil
}
func (s *Service) checkTestPlanRunEvidenceTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) error {
	fixed, found, err := s.testPlanRunTx(ctx, tx, tenant, project, run)
	if err != nil {
		return err
	}
	if found {
		return s.checkCurrentTaskTestPlanRunTx(ctx, tx, tenant, project, fixed.TaskID, run)
	}
	// A missing protected binding cannot silently remove a currently required
	// plan, including when this run is reached through predecessor acceptance.
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run)
	if err != nil {
		return err
	}
	tasks := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		tasks = append(tasks, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if err = s.checkCurrentTaskTestPlanRunTx(ctx, tx, tenant, project, task, run); err != nil {
			return err
		}
	}
	return nil
}
func testPlanRunID(value string) bool {
	if len(value) < 1 || len(value) > 100 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c)) {
			return false
		}
	}
	return true
}
