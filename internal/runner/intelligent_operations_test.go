package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The fixture exercises local probe, launch and complete UTF-8 result framing;
// it never contacts a model or control service.
func intelligentOperationsFixtureSpec(t *testing.T) CommandSpec {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(t.TempDir(), "operations-fixture.mjs")
	source := `import{createInterface}from'node:readline';
const out=x=>process.stdout.write(JSON.stringify(x)+'\n');
if(process.argv.includes('--version')){console.log('suma-agent-runtime fixture-protocol-only');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN --intelligent-operations-probe');process.exit(0)}
if(process.argv.includes('--intelligent-operations-probe')){if(process.env.SUMA_MODEL_API_KEY||process.env.INTENT_PROBE_TOKEN)process.exit(9);out({protocol:'suma-intelligent-operations-v1',model:process.env.SUMA_MODEL,endpoint_origin:new URL(process.env.SUMA_MODEL_BASE_URL).origin});process.exit(0)}
createInterface({input:process.stdin}).on('line',line=>{const r=JSON.parse(line);out({type:'started',request_id:r.request_id,model:process.env.SUMA_MODEL,tools:[]});out({type:'request_started',request_id:r.request_id,step:0,output_allowance:r.max_tokens});out({type:'usage',request_id:r.request_id,step:0,input_tokens:23,output_tokens:17,usage_state:'complete',usage_reported:true});process.stderr.write('separate diagnostic\n');out({type:'completed',request_id:r.request_id,output:JSON.stringify(r),usage_state:'complete'});});`
	if err = os.WriteFile(entry, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	// Decode the actual local configuration boundary so the initial regression
	// fails on ignored capability semantics, rather than a missing Go field.
	raw, err := json.Marshal(map[string]any{"adapter": "suma", "protocol": "suma-jsonl-v1", "executable": node, "args": []string{entry}, "input": "agent-jsonl", "read_only": true, "intelligent_operations": true, "env": map[string]string{"SUMA_MODEL": "fixture-operations", "SUMA_MODEL_BASE_URL": "http://127.0.0.1:1/v1", "SUMA_MODEL_API_KEY": "private-key-fixture"}, "env_from": []string{"INTENT_PROBE_TOKEN"}})
	if err != nil {
		t.Fatal(err)
	}
	var spec CommandSpec
	if err = json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestIntelligentOperationsConfigurationPurposeAndBudgets(t *testing.T) {
	spec := intelligentOperationsFixtureSpec(t)
	if err := ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"operations": spec}}); err != nil {
		t.Fatal("valid dedicated local configuration rejected", err)
	}
	for _, mutate := range []func(*CommandSpec){
		func(v *CommandSpec) { v.ReadOnly = false }, func(v *CommandSpec) { v.Planning = true },
		func(v *CommandSpec) { v.KnowledgeExtraction = true }, func(v *CommandSpec) { v.FailureDiagnosis = true },
		func(v *CommandSpec) { v.NativeWorkspace = &NativeWorkspacePolicy{ConflictRepair: true} },
		func(v *CommandSpec) { v.ModelExecutionProtocol = "suma-model-execution-v1" },
		func(v *CommandSpec) { v.TestEvidence = true }, func(v *CommandSpec) { v.CandidateComposition = true },
		func(v *CommandSpec) { v.ArtifactInput = true }, func(v *CommandSpec) { v.ArtifactOutput = "artifact" },
		func(v *CommandSpec) { v.EnvFrom = []string{"SUMA_MODEL"} },
		func(v *CommandSpec) { v.EnvFrom = []string{"suma_model_base_url"} },
	} {
		bad := spec
		mutate(&bad)
		if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"operations": bad}}) == nil {
			t.Fatal("intelligent operations configuration gained another purpose or effect")
		}
	}
	profile := structuredProfile("suma-jsonl-v1")
	profile.IntelligentOperations = &tc.IntelligentOperationsCapability{Protocol: tc.IntelligentOperationsProtocol, Model: "fixture-operations", EndpointOrigin: "http://127.0.0.1:1"}
	input, err := modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "intelligent_operations"}}, spec, profile)
	if err != nil || input["purpose"] != "intelligent_operations" || input["max_steps"] != 1 || input["max_tokens"] != 4096 {
		t.Fatal("dedicated one-call defaults unavailable", input, err)
	}
	for _, config := range []map[string]any{{}, {"purpose": "planning"}, {"purpose": "knowledge_extraction"}, {"purpose": "failure_diagnosis"}, {"purpose": "intelligent_operations", "max_steps": 2}, {"purpose": "intelligent_operations", "max_output_tokens": 8193}, {"purpose": "intelligent_operations", "max_steps": nil}, {"purpose": "intelligent_operations", "max_output_tokens": json.Number("1e0")}} {
		if _, err = modelPurposeRequest(Assignment{Type: "agent", Config: config}, spec, profile); err == nil {
			t.Fatal("invalid dedicated purpose or budget accepted", config)
		}
	}
	mixed := tc.CloneProfile(profile)
	mixed.Planning = &tc.PlanningCapability{Protocol: tc.PlanningProtocol, Model: "fixture-operations", EndpointOrigin: "http://127.0.0.1:1"}
	if _, err = modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "intelligent_operations"}}, spec, mixed); err == nil {
		t.Fatal("mixed observed capability reached the model")
	}
	raw, _ := json.Marshal(spec)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "intelligent_operations")
	raw, _ = json.Marshal(fields)
	var ordinary CommandSpec
	_ = json.Unmarshal(raw, &ordinary)
	if _, err = modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{}}, ordinary, profile); err == nil {
		t.Fatal("observed operations capability fell back to ordinary coding")
	}
}

