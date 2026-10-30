package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/workbuddy"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudLookupRequest(a Assignment) wbc.CloudLookupRequest {
	return wbc.CloudLookupRequest{ProjectID: "project", DelegationID: "delegation-155", Assignment: candidateRef(a)}
}

func cloudLookupResult(req wbc.CloudLookupRequest, state string) wbc.CloudLookupResult {
	out := wbc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: state}
	if state == "not_found" {
		return out
	}
	out.Submission = &wbc.CloudSubmission{ID: "submission-155", ProjectID: req.ProjectID, DelegationID: req.DelegationID, Version: 1, RunID: req.Assignment.RunID, NodeID: req.Assignment.NodeID, Assignment: req.Assignment, RequestSHA256: strings.Repeat("a", 64), State: state, UsageState: "unreported", CreatedAt: "2026-10-30T08:00:00Z", UpdatedAt: "2026-10-30T08:00:00Z"}
	if state == "task_known" {
		out.Submission.Task = &workbuddy.Task{ID: "original-task-155", Status: "working"}
		out.Submission.ObservedAt = out.Submission.UpdatedAt
	}
	return out
}

func TestWorkBuddyCloudLookupExactRoutesAndFourStoredStates(t *testing.T) {
	for _, pool := range []bool{false, true} {
		for _, state := range []string{"not_found", "prepared", "unknown", "task_known"} {
			namespace := "runner"
			if pool {
				namespace = "pool-runner"
			}
			t.Run(namespace+"/"+state, func(t *testing.T) {
				a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
				if pool {
					a = poolTestAssignment()
				}
				request := cloudLookupRequest(a)
				current := a
				current.LeaseUntil, current.DispatchState = "2031-01-01T00:00:00Z", "closed"
				var calls atomic.Int32
				s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Method != http.MethodPost || r.URL.Path != "/api/v1/"+namespace+"/workbuddy-cloud-submissions/lookup" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer platform-machine-only" || r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" {
						t.Error("lookup route or fixed platform headers changed")
					}
					raw, _ := io.ReadAll(r.Body)
					var got wbc.CloudLookupRequest
					if wbc.Decode(raw, &got) != nil || got != request || strings.Contains(string(raw), "idempotency") || strings.Contains(string(raw), "prompt") || strings.Contains(string(raw), "token") {
						t.Error("lookup replaced the original identity or carried a mutation/provider field")
					}
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					_ = json.NewEncoder(w).Encode(cloudLookupResult(request, state))
				}))
				defer s.Close()
				c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project"}
				if pool {
					c.Pool = &PoolContext{PoolID: current.PoolID, RunnerID: current.PoolRunnerID, Assignment: &current}
				}
				out, err := c.LookupWorkBuddyCloud(context.Background(), request)
				if err != nil || out.Validate() != nil || out.State != state || out.Assignment != request.Assignment || calls.Load() != 1 {
					t.Fatal("exact stored lookup failed", state, err)
				}
				if pool && (current.LeaseUntil != "2031-01-01T00:00:00Z" || current.DispatchState != "closed") {
					t.Fatal("lookup mutated the current Pool context")
				}
			})
		}
	}
}

