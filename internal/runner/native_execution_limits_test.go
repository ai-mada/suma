package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This bounded child records only limit fields, not the bridge credential. It
// exercises actual stdin delivery without a provider, SDK loop or formal test.
func nativeLimitFixture(t *testing.T, repair bool) (ExecConfig, string) {
	t.Helper()
	cfg, entry := nativeHostFixture(t)
	capture := filepath.Join(filepath.Dir(entry), "received-limits.json")
	script := `import {createInterface} from 'node:readline';import {writeFileSync} from 'node:fs';
if(process.argv.includes('--version')){console.log('suma-agent-runtime 1.0.0');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN');process.exit(0)}
const lines=createInterface({input:process.stdin});lines.once('line',line=>{const r=JSON.parse(line);writeFileSync(process.env.SUMA_LIMIT_CAPTURE,JSON.stringify({max_steps:r.max_steps,max_tokens:r.max_tokens,bridge_present:!!r.workspace_bridge,repair_protocol:r.workspace_bridge?.conflict_repair_protocol||''}));const emit=e=>console.log(JSON.stringify({...e,request_id:r.request_id}));emit({type:'started',model:'host-fixture',tools:[]});emit({type:'request_started',step:0,output_allowance:1});emit({type:'usage',step:0,input_tokens:0,output_tokens:0,usage_state:'complete',usage_reported:true});emit({type:'completed',output:'Limits received',usage_state:'complete',output_tokens:0});lines.close()});`
	if err := os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	spec := cfg.Commands["native"]
	spec.NativeWorkspace.ConflictRepair = repair
	spec.Env = map[string]string{"SUMA_LIMIT_CAPTURE": capture}
	manifest := nativeRuntimeManifest{SchemaVersion: 1, Entry: filepath.Base(entry), EntrySHA256: digest([]byte(script)), ExternalImports: []string{"node:readline", "node:fs"}, Dependencies: map[string]string{"@modelcontextprotocol/client": "2.4.0"}}
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
	profile := tc.Profile{CommandID: "native", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: profileSHA, ExecutableSHA256: executableSHA, ToolVersion: "limit-host-fixture", WorkspaceToolsVersion: NativeWorkspaceVersion}
	if repair {
		profile.ConflictRepairProtocol = ccc.CandidateConflictRepairProtocol
	}
	cfg.ToolInventory = []tc.Observation{{Profile: profile, Available: true}}
	return cfg, capture
}

func assertReceivedNativeLimits(t *testing.T, capture string, steps, tokens int, repair bool) {
	t.Helper()
	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal("child did not receive stdin", err)
	}
	var received struct {
		Steps    int    `json:"max_steps"`
		Tokens   int    `json:"max_tokens"`
		Bridge   bool   `json:"bridge_present"`
		Protocol string `json:"repair_protocol"`
	}
	if err = json.Unmarshal(raw, &received); err != nil || received.Steps != steps || received.Tokens != tokens || !received.Bridge || (received.Protocol == ccc.CandidateConflictRepairProtocol) != repair {
		t.Fatalf("stdin lost the validated execution limits: %s, %v", raw, err)
	}
}

func TestNativeExecutionLimitsReachActualStdin(t *testing.T) {
	cfg, capture := nativeLimitFixture(t, false)
	for _, explicit := range []bool{false, true} {
		a := Assignment{RunID: fmt.Sprintf("limits-%t", explicit), NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "Verify only stdin", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256}}
		steps, tokens := 8, 4096
		if explicit {
			a.Config["max_steps"], a.Config["max_output_tokens"] = json.Number("16"), float64(32768)
			steps, tokens = 16, 32768
		}
		result := Execute(context.Background(), a, cfg)
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown {
			t.Fatalf("bounded native stdin fixture failed: %+v", result)
		}
		assertReceivedNativeLimits(t, capture, steps, tokens, false)
	}

	// The exact typed repair purpose is validated before using the same payload
	// builder as Execute. Its real stdin is checked without creating a fake
	// conflict/worktree or claiming this is an independently tested repair run.
	repairCfg, repairCapture := nativeLimitFixture(t, true)
	for _, explicit := range []bool{false, true} {
		a := startupRepairAssignment(t)
		steps, tokens := 64, 4096
		if explicit {
			a.Config["max_steps"], a.Config["max_output_tokens"] = int64(128), json.Number("131072")
			steps, tokens = 128, 131072
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
		child := exec.Command(spec.Executable, spec.Args...)
		child.Env, _, err = commandEnvironment(spec)
		if err != nil {
			t.Fatal(err)
		}
		child.Stdin = bytes.NewReader(append(payload, '\n'))
		if output, err := child.CombinedOutput(); err != nil {
			t.Fatalf("repair stdin fixture: %v %s", err, output)
		}
		assertReceivedNativeLimits(t, repairCapture, steps, tokens, true)
	}
}

func TestNativeExecutionLimitsRejectBeforeWorkspace(t *testing.T) {
	invalid := []any{nil, "2", true, 0, -1, 1.5, math.NaN(), math.Inf(1), json.Number("1.0"), json.Number("1e0"), json.Number("01"), json.Number("+1"), json.Number("9007199254740992")}
	for _, name := range []string{"max_steps", "max_output_tokens"} {
		for _, value := range invalid {
			if _, err := nativeExecutionLimits(Assignment{Config: map[string]any{name: value}}); err == nil || err.Error() != "Native agent execution limits are invalid" {
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

func TestNativeExecutionLimitsRepairStreamBoundary(t *testing.T) {
	for _, test := range []struct {
		name, protocol   string
		steps, allowance int
		valid            bool
	}{
		{"repair128", ccc.CandidateConflictRepairProtocol, 128, 1024, true},
		{"repair129", ccc.CandidateConflictRepairProtocol, 129, 1024, false},
		{"repair_allowance", ccc.CandidateConflictRepairProtocol, 1, 1025, false},
		{"ordinary16", "", 16, 4096, true},
		{"ordinary17", "", 17, 4096, false},
		{"foreign_marker", "foreign-protocol", 17, 1024, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := structuredProfile("suma-jsonl-v1")
			profile.ReadOnly, profile.WorkspaceToolsVersion, profile.ConflictRepairProtocol = false, NativeWorkspaceVersion, test.protocol
			stream := newToolStream(profile, nil, nil, "limits-request")
			frames := strings.Builder{}
			frames.WriteString("{\"type\":\"started\",\"request_id\":\"limits-request\",\"tools\":[]}\n")
			for step := 0; step < test.steps; step++ {
				fmt.Fprintf(&frames, "{\"type\":\"request_started\",\"request_id\":\"limits-request\",\"step\":%d,\"output_allowance\":%d}\n", step, test.allowance)
				fmt.Fprintf(&frames, "{\"type\":\"usage\",\"request_id\":\"limits-request\",\"step\":%d,\"input_tokens\":0,\"output_tokens\":0,\"usage_state\":\"complete\",\"usage_reported\":true}\n", step)
			}
			frames.WriteString("{\"type\":\"completed\",\"request_id\":\"limits-request\",\"output\":\"Frame validation only\",\"usage_state\":\"complete\"}\n")
			_, _ = stream.Write([]byte(frames.String()))
			evidence, err := stream.finish()
			if test.valid != (err == nil && evidence.Terminal == "completed") {
				t.Fatalf("frame boundary valid=%v: %+v %v", test.valid, evidence, err)
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
