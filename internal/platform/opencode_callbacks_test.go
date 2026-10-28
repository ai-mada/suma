package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func openCodeCallbackFixture(t *testing.T, pooled bool) startupHTTPFixture {
	t.Helper()
	f, token, req, p := openCodeExecutionFixture(t)
	ctx := context.Background()
	var machine ExecutionPoolCredentialResponse
	if pooled {
		pool, grant := dispatchAuthorizedPool(t, f)
		dispatchSetRoute(t, f, 0, &grant)
		var err error
		machine, err = f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "SDK callback Pool", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		token = machine.Token
		f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: machine.Runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: openCodeTestProfile(), Available: true, WorkspaceObservationProtocol: "workspace-observation-v1", WorkspaceWriteProtocol: "workspace-write-v1"}}}, 200)
		p, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.Runner.WorkerID, Name: "Fixed Pool SDK", Profile: openCodeTestProfile(), IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		req.ToolProfileID, req.ExpectedToolVersion = p.ID, p.Version
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if pooled {
		wire = f.bearer("POST", "/pool-runner/claim", token, dag.PoolClaimRequest{PoolID: machine.Runner.PoolID, RunnerID: machine.Runner.ID, WorkerID: machine.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + p.Profile.CommandID}}}}, 200)
	} else {
		wire = f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	}
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(wire["assignment"])), &a) != nil || a.RunID != run.ID {
		t.Fatal("missing SDK callback assignment", wire)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: newID(), Stopped: true, ToolEvidence: &tc.Evidence{Profile: p.Profile, SessionID: "ses_original", Terminal: "unknown", UsageState: tc.UsageUnreported, OpenCodeExecution: &tc.OpenCodeExecutionEvidence{ConfigurationSHA256: p.Profile.OpenCodeExecution.ConfigurationSHA256, SessionID: "ses_original", UserMessageID: "msg_user", RequestedProvider: "owned", RequestedModel: "model-a", ServerStopped: true, AbortAcknowledged: true}}}
	return startupHTTPFixture{f: f, token: token, assignment: a, result: result, pooled: pooled}
}

func TestOpenCodeStoppedCallbacksPrivatePoolRejectForgedReceiptAndPreserveOriginal(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "Pool"}[pooled], func(t *testing.T) {
			h := openCodeCallbackFixture(t, pooled)
			before := h.ledger(t)
			for _, kind := range []string{"missing-proof", "live-server", "missing-abort", "foreign-session"} {
				bad := candidateJSON(t, h.result)
				bad["operation_id"] = newID()
				proof := bad["tool_evidence"].(map[string]any)["opencode_execution"].(map[string]any)
				switch kind {
				case "missing-proof":
					delete(bad, "tool_evidence")
				case "live-server":
					proof["server_stopped"] = false
				case "missing-abort":
					proof["abort_acknowledged"] = false
				case "foreign-session":
					proof["session_id"] = "ses_foreign"
				}
				h.post(bad, 400)
				if h.ledger(t) != before {
					t.Fatal("invalid stop changed original ledger", kind)
				}
				current, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, h.result.RunID)
				if err != nil || current.State != "running" {
					t.Fatal("forged stop changed active run", kind, current, err)
				}
			}
			h.post(candidateJSON(t, h.result), 200)
			settled := h.ledger(t)
			h.post(candidateJSON(t, h.result), 200)
			if h.ledger(t) != settled {
				t.Fatal("original stopped result replay settled twice")
			}
			current, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, h.result.RunID)
			if err != nil || current.State != "failed" {
				t.Fatal("confirmed stop became successful execution", current, err)
			}
			for _, node := range current.Nodes {
				if node.Type == "test" && node.Attempt != 0 {
					t.Fatal("stopped SDK dispatched independent tests", node)
				}
			}
		})
	}
}

func TestOpenCodeDirectWorkflowRequiresBoundFixedConfiguration(t *testing.T) {
	_, _, _, p := openCodeExecutionFixture(t)
	base := dag.Node{ID: "develop", Type: "agent", Command: []string{p.Profile.CommandID}, Config: map[string]any{"goal": "fixture", "opencode_execution": p.Profile.OpenCodeExecution}}
	if err := dag.Compile(dag.Workflow{Nodes: []dag.Node{base}}); err == nil {
		t.Fatal("unbound SDK configuration compiled")
	}
	base.Config["tool_profile_id"], base.Config["tool_profile_sha256"] = p.ID, p.Profile.ProfileSHA256
	if err := dag.Compile(dag.Workflow{Nodes: []dag.Node{base}}); err != nil {
		t.Fatal("exact bound SDK fact did not compile", err)
	}
	for _, field := range []string{"model", "provider", "env", "max_output_tokens", "hard_budget", "session_id", "mcp_servers", "codex_execution"} {
		base.Config[field] = nil
		if err := dag.Compile(dag.Workflow{Nodes: []dag.Node{base}}); err == nil {
			t.Fatal("workflow overrode SDK administrator policy", field)
		}
		delete(base.Config, field)
	}
}
