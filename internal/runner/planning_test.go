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

func planningFixtureSpec(t *testing.T) CommandSpec {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for planning protocol fixture")
	}
	entry := filepath.Join(t.TempDir(), "planning-fixture.mjs")
	source := `import{createInterface}from'node:readline';
const out=x=>process.stdout.write(JSON.stringify(x)+'\n');
if(process.argv.includes('--version')){console.log('suma-agent-runtime fixture-protocol-only');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN --planning-probe');process.exit(0)}
if(process.argv.includes('--planning-probe')){out({protocol:'suma-schedule-plan-v1',model:process.env.SUMA_MODEL,endpoint_origin:new URL(process.env.SUMA_MODEL_BASE_URL).origin});process.exit(0)}
createInterface({input:process.stdin}).on('line',line=>{const r=JSON.parse(line);out({type:'started',request_id:r.request_id});out({type:'request_started',request_id:r.request_id,step:0,output_allowance:r.max_tokens??4096});out({type:'usage',request_id:r.request_id,step:0,usage_state:'unreported',usage_reported:false});out({type:'completed',request_id:r.request_id,output:JSON.stringify(r)});});`
	if err = os.WriteFile(entry, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true, Planning: true, Env: map[string]string{"SUMA_MODEL": "fixture", "SUMA_MODEL_BASE_URL": "http://127.0.0.1:1/v1"}}
}

func TestPlanningConfigurationAndBoundedInput(t *testing.T) {
	spec := planningFixtureSpec(t)
	for name, mutate := range map[string]func(*CommandSpec){"write": func(s *CommandSpec) { s.ReadOnly = false }, "adapter": func(s *CommandSpec) { s.Adapter = "command" }, "forward-model": func(s *CommandSpec) { s.EnvFrom = []string{"SUMA_MODEL"} }, "forward-endpoint-case": func(s *CommandSpec) { s.EnvFrom = []string{"suma_model_base_url"} }, "workspace": func(s *CommandSpec) { s.NativeWorkspace = &NativeWorkspacePolicy{} }, "missing-model": func(s *CommandSpec) { s.Env = map[string]string{"SUMA_MODEL_BASE_URL": "https://models.example/v1"} }} {
		t.Run(name, func(t *testing.T) {
			bad := spec
			mutate(&bad)
			if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"planner": bad}}) == nil {
				t.Fatal("unsafe planning config accepted")
			}
		})
	}
	p, _ := localPlanningCapability(spec)
	profile := structuredProfile("suma-jsonl-v1")
	profile.Planning = p
	for _, config := range []map[string]any{{}, {"purpose": "planning", "max_steps": 5}, {"purpose": "planning", "max_output_tokens": 8193}, {"purpose": "planning", "max_steps": 1.5}, {"purpose": "planning", "max_steps": nil}, {"purpose": "other"}} {
		if _, err := planningRequest(Assignment{Type: "agent", Config: config}, spec, profile); err == nil {
			t.Fatalf("accepted invalid config: %+v", config)
		}
	}
	input, err := planningRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "planning"}}, spec, profile)
	if err != nil || input["max_steps"] != 3 || input["max_tokens"] != 4096 {
		t.Fatalf("defaults: %+v %v", input, err)
	}
	plain := spec
	plain.Planning = false
	profile.Planning = nil
	if _, err = planningRequest(Assignment{Type: "agent", Config: map[string]any{"purpose": "planning"}}, plain, profile); err == nil {
		t.Fatal("ordinary agent became planner")
	}
	if input, err = planningRequest(Assignment{Type: "agent", Config: map[string]any{"goal": "ordinary"}}, plain, profile); err != nil || len(input) != 0 {
		t.Fatal("ordinary input changed")
	}
}

func TestPlanningActualNodeProbeExecutionAndProfileDrift(t *testing.T) {
	spec := planningFixtureSpec(t)
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"planner": spec}}
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(items) != 1 || !items[0].Available || items[0].Profile.Planning == nil {
		t.Fatalf("probe: %+v %v", items, err)
	}
	profile := items[0].Profile
	raw, _ := json.Marshal(items)
	if err = json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	cfg.ToolInventory = items
	a := Assignment{RunID: "planning-run", NodeID: "planner", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"planner"}, Config: map[string]any{"goal": strings.Repeat("fixture", 1500), "purpose": "planning", "max_steps": float64(4), "max_output_tokens": float64(8192), "tool_profile_sha256": profile.ProfileSHA256}}
	result := Execute(context.Background(), a, cfg)
	var completion map[string]any
	if err = json.Unmarshal([]byte(result.Output), &completion); err != nil || completion["goal"] != a.Config["goal"] {
		t.Fatalf("planning completion was prefixed or truncated: %v", err)
	}
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || !tc.EqualProfile(result.ToolEvidence.Profile, profile) || !strings.Contains(result.Output, `"purpose":"planning"`) || !strings.Contains(result.Output, `"max_tokens":8192`) {
		t.Fatalf("actual fixture result: %+v", result)
	}
	items[0].Profile.Planning.Model = "foreign"
	result = Execute(context.Background(), a, cfg)
	if result.ExitCode == 0 || !strings.Contains(result.Error, "planning capability") {
		t.Fatalf("mismatched declaration launched: %+v", result)
	}
	spec.Env["SUMA_MODEL"] = "changed"
	cfg.Commands["planner"] = spec
	result = Execute(context.Background(), a, cfg)
	if result.ExitCode == 0 || !strings.Contains(result.Error, "profile") {
		t.Fatal("fixed model drift launched")
	}
}

func TestPlanningInventoryPointerSnapshots(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	p := structuredProfile("suma-jsonl-v1")
	p.Planning = &tc.PlanningCapability{Protocol: tc.PlanningProtocol, Model: "fixed", EndpointOrigin: "https://models.example"}
	items := []tc.Observation{{Profile: p, Available: true, Reason: "fixture"}}
	report, err := j.NewInventory("worker", items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].Profile.Planning.Model = "input-mutated"
	report.Items[0].Profile.Planning.Model = "output-mutated"
	pending := j.PendingInventory()
	if pending.Items[0].Profile.Planning.Model != "fixed" {
		t.Fatal("durable planning report aliases caller")
	}
	pending.Items[0].Profile.Planning.Model = "pending-mutated"
	if j.PendingInventory().Items[0].Profile.Planning.Model != "fixed" {
		t.Fatal("pending report aliases journal")
	}
}