func TestIntelligentOperationsActualProbeFullFinalUsageAndDrift(t *testing.T) {
	t.Setenv("INTENT_PROBE_TOKEN", "private-probe-token")
	spec := intelligentOperationsFixtureSpec(t)
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"operations": spec}}
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(items) != 1 || !items[0].Available || items[0].Profile.IntelligentOperations == nil {
		t.Fatal("dedicated credential-free offline probe unavailable", items, err)
	}
	profile := tc.CloneProfile(items[0].Profile)
	cfg.ToolInventory = items
	a := Assignment{RunID: "operations-run", NodeID: "analyze", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"operations"}, Config: map[string]any{"goal": strings.Repeat("fixed source 原文\n", 700), "purpose": "intelligent_operations", "max_steps": 1, "max_output_tokens": 8192, "tool_profile_sha256": profile.ProfileSHA256}}
	result := Execute(context.Background(), a, cfg)
	var completion map[string]any
	if err = json.Unmarshal([]byte(result.Output), &completion); err != nil || completion["goal"] != a.Config["goal"] || completion["purpose"] != "intelligent_operations" || completion["max_steps"] != float64(1) || completion["max_tokens"] != float64(8192) {
		t.Fatal("complete UTF-8 result clipped, prefixed or given a different budget", err)
	}
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || !tc.EqualProfile(result.ToolEvidence.Profile, profile) || result.ToolEvidence.UsageState != tc.UsageComplete || *result.ToolEvidence.Usage.InputTokens != 23 || *result.ToolEvidence.Usage.OutputTokens != 17 {
		t.Fatal("dedicated final evidence or actual usage changed", result.Error)
	}
	items[0].Profile.IntelligentOperations.Model = "foreign"
	if changed := Execute(context.Background(), a, cfg); changed.ExitCode == 0 {
		t.Fatal("tampered observed capability reached execution")
	}
	for _, replacement := range []string{"suma-knowledge-extraction-v1", "suma-intelligent-operations-v1',undeclared:true,fixture:'"} {
		bad := intelligentOperationsFixtureSpec(t)
		raw, readErr := os.ReadFile(bad.Args[0])
		if readErr != nil {
			t.Fatal(readErr)
		}
		if err = os.WriteFile(bad.Args[0], []byte(strings.Replace(string(raw), "suma-intelligent-operations-v1", replacement, 1)), 0600); err != nil {
			t.Fatal(err)
		}
		badItems, describeErr := DescribeTools(context.Background(), ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"operations": bad}})
		if describeErr != nil || len(badItems) != 1 || badItems[0].Available {
			t.Fatal("wrong or extended offline protocol advertised execution eligibility", badItems, describeErr)
		}
	}
}

func TestIntelligentOperationsStreamRejectsExtraRequestsAndTools(t *testing.T) {
	profile := structuredProfile("suma-jsonl-v1")
	profile.IntelligentOperations = &tc.IntelligentOperationsCapability{Protocol: tc.IntelligentOperationsProtocol, Model: "fixed", EndpointOrigin: "https://models.example"}
	start := "{\"type\":\"started\",\"model\":\"fixed\",\"tools\":[]}\n"
	request := "{\"type\":\"request_started\",\"step\":0,\"output_allowance\":8192}\n{\"type\":\"usage\",\"step\":0,\"usage_state\":\"unreported\",\"usage_reported\":false}\n"
	for _, scenario := range []struct{ body, reason string }{
		{start + request + "{\"type\":\"request_started\",\"step\":1,\"output_allowance\":1}\n", "invalid or out-of-order"},
		{start + "{\"type\":\"tool_started\",\"call_id\":\"invented\"}\n", "cannot request or execute tools"},
		{start + "{\"type\":\"completed\",\"output\":\"{}\"}\n", "exactly one settled model request"},
		{"{\"type\":\"started\",\"model\":\"fixed\",\"tools\":[\"project.search\"]}\n", "empty tool catalog"},
		{"{\"type\":\"started\",\"model\":\"foreign\",\"tools\":[]}\n", "fixed model"},
	} {
		stream := newToolStream(profile, nil, nil, "operations-stream")
		stream.Write([]byte(strings.ReplaceAll(scenario.body, `{"type":`, `{"request_id":"operations-stream","type":`)))
		if _, err := stream.finish(); err == nil || !strings.Contains(err.Error(), scenario.reason) {
			t.Fatal("dedicated stream rejected for the wrong reason", scenario.reason, err)
		}
	}
}
