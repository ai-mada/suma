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

// This is an explicit protocol fixture, not discovery of a real Claude install
// or evidence that a vendor account/model is usable on this host.
func claudeTestProfile() tc.Profile {
	return tc.Profile{CommandID: "claude-code-cli", Adapter: "claude", Protocol: tc.ClaudeProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: tc.ClaudeCLIVersion, ClaudeExecution: &tc.ClaudeExecutionCapability{Model: "claude-sonnet-5", ModelSource: "administrator_cli_argument", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "existing_cli_login"}}
}

func claudeExecutionFixture(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile) {
	t.Helper()
	f, token, raw, _ := taskDispatchFixture(t)
	ctx := context.Background()
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Fixed Claude CLI protocol fixture", Profile: claudeTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal("fixed CLI profile unavailable", err)
	}
	var req TaskDispatchPreviewRequest
	if json.Unmarshal([]byte(stringify(raw)), &req) != nil {
		t.Fatal("invalid base fixture")
	}
	req.ToolProfileID, req.ExpectedToolVersion = p.ID, p.Version
	f.e.AttemptAccounting = f.s
	return f, token, req, p
}
func claudeFactMatches(raw any, c *tc.ClaudeExecutionCapability) bool {
	var got tc.ClaudeExecutionCapability
	return c != nil && json.Unmarshal([]byte(stringify(raw)), &got) == nil && got == *c
}

func TestClaudeControlPrivateFrozenModelAndUnpricedUsage(t *testing.T) {
	f, token, req, p := claudeExecutionFixture(t)
	ctx := context.Background()
	options := f.call("GET", "/model-execution/options?project_id="+f.project+"&tool_profile_id="+p.ID+"&expected_tool_version="+strconv.Itoa(p.Version), nil, 200)
	if options["supported"] != false || options["codex_supported"] != false || !claudeFactMatches(options["fixed_claude"], p.Profile.ClaudeExecution) || options["reason"] != "fixed_by_tool" || len(options["items"].([]any)) != 0 {
		t.Fatal("CLI configuration became gateway/model selection", options)
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	n := codexPlanNode(t, plan)
	if plan.ClaudeExecution == nil || *plan.ClaudeExecution != *p.Profile.ClaudeExecution || !claudeFactMatches(n.Config["claude_execution"], p.Profile.ClaudeExecution) || n.MaxRetries != 0 || plan.ModelExecution != nil || plan.CodexExecution != nil || plan.OpenCodeExecution != nil {
		t.Fatal("dispatch lost fixed CLI identity or invented a native binding")
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	reply := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID != run.ID || !claudeFactMatches(a.Config["claude_execution"], p.Profile.ClaudeExecution) {
		t.Fatal("private claim lost original CLI configuration", reply)
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Rate != nil || page.Items[0].ReservedTokens != 0 || page.Items[0].EstimatedCostMicros != nil {
		t.Fatal("CLI configuration created a rate or hard allowance", page, err)
	}
	input, output := int64(7), int64(3)
	evidence := tc.Evidence{Profile: p.Profile, SessionID: "fixture-cli-session", Terminal: "unknown", EventCount: 3, Usage: &tc.Usage{InputTokens: &input, OutputTokens: &output}, UsageState: tc.UsagePartial, ClaudeExecution: &tc.ClaudeExecutionEvidence{ConfigurationSHA256: p.Profile.ClaudeExecution.ConfigurationSHA256, SessionID: "fixture-cli-session", RequestedModel: p.Profile.ClaudeExecution.Model, AcknowledgedModel: p.Profile.ClaudeExecution.Model, ResultSubtype: "error_during_execution"}}
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, Spec: n}
	if err = f.s.RecordResultTx(ctx, tx, ref, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, OutcomeUnknown: true, ToolEvidence: &evidence}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].UsageState != tc.UsagePartial || page.Items[0].Usage == nil || *page.Items[0].Usage.InputTokens != 7 || page.Items[0].State != "reconciling" || page.Items[0].EstimatedCostMicros != nil {
		t.Fatal("unknown CLI erased usage or asserted billing", page, err)
	}
}

func TestClaudeControlTwoLevelBudgetRejectsEachAdmissionPhase(t *testing.T) {
	for _, scope := range []string{"organization", "project"} {
		for _, phase := range []string{"preview", "start", "claim"} {
			t.Run(scope+"_"+phase, func(t *testing.T) {
				f, token, req, p := claudeExecutionFixture(t)
				ctx := context.Background()
				var plan TaskDispatchPlan
				var run dag.Run
				var err error
				if phase != "preview" {
					plan, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
					if err != nil {
						t.Fatal(err)
					}
				}
				if phase == "claim" {
					run, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
					if err != nil {
						t.Fatal(err)
					}
				}
				codexPolicy(t, f, scope, "enforce", 0)
				switch phase {
				case "preview":
					if _, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req); err == nil {
						t.Fatal("enforce admitted CLI preview")
					}
				case "start":
					if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil {
						t.Fatal("enforce admitted CLI start")
					}
				case "claim":
					reply := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
					if reply["assignment"] != nil {
						t.Fatal("enforce acquired CLI attempt")
					}
					current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
					if e != nil || current.Nodes[0].Attempt != 0 {
						t.Fatal("denied CLI claim consumed an attempt", e)
					}
					tx, e := f.s.DB.BeginTx(ctx, nil)
					if e != nil {
						t.Fatal(e)
					}
					defer tx.Rollback()
					ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: run.ID, NodeID: plan.AgentNodeID, WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: codexPlanNode(t, plan)}
					allowed, e := f.s.CheckAttemptTx(ctx, tx, ref)
					if e != nil || allowed {
						t.Fatal("atomic accounting check bypassed enforce", e)
					}
					if e = f.s.ReserveAttemptTx(ctx, tx, ref); e == nil {
						t.Fatal("atomic accounting reserve bypassed enforce")
					}
					if e = tx.Rollback(); e != nil {
						t.Fatal(e)
					}
				}
				if expertCount(t, f, "suma_usage_attempts") != 0 || phase != "claim" && expertCount(t, f, "dag_runs") != 0 {
					t.Fatal("rejection wrote a run or financial hold")
				}
			})
		}
	}
}

