package platform

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func acpDispatchFixture(t *testing.T) (*fixture, string, TaskDispatchPreviewRequest, ToolProfile) {
	t.Helper()
	f, token, raw, _ := taskDispatchFixture(t)
	ctx := context.Background()
	profile := tc.Profile{CommandID: "qoder-fixed170", Adapter: "qoder", Protocol: ac.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "Qoder reviewed local fixture170", ACPExecution: &ac.Capability{Runtime: "qoder", ProtocolVersion: 1, SDKVersion: ac.SDKVersion, ConfigurationSHA256: strings.Repeat("c", 64), Model: "fixed-model170", ModelSource: "administrator_session_config", BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "existing_cli_login", PermissionMode: "human_once"}}
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Controlled ACP dispatch fixture", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || !p.Available {
		t.Fatal(err)
	}
	var req TaskDispatchPreviewRequest
	if json.Unmarshal([]byte(stringify(raw)), &req) != nil {
		t.Fatal("base fixture invalid")
	}
	req.ToolProfileID, req.ExpectedToolVersion = p.ID, p.Version
	f.e.AttemptAccounting = f.s
	return f, token, req, p
}

func TestACPDispatchKeepsFrozenModelAndOriginalClaim(t *testing.T) {
	f, token, req, p := acpDispatchFixture(t)
	ctx := context.Background()
	options := f.call("GET", "/model-execution/options?project_id="+f.project+"&tool_profile_id="+p.ID+"&expected_tool_version="+strconv.Itoa(p.Version), nil, 200)
	var fixed ac.Capability
	if json.Unmarshal([]byte(stringify(options["fixed_acp"])), &fixed) != nil || fixed != *p.Profile.ACPExecution || options["supported"] != false || options["codex_supported"] != false || len(options["items"].([]any)) != 0 {
		t.Fatal("fixed ACP configuration became a gateway/model-health claim")
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	node := codexPlanNode(t, plan)
	if plan.ACPExecution == nil || *plan.ACPExecution != fixed || !acpNodeMatchesProfile(node, p) || plan.ModelExecution != nil || plan.ClaudeExecution != nil || plan.CodexExecution != nil {
		t.Fatal("plan lost exact ACP source")
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	reply := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment) != nil || assignment.RunID != run.ID {
		t.Fatal("current original claim unavailable")
	}
	actual, err := tc.DecodeACPNodeConfiguration(assignment.Config)
	if err != nil || actual == nil || *actual != fixed {
		t.Fatal("claim changed fixed model", err)
	}
}

func TestACPEnforcedBudgetRejectsEveryAdmissionBoundary(t *testing.T) {
	for _, scope := range []string{"organization", "project"} {
		for _, phase := range []string{"preview", "start", "claim"} {
			t.Run(scope+"_"+phase, func(t *testing.T) {
				f, token, req, p := acpDispatchFixture(t)
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
					_, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
					if err == nil {
						t.Fatal("enforce admitted ACP preview")
					}
				case "start":
					_, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
					if err == nil {
						t.Fatal("enforce admitted ACP start")
					}
				case "claim":
					reply := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
					if reply["assignment"] != nil {
						t.Fatal("enforce acquired an ACP attempt")
					}
					tx, e := f.s.DB.BeginTx(ctx, nil)
					if e != nil {
						t.Fatal(e)
					}
					defer tx.Rollback()
					ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: run.ID, NodeID: plan.AgentNodeID, WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: codexPlanNode(t, plan)}
					allowed, e := f.s.CheckAttemptTx(ctx, tx, ref)
					if e != nil || allowed {
						t.Fatal("atomic check bypassed enforce", e)
					}
					if e = f.s.ReserveAttemptTx(ctx, tx, ref); e == nil {
						t.Fatal("atomic reservation bypassed enforce")
					}
					// Release the one-connection fixture transaction before the
					// independent post-condition reads acquire that connection.
					if e = tx.Rollback(); e != nil {
						t.Fatal(e)
					}
				}
				if expertCount(t, f, "suma_usage_attempts") != 0 || phase != "claim" && expertCount(t, f, "dag_runs") != 0 {
					t.Fatal("rejected admission wrote execution/accounting state")
				}
			})
		}
	}
}
