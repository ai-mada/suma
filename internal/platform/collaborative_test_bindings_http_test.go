package platform

import (
	"encoding/json"
	"testing"
)

// Every collaborative transport must retain field presence and exact JSON
// numbers before typed decoding can erase null or normalize an exponent.
func TestCollaborativeTestBindingsHTTPPresenceAcrossThreeEntrances(t *testing.T) {
	f := newFixture(t)
	clone := func(v any) map[string]any {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			t.Fatal("fixture JSON")
		}
		return m
	}
	choice := DispatchRecommendationChoice{CandidateID: "candidate223", Reason: "Human selected candidate"}
	bases := []struct {
		route string
		body  map[string]any
		team  bool
	}{
		{"/task-discussion-development-plans", clone(TaskDiscussionDevelopmentCreateRequest{ProjectID: f.project}), false},
		{"/dispatch-recommendations/recommendation223/adopt", clone(DispatchRecommendationAdoptRequest{ProjectID: f.project, ExpectedVersion: 1, SourceSHA256: digest("source223"), Choice: choice, Reason: "Review exact selection", IdempotencyKey: newID()}), false},
		{"/expert-team-plans", map[string]any{"project_id": f.project, "items": []any{map[string]any{"task_id": "task223", "expected_task_version": 1, "role_key": "developer"}}, "idempotency_key": newID()}, true},
	}
	invalid := []any{nil, "not-array", []any{nil}, []any{map[string]any{"node_id": "test", "tool_profile_id": "tool223", "expected_tool_version": json.Number("1e0")}}, []any{map[string]any{"node_id": "test", "tool_profile_id": "tool223", "expected_tool_version": 1, "worker_id": "injected"}}}
	for _, base := range bases {
		for _, bad := range invalid {
			body := clone(base.body)
			target := body
			if base.team {
				target = body["items"].([]any)[0].(map[string]any)
			}
			target["test_bindings"] = bad
			f.call("POST", base.route, body, 400)
		}
		body := clone(base.body)
		target := body
		if base.team {
			target = body["items"].([]any)[0].(map[string]any)
		}
		target["TEST_BINDINGS"] = []any{}
		f.call("POST", base.route, body, 400)
	}
	for _, bad := range []any{nil, "not-array", []any{nil}} {
		body := clone(bases[2].body)
		body["items"] = bad
		f.call("POST", bases[2].route, body, 400)
	}
	for _, table := range []string{"suma_task_dispatch_plans", "dag_runs", "suma_expert_team_plans"} {
		var n int
		if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE tenant_id=? AND project_id=?", f.user.TenantID, f.project).Scan(&n); err != nil || n != 0 {
			t.Fatal("invalid wire wrote business facts", table, n, err)
		}
	}
}
