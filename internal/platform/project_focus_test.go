package platform

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func focusTask(t *testing.T, f *fixture, title, status, runState string) Record {
	t.Helper()
	task, err := f.s.Create(context.Background(), f.actor, f.project, "tasks", f.body(map[string]any{"title": title, "status": status}))
	if err != nil {
		t.Fatal(err)
	}
	if runState != "" {
		run := provenanceRun(t, f)
		if err = f.s.AttachRun(context.Background(), f.actor, f.project, task.ID, run); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.DB.Exec(`UPDATE dag_runs SET state=?,updated_at='2026-10-29T12:00:00Z' WHERE id=?`, runState, run); err != nil {
			t.Fatal(err)
		}
		task, err = f.s.Get(context.Background(), f.actor, f.project, "tasks", task.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return task
}

func focusSection(t *testing.T, out ProjectFocus, key string) ProjectFocusSection {
	t.Helper()
	for _, section := range out.Sections {
		if section.Key == key {
			return section
		}
	}
	t.Fatalf("missing focus section %q: %+v", key, out)
	return ProjectFocusSection{}
}

func TestProjectFocusBoundedExactCountsAndStableOrder(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	for i := 0; i < 9; i++ {
		focusTask(t, f, fmt.Sprintf("Planning %02d", i), "todo", "")
	}
	for _, status := range []string{"accepted", "done", "completed", "cancelled", "archived", "deleted", "custom_future_state"} {
		task := focusTask(t, f, status, "todo", "")
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status=? WHERE id=?`, status, task.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET updated_at='2026-10-29T00:00:00Z' WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	before := expertCount(t, f, "dag_runs")
	out, err := f.s.GetProjectFocus(ctx, f.actor, f.project)
	if err != nil || out.ProjectID != f.project || out.ObservedAt == "" || len(out.Sections) != 4 {
		t.Fatal(out, err)
	}
	planning := focusSection(t, out, "planning")
	if planning.Total != 9 || len(planning.Items) != 6 || !planning.Truncated {
		t.Fatalf("focus count used its visible page: %+v", planning)
	}
	for i, item := range planning.Items {
		if item.Reason != "task_unstarted" || item.TaskStatus != "todo" || item.RunID != "" || item.RunState != "" || item.TaskVersion != 1 || i > 0 && planning.Items[i-1].TaskID >= item.TaskID {
			t.Fatalf("unstable/untrue task facts: %+v", planning.Items)
		}
	}
	for _, key := range []string{"execution", "review", "attention"} {
		section := focusSection(t, out, key)
		if section.Total != 0 || section.Items == nil || section.Truncated {
			t.Fatal(section)
		}
	}
	if expertCount(t, f, "dag_runs") != before {
		t.Fatal("focus read dispatched work")
	}
}

func TestProjectFocusExactCurrentRunAndDisjointFacts(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	active := focusTask(t, f, "Current run only", "todo", "running")
	old := active.RunID
	if _, err := f.s.DB.Exec(`UPDATE dag_runs SET state='failed' WHERE id=?`, old); err != nil {
		t.Fatal(err)
	}
	if err := f.s.AttachRun(ctx, f.actor, f.project, active.ID, provenanceRun(t, f)); err != nil {
		t.Fatal(err)
	}
	review := focusTask(t, f, "Completed run still needs review", "todo", "succeeded")
	failed := focusTask(t, f, "Failure", "todo", "failed")
	blocked := focusTask(t, f, "Explicit business blocker", "blocked", "")
	unknownRun := focusTask(t, f, "Unknown execution state", "todo", "future_state")
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET updated_at='2026-10-29T00:00:00Z' WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.GetProjectFocus(ctx, f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, section := range out.Sections {
		for _, item := range section.Items {
			if seen[item.TaskID] != "" || item.TaskID == unknownRun.ID || item.RunID == old {
				t.Fatalf("focus duplicated tasks or projected history: %+v", item)
			}
			seen[item.TaskID] = section.Key + ":" + item.Reason
			if item.TaskID == review.ID && (item.RunState != "succeeded" || item.UpdatedAt != "2026-10-29T12:00:00Z" || item.TaskStatus != "in_progress") {
				t.Fatal("run/task facts collapsed", item)
			}
		}
	}
	if seen[active.ID] != "execution:run_active" || seen[review.ID] != "review:run_succeeded_review_pending" || seen[failed.ID] != "attention:run_failed" || seen[blocked.ID] != "attention:task_blocked" || len(seen) != 4 {
		t.Fatal(seen)
	}
	encoded := stringify(out)
	for _, secret := range []string{`"output":`, `"command":`, `"data":`, `"formal_acceptance":`, `"tests":`} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("focus leaks detail or implies quality facts: %s", encoded)
		}
	}
}

func TestProjectFocusSucceededSourceDriftAndUnavailableProof(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	changed := focusTask(t, f, "Original completed source", "todo", "succeeded")
	if _, err := f.s.Update(ctx, f.actor, f.project, "tasks", changed.ID, f.body(map[string]any{"title": "Changed after completion", "expected_version": changed.Version})); err != nil {
		t.Fatal(err)
	}
	unavailable := focusTask(t, f, "Unproven original binding", "todo", "succeeded")
	if _, err := f.s.DB.Exec(`DELETE FROM suma_task_run_provenance WHERE run_id=?`, unavailable.RunID); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.GetProjectFocus(ctx, f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	if section := focusSection(t, out, "review"); section.Total != 0 {
		t.Fatal("stale/unknown original source was presented for review", section)
	}
	attention := focusSection(t, out, "attention")
	reasons := map[string]string{}
	for _, item := range attention.Items {
		reasons[item.TaskID] = item.Reason
	}
	if attention.Total != 2 || reasons[changed.ID] != "run_basis_changed" || reasons[unavailable.ID] != "run_basis_unavailable" {
		t.Fatal(attention)
	}
}

func TestProjectFocusScopedMissingRunAndReconcilingNode(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	missingRun := focusTask(t, f, "Foreign scope must not reveal run", "todo", "running")
	if _, err := f.s.DB.Exec(`UPDATE dag_runs SET tenant_id='other-tenant',project_id='other-project' WHERE id=?`, missingRun.RunID); err != nil {
		t.Fatal(err)
	}
	reconciling := focusTask(t, f, "Unknown side effect", "todo", "running")
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state='reconciling',reason='private raw details' WHERE run_id=?`, reconciling.RunID); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.GetProjectFocus(ctx, f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	attention := focusSection(t, out, "attention")
	reasons := map[string]string{}
	for _, item := range attention.Items {
		reasons[item.TaskID] = item.Reason
		if item.TaskID == missingRun.ID && (item.RunID != missingRun.RunID || item.RunState != "") {
			t.Fatal("foreign run state leaked", item)
		}
	}
	if attention.Total != 2 || reasons[missingRun.ID] != "run_missing" || reasons[reconciling.ID] != "run_reconciling" || focusSection(t, out, "execution").Total != 0 || strings.Contains(stringify(out), "private raw details") {
		t.Fatal(out)
	}
}

func TestProjectFocusCurrentAuthorizationAndStrictHTTP(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	focusTask(t, f, "Authorized task", "todo", "")
	f.call("GET", "/project-focus?project_id="+f.project, nil, 200)
	for _, query := range []string{"", "?project_id=" + f.project + "&project_id=" + f.project, "?project_id=" + f.project + "&limit=6"} {
		f.call("GET", "/project-focus"+query, nil, 400)
	}
	_, err := f.s.GetProjectFocus(ctx, f.actor, "other-project")
	requireAuthorityStatus(t, err, 404)
	_, err = f.s.GetProjectFocus(ctx, Actor{kind: actorProjectRunner}, f.project)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.GetProjectFocus(ctx, Actor{kind: actorAgentRead}, f.project)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.GetProjectFocus(ctx, f.actor, "")
	requireAuthorityStatus(t, err, 400)
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "Project focus read", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	apiActor, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetProjectFocus(ctx, apiActor, f.project); err != nil {
		t.Fatal("current scoped API could not read its project", err)
	}
	f.bearer("GET", "/project-focus?project_id="+f.project, issued.Token, nil, 200)
	_, err = f.s.GetProjectFocus(ctx, apiActor, "other-project")
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetProjectFocus(ctx, f.actor, f.project)
	requireAuthorityStatus(t, err, 401)
}
