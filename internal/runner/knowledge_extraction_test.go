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

func knowledgeExtractionFixtureSpec(t *testing.T) CommandSpec {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(t.TempDir(), "extraction-fixture.mjs")
	source := `import{createInterface}from'node:readline';
const out=x=>process.stdout.write(JSON.stringify(x)+'\n');
if(process.argv.includes('--version')){console.log('suma-agent-runtime fixture-protocol-only');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN --knowledge-extraction-probe');process.exit(0)}
if(process.argv.includes('--knowledge-extraction-probe')){out({protocol:'suma-knowledge-extraction-v1',model:process.env.SUMA_MODEL,endpoint_origin:new URL(process.env.SUMA_MODEL_BASE_URL).origin});process.exit(0)}
createInterface({input:process.stdin}).on('line',line=>{const r=JSON.parse(line);out({type:'started',request_id:r.request_id,model:process.env.SUMA_MODEL,tools:[]});out({type:'request_started',request_id:r.request_id,step:0,output_allowance:r.max_tokens});out({type:'usage',request_id:r.request_id,step:0,input_tokens:23,output_tokens:17,usage_state:'complete',usage_reported:true});process.stderr.write('separate diagnostic\n');out({type:'completed',request_id:r.request_id,output:JSON.stringify(r),usage_state:'complete'});});`
	if err = os.WriteFile(entry, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true, KnowledgeExtraction: true, Env: map[string]string{"SUMA_MODEL": "fixture-extractor", "SUMA_MODEL_BASE_URL": "http://127.0.0.1:1/v1"}}
}

func TestKnowledgeExtractionConfigurationAndPurposeBoundaries(t *testing.T) {
	spec := knowledgeExtractionFixtureSpec(t)
	for _, mutate := range []func(*CommandSpec){
		func(v *CommandSpec) { v.ReadOnly = false }, func(v *CommandSpec) { v.Planning = true },
		func(v *CommandSpec) { v.NativeWorkspace = &NativeWorkspacePolicy{} }, func(v *CommandSpec) { v.TestEvidence = true },
		func(v *CommandSpec) { v.ArtifactInput = true }, func(v *CommandSpec) { v.ArtifactOutput = "artifact" },
		func(v *CommandSpec) { v.EnvFrom = []string{"SUMA_MODEL"} }, func(v *CommandSpec) { v.EnvFrom = []string{"suma_model_base_url"} },
		func(v *CommandSpec) {
			v.Env = map[string]string{"suma_model": "fixture", "SUMA_MODEL_BASE_URL": "https://models.example/v1"}
		},
		func(v *CommandSpec) {
			v.Env = map[string]string{"SUMA_MODEL": "fixed", "SUMA_MODEL_BASE_URL": "http://remote.example/v1"}
		},
	} {
		bad := spec
		mutate(&bad)
		if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"extractor": bad}}) == nil {
			t.Fatal("unsafe extraction configuration accepted")
		}
	}
	profile := structuredProfile("suma-jsonl-v1")
	profile.KnowledgeExtraction, _ = localKnowledgeExtractionCapability(spec)
	for _, config := range []map[string]any{{}, {"purpose": "planning"}, {"purpose": "knowledge_extraction", "max_steps": 2}, {"purpose": "knowledge_extraction", "max_output_tokens": 8193}, {"purpose": "knowledge_extraction", "max_steps": nil}, {"purpose": "knowledge_extraction", "max_output_tokens": json.Number("1e0")}} {
		if _, err := modelPurposeRequest(Assignment{Type: "agent", Config: config}, spec, profile); err == nil {
			t.Fatalf("invalid extraction purpose or budget accepted: %+v", config)
		}
	}
	input, err := modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "knowledge_extraction"}}, spec, profile)
	if err != nil || input["max_steps"] != 1 || input["max_tokens"] != 4096 {
		t.Fatal("single-request budget defaults changed", input, err)
	}
	ordinary := spec
	ordinary.KnowledgeExtraction = false
	profile.KnowledgeExtraction = nil
	if _, err = modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "knowledge_extraction"}}, ordinary, profile); err == nil {
		t.Fatal("ordinary profile silently became an extractor")
	}
	if input, err = modelPurposeRequest(Assignment{Type: "agent", Config: map[string]any{}}, ordinary, profile); err != nil || len(input) != 0 {
		t.Fatal("ordinary agent behavior changed")
	}
}

