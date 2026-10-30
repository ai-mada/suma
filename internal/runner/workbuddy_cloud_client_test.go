package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ai-mada/suma/internal/workbuddy"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudClientRequest(a Assignment) wbc.CloudCreateRequest {
	return wbc.CloudCreateRequest{ProjectID: "project", DelegationID: "delegation-152", ExpectedVersion: 1, Assignment: candidateRef(a), IdempotencyKey: "operation-152"}
}

func cloudClientSubmission(req wbc.CloudCreateRequest) wbc.CloudSubmission {
	return wbc.CloudSubmission{ID: "submission-152", ProjectID: req.ProjectID, DelegationID: req.DelegationID, Version: 2, RunID: req.Assignment.RunID, NodeID: req.Assignment.NodeID, Assignment: req.Assignment, RequestSHA256: strings.Repeat("a", 64), State: "task_known", Task: &workbuddy.Task{ID: "task-152", Status: "working"}, ObservedAt: "2026-10-30T08:00:00Z", UsageState: "unreported", CreatedAt: "2026-10-30T08:00:00Z", UpdatedAt: "2026-10-30T08:00:00Z"}
}

func TestWorkBuddyCloudClientUsesExactControlScopeAndNoVendorCredential(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "project", true: "pool"}[pool], func(t *testing.T) {
			a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
			if pool {
				a = poolTestAssignment()
			}
			req := cloudClientRequest(a)
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				ns := "runner"
				if pool {
					ns = "pool-runner"
				}
				if r.Method != "POST" || r.URL.Path != "/api/v1/"+ns+"/workbuddy-cloud-submissions" || r.Header.Get("Authorization") != "Bearer platform-machine-only" {
					t.Error("wrong route or identity")
				}
				raw, _ := io.ReadAll(r.Body)
				var got wbc.CloudCreateRequest
				if wbc.Decode(raw, &got) != nil || got != req {
					t.Error("frozen request changed")
				}
				if strings.Contains(string(raw), "prompt") || strings.Contains(string(raw), "token") {
					t.Error("provider input or credential leaked")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(cloudClientSubmission(req))
			}))
			defer s.Close()
			c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project"}
			if pool {
				c.Pool = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}
			}
			out, err := c.CreateWorkBuddyCloud(context.Background(), req)
			if err != nil || out.Task == nil || out.Task.ID != "task-152" || calls.Load() != 1 {
				t.Fatal("exact create not observed", err)
			}
		})
	}
}

func TestWorkBuddyCloudClientRejectsScopeDriftBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer s.Close()
	a := poolTestAssignment()
	c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project", Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}}
	for _, change := range []func(*wbc.CloudCreateRequest){
		func(r *wbc.CloudCreateRequest) { r.ProjectID = "other" },
		func(r *wbc.CloudCreateRequest) { r.Assignment.Fence++ },
		func(r *wbc.CloudCreateRequest) { r.Assignment.PoolGrantVersion++ },
		func(r *wbc.CloudCreateRequest) { r.Assignment.PoolID = "" },
	} {
		req := cloudClientRequest(a)
		change(&req)
		if _, err := c.CreateWorkBuddyCloud(context.Background(), req); err == nil {
			t.Fatal("changed frozen scope accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached network")
	}
}

func TestWorkBuddyCloudClientRejectsUnrelatedOrSecretResponse(t *testing.T) {
	a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
	req := cloudClientRequest(a)
	for _, variant := range []string{"project", "delegation", "attempt", "task_extra", "duplicate", "tail", "prepared"} {
		t.Run(variant, func(t *testing.T) {
			reply := cloudClientSubmission(req)
			switch variant {
			case "project":
				reply.ProjectID = "other"
			case "delegation":
				reply.DelegationID = "other"
			case "attempt":
				reply.Assignment.Fence++
			case "prepared":
				reply.State = "prepared"
				reply.Task = nil
				reply.ObservedAt = ""
			}
			raw, _ := json.Marshal(reply)
			switch variant {
			case "task_extra":
				raw = []byte(strings.Replace(string(raw), `"status":"working"`, `"status":"working","token":"private-acp"`, 1))
			case "duplicate":
				raw = []byte(strings.TrimSuffix(string(raw), "}") + `,"id":"unrelated"}`)
			case "tail":
				raw = append(raw, []byte(` {}`)...)
			}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(raw)
			}))
			defer s.Close()
			c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project"}
			out, err := c.CreateWorkBuddyCloud(context.Background(), req)
			if err == nil || out.ID != "" || strings.Contains(err.Error(), "private-acp") {
				t.Fatal("invalid control response trusted or exposed", err)
			}
		})
	}
}

func TestWorkBuddyCloudClientRefreshPinsOriginalTaskAndVersion(t *testing.T) {
	a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
	original := cloudClientSubmission(cloudClientRequest(a))
	req := wbc.CloudRefreshRequest{ProjectID: "project", Assignment: candidateRef(a), ExpectedVersion: original.Version, IdempotencyKey: "refresh-152"}
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/v1/runner/workbuddy-cloud-submissions/submission-152/refresh" {
			t.Error("wrong exact submission")
		}
		reply := original
		reply.Version++
		if calls.Load() > 1 {
			task := *reply.Task
			task.ID = "other-task"
			reply.Task = &task
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer s.Close()
	c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project"}
	if out, err := c.RefreshWorkBuddyCloud(context.Background(), original, req); err != nil || out.Version != original.Version+1 {
		t.Fatal(err)
	}
	if _, err := c.RefreshWorkBuddyCloud(context.Background(), original, req); err == nil {
		t.Fatal("different provider task adopted")
	}
	req.ExpectedVersion++
	if _, err := c.RefreshWorkBuddyCloud(context.Background(), original, req); err == nil || calls.Load() != 2 {
		t.Fatal("changed source version sent")
	}
}

func TestWorkBuddyCloudClientNeverRedirectsReplaysOrExposesHTTPBody(t *testing.T) {
	a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
	req := cloudClientRequest(a)
	var escaped atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer target.Close()
	for _, code := range []int{307, 503} {
		var calls atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", target.URL)
			w.WriteHeader(code)
			_, _ = w.Write([]byte("private-control-body"))
		}))
		c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project", HTTP: s.Client()}
		_, err := c.CreateWorkBuddyCloud(context.Background(), req)
		var failure *workBuddyCloudControlError
		if !errors.As(err, &failure) || failure.StatusCode != code || calls.Load() != 1 || strings.Contains(err.Error(), "private-control-body") {
			t.Fatal("HTTP failure not bounded and safe", err)
		}
		s.Close()
	}
	if escaped.Load() != 0 {
		t.Fatal("control identity followed redirect")
	}
}
