package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// This is a control-plane protocol fixture, not an installed Cursor or model.
func cursorACPProfile189() tc.Profile {
	return tc.Profile{CommandID: "cursor-fixed189", Adapter: "cursor", Protocol: ac.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "Controlled Cursor ACP fixture189", ACPExecution: &ac.Capability{Runtime: "cursor", ProtocolVersion: 1, SDKVersion: ac.SDKVersion, ConfigurationSHA256: strings.Repeat("c", 64), Model: "fixed-cursor-model189", ModelSource: "administrator_session_config", BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "explicit_api_key_env", PermissionMode: "human_once"}}
}

func cursorACPInventory189(t *testing.T, h toolPermissionFixture, profile tc.Profile, available bool) {
	t.Helper()
	item := tc.Observation{Profile: profile, Available: available}
	if !available {
		item.Reason = "Controlled unavailable Cursor fixture"
	}
	if h.pool {
		h.f.bearer("POST", "/pool-runner/tool-inventory", h.token, PoolToolInventoryRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.f.user.TenantID, ProjectID: h.f.project, OperationID: newID(), Items: []tc.Observation{item}}, 200)
	} else if _, err := h.f.s.ReportToolInventory(context.Background(), h.actor, h.f.project, ToolInventoryRequest{ProjectID: h.f.project, WorkerID: h.profile.WorkerID, OperationID: newID(), Items: []tc.Observation{item}}); err != nil {
		t.Fatal(err)
	}
}

func cursorACPRegister189(t *testing.T, h toolPermissionFixture) toolPermissionFixture {
	t.Helper()
	var err error
	h.profile, err = h.f.s.CreateToolProfile(context.Background(), h.f.actor, h.f.project, ToolProfileCreateRequest{ProjectID: h.f.project, WorkerID: h.profile.WorkerID, Name: "Explicit Cursor ACP fixture", Profile: cursorACPProfile189(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	cursorACPInventory189(t, h, h.profile.Profile, true)
	h.profile, err = h.f.s.CommandToolProfile(context.Background(), h.f.actor, h.f.project, h.profile.ID, ToolProfileCommandRequest{ProjectID: h.f.project, ExpectedVersion: h.profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !h.profile.Available {
		t.Fatal(err)
	}
	return h
}

func cursorACPFixture189(t *testing.T, pooled bool) (toolPermissionFixture, TaskDispatchPreviewRequest) {
	t.Helper()
	h := toolPermissionFixture{pool: pooled}
	if pooled {
		p := newPoolMachineFixture(t)
		h.f, h.token = p.f, p.credential.Token
		h.profile.WorkerID = p.credential.Runner.WorkerID
		h.assignment = dag.Assignment{PoolID: p.pool.ID, PoolRunnerID: p.credential.Runner.ID}
	} else {
		h.f = newFixture(t)
		h.profile.WorkerID = "cursor-worker189"
		h.token = h.f.call("POST", "/release-workers", h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
		h.actor = toolActorForTest(t, h.f, h.token)
	}
	h = cursorACPRegister189(t, h)
	f, ctx := h.f, context.Background()
	w, _, _ := taskDispatchBuildFixture()
	w.ID, w.Version = "", 0
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, w)
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Original fixed Cursor development", "acceptance_criteria": []any{"Independent candidate tests and human review"}}))
	if err != nil {
		t.Fatal(err)
	}
	f.e.AttemptAccounting = f.s
	return h, TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: w.ID, WorkflowVersion: w.Version, AgentNodeID: "develop", ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version, IdempotencyKey: newID()}
}

func cursorACPClaim189(t *testing.T, h toolPermissionFixture) map[string]any {
	t.Helper()
	if h.pool {
		return h.f.bearer("POST", "/pool-runner/claim", h.token, dag.PoolClaimRequest{PoolID: h.assignment.PoolID, RunnerID: h.assignment.PoolRunnerID, WorkerID: h.profile.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: h.f.user.TenantID, ProjectID: h.f.project, Capabilities: []string{"command:" + h.profile.Profile.CommandID}}}}, 200)
	}
	return h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}, "idempotency_key": newID()}), 200)
}

