package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/executioncontract"
)

func TestClosedResultReplayRetainsOriginalAndUnblocksWorker(t *testing.T) {
	for _, kind := range []string{"runner", "pool-runner"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			j, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
			if kind == "pool-runner" {
				a = poolTestAssignment()
			}
			a.Type = "command"
			if err := j.Accept(a); err != nil {
				t.Fatal(err)
			}
			if err := j.Finish(Result{Assignment: a, Output: "original uncommitted output"}); err != nil {
				t.Fatal(err)
			}
			original := j.Pending()[0]
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var body struct{ Result Result }
				if req.URL.Path != "/api/v1/"+kind+"/results" || json.NewDecoder(req.Body).Decode(&body) != nil {
					t.Error("unexpected result request")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if body.Result.RunID == "follow-up-run" && body.Result.Output == "follow-up output" {
					calls.Add(1)
					w.WriteHeader(http.StatusOK)
					return
				}
				if body.Result.OperationID != original.OperationID || body.Result.Output != original.Output || body.Result.Fence != a.Fence {
					t.Error("unexpected replayed result")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				status, code := http.StatusConflict, executioncontract.RunnerResultClosedCode
				switch calls.Add(1) {
				case 1:
					status = http.StatusServiceUnavailable
				case 2:
					code = "version_conflict"
				}
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
			}))
			defer srv.Close()
			c := &Client{BaseURL: srv.URL, Token: "fixture", ProjectID: "project"}
			replay := Replay
			if kind == "pool-runner" {
				c.Pool = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}
				replay = replayPool
			}
			for range 2 {
				if err := replay(ctx, c, j); err == nil || len(j.Pending()) != 1 {
					t.Fatalf("unproven response advanced the result: %v", err)
				}
			}
			if err := replay(ctx, c, j); err != nil || len(j.Pending()) != 0 {
				t.Fatalf("definitive closed attempt still blocked replay: %v", err)
			}
			if err := j.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenJournal(dir)
			if err != nil {
				t.Fatal(err)
			}
			j = reopened
			entry := j.entries[key(a)]
			if entry.Acked || entry.ResultRejection != ResultRejectedClosedAttempt || entry.Result == nil ||
				entry.Result.OperationID != original.OperationID || entry.Result.Output != original.Output {
				t.Fatal("closed result was changed or falsely acknowledged")
			}
			if err := replay(ctx, c, j); err != nil || calls.Load() != 3 {
				t.Fatalf("closed result was sent again after restart: %v", err)
			}
			next := a
			next.RunID, next.DispatchID = "follow-up-run", "follow-up-dispatch"
			if err := j.Accept(next); err != nil {
				t.Fatal(err)
			}
			if err := j.Finish(Result{Assignment: next, Output: "follow-up output"}); err != nil {
				t.Fatal(err)
			}
			if err := replay(ctx, c, j); err != nil || calls.Load() != 4 || len(j.Pending()) != 0 || !j.entries[key(next)].Acked {
				t.Fatalf("next execution did not deliver and acknowledge: %v", err)
			}
		})
	}
}
