package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func dispatchOptionsRequest(f *fixture, request map[string]any) TaskDispatchOptionsRequest {
	return TaskDispatchOptionsRequest{ProjectID: f.project, TaskID: request["task_id"].(string), ExpectedTaskVersion: int(request["expected_task_version"].(float64)), WorkflowID: request["workflow_id"].(string), WorkflowVersion: request["workflow_version"].(int), AgentNodeID: request["agent_node_id"].(string)}
}

func dispatchOptionsCount(t *testing.T, f *fixture, table string) int {
	t.Helper()
	var n int
	if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTaskDispatchOptionsCurrentCompatibilityNoExecution(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	req := dispatchOptionsRequest(f, request)
	ctx := context.Background()
	before := map[string]int{}
	for _, table := range []string{"dag_runs", "suma_task_dispatch_plans", "suma_usage_attempts"} {
		before[table] = dispatchOptionsCount(t, f, table)
	}
	var original string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, req.WorkflowID, req.WorkflowVersion).Scan(&original); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	if err != nil || got.TaskVersion != 1 || got.Context.Requirement == nil || len(got.Items) != 1 || !got.Items[0].Eligible || got.Items[0].ReasonCode != "eligible" || got.RecommendedToolProfileID != request["tool_profile_id"] || got.Items[0].ModelState != "fixed_by_tool" || got.Items[0].CostState != "unknown" || got.ObservedAt == "" {
		t.Fatal(got, err)
	}
	for table, count := range before {
		if dispatchOptionsCount(t, f, table) != count {
			t.Fatal("read created execution or accounting records", table)
		}
	}
	var after string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, req.WorkflowID, req.WorkflowVersion).Scan(&after); err != nil || after != original {
		t.Fatal("compatibility read changed the published template", err)
	}
	req.ExpectedTaskVersion++
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	expertStatus(t, err, 409)
	req.ExpectedTaskVersion--
	req.ProjectID = newID()
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	expertStatus(t, err, 400)
}

func TestTaskDispatchOptionsPagingAndCurrentProfileChanges(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	req, ctx := dispatchOptionsRequest(f, request), context.Background()
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Unapproved alternative one", "Unapproved alternative two"} {
		// A display name is not another configuration. Each alternative has an
		// independent immutable identity under the existing duplicate guard.
		alternative := profile.Profile
		alternative.ProfileSHA256 = digest(name)
		_, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: profile.WorkerID, Name: name, Profile: alternative, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
	}
	req.Limit = 1
	seen := map[string]bool{}
	for {
		page, e := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
		if e != nil || page.Total != 3 || len(page.Items) != 1 || seen[page.Items[0].Tool.ID] {
			t.Fatal(page, e)
		}
		item := page.Items[0]
		seen[item.Tool.ID] = true
		if item.Tool.ID != profile.ID && (item.Eligible || item.ReasonCode != "tool_unavailable" || page.RecommendedToolProfileID != "") {
			t.Fatal("paused alternative was recommended", page)
		}
		if page.NextCursor == "" {
			break
		}
		req.Before = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
	req.Before = "forged"
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	if err == nil {
		t.Fatal("unsigned cursor accepted")
	}
	_, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	req.Before, req.Limit = "", 50
	current, err := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	if err != nil || current.RecommendedToolProfileID != "" {
		t.Fatal(current, err)
	}
	var preview TaskDispatchPreviewRequest
	if err = json.Unmarshal([]byte(stringify(request)), &preview); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, preview)
	expertStatus(t, err, 409)
}

func TestTaskDispatchOptionsRouteAndWorkflowExclusions(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	req, ctx := dispatchOptionsRequest(f, request), context.Background()
	flow, _, _ := taskDispatchBuildFixture()
	flow.ID = newID()
	flow.Version = 1
	flow.Nodes = []dag.Node{flow.Nodes[0]}
	flow.Edges = nil
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, flow); err != nil {
		t.Fatal(err)
	}
	req.WorkflowID = flow.ID
	got, err := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	if err != nil || len(got.Items) != 1 || got.Items[0].Eligible || got.Items[0].ReasonCode != "workflow_incompatible" || got.Items[0].Reason == "" {
		t.Fatal(got, err)
	}
	req.WorkflowID = request["workflow_id"].(string)
	_, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	got, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	if err != nil || got.ExecutionRoute.PoolID == "" || len(got.Items) != 1 || got.Items[0].Eligible || got.Items[0].ReasonCode != "route_incompatible" {
		t.Fatal(got, err)
	}
}

func TestTaskDispatchOptionsCursorBindsExactEngineeringBasis(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	req, ctx := dispatchOptionsRequest(f, request), context.Background()
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	alternative := profile.Profile
	alternative.ProfileSHA256 = digest("Second page")
	if _, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: profile.WorkerID, Name: "Second page", Profile: alternative, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	req.Limit = 1
	page, err := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req)
	if err != nil || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	req.Before = page.NextCursor
	if next, e := f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, req.TaskID, req); e != nil || len(next.Items) != 1 || next.Items[0].Tool.ID == page.Items[0].Tool.ID {
		t.Fatal("original scoped cursor failed", next, e)
	}
	other := f.call("POST", "/tasks", f.body(map[string]any{"title": "Another actual task"}), 201)
	changed := req
	changed.TaskID = other["id"].(string)
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, changed.TaskID, changed)
	expertStatus(t, err, 400)
	changed = req
	changed.AgentNodeID = "other-valid-node"
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, changed.TaskID, changed)
	expertStatus(t, err, 400)
	flow, _, _ := taskDispatchBuildFixture()
	flow.ID = newID()
	flow.Version = 1
	if _, err = f.e.Publish(ctx, f.user.TenantID, f.project, flow); err != nil {
		t.Fatal(err)
	}
	changed = req
	changed.WorkflowID = flow.ID
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, changed.TaskID, changed)
	expertStatus(t, err, 400)
	var originalFlow dag.Workflow
	var originalSpec string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE id=? AND version=?`, req.WorkflowID, req.WorkflowVersion).Scan(&originalSpec); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal([]byte(originalSpec), &originalFlow); err != nil {
		t.Fatal(err)
	}
	originalFlow.Version++
	if _, err = f.e.Publish(ctx, f.user.TenantID, f.project, originalFlow); err != nil {
		t.Fatal(err)
	}
	changed = req
	changed.WorkflowVersion = originalFlow.Version
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, changed.TaskID, changed)
	expertStatus(t, err, 400)
	updated := f.call("PATCH", "/tasks/"+req.TaskID, f.body(map[string]any{"expected_version": req.ExpectedTaskVersion, "title": "Updated engineering objective"}), 200)
	changed = req
	changed.ExpectedTaskVersion = int(updated["version"].(float64))
	_, err = f.s.GetTaskDispatchOptions(ctx, f.e, f.actor, f.project, changed.TaskID, changed)
	expertStatus(t, err, 400)
}
