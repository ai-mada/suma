package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/deployer"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
)

const releaseVariantHTML = "<!doctype html><title>Suma release B</title><h1>actual second immutable package B</h1>\n"

func TestReleaseDeploymentProcess(t *testing.T) {
	phase := os.Getenv("SUMA_RELEASE_VARIANT_PHASE")
	if phase == "" {
		return
	}
	html := os.Getenv("SUMA_RELEASE_VARIANT_HTML")
	fail := func(err any) { fmt.Fprintln(os.Stderr, err); os.Exit(41) }
	switch phase {
	case "develop":
		if err := os.WriteFile("site.html", []byte(html), 0600); err != nil {
			fail(err)
		}
		for _, args := range [][]string{{"add", "site.html"}, {"commit", "-m", "fixture: second candidate B"}} {
			if b, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				fail(string(b))
			}
		}
	case "build":
		b, err := os.ReadFile("site.html")
		if err != nil || string(b) != html {
			fail("build did not receive fixed second candidate")
		}
		if err = os.MkdirAll(".cache/site", 0700); err != nil {
			fail(err)
		}
		if err = os.WriteFile(".cache/site/index.html", b, 0600); err != nil {
			fail(err)
		}
	case "package_test":
		b, err := os.ReadFile("site.html")
		if err != nil || string(b) != html {
			fail("test harness has wrong source")
		}
		site := httptest.NewServer(http.FileServer(http.Dir(os.Getenv("SUMA_ARTIFACT_DIR"))))
		r, err := site.Client().Get(site.URL + "/index.html")
		if err != nil {
			site.Close()
			fail(err)
		}
		b, err = io.ReadAll(r.Body)
		r.Body.Close()
		site.Close()
		if err != nil || r.StatusCode != 200 || string(b) != html {
			fail("real second package HTTP contract failed")
		}
	default:
		fail("unknown variant process phase")
	}
	fmt.Println("actual second candidate process completed:", phase)
	os.Exit(0)
}

