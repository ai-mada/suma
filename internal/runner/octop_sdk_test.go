package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Opt-in actual released SDK acceptance. The endpoint below is a deterministic
// OpenAI-protocol fixture, never a vendor model. No Python installation or
// dependency download happens inside a test. Ordinary CI runs the contract
// tests; a prepared SDK environment runs this single vertical journey.
func TestOctopSDKActualHostCandidateAndIndependentJUnit(t *testing.T) {
	python, bundle := os.Getenv("SUMA_TEST_OCTOP_PYTHON"), os.Getenv("SUMA_TEST_OCTOP_BUNDLE")
	if python == "" || bundle == "" {
		t.Skip("set explicit prepared Octop Python and zipapp paths for actual SDK acceptance")
	}
	var calls atomic.Int32
	var invalid atomic.Bool
	const providerKey = "synthetic-octop-provider-only-key"
	t.Setenv("OCTOP_TEST_PROVIDER_KEY", providerKey)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model               string `json:"model"`
			Stream              bool   `json:"stream"`
			MaxTokens           int    `json:"max_tokens"`
			MaxCompletionTokens int    `json:"max_completion_tokens"`
			Tools               []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		n := calls.Add(1)
		allowance := 512 - 3*int(n-1)
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+providerKey || json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request) != nil || request.Model != "coding117" || request.MaxTokens != allowance && request.MaxCompletionTokens != allowance || len(request.Tools) != 6 || !request.Stream {
			invalid.Store(true)
			http.Error(w, "fixed provider request mismatch", http.StatusBadRequest)
			return
		}
		for _, tool := range request.Tools {
			if !strings.HasPrefix(tool.Function.Name, "suma_workspace_") {
				invalid.Store(true)
			}
		}
		name, arguments := "", ""
		switch n {
		case 1:
			name, arguments = "suma_workspace_read", `{"path":"file.txt"}`
		case 2:
			name, arguments = "suma_workspace_write", `{"path":"output117.txt","content":"actual Harness output\n","expected_absent":true}`
		case 3:
			name, arguments = "suma_workspace_check", `{"check_id":"check117"}`
		case 4:
		default:
			invalid.Store(true)
			http.Error(w, "unexpected model retry or extra turn", 400)
			return
		}
		delta := map[string]any{"role": "assistant", "content": "Completed through the host tools."}
		finish := "stop"
		if name != "" {
			delta = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("call117_%d", n), "type": "function", "function": map[string]string{"name": name, "arguments": arguments}}}}
			finish = "tool_calls"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, choice := range []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}, map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}} {
			raw, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("completion117_%d", n), "object": "chat.completion.chunk", "created": 1, "model": request.Model, "choices": []any{choice}})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		// Usage is its own terminal suffix, not mixed into a content delta. The
		// host must persist it before releasing this turn to the SDK.
		usage, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("completion117_%d", n), "object": "chat.completion.chunk", "created": 1, "model": request.Model, "choices": []any{}, "usage": map[string]int{"prompt_tokens": 7, "completion_tokens": 3, "total_tokens": 10}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", usage)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer endpoint.Close()
	repo, base := gitFixture(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	binding := octopModelBinding(t, endpoint.URL+"/v1")
	spec := CommandSpec{Adapter: "octop", Protocol: tc.OctopProtocol, ModelExecutionProtocol: mc.Protocol, Executable: python, Args: []string{bundle}, Input: "octop-acp-jsonl", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{"check117"}, RuntimeManifest: filepath.Join(filepath.Dir(bundle), "suma-octop.manifest.json")}}
	check := CommandSpec{Executable: node, Args: []string{"-e", `if(require('node:fs').readFileSync('output117.txt','utf8')!=='actual Harness output\n')process.exit(2)`}}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, ModelDeployments: []ModelDeploymentSpec{{Deployment: binding.Deployment, BaseURL: endpoint.URL + "/v1", APIKeyEnvRef: "OCTOP_TEST_PROVIDER_KEY"}}, Commands: map[string]CommandSpec{"octop117": spec, "check117": check}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	items, err := DescribeTools(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var profile tc.Profile
	for _, item := range items {
		if item.Profile.CommandID == "octop117" {
			if !item.Available {
				t.Fatalf("actual SDK probe unavailable: %s", item.Reason)
			}
			profile = item.Profile
		}
	}
	if !tc.SupportsOctopExecutionProfile(profile) {
		t.Fatal("actual probe did not establish the fixed host-tool profile")
	}
	cfg.ToolInventory = items
	cfg.ToolProgress = func(event tc.Event) error {
		if event.Kind == "error" {
			t.Logf("bounded runtime error: status=%s message=%s", event.Status, event.Message)
		}
		return nil
	}
	a := Assignment{RunID: "octop117", NodeID: "develop", WorkerID: "developer117", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"octop117"}, Config: map[string]any{"goal": "Read file.txt, create output117.txt, run check117 and finish.", "tool_profile_sha256": profile.ProfileSHA256, "model_execution": binding, "max_steps": binding.Parameters.MaxSteps, "max_output_tokens": binding.Parameters.MaxOutputTokens}}
	r := Execute(ctx, a, cfg)
	if r.ExitCode != 0 || r.Error != "" || r.OutcomeUnknown || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed" || tc.ValidateEvidence(*r.ToolEvidence) != nil {
		// Only this credential-free synthetic attempt is inspected. Preserve the
		// bounded failure trace before t.TempDir cleanup; never dump user logs.
		log := filepath.Join(repo, ".cache", "runner", "worktrees", attemptKey(a), "octop-home", "logs", "runtime.log")
		if data, readErr := os.ReadFile(log); readErr == nil {
			if len(data) > 12000 {
				data = data[len(data)-12000:]
			}
			t.Logf("synthetic SDK private failure trace: %s", data)
		}
		t.Logf("local model requests=%d, request mismatch=%v", calls.Load(), invalid.Load())
		t.Fatalf("actual SDK did not deliver a proven candidate: %+v", r)
	}
	evidence, ok := r.WorkspaceEvidence["native_workspace"].(*NativeWorkspaceEvidence)
	if !ok || evidence.Generation != 1 || len(evidence.Files) != 1 || len(evidence.Checks) != 1 || evidence.Seal == nil || evidence.Seal.State != "sealed" || calls.Load() != 4 || invalid.Load() {
		t.Fatalf("SDK bypassed the fixed model/tool/ledger/check boundary: calls=%d evidence=%+v", calls.Load(), evidence)
	}
	model := r.ToolEvidence.ModelExecution
	if model == nil || !mc.EvidenceResponsesComplete(binding, *model) || len(model.Requests) != 4 || r.ToolEvidence.UsageState != tc.UsageComplete || r.ToolEvidence.Usage == nil || r.ToolEvidence.Usage.InputTokens == nil || *r.ToolEvidence.Usage.InputTokens != 28 || r.ToolEvidence.Usage.OutputTokens == nil || *r.ToolEvidence.Usage.OutputTokens != 12 {
		t.Fatalf("provider HTTP receipts did not establish complete host usage: %+v", r.ToolEvidence)
	}
	for i, request := range model.Requests {
		if request.OutputAllowance != int64(512-3*i) {
			t.Fatalf("request %d did not clamp cumulative output: %+v", i, request)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "output117.txt")); !os.IsNotExist(err) || strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD")) != base {
		t.Fatal("SDK modified administrator checkout")
	}
	// A different Runner identity imports the exact sealed SHA and writes an
	// ordinary JUnit artifact. The model's check is not formal test evidence.
	cfg.Commands = map[string]CommandSpec{"verify117": {Executable: node, Args: []string{"-e", `const fs=require('node:fs');if(fs.readFileSync('output117.txt','utf8')!=='actual Harness output\n')process.exit(2);fs.writeFileSync(process.argv[1],'<testsuite name="independent117" tests="1"><testcase classname="candidate117" name="output"/></testsuite>')`, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}
	cfg.ToolInventory = nil
	test := Assignment{RunID: a.RunID, NodeID: "test", WorkerID: "independent117", Attempt: 1, Fence: 1, Type: "test", Command: []string{"verify117"}, CandidateFrom: a.NodeID, CandidateCommit: r.WorkspaceEvidence["head_commit"].(string), CandidateAttempt: a.Attempt, CandidateFence: a.Fence, CandidateEvidenceSHA256: r.WorkspaceEvidence["evidence_sha256"].(string), Config: map[string]any{"candidate_from": a.NodeID}}
	checked := Execute(ctx, test, cfg)
	if checked.ExitCode != 0 || checked.Error != "" || checked.WorkspaceEvidence["head_commit"] != test.CandidateCommit || checked.WorkspaceEvidence["test_command"] != true || checked.TestReport == nil {
		t.Fatalf("independent same-SHA JUnit failed: %+v", checked)
	}
	t.Logf("actual Harness1.0.5 / ACP0.12.1; model fixture calls=%d; Go ledger/check/seal; independent same-SHA JUnit", calls.Load())
}

// Cancel only after a real SDK model request starts. The ACP notification is
// not an abort ACK; the separate Go tree receipt must remain the stop authority.
func TestOctopSDKActualCancellationUsesGoTree(t *testing.T) {
	python, bundle := os.Getenv("SUMA_TEST_OCTOP_PYTHON"), os.Getenv("SUMA_TEST_OCTOP_BUNDLE")
	if python == "" || bundle == "" {
		t.Skip("prepared Octop SDK environment required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var calls atomic.Int32
	const providerKey = "synthetic-octop-cancel-provider-key"
	t.Setenv("OCTOP_TEST_PROVIDER_KEY", providerKey)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+providerKey {
			http.Error(w, "supplier key did not remain host-owned", http.StatusUnauthorized)
			return
		}
		calls.Add(1)
		cancel()
		select {
		case <-r.Context().Done():
		case <-time.After(15 * time.Second):
		}
	}))
	defer endpoint.Close()
	repo, base := gitFixture(t)
	binding := octopModelBinding(t, endpoint.URL+"/v1")
	spec := CommandSpec{Adapter: "octop", Protocol: tc.OctopProtocol, ModelExecutionProtocol: mc.Protocol, Executable: python, Args: []string{bundle}, Input: "octop-acp-jsonl", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}, RuntimeManifest: filepath.Join(filepath.Dir(bundle), "suma-octop.manifest.json")}}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, ModelDeployments: []ModelDeploymentSpec{{Deployment: binding.Deployment, BaseURL: endpoint.URL + "/v1", APIKeyEnvRef: "OCTOP_TEST_PROVIDER_KEY"}}, Commands: map[string]CommandSpec{"octop117": spec}}
	items, err := DescribeTools(ctx, cfg)
	if err != nil || len(items) != 1 || !items[0].Available {
		t.Fatalf("actual SDK probe failed: %+v %v", items, err)
	}
	cfg.ToolInventory = items
	p := items[0].Profile
	a := Assignment{RunID: "cancel117", NodeID: "develop", WorkerID: "developer117", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"octop117"}, Config: map[string]any{"goal": "Wait for the assigned model.", "tool_profile_sha256": p.ProfileSHA256, "model_execution": binding, "max_steps": binding.Parameters.MaxSteps, "max_output_tokens": binding.Parameters.MaxOutputTokens}}
	r := Execute(ctx, a, cfg)
	if calls.Load() != 1 || !r.Stopped || r.OutcomeUnknown || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "unknown" || r.ToolEvidence.OctopExecution == nil || !r.ToolEvidence.OctopExecution.CancelRequested || !r.ToolEvidence.OctopExecution.ProcessStopped || tc.ValidateOctopResultStop(*r.ToolEvidence, true, false) != nil {
		t.Fatalf("ACP cancellation became completion or lost its Go stop fact: calls=%d %+v", calls.Load(), r)
	}
	model := r.ToolEvidence.ModelExecution
	if model == nil || !model.GatewayClosed || len(model.Requests) != 1 || model.Requests[0].State != "unknown" || r.ToolEvidence.UsageState == tc.UsageComplete || mc.EvidenceResponsesComplete(binding, *model) {
		t.Fatalf("cancelled provider request lost its unknown usage hold: %+v", r.ToolEvidence)
	}
}
