package runner

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
	"sync"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

func TestCodeBaselineNeverFallsBackToAdministratorBase(t *testing.T) {
	a := Assignment{Type: "agent", CodeBaseline: &ccc.CodeBaselineBinding{Protocol: ccc.CodeBaselineProtocol}}
	if _, err := candidateBase(context.Background(), "unused", a, ExecConfig{BaseCommit: strings.Repeat("a", 40)}); err == nil {
		t.Fatal("invalid or unauthorized baseline silently selected the administrator base")
	}
}

type compositionFixture struct {
	mu           sync.Mutex
	inputs       map[string][]byte
	reads        []ccc.CodeInputRequest
	binding      cc.Binding
	bundle       []byte
	starts, puts int
}

func (f *compositionFixture) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing original runner authentication")
			w.WriteHeader(401)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/runner/")
		switch {
		case path == "code-inputs/bundle":
			var q ccc.CodeInputRequest
			if r.Method != "POST" || json.NewDecoder(r.Body).Decode(&q) != nil || ccc.ValidateCodeInputRequest(q) != nil {
				t.Error("invalid exact code input request")
				w.WriteHeader(400)
				return
			}
			data, ok := f.inputs[q.TransferID]
			if !ok {
				w.WriteHeader(403)
				return
			}
			f.reads = append(f.reads, q)
			_, _ = w.Write(data)
		case path == "candidate-transfers":
			var q cc.StartRequest
			if json.NewDecoder(r.Body).Decode(&q) != nil || q.Validate() != nil {
				t.Error("invalid produced manifest")
				w.WriteHeader(400)
				return
			}
			f.starts++
			f.binding = cc.Binding{SchemaVersion: 1, TransferID: fmt.Sprintf("output-%d", f.starts), ProjectID: q.ProjectID, Source: q.Assignment, BaseCommit: q.BaseCommit, BundleBaseCommit: q.BundleBaseCommit, HeadCommit: q.HeadCommit, EvidenceSHA256: q.EvidenceSHA256, BundleSHA256: q.BundleSHA256, SizeBytes: q.SizeBytes, State: "uploading"}
			_ = json.NewEncoder(w).Encode(f.binding)
		case path == "candidate-transfers/resolve":
			_ = json.NewEncoder(w).Encode(f.binding)
		case strings.HasSuffix(path, "/bundle") && r.Method == "PUT":
			f.puts++
			f.bundle, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]bool{"acknowledged": true})
		case strings.HasSuffix(path, "/bundle") && r.Method == "GET":
			_, _ = w.Write(f.bundle)
		case strings.HasSuffix(path, "/seal"):
			f.binding.State = "sealed"
			_ = json.NewEncoder(w).Encode(f.binding)
		default:
			t.Errorf("unexpected composition transport %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
}

func compositionSourcesFixture(t *testing.T, conflict bool) (string, string, []cc.Binding, map[string][]byte) {
	t.Helper()
	repo, base := gitFixture(t)
	sources := []cc.Binding{}
	bundles := map[string][]byte{}
	for i := 0; i < 2; i++ {
		gitTest(t, repo, "checkout", "--detach", base)
		name := []string{"a.txt", "b.txt"}[i]
		if conflict {
			name = "file.txt"
		}
		if err := os.WriteFile(filepath.Join(repo, name), []byte(fmt.Sprintf("source-%d\n", i)), 0600); err != nil {
			t.Fatal(err)
		}
		gitTest(t, repo, "add", name)
		gitTest(t, repo, "commit", "-m", fmt.Sprintf("source-%d", i))
		head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
		path := filepath.Join(t.TempDir(), "source.bundle")
		gitTest(t, repo, "bundle", "create", path, "HEAD", "^"+base)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("source-%d", i)
		b := cc.Binding{SchemaVersion: 1, TransferID: id, ProjectID: "project", State: "sealed", Source: cc.AssignmentRef{RunID: id, NodeID: "code", WorkerID: "source", Attempt: 1, Fence: 1}, BaseCommit: base, HeadCommit: head, EvidenceSHA256: strings.Repeat("a", 64), BundleSHA256: digest(data), SizeBytes: int64(len(data))}
		sources = append(sources, b)
		bundles[id] = data
	}
	gitTest(t, repo, "checkout", "--detach", base)
	return repo, base, sources, bundles
}
func compositionAssignment(t *testing.T, base string, sources []cc.Binding) Assignment {
	t.Helper()
	sha, err := ccc.CandidateCompositionSourcesDigest(sources)
	if err != nil {
		t.Fatal(err)
	}
	e := &ccc.CandidateCompositionExecution{Protocol: ccc.CandidateCompositionProtocol, PlanID: "plan", PlanVersion: 1, ProjectID: "project", BaseCommit: base, Sources: sources, SourcesSHA256: sha, FixedTimestamp: "2026-10-27T00:00:00Z", OperationID: "compose-op"}
	return Assignment{RunID: "composition-run", NodeID: "compose", WorkerID: "composer", Attempt: 1, Fence: 1, Type: "command", Command: []string{ccc.CompositionCommand}, Config: map[string]any{"purpose": "candidate_composition"}, CandidateComposition: e}
}
func compositionConfig(t *testing.T, repo, base string, client *Client) ExecConfig {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	return ExecConfig{AllowExec: true, Repository: repo, BaseCommit: base, Workspace: repo, Artifacts: client, Commands: map[string]CommandSpec{ccc.CompositionCommand: {Executable: git, CandidateComposition: true}}}
}