func releaseVariantCandidate(t *testing.T, f *fixture) ReleaseCandidate {
	t.Helper()
	ctx := context.Background()
	repo := t.TempDir()
	releaseGit(t, repo, "init", "--initial-branch=main")
	for name, body := range map[string]string{".gitignore": ".cache/\n", "site.html": "independent B base\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	releaseGit(t, repo, "add", ".gitignore", "site.html")
	releaseGit(t, repo, "commit", "-m", "fixture second package base")
	base := releaseGit(t, repo, "rev-parse", "HEAD")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := func(phase string) runner.CommandSpec {
		return runner.CommandSpec{Executable: exe, Args: []string{"-test.run=^TestReleaseDeploymentProcess$", "--"}, Env: map[string]string{"SUMA_RELEASE_VARIANT_PHASE": phase, "SUMA_RELEASE_VARIANT_HTML": releaseVariantHTML, "GIT_AUTHOR_NAME": "Suma fixture", "GIT_AUTHOR_EMAIL": "fixture@example.test", "GIT_COMMITTER_NAME": "Suma fixture", "GIT_COMMITTER_EMAIL": "fixture@example.test", "GIT_CONFIG_NOSYSTEM": "1"}}
	}
	develop, build, test := command("develop"), command("build"), command("package_test")
	build.ArtifactOutput = ".cache/site"
	test.ArtifactInput = true
	test.TestEvidence = true
	profiles := []ReleaseProfileRegistration{}
	for _, entry := range []struct {
		id, kind string
		spec     runner.CommandSpec
	}{{"variant-build", "build", build}, {"variant-test", "package_test", test}} {
		s := entry.spec
		hash, e := rc.ProfileDigest(rc.CommandProfile{ID: entry.id, Kind: entry.kind, Executable: s.Executable, Args: s.Args, Env: s.Env, EnvFrom: s.EnvFrom, Input: s.Input, ReadOnly: s.ReadOnly, TestEvidence: s.TestEvidence, ArtifactOutput: s.ArtifactOutput, ArtifactInput: s.ArtifactInput})
		if e != nil {
			t.Fatal(e)
		}
		profiles = append(profiles, ReleaseProfileRegistration{ID: entry.id, Kind: entry.kind, SHA256: hash})
	}
	registered, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, map[string]any{"worker_id": "variant-builder", "profiles": profiles, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "第二个真实内容包B", "acceptance_criteria": []string{"真实固定B源码及封存包HTTP测试"}}), 201)
	nodes := []dag.Node{{ID: "develop", Type: "command", Command: []string{"variant-develop"}}, {ID: "build", Type: "command", Command: []string{"variant-build"}, DependsOn: []string{"develop"}, Config: map[string]any{"candidate_from": "develop"}}, {ID: "package-test", Type: "test", Command: []string{"variant-test"}, DependsOn: []string{"build"}, Config: map[string]any{"artifact_from": "build"}}}
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "真实第二候选包B", "nodes": nodes}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "task_id": task["id"], "expected_task_version": task["version"]}), 201)
	control := httptest.NewServer(f.handler)
	defer control.Close()
	client := &runner.Client{BaseURL: control.URL, Token: registered["token"].(string), ProjectID: f.project}
	journal, err := runner.OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	config := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"variant-develop": develop, "variant-build": build, "variant-test": test}, Artifacts: client, Journal: journal}
	runReleaseFixtureWorker(t, ctx, f, client, journal, "variant-builder", run["id"].(string), config, 0)
	current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	accepted := f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "第二候选B已实际构建、收包、HTTP包测试"}), 200)
	artifacts, _, _, err := f.s.ReleaseArtifacts(ctx, f.user, f.project, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		var buildID, testID string
		for _, proof := range artifact.Proofs {
			if proof.Assignment.RunID == run["id"] {
				if proof.Kind == "build" {
					buildID = proof.ID
				} else if proof.Kind == "package_test" {
					testID = proof.ID
				}
			}
		}
		if buildID == "" || testID == "" {
			continue
		}
		view, e := f.s.CreateReleaseCandidate(ctx, f.user, f.project, map[string]any{"name": "实际第二内容B候选", "artifact_id": artifact.ID, "build_attestation_id": buildID, "package_test_attestation_id": testID, "tasks": []any{map[string]any{"id": accepted["id"], "expected_version": accepted["version"]}}, "idempotency_key": newID()})
		if e != nil {
			t.Fatal(e)
		}
		return view.Candidate
	}
	t.Fatal("actual B proofs missing")
	return ReleaseCandidate{}
}

