package platform

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func openCodeTestProfile() tc.Profile {
	return tc.Profile{CommandID: "opencode-sdk", Adapter: "opencode", Protocol: tc.OpenCodeSDKProtocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: tc.OpenCodeSDKVersion, WorkspaceToolsVersion: "attempt-workspace-v1", OpenCodeExecution: &tc.OpenCodeExecutionCapability{SDKVersion: tc.OpenCodeSDKVersion, ServerVersion: tc.OpenCodeSDKVersion, ProviderID: "owned", Model: "model-a", EndpointOrigin: "https://models.example", ConfigurationSHA256: strings.Repeat("d", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}}
}

func openCodeExecutionFixture(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile) {
	t.Helper()
	f, token, raw, _ := taskDispatchFixture(t)
	p, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Fixed OpenCode SDK", Profile: openCodeTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	reportOpenCodeInventory(t, f, token, p)
	p, err = f.s.CommandToolProfile(context.Background(), f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal("probed SDK profile not dispatchable", p, err)
	}
	var request TaskDispatchPreviewRequest
	if err := json.Unmarshal([]byte(stringify(raw)), &request); err != nil {
		t.Fatal(err)
	}
	request.ToolProfileID, request.ExpectedToolVersion = p.ID, p.Version
	f.e.AttemptAccounting = f.s
	return f, token, request, p
}

func reportOpenCodeInventory(t *testing.T, f *fixture, token string, p ToolProfile) {
	t.Helper()
	_, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, WorkspaceObservationProtocol: "workspace-observation-v1", WorkspaceWriteProtocol: "workspace-write-v1"}}})
	if err != nil {
		t.Fatal(err)
	}
}

func openCodeFactMatches(value any, expected *tc.OpenCodeExecutionCapability) bool {
	var actual tc.OpenCodeExecutionCapability
	return expected != nil && json.Unmarshal([]byte(stringify(value)), &actual) == nil && actual == *expected
}

func TestOpenCodeControlFixedProfileHTTPAndPlanIdentity(t *testing.T) {
	f, token, req, p := openCodeExecutionFixture(t)
	ctx := context.Background()
	options := f.call("GET", "/model-execution/options?project_id="+f.project+"&tool_profile_id="+p.ID+"&expected_tool_version="+strconv.Itoa(p.Version), nil, 200)
	fixed, _ := options["fixed_opencode"].(map[string]any)
	if options["supported"] != false || options["codex_supported"] != false || len(fixed) == 0 || fixed["model"] != "model-a" || options["reason"] != "fixed_by_tool" || len(options["items"].([]any)) != 0 {
		t.Fatal("fixed administrator SDK model was lost or exposed as independently selectable", options)
	}
	plan := f.call("POST", "/task-dispatch-plans", candidateJSON(t, req), 201)
	if plan["model_execution"] != nil || plan["codex_execution"] != nil || !openCodeFactMatches(plan["opencode_execution"], p.Profile.OpenCodeExecution) {
		t.Fatal("preview replaced fixed SDK facts", plan)
	}
	var typed TaskDispatchPlan
	if json.Unmarshal([]byte(stringify(plan)), &typed) != nil {
		t.Fatal("invalid returned plan")
	}
	n := codexPlanNode(t, typed)
	if !openCodeFactMatches(n.Config["opencode_execution"], p.Profile.OpenCodeExecution) || n.MaxRetries != 0 {
		t.Fatal("workflow did not freeze original SDK configuration/no-retry policy", n)
	}
	started := f.call("POST", "/task-dispatch-plans/"+typed.ID+"/start", f.body(map[string]any{"expected_version": typed.Version}), 201)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &a) != nil || a.RunID != started["id"] || !openCodeFactMatches(a.Config["opencode_execution"], p.Profile.OpenCodeExecution) {
		t.Fatal("SDK claim changed reviewed configuration", claim)
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Rate != nil || page.Items[0].ReservedTokens != 0 || page.Items[0].EstimatedCostMicros != nil {
		t.Fatal("fixed configuration created native hard allowance or priced identity", page, err)
	}
	bad := candidateJSON(t, req)
	bad["opencode_execution_selection"] = map[string]any{"model": "other"}
	bad["idempotency_key"] = newID()
	f.call("POST", "/task-dispatch-plans", bad, 400)
}

