package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
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

func TestExpertTeamPlanHTTPFiltersExactTeamBeforePaging(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	first, second := h.preview(t), h.preview(t)
	other := f.call("POST", "/teams", map[string]any{"name": "Other expert team", "expected_version": 0, "idempotency_key": newID()}, 201)["id"].(string)
	if _, err := f.s.SaveTeamMember(ctx, f.actor, other, map[string]any{"resource_kind": "agent", "resource_id": h.resource.ID, "project_id": f.project, "role": "member", "expected_version": float64(0), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	config := f.call("POST", "/expert-teams/"+other, ExpertTeamSaveRequest{ProjectID: f.project, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()}, 200)
	config = f.call("POST", "/expert-teams/"+other+"/commands", f.body(map[string]any{"expected_version": config["version"], "action": "publish", "reason": "Review the second team's execution source"}), 200)
	request := h.request()
	request.TeamID, request.ExpectedConfigurationVersion = other, int(config["version"].(float64))
	foreign := f.call("POST", "/expert-team-plans", request, 201)
	owner := f.cookie
	const reviewerEmail = "expert-team-filter-reviewer@example.test"
	invitation := governanceInvite(f, reviewerEmail, "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
	governanceAccept(f, invitation, reviewerEmail)
	reviewer := f.cookie
	f.cookie = owner
	base := "/expert-team-plans?project_id=" + f.project
	filtered := base + "&team_id=" + h.team
	operations := expertCount(t, f, "suma_operations")
	page := f.call("GET", filtered+"&limit=1", nil, 200)
	if page["total"] != float64(2) || len(page["items"].([]any)) != 1 {
		t.Fatal("team restriction was not applied to count and page")
	}
	cursor := url.QueryEscape(page["next_cursor"].(string))
	next := f.call("GET", filtered+"&limit=1&before="+cursor, nil, 200)
	if next["total"] != float64(2) || len(next["items"].([]any)) != 1 || next["next_cursor"] != nil {
		t.Fatal("continuation did not end at the exact team boundary")
	}
	matched := map[string]bool{}
	for _, result := range []map[string]any{page, next} {
		item := result["items"].([]any)[0].(map[string]any)
		if item["team_id"] != h.team {
			t.Fatal("another team appeared in the filtered window")
		}
		matched[item["id"].(string)] = true
	}
	if !matched[first.ID] || !matched[second.ID] {
		t.Fatal("filtered paging did not return both distinct plans")
	}
	otherPage := f.call("GET", base+"&team_id="+other, nil, 200)
	if otherPage["total"] != float64(1) || otherPage["items"].([]any)[0].(map[string]any)["id"] != foreign["id"] {
		t.Fatal("other team did not return its own plan")
	}
	if differentCase := strings.ToUpper(h.team); differentCase != h.team {
		if empty := f.call("GET", base+"&team_id="+differentCase, nil, 200); empty["total"] != float64(0) || len(empty["items"].([]any)) != 0 {
			t.Fatal("case variant selected a team")
		}
	}
	f.call("GET", base+"&team_id="+other+"&before="+cursor, nil, 400)
	f.call("GET", base+"&before="+cursor, nil, 400)
	unfiltered := f.call("GET", base+"&limit=1", nil, 200)
	oldCursor := url.QueryEscape(unfiltered["next_cursor"].(string))
	if old := f.call("GET", base+"&before="+oldCursor, nil, 200); old["total"] != float64(3) || len(old["items"].([]any)) != 2 {
		t.Fatal("unfiltered continuation changed")
	}
	f.call("GET", filtered+"&before="+oldCursor, nil, 400)
	for _, invalid := range []string{"", "other%2Fteam", "team.id", url.QueryEscape("团队"), h.team + "&team_id=" + other} {
		f.call("GET", base+"&team_id="+invalid, nil, 400)
	}
	f.call("GET", "/expert-teams?project_id="+f.project+"&team_id="+h.team, nil, 400)
	f.call("GET", "/expert-team-plans/"+first.ID+"/revisions?project_id="+f.project+"&team_id="+h.team, nil, 400)
	f.call("GET", "/expert-team-plans?project_id="+newID()+"&team_id="+h.team, nil, 404)
	f.cookie = reviewer
	f.call("GET", filtered, nil, 200)
	f.call("GET", filtered+"&before="+cursor, nil, 400)
	if expertCount(t, f, "suma_operations") != operations || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("read-only team listing changed operations or execution")
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
