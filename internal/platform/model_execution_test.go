package platform

import (
	"context"
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strconv"
	"strings"
	"testing"
)

func modelExecutionFixture(t *testing.T) (*fixture, string, map[string]any, ToolProfile, []modelcontract.Deployment) {
	t.Helper()
	f, token, request, _ := taskDispatchFixture(t)
	ctx := context.Background()
	p, err := f.s.GetToolProfile(ctx, f.actor, f.project, request["tool_profile_id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	p.Profile.ModelExecutionProtocol = modelcontract.Protocol
	p.Profile.WorkspaceToolsVersion = "attempt-workspace-v1"
	p.Profile.ProfileSHA256 = strings.Repeat("c", 64)
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_profiles SET profile=? WHERE id=?`, stringify(p.Profile), p.ID); err != nil {
		t.Fatal(err)
	}
	deployments := []modelcontract.Deployment{}
	for _, id := range []string{"model-one", "model-two"} {
		d := modelcontract.Deployment{ID: id, Revision: 1, Protocol: modelcontract.ProviderProtocol, Model: id, EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("d", 64), ContextWindow: 32000}
		d.SHA256 = modelcontract.DeploymentDigest(d)
		deployments = append(deployments, d)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, ModelDeployments: deployments}}})
	if err != nil {
		t.Fatal(err)
	}
	selection := modelcontract.Selection{DeploymentID: deployments[0].ID, DeploymentRevision: 1, DeploymentSHA256: deployments[0].SHA256, Parameters: modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}}
	request["model_execution_selection"] = selection
	f.e.AttemptAccounting = f.s
	return f, token, request, p, deployments
}
func modelRateForTest(t *testing.T, f *fixture, p ToolProfile, d modelcontract.Deployment, version int) UsageRateRevision {
	t.Helper()
	r, err := f.s.WriteUsageRate(context.Background(), f.actor, f.project, UsageRateWriteRequest{ProjectID: f.project, ToolProfileID: p.ID, ExpectedToolVersion: p.Version, ExpectedVersion: version, ModelDeploymentID: d.ID, ModelDeploymentRevision: d.Revision, ModelDeploymentSHA256: d.SHA256, Currency: "CNY", InputRateMicrosPerMillion: 1000000, OutputRateMicrosPerMillion: 2000000, InputTokenReservation: 1, OutputTokenReservation: 1, Reason: "Explicit operator estimate", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestModelExecutionFrozenRateClaimReservationAndSingleSettlement(t *testing.T) {
	f, token, request, p, deployments := modelExecutionFixture(t)
	rate := modelRateForTest(t, f, p, deployments[0], 0)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	modelRateForTest(t, f, p, deployments[1], rate.Version)
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
	if claim["assignment"] == nil {
		t.Fatal("matching model was not claimed", claim)
	}
	var a dag.Assignment
	raw, _ := json.Marshal(claim["assignment"])
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	var spec dag.Node
	var stored string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=? AND id=?`, run["id"], "develop").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(stored), &spec); err != nil {
		t.Fatal(err)
	}
	ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, Spec: spec}
	b, err := dag.ModelBinding(spec)
	if err != nil || b == nil {
		t.Fatal(b, err)
	}
	check := func() UsageAttempt {
		page, e := f.s.ListUsageAttempts(context.Background(), f.actor, f.project, "", "", 100, "")
		if e != nil || len(page.Items) != 1 {
			t.Fatal(page, e)
		}
		return page.Items[0]
	}
	attempt := check()
	if attempt.Rate == nil || attempt.Rate.ID != rate.ID || attempt.Model != "model-one" || attempt.ReservedTokens != 64100 || attempt.ReservedCostMicros == nil || *attempt.ReservedCostMicros != 64200 {
		t.Fatal("frozen rate or conservative reservation lost", attempt)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: p.Profile, Terminal: "completed", EventCount: 4, Usage: &tc.Usage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}, UsageState: tc.UsageComplete, ModelExecution: &modelcontract.Evidence{GatewayProtocol: modelcontract.GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: b.Deployment.SHA256, ParametersSHA256: b.ParametersSHA256, Model: b.Deployment.Model, Requests: []modelcontract.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: b.Deployment.Model, OutputAllowance: 100, UsageState: tc.UsageComplete, Usage: &modelcontract.RequestUsage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}}}}}}
	for i := 0; i < 2; i++ {
		tx, e := f.s.DB.Begin()
		if e != nil {
			t.Fatal(e)
		}
		if e = f.s.RecordResultTx(context.Background(), tx, ref, result); e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	attempt = check()
	if attempt.State != "settled" || attempt.ReportedTokens == nil || *attempt.ReportedTokens != 40 || attempt.EstimatedCostMicros == nil || *attempt.EstimatedCostMicros != 57 || attempt.HeldTokens != 0 || attempt.Version != 2 {
		t.Fatal("actual counters did not settle exactly once", attempt)
	}
}
func TestModelExecutionInventoryDriftAndMissingSelectionPreventStart(t *testing.T) {
	f, token, request, p, deployments := modelExecutionFixture(t)
	options, err := f.s.GetModelExecutionOptions(context.Background(), f.actor, f.project, ModelExecutionOptionsRequest{ProjectID: f.project, ToolProfileID: p.ID, ExpectedToolVersion: p.Version})
	if err != nil || !options.Supported || len(options.Items) != 2 || options.Items[0].CostState != "unpriced" {
		t.Fatal(options, err)
	}
	path := "/model-execution/options?project_id=" + f.project + "&tool_profile_id=" + p.ID + "&expected_tool_version=" + strconv.Itoa(p.Version)
	read := f.call("GET", path, nil, 200)
	if read["supported"] != true || read["tool_profile_id"] != p.ID || len(read["items"].([]any)) != 2 {
		t.Fatal("HTTP model inventory scope lost", read)
	}
	for _, suffix := range []string{"&unknown=1", "&expected_tool_version=2", "&tool_profile_id=" + p.ID} {
		f.call("GET", path+suffix, nil, 400)
	}
	f.call("GET", "/model-execution/options?project_id="+f.project+"&tool_profile_id="+p.ID+"&expected_tool_version=01", nil, 400)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	_, err = f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, ModelDeployments: deployments[1:]}}})
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 409)
	delete(request, "model_execution_selection")
	request["idempotency_key"] = newID()
	f.call("POST", "/task-dispatch-plans", request, 400)
	for _, table := range []string{"dag_runs", "suma_usage_attempts"} {
		if count := dispatchOptionsCount(t, f, table); count != 0 {
			t.Fatal("rejected model selection created effects", table, count)
		}
	}
}

