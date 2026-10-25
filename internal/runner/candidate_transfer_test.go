package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

type candidateFixtureServer struct {
	mu                  sync.Mutex
	binding             cc.Binding
	bundle              []byte
	failSeal, cancel    bool
	loseSealACK         bool
	heartbeatStatus     int
	puts, gets, results int
	operations          []string
	binaryRefs          []cc.AssignmentRef
}

func (s *candidateFixtureServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fixture" {
			t.Error("missing fixture authorization")
		}
		prefix := "/api/v1/runner/"
		if strings.Contains(r.URL.Path, "/pool-runner/") {
			prefix = "/api/v1/pool-runner/"
		}
		path := strings.TrimPrefix(r.URL.Path, prefix)
		switch {
		case path == "heartbeat":
			if s.heartbeatStatus != 0 {
				w.WriteHeader(s.heartbeatStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"cancel_requested": s.cancel})
		case path == "results":
			s.results++
			w.WriteHeader(200)
		case path == "candidate-transfers" && r.Method == http.MethodPost:
			var q cc.StartRequest
			if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if err := q.Validate(); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			s.operations = append(s.operations, q.OperationID)
			if s.binding.ValidSealed() {
				_ = json.NewEncoder(w).Encode(s.binding)
				return
			}
			s.binding = cc.Binding{SchemaVersion: 1, TransferID: "transfer1", ProjectID: q.ProjectID, State: "uploading", Source: q.Assignment, BaseCommit: q.BaseCommit, HeadCommit: q.HeadCommit, EvidenceSHA256: q.EvidenceSHA256, BundleSHA256: q.BundleSHA256, SizeBytes: q.SizeBytes}
			_ = json.NewEncoder(w).Encode(s.binding)
		case path == "candidate-transfers/transfer1/bundle":
			raw, err := base64.RawURLEncoding.DecodeString(r.Header.Get("X-Suma-Assignment"))
			var a cc.AssignmentRef
			if err != nil || json.Unmarshal(raw, &a) != nil || !a.Valid() || r.Header.Get("X-Suma-Operation-ID") == "" {
				t.Error("missing binary assignment identity")
				w.WriteHeader(400)
				return
			}
			if r.Method == http.MethodPut {
				s.binaryRefs = append(s.binaryRefs, a)
				s.puts++
				s.bundle, _ = io.ReadAll(r.Body)
				_ = json.NewEncoder(w).Encode(map[string]bool{"acknowledged": true})
				return
			}
			s.gets++
			_, _ = w.Write(s.bundle)
		case path == "candidate-transfers/transfer1/seal":
			if s.failSeal {
				w.WriteHeader(503)
				return
			}
			s.binding.State = "sealed"
			if s.loseSealACK {
				s.loseSealACK = false
				w.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(w).Encode(s.binding)
		case path == "candidate-transfers/resolve":
			_ = json.NewEncoder(w).Encode(s.binding)
		default:
			t.Errorf("unexpected candidate request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
}

func candidateConfig(t *testing.T, repo, base string, c *Client, mode string) ExecConfig {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return ExecConfig{AllowExec: true, Repository: repo, Workspace: repo, BaseCommit: base, Artifacts: c, Commands: map[string]CommandSpec{"candidate": {Executable: exe, Args: []string{"-test.run=TestCandidateTransferCommandHelper", "--"}, Env: map[string]string{"SUMA_CANDIDATE_HELPER": mode}, TestEvidence: mode == "test"}}}
}

func TestCandidateTransferTwoClones(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "pool"}[pool], func(t *testing.T) { testCandidateTransferTwoClones(t, pool) })
	}
}

