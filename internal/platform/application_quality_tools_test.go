package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/buildinfo"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A real SDK client must see the same authorized compact facts as HTTP; a
// process success must not turn a known failed test case into acceptance.
func TestApplicationQualityRealMCPDeliveryAndCurrentScope(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f, ctx := h.f, context.Background()
	a := h.run(t)
	xml := `<testsuite tests="1" failures="1"><testcase classname="delivery" name="contract"><failure message="private-case-diagnostic">private-report-body</failure></testcase></testsuite>`
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(a, xml)}), 200)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	api, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	agent, runner := applicationActor(t, f, "agent"), applicationActor(t, f, "runner")
	server := httptest.NewServer((&Server{Service: f.s}).NewMCPHandler())
	t.Cleanup(server.Close)
	client := mcpTestClient(t, server.URL, f.project, token, f.user, "2025-11-25")
	initialized := client.InitializeResult()
	if initialized == nil || initialized.ServerInfo == nil || initialized.ServerInfo.Version != buildinfo.Version {
		t.Fatalf("MCP server identity differs from current product: %+v", initialized)
	}
	list, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tool := range list.Tools {
		if tool.Name == "suma_project_task_delivery" || tool.Name == "suma_project_quality_execution" {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				t.Fatal("quality read gained a write effect", tool.Name)
			}
			seen[tool.Name] = true
		}
	}
	if len(seen) != 2 {
		t.Fatal("shared MCP catalogue is missing compact delivery and quality reads", seen)
	}
	before := map[string]int{}
	for _, table := range []string{"dag_runs", "suma_operations", "suma_activity", "suma_issues", "suma_reviews"} {
		before[table] = domainACount(t, f.s, table)
	}
	delivery, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_task_delivery", Arguments: map[string]any{"task_id": h.task.ID}})
	if err != nil || delivery.IsError {
		t.Fatalf("MCP delivery read: %+v %v", delivery, err)
	}
	d := mcpTestResult(t, delivery)
	if d["task_id"] != h.task.ID || d["test_plan_id"] != h.plan.ID || d["formal_acceptance"] != false || d["run"].(map[string]any)["run_id"] != a.RunID {
		t.Fatal("delivery differs from original task/run", d)
	}
	quality, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_quality_execution", Arguments: map[string]any{"task_id": h.task.ID, "run_id": a.RunID}})
	if err != nil || quality.IsError {
		t.Fatalf("MCP quality read: %+v %v", quality, err)
	}
	q := mcpTestResult(t, quality)
	if q["state"] != "failed" || q["run_state"] != "succeeded" || q["failed"] != float64(1) || q["basis_current"] != true || q["plan_sha256"] != h.plan.Published.SHA256 || q["source_commit_total"] != float64(1) || q["source_commits_complete"] != true {
		t.Fatal("failed case was not preserved independently of process success", q)
	}
	raw, _ := json.Marshal(q)
	for _, private := range []string{"private-case-diagnostic", "private-report-body", "<testsuite", `"xml"`, `"message"`, `"command"`, `"environment"`} {
		if strings.Contains(string(raw), private) {
			t.Fatal("compact quality read exposed non-reference content", private)
		}
	}
	for _, current := range []Actor{agent, runner, f.actor} {
		for _, tool := range []string{"project.task_delivery", "project.quality_execution"} {
			if _, err = applicationCall(t, f, current, tool, `{"task_id":"`+h.task.ID+`","run_id":"`+a.RunID+`"}`); applicationStatus(err) != 403 {
				t.Fatal("non-API identity gained quality read", tool, err)
			}
		}
	}
	if _, err = f.s.InvokeApplicationTool(ctx, api, ApplicationToolCall{ProjectID: "foreign-project", Tool: "project.task_delivery", Arguments: json.RawMessage(`{"task_id":"` + h.task.ID + `"}`)}); applicationStatus(err) != 403 {
		t.Fatal("transport project overridden", err)
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("read created domain effects", table)
		}
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=?`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err = client.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_task_delivery", Arguments: map[string]any{"task_id": h.task.ID}}); err == nil {
		t.Fatal("MCP initialization survived current credential revocation")
	}
}

// Paging follows the fixed published case order, not the live draft, and the
// compact model must retain domain ambiguity while bounding evidence fan-out.
func TestApplicationQualityExecutionPagingOriginalFactsAndIsolation(t *testing.T) {
	h := newQualityPlanFixture(t, false)
	f, ctx := h.f, context.Background()
	second := qualityCase()
	second.Key, second.Title, second.Selector.Name = "second", "Second fixed case", "second"
	draft, err := f.s.SaveTestPlan(ctx, f.actor, f.project, h.plan.ID, TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, ExpectedTaskVersion: h.task.Version, Title: "Two ordered cases", Cases: []QualityTestCase{qualityCase(), second}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.plan, err = f.s.CommandTestPlan(ctx, f.actor, f.project, h.plan.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", Reason: "Explicit ordered-case fixture publication", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	nodes := []any{}
	for _, id := range []string{"verify-a", "verify-b", "verify-c", "verify-d"} {
		nodes = append(nodes, map[string]any{"id": id, "type": "test", "command": []string{h.profile.Profile.CommandID}, "config": map[string]any{"tool_profile_id": h.profile.ID, "tool_profile_sha256": h.profile.Profile.ProfileSHA256}})
	}
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Four exact report sources", "nodes": nodes}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": h.task.ID}), 201)
	runID := run["id"].(string)
	for range nodes {
		claimed := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
		var a dag.Assignment
		if json.Unmarshal([]byte(stringify(claimed["assignment"])), &a) != nil || a.RunID != runID {
			t.Fatal("missing fixed report assignment", claimed)
		}
		xml := `<testsuite tests="2"><testcase classname="delivery" name="contract"/><testcase classname="delivery" name="second"/></testsuite>`
		f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(a, xml)}), 200)
	}
	// A newer reversed draft must not change the frozen run's page order.
	currentTask := f.call("GET", "/tasks/"+h.task.ID+"?project_id="+f.project, nil, 200)
	if _, err = f.s.SaveTestPlan(ctx, f.actor, f.project, h.plan.ID, TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, ExpectedTaskVersion: int(currentTask["version"].(float64)), Title: "Reversed unadopted draft", Cases: []QualityTestCase{second, qualityCase()}, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	api := applicationActor(t, f, "api")
	input := `{"task_id":"` + h.task.ID + `","run_id":"` + runID + `","limit":1}`
	first, err := applicationCall(t, f, api, "project.quality_execution", input)
	if err != nil {
		t.Fatal("quality page missing", err)
	}
	page := first.(ApplicationQualityExecutionPage)
	original, err := f.s.GetTestPlanExecution(ctx, api, f.project, h.plan.ID, runID)
	if err != nil || page.State != original.State || page.RunState != original.RunState || page.BasisCurrent != original.BasisCurrent || page.Total != original.Total || page.Passed != original.Passed || page.Revision != h.plan.PublishedRevision || page.SourceSHA256 != h.plan.Published.Source.SHA256 || page.Complete || page.NextCase != "contract" || len(page.Cases) != 1 || page.SourceCommitTotal != 1 || !page.SourceCommitsComplete {
		t.Fatal("compact page changed current domain facts or order", page, original, err)
	}
	c := page.Cases[0]
	if c.CaseKey != "contract" || c.Status != "ambiguous" || c.EvidenceTotal != 4 || c.EvidenceComplete || len(c.Evidence) != 3 || !reflect.DeepEqual(c.Covers, []int{1}) {
		t.Fatal("evidence cap concealed ambiguity or count", c)
	}
	next, err := applicationCall(t, f, api, "project.quality_execution", strings.TrimSuffix(input, "}")+`,"after_case":"contract"}`)
	if err != nil {
		t.Fatal(err)
	}
	last := next.(ApplicationQualityExecutionPage)
	if !last.Complete || last.NextCase != "" || len(last.Cases) != 1 || last.Cases[0].CaseKey != "second" || last.Total != 2 {
		t.Fatal("next page changed the fixed case order", last)
	}
	for _, suffix := range []string{`,"after_case":"missing"`, `,"after_case":""`, `,"limit":0`, `,"limit":21`, `,"limit":1.0`, `,"limit":1e0`, `,"project_id":"foreign"`, `,"status":"passed"`} {
		base := `{"task_id":"` + h.task.ID + `","run_id":"` + runID + `"`
		if _, err = applicationCall(t, f, api, "project.quality_execution", base+suffix+`}`); applicationStatus(err) != 422 {
			t.Fatal("invalid quality selector was accepted", suffix, err)
		}
	}
	other := f.call("POST", "/tasks", f.body(map[string]any{"title": "An unrelated task"}), 201)
	if _, err = applicationCall(t, f, api, "project.quality_execution", strings.Replace(input, h.task.ID, other["id"].(string), 1)); applicationStatus(err) != 404 {
		t.Fatal("foreign task reused another run/plan", err)
	}
	if _, err = applicationCall(t, f, api, "project.quality_execution", strings.Replace(input, runID, strings.Repeat("b", 32), 1)); applicationStatus(err) != 404 {
		t.Fatal("unknown run was inferred", err)
	}
	if domainACount(t, f.s, "suma_issues") != 0 || domainACount(t, f.s, "suma_reviews") != 0 {
		t.Fatal("quality read mutated issue/approval state")
	}
}
