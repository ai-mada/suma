package platform

import (
	"context"
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
	"testing"
)

// Reuse the real native selection/accounting fixture: Octop does not acquire a
// second model catalog, rate or budget lane by changing its adapter.
func octopExecutionFixture(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile, []mc.Deployment) {
	t.Helper()
	f, token, raw, p, deployments := modelExecutionFixture(t)
	p.Profile.Adapter, p.Profile.Protocol, p.Profile.ToolVersion = "octop", tc.OctopProtocol, tc.OctopHarnessVersion
	p.Profile.OctopExecution = &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion}
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET profile=? WHERE id=?`, stringify(p.Profile), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, ModelDeployments: deployments}}}); err != nil {
		t.Fatal(err)
	}
	var req TaskDispatchPreviewRequest
	if err := json.Unmarshal([]byte(stringify(raw)), &req); err != nil {
		t.Fatal(err)
	}
	return f, token, req, p, deployments
}

func TestOctopControlUsesUnifiedSelectionAndSettlesKnownCancelledCost(t *testing.T) {
	f, token, req, p, deployments := octopExecutionFixture(t)
	ctx := context.Background()
	options, err := f.s.GetModelExecutionOptions(ctx, f.actor, f.project, ModelExecutionOptionsRequest{ProjectID: f.project, ToolProfileID: p.ID, ExpectedToolVersion: p.Version})
	if err != nil || !options.Supported || len(options.Items) != 2 || options.ParameterLimits == nil || options.ParameterLimits.MaxSteps != mc.MaxSteps || options.ParameterLimits.MaxOutputTokens != mc.MaxOutputTokens {
		t.Fatal("unified authorized model catalog/limits lost", options, err)
	}
	modelRateForTest(t, f, p, deployments[0], 0)
	if _, err = f.s.WriteUsagePolicy(ctx, f.actor, f.project, UsagePolicyWriteRequest{ProjectID: f.project, Scope: "organization", Mode: "enforce", ExpectedVersion: 0, Currency: "CNY", TokenLimit: usageInt(64100), CostLimitMicros: usageInt(64200), AcknowledgeEnforcement: true, Reason: "Exact model reservation", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil || plan.ModelExecution == nil {
		t.Fatal("enforced exact-rate Octop preview failed", plan, err)
	}
	n := codexPlanNode(t, plan)
	if _, present := n.Config["octop_execution"]; present {
		t.Fatal("duplicate Octop model snapshot survived")
	}
	if n.MaxRetries != 0 || tc.ValidateOctopNodeConfiguration(n.Config) != nil {
		t.Fatal("unreviewed overrides/retries survived")
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	reply := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID != run.ID {
		t.Fatal("unified Octop model was not claimed", reply)
	}
	b, err := dag.ModelBinding(n)
	if err != nil || b == nil || b.BindingSHA256 != plan.ModelExecution.BindingSHA256 {
		t.Fatal("claim changed model authority", b, err)
	}
	me := &mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: b.Deployment.SHA256, ParametersSHA256: b.ParametersSHA256, Model: b.Deployment.Model, Requests: []mc.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: b.Deployment.Model, OutputAllowance: 100, UsageState: "complete", Usage: &mc.RequestUsage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}}}}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: newID(), Stopped: true, ExitCode: -1, Error: "cancelled after model response", ToolEvidence: &tc.Evidence{Profile: p.Profile, SessionID: "real-acp-session", Terminal: "unknown", EventCount: 4, UsageState: tc.UsageComplete, Usage: &tc.Usage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}, ModelExecution: me, OctopExecution: &tc.OctopExecutionEvidence{BindingSHA256: b.BindingSHA256, SessionID: "real-acp-session", ProcessStopped: true, CancelRequested: true}}}
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, Spec: n}
	for i := 0; i < 2; i++ {
		tx, e := f.s.DB.Begin()
		if e != nil {
			t.Fatal(e)
		}
		if e = f.s.RecordResultTx(ctx, tx, ref, result); e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	billed := page.Items[0]
	if billed.State != "settled" || billed.ReportedTokens == nil || *billed.ReportedTokens != 40 || billed.EstimatedCostMicros == nil || *billed.EstimatedCostMicros != 57 || billed.HeldTokens != 0 || billed.Version != 2 {
		t.Fatal("known costs were erased by code cancellation or counted twice", billed)
	}
}

func TestOctopControlRejectsOldSnapshotsAndReviewedModelDrift(t *testing.T) {
	for _, kind := range []string{"old-snapshot", "retry", "limits", "resume", "binding"} {
		t.Run(kind, func(t *testing.T) {
			f, _, req, _, _ := octopExecutionFixture(t)
			plan, err := f.s.CreateTaskDispatchPlan(context.Background(), f.e, f.actor, f.project, req)
			if err != nil {
				t.Fatal(err)
			}
			wire := candidateJSON(t, plan)
			var node map[string]any
			for _, raw := range wire["workflow"].(map[string]any)["nodes"].([]any) {
				v := raw.(map[string]any)
				if v["id"] == plan.AgentNodeID {
					node = v
					break
				}
			}
			config := node["config"].(map[string]any)
			switch kind {
			case "old-snapshot":
				config["octop_execution"] = map[string]any{"model": "old"}
			case "retry":
				node["max_retries"] = 1
			case "limits":
				config["max_steps"] = 3
			case "resume":
				config["resume"] = "foreign"
			case "binding":
				delete(config, "model_execution")
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(wire), plan.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.StartTaskDispatchPlan(context.Background(), f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil || expertCount(t, f, "dag_runs") != 0 {
				t.Fatal("unreviewed Octop input started", kind, err)
			}
		})
	}
}
