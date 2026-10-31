package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	pc "github.com/ai-mada/suma/internal/toolpermissioncontract"
)

func codexPermissionFixture174(t *testing.T) (Assignment, tc.Profile, pc.Request, pc.Permission) {
	t.Helper()
	catalog, err := cc.NewCatalog(strings.Repeat("c", 64), []cc.ModelOption{{Model: "owned174", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned174", ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	p := tc.Profile{CommandID: "codex174", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture174"}
	a := Assignment{RunID: "run174", NodeID: "node174", Attempt: 1, Fence: 1, DispatchID: "dispatch174", WorkerID: "worker174", Type: "agent", Command: []string{p.CommandID}, Config: map[string]any{"codex_execution": binding, "tool_profile_sha256": p.ProfileSHA256}}
	r := pc.Request{IdempotencyKey: "original174-request", Protocol: cc.Protocol, Codex: &cc.CodexApprovalRequest{RequestID: cc.CodexRPCRequestID{Kind: "number", Value: "9007199254740993"}, Method: "item/commandExecution/requestApproval", ThreadID: "thread174", TurnID: "turn174", ItemID: "item174", StartedAtMS: 1793404800000, BindingSHA256: binding.BindingSHA256, ParamsSHA256: strings.Repeat("d", 64), ItemSHA256: strings.Repeat("e", 64), Title: "Review exact command", Details: "Run the project test once", DetailsComplete: true, ScopeSupported: true, Command: "go test ./internal/fixture", CWD: "."}}
	d := pc.Decision{Choice: pc.Choice{Protocol: cc.Protocol, Codex: &cc.CodexApprovalChoice{Decision: "accept"}}, ActorID: "human174", DecidedAt: "2026-10-31T00:00:01Z"}
	permission := pc.Permission{ID: "permission174", ProjectID: "project174", RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: p.ProfileSHA256, Version: 2, State: "decided", Request: r, Decision: &d, CreatedAt: "2026-10-31T00:00:00Z", ExpiresAt: "2026-10-31T00:10:00Z", AllowedActions: []string{}}
	return a, p, r, permission
}
func savedCodexPermission174(t *testing.T, a Assignment, p tc.Profile, r pc.Request, permission pc.Permission) (*Journal, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "journal")
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.beginToolPermissionExecution(a, p); err != nil {
		t.Fatal(err)
	}
	if err = j.recordToolPermission(a, r); err != nil {
		t.Fatal(err)
	}
	if err = j.observeToolPermission(a, permission); err != nil {
		t.Fatal(err)
	}
	return j, dir
}

func TestCodexPermissionDurableIntentCannotReplayAfterCrash(t *testing.T) {
	a, p, r, permission := codexPermissionFixture174(t)
	j, dir := savedCodexPermission174(t, a, p, r, permission)
	o := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true}
	if err := j.recordToolPermissionObservation(a, r, permission.ID, o, false); err != nil {
		t.Fatal(err)
	}
	o.Sequence = 3
	o.ReplyStarted = true
	if err := j.recordToolPermissionObservation(a, r, permission.ID, o, true); err != nil {
		t.Fatal(err)
	}
	if j.recordToolPermissionObservation(a, r, permission.ID, o, true) == nil {
		t.Fatal("second native write permit reconstructed")
	}
	if err := j.recordToolPermissionObservation(a, r, permission.ID, o, false); err != nil {
		t.Fatal("exact persisted observation retry rejected", err)
	}
	changed := o
	changed.ReplyWritten = true
	if j.recordToolPermissionObservation(a, r, permission.ID, changed, false) == nil {
		t.Fatal("same sequence accepted changed write fact")
	}
	changed.Sequence = 4
	changed.ReplyStarted = false
	if j.recordToolPermissionObservation(a, r, permission.ID, changed, false) == nil {
		t.Fatal("write intent regressed")
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Pending()) != 1 || !reopened.Pending()[0].OutcomeUnknown {
		t.Fatal("crash claimed successful native write")
	}
	if len(reopened.pendingToolPermissionObservations()) != 2 {
		t.Fatal("durable unacknowledged observations lost")
	}
	if reopened.beginToolPermissionExecution(a, p) == nil {
		t.Fatal("crash recreated process launch")
	}
}

