package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func testModelBinding(t *testing.T, endpoint string) mc.Binding {
	t.Helper()
	sha, err := mc.EndpointDigest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deployment := mc.Deployment{ID: "model-a", Revision: 1, Protocol: mc.ProviderProtocol, Model: "fixture-model-a", EndpointOrigin: endpoint, EndpointConfigSHA256: sha, ContextWindow: 8192}
	deployment.SHA256 = mc.DeploymentDigest(deployment)
	binding, err := mc.NewBinding(deployment, mc.Parameters{MaxSteps: 2, MaxOutputTokens: 128}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func modelBindingValue(t *testing.T, binding mc.Binding) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(binding)
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// These are real Runner processes, the delivery bundle, the official MCP
// client and two locally owned HTTP protocol identities. No vendor is called.
func TestModelExecutionTwoDeploymentsReachActualNativeBundle(t *testing.T) {
	var calls []map[string]any
	var requestDigests, responseDigests []string
	var replyStatus atomic.Int32
	var failedCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, readErr := io.ReadAll(r.Body)
		if r.Method != "POST" || readErr != nil || json.Unmarshal(raw, &body) != nil {
			t.Error("unexpected model request")
			w.WriteHeader(400)
			return
		}
		model, _ := body["model"].(string)
		if model != "fixture-model-a" && model != "fixture-model-b" {
			t.Error("provider received an unselected model")
		}
		suffix := strings.TrimPrefix(model, "fixture-model-")
		if r.URL.Path != "/"+suffix+"/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-only-"+suffix {
			t.Error("provider endpoint or separate credential changed")
		}
		if body["max_completion_tokens"] != float64(128) || body["stream"] != false {
			t.Error("provider limits or protocol were not applied")
		}
		foundWrite := false
		tools, _ := body["tools"].([]any)
		for _, tool := range tools {
			entry, _ := tool.(map[string]any)
			function, _ := entry["function"].(map[string]any)
			schema, _ := function["parameters"].(map[string]any)
			fields, _ := schema["properties"].(map[string]any)
			if fields["expected_sha256"] != nil && fields["expected_absent"] != nil {
				foundWrite = true
				if _, incompatible := schema["oneOf"]; incompatible {
					t.Error("OpenAI rejects the workspace write schema's top-level oneOf")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}
		}
		if !foundWrite {
			t.Error("the provider request lost the workspace write tool")
		}
		switch status := replyStatus.Load(); status {
		case -1:
			failedCalls.Add(1)
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		case http.StatusBadRequest:
			failedCalls.Add(1)
			http.Error(w, "upstream-only diagnostic", int(status))
			return
		}
		calls = append(calls, body)
		requestDigests = append(requestDigests, digest(raw))
		w.Header().Set("Content-Type", "application/json")
		responseRaw, _ := json.Marshal(map[string]any{"id": "fixture-response-" + suffix, "model": model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "Two-deployment local protocol evidence only"}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3}})
		responseDigests = append(responseDigests, digest(responseRaw))
		_, _ = w.Write(responseRaw)
	}))
	defer server.Close()
	cfg, _ := nativeHostFixture(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "packages", "agent-runtime", "dist", "suma-agent.bundle.mjs")
	manifest := filepath.Join(root, "packages", "agent-runtime", "dist", "suma-agent.manifest.json")
	if _, err = os.Stat(bundle); err != nil {
		t.Fatal("build the native runtime bundle before this explicit integration gate", err)
	}
	spec := cfg.Commands["native"]
	spec.Args = []string{bundle}
	spec.NativeWorkspace.RuntimeManifest = manifest
	spec.ModelExecutionProtocol = mc.Protocol
	// Native workspace configuration cannot forward provider environments.
	// Ambient values must not replace the Go-owned immutable deployment.
	spec.Env, spec.EnvFrom = nil, nil
	t.Setenv("SUMA_MODEL", "ambient-must-not-run")
	t.Setenv("SUMA_MODEL_BASE_URL", "https://unused.invalid")
	cfg.Commands["native"] = spec
	bindings := []mc.Binding{}
	for _, suffix := range []string{"a", "b"} {
		binding := testModelBinding(t, server.URL)
		binding.Deployment.ID = "model-" + suffix
		binding.Deployment.Model = "fixture-model-" + suffix
		endpoint := server.URL + "/" + suffix + "/v1"
		binding.Deployment.EndpointConfigSHA256, _ = mc.EndpointDigest(endpoint)
		binding.Deployment.SHA256 = mc.DeploymentDigest(binding.Deployment)
		binding, err = mc.NewBinding(binding.Deployment, binding.Parameters, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		envRef := "SUMA_FIXTURE_MODEL_" + strings.ToUpper(suffix)
		t.Setenv(envRef, "fixture-only-"+suffix)
		cfg.ModelDeployments = append(cfg.ModelDeployments, ModelDeploymentSpec{Deployment: binding.Deployment, BaseURL: endpoint, APIKeyEnvRef: envRef})
		bindings = append(bindings, binding)
	}
	inventory, err := DescribeTools(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || !inventory[0].Available || inventory[0].Profile.ModelExecutionProtocol != mc.Protocol || len(inventory[0].ModelDeployments) != 2 {
		t.Fatalf("actual runtime did not prove model support: %+v", inventory)
	}
	rawInventory, _ := json.Marshal(inventory)
	if strings.Contains(string(rawInventory), "fixture-only") || strings.Contains(string(rawInventory), "api_key") || strings.Contains(string(rawInventory), "/a/v1") {
		t.Fatal("model inventory leaked private configuration")
	}
	cfg.ToolInventory = inventory
	for index, binding := range bindings {
		a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: fmt.Sprintf("actual-model-%d", index), NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "Inspect the supplied native workspace tools; report local protocol completion only", "tool_profile_sha256": inventory[0].Profile.ProfileSHA256, "model_execution": modelBindingValue(t, binding)}}
		result := Execute(context.Background(), a, cfg)
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil {
			t.Fatalf("actual selected model runtime failed: exit=%d error=%s unknown=%t output=%s", result.ExitCode, result.Error, result.OutcomeUnknown, result.Output)
		}
		evidence := result.ToolEvidence
		if evidence.UsageState != tc.UsageComplete || evidence.Usage == nil || evidence.Usage.InputTokens == nil || *evidence.Usage.InputTokens != 7 || evidence.Usage.OutputTokens == nil || *evidence.Usage.OutputTokens != 3 || evidence.ModelExecution == nil || mc.ValidateEvidence(*evidence.ModelExecution) != nil {
			t.Fatalf("complete native usage evidence missing: %+v", evidence)
		}
		actual := evidence.ModelExecution
		if actual.GatewayProtocol != mc.GatewayProtocol || !actual.GatewayClosed || actual.BindingSHA256 != binding.BindingSHA256 || actual.ParametersSHA256 != binding.ParametersSHA256 || actual.DeploymentSHA256 != binding.Deployment.SHA256 || actual.Model != binding.Deployment.Model || len(actual.Requests) != 1 || actual.Requests[0].State != "completed" || actual.Requests[0].ProviderModel != binding.Deployment.Model || actual.Requests[0].ProviderRequestID != fmt.Sprintf("fixture-response-%s", []string{"a", "b"}[index]) || !mc.ValidDigest(actual.Requests[0].ResponseSHA256) {
			t.Fatalf("actual model evidence changed binding: %+v", actual)
		}
		if actual.Requests[0].RequestSHA256 != requestDigests[index] || actual.Requests[0].ResponseSHA256 != responseDigests[index] {
			t.Fatal("request or response evidence differs from actual HTTP bytes")
		}
		retained, _ := json.Marshal(actual)
		t.Logf("native_model_receipt=%s", retained)
	}
	if len(calls) != 2 {
		t.Fatalf("expected exactly two owned protocol model requests, got %d", len(calls))
	}
	for _, failure := range []struct {
		status        int32
		state, detail string
	}{
		{http.StatusBadRequest, "rejected", "Model provider rejected the request (HTTP 400)"},
		{-1, "unknown", "Model provider outcome is unknown"},
	} {
		t.Run("provider request "+failure.state, func(t *testing.T) {
			before := failedCalls.Load()
			replyStatus.Store(failure.status)
			a := Assignment{TenantID: "tenant", ProjectID: "project", RunID: "actual-model-" + failure.state, NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"native"}, Config: map[string]any{
				"goal": "Verify the original provider failure", "tool_profile_sha256": inventory[0].Profile.ProfileSHA256, "model_execution": modelBindingValue(t, bindings[0]),
			}}
			result := Execute(context.Background(), a, cfg)
			if result.ExitCode == 0 || !strings.Contains(result.Error, failure.detail) || strings.Contains(result.Error+result.Output, "upstream-only diagnostic") {
				t.Fatalf("original provider failure was obscured or leaked: exit=%d error=%s", result.ExitCode, result.Error)
			}
			if failedCalls.Load() != before+1 || result.ToolEvidence == nil || result.ToolEvidence.UsageState != tc.UsageUnreported || result.ToolEvidence.ModelExecution == nil ||
				len(result.ToolEvidence.ModelExecution.Requests) != 1 || result.ToolEvidence.ModelExecution.Requests[0].State != failure.state {
				t.Fatalf("original request or unknown usage was not retained: %+v", result.ToolEvidence)
			}
		})
	}
}