func TestCandidateCompositionRealGitAndInheritedNoDelta(t *testing.T) {
	repo, base, sources, bundles := compositionSourcesFixture(t, false)
	// Independent repositories have only B, not either unreferenced source head.
	consumer := filepath.Join(t.TempDir(), "consumer")
	gitTest(t, repo, "clone", "--no-local", repo, consumer)
	developer := filepath.Join(t.TempDir(), "developer")
	gitTest(t, repo, "clone", "--no-local", repo, developer)
	fixture := &compositionFixture{inputs: bundles}
	server := fixture.server(t)
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	a := compositionAssignment(t, base, sources)
	cfg := compositionConfig(t, repo, base, client)
	if err := ValidateToolConfiguration(cfg); err != nil {
		t.Fatal(err)
	}
	tools, err := DescribeTools(context.Background(), cfg)
	if err != nil || len(tools) != 1 || !tools[0].Available || !strings.HasPrefix(tools[0].Profile.ToolVersion, "git version ") {
		t.Fatalf("real Git inventory: %+v %v", tools, err)
	}
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 || r.Error != "" || r.OutcomeUnknown {
		t.Fatalf("real composition: %+v", r)
	}
	b := fixture.binding
	if !b.ValidSealed() || fixture.starts != 1 || fixture.puts != 1 || len(fixture.reads) != 2 {
		t.Fatalf("missing single sealed composition: %+v", fixture)
	}
	var facts ccc.CandidateCompositionEvidence
	raw, _ := json.Marshal(r.WorkspaceEvidence["candidate_composition"])
	_ = json.Unmarshal(raw, &facts)
	if facts.State != "completed" || facts.ResultCommit != b.HeadCommit || len(facts.Parents) != 2 || facts.Parents[0] != sources[0].HeadCommit || facts.Parents[1] != sources[1].HeadCommit || len(facts.MergeCommits) != 1 {
		t.Fatalf("ordered true merge provenance: %+v", facts)
	}
	for i, q := range fixture.reads {
		if q.InputKind != "composition" || q.BindingID != "plan" || q.Assignment != candidateRef(a) || q.TransferID != sources[i].TransferID {
			t.Fatalf("unfrozen source download %+v", q)
		}
	}
	if head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD")); head != base {
		t.Fatal("administrator checkout changed")
	}
	if rerun := Execute(context.Background(), a, cfg); rerun.ExitCode == 0 || fixture.starts != 1 {
		t.Fatal("same attempt recomposed")
	}
	// Normal same-run candidate_from remains the only formal-test input mode.
	read := Assignment{RunID: a.RunID, NodeID: "test", WorkerID: "tester", Attempt: 1, Fence: 2, Type: "test", Command: []string{"verify"}, Config: map[string]any{"candidate_from": a.NodeID}, CandidateFrom: a.NodeID, CandidateCommit: b.HeadCommit, CandidateAttempt: 1, CandidateFence: 1, CandidateEvidenceSHA256: b.EvidenceSHA256}
	exe, _ := os.Executable()
	verifySpec := CommandSpec{Executable: exe, Args: []string{"-test.run=TestCandidateCompositionProcessHelper", "--"}, Env: map[string]string{"SUMA_COMPOSITION_HELPER": "test"}, TestEvidence: true}
	testCfg := ExecConfig{AllowExec: true, Repository: consumer, Workspace: consumer, BaseCommit: base, Artifacts: client, Commands: map[string]CommandSpec{"verify": verifySpec}}
	tested := Execute(context.Background(), read, testCfg)
	if tested.ExitCode != 0 || tested.Error != "" || tested.WorkspaceEvidence["head_commit"] != b.HeadCommit {
		t.Fatalf("independent exact merge test: %+v", tested)
	}
	// The new development input freezes M while the complete bundle anchor stays B.
	baseline := &ccc.CodeBaselineBinding{Protocol: ccc.CodeBaselineProtocol, ID: "baseline", ProjectID: "project", TaskID: "downstream", TaskVersion: 2, CompositionID: "plan", CompositionVersion: 2, AcceptedBasisSHA256: strings.Repeat("b", 64), Source: b}
	baseline.BindingSHA256, _ = ccc.CodeBaselineBindingDigest(*baseline)
	fixture.inputs[b.TransferID] = append([]byte(nil), fixture.bundle...)
	dev := Assignment{RunID: "downstream-run", NodeID: "code", WorkerID: "developer", Attempt: 1, Fence: 3, Type: "agent", Command: []string{"develop"}, Config: map[string]any{"goal": "verify inherited code without editing"}, CodeBaseline: baseline}
	devSpec := verifySpec
	devSpec.TestEvidence = false
	devSpec.Input = "goal"
	devSpec.Env = map[string]string{"SUMA_COMPOSITION_HELPER": "develop"}
	devCfg := ExecConfig{AllowExec: true, Repository: developer, Workspace: developer, BaseCommit: base, Artifacts: client, Commands: map[string]CommandSpec{"develop": devSpec}}
	developed := Execute(context.Background(), dev, devCfg)
	if developed.ExitCode != 0 || developed.Error != "" {
		t.Fatalf("inherited no-delta development: %+v", developed)
	}
	inherited := fixture.binding
	if inherited.BaseCommit != b.HeadCommit || inherited.HeadCommit != b.HeadCommit || inherited.BundleBase() != base || !inherited.ValidSealed() || developed.WorkspaceEvidence["base_commit"] != b.HeadCommit || developed.WorkspaceEvidence["admin_base_commit"] != base || developed.WorkspaceEvidence["code_baseline"] == nil {
		t.Fatalf("selected M / anchor B lost: %+v %+v", inherited, developed.WorkspaceEvidence)
	}
	onlyB := filepath.Join(t.TempDir(), "only-b")
	gitTest(t, repo, "clone", "--no-local", repo, onlyB)
	path := filepath.Join(t.TempDir(), "inherited.bundle")
	if err = os.WriteFile(path, fixture.bundle, 0600); err != nil {
		t.Fatal(err)
	}
	if err = importCandidateFile(context.Background(), onlyB, path, inherited); err != nil {
		t.Fatal("inherited closure must import from B:", err)
	}
	if len(fixture.reads) != 3 || fixture.reads[2].InputKind != "code_baseline" || fixture.reads[2].BindingID != baseline.ID || fixture.reads[2].Assignment != candidateRef(dev) {
		t.Fatal("baseline skipped fresh current-authority read")
	}
}

