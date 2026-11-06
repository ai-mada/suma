package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestFailureIssueKeyCannotBeReservedByProjectOrCandidateCommand(t *testing.T) {
	f, token, candidate := candidateTransferFixture(t)
	ctx := context.Background()
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{
		Name: "Reserved failure projection receipt", Nodes: []dag.Node{{ID: "gate", Type: "approval"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	key := failureIssueKey(run.ID)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Independent test plan source", "acceptance_criteria": []string{"Reviewed check"}}), 201)
	plan := TestPlanCreateRequest{ProjectID: f.project, TaskID: task["id"].(string), ExpectedTaskVersion: 1, Title: "Valid independent draft", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: key}
	f.call("POST", "/test-plans", plan, 400)
	plan.IdempotencyKey = strings.ToUpper(key)
	f.call("POST", "/test-plans", plan, 400)
	ordinaryCandidateKey := candidate.OperationID
	candidate.OperationID = key
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, candidate), 400)
	var receipts int
	if err = f.s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, key).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("public write reserved the failure projection slot: %d, %v", receipts, err)
	}
	if expertCount(t, f, "suma_test_plans") != 0 || expertCount(t, f, "suma_candidate_transfers") != 0 {
		t.Fatal("rejected command left a partial resource")
	}
	// Both requests are otherwise valid and recover normally with public keys.
	plan.IdempotencyKey = newID()
	f.call("POST", "/test-plans", plan, 201)
	candidate.OperationID = ordinaryCandidateKey
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, candidate), 201)

	if _, err = f.e.Command(ctx, f.user.TenantID, f.project, run.ID, "approve", newID(), run.Version, map[string]any{"node_id": "gate", "decision": "rejected"}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.s.ProjectFailures(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var hash, response string
	if err = f.s.DB.QueryRowContext(ctx, `SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, key).Scan(&hash, &response); err != nil {
		t.Fatal(err)
	}
	issueID, err := failureIssueReceiptID(response, hash, failureIssueHash(failureIssueBody(run.ID, run.WorkflowName)), f.user.TenantID, f.project, run.ID)
	if err != nil || expertCount(t, f, "suma_issues") != 1 || expertCount(t, f, "suma_failure_projection") != 1 {
		t.Fatalf("genuine failed run did not create its one authoritative issue: %v", err)
	}
	intakePath := "/issues/" + issueID + "/intake"
	intake := f.body(map[string]any{})
	first := f.call("POST", intakePath, intake, 201)
	replayed := f.call("POST", intakePath, intake, 201)
	if first["id"] != replayed["id"] || expertCount(t, f, "suma_requirements") != 1 {
		t.Fatal("projected issue did not retain one intake requirement")
	}
}

func TestFailureIssueKeyReservationRejectsOceanBaseUnicodeAliases(t *testing.T) {
	s := &Service{Dialect: "oceanbase"}
	for _, key := range []string{"Ｆailure-run", "fáilure-run", " failure-run", "FAILURE-run"} {
		if err := s.checkFailureIssueKeyReservation(key); err == nil {
			t.Errorf("unexpectedly allowed %q", key)
		}
	}
	for _, key := range []string{"review-rejected-review", "issue-intake-issue", "candidate:ack_2"} {
		if err := s.checkFailureIssueKeyReservation(key); err != nil {
			t.Errorf("rejected %q: %v", key, err)
		}
	}
}
