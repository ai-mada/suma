package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func inspectionFixture194(project, worker, claim string) ti.Inspection {
	now := time.Now().UTC()
	return ti.Inspection{ID: "inspection194", ProjectID: project, WorkerID: worker, ToolProfileID: "profile194", ToolProfileVersion: 1, ProfileSHA256: strings.Repeat("a", 64), CommandID: "inspect", Version: 2, Fence: 1, State: "claimed", RequestedBy: "owner194", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339Nano), ClaimID: claim, ClaimedAt: now.Format(time.RFC3339Nano), LeaseUntil: now.Add(time.Minute).Format(time.RFC3339Nano)}
}

// These are protocol fixtures; the integrated HTTP/SQLite/native read-only
// inspection journey belongs to the single platform acceptance check.
func TestToolInspectionRecoversOriginalClaimAndReportWithoutReprobe(t *testing.T) {
	ctx := context.Background()
	var claims []string
	var reports []InventoryReport
	var original ti.Inspection
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/tool-inspections/claim":
			var body ti.ClaimRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			claims = append(claims, body.ClaimID)
			if len(claims) == 1 {
				w.WriteHeader(503)
				return
			}
			if len(claims) > 2 {
				_, _ = w.Write([]byte(`{"inspection":null}`))
				return
			}
			original = inspectionFixture194("project194", "worker194", body.ClaimID)
			_ = json.NewEncoder(w).Encode(ti.ClaimReply{Inspection: &original})
		case "/api/v1/runner/tool-inventory":
			var body InventoryReport
			_ = json.NewDecoder(r.Body).Decode(&body)
			reports = append(reports, body)
			if body.Inspection != nil && len(reports) == 2 {
				w.WriteHeader(503)
				return
			}
			observed := time.Now().UTC().Format(time.RFC3339Nano)
			if body.Inspection != nil {
				observed = "2026-10-24T03:00:00Z"
			} // An original committed ACK does not renew its TTL.
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": observed, "inspection": body.Inspection, "inspection_state": func() string {
				if body.Inspection != nil {
					return "completed"
				}
				return ""
			}()})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{BaseURL: s.URL, ProjectID: "project194", Token: "fixture194"}
	old, err := j.NewInventory(s.URL, c.ProjectID, "worker194", nil)
	if err != nil {
		t.Fatal(err)
	}
	w := &Worker{Client: c, Journal: j, ID: "worker194", Exec: ExecConfig{}}
	if _, err = w.serviceToolInventory(ctx, false); err == nil {
		t.Fatal("lost claim ACK not retained")
	}
	if len(reports) != 1 || reports[0].OperationID != old.OperationID || reports[0].Inspection != nil {
		t.Fatal("old report was not recovered before requested inspection")
	}
	if _, err = w.serviceToolInventory(ctx, false); err == nil {
		t.Fatal("lost report ACK not retained")
	}
	if len(claims) != 2 || claims[0] != claims[1] || len(reports) != 2 || reports[1].Inspection == nil {
		t.Fatal("original claim/report identity lost")
	}
	ref := ti.Reference{InspectionID: original.ID, ClaimID: original.ClaimID, Fence: original.Fence}
	if *reports[1].Inspection != ref {
		t.Fatal("wrong original reference")
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	w.Journal = j
	// Re-probing this invalid configuration would fail. Only the saved report
	// may be recovered after a restart at the native-effect boundary.
	w.Exec = ExecConfig{Commands: map[string]CommandSpec{"invalid id": {}}}
	if _, err = w.serviceToolInventory(ctx, false); err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 || len(reports) != 3 || reports[1].OperationID != reports[2].OperationID || *reports[2].Inspection != ref {
		t.Fatal("restarted inspection was re-claimed or re-probed")
	}
	cp, err := j.toolInspectionCheckpoint()
	if err != nil || cp == nil || !cp.Complete {
		t.Fatal("original inspection completion was not durable", err)
	}
	if w.canClaimInventory() || !w.inventoryNext.IsZero() {
		t.Fatal("recovered old ACK incorrectly restored current inventory readiness")
	}
	w.Exec = ExecConfig{}
	if err = w.tickToolInventory(ctx); err != nil {
		t.Fatal(err)
	}
	if !w.canClaimInventory() || len(reports) != 4 || reports[3].Inspection != nil || reports[3].OperationID == reports[2].OperationID {
		t.Fatal("current periodic snapshot did not independently restore readiness")
	}

	// A second crash point is after AckInventory was saved but before the
	// inspection checkpoint completed. That saved ACK has the same stale TTL.
	j2, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()
	cp2, err := j2.beginToolInspection(c, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	source2 := inspectionFixture194(c.ProjectID, w.ID, cp2.ClaimID)
	cp2.Responded = true
	cp2.Inspection = &source2
	cp2.Started = true
	if err = j2.saveToolInspection(*cp2); err != nil {
		t.Fatal(err)
	}
	ref2 := inspectionReference(source2)
	saved, err := j2.newInspectionInventory(c.BaseURL, c.ProjectID, w.ID, nil, &ref2)
	if err != nil {
		t.Fatal(err)
	}
	if err = j2.AckInventory(saved.OperationID, "2026-10-24T03:00:00Z"); err != nil {
		t.Fatal(err)
	}
	w2 := &Worker{Client: c, Journal: j2, ID: w.ID, Exec: ExecConfig{Commands: map[string]CommandSpec{"invalid id": {}}}, inventoryReady: true, inventoryNext: time.Now().Add(time.Hour)}
	if fresh, e := w2.serviceToolInventory(ctx, false); e != nil || fresh {
		t.Fatal("saved old ACK was treated as a new snapshot", e)
	}
	if w2.canClaimInventory() || !w2.inventoryNext.IsZero() || len(reports) != 4 || len(claims) != 3 {
		t.Fatal("saved ACK recovery re-probed or opened readiness")
	}
	cp2, err = j2.toolInspectionCheckpoint()
	if err != nil || !cp2.Complete {
		t.Fatal("saved ACK did not complete original inspection", err)
	}
	w2.Exec = ExecConfig{}
	if err = w2.tickToolInventory(ctx); err != nil {
		t.Fatal(err)
	}
	if !w2.canClaimInventory() || len(reports) != 5 || reports[4].Inspection != nil || reports[4].OperationID == saved.OperationID {
		t.Fatal("saved ACK recovery did not request an independent current periodic snapshot")
	}
}

func TestToolInspectionStartedCheckpointBecomesUnknownAndRejectsFalseAck(t *testing.T) {
	var failures []ti.FailureRequest
	var source ti.Inspection
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/tool-inspections/recovery":
			_, _ = w.Write([]byte(`{"recovery":null}`))
		case "/api/v1/runner/tool-inspections/results":
			var body ti.FailureRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			failures = append(failures, body)
			if len(failures) == 1 {
				w.WriteHeader(503)
				return
			}
			result := source
			result.State = "unknown"
			result.ResultCode = body.FailureCode
			result.Version++
			_ = json.NewEncoder(w).Encode(result)
		case "/api/v1/runner/tool-inventory":
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano), "inspection": ti.Reference{InspectionID: "other", ClaimID: "wrong194", Fence: 2}, "inspection_state": "completed"})
		default:
			t.Errorf("unknown execution must not re-claim or probe: %s", r.URL.Path)
			w.WriteHeader(403)
		}
	}))
	defer s.Close()
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	c := &Client{BaseURL: s.URL, ProjectID: "project194", Token: "fixture194"}
	cp, err := j.beginToolInspection(c, "worker194")
	if err != nil {
		t.Fatal(err)
	}
	source = inspectionFixture194(c.ProjectID, "worker194", cp.ClaimID)
	cp.Responded = true
	cp.Inspection = &source
	cp.Started = true
	if err = j.saveToolInspection(*cp); err != nil {
		t.Fatal(err)
	}
	w := &Worker{Client: c, Journal: j, ID: "worker194", Exec: ExecConfig{Commands: map[string]CommandSpec{"invalid id": {}}}}
	if _, err = w.serviceToolInventory(context.Background(), true); err == nil {
		t.Fatal("unknown failure ACK lost")
	}
	if _, err = w.serviceToolInventory(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(failures) != 2 || failures[0].IdempotencyKey != failures[1].IdempotencyKey || failures[1].Outcome != "unknown" || failures[1].FailureCode != "probe_interrupted" {
		t.Fatal("unknown effect automatically retried or original failure replaced")
	}
	if _, _, err = w.ReportTools(context.Background()); err == nil {
		t.Fatal("single-step report bypassed unresolved original inspection")
	}
	ref := ti.Reference{InspectionID: source.ID, ClaimID: source.ClaimID, Fence: source.Fence}
	r, err := j.newInspectionInventory(c.BaseURL, c.ProjectID, w.ID, nil, &ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ToolInventory(context.Background(), r); err == nil {
		t.Fatal("unrelated reference accepted as persisted inspection")
	}
	if j.PendingInventory() == nil {
		t.Fatal("unknown ACK erased original report")
	}
}

func TestToolInspectionBusyHelper194(t *testing.T) {
	started := os.Getenv("SUMA_INSPECTION194_STARTED")
	if started == "" {
		return
	}
	if os.WriteFile(started, []byte("local fixture running"), 0600) != nil {
		os.Exit(71)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(os.Getenv("SUMA_INSPECTION194_RELEASE")); err == nil {
			os.Exit(0)
		}
		time.Sleep(5 * time.Millisecond)
	}
	os.Exit(72)
}

func TestPoolToolInspectionContinuesWhileEveryExecutionSlotIsOccupied(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusConflict} {
		t.Run(strconv.Itoa(status), func(t *testing.T) { testPoolInspectionScopeFailure194(t, status) })
	}
}

