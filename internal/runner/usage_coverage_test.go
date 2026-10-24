package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func assertUsageFacts(t *testing.T, evidence any, state string, counts map[string]any) {
	t.Helper()
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var facts map[string]any
	if err = json.Unmarshal(raw, &facts); err != nil {
		t.Fatal(err)
	}
	if facts["usage_state"] != state {
		t.Fatalf("coverage is not explicit: %s", raw)
	}
	if counts == nil {
		if facts["usage"] != nil {
			t.Fatalf("unknown usage turned into measured zero: %s", raw)
		}
		return
	}
	usage, ok := facts["usage"].(map[string]any)
	if !ok || len(usage) != len(counts) {
		t.Fatalf("incorrect count presence: %s", raw)
	}
	for name, expected := range counts {
		if usage[name] != expected {
			t.Fatalf("count %s changed: %s", name, raw)
		}
	}
}

func TestNativeUsageCoverageTracksEveryStartedRequest(t *testing.T) {
	prefix := `{"type":"started","request_id":"expected"}` + "\n"
	start := func(step int) string {
		return fmt.Sprintf(`{"type":"request_started","request_id":"expected","step":%d,"output_allowance":5}`+"\n", step)
	}
	known := func(step int) string {
		return fmt.Sprintf(`{"type":"usage","request_id":"expected","step":%d,"usage_reported":true,"usage_state":"complete","input_tokens":0,"output_tokens":0}`+"\n", step)
	}
	missing := `{"type":"usage","request_id":"expected","step":1,"usage_reported":false,"usage_state":"unreported"}` + "\n"
	end := `{"type":"completed","request_id":"expected","output":"fixture"}` + "\n"
	for _, test := range []struct {
		name, body, state string
		counts            map[string]any
		invalid           bool
	}{
		{"zero-full", start(0) + known(0) + end, "complete", map[string]any{"input_tokens": float64(0), "output_tokens": float64(0)}, false},
		{"mixed", start(0) + known(0) + start(1) + missing + end, "partial", map[string]any{"input_tokens": float64(0), "output_tokens": float64(0)}, false},
		{"pending-second", start(0) + known(0) + start(1), "partial", map[string]any{"input_tokens": float64(0), "output_tokens": float64(0)}, true},
		{"none", end, "unreported", nil, false},
		{"unreported", start(0) + strings.ReplaceAll(missing, `"step":1`, `"step":0`) + end, "unreported", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "expected")
			_, _ = s.Write([]byte(prefix + test.body))
			e, err := s.finish()
			if (err != nil) != test.invalid {
				t.Fatalf("unexpected terminal validation: %+v %v", e, err)
			}
			assertUsageFacts(t, e, test.state, test.counts)
		})
	}
}

func TestNativeUsageRejectsStepMismatchAndUnsafeAggregate(t *testing.T) {
	prefix := `{"type":"started","request_id":"expected"}` + "\n"
	for _, body := range []string{
		`{"type":"request_started","step":1,"output_allowance":5}`,
		`{"type":"request_started","step":0,"output_allowance":0}`,
		`{"type":"usage","step":0,"usage_state":"complete","usage_reported":true,"input_tokens":0,"output_tokens":0}`,
		`{"type":"request_started","step":0,"output_allowance":5}` + "\n" + `{"type":"usage","step":1,"usage_state":"complete","usage_reported":true,"input_tokens":0,"output_tokens":0}`,
		`{"type":"request_started","step":0,"output_allowance":5}` + "\n" + `{"type":"usage","step":0,"usage_state":"complete","usage_reported":true,"input_tokens":9007199254740992,"output_tokens":0}`,
		`{"type":"request_started","step":0,"output_allowance":5}` + "\n" + `{"type":"tool_started","call_id":"premature"}` + "\n" + `{"type":"usage","step":0,"usage_state":"complete","usage_reported":true,"input_tokens":0,"output_tokens":0}`,
	} {
		s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "expected")
		_, _ = s.Write([]byte(prefix))
		for _, line := range strings.Split(body, "\n") {
			var obj map[string]any
			_ = json.Unmarshal([]byte(line), &obj)
			obj["request_id"] = "expected"
			raw, _ := json.Marshal(obj)
			_, _ = s.Write(append(raw, '\n'))
		}
		_, _ = s.Write([]byte(`{"type":"completed","request_id":"expected","output":"fixture"}` + "\n"))
		e, err := s.finish()
		if err == nil || e.Terminal == "completed" {
			t.Fatalf("invalid request usage accepted %+v %v", e, err)
		}
	}
}