func TestCursorACPOriginalDispatchCurrentInventoryPermissionAndResult(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint(pooled), func(t *testing.T) {
			h, request := cursorACPFixture189(t, pooled)
			f, ctx := h.f, context.Background()
			options, err := f.s.GetModelExecutionOptions(ctx, f.actor, f.project, ModelExecutionOptionsRequest{ProjectID: f.project, ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version})
			if err != nil || options.FixedACP == nil || *options.FixedACP != *h.profile.Profile.ACPExecution || options.Reason != "fixed_by_tool" || options.Supported || options.CodexSupported || len(options.Items) != 0 {
				t.Fatal(options, err)
			}
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
			if err != nil || plan.ACPExecution == nil || *plan.ACPExecution != *options.FixedACP || plan.ModelExecution != nil || plan.CodexExecution != nil || !acpNodeMatchesProfile(codexPlanNode(t, plan), h.profile) {
				t.Fatal(plan, err)
			}
			changed := tc.CloneProfile(h.profile.Profile)
			changed.ACPExecution.Model = "different-cursor-model189"
			cursorACPInventory189(t, h, changed, true)
			start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
			if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start); err == nil {
				t.Fatal("changed inventory started old fixed model")
			}
			cursorACPInventory189(t, h, h.profile.Profile, true)
			run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
			if err != nil {
				t.Fatal(err)
			}
			cursorACPInventory189(t, h, h.profile.Profile, false)
			if reply := cursorACPClaim189(t, h); reply["assignment"] != nil {
				t.Fatal("unavailable current inventory acquired attempt", reply)
			}
			cursorACPInventory189(t, h, h.profile.Profile, true)
			reply := cursorACPClaim189(t, h)
			if json.Unmarshal([]byte(stringify(reply["assignment"])), &h.assignment) != nil || h.assignment.RunID != run.ID {
				t.Fatal(reply)
			}
			fixed, err := tc.DecodeACPNodeConfiguration(h.assignment.Config)
			if err != nil || fixed == nil || *fixed != *options.FixedACP {
				t.Fatal(fixed, err)
			}
			wrong := h
			wrong.assignment.Fence++
			if _, err = wrong.open(ctx, permissionRequest170()); err == nil {
				t.Fatal("wrong fence requested Cursor permission")
			}
			permission, err := h.open(ctx, permissionRequest170())
			if err != nil {
				t.Fatal(err)
			}
			permission, err = f.s.DecideToolPermission(ctx, f.actor, permission.ID, pc.DecisionRequest{ProjectID: f.project, ExpectedVersion: permission.Version, IdempotencyKey: newID(), Choice: permissionChoice170("once")})
			if err != nil || permission.Decision == nil || permission.Decision.ACP.Kind != "allow_once" {
				t.Fatal(permission, err)
			}
			if read, e := h.poll(ctx, permission.ID); e != nil || read.Decision == nil || read.Decision.ACP.OptionID != "once" {
				t.Fatal(read, e)
			}
			result := dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, SessionID: "session170", Terminal: "completed", EventCount: 4, UsageState: tc.UsagePartial, Usage: &tc.Usage{InputTokens: usageInt(2), OutputTokens: usageInt(3)}, ACPExecution: &ac.Evidence{ConfigurationSHA256: fixed.ConfigurationSHA256, RequestedModel: fixed.Model, SelectedModel: fixed.Model, ModelSelectionAcknowledged: true, SessionID: "session170", ProtocolVersion: 1, AgentName: "Controlled Cursor", AgentVersion: "fixture189", PromptStarted: true, PromptCompleted: true, StopReason: "end_turn", ToolsComplete: true, PermissionsRequested: 1, HostProcessStopped: true, HostProcessTreeStopped: true}}}
			post := func(r dag.Result, status int) {
				if pooled {
					f.bearer("POST", "/pool-runner/results", h.token, PoolRunnerResultRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Result: r}, status)
				} else {
					f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": r}), status)
				}
			}
			bad := result
			bad.ToolEvidence = &tc.Evidence{}
			*bad.ToolEvidence = *result.ToolEvidence
			proof := *result.ToolEvidence.ACPExecution
			proof.SelectedModel = "unapproved-model189"
			bad.ToolEvidence.ACPExecution = &proof
			post(bad, 400)
			post(result, 200)
			post(result, 200)
			ledger, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 20, "")
			if err != nil || len(ledger.Items) != 1 || ledger.Items[0].UsageState != tc.UsagePartial || ledger.Items[0].Rate != nil || ledger.Items[0].ReservedTokens != 0 || ledger.Items[0].ModelExecution != nil || ledger.Items[0].CodexExecution != nil {
				t.Fatal(ledger, err)
			}
		})
	}
}

