package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wb "github.com/ai-mada/suma/internal/workbuddy"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudNeverFallsThroughToGenericExternal(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"generic-run","state":"succeeded"}`))
	}))
	defer server.Close()
	t.Setenv("SUMA_TEST_CLOUD_GENERIC_TOKEN", "fixture-token")
	a := Assignment{RunID: "run-155", NodeID: "cloud", Attempt: 1, Fence: 1, WorkerID: "worker-155", Type: "external_workflow", Config: map[string]any{"plugin": "workbuddy-cloud", "workflow": "workbuddy-cloud/v1"}}
	r := Execute(context.Background(), a, ExecConfig{External: map[string]ExternalSpec{"workbuddy-cloud": {URL: server.URL, TokenEnv: "SUMA_TEST_CLOUD_GENERIC_TOKEN"}}})
	if requests.Load() != 0 || r.ExitCode == 0 || r.Stopped {
		t.Fatalf("unbound cloud assignment reached generic external dispatch: requests=%d exit=%d stopped=%v", requests.Load(), r.ExitCode, r.Stopped)
	}
}

func cloudExecutionFixture(t *testing.T) (*Journal, Assignment) {
	t.Helper()
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	a := Assignment{RunID: "run-155", NodeID: "cloud", Attempt: 1, Fence: 1, WorkerID: "worker-155", Type: "external_workflow", DispatchID: "claim-155", DispatchState: "active", LeaseUntil: time.Now().Add(time.Minute).UTC().Format(time.RFC3339), WorkBuddyCloud: &wbc.CloudAssignmentBinding{DelegationID: "delegation-155", DelegationVersion: 1, DelegationSHA256: strings.Repeat("a", 64)}, Config: map[string]any{"plugin": wbc.Plugin, "workflow": wbc.Protocol, "workbuddy_cloud": wbc.NodeBinding{ConnectionID: "connection-155", GrantGeneration: 1, Request: wb.CreateRequest{Prompt: "Original reviewed prompt"}}}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	return j, a
}

func cloudFixtureSubmission(a Assignment, req wbc.CloudCreateRequest, state string) wbc.CloudSubmission {
	node, _ := wbc.ParseNodeBinding(a.Config["workbuddy_cloud"])
	sha, _ := node.RequestDigest()
	s := wbc.CloudSubmission{ID: "submission-155", ProjectID: req.ProjectID, DelegationID: req.DelegationID, Version: 2, RunID: a.RunID, NodeID: a.NodeID, Assignment: req.Assignment, RequestSHA256: sha, State: state, UsageState: "unreported", CreatedAt: "2026-10-30T08:00:00Z", UpdatedAt: "2026-10-30T08:00:00Z"}
	if state == "task_known" {
		s.Task = &wb.Task{ID: "task-155", Status: "completed"}
		s.ObservedAt = s.UpdatedAt
	}
	return s
}

func assertCloudWait(t *testing.T, r Result, cause string) {
	t.Helper()
	if r.WorkBuddyCloudWait == nil || r.WorkBuddyCloudWait.Validate() != nil || r.WorkBuddyCloudWait.Cause != cause || !r.OutcomeUnknown || r.Stopped || r.ExitCode == 0 || r.ToolEvidence != nil || r.WorkspaceEvidence != nil {
		t.Fatalf("not an original cloud wait: %#v", r)
	}
}

func TestWorkBuddyCloudCheckpointBeforeCreateAndCompletedNeverSucceeds(t *testing.T) {
	j, a := cloudExecutionFixture(t)
	var creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/lookup"):
			var req wbc.CloudLookupRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "not_found"})
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
		case strings.HasSuffix(r.URL.Path, "/workbuddy-cloud-submissions"):
			creates.Add(1)
			var req wbc.CloudCreateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			p, err := j.cloudExecution(a)
			if err != nil || p.Create != req {
				t.Error("transport preceded its durable original body", err)
			}
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(cloudFixtureSubmission(a, req, "task_known"))
		default:
			t.Error("unexpected endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}
	r := Execute(context.Background(), a, ExecConfig{Journal: j, CloudClient: c})
	assertCloudWait(t, r, "delivery_required")
	if creates.Load() != 1 {
		t.Fatal("expected one control create", creates.Load())
	}
	if err := j.Finish(r); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(filepath.Dir(j.path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	pending := reopened.Pending()
	if len(pending) != 1 || pending[0].WorkBuddyCloudWait == nil || pending[0].OperationID != r.OperationID || pending[0].WorkBuddyCloud == nil {
		t.Fatal("waiting result or protected inbox lost across reopen")
	}
	pending[0].WorkBuddyCloudWait.Cause = "interrupted"
	if reopened.Pending()[0].WorkBuddyCloudWait.Cause != "delivery_required" {
		t.Fatal("outbox read exposed mutable original cloud result")
	}
}

func TestWorkBuddyCloudLostCreateRecoveryReadsOriginalWithoutAnotherCreate(t *testing.T) {
	j, a := cloudExecutionFixture(t)
	var creates atomic.Int32
	var missingOriginal atomic.Bool
	var saved *wbc.CloudSubmission
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/lookup") {
			var req wbc.CloudLookupRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			out := wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "not_found"}
			if saved != nil && !missingOriginal.Load() {
				out.State, out.Submission = saved.State, saved
			}
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/heartbeat") {
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
			return
		}
		creates.Add(1)
		var req wbc.CloudCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s := cloudFixtureSubmission(a, req, "task_known")
		saved = &s
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}
	r := Execute(context.Background(), a, ExecConfig{Journal: j, CloudClient: c})
	assertCloudWait(t, r, "creation_unconfirmed")
	p, _ := j.cloudExecution(a)
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(filepath.Dir(j.path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Pending()) != 0 || len(reopened.pendingCloudExecutions()) != 1 {
		t.Fatal("restart sealed cloud as generic unknown")
	}
	r = Execute(context.Background(), a, ExecConfig{Journal: reopened, CloudClient: c})
	assertCloudWait(t, r, "delivery_required")
	q, _ := reopened.cloudExecution(a)
	if creates.Load() != 1 || p.CreateBody != q.CreateBody || p.ResultOperationID != r.OperationID {
		t.Fatal("recovery changed original create or result operation")
	}
	missingOriginal.Store(true)
	r = Execute(context.Background(), a, ExecConfig{Journal: reopened, CloudClient: c})
	assertCloudWait(t, r, "remote_unknown")
	if creates.Load() != 1 {
		t.Fatal("missing central record replaced a locally known task")
	}
}

func TestWorkBuddyCloudPreparedAndUnknownNeverResubmit(t *testing.T) {
	for _, state := range []string{"prepared", "unknown"} {
		t.Run(state, func(t *testing.T) {
			j, a := cloudExecutionFixture(t)
			var other atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if !strings.HasSuffix(r.URL.Path, "/lookup") {
					other.Add(1)
					w.WriteHeader(500)
					return
				}
				var req wbc.CloudLookupRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				s := cloudFixtureSubmission(a, wbc.CloudCreateRequest{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment}, state)
				_ = json.NewEncoder(w).Encode(wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: state, Submission: &s})
			}))
			defer server.Close()
			r := Execute(context.Background(), a, ExecConfig{Journal: j, CloudClient: &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}})
			assertCloudWait(t, r, "creation_unconfirmed")
			if other.Load() != 0 || r.WorkBuddyCloudWait.SubmissionID != "submission-155" {
				t.Fatal("uncertain original effect repeated or lost")
			}
		})
	}
}

func TestWorkBuddyCloudUndurableInboxAndChangedOriginSendNothing(t *testing.T) {
	j, a := cloudExecutionFixture(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	c := &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}
	if _, err := j.beginCloudExecution(c, a); err != nil {
		t.Fatal(err)
	}
	changed := *c
	changed.BaseURL += "/other-control"
	_ = Execute(context.Background(), a, ExecConfig{Journal: j, CloudClient: &changed})
	if err := os.WriteFile(j.path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	_ = Execute(context.Background(), a, ExecConfig{Journal: j, CloudClient: c})
	if requests.Load() != 0 {
		t.Fatal("invalid durable scope reached network")
	}
}

func TestWorkBuddyCloudRefreshLostReceiptReplaysOriginalDespiteNewLookup(t *testing.T) {
	j, a := cloudExecutionFixture(t)
	var refreshBodies []string
	version := 2
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/lookup") {
			var req wbc.CloudLookupRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			s := cloudFixtureSubmission(a, wbc.CloudCreateRequest{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment}, "task_known")
			s.Version = version
			s.Task.Status = "working"
			if version == 3 {
				s.Task.Status = "completed"
			}
			_ = json.NewEncoder(w).Encode(wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "task_known", Submission: &s})
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/refresh") {
			t.Error("unexpected create or request", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		body, _ := io.ReadAll(r.Body)
		refreshBodies = append(refreshBodies, string(body))
		var req wbc.CloudRefreshRequest
		_ = json.Unmarshal(body, &req)
		version = 3
		if len(refreshBodies) == 1 {
			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()
			return
		}
		s := cloudFixtureSubmission(a, wbc.CloudCreateRequest{ProjectID: req.ProjectID, DelegationID: a.WorkBuddyCloud.DelegationID, Assignment: req.Assignment}, "task_known")
		s.Version = 3
		_ = json.NewEncoder(w).Encode(s)
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}
	cfg := ExecConfig{Journal: j, CloudClient: c, CloudPollInterval: time.Millisecond}
	r := Execute(context.Background(), a, cfg)
	assertCloudWait(t, r, "observation_unconfirmed")
	p, _ := j.cloudExecution(a)
	if p.Refresh == nil || p.Refresh.Request.ExpectedVersion != 2 {
		t.Fatal("lost observation request was forgotten")
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(filepath.Dir(j.path))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cfg.Journal = reopened
	r = Execute(context.Background(), a, cfg)
	assertCloudWait(t, r, "delivery_required")
	if len(refreshBodies) != 2 || refreshBodies[0] != refreshBodies[1] {
		t.Fatal("later current GET replaced original observation ACK", refreshBodies)
	}
	p, _ = reopened.cloudExecution(a)
	if p.Refresh != nil || p.Submission.Version != 3 {
		t.Fatal("original ACK was not saved")
	}
}

func TestWorkBuddyCloudOriginalPoolScopeOnlyRelaxesLeaseObservation(t *testing.T) {
	a := poolTestAssignment()
	current := a
	current.LeaseUntil = "2030-01-02T00:00:00Z"
	current.DispatchState = "reconciling"
	c := &Client{ProjectID: a.ProjectID, Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &current}}
	if c.cloudRequestScope(a.ProjectID, candidateRef(a)) {
		t.Fatal("strict original execution check unexpectedly relaxed")
	}
	original, err := originalCloudClientScope(c, a)
	if err != nil || !original.cloudRequestScope(a.ProjectID, candidateRef(a)) || c.Pool.Assignment.LeaseUntil != current.LeaseUntil {
		t.Fatal("original-body clone changed live carrier or failed", err)
	}
	changed := a
	changed.PoolGrantVersion++
	if _, err = originalCloudClientScope(c, changed); err == nil {
		t.Fatal("changed grant admitted as original scope")
	}
}

func TestWorkBuddyCloudWorkerRunClaimsNativeCapabilityAndReplaysWait(t *testing.T) {
	fixture, a := cloudExecutionFixture(t)
	_ = fixture.Close()
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var claims, creates, results atomic.Int32
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/tool-inspections/claim"):
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case strings.HasSuffix(r.URL.Path, "/tool-inventory"):
			fixtureToolInventoryAck195(w, r, time.Now().UTC().Format(time.RFC3339Nano))
		case strings.HasSuffix(r.URL.Path, "/claim"):
			var req struct {
				Capabilities []string `json:"capabilities"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			found := false
			for _, cap := range req.Capabilities {
				found = found || cap == "external_workflow:"+wbc.Plugin
			}
			if !found {
				t.Error("formal Worker did not advertise native cloud support")
			}
			if claims.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"assignment": a})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"assignment": nil})
				select {
				case <-done:
				default:
					close(done)
				}
			}
		case strings.HasSuffix(r.URL.Path, "/lookup"):
			var req wbc.CloudLookupRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "not_found"})
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
		case strings.HasSuffix(r.URL.Path, "/workbuddy-cloud-submissions"):
			creates.Add(1)
			var req wbc.CloudCreateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(cloudFixtureSubmission(a, req, "task_known"))
		case strings.HasSuffix(r.URL.Path, "/results"):
			var req struct {
				Result Result `json:"result"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			assertCloudWait(t, req.Result, "delivery_required")
			results.Add(1)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Error("unexpected non-cloud broker call", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	worker := Worker{Client: &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}, Journal: j, ID: a.WorkerID, PollInterval: time.Millisecond, HeartbeatInterval: 10 * time.Millisecond}
	finished := make(chan error, 1)
	go func() { finished <- worker.Run(ctx) }()
	select {
	case <-done:
		cancel()
	case <-ctx.Done():
		t.Fatal("formal worker did not finish original cloud wait")
	}
	<-finished
	if creates.Load() != 1 || results.Load() != 1 || len(j.Pending()) != 0 {
		t.Fatal("formal cloud outbox did not settle once", creates.Load(), results.Load())
	}
}

func TestWorkBuddyCloudInterruptedNeverClaimsRemoteStop(t *testing.T) {
	j, a := cloudExecutionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Execute(ctx, a, ExecConfig{Journal: j, CloudClient: &Client{BaseURL: "http://127.0.0.1:4319", ProjectID: "project-155", Token: "fixture-token"}})
	assertCloudWait(t, r, "interrupted")
	if err := j.Finish(r); err != nil {
		t.Fatal(err)
	}
}

func TestWorkBuddyCloudWorkerProcess(t *testing.T) {
	if os.Getenv("SUMA_FOLLOW155_WORKER_HELPER") != "1" {
		t.Skip("owned Worker crash-recovery subprocess only")
	}
	j, err := OpenJournal(os.Getenv("SUMA_FOLLOW155_WORKER_STATE"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	w := Worker{Client: &Client{BaseURL: os.Getenv("SUMA_FOLLOW155_WORKER_ORIGIN"), ProjectID: "project-155", Token: "fixture-token"}, Journal: j, ID: "worker-155", PollInterval: 5 * time.Millisecond, HeartbeatInterval: 20 * time.Millisecond}
	if err = w.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkBuddyCloudWorkerCrashBeforeFinishRecoversOriginalAndReplaysLostResultACK(t *testing.T) {
	fixture, a := cloudExecutionFixture(t)
	_ = fixture.Close()
	state := t.TempDir()
	var mu sync.Mutex
	var saved *wbc.CloudSubmission
	var resultBodies []string
	var creates, claims atomic.Int32
	createHeld, done := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/tool-inspections/claim"):
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case strings.HasSuffix(r.URL.Path, "/tool-inventory"):
			fixtureToolInventoryAck195(w, r, time.Now().UTC().Format(time.RFC3339Nano))
		case strings.HasSuffix(r.URL.Path, "/claim"):
			if claims.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"assignment": a})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"assignment": nil})
				select {
				case <-done:
				default:
					close(done)
				}
			}
		case strings.HasSuffix(r.URL.Path, "/lookup"):
			var req wbc.CloudLookupRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			out := wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "not_found"}
			mu.Lock()
			if saved != nil {
				s := *saved
				out.State, out.Submission = s.State, &s
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(out)
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
		case strings.HasSuffix(r.URL.Path, "/workbuddy-cloud-submissions"):
			if creates.Add(1) != 1 {
				t.Error("restarted Worker repeated cloud Create")
			}
			var req wbc.CloudCreateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			s := cloudFixtureSubmission(a, req, "task_known")
			mu.Lock()
			saved = &s
			mu.Unlock()
			close(createHeld)
			<-r.Context().Done() // Original effect is committed; no response reaches the killed process.
		case strings.HasSuffix(r.URL.Path, "/results"):
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			resultBodies = append(resultBodies, string(body))
			n := len(resultBodies)
			mu.Unlock()
			if n == 1 {
				conn, _, _ := w.(http.Hijacker).Hijack()
				_ = conn.Close()
				return
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Error("unexpected cloud worker endpoint", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	process := exec.Command(os.Args[0], "-test.run=^TestWorkBuddyCloudWorkerProcess$", "-test.timeout=20s")
	process.Env = append(os.Environ(), "SUMA_FOLLOW155_WORKER_HELPER=1", "SUMA_FOLLOW155_WORKER_STATE="+state, "SUMA_FOLLOW155_WORKER_ORIGIN="+server.URL)
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Process.Kill() })
	select {
	case <-createHeld:
	case <-time.After(5 * time.Second):
		_ = process.Process.Kill()
		_ = process.Wait()
		t.Fatal("owned child did not reach committed/held cloud Create")
	}
	if err := process.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err == nil {
		t.Fatal("expected intentional owned-process termination")
	}
	j, err := OpenJournal(state)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if len(j.Pending()) != 0 || len(j.pendingCloudExecutions()) != 1 {
		t.Fatal("crash did not leave the real Worker checkpoint before Finish")
	}
	p, err := j.cloudExecution(a)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	w := Worker{Client: &Client{BaseURL: server.URL, ProjectID: "project-155", Token: "fixture-token"}, Journal: j, ID: a.WorkerID, PollInterval: 5 * time.Millisecond, HeartbeatInterval: 20 * time.Millisecond}
	finished := make(chan error, 1)
	go func() { finished <- w.Run(ctx) }()
	select {
	case <-done:
		cancel()
	case <-ctx.Done():
		t.Fatal("original cloud recovery/outbox replay did not finish")
	}
	<-finished
	mu.Lock()
	defer mu.Unlock()
	if creates.Load() != 1 || len(resultBodies) != 2 || resultBodies[0] != resultBodies[1] || len(j.Pending()) != 0 {
		t.Fatal("recovery duplicated create or changed original result", creates.Load(), len(resultBodies))
	}
	var sent struct {
		Result Result `json:"result"`
	}
	if json.Unmarshal([]byte(resultBodies[0]), &sent) != nil {
		t.Fatal("invalid result transport")
	}
	assertCloudWait(t, sent.Result, "delivery_required")
	if sent.Result.OperationID != p.ResultOperationID || sent.Result.WorkBuddyCloudWait.CreateOperationID != p.Create.IdempotencyKey {
		t.Fatal("restart replaced original operations")
	}
}
