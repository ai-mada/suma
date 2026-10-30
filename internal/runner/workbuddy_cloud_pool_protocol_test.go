package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	wb "github.com/ai-mada/suma/internal/workbuddy"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudPoolCallbacksKeepProtectedBindingWithoutWideningRefRequests(t *testing.T) {
	a := poolTestAssignment()
	a.Type = wbc.NodeType
	a.WorkBuddyCloud = &wbc.CloudAssignmentBinding{DelegationID: "delegation-155", DelegationVersion: 1, DelegationSHA256: strings.Repeat("a", 64)}
	a.Config = map[string]any{"plugin": wbc.Plugin, "workflow": wbc.Protocol, "workbuddy_cloud": wbc.NodeBinding{ConnectionID: "connection-155", GrantGeneration: 1, Request: wb.CreateRequest{Prompt: "must-not-enter-callback-envelope"}}}
	callbacks, refs := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var raw map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&raw) != nil {
			t.Error("invalid callback body")
			w.WriteHeader(400)
			return
		}
		var assignmentFields map[string]json.RawMessage
		if json.Unmarshal(raw["assignment"], &assignmentFields) != nil {
			t.Error("assignment missing")
			w.WriteHeader(400)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/lookup") || strings.HasSuffix(r.URL.Path, "/workbuddy-cloud-submissions") {
			refs++
			if assignmentFields["workbuddy_cloud"] != nil || assignmentFields["config"] != nil {
				t.Error("AssignmentRef protocol widened")
				w.WriteHeader(400)
				return
			}
			var refRequest wbc.CloudCreateRequest
			if strings.HasSuffix(r.URL.Path, "/lookup") {
				var req wbc.CloudLookupRequest
				_ = json.Unmarshal(mustJSON(raw), &req)
				s := cloudFixtureSubmission(a, wbc.CloudCreateRequest{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment}, "task_known")
				_ = json.NewEncoder(w).Encode(wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "task_known", Submission: &s})
				return
			}
			_ = json.Unmarshal(mustJSON(raw), &refRequest)
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(cloudFixtureSubmission(a, refRequest, "task_known"))
			return
		}
		callbacks++
		var supplied Assignment
		if json.Unmarshal(raw["assignment"], &supplied) != nil || supplied.WorkBuddyCloud == nil || *supplied.WorkBuddyCloud != *a.WorkBuddyCloud || !samePoolAssignmentIdentity(supplied, a) {
			t.Error("strict full callback envelope lost original protected binding")
			w.WriteHeader(409)
			return
		}
		if assignmentFields["config"] != nil || assignmentFields["command"] != nil {
			t.Error("callback copied prompt or execution inputs")
		}
		_, _ = w.Write([]byte(`{"acknowledged":true,"cancel_requested":false}`))
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, Token: "fixture-token", ProjectID: a.ProjectID, Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}}
	ctx := context.Background()
	if _, err := c.Heartbeat(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := c.Progress(ctx, Progress{Assignment: a, OperationID: "progress-155", Message: "cloud observation"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Complete(ctx, Result{Assignment: a, ExitCode: -1, OutcomeUnknown: true, OperationID: "result-155"}); err != nil {
		t.Fatal(err)
	}
	req := wbc.CloudCreateRequest{ProjectID: a.ProjectID, DelegationID: a.WorkBuddyCloud.DelegationID, ExpectedVersion: 1, Assignment: candidateRef(a), IdempotencyKey: "create-155"}
	if _, err := c.CreateWorkBuddyCloud(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LookupWorkBuddyCloud(ctx, wbc.CloudLookupRequest{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment}); err != nil {
		t.Fatal(err)
	}
	if callbacks != 3 || refs != 2 {
		t.Fatal("wrong callback/ref coverage", callbacks, refs)
	}
	projected := poolAssignmentIdentity(a)
	a.WorkBuddyCloud.DelegationVersion++
	if projected.WorkBuddyCloud == nil || projected.WorkBuddyCloud.DelegationVersion != 1 {
		t.Fatal("callback projection aliases caller binding")
	}
}

func mustJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
