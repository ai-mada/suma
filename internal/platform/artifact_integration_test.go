package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/runner"
)

const releaseFixtureHTML = "<!doctype html><title>Suma release C</title><h1>actual candidate C</h1>\n"

// Child processes are real allowlisted build/test commands. The package test
// serves the actual downloaded sealed files and asserts an actual HTTP body.
func TestReleaseFixtureProcess(t *testing.T) {
	mode := os.Getenv("SUMA_RELEASE_FIXTURE_PROCESS")
	if mode == "" {
		return
	}
	fail := func(code int, err any) { fmt.Fprintln(os.Stderr, err); os.Exit(code) }
	switch mode {
	case "develop":
		if err := os.WriteFile("site.html", []byte(releaseFixtureHTML), 0600); err != nil {
			fail(31, err)
		}
		for _, args := range [][]string{{"add", "site.html"}, {"commit", "-m", "fixture: candidate C"}} {
			cmd := exec.Command("git", args...)
			if b, err := cmd.CombinedOutput(); err != nil {
				fail(32, string(b))
			}
		}
	case "build":
		b, err := os.ReadFile("site.html")
		if err != nil || string(b) != releaseFixtureHTML {
			fail(33, "build did not receive source C")
		}
		if err = os.MkdirAll(".cache/site", 0700); err != nil {
			fail(34, err)
		}
		if err = os.WriteFile(".cache/site/index.html", b, 0600); err != nil {
			fail(35, err)
		}
	case "package_test":
		b, err := os.ReadFile("site.html")
		if err != nil || string(b) != releaseFixtureHTML {
			fail(36, "test harness is not at source C")
		}
		server := httptest.NewServer(http.FileServer(http.Dir(os.Getenv("SUMA_ARTIFACT_DIR"))))
		response, err := server.Client().Get(server.URL + "/index.html")
		if err != nil {
			server.Close()
			fail(37, err)
		}
		b, err = io.ReadAll(response.Body)
		response.Body.Close()
		server.Close()
		if err != nil || response.StatusCode != 200 || string(b) != releaseFixtureHTML {
			fail(38, "actual package HTTP contract failed")
		}
	default:
		fail(39, "unknown fixture mode")
	}
	fmt.Println("actual process completed:", mode)
	os.Exit(0)
}

func releaseGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "empty-global-config"), "GIT_AUTHOR_NAME=Suma fixture", "GIT_AUTHOR_EMAIL=fixture@example.test", "GIT_COMMITTER_NAME=Suma fixture", "GIT_COMMITTER_EMAIL=fixture@example.test")
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture Git %v: %v %s", args, err, b)
	}
	return strings.TrimSpace(string(b))
}

