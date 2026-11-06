package platform

import (
	"context"
	"net/url"
	"testing"
)

func TestTestPlanCatalogFiltersStateAndScopesPagination(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	create := func(title string) TestPlanView {
		t.Helper()
		task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": title, "acceptance_criteria": []any{"Run the project check"}}))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := f.s.CreateTestPlan(ctx, f.actor, f.project, TestPlanCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Title: title, Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		return plan
	}
	command := func(plan TestPlanView, action string) TestPlanView {
		t.Helper()
		result, err := f.s.CommandTestPlan(ctx, f.actor, f.project, plan.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Action: action, Reason: "Explicit catalog fixture decision", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	check := func(page map[string]any, state string, total, size int) []any {
		t.Helper()
		items := page["items"].([]any)
		if page["total"] != float64(total) || len(items) != size {
			t.Fatalf("unexpected page for state %q: total=%v, items=%d", state, page["total"], len(items))
		}
		for _, value := range items {
			if state != "" && value.(map[string]any)["state"] != state {
				t.Fatalf("filter %q returned an item in state %v", state, value.(map[string]any)["state"])
			}
		}
		return items
	}
	snapshot := func() [4]int {
		t.Helper()
		var state [4]int
		err := f.s.DB.QueryRow("SELECT (SELECT COUNT(*) FROM suma_operations),(SELECT COUNT(*) FROM suma_activity),(SELECT COUNT(*) FROM suma_test_plan_revisions),(SELECT COALESCE(SUM(version),0) FROM suma_test_plans)").Scan(&state[0], &state[1], &state[2], &state[3])
		if err != nil {
			t.Fatal(err)
		}
		return state
	}

	firstDraft := create("Draft one")
	secondDraft := create("Draft two")
	published := command(create("Published"), "publish")
	retired := command(command(create("Retired"), "publish"), "retire")
	otherProject := f.call("POST", "/projects", map[string]any{"title": "Separate test plan project", "idempotency_key": newID()}, 201)["id"].(string)
	token, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}

	before := snapshot()
	path := "/test-plans?project_id=" + f.project
	first := f.call("GET", path+"&state=draft&limit=1", nil, 200)
	id := check(first, "draft", 2, 1)[0].(map[string]any)["id"].(string)
	cursor := first["next_cursor"].(string)
	if cursor == "" || cursor == id {
		t.Fatal("the filtered page did not return an opaque cursor")
	}
	nextPath := path + "&state=draft&limit=1&before=" + url.QueryEscape(cursor)
	next := f.call("GET", nextPath, nil, 200)
	nextID := check(next, "draft", 2, 1)[0].(map[string]any)["id"].(string)
	if nextID == id || next["next_cursor"] != "" {
		t.Fatal("filtered continuation repeated a plan or left a spurious cursor")
	}

	taskPath := path + "&task_id=" + published.TaskID
	if check(f.call("GET", taskPath+"&state=published", nil, 200), "published", 1, 1)[0].(map[string]any)["id"] != published.ID {
		t.Fatal("exact task and state did not intersect")
	}
	check(f.call("GET", taskPath+"&state=draft", nil, 200), "draft", 0, 0)
	if check(f.bearer("GET", path+"&state=retired", token.Token, nil, 200), "retired", 1, 1)[0].(map[string]any)["id"] != retired.ID {
		t.Fatal("API token did not see the authorized retired plan")
	}

	plain := f.call("GET", path+"&limit=1", nil, 200)
	plainID := check(plain, "", 4, 1)[0].(map[string]any)["id"].(string)
	if plain["next_cursor"] != plainID {
		t.Fatal("unfiltered continuation no longer uses the original plan ID")
	}
	unfilteredNext := f.call("GET", path+"&limit=1&before="+plainID, nil, 200)
	if check(unfilteredNext, "", 4, 1)[0].(map[string]any)["id"] == plainID {
		t.Fatal("unfiltered continuation repeated the first row")
	}
	for _, query := range []string{
		"&state=", "&state=all", "&state=draft&state=draft", "&state=Draft",
		"&before=" + url.QueryEscape(cursor),
		"&state=published&before=" + url.QueryEscape(cursor),
		"&state=draft&task_id=" + firstDraft.TaskID + "&before=" + url.QueryEscape(cursor),
		"&state=draft&before=" + plainID,
		"&state=draft&before=" + url.QueryEscape(cursor+"="),
	} {
		f.call("GET", path+query, nil, 400)
	}
	f.call("GET", "/test-plans/"+published.ID+"?project_id="+f.project+"&state=published", nil, 400)
	f.call("GET", "/test-plans?project_id="+otherProject+"&state=draft&before="+url.QueryEscape(cursor), nil, 400)
	f.bearer("GET", "/test-plans?project_id="+otherProject+"&state=draft&before="+url.QueryEscape(cursor), token.Token, nil, 403)
	if after := snapshot(); after != before {
		t.Fatalf("catalog reads changed persistent state: before %v, after %v", before, after)
	}

	anchor := firstDraft
	if id == secondDraft.ID {
		anchor = secondDraft
	}
	if anchor.ID != id {
		t.Fatal("filtered page returned an unexpected draft")
	}
	command(anchor, "publish")
	after := f.call("GET", nextPath, nil, 200)
	if check(after, "draft", 1, 1)[0].(map[string]any)["id"] != nextID {
		t.Fatal("state transition changed the original continuation position")
	}
}