func TestActualNativeRejectionClassDoesNotGuessFromTextOrUnrelatedHTTP(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for explicit rejection transport fixture")
	}
	entry := filepath.Join(t.TempDir(), "rejection-fixture.js")
	script := `if(process.argv[2]==='--version'){console.log('suma-agent-runtime fixture-rejection-transport-only');process.exit(0)}if(process.argv[2]==='--help'){console.log('JSONL SUMA_AGENT_TOKEN');process.exit(0)}let raw='';process.stdin.on('data',b=>raw+=b);process.stdin.on('end',()=>{const r=JSON.parse(raw),mode=process.env.SUMA_REJECTION_FIXTURE;const emit=e=>console.log(JSON.stringify({...e,request_id:r.request_id}));emit({type:'started'});emit({type:'request_started',step:0,output_allowance:5});emit({type:'usage',step:0,usage_state:'unreported',usage_reported:false});const e={type:'error',code:mode==='network'?'runtime_error':'upstream_rejected',message:'HTTP 429 fixture text alone is not evidence'};if(mode!=='text'&&mode!=='network')Object.assign(e,{source:mode==='control'?'control':'model',http_status:429,failure_class:'transient'});emit(e);process.exitCode=1;});`
	if err = os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"known", "text", "network", "control"} {
		t.Run(mode, func(t *testing.T) {
			cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"native": {Executable: node, Adapter: "suma", Protocol: "suma-jsonl-v1", Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true, Env: map[string]string{"SUMA_REJECTION_FIXTURE": mode}}}}
			cfg.ToolInventory = describeForTest(t, cfg)
			if !cfg.ToolInventory[0].Available {
				t.Fatal("fixture unavailable")
			}
			r := Execute(context.Background(), Assignment{Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "rejection transport fixture only", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256}}, cfg)
			want := ""
			if mode == "known" {
				want = "transient"
			}
			if r.FailureClass != want || r.ExitCode == 0 || r.OutcomeUnknown {
				t.Fatalf("incorrect classification: %+v", r)
			}
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), `"failure_class":"transient"`) != (mode == "known") {
				t.Fatalf("wire classification lost or invented: %s", raw)
			}
		})
	}
}

func TestUsageJournalCannotMutateReportedZeroThroughPointers(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1, WorkerID: "w"}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	input, output := int64(0), int64(0)
	profile := structuredProfile("suma-jsonl-v1")
	e := tc.Event{SchemaVersion: 1, CommandID: profile.CommandID, ProfileSHA256: profile.ProfileSHA256, Protocol: profile.Protocol, Kind: "usage", UsageState: tc.UsageComplete, Usage: &tc.Usage{InputTokens: &input, OutputTokens: &output}}
	if err = j.RecordToolProgress(a, e); err != nil {
		t.Fatal(err)
	}
	input = 7
	pending := j.PendingProgress()
	if len(pending) != 1 || *pending[0].ToolEvent.Usage.InputTokens != 0 {
		t.Fatal("caller mutated journal source through an input pointer")
	}
	*pending[0].ToolEvent.Usage.InputTokens = 9
	if *j.PendingProgress()[0].ToolEvent.Usage.InputTokens != 0 {
		t.Fatal("caller mutated journal through a returned pointer")
	}
}