func TestWorkBuddyCloudLookupRejectsImmutableScopeDriftBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
	defer s.Close()
	a := poolTestAssignment()
	c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project", Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}}
	cases := map[string]func(*wbc.CloudLookupRequest){
		"project":            func(r *wbc.CloudLookupRequest) { r.ProjectID = "another" },
		"delegation":         func(r *wbc.CloudLookupRequest) { r.DelegationID = "" },
		"run":                func(r *wbc.CloudLookupRequest) { r.Assignment.RunID = "another" },
		"node":               func(r *wbc.CloudLookupRequest) { r.Assignment.NodeID = "another" },
		"attempt":            func(r *wbc.CloudLookupRequest) { r.Assignment.Attempt++ },
		"fence":              func(r *wbc.CloudLookupRequest) { r.Assignment.Fence++ },
		"worker":             func(r *wbc.CloudLookupRequest) { r.Assignment.WorkerID = "another" },
		"tenant":             func(r *wbc.CloudLookupRequest) { r.Assignment.TenantID = "another" },
		"assignment_project": func(r *wbc.CloudLookupRequest) { r.Assignment.ProjectID = "another" },
		"pool":               func(r *wbc.CloudLookupRequest) { r.Assignment.PoolID = "another" },
		"runner":             func(r *wbc.CloudLookupRequest) { r.Assignment.PoolRunnerID = "another" },
		"slot":               func(r *wbc.CloudLookupRequest) { r.Assignment.PoolSlotID = "1" },
		"grant":              func(r *wbc.CloudLookupRequest) { r.Assignment.PoolGrantID = "another" },
		"grant_version":      func(r *wbc.CloudLookupRequest) { r.Assignment.PoolGrantVersion++ },
		"credential":         func(r *wbc.CloudLookupRequest) { r.Assignment.PoolCredentialID = "another" },
		"credential_version": func(r *wbc.CloudLookupRequest) { r.Assignment.PoolCredentialVersion++ },
		"dispatch":           func(r *wbc.CloudLookupRequest) { r.Assignment.DispatchID = "another" },
		"stripped_pool": func(r *wbc.CloudLookupRequest) {
			r.Assignment = cc.AssignmentRef{RunID: "run", NodeID: "node", Attempt: 1, Fence: 1, WorkerID: a.WorkerID}
		},
		"malformed_lease": func(r *wbc.CloudLookupRequest) { r.Assignment.LeaseUntil = "unknown" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			request := cloudLookupRequest(a)
			change(&request)
			out, err := c.LookupWorkBuddyCloud(context.Background(), request)
			if err == nil || out.State != "" {
				t.Fatal("invalid lookup scope obtained a result")
			}
		})
	}
	private := &Client{BaseURL: s.URL, Token: c.Token, ProjectID: "project"}
	if _, err := private.LookupWorkBuddyCloud(context.Background(), cloudLookupRequest(a)); err == nil {
		t.Fatal("private identity stripped a Pool lookup")
	}
	c.Pool.RunnerID = "another-wrapper"
	if _, err := c.LookupWorkBuddyCloud(context.Background(), cloudLookupRequest(a)); err == nil {
		t.Fatal("current Pool wrapper did not match immutable assignment")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid scope reached network", calls.Load())
	}
}

func TestWorkBuddyCloudLookupRejectsUnrelatedOrUnsafeResponses(t *testing.T) {
	a := Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1}
	request := cloudLookupRequest(a)
	for _, variant := range []string{"project", "delegation", "attempt", "state", "missing_submission", "extra", "task_secret", "duplicate", "trailing", "mime", "oversize"} {
		t.Run(variant, func(t *testing.T) {
			out := cloudLookupResult(request, "task_known")
			switch variant {
			case "project":
				out.ProjectID, out.Submission.ProjectID = "other", "other"
			case "delegation":
				out.DelegationID, out.Submission.DelegationID = "other", "other"
			case "attempt":
				out.Assignment.Attempt++
				out.Submission.Assignment = out.Assignment
			case "state":
				out.State = "accepted"
			case "missing_submission":
				out.Submission = nil
			}
			raw, _ := json.Marshal(out)
			switch variant {
			case "extra":
				raw = []byte(strings.TrimSuffix(string(raw), "}") + `,"token":"private-secret"}`)
			case "task_secret":
				raw = []byte(strings.Replace(string(raw), `"status":"working"`, `"status":"working","token":"private-secret"`, 1))
			case "duplicate":
				raw = []byte(strings.TrimSuffix(string(raw), "}") + `,"state":"not_found"}`)
			case "trailing":
				raw = append(raw, []byte(` {}`)...)
			case "oversize":
				raw = []byte(strings.Repeat(" ", (1<<20)+1))
			}
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if variant == "mime" {
					w.Header().Set("Content-Type", "text/plain")
				}
				_, _ = w.Write(raw)
			}))
			defer s.Close()
			c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project"}
			result, err := c.LookupWorkBuddyCloud(context.Background(), request)
			if err == nil || result.State != "" || result.Submission != nil || calls.Load() != 1 || strings.Contains(err.Error(), "private-secret") {
				t.Fatal("unsafe response became a lookup fact or exposed content", err)
			}
		})
	}
}

