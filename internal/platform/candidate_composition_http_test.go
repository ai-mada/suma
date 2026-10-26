package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
)

func TestCandidateCompositionHTTPExactIngressAndOriginalStart(t *testing.T) {
	h := newCompositionFixture(t)
	f := h.f
	location := "?project_id=" + f.project
	f.call("GET", "/candidate-compositions/options"+location+"&task_id="+h.req.TaskID, nil, 200)
	for _, suffix := range []string{"&project_id=" + f.project, "&limit=01", "&limit=1e0", "&limit=101", "&unknown=true"} {
		f.call("GET", "/candidate-compositions"+location+suffix, nil, 400)
	}
	for _, invalid := range []any{json.Number("1e0"), json.Number("1.0"), nil, "1"} {
		body := candidateJSON(t, h.req)
		body["expected_task_version"] = invalid
		f.call("POST", "/candidate-compositions", body, 400)
	}
	body := candidateJSON(t, h.req)
	body["sources"].([]any)[0].(map[string]any)["unknown"] = true
	f.call("POST", "/candidate-compositions", body, 400)
	created := f.call("POST", "/candidate-compositions", h.req, 201)
	if created["run_id"] != "" || created["state"] != "ready" {
		t.Fatal("preview dispatched work", created)
	}
	id := created["id"].(string)
	start := CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: 1, Acknowledge: true, IdempotencyKey: newID()}
	started := f.call("POST", "/candidate-compositions/"+id+"/start", start, 200)
	replayed := f.call("POST", "/candidate-compositions/"+id+"/start", start, 200)
	if started["run_id"] == "" || started["run_id"] != replayed["run_id"] {
		t.Fatal("start replay changed actual run", started, replayed)
	}
	f.call("GET", "/candidate-compositions/"+id+location, nil, 200)
	f.call("GET", "/tasks/"+h.req.TaskID+"/code-baseline"+location, nil, 200)
	// A zero binding version is a real first-bind CAS, not missing input. The
	// domain rejects this unaccepted source only after transport accepts zero.
	bind := TaskCodeBaselineBindRequest{ProjectID: f.project, ExpectedTaskVersion: h.req.ExpectedTaskVersion, ExpectedBindingVersion: 0, CompositionID: id, ExpectedCompositionVersion: 2, Acknowledge: true, Reason: "Explicit fixed source", IdempotencyKey: newID()}
	f.call("POST", "/tasks/"+h.req.TaskID+"/code-baseline", bind, 409)
	for _, invalid := range []any{nil, "0", json.Number("0.0"), json.Number("0e0")} {
		bad := candidateJSON(t, bind)
		bad["expected_binding_version"] = invalid
		f.call("POST", "/tasks/"+h.req.TaskID+"/code-baseline", bad, 400)
	}
	f.call("POST", "/tasks", f.body(map[string]any{"title": "Forged code baseline", "code_baseline": created}), 400)
	f.call("POST", "/tasks", f.body(map[string]any{"title": "Forged nested baseline", "data": map[string]any{"code_baseline": created}}), 400)
	for _, locale := range []string{"zh-CN", "en-US"} {
		req := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/candidate-compositions", strings.NewReader(`{"project_id":"`+f.project+`"}`))
		req.AddCookie(f.cookie)
		req.Header.Set("X-Suma-Language", locale)
		req.Header.Set("X-Suma-Expected-User", f.user.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		var response map[string]any
		if json.Unmarshal(w.Body.Bytes(), &response) != nil || w.Code != http.StatusBadRequest {
			t.Fatal(w.Code, w.Body.String())
		}
		diagnostic, ok := response["error"].(map[string]any)
		if !ok || diagnostic["message_key"] != "candidate_composition.invalid" || diagnostic["localized"] != true || diagnostic["locale"] != locale {
			t.Fatal("controlled error lost language identity", response)
		}
	}
}

func TestCandidateCompositionPrivateClaimAndAuthorizedSourceBytes(t *testing.T) {
	h := newCompositionFixture(t)
	f := h.f
	p := h.start(t, h.create(t))
	claimKey := newID()
	claim := map[string]any{"project_id": f.project, "worker_id": "composition-http-worker", "capabilities": []string{"command:suma-compose"}, "idempotency_key": claimKey}
	response := f.bearer("POST", "/runner/claim", h.token, claim, 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(response["assignment"])), &assignment) != nil || assignment.CandidateComposition == nil || assignment.CandidateComposition.PlanID != p.ID || assignment.CodeBaseline != nil {
		t.Fatal("claim did not decorate fixed composition", response)
	}
	again := f.bearer("POST", "/runner/claim", h.token, claim, 200)
	if stringify(again) != stringify(response) {
		t.Fatal("claim recovery changed its fixed inputs")
	}
	input := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(assignment), InputKind: "composition", BindingID: p.ID, TransferID: p.Sources[0].Source.TransferID}
	download := func(input ccmp.CodeInputRequest, expected int) []byte {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/runner/code-inputs/bundle", bytes.NewBufferString(stringify(input)))
		req.Header.Set("Authorization", "Bearer "+h.token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != expected {
			t.Fatalf("code input %d want %d: %s", w.Code, expected, w.Body.String())
		}
		return w.Body.Bytes()
	}
	if string(download(input, 200)) != "candidate bytes" {
		t.Fatal("download changed frozen bytes")
	}
	wrong := input
	wrong.TransferID = newID()
	download(wrong, 403)
	wrong = input
	wrong.BindingID = newID()
	download(wrong, 403)
	f.call("POST", "/runner/code-inputs/bundle", input, 403)
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, assignment.RunID, assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	download(input, 409)
}
