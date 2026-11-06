package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/ai-mada/suma/internal/executioncontract"
)

func TestStaleProgressReplayPreservesRejectionAndDeliversStop(t *testing.T) {
	for _, kind := range []string{"runner", "pool-runner"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			a := Assignment{RunID: "r", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1}
			if kind == "pool-runner" {
				a = poolTestAssignment()
			}
			if err := j.Accept(a); err != nil {
				t.Fatal(err)
			}
			for _, message := range []string{"undelivered progress", "previously committed progress"} {
				if err := j.RecordProgress(a, message); err != nil {
					t.Fatal(err)
				}
			}
			original := j.PendingProgress()
			if err := j.Finish(Result{Assignment: a, Stopped: true}); err != nil {
				t.Fatal(err)
			}
			finalID := j.Pending()[0].OperationID
			var calls []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					OperationID string `json:"operation_id"`
					Result      Result `json:"result"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				switch r.URL.Path {
				case "/api/v1/" + kind + "/progress":
					calls = append(calls, body.OperationID)
					if body.OperationID == original[0].OperationID {
						code := executioncontract.RunnerProgressStaleCode
						status := http.StatusConflict
						switch len(calls) {
						case 1:
							status = http.StatusServiceUnavailable // Same code at another status is insufficient.
						case 2:
							code = "version_conflict"
						}
						w.WriteHeader(status)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
						return
					}
				case "/api/v1/" + kind + "/results":
					calls = append(calls, body.Result.OperationID)
					if !body.Result.Stopped || body.Result.OperationID != finalID {
						t.Error("original stop result was not replayed")
					}
					if len(calls) == 5 {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, Token: "fixture", ProjectID: "p"}
			replay := Replay
			if kind == "pool-runner" {
				c.Pool = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}
				replay = replayPool
			}
			for i := 1; i <= 2; i++ {
				if err := replay(ctx, c, j); err == nil || len(calls) != i || len(j.PendingProgress()) != 2 {
					t.Fatalf("uncertain rejection advanced the queue: %v, %v", err, calls)
				}
			}
			if err := replay(ctx, c, j); err == nil || len(j.PendingProgress()) != 0 || len(j.Pending()) != 1 {
				t.Fatalf("stale progress did not release final replay: %v, %v", err, calls)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			j = reopened
			progress := j.entries[key(a)].Progress
			if len(progress) != 2 || progress[0].OperationID != original[0].OperationID || progress[0].Message != original[0].Message ||
				progress[0].Rejection != ProgressRejectedStaleAttempt || progress[0].Acked || !progress[1].Acked || len(j.PendingProgress()) != 0 {
				t.Fatalf("journal lost the distinct rejected/acknowledged evidence: %+v", progress)
			}
			if err := replay(ctx, c, j); err != nil || len(j.Pending()) != 0 {
				t.Fatalf("stop result did not survive restart: %v", err)
			}
			want := []string{original[0].OperationID, original[0].OperationID, original[0].OperationID, original[1].OperationID, finalID, finalID}
			if !reflect.DeepEqual(calls, want) {
				t.Fatalf("replay order = %v, want %v", calls, want)
			}
		})
	}
}
