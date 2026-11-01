package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

const claudeFormalNativeMarker192 = "Suma owned native Agent SDK wire fixture192; not Claude, no vendor or model\n"
const claudeFormalSession192 = "owned-sdk-session192"
const claudeFormalQuestion192 = "选择需要保留的检查？"

// Re-entry requires both an explicitly named copied native executable and an
// adjacent owned marker. The ordinary test binary cannot accidentally become
// this fixture. The reviewed SDK still performs the actual stdio transport.
func init() {
	base := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if base != "suma-owned-claude-native192" {
		return
	}
	marker, err := os.ReadFile(filepath.Join(filepath.Dir(os.Args[0]), "owned-native-marker192"))
	if err != nil || string(marker) != claudeFormalNativeMarker192 {
		return
	}
	os.Exit(claudeFormalNativeWire192())
}

func claudeFormalNativeWire192() int {
	directory := filepath.Dir(os.Args[0])
	if reflect.DeepEqual(os.Args[1:], []string{"--version"}) {
		// A controlled protocol-version declaration, never a discovered vendor
		// binary identity. Every fixture artifact is separately labeled as owned.
		fmt.Println(cl.CLIVersion)
		f, err := os.OpenFile(filepath.Join(directory, "version-probes.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return 60
		}
		err = json.NewEncoder(f).Encode(map[string]any{"pid": os.Getpid(), "owned_native_fixture": true, "real_claude": false, "declared_wire_version": cl.CLIVersion})
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return 60
		}
		return 0
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "owned-non-vendor-key192" || os.Getenv("SUMA_RUNNER_TOKEN") != "" || os.Getenv("SUMA_POOL_TOKEN") != "" {
		return 61
	}
	facts := map[string]any{"pid": os.Getpid(), "owned_native_fixture": true, "real_claude": false, "real_model": false, "vendor_calls": 0, "model_calls": 0, "native_init_requests": 0, "native_user_turns": 0, "native_permission_responses": 0, "native_question_responses": 0}
	defer func() {
		raw, _ := json.MarshalIndent(facts, "", "  ")
		_ = os.WriteFile(filepath.Join(directory, "native-facts.json"), raw, 0600)
	}()
	emit := func(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
	assistant := func(id, name string, input any) {
		emit(map[string]any{"type": "assistant", "session_id": claudeFormalSession192, "parent_tool_use_id": nil, "message": map[string]any{"id": "message-" + id, "role": "assistant", "model": "claude-sonnet-5", "usage": map[string]any{"input_tokens": 1, "output_tokens": 0}, "content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}}})
	}
	request := func(id, name, toolID string, input any) {
		emit(map[string]any{"type": "control_request", "request_id": id, "request": map[string]any{"subtype": "can_use_tool", "tool_name": name, "tool_use_id": toolID, "input": input}})
	}
	toolResult := func(id string) {
		emit(map[string]any{"type": "user", "session_id": claudeFormalSession192, "parent_tool_use_id": nil, "message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": "Owned original fixture operation completed", "is_error": false}}}})
	}
	question := map[string]any{"questions": []any{map[string]any{"question": claudeFormalQuestion192, "header": "独立质量", "multiSelect": true, "options": []any{map[string]any{"label": "固定候选", "description": "测试准确提交"}, map[string]any{"label": "独立验证", "description": "由另一执行节点运行"}}}}, "answers": map[string]any{}}
	command := "printf 'candidate192\\n' > file.txt && git add -- file.txt && git -c user.name='Owned fixture192' -c user.email='owned192@example.invalid' -c commit.gpgsign=false commit -m 'fixture: approved candidate192'"
	permission := map[string]any{"command": command, "description": "Only file.txt and its isolated Git commit; owned fixture executes the equivalent fixed file/Git operations"}
	state, initialized := 0, false
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var packet map[string]any
		if json.Unmarshal(scanner.Bytes(), &packet) != nil {
			return 62
		}
		switch packet["type"] {
		case "control_request":
			r, _ := packet["request"].(map[string]any)
			if r["subtype"] != "initialize" || initialized {
				return 63
			}
			initialized = true
			facts["native_init_requests"] = 1
			emit(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": packet["request_id"], "response": map[string]any{"commands": []any{}, "models": []any{}, "account": map[string]any{}, "output_style": "default", "available_output_styles": []any{}}}})
		case "user":
			if !initialized || state != 0 {
				return 64
			}
			state = 1
			facts["native_user_turns"] = 1
			cwd, err := os.Getwd()
			if err != nil {
				return 65
			}
			facts["cwd"] = cwd
			emit(map[string]any{"type": "system", "subtype": "init", "session_id": claudeFormalSession192, "model": "claude-sonnet-5", "claude_code_version": cl.CLIVersion, "permissionMode": "default", "apiKeySource": "ANTHROPIC_API_KEY", "cwd": cwd, "tools": []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash", "AskUserQuestion"}, "mcp_servers": []any{}, "slash_commands": []any{}, "uuid": "owned-init192"})
			assistant("owned-question192", "AskUserQuestion", question)
			request("native-question192", "AskUserQuestion", "owned-question192", question)
		case "control_response":
			response, _ := packet["response"].(map[string]any)
			decision, _ := response["response"].(map[string]any)
			if response["subtype"] != "success" || decision["behavior"] != "allow" || decision["updatedPermissions"] != nil {
				return 66
			}
			if state == 1 {
				updated := map[string]any{"questions": question["questions"], "answers": map[string]any{claudeFormalQuestion192: "固定候选, 独立验证"}}
				if response["request_id"] != "native-question192" || !reflect.DeepEqual(decision["updatedInput"], updated) {
					return 67
				}
				state = 2
				facts["native_question_responses"] = 1
				toolResult("owned-question192")
				assistant("owned-bash192", "Bash", permission)
				request("native-permission192", "Bash", "owned-bash192", permission)
				continue
			}
			if state != 2 || response["request_id"] != "native-permission192" || !reflect.DeepEqual(decision["updatedInput"], permission) {
				return 68
			}
			state = 3
			facts["native_permission_responses"] = 1
			if err := os.WriteFile("file.txt", []byte("candidate192\n"), 0600); err != nil {
				return 69
			}
			for _, args := range [][]string{{"add", "--", "file.txt"}, {"-c", "user.name=Owned fixture192", "-c", "user.email=owned192@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture: approved candidate192"}} {
				if _, err := exec.Command("git", args...).CombinedOutput(); err != nil {
					return 70
				}
			}
			facts["candidate_committed"] = true
			// The SDK may close/terminate its native child immediately after the
			// terminal result. Preserve completed local effects before that race.
			rawFacts, _ := json.MarshalIndent(facts, "", "  ")
			if err := os.WriteFile(filepath.Join(directory, "native-facts.json"), rawFacts, 0600); err != nil {
				return 73
			}
			toolResult("owned-bash192")
			emit(map[string]any{"type": "result", "subtype": "success", "is_error": false, "session_id": claudeFormalSession192, "result": "Owned SDK fixture candidate complete", "permission_denials": []any{}, "num_turns": 1, "duration_ms": 1, "duration_api_ms": 0, "total_cost_usd": 0, "usage": map[string]any{"input_tokens": 2, "output_tokens": 3, "cache_read_input_tokens": 0, "cache_creation_input_tokens": 0}, "modelUsage": map[string]any{"claude-sonnet-5": map[string]any{"inputTokens": 2, "outputTokens": 3, "cacheReadInputTokens": 0, "cacheCreationInputTokens": 0}}, "uuid": "owned-result192"})
			return 0
		default:
			return 71
		}
	}
	if scanner.Err() != nil || state != 3 {
		return 72
	}
	return 0
}

// Root runs this opt-in journey once against the already built pinned SDK
// bundle. It uses an owned native transport fixture, not a vendor account/model.
func TestClaudeSDKFormalOriginalWorkersNativeCallbacksCandidateAndQuality(t *testing.T) {
	if os.Getenv("SUMA_CLAUDE_SDK_FORMAL") != "1" {
		t.Skip("explicit owned Agent SDK formal fixture gate required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	entry := os.Getenv("SUMA_CLAUDE_SDK_FORMAL_BUNDLE")
	if !filepath.IsAbs(entry) {
		t.Fatal("exact already-built SDK bundle absolute path required")
	}
	baseDirectory := filepath.Join(root, ".cache", "claude-runtime-192", "formal")
	directory := os.Getenv("SUMA_CLAUDE_SDK_FORMAL_RUN_DIR")
	if directory == "" {
		directory = filepath.Join(baseDirectory, "run")
	}
	within, err := filepath.Rel(baseDirectory, directory)
	if err != nil || !filepath.IsAbs(directory) || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		t.Fatal("formal run directory must remain in this stage cache")
	}
	if err = os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal("original formal run must not be overwritten", err)
	}
	temporary := filepath.Join(directory, "tmp")
	if err = os.Mkdir(temporary, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(name, temporary)
	}
	t.Setenv("LOCAL_CLAUDE_SDK_FIXTURE_KEY192", "owned-non-vendor-key192")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	receipt := map[string]any{"stage": "T-CLAUDE-SDK-192", "kind": "actual_sdk_owned_native_transport_original_workers_quality", "real_claude": false, "vendor_calls": 0, "model_calls": 0, "customer_signoff": false, "sdk_bundle": entry, "process_temp": temporary, "status": "started"}
	defer func() {
		if t.Failed() {
			receipt["status"] = "failed"
		}
		snapshotCtx, snapshotCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer snapshotCancel()
		if _, e := f.s.DB.ExecContext(snapshotCtx, "VACUUM INTO ?", filepath.Join(directory, "private.sqlite")); e != nil {
			receipt["snapshot_error"] = e.Error()
			t.Error(e)
		}
		raw, _ := json.MarshalIndent(receipt, "", "  ")
		if e := os.WriteFile(filepath.Join(directory, "receipt.json"), raw, 0600); e != nil {
			t.Error(e)
		}
	}()
	nativeDirectory := filepath.Join(directory, "owned-native")
	if err = os.Mkdir(nativeDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	leaf := "suma-owned-claude-native192"
	if runtime.GOOS == "windows" {
		leaf += ".exe"
	}
	native := filepath.Join(nativeDirectory, leaf)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(native, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		input.Close()
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	input.Close()
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	if err = os.WriteFile(filepath.Join(nativeDirectory, "owned-native-marker192"), []byte(claudeFormalNativeMarker192), 0600); err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	development, err := runner.BuildClaudeSDKCommand(node, entry, runner.ClaudeSDKSpec{NativeCLIExecutable: native, Model: "claude-sonnet-5", CredentialEnvRef: "LOCAL_CLAUDE_SDK_FIXTURE_KEY192", ReviewedCLIVersion: cl.CLIVersion})
	if err != nil {
		t.Fatal("pinned actual SDK and owned native package unavailable", err)
	}
	receipt["native_fixture_path"] = native
	repo := filepath.Join(directory, "repository")
	if err = os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "file.txt", "baseline192\n")
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "file.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: original baseline192")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	workspaceGitTestWrite(t, repo, "file.txt", "user draft retained192\n")
	const verify = `const fs=require('node:fs'),cp=require('node:child_process');const passed=fs.readFileSync('file.txt','utf8')==='candidate192\n'&&cp.execFileSync('git',['status','--porcelain=v1'],{encoding:'utf8'}).trim()==='';fs.writeFileSync(process.argv[1],'<testsuite name="delivery192" tests="1" failures="'+(passed?'0':'1')+'"><testcase classname="delivery" name="candidate192">'+(passed?'':'<failure message="exact clean candidate required"/>')+'</testcase></testsuite>');process.exitCode=passed?0:1;`
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "developer-worktrees"), Commands: map[string]runner.CommandSpec{"claude-sdk-owned192": development}}
	independent := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "independent-worktrees"), Commands: map[string]runner.CommandSpec{"claude-sdk-independent192": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", verify, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	developerProfile, developerToken := issueRepairRuntimeTool(t, f, "claude-sdk-developer192", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "claude-sdk-independent192", &independent)
	receipt["developer_profile"], receipt["independent_profile"] = developerProfile, testProfile
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Owned Agent SDK exact candidate", "description": "Actual pinned SDK, owned native transport, no vendor model", "acceptance_criteria": []string{"The exact candidate contains candidate192"}}), 201))
	quality := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "candidate192", Title: "Exact clean SDK candidate", Description: "Independent real command and JUnit", Preconditions: "Original producer sealed the candidate", Steps: []string{"Read file.txt", "Verify a clean exact candidate commit"}, ExpectedResult: "candidate192 at the pinned commit", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "delivery", Name: "candidate192"}})
	w, _, _ := taskDispatchBuildFixture()
	w.ID, w.Version = "", 0
	w.Nodes[0].Command, w.Nodes[0].TimeoutSeconds = []string{"claude-sdk-owned192"}, 90
	w.Nodes[0].Config = map[string]any{"goal": "Ask which independent checks to retain, then obtain one exact original permission to write candidate192 and commit in this isolated workspace."}
	w.Nodes[1].Command = []string{"claude-sdk-independent192"}
	w.Nodes[1].Config["tool_profile_id"], w.Nodes[1].Config["tool_profile_sha256"] = testProfile.ID, testProfile.Profile.ProfileSHA256
	w, err = f.e.Publish(ctx, f.user.TenantID, f.project, w)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: w.ID, WorkflowVersion: w.Version, AgentNodeID: "develop", ToolProfileID: developerProfile.ID, ExpectedToolVersion: developerProfile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	reviewer := invitedReviewer(t, f)
	control := httptest.NewServer(f.handler)
	defer control.Close()
	shared := originalDevelopmentRuntime{f: f, control: control, directory: directory, receipt: receipt}
	stopDeveloper := shared.startWorker(t, ctx, developerProfile.WorkerID, developerToken, "developer-journal", developer)
	defer func() {
		if stopDeveloper != nil {
			stopDeveloper()
		}
	}()
	var produced dag.NodeRun
	permissionCount, inputCount := 0, 0
	answered := map[string]bool{}
	for {
		permissions, e := f.s.ListToolPermissions(ctx, f.actor, f.project, run.ID, "develop")
		if e != nil {
			t.Fatal(e)
		}
		for _, p := range permissions.Items {
			if p.State != "pending" {
				continue
			}
			if p.Request.Claude == nil || p.Request.Claude.SessionID != claudeFormalSession192 || p.Request.Claude.RequestID != "native-permission192" {
				t.Fatal("incorrect original SDK permission source")
			}
			f.call("POST", "/tool-permissions/"+p.ID+"/decision", pc.DecisionRequest{Choice: pc.Choice{Protocol: cl.Protocol, Claude: &cl.PermissionChoice{Outcome: "allow_once"}}, ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}, 200)
			permissionCount++
		}
		current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
		if e != nil {
			t.Fatal(e)
		}
		for _, n := range current.Nodes {
			if n.ID == "develop" {
				produced = n
			}
		}
		if produced.Attempt > 0 && produced.Fence > 0 {
			path := fmt.Sprintf("/runs/%s/nodes/develop/tool-inputs?project_id=%s&attempt=%d&fence=%d", run.ID, f.project, produced.Attempt, produced.Fence)
			inputs := issueRepairRuntimeDecode[ic.ToolInputList](t, f.call("GET", path, nil, 200))
			if inputs.Protocol != cl.InputProtocol {
				t.Fatal("SDK input capability lost")
			}
			for _, v := range inputs.Items {
				if v.State != "pending" || answered[v.ID] {
					continue
				}
				if v.Request.Claude == nil || v.Request.Claude.SessionID != claudeFormalSession192 || v.Request.Claude.RequestID != "native-question192" || len(v.AllowedActions) != 1 {
					t.Fatal("SDK question source changed")
				}
				f.call("POST", "/tool-inputs/"+v.ID+"/answer", ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: v.Version, IdempotencyKey: newID(), Claude: &cl.InputDecision{Outcome: "answered", Answers: []cl.InputSelection{{QuestionID: "q1", OptionIDs: []string{"o2", "o1"}}}}}, 200)
				answered[v.ID] = true
				inputCount++
			}
		}
		if produced.State == "succeeded" {
			break
		}
		if produced.State == "failed" || produced.State == "reconciling" || ctx.Err() != nil {
			receipt["producer"] = produced
			t.Fatal("original SDK producer did not succeed", produced.State, produced.Error)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(75 * time.Millisecond):
		}
	}
	stopDeveloper()
	stopDeveloper = nil
	candidate, _ := produced.WorkspaceEvidence["head_commit"].(string)
	if permissionCount != 1 || inputCount != 1 || candidate == "" || candidate == base || produced.WorkerID != developerProfile.WorkerID || produced.WorkspaceEvidence["clean"] != true || produced.ToolEvidence == nil || produced.ToolEvidence.ClaudeSDKExecution == nil {
		t.Fatal("exact SDK candidate or original callback facts missing")
	}
	proof := produced.ToolEvidence.ClaudeSDKExecution
	if len(proof.Permissions) != 1 || len(proof.UserInputs) != 1 || proof.Permissions[0].Observation == nil || proof.UserInputs[0].Observation == nil || proof.Permissions[0].Observation.RequestResolved || proof.UserInputs[0].Observation.RequestResolved || !proof.Permissions[0].Observation.ReplyWritten || !proof.UserInputs[0].Observation.ExecutionClosed || !proof.HostProcessStopped || !proof.HostProcessTreeStopped {
		t.Fatal("SDK callback delivery was replaced with a fabricated ACK or incomplete stop")
	}
	verified, execution, accepted := shared.finishQuality(t, ctx, originalDevelopmentQuality{Repository: repo, BaseCommit: base, CandidateCommit: candidate, ExpectedDraft: "user draft retained192\n", Task: task, Plan: quality, Run: run, Producer: produced, TestProfile: testProfile, TestToken: testToken, Independent: independent, Reviewer: reviewer})
	var facts map[string]any
	data, err := os.ReadFile(filepath.Join(nativeDirectory, "native-facts.json"))
	if err != nil || json.Unmarshal(data, &facts) != nil || facts["candidate_committed"] != true || facts["native_permission_responses"] != float64(1) || facts["native_question_responses"] != float64(1) {
		t.Fatal("owned native transport facts incomplete", err)
	}
	receipt["status"], receipt["base_commit"], receipt["candidate_commit"] = "controlled_vertical_confirmed", base, candidate
	receipt["run_id"], receipt["task_id"], receipt["task_status"], receipt["reviewer_id"] = run.ID, task.ID, accepted.Status, reviewer.Projection().ID
	receipt["producer"], receipt["independent"], receipt["quality"], receipt["native"] = produced, verified, execution, facts
	receipt["administrator_checkout_retained"], receipt["permission_count"], receipt["input_count"] = true, permissionCount, inputCount
}
