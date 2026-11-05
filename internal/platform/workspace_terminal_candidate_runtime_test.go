package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

type terminalCandidateNetwork struct {
	mu       sync.Mutex
	Dropped  bool              `json:"stopped_source_ack_lost_after_commit"`
	Replays  int               `json:"original_operation_replays"`
	Requests map[string]string `json:"operation_sha256"`
}

func (n *terminalCandidateNetwork) handler(t *testing.T, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || !strings.Contains(r.URL.Path, "/runner/terminal-sessions/") || !strings.HasSuffix(r.URL.Path, "/exchange") {
			next.ServeHTTP(w, r)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var req term.ExchangeRequest
		if json.Unmarshal(raw, &req) != nil {
			t.Error("original terminal request is invalid")
			w.WriteHeader(400)
			return
		}
		n.mu.Lock()
		if previous, exists := n.Requests[req.OperationID]; exists {
			if previous != digest(string(raw)) {
				t.Error("original exchange changed its request bytes")
			}
			n.Replays++
		} else {
			n.Requests[req.OperationID] = digest(string(raw))
		}
		n.mu.Unlock()
		response := httptest.NewRecorder()
		next.ServeHTTP(response, r)
		n.mu.Lock()
		lose := !n.Dropped && response.Code >= 200 && response.Code < 300 && req.TreeStopped && req.SourceSnapshotSHA256 != ""
		if lose {
			n.Dropped = true
		}
		n.mu.Unlock()
		if lose {
			connection, _, e := w.(http.Hijacker).Hijack()
			if e != nil {
				t.Error(e)
				return
			}
			_ = connection.Close()
			return
		}
		for key, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
}

func terminalCandidateRuntimeTools(t *testing.T, f *fixture, worker string, cfg *runner.ExecConfig) (map[string]ToolProfile, string) {
	t.Helper()
	ctx := context.Background()
	inventory, err := runner.DescribeTools(ctx, *cfg)
	if err != nil || len(inventory) != len(cfg.Commands) {
		t.Fatalf("real fixed inventory: %+v %v", inventory, err)
	}
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	profiles := map[string]ToolProfile{}
	for _, item := range inventory {
		if !item.Available {
			t.Fatal("actual fixed profile is unavailable", item)
		}
		p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Owned terminal candidate runtime command", Profile: item.Profile, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profiles[item.Profile.CommandID] = p
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	for id, p := range profiles {
		p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profiles[id] = p
	}
	cfg.ToolInventory = inventory
	return profiles, token
}

func terminalCandidateRefreshRuntimeTools(t *testing.T, ctx context.Context, f *fixture, worker, token string, cfg *runner.ExecConfig) {
	t.Helper()
	inventory, err := runner.DescribeTools(ctx, *cfg)
	if err != nil || len(inventory) != len(cfg.Commands) {
		t.Fatalf("current actual tool probe failed: %+v %v", inventory, err)
	}
	for _, item := range inventory {
		if !item.Available {
			t.Fatal("current actual tool probe reported an unavailable command", item)
		}
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	cfg.ToolInventory = inventory
}

// Actual Windows ConPTY edits are copied only after the original tree stopped.
// The fixture owner exercises Review; no customer sign-off or model is claimed.
func TestWorkspaceTerminalCandidateActualStoppedSourceIndependentQuality(t *testing.T) {
	terminalCandidateActualJourney(t, false, false)
}

func TestWorkspaceTerminalAcceptedBaselineActualFollowupDevelopment(t *testing.T) {
	terminalCandidateActualJourney(t, true, false)
}

func TestWorkspaceTerminalCandidateActualWorkBuddyIndependentQuality(t *testing.T) {
	terminalCandidateActualJourney(t, false, true)
}

func terminalCandidateActualJourney(t *testing.T, continueBaseline, fromWorkBuddy bool) {
	if runtime.GOOS != "windows" {
		t.Skip("actual Windows ConPTY containment fixture")
	}
	duration := 45 * time.Second
	if fromWorkBuddy {
		duration = 60 * time.Second
	}
	if continueBaseline {
		duration = 180 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	var f *fixture
	var cloud workBuddyTaskDispatchFixture
	if fromWorkBuddy {
		cloud = newWorkBuddyTaskDispatchFixture(t)
		f = cloud.f
	} else {
		f = newFixture(t)
	}
	f.e.AttemptAccounting = f.s
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Fatal(err)
	}
	evidenceDir := ""
	stage, owned, environment := "T-TERMINAL-130", "terminal-130", "SUMA_TERMINAL_CANDIDATE_EVIDENCE_DIR"
	if continueBaseline {
		stage, owned, environment = "T-BASELINE-131", "baseline-131", "SUMA_BASELINE_EVIDENCE_DIR"
	}
	if requested := os.Getenv(environment); requested != "" {
		projectRoot, e := filepath.Abs(filepath.Join("..", ".."))
		if e != nil {
			t.Fatal(e)
		}
		evidenceDir = workbenchRetainedDirectory(t, requested, filepath.Join(projectRoot, ".cache", "e2e", owned), true)
	}
	defer func() {
		if evidenceDir == "" || !t.Failed() {
			return
		}
		receipt := map[string]any{"stage": stage, "status": "failed", "kind": "actual_terminal_candidate_failure_forensics", "production_complete": false}
		counts := map[string]int{}
		for _, table := range []string{"dag_runs", "dag_attempts", "suma_tasks", "suma_workspace_terminal_sessions", "suma_workspace_terminal_candidate_plans", "suma_candidate_transfers", "suma_test_plan_reports", "suma_reviews", "suma_issues", "suma_requirements"} {
			var count int
			if e := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); e != nil {
				receipt["count_error_"+table] = e.Error()
			} else {
				counts[table] = count
			}
		}
		receipt["table_counts"] = counts
		backup := filepath.Join(evidenceDir, "failure.fixture.sqlite")
		if _, e := os.Lstat(backup); os.IsNotExist(e) {
			if _, e = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(backup, "'", "''") + "'"); e != nil {
				receipt["database_backup_error"] = e.Error()
			} else if raw, e := os.ReadFile(backup); e == nil {
				receipt["database_sha256"], receipt["database_bytes"] = digest(string(raw)), len(raw)
			}
		}
		path := filepath.Join(evidenceDir, "failure.public.json")
		if _, e := os.Lstat(path); os.IsNotExist(e) {
			raw, _ := json.MarshalIndent(receipt, "", "  ")
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Error(e)
			}
		}
	}()
	repo := t.TempDir()
	if evidenceDir != "" {
		repo = filepath.Join(evidenceDir, "source")
		if err = os.Mkdir(repo, 0700); err != nil {
			t.Fatal(err)
		}
	}
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	const original = "module.exports=(a,b)=>a-b;\n"
	workspaceGitTestWrite(t, repo, "sum.cjs", original)
	workspaceGitTestCommand(t, repo, "add", "--", ".gitignore", "sum.cjs")
	workspaceGitTestCommand(t, repo, "commit", "-m", "Owned original terminal candidate source")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	independentRepo := t.TempDir()
	if evidenceDir != "" {
		independentRepo = filepath.Join(evidenceDir, "independent-source")
	}
	workspaceGitTestCommand(t, repo, "clone", "--no-hardlinks", "--", repo, independentRepo)
	if got := workspaceGitTestCommand(t, independentRepo, "rev-parse", "HEAD"); got != base {
		t.Fatal("independent original clone differs from base")
	}
	t.Setenv("SUMA_WORKSPACE_ROOT", repo)
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": repo}), 200)
	const check = `if(require('./sum.cjs')(2,3)!==5)process.exit(1);console.log('Fixed development check passed; independent test and Review remain required.');`
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{
		"terminal130":        {Adapter: "command", Protocol: "text-v1", Executable: shell, Args: []string{"/d", "/q", "/k"}, TerminalSession: true},
		"candidate-check130": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", check}},
	}}
	const junit = `const fs=require('node:fs'),ok=require('./sum.cjs')(2,3)===5;fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="terminal130" name="adds">'+(ok?'':'<failure message="arithmetic mismatch"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	verifier := runner.ExecConfig{AllowExec: true, Workspace: independentRepo, Repository: independentRepo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"candidate-test130": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", junit, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	profiles, token := terminalCandidateRuntimeTools(t, f, "terminal-source-worker130", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "terminal-independent-worker130", &verifier)
	network := &terminalCandidateNetwork{Requests: map[string]string{}}
	control := httptest.NewServer(network.handler(t, f.handler))
	defer control.Close()
	client := &runner.Client{BaseURL: control.URL, Token: token, ProjectID: f.project}
	independent := &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	developer.TerminalClient, developer.Artifacts = client, client
	verifier.Artifacts = independent
	task := cloud.task
	if !fromWorkBuddy {
		task = issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Review original stopped terminal changes", "acceptance_criteria": []string{"sum(2,3) is 5"}}), 201))
	}
	quality := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "terminal-adds", Title: "Stopped terminal candidate adds", Description: "Verify the exact sealed SHA on a different Worker", Preconditions: "Use original transferred source", Steps: []string{"Evaluate sum(2,3)"}, ExpectedResult: "5", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "terminal130", Name: "adds"}})
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	var originalCloud dag.Assignment
	if fromWorkBuddy {
		cloud.req.ExpectedTaskVersion = task.Version
		originalCloud = terminalCandidateWaitingWorkBuddy(t, cloud)
		task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
		if err != nil || task.RunID != originalCloud.RunID {
			t.Fatal("original cloud task binding missing", err)
		}
	}
	before := workspaceTerminalTaskSQL(t, f, task.ID)
	shellProfile := profiles["terminal130"]
	session, err := f.s.CreateWorkspaceTerminal(ctx, f.e, f.actor, f.project, WorkspaceTerminalCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ToolProfileID: shellProfile.ID, ExpectedToolVersion: shellProfile.Version, Columns: 100, Rows: 30, TimeoutSeconds: 120, AcknowledgeShell: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := client.Claim(ctx, shellProfile.WorkerID, []string{"command:terminal130", "command:candidate-check130"})
	if err != nil || assignment == nil {
		t.Fatal("real terminal assignment", err)
	}
	terminalDone := make(chan runner.Result, 1)
	go func() { defer close(terminalDone); terminalDone <- runner.Execute(ctx, *assignment, developer) }()
	defer func() {
		cancel()
		select {
		case <-terminalDone:
		case <-time.After(5 * time.Second):
		}
	}()
	for deadline := time.Now().Add(15 * time.Second); ; {
		session, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if session.State == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("original PTY did not start: %+v", session)
		}
		time.Sleep(30 * time.Millisecond)
	}
	input := fmt.Sprintf("\"%s\" -e \"require('node:fs').writeFileSync('sum.cjs','module.exports=(a,b)=>a+b;\\n')\"\r\nexit\r\n", node)
	inputID, key := newID(), newID()
	for retry := 0; ; retry++ {
		_, err = f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, WorkspaceTerminalCommandRequest{ProjectID: f.project, ExpectedVersion: session.Version, ControlEpoch: session.ControlEpoch, Action: "input", InputID: inputID, InputSequence: session.NextInputSequence, DataBase64: base64.StdEncoding.EncodeToString([]byte(input)), IdempotencyKey: key})
		if err == nil {
			break
		}
		if retry >= 4 {
			t.Fatal(err)
		}
		session, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	var terminalResult runner.Result
	select {
	case terminalResult = <-terminalDone:
	case <-ctx.Done():
		t.Fatal("original shell did not complete")
	}
	if terminalResult.OutcomeUnknown || terminalResult.Error != "" || terminalResult.ExitCode != 0 {
		t.Fatalf("actual terminal result: %+v", terminalResult)
	}
	terminalResult.OperationID = newID()
	if err = client.Complete(ctx, terminalResult); err != nil {
		t.Fatal(err)
	}
	session, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil || session.SourceSnapshotSHA256 == "" || session.BaseCommit != base {
		t.Fatalf("safe stopped source is missing: %+v %v", session, err)
	}
	producer := profiles["candidate-check130"]
	wire := f.call("POST", "/workflows", f.body(map[string]any{"name": "Original terminal candidate and independent verification", "nodes": []any{
		map[string]any{"id": "capture", "type": "command", "command": []string{"candidate-check130"}, "config": map[string]any{"tool_profile_id": producer.ID, "tool_profile_sha256": producer.Profile.ProfileSHA256}},
		map[string]any{"id": "independent", "type": "test", "command": []string{"candidate-test130"}, "depends_on": []string{"capture"}, "config": map[string]any{"candidate_from": "capture", "tool_profile_id": testProfile.ID, "tool_profile_sha256": testProfile.Profile.ProfileSHA256}},
	}}), 201)
	terminalCandidateRefreshRuntimeTools(t, ctx, f, producer.WorkerID, token, &developer)
	terminalCandidateRefreshRuntimeTools(t, ctx, f, testProfile.WorkerID, testToken, &verifier)
	request := WorkspaceTerminalCandidatePreviewRequest{ProjectID: f.project, ExpectedSessionVersion: session.Version, ExpectedTaskVersion: task.Version, ToolProfileID: producer.ID, ExpectedToolVersion: producer.Version, WorkflowID: wire["id"].(string), WorkflowVersion: int(wire["version"].(float64)), ProducerNodeID: "capture", AcknowledgeStoppedChanges: true, IdempotencyKey: newID()}
	plan := issueRepairRuntimeDecode[WorkspaceTerminalCandidatePlan](t, f.call("POST", "/workspace/terminal-sessions/"+session.ID+"/candidate-plans", request, 201))
	if !plan.Valid || plan.Source.SnapshotSHA256 != session.SourceSnapshotSHA256 || workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != before.SHA256 {
		t.Fatal("preview changed the task or lost stopped source", plan)
	}
	if fromWorkBuddy && (plan.Source.WorkBuddyOrigin == nil || plan.Source.WorkBuddyOrigin.RunID != originalCloud.RunID || plan.Source.WorkBuddyOrigin.Attempt != originalCloud.Attempt || plan.Source.WorkBuddyOrigin.Fence != originalCloud.Fence) {
		t.Fatal("candidate preview lost the original cloud attempt")
	}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	run := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/workspace/terminal-candidate-plans/"+plan.ID+"/start", start, 201))
	if again := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/workspace/terminal-candidate-plans/"+plan.ID+"/start", start, 201)); again.ID != run.ID {
		t.Fatal("start replay created another run")
	}
	development, err := client.Claim(ctx, producer.WorkerID, []string{"command:candidate-check130"})
	if err != nil || development == nil || development.RunID != run.ID {
		t.Fatal("protected producer claim missing", err)
	}
	produced := runner.Execute(ctx, *development, developer)
	if produced.ExitCode != 0 || produced.Error != "" || produced.WorkspaceEvidence["clean"] != true || produced.WorkspaceEvidence["terminal_candidate"] == nil {
		t.Fatalf("actual candidate failed: %+v", produced)
	}
	var proof term.TerminalCandidateEvidence
	if json.Unmarshal([]byte(stringify(produced.WorkspaceEvidence["terminal_candidate"])), &proof) != nil || proof.Source.SnapshotSHA256 != session.SourceSnapshotSHA256 || proof.Seal.Commit == base {
		t.Fatal("candidate provenance changed", proof)
	}
	if stringify(proof.Source.WorkBuddyOrigin) != stringify(plan.Source.WorkBuddyOrigin) {
		t.Fatal("sealed local candidate lost its frozen cloud origin")
	}
	produced.OperationID = newID()
	if err = client.Complete(ctx, produced); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Early review must remain blocked before independent tests"}), 400)
	test, err := independent.Claim(ctx, testProfile.WorkerID, []string{"command:candidate-test130"})
	if err != nil || test == nil || test.CandidateCommit != proof.Seal.Commit || test.WorkerID == development.WorkerID {
		t.Fatal("independent same-SHA candidate missing", err)
	}
	checked := runner.Execute(ctx, *test, verifier)
	if checked.ExitCode != 0 || checked.Error != "" || checked.TestReport == nil {
		t.Fatalf("actual isolated JUnit failed: %+v", checked)
	}
	checked.OperationID = newID()
	if err = independent.Complete(ctx, checked); err != nil {
		t.Fatal(err)
	}
	execution := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+quality.ID+"/executions/"+run.ID+"?project_id="+f.project, nil, 200))
	if execution.Passed != 1 {
		t.Fatal("published test plan did not receive exact source evidence", execution)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Fixture owner explicitly accepts original stopped edits after independent same-SHA JUnit"}), 200)
	accepted, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fromWorkBuddy {
		original, e := f.e.GetRun(ctx, f.user.TenantID, f.project, originalCloud.RunID)
		if e != nil || original.State != "running" || len(original.Nodes) != 1 || original.Nodes[0].State != "waiting" || original.Nodes[0].Reason != "cloud.delivery_required" || original.Nodes[0].Attempt != originalCloud.Attempt || original.Nodes[0].Fence != originalCloud.Fence || cloud.creates.Load() != 1 || accepted.Status != "accepted" || accepted.RunID != run.ID {
			t.Fatal("local acceptance changed the original cloud outcome or lost the new task binding", e)
		}
		return
	}
	startedPlan, err := f.s.GetWorkspaceTerminalCandidatePlan(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	followupBody := map[string]any{"title": "Continue from the explicitly accepted original terminal candidate"}
	if continueBaseline {
		followupBody["acceptance_criteria"] = []string{"Preserve the exact adopted arithmetic behavior", "Add the explicitly requested follow-up source marker"}
	}
	followup := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(followupBody), 201))
	adopt := map[string]any{"project_id": f.project, "expected_task_version": followup.Version, "expected_binding_version": 0, "terminal_candidate_plan_id": plan.ID, "expected_terminal_candidate_plan_version": startedPlan.Version, "acknowledge": true, "reason": "Fixture owner explicitly adopts the accepted terminal candidate as this unstarted task's baseline", "idempotency_key": newID()}
	bound := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks/"+followup.ID+"/code-baseline", adopt, 200))
	if bound.CodeBaseline == nil || bound.CodeBaseline.Source.HeadCommit != proof.Seal.Commit || bound.RunID != "" {
		t.Fatal("follow-up task lost its exact accepted baseline", bound)
	}
	if repeated := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks/"+followup.ID+"/code-baseline", adopt, 200)); repeated.Version != bound.Version {
		t.Fatal("original adoption replay changed follow-up task")
	}
	var continuation map[string]any
	if continueBaseline {
		continuation = terminalBaselineActualContinuation(t, ctx, f, bound, developer, verifier, profiles, client, control.URL, proof.Seal.Commit, base, evidenceDir)
	}
	beforeIssues := expertCount(t, f, "suma_issues")
	reject := f.body(map[string]any{"expected_version": accepted.Version, "run_id": run.ID, "decision": "rejected", "reason": "Fixture owner records additional requirements after the original candidate was accepted"})
	f.call("POST", "/tasks/"+task.ID+"/review", reject, 200)
	f.call("POST", "/tasks/"+task.ID+"/review", reject, 200)
	if continueBaseline {
		currentFollowup, e := f.s.Get(ctx, f.actor, f.project, "tasks", followup.ID)
		if e != nil {
			t.Fatal(e)
		}
		beforeReviews := expertCount(t, f, "suma_reviews")
		if _, e = f.s.Review(ctx, f.actor, f.project, followup.ID, f.body(map[string]any{"expected_version": currentFollowup.Version, "run_id": currentFollowup.RunID, "decision": "accepted", "reason": "A new follow-up approval must revalidate the rejected upstream source"}), f.e); e == nil {
			t.Fatal("a new follow-up approval reused a rejected baseline")
		}
		if expertCount(t, f, "suma_reviews") != beforeReviews {
			t.Fatal("rejected upstream basis still wrote a follow-up review")
		}
		continuation["upstream_rejection_blocks_new_followup_review"] = true
	}
	if expertCount(t, f, "suma_issues") != beforeIssues+1 {
		t.Fatal("original rejection did not create exactly one canonical issue")
	}
	delivery := f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	issues := delivery["issues"].(map[string]any)["items"].([]any)
	if len(issues) != 1 {
		t.Fatal("original terminal review issue missing", delivery)
	}
	issue := issues[0].(map[string]any)
	if issue["source"] != "review_rejected" {
		t.Fatal("terminal review issue lost canonical source", issue)
	}
	requirement, err := f.s.IssueToRequirement(ctx, f.actor, f.project, issue["issue_id"].(string), f.body(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if repeated, err := f.s.IssueToRequirement(ctx, f.actor, f.project, issue["issue_id"].(string), f.body(map[string]any{})); err != nil || repeated.ID != requirement.ID {
		t.Fatal("original issue intake duplicated its requirement", err)
	}
	baseline := issueRepairRuntimeDecode[CandidateCompositionBaseline](t, f.call("GET", "/tasks/"+followup.ID+"/code-baseline?project_id="+f.project, nil, 200))
	if len(baseline.Blockers) == 0 {
		t.Fatal("later rejection left the adopted source qualified", baseline)
	}
	history := issueRepairRuntimeDecode[WorkspaceTerminalCandidatePlanPage](t, f.call("GET", "/workspace/terminal-sessions/"+session.ID+"/candidate-plans?project_id="+f.project, nil, 200))
	if len(history.Items) != 1 || history.Items[0].ID != plan.ID || history.Items[0].State != "started" {
		t.Fatal("current authorized history lost the original started plan after task changes", history)
	}
	if parent, err := os.ReadFile(filepath.Join(repo, "sum.cjs")); err != nil || string(parent) != original {
		t.Fatal("parent user source changed", err)
	}
	if source, err := os.ReadFile(filepath.Join(session.Directory, "sum.cjs")); err != nil || !strings.Contains(string(source), "a+b") {
		t.Fatal("original terminal edits were lost", err)
	}
	if parent, err := os.ReadFile(filepath.Join(independentRepo, "sum.cjs")); err != nil || string(parent) != original {
		t.Fatal("independent administrator checkout changed", err)
	}
	if checked.WorkspaceEvidence["candidate_import"] == nil {
		t.Fatal("separate Worker did not import the original sealed bundle")
	}
	network.mu.Lock()
	if !network.Dropped || network.Replays < 1 {
		t.Error("original stopped source acknowledgement recovery was not exercised")
	}
	networkResult := map[string]any{"stopped_source_ack_lost_after_commit": network.Dropped, "original_operation_replays": network.Replays, "operation_sha256": network.Requests}
	network.mu.Unlock()
	if evidenceDir != "" {
		var acceptedReview, rejectedReview string
		if err = f.s.DB.QueryRow(`SELECT id FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND decision='accepted'`, f.user.TenantID, f.project, task.ID, run.ID).Scan(&acceptedReview); err != nil {
			t.Fatal(err)
		}
		if err = f.s.DB.QueryRow(`SELECT id FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND decision='rejected'`, f.user.TenantID, f.project, task.ID, run.ID).Scan(&rejectedReview); err != nil {
			t.Fatal(err)
		}
		journal, err := os.ReadFile(filepath.Join(filepath.Dir(session.Directory), "terminal-journal", "terminal.json"))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := os.ReadFile(filepath.Join(filepath.Dir(session.Directory), "terminal-candidate-source.json"))
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]any{"stage": stage, "kind": "actual_windows_pty_stopped_candidate_independent_git_http_junit_review", "project_id": f.project, "task_id": task.ID, "terminal_session_id": session.ID, "terminal_run_id": session.RunID, "terminal_snapshot_sha256": session.SourceSnapshotSHA256, "terminal_journal_sha256": digest(string(journal)), "terminal_manifest_file_sha256": digest(string(manifest)), "terminal_candidate_plan_id": plan.ID, "terminal_candidate_plan_version": startedPlan.Version, "candidate_run_id": run.ID, "base_commit": base, "candidate_commit": proof.Seal.Commit, "producer_worker": development.WorkerID, "independent_worker": test.WorkerID, "independent_source_repo": independentRepo, "junit_sha256": checked.TestReport.SHA256, "formal_cases_passed": execution.Passed, "accepted_review_id": acceptedReview, "followup_task_id": followup.ID, "followup_binding": bound.CodeBaseline, "rejected_review_id": rejectedReview, "canonical_issue_id": issue["issue_id"], "canonical_requirement_id": requirement.ID, "late_rejection_invalidates_followup_baseline": len(baseline.Blockers) > 0, "parent_checkouts_unchanged": true, "model_calls": 0, "customer_human_signoff": false, "production_complete": false}
		if continuation != nil {
			result["followup_development"] = continuation
		}
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "result.public.json"), result)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "network.public.json"), networkResult)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "terminal.public.json"), session)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "candidate-plan.public.json"), startedPlan)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "quality.public.json"), execution)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "delivery.public.json"), delivery)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "business-tables.public.json"), terminalRetainedBusinessTables(t, f.s.DB))
		backup := filepath.Join(evidenceDir, "fixture.sqlite")
		if _, err = f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(backup, "'", "''") + "'"); err != nil {
			t.Fatal(err)
		}
		if raw, err := os.ReadFile(backup); err != nil {
			t.Fatal(err)
		} else {
			modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "database.public.json"), map[string]any{"bytes": len(raw), "sha256": digest(string(raw))})
		}
	}
	t.Logf("actual original terminal=%s source=%s candidate=%s independent_worker=%s cases=%d; fixed command+seal is separate from Review", session.ID, session.SourceSnapshotSHA256, proof.Seal.Commit, test.WorkerID, execution.Passed)
}
