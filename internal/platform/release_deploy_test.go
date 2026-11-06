package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
	"github.com/ai-mada/suma/internal/store"
)

func releaseTestSetup(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t)
	if err := f.s.InitializeArtifacts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeReleaseDeployment(context.Background()); err != nil {
		t.Fatal(err)
	}
	secret := "provider-test-" + newID()
	f.s.ReleaseProviders = []ReleaseProviderConfig{{ID: "local-static-test", TenantID: f.user.TenantID, ProjectID: f.project, TokenHash: digest(secret), SiteIDs: []string{"fixture-site"}}}
	return f, secret
}
func releaseWantStatus(t *testing.T, err error, want int) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != want {
		t.Fatalf("got %v, want HTTP %d", err, want)
	}
}
func releaseTestEnvironment(t *testing.T, f *fixture) ReleaseEnvironment {
	t.Helper()
	out, err := f.s.SaveReleaseEnvironment(context.Background(), f.actor, f.project, "", map[string]any{"project_id": f.project, "name": "实际本地环境", "provider_id": "local-static-test", "site_id": "fixture-site", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestReleaseDeploymentEnvironmentAuthorityAndHistory(t *testing.T) {
	f, secret := releaseTestSetup(t)
	ctx := context.Background()
	ownerToken := f.cookie.Value
	ownerCookie := f.cookie
	env := releaseTestEnvironment(t, f)
	options, err := f.s.ReleaseProviderOptions(ctx, f.actor, f.project)
	if err != nil || len(options) != 1 {
		t.Fatal("registered options missing", err)
	}
	raw, _ := json.Marshal(options)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), digest(secret)) {
		t.Fatal("provider options leaked credential material")
	}
	_, err = f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, ownerToken), f.project, "", map[string]any{"project_id": f.project, "name": "冒用", "provider_id": "local-static-test", "site_id": "fixture-site", "idempotency_key": newID()})
	releaseWantStatus(t, err, 409)
	_, err = f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, ownerToken), f.project, env.ID, map[string]any{"project_id": f.project, "name": "错误版本", "expected_version": 1.5, "idempotency_key": newID()})
	releaseWantStatus(t, err, 400)
	worker, err := f.s.RegisterReleaseWorker(ctx, domainCActor(t, f.s, ownerToken), f.project, map[string]any{"worker_id": "deployer-test", "profiles": []any{}, "deployment": true, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, worker["token"].(string)), f.project, env.ID, map[string]any{"project_id": f.project, "name": "Runner不能配置环境", "expected_version": 1, "idempotency_key": newID()})
	releaseWantStatus(t, err, 403)
	inv := governanceInvite(f, "release-reviewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
	accepted := governanceAccept(f, inv, "release-reviewer@example.test")
	var reviewer Principal
	raw, _ = json.Marshal(accepted["user"])
	json.Unmarshal(raw, &reviewer)
	_, err = f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, f.cookie.Value), f.project, env.ID, map[string]any{"project_id": f.project, "name": "reviewer不能配置", "expected_version": 1, "idempotency_key": newID()})
	releaseWantStatus(t, err, 403)
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": reviewer.ID, "role": "admin", "expected_version": float64(1), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	// A real invited project admin has member org role and must still be allowed.
	_, reviewerToken, err := f.s.Login(ctx, "release-reviewer@example.test", "invited-password-strong-42")
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"project_id": f.project, "name": "项目管理员配置", "expected_version": 1, "idempotency_key": newID()}
	changed, err := f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, reviewerToken), f.project, env.ID, body)
	if err != nil || changed.Version != 2 {
		t.Fatal("project admin rejected", err)
	}
	_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": reviewer.ID, "role": "viewer", "expected_version": float64(2), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, reviewerToken), f.project, env.ID, body)
	releaseWantStatus(t, err, 403)
	f.s.ReleaseProviders[0].SiteIDs = []string{"another-site"}
	_, err = f.s.PreviewReleasePlan(ctx, domainCActor(t, f.s, ownerToken), f.project, map[string]any{"project_id": f.project, "candidate_id": "not-a-real-candidate", "environment_id": env.ID, "action": "deploy", "expected_environment_generation": 0})
	releaseWantStatus(t, err, 503)
	f.s.ReleaseProviders[0].SiteIDs = []string{"fixture-site"}
	archived, err := f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, ownerToken), f.project, env.ID, map[string]any{"project_id": f.project, "status": "archived", "expected_version": 2, "idempotency_key": newID()})
	if err != nil || archived.Generation != 0 || archived.PolicyVersion != 3 {
		t.Fatal("archive reset historical identity", err)
	}
	_, err = f.s.SaveReleaseEnvironment(ctx, domainCActor(t, f.s, ownerToken), f.project, "", map[string]any{"project_id": f.project, "name": "不能重建重置代数", "provider_id": "local-static-test", "site_id": "fixture-site", "idempotency_key": newID()})
	releaseWantStatus(t, err, 409)
}
func TestReleaseDeploymentEnvironmentTwoConnectionCAS(t *testing.T) {
	f, _ := releaseTestSetup(t)
	env := releaseTestEnvironment(t, f)
	var path string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	second := &Service{DB: db, Dialect: "sqlite", DataDir: f.s.DataDir, ReleaseProviders: f.s.ReleaseProviders}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, service := range []*Service{f.s, second} {
		wg.Add(1)
		go func(i int, s *Service) {
			defer wg.Done()
			<-start
			_, e := s.SaveReleaseEnvironment(context.Background(), f.actor, f.project, env.ID, map[string]any{"project_id": f.project, "name": "并发更新", "expected_version": 1, "idempotency_key": newID()})
			errs <- e
		}(i, service)
	}
	close(start)
	wg.Wait()
	close(errs)
	ok, conflicts := 0, 0
	for e := range errs {
		if e == nil {
			ok++
		} else {
			var api *APIError
			if errors.As(e, &api) && api.Status == 409 {
				conflicts++
			} else {
				t.Fatal(e)
			}
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatalf("CAS winners=%d conflicts=%d", ok, conflicts)
	}
}
func TestReleaseDeploymentObservationRequiresActualBoundFacts(t *testing.T) {
	d := ReleaseDeployment{OperationID: "deploy-observation-1", PlanID: "plan", PlanRevision: 1, PlanSHA256: strings.Repeat("a", 64), EnvironmentID: "environment", ProviderID: "provider", Generation: 1, ArtifactSHA256: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("c", 64)}
	p := ReleasePlan{Content: rc.ReleasePlanContent{Health: rc.HealthPolicy{Checks: []rc.HealthCheck{{Path: "/", ExpectedStatus: 200, ExpectedSHA256: strings.Repeat("d", 64)}}}}}
	o := rc.DeploymentObservation{Protocol: rc.ReleaseProtocol, OperationID: d.OperationID, PlanID: d.PlanID, PlanRevision: 1, PlanSHA256: d.PlanSHA256, EnvironmentID: d.EnvironmentID, ProviderID: d.ProviderID, Generation: 1, FenceGeneration: 1, RequestSHA256: d.RequestSHA256, Version: 1, State: "succeeded", Effect: rc.DeploymentEffect{State: "applied", ActiveGeneration: 1, ActiveArtifactSHA256: d.ArtifactSHA256, ObservedAt: now()}, Health: rc.HealthObservation{State: "passed", Generation: 1, ArtifactSHA256: d.ArtifactSHA256, Checks: []rc.HealthCheckResult{{Path: "/", StatusCode: 200, BodySHA256: strings.Repeat("d", 64), ObservedAt: now()}}}}
	o.EvidenceSHA256, _ = rc.ObservationDigest(o)
	if err := validateReleaseObservation(d, p, o); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"unknown", "wrong-content", "wrong-generation", "missing-fence", "no-real-http-time", "tampered-digest"} {
		t.Run(scenario, func(t *testing.T) {
			bad := o
			bad.Health.Checks = append([]rc.HealthCheckResult{}, o.Health.Checks...)
			switch scenario {
			case "unknown":
				bad.Effect.State = "unknown"
			case "wrong-content":
				bad.Health.Checks[0].BodySHA256 = strings.Repeat("e", 64)
			case "wrong-generation":
				bad.Health.Generation = 2
			case "missing-fence":
				bad.FenceGeneration = 0
			case "no-real-http-time":
				bad.Health.Checks[0].ObservedAt = ""
			}
			bad.EvidenceSHA256, _ = rc.ObservationDigest(bad)
			if scenario == "tampered-digest" {
				bad.EvidenceSHA256 = strings.Repeat("f", 64)
			}
			if validateReleaseObservation(d, p, bad) == nil {
				t.Fatal("unproven success accepted")
			}
		})
	}
}
func TestReleaseDeploymentProviderIndependentIdentityAndStrictJSON(t *testing.T) {
	f, _ := releaseTestSetup(t)
	handler := &Server{Service: f.s, Engine: f.e}
	for _, token := range []string{"", f.cookie.Value, "ordinary-runner-token"} {
		req, _ := http.NewRequest("POST", "http://suma.test/api/v1/provider/deployment-authorizations/consume", strings.NewReader(`{"provider_id":"local-static-test","operation_id":"deploy-does-not-exist"}`))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		if !handler.releaseProviderHTTP(w, req, "provider/deployment-authorizations/consume") {
			t.Fatal("provider route not handled")
		}
		if w.Code != 403 {
			t.Fatalf("browser/unknown provider accepted: %d", w.Code)
		}
	}
	for _, b := range []string{`{"generation":1,"generation":2}`, `{"nested":{"state":"unknown","state":"succeeded"}}`, `{} {}`} {
		if RejectDuplicateReleaseJSON([]byte(b)) == nil {
			t.Fatal("duplicate or trailing JSON accepted")
		}
	}
	if RejectDuplicateReleaseJSON([]byte(strings.Repeat("[", 130)+"0"+strings.Repeat("]", 130))) == nil {
		t.Fatal("unbounded JSON depth accepted")
	}
}

