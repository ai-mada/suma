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
	"testing"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

const cursorFormalVersion189 = "Suma owned ACP package fixture189; not Cursor or Node"
const cursorFormalMarker189 = "suma-owned-cursor-acp-189-fixture\n"
const cursorFormalVersion190 = "Suma owned ACP input fixture190; not Cursor or Node"
const cursorFormalMarker190 = "suma-owned-cursor-inputs-190-fixture\n"

// The reviewed fixture package contains a copy of this native test executable
// named node[.exe], plus a fixed index.js marker. Production package inspection,
// SDK, execution and process supervision remain unchanged. This is not Node or
// the vendor package; it never authenticates or calls a model.
func init() {
	if len(os.Args) < 3 || filepath.Base(os.Args[1]) != "index.js" {
		return
	}
	base := filepath.Base(os.Args[0])
	if base != "node" && base != "node.exe" {
		return
	}
	marker, err := os.ReadFile(os.Args[1])
	if err != nil || string(marker) != cursorFormalMarker189 && string(marker) != cursorFormalMarker190 {
		return
	}
	os.Exit(cursorFormalCLI(string(marker) == cursorFormalMarker190))
}

func cursorFormalCLI(withInputs bool) int {
	if reflect.DeepEqual(os.Args[2:], []string{"--disable-auto-update", "--version"}) {
		trace, err := os.OpenFile(filepath.Join(filepath.Dir(filepath.Dir(os.Args[1])), "version-probes.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return 60
		}
		err = json.NewEncoder(trace).Encode(map[string]any{"pid": os.Getpid(), "native_fixture": true, "real_cursor": false, "version_only": true})
		closeErr := trace.Close()
		if err != nil || closeErr != nil {
			return 60
		}
		version := cursorFormalVersion189
		if withInputs {
			version = cursorFormalVersion190
		}
		fmt.Println(version)
		return 0
	}
	if !reflect.DeepEqual(os.Args[2:], []string{"--disable-auto-update", "--sandbox", "enabled", "acp"}) || os.Getenv("CURSOR_API_KEY") != "owned-non-vendor-key189" || os.Getenv("SUMA_RUNNER_TOKEN") != "" || os.Getenv("SUMA_POOL_TOKEN") != "" {
		return 61
	}
	counts := map[string]int{}
	facts := map[string]any{"pid": os.Getpid(), "native_fixture": true, "real_cursor": false, "real_node": false, "vendor_requests": 0, "model_requests": 0, "explicit_fixture_credential": true, "methods": counts}
	defer func() {
		raw, _ := json.MarshalIndent(facts, "", "  ")
		_ = os.WriteFile(filepath.Join(filepath.Dir(filepath.Dir(os.Args[1])), "native-facts.json"), raw, 0600)
	}()
	emit := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	reply := func(id json.RawMessage, v any) { emit(map[string]any{"jsonrpc": "2.0", "id": id, "result": v}) }
	options := func() []any {
		return []any{map[string]any{"id": "model", "name": "Owned fixed model", "category": "model", "type": "select", "currentValue": "owned-cursor-model189", "options": []any{map[string]any{"value": "owned-cursor-model189", "name": "Owned protocol fixture"}}}}
	}
	requestPermission := func() {
		emit(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "owned-cursor-session189", "update": map[string]any{"sessionUpdate": "tool_call", "toolCallId": "owned-edit189", "title": "Edit exact isolated candidate file", "kind": "edit", "status": "pending"}}})
		emit(map[string]any{"jsonrpc": "2.0", "id": "owned-permission189", "method": "session/request_permission", "params": map[string]any{"sessionId": "owned-cursor-session189", "toolCall": map[string]any{"toolCallId": "owned-edit189", "title": "Commit file.txt only in the isolated worktree", "kind": "edit", "rawInput": map[string]any{"path": "file.txt", "after": "candidate189"}}, "options": []any{map[string]any{"optionId": "owned-once189", "name": "Allow once", "kind": "allow_once"}, map[string]any{"optionId": "owned-reject189", "name": "Reject once", "kind": "reject_once"}}}})
	}
	var prompt json.RawMessage
	permissionResponses := 0
	input := bufio.NewScanner(os.Stdin)
	input.Buffer(make([]byte, 4096), 1<<20)
	for input.Scan() {
		var packet map[string]json.RawMessage
		if json.Unmarshal(input.Bytes(), &packet) != nil {
			return 62
		}
		var method string
		_ = json.Unmarshal(packet["method"], &method)
		counts[method]++
		switch method {
		case "initialize":
			reply(packet["id"], map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "Owned ACP native fixture, not Cursor", "version": "189"}, "authMethods": []any{}, "agentCapabilities": map[string]any{}})
		case "session/new":
			var params struct {
				Cwd string `json:"cwd"`
			}
			cwd, e := os.Getwd()
			if json.Unmarshal(packet["params"], &params) != nil || e != nil || filepath.Clean(params.Cwd) != filepath.Clean(cwd) {
				return 63
			}
			facts["session_cwd_matches_process"] = true
			reply(packet["id"], map[string]any{"sessionId": "owned-cursor-session189", "configOptions": options()})
		case "session/set_config_option":
			var params struct {
				ConfigID string `json:"configId"`
				Value    string `json:"value"`
			}
			if json.Unmarshal(packet["params"], &params) != nil || params.ConfigID != "model" || params.Value != "owned-cursor-model189" {
				return 64
			}
			facts["model_config_ack"] = true
			reply(packet["id"], map[string]any{"configOptions": options()})
		case "session/prompt":
			if len(prompt) != 0 || counts["session/set_config_option"] != 1 {
				return 65
			}
			prompt = append(json.RawMessage{}, packet["id"]...)
			if withInputs {
				emit(map[string]any{"jsonrpc": "2.0", "id": 190, "method": cu.AskQuestionMethod, "params": map[string]any{"toolCallId": "owned-question-call190", "title": "Choose the exact checks", "questions": []any{map[string]any{"id": "checks", "prompt": "Which checks should this candidate preserve?", "options": []any{map[string]any{"id": "first", "label": "First check"}, map[string]any{"id": "second", "label": "Second check"}}, "allowMultiple": true}}}})
			} else {
				requestPermission()
			}
		case "session/cancel":
			if len(prompt) != 0 {
				reply(prompt, map[string]any{"stopReason": "cancelled"})
			}
		case "":
			if withInputs && string(packet["id"]) == "190" {
				var result cu.CursorInputNativeResult
				if packet["error"] != nil || json.Unmarshal(packet["result"], &result) != nil || result.Outcome.Outcome != "answered" || len(result.Outcome.Answers) != 1 || result.Outcome.Answers[0].QuestionID != "checks" || !reflect.DeepEqual(result.Outcome.Answers[0].SelectedOptionIDs, []string{"first", "second"}) {
					return 71
				}
				facts["question_answered"], facts["question_response_id_kind"], facts["selected_option_ids"] = true, "number", result.Outcome.Answers[0].SelectedOptionIDs
				emit(map[string]any{"jsonrpc": "2.0", "id": "owned-plan190", "method": cu.CreatePlanMethod, "params": map[string]any{"toolCallId": "owned-plan-call190", "name": "A proposal requiring explicit review", "overview": "Do not replace the original task or permissions", "plan": "# Proposed plan\nKeep the original task scope and independent quality gate.\n", "todos": []any{map[string]any{"id": "task", "content": "Preserve the original scope", "status": "pending"}}, "isProject": false}})
				continue
			}
			if withInputs && string(packet["id"]) == `"owned-plan190"` {
				var result cu.CursorInputNativeResult
				if packet["error"] != nil || json.Unmarshal(packet["result"], &result) != nil || result.Outcome.Outcome != "rejected" || result.Outcome.Reason != "Keep the original task scope" || facts["question_answered"] != true {
					return 72
				}
				facts["plan_rejected"], facts["plan_response_id_kind"], facts["plan_rpc_error"] = true, "string", false
				requestPermission()
				continue
			}
			var result struct {
				Outcome struct{ Outcome, OptionID string } `json:"outcome"`
			}
			if string(packet["id"]) != `"owned-permission189"` || len(prompt) == 0 || json.Unmarshal(packet["result"], &result) != nil || result.Outcome.Outcome != "selected" || result.Outcome.OptionID != "owned-once189" {
				return 66
			}
			permissionResponses++
			if permissionResponses != 1 || withInputs && facts["plan_rejected"] != true {
				return 73
			}
			if err := os.WriteFile("file.txt", []byte("candidate189\n"), 0600); err != nil {
				return 67
			}
			for _, args := range [][]string{{"add", "--", "file.txt"}, {"-c", "user.name=Owned fixture189", "-c", "user.email=owned189@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture: exact isolated candidate189"}} {
				cmd := exec.Command("git", args...)
				if _, err := cmd.CombinedOutput(); err != nil {
					return 68
				}
			}
			facts["permission_responses"] = permissionResponses
			facts["candidate_committed"] = true
			emit(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "owned-cursor-session189", "update": map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "owned-edit189", "status": "completed"}}})
			reply(prompt, map[string]any{"stopReason": "end_turn"})
		default:
			return 69
		}
	}
	if input.Err() != nil {
		return 70
	}
	return 0
}

