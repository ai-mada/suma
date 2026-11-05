package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	rp "github.com/ai-mada/suma/internal/repositoryprovider"
	"github.com/ai-mada/suma/internal/runner"
)

// Continue the same tested and accepted Pool delivery through the managed Git
// integration. The target begins at the baseline and can acquire the candidate
// only by importing the accepted sealed bundle from the control plane.
func assertActualRepositoryDeliveryIntegration(t *testing.T, ctx context.Context, f *fixture, controlURL, git, baselineRepo string, accepted Record, intake TaskDeliveryRepositoryIntake, pool ExecutionPool, grant ExecutionPoolGrant) {
	t.Helper()
	const providerID, alias, targetRef = "repository-delivery-target248", "main", "refs/heads/main"
	const controlEnv, runnerEnv, reconcileEnv, sourceReadEnv = "SUMA_TEST_DELIVERY248_CONTROL", "SUMA_TEST_DELIVERY248_RUNNER", "SUMA_TEST_DELIVERY248_RECONCILE", "SUMA_TEST_DELIVERY249_SOURCE"
	// The incoming provider already has its own callback credential. Control
	// authenticates this destination as a separate provider identity.
	t.Setenv(controlEnv, strings.Repeat("d", 48))
	t.Setenv(runnerEnv, strings.Repeat("u", 48))
	t.Setenv(reconcileEnv, strings.Repeat("y", 48))
	t.Setenv(sourceReadEnv, strings.Repeat("z", 48))
	dir := t.TempDir()
	bare := filepath.Join(dir, "target.git")
	workspaceGitTestCommand(t, baselineRepo, "clone", "--bare", "--no-hardlinks", "--", baselineRepo, bare)
	assertTarget := func(want string) {
		t.Helper()
		if actual := workspaceGitTestCommand(t, bare, "rev-parse", targetRef); actual != want {
			t.Fatalf("managed Git target is %s, want %s", actual, want)
		}
	}
	assertTarget(intake.BaseCommit)
	provider, err := rp.New(rp.Config{
		ProviderID: providerID, TenantID: f.user.TenantID, ProjectID: f.project,
		ControlURL: controlURL, ControlTokenEnv: controlEnv, RunnerTokenEnv: runnerEnv, ReconcileTokenEnv: reconcileEnv, SourceReadTokenEnv: sourceReadEnv,
		GitExecutable: git, StateDir: filepath.Join(dir, "provider"),
		Targets: map[string]rp.TargetConfig{alias: {BareRepository: bare, Ref: targetRef}},
	})
	if err != nil {
		t.Fatal("start real managed Git provider", err)
	}
	defer provider.Close()
	server := httptest.NewServer(provider.Handler())
	defer server.Close()
	f.s.RepositoryProviders = append(f.s.RepositoryProviders, RepositoryProviderConfig{
		ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, TargetAliases: []string{alias},
		Origin: server.URL, TokenHash: digest(os.Getenv(controlEnv)), ReconcileToken: os.Getenv(reconcileEnv), SourceReadToken: os.Getenv(sourceReadEnv),
	})
	target := issueRepairRuntimeDecode[RepositoryIntegrationTarget](t, f.call("POST", "/repository-integration-targets",
		RepositoryIntegrationTargetCreateRequest{ProjectID: f.project, ProviderID: providerID, TargetAlias: alias, Name: "Governed isolated target", IdempotencyKey: newID()}, http.StatusCreated))
	options := issueRepairRuntimeDecode[RepositoryIntegrationOptions](t, f.call("GET",
		"/repository-integrations/options?project_id="+f.project+"&task_id="+accepted.ID, nil, http.StatusOK))
	if !options.CanCreate || options.TaskVersion != accepted.Version || len(options.Sources) != 1 || options.Sources[0].SourceNodeID != "repository-source" ||
		!options.Sources[0].Eligible || options.Sources[0].Source.Source.RunID != accepted.RunID ||
		options.Sources[0].Source.HeadCommit != intake.HeadCommit || options.Sources[0].Source.BundleSHA256 != intake.BundleSHA256 {
		t.Fatalf("managed integration did not offer the exact independently accepted candidate: %+v", options)
	}
	create := RepositoryIntegrationPlanCreateRequest{
		ProjectID: f.project, TaskID: accepted.ID, ExpectedTaskVersion: accepted.Version, SourceNodeID: options.Sources[0].SourceNodeID,
		TargetID: target.ID, ExpectedTargetVersion: target.Version, IdempotencyKey: newID(),
	}
	plan := issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, f.call("POST", "/repository-integrations/plans", create, http.StatusCreated))
	if plan.Basis.RunID != accepted.RunID || plan.Basis.Source != options.Sources[0].Source || plan.TargetID == intake.TargetID {
		t.Fatal("prepare did not bind the accepted candidate and the separate managed target")
	}
	path := "/repository-integrations/plans/" + plan.ID
	read := func() RepositoryIntegrationPlan {
		t.Helper()
		return issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, f.call("GET", path+"?project_id="+f.project, nil, http.StatusOK))
	}
	command := func(version int, action string, status int) RepositoryIntegrationPlan {
		t.Helper()
		body := f.call("POST", path+"/commands", RepositoryIntegrationPlanCommandRequest{
			ProjectID: f.project, ExpectedVersion: version, Action: action, Acknowledge: true,
			Reason: "Fixture owner explicitly reviews the isolated Git target", IdempotencyKey: newID(),
		}, status)
		if status != http.StatusOK {
			return RepositoryIntegrationPlan{}
		}
		return issueRepairRuntimeDecode[RepositoryIntegrationPlan](t, body)
	}
	credential, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{
		Name: "Governed repository Pool Runner", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	machine := credential.Runner
	client := runner.Client{BaseURL: controlURL, Token: credential.Token, ProjectID: f.project,
		Pool: &runner.PoolContext{PoolID: pool.ID, RunnerID: machine.ID}}
	execute := func(phase, runID string) (runner.Result, *runner.Client) {
		t.Helper()
		a, err := client.ClaimPool(ctx, runner.PoolClaimRequest{
			PoolID: pool.ID, RunnerID: machine.ID, WorkerID: machine.WorkerID, SlotID: "0", OperationID: newID(),
			Scopes: []runner.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"external_workflow:" + providerID}}},
		})
		if err != nil || a == nil || a.RunID != runID || a.RepositoryIntegration == nil || a.RepositoryIntegration.Phase != phase ||
			a.PoolGrantID != grant.ID || a.PoolGrantVersion != grant.Version || a.RepositoryIntegration.Source != plan.Basis.Source {
			t.Fatalf("missing candidate-bound Pool assignment for %s: %+v %v", phase, a, err)
		}
		scoped := client
		identity := *client.Pool
		identity.Assignment = a
		scoped.Pool = &identity
		result := runner.Execute(ctx, *a, runner.ExecConfig{
			AllowExec: true, Artifacts: &scoped,
			External: map[string]runner.ExternalSpec{providerID: {URL: server.URL, TokenEnv: runnerEnv, Protocol: ri.RepositoryProtocol}},
		})
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.IntegrationEvidence == nil ||
			result.IntegrationEvidence.OperationID != a.RepositoryIntegration.OperationID {
			// The provider's persisted reason can distinguish missing acknowledgement
			// from an actual Git failure without retrying the original effect.
			var reason string
			if evidence := result.IntegrationEvidence; evidence != nil {
				var raw string
				if err := f.s.DB.QueryRowContext(ctx, f.s.q(`SELECT data FROM suma_repository_observations WHERE tenant_id=? AND project_id=? AND operation_id=? AND version=?`), f.user.TenantID, f.project, evidence.OperationID, evidence.Version).Scan(&raw); err == nil {
					var observation ri.RepositoryObservation
					if json.Unmarshal([]byte(raw), &observation) == nil {
						reason = observation.Reason
					}
				}
			}
			t.Fatalf("real provider %s failed: exit=%d error=%q output=%q provider_reason=%q", phase, result.ExitCode, result.Error, result.Output, reason)
		}
		result.OperationID = newID()
		return result, &scoped
	}
	prepared, prepareClient := execute("prepare", plan.PrepareRunID)
	if err = prepareClient.Complete(ctx, prepared); err != nil {
		t.Fatal("complete real prepare", err)
	}
	plan = read()
	if plan.State != "ready" || plan.CanExecute || plan.ApplyRunID != "" || plan.Preview == nil || !plan.Preview.FastForward ||
		plan.Preview.TargetHead != intake.BaseCommit || plan.Preview.CandidateHead != intake.HeadCommit || plan.Preview.SourceBundleSHA256 != intake.BundleSHA256 ||
		len(plan.Preview.Files) != 1 || plan.Preview.Files[0].Path != "sum.cjs" {
		t.Fatalf("prepare did not preserve the actual fast-forward diff and human gate: %+v", plan)
	}
	command(plan.Version, "execute", http.StatusConflict)
	assertTarget(intake.BaseCommit)
	readyVersion := plan.Version
	plan = command(readyVersion, "approve", http.StatusOK)
	if plan.State != "approved" || plan.Approval == nil || plan.Approval.PlanVersion != readyVersion ||
		plan.Approval.PreparedSHA256 != plan.PreparedSHA256 || plan.Approval.BasisSHA256 != plan.Basis.BasisSHA256 {
		t.Fatal("approval did not freeze the original tested source and reviewed preview")
	}
	command(readyVersion, "execute", http.StatusConflict)
	assertTarget(intake.BaseCommit)
	plan = command(plan.Version, "execute", http.StatusOK)
	assertTarget(intake.BaseCommit)
	applied, applyClient := execute("apply", plan.ApplyRunID)
	// A real prepare receipt cannot stand in for the apply operation, even
	// after Git performed the authorized effect. The genuine retry must win.
	forged := applied
	forged.IntegrationEvidence = prepared.IntegrationEvidence
	if err = applyClient.Complete(ctx, forged); err == nil {
		t.Fatal("prepare receipt completed the wrong apply operation")
	}
	if pending := read(); pending.State != "applying" {
		t.Fatal("wrong operation receipt advanced the plan", pending.State)
	}
	if err = applyClient.Complete(ctx, applied); err != nil {
		t.Fatal("complete original real apply", err)
	}
	plan = read()
	if plan.State != "integrated" || plan.Observation == nil || plan.Observation.State != "applied" || plan.Observation.Receipt == nil ||
		plan.Observation.Receipt.OperationID != applied.IntegrationEvidence.OperationID || plan.Observation.Receipt.BeforeCommit != intake.BaseCommit ||
		plan.Observation.Receipt.ResultCommit != intake.HeadCommit || plan.Observation.Receipt.SourceBundleSHA256 != intake.BundleSHA256 ||
		plan.Observation.Receipt.PreparedSHA256 != plan.PreparedSHA256 {
		t.Fatal("original operation lacks a persisted receipt for the exact tested commit", plan.State)
	}
	assertTarget(intake.HeadCommit)
	ref := "refs/suma/repository/receipts/" + digest(plan.Observation.OperationID)
	if oid := workspaceGitTestCommand(t, bare, "rev-parse", ref); oid != plan.Observation.ReceiptOID {
		t.Fatal("provider observation differs from the durable Git receipt reference", oid)
	}
	var receipt ri.RepositoryReceipt
	if err = json.Unmarshal([]byte(workspaceGitTestCommand(t, bare, "cat-file", "blob", ref)), &receipt); err != nil || receipt != *plan.Observation.Receipt {
		t.Fatal("provider observation differs from the durable Git receipt blob", err)
	}
	if err = applyClient.Complete(ctx, applied); err != nil {
		t.Fatal("replay original apply acknowledgement", err)
	}
	if replay := read(); replay.Version != plan.Version || replay.State != "integrated" || replay.Observation == nil || replay.Observation.ReceiptOID != plan.Observation.ReceiptOID {
		t.Fatal("replaying the original result changed the integrated plan or receipt")
	}
	t.Run("application_source_handoff", func(t *testing.T) {
		applicationFixture := *f
		applicationFixture.t = t
		assertActualPoolApplicationVersion(t, ctx, &applicationFixture, bare, accepted, plan)
	})
}