func TestCandidateCompositionConflictNeverSeals(t *testing.T) {
	repo, base, sources, bundles := compositionSourcesFixture(t, true)
	fixture := &compositionFixture{inputs: bundles}
	server := fixture.server(t)
	defer server.Close()
	a := compositionAssignment(t, base, sources)
	r := Execute(context.Background(), a, compositionConfig(t, repo, base, &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}))
	var facts ccc.CandidateCompositionEvidence
	raw, _ := json.Marshal(r.WorkspaceEvidence["candidate_composition"])
	_ = json.Unmarshal(raw, &facts)
	if r.ExitCode != 1 || r.OutcomeUnknown || facts.State != "conflicted" || facts.ResultCommit != "" || len(facts.Conflicts) != 1 || facts.Conflicts[0] != "file.txt" || facts.ConflictSourceIndex != 1 || fixture.starts != 0 || fixture.puts != 0 || r.WorkspaceEvidence["head_commit"] != base {
		t.Fatalf("conflict falsely sealed or missing original paths: %+v %+v", r, facts)
	}
}

func TestCandidateCompositionDedicatedCommandAndCurrentPoolInput(t *testing.T) {
	valid := CommandSpec{Executable: "git", CandidateComposition: true}
	for name, change := range map[string]func(*CommandSpec){"args": func(s *CommandSpec) { s.Args = []string{"status"} }, "env": func(s *CommandSpec) { s.EnvFrom = []string{"GIT_CONFIG"} }, "workspace": func(s *CommandSpec) { s.NativeWorkspace = &NativeWorkspacePolicy{} }, "read_only": func(s *CommandSpec) { s.ReadOnly = true }} {
		t.Run(name, func(t *testing.T) {
			spec := valid
			change(&spec)
			if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{ccc.CompositionCommand: spec}}) == nil {
				t.Fatal("composition accepted arbitrary local command behavior")
			}
		})
	}
	missing := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "command", Command: []string{ccc.CompositionCommand}, Config: map[string]any{"purpose": "candidate_composition"}}
	if r := Execute(context.Background(), missing, ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{ccc.CompositionCommand: valid}}); r.ExitCode == 0 || r.Error == "" {
		t.Fatal("workflow config manufactured composition authority")
	}
	// Real full Pool tuple reaches only the dedicated code-input endpoint.
	a := poolTestAssignment()
	body := []byte("bounded frozen bytes")
	b := cc.Binding{SchemaVersion: 1, TransferID: "fixed", ProjectID: a.ProjectID, State: "sealed", Source: candidateRef(a), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), BundleSHA256: digest(body), EvidenceSHA256: strings.Repeat("c", 64), SizeBytes: int64(len(body))}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var q ccc.CodeInputRequest
		if r.URL.Path != "/api/v1/pool-runner/code-inputs/bundle" || r.Header.Get("Authorization") != "Bearer fixture" || json.NewDecoder(r.Body).Decode(&q) != nil || q.Assignment != candidateRef(a) || q.BindingID != "plan" || q.TransferID != b.TransferID {
			t.Error("Pool input scope changed")
			w.WriteHeader(403)
			return
		}
		_, _ = w.Write(body)
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: a.ProjectID, Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}}
	if err := c.codeInputBundle(context.Background(), a, "composition", "plan", b, filepath.Join(t.TempDir(), "pool.bundle")); err != nil {
		t.Fatal(err)
	}
	wrong := a
	wrong.PoolCredentialVersion++
	if err := c.codeInputBundle(context.Background(), wrong, "composition", "plan", b, filepath.Join(t.TempDir(), "wrong.bundle")); err == nil || calls != 1 {
		t.Fatal("wrong current Pool tuple reached network")
	}
	foreign := b
	foreign.ProjectID = "another-project"
	if err := c.codeInputBundle(context.Background(), a, "composition", "plan", foreign, filepath.Join(t.TempDir(), "foreign.bundle")); err == nil || calls != 1 {
		t.Fatal("foreign source reached network")
	}
}

