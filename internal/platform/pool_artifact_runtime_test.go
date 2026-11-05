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

	const buildID, testID = "build-application251", "test-package251"
	const output = "<!doctype html><title>Pool library</title><h1>Pool artifact</h1>"
	build := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: basis.SourceCommit, Commands: map[string]runner.CommandSpec{
		buildID: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", "const fs=require('node:fs');if(require('./package.json').name!=='@fixture/library')process.exit(4);fs.mkdirSync('.cache/site',{recursive:true});fs.writeFileSync('.cache/site/index.html',process.argv[1]);", output}, ArtifactOutput: ".cache/site"},
	}}
	tester := runner.ExecConfig{AllowExec: true, Workspace: separate, Repository: separate, BaseCommit: basis.SourceCommit, Commands: map[string]runner.CommandSpec{
		testID: {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", "const fs=require('node:fs'),p=require('node:path');if(fs.readFileSync(p.join(process.env.SUMA_ARTIFACT_DIR,'index.html'),'utf8')!==process.argv[1])process.exit(5);console.log('sealed bytes verified');", output}, ArtifactInput: true, TestEvidence: true},
	}}
	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	producer, producerClient := repositoryDispatchPoolTool(t, ctx, f, pool.ID, h.control.URL, "Scoped build Pool Runner", &build)
	independent, independentClient := repositoryDispatchPoolTool(t, ctx, f, pool.ID, h.control.URL, "Independent package Pool Runner", &tester)
	if producer.Profile.ArtifactKind != "build" || independent.Profile.ArtifactKind != "package_test" || producer.WorkerID == independent.WorkerID {
		t.Fatal("approved release profiles missing")
	}
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Pool application artifact", "acceptance_criteria": []string{"Verify the sealed artifact"}}), 201))
	wf := dag.Workflow{Name: "Pool scoped application release artifact", Nodes: []dag.Node{
		{ID: "build", Type: "command", Command: []string{buildID}, Config: map[string]any{"tool_profile_id": producer.ID, "tool_profile_sha256": producer.Profile.ProfileSHA256, rc.ApplicationBuildConfigKey: basis}},
		{ID: "package", Type: "test", Command: []string{testID}, DependsOn: []string{"build"}, Config: map[string]any{"artifact_from": "build", "candidate_from": "build", "tool_profile_id": independent.ID, "tool_profile_sha256": independent.Profile.ProfileSHA256}},
	}}
	published := f.call("POST", "/workflows", f.body(map[string]any{"name": wf.Name, "nodes": wf.Nodes}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": published["id"], "workflow_version": published["version"], "task_id": task.ID, "expected_version": task.Version}), 201)
	claim := func(client *runner.Client, profile ToolProfile, capability string) (*runner.Client, *runner.Assignment) {
		t.Helper()
		a, err := client.ClaimPool(ctx, runner.PoolClaimRequest{PoolID: pool.ID, RunnerID: client.Pool.RunnerID, WorkerID: profile.WorkerID, SlotID: "0", OperationID: newID(),
			Scopes: []runner.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{capability}}}})
		if err != nil || a == nil || a.RunID != run["id"] || a.PoolGrantID != grant.ID || a.PoolGrantVersion != grant.Version {
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
	bc, ba := claim(producerClient, producer, "command:"+buildID)
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

	tc, ta := claim(independentClient, independent, "command:"+testID)
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
}
