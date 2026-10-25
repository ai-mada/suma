package platform

import (
	"bytes"
	"context"
	"github.com/ai-mada/suma/internal/dag"
	"net/http/httptest"
	"testing"
)

func TestSchedulingRawFractionCannotRoundIntoSafeInteger(t *testing.T) {
	f := newFixture(t)
	raw := `{"project_id":"` + f.project + `","expected_version":0,"policy":{"concurrency_limit":9007199254740991.1,"resource_limits":{},"budget_limit":null},"idempotency_key":"raw-fraction-policy"}`
	req := httptest.NewRequest("POST", "http://suma.test/api/v1/scheduling/policy-previews", bytes.NewBufferString(raw))
	req.AddCookie(f.cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Suma-Expected-User", f.view.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("raw fractional value was rounded into accepted capacity: %d %s", w.Code, w.Body.String())
	}
}

func TestSchedulingPolicyPreviewCommitAndReceipt(t *testing.T) {
	f := newFixture(t)
	policy := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	if policy["concurrency_limit"] != float64(1) || policy["unit"] != "admission_units" {
		t.Fatalf("default execution quota is not explicit: %#v", policy)
	}
	previewRequest := f.body(map[string]any{"expected_version": policy["version"], "policy": map[string]any{"concurrency_limit": 0, "resource_limits": map[string]any{"gpu_slots": 0}, "budget_limit": 2}})
	preview := f.call("POST", "/scheduling/policy-previews", previewRequest, 201)
	if preview["state"] != "ready" || preview["valid"] != true {
		t.Fatalf("preview cannot be reviewed: %#v", preview)
	}
	unchanged := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	if unchanged["concurrency_limit"] != float64(1) || unchanged["version"] != policy["version"] {
		t.Fatal("preview applied quota before confirmation")
	}
	path := "/scheduling/policy-previews/" + preview["id"].(string) + "/commit"
	commit := f.body(map[string]any{"expected_version": 1})
	applied := f.call("POST", path, commit, 200)
	if applied["concurrency_limit"] != float64(0) || applied["budget_limit"] != float64(2) {
		t.Fatalf("confirmed policy not applied: %#v", applied)
	}
	replayed := f.call("POST", path, commit, 200)
	if replayed["version"] != applied["version"] {
		t.Fatal("same commit operation mutated policy twice")
	}
	f.call("POST", path, f.body(map[string]any{"expected_version": 1}), 409)
	read := f.call("GET", "/scheduling/policy-previews/"+preview["id"].(string)+"?project_id="+f.project, nil, 200)
	if read["state"] != "committed" || read["applied_policy_version"] != applied["version"] {
		t.Fatal("refresh lost the applied policy revision")
	}
}

func TestSchedulingReadScopeCannotLeakOtherProject(t *testing.T) {
	f := newFixture(t)
	f.call("GET", "/scheduling/policy?project_id=foreign-project", nil, 404)
	f.call("GET", "/scheduling/queue?project_id=foreign-project", nil, 404)
	f.call("GET", "/runs/foreign-run/nodes/foreign-node/scheduling?project_id="+f.project, nil, 404)
}

func schedulingPreview(f *fixture, slots int, budget any) map[string]any {
	f.t.Helper()
	policy := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	return f.call("POST", "/scheduling/policy-previews", f.body(map[string]any{"expected_version": policy["version"], "policy": map[string]any{"concurrency_limit": slots, "resource_limits": map[string]any{}, "budget_limit": budget}}), 201)
}

