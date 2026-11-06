package platform

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/deployer"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
)

// The repair and its intervening candidate run in the project's default Pool.
// Deployments use a dedicated Runner and isolated loopback static provider.
type repairDeploymentFixture struct {
	f            *fixture
	env          ReleaseEnvironment
	client       *runner.Client
	poolClient   *runner.Client
	poolWorkerID string
	config       runner.ExecConfig
	provider     *deployer.Server
}

func testProjectExecutionRoute(t *testing.T, f *fixture) dag.ProjectExecutionRoute {
	t.Helper()
	ctx := context.Background()
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	route, err := f.e.ReadProjectExecutionRouteTx(ctx, tx, f.user.TenantID, f.project)
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func assertDedicatedReleaseRun(t *testing.T, ctx context.Context, f *fixture, id string) {
	t.Helper()
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, id)
	if err != nil {
		t.Fatal(err)
	}
	if run.ExecutionPoolID != "" || run.ExecutionPoolGrantID != "" || run.ExecutionPoolGrantVersion != 0 {
		t.Fatalf("managed run %s adopted an ordinary Pool route: pool=%q grant=%q", id, run.ExecutionPoolID, run.ExecutionPoolGrantID)
	}
}

func assertRepairReleaseSource(t *testing.T, record map[string]any, handoff IssueRepairReleaseHandoff, candidate string) {
	t.Helper()
	source, ok := record["repair_source"].(map[string]any)
	if !ok || source["issue_id"] != handoff.IssueID || source["candidate_id"] != candidate || number(source, "cycle") != handoff.Cycle {
		t.Fatal("release detail lost its exact historical repair source", source)
	}
}