func TestKnowledgeExtractionActualProbeFullFinalUsageAndDrift(t *testing.T) {
	spec := knowledgeExtractionFixtureSpec(t)
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"extractor": spec}}
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(items) != 1 || !items[0].Available || items[0].Profile.KnowledgeExtraction == nil || items[0].Profile.Planning != nil {
		t.Fatal("dedicated actual offline probe failed", items, err)
	}
	profile := tc.CloneProfile(items[0].Profile)
	raw, _ := json.Marshal(items)
	json.Unmarshal(raw, &items)
	cfg.ToolInventory = items
	a := Assignment{RunID: "extraction-run", NodeID: "extract", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"extractor"}, Config: map[string]any{"goal": strings.Repeat("fixed source 原文\n", 700), "purpose": "knowledge_extraction", "max_steps": 1, "max_output_tokens": 8192, "tool_profile_sha256": profile.ProfileSHA256}}
	result := Execute(context.Background(), a, cfg)
	var completion map[string]any
	if err = json.Unmarshal([]byte(result.Output), &completion); err != nil || completion["goal"] != a.Config["goal"] || completion["purpose"] != "knowledge_extraction" || completion["max_steps"] != float64(1) || completion["max_tokens"] != float64(8192) {
		t.Fatal("completed final JSON was clipped, prefixed or lost exact request bounds", err)
	}
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || !tc.EqualProfile(result.ToolEvidence.Profile, profile) || result.ToolEvidence.UsageState != tc.UsageComplete || *result.ToolEvidence.Usage.InputTokens != 23 || *result.ToolEvidence.Usage.OutputTokens != 17 {
		t.Fatalf("actual usage/final evidence changed: %+v", result)
	}
	items[0].Profile.KnowledgeExtraction.Model = "foreign"
	if changed := Execute(context.Background(), a, cfg); changed.ExitCode == 0 || !strings.Contains(changed.Error, "knowledge extraction capability") {
		t.Fatal("tampered capability reached execution", changed.Error)
	}
	spec.Env["SUMA_MODEL"] = "local-drift"
	cfg.Commands["extractor"] = spec
	if changed := Execute(context.Background(), a, cfg); changed.ExitCode == 0 || !strings.Contains(changed.Error, "profile") {
		t.Fatal("local fixed model drift reached execution", changed.Error)
	}
	// Version/help alone cannot advertise this purpose. Its own offline probe
	// must return exactly the declared identity, without undeclared fields.
	for _, replacement := range []string{"suma-schedule-plan-v1", "suma-knowledge-extraction-v1',undeclared:true,fixture:'"} {
		bad := knowledgeExtractionFixtureSpec(t)
		entry, readErr := os.ReadFile(bad.Args[0])
		if readErr != nil {
			t.Fatal(readErr)
		}
		entry = []byte(strings.Replace(string(entry), "suma-knowledge-extraction-v1", replacement, 1))
		if err = os.WriteFile(bad.Args[0], entry, 0600); err != nil {
			t.Fatal(err)
		}
		badItems, describeErr := DescribeTools(context.Background(), ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"extractor": bad}})
		if describeErr != nil || len(badItems) != 1 || badItems[0].Available || !strings.Contains(badItems[0].Reason, "probe") {
			t.Fatal("invalid dedicated probe advertised an available extraction profile", badItems, describeErr)
		}
	}
}

func TestKnowledgeExtractionRejectsExtraRequestsToolsAndAliasedInventory(t *testing.T) {
	profile := structuredProfile("suma-jsonl-v1")
	profile.KnowledgeExtraction = &tc.KnowledgeExtractionCapability{Protocol: tc.KnowledgeExtractionProtocol, Model: "fixed", EndpointOrigin: "https://models.example"}
	start := "{\"type\":\"started\",\"model\":\"fixed\",\"tools\":[]}\n"
	request := "{\"type\":\"request_started\",\"step\":0,\"output_allowance\":8192}\n{\"type\":\"usage\",\"step\":0,\"usage_state\":\"unreported\",\"usage_reported\":false}\n"
	identify := func(body string) []byte {
		return []byte(strings.ReplaceAll(body, `{"type":`, `{"request_id":"extract-stream","type":`))
	}
	for _, scenario := range []struct{ body, reason string }{
		{start + request + "{\"type\":\"request_started\",\"step\":1,\"output_allowance\":1}\n", "invalid or out-of-order"},
		{start + "{\"type\":\"tool_started\",\"call_id\":\"invented\"}\n", "cannot request or execute tools"},
		{start + "{\"type\":\"completed\",\"output\":\"{}\"}\n", "exactly one settled model request"},
		{"{\"type\":\"started\",\"model\":\"fixed\",\"tools\":[\"project.search\"]}\n", "empty tool catalog"},
	} {
		s := newToolStream(profile, nil, nil, "extract-stream")
		s.Write(identify(scenario.body))
		if _, err := s.finish(); err == nil || !strings.Contains(err.Error(), scenario.reason) {
			t.Fatalf("extraction protocol rejected for the wrong reason, want %q: %v", scenario.reason, err)
		}
	}
	var events []tc.Event
	stream := newToolStream(profile, nil, func(event tc.Event) error { events = append(events, event); return nil }, "extract-stream")
	stream.Write(identify(start + request + "{\"type\":\"error\",\"code\":\"knowledge_tools_forbidden\",\"message\":\"Knowledge extraction cannot request or execute tools.\",\"display\":{\"message_key\":\"native.knowledge.tools_forbidden\"}}\n"))
	if evidence, finishErr := stream.finish(); finishErr != nil || evidence.Terminal != "failed" || events[len(events)-1].Diagnostic == nil || events[len(events)-1].Diagnostic.MessageKey != "native.knowledge.tools_forbidden" {
		t.Fatal("paired native diagnostic lost its exact raw-source proof", finishErr)
	}
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	items := []tc.Observation{{Profile: profile, Available: true}}
	report, err := j.NewInventory("worker", items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].Profile.KnowledgeExtraction.Model = "input mutation"
	report.Items[0].Profile.KnowledgeExtraction.Model = "output mutation"
	pending := j.PendingInventory()
	if pending.Items[0].Profile.KnowledgeExtraction.Model != "fixed" {
		t.Fatal("inventory aliases a caller capability")
	}
	pending.Items[0].Profile.KnowledgeExtraction.Model = "pending mutation"
	if j.PendingInventory().Items[0].Profile.KnowledgeExtraction.Model != "fixed" {
		t.Fatal("pending inventory aliases durable state")
	}
}
