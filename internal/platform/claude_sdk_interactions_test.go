package platform

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	"github.com/ai-mada/suma/internal/dag"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

// This is an explicit local domain fixture, never a vendor installation,
// account, model call or native callback execution claim.
func claudeSDKProfile192() tc.Profile {
	return tc.Profile{CommandID: "claude-sdk192", Adapter: "claude", Protocol: cl.Protocol, UserInputProtocol: cl.InputProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: cl.CLIVersion, ClaudeSDKExecution: &tc.ClaudeSDKExecutionCapability{SDKVersion: cl.SDKVersion, CLIVersion: cl.CLIVersion, Model: "claude-sonnet-5", ConfigurationSHA256: strings.Repeat("c", 64), ModelSource: "administrator_sdk_option", BudgetMode: "observe_only", IdentityEvidence: "sdk_init_model_ack_only", AuthStrategy: "api_key_env_ref"}}
}

func claudeSDKInteraction192(t *testing.T, pooled bool) toolPermissionFixture {
	t.Helper()
	ctx := context.Background()
	h := toolPermissionFixture{pool: pooled}
	worker, runnerID, poolID := "claude-sdk-worker192", "", ""
	if pooled {
		p := newPoolMachineFixture(t)
		h.f, h.token = p.f, p.credential.Token
		worker, runnerID, poolID = p.credential.Runner.WorkerID, p.credential.Runner.ID, p.pool.ID
	} else {
		h.f = newFixture(t)
		w := h.f.call("POST", "/release-workers", h.f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)
		h.token = w["token"].(string)
		h.actor = toolActorForTest(t, h.f, h.token)
	}
	f, profile := h.f, claudeSDKProfile192()
	var err error
	h.profile, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Owned Claude SDK source fixture", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	observation := tc.Observation{Profile: profile, Available: true}
	if pooled {
		f.bearer("POST", "/pool-runner/tool-inventory", h.token, PoolToolInventoryRequest{RunnerID: runnerID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{observation}}, 200)
	} else if _, err = f.s.ReportToolInventory(ctx, h.actor, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{observation}}); err != nil {
		t.Fatal(err)
	}
	h.profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, h.profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: h.profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !h.profile.Available {
		t.Fatal("SDK fixture profile unavailable", err)
	}
	f.e.AttemptAccounting = f.s
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Original Claude SDK interaction scope", Nodes: []dag.Node{{ID: "work", Type: "agent", Command: []string{profile.CommandID}, Config: map[string]any{"goal": "Use the original explicitly reviewed callback decisions", "tool_profile_id": h.profile.ID, "tool_profile_sha256": profile.ProfileSHA256, "claude_sdk_execution": issueRepairRuntimeDecode[map[string]any](t, profile.ClaudeSDKExecution)}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	var claim map[string]any
	if pooled {
		claim = f.bearer("POST", "/pool-runner/claim", h.token, dag.PoolClaimRequest{PoolID: poolID, RunnerID: runnerID, WorkerID: worker, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}, 200)
	} else {
		claim = f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": worker, "capabilities": []string{"command:" + profile.CommandID}}), 200)
	}
	h.assignment = issueRepairRuntimeDecode[dag.Assignment](t, claim["assignment"])
	if h.assignment.RunID != run.ID {
		t.Fatal("original SDK assignment not claimed", claim)
	}
	return h
}

func claudeBasis192(h toolPermissionFixture, id, raw string) cl.Basis {
	return cl.Basis{SessionID: "session192", ToolUseID: "tool-" + id, RequestID: "native-" + id, CallbackID: "host-" + id, ConfigurationSHA256: h.profile.Profile.ClaudeSDKExecution.ConfigurationSHA256, ParamsSHA256: cl.PayloadDigest(raw)}
}

func claudePermission192(h toolPermissionFixture, id string) pc.Request {
	raw := `{"command":"git status"}`
	return pc.Request{IdempotencyKey: newID(), Protocol: cl.Protocol, Claude: &cl.PermissionSource{Basis: claudeBasis192(h, id, raw), ToolName: "Bash", InputJSON: raw}}
}