func TestNativeUsageOptionalCoverageAndSafeAggregation(t *testing.T) {
	for _, test := range []struct {
		name      string
		second    string
		wantState string
		invalid   bool
	}{
		{"split-partial", `{"input_tokens":0,"output_tokens":0}`, "partial", false},
		{"split-complete", `{"input_tokens":0,"output_tokens":0,"cached_input_tokens":0}`, "complete", false},
		{"overflow", `{"input_tokens":9007199254740991,"output_tokens":0}`, "partial", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "expected")
			lines := []string{`{"type":"started"}`, `{"type":"request_started","step":0,"output_allowance":5}`, `{"type":"usage","step":0,"usage_reported":true,"usage_state":"complete","input_tokens":1,"output_tokens":0,"cached_input_tokens":0}`, `{"type":"request_started","step":1,"output_allowance":5}`}
			var second map[string]any
			_ = json.Unmarshal([]byte(test.second), &second)
			second["type"] = "usage"
			second["step"] = 1
			second["usage_reported"] = true
			second["usage_state"] = "complete"
			raw, _ := json.Marshal(second)
			lines = append(lines, string(raw), `{"type":"completed","output":"fixture"}`)
			for _, line := range lines {
				var obj map[string]any
				_ = json.Unmarshal([]byte(line), &obj)
				obj["request_id"] = "expected"
				raw, _ := json.Marshal(obj)
				_, _ = s.Write(append(raw, '\n'))
			}
			e, err := s.finish()
			if (err != nil) != test.invalid {
				t.Fatalf("aggregate validation %+v %v", e, err)
			}
			if e.UsageState != test.wantState {
				t.Fatalf("wrong coverage %+v", e)
			}
			if e.Usage.CachedInputTokens == nil || *e.Usage.CachedInputTokens != 0 {
				t.Fatal("reported split zero lost")
			}
			if test.invalid && *e.Usage.InputTokens != 1 {
				t.Fatal("overflow mutated an accepted subtotal")
			}
		})
	}
}

func TestActualNativeProcessUsageAndInterruptedSecondRequest(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for explicit local process fixture")
	}
	entry := filepath.Join(t.TempDir(), "usage-fixture.js")
	script := `if(process.argv[2]==='--version'){console.log('suma-agent-runtime fixture-usage-only');process.exit(0)}if(process.argv[2]==='--help'){console.log('JSONL SUMA_AGENT_TOKEN');process.exit(0)}let raw='';process.stdin.on('data',b=>raw+=b);process.stdin.on('end',()=>{const r=JSON.parse(raw),mode=process.env.SUMA_USAGE_FIXTURE;const emit=e=>console.log(JSON.stringify({...e,request_id:r.request_id}));emit({type:'started'});emit({type:'request_started',step:0,output_allowance:5});emit({type:'usage',step:0,usage_state:'complete',usage_reported:true,input_tokens:0,output_tokens:0});if(mode==='interrupted'){emit({type:'request_started',step:1,output_allowance:5});setTimeout(()=>emit({type:'error',message:'fixture must be cancelled'}),20000);return}if(mode==='mixed'){emit({type:'request_started',step:1,output_allowance:5});emit({type:'usage',step:1,usage_state:'unreported',usage_reported:false})}emit({type:'completed',output:'fixture completed'});});`
	if err = os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"zero", "mixed", "interrupted"} {
		t.Run(mode, func(t *testing.T) {
			cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"native": {Executable: node, Adapter: "suma", Protocol: "suma-jsonl-v1", Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true, Env: map[string]string{"SUMA_USAGE_FIXTURE": mode}}}}
			cfg.ToolInventory = describeForTest(t, cfg)
			if !cfg.ToolInventory[0].Available {
				t.Fatalf("fixture unavailable %+v", cfg.ToolInventory)
			}
			a := Assignment{Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "usage fixture only", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256}}
			ctx := context.Background()
			if mode == "interrupted" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Second)
				defer cancel()
			}
			r := Execute(ctx, a, cfg)
			expected := "complete"
			if mode != "zero" {
				expected = "partial"
			}
			assertUsageFacts(t, r.ToolEvidence, expected, map[string]any{"input_tokens": float64(0), "output_tokens": float64(0)})
			if mode == "interrupted" {
				if r.ExitCode == 0 || r.ToolEvidence.Terminal == "completed" {
					t.Fatalf("interrupted process succeeded %+v", r)
				}
			} else if r.ExitCode != 0 {
				t.Fatalf("valid fixture process failed %+v", r)
			}
		})
	}
}
