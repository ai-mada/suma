package platform

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
)

// Exercise the actual browser command boundary, including JSON's otherwise
// case-insensitive struct matching and the server-owned provenance namespace.
func TestCandidateRepairRevisionHTTPIngressAndHistory(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f := h.f
	repair, _, _ := compositionRepairRequestFixture(t, h, original)
	parent, err := f.s.CreateCandidateCompositionRepair(context.Background(), f.e, f.actor, f.project, original.ID, repair)
	if err != nil {
		t.Fatal(err)
	}
	path := "/candidate-compositions/" + parent.ID
	query := "?project_id=" + f.project
	view := f.call("GET", path+"/repair-revision-context"+query, nil, 200)
	if view["can_revise"] != true || view["eligibility_mode"] != "ready" {
		t.Fatal(view)
	}
	f.call("GET", path+"/repair-revision-context"+query+"&project_id="+f.project, nil, 400)
	f.call("GET", path+"/repair-revision-context"+query+"&unknown=1", nil, 400)
	f.call("GET", path+"/repair-revision-context?project_id=unowned", nil, 404)
	r := CandidateCompositionRepairRevisionRequest{ProjectID: f.project, ExpectedVersion: parent.Version, ExpectedTaskVersion: int(view["task_version"].(float64)), ExpectedConflictSHA256: view["conflict_basis"].(map[string]any)["sha256"].(string), WorkflowID: repair.WorkflowID, WorkflowVersion: repair.WorkflowVersion, ProducerNodeID: repair.ProducerNodeID, ToolProfileID: repair.ToolProfileID, ExpectedToolVersion: repair.ExpectedToolVersion, Reason: "Reviewed replacement before any execution", Acknowledge: true, IdempotencyKey: newID()}
	for _, field := range []string{"expected_version", "expected_task_version", "workflow_version", "expected_tool_version"} {
		for _, value := range []any{nil, "1", json.Number("1.0"), json.Number("1e0")} {
			bad := candidateJSON(t, r)
			bad[field] = value
			f.call("POST", path+"/repair-revisions", bad, 400)
		}
	}
	for _, field := range []string{"Project_ID", "task_id", "repair_revision", "repair_recovery", "startup_failure", "run_provenance"} {
		bad := candidateJSON(t, r)
		bad[field] = "forged"
		f.call("POST", path+"/repair-revisions", bad, 400)
	}
	bad := candidateJSON(t, r)
	bad["expected_recovery_sha256"] = nil
	f.call("POST", path+"/repair-revisions", bad, 400)
	created := f.call("POST", path+"/repair-revisions", r, 201)
	if created["task_id"] != parent.TaskID || created["id"] == parent.ID || created["run_id"] != "" {
		t.Fatal(created)
	}
	replay := f.call("POST", path+"/repair-revisions", r, 201)
	if stringify(replay) != stringify(created) {
		t.Fatal("original revision receipt changed")
	}
	f.call("POST", path+"/start", CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: parent.Version, Acknowledge: true, IdempotencyKey: newID()}, 409)
	childPath := "/candidate-compositions/" + created["id"].(string)
	started := f.call("POST", childPath+"/start", CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: int(created["version"].(float64)), Acknowledge: true, IdempotencyKey: newID()}, 200)
	historyPath := "/tasks/" + parent.TaskID + "/run-history" + query
	history := f.call("GET", historyPath+"&limit=1", nil, 200)
	items := history["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["run_id"] != started["run_id"] || items[0].(map[string]any)["current"] != true || items[0].(map[string]any)["binding_kind"] != "bound" {
		t.Fatal(history)
	}
	for _, suffix := range []string{"&limit=0", "&limit=101", "&limit=01", "&limit=1&limit=2", "&before=" + url.QueryEscape("forged-cursor")} {
		f.call("GET", historyPath+suffix, nil, 400)
	}
	for _, field := range []string{"repair_revision", "repair_recovery", "startup_failure", "run_provenance", "task_run_provenance"} {
		f.call("POST", "/tasks", f.body(map[string]any{"title": "forged provenance", "data": map[string]any{field: map[string]any{}}}), 400)
	}
}
