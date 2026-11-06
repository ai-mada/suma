package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestRunnerProgressHTTPAuthorizationAndDurableReceipt(t *testing.T) {
	f := newFixture(t)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Progress integration", "nodes": []any{map[string]any{"id": "work", "type": "command", "command": []string{"inspect"}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"]}), 201)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "integration-worker", "capabilities": []string{"command:inspect"}}), 200)
	var a dag.Assignment
	b, _ := json.Marshal(claimed["assignment"])
	if err := json.Unmarshal(b, &a); err != nil || a.WorkerID != "integration-worker" {
		t.Fatalf("claim: %+v %v", a, err)
	}
	request := f.body(map[string]any{"assignment": a, "operation_id": "actual-progress-1", "message": "stdout token=fixture-secret\n"})
	f.call("POST", "/runner/progress", request, 401)
	for _, kind := range []string{"api", "agent"} {
		scoped := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": kind}), 201)["token"].(string)
		f.bearer("POST", "/runner/progress", scoped, request, 403)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, 201)
	foreign := map[string]any{"project_id": other["id"], "assignment": a, "operation_id": "foreign-project", "message": "text"}
	f.bearer("POST", "/runner/progress", token, foreign, 403)
	stale := a
	stale.Fence++
	unknown := f.bearer("POST", "/runner/progress", token, f.body(map[string]any{"assignment": stale, "operation_id": "stale-progress", "message": "text"}), 409)
	if e := unknown["error"].(map[string]any); e["code"] != "version_conflict" {
		t.Fatalf("unknown fence must keep its generic rejection: %+v", e)
	}
	f.bearer("POST", "/runner/progress", token, f.body(map[string]any{"assignment": a, "operation_id": "large-progress", "message": strings.Repeat("x", 8193)}), 400)
	for range 2 {
		ack := f.bearer("POST", "/runner/progress", token, request, 200)
		if ack["acknowledged"] != true {
			t.Fatal("progress did not ACK persistence")
		}
	}
	conflict := f.bearer("POST", "/runner/progress", token, f.body(map[string]any{"assignment": a, "operation_id": "actual-progress-1", "message": "changed request"}), 409)
	if e := conflict["error"].(map[string]any); e["code"] != "version_conflict" {
		t.Fatalf("changed receipt body must keep its conflict: %+v", e)
	}
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 0, Output: "final output", OperationID: "final-progress-result"}}), 200)
	f.bearer("POST", "/runner/progress", token, request, 200)
	closed := f.bearer("POST", "/runner/progress", token, f.body(map[string]any{"assignment": a, "operation_id": "late-progress", "message": "new output after final"}), 409)
	if e := closed["error"].(map[string]any); e["code"] != "runner_progress_stale" || e["message_key"] != "dag.error.stale_assignment" {
		t.Fatalf("closed assignment must return its localized progress rejection: %+v", e)
	}
	current := f.call("GET", "/runs/"+run["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["state"] != "succeeded" || current["nodes"].([]any)[0].(map[string]any)["output"] != "final output" {
		t.Fatal("progress replaced final result")
	}
	events := f.call("GET", "/runs/"+run["id"].(string)+"/events?project_id="+f.project, nil, 200)
	count := 0
	for _, raw := range events["items"].([]any) {
		ev := raw.(map[string]any)
		if ev["type"] == "node_progress" {
			count++
			message := ev["data"].(map[string]any)["message"].(string)
			if strings.Contains(message, "fixture-secret") {
				t.Fatal("raw credential stored")
			}
		}
	}
	if count != 1 {
		t.Fatalf("duplicate durable progress: %d", count)
	}
}

func TestDynamicMapRunReturnsPinnedCapabilitiesAndCurrentAttemptEvidence(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	wf := dag.Workflow{ID: "dynamic-evidence", Name: "Dynamic evidence", Nodes: []dag.Node{{ID: "fan", Type: "map", Config: map[string]any{"items": []any{"item"}, "template": map[string]any{"type": "test", "command": []string{"test"}, "idempotent": true, "config": map[string]any{"compensation": map[string]any{"type": "command", "command": []string{"undo"}, "idempotent": true}}}}}}}
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, wf); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, wf.ID, "dynamic-run")
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "dynamic-worker", toolFixtureCapabilities(t, f))
	if err != nil || a == nil {
		t.Fatalf("claim: %v", err)
	}
	current := f.call("GET", "/runs/"+run.ID+"?project_id="+f.project, nil, 200)
	assertDynamic := func(snapshot map[string]any, wantEvidence bool) {
		t.Helper()
		found := false
		for _, raw := range snapshot["nodes"].([]any) {
			node := raw.(map[string]any)
			if node["id"] == a.NodeID {
				found = true
				if node["idempotent"] != true || node["compensatable"] != true {
					t.Fatalf("dynamic node capability lost: %+v", node)
				}
				evidence, ok := node["workspace_evidence"].(map[string]any)
				if wantEvidence && (!ok || evidence["head_commit"] != strings.Repeat("c", 40)) {
					t.Fatalf("evidence lost: %+v", node)
				}
				if !wantEvidence && ok {
					t.Fatal("new attempt inherited previous evidence")
				}
			}
		}
		if !found {
			t.Fatal("dynamic child absent")
		}
	}
	assertDynamic(current, false)
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, Error: "fixture failure", OperationID: "dynamic-first", WorkspaceEvidence: reviewWorkspaceEvidence("c", false)}); err != nil {
		t.Fatal(err)
	}
	current = f.call("GET", "/runs/"+run.ID+"?project_id="+f.project, nil, 200)
	assertDynamic(current, true)
	f.call("POST", "/runs/"+run.ID+"/commands", f.body(map[string]any{"expected_version": current["version"], "action": "retry", "payload": map[string]any{"node_id": a.NodeID}}), 200)
	current = f.call("GET", "/runs/"+run.ID+"?project_id="+f.project, nil, 200)
	assertDynamic(current, false)
	a, err = f.e.Claim(ctx, f.user.TenantID, f.project, "dynamic-worker", toolFixtureCapabilities(t, f))
	if err != nil || a == nil || a.Attempt != 2 {
		t.Fatalf("retry claim: %+v %v", a, err)
	}
	current = f.call("GET", "/runs/"+run.ID+"?project_id="+f.project, nil, 200)
	assertDynamic(current, false)
}

