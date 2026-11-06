package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestFailureDiagnosisIssueSourceUsesOnlyCurrentScopedReceipt(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{
		ID: "approval", Name: "Approval failure", Nodes: []dag.Node{{ID: "gate", Type: "approval"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fail := func() (run string, issue Record) {
		t.Helper()
		r, e := f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow.ID, newID())
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.e.Command(ctx, f.user.TenantID, f.project, r.ID, "approve", newID(), r.Version, map[string]any{"node_id": "gate", "decision": "rejected"}); e != nil {
			t.Fatal(e)
		}
		if e = f.s.ProjectFailures(ctx); e != nil {
			t.Fatal(e)
		}
		var raw string
		if e = f.s.DB.QueryRowContext(ctx, `SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, failureIssueKey(r.ID)).Scan(&raw); e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal([]byte(raw), &issue); e != nil || issue.ID == "" {
			t.Fatalf("invalid projected issue: %v", e)
		}
		return r.ID, issue
	}
	run, issue := fail()
	otherRun, otherIssue := fail()
	base := "/failure-diagnosis/issue-source?project_id=" + f.project
	path := base + "&issue_id=" + issue.ID
	assertSource := func() {
		t.Helper()
		response := f.call("GET", path, nil, 200)
		if response["project_id"] != f.project || response["issue_id"] != issue.ID || response["run_id"] != run || len(response) != 3 {
			t.Fatalf("unexpected source: %+v", response)
		}
	}
	f.call("PATCH", "/issues/"+issue.ID, f.body(map[string]any{
		"expected_version": issue.Version, "data": map[string]any{"run_id": otherRun},
	}), 200)
	assertSource()
	manual := f.call("POST", "/issues", f.body(map[string]any{
		"title": "Manual issue", "data": map[string]any{"run_id": run, "origin": "execution"},
	}), 201)["id"].(string)
	response := f.call("GET", base+"&issue_id="+manual, nil, 200)
	if _, supplied := response["run_id"]; supplied || response["issue_id"] != manual || len(response) != 2 {
		t.Fatalf("manual data created an execution source: %+v", response)
	}
	f.call("GET", base+"&issue_id="+newID(), nil, 404)
	for _, bad := range []string{base, base + "&issue_id=", path + "&issue_id=" + issue.ID, path + "&run_id=" + run, path + "&project_id=" + f.project} {
		f.call("GET", bad, nil, 400)
	}
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "source-worker", "profiles": []any{}, "deployment": false}), 201)
	f.bearer("GET", path, worker["token"].(string), nil, 403)

	// A malformed unrelated receipt cannot poison valid lookup in SQLite.
	setReceipt := func(run, raw string) {
		t.Helper()
		if _, e := f.s.DB.ExecContext(ctx, `UPDATE suma_operations SET response=? WHERE tenant_id=? AND project_id=? AND operation_id=?`, raw, f.user.TenantID, f.project, failureIssueKey(run)); e != nil {
			t.Fatal(e)
		}
	}
	setReceipt(otherRun, `{"bad":`)
	assertSource()
	otherIssue.ID = issue.ID
	setReceipt(otherRun, stringify(otherIssue))
	f.call("GET", path, nil, 409)
}
