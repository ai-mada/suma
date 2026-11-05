package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The installed CLI, Git and test process are real. The Responses endpoint,
// review identity and task belong exclusively to this isolated fixture.
func TestCodexCandidateDeliveryActualInstalledCLI(t *testing.T) {
	if os.Getenv("SUMA_CODEX_DELIVERY_ACTUAL") != "1" {
		t.Skip("explicit installed Codex candidate-delivery opt-in required")
	}
	executable, expectedSHA := os.Getenv("SUMA_CODEX_DELIVERY_EXECUTABLE"), os.Getenv("SUMA_CODEX_DELIVERY_SHA256")
	if !filepath.IsAbs(executable) || !tc.ValidDigest(expectedSHA) {
		t.Fatal("select the exact absolute installed executable and its lowercase SHA-256")
	}
	file, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		t.Fatal("installed executable must be a regular file", statErr)
	}
	checksum := sha256.New()
	_, err = io.Copy(checksum, file)
	closeErr := file.Close()
	if err != nil || closeErr != nil || hex.EncodeToString(checksum.Sum(nil)) != expectedSHA {
		t.Fatal("installed executable does not match the reviewed SHA-256")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".cache", "codex-delivery-227", "actual"))
	if err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("SUMA_CODEX_DELIVERY_ACTUAL_DIR")
	if dir == "" {
		dir = filepath.Join(root, time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	dir = workbenchRetainedDirectory(t, dir, root, true)
	tmp := filepath.Join(dir, "tmp")
	if err = os.Mkdir(tmp, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		t.Setenv(key, tmp)
	}
	receipt := map[string]any{
		"status": "running", "step": "fixture_setup", "installed_executable_sha256": expectedSHA,
		"commercial_model_calls": 0, "customer_signoff": false,
	}
	defer func() {
		if t.Failed() {
			receipt["status"] = "failed"
		}
		codexActualJSON(t, filepath.Join(dir, "receipt.local.json"), receipt)
		t.Log("installed Codex candidate-delivery evidence retained at", dir)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	f := codexActualFixture(t, dir)
	f.e.AttemptAccounting = f.s
	reviewer := invitedReviewer(t, f)
	repo, testRepo := filepath.Join(dir, "repo"), filepath.Join(dir, "test-repo")
	if err = os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	const marker = "Immutable fixture marker 227\n"
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, ".gitattributes", "* -text\n")
	workspaceGitTestWrite(t, repo, "file.txt", marker)
	workspaceGitTestWrite(t, repo, "source.txt", codexActualOriginal)
	workspaceGitTestCommand(t, repo, "add", "--", ".gitignore", ".gitattributes", "file.txt", "source.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "Fixture: immutable Codex delivery base")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	workspaceGitTestCommand(t, dir, "clone", "--no-hardlinks", "--", repo, testRepo)
	if workspaceGitTestCommand(t, testRepo, "rev-parse", "HEAD") != base {
		t.Fatal("independent checkout has a different base")
	}

	owned := &codexOwnedResponses{t: t, worktrees: filepath.Join(repo, ".cache", "runner", "worktrees")}
	providerArrived, providerRelease := make(chan struct{}), make(chan struct{})
	releaseProvider := sync.OnceFunc(func() { close(providerRelease) })
	var firstResponse sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		firstResponse.Do(func() {
			close(providerArrived)
			select {
			case <-providerRelease:
			case <-r.Context().Done():
			}
		})
		owned.ServeHTTP(w, r)
	}))
	defer func() {
		releaseProvider()
		provider.Close()
		receipt["owned_responses"] = owned.receipt()
	}()
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{
		"codex-delivery-227": {Adapter: "codex", Protocol: cc.Protocol, Executable: executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &runner.CodexAppServerSpec{ProviderID: "owned227", BaseURL: provider.URL + "/v1", WindowsSandbox: "unelevated"}},
	}}
	check := fmt.Sprintf(`const fs=require('node:fs');const ok=fs.readFileSync('source.txt','utf8')===%q&&fs.readFileSync('file.txt','utf8')===%q;fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="codex227" name="exact-source">'+(ok?'':'<failure message="candidate source differs"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`, codexActualChanged, marker)
	independent := runner.ExecConfig{AllowExec: true, Workspace: testRepo, Repository: testRepo, BaseCommit: base, Commands: map[string]runner.CommandSpec{
		"independent-test-227": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", check, "{{test_report_path}}"}, TestEvidence: true, TestReport: true},
	}}
	receipt["step"] = "tool_discovery"
	producer, producerToken := issueRepairRuntimeTool(t, f, "codex-developer-227", &developer)
	tester, testerToken := issueRepairRuntimeTool(t, f, "codex-independent-227", &independent)
	catalog := developer.ToolInventory[0].CodexCatalog
	if catalog == nil || producer.Profile.ExecutableSHA256 != expectedSHA {
		t.Fatal("installed Codex catalog/executable does not match the approved source")
	}
	selection := cc.Selection{CatalogSHA256: catalog.CatalogSHA256, TokenNoticeLimit: usageInt(1)}
	for _, option := range catalog.Models {
		if option.DefaultReasoningEffort != "" {
			selection.Model, selection.ReasoningEffort = option.Model, option.DefaultReasoningEffort
			break
		}
	}
	if selection.Model == "" {
		t.Fatal("installed Codex did not advertise an explicit model and reasoning effort")
	}
	owned.mu.Lock()
	owned.model = selection.Model
	owned.mu.Unlock()
	receipt["step"] = "preview_and_start"
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{
		"title": "Deliver the owned Codex source change", "description": "Change source.txt to exactly Codex app-server owned edit 041 followed by a newline.",
		"acceptance_criteria": []string{"An independent worker tests the exact sealed source before review"},
	}), 201))
	quality := issueRepairRuntimePlan(t, f, task, QualityTestCase{
		Key: "exact-source", Title: "Independent exact source", Description: "Check committed fixture bytes on the separate checkout",
		Preconditions: "Original candidate is sealed", Steps: []string{"Read source.txt and the immutable marker"}, ExpectedResult: "Exact edited bytes and unchanged marker",
		Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "codex227", Name: "exact-source"},
	})
	workflow := acceptedBaselineActualWorkflow(t, ctx, f, "Owned Codex candidate delivery", tester)
	plan := issueRepairRuntimeDecode[TaskDispatchPlan](t, f.call("POST", "/task-dispatch-plans", TaskDispatchPreviewRequest{
		ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: workflow.ID, WorkflowVersion: workflow.Version,
		AgentNodeID: "develop", ToolProfileID: producer.ID, ExpectedToolVersion: producer.Version, CodexExecutionSelection: &selection,
		TestBindings: []TaskDispatchTestSelection{{NodeID: "test", ToolProfileID: tester.ID, ExpectedToolVersion: tester.Version}}, IdempotencyKey: newID(),
	}, 201))
	expectedBinding, err := cc.NewBinding(*catalog, selection)
	if err != nil || plan.CodexExecution == nil || cc.ValidateBinding(*plan.CodexExecution) != nil || plan.CodexExecution.BindingSHA256 != expectedBinding.BindingSHA256 || plan.ModelExecution != nil {
		t.Fatal("the original preview lost the exact selected model, effort or observe-only threshold", err)
	}
	run := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/task-dispatch-plans/"+plan.ID+"/start", TaskDispatchStartRequest{
		ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID(),
	}, 201))
	receipt["binding"], receipt["run_id"], receipt["base_commit"] = plan.CodexExecution, run.ID, base
	control := httptest.NewServer(f.handler)
	defer control.Close()
	shared := originalDevelopmentRuntime{f: f, control: control, directory: dir, receipt: receipt}
	receipt["step"] = "live_progress"
	stopProducer := shared.startWorker(t, ctx, producer.WorkerID, producerToken, "developer-journal", developer)
	defer func() {
		releaseProvider()
		if stopProducer != nil {
			stopProducer()
		}
	}()
	select {
	case <-providerArrived:
	case <-ctx.Done():
		t.Fatal("installed CLI never reached the isolated Responses endpoint")
	}
	liveCtx, stopLive := context.WithTimeout(ctx, 10*time.Second)
	defer stopLive()
	for {
		events, e := f.e.Events(liveCtx, f.user.TenantID, f.project, run.ID, 0)
		if e != nil {
			t.Fatal(e)
		}
		sessionStarted, turnStarted := false, false
		for _, event := range events {
			if event.Type != "tool_progress" || event.NodeID != "develop" || event.Data["worker_id"] != producer.WorkerID {
				continue
			}
			tool, _ := event.Data["tool_event"].(map[string]any)
			sessionStarted = sessionStarted || tool["kind"] == "session_started"
			turnStarted = turnStarted || tool["kind"] == "turn_started"
		}
		if sessionStarted && turnStarted {
			receipt["live_progress_before_first_response"] = true
			break
		}
		select {
		case <-liveCtx.Done():
			t.Fatal("original session/turn progress was not persisted before the first Responses reply")
		case <-time.After(75 * time.Millisecond):
		}
	}
	releaseProvider()

	receipt["step"] = "candidate"
	produced := acceptedBaselineActualWait(t, ctx, f, run.ID)
	stopProducer()
	stopProducer = nil
	receipt["producer"] = produced
	if err = owned.verify(); err != nil {
		t.Fatal(err)
	}
	candidate, _ := produced.WorkspaceEvidence["head_commit"].(string)
	if produced.WorkerID != producer.WorkerID || !gitRevision.MatchString(candidate) || candidate == base || produced.WorkspaceEvidence["clean"] != true {
		t.Fatal("installed CLI did not produce an independently testable clean candidate")
	}
	receipt["candidate_commit"] = candidate
	evidence := produced.ToolEvidence
	if evidence == nil || evidence.Terminal != "completed" || evidence.CodexExecution == nil || !cc.EvidenceMatchesBinding(expectedBinding, *evidence.CodexExecution) || evidence.UsageState != tc.UsagePartial || !evidence.CodexExecution.TokenNoticeExceeded {
		t.Fatal("completed development lost original Codex configuration/usage/notice evidence")
	}
	history := issueRepairRuntimeDecode[dag.AttemptToolEventPage](t, f.call("GET", fmt.Sprintf("/runs/%s/nodes/develop/attempts/%d/tool-events?project_id=%s", run.ID, produced.Attempt, f.project), nil, 200))
	if history.Fence != produced.Fence || history.WorkerID != producer.WorkerID || history.HasMore || len(history.Items) != evidence.EventCount {
		t.Fatal("persisted tool progress does not match the original exact attempt")
	}
	receipt["progress_event_count"] = len(history.Items)
	ledger, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(ledger.Items) != 1 {
		t.Fatal("original Codex attempt did not produce exactly one usage observation", err)
	}
	usage := ledger.Items[0]
	if usage.CodexExecution == nil || usage.CodexExecution.BindingSHA256 != expectedBinding.BindingSHA256 || usage.UsageState != tc.UsagePartial || usage.ReportedTokens == nil || *usage.ReportedTokens <= 1 || usage.Model != "" || usage.Rate != nil || usage.EstimatedCostMicros != nil || usage.ReservedTokens != 0 {
		t.Fatal("observed usage became an enforced reservation, a priced charge or a provider identity")
	}
	receipt["usage"] = usage

	receipt["step"] = "independent_test_and_review"
	verified, tested, accepted := shared.finishQuality(t, ctx, originalDevelopmentQuality{
		Repository: repo, BaseCommit: base, CandidateCommit: candidate, ExpectedDraft: marker, Task: task, Plan: quality, Run: run, Producer: produced,
		TestProfile: tester, TestToken: testerToken, Independent: independent, Reviewer: reviewer,
	})
	if verified.WorkspaceEvidence["candidate_import"] == nil {
		t.Fatal("separate worker did not import the original sealed candidate")
	}
	for _, checkout := range []string{repo, testRepo} {
		source, e := os.ReadFile(filepath.Join(checkout, "source.txt"))
		if e != nil || string(source) != codexActualOriginal || workspaceGitTestCommand(t, checkout, "rev-parse", "HEAD") != base {
			t.Fatal("an administrator checkout changed", e)
		}
	}
	receipt["independent_test"], receipt["test_plan_execution"], receipt["accepted_task"] = verified, tested, accepted
	receipt["review_before_test_rejected"] = true
	receipt["status"] = "passed"
	delete(receipt, "step")
}
