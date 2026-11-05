package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

type testPlanPackageAttempt struct {
	NodeID  string
	Attempt int
	Fence   int64
}

type testPlanPackageFailure struct {
	input    *rc.ArtifactBinding
	exitCode int
}

func testPlanPackageNodeID(value string) bool {
	return testPlanRunID(value) && !strings.ContainsRune("._-", rune(value[0]))
}

// The Runner launches the child only after it verifies the materialized
// manifest. A positive exit on the fixed test profile and clean source, paired
// with the exact authorized input receipt, distinguishes a package failure from
// a checkout, transfer, startup, stop, timeout, or reconciliation failure.
func (s *Service) testPlanPackageFailureCurrentTx(ctx context.Context, tx *sql.Tx, tenant, project, run string, fixed testPlanPackageBinding, node testPlanNodeFact) (*testPlanPackageFailure, error) {
	if node.State != "failed" || node.Attempt < 1 || node.Fence < 1 || node.Worker == fixed.SourceWorkerID {
		return nil, nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT a.result FROM dag_attempts a JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.run_id=? AND a.node_id=? AND a.attempt=? AND a.fence=? AND a.worker_id=? AND a.state='failed' AND a.completed_at<>''`), tenant, project, run, node.ID, node.Attempt, node.Fence, node.Worker).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result dag.Result
	if json.Unmarshal([]byte(raw), &result) != nil || result.RunID != run || result.NodeID != node.ID || result.Attempt != node.Attempt || result.Fence != node.Fence || result.WorkerID != node.Worker || result.ExitCode <= 0 || result.Stopped || result.OutcomeUnknown || result.StartupFailure != nil || result.ArtifactEvidence != nil || result.ToolEvidence == nil || result.ToolEvidence.Terminal != "failed" {
		return nil, nil
	}
	if validateTestPlanPackageResult(fixed, node.Spec, result) != nil {
		return nil, nil
	}
	workspace := result.WorkspaceEvidence
	head, _ := workspace["head_commit"].(string)
	hash, _ := workspace["evidence_sha256"].(string)
	if !gitRevision.MatchString(head) || !contentDigest.MatchString(hash) || workspace["base_commit"] != head || workspace["clean"] != true || workspace["state"] != "completed" || workspace["test_command"] != true || workspace["outcome_unknown"] != false || workspace["exit_code"] != float64(result.ExitCode) {
		return nil, nil
	}
	input, err := s.testPlanPackageInputCurrentTx(ctx, tx, tenant, project, run, fixed, node)
	if err != nil || input == nil {
		return nil, err
	}
	_, manifestSHA, err := rc.CanonicalManifest(input.Manifest)
	if err != nil || manifestSHA != input.ManifestSHA256 || input.SourceCommit != head {
		return nil, nil
	}
	return &testPlanPackageFailure{input: input, exitCode: result.ExitCode}, nil
}

// CreateTestPlanPackageDefect keeps failure feedback separate from JUnit
// selectors. The protected package attempt slot and issue commit together.
func (s *Service) CreateTestPlanPackageDefect(ctx context.Context, actor Actor, project, id, run string, request TestPlanPackageDefectRequest) (out Record, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if request.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || !testPlanRunID(run) || !testPlanPackageNodeID(request.NodeID) || !designVersion(request.ExpectedVersion) || request.ExpectedVersion < 1 || !designVersion(request.ExpectedAttempt) || request.ExpectedAttempt < 1 || request.ExpectedFence < 1 || request.ExpectedFence > 9007199254740991 || !testPlanText(request.Reason, 2000, true) || !planningKey(request.IdempotencyKey) {
		return out, invalid(testPlanInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"test_plan_package_defect": id, "run": run, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	tenant := auth.target.tenantID
	current, err := s.testPlanTx(ctx, auth.tx, tenant, project, id)
	if err != nil {
		return out, err
	}
	if current.Version != request.ExpectedVersion {
		return out, conflict(testPlanVersion)
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, tenant, project, run)
	if err != nil {
		return out, err
	}
	if !found || fixed.PlanID != id || fixed.TaskID != current.TaskID || fixed.Revision == nil {
		return out, missing()
	}
	basis, err := s.testPlanExecutionBasisTx(ctx, auth.tx, tenant, project, current, fixed, run)
	if err != nil {
		return out, err
	}
	if basis != "" {
		return out, conflict(testPlanSource)
	}
	binding, found := testPlanPackageBindingFor(fixed, request.NodeID)
	if !found {
		return out, invalid(testPlanPackageDefect)
	}
	node, err := s.testPlanPackageNodeTx(ctx, auth.tx, tenant, project, run, request.NodeID)
	if err != nil {
		return out, err
	}
	if node.Attempt > 0 && (node.Attempt != request.ExpectedAttempt || node.Fence != request.ExpectedFence) {
		return out, conflict(testPlanPackageAttemptChanged)
	}
	failure, err := s.testPlanPackageFailureCurrentTx(ctx, auth.tx, tenant, project, run, binding, node)
	if err != nil {
		return out, err
	}
	if failure == nil {
		return out, invalid(testPlanPackageDefect)
	}
	keys := []any{tenant, project, id, run, node.ID, node.Attempt, node.Fence}
	var issueID string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT issue_id FROM suma_test_plan_package_defects WHERE tenant_id=? AND project_id=? AND plan_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`), keys...).Scan(&issueID)
	if err == nil {
		out, err = s.getRecordTx(ctx, auth, project, "issues", issueID)
		if err != nil {
			return out, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	} else {
		input := failure.input
		// Retain only bounded provenance, never child output or manifest paths.
		data := map[string]any{
			"origin": "test_plan_package", "plan_id": id, "plan_revision": fixed.Revision.Revision, "plan_sha256": fixed.Revision.SHA256,
			"task_id": current.TaskID, "run_id": run, "node_id": node.ID, "attempt": node.Attempt, "fence": node.Fence, "worker_id": node.Worker,
			"exit_code": failure.exitCode, "artifact_id": input.ArtifactID, "manifest_sha256": input.ManifestSHA256, "source_commit": input.SourceCommit,
			"build_run_id": input.BuildRunID, "build_node_id": input.BuildNodeID, "build_attempt": input.BuildAttempt, "build_fence": input.BuildFence,
			"build_worker_id": binding.SourceWorkerID,
		}
		title := strings.TrimSpace(node.Spec.Name)
		if !testPlanText(title, 200, true) {
			title = node.ID
		}
		out = recordFrom(auth.currentPrincipal, project, map[string]any{"title": title, "description": request.Reason, "status": "open", "priority": "high", "data": data})
		if err = s.validateRecord(ctx, auth.tx, auth, out, "issues"); err != nil {
			return out, err
		}
		if err = s.insertCommandRecord(ctx, auth.tx, auth, out, "issues"); err != nil {
			return out, err
		}
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_test_plan_package_defects(tenant_id,project_id,plan_id,run_id,node_id,attempt,fence,issue_id,created_at,evidence)VALUES(?,?,?,?,?,?,?,?,?,?)`), append(keys, out.ID, now(), stringify(data))...); err != nil {
			return out, err
		}
		if err = s.commandActivity(ctx, auth, "test_plan.package_defect_created", out.ID, request.Reason); err != nil {
			return out, err
		}
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return Record{}, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) testPlanPackageNodeTx(ctx context.Context, tx *sql.Tx, tenant, project, run, nodeID string) (out testPlanNodeFact, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT n.id,n.state,n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`), tenant, project, run, nodeID).Scan(&out.ID, &out.State, &out.Attempt, &out.Fence, &out.Worker, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out.Spec)
	return out, err
}

func (s *Service) testPlanPackageIssuesTx(ctx context.Context, tx *sql.Tx, tenant, project, plan, run string) (map[testPlanPackageAttempt]string, error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT node_id,attempt,fence,issue_id FROM suma_test_plan_package_defects WHERE tenant_id=? AND project_id=? AND plan_id=? AND run_id=?`), tenant, project, plan, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[testPlanPackageAttempt]string{}
	for rows.Next() {
		var key testPlanPackageAttempt
		var issueID string
		if err = rows.Scan(&key.NodeID, &key.Attempt, &key.Fence, &issueID); err != nil {
			return nil, err
		}
		out[key] = issueID
	}
	return out, rows.Err()
}
