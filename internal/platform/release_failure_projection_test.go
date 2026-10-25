package platform

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

func TestReleaseManagedFailureProjectionUsesOperationIssue(t *testing.T) {
	f, _, _, plan, _, _ := verifiedReleaseDeploymentPlan(t)
	ctx := context.Background()
	d, err := f.s.ExecuteReleasePlan(ctx, f.actor, f.project, plan.ID, releaseExecuteBody(f, plan), f.e)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := f.s.RegisterReleaseWorker(ctx, f.actor, f.project, map[string]any{"worker_id": "failed-release-worker", "profiles": []any{}, "deployment": true, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	control := httptest.NewServer(f.handler)
	defer control.Close()
	client := &runner.Client{BaseURL: control.URL, Token: worker["token"].(string), ProjectID: f.project}
	a, err := client.Claim(ctx, "failed-release-worker", []string{"external_workflow:local-static-test"})
	if err != nil || a == nil {
		t.Fatal("missing protected assignment", err)
	}
	// A real Runner failure before contacting the provider must still surface,
	// without inventing an authenticated observation or releasing its barrier.
	if err = client.Complete(ctx, runner.Result{Assignment: *a, OperationID: newID(), ExitCode: 1, Error: "local provider transport unavailable before request"}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.ReleaseDeployment(ctx, f.actor, f.project, d.OperationID)
	if err != nil || current.IssueID == "" {
		t.Fatalf("run failure missing canonical operation issue: %+v %v", current, err)
	}
	if current.Observation != nil {
		t.Fatal("Runner failure fabricated provider observation")
	}
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_issues WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&count); err != nil || count != 1 {
		t.Fatalf("projector duplicated issue: %d %v", count, err)
	}
	issue, err := f.s.Get(ctx, f.actor, f.project, "issues", current.IssueID)
	if err != nil || issue.RunID != d.RunID || issue.Data["release_operation_id"] != d.OperationID {
		t.Fatal("issue lost exact operation/run scope", err)
	}
	if issue.Data["failure_source"] != "execution" || issue.Data["provider_evidence_sha256"] != nil {
		t.Fatal("Runner failure was relabeled as an authenticated provider failure")
	}
	if _, err = f.s.IssueToRequirement(ctx, f.actor, f.project, issue.ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.IssueToRequirement(ctx, f.actor, f.project, issue.ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_requirements WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&count); err != nil || count != 1 {
		t.Fatalf("operation issue intake not unique: %d %v", count, err)
	}
}