func TestClaudeControlRejectsReviewedConfigurationDrift(t *testing.T) {
	for _, kind := range []string{"workflow_model", "snapshot_model", "missing_node_binding", "retry", "null_permission_override"} {
		t.Run(kind, func(t *testing.T) {
			f, _, req, _ := claudeExecutionFixture(t)
			ctx := context.Background()
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
			if err != nil {
				t.Fatal(err)
			}
			wire := candidateJSON(t, plan)
			flow := wire["workflow"].(map[string]any)
			var node map[string]any
			for _, item := range flow["nodes"].([]any) {
				v := item.(map[string]any)
				if v["id"] == plan.AgentNodeID {
					node = v
					break
				}
			}
			config := node["config"].(map[string]any)
			switch kind {
			case "workflow_model":
				config["claude_execution"].(map[string]any)["model"] = "claude-opus-5-5"
			case "snapshot_model":
				wire["claude_execution"].(map[string]any)["model"] = "claude-opus-5-5"
			case "missing_node_binding":
				delete(config, "claude_execution")
			case "retry":
				node["max_retries"] = 1
			case "null_permission_override":
				config["permission_mode"] = nil
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(wire), plan.ID); err != nil {
				t.Fatal(err)
			}
			current, err := f.s.GetTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID)
			if err != nil || current.Valid {
				t.Fatal("drifted CLI snapshot remained valid", err)
			}
			if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil || expertCount(t, f, "dag_runs") != 0 {
				t.Fatal("drifted CLI preview started")
			}
		})
	}
}

func TestClaudeControlPoolUsesSameFixedProfileAndBudgetAuthority(t *testing.T) {
	h := newPoolMachineFixture(t)
	f, ctx := h.f, context.Background()
	f.e.AttemptAccounting = f.s
	runner := h.credential.Runner
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Pool Claude CLI protocol fixture", Profile: claudeTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/tool-inventory", h.credential.Token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}, 200)
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal(err)
	}
	flow, profile, task := taskDispatchBuildFixture()
	profile = p.Profile
	flow, err = buildTaskDispatchWorkflow(flow, "develop", p.ID, profile, task)
	if err != nil {
		t.Fatal(err)
	}
	flow.Version = 1
	published, err := f.e.Publish(ctx, f.user.TenantID, f.project, flow)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, published.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	claim := dag.PoolClaimRequest{PoolID: h.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + p.Profile.CommandID}}}}
	for _, scope := range []string{"organization", "project"} {
		policy := codexPolicy(t, f, scope, "enforce", 0)
		claim.OperationID = newID()
		reply := f.bearer("POST", "/pool-runner/claim", h.credential.Token, claim, 200)
		if reply["assignment"] != nil || expertCount(t, f, "suma_usage_attempts") != 0 {
			t.Fatal("pool inherited unenforced native budget")
		}
		codexPolicy(t, f, scope, "observe", policy.Version)
	}
	claim.OperationID = newID()
	reply := f.bearer("POST", "/pool-runner/claim", h.credential.Token, claim, 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID != run.ID || a.PoolID != h.pool.ID || a.PoolGrantVersion != h.grants[0].Version || !claudeFactMatches(a.Config["claude_execution"], p.Profile.ClaudeExecution) {
		t.Fatal("pool claim replaced fixed CLI scope/config", reply)
	}
}