func testCandidateTransferTwoClones(t *testing.T, pool bool) {
	repo, base := gitFixture(t)
	consumer := filepath.Join(t.TempDir(), "consumer")
	gitTest(t, repo, "clone", "--no-local", repo, consumer)
	fixture := &candidateFixtureServer{}
	server := fixture.serve(t)
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	a := Assignment{RunID: "transfer-run", NodeID: "developer", WorkerID: "writer", Attempt: 1, Fence: 1, Type: "command", Command: []string{"candidate"}}
	if pool {
		a = poolTestAssignment()
		a.Type = "command"
		a.Command = []string{"candidate"}
		client.Pool = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}
	}
	r := Execute(context.Background(), a, candidateConfig(t, repo, base, client, "write"))
	if r.ExitCode != 0 || r.Error != "" {
		t.Fatalf("source execution: %+v", r)
	}
	var b cc.Binding
	raw, _ := json.Marshal(r.WorkspaceEvidence["candidate_bundle"])
	_ = json.Unmarshal(raw, &b)
	if !b.ValidSealed() {
		t.Fatalf("known clean writer did not publish sealed candidate: %s", raw)
	}
	if _, err := gitText(context.Background(), consumer, "rev-parse", "--verify", b.HeadCommit+"^{commit}"); err == nil {
		t.Fatal("consumer already has candidate")
	}
	read := Assignment{RunID: a.RunID, NodeID: "tests", WorkerID: "tester", Attempt: 1, Fence: 2, Type: "test", Command: []string{"candidate"}, Config: map[string]any{"candidate_from": a.NodeID}, CandidateCommit: b.HeadCommit, CandidateFrom: a.NodeID, CandidateAttempt: a.Attempt, CandidateFence: a.Fence, CandidateEvidenceSHA256: b.EvidenceSHA256}
	if pool {
		read.TenantID = a.TenantID
		read.ProjectID = a.ProjectID
		read.PoolID = a.PoolID
		read.PoolRunnerID = "consumer"
		read.PoolSlotID = "1"
		read.PoolGrantID = a.PoolGrantID
		read.PoolGrantVersion = a.PoolGrantVersion
		read.PoolCredentialID = "consumer-credential"
		read.PoolCredentialVersion = 1
		read.WorkerID = "pool-consumer"
		read.DispatchID = "consumer-dispatch"
		read.DispatchState = "active"
		read.LeaseUntil = a.LeaseUntil
		client = &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project", Pool: &PoolContext{PoolID: read.PoolID, RunnerID: read.PoolRunnerID, Assignment: &read}}
	}
	tested := Execute(context.Background(), read, candidateConfig(t, consumer, base, client, "test"))
	if tested.ExitCode != 0 || tested.Error != "" || tested.WorkspaceEvidence["head_commit"] != b.HeadCommit {
		t.Fatalf("independent candidate test: %+v", tested)
	}
	var receipt cc.ImportReceipt
	raw, _ = json.Marshal(tested.WorkspaceEvidence["candidate_import"])
	_ = json.Unmarshal(raw, &receipt)
	if !receipt.Imported || receipt.TargetWorkerID != read.WorkerID || receipt.Binding.BundleSHA256 != b.BundleSHA256 {
		t.Fatalf("missing import receipt %s", raw)
	}
	if got := strings.TrimSpace(gitTest(t, consumer, "rev-parse", "HEAD")); got != base {
		t.Fatal("consumer checkout was changed")
	}
	fixture.mu.Lock()
	gets := fixture.gets
	fixture.mu.Unlock()
	read.Attempt++
	read.Fence++
	again := Execute(context.Background(), read, candidateConfig(t, consumer, base, client, "test"))
	if again.ExitCode != 0 || again.WorkspaceEvidence["candidate_import"] != nil {
		t.Fatalf("local-object test: %+v", again)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.gets != gets {
		t.Fatal("already local object was downloaded again")
	}
}

func TestCandidateTransferCommandHelper(t *testing.T) {
	mode := os.Getenv("SUMA_CANDIDATE_HELPER")
	if mode == "" {
		return
	}
	if mode == "test" {
		b, err := os.ReadFile("file.txt")
		if err != nil || string(b) != "candidate\n" {
			os.Exit(30)
		}
		os.Exit(0)
	}
	if mode != "write" {
		os.Exit(31)
	}
	if counter := os.Getenv("SUMA_CANDIDATE_COUNTER"); counter != "" {
		f, err := os.OpenFile(counter, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			os.Exit(34)
		}
		_, _ = f.WriteString("executed\n")
		f.Close()
	}
	if os.WriteFile("file.txt", []byte("candidate\n"), 0600) != nil {
		os.Exit(32)
	}
	for _, args := range [][]string{{"add", "file.txt"}, {"-c", "user.name=Candidate Fixture", "-c", "user.email=candidate@example.invalid", "commit", "-m", "fixture candidate"}} {
		if exec.Command("git", args...).Run() != nil {
			os.Exit(33)
		}
	}
	os.Exit(0)
}