func reviewWorkspaceEvidence(head string, test bool) map[string]any {
	return map[string]any{"attempt_key": strings.Repeat("a", 32), "base_commit": strings.Repeat("b", 40), "head_commit": strings.Repeat(head, 40), "state": "completed", "exit_code": 0, "outcome_unknown": false, "test_command": test, "clean": true, "diff_sha256": strings.Repeat("d", 64), "status_sha256": strings.Repeat("e", 64), "untracked_sha256": strings.Repeat("f", 64), "evidence_sha256": strings.Repeat("a", 64)}
}

// Register an actual separate fixture identity. A second slot or a changed
// string in a result must not masquerade as an independently claimed attempt.
func independentReviewWorker(t *testing.T, f *fixture) (string, string) {
	t.Helper()
	id := "review-test-" + newID()
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": id, "profiles": []any{}, "deployment": false}), 201)
	return id, worker["token"].(string)
}

func TestReviewRequiresIndependentWorkerForCurrentCandidate(t *testing.T) {
	for _, independent := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-worker-rejected", true: "separate-worker-accepted"}[independent], func(t *testing.T) {
			f := newFixture(t)
			task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Review the exact independently tested candidate"}), 201)
			workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Explicit producer and fixed candidate test", "nodes": []any{
				map[string]any{"id": "code", "type": "command", "command": []string{"edit"}},
				map[string]any{"id": "test", "type": "test", "command": []string{"verify"}, "depends_on": []string{"code"}, "config": map[string]any{"candidate_from": "code"}},
			}}), 201)
			run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": task["id"]}), 201)
			producerID, producerToken := independentReviewWorker(t, f)
			testID, testToken := producerID, producerToken
			if independent {
				testID, testToken = independentReviewWorker(t, f)
			}
			for index := 0; index < 2; index++ {
				worker, token := producerID, producerToken
				if index == 1 {
					worker, token = testID, testToken
				}
				wire := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": worker, "capabilities": toolFixtureCapabilities(t, f)}), 200)
				var assignment dag.Assignment
				if json.Unmarshal([]byte(stringify(wire["assignment"])), &assignment) != nil || assignment.RunID != run["id"] || assignment.WorkerID != worker {
					t.Fatal("original worker did not claim its actual current assignment")
				}
				proof := reviewWorkspaceEvidence("c", assignment.Type == "test")
				if assignment.Type == "test" {
					if assignment.CandidateFrom != "code" || assignment.CandidateCommit == "" {
						t.Fatal("test assignment did not receive the scheduler's candidate pin")
					}
					proof["base_commit"], proof["head_commit"] = assignment.CandidateCommit, assignment.CandidateCommit
					proof["candidate_from"], proof["candidate_attempt"], proof["candidate_fence"], proof["candidate_evidence_sha256"] = assignment.CandidateFrom, assignment.CandidateAttempt, assignment.CandidateFence, assignment.CandidateEvidenceSHA256
				}
				result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}
				f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
			}
			current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
			want := 400
			if independent {
				want = 200
			}
			f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "Explicit fixture review of the current candidate pair, not vendor execution"}), want)
		})
	}
}

func TestWorkspaceMetadataCannotReplaceExplicitTestNode(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Command is not a test"}), 201)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Command only", "nodes": []any{map[string]any{"id": "code", "type": "command", "command": []string{"edit"}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": task["id"]}), 201)
	a, err := f.e.Claim(context.Background(), f.user.TenantID, f.project, "worker", toolFixtureCapabilities(t, f))
	if err != nil || a == nil {
		t.Fatal(err)
	}
	if err = f.e.Complete(context.Background(), f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "command-done", WorkspaceEvidence: reviewWorkspaceEvidence("c", true)}); err != nil {
		t.Fatal(err)
	}
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "Metadata cannot replace selected test node"}), 400)
}

// Regression: a test of the administrator's old base must not certify the
// different candidate produced by a preceding isolated coding attempt.
func TestReviewRejectsWorkspaceCandidateNotCoveredByTests(t *testing.T) {
	for _, scenario := range []string{"different-candidate", "not-admin-test"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Bind actual test source"}), 201)
			wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Candidate then tests", "nodes": []any{map[string]any{"id": "code", "type": "command", "command": []string{"edit"}}, map[string]any{"id": "test", "type": "test", "command": []string{"verify"}, "depends_on": []string{"code"}}}}), 201)
			run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": task["id"]}), 201)
			for i := 0; i < 2; i++ {
				a, err := f.e.Claim(context.Background(), f.user.TenantID, f.project, "worker", toolFixtureCapabilities(t, f))
				if err != nil || a == nil {
					t.Fatal(err)
				}
				head := "c"
				isTest := i == 1
				if scenario == "different-candidate" && i == 1 {
					head = "b"
				}
				if scenario == "not-admin-test" {
					isTest = false
				}
				if err = f.e.Complete(context.Background(), f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: scenario + a.NodeID, WorkspaceEvidence: reviewWorkspaceEvidence(head, isTest)}); err != nil {
					t.Fatal(err)
				}
			}
			current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
			f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "Do not certify untested candidate"}), 400)
		})
	}
}
