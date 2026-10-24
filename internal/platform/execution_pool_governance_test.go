package platform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPoolHTTPCrossOrganizationPrivacyAndProjectRunnerChoices(t *testing.T) {
	f := newFixture(t)
	ownerCookie := f.cookie
	pool := f.call("POST", "/execution-pools", poolCreateBody(), 201)
	poolID := pool["id"].(string)
	machine := f.call("POST", "/execution-pools/"+poolID+"/runners", map[string]any{"name": "跨组织共享执行身份", "max_slots": 2, "ttl_seconds": 3600, "idempotency_key": newID()}, 201)
	// The second organization and project are created through their real APIs.
	org := f.call("POST", "/organizations", map[string]any{"name": "项目委托方", "idempotency_key": newID()}, 201)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": org["id"], "idempotency_key": newID()}, 200)
	project := f.call("POST", "/projects", map[string]any{"name": "明确共享项目名", "idempotency_key": newID()}, 201)
	projectID := project["id"].(string)
	projectCookie := f.cookie
	request := map[string]any{"project_id": projectID, "pool_id": poolID, "project_weight": 2, "idempotency_key": newID()}
	grant := f.call("POST", "/project-execution/grants", request, 201)
	// Organization-local pool listings cannot disclose a foreign registry.
	if f.call("GET", "/execution-pools", nil, 200)["total"] != float64(0) {
		t.Fatal("foreign owned registry exposed")
	}
	f.call("GET", "/execution-pools/"+poolID+"/runners", nil, 404)
	if got := f.call("GET", "/project-execution/runners?project_id="+projectID, nil, 200); len(got["items"].([]any)) != 0 {
		t.Fatal("pending/unrouted project advertised machine authority")
	}
	f.cookie = ownerCookie
	inbox := f.call("GET", "/execution-pools/"+poolID+"/grants", nil, 200)["items"].([]any)[0].(map[string]any)
	if inbox["can_open_project"] != false || inbox["shared_project_name"] != "明确共享项目名" {
		t.Fatal("foreign project privacy/navigation boundary lost")
	}
	grant = f.call("POST", "/execution-pools/"+poolID+"/grants/"+grant["id"].(string)+"/commands", map[string]any{"action": "approve", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.cookie = projectCookie
	preview := f.call("POST", "/project-execution/route-previews", map[string]any{"project_id": projectID, "expected_version": 0, "route": map[string]any{"mode": "pool", "pool_id": poolID, "grant_id": grant["id"], "grant_version": 2}, "idempotency_key": newID()}, 201)
	f.call("POST", "/project-execution/route-previews/"+preview["id"].(string)+"/commit", map[string]any{"project_id": projectID, "expected_version": 1, "idempotency_key": newID()}, 200)
	choices := f.call("GET", "/project-execution/runners?project_id="+projectID, nil, 200)
	items := choices["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["worker_id"] != machine["runner"].(map[string]any)["worker_id"] {
		t.Fatal("delegated identity not selectable")
	}
	raw, _ := json.Marshal(choices)
	if strings.Contains(string(raw), machine["token"].(string)) || strings.Contains(string(raw), "token_hash") || strings.Contains(string(raw), "allowed_actions") {
		t.Fatal("project choices exposed owner machine administration")
	}
	// Archive is an explicit test-owned database state because project archival
	// has no public mutation yet. Never advertise it as a completed UI journey.
	if _, err := f.s.DB.Exec(`UPDATE suma_projects SET status='archived' WHERE id=?`, projectID); err != nil {
		t.Fatal(err)
	}
	session := f.bearer("GET", "/pool-runner/session?runner_id="+machine["runner"].(map[string]any)["id"].(string), machine["token"].(string), nil, 200)
	if len(session["scopes"].([]any)) != 0 || len(f.call("GET", "/project-execution/runners?project_id="+projectID, nil, 200)["items"].([]any)) != 0 {
		t.Fatal("archived project still advertised machine admission")
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_projects SET status='active' WHERE id=?`, projectID); err != nil {
		t.Fatal(err)
	}
	// Current owner pause is a visible admission reason in the exact node read.
	wf := f.call("POST", "/workflows", map[string]any{"project_id": projectID, "name": "共享执行诊断", "nodes": []any{map[string]any{"id": "inspect", "type": "command", "command": []string{"inspect"}}}, "edges": []any{}, "idempotency_key": newID()}, 201)
	run := f.call("POST", "/runs", map[string]any{"project_id": projectID, "workflow_id": wf["id"], "workflow_version": 1, "idempotency_key": newID()}, 201)
	f.cookie = ownerCookie
	f.call("POST", "/execution-pools/"+poolID+"/commands", map[string]any{"action": "pause", "expected_version": 1, "idempotency_key": newID()}, 200)
	f.cookie = projectCookie
	facts := f.call("GET", "/runs/"+run["id"].(string)+"/nodes/inspect/scheduling?project_id="+projectID, nil, 200)
	found := false
	for _, b := range facts["blockers"].([]any) {
		if b.(map[string]any)["code"] == "pool_paused" {
			found = true
		}
	}
	if !found || facts["status"] != "blocked" {
		t.Fatal("pool pause hidden by resource-only diagnostics")
	}
	if len(f.call("GET", "/project-execution/runners?project_id="+projectID, nil, 200)["items"].([]any)) != 0 {
		t.Fatal("paused pool advertised new tool execution")
	}
	f.call("POST", "/project-execution/grants/"+grant["id"].(string)+"/commands", map[string]any{"project_id": projectID, "action": "withdraw", "expected_version": 2, "idempotency_key": newID()}, 200)
	facts = f.call("GET", "/runs/"+run["id"].(string)+"/nodes/inspect/scheduling?project_id="+projectID, nil, 200)
	found = false
	for _, b := range facts["blockers"].([]any) {
		if b.(map[string]any)["code"] == "pool_grant_not_current" {
			found = true
		}
	}
	if !found {
		t.Fatal("withdrawn grant not shown on original run")
	}
}
