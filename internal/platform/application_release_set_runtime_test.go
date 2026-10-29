package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/deployer"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

func releaseSetRuntimeHTML(app string) string {
	return "<!doctype html><title>Owned " + app + " static demo</title><h1>" + app + ":5</h1>" + string(rune(10))
}

func applicationReleaseSetRuntimeHold(t *testing.T, h *actualApplicationFixture, receipt map[string]any) {
	actualApplicationRuntimeHold(t, h.f, h.delivery, h.manifests, receipt, actualApplicationHoldOptions{CacheDirectory: "release-set-124", DirectoryEnvironment: "SUMA_RELEASE_SET_UI_DIR", WebEnvironment: "SUMA_RELEASE_SET_WEB_DIR", Hostname: "releaseset124.localhost", Kind: "actual_application_release_set", AdditionalTables: []string{"suma_application_release_sets", "suma_application_release_set_plan_groups", "suma_application_release_set_plan_bindings", "suma_release_candidates", "suma_release_artifacts", "suma_artifact_proofs", "suma_release_plans", "suma_release_deployments", "suma_release_environments"}, ProtectedQueries: []string{`SELECT payload FROM suma_application_release_sets ORDER BY id`, `SELECT payload FROM suma_application_release_set_plan_groups ORDER BY id`, `SELECT data FROM suma_release_plans ORDER BY id`, `SELECT data FROM suma_release_deployments ORDER BY operation_id`, `SELECT data FROM suma_release_environments ORDER BY id`}})
}

// This fixture stops at prepared, unapproved original plans. Ordered-rollout
// verification can reuse the exact source/build journey without running a
// second positive deployment chain or replacing actual provider observations.
type actualReleaseSetFixture struct {
	application               *actualApplicationFixture
	candidates                map[string]ReleaseCandidate
	registered                ApplicationReleaseSetView
	group                     ApplicationReleaseSetPlanGroupView
	environments              map[string]ReleaseEnvironment
	provider                  *deployer.Server
	client                    *runner.Client
	config                    runner.ExecConfig
	providerID                string
	providerKey, transportKey string
}

