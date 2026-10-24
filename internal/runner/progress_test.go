package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProgressSurvivesLostAckAndRestartBeforeFinalResult(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "r", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.RecordProgress(a, "real output\n"); err != nil {
		t.Fatal(err)
	}
	if err = j.Finish(Result{Assignment: a, ExitCode: 0, Output: "final"}); err != nil {
		t.Fatal(err)
	}
	p := j.PendingProgress()
	if len(p) != 1 {
		t.Fatal(p)
	}
	id := p[0].OperationID
	calls := []string{}
	reject := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing credential")
		}
		calls = append(calls, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "progress") {
			var body struct {
				Project     string     `json:"project_id"`
				Assignment  Assignment `json:"assignment"`
				OperationID string     `json:"operation_id"`
				Message     string     `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Project != "p" || body.OperationID != id || body.Message != "real output\n" || body.Assignment.WorkerID != "w" {
				t.Errorf("progress wire: %+v", body)
			}
			if reject {
				w.WriteHeader(503)
				return
			}
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "secret", ProjectID: "p"}
	if err = Replay(context.Background(), c, j); err == nil {
		t.Fatal("lost progress ACK accepted")
	}
	if len(calls) != 1 || !strings.HasSuffix(calls[0], "progress") || len(j.Pending()) != 1 {
		t.Fatal("result passed undelivered progress")
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if len(j.PendingProgress()) != 1 || j.PendingProgress()[0].OperationID != id {
		t.Fatal("progress replay identity lost")
	}
	reject = false
	if err = Replay(context.Background(), c, j); err != nil {
		t.Fatal(err)
	}
	if len(j.PendingProgress()) != 0 || len(j.Pending()) != 0 || len(calls) != 3 {
		t.Fatal("durable ACK missing")
	}
	if err = j.RecordProgress(a, "late"); err == nil {
		t.Fatal("progress after final result accepted")
	}
	if err = j.Finish(Result{Assignment: a, ExitCode: 9}); err == nil {
		t.Fatal("final result overwritten")
	}
}

func TestProgressCannotLeaveMemoryBeforeDurableSave(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "r", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	savedPath := j.path
	j.path = filepath.Join(t.TempDir(), "missing", "journal.json")
	if err = j.RecordProgress(a, "not durable"); err == nil {
		t.Fatal("missing storage accepted")
	}
	if len(j.PendingProgress()) != 0 {
		t.Fatal("undurable progress became uploadable")
	}
	j.path = savedPath
	// Saturating bounded progress never converts a successful command into a
	// failure; final output remains a separate durable result.
	e := j.entries[key(a)]
	e.Progress = make([]Progress, 1024)
	j.entries[key(a)] = e
	if err = j.RecordProgress(a, "beyond bound"); err != nil {
		t.Fatal(err)
	}
	if !j.entries[key(a)].ProgressTruncated || len(j.entries[key(a)].Progress) != 1024 {
		t.Fatal("progress bound missing")
	}
}

func TestSlowProgressUploadDoesNotBlockHeartbeat(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "r", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1, Type: "command", Command: []string{"output"}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	var beats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "heartbeat") {
			beats.Add(1)
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
			return
		}
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
		w.WriteHeader(503)
	}))
	defer srv.Close()
	exe, _ := os.Executable()
	w := Worker{Client: &Client{BaseURL: srv.URL, Token: "secret", ProjectID: "p"}, Journal: j, Exec: ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"output": {Executable: exe, ReadOnly: true, Args: []string{"-test.run=TestProgressOutputHelper", "--"}, Env: map[string]string{"SUMA_PROGRESS_HELPER": "long"}}}}}
	if err = w.runAssignment(context.Background(), a, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if beats.Load() < 2 {
		t.Fatalf("heartbeats starved by progress: %d", beats.Load())
	}
	if len(j.Pending()) != 1 || len(j.PendingProgress()) == 0 {
		t.Fatal("lost final or pending progress")
	}
}

func TestProgressLostAckReplaysThroughRealDurableScheduler(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "scheduler.db")
	db, err := store.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err = engine.Publish(ctx, "t", "p", dag.Workflow{ID: "wf", Name: "actual progress", Nodes: []dag.Node{{ID: "n", Type: "command", Command: []string{"local"}}}}); err != nil {
		t.Fatal(err)
	}
	run, err := engine.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := engine.Claim(ctx, "t", "p", "w", nil)
	if err != nil || assignment == nil {
		t.Fatalf("claim: %v", err)
	}
	var a Assignment
	b, _ := json.Marshal(assignment)
	_ = json.Unmarshal(b, &a)
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.RecordProgress(a, "actual token=hidden\n"); err != nil {
		t.Fatal(err)
	}
	lostAck := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "progress") {
			var body struct {
				Assignment  dag.Assignment `json:"assignment"`
				OperationID string         `json:"operation_id"`
				Message     string         `json:"message"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if err := engine.Progress(ctx, "t", "p", body.Assignment, body.OperationID, body.Message); err != nil {
				t.Error(err)
				w.WriteHeader(409)
				return
			}
			if lostAck {
				w.WriteHeader(503)
				return
			}
			w.WriteHeader(200)
			return
		}
		var body struct {
			Result dag.Result `json:"result"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if err := engine.Complete(ctx, "t", "p", body.Result); err != nil {
			t.Error(err)
			w.WriteHeader(409)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "secret", ProjectID: "p"}
	if err = ReplayProgress(ctx, c, j); err == nil {
		t.Fatal("lost ACK was forgotten")
	}
	id := j.PendingProgress()[0].OperationID
	if err = j.Finish(Result{Assignment: a, ExitCode: 0, Output: "authoritative final"}); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	lostAck = false
	if j.PendingProgress()[0].OperationID != id {
		t.Fatal("crash changed operation")
	}
	if err = Replay(ctx, c, j); err != nil {
		t.Fatal(err)
	}
	events, err := engine.Events(ctx, "t", "p", run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, e := range events {
		if e.Type == "node_progress" {
			count++
			if strings.Contains(fmt.Sprint(e.Data), "hidden") {
				t.Fatal("secret persisted")
			}
		}
	}
	if count != 1 {
		t.Fatalf("duplicated real event: %d", count)
	}
	final, err := engine.GetRun(ctx, "t", "p", run.ID)
	if err != nil || final.State != "succeeded" || final.Nodes[0].Output != "authoritative final" {
		t.Fatalf("final invariant: %+v %v", final, err)
	}
}

func TestRealOutputProgressIsBoundedAndRedactsSplitSecret(t *testing.T) {
	exe, _ := os.Executable()
	messages := []string{}
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Secrets: []string{"private-secret"}, Commands: map[string]CommandSpec{"output": {Executable: exe, ReadOnly: true, Args: []string{"-test.run=TestProgressOutputHelper", "--"}, Env: map[string]string{"SUMA_PROGRESS_HELPER": "1"}}}, Progress: func(message string) error { messages = append(messages, message); return nil }}
	r := Execute(context.Background(), Assignment{Type: "command", Command: []string{"output"}}, cfg)
	if r.ExitCode != 0 || len(messages) < 2 || !strings.Contains(strings.Join(messages, ""), "real first") {
		t.Fatalf("no actual streaming: %+v %q", r, messages)
	}
	for _, s := range messages {
		if len(s) > 8192 || strings.Contains(s, "private-secret") || strings.Contains(s, "private-") {
			t.Fatalf("unsafe progress %q", s)
		}
	}
	if !strings.Contains(r.Output, "real last") || strings.Contains(r.Output, "private-secret") {
		t.Fatal("final output changed")
	}
}

func TestProgressOutputHelper(t *testing.T) {
	if mode := os.Getenv("SUMA_PROGRESS_HELPER"); mode != "" {
		fmt.Print("real first\nprivate-")
		if mode == "long" {
			time.Sleep(200 * time.Millisecond)
		} else {
			time.Sleep(30 * time.Millisecond)
		}
		fmt.Print("secret\nreal last\n")
		os.Exit(0)
	}
}
