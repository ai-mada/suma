package platform

import (
	"bytes"
	"encoding/json"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
	"net/http"
	"net/http/httptest"
	"testing"
)

// This consumes the public ingress and worker transport. Git/process semantics
// are covered independently by the Runner's real multi-source regressions.
func TestCandidateConflictRepairHTTPIngressAndPrivateInputs(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f := h.f
	request, profile, token := compositionRepairRequestFixture(t, h, original)
	path := "/candidate-compositions/" + original.ID
	f.call("GET", path+"/conflict?project_id="+f.project, nil, 200)
	f.call("GET", path+"/conflict?project_id="+f.project+"&unknown=1", nil, 400)
	for _, name := range []string{"expected_composition_version", "workflow_version", "expected_tool_version"} {
		for _, value := range []any{nil, "1", json.Number("1e0"), json.Number("1.0")} {
			bad := candidateJSON(t, request)
			bad[name] = value
			f.call("POST", path+"/repairs", bad, 400)
		}
	}
	bad := candidateJSON(t, request)
	bad["repair"].(map[string]any)["conflict_basis"] = map[string]any{}
	f.call("POST", path+"/repairs", bad, 400)
	for _, name := range []string{"priority", "owner_id"} {
		bad := candidateJSON(t, request)
		bad["repair"].(map[string]any)[name] = nil
		f.call("POST", path+"/repairs", bad, 400)
	}
	created := f.call("POST", path+"/repairs", request, 201)
	id := created["id"].(string)
	if id == original.ID || created["task_id"] == original.TaskID || created["run_id"] != "" || created["state"] != "ready" {
		t.Fatal("repair rewrote original or executed during creation", created)
	}
	again := f.call("POST", path+"/repairs", request, 201)
	if again["id"] != id || again["task_id"] != created["task_id"] {
		t.Fatal("duplicate task/plan", created, again)
	}
	start := CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: 1, Acknowledge: true, IdempotencyKey: newID()}
	started := f.call("POST", "/candidate-compositions/"+id+"/start", start, 200)
	key := newID()
	claim := map[string]any{"project_id": f.project, "worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}, "idempotency_key": key}
	response := f.bearer("POST", "/runner/claim", token, claim, 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(response["assignment"])), &assignment) != nil || assignment.CandidateConflictRepair == nil || assignment.CandidateComposition != nil || assignment.CodeBaseline != nil || assignment.CandidateConflictRepair.PlanID != id || assignment.CandidateConflictRepair.CompositionID != original.ID || assignment.RunID != started["run_id"] {
		t.Fatal("repair claim lost protected typed source identity", response)
	}
	repeat := f.bearer("POST", "/runner/claim", token, claim, 200)
	if stringify(repeat) != stringify(response) {
		t.Fatal("claim replay changed frozen input")
	}
	input := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(assignment), InputKind: "conflict_repair", BindingID: id, TransferID: original.Sources[0].Source.TransferID}
	download := func(q ccmp.CodeInputRequest, status int) []byte {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/runner/code-inputs/bundle", bytes.NewBufferString(stringify(q)))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("code input %d want%d: %s", w.Code, status, w.Body.String())
		}
		return w.Body.Bytes()
	}
	if content := download(input, 200); digest(string(content)) != original.Sources[0].Source.BundleSHA256 {
		t.Fatal("frozen source bytes changed")
	}
	wrong := input
	wrong.BindingID = original.ID
	download(wrong, 403)
	wrong = input
	wrong.Assignment.Fence++
	download(wrong, 409)
	wrong = input
	wrong.TransferID = newID()
	download(wrong, 403)
	f.call("POST", "/tasks", f.body(map[string]any{"title": "forged repair input", "data": map[string]any{"candidate_conflict_repair": assignment.CandidateConflictRepair}}), 400)
}