func TestWorkBuddyCloudLookupAcceptsOnly200WithoutCookiesOrRedirects(t *testing.T) {
	request := cloudLookupRequest(Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1})
	var escaped atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer target.Close()
	for _, status := range []int{201, 204, 307, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Cookie") != "" {
					t.Error("unrelated browser cookie was forwarded")
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
				_, _ = w.Write([]byte("private-control-body"))
			}))
			defer s.Close()
			jar, _ := cookiejar.New(nil)
			u, _ := url.Parse(s.URL)
			jar.SetCookies(u, []*http.Cookie{{Name: "private-cookie", Value: "not-for-runner", Path: "/"}})
			httpClient := s.Client()
			httpClient.Jar, httpClient.CheckRedirect = jar, func(*http.Request, []*http.Request) error { return nil }
			c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project", HTTP: httpClient}
			out, err := c.LookupWorkBuddyCloud(context.Background(), request)
			var failure *workBuddyCloudControlError
			if !errors.As(err, &failure) || failure.StatusCode != status || out.State != "" || calls.Load() != 1 || strings.Contains(err.Error(), "private-control-body") {
				t.Fatal("lookup treated a non200 response as facts or retried", err)
			}
			if httpClient.Jar != jar || httpClient.CheckRedirect == nil {
				t.Fatal("lookup mutated the caller's client")
			}
		})
	}
	if escaped.Load() != 0 {
		t.Fatal("lookup escaped the control origin")
	}
}

type cloudLookupRoundTripper func(*http.Request) (*http.Response, error)

func (f cloudLookupRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWorkBuddyCloudLookupTransportIsBoundedNoReplayAndSafe(t *testing.T) {
	request := cloudLookupRequest(Assignment{RunID: "run", NodeID: "node", WorkerID: "worker", Attempt: 1, Fence: 1})
	var calls atomic.Int32
	httpClient := &http.Client{Transport: cloudLookupRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 30*time.Second || r.GetBody != nil || r.Header.Get("Idempotency-Key") != "" || r.Header.Get("X-Idempotency-Key") != "" {
			t.Error("lookup transport has no deadline or permits implicit replay")
		}
		return nil, errors.New("private URL query body and platform-machine-only")
	})}
	c := &Client{BaseURL: "https://suma.example", Token: "platform-machine-only", ProjectID: "project", HTTP: httpClient}
	out, err := c.LookupWorkBuddyCloud(context.Background(), request)
	if err == nil || out.State != "" || calls.Load() != 1 || strings.Contains(err.Error(), "platform-machine-only") || strings.Contains(err.Error(), "private URL") {
		t.Fatal("transport failure was retried, exposed or interpreted as not_found", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = c.LookupWorkBuddyCloud(ctx, request); err == nil || calls.Load() != 1 {
		t.Fatal("cancelled lookup reached network")
	}
	c.BaseURL = "https://suma.example?token=private"
	if _, err = c.LookupWorkBuddyCloud(context.Background(), request); err == nil || calls.Load() != 1 {
		t.Fatal("a query-bearing control URL was accepted")
	}
}

func TestWorkBuddyCloudLookupObservationDriftDoesNotRelaxWrites(t *testing.T) {
	original := poolTestAssignment()
	request := cloudLookupRequest(original)
	current := original
	current.LeaseUntil, current.DispatchState = "2031-01-01T00:00:00Z", "reconciling"
	var calls atomic.Int32
	var badEcho atomic.Bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		reply := cloudLookupResult(request, "task_known")
		if badEcho.Load() {
			reply.Assignment.LeaseUntil = current.LeaseUntil
			reply.Submission.Assignment = reply.Assignment
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer s.Close()
	c := &Client{BaseURL: s.URL, Token: "platform-machine-only", ProjectID: "project", Pool: &PoolContext{PoolID: current.PoolID, RunnerID: current.PoolRunnerID, Assignment: &current}}
	if _, err := c.LookupWorkBuddyCloud(context.Background(), request); err != nil {
		t.Fatal("read-only original lookup could not survive observation changes", err)
	}
	create := wbc.CloudCreateRequest{ProjectID: "project", DelegationID: request.DelegationID, ExpectedVersion: 1, Assignment: request.Assignment, IdempotencyKey: "original-create-155"}
	if _, err := c.CreateWorkBuddyCloud(context.Background(), create); err == nil {
		t.Fatal("lookup relaxation leaked into Create")
	}
	known := cloudLookupResult(request, "task_known").Submission
	refresh := wbc.CloudRefreshRequest{ProjectID: "project", ExpectedVersion: known.Version, Assignment: request.Assignment, IdempotencyKey: "original-refresh-155"}
	if _, err := c.RefreshWorkBuddyCloud(context.Background(), *known, refresh); err == nil {
		t.Fatal("lookup relaxation leaked into Refresh")
	}
	if calls.Load() != 1 || current.LeaseUntil != "2031-01-01T00:00:00Z" || current.DispatchState != "reconciling" {
		t.Fatal("writes reached network or lookup modified the Pool assignment")
	}
	badEcho.Store(true)
	if _, err := c.LookupWorkBuddyCloud(context.Background(), request); err == nil || calls.Load() != 2 {
		t.Fatal("a response echoed current lease instead of the original request")
	}
}
