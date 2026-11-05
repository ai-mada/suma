package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

type poolArtifactTraceTransport struct {
	http.RoundTripper
	resolve []byte
}

func (t *poolArtifactTraceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/pool-runner/artifacts/resolve" && r.GetBody != nil {
		body, err := r.GetBody()
		if err != nil {
			return nil, err
		}
		t.resolve, err = io.ReadAll(io.LimitReader(body, 1<<20))
		_ = body.Close()
		if err != nil {
			return nil, err
		}
	}
	return t.RoundTripper.RoundTrip(r)
}

func poolArtifactTestRef(a runner.Assignment) cc.AssignmentRef {
	return cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID,
		TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID,
		PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID,
		PoolCredentialVersion: a.PoolCredentialVersion, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
}

func TestPoolApplicationArtifactsActualBuildAndIndependentPackage(t *testing.T) {
	h := newActualApplicationFixture(t)
	f := h.f
	ctx := context.Background()
	basis := releaseSetRuntimeBuildBasis(t, h.libraryVersion.Version)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	repo := h.delivery.Repository
	separate := t.TempDir()
	workspaceGitTestCommand(t, repo, "clone", "--no-hardlinks", "--", repo, separate)
	workspaceGitTestCommand(t, separate, "fetch", "--no-tags", "--", repo, basis.SourceCommit)
	workspaceGitTestCommand(t, separate, "switch", "--detach", basis.SourceCommit)

	const buildID, testID, junitID = "build-application251", "test-package251", "test-source252"
	const output = "<!doctype html><title>Pool library</title><h1>Pool artifact</h1>"
	const buildScript = "const fs=require('node:fs');if(require('./package.json').name!=='@fixture/library')process.exit(4);fs.mkdirSync('.cache/site',{recursive:true});fs.writeFileSync('.cache/site/index.html',process.argv[1]);"
	build := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: basis.SourceCommit, Commands: map[string]runner.CommandSpec{
		buildID: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", buildScript, output}, ArtifactOutput: ".cache/site"},
	}}
	tester := runner.ExecConfig{AllowExec: true, Workspace: separate, Repository: separate, BaseCommit: basis.SourceCommit, Commands: map[string]runner.CommandSpec{
		testID: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", "const fs=require('node:fs'),p=require('node:path');if(fs.readFileSync(p.join(process.env.SUMA_ARTIFACT_DIR,'index.html'),'utf8')!==process.argv[1]){console.log('sealed bytes read; package assertion failed');process.exitCode=7;}else console.log('sealed bytes verified');", output}, ArtifactInput: true, TestEvidence: true},
	}}
	const sourceTest = `const fs=require('node:fs');const ok=require('./sum.cjs')(2,3)===5&&require('./packages/library/package.json').name==='@fixture/library';fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="pool.release" name="registered-source">'+(ok?'':'<failure message="registered source mismatch"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	junit := runner.ExecConfig{AllowExec: true, Workspace: separate, Repository: separate, BaseCommit: basis.SourceCommit, Commands: map[string]runner.CommandSpec{
		junitID: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", sourceTest, "{{test_report_path}}"}, TestEvidence: true, TestReport: true},
	}}
	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	producer, producerClient := repositoryDispatchPoolTool(t, ctx, f, pool.ID, h.control.URL, "Scoped build Pool Runner", &build)
	independent, independentClient := repositoryDispatchPoolTool(t, ctx, f, pool.ID, h.control.URL, "Independent package Pool Runner", &tester)
	source, sourceClient := repositoryDispatchPoolTool(t, ctx, f, pool.ID, h.control.URL, "Independent source JUnit Pool Runner", &junit)
	if producer.Profile.ArtifactKind != "build" || independent.Profile.ArtifactKind != "package_test" || producer.WorkerID == independent.WorkerID {
		t.Fatal("approved release profiles missing")
	}
	criteria := []string{"Check the exact registered library source and its independently tested sealed package"}
	requirement := issueRepairRuntimeDecode[Record](t, f.call("POST", "/requirements", f.body(map[string]any{"title": "Release the verified Pool library", "acceptance_criteria": criteria}), 201))
	createPublishedPlan := func(t *testing.T, title string) (Record, TestPlanView) {
		t.Helper()
		task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": title, "requirement_id": requirement.ID, "acceptance_criteria": criteria}), 201))
		plan := issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans", TestPlanCreateRequest{
			ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ExpectedRequirementVersion: requirement.Version,
			Title: "Pool library delivery checks", Description: "Source assertions and sealed-package evidence are independent gates.",
			Cases: []QualityTestCase{{Key: "registered-source", Title: "Verify the registered library source", Description: "Use the build's exact source commit",
				Preconditions: "The registered library build has finished", Steps: []string{"Check package identity and shared arithmetic"}, ExpectedResult: "Both source assertions pass",
				Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "pool.release", Name: "registered-source"}}}, IdempotencyKey: newID(),
		}, 201))
		plan = issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans/"+plan.ID+"/commands", TestPlanCommandRequest{
			ProjectID: f.project, ExpectedVersion: plan.Version, Action: "publish", Reason: "Fixture owner publishes the source test and requires independent package proof.", IdempotencyKey: newID(),
		}, 200))
		return task, plan
	}
	task, plan := createPublishedPlan(t, "Pool application artifact")
	wf := dag.Workflow{Name: "Pool scoped application release artifact", Nodes: []dag.Node{
		{ID: "build", Type: "command", Command: []string{buildID}, Config: map[string]any{"tool_profile_id": producer.ID, "tool_profile_sha256": producer.Profile.ProfileSHA256, rc.ApplicationBuildConfigKey: basis}},
		{ID: "package", Type: "test", Command: []string{testID}, DependsOn: []string{"build"}, Config: map[string]any{"artifact_from": "build", "candidate_from": "build", "tool_profile_id": independent.ID, "tool_profile_sha256": independent.Profile.ProfileSHA256}},
		{ID: "junit", Type: "test", Command: []string{junitID}, DependsOn: []string{"build"}, Config: map[string]any{"candidate_from": "build", "tool_profile_id": source.ID, "tool_profile_sha256": source.Profile.ProfileSHA256}},
	}}
	packageOnly := f.call("POST", "/workflows", f.body(map[string]any{"name": "Pool package without source report", "nodes": wf.Nodes[:2]}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": packageOnly["id"], "workflow_version": packageOnly["version"], "task_id": task.ID, "expected_version": task.Version}), 400)
	published := f.call("POST", "/workflows", f.body(map[string]any{"name": wf.Name, "nodes": wf.Nodes}), 201)
	// An application build cannot replace the very acceptance that authorizes its source.
	originID := h.libraryVersion.Version.Sources[0].TaskBasis.ID
	originPath := "/tasks/" + originID + "?project_id=" + f.project
	origin := issueRepairRuntimeDecode[Record](t, f.call("GET", originPath, nil, 200))
	if origin.Status != "accepted" {
		t.Fatalf("application source task must already be accepted: %s", origin.Status)
	}
	runCount := domainACount(t, f.s, "dag_runs")
	rejected := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": published["id"], "workflow_version": published["version"], "task_id": originID, "expected_version": origin.Version}), 409)
	if !strings.Contains(stringify(rejected), "应用构建的当前授权或源码依据已变化") {
		t.Fatalf("expected application source-basis conflict, got %+v", rejected)
	}
	originAfter := issueRepairRuntimeDecode[Record](t, f.call("GET", originPath, nil, 200))
	if originAfter.RunID != origin.RunID || originAfter.Version != origin.Version || originAfter.Status != origin.Status || domainACount(t, f.s, "dag_runs") != runCount {
		t.Fatal("rejected source-task rebind changed the original acceptance or left a new run")
	}
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": published["id"], "workflow_version": published["version"], "task_id": task.ID, "expected_version": task.Version}), 201)
	runID := run["id"].(string)
	qualityExecution := func(t *testing.T, planID, runID string) TestPlanExecution {
		t.Helper()
		return issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+planID+"/executions/"+runID+"?project_id="+f.project, nil, 200))
	}
	requestAssignment := func(client *runner.Client, profile ToolProfile, capability string) (*runner.Assignment, error) {
		return client.ClaimPool(ctx, runner.PoolClaimRequest{PoolID: pool.ID, RunnerID: client.Pool.RunnerID, WorkerID: profile.WorkerID, SlotID: "0", OperationID: newID(),
			Scopes: []runner.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{capability}}}})
	}
	claim := func(t *testing.T, runID string, client *runner.Client, profile ToolProfile, capability string) (*runner.Client, *runner.Assignment) {
		t.Helper()
		a, err := requestAssignment(client, profile, capability)
		if err != nil || a == nil || a.RunID != runID || a.PoolGrantID != grant.ID || a.PoolGrantVersion != grant.Version {
			t.Fatalf("Pool did not grant the exact artifact attempt: %+v %v", a, err)
		}
		scoped := *client
		identity := *client.Pool
		identity.Assignment = a
		scoped.Pool = &identity
		return &scoped, a
	}
	rawPost := func(client *runner.Client, path string, ref cc.AssignmentRef, body any, want int) []byte {
		t.Helper()
		data, _ := json.Marshal(body)
		envelope, _ := json.Marshal(ref)
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, h.control.URL+"/api/v1/"+path, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Authorization", "Bearer "+client.Token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString(envelope))
		resp, e := h.control.Client().Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer resp.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if e != nil || want != 0 && resp.StatusCode != want || want == 0 && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusConflict {
			t.Fatalf("%s got HTTP %d want %d: %s %v", path, resp.StatusCode, want, raw, e)
		}
		return raw
	}
	bc, ba := claim(t, runID, producerClient, producer, "command:"+buildID)
	build.Artifacts = bc
	built := runner.Execute(ctx, *ba, build)
	if built.ExitCode != 0 || built.Error != "" || built.ArtifactEvidence == nil || built.ArtifactEvidence.ApplicationBuild == nil || *built.ArtifactEvidence.ApplicationBuild != basis {
		t.Fatalf("Pool build did not seal exact app scope: exit=%d error=%s output=%q", built.ExitCode, built.Error, built.Output)
	}
	// Even a valid build lease cannot issue the package test's read authority.
	buildRef := poolArtifactTestRef(*ba)
	rawPost(bc, "pool-runner/artifacts/resolve", buildRef, rc.ArtifactResolveRequest{ProjectID: f.project, Assignment: artifactCoreAssignment(buildRef), OperationID: newID(), SourceNodeID: "build"}, http.StatusForbidden)
	if built.OperationID == "" {
		built.OperationID = newID()
	}
	if err = bc.Complete(ctx, built); err != nil {
		t.Fatal("complete exact scoped build", err)
	}
	jc, ja := claim(t, runID, sourceClient, source, "command:"+junitID)
	junit.Artifacts = jc
	reported := runner.Execute(ctx, *ja, junit)
	if reported.ExitCode != 0 || reported.Error != "" || reported.TestReport == nil || reported.ArtifactEvidence != nil || reported.WorkspaceEvidence["head_commit"] != basis.SourceCommit {
		t.Fatalf("Pool JUnit source test did not report the exact build commit: exit=%d error=%s output=%q", reported.ExitCode, reported.Error, reported.Output)
	}
	if reported.OperationID == "" {
		reported.OperationID = newID()
	}
	if err = jc.Complete(ctx, reported); err != nil {
		t.Fatal("complete independent source test", err)
	}
	pending := qualityExecution(t, plan.ID, runID)
	if pending.State != "pending" || pending.Passed != 1 || len(pending.Reports) != 1 || pending.Reports[0].NodeID != "junit" || pending.Reports[0].WorkerID != source.WorkerID || pending.Reports[0].SourceCommit != basis.SourceCommit ||
		len(pending.PackageEvidence) != 1 || pending.PackageEvidence[0].NodeID != "package" || pending.PackageEvidence[0].SourceNodeID != "build" ||
		pending.PackageEvidence[0].State != "pending" || pending.PackageEvidence[0].ArtifactID != "" || pending.PackageEvidence[0].ManifestSHA256 != "" || pending.PackageEvidence[0].SourceCommit != "" {
		t.Fatal("a passing JUnit report prematurely cleared the independent package gate", pending)
	}
	beforePackage := f.call("GET", "/tasks/"+task.ID+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": beforePackage["version"], "run_id": run["id"], "decision": "accepted", "reason": "A passing source case cannot replace the unfinished package test"}), 400)

	// A current enabled profile can still be newer than the published run's
	// frozen approval. The scheduler must not lease against that substitution.
	setPackageProfileVersion := func(from, to int) {
		t.Helper()
		res, err := f.s.DB.ExecContext(ctx, f.s.q(`UPDATE suma_tool_profiles SET version=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), to, f.user.TenantID, f.project, independent.ID, from)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			t.Fatal("fixture package profile missing", n, err)
		}
	}
	setPackageProfileVersion(independent.Version, independent.Version+1)
	if a, err := requestAssignment(independentClient, independent, "command:"+testID); err != nil || a != nil {
		t.Fatalf("profile version drift should leave the original package node unclaimed: %+v %v", a, err)
	}
	setPackageProfileVersion(independent.Version+1, independent.Version)
	tc, ta := claim(t, runID, independentClient, independent, "command:"+testID)
	tester.Artifacts = tc
	original := poolArtifactTestRef(*ta)
	resolve := rc.ArtifactResolveRequest{ProjectID: f.project, Assignment: artifactCoreAssignment(original), OperationID: newID(), SourceNodeID: "build"}
	wrong := original
	wrong.PoolGrantVersion++
	rawPost(tc, "pool-runner/artifacts/resolve", wrong, resolve, 0)
	wrong = original
	wrong.PoolSlotID = "1"
	rawPost(tc, "pool-runner/artifacts/resolve", wrong, resolve, 0)
	rawPost(bc, "pool-runner/artifacts/resolve", original, resolve, http.StatusForbidden)
	transport := &poolArtifactTraceTransport{RoundTripper: http.DefaultTransport}
	tc.HTTP = &http.Client{Transport: transport}
	tested := runner.Execute(ctx, *ta, tester)
	if tested.ExitCode != 0 || tested.Error != "" || tested.ArtifactEvidence == nil || tested.ArtifactEvidence.ApplicationBuild != nil ||
		tested.ArtifactEvidence.ArtifactID != built.ArtifactEvidence.ArtifactID || tested.ArtifactEvidence.ManifestSHA256 != built.ArtifactEvidence.ManifestSHA256 ||
		!strings.Contains(tested.Output, "sealed bytes verified") {
		t.Fatalf("Pool independent package test did not use exact sealed bytes: exit=%d error=%s output=%q", tested.ExitCode, tested.Error, tested.Output)
	}
	if err = json.Unmarshal(transport.resolve, &resolve); err != nil {
		t.Fatal("Runner resolve operation was not observed", err)
	}
	first := rawPost(tc, "pool-runner/artifacts/resolve", original, resolve, http.StatusOK)
	var replay rc.ArtifactBinding
	if err = json.Unmarshal(first, &replay); err != nil || replay.ArtifactID != built.ArtifactEvidence.ArtifactID ||
		!bytes.Equal(first, rawPost(tc, "pool-runner/artifacts/resolve", original, resolve, http.StatusOK)) {
		t.Fatal("original resolve receipt drifted", err)
	}
	if tested.OperationID == "" {
		tested.OperationID = newID()
	}
	wrongProof := tested
	wrongProof.OperationID = newID()
	wrongProof.TestReport = reported.TestReport
	if err = tc.Complete(ctx, wrongProof); err == nil {
		t.Fatal("a package attempt accepted a copied source JUnit report")
	}
	// Simulate a newer bilateral grant while this exact package attempt is
	// held. Neither an existing receipt nor a draining fact callback may
	// manufacture a new release proof under the old grant.
	setGrantVersion := func(version int) {
		t.Helper()
		res, err := f.s.DB.ExecContext(ctx, f.s.q(`UPDATE suma_execution_pool_grants SET version=? WHERE id=?`), version, grant.ID)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			t.Fatal("fixture grant missing", n, err)
		}
	}
	setGrantVersion(grant.Version + 1)
	rawPost(tc, "pool-runner/artifacts/resolve", original, resolve, http.StatusForbidden)
	if err = tc.Complete(ctx, tested); err == nil {
		t.Fatal("current grant drift accepted an old package proof")
	}
	setGrantVersion(grant.Version)
	if err = tc.Complete(ctx, tested); err != nil {
		t.Fatal("complete independent package test", err)
	}
	current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, ba.RunID)
	if err != nil || current.State != "succeeded" {
		t.Fatal("Pool release run did not finish", current.State, err)
	}
	quality := qualityExecution(t, plan.ID, runID)
	if quality.State != "passed" || !quality.BasisCurrent || quality.Total != 1 || quality.Passed != 1 || len(quality.Reports) != 1 || quality.Reports[0].NodeID != "junit" ||
		len(quality.PackageEvidence) != 1 || quality.PackageEvidence[0].NodeID != "package" || quality.PackageEvidence[0].SourceNodeID != "build" || quality.PackageEvidence[0].State != "verified" ||
		quality.PackageEvidence[0].Attempt != ta.Attempt || quality.PackageEvidence[0].WorkerID != independent.WorkerID || quality.PackageEvidence[0].SourceWorkerID != producer.WorkerID ||
		quality.PackageEvidence[0].ArtifactID != built.ArtifactEvidence.ArtifactID || quality.PackageEvidence[0].ManifestSHA256 != built.ArtifactEvidence.ManifestSHA256 || quality.PackageEvidence[0].SourceCommit != basis.SourceCommit {
		t.Fatal("quality did not preserve independent source and exact package evidence", quality)
	}
	record := f.call("GET", "/tasks/"+task.ID+"?project_id="+f.project, nil, 200)
	reviewed := f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": record["version"], "run_id": current.ID, "decision": "accepted", "reason": "Fixture owner reviews scoped build and independent sealed package"}), 200)
	artifact, err := f.s.ReleaseArtifact(ctx, f.actor, f.project, built.ArtifactEvidence.ArtifactID)
	if err != nil || len(artifact.Proofs) != 2 {
		t.Fatal("expected the two original scoped proofs", err, artifact.Proofs)
	}
	var buildProof, testProof ArtifactProof
	for _, proof := range artifact.Proofs {
		if proof.Kind == "build" {
			buildProof = proof
		} else if proof.Kind == "package_test" {
			testProof = proof
		}
	}
	if buildProof.ApplicationBuild == nil || *buildProof.ApplicationBuild != basis || buildProof.Assignment.WorkerID != producer.WorkerID || testProof.Assignment.WorkerID != independent.WorkerID {
		t.Fatal("published proof identity mismatch")
	}
	candidate, err := f.s.CreateReleaseCandidate(ctx, f.actor, f.project, f.body(map[string]any{"name": "Pool exact application candidate", "artifact_id": artifact.ID, "build_attestation_id": buildProof.ID, "package_test_attestation_id": testProof.ID, "tasks": []any{map[string]any{"id": task.ID, "expected_version": reviewed["version"]}}, "idempotency_key": newID()}))
	if err != nil || !candidate.Eligibility || candidate.Candidate.BuildProof.ApplicationBuild == nil || *candidate.Candidate.BuildProof.ApplicationBuild != basis {
		t.Fatal("scoped Pool result did not reach current candidate proof", err, candidate.Blockers)
	}
	t.Logf("pool_artifact=%s manifest_sha256=%s candidate=%s commit=%s scope=%s tree=%s build=%s test=%s", artifact.ID, artifact.ManifestSHA256, candidate.Candidate.ID, basis.SourceCommit, basis.ScopeRoot, basis.ScopeTreeOID, buildProof.Assignment.WorkerID, testProof.Assignment.WorkerID)

	t.Run("package_failure_issue_intake", func(t *testing.T) {
		originalTest := f.t
		f.t = t
		defer func() { f.t = originalTest }()
		const badBuildID = "build-package-misconfigured254"
		brokenBuild := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: basis.SourceCommit, Commands: map[string]runner.CommandSpec{
			badBuildID: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", buildScript, "<!doctype html><h1>Incorrect build output</h1>"}, ArtifactOutput: ".cache/site"},
		}}
		badProducer, badClient := repositoryDispatchPoolTool(t, ctx, f, pool.ID, h.control.URL, "Misconfigured package build Pool Runner", &brokenBuild)
		badBuildNode := dag.Node{ID: "build", Type: "command", Command: []string{badBuildID}, Config: map[string]any{
			"tool_profile_id": badProducer.ID, "tool_profile_sha256": badProducer.Profile.ProfileSHA256, rc.ApplicationBuildConfigKey: basis,
		}}
		// Both the original failure and repaired artifact run this exact approved
		// package assertion. The build configuration is the repaired input.
		packageNode := wf.Nodes[1]
		packageNode.ID = "package-failed"
		unknownNode := packageNode
		unknownNode.ID, unknownNode.DependsOn = "package-unknown", []string{"build", "junit"}
		failureWorkflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Pool package failure and unknown outcome",
			"nodes": []dag.Node{badBuildNode, packageNode, wf.Nodes[2], unknownNode}}), 201)
		failureTask, failurePlan := createPublishedPlan(t, "Pool package failure follow-up")
		failureRun := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": failureWorkflow["id"], "workflow_version": failureWorkflow["version"], "task_id": failureTask.ID, "expected_version": failureTask.Version}), 201)
		runID := failureRun["id"].(string)
		defectsPath := "/test-plans/" + failurePlan.ID + "/executions/" + runID + "/package-defects"

		bc, ba := claim(t, runID, badClient, badProducer, "command:"+badBuildID)
		brokenBuild.Artifacts = bc
		built := runner.Execute(ctx, *ba, brokenBuild)
		built.OperationID = newID()
		if built.ExitCode != 0 || built.ArtifactEvidence == nil || built.ArtifactEvidence.ManifestSHA256 == artifact.ManifestSHA256 {
			t.Fatalf("failure fixture did not seal its build: %d %s", built.ExitCode, built.Error)
		}
		if err := bc.Complete(ctx, built); err != nil {
			t.Fatal("complete failure fixture build", err)
		}

		tc, ta := claim(t, runID, independentClient, independent, "command:"+testID)
		if ta.NodeID != packageNode.ID {
			t.Fatalf("expected failed package to be claimed before the unknown sibling, got %s", ta.NodeID)
		}
		req := TestPlanPackageDefectRequest{ProjectID: f.project, ExpectedVersion: failurePlan.Version, NodeID: ta.NodeID, ExpectedAttempt: ta.Attempt, ExpectedFence: ta.Fence,
			Reason: "Verified package assertion failure on the original sealed artifact", IdempotencyKey: newID()}
		f.call("POST", defectsPath, req, 400)
		tester.Artifacts = tc
		failed := runner.Execute(ctx, *ta, tester)
		failed.OperationID = newID()
		if failed.ExitCode != 7 || failed.Stopped || failed.OutcomeUnknown || failed.ArtifactEvidence != nil || failed.TestReport != nil ||
			failed.ToolEvidence == nil || failed.ToolEvidence.Terminal != "failed" || !strings.Contains(failed.Output, "sealed bytes read") {
			t.Fatalf("real sealed package failure was lost: exit=%d error=%s output=%q", failed.ExitCode, failed.Error, failed.Output)
		}
		if err := tc.Complete(ctx, failed); err != nil {
			t.Fatal("complete exact failed package", err)
		}

		jc, ja := claim(t, runID, sourceClient, source, "command:"+junitID)
		junit.Artifacts = jc
		reported := runner.Execute(ctx, *ja, junit)
		reported.OperationID = newID()
		if reported.ExitCode != 0 || reported.TestReport == nil {
			t.Fatalf("failed package erased independent source report: %d %s", reported.ExitCode, reported.Error)
		}
		if err := jc.Complete(ctx, reported); err != nil {
			t.Fatal("complete failure fixture source test", err)
		}

		uc, ua := claim(t, runID, independentClient, independent, "command:"+testID)
		if ua.NodeID != unknownNode.ID {
			t.Fatalf("expected unknown sibling, got %s", ua.NodeID)
		}
		// The fixture owns no live process for this attempt. Report a confirmed
		// stop so the process slot frees while the result stays unknown.
		if err := uc.Complete(ctx, runner.Result{Assignment: *ua, ExitCode: -1, Stopped: true, OutcomeUnknown: true, OperationID: newID()}); err != nil {
			t.Fatal("complete unknown outcome fixture", err)
		}
		unknownReq := req
		unknownReq.NodeID, unknownReq.ExpectedAttempt, unknownReq.ExpectedFence, unknownReq.IdempotencyKey = ua.NodeID, ua.Attempt, ua.Fence, newID()
		f.call("POST", defectsPath, unknownReq, 400)
		wrongFence := req
		wrongFence.ExpectedFence, wrongFence.IdempotencyKey = req.ExpectedFence+1, newID()
		f.call("POST", defectsPath, wrongFence, 409)
		staleAttempt := req
		staleAttempt.ExpectedAttempt, staleAttempt.IdempotencyKey = req.ExpectedAttempt+1, newID()
		f.call("POST", defectsPath, staleAttempt, 409)

		execution := qualityExecution(t, failurePlan.ID, runID)
		if !execution.BasisCurrent || execution.Passed != 1 || len(execution.PackageEvidence) != 2 ||
			execution.PackageEvidence[0].NodeID != ta.NodeID || execution.PackageEvidence[0].State != "failed" || execution.PackageEvidence[0].Fence != ta.Fence ||
			!execution.PackageEvidence[0].CanCreateIssue || execution.PackageEvidence[1].State != "unknown" || execution.PackageEvidence[1].CanCreateIssue {
			t.Fatal("confirmed package failure and unknown outcome were not kept separate", execution)
		}
		taskBefore := f.call("GET", "/tasks/"+failureTask.ID+"?project_id="+f.project, nil, 200)
		f.call("POST", "/tasks/"+failureTask.ID+"/review", f.body(map[string]any{"expected_version": taskBefore["version"], "run_id": runID, "decision": "accepted", "reason": "A passing source test cannot replace a failed package gate"}), 400)
		issue := issueRepairRuntimeDecode[Record](t, f.call("POST", defectsPath, req, 201))
		if issue.Status != "open" || issue.Data["origin"] != "test_plan_package" || issue.Data["run_id"] != runID ||
			issue.Data["node_id"] != ta.NodeID || number(issue.Data, "attempt") != ta.Attempt || int64(number(issue.Data, "fence")) != ta.Fence ||
			issue.Data["artifact_id"] != built.ArtifactEvidence.ArtifactID || issue.Data["manifest_sha256"] != built.ArtifactEvidence.ManifestSHA256 ||
			issue.Data["source_commit"] != basis.SourceCommit || issue.Data["build_worker_id"] != badProducer.WorkerID {
			t.Fatal("open issue lost the original failed attempt or artifact lineage", issue)
		}
		if replay := f.call("POST", defectsPath, req, 201); replay["id"] != issue.ID {
			t.Fatal("original package issue receipt was not recovered", replay["id"])
		}
		fresh := req
		fresh.IdempotencyKey = newID()
		if repeated := f.call("POST", defectsPath, fresh, 201); repeated["id"] != issue.ID {
			t.Fatal("fresh key duplicated the protected package issue", repeated["id"])
		}
		if evidence := qualityExecution(t, failurePlan.ID, runID).PackageEvidence[0]; evidence.IssueID != issue.ID || evidence.CanCreateIssue {
			t.Fatal("quality evidence lost the protected package issue", evidence)
		}
		delivery, err := f.s.GetTaskDelivery(ctx, f.actor, f.project, failureTask.ID)
		if err != nil {
			t.Fatal(err)
		}
		linked := false
		for _, ref := range delivery.Issues.Items {
			linked = linked || ref.IssueID == issue.ID && ref.Source == "package_test"
		}
		if !linked {
			t.Fatal("task delivery lost the protected package issue", delivery.Issues)
		}
		intakePath := "/issues/" + issue.ID + "/intake"
		intakeBody := f.body(map[string]any{"expected_version": issue.Version, "title": "Repair the failed sealed-package assertion"})
		intake := f.call("POST", intakePath, intakeBody, 201)
		if repeated := f.call("POST", intakePath, intakeBody, 201); repeated["id"] != intake["id"] {
			t.Fatal("package issue intake duplicated its follow-up requirement", repeated["id"])
		}

		retired := issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans/"+failurePlan.ID+"/commands", TestPlanCommandRequest{
			ProjectID: f.project, ExpectedVersion: failurePlan.Version, Action: "retire", Reason: "Verify stale evidence cannot authorize a new defect", IdempotencyKey: newID(),
		}, 200))
		fresh.ExpectedVersion, fresh.IdempotencyKey = retired.Version, newID()
		f.call("POST", defectsPath, fresh, 409)
		t.Logf("package_failure_run=%s attempt=%d fence=%d artifact=%s issue=%s requirement=%s", runID, ta.Attempt, ta.Fence, built.ArtifactEvidence.ArtifactID, issue.ID, intake["id"])

		repairPath := "/issues/" + issue.ID + "/repair"
		historical := issueRepairRuntimeDecode[IssueRepairView](t, f.call("GET", repairPath+"?project_id="+f.project, nil, 200)).OriginalPackage
		if historical == nil || historical.RunID != runID || historical.NodeID != ta.NodeID || historical.Attempt != ta.Attempt || historical.Fence != ta.Fence ||
			historical.ArtifactID != built.ArtifactEvidence.ArtifactID || historical.ManifestSHA256 != built.ArtifactEvidence.ManifestSHA256 ||
			historical.SourceCommit != basis.SourceCommit || historical.ApplicationID != basis.ApplicationID || historical.ScopeID != basis.ScopeID ||
			historical.ScopeRoot != basis.ScopeRoot || historical.RepositoryID != h.target.ID {
			t.Fatal("historical package origin was lost when its test plan retired", historical)
		}
		prepared := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", repairPath+"/prepare", IssueRepairPrepareRequest{
			ProjectID: f.project, ExpectedIssueVersion: issue.Version, Title: "Correct the package build configuration",
			Description:        "The exact same approved package assertion must pass with the repaired build configuration.",
			AcceptanceCriteria: criteria, Reason: "Fixture owner prepares the repair of the original sealed package failure.", IdempotencyKey: newID(),
		}, 201))
		if prepared.Task == nil || prepared.Current == nil || prepared.Issue.Status != "in_progress" ||
			prepared.Task.RequirementID != intake["id"] || prepared.Current.RequirementID != intake["id"] {
			t.Fatal("package repair did not reuse the protected issue intake requirement", prepared)
		}
		repairTask := *prepared.Task
		sourceCase := failurePlan.Published.Cases[0]
		regression := issueRepairRuntimePlan(t, f, repairTask, sourceCase)
		bound := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", repairPath+"/regression", IssueRepairBindRequest{
			ProjectID: f.project, ExpectedIssueVersion: prepared.Issue.Version, ExpectedRepairVersion: prepared.Current.Version,
			ExpectedPlanVersion: regression.Version, PlanID: regression.ID, CaseKeys: []string{sourceCase.Key},
			Reason: "Pin the published repair plan and its independent source case.", IdempotencyKey: newID(),
		}, 200))
		if bound.Current == nil || bound.Current.Regression == nil || bound.Current.Regression.PlanSHA256 != regression.Published.SHA256 {
			t.Fatal("repair lost the exact regression plan", bound)
		}
		readRepair := func() IssueRepairView {
			t.Helper()
			return issueRepairRuntimeDecode[IssueRepairView](t, f.call("GET", repairPath+"?project_id="+f.project, nil, 200))
		}
		runRepair := func(workflow map[string]any) (string, runner.Result) {
			t.Helper()
			taskPath := "/tasks/" + repairTask.ID + "?project_id=" + f.project
			currentTask := f.call("GET", taskPath, nil, 200)
			run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "workflow_version": workflow["version"],
				"task_id": repairTask.ID, "expected_version": currentTask["version"]}), 201)
			repairRun := run["id"].(string)
			builder, buildAssignment := claim(t, repairRun, producerClient, producer, "command:"+buildID)
			build.Artifacts = builder
			result := runner.Execute(ctx, *buildAssignment, build)
			result.OperationID = newID()
			if result.ExitCode != 0 || result.ArtifactEvidence == nil || result.ArtifactEvidence.ManifestSHA256 == built.ArtifactEvidence.ManifestSHA256 ||
				result.ArtifactEvidence.ApplicationBuild == nil || *result.ArtifactEvidence.ApplicationBuild != basis {
				t.Fatalf("corrected configuration did not seal the changed bytes for the original scope: exit=%d error=%s", result.ExitCode, result.Error)
			}
			if err := builder.Complete(ctx, result); err != nil {
				t.Fatal("complete the corrected scope build", err)
			}
			sourceVerifier, sourceAssignment := claim(t, repairRun, sourceClient, source, "command:"+junitID)
			junit.Artifacts = sourceVerifier
			report := runner.Execute(ctx, *sourceAssignment, junit)
			report.OperationID = newID()
			if report.ExitCode != 0 || report.TestReport == nil || sourceAssignment.CandidateCommit != basis.SourceCommit ||
				sourceAssignment.WorkerID == buildAssignment.WorkerID {
				t.Fatalf("repair did not retain the independent source check: exit=%d error=%s", report.ExitCode, report.Error)
			}
			if err := sourceVerifier.Complete(ctx, report); err != nil {
				t.Fatal("complete repair source regression", err)
			}
			return repairRun, result
		}
		reviewRepair := func(runID string) {
			t.Helper()
			currentTask := f.call("GET", "/tasks/"+repairTask.ID+"?project_id="+f.project, nil, 200)
			f.call("POST", "/tasks/"+repairTask.ID+"/review", f.body(map[string]any{"expected_version": currentTask["version"],
				"run_id": runID, "decision": "accepted", "reason": "Fixture owner explicitly accepts the exact repair run."}), 200)
		}
		closeRepair := func(view IssueRepairView, runID string) IssueRepairCloseRequest {
			t.Helper()
			return IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: view.Issue.Version, ExpectedRepairVersion: view.Current.Version,
				RunID: runID, Reason: "Explicit fixture closure after checking the original package repair.", IdempotencyKey: newID()}
		}
		hasBlocker := func(view IssueRepairView, code string) bool {
			for _, blocker := range view.Blockers {
				if blocker.Code == code {
					return true
				}
			}
			return false
		}

		sourceOnly := f.call("POST", "/workflows", f.body(map[string]any{"name": "Repair source checks without sealed package regression",
			"nodes": []dag.Node{wf.Nodes[0], wf.Nodes[2]}}), 201)
		sourceOnlyRun, _ := runRepair(sourceOnly)
		reviewRepair(sourceOnlyRun)
		sourceOnlyView := readRepair()
		if sourceOnlyView.Task == nil || sourceOnlyView.Task.Status != "accepted" || sourceOnlyView.CanClose ||
			!hasBlocker(sourceOnlyView, "package_regression_missing") {
			t.Fatal("accepted source-only run bypassed the package issue gate", sourceOnlyView)
		}
		f.call("POST", repairPath+"/close", closeRepair(sourceOnlyView, sourceOnlyRun), 409)

		fullRun, repairedBuild := runRepair(published)
		pkgVerifier, pkgAssignment := claim(t, fullRun, independentClient, independent, "command:"+testID)
		tester.Artifacts = pkgVerifier
		pkgResult := runner.Execute(ctx, *pkgAssignment, tester)
		pkgResult.OperationID = newID()
		if pkgResult.ExitCode != 0 || pkgResult.ArtifactEvidence == nil || pkgResult.ToolEvidence == nil || failed.ToolEvidence.Profile.ProfileSHA256 != pkgResult.ToolEvidence.Profile.ProfileSHA256 ||
			pkgAssignment.WorkerID == repairedBuild.WorkerID || pkgResult.ArtifactEvidence.ArtifactID != repairedBuild.ArtifactEvidence.ArtifactID ||
			pkgResult.ArtifactEvidence.ManifestSHA256 != repairedBuild.ArtifactEvidence.ManifestSHA256 {
			t.Fatalf("unchanged independent package check did not verify the repaired seal: exit=%d error=%s", pkgResult.ExitCode, pkgResult.Error)
		}
		if err := pkgVerifier.Complete(ctx, pkgResult); err != nil {
			t.Fatal("complete the independent regression on the repaired package", err)
		}
		passing := qualityExecution(t, regression.ID, fullRun)
		if passing.State != "passed" || passing.Passed != 1 || len(passing.PackageEvidence) != 1 || passing.PackageEvidence[0].State != "verified" {
			t.Fatal("repair lacked independent source and package proofs", passing)
		}
		beforeReview := readRepair()
		if beforeReview.CanClose || !hasBlocker(beforeReview, "formal_review_missing") {
			t.Fatal("passing independent regression bypassed formal review", beforeReview)
		}
		reviewRepair(fullRun)
		ready := readRepair()
		if !ready.CanClose || len(ready.Blockers) != 0 || ready.Task == nil || ready.Task.RunID != fullRun {
			t.Fatal("current reviewed repair with independent proofs could not close", ready)
		}
		closeReq := closeRepair(ready, fullRun)
		closed := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", repairPath+"/close", closeReq, 200))
		if closed.Issue.Status != "closed" || closed.Current == nil || closed.Current.Closure == nil || closed.Current.Closure.RunID != fullRun ||
			closed.Current.Closure.ReviewID == "" || closed.Current.Closure.PlanSHA256 != regression.Published.SHA256 {
			t.Fatal("closure lost the exact accepted repair evidence", closed)
		}
		packageRegression := closed.Current.Closure.PackageRegression
		if packageRegression == nil || packageRegression.ProofID == "" || packageRegression.NodeID != pkgAssignment.NodeID ||
			packageRegression.Attempt != pkgAssignment.Attempt || packageRegression.Fence != pkgAssignment.Fence ||
			packageRegression.ArtifactID != repairedBuild.ArtifactEvidence.ArtifactID || packageRegression.ManifestSHA256 != repairedBuild.ArtifactEvidence.ManifestSHA256 ||
			packageRegression.SourceCommit != basis.SourceCommit || closed.OriginalPackage == nil || *closed.OriginalPackage != *historical {
			t.Fatal("closure did not retain the original failure and exact independent repaired package separately", closed)
		}
		replay := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", repairPath+"/close", closeReq, 200))
		if replay.Issue.Version != closed.Issue.Version || replay.Current == nil || replay.Current.Closure == nil || replay.Current.Closure.ClosedAt != closed.Current.Closure.ClosedAt ||
			replay.Current.Closure.PackageRegression == nil || *replay.Current.Closure.PackageRegression != *packageRegression {
			t.Fatal("close replay changed the protected repair", replay)
		}
		t.Logf("package_repair_run=%s issue=%s original_artifact=%s repaired_artifact=%s original_manifest=%s repaired_manifest=%s source_commit=%s package_proof=%s review=%s",
			fullRun, issue.ID, built.ArtifactEvidence.ArtifactID, repairedBuild.ArtifactEvidence.ArtifactID, built.ArtifactEvidence.ManifestSHA256,
			repairedBuild.ArtifactEvidence.ManifestSHA256, basis.SourceCommit, packageRegression.ProofID, closed.Current.Closure.ReviewID)
	})
}
