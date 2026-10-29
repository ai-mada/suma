package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestWorkspaceTaskContextAcceptedExactFactsAndReadOnly(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f, ctx := h.f, context.Background()
	before, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, h.task.ID)
	if err != nil || before.Quality.State != "not_run" || before.Quality.NotRun != 1 || before.Quality.Passed != 0 || before.Delivery.Run != nil {
		t.Fatal("publication without execution was not represented accurately", before, err)
	}
	a := h.run(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(a, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	current := f.call("GET", "/tasks/"+h.task.ID+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+h.task.ID+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": a.RunID, "decision": "accepted", "reason": "Explicit acceptance of the original published-plan evidence"}), 200)
	counts := map[string]int{}
	for _, table := range []string{"dag_runs", "suma_operations", "suma_reviews", "suma_activity", "suma_issues"} {
		counts[table] = domainACount(t, f.s, table)
	}
	out, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, h.task.ID)
	if err != nil || out.Task.Status != "accepted" || out.Task.Version != out.Delivery.TaskVersion || out.Task.RunID != a.RunID || !out.Delivery.FormalAcceptance || out.Delivery.Run == nil || out.Delivery.Run.RunID != a.RunID || out.Quality.PlanID != h.plan.ID || out.Quality.PlanSHA256 != h.plan.Published.SHA256 || out.Quality.RunID != a.RunID || out.Quality.State != "passed" || !out.Quality.BasisCurrent || out.Quality.Passed != 1 {
		t.Fatal("accepted task did not retain exact current task/run/quality facts", out, err)
	}
	if out.Requirement != nil || out.Owner != nil || out.Design.State != "none" || out.Design.Context != nil || out.Task.AcceptanceCriteria == nil {
		t.Fatal("absent facts were invented or arrays became null", out)
	}
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	// Token issuance is the test's explicit mutation, outside the read check.
	counts["suma_operations"] = domainACount(t, f.s, "suma_operations")
	counts["suma_activity"] = domainACount(t, f.s, "suma_activity")
	viaAPI, err := f.s.GetWorkspaceTaskContext(ctx, api, f.project, h.task.ID)
	if err != nil || stringify(viaAPI.Task) != stringify(out.Task) || stringify(viaAPI.Quality) != stringify(out.Quality) {
		t.Fatal("project API did not use the same read authority", viaAPI, err)
	}
	for table, count := range counts {
		if domainACount(t, f.s, table) != count {
			t.Fatal("reading task context changed business state", table)
		}
	}
	encoded := stringify(out)
	for _, private := range []string{`"data":`, `"xml":`, `"command":`, `"output":`, `"email":`, `"workspace_binding":`} {
		if strings.Contains(encoded, private) {
			t.Fatal("compact task context leaked a private execution or resource field", private)
		}
	}
	// A plan retired before any execution is a known governance fact, not an
	// absent plan. This uses the original publication/retirement commands.
	unstarted := newQualityPlanFixture(t, true)
	if _, err = unstarted.f.s.CommandTestPlan(ctx, unstarted.f.actor, unstarted.f.project, unstarted.plan.ID, TestPlanCommandRequest{ProjectID: unstarted.f.project, ExpectedVersion: unstarted.plan.Version, Action: "retire", Reason: "Retire the unused publication explicitly", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	retired, err := unstarted.f.s.GetWorkspaceTaskContext(ctx, unstarted.f.actor, unstarted.f.project, unstarted.task.ID)
	if err != nil || retired.Quality.State != "retired" || retired.Quality.PlanID != unstarted.plan.ID || retired.Quality.BasisCurrent || retired.Quality.RunID != "" || retired.Quality.Passed != 0 {
		t.Fatal("retired plan without an execution was misreported as absent", retired, err)
	}
}

func TestWorkspaceTaskContextStaleDesignAndCurrentAuthority(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f, ctx := h.f, context.Background()
	f.call("PATCH", "/tasks/"+h.task.ID, f.body(map[string]any{"expected_version": h.task.Version, "title": "Current task after its publication"}), 200)
	out, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, h.task.ID)
	if err != nil || out.Quality.State != "source_changed" || out.Quality.BasisCurrent || out.Quality.Passed != 0 || out.Task.Version != out.Delivery.TaskVersion {
		t.Fatal("source drift was hidden behind a passing/current plan", out, err)
	}
	req := f.call("POST", "/requirements", f.body(map[string]any{"title": "Requirement with current adopted design", "acceptance_criteria": []string{"A usable outcome"}}), 201)
	id := req["id"].(string)
	saved, err := f.s.SaveRequirementDesign(ctx, f.actor, f.project, id, designUnitDraft(f))
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: saved.Version, ExpectedRequirementVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.PreviewRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignPreviewRequest{ProjectID: f.project, ExpectedVersion: published.Version, Revision: published.PublishedRevision, SHA256: published.Published.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	adopted, err := f.s.AdoptRequirementDesign(ctx, f.actor, f.project, id, RequirementDesignAdoptRequest{ProjectID: f.project, ExpectedVersion: preview.Version, Revision: preview.Revision, SHA256: preview.SHA256, PlanSHA256: preview.PlanSHA256, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	taskID := adopted.Tasks[0].TaskID
	current, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, taskID)
	if err != nil || current.Design.State != "current" || current.Design.Context == nil || current.Requirement == nil || current.Design.Context.RequirementVersion != current.Requirement.Version || current.Requirement.ID != id {
		t.Fatal("current design/requirement/task were not composed together", current, err)
	}
	f.call("PATCH", "/requirements/"+id, f.body(map[string]any{"expected_version": 1, "description": "Changed business source after adoption"}), 200)
	stale, err := f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, taskID)
	if err != nil || stale.Design.State != "stale" || stale.Design.Context != nil || stale.Requirement == nil || stale.Requirement.Version != 2 {
		t.Fatal("stale adoption was presented as current or lost current source", stale, err)
	}
	_, err = f.s.GetWorkspaceTaskContext(ctx, toolActorForTest(t, f, h.token), f.project, taskID)
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetWorkspaceTaskContext(ctx, f.actor, f.project, taskID)
	requireAuthorityStatus(t, err, 401)
	_, err = f.s.ListWorkspaceTaskOptions(ctx, f.actor, f.project, "", 20, "")
	requireAuthorityStatus(t, err, 401)
}

func TestWorkspaceTaskOptionsLiteralSearchAndScopedCursor(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	for _, title := range []string{"alpha%_!bravo one", "alpha%_!bravo two", "alpha%_!bravo three", "alphaxx!bravo decoy", "unrelated"} {
		f.call("POST", "/tasks", f.body(map[string]any{"title": title}), 201)
	}
	// Fixture-only equal timestamps exercise the ID tie-breaker deterministically.
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET updated_at='2026-10-29T00:00:00Z' WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.ListWorkspaceTaskOptions(ctx, f.actor, f.project, "%_!", 1, "")
	if err != nil || first.Total != 3 || len(first.Items) != 1 || first.NextCursor == "" || !strings.Contains(first.Items[0].Title, "%_!") {
		t.Fatal("LIKE wildcard input was not treated as a literal project-wide keyword", first, err)
	}
	seen := map[string]bool{first.Items[0].ID: true}
	next := first.NextCursor
	for next != "" {
		page, e := f.s.ListWorkspaceTaskOptions(ctx, f.actor, f.project, "%_!", 1, next)
		if e != nil || page.Total != 3 || len(page.Items) != 1 || seen[page.Items[0].ID] || page.Items[0].ID <= first.Items[0].ID {
			t.Fatal("stable paging repeated/mis-scoped a matching task", page, e)
		}
		seen[page.Items[0].ID] = true
		next = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatal("literal search failed to cover the full result set", seen)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Other task-picker project", "idempotency_key": newID()}, 201)["id"].(string)
	_, err = f.s.ListWorkspaceTaskOptions(ctx, f.actor, other, "%_!", 1, first.NextCursor)
	requireAuthorityStatus(t, err, 400)
	_, err = f.s.ListWorkspaceTaskOptions(ctx, f.actor, f.project, "other", 1, first.NextCursor)
	requireAuthorityStatus(t, err, 400)
	raw, err := base64.RawURLEncoding.DecodeString(first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	var forged map[string]any
	if err = json.Unmarshal(raw, &forged); err != nil {
		t.Fatal(err)
	}
	forged["unexpected"] = "not allowed"
	_, err = f.s.ListWorkspaceTaskOptions(ctx, f.actor, f.project, "%_!", 1, base64.RawURLEncoding.EncodeToString([]byte(stringify(forged))))
	requireAuthorityStatus(t, err, 400)
	empty, err := f.s.ListWorkspaceTaskOptions(ctx, f.actor, f.project, "No such title", 0, "")
	if err != nil || empty.Total != 0 || empty.Items == nil || len(empty.Items) != 0 || empty.Limit != 20 || empty.NextCursor != "" {
		t.Fatal("empty or default page was misrepresented", empty, err)
	}
}

func TestWorkspaceTaskContextHTTPStrictReadOnly(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "HTTP title %_!", "acceptance_criteria": []string{}}), 201)
	root := "/workspace/task-context?project_id=" + f.project + "&task_id=" + task["id"].(string)
	before := map[string]int{}
	for _, table := range []string{"dag_runs", "suma_operations", "suma_reviews", "suma_activity", "suma_issues"} {
		before[table] = domainACount(t, f.s, table)
	}
	out := f.call("GET", root, nil, 200)
	if out["project_id"] != f.project || out["requirement"] != nil || out["owner"] != nil || out["design"].(map[string]any)["context"] != nil || out["task"].(map[string]any)["acceptance_criteria"] == nil || out["quality"].(map[string]any)["state"] != "none" {
		t.Fatal("HTTP did not preserve explicit absent facts", out)
	}
	picker := "/workspace/task-context/tasks?project_id=" + f.project
	page := f.call("GET", picker+"&search="+url.QueryEscape("%_!"), nil, 200)
	if page["total"] != float64(1) || len(page["items"].([]any)) != 1 {
		t.Fatal("HTTP search did not use the real title query", page)
	}
	for _, path := range []string{root + "&task_id=" + task["id"].(string), root + "&unknown=x", picker + "&limit=01", picker + "&limit=1e1", picker + "&limit=51", picker + "&cursor=", picker + "&search=" + strings.Repeat("a", 201), picker + "&task_id=" + task["id"].(string)} {
		f.call("GET", path, nil, 400)
	}
	// Browser project selection must not weaken a fixed project API credential.
	issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	before["suma_operations"] = domainACount(t, f.s, "suma_operations")
	before["suma_activity"] = domainACount(t, f.s, "suma_activity")
	f.bearer("GET", root, issued.Token, nil, 200)
	f.bearer("GET", strings.Replace(root, f.project, newID(), 1), issued.Token, nil, 403)
	f.call("POST", "/workspace/task-context", f.body(map[string]any{"task_id": task["id"]}), 404)
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatal("HTTP reads or rejected writes created business state", table)
		}
	}
}