func TestCursorACPDiscussionAndExpertReuseFixedDevelopmentInput(t *testing.T) {
	t.Run("discussion", func(t *testing.T) {
		h, request := taskDiscussionDevelopmentFixture(t)
		f, ctx := h.f, context.Background()
		worker := "cursor-discussion189"
		token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
		machine := cursorACPRegister189(t, toolPermissionFixture{f: f, token: token, actor: toolActorForTest(t, f, token), profile: ToolProfile{WorkerID: worker}})
		request.ToolProfileID, request.ExpectedToolVersion = machine.profile.ID, machine.profile.Version
		request.ModelExecutionSelection, request.CodexExecutionSelection = nil, nil
		plan, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
		if err != nil || plan.DispatchPlan.ACPExecution == nil || *plan.DispatchPlan.ACPExecution != *machine.profile.Profile.ACPExecution || plan.DispatchPlan.ModelExecution != nil || plan.DispatchPlan.CodexExecution != nil || plan.Message.ActorKind != "assistant" || plan.Message.ReplyID != request.ReplyID || plan.DispatchPlan.Workflow.Nodes[1].Config["candidate_from"] != "develop" {
			t.Fatal(plan, err)
		}
		if _, err = f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("expert", func(t *testing.T) {
		h := newExpertFixture(t, false)
		f, ctx := h.f, context.Background()
		machine := cursorACPRegister189(t, toolPermissionFixture{f: f, token: h.runnerToken, actor: toolActorForTest(t, f, h.runnerToken), profile: ToolProfile{WorkerID: "tool-worker"}})
		h.role.ToolProfileID, h.role.ExpectedToolVersion = machine.profile.ID, machine.profile.Version
		var err error
		h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Review fixed Cursor ACP source", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		plan, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, h.request())
		if err != nil || len(plan.Items) != 2 {
			t.Fatal(plan, err)
		}
		for _, item := range plan.Items {
			if item.Plan.ACPExecution == nil || *item.Plan.ACPExecution != *machine.profile.Profile.ACPExecution || item.Plan.ModelExecution != nil || item.Plan.CodexExecution != nil || !acpNodeMatchesProfile(codexPlanNode(t, item.Plan), machine.profile) {
				t.Fatal(item)
			}
		}
	})
}

func TestCursorACPEnforceRejectsOriginalAdmissionBoundaries(t *testing.T) {
	for _, phase := range []string{"preview", "start", "claim"} {
		t.Run(phase, func(t *testing.T) {
			h, request := cursorACPFixture189(t, false)
			f, ctx := h.f, context.Background()
			var plan TaskDispatchPlan
			var err error
			if phase != "preview" {
				plan, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
				if err != nil {
					t.Fatal(err)
				}
			}
			if phase == "claim" {
				if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}); err != nil {
					t.Fatal(err)
				}
			}
			scope := "organization"
			if phase == "start" {
				scope = "project"
			}
			codexPolicy(t, f, scope, "enforce", 0)
			switch phase {
			case "preview":
				_, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
			case "start":
				_, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
			case "claim":
				if reply := cursorACPClaim189(t, h); reply["assignment"] != nil {
					t.Fatal("enforce acquired Cursor attempt", reply)
				}
				return
			}
			if err == nil || expertCount(t, f, "suma_usage_attempts") != 0 || expertCount(t, f, "dag_runs") != 0 {
				t.Fatal("enforce admitted original Cursor execution", err)
			}
		})
	}
}