func TestCodexPermissionObservationHTTPKeepsExactReceiptAndOrdersResult(t *testing.T) {
	a, p, r, permission := codexPermissionFixture174(t)
	j, _ := savedCodexPermission174(t, a, p, r, permission)
	o := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true, ReplyStarted: true}
	if err := j.recordToolPermissionObservation(a, r, permission.ID, o, true); err != nil {
		t.Fatal(err)
	}
	if err := j.Finish(Result{Assignment: a, ExitCode: -1, OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	var originalBody string
	observationCalls := 0
	wrongReceipt := true
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(req.URL.Path, "/results") {
			w.WriteHeader(200)
			return
		}
		var body struct {
			ProjectID    string                      `json:"project_id"`
			Assignment   Assignment                  `json:"assignment"`
			PermissionID string                      `json:"permission_id"`
			Operation    string                      `json:"idempotency_key"`
			Observation  cc.CodexApprovalObservation `json:"observation"`
		}
		if json.NewDecoder(req.Body).Decode(&body) != nil {
			t.Error("bad HTTP envelope")
			w.WriteHeader(400)
			return
		}
		raw, _ := json.Marshal(body)
		if originalBody == "" {
			originalBody = string(raw)
		} else if originalBody != string(raw) {
			t.Error("HTTP retry changed original operation/body")
		}
		observationCalls++
		response := permission
		response.Observation = &body.Observation
		if wrongReceipt {
			copy := body.Observation
			copy.Sequence++
			response.Observation = &copy
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer s.Close()
	c := &Client{BaseURL: s.URL, ProjectID: "project174", Token: "fake-private174", HTTP: s.Client()}
	if Replay(context.Background(), c, j) == nil {
		t.Fatal("accepted current snapshot as original observation receipt")
	}
	if len(calls) != 1 || len(j.pendingToolPermissionObservations()) != 1 {
		t.Fatal("result bypassed unconfirmed native observation")
	}
	wrongReceipt = false
	if err := Replay(context.Background(), c, j); err != nil {
		t.Fatal(err)
	}
	if observationCalls != 2 || len(calls) != 3 || !strings.HasSuffix(calls[2], "/results") || len(j.pendingToolPermissionObservations()) != 0 || len(j.Pending()) != 0 {
		t.Fatal("original observation did not precede result", calls)
	}
}

func TestCodexPermissionBeforeWriteRechecksCurrentHumanSource(t *testing.T) {
	for _, state := range []string{"decided", "expired", "superseded"} {
		t.Run(state, func(t *testing.T) {
			a, p, r, permission := codexPermissionFixture174(t)
			j, _ := savedCodexPermission174(t, a, p, r, permission)
			h := codexPermissionHandler(a, ExecConfig{Journal: j, ToolPermissionPoll: func(context.Context, string) (pc.Permission, error) {
				copy := permission
				copy.State = state
				return copy, nil
			}})
			o := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true, ReplyStarted: true}
			// The production request key is deterministic and includes the complete
			// typed RPC identity; seed exactly that same request via Decide transport.
			var actual pc.Request
			cfg := ExecConfig{Journal: j, ToolPermission: func(_ context.Context, request pc.Request) (pc.Decision, error) {
				actual = request
				copy := permission
				copy.Request = request
				if err := j.observeToolPermission(a, copy); err != nil {
					return pc.Decision{}, err
				}
				return *permission.Decision, nil
			}, ToolPermissionPoll: func(context.Context, string) (pc.Permission, error) {
				copy := permission
				copy.Request = actual
				copy.State = state
				return copy, nil
			}}
			h = codexPermissionHandler(a, cfg)
			decision, err := h.Decide(context.Background(), *r.Codex)
			if err != nil {
				t.Fatal(err)
			}
			err = h.BeforeWrite(context.Background(), *r.Codex, decision, o)
			if (err == nil) != (state == "decided") {
				t.Fatal("historical human decision became fresh write permission", state, err)
			}
			if state == "decided" && h.BeforeWrite(context.Background(), *r.Codex, decision, o) == nil {
				t.Fatal("second response writer acquired same intent")
			}
		})
	}
}

func TestCodexPermissionCleanupPersistsWithoutHumanDecision(t *testing.T) {
	a, p, r, permission := codexPermissionFixture174(t)
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	_ = j.Accept(a)
	if err = j.beginToolPermissionExecution(a, p); err != nil {
		t.Fatal(err)
	}
	h := codexPermissionHandler(a, ExecConfig{Journal: j})
	o := cc.CodexApprovalObservation{Sequence: 2, ParamsSHA256: r.Codex.ParamsSHA256, RequestResolved: true, ItemStatus: "declined"}
	if err = h.Observe(context.Background(), *r.Codex, codexApprovalDecision{}, o); err != nil {
		t.Fatal(err)
	}
	if err = h.Observe(context.Background(), *r.Codex, codexApprovalDecision{}, o); err != nil {
		t.Fatal(err)
	}
	pending := j.pendingToolPermissionObservations()
	if len(pending) != 1 || pending[0].PermissionID != "" || pending[0].Observation.DecisionObserved || pending[0].Observation.ReplyWritten {
		t.Fatal("cleanup was lost or invented a human/native ACK", pending)
	}
	opens, observations := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		response := permission
		response.Request = pending[0].Request
		response.Decision = nil
		response.State = "pending"
		response.Version = 1
		switch req.URL.Path {
		case "/api/v1/runner/tool-permissions":
			opens++
		case "/api/v1/runner/tool-permissions/observations":
			observations++
			response.Observation = &o
			response.State = "superseded"
			response.Version = 2
		default:
			t.Error("unexpected cleanup side effect", req.URL.Path)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, ProjectID: "project174", Token: "fake-private174", HTTP: server.Client()}
	if err = ReplayToolPermissionObservations(context.Background(), client, j); err != nil {
		t.Fatal(err)
	}
	if opens != 1 || observations != 1 || len(j.pendingToolPermissionObservations()) != 0 {
		t.Fatal("original registration/observation recovery changed", opens, observations)
	}
	// A never-submitted Open cannot be invented after its machine/source has
	// expired. Preserve the local fact and surface reconciliation once.
	rejectedJournal, err := OpenJournal(filepath.Join(t.TempDir(), "rejected"))
	if err != nil {
		t.Fatal(err)
	}
	defer rejectedJournal.Close()
	_ = rejectedJournal.Accept(a)
	if err = rejectedJournal.beginToolPermissionExecution(a, p); err != nil {
		t.Fatal(err)
	}
	rejectedHandler := codexPermissionHandler(a, ExecConfig{Journal: rejectedJournal})
	if err = rejectedHandler.Observe(context.Background(), *r.Codex, codexApprovalDecision{}, o); err != nil {
		t.Fatal(err)
	}
	rejectedCalls := 0
	rejectedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { rejectedCalls++; w.WriteHeader(http.StatusForbidden) }))
	defer rejectedServer.Close()
	rejectedClient := &Client{BaseURL: rejectedServer.URL, ProjectID: "project174", Token: "fake-private174", HTTP: rejectedServer.Client()}
	for i := 0; i < 2; i++ {
		if !errors.Is(ReplayToolPermissionObservations(context.Background(), rejectedClient, rejectedJournal), errToolPermissionReconciliation) {
			t.Fatal("source rejection was hidden")
		}
	}
	retained := rejectedJournal.pendingToolPermissionObservations()
	if rejectedCalls != 1 || len(retained) != 1 || !retained[0].ReconciliationRequired || retained[0].Acked || retained[0].PermissionID != "" {
		t.Fatal("expired source was retried, fabricated or silently acknowledged", rejectedCalls, retained)
	}
}