func TestCandidateTransferPendingRestartNeverReexecutes(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		pool, lostACK bool
	}{{"project", false, false}, {"pool", true, false}, {"sealed-ACK", false, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			pool := scenario.pool
			repo, base := gitFixture(t)
			fixture := &candidateFixtureServer{failSeal: !scenario.lostACK, loseSealACK: scenario.lostACK}
			server := fixture.serve(t)
			defer server.Close()
			a := Assignment{RunID: "pending-run", NodeID: "developer", WorkerID: "writer", Attempt: 1, Fence: 1}
			a.DispatchID = "ordinary-dispatch"
			a.DispatchState = "active"
			a.LeaseUntil = "2030-01-01T00:00:00Z"
			if pool {
				a = poolTestAssignment()
			}
			a.Type = "command"
			a.Command = []string{"candidate"}
			c := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
			var replayClient *Client = c
			if pool {
				replayClient = &Client{BaseURL: server.URL, Token: "fixture", Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}}
				var err error
				c, err = replayClient.forPoolAssignment(a)
				if err != nil {
					t.Fatal(err)
				}
			}
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			cfg := candidateConfig(t, repo, base, c, "write")
			cfg.Journal = j
			counter := filepath.Join(t.TempDir(), "counter.txt")
			spec := cfg.Commands["candidate"]
			spec.Env["SUMA_CANDIDATE_COUNTER"] = counter
			cfg.Commands["candidate"] = spec
			r := Execute(context.Background(), a, cfg)
			if !r.candidatePending || len(j.Pending()) != 0 || len(j.pendingCandidates()) != 1 {
				t.Fatalf("upload failure was finalized or lost: %+v", r)
			}
			p := j.pendingCandidates()[0]
			if p.Result.ExitCode != 0 || p.Result.Error != "" || p.Result.OutcomeUnknown {
				t.Fatal("checkpoint replaced original known process result")
			}
			_ = j.Close()
			j, err = OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			if len(j.Pending()) != 0 || len(j.pendingCandidates()) != 1 {
				t.Fatal("restart manufactured unknown result or lost transfer")
			}
			fixture.mu.Lock()
			fixture.cancel = true
			puts := fixture.puts
			fixture.mu.Unlock()
			if err = ReplayCandidates(context.Background(), replayClient, j); err == nil {
				t.Fatal("cancelled transfer resumed")
			}
			fixture.mu.Lock()
			if fixture.puts != puts || fixture.results != 0 {
				t.Error("cancelled recovery transmitted bytes or succeeded")
			}
			fixture.cancel = false
			fixture.heartbeatStatus = 403
			fixture.mu.Unlock()
			if err = ReplayCandidates(context.Background(), replayClient, j); err == nil {
				t.Fatal("revoked heartbeat permitted candidate recovery")
			}
			fixture.mu.Lock()
			if fixture.puts != puts || fixture.results != 0 {
				t.Error("revoked recovery transmitted bytes or succeeded")
			}
			fixture.heartbeatStatus = 0
			fixture.failSeal = false
			fixture.mu.Unlock()
			if pool {
				err = replayPool(context.Background(), replayClient, j)
			} else {
				err = Replay(context.Background(), replayClient, j)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(j.Pending()) != 0 || len(j.pendingCandidates()) != 0 {
				t.Fatal("sealed original result was not acknowledged")
			}
			b, _ := os.ReadFile(counter)
			if string(b) != "executed\n" {
				t.Fatalf("code ran more than once: %s", b)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.results != 1 || len(fixture.operations) != 2 || fixture.operations[0] != fixture.operations[1] {
				t.Fatal("transfer replay changed operation or duplicated result")
			}
			wantPuts := 2
			if scenario.lostACK {
				wantPuts = 1
			}
			if fixture.binding.Source != candidateRef(a) || len(fixture.binaryRefs) != wantPuts || fixture.puts != wantPuts {
				t.Fatal("transfer lost frozen assignment fields")
			}
			for _, ref := range fixture.binaryRefs {
				if ref != candidateRef(a) {
					t.Fatal("binary transfer changed original identity")
				}
			}
		})
	}
}