func newActualReleaseSetFixture(t *testing.T, wrapControl, wrapManagement func(http.Handler) http.Handler) *actualReleaseSetFixture {
	t.Helper()
	h := newActualApplicationFixture(t)
	f := h.f
	ctx := context.Background()
	candidates := releaseSetRuntimeCandidates(t, h)
	request := ApplicationReleaseSetPreviewRequest{ProjectID: f.project, Label: "owned-pair124", Members: []ApplicationReleaseSetMemberInput{{ApplicationID: h.library.ID, ApplicationVersionID: h.libraryVersion.Version.ID, ApplicationVersionSHA256: h.libraryVersion.Version.SnapshotSHA256, CandidateID: candidates["library"].ID}, {ApplicationID: h.web.ID, ApplicationVersionID: h.webVersion.Version.ID, ApplicationVersionSHA256: h.webVersion.Version.SnapshotSHA256, CandidateID: candidates["web"].ID}}, AcknowledgeUnverifiedCompatibility: true, Reason: "Fixture owner explicitly associates each exact static-demo artifact candidate with its registered application version; this is not a new application-scoped build attestation or verified runtime compatibility."}
	preview := issueRepairRuntimeDecode[ApplicationReleaseSetPreview](t, f.call("POST", "/application-release-sets/preview", request, 200))
	if !preview.CanRegister || len(preview.Set.Members) != 2 || len(preview.Set.Dependencies) != 1 {
		t.Fatal("exact dependency-closed release set could not be previewed", preview.Blockers)
	}
	for _, member := range preview.Set.Members {
		if member.SourceCommit != h.delivery.CandidateCommit || member.BuildProof.Assignment.WorkerID == member.PackageTestProof.Assignment.WorkerID || len(member.SourceTasks) != 1 || len(member.CandidateTasks) != 1 || member.SourceTasks[0].ID == member.CandidateTasks[0].ID {
			t.Fatal("source acceptance and independently proven build-task facts were not preserved separately", member)
		}
	}
	registered := issueRepairRuntimeDecode[ApplicationReleaseSetView](t, f.call("POST", "/application-release-sets", ApplicationReleaseSetRegisterRequest{ApplicationReleaseSetPreviewRequest: request, ExpectedPreviewSHA256: preview.PreviewSHA256, IdempotencyKey: newID()}, 201))
	if !registered.BasisCurrent || registered.Set.SnapshotSHA256 == "" {
		t.Fatal("immutable release set registration lost its current basis", registered)
	}
	const providerID = "release-set-static124"
	providerKey, transportKey, reconcileKey := "provider-"+newID(), "worker-"+newID(), "reconcile-"+newID()
	t.Setenv("SUMA_SET_CONTROL124", providerKey)
	t.Setenv("SUMA_SET_TRANSPORT124", transportKey)
	t.Setenv("SUMA_SET_RECONCILE124", reconcileKey)
	f.s.ReleaseProviders = []ReleaseProviderConfig{{ID: providerID, TenantID: f.user.TenantID, ProjectID: f.project, TokenHash: digest(providerKey), SiteIDs: []string{"library-static124", "web-static124"}}}
	deploymentWorker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "release-set-deployer124", "profiles": []any{}, "deployment": true, "idempotency_key": newID()}), 201)
	client := &runner.Client{BaseURL: h.control.URL, Token: deploymentWorker["token"].(string), ProjectID: f.project}
	providerControl := h.control.URL
	if wrapControl != nil {
		wrapped := httptest.NewServer(wrapControl(f.handler))
		t.Cleanup(wrapped.Close)
		providerControl = wrapped.URL
	}
	provider, err := deployer.New(deployer.Config{ProviderID: providerID, Root: filepath.Join(t.TempDir(), "actual-set-provider"), ControlURL: providerControl, ControlTokenEnv: "SUMA_SET_CONTROL124", ReconcileTokenEnv: "SUMA_SET_RECONCILE124", Workers: map[string]string{"release-set-deployer124": "SUMA_SET_TRANSPORT124"}, Sites: map[string]deployer.SiteConfig{"library-static124": {Address: "127.0.0.1:0"}, "web-static124": {Address: "127.0.0.1:0"}}})
	if err != nil {
		t.Fatal("start actual two-site provider", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	managementHandler := provider.Handler()
	if wrapManagement != nil {
		managementHandler = wrapManagement(managementHandler)
	}
	management := httptest.NewServer(managementHandler)
	t.Cleanup(management.Close)
	f.s.ReleaseProviders[0].Origin = management.URL
	f.s.ReleaseProviders[0].ReconcileToken = reconcileKey
	environments := map[string]ReleaseEnvironment{}
	for _, app := range []string{"library", "web"} {
		environment, err := f.s.SaveReleaseEnvironment(ctx, f.actor, f.project, "", f.body(map[string]any{"name": app + " actual static demo", "provider_id": providerID, "site_id": app + "-static124", "idempotency_key": newID()}))
		if err != nil {
			t.Fatal(err)
		}
		environments[app] = environment
	}
	deployRequest := ApplicationReleaseSetDeploymentPreviewRequest{ProjectID: f.project, ExpectedSetSHA256: registered.Set.SnapshotSHA256, Environments: []ApplicationReleaseSetEnvironmentInput{{ApplicationID: h.library.ID, EnvironmentID: environments["library"].ID, ExpectedEnvironmentGeneration: 0}, {ApplicationID: h.web.ID, EnvironmentID: environments["web"].ID, ExpectedEnvironmentGeneration: 0}}, Reason: "Explicitly prepare original per-application deployment plans; no collection-wide approval or execution is granted."}
	beforeRuns := expertCount(t, f, "dag_runs")
	deploymentPreview := issueRepairRuntimeDecode[ApplicationReleaseSetDeploymentPreview](t, f.call("POST", "/application-release-sets/"+registered.Set.ID+"/deployment-preview", deployRequest, 200))
	if !deploymentPreview.CanPrepare || len(deploymentPreview.Items) != 2 {
		t.Fatal("original release-plan previews unavailable", deploymentPreview.Blockers)
	}
	group := issueRepairRuntimeDecode[ApplicationReleaseSetPlanGroupView](t, f.call("POST", "/application-release-sets/"+registered.Set.ID+"/deployment-plans", ApplicationReleaseSetDeploymentPrepareRequest{ApplicationReleaseSetDeploymentPreviewRequest: deployRequest, ExpectedPreviewSHA256: deploymentPreview.PreviewSHA256, IdempotencyKey: newID()}, 201))
	if len(group.Group.Bindings) != 2 || expertCount(t, f, "dag_runs") != beforeRuns {
		t.Fatal("preparing original plans dispatched work or lost an application")
	}
	for _, item := range group.Items {
		if item.DeploymentOperationID != "" || item.PlanState != "pending_approval" {
			t.Fatal("release-set preparation fabricated approval or a deployment", item)
		}
	}
	cfg := runner.ExecConfig{AllowExec: true, Artifacts: client, External: map[string]runner.ExternalSpec{providerID: {URL: management.URL, TokenEnv: "SUMA_SET_TRANSPORT124", Protocol: rc.ReleaseProtocol}}}
	return &actualReleaseSetFixture{application: h, candidates: candidates, registered: registered, group: group, environments: environments, provider: provider, client: client, config: cfg, providerID: providerID, providerKey: providerKey, transportKey: transportKey}
}

func TestApplicationReleaseSetActualArtifactsPlansAndIndependentDeployment(t *testing.T) {
	p := newActualReleaseSetFixture(t, nil, nil)
	h, candidates, registered, group := p.application, p.candidates, p.registered, p.group
	f, provider, client, cfg, providerID := h.f, p.provider, p.client, p.config, p.providerID
	providerKey, transportKey := p.providerKey, p.transportKey
	ctx := context.Background()
	var partial ApplicationReleaseSetView
	for index, app := range []string{"library", "web"} {
		appID := h.library.ID
		if app == "web" {
			appID = h.web.ID
		}
		var binding ApplicationReleaseSetPlanBinding
		for _, candidate := range group.Group.Bindings {
			if candidate.ApplicationID == appID {
				binding = candidate
			}
		}
		if binding.PlanID == "" {
			t.Fatal("missing original application release plan")
		}
		plan, err := f.s.ReleasePlan(ctx, f.actor, f.project, binding.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		plan, err = f.s.ApproveReleasePlan(ctx, f.actor, f.project, plan.ID, f.body(map[string]any{"expected_version": plan.Version, "plan_revision": plan.Revision, "plan_sha256": plan.SHA256, "decision": "approved", "reason": "Fixture owner separately approves this exact application static-demo plan, not the collection.", "idempotency_key": newID()}))
		if err != nil {
			t.Fatal(err)
		}
		deployment, err := f.s.ExecuteReleasePlan(ctx, f.actor, f.project, plan.ID, releaseExecuteBody(f, plan), f.e)
		if err != nil {
			t.Fatal(err)
		}
		a, err := client.Claim(ctx, "release-set-deployer124", []string{"external_workflow:" + providerID})
		if err != nil || a == nil || a.ReleaseExecution == nil || a.ReleaseExecution.OperationID != deployment.OperationID {
			t.Fatal("actual original deployment claim missing", err)
		}
		result := runner.Execute(ctx, *a, cfg)
		if result.OperationID == "" {
			result.OperationID = newID()
		}
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ReleaseEvidence == nil {
			t.Fatalf("actual %s deployment failed: exit=%d error=%s", app, result.ExitCode, result.Error)
		}
		if err = client.Complete(ctx, result); err != nil {
			t.Fatal("actual provider receipt rejected", err)
		}
		actual, err := f.s.ReleaseDeployment(ctx, f.actor, f.project, deployment.OperationID)
		if err != nil || actual.State != "deployed_healthy" || actual.Observation == nil || actual.Observation.Effect.State != "applied" || actual.Observation.Health.State != "passed" || actual.Observation.Effect.ActiveArtifactSHA256 != candidates[app].ArtifactSHA256 {
			t.Fatal("original deployment lacks exact active/health evidence", app, err)
		}
		response, err := http.Get(provider.SiteURL(app+"-static124") + "/")
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 4096))
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || string(body) != releaseSetRuntimeHTML(app) {
			t.Fatal("actual served bytes differ from this application's sealed package", app, err)
		}
		current := issueRepairRuntimeDecode[ApplicationReleaseSetView](t, f.call("GET", "/application-release-sets/"+registered.Set.ID+"?project_id="+f.project, nil, 200))
		if index == 0 {
			partial = current
			deployed, pending := 0, 0
			for _, view := range current.PlanGroups {
				for _, item := range view.Items {
					if item.DeploymentState == "deployed_healthy" {
						deployed++
					}
					if item.DeploymentOperationID == "" {
						pending++
					}
				}
			}
			if deployed != 1 || pending != 1 {
				t.Fatal("one deployment incorrectly made the complete set look deployed", current)
			}
		}
	}
	final := issueRepairRuntimeDecode[ApplicationReleaseSetView](t, f.call("GET", "/application-release-sets/"+registered.Set.ID+"?project_id="+f.project, nil, 200))
	healthy := 0
	for _, view := range final.PlanGroups {
		for _, item := range view.Items {
			if item.DeploymentState == "deployed_healthy" && item.EffectState == "applied" && item.HealthState == "passed" {
				healthy++
			}
		}
	}
	if healthy != 2 || final.Set.SnapshotSHA256 != registered.Set.SnapshotSHA256 {
		t.Fatal("final per-application deployment facts lost immutable set identity", final)
	}
	if raw, _ := json.Marshal(final); strings.Contains(string(raw), providerKey) || strings.Contains(string(raw), transportKey) || strings.Contains(string(raw), "content_base64") {
		t.Fatal("release set leaked provider/source transport data")
	}
	receipt := map[string]any{"stage": "T-RELEASESET-124", "release_set_id": registered.Set.ID, "release_set_sha256": registered.Set.SnapshotSHA256, "plan_group_id": group.Group.ID, "candidate_commit": h.delivery.CandidateCommit, "source_task_id": h.task.ID, "build_task_id": candidates["library"].Tasks[0].ID, "library_candidate_id": candidates["library"].ID, "web_candidate_id": candidates["web"].ID, "library_artifact_sha256": candidates["library"].ArtifactSHA256, "web_artifact_sha256": candidates["web"].ArtifactSHA256, "partial": partial, "final": final, "model_calls": 0, "static_demo_only": true, "customer_human_signoff": false}
	t.Logf("application_release_set_actual_receipt=%s", stringify(receipt))
	applicationReleaseSetRuntimeHold(t, h, receipt)
}

