package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	rp "github.com/ai-mada/suma/internal/repositoryprovider"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

func TestTaskDeliveryRepositoryDispatchActualIndependentQuality(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is required for the repository delivery journey")
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the deterministic independent JUnit case")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cloud := newWorkBuddyTaskDispatchFixture(t)
	f := cloud.f
	f.e.AttemptAccounting = f.s
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: cloud.w}).Handler()

	source := t.TempDir()
	workspaceGitTestCommand(t, source, "init", "--initial-branch=main", "--object-format=sha1")
	const oldCode = "module.exports=(a,b)=>a-b;\n"
	workspaceGitTestWrite(t, source, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, source, "sum.cjs", oldCode)
	workspaceGitTestCommand(t, source, "add", "--", ".gitignore", "sum.cjs")
	workspaceGitTestCommand(t, source, "commit", "-m", "Administrator baseline")
	base := workspaceGitTestCommand(t, source, "rev-parse", "HEAD")
	// Clone before the delivery commit exists so each Runner must receive and
	// import its own authenticated bundle to obtain that exact Git object.
	producerRepo, independentRepo := t.TempDir(), t.TempDir()
	for _, dir := range []string{producerRepo, independentRepo} {
		workspaceGitTestCommand(t, source, "clone", "--no-hardlinks", "--", source, dir)
	}
	workspaceGitTestCommand(t, source, "switch", "-c", "delivery")
	workspaceGitTestWrite(t, source, "sum.cjs", "module.exports=(a,b)=>a+b;\n")
	workspaceGitTestCommand(t, source, "add", "--", "sum.cjs")
	workspaceGitTestCommand(t, source, "commit", "-m", "Exact approved delivery")
	head := workspaceGitTestCommand(t, source, "rev-parse", "HEAD")
	bare := filepath.Join(t.TempDir(), "provider.git")
	workspaceGitTestCommand(t, source, "clone", "--bare", "--no-hardlinks", "--", source, bare)

	const producerCommand = "repository-producer245"
	const independentCommand = "repository-independent245"
	producerCfg := runner.ExecConfig{AllowExec: true, Workspace: producerRepo, Repository: producerRepo, BaseCommit: base, Commands: map[string]runner.CommandSpec{
		// The protected producer is a Git import, so the pinned local command
		// is used for authority and inventory only. It must never be invoked.
		producerCommand: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", "process.exit(81)"}},
	}}
	const junit = "const fs=require('node:fs'),ok=require('./sum.cjs')(2,3)===5;fs.writeFileSync(process.argv[1],'<testsuite tests=\"1\" failures=\"'+(ok?'0':'1')+'\"><testcase classname=\"repository245\" name=\"adds\">'+(ok?'':'<failure message=\"wrong candidate content\"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;"
	independentCfg := runner.ExecConfig{AllowExec: true, Workspace: independentRepo, Repository: independentRepo, BaseCommit: base, Commands: map[string]runner.CommandSpec{
		independentCommand: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", junit, "{{test_report_path}}"}, TestEvidence: true, TestReport: true},
	}}
	producerProfile, producerToken := issueRepairRuntimeTool(t, f, "repository-source-worker245", &producerCfg)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "repository-test-worker245", &independentCfg)
	quality := issueRepairRuntimePlan(t, f, cloud.task, QualityTestCase{
		Key: "repository-adds", Title: "Verify the exact delivery commit", Description: "Use a different Runner and the sealed candidate",
		Preconditions: "Start from the administrator baseline", Steps: []string{"Evaluate sum(2,3)"}, ExpectedResult: "5",
		Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "repository245", Name: "adds"},
	})
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", cloud.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	cloud.req.ExpectedTaskVersion = task.Version
	origin := terminalCandidateWaitingWorkBuddy(t, cloud)
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil || task.RunID != origin.RunID {
		t.Fatal("original WorkBuddy task binding is missing", err)
	}

	const providerID, alias = "repository-delivery-provider245", "incoming"
	const controlEnv, runnerEnv, reconcileEnv, exportEnv = "SUMA_TEST_DISPATCH245_CONTROL", "SUMA_TEST_DISPATCH245_RUNNER", "SUMA_TEST_DISPATCH245_RECONCILE", "SUMA_TEST_DISPATCH245_EXPORT"
	t.Setenv(controlEnv, strings.Repeat("c", 48))
	t.Setenv(runnerEnv, strings.Repeat("r", 48))
	t.Setenv(reconcileEnv, strings.Repeat("x", 48))
	t.Setenv(exportEnv, strings.Repeat("e", 48))
	provider, err := rp.New(rp.Config{
		ProviderID: providerID, TenantID: f.user.TenantID, ProjectID: f.project,
		ControlURL: "http://127.0.0.1:1", ControlTokenEnv: controlEnv, RunnerTokenEnv: runnerEnv,
		ReconcileTokenEnv: reconcileEnv, SourceExportTokenEnv: exportEnv,
		GitExecutable: git, StateDir: filepath.Join(t.TempDir(), "provider"),
		Targets: map[string]rp.TargetConfig{
			alias:      {BareRepository: bare, Ref: "refs/heads/delivery", DeliveryBaseAlias: "baseline"},
			"baseline": {BareRepository: bare, Ref: "refs/heads/main"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	var providerRequests atomic.Int32
	providerHandler := provider.Handler()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequests.Add(1)
		providerHandler.ServeHTTP(w, r)
	}))
	defer remote.Close()
	f.s.RepositoryProviders = []RepositoryProviderConfig{{
		ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, TargetAliases: []string{alias},
		TokenHash: digest(os.Getenv(controlEnv)), Origin: remote.URL,
		ReconcileToken: os.Getenv(reconcileEnv), SourceExportToken: os.Getenv(exportEnv),
	}}
	target := issueRepairRuntimeDecode[RepositoryIntegrationTarget](t, f.call("POST", "/repository-integration-targets",
		RepositoryIntegrationTargetCreateRequest{ProjectID: f.project, Name: "Incoming source dispatch", ProviderID: providerID, TargetAlias: alias, IdempotencyKey: newID()}, 201))
	intakesPath := "/tasks/" + task.ID + "/delivery/repository-source/intakes"
	intake := issueRepairRuntimeDecode[TaskDeliveryRepositoryIntake](t, f.call("POST", intakesPath,
		TaskDeliveryRepositoryIntakeRequest{
			ProjectID: f.project, TaskVersion: task.Version, RunID: origin.RunID, TargetID: target.ID, TargetVersion: target.Version,
			ExpectedHead: head, ExpectedBase: base, IdempotencyKey: newID(),
		}, 201))
	if intake.HeadCommit != head || intake.BaseCommit != base {
		t.Fatal("approval did not retain the exact delivery source", intake)
	}
	providerReads := providerRequests.Load()

	control := httptest.NewServer(f.handler)
	defer control.Close()
	producer := &runner.Client{BaseURL: control.URL, Token: producerToken, ProjectID: f.project}
	independent := &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	producerCfg.Artifacts = producer
	independentCfg.Artifacts = independent
	terminalCandidateRefreshRuntimeTools(t, ctx, f, producerProfile.WorkerID, producerToken, &producerCfg)
	terminalCandidateRefreshRuntimeTools(t, ctx, f, testProfile.WorkerID, testToken, &independentCfg)
	previewPath := intakesPath + "/" + intake.ID + "/dispatch-plans"
	preview := f.body(map[string]any{
		"expected_task_version": task.Version, "producer_profile_id": producerProfile.ID,
		"expected_producer_profile_version": producerProfile.Version, "test_profile_id": testProfile.ID,
		"expected_test_profile_version": testProfile.Version,
	})
	before := workspaceTerminalTaskSQL(t, f, task.ID)
	plan := issueRepairRuntimeDecode[TaskDispatchPlan](t, f.call("POST", previewPath, preview, 201))
	if !plan.Valid || len(plan.Workflow.Nodes) != 2 || workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != before.SHA256 {
		t.Fatal("preview failed to freeze two-stage execution without changing the original task", plan)
	}
	startPath := "/task-dispatch-plans/" + plan.ID + "/start"
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	ownerCookie, ownerView := f.cookie, f.view
	const developerEmail = "repository-dispatch-developer245@example.test"
	invite := governanceInvite(f, developerEmail, "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	governanceAccept(f, invite, developerEmail)
	f.call("POST", startPath, start, 403)
	f.cookie, f.view = ownerCookie, ownerView
	runsBeforeStart := domainACount(t, f.s, "dag_runs")
	run := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", startPath, start, 201))
	afterStart := workspaceTerminalTaskSQL(t, f, task.ID)
	replay := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", startPath, start, 201))
	if run.ID == origin.RunID || replay.ID != run.ID || replay.Version != run.Version ||
		domainACount(t, f.s, "dag_runs") != runsBeforeStart+1 || workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != afterStart.SHA256 || providerRequests.Load() != providerReads {
		t.Fatal("dispatch did not reuse the retained approval or replayed as another run")
	}

	// The independent worker cannot obtain a testing assignment before the
	// producer's real completion has durably sealed the exact delivery.
	if early, claimErr := independent.Claim(ctx, testProfile.WorkerID, []string{"command:" + independentCommand}); claimErr != nil || early != nil {
		t.Fatal("independent testing became available before candidate sealing", claimErr)
	}
	assignment, err := producer.Claim(ctx, producerProfile.WorkerID, []string{"command:" + producerCommand})
	if err != nil || assignment == nil || assignment.RunID != run.ID || assignment.NodeID != "repository-source" {
		t.Fatal("protected repository producer assignment missing", err)
	}
	pinned, err := ri.DecodeRepositorySourceExecution(assignment.Config[ri.RepositorySourceExecutionKey])
	if err != nil || pinned == nil || pinned.IntakeID != intake.ID || pinned.PlanID != plan.ID || pinned.BaseCommit != base || pinned.HeadCommit != head || pinned.BundleSHA256 != intake.BundleSHA256 || pinned.SizeBytes != intake.SizeBytes {
		t.Fatal("Runner lease did not bind the exact approved intake", pinned, err)
	}
	ref := cc.AssignmentRef{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID}
	bundleURL := "/api/v1/runner/repository-source-intakes/" + intake.ID + "/bundle?project_id=" + f.project
	assertBundleDenied := func(token string, supplied cc.AssignmentRef, status int) {
		t.Helper()
		header, err := json.Marshal(supplied)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, bundleURL, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString(header))
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != status || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Content-Disposition") != "" || bytes.Contains(w.Body.Bytes(), []byte("PACK")) {
			t.Fatalf("unexpected bundle denial: status=%d, want %d, content-type=%q, response bytes=%d", w.Code, status, w.Header().Get("Content-Type"), w.Body.Len())
		}
	}
	assertBundleDenied(testToken, ref, http.StatusForbidden)
	stale := ref
	stale.Fence++
	assertBundleDenied(producerToken, stale, http.StatusConflict)

	beforeTransfers := domainACount(t, f.s, "suma_candidate_transfers")
	forged, err := json.Marshal(cc.StartRequest{
		ProjectID: f.project, Assignment: ref, OperationID: newID(), BaseCommit: base, BundleBaseCommit: base, HeadCommit: head,
		BundleSHA256: digest("wrong repository intake bytes"), SizeBytes: intake.SizeBytes, EvidenceSHA256: digest("forged evidence"),
	})
	if err != nil {
		t.Fatal(err)
	}
	startTransfer := httptest.NewRequest(http.MethodPost, "/api/v1/runner/candidate-transfers", bytes.NewReader(forged))
	startTransfer.Header.Set("Authorization", "Bearer "+producerToken)
	startTransfer.Header.Set("Content-Type", "application/json")
	transfer := httptest.NewRecorder()
	f.handler.ServeHTTP(transfer, startTransfer)
	if transfer.Code != http.StatusConflict || domainACount(t, f.s, "suma_candidate_transfers") != beforeTransfers {
		t.Fatalf("wrong bundle digest created a transfer: status=%d", transfer.Code)
	}
	produced := runner.Execute(ctx, *assignment, producerCfg)
	if produced.ExitCode != 0 || produced.Error != "" || produced.OutcomeUnknown || produced.WorkspaceEvidence["head_commit"] != head || produced.WorkspaceEvidence["candidate_bundle"] == nil {
		t.Fatalf("real repository producer failed to seal the approved head: %+v", produced)
	}
	sealed, ok := produced.WorkspaceEvidence["candidate_bundle"].(cc.Binding)
	if !ok || !sealed.ValidSealed() || sealed.BaseCommit != base || sealed.HeadCommit != head || sealed.BundleSHA256 != intake.BundleSHA256 || sealed.SizeBytes != intake.SizeBytes {
		t.Fatal("candidate seal did not preserve the approved bundle bytes", sealed)
	}
	if produced.OperationID == "" {
		produced.OperationID = newID()
	}
	beforeCompletion, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	withoutSeal := produced
	withoutSeal.WorkspaceEvidence = maps.Clone(produced.WorkspaceEvidence)
	delete(withoutSeal.WorkspaceEvidence, "candidate_bundle")
	if err = producer.Complete(ctx, withoutSeal); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
		t.Fatal("completion without the sealed transfer was not rejected as a conflict", err)
	}
	stillRunning, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
	if err != nil || stillRunning.State != "running" || stillRunning.Version != beforeCompletion.Version {
		t.Fatal("rejected completion changed the run state", err)
	}
	for _, node := range stillRunning.Nodes {
		if node.ID == assignment.NodeID && node.State != "running" || node.ID == "repository-test" && node.State == "ready" {
			t.Fatal("rejected completion advanced the producer or test", node.ID, node.State)
		}
	}
	// Retry with the original operation ID and genuine sealed-transfer proof.
	if err = producer.Complete(ctx, produced); err != nil {
		t.Fatal(err)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{
		"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Fixture rejects Review before independent tests",
	}), 400)

	check, err := independent.Claim(ctx, testProfile.WorkerID, []string{"command:" + independentCommand})
	if err != nil || check == nil || check.RunID != run.ID || check.NodeID != "repository-test" || check.CandidateCommit != head || check.WorkerID == assignment.WorkerID {
		t.Fatal("independent Runner did not claim the approved commit", err)
	}
	tested := runner.Execute(ctx, *check, independentCfg)
	if tested.ExitCode != 0 || tested.Error != "" || tested.TestReport == nil || tested.WorkspaceEvidence["head_commit"] != head {
		t.Fatalf("independent Runner did not test the exact imported head: %+v", tested)
	}
	imported, ok := tested.WorkspaceEvidence["candidate_import"].(cc.ImportReceipt)
	if !ok || !imported.Imported || imported.TargetWorkerID != check.WorkerID || imported.Binding.TransferID != sealed.TransferID || imported.Binding.BundleSHA256 != intake.BundleSHA256 {
		t.Fatal("independent Runner did not import the sealed source bundle", imported)
	}
	if tested.OperationID == "" {
		tested.OperationID = newID()
	}
	if err = independent.Complete(ctx, tested); err != nil {
		t.Fatal(err)
	}
	qualityRun := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+quality.ID+"/executions/"+run.ID+"?project_id="+f.project, nil, 200))
	if qualityRun.Passed != 1 {
		t.Fatal("published plan did not receive independent JUnit", qualityRun)
	}
	current, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{
		"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Fixture owner accepts the independently tested source",
	}), 200)
	accepted, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil || accepted.Status != "accepted" || accepted.RunID != run.ID {
		t.Fatal("formal acceptance did not bind to the new candidate execution", err)
	}
	originalRun, err := f.e.GetRun(ctx, f.user.TenantID, f.project, origin.RunID)
	if err != nil || originalRun.State != "running" || len(originalRun.Nodes) != 1 || originalRun.Nodes[0].State != "waiting" ||
		originalRun.Nodes[0].Reason != "cloud.delivery_required" || originalRun.Nodes[0].Attempt != origin.Attempt || originalRun.Nodes[0].Fence != origin.Fence ||
		cloud.creates.Load() != 1 || cloud.queries.Load() != 0 || providerRequests.Load() != providerReads {
		t.Fatal("repository delivery unexpectedly changed the original WorkBuddy outcome or contacted an external service", err)
	}
	for _, dir := range []string{producerRepo, independentRepo} {
		if currentHead := workspaceGitTestCommand(t, dir, "rev-parse", "HEAD"); currentHead != base {
			t.Fatal("a canonical Runner checkout moved during isolated execution", currentHead)
		}
		if status := workspaceGitTestCommand(t, dir, "status", "--porcelain=v1"); status != "" {
			t.Fatal("a canonical Runner checkout changed during isolated execution", status)
		}
	}
	if err = exec.CommandContext(ctx, git, "-C", producerRepo, "cat-file", "-e", head+"^{commit}").Run(); err == nil {
		t.Fatal("producer wrote the approved Git objects to the canonical repository")
	} else if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
}
