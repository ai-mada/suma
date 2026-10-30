package platform

import "testing"

// Use the production HTTP router, not just SaveTestPlan. PUT must decode the
// exact closed body before domain authorization, source CAS and receipt replay.
func TestTestPlanHTTPDraftSaveUsesCurrentRoleAndOriginalReceipt(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "HTTP draft source", "acceptance_criteria": []string{"Reviewed check"}}), 201)
	plan := f.call("POST", "/test-plans", map[string]any{"project_id": f.project, "task_id": task["id"], "expected_task_version": 1, "expected_requirement_version": 0, "title": "HTTP draft", "description": "", "cases": []QualityTestCase{qualityCase()}, "idempotency_key": newID()}, 201)
	ownerCookie := f.cookie
	for i, role := range []string{"developer", "reviewer", "viewer"} {
		f.cookie = ownerCookie
		f.call("GET", "/auth/session", nil, 200)
		email := "http-plan-" + role + "@example.test"
		invitation := governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": role}})
		governanceAccept(f, invitation, email)
		body := map[string]any{"project_id": f.project, "expected_version": i + 1, "expected_task_version": 1, "expected_requirement_version": 0, "title": role + " draft", "description": "An explicit draft save, not publication.", "cases": []QualityTestCase{qualityCase()}, "idempotency_key": newID()}
		path := "/test-plans/" + plan["id"].(string)
		if role == "viewer" {
			f.call("PUT", path, body, 403)
			continue
		}
		saved := f.call("PUT", path, body, 200)
		replayed := f.call("PUT", path, body, 200)
		if saved["version"] != float64(i+2) || replayed["version"] != saved["version"] || saved["published_revision"] != float64(0) || saved["state"] != "draft" {
			t.Fatalf("save/replay must retain one unpublished revision for %s", role)
		}
		unknown := make(map[string]any, len(body)+1)
		for key, value := range body {
			unknown[key] = value
		}
		unknown["passed"] = true
		unknown["idempotency_key"] = newID()
		f.call("PUT", path, unknown, 400) // Closed DTO stays closed.
		f.call("PUT", path, map[string]any{"project_id": f.project}, 400)
	}
}