func newRepairDeploymentFixture(t *testing.T, f *fixture, controlURL string, poolClient *runner.Client, poolWorkerID string) *repairDeploymentFixture {
	t.Helper()
	const providerID, workerID, siteID = "local-static-test", "repair-provider-worker", "fixture-site"
	const providerEnv, workerEnv = "SUMA_REPAIR_FIXTURE_PROVIDER_TOKEN", "SUMA_REPAIR_FIXTURE_WORKER_TOKEN"
	const reconcileEnv = "SUMA_REPAIR_FIXTURE_RECONCILE_TOKEN"
	providerToken, workerToken, reconcileToken := "provider-"+newID(), "transport-"+newID(), "reconcile-"+newID()
	t.Setenv(providerEnv, providerToken)
	t.Setenv(workerEnv, workerToken)
	t.Setenv(reconcileEnv, reconcileToken)
	provider, err := deployer.New(deployer.Config{
		ProviderID: providerID, Root: filepath.Join(t.TempDir(), "repair-static-provider"),
		ControlURL: controlURL, ControlTokenEnv: providerEnv, ReconcileTokenEnv: reconcileEnv,
		Workers: map[string]string{workerID: workerEnv},
		Sites:   map[string]deployer.SiteConfig{siteID: {Address: "127.0.0.1:0"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Close(); err != nil {
			t.Error(err)
		}
	})
	management := httptest.NewServer(provider.Handler())
	t.Cleanup(management.Close)
	f.s.ReleaseProviders = []ReleaseProviderConfig{{
		ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project,
		TokenHash: digest(providerToken), SiteIDs: []string{siteID}, Origin: management.URL, ReconcileToken: reconcileToken,
	}}
	registered, err := f.s.RegisterReleaseWorker(context.Background(), f.actor, f.project, map[string]any{
		"worker_id": workerID, "profiles": []any{}, "deployment": true, "idempotency_key": newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &runner.Client{BaseURL: controlURL, Token: registered["token"].(string), ProjectID: f.project}
	return &repairDeploymentFixture{
		f: f, env: releaseTestEnvironment(t, f), client: client, provider: provider,
		poolClient: poolClient, poolWorkerID: poolWorkerID,
		config: runner.ExecConfig{Artifacts: client, External: map[string]runner.ExternalSpec{
			providerID: {URL: management.URL, TokenEnv: workerEnv, Protocol: rc.ReleaseProtocol, PollMilliseconds: 50},
		}},
	}
}

func (h *repairDeploymentFixture) forwardInput(candidate string, generation int64) map[string]any {
	return map[string]any{
		"project_id": h.f.project, "candidate_id": candidate, "environment_id": h.env.ID,
		"action": "deploy", "expected_environment_generation": generation,
	}
}

func (h *repairDeploymentFixture) approve(t *testing.T, input map[string]any, previewSHA string) ReleasePlan {
	t.Helper()
	if previewSHA == "" {
		preview := issueRepairRuntimeDecode[ReleasePlanPreview](t, h.f.call("POST", "/release-plans/preview", input, 200))
		if preview.ExecutionLane != releaseExecutionLane {
			t.Fatal("managed preview did not disclose the dedicated project Runner lane", preview.ExecutionLane)
		}
		previewSHA = preview.PreviewSHA256
	}
	input["preview_sha256"] = previewSHA
	input["idempotency_key"] = newID()
	plan := issueRepairRuntimeDecode[ReleasePlan](t, h.f.call("POST", "/release-plans", input, 201))
	return issueRepairRuntimeDecode[ReleasePlan](t, h.f.call("POST", "/release-plans/"+plan.ID+"/approve", h.f.body(map[string]any{
		"expected_version": plan.Version, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256,
		"decision": "approved", "reason": "Approve the exact isolated repair deployment plan.",
	}), 200))
}

func (h *repairDeploymentFixture) deploy(t *testing.T, plan ReleasePlan, wantBody string) ReleaseDeployment {
	t.Helper()
	if plan.ExecutionLane != releaseExecutionLane {
		t.Fatal("managed plan did not retain the dedicated project Runner lane", plan.ExecutionLane)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	queued := issueRepairRuntimeDecode[ReleaseDeployment](t, h.f.call("POST", "/release-plans/"+plan.ID+"/execute", releaseExecuteBody(h.f, plan), 201))
	assertDedicatedReleaseRun(t, ctx, h.f, queued.RunID)
	capabilities := runner.ExternalCapabilities(h.config.External)
	pool := h.poolClient.Pool
	poolAssignment, err := h.poolClient.ClaimPool(ctx, runner.PoolClaimRequest{
		PoolID: pool.PoolID, RunnerID: pool.RunnerID, WorkerID: h.poolWorkerID, SlotID: "0", OperationID: newID(),
		Scopes: []runner.PoolCapabilityScope{{TenantID: h.f.user.TenantID, ProjectID: h.f.project, Capabilities: capabilities}},
	})
	if err != nil || poolAssignment != nil {
		t.Fatalf("authenticated Pool claim must not pick up the queued dedicated deployment: assignment=%+v error=%v", poolAssignment, err)
	}
	assignment, err := h.client.Claim(ctx, "repair-provider-worker", capabilities)
	if err != nil || assignment == nil || assignment.ReleaseExecution == nil || assignment.RunID != queued.RunID ||
		assignment.ReleaseExecution.OperationID != queued.OperationID || assignment.ReleaseExecution.ArtifactSHA256 != plan.Content.ArtifactSHA256 {
		t.Fatalf("claim did not return the managed deployment for the exact plan: %v", err)
	}
	result := runner.Execute(ctx, *assignment, h.config)
	if result.ExitCode != 0 || result.ReleaseEvidence == nil || result.OutcomeUnknown {
		t.Fatalf("real release Runner did not confirm healthy sealed content: %s", result.Error)
	}
	result.OperationID = newID()
	if err = h.client.Complete(ctx, result); err != nil {
		t.Fatal(err)
	}
	deployment := issueRepairRuntimeDecode[ReleaseDeployment](t, h.f.call("GET", "/deployments/"+queued.OperationID+"?project_id="+h.f.project, nil, 200))
	if deployment.State != "deployed_healthy" || deployment.Observation == nil ||
		deployment.Observation.EvidenceSHA256 != result.ReleaseEvidence.EvidenceSHA256 ||
		deployment.ArtifactID != plan.Content.ArtifactID || deployment.ArtifactSHA256 != plan.Content.ArtifactSHA256 {
		t.Fatal("control plane did not retain the exact healthy provider observation", deployment)
	}
	env := issueRepairRuntimeDecode[ReleaseEnvironment](t, h.f.call("GET", "/environments/"+h.env.ID+"?project_id="+h.f.project, nil, 200))
	if env.Generation != plan.Content.Generation || env.ActiveGeneration != env.Generation ||
		env.ActiveArtifactID != plan.Content.ArtifactID || env.ActiveArtifactSHA256 != plan.Content.ArtifactSHA256 || env.BarrierOperationID != "" {
		t.Fatal("deployment did not activate the exact generation and release its barrier", env)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.provider.SiteURL("fixture-site")+"/index.html", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != wantBody {
		t.Fatal("real site did not serve the expected sealed bytes", response.StatusCode, err)
	}
	return deployment
}