func TestReleaseDeploymentActualProviderHealthAndNewGenerationRollback(t *testing.T) {
	f, candidate, env, plan, _, providerToken := verifiedReleaseDeploymentPlan(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	control := httptest.NewServer(f.handler)
	defer control.Close()
	worker, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, map[string]any{"worker_id": "actual-provider-worker", "profiles": []any{}, "deployment": true, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	workerToken := worker["token"].(string)
	workerPrincipal, err := f.s.Authenticate(ctx, workerToken)
	if err != nil {
		t.Fatal(err)
	}
	client := &runner.Client{BaseURL: control.URL, Token: workerToken, ProjectID: f.project}
	transportToken := "transport-" + newID()
	reconcilerToken := "reconciler-" + newID()
	t.Setenv("SUMA_CONTROL_PROVIDER_TEST_TOKEN", providerToken)
	t.Setenv("SUMA_CONTROL_PROVIDER_TEST_WORKER", transportToken)
	t.Setenv("SUMA_CONTROL_PROVIDER_TEST_RECONCILE", reconcilerToken)
	config := deployer.Config{ProviderID: "local-static-test", Root: filepath.Join(t.TempDir(), "actual-static-provider"), ControlURL: control.URL, ControlTokenEnv: "SUMA_CONTROL_PROVIDER_TEST_TOKEN", ReconcileTokenEnv: "SUMA_CONTROL_PROVIDER_TEST_RECONCILE", Workers: map[string]string{"actual-provider-worker": "SUMA_CONTROL_PROVIDER_TEST_WORKER"}, Sites: map[string]deployer.SiteConfig{"fixture-site": {Address: "127.0.0.1:0"}}}
	provider, err := deployer.New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	management := httptest.NewServer(provider.Handler())
	defer management.Close()
	f.s.ReleaseProviders[0].Origin = management.URL
	f.s.ReleaseProviders[0].ReconcileToken = reconcilerToken
	callProvider := func(method, path string, body any) rc.DeploymentObservation {
		t.Helper()
		var reader io.Reader
		if body != nil {
			b, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			reader = bytes.NewReader(b)
		}
		req, e := http.NewRequestWithContext(ctx, method, management.URL+path, reader)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+transportToken)
		req.Header.Set("Content-Type", "application/json")
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 && resp.StatusCode != 202 {
			t.Fatalf("provider %s status %d", path, resp.StatusCode)
		}
		var observation rc.DeploymentObservation
		if e = json.NewDecoder(resp.Body).Decode(&observation); e != nil {
			t.Fatal(e)
		}
		return observation
	}
	loseRunnerResult := false
	doDeploy := func(p ReleasePlan) ReleaseDeployment {
		t.Helper()
		d, e := f.s.ExecuteReleasePlan(ctx, f.user, f.project, p.ID, f.cookie.Value, releaseExecuteBody(f, p), f.e)
		if e != nil {
			t.Fatal(e)
		}
		a, e := client.Claim(ctx, "actual-provider-worker", []string{"external_workflow:local-static-test"})
		if e != nil || a == nil || a.ReleaseExecution == nil {
			t.Fatal("claim protected actual deployment", e)
		}
		ref := rc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
		authorization, e := f.s.IssueReleaseAuthorization(ctx, workerPrincipal, workerToken, rc.ReleaseAuthorizationRequest{ProjectID: f.project, Assignment: ref, OperationID: d.OperationID})
		if e != nil {
			t.Fatal(e)
		}
		request := rc.DeploymentRequest{ReleaseExecution: *a.ReleaseExecution, Assignment: ref, AuthorizationTicket: authorization.AuthorizationTicket}
		observation := callProvider(http.MethodPost, "/v2/deployments", request)
		for observation.State != "succeeded" && observation.State != "failed" && observation.State != "cancelled" {
			select {
			case <-ctx.Done():
				t.Fatal("provider did not reach observed terminal state")
			case <-time.After(30 * time.Millisecond):
			}
			observation = callProvider(http.MethodGet, "/v2/operations/"+d.OperationID, nil)
		}
		if observation.State != "succeeded" || observation.Effect.State != "applied" || observation.Health.State != "passed" {
			t.Fatalf("actual provider did not pass health: state=%s effect=%s health=%s", observation.State, observation.Effect.State, observation.Health.State)
		}
		persisted, e := f.s.ReleaseDeployment(ctx, f.user, f.project, d.OperationID)
		if e != nil || persisted.State != "deployed_healthy" || persisted.Observation == nil {
			t.Fatal("provider observation not projected", e)
		}
		if loseRunnerResult {
			original := runner.Result{Assignment: *a, OperationID: newID(), ExitCode: -1, Error: "transport outcome unknown before final result ACK", OutcomeUnknown: true}
			if e = client.Complete(ctx, original); e != nil {
				t.Fatal(e)
			}
			currentPlan, e := f.s.ReleasePlan(ctx, f.user, f.project, p.ID)
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.s.ApproveReleasePlan(ctx, f.user, f.project, p.ID, f.cookie.Value, map[string]any{"project_id": f.project, "expected_version": currentPlan.Version, "plan_revision": p.Revision, "plan_sha256": p.SHA256, "decision": "revoked", "reason": "实际激活后撤销后续授权，仍须保留已发生事实", "idempotency_key": newID()})
			if e != nil {
				t.Fatal(e)
			}
			task := f.call("GET", "/tasks/"+candidate.Tasks[0].ID+"?project_id="+f.project, nil, 200)
			f.call("PATCH", "/tasks/"+candidate.Tasks[0].ID, f.body(map[string]any{"title": "激活后的范围变化须新候选，但不能抹掉历史部署事实", "expected_version": task["version"]}), 200)
			if _, e = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, a.RunID, a.NodeID); e != nil {
				t.Fatal(e)
			}
			if _, e = f.s.DB.Exec(`UPDATE dag_wakeups SET due_at=0 WHERE run_id=? AND node_id=? AND kind='lease'`, a.RunID, a.NodeID); e != nil {
				t.Fatal(e)
			}
			if e = f.e.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			_, e = f.s.ReconcileReleaseDeployment(ctx, f.user, f.project, d.OperationID, f.cookie.Value, map[string]any{"project_id": f.project, "expected_version": persisted.Version, "idempotency_key": newID()}, f.e)
			if e != nil {
				t.Fatal(e)
			}
			resolved, e := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
			if e != nil || resolved.State != "succeeded" {
				t.Fatalf("real healthy provider after lost Runner result did not resolve managed DAG: run=%s node=%s err=%v", resolved.State, resolved.Nodes[0].State, e)
			}
			var originalRaw string
			if e = f.s.DB.QueryRow(`SELECT original_result FROM dag_managed_resolutions WHERE run_id=? AND node_id=? AND attempt=?`, a.RunID, a.NodeID, a.Attempt).Scan(&originalRaw); e != nil {
				t.Fatal(e)
			}
			var preserved dag.Result
			if json.Unmarshal([]byte(originalRaw), &preserved) != nil || !preserved.OutcomeUnknown || preserved.Error != original.Error {
				t.Fatal("original unknown Runner result was erased")
			}
			currentPlan, e = f.s.ReleasePlan(ctx, f.user, f.project, p.ID)
			if e != nil || currentPlan.Approval.Decision != "revoked" || len(currentPlan.ApprovalHistory) != 2 || currentPlan.Eligibility {
				t.Fatal("fact recovery fabricated approval or discarded approval history", e)
			}
			return persisted
		}
		result := runner.Result{Assignment: *a, OperationID: newID(), ExitCode: 0, ReleaseEvidence: &rc.ReleaseResult{OperationID: d.OperationID, RequestSHA256: observation.RequestSHA256, EvidenceSHA256: observation.EvidenceSHA256, Version: observation.Version}}
		if e = client.Complete(ctx, result); e != nil {
			t.Fatal("actual provider final proof failed", e)
		}
		// Idempotent POST/inspect is observable; it must preserve the same operation.
		again := callProvider(http.MethodPost, "/v2/deployments", request)
		if again.OperationID != observation.OperationID || again.EvidenceSHA256 != observation.EvidenceSHA256 {
			t.Fatal("repeat POST changed actual operation")
		}
		return persisted
	}
	first := doDeploy(plan)
	site, err := http.Get(provider.SiteURL("fixture-site") + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(site.Body)
	site.Body.Close()
	if err != nil || site.StatusCode != 200 || string(body) != releaseFixtureHTML {
		t.Fatal("actual active site did not serve sealed candidate bytes", err)
	}
	currentEnv, err := f.s.ReleaseEnvironment(ctx, f.user, f.project, env.ID)
	if err != nil || currentEnv.Generation != 1 || currentEnv.ActiveGeneration != 1 || currentEnv.ActiveArtifactID != candidate.ArtifactID || currentEnv.BarrierOperationID != "" {
		t.Fatal("actual healthy operation did not release reservation", err)
	}
	variant := releaseVariantCandidate(t, f)
	if variant.ArtifactSHA256 == candidate.ArtifactSHA256 {
		t.Fatal("second candidate did not change actual package bytes")
	}
	variantInput := map[string]any{"project_id": f.project, "candidate_id": variant.ID, "environment_id": env.ID, "action": "deploy", "expected_environment_generation": currentEnv.Generation}
	variantPreview, err := f.s.PreviewReleasePlan(ctx, f.user, f.project, f.cookie.Value, variantInput)
	if err != nil {
		t.Fatal(err)
	}
	variantInput["preview_sha256"] = variantPreview.PreviewSHA256
	variantInput["idempotency_key"] = newID()
	variantPlan, err := f.s.CreateReleasePlan(ctx, f.user, f.project, f.cookie.Value, variantInput)
	if err != nil {
		t.Fatal(err)
	}
	variantPlan, err = f.s.ApproveReleasePlan(ctx, f.user, f.project, variantPlan.ID, f.cookie.Value, map[string]any{"project_id": f.project, "expected_version": variantPlan.Version, "plan_revision": variantPlan.Revision, "plan_sha256": variantPlan.SHA256, "decision": "approved", "reason": "真实第二内容B已构建、包测试与验收", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	actualB := doDeploy(variantPlan)
	if actualB.Generation != 2 || actualB.ArtifactSHA256 == first.ArtifactSHA256 {
		t.Fatal("second content did not activate new generation")
	}
	site, err = http.Get(provider.SiteURL("fixture-site") + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(site.Body)
	site.Body.Close()
	if err != nil || string(body) != releaseVariantHTML {
		t.Fatal("actual site did not change to second package B", err)
	}
	currentEnv, err = f.s.ReleaseEnvironment(ctx, f.user, f.project, env.ID)
	if err != nil {
		t.Fatal(err)
	}
	rollback, err := f.s.RollbackReleasePreview(ctx, f.user, f.project, first.OperationID, f.cookie.Value, map[string]any{"project_id": f.project, "expected_environment_generation": currentEnv.Generation})
	if err != nil {
		t.Fatal(err)
	}
	input := rollback["input"].(ReleasePlanInput)
	preview := rollback["preview"].(ReleasePlanPreview)
	input.PreviewSHA256 = preview.PreviewSHA256
	input.IdempotencyKey = newID()
	data, _ := json.Marshal(input)
	var create map[string]any
	json.Unmarshal(data, &create)
	next, err := f.s.CreateReleasePlan(ctx, f.user, f.project, f.cookie.Value, create)
	if err != nil {
		t.Fatal(err)
	}
	next, err = f.s.ApproveReleasePlan(ctx, f.user, f.project, next.ID, f.cookie.Value, map[string]any{"project_id": f.project, "expected_version": next.Version, "plan_revision": next.Revision, "plan_sha256": next.SHA256, "decision": "approved", "reason": "回退到同环境真实历史健康制品，使用新代数重新核验", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	loseRunnerResult = true
	second := doDeploy(next)
	if second.OperationID == first.OperationID || second.Generation != 3 || second.ArtifactID != first.ArtifactID {
		t.Fatal("rollback reused old operation/generation or changed historical artifact")
	}
	currentEnv, err = f.s.ReleaseEnvironment(ctx, f.user, f.project, env.ID)
	if err != nil || currentEnv.Generation != 3 || currentEnv.ActiveGeneration != 3 || currentEnv.BarrierOperationID != "" {
		t.Fatal("actual rollback generation not observed", err)
	}
	site, err = http.Get(provider.SiteURL("fixture-site") + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(site.Body)
	site.Body.Close()
	if err != nil || string(body) != releaseFixtureHTML {
		t.Fatal("actual rollback did not restore package A bytes", err)
	}
	// Late exact old observation remains audit history and cannot replace active=3.
	if _, err = f.s.ObserveReleaseDeployment(ctx, providerToken, *first.Observation); err != nil {
		t.Fatal(err)
	}
	currentEnv, err = f.s.ReleaseEnvironment(ctx, f.user, f.project, env.ID)
	if err != nil || currentEnv.ActiveGeneration != 3 {
		t.Fatal("late old success replaced current active generation", err)
	}
	before := second.Version
	reconciled, err := f.s.ReconcileReleaseDeployment(ctx, f.user, f.project, second.OperationID, f.cookie.Value, map[string]any{"project_id": f.project, "expected_version": before, "idempotency_key": newID()}, f.e)
	if err != nil || reconciled.Observation == nil || reconciled.Observation.EvidenceSHA256 != second.Observation.EvidenceSHA256 {
		t.Fatal("real provider read-only reconciliation failed", err)
	}
}