func TestCursorACPFormalOriginalWorkersCandidateIndependentJUnitAndReview(t *testing.T) {
	if os.Getenv("SUMA_CURSOR_ACP_FORMAL") != "1" {
		t.Skip("explicit owned Cursor ACP formal fixture gate required")
	}
	cursorFormalRuntimeJourney(t, false)
}

func TestCursorInputsFormalOriginalQuestionsPlansWorkerCandidateAndQuality(t *testing.T) {
	if os.Getenv("SUMA_CURSOR_INPUTS_FORMAL") != "1" {
		t.Skip("explicit owned Cursor input formal fixture gate required")
	}
	cursorFormalRuntimeJourney(t, true)
}

func cursorFormalRuntimeJourney(t *testing.T, withInputs bool) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cacheStage, stage, version, marker := "cursor-acp-189", "T-CURSOR-ACP-189", cursorFormalVersion189, cursorFormalMarker189
	if withInputs {
		cacheStage, stage, version, marker = "cursor-inputs-190", "T-CURSOR-INPUTS-190", cursorFormalVersion190, cursorFormalMarker190
	}
	directory := filepath.Join(root, ".cache", cacheStage, "formal", "run")
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
	t.Setenv("TMP", temporary)
	t.Setenv("TEMP", temporary)
	t.Setenv("TMPDIR", temporary)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	t.Setenv("LOCAL_CURSOR_FIXTURE_KEY", "owned-non-vendor-key189")
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	receipt := map[string]any{"stage": stage, "kind": "owned_native_package_original_workers_git_junit_review", "real_cursor": false, "real_node": false, "vendor_requests": 0, "model_requests": 0, "customer_signoff": false, "status": "started"}
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
	packageRoot := filepath.Join(directory, "owned-package")
	if err = os.Mkdir(packageRoot, 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	native := "node"
	if runtime.GOOS == "windows" {
		native += ".exe"
	}
	input, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.OpenFile(filepath.Join(packageRoot, native), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
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
	if err = os.WriteFile(filepath.Join(packageRoot, "index.js"), []byte(marker), 0600); err != nil {
		t.Fatal(err)
	}
	development, err := runner.BuildCursorACPCommand(packageRoot, version, "owned-cursor-model189", "LOCAL_CURSOR_FIXTURE_KEY")
	if err != nil {
		t.Fatal("strict reviewed owned package unavailable", err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(directory, "repository")
	if err = os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "file.txt", "baseline189\n")
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "file.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: original baseline189")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	workspaceGitTestWrite(t, repo, "file.txt", "user draft retained189\n")
	const verify = `const fs=require('node:fs'),cp=require('node:child_process');const passed=fs.readFileSync('file.txt','utf8')==='candidate189\n'&&cp.execFileSync('git',['status','--porcelain=v1'],{encoding:'utf8'}).trim()==='';fs.writeFileSync(process.argv[1],'<testsuite name="delivery189" tests="1" failures="'+(passed?'0':'1')+'"><testcase classname="delivery" name="candidate189">'+(passed?'':'<failure message="exact clean candidate required"/>')+'</testcase></testsuite>');process.exitCode=passed?0:1;`
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "developer-worktrees"), Commands: map[string]runner.CommandSpec{"cursor-owned189": development}}
	independent := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "independent-worktrees"), Commands: map[string]runner.CommandSpec{"cursor-independent189": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", verify, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	developerProfile, developerToken := issueRepairRuntimeTool(t, f, "cursor-formal-developer189", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "cursor-formal-independent189", &independent)
	receipt["developer_profile"], receipt["independent_profile"] = developerProfile, testProfile
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Owned Cursor ACP exact candidate", "description": "Controlled native fixture, no vendor or model", "acceptance_criteria": []string{"The exact candidate contains candidate189"}}), 201))
	quality := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "candidate189", Title: "Exact clean candidate", Description: "Independent real command and JUnit", Preconditions: "Original producer sealed the candidate", Steps: []string{"Read file.txt", "Verify a clean exact candidate commit"}, ExpectedResult: "candidate189 at the scheduler-pinned commit", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "delivery", Name: "candidate189"}})
	w, _, _ := taskDispatchBuildFixture()
	w.ID, w.Version = "", 0
	w.Nodes[0].Command, w.Nodes[0].TimeoutSeconds = []string{"cursor-owned189"}, 60
	w.Nodes[0].Config = map[string]any{"goal": "Edit file.txt to candidate189 and commit this isolated candidate after exact once approval."}
	w.Nodes[1].Command = []string{"cursor-independent189"}
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
	sharedRuntime := originalDevelopmentRuntime{f: f, control: control, directory: directory, receipt: receipt}
	startWorker := func(id, token, journalName string, cfg runner.ExecConfig) func() {
		return sharedRuntime.startWorker(t, ctx, id, token, journalName, cfg)
	}
	stopDeveloper := startWorker(developerProfile.WorkerID, developerToken, "developer-journal", developer)
	defer func() {
		if stopDeveloper != nil {
			stopDeveloper()
		}
	}()
	var produced dag.NodeRun
	permissionCount := 0
	questionAnswers, planDecisions := 0, 0
	answeredInputs := map[string]bool{}
	for {
		permissions, e := f.s.ListToolPermissions(ctx, f.actor, f.project, run.ID, "develop")
		if e != nil {
			t.Fatal(e)
		}
		for _, permission := range permissions.Items {
			if permission.State == "pending" {
				if permission.Request.ACP == nil || permission.Request.ACP.SessionID != "owned-cursor-session189" {
					t.Fatal("wrong permission source")
				}
				_, e = f.s.DecideToolPermission(ctx, f.actor, permission.ID, pc.DecisionRequest{ProjectID: f.project, ExpectedVersion: permission.Version, IdempotencyKey: newID(), Choice: pc.Choice{Protocol: ac.Protocol, ACP: &ac.Choice{OptionID: "owned-once189", Kind: "allow_once"}}})
				if e != nil {
					t.Fatal(e)
				}
				permissionCount++
			}
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
		if withInputs && produced.Attempt > 0 && produced.Fence > 0 {
			path := fmt.Sprintf("/runs/%s/nodes/develop/tool-inputs?project_id=%s&attempt=%d&fence=%d", run.ID, f.project, produced.Attempt, produced.Fence)
			inputs := issueRepairRuntimeDecode[ic.ToolInputList](t, f.call("GET", path, nil, 200))
			if inputs.Protocol != cu.InputProtocol {
				t.Fatal("original Cursor execution did not publish its input capability")
			}
			for _, input := range inputs.Items {
				if input.State != "pending" || answeredInputs[input.ID] {
					continue
				}
				if input.Request.Cursor == nil || input.Request.Cursor.SessionID != "owned-cursor-session189" || len(input.AllowedActions) != 1 {
					t.Fatal("input did not retain the actual original Cursor source")
				}
				var decision cu.CursorInputDecision
				switch input.Request.Cursor.Method {
				case cu.AskQuestionMethod:
					decision = cu.CursorInputDecision{Outcome: "answered", Answers: []cu.CursorInputSelection{{QuestionID: "checks", OptionIDs: []string{"second", "first"}}}}
					questionAnswers++
				case cu.CreatePlanMethod:
					decision = cu.CursorInputDecision{Outcome: "rejected", Reason: "Keep the original task scope"}
					planDecisions++
				default:
					t.Fatal("unexpected native input method")
				}
				answer := ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: input.Version, IdempotencyKey: newID(), Cursor: &decision}
				f.call("POST", "/tool-inputs/"+input.ID+"/answer", candidateJSON(t, answer), 200)
				answeredInputs[input.ID] = true
			}
		}
		if produced.State == "succeeded" {
			break
		}
		if produced.State == "failed" || produced.State == "reconciling" || ctx.Err() != nil {
			receipt["producer"] = produced
			t.Fatal("original Cursor producer did not succeed", produced.State, produced.Error)
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
	if permissionCount != 1 || candidate == "" || candidate == base || produced.WorkerID != developerProfile.WorkerID || produced.WorkspaceEvidence["clean"] != true || produced.ToolEvidence == nil || produced.ToolEvidence.ACPExecution == nil || !produced.ToolEvidence.ACPExecution.HostProcessStopped || produced.ToolEvidence.ACPExecution.SelectedModel != "owned-cursor-model189" {
		t.Fatal("exact original Cursor candidate facts missing")
	}
	if withInputs {
		proofs := produced.ToolEvidence.ACPExecution.UserInputs
		if questionAnswers != 1 || planDecisions != 1 || len(proofs) != 2 {
			t.Fatal("the original human interactions were omitted or repeated", questionAnswers, planDecisions, len(proofs))
		}
		for _, proof := range proofs {
			if proof.Observation == nil || !proof.Observation.AnswerObserved || !proof.Observation.ReplyStarted || !proof.Observation.ReplyWritten || !proof.Observation.ExecutionClosed || proof.Observation.RequestResolved || proof.Observation.FailureCode != "" {
				t.Fatal("native replies and durable closure did not precede Result", proof)
			}
		}
		receipt["question_answer_api_calls"], receipt["plan_answer_api_calls"], receipt["input_result_proofs"] = questionAnswers, planDecisions, len(proofs)
	}
	verified, execution, accepted := sharedRuntime.finishQuality(t, ctx, originalDevelopmentQuality{Repository: repo, BaseCommit: base, CandidateCommit: candidate, ExpectedDraft: "user draft retained189\n", Task: task, Plan: quality, Run: run, Producer: produced, TestProfile: testProfile, TestToken: testToken, Independent: independent, Reviewer: reviewer})
	receipt["status"], receipt["base_commit"], receipt["candidate_commit"] = "controlled_vertical_confirmed", base, candidate
	receipt["run_id"], receipt["task_id"], receipt["task_status"], receipt["reviewer_id"] = run.ID, task.ID, accepted.Status, reviewer.Projection().ID
	receipt["producer"], receipt["independent"], receipt["quality"] = produced, verified, execution
	receipt["administrator_checkout_retained"], receipt["permission_count"] = true, permissionCount
}
