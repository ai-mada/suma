package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	const controlEnv, runnerEnv, reconcileEnv = "SUMA_TEST_DELIVERY248_CONTROL", "SUMA_TEST_DELIVERY248_RUNNER", "SUMA_TEST_DELIVERY248_RECONCILE"
	// The incoming provider already has its own callback credential. Control
	// authenticates this destination as a separate provider identity.
	t.Setenv(controlEnv, strings.Repeat("d", 48))
	t.Setenv(runnerEnv, strings.Repeat("u", 48))
	t.Setenv(reconcileEnv, strings.Repeat("y", 48))
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
		ControlURL: controlURL, ControlTokenEnv: controlEnv, RunnerTokenEnv: runnerEnv, ReconcileTokenEnv: reconcileEnv,
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
		Origin: server.URL, TokenHash: digest(os.Getenv(controlEnv)), ReconcileToken: os.Getenv(reconcileEnv),
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
			t.Fatalf("real provider %s failed: exit=%d error=%q output=%q", phase, result.ExitCode, result.Error, result.Output)
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
}