func TestModelExecutionUncertainProviderUsageKeepsKnownCountersAndHold(t *testing.T) {
	for _, scenario := range []string{"foreign_provider_model", "missing_host_gateway_receipt", "output_overrun"} {
		t.Run(scenario, func(t *testing.T) {
			f, _, _, p, deployments := modelExecutionFixture(t)
			rate := modelRateForTest(t, f, p, deployments[0], 0)
			binding, err := modelcontract.NewBinding(deployments[0], modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, &modelcontract.RateSnapshot{ID: rate.ID, Version: rate.Version, Currency: rate.Currency, InputRateMicrosPerMillion: rate.InputRateMicrosPerMillion, OutputRateMicrosPerMillion: rate.OutputRateMicrosPerMillion})
			if err != nil {
				t.Fatal(err)
			}
			ref := dag.AccountingAttempt{TenantID: f.user.TenantID, ProjectID: f.project, RunID: newID(), NodeID: "develop", WorkerID: p.WorkerID, Attempt: 1, Fence: 1, Spec: dag.Node{ID: "develop", Type: "agent", Command: []string{p.Profile.CommandID}, Config: map[string]any{"tool_profile_id": p.ID, "tool_profile_sha256": p.Profile.ProfileSHA256, "model_execution": binding, "max_steps": 2, "max_output_tokens": 100}}}
			tx, err := f.s.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.ReserveAttemptTx(context.Background(), tx, ref); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			evidence := &modelcontract.Evidence{GatewayProtocol: modelcontract.GatewayProtocol, GatewayClosed: true, BindingSHA256: binding.BindingSHA256, DeploymentSHA256: binding.Deployment.SHA256, ParametersSHA256: binding.ParametersSHA256, Model: binding.Deployment.Model, Requests: []modelcontract.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: binding.Deployment.Model, OutputAllowance: 100, UsageState: tc.UsageComplete, Usage: &modelcontract.RequestUsage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}}}}
			output := int64(17)
			switch scenario {
			case "foreign_provider_model":
				evidence.Requests[0].ProviderModel = "foreign"
			case "missing_host_gateway_receipt":
				evidence = nil
			case "output_overrun":
				output = 101
			}
			if evidence != nil {
				evidence.Requests[0].Usage.OutputTokens = &output
			}
			result := dag.Result{RunID: ref.RunID, NodeID: ref.NodeID, WorkerID: ref.WorkerID, Attempt: 1, Fence: 1, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: p.Profile, Terminal: "failed", EventCount: 4, Usage: &tc.Usage{InputTokens: usageInt(23), OutputTokens: &output}, UsageState: tc.UsageComplete, ModelExecution: evidence}}
			tx, err = f.s.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.RecordResultTx(context.Background(), tx, ref, result); err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			page, err := f.s.ListUsageAttempts(context.Background(), f.actor, f.project, "", "", 100, "")
			if err != nil || len(page.Items) != 1 {
				t.Fatal(page, err)
			}
			a := page.Items[0]
			if scenario == "missing_host_gateway_receipt" {
				if a.State != "reconciling" || a.ReportedTokens != nil || a.HeldTokens == 0 {
					t.Fatal("unproven SDK counters were trusted or reservation released", a)
				}
				return
			}
			if a.State != "reconciling" || a.ReportedTokens == nil || *a.ReportedTokens != 23+output || a.EstimatedCostMicros == nil || *a.EstimatedCostMicros != 23+output*2 || a.HeldTokens == 0 || a.HeldCostMicros == nil || *a.HeldCostMicros == 0 {
				t.Fatal("uncertain execution lost usage or reservation", a)
			}
		})
	}
}