func TestCandidateCompositionHermeticDeterminismAndDriverRejection(t *testing.T) {
	repo, base, sources, bundles := compositionSourcesFixture(t, false)
	second := filepath.Join(t.TempDir(), "second")
	gitTest(t, repo, "clone", "--no-local", repo, second)
	fixture := &compositionFixture{inputs: bundles}
	server := fixture.server(t)
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	a := compositionAssignment(t, base, sources)
	first := Execute(context.Background(), a, compositionConfig(t, repo, base, client))
	if first.ExitCode != 0 {
		t.Fatalf("first merge: %+v", first)
	}
	head := fixture.binding.HeadCommit
	// Host-local Git merge policy and process config cannot change the operation's
	// fixed Git result; merge-tree executes in the new empty-config object repo.
	gitTest(t, second, "config", "merge.renormalize", "true")
	gitTest(t, second, "config", "merge.conflictStyle", "diff3")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "merge.renames")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")
	a.RunID = "deterministic-second"
	a.WorkerID = "second"
	repeated := Execute(context.Background(), a, compositionConfig(t, second, base, client))
	if repeated.ExitCode != 0 || fixture.binding.HeadCommit != head {
		t.Fatalf("same fixed inputs yielded another merge: %+v", repeated)
	}
	// An accepted object's .gitattributes cannot select an administrator/local
	// executable merge driver. Reject it before merge and before any seal.
	gitTest(t, repo, "checkout", "--detach", sources[0].HeadCommit)
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt merge=suma-forbidden\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".gitattributes")
	gitTest(t, repo, "commit", "-m", "unsupported driver")
	path := filepath.Join(t.TempDir(), "driver.bundle")
	gitTest(t, repo, "bundle", "create", path, "HEAD", "^"+base)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	driver := sources[0]
	driver.TransferID = "driver"
	driver.HeadCommit = strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	driver.BundleSHA256 = digest(raw)
	driver.SizeBytes = int64(len(raw))
	fixture.inputs[driver.TransferID] = raw
	gitTest(t, repo, "checkout", "--detach", base)
	starts := fixture.starts
	bad := compositionAssignment(t, base, []cc.Binding{driver, sources[1]})
	bad.RunID = "driver-reject"
	rejected := Execute(context.Background(), bad, compositionConfig(t, repo, base, client))
	if rejected.ExitCode == 0 || !strings.Contains(rejected.Error, "merge drivers are forbidden") || fixture.starts != starts {
		t.Fatalf("custom merge policy passed: %+v", rejected)
	}
}

func TestCandidateCompositionProcessHelper(t *testing.T) {
	mode := os.Getenv("SUMA_COMPOSITION_HELPER")
	if mode == "" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	for i, name := range []string{"a.txt", "b.txt"} {
		raw, err := os.ReadFile(name)
		if err != nil || string(raw) != fmt.Sprintf("source-%d\n", i) {
			os.Exit(2)
		}
	}
	if mode != "test" && mode != "develop" {
		os.Exit(3)
	}
	os.Exit(0)
}
