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

func TestFailureDiagnosisStreamRejectsToolRequests(t *testing.T) {
	profile := structuredProfile("suma-jsonl-v1")
	profile.FailureDiagnosis = &tc.FailureDiagnosisCapability{Protocol: tc.FailureDiagnosisProtocol, Model: "fixed", EndpointOrigin: "https://models.example"}
	start := "{\"type\":\"started\",\"model\":\"fixed\",\"tools\":[]}\n"
	request := "{\"type\":\"request_started\",\"step\":0,\"output_allowance\":8192}\n{\"type\":\"usage\",\"step\":0,\"usage_state\":\"unreported\",\"usage_reported\":false}\n"
	identify := func(body string) []byte {
		return []byte(strings.ReplaceAll(body, `{"type":`, `{"request_id":"diagnose-stream","type":`))
	}
	for _, scenario := range []struct{ body, reason string }{
		{start + "{\"type\":\"tool_started\",\"call_id\":\"invented\"}\n", "cannot request or execute tools"},
		{start + request + "{\"type\":\"request_started\",\"step\":1,\"output_allowance\":1}\n", "invalid or out-of-order"},
		{start + "{\"type\":\"completed\",\"output\":\"{}\"}\n", "exactly one settled model request"},
		{"{\"type\":\"started\",\"model\":\"fixed\",\"tools\":[\"project.search\"]}\n", "empty tool catalog"},
		{"{\"type\":\"started\",\"model\":\"foreign\",\"tools\":[]}\n", "fixed model"},
	} {
		stream := newToolStream(profile, nil, nil, "diagnose-stream")
		stream.Write(identify(scenario.body))
		if _, err := stream.finish(); err == nil || !strings.Contains(err.Error(), scenario.reason) {
			t.Fatalf("diagnosis admitted invalid activity, want %q: %v", scenario.reason, err)
		}
	}
	var events []tc.Event
	stream := newToolStream(profile, nil, func(e tc.Event) error { events = append(events, e); return nil }, "diagnose-stream")
	stream.Write(identify(start + request + "{\"type\":\"error\",\"code\":\"failure_tools_forbidden\",\"message\":\"Failure diagnosis cannot request or execute tools.\",\"display\":{\"message_key\":\"native.failure.tools_forbidden\"}}\n"))
	if evidence, err := stream.finish(); err != nil || evidence.Terminal != "failed" || events[len(events)-1].Diagnostic == nil || events[len(events)-1].Diagnostic.MessageKey != "native.failure.tools_forbidden" {
		t.Fatal("paired diagnosis diagnostic lost its exact source proof", err)
	}
}