func TestModelExecutionDispatchCopyClearsAutomaticRetryWithoutChangingTemplate(t *testing.T) {
	source, profile, task := taskDispatchBuildFixture()
	source.Nodes[0].Idempotent = true
	source.Nodes[0].MaxRetries = 2
	derived, err := buildTaskDispatchWorkflow(source, "develop", "fixed-profile", profile, task)
	if err != nil {
		t.Fatal(err)
	}
	d := modelcontract.Deployment{ID: "bound-model", Revision: 1, Protocol: modelcontract.ProviderProtocol, Model: "bound-model", EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("a", 64), ContextWindow: 8192}
	d.SHA256 = modelcontract.DeploymentDigest(d)
	b, err := modelcontract.NewBinding(d, modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = bindModelExecutionWorkflow(&derived, "develop", &b); err != nil {
		t.Fatal(err)
	}
	if derived.Nodes[0].MaxRetries != 0 || source.Nodes[0].MaxRetries != 2 {
		t.Fatal("model preview did not disable retries only in its copied node", derived.Nodes[0].MaxRetries, source.Nodes[0].MaxRetries)
	}
}

// These helpers keep typed workspace/repair service fixtures on the current
// model contract. They create no provider, process or claimed real inference.
func nativeWorkspaceModelForTest(t *testing.T) modelcontract.Binding {
	t.Helper()
	d := modelcontract.Deployment{ID: "workspace-model-fixture", Revision: 1, Protocol: modelcontract.ProviderProtocol, Model: "workspace-model-fixture", EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("d", 64), ContextWindow: 8192}
	d.SHA256 = modelcontract.DeploymentDigest(d)
	b, err := modelcontract.NewBinding(d, modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func nativeWorkspaceSelectionForTest(t *testing.T) *modelcontract.Selection {
	b := nativeWorkspaceModelForTest(t)
	return &modelcontract.Selection{DeploymentID: b.Deployment.ID, DeploymentRevision: b.Deployment.Revision, DeploymentSHA256: b.Deployment.SHA256, Parameters: b.Parameters}
}
func nativeGatewayReceiptForTest(t *testing.T, b modelcontract.Binding) *modelcontract.Evidence {
	t.Helper()
	return &modelcontract.Evidence{GatewayProtocol: modelcontract.GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: b.Deployment.SHA256, ParametersSHA256: b.ParametersSHA256, Model: b.Deployment.Model, Requests: []modelcontract.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: b.Deployment.Model, OutputAllowance: 100, UsageState: "unreported"}}}
}