func TestOpenCodeControlEnforceBlocksPreviewStartClaimAndAtomicReserve(t *testing.T) {
	f, token, req, p := openCodeExecutionFixture(t)
	ctx := context.Background()
	org := codexPolicy(t, f, "organization", "enforce", 0)
	if _, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req); err == nil {
		t.Fatal("organization enforce admitted observe-only SDK preview")
	}
	org = codexPolicy(t, f, "organization", "observe", org.Version)
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	project := codexPolicy(t, f, "project", "enforce", 0)
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	if _, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start); err == nil || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("changed project enforce admitted Start", err)
	}
	codexPolicy(t, f, "project", "observe", project.Version)
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	codexPolicy(t, f, "organization", "enforce", org.Version)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	if claim["assignment"] != nil || expertCount(t, f, "suma_usage_attempts") != 0 {
		t.Fatal("enforce acquired SDK attempt or ledger hold", claim)
	}
	current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
	if err != nil || current.Nodes[0].Attempt != 0 {
		t.Fatal("ineligible SDK consumed an attempt", current, err)
	}
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: run.ID, NodeID: plan.AgentNodeID, WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: codexPlanNode(t, plan)}
	allowed, err := f.s.CheckAttemptTx(ctx, tx, ref)
	if err != nil || allowed {
		t.Fatal("accounting Check admitted SDK enforce", allowed, err)
	}
	if err := f.s.ReserveAttemptTx(ctx, tx, ref); err == nil {
		t.Fatal("accounting Reserve admitted SDK enforce")
	}
}

func TestOpenCodeControlRejectsWorkflowAndReviewedSnapshotDrift(t *testing.T) {
	for _, kind := range []string{"workflow-model", "missing-workflow-fact", "snapshot-model", "retry"} {
		t.Run(kind, func(t *testing.T) {
			f, _, req, _ := openCodeExecutionFixture(t)
			ctx := context.Background()
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
			if err != nil {
				t.Fatal(err)
			}
			wire := candidateJSON(t, plan)
			flow := wire["workflow"].(map[string]any)
			nodes := flow["nodes"].([]any)
			var node map[string]any
			for _, item := range nodes {
				value := item.(map[string]any)
				if value["id"] == plan.AgentNodeID {
					node = value
				}
			}
			switch kind {
			case "workflow-model":
				node["config"].(map[string]any)["opencode_execution"].(map[string]any)["model"] = "other"
			case "missing-workflow-fact":
				delete(node["config"].(map[string]any), "opencode_execution")
			case "snapshot-model":
				wire["opencode_execution"].(map[string]any)["model"] = "other"
			case "retry":
				node["max_retries"] = 1
			}
			if _, err := f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(wire), plan.ID); err != nil {
				t.Fatal(err)
			}
			view, err := f.s.GetTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID)
			if err != nil || view.Valid {
				t.Fatal("changed reviewed SDK facts remained valid", view, err)
			}
			if _, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil || expertCount(t, f, "dag_runs") != 0 {
				t.Fatal("changed SDK snapshot started", err)
			}
		})
	}
}

func TestOpenCodeExpertPreviewFreezesSameAdministratorModelAndBudgetGate(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Fixed expert SDK", Profile: openCodeTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	reportOpenCodeInventory(t, f, h.runnerToken, p)
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.role.ToolProfileID, h.role.ExpectedToolVersion = p.ID, p.Version
	h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Review exact SDK fixed model", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, h.request())
	if err != nil || len(preview.Items) != 2 {
		t.Fatal(preview, err)
	}
	for _, item := range preview.Items {
		wire := candidateJSON(t, item.Plan)
		if !openCodeFactMatches(wire["opencode_execution"], p.Profile.OpenCodeExecution) || item.Plan.CodexExecution != nil || item.Plan.ModelExecution != nil {
			t.Fatal("expert plan invented an independent choice or changed fixed model", wire)
		}
	}
	codexPolicy(t, f, "project", "enforce", 0)
	if _, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, h.start(preview)); err == nil || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("expert SDK start bypassed enforce or started partial batch", err)
	}
}

func TestOpenCodeAccountingPreservesPartialUsageWithoutInventingBilling(t *testing.T) {
	f, token, req, p := openCodeExecutionFixture(t)
	ctx := context.Background()
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	wire := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(wire["assignment"])), &a) != nil || a.RunID != run.ID {
		t.Fatal(wire)
	}
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, Spec: codexPlanNode(t, plan)}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: p.Profile, SessionID: "ses_original", Terminal: "completed", UsageState: tc.UsagePartial, Usage: &tc.Usage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}, OpenCodeExecution: &tc.OpenCodeExecutionEvidence{ConfigurationSHA256: p.Profile.OpenCodeExecution.ConfigurationSHA256, SessionID: "ses_original", UserMessageID: "msg_user", AssistantMessageID: "msg_assistant", RequestedProvider: "owned", RequestedModel: "model-a", AcknowledgedProvider: "owned", AcknowledgedModel: "model-a", Finish: "stop", CompletedAt: "2026-10-28T00:00:00Z", ServerStopped: true}}}
	for i := 0; i < 2; i++ {
		tx, err := f.s.DB.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err := f.s.RecordResultTx(ctx, tx, ref, result); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatal(page, err)
	}
	usage := page.Items[0]
	if usage.ReportedTokens == nil || *usage.ReportedTokens != 40 || usage.UsageState != tc.UsagePartial || usage.State != "partial" || usage.Model != "" || usage.Rate != nil || usage.EstimatedCostMicros != nil || usage.Version != 2 {
		t.Fatal("partial SDK observations became complete billing or settled twice", usage)
	}
}