// The local Node fixture exercises real probe/launch/wire framing, not vendor inference.
func failureDiagnosisFixtureSpec(t *testing.T) CommandSpec {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(t.TempDir(), "diagnosis-fixture.mjs")
	source := `import{createInterface}from'node:readline';
const out=x=>process.stdout.write(JSON.stringify(x)+'\n');
if(process.argv.includes('--version')){console.log('suma-agent-runtime fixture-protocol-only');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN --failure-diagnosis-probe');process.exit(0)}
if(process.argv.includes('--failure-diagnosis-probe')){out({protocol:'suma-failure-diagnosis-v1',model:process.env.SUMA_MODEL,endpoint_origin:new URL(process.env.SUMA_MODEL_BASE_URL).origin});process.exit(0)}
createInterface({input:process.stdin}).on('line',line=>{const r=JSON.parse(line);out({type:'started',request_id:r.request_id,model:process.env.SUMA_MODEL,tools:[]});out({type:'request_started',request_id:r.request_id,step:0,output_allowance:r.max_tokens});out({type:'usage',request_id:r.request_id,step:0,input_tokens:23,output_tokens:17,usage_state:'complete',usage_reported:true});process.stderr.write('separate diagnostic\n');out({type:'completed',request_id:r.request_id,output:JSON.stringify(r),usage_state:'complete'});});`
	if err = os.WriteFile(entry, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true, FailureDiagnosis: true, Env: map[string]string{"SUMA_MODEL": "fixture-diagnostician", "SUMA_MODEL_BASE_URL": "http://127.0.0.1:1/v1"}}
}
func TestFailureDiagnosisConfigurationPurposeAndBudgets(t *testing.T) {
	spec := failureDiagnosisFixtureSpec(t)
	for _, mutate := range []func(*CommandSpec){
		func(v *CommandSpec) { v.ReadOnly = false }, func(v *CommandSpec) { v.Planning = true }, func(v *CommandSpec) { v.KnowledgeExtraction = true },
		func(v *CommandSpec) { v.NativeWorkspace = &NativeWorkspacePolicy{} }, func(v *CommandSpec) { v.TestEvidence = true }, func(v *CommandSpec) { v.ArtifactInput = true }, func(v *CommandSpec) { v.ArtifactOutput = "artifact" }, func(v *CommandSpec) { v.EnvFrom = []string{"suma_model_base_url"} },
	} {
		v := spec
		mutate(&v)
		if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"diagnostician": v}}) == nil {
			t.Fatal("dedicated diagnosis configuration gained another purpose or effect")
		}
	}
	profile := structuredProfile("suma-jsonl-v1")
	profile.FailureDiagnosis, _ = localFailureDiagnosisCapability(spec)
	profile.ReadOnly = true
	for _, config := range []map[string]any{{}, {"purpose": "planning"}, {"purpose": "knowledge_extraction"}, {"purpose": "failure_diagnosis", "max_steps": 2}, {"purpose": "failure_diagnosis", "max_output_tokens": 8193}, {"purpose": "failure_diagnosis", "max_steps": nil}, {"purpose": "failure_diagnosis", "max_output_tokens": json.Number("1e0")}} {
		if _, err := modelPurposeRequest(Assignment{Type: "agent", Config: config}, spec, profile); err == nil {
			t.Fatal("invalid dedicated purpose/budget accepted", config)
		}
	}
	input, err := modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "failure_diagnosis"}}, spec, profile)
	if err != nil || input["max_steps"] != 1 || input["max_tokens"] != 4096 {
		t.Fatal(input, err)
	}
	ordinary := spec
	ordinary.FailureDiagnosis = false
	if _, err = modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "failure_diagnosis"}}, ordinary, profile); err == nil {
		t.Fatal("ordinary profile admitted diagnosis purpose")
	}
}
func TestFailureDiagnosisActualProbeFullFinalUsageDriftAndInventoryClone(t *testing.T) {
	spec := failureDiagnosisFixtureSpec(t)
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"diagnostician": spec}}
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(items) != 1 || !items[0].Available || items[0].Profile.FailureDiagnosis == nil || items[0].Profile.KnowledgeExtraction != nil || items[0].Profile.Planning != nil {
		t.Fatal("dedicated offline probe unavailable", items, err)
	}
	profile := tc.CloneProfile(items[0].Profile)
	cfg.ToolInventory = items
	a := Assignment{RunID: "diagnosis-run", NodeID: "diagnose", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"diagnostician"}, Config: map[string]any{"goal": strings.Repeat("fixed evidence 原文\n", 700), "purpose": "failure_diagnosis", "max_steps": 1, "max_output_tokens": 8192, "tool_profile_sha256": profile.ProfileSHA256}}
	result := Execute(context.Background(), a, cfg)
	var completion map[string]any
	if err = json.Unmarshal([]byte(result.Output), &completion); err != nil || completion["goal"] != a.Config["goal"] || completion["purpose"] != "failure_diagnosis" || completion["max_steps"] != float64(1) || completion["max_tokens"] != float64(8192) {
		t.Fatal("final document clipped or prefixed", err)
	}
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || !tc.EqualProfile(result.ToolEvidence.Profile, profile) || result.ToolEvidence.UsageState != tc.UsageComplete || *result.ToolEvidence.Usage.InputTokens != 23 || *result.ToolEvidence.Usage.OutputTokens != 17 {
		t.Fatal("actual final evidence or usage changed", result.Error)
	}
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	report, err := j.NewInventory("worker", items)
	if err != nil {
		t.Fatal(err)
	}
	report.Items[0].Profile.FailureDiagnosis.Model = "output-mutation"
	pending := j.PendingInventory()
	pending.Items[0].Profile.FailureDiagnosis.Model = "pending-mutation"
	if j.PendingInventory().Items[0].Profile.FailureDiagnosis.Model != profile.FailureDiagnosis.Model {
		t.Fatal("diagnosis inventory capability aliased caller")
	}
	items[0].Profile.FailureDiagnosis.Model = "foreign"
	if j.PendingInventory().Items[0].Profile.FailureDiagnosis.Model != profile.FailureDiagnosis.Model {
		t.Fatal("input capability aliases durable inventory")
	}
	if changed := Execute(context.Background(), a, cfg); changed.ExitCode == 0 || !strings.Contains(changed.Error, "failure diagnosis capability") {
		t.Fatal("tampered capability reached execution", changed.Error)
	}
	spec.Env["SUMA_MODEL"] = "local-drift"
	cfg.Commands["diagnostician"] = spec
	if changed := Execute(context.Background(), a, cfg); changed.ExitCode == 0 || !strings.Contains(changed.Error, "profile") {
		t.Fatal("fixed local model drift reached execution", changed.Error)
	}
	for _, replacement := range []string{"suma-knowledge-extraction-v1", "suma-failure-diagnosis-v1',undeclared:true,fixture:'"} {
		bad := failureDiagnosisFixtureSpec(t)
		raw, err := os.ReadFile(bad.Args[0])
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(bad.Args[0], []byte(strings.Replace(string(raw), "suma-failure-diagnosis-v1", replacement, 1)), 0600); err != nil {
			t.Fatal(err)
		}
		observed, err := DescribeTools(context.Background(), ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"diagnostician": bad}})
		if err != nil || len(observed) != 1 || observed[0].Available || !strings.Contains(observed[0].Reason, "probe") {
			t.Fatal("invalid purpose probe became available", observed, err)
		}
	}
}