func TestToolPermissionTransportKeepsOrdinaryAndPoolNativeIdentity(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "pool"}[pool], func(t *testing.T) {
			a, _, request, permission := codexPermissionFixture174(t)
			var scope *PoolContext
			namespace := "runner"
			if pool {
				a.TenantID = "tenant174"
				a.ProjectID = "project174"
				a.PoolID = "pool174"
				a.PoolRunnerID = "machine174"
				a.PoolSlotID = "0"
				a.PoolGrantID = "grant174"
				a.PoolGrantVersion = 1
				a.PoolCredentialID = "credential174"
				a.PoolCredentialVersion = 1
				a.DispatchState = "active"
				a.LeaseUntil = "2026-10-31T00:10:00Z"
				scope = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &a}
				namespace = "pool-runner"
			}
			paths := []string{}
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				var body map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid typed transport")
					w.WriteHeader(400)
					return
				}
				if !strings.HasPrefix(r.URL.Path, "/api/v1/"+namespace+"/") {
					t.Error("changed machine namespace")
				}
				var source Assignment
				_ = json.Unmarshal(body["assignment"], &source)
				if source.RunID != a.RunID || source.Fence != a.Fence || source.DispatchID != a.DispatchID {
					t.Error("machine identity changed")
				}
				if pool {
					if len(body["runner_id"]) == 0 || len(body["tenant_id"]) == 0 || source.Config != nil || source.Command != nil {
						t.Error("pool native call copied command input or lost frozen identity")
					}
				}
				response := permission
				if strings.HasSuffix(r.URL.Path, "/observations") {
					var o cc.CodexApprovalObservation
					_ = json.Unmarshal(body["observation"], &o)
					response.Observation = &o
					var operation string
					_ = json.Unmarshal(body["idempotency_key"], &operation)
					if operation != "receipt174-original" {
						t.Error("observation operation changed")
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer s.Close()
			c := &Client{BaseURL: s.URL, Token: "fake-private174", ProjectID: "project174", Pool: scope, HTTP: s.Client()}
			if _, err := c.OpenToolPermission(context.Background(), a, request); err != nil {
				t.Fatal(err)
			}
			if _, err := c.PollToolPermission(context.Background(), a, permission.ID); err != nil {
				t.Fatal(err)
			}
			o := cc.CodexApprovalObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, Decision: "accept", DecisionObserved: true}
			if _, err := c.ObserveToolPermission(context.Background(), a, request, permission.ID, "receipt174-original", o); err != nil {
				t.Fatal(err)
			}
			if len(paths) != 3 {
				t.Fatal("unexpected extra native transport", paths)
			}
		})
	}
}