func claudeObservePermission192(t *testing.T, h toolPermissionFixture, id, key string, o delivery.Observation) pc.Permission {
	t.Helper()
	var out map[string]any
	if h.pool {
		out = h.f.bearer("POST", "/pool-runner/tool-permissions/observations", h.token, PoolToolPermissionObservationRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: h.f.project, Assignment: h.assignment}, PermissionID: id, IdempotencyKey: key, ClaudeObservation: &o}, 200)
	} else {
		out = h.f.bearer("POST", "/runner/tool-permissions/observations", h.token, ToolPermissionObservationRequest{ProjectID: h.f.project, Assignment: h.assignment, PermissionID: id, IdempotencyKey: key, ClaudeObservation: &o}, 200)
	}
	return issueRepairRuntimeDecode[pc.Permission](t, out)
}

func TestClaudeSDKInteractionsKeepNativeSourcePrivatePoolAndResultAuthority(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint(pooled), func(t *testing.T) {
			h := claudeSDKInteraction192(t, pooled)
			f, ctx := h.f, context.Background()
			r := claudePermission192(h, "permission")
			permission, err := h.open(ctx, r)
			if err != nil {
				t.Fatal(err)
			}
			choice := pc.Choice{Protocol: cl.Protocol, Claude: &cl.PermissionChoice{Outcome: "allow_once"}}
			decision := pc.DecisionRequest{Choice: choice, ProjectID: f.project, ExpectedVersion: permission.Version, IdempotencyKey: newID()}
			permission = issueRepairRuntimeDecode[pc.Permission](t, f.call("POST", "/tool-permissions/"+permission.ID+"/decision", decision, 200))
			answerSHA, _ := cl.PermissionDecisionDigest(*r.Claude, *choice.Claude)
			o := delivery.Observation{Sequence: 1, ParamsSHA256: r.Claude.ParamsSHA256, AnswerSHA256: answerSHA, AnswerObserved: true, ReplyStarted: true}
			claudeObservePermission192(t, h, permission.ID, newID(), o)
			o.Sequence, o.ReplyWritten = 2, true
			claudeObservePermission192(t, h, permission.ID, newID(), o)
			o.Sequence, o.ExecutionClosed = 3, true
			key := newID()
			permission = claudeObservePermission192(t, h, permission.ID, key, o)
			// The real request ID cannot be reused with another host callback.
			r.IdempotencyKey, r.Claude.CallbackID = newID(), "replacement-host"
			if _, err = h.open(ctx, r); err == nil {
				t.Fatal("host callback bypassed native source uniqueness")
			}
			raw := `{"questions":[{"question":"Choose the implementation","header":"Design","options":[{"label":"Keep","description":"Use current modules"},{"label":"Refine","description":"Improve the interface"}],"multiSelect":false}]}`
			native, err := cl.NewInputRequest(claudeBasis192(h, "input", raw), raw)
			if err != nil {
				t.Fatal(err)
			}
			request := ic.ToolInputRequest{IdempotencyKey: newID(), Protocol: cl.Protocol, Claude: &native}
			var input ic.ToolInput
			if pooled {
				input, err = f.s.OpenPoolToolInput(ctx, h.token, f.project, h.assignment, request)
			} else {
				input, err = f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, request)
			}
			if err != nil {
				t.Fatal(err)
			}
			input = issueRepairRuntimeDecode[ic.ToolInput](t, f.call("POST", "/tool-inputs/"+input.ID+"/answer", ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: input.Version, IdempotencyKey: newID(), Claude: &cl.InputDecision{Outcome: "answered", Answers: []cl.InputSelection{{QuestionID: "q1", OptionIDs: []string{"o2"}}}}}, 200))
			io := delivery.Observation{Sequence: 1, ParamsSHA256: native.ParamsSHA256, AnswerSHA256: input.Answer.AnswerSHA256, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}
			if pooled {
				input, err = f.s.ObservePoolToolInput(ctx, h.token, f.project, h.assignment, input.ID, newID(), io)
			} else {
				input, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), io)
			}
			if err != nil {
				t.Fatal(err)
			}
			proof := tc.ClaudeSDKExecutionEvidence{SDKVersion: cl.SDKVersion, CLIVersion: cl.CLIVersion, ConfigurationSHA256: h.profile.Profile.ClaudeSDKExecution.ConfigurationSHA256, SessionID: "session192", RequestedModel: h.profile.Profile.ClaudeSDKExecution.Model, AcknowledgedModel: h.profile.Profile.ClaudeSDKExecution.Model, ResultSubtype: "success", TurnEnded: true, ToolsCompleted: true, HostProcessStopped: true, HostProcessTreeStopped: true, PermissionRequests: 1, Permissions: []cl.PermissionEvidence{{PermissionID: permission.ID, Basis: permission.Request.Claude.Basis, ToolName: permission.Request.Claude.ToolName, Observation: permission.ClaudeObservation}}, UserInputs: []cl.InputEvidence{{InputID: input.ID, Basis: native.Basis, QuestionCount: len(native.Questions), Observation: input.Observation}}}
			// This is explicitly observed fixture data, not an absent native list
			// silently replaced with zero. Original human decisions are still checked.
			nativeDenials := 0
			proof.NativePermissionDenials = &nativeDenials
			result := dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, SessionID: "session192", Terminal: "completed", EventCount: 4, UsageState: tc.UsageUnreported, ClaudeSDKExecution: &proof}}
			tx, err := f.s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err = f.s.validateClaudePermissionResultTx(ctx, tx, f.user.TenantID, f.project, result); err != nil {
				t.Fatal("original permission evidence rejected", err)
			}
			if err = f.s.validateToolInputResultTx(ctx, tx, f.user.TenantID, f.project, result); err != nil {
				t.Fatal("original question evidence rejected", err)
			}
			proof.Permissions[0].RequestID = "other-request"
			if f.s.validateClaudePermissionResultTx(ctx, tx, f.user.TenantID, f.project, result) == nil {
				t.Fatal("another native permission source borrowed evidence")
			}
			proof.Permissions[0].RequestID = permission.Request.Claude.RequestID
			proof.UserInputs[0].ToolUseID = "other-tool"
			if f.s.validateToolInputResultTx(ctx, tx, f.user.TenantID, f.project, result) == nil {
				t.Fatal("another native question source borrowed evidence")
			}
			proof.UserInputs[0].ToolUseID = native.ToolUseID
			tx.Rollback()
			// The machine may lie about aggregate denied counters. The original
			// persisted human decisions, not the local result, decide admission.
			deniedRequest := claudePermission192(h, "denied")
			denied, err := h.open(ctx, deniedRequest)
			if err != nil {
				t.Fatal(err)
			}
			denial := pc.Choice{Protocol: cl.Protocol, Claude: &cl.PermissionChoice{Outcome: "reject_once", Reason: "Keep the original workspace"}}
			denied = issueRepairRuntimeDecode[pc.Permission](t, f.call("POST", "/tool-permissions/"+denied.ID+"/decision", pc.DecisionRequest{Choice: denial, ProjectID: f.project, ExpectedVersion: denied.Version, IdempotencyKey: newID()}, 200))
			deniedSHA, _ := cl.PermissionDecisionDigest(*deniedRequest.Claude, *denial.Claude)
			deniedObs := delivery.Observation{Sequence: 1, ParamsSHA256: deniedRequest.Claude.ParamsSHA256, AnswerSHA256: deniedSHA, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}
			denied = claudeObservePermission192(t, h, denied.ID, newID(), deniedObs)
			deniedResult := issueRepairRuntimeDecode[dag.Result](t, result)
			deniedResult.ToolEvidence.ClaudeSDKExecution.PermissionRequests = 2
			deniedResult.ToolEvidence.ClaudeSDKExecution.Permissions = append(deniedResult.ToolEvidence.ClaudeSDKExecution.Permissions, cl.PermissionEvidence{PermissionID: denied.ID, Basis: deniedRequest.Claude.Basis, ToolName: deniedRequest.Claude.ToolName, Observation: denied.ClaudeObservation})
			if tc.ValidateEvidence(*deniedResult.ToolEvidence) != nil {
				t.Fatal("fixture must isolate the platform decision gate, not an invalid aggregate")
			}
			tx, err = f.s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if f.s.validateClaudePermissionResultTx(ctx, tx, f.user.TenantID, f.project, deniedResult) == nil {
				t.Error("saved human rejection was promoted by fabricated zero-denied success")
			}
			tx.Rollback()
			cancelNative, err := cl.NewInputRequest(claudeBasis192(h, "cancelled", raw), raw)
			if err != nil {
				t.Fatal(err)
			}
			cancelRequest := ic.ToolInputRequest{IdempotencyKey: newID(), Protocol: cl.Protocol, Claude: &cancelNative}
			var cancelled ic.ToolInput
			if pooled {
				cancelled, err = f.s.OpenPoolToolInput(ctx, h.token, f.project, h.assignment, cancelRequest)
			} else {
				cancelled, err = f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, cancelRequest)
			}
			if err != nil {
				t.Fatal(err)
			}
			cancelled = issueRepairRuntimeDecode[ic.ToolInput](t, f.call("POST", "/tool-inputs/"+cancelled.ID+"/answer", ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: cancelled.Version, IdempotencyKey: newID(), Claude: &cl.InputDecision{Outcome: "cancelled", Reason: "Stop this question"}}, 200))
			cancelObs := delivery.Observation{Sequence: 1, ParamsSHA256: cancelNative.ParamsSHA256, AnswerSHA256: cancelled.Answer.AnswerSHA256, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}
			if pooled {
				cancelled, err = f.s.ObservePoolToolInput(ctx, h.token, f.project, h.assignment, cancelled.ID, newID(), cancelObs)
			} else {
				cancelled, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, cancelled.ID, newID(), cancelObs)
			}
			if err != nil {
				t.Fatal(err)
			}
			cancelResult := issueRepairRuntimeDecode[dag.Result](t, result)
			cancelResult.ToolEvidence.ClaudeSDKExecution.UserInputs = append(cancelResult.ToolEvidence.ClaudeSDKExecution.UserInputs, cl.InputEvidence{InputID: cancelled.ID, Basis: cancelNative.Basis, QuestionCount: len(cancelNative.Questions), Observation: cancelled.Observation})
			if tc.ValidateEvidence(*cancelResult.ToolEvidence) != nil {
				t.Fatal("cancel fixture must isolate the platform answer gate")
			}
			tx, err = f.s.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if f.s.validateToolInputResultTx(ctx, tx, f.user.TenantID, f.project, cancelResult) == nil {
				t.Error("saved cancellation was promoted by successful local callback delivery")
			}
			tx.Rollback()
			if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
				t.Fatal(err)
			}
			replay := claudeObservePermission192(t, h, permission.ID, key, o)
			if replay.Version != permission.Version {
				t.Fatal("lost acknowledgement changed the original observation")
			}
			if pooled {
				_, err = f.s.ObservePoolClaudeToolPermission(ctx, h.token, f.project, h.assignment, permission.ID, newID(), o)
			} else {
				_, err = f.s.ObserveClaudeToolPermission(ctx, h.actor, f.project, h.assignment, permission.ID, newID(), o)
			}
			if err == nil {
				t.Fatal("new observation used an expired execution lease")
			}
		})
	}
}

