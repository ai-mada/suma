package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	testTaskDeliveryRepositoryDispatchActualIndependentQuality(t, false)
}

func repositoryDispatchPoolTool(t *testing.T, ctx context.Context, f *fixture, poolID, controlURL, name string, cfg *runner.ExecConfig) (ToolProfile, *runner.Client) {
	t.Helper()
	credential, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, poolID, ExecutionPoolRunnerCreateRequest{
		Name: name, MaxSlots: 2, TTLSeconds: 3600, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	machine := credential.Runner
	client := &runner.Client{BaseURL: controlURL, Token: credential.Token, ProjectID: f.project,
		Pool: &runner.PoolContext{PoolID: poolID, RunnerID: machine.ID}}
	inventoryClient := *client
	inventoryClient.Pool = &runner.PoolContext{PoolID: poolID, RunnerID: machine.ID, TenantID: f.user.TenantID, ProjectID: f.project}
	inventory, err := runner.DescribeTools(ctx, *cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatalf("actual Pool command inventory unavailable: %+v %v", inventory, err)
	}
	scope, err := runner.InventoryScopeSHA256(controlURL, f.project, machine.WorkerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inventoryClient.ToolInventory(ctx, runner.InventoryReport{
		WorkerID: machine.WorkerID, ScopeSHA256: scope, Items: inventory, OperationID: newID(),
	}); err != nil {
		t.Fatal(err)
	}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{
		ProjectID: f.project, WorkerID: machine.WorkerID, Name: name, Profile: inventory[0].Profile, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{
		ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID(),
	})
	if err != nil || !profile.Available {
		t.Fatalf("approved Pool tool profile unavailable: %+v %v", profile, err)
	}
	cfg.ToolInventory = inventory
	return profile, client
}

func TestTaskDeliveryRepositoryDispatchPoolActualIndependentQuality(t *testing.T) {
	testTaskDeliveryRepositoryDispatchActualIndependentQuality(t, true)
}

func testTaskDeliveryRepositoryDispatchActualIndependentQuality(t *testing.T, pooled bool) {
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
	control := httptest.NewServer(f.handler)
	defer control.Close()
	var producerProfile, testProfile ToolProfile
	var producer, independent *runner.Client
	var pool ExecutionPool
	var grant ExecutionPoolGrant
	if pooled {
		// Leave the original WorkBuddy attempt on its frozen project route.
		// Only the new candidate and its independent test use the approved Pool.
		pool, grant = dispatchAuthorizedPool(t, f)
		dispatchSetRoute(t, f, 0, &grant)
		producerProfile, producer = repositoryDispatchPoolTool(t, ctx, f, pool.ID, control.URL, "Repository source Pool Runner", &producerCfg)
		testProfile, independent = repositoryDispatchPoolTool(t, ctx, f, pool.ID, control.URL, "Independent JUnit Pool Runner", &independentCfg)
	} else {
		var producerToken, testToken string
		producerProfile, producerToken = issueRepairRuntimeTool(t, f, "repository-source-worker245", &producerCfg)
		testProfile, testToken = issueRepairRuntimeTool(t, f, "repository-test-worker245", &independentCfg)
		producer = &runner.Client{BaseURL: control.URL, Token: producerToken, ProjectID: f.project}
		independent = &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	}
	claim := func(client *runner.Client, profile ToolProfile, command, slot string) (*runner.Assignment, error) {
		if !pooled {
			return client.Claim(ctx, profile.WorkerID, []string{"command:" + command})
		}
		return client.ClaimPool(ctx, runner.PoolClaimRequest{
			PoolID: pool.ID, RunnerID: client.Pool.RunnerID, WorkerID: profile.WorkerID, SlotID: slot, OperationID: newID(),
			Scopes: []runner.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + command}}},
		})
	}
	forAssignment := func(client *runner.Client, a *runner.Assignment) *runner.Client {
		if !pooled {
			return client
		}
		if a.PoolID != pool.ID || a.PoolGrantID != grant.ID || a.PoolGrantVersion != grant.Version || a.PoolRunnerID != client.Pool.RunnerID || a.PoolSlotID == "" || a.TenantID != f.user.TenantID || a.ProjectID != f.project {
			t.Fatalf("unexpected frozen Pool assignment authority: %+v", a)
		}
		scoped := *client
		poolContext := *client.Pool
		poolContext.Assignment = a
		scoped.Pool = &poolContext
		return &scoped
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
	var basisOffline atomic.Bool
	providerHandler := provider.Handler()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerRequests.Add(1)
		if basisOffline.Load() && strings.HasPrefix(r.URL.Path, "/source-export-basis/") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
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
	if !pooled {
		terminalCandidateRefreshRuntimeTools(t, ctx, f, producerProfile.WorkerID, producer.Token, &producerCfg)
		terminalCandidateRefreshRuntimeTools(t, ctx, f, testProfile.WorkerID, independent.Token, &independentCfg)
	}
	optionsPath := "/tasks/" + task.ID + "/delivery/repository-source/options?" + url.Values{
		"project_id": {f.project}, "task_version": {strconv.Itoa(task.Version)}, "run_id": {origin.RunID},
		"target_id": {target.ID}, "target_version": {strconv.Itoa(target.Version)},
	}.Encode()
	providerReads := providerRequests.Load()
	f.call("GET", optionsPath+"&unexpected=1", nil, 400)
	if providerRequests.Load() != providerReads {
		t.Fatal("invalid workbench options request reached the provider")
	}
	assertOptions := func(wantLive string) {
		t.Helper()
		options := issueRepairRuntimeDecode[TaskDeliveryRepositorySourceOptions](t, f.call("GET", optionsPath, nil, 200))
		if options.LiveState != wantLive || options.ActiveTerminal || options.PublishedCaseCount != 1 ||
			options.RunID != origin.RunID || options.NodeID != origin.NodeID || options.Attempt != origin.Attempt ||
			len(options.RetainedIntakes) != 1 || options.RetainedIntakes[0] != intake ||
			len(options.ProducerProfiles) != 1 || options.ProducerProfiles[0].ID != producerProfile.ID ||
			len(options.TestProfiles) != 1 || options.TestProfiles[0].ID != testProfile.ID {
			t.Fatalf("unexpected repository handoff options: %+v", options)
		}
		if wantLive == "ready" && (options.HeadCommit != head || options.BaseCommit != base) ||
			wantLive == "unavailable" && (options.HeadCommit != "" || options.BaseCommit != "") {
			t.Fatalf("unexpected live repository evidence: %+v", options)
		}
	}
	assertOptions("ready")
	basisOffline.Store(true)
	assertOptions("unavailable")
	// Subsequent plan and start must use the retained source without a live
	// provider, which was just observed to be unavailable.
	providerReads = providerRequests.Load()
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
	f.call("GET", optionsPath, nil, 403)
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

	if pooled && os.Getenv(repositoryPoolRunnerExecutableEnv) != "" {
		runRepositoryPoolProcesses(t, ctx, f, run.ID, intake, producer, producerCfg, independent, independentCfg)
	} else {
		// The independent worker cannot obtain a testing assignment before the
		// producer's real completion has durably sealed the exact delivery.
		if early, claimErr := claim(independent, testProfile, independentCommand, "1"); claimErr != nil || early != nil {
			t.Fatal("independent testing became available before candidate sealing", claimErr)
		}
		assignment, err := claim(producer, producerProfile, producerCommand, "0")
		if err != nil || assignment == nil || assignment.RunID != run.ID || assignment.NodeID != "repository-source" {
			t.Fatal("protected repository producer assignment missing", err)
		}
		pinned, err := ri.DecodeRepositorySourceExecution(assignment.Config[ri.RepositorySourceExecutionKey])
		if err != nil || pinned == nil || pinned.IntakeID != intake.ID || pinned.PlanID != plan.ID || pinned.BaseCommit != base || pinned.HeadCommit != head || pinned.BundleSHA256 != intake.BundleSHA256 || pinned.SizeBytes != intake.SizeBytes {
			t.Fatal("Runner lease did not bind the exact approved intake", pinned, err)
		}
		producerCfg.Artifacts = forAssignment(producer, assignment)
		ref := cc.AssignmentRef{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID}
		namespace := "runner"
		if pooled {
			namespace = "pool-runner"
			ref.TenantID, ref.ProjectID = assignment.TenantID, assignment.ProjectID
			ref.PoolID, ref.PoolRunnerID, ref.PoolSlotID = assignment.PoolID, assignment.PoolRunnerID, assignment.PoolSlotID
			ref.PoolGrantID, ref.PoolGrantVersion = assignment.PoolGrantID, assignment.PoolGrantVersion
			ref.PoolCredentialID, ref.PoolCredentialVersion = assignment.PoolCredentialID, assignment.PoolCredentialVersion
			ref.DispatchID, ref.DispatchState, ref.LeaseUntil = assignment.DispatchID, assignment.DispatchState, assignment.LeaseUntil
		}
		bundleURL := "/api/v1/" + namespace + "/repository-source-intakes/" + intake.ID + "/bundle?project_id=" + f.project
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
			denied := w.Code == status || status == 0 && (w.Code == http.StatusForbidden || w.Code == http.StatusConflict)
			if !denied || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") || w.Header().Get("Content-Disposition") != "" || bytes.Contains(w.Body.Bytes(), []byte("PACK")) {
				t.Fatalf("unexpected bundle denial: status=%d, want %d, content-type=%q, response bytes=%d", w.Code, status, w.Header().Get("Content-Type"), w.Body.Len())
			}
		}
		assertBundleDenied(independent.Token, ref, http.StatusForbidden)
		stale := ref
		stale.Fence++
		if pooled {
			assertBundleDenied(producer.Token, stale, 0)
			wrongSlot := ref
			wrongSlot.PoolSlotID = "1"
			assertBundleDenied(producer.Token, wrongSlot, 0)
			wrongGrant := ref
			wrongGrant.PoolGrantID = newID()
			assertBundleDenied(producer.Token, wrongGrant, 0)
			wrongGrantVersion := ref
			wrongGrantVersion.PoolGrantVersion++
			assertBundleDenied(producer.Token, wrongGrantVersion, 0)
			// Simulate current-grant drift while preserving the original signed
			// assignment. A still-approved grant row cannot revive the old version.
			if _, err = f.s.DB.ExecContext(ctx, `UPDATE suma_execution_pool_grants SET version=version+1 WHERE id=?`, ref.PoolGrantID); err != nil {
				t.Fatal(err)
			}
			assertBundleDenied(producer.Token, ref, 0)
			if _, err = f.s.DB.ExecContext(ctx, `UPDATE suma_execution_pool_grants SET version=? WHERE id=?`, ref.PoolGrantVersion, ref.PoolGrantID); err != nil {
				t.Fatal(err)
			}
		} else {
			assertBundleDenied(producer.Token, stale, http.StatusConflict)
		}

		beforeTransfers := domainACount(t, f.s, "suma_candidate_transfers")
		forged, err := json.Marshal(cc.StartRequest{
			ProjectID: f.project, TenantID: ref.TenantID, RunnerID: ref.PoolRunnerID, Assignment: ref, OperationID: newID(), BaseCommit: base, BundleBaseCommit: base, HeadCommit: head,
			BundleSHA256: digest("wrong repository intake bytes"), SizeBytes: intake.SizeBytes, EvidenceSHA256: digest("forged evidence"),
		})
		if err != nil {
			t.Fatal(err)
		}
		startTransfer := httptest.NewRequest(http.MethodPost, "/api/v1/"+namespace+"/candidate-transfers", bytes.NewReader(forged))
		startTransfer.Header.Set("Authorization", "Bearer "+producer.Token)
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
		if err = producerCfg.Artifacts.Complete(ctx, withoutSeal); err == nil || !strings.Contains(err.Error(), "HTTP 409") {
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
		if err = producerCfg.Artifacts.Complete(ctx, produced); err != nil {
			t.Fatal(err)
		}
		current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{
			"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Fixture rejects Review before independent tests",
		}), 400)

		check, err := claim(independent, testProfile, independentCommand, "1")
		if err != nil || check == nil || check.RunID != run.ID || check.NodeID != "repository-test" || check.CandidateCommit != head || check.WorkerID == assignment.WorkerID {
			t.Fatal("independent Runner did not claim the approved commit", err)
		}
		independentCfg.Artifacts = forAssignment(independent, check)
		if pooled && (assignment.PoolSlotID != "0" || check.PoolSlotID != "1" || check.PoolRunnerID == assignment.PoolRunnerID || check.PoolCredentialID == assignment.PoolCredentialID || check.DispatchID == assignment.DispatchID) {
			t.Fatal("producer and independent test did not use distinct live Pool identities, slots and dispatches")
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
		if err = independentCfg.Artifacts.Complete(ctx, tested); err != nil {
			t.Fatal(err)
		}
	}
	qualityRun := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+quality.ID+"/executions/"+run.ID+"?project_id="+f.project, nil, 200))
	if qualityRun.Passed != 1 {
		t.Fatal("published plan did not receive independent JUnit", qualityRun)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
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
	if pooled {
		assertActualRepositoryDeliveryIntegration(t, ctx, f, control.URL, git, producerRepo, accepted, intake, pool, grant)
	}
}
