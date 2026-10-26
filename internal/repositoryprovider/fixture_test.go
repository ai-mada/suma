package repositoryprovider

import (
	"bytes"
	"context"
	"encoding/json"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type gitFixture struct {
	s                          *Server
	cfg                        Config
	target, source, base, head string
	bundle                     []byte
	server                     *httptest.Server
	executions                 map[string]rc.RepositoryExecution
	mu                         sync.Mutex
	effects, observations      int
}

func newGitFixture(t *testing.T) *gitFixture {
	return newGitFixtureContent(t, "candidate\n")
}
func newGitFixtureContent(t *testing.T, content string) *gitFixture {
	t.Helper()
	git, e := exec.LookPath("git")
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.git"), filepath.Join(dir, "target.git")
	command := func(repo, input string, args ...string) string {
		t.Helper()
		c := exec.Command(git, args...)
		c.Dir = repo
		c.Stdin = strings.NewReader(input)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("Git fixture: %v: %s", e, b)
		}
		return strings.TrimSpace(string(b))
	}
	command(dir, "", "init", "--bare", source)
	commit := func(content, parent string) string {
		blob := command(source, content, "hash-object", "-w", "--stdin")
		tree := command(source, "100644 blob "+blob+"\tfile.txt\n", "mktree")
		args := []string{"commit-tree", tree}
		if parent != "" {
			args = append(args, "-p", parent)
		}
		return command(source, "fixture\n", args...)
	}
	base := commit("base\n", "")
	command(source, "", "update-ref", "refs/heads/main", base)
	command(source, "", "symbolic-ref", "HEAD", "refs/heads/main")
	command(dir, "", "clone", "--bare", "--no-hardlinks", source, target)
	head := commit(content, base)
	command(source, "", "update-ref", "refs/heads/main", head, base)
	bundlePath := filepath.Join(dir, "candidate.bundle")
	command(source, "", "bundle", "create", bundlePath, "HEAD", "^"+base)
	bundle, e := os.ReadFile(bundlePath)
	if e != nil {
		t.Fatal(e)
	}
	f := &gitFixture{target: target, source: source, base: base, head: head, bundle: bundle, executions: map[string]rc.RepositoryExecution{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch strings.TrimPrefix(r.URL.Path, "/api/v1/repository-provider/") {
		case "inspect-authorization":
			w.WriteHeader(404)
		case "consume":
			var q rc.RepositoryConsumeAuthorization
			_ = json.NewDecoder(r.Body).Decode(&q)
			e := f.executions[q.OperationID]
			_ = json.NewEncoder(w).Encode(rc.RepositoryAuthorizationGrant{AuthorizationID: "authorization-fixed", OperationID: q.OperationID, ProviderID: e.ProviderID, Assignment: q.Assignment, Execution: e, RequestSHA256: q.RequestSHA256, State: "consumed"})
		case "bundle":
			_, _ = w.Write(f.bundle)
		case "begin-effect":
			var q rc.RepositoryBeginEffect
			_ = json.NewDecoder(r.Body).Decode(&q)
			f.effects++
			_ = json.NewEncoder(w).Encode(rc.RepositoryEffectGrant{EffectGrantID: "effect-fixed", OperationID: q.OperationID, Generation: q.Generation, State: "granted"})
		case "observations":
			var o rc.RepositoryObservation
			_ = json.NewDecoder(r.Body).Decode(&o)
			f.observations++
			w.WriteHeader(204)
		case "consume-reconciliation":
			var q rc.RepositoryConsumeReconciliation
			_ = json.NewDecoder(r.Body).Decode(&q)
			_ = json.NewEncoder(w).Encode(rc.RepositoryReconciliationGrant{ReconciliationID: "reconcile-fixed", OperationID: q.OperationID, RequestSHA256: q.RequestSHA256, Generation: q.Execution.Generation, State: "granted", Execution: q.Execution, Assignment: q.Assignment})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	t.Setenv("REPO_CONTROL_TEST", strings.Repeat("c", 32))
	t.Setenv("REPO_RUNNER_TEST", strings.Repeat("r", 32))
	t.Setenv("REPO_RECON_TEST", strings.Repeat("x", 32))
	f.cfg = Config{ProviderID: "provider-test", TenantID: "tenant", ProjectID: "project", ControlURL: f.server.URL, ControlTokenEnv: "REPO_CONTROL_TEST", RunnerTokenEnv: "REPO_RUNNER_TEST", ReconcileTokenEnv: "REPO_RECON_TEST", GitExecutable: git, StateDir: filepath.Join(dir, "state"), Targets: map[string]TargetConfig{"main": {BareRepository: target, Ref: "refs/heads/main"}}}
	f.s, e = New(f.cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if f.s != nil {
			f.s.Close()
		}
	})
	return f
}
func (f *gitFixture) request(t *testing.T, phase, id, prepared string) rc.RepositoryRequest {
	t.Helper()
	o, e := f.s.Target(context.Background(), "main")
	if e != nil {
		t.Fatal(e)
	}
	execution := rc.RepositoryExecution{Protocol: rc.RepositoryProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "provider-test", TargetID: "target", TargetAlias: "main", TargetRef: o.Ref, TargetConfigSHA256: o.ConfigSHA256, PlanID: "plan", PlanVersion: 1, Phase: phase, OperationID: id, ExpectedTargetHead: f.base, ExpectedFenceOID: o.FenceOID, ExpectedGitGeneration: o.Generation, Generation: o.Generation + 1, PreparedSHA256: prepared, Source: cc.CandidateBinding{SchemaVersion: 1, TransferID: "transfer", ProjectID: "project", State: "sealed", Source: cc.CandidateAssignmentRef{RunID: "closed-source", NodeID: "develop", Attempt: 1, Fence: 1, WorkerID: "source-worker"}, BaseCommit: f.base, HeadCommit: f.head, EvidenceSHA256: strings.Repeat("a", 64), BundleSHA256: hashBytes(f.bundle), SizeBytes: int64(len(f.bundle))}}
	f.executions[id] = execution
	return rc.RepositoryRequest{RepositoryExecution: execution, Assignment: cc.CandidateAssignmentRef{RunID: "integration-run", NodeID: phase, Attempt: 1, Fence: 1, WorkerID: "worker"}, AuthorizationTicket: "private-ticket"}
}
func providerCall(t *testing.T, s *Server, method, path string, body any, reconcile bool) (int, rc.RepositoryObservation) {
	t.Helper()
	var data io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		data = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, data)
	token := strings.Repeat("r", 32)
	if reconcile {
		token = strings.Repeat("x", 32)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	var o rc.RepositoryObservation
	_ = json.Unmarshal(w.Body.Bytes(), &o)
	if w.Code >= 500 {
		t.Logf("provider error: %s", w.Body.String())
	}
	return w.Code, o
}
func prepareFixture(t *testing.T, f *gitFixture) string {
	t.Helper()
	q := f.request(t, "prepare", "prepare-0001", "")
	code, o := providerCall(t, f.s, "POST", "/operations", q, false)
	if code != 200 || o.State != "prepared" || o.Preview == nil {
		t.Fatalf("prepare %d %#v", code, o)
	}
	h, _ := rc.RepositoryRequestDigest(q)
	if e := rc.RepositoryValidateObservation(o, q.RepositoryExecution, h); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(o.Preview.Diff, "+candidate") {
		t.Fatal("preview is not actual Git diff")
	}
	target, _ := f.s.Target(context.Background(), "main")
	if target.HeadCommit != f.base || target.Generation != 0 {
		t.Fatal("prepare changed target/fence")
	}
	return o.Preview.PreparedSHA256
}