// One existing DAG produces two independently proven artifacts at the unchanged
// registered commit. Source JUnit and sealed-package HTTP tests remain separate.
func releaseSetRuntimeCandidates(t *testing.T, h *actualApplicationFixture) map[string]ReleaseCandidate {
	t.Helper()
	f := h.f
	ctx := context.Background()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	criteria := []string{"The library static demo has exact-source JUnit and independent sealed-package HTTP proof", "The web static demo has exact-source JUnit and independent sealed-package HTTP proof"}
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "Build the registered static demonstration pair", "acceptance_criteria": criteria}), 201)
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Build exact registered application sources", "requirement_id": requirement["id"], "acceptance_criteria": criteria}), 201))
	const buildScript = `const fs=require('node:fs'),path=require('node:path');const app=process.argv[1];if(!['library','web'].includes(app)||require('./sum.cjs')(2,3)!==5)throw Error('wrong exact source');const manifest=app==='library'?'packages/library/package.json':'apps/web/package.json';if(JSON.parse(fs.readFileSync(manifest,'utf8')).name!=='@fixture/'+app)throw Error('wrong source scope');const out=path.join('.cache','release124',app);fs.mkdirSync(out,{recursive:true});fs.writeFileSync(path.join(out,'index.html'),'<!doctype html><title>Owned '+app+' static demo</title><h1>'+app+':5</h1>'+String.fromCharCode(10));console.log('Owned exact-source static package built.');`
	const sourceTest = `const fs=require('node:fs');const app=process.argv[1],manifest=app==='library'?'packages/library/package.json':'apps/web/package.json';const ok=require('./sum.cjs')(2,3)===5&&JSON.parse(fs.readFileSync(manifest,'utf8')).name==='@fixture/'+app;fs.writeFileSync(process.argv[2],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="releaseset124" name="'+app+'">'+(ok?'':'<failure message="exact source failed"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	const packageTest = `const fs=require('node:fs'),http=require('node:http'),path=require('node:path');const app=process.argv[1],expected='<!doctype html><title>Owned '+app+' static demo</title><h1>'+app+':5</h1>'+String.fromCharCode(10);(async()=>{const bytes=fs.readFileSync(path.join(process.env.SUMA_ARTIFACT_DIR,'index.html'));if(bytes.toString()!==expected)throw Error('wrong sealed bytes');const server=http.createServer((req,res)=>{res.writeHead(200,{'Content-Type':'text/html'});res.end(bytes)});await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));try{const response=await fetch('http://127.0.0.1:'+server.address().port+'/');if(response.status!==200||await response.text()!==expected)throw Error('package HTTP failed');console.log('Owned sealed package HTTP checked.')}finally{await new Promise(resolve=>server.close(resolve))}})().catch(()=>{console.error('owned_package_check_failed');process.exitCode=1});`
	base := runner.ExecConfig{AllowExec: true, Workspace: h.delivery.Repository, Repository: h.delivery.Repository, BaseCommit: h.delivery.CandidateCommit}
	builder := base
	builder.Commands = map[string]runner.CommandSpec{"source124": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", `if(require('./sum.cjs')(2,3)!==5)throw Error('wrong accepted source');console.log('Owned unchanged source verified.');`}}, "build-library124": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", buildScript, "library"}, ArtifactOutput: ".cache/release124/library"}, "build-web124": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", buildScript, "web"}, ArtifactOutput: ".cache/release124/web"}}
	worker, token, profiles := releaseSetRuntimeWorker(t, f, "release-set-builder124", &builder)
	configs := map[string]runner.ExecConfig{}
	clients := map[string]*runner.Client{}
	toolProfiles := map[string]ToolProfile{}
	workers := map[string]string{}
	for _, app := range []string{"library", "web"} {
		cfg := base
		cfg.Commands = map[string]runner.CommandSpec{"package-" + app + "124": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", packageTest, app}, ArtifactInput: true, TestEvidence: true}, "junit-" + app + "124": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", sourceTest, app, "{{test_report_path}}"}, TestReport: true, TestEvidence: true}}
		id, key, pinned := releaseSetRuntimeWorker(t, f, "release-set-independent-"+app+"124", &cfg)
		client := &runner.Client{BaseURL: h.control.URL, Token: key, ProjectID: f.project}
		cfg.Artifacts = client
		configs[app], clients[app], workers[app] = cfg, client, id
		for command, profile := range pinned {
			toolProfiles[command] = profile
		}
	}
	client := &runner.Client{BaseURL: h.control.URL, Token: token, ProjectID: f.project}
	builder.Artifacts = client
	cases := []QualityTestCase{}
	for i, app := range []string{"library", "web"} {
		cases = append(cases, QualityTestCase{Key: app + "-source124", Title: app + " exact source", Description: "Source JUnit is separate from the sealed-package HTTP proof", Preconditions: "Use the unchanged registered commit", Steps: []string{"Check arithmetic and package identity"}, ExpectedResult: "Correct arithmetic and exact package identity", Priority: "high", Covers: []int{i + 1}, Selector: tr.TestCaseSelector{ClassName: "releaseset124", Name: app}})
	}
	plan := issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans", TestPlanCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ExpectedRequirementVersion: int(requirement["version"].(float64)), Title: "Registered source checks", Description: "Independent source checks supplement actual package HTTP proofs", Cases: cases, IdempotencyKey: newID()}, 201))
	plan = issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans/"+plan.ID+"/commands", TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Action: "publish", Reason: "Fixture owner publishes the exact independent source checks.", IdempotencyKey: newID()}, 200))
	nodes := []any{map[string]any{"id": "source", "type": "command", "command": []string{"source124"}, "config": map[string]any{"tool_profile_id": profiles["source124"].ID, "tool_profile_sha256": profiles["source124"].Profile.ProfileSHA256}}}
	for _, app := range []string{"library", "web"} {
		buildID, pkgID, junitID := "build-"+app+"124", "package-"+app+"124", "junit-"+app+"124"
		nodes = append(nodes, map[string]any{"id": "build-" + app, "type": "command", "command": []string{buildID}, "depends_on": []string{"source"}, "config": map[string]any{"candidate_from": "source", "tool_profile_id": profiles[buildID].ID, "tool_profile_sha256": profiles[buildID].Profile.ProfileSHA256}}, map[string]any{"id": "package-" + app, "type": "test", "command": []string{pkgID}, "depends_on": []string{"build-" + app}, "config": map[string]any{"candidate_from": "build-" + app, "artifact_from": "build-" + app, "tool_profile_id": toolProfiles[pkgID].ID, "tool_profile_sha256": toolProfiles[pkgID].Profile.ProfileSHA256}}, map[string]any{"id": "junit-" + app, "type": "test", "command": []string{junitID}, "depends_on": []string{"build-" + app}, "config": map[string]any{"candidate_from": "source", "tool_profile_id": toolProfiles[junitID].ID, "tool_profile_sha256": toolProfiles[junitID].Profile.ProfileSHA256}})
	}
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Exact registered source to two independently checked static packages", "nodes": nodes}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "workflow_version": wf["version"], "task_id": task.ID, "expected_version": task.Version}), 201)
	execute := func(client *runner.Client, id string, cfg runner.ExecConfig, commands []string, count int) {
		t.Helper()
		for i := 0; i < count; i++ {
			a, err := client.Claim(ctx, id, commands)
			if err != nil || a == nil || a.RunID != run["id"] {
				t.Fatal("actual fixed build/test claim missing", id, err)
			}
			result := runner.Execute(ctx, *a, cfg)
			if result.OperationID == "" {
				result.OperationID = newID()
			}
			if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.WorkspaceEvidence["head_commit"] != h.delivery.CandidateCommit {
				t.Fatalf("actual package execution failed node=%s exit=%d error=%s output=%q", a.NodeID, result.ExitCode, result.Error, result.Output[:min(len(result.Output), 3000)])
			}
			if err = client.Complete(ctx, result); err != nil {
				t.Fatal("actual package result rejected", a.NodeID, err)
			}
		}
	}
	execute(client, worker, builder, []string{"command:source124", "command:build-library124", "command:build-web124"}, 3)
	for _, app := range []string{"library", "web"} {
		execute(clients[app], workers[app], configs[app], []string{"command:package-" + app + "124", "command:junit-" + app + "124"}, 2)
	}
	quality := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+plan.ID+"/executions/"+run["id"].(string)+"?project_id="+f.project, nil, 200))
	if quality.State != "passed" || !quality.BasisCurrent || quality.Passed != 2 {
		t.Fatal("independent source JUnit facts missing", quality)
	}
	current := f.call("GET", "/tasks/"+task.ID+"?project_id="+f.project, nil, 200)
	accepted := f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": current["version"], "run_id": run["id"], "decision": "accepted", "reason": "Fixture owner reviews source JUnit and separate sealed-package HTTP proofs; no production sign-off is claimed."}), 200)
	artifacts, _, _, err := f.s.ReleaseArtifacts(ctx, f.actor, f.project, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ReleaseCandidate{}
	for _, app := range []string{"library", "web"} {
		var artifactID, buildProof, packageProof string
		for _, artifact := range artifacts {
			for _, proof := range artifact.Proofs {
				if proof.Assignment.RunID == run["id"] && proof.Assignment.NodeID == "build-"+app {
					artifactID, buildProof = artifact.ID, proof.ID
				}
			}
			if artifact.ID == artifactID {
				for _, proof := range artifact.Proofs {
					if proof.Kind == "package_test" {
						if proof.Assignment.WorkerID == worker {
							t.Fatal("package test reused builder")
						}
						packageProof = proof.ID
					}
				}
			}
		}
		if artifactID == "" || buildProof == "" || packageProof == "" {
			t.Fatal("actual build/package proofs missing", app)
		}
		view, err := f.s.CreateReleaseCandidate(ctx, f.actor, f.project, f.body(map[string]any{"name": app + " owned static demo", "artifact_id": artifactID, "build_attestation_id": buildProof, "package_test_attestation_id": packageProof, "tasks": []any{map[string]any{"id": accepted["id"], "expected_version": accepted["version"]}}, "idempotency_key": newID()}))
		if err != nil || !view.Eligibility || view.Candidate.SourceCommit != h.delivery.CandidateCommit {
			t.Fatal("candidate differs from registered code", app, err, view.Blockers, view.Candidate.SourceCommit)
		}
		out[app] = view.Candidate
	}
	if out["library"].ArtifactID == out["web"].ArtifactID || out["library"].ArtifactSHA256 == out["web"].ArtifactSHA256 {
		t.Fatal("the two actual packages were not distinct")
	}
	return out
}

func releaseSetRuntimeWorker(t *testing.T, f *fixture, id string, cfg *runner.ExecConfig) (string, string, map[string]ToolProfile) {
	t.Helper()
	ctx := context.Background()
	inventory, err := runner.DescribeTools(ctx, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	releaseProfiles := []ReleaseProfileRegistration{}
	for command, spec := range cfg.Commands {
		kind := ""
		if spec.ArtifactOutput != "" {
			kind = "build"
		}
		if spec.ArtifactInput {
			kind = "package_test"
		}
		if kind != "" {
			definition := rc.CommandProfile{ID: command, Kind: kind, Executable: spec.Executable, Args: spec.Args, Env: spec.Env, EnvFrom: spec.EnvFrom, Input: spec.Input, ReadOnly: spec.ReadOnly, TestEvidence: spec.TestEvidence, ArtifactOutput: spec.ArtifactOutput, ArtifactInput: spec.ArtifactInput}
			sha, err := rc.ProfileDigest(definition)
			if err != nil {
				t.Fatal(err)
			}
			releaseProfiles = append(releaseProfiles, ReleaseProfileRegistration{ID: command, Kind: kind, SHA256: sha})
		}
	}
	registered := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": id, "profiles": releaseProfiles, "deployment": false, "idempotency_key": newID()}), 201)
	token := registered["token"].(string)
	profiles := map[string]ToolProfile{}
	for _, observation := range inventory {
		if !observation.Available {
			t.Fatal("fixed build/test profile unavailable", observation.Reason)
		}
		profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: id, Name: "Owned application package " + observation.Profile.CommandID, Profile: observation.Profile, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profiles[observation.Profile.CommandID] = profile
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: id, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	for command, profile := range profiles {
		profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profiles[command] = profile
	}
	cfg.ToolInventory = inventory
	return id, token, profiles
}