func TestCandidateTransferRejectsCorruptPackHeaderAndFrozenSource(t *testing.T) {
	repo, base := gitFixture(t)
	fixture := &candidateFixtureServer{}
	server := fixture.serve(t)
	defer server.Close()
	c := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	a := Assignment{RunID: "bad-transfer-run", NodeID: "developer", WorkerID: "writer", Attempt: 1, Fence: 1, Type: "command", Command: []string{"candidate"}}
	r := Execute(context.Background(), a, candidateConfig(t, repo, base, c, "write"))
	if r.ExitCode != 0 {
		t.Fatalf("source: %+v", r)
	}
	original := fixture.binding
	originalBytes := append([]byte(nil), fixture.bundle...)
	for _, kind := range []string{"hash", "header-head", "pack", "fence", "base", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			consumer := filepath.Join(t.TempDir(), "consumer")
			gitTest(t, repo, "clone", "--no-local", repo, consumer)
			fixture.mu.Lock()
			fixture.binding = original
			fixture.bundle = append([]byte(nil), originalBytes...)
			switch kind {
			case "hash":
				fixture.bundle[len(fixture.bundle)-1] ^= 1
			case "header-head":
				fixture.bundle = []byte(strings.Replace(string(fixture.bundle), original.HeadCommit+" HEAD", base+" HEAD", 1))
				fixture.binding.BundleSHA256 = digest(fixture.bundle)
			case "pack":
				fixture.bundle[len(fixture.bundle)-1] ^= 1
				fixture.binding.BundleSHA256 = digest(fixture.bundle)
			case "fence":
				fixture.binding.Source.Fence++
			case "base":
				fixture.binding.BaseCommit = strings.Repeat("f", 40)
			case "oversize":
				fixture.binding.SizeBytes = cc.MaxBundleBytes + 1
			}
			fixture.mu.Unlock()
			read := Assignment{RunID: a.RunID, NodeID: "test", WorkerID: "tester", Attempt: 1, Fence: 2, Type: "test", Command: []string{"candidate"}, Config: map[string]any{"candidate_from": a.NodeID}, CandidateCommit: original.HeadCommit, CandidateFrom: a.NodeID, CandidateAttempt: a.Attempt, CandidateFence: a.Fence, CandidateEvidenceSHA256: original.EvidenceSHA256}
			got := Execute(context.Background(), read, candidateConfig(t, consumer, base, c, "test"))
			if got.ExitCode == 0 || got.WorkspaceEvidence != nil {
				t.Fatalf("invalid candidate executed: %+v", got)
			}
			if kind == "pack" && !strings.Contains(got.Error, "strict verification") {
				t.Fatalf("corrupt pack did not reach strict Git verification: %s", got.Error)
			}
			if _, err := gitText(context.Background(), consumer, "rev-parse", "--verify", original.HeadCommit+"^{commit}"); err == nil {
				t.Fatal("rejected candidate became available")
			}
		})
	}
}

func TestCandidateTransferStrictPackRejectsMissingTree(t *testing.T) {
	repo, base := gitFixture(t)
	consumer := filepath.Join(t.TempDir(), "consumer")
	gitTest(t, repo, "clone", "--no-local", repo, consumer)
	// This synthetic commit has a valid object ID and pack checksum but references
	// an absent tree. Bundle header/prerequisite checks alone cannot reject it.
	body := "tree " + strings.Repeat("1", 40) + "\nparent " + base + "\nauthor Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\ninvalid tree fixture\n"
	object := candidateGit(context.Background(), repo, "hash-object", "--literally", "-w", "-t", "commit", "--stdin")
	object.Stdin = strings.NewReader(body)
	raw, err := object.Output()
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(string(raw))
	pack := candidateGit(context.Background(), repo, "pack-objects", "--stdout")
	pack.Stdin = strings.NewReader(head + "\n")
	packed, err := pack.Output()
	if err != nil {
		t.Fatal(err)
	}
	bundle := append([]byte("# v2 git bundle\n-"+base+" fixture base\n"+head+" HEAD\n\n"), packed...)
	source := Assignment{RunID: "strict-run", NodeID: "developer", Attempt: 1, Fence: 1, WorkerID: "writer"}
	b := cc.Binding{SchemaVersion: 1, TransferID: "transfer1", ProjectID: "project", State: "sealed", Source: candidateRef(source), BaseCommit: base, HeadCommit: head, EvidenceSHA256: strings.Repeat("e", 64), BundleSHA256: digest(bundle), SizeBytes: int64(len(bundle))}
	fixture := &candidateFixtureServer{binding: b, bundle: bundle}
	server := fixture.serve(t)
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "fixture", ProjectID: "project"}
	a := Assignment{RunID: source.RunID, NodeID: "test", Attempt: 1, Fence: 2, WorkerID: "tester", CandidateCommit: head, CandidateFrom: source.NodeID, CandidateAttempt: 1, CandidateFence: 1, CandidateEvidenceSHA256: b.EvidenceSHA256}
	_, err = importCandidate(context.Background(), consumer, a, ExecConfig{BaseCommit: base, Artifacts: client})
	if err == nil || !strings.Contains(err.Error(), "strict verification") {
		t.Fatalf("invalid object graph was not rejected by strict index-pack: %v", err)
	}
	if _, err = gitText(context.Background(), consumer, "rev-parse", "--verify", head+"^{commit}"); err == nil {
		t.Fatal("strictly rejected object was published into consumer object database")
	}
}
