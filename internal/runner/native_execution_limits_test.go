package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This child records validated stdin limits and makes one real request through
// the host gateway to an owned loopback provider. It does not claim a real SDK,
// filesystem edit, formal test or independently executed conflict repair.
func nativeLimitFixture(t *testing.T, repair bool) (ExecConfig, string) {
	t.Helper()
	cfg, entry := nativeHostFixture(t)
	capture := filepath.Join(filepath.Dir(entry), "received-limits.json")
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer native-limit-owned-key" {
			t.Error("limits fixture reached an unfrozen model route")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"limit-response","model":"fixture-model-a","choices":[{"index":0,"message":{"role":"assistant","content":"Limits received"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1}}`))
	}))
	t.Cleanup(provider.Close)
	endpoint := provider.URL + "/v1"
	deployment := testModelBinding(t, provider.URL).Deployment
	deployment.EndpointConfigSHA256, _ = mc.EndpointDigest(endpoint)
	deployment.SHA256 = mc.DeploymentDigest(deployment)
	t.Setenv("SUMA_NATIVE_LIMIT_FIXTURE_KEY", "native-limit-owned-key")
	cfg.ModelDeployments = []ModelDeploymentSpec{{Deployment: deployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_NATIVE_LIMIT_FIXTURE_KEY"}}
	captureJSON, _ := json.Marshal(capture)
	script := `import {createInterface} from 'node:readline';import {writeFileSync} from 'node:fs';import {createHash} from 'node:crypto';
if(process.argv.includes('--version')){console.log('suma-agent-runtime 1.0.0');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN');process.exit(0)}
if(process.argv.includes('--model-execution-probe')){console.log(JSON.stringify({protocol:'suma-model-execution-v1',provider_protocol:'openai-chat-completions-v1',parameters:['max_steps','max_output_tokens']}));process.exit(0)}
const capture=CAPTURE_LITERAL;
const hash=value=>createHash('sha256').update(value).digest('hex');
const lines=createInterface({input:process.stdin});lines.once('line',async line=>{
 const r=JSON.parse(line);const local=JSON.parse(process.env.SUMA_MODEL_EXECUTION||'null');const b=r.model_execution;
 const emit=e=>console.log(JSON.stringify({...e,request_id:r.request_id,binding_sha256:b.binding_sha256,deployment_sha256:b.deployment.deployment_sha256,parameters_sha256:b.parameters_sha256,model:b.deployment.model}));
 try{
  if(!local||local.binding.binding_sha256!==b.binding_sha256||local.gateway.binding_sha256!==b.binding_sha256||local.gateway.request_id!==r.request_id)throw Error('host binding required');
  writeFileSync(capture,JSON.stringify({max_steps:r.max_steps,max_tokens:r.max_tokens,bridge_present:!!r.workspace_bridge,repair_protocol:r.workspace_bridge?.conflict_repair_protocol||'',binding_sha256:b.binding_sha256}));
  const allowance=Math.min(r.max_tokens,b.deployment.context_window);
  const body=JSON.stringify({model:b.deployment.model,messages:[{role:'user',content:'Verify the supplied limit fields only.'}],stream:false,max_completion_tokens:allowance});
  emit({type:'started',tools:[]});emit({type:'request_started',step:0,output_allowance:allowance,request_sha256:hash(body),endpoint_config_sha256:b.deployment.endpoint_config_sha256});
  const response=await fetch(local.gateway.base_url+'/chat/completions',{method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+local.gateway.token,'X-Suma-Model-Step':'0'},body});
  if(!response.ok)throw Error('host model gateway rejected request');
  const raw=await response.text();const result=JSON.parse(raw);
  emit({type:'usage',step:0,input_tokens:result.usage.prompt_tokens,output_tokens:result.usage.completion_tokens,usage_state:'complete',usage_reported:true,provider_model:result.model,provider_request_id:result.id,response_sha256:hash(raw)});
  emit({type:'completed',output:'Limits received',usage_state:'complete',output_tokens:result.usage.completion_tokens});
 }catch{emit({type:'error',code:'limit_fixture_failed',message:'Owned model limit fixture failed'});process.exitCode=1}finally{lines.close()}
});`
	script = strings.Replace(script, "CAPTURE_LITERAL", string(captureJSON), 1)
	if err := os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	spec := cfg.Commands["native"]
	spec.NativeWorkspace.ConflictRepair = repair
	spec.ModelExecutionProtocol = mc.Protocol
	spec.Env, spec.EnvFrom = nil, nil
	manifest := nativeRuntimeManifest{SchemaVersion: 1, Entry: filepath.Base(entry), EntrySHA256: digest([]byte(script)), ExternalImports: []string{"node:readline", "node:fs", "node:crypto"}, Dependencies: map[string]string{"@modelcontextprotocol/client": "2.4.0"}}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(spec.NativeWorkspace.RuntimeManifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Commands["native"] = spec
	_, executableSHA, err := commandExecutable(spec)
	if err != nil {
		t.Fatal(err)
	}
	profileSHA, err := effectiveCommandProfileDigest(cfg, spec, executableSHA)
	if err != nil {
		t.Fatal(err)
	}
	profile := tc.Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: profileSHA, ExecutableSHA256: executableSHA, ToolVersion: "limit-host-fixture", WorkspaceToolsVersion: NativeWorkspaceVersion, ModelExecutionProtocol: mc.Protocol}
	if repair {
		profile.ConflictRepairProtocol = ccc.CandidateConflictRepairProtocol
	}
	cfg.ToolInventory = []tc.Observation{{Profile: profile, Available: true, ModelDeployments: []mc.Deployment{deployment}}}
	return cfg, capture
}

func nativeLimitModelBinding(t *testing.T, cfg ExecConfig, steps, tokens int) mc.Binding {
	t.Helper()
	binding, err := mc.NewBinding(cfg.ModelDeployments[0].Deployment, mc.Parameters{MaxSteps: steps, MaxOutputTokens: int64(tokens)}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func assertReceivedNativeLimits(t *testing.T, capture string, steps, tokens int, repair bool, bindingSHA string) {
	t.Helper()
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal("child did not receive stdin", err)
	}
	var received struct {
		Steps         int    `json:"max_steps"`
		Tokens        int    `json:"max_tokens"`
		Bridge        bool   `json:"bridge_present"`
		Protocol      string `json:"repair_protocol"`
		BindingSHA256 string `json:"binding_sha256"`
	}
	if err = json.Unmarshal(raw, &received); err != nil || received.Steps != steps || received.Tokens != tokens || received.BindingSHA256 != bindingSHA || !received.Bridge || (received.Protocol == ccc.CandidateConflictRepairProtocol) != repair {
		t.Fatalf("stdin lost the validated execution limits: %s, %v", raw, err)
	}
}

func TestNativeExecutionLimitsReachActualStdin(t *testing.T) {
	cfg, capture := nativeLimitFixture(t, false)
	for _, explicit := range []bool{false, true} {
		steps, tokens := 8, 4096
		if explicit {
			steps, tokens = mc.MaxSteps, int(mc.MaxOutputTokens)
		}
		binding := nativeLimitModelBinding(t, cfg, steps, tokens)
		a := Assignment{RunID: fmt.Sprintf("limits-%t", explicit), NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "Verify only stdin", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256, "model_execution": modelBindingValue(t, binding)}}
		if explicit {
			a.Config["max_steps"], a.Config["max_output_tokens"] = json.Number(fmt.Sprint(steps)), float64(tokens)
		}
		result := Execute(context.Background(), a, cfg)
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.ModelExecution == nil || !result.ToolEvidence.ModelExecution.GatewayClosed || len(result.ToolEvidence.ModelExecution.Requests) != 1 || result.ToolEvidence.ModelExecution.Requests[0].State != "completed" {
			t.Fatalf("bounded native host/gateway fixture failed: %+v", result)
		}
		assertReceivedNativeLimits(t, capture, steps, tokens, false, binding.BindingSHA256)
	}

	// Exercise the typed repair input builder without fabricating a conflict
	// workspace or claiming a real repair. Its child uses a real Go gateway too.
	repairCfg, repairCapture := nativeLimitFixture(t, true)
	for _, explicit := range []bool{false, true} {
		a := startupRepairAssignment(t)
		steps, tokens := 64, 4096
		if explicit {
			steps, tokens = mc.MaxSteps, int(mc.MaxOutputTokens)
		}
		binding := nativeLimitModelBinding(t, repairCfg, steps, tokens)
		a.Config["model_execution"] = modelBindingValue(t, binding)
		if explicit {
			a.Config["max_steps"], a.Config["max_output_tokens"] = int64(steps), json.Number(fmt.Sprint(tokens))
		}
		spec := repairCfg.Commands["native"]
		if _, err := modelPurposeRequest(a, spec, repairCfg.ToolInventory[0].Profile); err != nil {
			t.Fatal("typed repair precondition failed", err)
		}
		limits, err := nativeExecutionLimits(a)
		if err != nil {
			t.Fatal(err)
		}
		payload, err := nativeAgentInput(a, limits, NativeWorkspaceBridge{Version: 1, ConflictRepairProtocol: ccc.CandidateConflictRepairProtocol, RequestID: digest([]byte(key(a)))})
		if err != nil {
			t.Fatal(err)
		}
		local := repairCfg.ModelDeployments[0]
		gateway, err := mg.Open(mg.Config{Context: context.Background(), RequestID: digest([]byte(key(a))), Directory: filepath.Join(filepath.Dir(repairCapture), fmt.Sprintf("repair-model-gateway-%t", explicit)), Binding: binding, BaseURL: local.BaseURL, APIKey: os.Getenv(local.APIKeyEnvRef)})
		if err != nil {
			t.Fatal(err)
		}
		child := exec.Command(spec.Executable, spec.Args...)
		child.Env, _, err = commandEnvironment(spec)
		if err != nil {
			gateway.Close()
			t.Fatal(err)
		}
		child.Env = applyModelExecutionEnvironment(child.Env, binding, gateway.Connection())
		child.Stdin = bytes.NewReader(append(payload, '\n'))
		output, runErr := child.CombinedOutput()
		snapshot, closeErr := gateway.Close()
		if runErr != nil || closeErr != nil || !mc.EvidenceResponsesComplete(binding, mg.Evidence(snapshot)) {
			t.Fatalf("typed repair stdin/gateway fixture: %v %v %s", runErr, closeErr, output)
		}
		assertReceivedNativeLimits(t, repairCapture, steps, tokens, true, binding.BindingSHA256)
	}
}

func TestNativeExecutionLimitsRejectBeforeWorkspace(t *testing.T) {
	parserBinding := testModelBinding(t, "http://127.0.0.1:19991")
	invalid := []any{nil, "2", true, 0, -1, 1.5, math.NaN(), math.Inf(1), json.Number("1.0"), json.Number("1e0"), json.Number("01"), json.Number("+1"), json.Number("9007199254740992")}
	for _, name := range []string{"max_steps", "max_output_tokens"} {
		for _, value := range invalid {
			if _, err := nativeExecutionLimits(Assignment{Config: map[string]any{name: value, "model_execution": modelBindingValue(t, parserBinding)}}); err == nil || err.Error() != "Native agent execution limits are invalid" {
				t.Fatalf("invalid %s=%v fell back or was accepted: %v", name, value, err)
			}
		}
	}
	for _, repair := range []bool{false, true} {
		cfg, capture := nativeLimitFixture(t, repair)
		a := Assignment{RunID: "invalid-native", NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "Must not execute", "max_output_tokens": nil}}
		if repair {
			a = startupRepairAssignment(t)
			a.Config["max_steps"] = 129
		}
		binding := nativeLimitModelBinding(t, cfg, 8, 4096)
		a.Config["model_execution"] = modelBindingValue(t, binding)
		a.Config["tool_profile_sha256"] = cfg.ToolInventory[0].Profile.ProfileSHA256
		result := Execute(context.Background(), a, cfg)
		if result.ExitCode == 0 || result.Error != "Native agent execution limits are invalid" || result.OutcomeUnknown || result.ToolEvidence != nil || result.WorkspaceEvidence != nil || (result.StartupFailure != nil) != repair {
			t.Fatalf("invalid limits entered execution or lost startup proof: %+v", result)
		}
		if _, err := os.Stat(capture); !os.IsNotExist(err) {
			t.Fatal("invalid limits started the main process", err)
		}
		if _, err := os.Stat(filepath.Join(cfg.Repository, ".cache", "runner", "worktrees")); !os.IsNotExist(err) {
			t.Fatal("invalid limits entered workspace preparation/MCP", err)
		}
	}
}

// SDK frames check order and frozen metadata only. Go gateway receipts own
// every request and usage fact, so even a valid diagnostic stream has no ledger.
func TestNativeExecutionLimitsRepairStreamBoundary(t *testing.T) {
	for _, test := range []struct {
		name, protocol   string
		steps, allowance int
		valid            bool
	}{
		{"repair128", ccc.CandidateConflictRepairProtocol, 128, 131072, true},
		{"repair129", ccc.CandidateConflictRepairProtocol, 129, 1024, false},
		{"repair_allowance", ccc.CandidateConflictRepairProtocol, 1, 131073, false},
		{"ordinary128", "", 128, 131072, true},
		{"ordinary129", "", 129, 4096, false},
		{"ordinary_allowance", "", 1, 131073, false},
		{"foreign_marker", "foreign-protocol", 17, 1024, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := structuredProfile("suma-jsonl-v1")
			profile.ReadOnly, profile.WorkspaceToolsVersion, profile.ConflictRepairProtocol = false, NativeWorkspaceVersion, test.protocol
			profile.ModelExecutionProtocol = mc.Protocol
			if err := tc.ValidateProfile(profile, true); err != nil {
				if test.valid {
					t.Fatal(err)
				}
				return
			}
			original := testModelBinding(t, "http://127.0.0.1:19991")
			deployment := original.Deployment
			deployment.ContextWindow = 131072
			deployment.SHA256 = mc.DeploymentDigest(deployment)
			binding, err := mc.NewBinding(deployment, mc.Parameters{MaxSteps: mc.MaxSteps, MaxOutputTokens: mc.MaxOutputTokens}, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			stream := newToolStream(profile, nil, nil, "limits-request")
			stream.modelBinding = &binding
			emit := func(frame map[string]any) {
				frame["request_id"] = "limits-request"
				frame["binding_sha256"] = binding.BindingSHA256
				frame["deployment_sha256"] = binding.Deployment.SHA256
				frame["parameters_sha256"] = binding.ParametersSHA256
				frame["model"] = binding.Deployment.Model
				raw, _ := json.Marshal(frame)
				_, _ = stream.Write(append(raw, '\n'))
			}
			emit(map[string]any{"type": "started", "tools": []string{}})
			for step := 0; step < test.steps; step++ {
				emit(map[string]any{"type": "request_started", "step": step, "output_allowance": test.allowance, "request_sha256": strings.Repeat("a", 64), "endpoint_config_sha256": binding.Deployment.EndpointConfigSHA256})
				emit(map[string]any{"type": "usage", "step": step, "input_tokens": 0, "output_tokens": 0, "usage_state": "complete", "usage_reported": true})
			}
			emit(map[string]any{"type": "completed", "output": "Diagnostic frame validation only", "usage_state": "complete"})
			evidence, err := stream.finish()
			if test.valid != (err == nil && evidence.Terminal == "completed") || evidence.ModelExecution != nil {
				t.Fatalf("diagnostic frame boundary valid=%v: %+v %v", test.valid, evidence, err)
			}
		})
	}
}

func TestNativeExecutionInputRejectsInvalidGoal(t *testing.T) {
	for _, goal := range []any{nil, 3, "", strings.Repeat("x", 32001)} {
		if _, err := nativeAgentInput(Assignment{Config: map[string]any{"goal": goal}}, nativeAgentLimits{MaxSteps: 8, MaxTokens: 4096}, NativeWorkspaceBridge{}); err == nil || err.Error() != "agent config.goal must contain 1–32000 characters" {
			t.Fatal("invalid direct input did not return its bounded goal error", err)
		}
	}
}
