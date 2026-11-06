package platform

import (
	"context"
	"strings"
	"testing"
)

func TestFailureDiagnosisRequiresProjectedIssueProvenance(t *testing.T) {
	h := newDiagnosisFixture(t)
	f, ctx := h.f, context.Background()
	initialRuns := expertCount(t, f, "dag_runs")
	assertRejected := func(t *testing.T, req FailureDiagnosisCreateRequest) {
		t.Helper()
		_, err := f.s.CreateFailureDiagnosis(ctx, f.e, f.actor, req)
		expertStatus(t, err, 409)
		if expertCount(t, f, "suma_failure_diagnoses") != 0 || expertCount(t, f, "dag_runs") != initialRuns {
			t.Fatal("rejected evidence created a diagnosis or scheduled a run")
		}
	}

	t.Run("manual issue with exact forged source digest", func(t *testing.T) {
		manual, err := f.s.Create(ctx, f.actor, f.project, "issues", f.body(map[string]any{
			"title": "Unrelated manual issue", "description": "Ordinary editable metadata",
			"data": map[string]any{"run_id": h.request.RunID, "origin": "execution"},
		}))
		if err != nil {
			t.Fatal(err)
		}
		// Supply the digest that the old metadata-based source resolver would
		// produce. A stale digest alone would already reject this request.
		forged := *h.context.Source
		forged.IssueID, forged.IssueVersion = manual.ID, manual.Version
		forged.Evidence = append([]FailureDiagnosisEvidence(nil), forged.Evidence...)
		forged.Evidence[0], err = diagnosisEvidence("issue", manual.Title+"\n"+manual.Description)
		if err != nil {
			t.Fatal(err)
		}
		forged.SHA256 = ""
		req := h.request
		req.IssueID, req.ExpectedIssueVersion = manual.ID, manual.Version
		req.ExpectedSourceSHA256, req.IdempotencyKey = digest(stringify(forged)), newID()
		assertRejected(t, req)
		f.call("POST", "/failure-diagnosis/analyses", req, 409)
	})

	key := "failure-" + h.request.RunID
	var hash string
	if err := f.s.DB.QueryRowContext(ctx, `SELECT request_hash FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, key).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mutation, restoration string
		mutate, restore             []any
	}{
		{
			name:        "missing receipt",
			mutation:    `UPDATE suma_operations SET operation_id=? WHERE tenant_id=? AND project_id=? AND operation_id=?`,
			mutate:      []any{"fixture-hidden-" + key, f.user.TenantID, f.project, key},
			restoration: `UPDATE suma_operations SET operation_id=? WHERE tenant_id=? AND project_id=? AND operation_id=?`,
			restore:     []any{key, f.user.TenantID, f.project, "fixture-hidden-" + key},
		},
		{
			name:        "invalid receipt",
			mutation:    `UPDATE suma_operations SET request_hash=? WHERE tenant_id=? AND project_id=? AND operation_id=?`,
			mutate:      []any{strings.Repeat("0", 64), f.user.TenantID, f.project, key},
			restoration: `UPDATE suma_operations SET request_hash=? WHERE tenant_id=? AND project_id=? AND operation_id=?`,
			restore:     []any{hash, f.user.TenantID, f.project, key},
		},
		{
			name:        "missing run_failed",
			mutation:    `UPDATE dag_events SET event_type=? WHERE tenant_id=? AND project_id=? AND run_id=? AND event_type=?`,
			mutate:      []any{"fixture-hidden", f.user.TenantID, f.project, h.request.RunID, "run_failed"},
			restoration: `UPDATE dag_events SET event_type=? WHERE tenant_id=? AND project_id=? AND run_id=? AND event_type=?`,
			restore:     []any{"run_failed", f.user.TenantID, f.project, h.request.RunID, "fixture-hidden"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.s.DB.ExecContext(ctx, tc.mutation, tc.mutate...); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := f.s.DB.ExecContext(ctx, tc.restoration, tc.restore...); err != nil {
					t.Error(err)
				}
			})
			view, err := f.s.FailureDiagnosisContext(ctx, f.e, f.actor, f.project, h.request.RunID, h.request.NodeID, h.request.Attempt)
			if err != nil {
				expertStatus(t, err, 409)
			}
			if view.Eligible || view.CanCreate || view.Source != nil {
				t.Fatal("unverified projection offered as a diagnosis source")
			}
			req := h.request
			req.IdempotencyKey = newID()
			assertRejected(t, req)
		})
	}

	t.Run("projected issue retains authority after display edit", func(t *testing.T) {
		issue, err := f.s.Update(ctx, f.actor, f.project, "issues", h.request.IssueID, f.body(map[string]any{
			"expected_version": h.request.ExpectedIssueVersion,
			"data":             map[string]any{"run_id": "edited-display-only"},
		}))
		if err != nil || issue.Data["run_id"] != "edited-display-only" {
			t.Fatalf("editable display reference was not changed: %v", err)
		}
		view, err := f.s.FailureDiagnosisContext(ctx, f.e, f.actor, f.project, h.request.RunID, h.request.NodeID, h.request.Attempt)
		if err != nil || !view.Eligible || !view.CanCreate || view.Source == nil || view.Source.IssueID != issue.ID || view.Source.IssueVersion != issue.Version || view.Source.RunID != h.request.RunID {
			t.Fatalf("original projection was not authoritative after the edit: %+v, %v", view, err)
		}
		delivery := f.call("GET", "/tasks/"+view.Source.TaskID+"/delivery?project_id="+f.project, nil, 200)
		refs := delivery["issues"].(map[string]any)["items"].([]any)
		if len(refs) != 1 || refs[0].(map[string]any)["issue_id"] != issue.ID || refs[0].(map[string]any)["source"] != "execution_failure" {
			t.Fatalf("task delivery did not use the same canonical issue: %+v", refs)
		}
		req := h.request
		req.ExpectedIssueVersion, req.ExpectedSourceSHA256, req.IdempotencyKey = issue.Version, view.Source.SHA256, newID()
		analysis, err := f.s.CreateFailureDiagnosis(ctx, f.e, f.actor, req)
		if err != nil || analysis.Source.IssueID != issue.ID || analysis.Source.RunID != h.request.RunID {
			t.Fatalf("refreshed request did not retain the original failure source: %+v, %v", analysis.Source, err)
		}
	})
}
