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

func octopTestProfile() tc.Profile {
	return tc.Profile{CommandID: "octop-acp", Adapter: "octop", Protocol: tc.OctopProtocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: tc.OctopHarnessVersion, WorkspaceToolsVersion: "attempt-workspace-v1", OctopExecution: &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion, ProviderID: "owned", Model: "model-a", EndpointOrigin: "https://models.example", ConfigurationSHA256: strings.Repeat("d", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}}
}

func octopExecutionFixture(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile) {
	t.Helper()
	f, token, raw, _ := taskDispatchFixture(t)
	p, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Fixed Octop ACP", Profile: octopTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, WorkspaceObservationProtocol: "workspace-observation-v1", WorkspaceWriteProtocol: "workspace-write-v1"}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(context.Background(), f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal("fixed local ACP tool was not eligible", p, err)
	}
	var req TaskDispatchPreviewRequest
	if err = json.Unmarshal([]byte(stringify(raw)), &req); err != nil {
		t.Fatal(err)
	}
	req.ToolProfileID, req.ExpectedToolVersion = p.ID, p.Version
	f.e.AttemptAccounting = f.s
	return f, token, req, p
}

func TestOctopControlFixedConfigurationAndEnforcedBudget(t *testing.T) {
	f, token, req, p := octopExecutionFixture(t)
	ctx := context.Background()
	options := f.call("GET", "/model-execution/options?project_id="+f.project+"&tool_profile_id="+p.ID+"&expected_tool_version="+strconv.Itoa(p.Version), nil, 200)
	fixed, _ := options["fixed_octop"].(map[string]any)
	if options["supported"] != false || options["codex_supported"] != false || options["reason"] != "fixed_by_tool" || fixed["model"] != "model-a" || fixed["acp_sdk_version"] != tc.OctopACPSDKVersion {
		t.Fatal("fixed profile was silently treated as selectable native model", options)
	}
	org := codexPolicy(t, f, "organization", "enforce", 0)
	if _, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req); err == nil {
		t.Fatal("enforce admitted observe-only preview")
	}
	org = codexPolicy(t, f, "organization", "observe", org.Version)
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil || plan.OctopExecution == nil || *plan.OctopExecution != *p.Profile.OctopExecution || plan.ModelExecution != nil || plan.CodexExecution != nil || plan.OpenCodeExecution != nil {
		t.Fatal("fixed original plan lost ACP identity", plan, err)
	}
	n := codexPlanNode(t, plan)
	if !octopNodeMatchesProfile(n, p) || n.MaxRetries != 0 {
		t.Fatal("reviewed node did not bind exact ACP profile", n)
	}
	project := codexPolicy(t, f, "project", "enforce", 0)
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start); err == nil || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("changed enforced budget admitted Start", err)
	}
	codexPolicy(t, f, "project", "observe", project.Version)
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	org = codexPolicy(t, f, "organization", "enforce", org.Version)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	if claim["assignment"] != nil || expertCount(t, f, "suma_usage_attempts") != 0 {
		t.Fatal("enforced policy consumed an attempt", claim)
	}
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: run.ID, NodeID: plan.AgentNodeID, WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: n}
	allowed, checkErr := f.s.CheckAttemptTx(ctx, tx, ref)
	reserveErr := f.s.ReserveAttemptTx(ctx, tx, ref)
	tx.Rollback()
	if checkErr != nil || allowed || reserveErr == nil {
		t.Fatal("atomic accounting manufactured a hard budget", allowed, checkErr, reserveErr)
	}
	codexPolicy(t, f, "organization", "observe", org.Version)
	claim = f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &a) != nil || a.RunID != run.ID {
		t.Fatal("eligible original run was not claimed", claim)
	}
	actual, err := tc.DecodeOctopNodeConfiguration(a.Config)
	if err != nil || actual == nil || *actual != *p.Profile.OctopExecution {
		t.Fatal("claim changed reviewed configuration", actual, err)
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Rate != nil || page.Items[0].ReservedTokens != 0 || page.Items[0].EstimatedCostMicros != nil {
		t.Fatal("configuration ACK was priced as provider identity", page, err)
	}
}

func TestOctopControlRejectsReviewedSnapshotDriftAndModelOverrides(t *testing.T) {
	for _, kind := range []string{"snapshot-model", "missing-node", "retry", "native-limits", "resume"} {
		t.Run(kind, func(t *testing.T) {
			f, _, req, _ := octopExecutionFixture(t)
			ctx := context.Background()
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
			if err != nil {
				t.Fatal(err)
			}
			wire := candidateJSON(t, plan)
			nodes := wire["workflow"].(map[string]any)["nodes"].([]any)
			var node map[string]any
			for _, item := range nodes {
				v := item.(map[string]any)
				if v["id"] == plan.AgentNodeID {
					node = v
					break
				}
			}
			switch kind {
			case "snapshot-model":
				wire["octop_execution"].(map[string]any)["model"] = "other"
			case "missing-node":
				delete(node["config"].(map[string]any), "octop_execution")
			case "retry":
				node["max_retries"] = 1
			case "native-limits":
				node["config"].(map[string]any)["max_steps"] = 2
			case "resume":
				node["config"].(map[string]any)["resume"] = "another-session"
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(wire), plan.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil || expertCount(t, f, "dag_runs") != 0 {
				t.Fatal("modified reviewed snapshot started a run", kind, err)
			}
		})
	}
}