func TestClaudeSDKOnceDecisionHasCASAndCurrentPauseAuthority(t *testing.T) {
	h := claudeSDKInteraction192(t, false)
	f, ctx := h.f, context.Background()
	r := claudePermission192(h, "cas")
	p, err := h.open(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, outcome := range []string{"allow_once", "reject_once"} {
		wg.Add(1)
		go func(outcome string) {
			defer wg.Done()
			_, e := f.s.DecideToolPermission(ctx, f.actor, p.ID, pc.DecisionRequest{Choice: pc.Choice{Protocol: cl.Protocol, Claude: &cl.PermissionChoice{Outcome: outcome}}, ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()})
			results <- e
		}(outcome)
	}
	wg.Wait()
	close(results)
	success, failure := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			failure++
		}
	}
	if success != 1 || failure != 1 {
		t.Fatal("concurrent decisions did not preserve one CAS winner", success, failure)
	}
	p, err = h.poll(ctx, p.ID)
	if err != nil || p.Decision == nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET state='paused' WHERE id=?`, h.assignment.RunID); err != nil {
		t.Fatal(err)
	}
	if p.Decision.Claude.Outcome == "allow_once" {
		p, err = h.poll(ctx, p.ID)
		if err != nil || p.State != "superseded" {
			t.Fatal("paused run exposed a new one-call grant", p, err)
		}
	}
	// A late human answer or callback does not turn elapsed time into renewal.
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=? WHERE run_id=? AND id=?`, time.Now().Add(-time.Minute).Unix(), h.assignment.RunID, h.assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	late := delivery.Observation{Sequence: 1, ParamsSHA256: r.Claude.ParamsSHA256, ExecutionClosed: true}
	if _, err = f.s.ObserveClaudeToolPermission(ctx, h.actor, f.project, h.assignment, p.ID, newID(), late); err == nil {
		t.Fatal("expired callback gained fresh authority")
	}
}

