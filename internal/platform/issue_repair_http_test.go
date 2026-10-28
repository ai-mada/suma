package platform

import (
	"context"
	"encoding/json"
	"maps"
	"testing"
)

// Exercise the public boundary, not merely the service methods: arbitrary
// record/status input and malformed scope must fail before preparing a task.
func TestIssueRepairHTTPClosedInputAndExactOriginalOperation(t *testing.T) {
	f := newFixture(t)
	issue, err := f.s.Create(context.Background(), f.actor, f.project, "issues", f.body(map[string]any{"title": "Report a real failure"}))
	if err != nil {
		t.Fatal(err)
	}
	path := "/issues/" + issue.ID + "/repair"
	f.call("GET", path+"?project_id="+f.project+"&project_id="+f.project, nil, 400)
	f.call("GET", path+"?project_id="+f.project+"&unknown=1", nil, 400)
	view := f.call("GET", path+"?project_id="+f.project, nil, 200)
	if view["can_prepare"] != true || view["current"] != nil {
		t.Fatal("fresh issue is not an explicit empty cycle", view)
	}
	body := f.body(map[string]any{"expected_issue_version": issue.Version, "title": "Repair the report", "description": "Preserve the original source", "owner_id": "", "acceptance_criteria": []string{"Regression is independently verified"}, "reason": "Explicit preparation"})
	bad := maps.Clone(body)
	bad["acceptance_criteria"] = nil
	f.call("POST", path+"/prepare", bad, 400)
	bad = maps.Clone(body)
	bad["status"] = "closed"
	f.call("POST", path+"/prepare", bad, 400)
	bad = maps.Clone(body)
	bad["Title"] = "A shadow input cannot replace the reviewed title"
	f.call("POST", path+"/prepare", bad, 400)
	created := f.call("POST", path+"/prepare", body, 201)
	replayed := f.call("POST", path+"/prepare", body, 201)
	var first, again IssueRepairView
	if json.Unmarshal([]byte(stringify(created)), &first) != nil || json.Unmarshal([]byte(stringify(replayed)), &again) != nil || first.Current == nil || again.Current == nil || first.Current.ID != again.Current.ID || first.Task.ID != again.Task.ID {
		t.Fatal("public receipt created a second repair task")
	}
	f.call("PATCH", "/issues/"+issue.ID, f.body(map[string]any{"expected_version": first.Issue.Version, "status": "closed"}), 400)
}

// The plan picker must filter before count and paging. A different task's
// cursor cannot silently change or broaden the currently selected repair.
func TestIssueRepairTaskScopedRegressionPlanPicker(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f := h.f
	other, err := f.s.Create(context.Background(), f.actor, f.project, "tasks", f.body(map[string]any{"title": "Another task"}))
	if err != nil {
		t.Fatal(err)
	}
	base := "/test-plans?project_id=" + f.project + "&task_id="
	page := f.call("GET", base+h.task.ID, nil, 200)
	if page["total"] != float64(1) || len(page["items"].([]any)) != 1 {
		t.Fatal("scoped plan was omitted", page)
	}
	page = f.call("GET", base+other.ID, nil, 200)
	if page["total"] != float64(0) || len(page["items"].([]any)) != 0 {
		t.Fatal("another task's plan leaked into repair picker", page)
	}
	f.call("GET", base+other.ID+"&before="+h.plan.ID, nil, 400)
	f.call("GET", base+"missing-task", nil, 404)
}