func testPoolInspectionScopeFailure194(t *testing.T, rejectStatus int) {
	root := t.TempDir()
	started := filepath.Join(root, "started")
	release := filepath.Join(root, "release")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var dispatched, requested, completed atomic.Bool
	var results atomic.Int32
	var scopeRejected atomic.Int32
	var rejectedClaim string
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/pool-runner/session" {
			_ = json.NewEncoder(w).Encode(map[string]any{"runner": map[string]any{"id": "runner", "pool_id": "pool", "worker_id": "pool-runner", "max_slots": 1, "credential_version": 1}, "pool_active": true, "scopes": []any{map[string]string{"tenant_id": "tenant", "project_id": "project"}, map[string]string{"tenant_id": "other-tenant", "project_id": "other-project"}}})
			return
		}
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/api/v1/pool-runner/tool-inspections/claim" || r.URL.Path == "/api/v1/pool-runner/tool-inventory" {
			project, tenant := string(body["project_id"]), string(body["tenant_id"])
			if (project != `"project"` || tenant != `"tenant"`) && (project != `"other-project"` || tenant != `"other-tenant"`) || string(body["runner_id"]) != `"runner"` || body["assignment"] != nil || body["worker_id"] != nil {
				t.Error("management crossed exact granted project/machine scope")
			}
			if project == `"other-project"` && r.URL.Path == "/api/v1/pool-runner/tool-inspections/claim" {
				if _, e := os.Stat(started); e == nil {
					_ = json.Unmarshal(body["claim_id"], &rejectedClaim)
					scopeRejected.Add(1)
					w.WriteHeader(rejectStatus)
					return
				}
				_, _ = w.Write([]byte(`{"inspection":null}`))
				return
			}
		}
		switch r.URL.Path {
		case "/api/v1/pool-runner/tool-inspections/claim":
			if _, e := os.Stat(started); e == nil && scopeRejected.Load() > 0 && requested.CompareAndSwap(false, true) {
				var claim string
				_ = json.Unmarshal(body["claim_id"], &claim)
				i := inspectionFixture194("project", "pool-runner", claim)
				_ = json.NewEncoder(w).Encode(ti.ClaimReply{Inspection: &i})
			} else {
				_, _ = w.Write([]byte(`{"inspection":null}`))
			}
		case "/api/v1/pool-runner/tool-inventory":
			var ref *ti.Reference
			_ = json.Unmarshal(body["inspection"], &ref)
			state := ""
			if ref != nil {
				if !requested.Load() || results.Load() != 0 {
					t.Error("inspection starved until slot completion")
				}
				state = "completed"
				completed.Store(true)
				if e := os.WriteFile(release, []byte("correlated report persisted"), 0600); e != nil {
					t.Error(e)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano), "inspection": ref, "inspection_state": state, "maintenance": body["maintenance"]})
		case "/api/v1/pool-runner/claim":
			var request PoolClaimRequest
			raw, _ := json.Marshal(body)
			_ = json.Unmarshal(raw, &request)
			available := false
			for _, scope := range request.Scopes {
				if scope.TenantID == "tenant" && scope.ProjectID == "project" {
					available = true
				}
			}
			if !available {
				_, _ = w.Write([]byte(`{"assignment":null}`))
				return
			}
			if dispatched.CompareAndSwap(false, true) {
				a := poolTestAssignment()
				a.Type = "command"
				a.Command = []string{"inspect"}
				_ = json.NewEncoder(w).Encode(map[string]any{"assignment": a})
			} else {
				_, _ = w.Write([]byte(`{"assignment":null}`))
			}
		case "/api/v1/pool-runner/heartbeat":
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
		case "/api/v1/pool-runner/progress":
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		case "/api/v1/pool-runner/results":
			var result Result
			_ = json.Unmarshal(body["result"], &result)
			if !completed.Load() || result.ExitCode != 0 {
				t.Error("occupied slot did not finish after independent inventory owner", result.ExitCode)
			}
			results.Add(1)
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
			cancel()
		default:
			t.Errorf("unexpected pool management path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant", ProjectID: "project", Workspace: root, Commands: map[string]CommandSpec{"inspect": {Executable: exe, Args: []string{"-test.run=^TestToolInspectionBusyHelper194$"}, ReadOnly: true, Env: map[string]string{"SUMA_INSPECTION194_STARTED": started, "SUMA_INSPECTION194_RELEASE": release}}}}}}
	other := filepath.Join(root, "isolated-scope")
	if err = os.Mkdir(other, 0700); err != nil {
		t.Fatal(err)
	}
	config.Scopes = append(config.Scopes, PoolLocalScope{TenantID: "other-tenant", ProjectID: "other-project", Workspace: other})
	w := PoolWorker{Client: &Client{BaseURL: s.URL, Token: "fixture-pool194", Pool: &PoolContext{PoolID: "pool", RunnerID: "runner"}}, Config: config, StateDir: filepath.Join(root, "state"), AllowExec: true, PollInterval: 5 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond}
	_ = w.Run(ctx)
	if !requested.Load() || !completed.Load() || results.Load() != 1 {
		t.Fatalf("full slot starved maintenance: requested=%v completed=%v results=%d", requested.Load(), completed.Load(), results.Load())
	}
	if scopeRejected.Load() != 1 {
		t.Fatal("project-local rejection hot-looped", scopeRejected.Load())
	}
	failedScope := w.local[scopeKey("other-tenant", "other-project")]
	cp, err := failedScope.Journal.toolInspectionCheckpoint()
	if err != nil || cp == nil || cp.ClaimID != rejectedClaim || cp.Responded || cp.Complete || failedScope.canClaimInventory() {
		t.Fatal("project-local authority failure discarded its original checkpoint or reopened its gate", err)
	}
}