func TestModelExecutionLocalConfigAndBindingsRejectTampering(t *testing.T) {
	binding := testModelBinding(t, "http://127.0.0.1:19991")
	local := ModelDeploymentSpec{Deployment: binding.Deployment, BaseURL: "http://127.0.0.1:19991", APIKeyEnvRef: "SUMA_FIXTURE_MODEL_A"}
	raw, _ := json.Marshal([]ModelDeploymentSpec{local})
	if items, err := DecodeModelDeployments(raw); err != nil || len(items) != 1 {
		t.Fatal("valid local allowlist rejected", err)
	}
	for _, raw := range [][]byte{[]byte(`null`), []byte(`[{"deployment":{},"base_url":"https://example.test","api_key_env_ref":"KEY","api_key_env_ref":"OTHER"}]`), []byte(`[{"deployment":{},"base_url":"https://example.test","api_key":"secret"}]`)} {
		if _, err := DecodeModelDeployments(raw); err == nil {
			t.Fatal("invalid local configuration accepted")
		}
	}
	changed := local
	changed.BaseURL += "/different-api"
	if validateModelDeployments([]ModelDeploymentSpec{changed}) == nil {
		t.Fatal("same-origin endpoint path drift accepted")
	}
	changed = local
	changed.APIKeyEnvRef = "SUMA_RUNNER_TOKEN"
	if validateModelDeployments([]ModelDeploymentSpec{changed}) == nil {
		t.Fatal("runner credential accepted as a model credential")
	}
	profile := structuredProfile("suma-jsonl-v1")
	profile.ReadOnly = false
	profile.WorkspaceToolsVersion = NativeWorkspaceVersion
	profile.ModelExecutionProtocol = mc.Protocol
	spec := CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Input: "agent-jsonl", ModelExecutionProtocol: mc.Protocol, NativeWorkspace: &NativeWorkspacePolicy{}}
	t.Setenv(local.APIKeyEnvRef, "fixture-only-secret")
	a := Assignment{Type: "agent", Config: map[string]any{"model_execution": modelBindingValue(t, binding)}}
	cfg := ExecConfig{ModelDeployments: []ModelDeploymentSpec{local}}
	if _, err := modelExecutionRequest(a, cfg, spec, profile); err != nil {
		t.Fatal("valid exact local model request rejected", err)
	}
	a.Config["env"] = map[string]any{"SUMA_MODEL_BASE_URL": "http://evil.invalid"}
	if _, err := modelExecutionRequest(a, cfg, spec, profile); err == nil {
		t.Fatal("node model environment override accepted")
	}
	delete(a.Config, "env")
	a.Config["max_output_tokens"] = 129
	if _, err := nativeExecutionLimits(a); err == nil {
		t.Fatal("node limits changed selected parameters")
	}
	delete(a.Config, "max_output_tokens")
	widest, err := mc.NewBinding(binding.Deployment, mc.Parameters{MaxSteps: mc.MaxSteps, MaxOutputTokens: mc.MaxOutputTokens}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	limits, err := nativeExecutionLimits(Assignment{Config: map[string]any{"model_execution": modelBindingValue(t, widest)}})
	if err != nil || limits.MaxSteps != mc.MaxSteps || int64(limits.MaxTokens) != mc.MaxOutputTokens {
		t.Fatal("ordinary native parameters differ from the shared model contract", limits, err)
	}
	spec.ModelExecutionProtocol = ""
	if _, err := modelExecutionRequest(a, cfg, spec, profile); err == nil {
		t.Fatal("old runtime profile claimed model support")
	}
}

