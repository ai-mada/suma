package platform

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExpertTeamPlanHTTPRevisionsCommandsAndExactIngress(t *testing.T) {
	h := newExpertFixture(t, false)
	f := h.f
	query := "?project_id=" + f.project
	for _, suffix := range []string{"&limit=01", "&limit=1e0", "&limit=101", "&limit=1&limit=2", "&extra=1"} {
		f.call("GET", "/expert-team-plans"+query+suffix, nil, 400)
	}
	created := f.call("POST", "/expert-team-plans", h.request(), 201)
	id := created["id"].(string)
	base := "/expert-team-plans/" + id
	page := f.call("GET", "/expert-team-plans"+query+"&limit=1", nil, 200)
	if page["total"] != float64(1) {
		t.Fatal("project history not backed by the actual plan")
	}
	f.call("GET", base+"/revisions"+query+"&limit=1&limit=2", nil, 400)
	f.call("GET", base+"/revisions/01"+query, nil, 400)
	revision := f.call("GET", base+"/revisions/1"+query, nil, 200)
	if revision["can_start"] != false || len(revision["allowed_actions"].([]any)) != 0 {
		t.Fatal("immutable source offered live decisions")
	}
	command := ExpertTeamPlanCommandRequest{ProjectID: f.project, ExpectedVersion: 1, Action: "stop", Reason: "Stop unstarted work in the current fixed source", Acknowledge: true, IdempotencyKey: newID()}
	raw, _ := json.Marshal(command)
	for _, replacement := range []string{"1.0", "1e0", "null", "\"1\""} {
		mutated := strings.Replace(string(raw), `"expected_version":1`, `"expected_version":`+replacement, 1)
		if mutated == string(raw) {
			t.Fatal("numeric fixture unchanged")
		}
		f.call("POST", base+"/commands", json.RawMessage(mutated), 400)
	}
	for _, extra := range []string{`,"worker_id":"injected"}`, `,"items":null}`, `,"expected_configuration_version":null}`} {
		f.call("POST", base+"/commands", json.RawMessage(strings.TrimSuffix(string(raw), "}")+extra), 400)
	}
	stopped := f.call("POST", base+"/commands", command, 200)
	replay := f.call("POST", base+"/commands", command, 200)
	if stopped["state"] != "stopped" || replay["id"] != id || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("stop/replay created or replaced execution")
	}
	current := f.call("GET", base+query, nil, 200)
	if current["state"] != "stopped" || current["can_start"] != false {
		t.Fatal("terminal plan became executable")
	}
}

func TestExpertTeamPlanClosedMessagesDoNotTranslateFrozenSource(t *testing.T) {
	const waiting = "Expert team task is waiting for formal predecessor acceptance"
	plan := ExpertTeamPlan{Blockers: []string{waiting}, Items: []ExpertTeamPlanItem{{Blockers: []string{waiting}, Plan: TaskDispatchPlan{Reason: waiting, Context: TaskContext{Task: TaskContextRecord{Description: waiting}}}}}}
	for _, locale := range []string{"zh-CN", "en-US"} {
		w := httptest.NewRecorder()
		w.Header().Set("Content-Language", locale)
		writeJSON(w, 200, plan)
		var out ExpertTeamPlan
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("invalid localized projection")
		}
		if locale == "zh-CN" && (out.Blockers[0] == waiting || out.Items[0].Blockers[0] == waiting) {
			t.Fatal("closed waiting message not translated")
		}
		if out.Items[0].Plan.Reason != waiting || out.Items[0].Plan.Context.Task.Description != waiting || plan.Items[0].Blockers[0] != waiting {
			t.Fatal("immutable source or original read snapshot translated")
		}
	}
}