func verifiedReleaseDeploymentPlan(t *testing.T) (*fixture, ReleaseCandidate, ReleaseEnvironment, ReleasePlan, string, string) {
	t.Helper()
	f, candidate, builderToken := newVerifiedReleaseCandidate(t)
	secret := "provider-test-" + newID()
	f.s.ReleaseProviders = []ReleaseProviderConfig{{ID: "local-static-test", TenantID: f.user.TenantID, ProjectID: f.project, TokenHash: digest(secret), SiteIDs: []string{"fixture-site"}}}
	env := releaseTestEnvironment(t, f)
	input := map[string]any{"project_id": f.project, "candidate_id": candidate.ID, "environment_id": env.ID, "action": "deploy", "expected_environment_generation": 0}
	preview, err := f.s.PreviewReleasePlan(context.Background(), f.actor, f.project, input)
	if err != nil {
		t.Fatal(err)
	}
	input["preview_sha256"] = preview.PreviewSHA256
	input["idempotency_key"] = newID()
	plan, err := f.s.CreateReleasePlan(context.Background(), f.actor, f.project, input)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = f.s.ApproveReleasePlan(context.Background(), f.actor, f.project, plan.ID, map[string]any{"project_id": f.project, "expected_version": plan.Version, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256, "decision": "approved", "reason": "实际构建、封存、包HTTP测试和人工验收均已核对", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	return f, candidate, env, plan, builderToken, secret
}
func releaseExecuteBody(f *fixture, p ReleasePlan) map[string]any {
	return map[string]any{"project_id": f.project, "expected_version": p.Version, "plan_revision": p.Revision, "plan_sha256": p.SHA256, "idempotency_key": newID()}
}

func TestReleaseDeploymentRealCandidateAtomicExecuteTicketsAndUnknownBarrier(t *testing.T) {
	f, _, env, plan, builderToken, providerToken := verifiedReleaseDeploymentPlan(t)
	ctx := context.Background()
	// This security scenario intentionally holds the protected attempt while
	// claiming a second ordinary run to test that copied config grants nothing.
	// Reserve both slots explicitly; the production default remains one.
	if err := f.e.SetResourceLimits(ctx, f.user.TenantID, f.project, map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	body := releaseExecuteBody(f, plan)
	// Both handles use the actual persisted candidate and reserved run, never a seeded proof.
	var path string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	other := &Service{DB: db, Dialect: "sqlite", DataDir: f.s.DataDir, ReleaseProviders: f.s.ReleaseProviders}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		out ReleaseDeployment
		err error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, s := range []*Service{f.s, other} {
		wg.Add(1)
		go func(i int, s *Service) {
			defer wg.Done()
			<-start
			e := f.e
			if i == 1 {
				e = engine
			}
			out, err := s.ExecuteReleasePlan(ctx, f.actor, f.project, plan.ID, body, e)
			results <- result{out, err}
		}(i, s)
	}
	close(start)
	wg.Wait()
	close(results)
	var deployment ReleaseDeployment
	for r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		if deployment.OperationID != "" && deployment.OperationID != r.out.OperationID {
			t.Fatal("concurrent retry created two operations")
		}
		deployment = r.out
	}
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_release_deployments WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&count); err != nil || count != 1 {
		t.Fatal("managed execute not unique", err, count)
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	builder := &runner.Client{BaseURL: server.URL, Token: builderToken, ProjectID: f.project}
	a, err := builder.Claim(ctx, "release-fixture-worker", releaseTestCapabilities("local-static-test"))
	if err != nil || a != nil {
		t.Fatal("non-deployment worker consumed protected run", err)
	}
	registration, err := f.s.RegisterReleaseWorker(ctx, f.actor, f.project, map[string]any{"worker_id": "protected-deployer", "profiles": []any{}, "deployment": true, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	workerToken := registration["token"].(string)
	_, err = f.s.AuthenticateActor(ctx, workerToken)
	if err != nil {
		t.Fatal(err)
	}
	client := &runner.Client{BaseURL: server.URL, Token: workerToken, ProjectID: f.project}
	a, err = client.Claim(ctx, "protected-deployer", releaseTestCapabilities("local-static-test"))
	if err != nil || a == nil || a.ReleaseExecution == nil {
		t.Fatal("protected assignment missing server binding", err)
	}
	ref := rc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	request := rc.ReleaseAuthorizationRequest{ProjectID: f.project, Assignment: ref, OperationID: deployment.OperationID}
	ordinary := f.call("POST", "/workflows", f.body(map[string]any{"name": "复制发布配置不能授予部署权限", "nodes": []dag.Node{{ID: "forged", Type: "external_workflow", Config: map[string]any{"plugin": "local-static-test", "workflow": rc.ReleaseProtocol, "input": map[string]any{"release_execution": *a.ReleaseExecution}, "release_execution": *a.ReleaseExecution}}}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": ordinary["id"]}), 201)
	copied, err := client.Claim(ctx, "protected-deployer", []string{"external_workflow:local-static-test"})
	if err != nil || copied == nil || copied.ReleaseExecution != nil {
		t.Fatal("copied Config acquired protected top-level release binding", err)
	}
	copiedRef := rc.AssignmentRef{RunID: copied.RunID, NodeID: copied.NodeID, Attempt: copied.Attempt, Fence: copied.Fence, WorkerID: copied.WorkerID}
	_, err = f.s.IssueReleaseAuthorization(ctx, domainCActor(t, f.s, workerToken), rc.ReleaseAuthorizationRequest{ProjectID: f.project, Assignment: copiedRef, OperationID: deployment.OperationID})
	releaseWantStatus(t, err, 403)
	first, err := f.s.IssueReleaseAuthorization(ctx, domainCActor(t, f.s, workerToken), request)
	if err != nil {
		t.Fatal(err)
	}
	renewed, err := f.s.IssueReleaseAuthorization(ctx, domainCActor(t, f.s, workerToken), request)
	if err != nil || first.AuthorizationTicket == renewed.AuthorizationTicket || first.OperationID != renewed.OperationID {
		t.Fatal("ticket renewal changed operation or reused secret", err)
	}
	consume := rc.ConsumeDeploymentAuthorization{ProviderID: "local-static-test", AuthorizationTicket: first.AuthorizationTicket, OperationID: deployment.OperationID, Assignment: ref, CallerWorkerID: ref.WorkerID, RequestSHA256: first.RequestSHA256}
	_, err = f.s.ConsumeReleaseAuthorization(ctx, providerToken, consume)
	releaseWantStatus(t, err, 403)
	consume.AuthorizationTicket = renewed.AuthorizationTicket
	_, err = f.s.ConsumeReleaseAuthorization(ctx, f.cookie.Value, consume)
	releaseWantStatus(t, err, 403)
	consume.CallerWorkerID = "different-worker"
	_, err = f.s.ConsumeReleaseAuthorization(ctx, providerToken, consume)
	releaseWantStatus(t, err, 403)
	consume.CallerWorkerID = ref.WorkerID
	grant, err := f.s.ConsumeReleaseAuthorization(ctx, providerToken, consume)
	if err != nil || grant.Plan.ArtifactID != deployment.ArtifactID {
		t.Fatal("provider did not receive trusted frozen plan", err)
	}
	again, err := f.s.ConsumeReleaseAuthorization(ctx, providerToken, consume)
	if err != nil || again.AuthorizationID != grant.AuthorizationID {
		t.Fatal("consume retry changed grant", err)
	}
	effect, err := f.s.BeginReleaseEffect(ctx, providerToken, rc.BeginDeploymentEffect{AuthorizationID: grant.AuthorizationID, OperationID: deployment.OperationID, Assignment: ref, RequestSHA256: grant.RequestSHA256, Generation: deployment.Generation})
	if err != nil || effect.EffectGrantID == "" {
		t.Fatal("begin effect grant missing", err)
	}
	// No remote success is fabricated. An intentionally unknown provider outcome
	// must remain unknown through persistence, repeat reports and lease loss.
	unknown := rc.DeploymentObservation{Protocol: rc.ReleaseProtocol, OperationID: deployment.OperationID, RequestSHA256: grant.RequestSHA256, ProviderID: deployment.ProviderID, PlanID: plan.ID, PlanRevision: plan.Revision, PlanSHA256: plan.SHA256, EnvironmentID: env.ID, Generation: deployment.Generation, FenceGeneration: 0, State: "reconciling", Effect: rc.DeploymentEffect{State: "unknown"}, Health: rc.HealthObservation{State: "unknown"}, Version: 5}
	unknown.EvidenceSHA256, _ = rc.ObservationDigest(unknown)
	observed, err := f.s.ObserveReleaseDeployment(ctx, providerToken, unknown)
	if err != nil || observed.State != "reconciling" || observed.IssueID == "" {
		t.Fatal("unknown outcome not persisted with issue", err)
	}
	againObserved, err := other.ObserveReleaseDeployment(ctx, providerToken, unknown)
	if err != nil || againObserved.Version != observed.Version || againObserved.IssueID != observed.IssueID {
		t.Fatal("repeat observation created another transition or issue", err)
	}
	late := unknown
	late.Version = 3
	late.EvidenceSHA256, _ = rc.ObservationDigest(late)
	lateResult, err := other.ObserveReleaseDeployment(ctx, providerToken, late)
	if err != nil || lateResult.Version != observed.Version || lateResult.Observation.Version != 5 {
		t.Fatal("late older observation replaced current projection", err)
	}
	var historicalCount int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_release_observations WHERE operation_id=?`, deployment.OperationID).Scan(&historicalCount); err != nil || historicalCount != 2 {
		t.Fatal("late history was not retained", err)
	}
	currentEnv, err := f.s.ReleaseEnvironment(ctx, f.actor, f.project, env.ID)
	if err != nil || currentEnv.Generation != 1 || currentEnv.ActiveGeneration != 0 || currentEnv.BarrierOperationID != deployment.OperationID {
		t.Fatal("unknown released or advanced actual active", err)
	}
	_, err = f.s.SaveReleaseEnvironment(ctx, f.actor, f.project, env.ID, map[string]any{"project_id": f.project, "status": "archived", "expected_version": currentEnv.Version, "idempotency_key": newID()})
	releaseWantStatus(t, err, 409)
	currentPlan, err := f.s.ReleasePlan(ctx, f.actor, f.project, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ApproveReleasePlan(ctx, f.actor, f.project, plan.ID, map[string]any{"project_id": f.project, "expected_version": currentPlan.Version, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256, "decision": "revoked", "reason": "未知副作用继续占用，撤销后续激活授权", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.BeginReleaseEffect(ctx, providerToken, rc.BeginDeploymentEffect{AuthorizationID: grant.AuthorizationID, OperationID: deployment.OperationID, Assignment: ref, RequestSHA256: grant.RequestSHA256, Generation: deployment.Generation})
	releaseWantStatus(t, err, 409)
	var revoked *APIError
	if !errors.As(err, &revoked) || revoked.Code != "release_approval_required" {
		t.Fatal("old effect grant replay bypassed live revoked approval", err)
	}
	historical, err := f.s.InspectReleaseAuthorization(ctx, providerToken, rc.InspectDeploymentAuthorization{OperationID: deployment.OperationID, Assignment: ref, RequestSHA256: grant.RequestSHA256})
	if err != nil || historical.AuthorizationID != grant.AuthorizationID {
		t.Fatal("lost consume ACK could not recover non-secret historical grant", err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=?`, deployment.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_wakeups SET due_at=0 WHERE run_id=? AND kind='lease'`, deployment.RunID); err != nil {
		t.Fatal(err)
	}
	if err = f.e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	currentEnv, err = f.s.ReleaseEnvironment(ctx, f.actor, f.project, env.ID)
	if err != nil || currentEnv.BarrierOperationID != deployment.OperationID {
		t.Fatal("lease expiry released effect barrier", err)
	}
	// A provider may report a failed request while its external effect remains
	// unknown. Even a real HTTP inspect must not turn this into a resolved DAG.
	failedUnknown := unknown
	failedUnknown.Version = 6
	failedUnknown.State = "failed"
	failedUnknown.FenceGeneration = deployment.Generation
	failedUnknown.EvidenceSHA256, _ = rc.ObservationDigest(failedUnknown)
	inspector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, failedUnknown) }))
	defer inspector.Close()
	f.s.ReleaseProviders[0].Origin = inspector.URL
	f.s.ReleaseProviders[0].ReconcileToken = "negative-unknown-inspection-token"
	unknownResult, err := f.s.ReconcileReleaseDeployment(ctx, f.actor, f.project, deployment.OperationID, map[string]any{"project_id": f.project, "expected_version": observed.Version, "idempotency_key": newID()}, f.e)
	if err != nil || unknownResult.State != "reconciling" {
		t.Fatal("failed request erased unknown effect", err)
	}
	unresolved, err := f.e.GetRun(ctx, f.user.TenantID, f.project, deployment.RunID)
	if err != nil || unresolved.Nodes[0].State != "reconciling" {
		t.Fatal("unknown external effect was resolved by request failure", err)
	}
	var rowsRaw string
	rows, err := f.s.DB.Query(`SELECT response FROM suma_operations`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var row string
		rows.Scan(&row)
		rowsRaw += row
	}
	rows.Close()
	listed, err := f.s.ReleaseDeployment(ctx, f.actor, f.project, deployment.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	listJSON, _ := json.Marshal(listed)
	for _, secret := range []string{first.AuthorizationTicket, renewed.AuthorizationTicket, providerToken, workerToken} {
		if strings.Contains(rowsRaw, secret) || strings.Contains(string(listJSON), secret) {
			t.Fatal("secret leaked to durable public receipt/read")
		}
	}
}

func TestReleaseDeploymentActualCandidateBasisChangedBlocksDispatch(t *testing.T) {
	f, candidate, env, plan, _, _ := verifiedReleaseDeploymentPlan(t)
	other := f.call("POST", "/projects", map[string]any{"name": "独立发布项目", "idempotency_key": newID()}, 201)
	otherID := other["id"].(string)
	_, scopeErr := f.s.ReleaseCandidate(context.Background(), f.actor, otherID, candidate.ID)
	releaseWantStatus(t, scopeErr, 404)
	_, scopeErr = f.s.ReleaseArtifact(context.Background(), f.actor, otherID, candidate.ArtifactID)
	releaseWantStatus(t, scopeErr, 404)
	_, scopeErr = f.s.PreviewReleasePlan(context.Background(), f.actor, otherID, map[string]any{"project_id": otherID, "candidate_id": candidate.ID, "environment_id": env.ID, "action": "deploy", "expected_environment_generation": 0})
	releaseWantStatus(t, scopeErr, 404)
	task := f.call("GET", "/tasks/"+candidate.Tasks[0].ID+"?project_id="+f.project, nil, 200)
	f.call("PATCH", "/tasks/"+candidate.Tasks[0].ID, f.body(map[string]any{"title": "审批后交付定义已变化", "expected_version": task["version"]}), 200)
	_, err := f.s.ExecuteReleasePlan(context.Background(), f.actor, f.project, plan.ID, releaseExecuteBody(f, plan), f.e)
	if err == nil {
		t.Fatal("stale accepted task basis dispatched")
	}
	current, err := f.s.ReleaseEnvironment(context.Background(), f.actor, f.project, env.ID)
	if err != nil || current.Generation != 0 || current.BarrierOperationID != "" {
		t.Fatal("failed execute left generation reservation", err)
	}
	var runs int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs WHERE id=?`, plan.Content.ReservedExecution.RunID).Scan(&runs); err != nil || runs != 0 {
		t.Fatal("failed execute left claimable managed run", err)
	}
}

func TestReleaseDeploymentCandidateRejectsUnknownFields(t *testing.T) {
	f, candidate, _ := newVerifiedReleaseCandidate(t)
	body := map[string]any{"name": "不接受未定义通过标记", "artifact_id": candidate.ArtifactID, "build_attestation_id": candidate.BuildAttestationID, "package_test_attestation_id": candidate.PackageTestAttestationID, "tasks": []any{map[string]any{"id": candidate.Tasks[0].ID, "expected_version": candidate.Tasks[0].Version}}, "passed": true, "idempotency_key": newID()}
	_, err := f.s.CreateReleaseCandidate(context.Background(), f.actor, f.project, body)
	releaseWantStatus(t, err, 400)
}

func TestReleaseDeploymentTaskBasisMutationRechecksLiveAuthority(t *testing.T) {
	f, _ := releaseTestSetup(t)
	ownerCookie := f.cookie
	inv := governanceInvite(f, "release-basis-editor@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	accepted := governanceAccept(f, inv, "release-basis-editor@example.test")
	var developer Principal
	b, _ := json.Marshal(accepted["user"])
	json.Unmarshal(b, &developer)
	developerActor, err := f.s.AuthenticateActor(context.Background(), f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = ownerCookie
	f.call("GET", "/auth/session", nil, 200)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "发布依据并发更新权限回归"}), 201)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service, gate := reviewAuthorityConnection(t, f, ctx, "before_authority_tx")
	done := make(chan error, 1)
	go func() {
		_, err := service.Update(ctx, developerActor, f.project, "tasks", task["id"].(string), map[string]any{"title": "撤权后不应写入", "expected_version": task["version"], "idempotency_key": newID()})
		done <- err
	}()
	select {
	case <-gate.observed:
	case <-ctx.Done():
		t.Fatal("did not reach real pre-transaction authorization read")
	}
	_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": developer.ID, "role": "viewer", "expected_version": float64(1), "idempotency_key": newID()})
	if err != nil {
		gate.release()
		t.Fatal(err)
	}
	gate.release()
	select {
	case err = <-done:
		releaseWantStatus(t, err, 403)
	case <-ctx.Done():
		t.Fatal("update did not finish after role change")
	}
}