func TestModelExecutionRequiresLocalDeploymentBeforeWorkspace(t *testing.T) {
	cfg, capture := nativeLimitFixture(t, false)
	binding := testModelBinding(t, "http://127.0.0.1:19991")
	a := Assignment{RunID: "model-bound", NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "Do not run an unapproved deployment", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256, "model_execution": binding}}
	result := Execute(context.Background(), a, cfg)
	if result.ExitCode == 0 || !strings.Contains(result.Error, "model execution") || result.OutcomeUnknown || result.ToolEvidence != nil || result.WorkspaceEvidence != nil {
		t.Fatalf("unapproved deployment entered execution: %+v", result)
	}
	if _, err := os.Stat(capture); !os.IsNotExist(err) {
		t.Fatal("unapproved model binding launched main process", err)
	}
	repairCfg, repairCapture := nativeLimitFixture(t, true)
	repair := startupRepairAssignment(t)
	repair.Config["model_execution"] = modelBindingValue(t, binding)
	repair.Config["tool_profile_sha256"] = repairCfg.ToolInventory[0].Profile.ProfileSHA256
	repairResult := Execute(context.Background(), repair, repairCfg)
	if repairResult.ExitCode == 0 || repairResult.StartupFailure == nil || repairResult.OutcomeUnknown || repairResult.WorkspaceEvidence != nil {
		t.Fatal("typed repair lost its trustworthy before-workspace failure proof")
	}
	if _, err := os.Stat(repairCapture); !os.IsNotExist(err) {
		t.Fatal("unapproved repair model launched process", err)
	}
}