func claudeSDKDispatchFixture192(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile) {
	t.Helper()
	f, token, raw, _ := taskDispatchFixture(t)
	ctx := context.Background()
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Fixed Claude SDK model fixture", Profile: claudeSDKProfile192(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	req := issueRepairRuntimeDecode[TaskDispatchPreviewRequest](t, raw)
	req.ToolProfileID, req.ExpectedToolVersion = p.ID, p.Version
	f.e.AttemptAccounting = f.s
	return f, token, req, p
}

func TestClaudeSDKFrozenModelAndTwoLevelBudgets(t *testing.T) {
	for _, scope := range []string{"organization", "project"} {
		for _, phase := range []string{"preview", "start", "claim"} {
			t.Run(scope+"_"+phase, func(t *testing.T) {
				f, token, req, p := claudeSDKDispatchFixture192(t)
				ctx := context.Background()
				if scope == "organization" && phase == "preview" {
					options := f.call("GET", "/model-execution/options?project_id="+f.project+"&tool_profile_id="+p.ID+"&expected_tool_version="+strconv.Itoa(p.Version), nil, 200)
					fixed := issueRepairRuntimeDecode[tc.ClaudeSDKExecutionCapability](t, options["fixed_claude_sdk"])
					if options["supported"] != false || options["reason"] != "fixed_by_tool" || fixed != *p.Profile.ClaudeSDKExecution || options["fixed_claude"] != nil || len(options["items"].([]any)) != 0 {
						t.Fatal("SDK model configuration became CLI/gateway selection", options)
					}
				}
				var plan TaskDispatchPlan
				var run dag.Run
				var err error
				if phase != "preview" {
					plan, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
					if err != nil {
						t.Fatal(err)
					}
					if plan.ClaudeSDKExecution == nil || *plan.ClaudeSDKExecution != *p.Profile.ClaudeSDKExecution || plan.ClaudeExecution != nil || !claudeSDKNodeMatchesProfile(codexPlanNode(t, plan), p) {
						t.Fatal("dispatch changed SDK source or borrowed old CLI")
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
						t.Fatal("enforced budget admitted SDK preview")
					}
				case "start":
					if _, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}); err == nil {
						t.Fatal("enforced budget admitted SDK start")
					}
				case "claim":
					out := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
					if out["assignment"] != nil {
						t.Fatal("enforced budget claimed a SDK attempt")
					}
					current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
					if e != nil || current.Nodes[0].Attempt != 0 {
						t.Fatal("denied SDK claim consumed attempt", e)
					}
					tx, e := f.s.DB.BeginTx(ctx, nil)
					if e != nil {
						t.Fatal(e)
					}
					ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: run.ID, NodeID: plan.AgentNodeID, WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: codexPlanNode(t, plan)}
					allowed, e := f.s.CheckAttemptTx(ctx, tx, ref)
					if e != nil || allowed {
						t.Fatal("atomic SDK accounting check bypassed enforce", e)
					}
					if e = f.s.ReserveAttemptTx(ctx, tx, ref); e == nil {
						t.Fatal("atomic SDK accounting reserve bypassed enforce")
					}
					tx.Rollback()
				}
				if expertCount(t, f, "suma_usage_attempts") != 0 || phase != "claim" && expertCount(t, f, "dag_runs") != 0 {
					t.Fatal("rejection wrote run or financial reservation")
				}
			})
		}
	}
}
