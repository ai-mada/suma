package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestDispatchRecommendationLateKnownOriginalAttemptPreservesFacts(t *testing.T) {
	for _, drift := range []bool{false, true} {
		name := "current"
		if drift {
			name = "source_changed"
		}
		t.Run(name, func(t *testing.T) {
			h := newDispatchRecommendationFixture(t)
			f := h.f
			ctx := context.Background()
			f.e.AttemptReconciliation = f.s
			a, assignment, known, _ := h.start(t)
			var unknown dag.Result
			if err := json.Unmarshal([]byte(stringify(known)), &unknown); err != nil {
				t.Fatal(err)
			}
			unknown.OperationID = newID()
			unknown.Output = `{"schema_version":1,"partial":"unfinished`
			unknown.OutcomeUnknown = true
			unknown.ToolEvidence.Terminal = "unknown"
			unknown.ToolEvidence.EventCount = 2
			unknown.ToolEvidence.UsageState = tc.UsageUnreported
			unknown.ToolEvidence.Usage = nil
			unknown.ToolEvidence.ModelExecution.GatewayClosed = false
			request := &unknown.ToolEvidence.ModelExecution.Requests[0]
			request.State = "started"
			request.ResponseSHA256 = ""
			request.ProviderModel = ""
			request.ProviderRequestID = ""
			request.UsageState = tc.UsageUnreported
			request.Usage = nil
			f.bearer("POST", "/runner/results", h.analysisToken, f.body(map[string]any{"result": unknown}), 200)
			observed, err := f.s.GetDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID)
			if err != nil || observed.State != "reconciling" || observed.Proposal != nil || observed.OriginalOutput != "" || observed.OriginalOutputSHA256 != "" {
				t.Fatal("unknown observation became a completed original", observed, err)
			}
			if drift {
				_, err = f.s.Update(ctx, f.actor, f.project, "tasks", h.request.TaskID, f.body(map[string]any{"expected_version": h.request.ExpectedTaskVersion, "title": "Changed scope while the original response was in flight"}))
				if err != nil {
					t.Fatal(err)
				}
			}
			var changed dag.Result
			if err = json.Unmarshal([]byte(stringify(known)), &changed); err != nil {
				t.Fatal(err)
			}
			changed.OperationID = newID()
			changed.ToolEvidence.ModelExecution.Requests[0].RequestSHA256 = strings.Repeat("c", 64)
			f.bearer("POST", "/runner/results", h.analysisToken, f.body(map[string]any{"result": changed}), 409)
			for i := 0; i < 2; i++ {
				f.bearer("POST", "/runner/results", h.analysisToken, f.body(map[string]any{"result": known}), 200)
			}
			observed, err = f.s.GetDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID)
			want := "ready"
			if drift {
				want = "failed"
			}
			if err != nil || observed.State != want || observed.RunID != a.RunID || observed.OriginalOutput != known.Output || observed.OriginalOutputSHA256 != digest(known.Output) || (observed.Proposal != nil) != (want == "ready") || drift && observed.BasisCurrent {
				t.Fatal("late known result did not preserve facts independently of source freshness", observed, err)
			}
			for _, fact := range []dag.Result{unknown, known} {
				var raw, stamp string
				if err = f.s.DB.QueryRow(`SELECT result,created_at FROM suma_dispatch_recommendation_outcomes WHERE recommendation_id=? AND attempt=? AND fence=? AND result_sha256=?`, a.ID, assignment.Attempt, assignment.Fence, digest(stringify(fact))).Scan(&raw, &stamp); err != nil || raw != stringify(fact) || stamp == "" {
					t.Fatal("original outcome fact was lost", err)
				}
			}
			var count, attempt int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_dispatch_recommendation_outcomes WHERE recommendation_id=?`, a.ID).Scan(&count); err != nil || count != 2 {
				t.Fatal("duplicate known receipt created another fact", count, err)
			}
			if err = f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, a.RunID, assignment.NodeID).Scan(&attempt); err != nil || attempt != 1 || dispatchOptionsCount(t, f, "dag_runs") != 1 {
				t.Fatal("completion dispatched another model attempt", attempt, err)
			}
			usage, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 20, "")
			if err != nil || len(usage.Items) != 1 || usage.Items[0].ReportedTokens == nil || *usage.Items[0].ReportedTokens != 5 || usage.Items[0].UsageState != tc.UsageComplete {
				t.Fatal("known usage was lost or double charged", usage, err)
			}
			stale := known
			stale.OperationID = newID()
			stale.Attempt = assignment.Attempt + 1
			f.bearer("POST", "/runner/results", h.analysisToken, f.body(map[string]any{"result": stale}), 409)
		})
	}
}