// Continue from the applied Pool receipt through the public application APIs.
// The real provider and bundled inspector prove the commit and tree used to
// register the immutable version. A catalog entry is only a source record.
func assertActualPoolApplicationVersion(t *testing.T, ctx context.Context, f *fixture, bare string, accepted Record, plan RepositoryIntegrationPlan) {
	t.Helper()
	entry, err := filepath.Abs(filepath.Join("..", "..", "packages", "source-inspector", "dist", "source-inspector.bundle.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry); os.IsNotExist(err) {
		t.Skip("build the Suma source inspector bundle to verify the application handoff")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	f.s.ApplicationSourceInspector, err = LoadApplicationSourceInspector(func(key string) string {
		return map[string]string{"SUMA_SOURCE_INSPECTOR_NODE": node, "SUMA_SOURCE_INSPECTOR_ENTRY": entry}[key]
	})
	if err != nil {
		t.Fatal("load the pinned source inspector", err)
	}

	targets, err := f.s.ListRepositoryIntegrationTargets(ctx, f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	var target RepositoryIntegrationTarget
	for _, value := range targets {
		if value.ID == plan.TargetID {
			target = value
			break
		}
	}
	if target.ID == "" {
		t.Fatal("the applied target is missing from the project")
	}
	created := issueRepairRuntimeDecode[ProjectApplicationView](t, f.call("POST", "/applications", ApplicationWriteRequest{
		ProjectID: f.project, Key: "pool-delivery", Name: "Pool delivery application", Kind: "application",
		OwnerTeamID: applicationTeamFixture(t, f), State: "active",
		CodeScopes: []ApplicationCodeScopeInput{{TargetID: target.ID, ExpectedTargetVersion: target.Version}},
		Reason:     "Assign the governed repository root to this application", IdempotencyKey: newID(),
	}, http.StatusCreated))
	app := created.Application
	if created.RegisteredVersionCount != 0 || app.CurrentRegisteredVersionID != "" || len(app.CodeScopes) != 1 {
		t.Fatal("creating the application unexpectedly registered a version", created)
	}

	versionsPath := "/applications/" + app.ID + "/versions"
	options := issueRepairRuntimeDecode[ApplicationVersionOptions](t, f.call("GET", versionsPath+"/options?project_id="+f.project, nil, http.StatusOK))
	commit := plan.Observation.Receipt.ResultCommit
	expectedOption := ApplicationIntegrationOption{ID: plan.ID, Version: plan.Version, TaskID: accepted.ID, TargetID: target.ID, Commit: commit, State: "integrated"}
	if len(options.Integrations) != 1 || options.Integrations[0] != expectedOption {
		t.Fatal("version options did not offer the applied candidate and receipt plan", options.Integrations)
	}
	input := ApplicationVersionPreviewRequest{
		ProjectID: f.project, ExpectedApplicationVersion: app.Version, Label: "pool-delivery-1",
		Sources:      []ApplicationVersionSourceInput{{ScopeID: app.CodeScopes[0].ID, IntegrationPlanID: plan.ID, ExpectedIntegrationVersion: plan.Version - 1}},
		Dependencies: []ApplicationDependencyInput{}, Reason: "Review the same tested and accepted Pool commit",
	}
	f.call("POST", versionsPath+"/preview", input, http.StatusConflict)
	input.Sources[0].ExpectedIntegrationVersion = plan.Version
	blocked := issueRepairRuntimeDecode[ApplicationVersionPreview](t, f.call("POST", versionsPath+"/preview", input, http.StatusOK))
	if blocked.CanRegister || len(blocked.Blockers) != 1 || blocked.Blockers[0] != "application_dependency_gaps" {
		t.Fatal("the manifest-free fixture should require explicit dependency-gap acknowledgement", blocked.Blockers)
	}
	input.AcknowledgeDependencyGaps = true
	preview := issueRepairRuntimeDecode[ApplicationVersionPreview](t, f.call("POST", versionsPath+"/preview", input, http.StatusOK))
	if !preview.CanRegister || preview.Version.DependencyCoverageComplete || len(preview.Version.Sources) != 1 {
		t.Fatal("the version preview did not retain the acknowledged source-only scope", preview.Blockers)
	}
	source := preview.Version.Sources[0]
	tree := workspaceGitTestCommand(t, bare, "rev-parse", commit+"^{tree}")
	if source.Commit != commit || source.Inspection.Commit != commit || source.Inspection.TreeOID != tree || source.Inspection.ScopeTreeOID != tree ||
		source.Scope != app.CodeScopes[0] || source.OperationID != plan.OperationID || source.ReceiptSHA256 != plan.Observation.ReceiptSHA256 ||
		source.ReceiptOID != plan.Observation.ReceiptOID || source.TaskBasis != plan.Basis.AcceptedTaskBasis || source.TaskBasis.RunID != accepted.RunID ||
		source.SourceNodeID != plan.Basis.SourceNodeID || source.Inspection.Coverage.Reason != "no_supported_manifests" {
		t.Fatal("application preview substituted the accepted run, receipt, commit, or Git tree", source)
	}
	list := issueRepairRuntimeDecode[ApplicationVersionPage](t, f.call("GET", versionsPath+"?project_id="+f.project, nil, http.StatusOK))
	if list.Total != 0 {
		t.Fatal("preview registered a version without explicit confirmation")
	}

	request := ApplicationVersionRegisterRequest{ApplicationVersionPreviewRequest: input, ExpectedPreviewSHA256: preview.PreviewSHA256, IdempotencyKey: newID()}
	registered := issueRepairRuntimeDecode[ApplicationVersionView](t, f.call("POST", versionsPath, request, http.StatusCreated))
	if !registered.BasisCurrent || registered.Version.Sequence != 1 || !reflect.DeepEqual(registered.Version.Sources, preview.Version.Sources) ||
		registered.Version.RegisteredPreviewSHA256 != preview.PreviewSHA256 || !registered.Version.DependencyGapsAcknowledged {
		t.Fatal("registration did not preserve the explicitly reviewed source snapshot", registered)
	}
	replayed := issueRepairRuntimeDecode[ApplicationVersionView](t, f.call("POST", versionsPath, request, http.StatusCreated))
	current := issueRepairRuntimeDecode[ProjectApplicationView](t, f.call("GET", "/applications/"+app.ID+"?project_id="+f.project, nil, http.StatusOK))
	if replayed.Version.ID != registered.Version.ID || replayed.Version.SnapshotSHA256 != registered.Version.SnapshotSHA256 ||
		current.RegisteredVersionCount != 1 || current.Application.CurrentRegisteredVersionID != registered.Version.ID {
		t.Fatal("registration replay changed the original version or registered a second version")
	}
	for _, table := range []string{"suma_release_candidates", "suma_application_release_sets", "suma_release_plans", "suma_release_deployments"} {
		if domainACount(t, f.s, table) != 0 {
			t.Fatalf("registering source metadata wrote %s", table)
		}
	}
	t.Logf("application_source_handoff app=%s version=%s run=%s review=%s integration=%s operation=%s receipt_sha256=%s commit=%s tree=%s snapshot_sha256=%s",
		app.ID, registered.Version.ID, accepted.RunID, source.TaskBasis.ReviewID, plan.ID, source.OperationID, source.ReceiptSHA256, commit, tree, registered.Version.SnapshotSHA256)
}