// newVerifiedReleaseCandidate performs the full real protocol. Other release
// tests may reuse it; no proof, accepted task, byte receipt or success is seeded.
func newVerifiedReleaseCandidate(t *testing.T) (*fixture, ReleaseCandidate, string) {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	repo := t.TempDir()
	releaseGit(t, repo, "init", "--initial-branch=main")
	for name, content := range map[string]string{".gitignore": ".cache/\n", "site.html": "base B\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	releaseGit(t, repo, "add", ".gitignore", "site.html")
	releaseGit(t, repo, "commit", "-m", "fixture base B")
	base := releaseGit(t, repo, "rev-parse", "HEAD")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := func(mode string) runner.CommandSpec {
		return runner.CommandSpec{Executable: exe, Args: []string{"-test.run=^TestReleaseFixtureProcess$", "--"}, Env: map[string]string{"SUMA_RELEASE_FIXTURE_PROCESS": mode, "GIT_AUTHOR_NAME": "Suma fixture", "GIT_AUTHOR_EMAIL": "fixture@example.test", "GIT_COMMITTER_NAME": "Suma fixture", "GIT_COMMITTER_EMAIL": "fixture@example.test", "GIT_CONFIG_NOSYSTEM": "1"}}
	}
	develop := command("develop")
	build := command("build")
	build.ArtifactOutput = ".cache/site"
	test := command("package_test")
	test.ArtifactInput = true
	test.TestEvidence = true
	profile := func(id, kind string, s runner.CommandSpec) ReleaseProfileRegistration {
		p := rc.CommandProfile{ID: id, Kind: kind, Executable: s.Executable, Args: s.Args, Env: s.Env, EnvFrom: s.EnvFrom, Input: s.Input, ReadOnly: s.ReadOnly, TestEvidence: s.TestEvidence, ArtifactOutput: s.ArtifactOutput, ArtifactInput: s.ArtifactInput}
		h, e := rc.ProfileDigest(p)
		if e != nil {
			t.Fatal(e)
		}
		return ReleaseProfileRegistration{ID: id, Kind: kind, SHA256: h}
	}
	profiles := []ReleaseProfileRegistration{profile("site-build", "build", build), profile("site-test", "package_test", test)}
	registered, err := f.s.RegisterReleaseWorker(ctx, f.user, f.project, f.cookie.Value, map[string]any{"worker_id": "release-fixture-worker", "profiles": profiles, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	token := registered["token"].(string)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "实际静态包交付", "acceptance_criteria": []string{"固定C构建", "下载封存包实际HTTP测试"}}), 201)
	wf := dag.Workflow{Name: "真实发布制品流水线", Nodes: []dag.Node{{ID: "develop", Name: "开发C", Type: "command", Command: []string{"develop"}}, {ID: "build", Name: "固定C构建", Type: "command", Command: []string{"site-build"}, DependsOn: []string{"develop"}, Config: map[string]any{"candidate_from": "develop"}}, {ID: "package-test", Name: "实际包HTTP测试", Type: "test", Command: []string{"site-test"}, DependsOn: []string{"build"}, Config: map[string]any{"artifact_from": "build"}}}}
	pub := f.call("POST", "/workflows", f.body(map[string]any{"name": wf.Name, "nodes": wf.Nodes}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": pub["id"], "task_id": task["id"], "expected_task_version": task["version"]}), 201)
	server := httptest.NewServer(f.handler)
	t.Cleanup(server.Close)
	client := &runner.Client{BaseURL: server.URL, Token: token, ProjectID: f.project}
	journal, err := runner.OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = journal.Close() })
	config := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"develop": develop, "site-build": build, "site-test": test}, Artifacts: client, Journal: journal}
	for _, want := range []string{"develop", "build", "package-test"} {
		a, err := client.Claim(ctx, "release-fixture-worker", []string{"command", "test"})
		if err != nil || a == nil || a.NodeID != want {
			t.Fatalf("claim %s: %+v %v", want, a, err)
		}
		if err = journal.Accept(*a); err != nil {
			t.Fatal(err)
		}
		result := runner.Execute(ctx, *a, config)
		if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown {
			t.Fatalf("actual %s: %+v", want, result)
		}
		if err = journal.Finish(result); err != nil {
			t.Fatal(err)
		}
		if err = runner.Replay(ctx, client, journal); err != nil {
			t.Fatalf("persist actual %s: %v", want, err)
		}
	}
	if original := releaseGit(t, repo, "rev-parse", "HEAD"); original != base {
		t.Fatal("original checkout HEAD changed")
	}
	b, _ := os.ReadFile(filepath.Join(repo, "site.html"))
	if string(b) != "base B\n" {
		t.Fatal("original checkout file changed")
	}
	current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run["id"].(string))
	if err != nil || current.State != "succeeded" {
		t.Fatalf("actual run: %+v %v", current, err)
	}
	bound := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	accepted := f.call("POST", "/tasks/"+task["id"].(string)+"/review", f.body(map[string]any{"expected_version": bound["version"], "run_id": current.ID, "decision": "accepted", "reason": "实际构建及封存包 HTTP 测试已核对"}), 200)
	artifacts, _, _, err := f.s.ReleaseArtifacts(ctx, f.user, f.project, 20, "")
	if err != nil || len(artifacts) != 1 || len(artifacts[0].Proofs) != 2 {
		t.Fatalf("derived proofs: %+v %v", artifacts, err)
	}
	var buildID, testID string
	for _, p := range artifacts[0].Proofs {
		if p.Kind == "build" {
			buildID = p.ID
		} else if p.Kind == "package_test" {
			testID = p.ID
		}
	}
	view, err := f.s.CreateReleaseCandidate(ctx, f.user, f.project, map[string]any{"name": "真实固定C候选", "artifact_id": artifacts[0].ID, "build_attestation_id": buildID, "package_test_attestation_id": testID, "tasks": []any{map[string]any{"id": accepted["id"], "expected_version": accepted["version"]}}, "idempotency_key": newID()})
	if err != nil || !view.Eligibility {
		t.Fatal("candidate from actual pipeline", err)
	}
	return f, view.Candidate, token
}

func TestArtifactsRealBytesDerivedProofAndAcceptedCandidate(t *testing.T) {
	f, candidate, _ := newVerifiedReleaseCandidate(t)
	ctx := context.Background()
	view, err := f.s.ReleaseCandidate(ctx, f.user, f.project, candidate.ID)
	if err != nil || !view.Eligibility {
		t.Fatal(err)
	}
	data, err := json.Marshal(view)
	if err != nil || strings.Contains(string(data), "release-fixture-worker-token") || strings.Contains(string(data), "storage_path") {
		t.Fatal("private artifact data leaked", err)
	}
	if candidate.BuildProof.SourceCommit != candidate.PackageTestProof.SourceCommit || candidate.BuildProof.ManifestSHA256 != candidate.PackageTestProof.ManifestSHA256 {
		t.Fatal("proof basis mismatch")
	}
	task := f.call("GET", "/tasks/"+candidate.Tasks[0].ID+"?project_id="+f.project, nil, 200)
	f.call("PATCH", "/tasks/"+candidate.Tasks[0].ID, f.body(map[string]any{"title": "范围已变化", "expected_version": task["version"]}), 200)
	view, err = f.s.ReleaseCandidate(ctx, f.user, f.project, candidate.ID)
	if err != nil || view.Eligibility || len(view.Blockers) == 0 {
		t.Fatalf("stale candidate eligible: %+v %v", view, err)
	}
}
