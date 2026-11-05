package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestWorkerShutdownPreservesOriginalResult(t *testing.T) {
	for _, tc := range []struct {
		name       string
		unknown    bool
		historical bool
	}{
		{name: "known_without_remote_ack"},
		{name: "outcome_unknown", unknown: true},
		{name: "historical_unknown", unknown: true, historical: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			assignment := Assignment{RunID: "run", NodeID: "develop", Attempt: 3, Fence: 7, WorkerID: "worker", DispatchID: "dispatch"}
			if err = j.Accept(assignment); err != nil {
				t.Fatal(err)
			}
			result := Result{Assignment: assignment, OperationID: "original-result", Stopped: !tc.unknown, OutcomeUnknown: tc.unknown, ExitCode: -1}
			if err = j.Finish(result); err != nil {
				t.Fatal(err)
			}
			if tc.historical {
				if err = j.Ack(result.OperationID); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("local shutdown must not require a remote response: %s", r.URL.Path)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			worker := Worker{Journal: j, ID: "worker", Client: &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture"}}
			err = worker.Run(ctx)
			if (err != nil) != (tc.unknown && !tc.historical) {
				t.Fatalf("unexpected stop verdict: %v", err)
			}
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			original := reopened.entries[key(assignment)]
			if original.Acked != tc.historical || original.Result == nil || original.Result.OperationID != result.OperationID || original.Result.OutcomeUnknown != tc.unknown {
				t.Fatalf("original result did not survive shutdown: %+v", original.Result)
			}
		})
	}
}

func TestWorkerShutdownDoesNotTrustAnUnpersistedACK(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "run", NodeID: "develop", Attempt: 1, Fence: 1, WorkerID: "worker"}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.Finish(Result{Assignment: a, OperationID: "original", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	path := j.path
	j.path = filepath.Join(dir, "nonexistent", "journal.json")
	err = j.Ack("original")
	j.path = path
	if err == nil {
		t.Fatal("ACK persistence must fail in the missing directory")
	}
	if pending := j.Pending(); len(pending) != 1 || pending[0].OperationID != "original" {
		t.Fatal("an undurable ACK erased the original pending result")
	}
}

func TestWorkerShutdownPreservesUnansweredClaim(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("shutdown must not retry or replace the unanswered claim: %s", r.URL.Path)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	original, err := j.BeginClaim(server.URL, "project", "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker := Worker{Journal: j, ID: "worker", Client: &Client{BaseURL: server.URL, ProjectID: "project", Token: "fixture"}}
	if err = worker.Run(ctx); err != nil {
		t.Fatalf("a durable unanswered claim without local execution must allow a local stop: %v", err)
	}
	recovered, err := j.BeginClaim(server.URL, "project", "worker", nil)
	if err != nil || recovered.OperationID != original.OperationID || recovered.Complete || recovered.Responded {
		t.Fatalf("original claim changed during shutdown: %+v (%v)", recovered, err)
	}
}

func TestWorkerShutdownRejectsUnfinishedAcceptedExecution(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "run", NodeID: "develop", Attempt: 1, Fence: 1, WorkerID: "worker"}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker := Worker{Journal: j, ID: "worker", Client: &Client{BaseURL: "http://127.0.0.1:4319", ProjectID: "project", Token: "fixture"}}
	if err = worker.Run(ctx); err == nil {
		t.Fatal("an accepted execution without a known outcome cannot prove local stop")
	}
	if original, ok := j.entries[key(a)]; !ok || original.Result != nil {
		t.Fatal("the unresolved original execution was modified")
	}
}
