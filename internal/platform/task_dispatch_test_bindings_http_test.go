package platform

import (
	"encoding/json"
	"testing"
)

func TestTaskDispatchExplicitTestsHTTPRejectsNullAliasesAndPreservesFrozenSelection(t *testing.T) {
	f, request, profile := dispatchTestBindingsFixture(t)
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var original map[string]any
	if err = json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE tenant_id=? AND project_id=?", f.user.TenantID, f.project).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	beforePlans, beforeRuns := count("suma_task_dispatch_plans"), count("dag_runs")
	valid := map[string]any{"node_id": "test", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version}
	for _, bad := range []any{nil, "not-an-array", []any{nil}, []any{map[string]any{"node_id": nil, "tool_profile_id": profile.ID, "expected_tool_version": profile.Version}}, []any{map[string]any{"node_id": "test", "tool_profile_id": profile.ID}}, []any{map[string]any{"Node_ID": "test", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version}}, []any{map[string]any{"node_id": "test", "tool_profile_id": profile.ID, "expected_tool_version": nil}}, []any{map[string]any{"node_id": "test", "tool_profile_id": profile.ID, "expected_tool_version": json.Number("1e0")}}, []any{map[string]any{"node_id": "test", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version, "worker_id": "untrusted"}}} {
		body := make(map[string]any, len(original))
		for key, value := range original {
			body[key] = value
		}
		body["idempotency_key"], body["test_bindings"] = newID(), bad
		f.call("POST", "/task-dispatch-plans", body, 400)
	}
	body := make(map[string]any, len(original))
	for key, value := range original {
		body[key] = value
	}
	delete(body, "test_bindings")
	body["TEST_BINDINGS"], body["idempotency_key"] = []any{valid}, newID()
	f.call("POST", "/task-dispatch-plans", body, 400)
	if count("suma_task_dispatch_plans") != beforePlans || count("dag_runs") != beforeRuns {
		t.Fatal("invalid selections created plans or executions")
	}
	response := f.call("POST", "/task-dispatch-plans", original, 201)
	encoded, err := json.Marshal(response)
	var plan TaskDispatchPlan
	if err != nil || json.Unmarshal(encoded, &plan) != nil || len(plan.TestBindings) != 1 || plan.TestBindings[0].ToolProfileID != profile.ID || plan.TestBindings[0].ToolProfileVersion != profile.Version || plan.TestBindings[0].WorkerID != profile.WorkerID {
		t.Fatal("HTTP did not preserve the explicit frozen tester", err)
	}
	if count("suma_task_dispatch_plans") != beforePlans+1 || count("dag_runs") != beforeRuns {
		t.Fatal("preview altered execution state")
	}
}
