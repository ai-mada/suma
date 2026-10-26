package platform

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Exercise the real transport, including API draft/dispatch versus browser
// publication, exact numeric ingress and the existing individual-start route.
func TestExpertTeamHTTPContractsAndAtomicDispatch(t *testing.T) {
	h := newExpertFixture(t, false)
	f := h.f
	base := "/expert-teams/" + h.team
	f.call("GET", "/expert-teams?project_id="+f.project+"&limit=1&limit=2", nil, 400)
	f.call("GET", base+"/revisions/02?project_id="+f.project, nil, 400)
	historical := f.call("GET", base+"/revisions/2?project_id="+f.project, nil, 200)
	if historical["can_edit"] != false || historical["can_publish"] != false {
		t.Fatal("historical snapshot offered current decisions", historical)
	}
	credential := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api", "label": "Expert team HTTP integration"}), 201)
	token, _ := credential["token"].(string)
	if token == "" {
		t.Fatal("missing scoped API credential")
	}
	callAPI := func(path string, body any, want int) map[string]any {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "http://suma.test/api/v1"+path, bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("expert API %s status %d, expected %d: %s", path, w.Code, want, w.Body.String())
		}
		var result map[string]any
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("invalid API JSON")
		}
		return result
	}
	save := ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: 2, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()}
	saved := callAPI(base, save, 200)
	if saved["state"] != "draft" || saved["version"] != float64(3) || saved["can_publish"] != false {
		t.Fatal("API draft gained publication authority", saved)
	}
	command := ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: 3, Action: "publish", Reason: "Review current role and execution basis", IdempotencyKey: newID()}
	callAPI(base+"/commands", command, 403)
	command.IdempotencyKey = newID()
	f.call("POST", base+"/commands", command, 200)
	create := h.request()
	create.ExpectedConfigurationVersion = 4
	raw, err := json.Marshal(create)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range [][2]string{{`"expected_configuration_version":4`, `"expected_configuration_version":4.0`}, {`"expected_task_version":1`, `"expected_task_version":1e0`}} {
		changed := strings.Replace(string(raw), mutation[0], mutation[1], 1)
		if changed == string(raw) {
			t.Fatal("numeric fixture did not change")
		}
		callAPI("/expert-team-plans", json.RawMessage(changed), 400)
	}
	callAPI("/expert-team-plans", json.RawMessage(strings.TrimSuffix(string(raw), "}")+`,"worker_id":"untrusted"}`), 400)
	created := callAPI("/expert-team-plans", create, 201)
	var plan ExpertTeamPlan
	encoded, _ := json.Marshal(created)
	if json.Unmarshal(encoded, &plan) != nil || len(plan.Items) != 2 || !plan.CanStart {
		t.Fatal("invalid fixed batch preview", created)
	}
	if expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("preview executed work")
	}
	first := plan.Items[0].Plan
	if first.ID != "" || first.RunID != "" {
		t.Fatal("preview exposed an unstarted dispatch/run")
	}
	start := h.start(plan)
	start.AcknowledgeDispatch = false
	callAPI("/expert-team-plans/"+plan.ID+"/start", start, 400)
	start.AcknowledgeDispatch = true
	started := callAPI("/expert-team-plans/"+plan.ID+"/start", start, 200)
	replayed := callAPI("/expert-team-plans/"+plan.ID+"/start", start, 200)
	if started["state"] != "active" || started["version"] != float64(2) || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("API dispatch did not bind exactly two independent runs", started)
	}
	var bound ExpertTeamPlan
	encodedBound, _ := json.Marshal(started)
	if json.Unmarshal(encodedBound, &bound) != nil {
		t.Fatal("invalid started plan")
	}
	first = bound.Items[0].Plan
	f.call("POST", "/task-dispatch-plans/"+first.ID+"/start", TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: first.Version, IdempotencyKey: newID()}, 409)
	a, _ := json.Marshal(started["items"])
	b, _ := json.Marshal(replayed["items"])
	if !bytes.Equal(a, b) {
		t.Fatal("API replay replaced original run identities")
	}
	f.call("GET", "/expert-team-plans/"+plan.ID+"?project_id="+f.project, nil, 200)
}
