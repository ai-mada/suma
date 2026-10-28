package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestIntelligentOperationsUnknownLateKnownAndBoundedOutput(t *testing.T) {
	for _, scenario := range []string{"late-known", "late-malformed", "malformed", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			h := newOperationFixture(t)
			h.f.e.AttemptReconciliation = h.f.s
			h.f.e.AttemptAccounting = h.f.s
			a := h.create(t)
			assignment := h.claim(t)
			late := strings.HasPrefix(scenario, "late-")
			var unknown dag.Result
			if late {
				unknown = h.result(assignment, `{"schema_version":1,"partial":"unfinished`)
				unknown.OutcomeUnknown = true
				unknown.ToolEvidence.Terminal = "unknown"
				h.complete(unknown)
				observed := h.get(t, a.ID)
				if observed.State != "reconciling" || observed.OriginalOutput != "" || observed.OriginalOutputSHA256 != "" || observed.Proposal != nil {
					t.Fatal("unknown observation became a completed model original", observed)
				}
				_, err := h.f.s.CommandIntelligentOperation(context.Background(), h.f.actor, h.f.project, a.ID, IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: observed.Version, Action: "dismiss", Reason: "An unknown result cannot be discarded as a known failure", IdempotencyKey: newID()})
				if err == nil {
					t.Fatal("discarded unknown original model operation")
				}
			}
			raw := stringify(operationTestOutput())
			if scenario == "malformed" || scenario == "late-malformed" {
				raw = `{"schema_version":1,"steps":[],"arbitrary_api":"release.deploy"}`
			}
			if scenario == "oversized" {
				raw = strings.Repeat("x", operationLimit+1)
			}
			known := h.result(assignment, raw)
			known.ToolEvidence.Usage = &toolcontract.Usage{InputTokens: usageInt(7), OutputTokens: usageInt(3)}
			known.ToolEvidence.UsageState = toolcontract.UsageComplete
			h.complete(known)
			h.complete(known)
			observed := h.get(t, a.ID)
			want := "failed"
			if scenario == "late-known" {
				want = "ready"
			}
			if observed.State != want || observed.OriginalOutputSHA256 != digest(raw) || observed.RunID != a.RunID || (observed.Proposal != nil) != (want == "ready") {
				t.Fatal(observed)
			}
			if scenario == "oversized" {
				if observed.OriginalOutput != "" {
					t.Fatal("oversized output was truncated and presented as an original")
				}
			} else if observed.OriginalOutput != raw {
				t.Fatal("known original bytes were changed")
			}
			var count, attempt int
			wantCount := 1
			if late {
				wantCount = 2
			}
			if err := h.f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_intelligent_operation_executions WHERE analysis_id=?`, a.ID).Scan(&count); err != nil || count != wantCount {
				t.Fatal("replay duplicated result facts", count, err)
			}
			if err := h.f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, assignment.NodeID).Scan(&attempt); err != nil || attempt != 1 {
				t.Fatal("unknown recovery reran the model", attempt, err)
			}
			usage, err := h.f.s.ListUsageAttempts(context.Background(), h.f.actor, h.f.project, "", "", 20, "")
			if err != nil || len(usage.Items) != 1 || usage.Items[0].ReportedTokens == nil || *usage.Items[0].ReportedTokens != 10 || usage.Items[0].UsageState != toolcontract.UsageComplete {
				t.Fatal("measured usage was confused with valid proposal output", usage, err)
			}
			run, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, a.RunID)
			if err != nil || run.State != "succeeded" {
				t.Fatal("process outcome was replaced by proposal validity", run, err)
			}
		})
	}
}

func TestIntelligentOperationsProjectAPICurrentScopeAndOriginalReceipt(t *testing.T) {
	h := newOperationFixture(t)
	ctx := context.Background()
	for _, scope := range []string{"runner", "agent"} {
		issued, err := h.f.s.IssueProjectCredential(ctx, h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		actor, err := h.f.s.AuthenticateActor(ctx, issued.Token)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = h.f.s.CreateIntelligentOperation(ctx, h.f.e, actor, h.f.project, h.request); err == nil {
			t.Fatal(scope, "self-authorized model planning")
		}
	}
	issued, err := h.f.s.IssueProjectCredential(ctx, h.f.actor, ProjectCredentialIssueRequest{ProjectID: h.f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := h.f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	a, err := h.f.s.CreateIntelligentOperation(ctx, h.f.e, actor, h.f.project, h.request)
	if err != nil {
		t.Fatal("current project API could not use shared planning domain", err)
	}
	assignment := h.claim(t)
	h.complete(h.result(assignment, stringify(operationTestOutput())))
	a, err = h.f.s.GetIntelligentOperation(ctx, actor, h.f.project, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := h.f.s.PreviewIntelligentOperation(ctx, actor, h.f.project, a.ID, IntelligentOperationPreviewRequest{ProjectID: h.f.project, ExpectedVersion: a.Version})
	if err != nil || !preview.CanApply {
		t.Fatal(preview, err)
	}
	command := IntelligentOperationCommandRequest{ProjectID: h.f.project, ExpectedVersion: a.Version, Action: "apply", PreviewSHA256: preview.PreviewSHA256, AcknowledgeDrafts: true, Reason: "Explicit current API user review", IdempotencyKey: newID()}
	applied, err := h.f.s.CommandIntelligentOperation(ctx, actor, h.f.project, a.ID, command)
	if err != nil || applied.State != "applied" {
		t.Fatal(applied, err)
	}
	first := applied.Results[0]
	_, err = h.f.s.Update(ctx, h.f.actor, h.f.project, "requirements", first.ResourceID, h.f.body(map[string]any{"expected_version": first.Version, "title": "Later ordinary user edit"}))
	if err != nil {
		t.Fatal(err)
	}
	replay, err := h.f.s.CommandIntelligentOperation(ctx, actor, h.f.project, a.ID, command)
	if err != nil || stringify(replay.Results) != stringify(applied.Results) || replay.Results[0].Version != 1 {
		t.Fatal("later edit rewrote the original batch receipt", replay, err)
	}
	h.f.bearer("GET", "/intelligent-operations/analyses/"+a.ID+"?project_id="+h.f.project, issued.Token, nil, 200)
	foreign := h.f.call("POST", "/projects", map[string]any{"title": "Other current project", "idempotency_key": newID()}, 201)["id"].(string)
	if _, err = h.f.s.GetIntelligentOperation(ctx, actor, foreign, a.ID); err == nil {
		t.Fatal("API credential escaped its project")
	}
}