// Accounting comes from a host HTTP snapshot, never SDK progress frames. These
// are typed host fixtures; this test itself does not contact a model provider.
func TestModelExecutionHostKeepsKnownUsageOnUnknownProviderIdentity(t *testing.T) {
	binding := testModelBinding(t, "http://127.0.0.1:19991")
	for _, provider := range []string{"foreign-alias", ""} {
		profile := structuredProfile("suma-jsonl-v1")
		profile.ReadOnly = false
		profile.WorkspaceToolsVersion = NativeWorkspaceVersion
		profile.ModelExecutionProtocol = mc.Protocol
		input, output := int64(7), int64(3)
		snapshot := mg.Snapshot{Version: 1, RequestID: strings.Repeat("a", 64), Binding: binding, Closed: true, FailureCode: "model_gateway_upstream_invalid", Records: []mg.Record{{RequestEvidence: mc.RequestEvidence{State: "unknown", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: provider, OutputAllowance: 128, UsageState: "complete", Usage: &mc.RequestUsage{InputTokens: &input, OutputTokens: &output}}, ClientSHA256: strings.Repeat("c", 64), HTTPStatus: 200}}}
		evidence := &tc.Evidence{Profile: profile, Terminal: "unknown", UsageState: tc.UsageComplete, Usage: &tc.Usage{InputTokens: cloneUsageCount(&output), OutputTokens: cloneUsageCount(&input)}}
		complete := applyModelGatewayEvidence(evidence, snapshot)
		if complete || evidence.Terminal != "unknown" || evidence.UsageState != tc.UsagePartial || evidence.Usage == nil || evidence.Usage.InputTokens == nil || *evidence.Usage.InputTokens != 7 || evidence.Usage.OutputTokens == nil || *evidence.Usage.OutputTokens != 3 || evidence.ModelExecution == nil || mc.ValidateEvidence(*evidence.ModelExecution) != nil {
			t.Fatalf("unknown provider identity was completed or lost original host accounting: %+v", evidence)
		}
	}
}