func TestSchedulingPreviewExpiryAndStalePolicyAreAtomic(t *testing.T) {
	f := newFixture(t)
	first := schedulingPreview(f, 0, 2)
	second := schedulingPreview(f, 3, nil)
	f.call("POST", "/scheduling/policy-previews/"+second["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 200)
	f.call("POST", "/scheduling/policy-previews/"+first["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 409)
	stale := f.call("GET", "/scheduling/policy-previews/"+first["id"].(string)+"?project_id="+f.project, nil, 200)
	if stale["valid"] != false || stale["reason"] == "" {
		t.Fatal("stale preview offered confirmation")
	}
	expired := schedulingPreview(f, 0, 0)
	// The payload is immutable in normal operation. Advance only its stored
	// deadline here to exercise expiry without a blocking wall-clock sleep.
	_, err := f.s.DB.Exec(`UPDATE suma_scheduling_previews SET payload=json_set(payload,'$.expires_at','2000-01-01T00:00:00Z') WHERE id=?`, expired["id"])
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/scheduling/policy-previews/"+expired["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 409)
	current := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	if current["concurrency_limit"] != float64(3) || current["budget_limit"] != nil {
		t.Fatal("rejected preview partially changed policy")
	}
}

func TestSchedulingPolicyRequiresCompleteSafeExplicitInput(t *testing.T) {
	f := newFixture(t)
	for name, policy := range map[string]map[string]any{
		"missing_slots":      {"resource_limits": map[string]any{}, "budget_limit": nil},
		"null_slots":         {"concurrency_limit": nil, "resource_limits": map[string]any{}, "budget_limit": nil},
		"missing_budget":     {"concurrency_limit": 1, "resource_limits": map[string]any{}},
		"fraction":           {"concurrency_limit": 1.5, "resource_limits": map[string]any{}, "budget_limit": nil},
		"unsafe":             {"concurrency_limit": 9007199254740992, "resource_limits": map[string]any{}, "budget_limit": nil},
		"duplicate_resource": {"concurrency_limit": 1, "resource_limits": map[string]any{"execution_slots": 2}, "budget_limit": nil},
		"negative":           {"concurrency_limit": 1, "resource_limits": map[string]any{"gpu_slots": -1}, "budget_limit": nil},
	} {
		t.Run(name, func(t *testing.T) {
			f.call("POST", "/scheduling/policy-previews", f.body(map[string]any{"expected_version": 0, "policy": policy}), 400)
		})
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_scheduling_previews`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid input persisted preview: %d %v", count, err)
	}
}

func TestSchedulingCurrentApprovalAuthorityPrecedesReceipt(t *testing.T) {
	f := newFixture(t)
	owner := f.cookie
	inv := governanceInvite(f, "scheduler-reviewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
	accepted := governanceAccept(f, inv, "scheduler-reviewer@example.test")
	memberID := accepted["user"].(map[string]any)["id"].(string)
	reviewer := f.cookie
	preview := schedulingPreview(f, 2, nil)
	body := f.body(map[string]any{"expected_version": 1})
	path := "/scheduling/policy-previews/" + preview["id"].(string) + "/commit"
	f.call("POST", path, body, 200)
	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": memberID, "role": "developer", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.cookie = reviewer
	f.call("GET", "/auth/session", nil, 200)
	f.call("POST", path, body, 403)
	policy := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	if len(policy["allowed_actions"].([]any)) != 0 {
		t.Fatal("revoked approval still advertised mutation")
	}
}

func TestSchedulingZeroPolicyBlocksActualAttemptAndBudgetThenAllows(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	preview := schedulingPreview(f, 0, 3)
	f.call("POST", "/scheduling/policy-previews/"+preview["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 200)
	wf := dag.Workflow{ID: "quota-flow", Name: "准入验证", Version: 1, Nodes: []dag.Node{{ID: "work", Name: "执行验证", Type: "command", Command: []string{"inspect"}}}}
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, wf); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, wf.ID, "quota-first-run")
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "quota-worker", []string{"command:inspect"})
	if err != nil || a != nil {
		t.Fatalf("zero quota dispatched: %#v %v", a, err)
	}
	queue := f.call("GET", "/scheduling/queue?project_id="+f.project, nil, 200)
	if queue["total"] != float64(1) {
		t.Fatal("blocked work disappeared from queue")
	}
	facts := f.call("GET", "/runs/"+run.ID+"/nodes/work/scheduling?project_id="+f.project, nil, 200)
	if facts["attempt"] != float64(0) || len(facts["blockers"].([]any)) == 0 {
		t.Fatal("blocker consumed attempt or missing explanation")
	}
	before := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	if before["budget_spent"] != float64(0) {
		t.Fatal("rejected attempt spent budget")
	}
	preview = schedulingPreview(f, 1, 3)
	f.call("POST", "/scheduling/policy-previews/"+preview["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 200)
	a, err = f.e.Claim(ctx, f.user.TenantID, f.project, "quota-worker", []string{"command:inspect"})
	if err != nil || a == nil || a.Attempt != 1 {
		t.Fatalf("raised quota did not admit actual first attempt: %#v %v", a, err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "quota-complete-1", ExitCode: 0}); err != nil {
		t.Fatal(err)
	}
	history := f.call("GET", "/runs/"+run.ID+"/nodes/work/attempts?project_id="+f.project, nil, 200)
	if history["total"] != float64(1) {
		t.Fatal("attempt history is not durable")
	}
	exact := f.call("GET", "/runs/"+run.ID+"/nodes/work/attempts/1?project_id="+f.project, nil, 200)
	if exact["state"] != "succeeded" || exact["result"] == nil {
		t.Fatalf("precise attempt missing completion evidence: %#v", exact)
	}
	after := f.call("GET", "/scheduling/policy?project_id="+f.project, nil, 200)
	if after["budget_spent"] != float64(1) {
		t.Fatal("default admission unit not recorded")
	}
}
