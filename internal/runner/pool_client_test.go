package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func poolTestAssignment() Assignment {
	return Assignment{TenantID: "tenant", ProjectID: "project", PoolID: "pool", PoolRunnerID: "runner", PoolSlotID: "0", PoolGrantID: "grant", PoolGrantVersion: 2, PoolCredentialID: "credential", PoolCredentialVersion: 1, RunID: "run", NodeID: "node", Attempt: 1, Fence: 1, WorkerID: "pool-runner", DispatchID: "dispatch", DispatchState: "active", LeaseUntil: "2030-01-01T00:00:00Z"}
}

func TestPoolClientUsesFrozenAssignmentScopeForHeartbeatAndResult(t *testing.T) {
	a := poolTestAssignment()
	var paths []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/api/v1/pool-runner/heartbeat" && r.URL.Path != "/api/v1/pool-runner/results" {
			http.Error(w, "wrong execution identity route", 404)
			return
		}
		var body struct {
			Assignment Assignment `json:"assignment"`
			ProjectID  string     `json:"project_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Assignment.PoolGrantID != a.PoolGrantID || body.Assignment.TenantID != a.TenantID || body.Assignment.ProjectID != a.ProjectID || body.ProjectID != a.ProjectID {
			t.Error("frozen scope absent or inherited a mutable project")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"cancel_requested":false,"acknowledged":true}`))
	}))
	defer s.Close()
	c := &Client{BaseURL: s.URL, Token: "fixture-pool-token", ProjectID: "wrong-mutable-project", Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}}
	if _, err := c.Heartbeat(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := c.Complete(context.Background(), Result{Assignment: a, OperationID: "result-operation"}); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatal(paths)
	}
}

func TestPoolJournalDoesNotAliasDifferentScopeAssignments(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := poolTestAssignment()
	b := a
	b.TenantID = "other-tenant"
	b.ProjectID = "other-project"
	b.PoolGrantID = "other-grant"
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.Accept(b); err != nil {
		t.Fatal("different scopes aliased the same inbox", err)
	}
}

func TestPoolJournalReloadRestoresFrozenResultScope(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := poolTestAssignment()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.Finish(Result{Assignment: a, OperationID: "fixed-result"}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	pending := j.Pending()
	if len(pending) != 1 || !samePoolAssignmentIdentity(pending[0].Assignment, a) {
		t.Fatal("restart lost frozen pool scope in completion outbox")
	}
}