func TestModelExecutionUnknownUsageCannotAuthorizeAutomaticRetry(t *testing.T) {
	binding := testModelBinding(t, "http://127.0.0.1:19991")
	profile := structuredProfile("suma-jsonl-v1")
	profile.ReadOnly = false
	profile.WorkspaceToolsVersion = NativeWorkspaceVersion
	profile.ModelExecutionProtocol = mc.Protocol
	stream := newToolStream(profile, nil, nil, "bound-rejection")
	stream.modelBinding = &binding
	for _, frame := range []map[string]any{{"type": "started", "tools": []string{}}, {"type": "request_started", "step": 0, "output_allowance": 128, "request_sha256": strings.Repeat("a", 64), "endpoint_config_sha256": binding.Deployment.EndpointConfigSHA256}, {"type": "usage", "step": 0, "usage_state": "unreported", "usage_reported": false}, {"type": "error", "code": "upstream_rejected", "http_status": 429, "source": "model", "failure_class": "transient", "message": "Owned rejection fixture"}} {
		frame["request_id"] = "bound-rejection"
		frame["binding_sha256"] = binding.BindingSHA256
		frame["deployment_sha256"] = binding.Deployment.SHA256
		frame["parameters_sha256"] = binding.ParametersSHA256
		frame["model"] = binding.Deployment.Model
		raw, _ := json.Marshal(frame)
		_, _ = stream.Write(append(raw, '\n'))
	}
	evidence, err := stream.finish()
	if err != nil || evidence.UsageState != tc.UsageUnreported || stream.failureClass != "" {
		t.Fatal("unknown bound usage was classified for automatic retry", stream.failureClass, err)
	}
}

func TestModelExecutionJournalPreservesOriginalInventoryAndReceipt(t *testing.T) {
	binding := testModelBinding(t, "http://127.0.0.1:19991")
	profile := structuredProfile("suma-jsonl-v1")
	profile.ReadOnly = false
	profile.WorkspaceToolsVersion = NativeWorkspaceVersion
	profile.ModelExecutionProtocol = mc.Protocol
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	items := []tc.Observation{{Profile: profile, Available: true, ModelDeployments: []mc.Deployment{binding.Deployment}}}
	report, err := journal.NewInventory("http://127.0.0.1:4310", "project", "worker", items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].ModelDeployments[0].Model = "mutated-input"
	report.Items[0].ModelDeployments[0].Model = "mutated-return"
	pending := journal.PendingInventory()
	if pending.Items[0].ModelDeployments[0].Model != binding.Deployment.Model {
		t.Fatal("mutable input changed original durable inventory operation")
	}
	pending.Items[0].ModelDeployments[0].Model = "mutated-readback"
	if journal.PendingInventory().Items[0].ModelDeployments[0].Model != binding.Deployment.Model {
		t.Fatal("mutable readback changed original inventory")
	}
	a := Assignment{RunID: "bound-durable", NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Config: map[string]any{"model_execution": modelBindingValue(t, binding)}}
	if err = journal.Accept(a); err != nil {
		t.Fatal(err)
	}
	// This receipt is a typed journal fixture, not real provider evidence.
	input, output := int64(7), int64(3)
	receipt := &mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: true, BindingSHA256: binding.BindingSHA256, DeploymentSHA256: binding.Deployment.SHA256, ParametersSHA256: binding.ParametersSHA256, Model: binding.Deployment.Model, Requests: []mc.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: binding.Deployment.Model, OutputAllowance: 128, UsageState: tc.UsageComplete, Usage: &mc.RequestUsage{InputTokens: &input, OutputTokens: &output}}}}
	if err = journal.Finish(Result{Assignment: a, ExitCode: 0, ToolEvidence: &tc.Evidence{Profile: profile, Terminal: "completed", EventCount: 4, UsageState: tc.UsageComplete, Usage: &tc.Usage{InputTokens: &input, OutputTokens: &output}, ModelExecution: receipt}}); err != nil {
		t.Fatal(err)
	}
	receipt.Requests[0].ProviderModel = "mutated-input"
	*receipt.Requests[0].Usage.OutputTokens = 99
	first := journal.Pending()
	if first[0].ToolEvidence.ModelExecution.Requests[0].ProviderModel != binding.Deployment.Model || *first[0].ToolEvidence.ModelExecution.Requests[0].Usage.OutputTokens != 3 {
		t.Fatal("mutable receipt changed durable original result")
	}
	first[0].ToolEvidence.ModelExecution.Requests[0].ProviderModel = "mutated-readback"
	*first[0].ToolEvidence.ModelExecution.Requests[0].Usage.InputTokens = 77
	if journal.Pending()[0].ToolEvidence.ModelExecution.Requests[0].ProviderModel != binding.Deployment.Model || *journal.Pending()[0].ToolEvidence.ModelExecution.Requests[0].Usage.InputTokens != 7 {
		t.Fatal("returned receipt changed durable original result")
	}
}
