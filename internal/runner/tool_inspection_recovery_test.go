package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func fixtureToolInventoryAck195(w http.ResponseWriter, r *http.Request, observed string) {
	var report InventoryReport
	_ = json.NewDecoder(r.Body).Decode(&report)
	_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": observed, "maintenance": report.Maintenance})
}

func recoveryFixture195(p toolInspectionCheckpoint) ti.ToolInspectionRecovery {
	now := time.Now().UTC()
	return ti.ToolInspectionRecovery{ID: "recovery195", ProjectID: p.Inspection.ProjectID, WorkerID: p.WorkerID, Inspection: inspectionReference(*p.Inspection), InspectionVersion: p.Inspection.Version, SourceSHA256: ti.InspectionSourceSHA256(*p.Inspection), Version: 1, State: "requested", RequestedBy: "current-manager195", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339Nano)}
}

func TestToolRecoveryOriginalReceiptAndAckResumeWithoutNewProbe(t *testing.T) {
	ctx := context.Background()
	var current ti.ToolInspectionRecovery
	var proofs, acks, claims, reports int
	var ackKeys []string
	var expireProof bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/api/v1/runner/tool-inspections/recovery":
			_ = json.NewEncoder(w).Encode(ti.ToolInspectionRecoveryPoll{Recovery: &current})
		case "/api/v1/runner/tool-inspections/recovery/proofs":
			proofs++
			if expireProof {
				current.State = "expired"
				w.WriteHeader(409)
				return
			}
			var proof ti.ToolInspectionStopProof
			_ = json.Unmarshal(body["proof"], &proof)
			current.Proof = &proof
			current.ProofSHA256 = ti.StopProofDigest(proof)
			current.State = "proof_stored"
			current.Version++
			current.ProofStoredAt = time.Now().UTC().Format(time.RFC3339Nano)
			_ = json.NewEncoder(w).Encode(current)
		case "/api/v1/runner/tool-inspections/recovery/acknowledgements":
			acks++
			var key string
			_ = json.Unmarshal(body["idempotency_key"], &key)
			ackKeys = append(ackKeys, key)
			if current.State != "closed_unknown" {
				current.State = "closed_unknown"
				current.Version++
				current.ClosedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
			if acks == 1 {
				w.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(w).Encode(current)
		case "/api/v1/runner/tool-inspections/claim":
			claims++
			if acks < 2 {
				w.WriteHeader(403)
				return
			}
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case "/api/v1/runner/tool-inventory":
			reports++
			if body["inspection"] != nil {
				t.Error("recovery reused original inspection for a new probe")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano)})
		default:
			t.Errorf("unexpected recovery side effect %s", r.URL.Path)
			w.WriteHeader(403)
		}
	}))
	defer s.Close()
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{BaseURL: s.URL, ProjectID: "project195", Token: "current-worker195"}
	cp, err := j.beginToolInspection(c, "worker195")
	if err != nil {
		t.Fatal(err)
	}
	i := inspectionFixture194(c.ProjectID, "worker195", cp.ClaimID)
	i.State = "unknown"
	i.ResultCode = "probe_interrupted"
	cp.Responded = true
	cp.Inspection = &i
	cp.Started = true
	cp.FailureCode = "probe_interrupted"
	cp.FailureOperation = operationID()
	cp.FailureAcked = true
	if err = j.saveToolInspection(*cp); err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollector(j, *cp)
	if err != nil {
		t.Fatal(err)
	}
	if err = collector.closeReturned(); err != nil {
		t.Fatal(err)
	} // exact empty native set; process behavior is tested separately
	current = recoveryFixture195(*cp)
	w := &Worker{Client: c, Journal: j, ID: cp.WorkerID, Exec: ExecConfig{Commands: map[string]CommandSpec{"invalid id": {}}}}
	if _, err = w.serviceToolInventory(ctx, true); err == nil {
		t.Fatal("lost closure ACK was discarded")
	}
	stored, err := j.toolInspectionCheckpoint()
	if err != nil || stored.Recovery == nil || stored.Recovery.Accepted == nil || stored.Recovery.AckOperation == "" || stored.Complete {
		t.Fatal("accepted proof receipt/ACK intent was not durable", err)
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	w.Journal = j
	if _, err = w.serviceToolInventory(ctx, true); err != nil {
		t.Fatal(err)
	}
	if proofs != 1 || acks != 2 || ackKeys[0] != ackKeys[1] || claims != 0 || reports != 0 || w.canClaimInventory() {
		t.Fatal("recovery replayed a probe or changed original closure operation", proofs, acks, claims, reports)
	}
	closed, err := j.toolInspectionCheckpoint()
	if err != nil || !closed.Complete || closed.Recovery.Closed == nil {
		t.Fatal("closed unknown did not retire original nonce", err)
	}
	receiptDir, _, err := j.toolProbeDirectory(*closed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(receiptDir, "closed-checkpoint.json")); err != nil {
		t.Fatal("original checkpoint was not archived", err)
	}
	w.Exec = ExecConfig{}
	if err = w.tickToolInventory(ctx); err != nil {
		t.Fatal(err)
	}
	if reports != 1 || !w.canClaimInventory() {
		t.Fatal("independent current periodic snapshot did not restore readiness")
	}

	// A hard crash with no original returned receipt has no stop proof. The
	// approved recovery stays blocked; it cannot trigger probe, kill or ACK.
	j2, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j2.Close()
	p2, err := j2.beginToolInspection(c, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	i2 := inspectionFixture194(c.ProjectID, w.ID, p2.ClaimID)
	i2.State = "unknown"
	i2.ResultCode = "probe_interrupted"
	p2.Responded = true
	p2.Inspection = &i2
	p2.Started = true
	p2.FailureCode = "probe_interrupted"
	p2.FailureOperation = operationID()
	p2.FailureAcked = true
	if err = j2.saveToolInspection(*p2); err != nil {
		t.Fatal(err)
	}
	current = recoveryFixture195(*p2)
	w2 := &Worker{Client: c, Journal: j2, ID: w.ID, Exec: ExecConfig{Commands: map[string]CommandSpec{"invalid id": {}}}}
	if _, err = w2.serviceToolInventory(ctx, true); err != nil {
		t.Fatal(err)
	}
	if proofs != 1 || acks != 2 || reports != 1 || w2.canClaimInventory() {
		t.Fatal("missing stop receipt manufactured a recovery")
	}

	// A rejected expired approval is read back under its original identity,
	// archived with its unsent/failed intents, and awaits a new explicit approval.
	j3, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j3.Close()
	p3, err := j3.beginToolInspection(c, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	i3 := inspectionFixture194(c.ProjectID, w.ID, p3.ClaimID)
	i3.State = "unknown"
	i3.ResultCode = "probe_interrupted"
	p3.Responded = true
	p3.Inspection = &i3
	p3.Started = true
	p3.FailureCode = "probe_interrupted"
	p3.FailureOperation = operationID()
	p3.FailureAcked = true
	if err = j3.saveToolInspection(*p3); err != nil {
		t.Fatal(err)
	}
	col3, err := newToolProbeCollector(j3, *p3)
	if err != nil {
		t.Fatal(err)
	}
	if err = col3.closeReturned(); err != nil {
		t.Fatal(err)
	}
	current = recoveryFixture195(*p3)
	expireProof = true
	w3 := &Worker{Client: c, Journal: j3, ID: w.ID, Exec: ExecConfig{Commands: map[string]CommandSpec{"invalid id": {}}}}
	if _, err = w3.serviceToolInventory(ctx, true); err != nil {
		t.Fatal(err)
	}
	afterExpiry, err := j3.toolInspectionCheckpoint()
	if err != nil || afterExpiry.Recovery != nil || afterExpiry.Complete || proofs != 2 || acks != 2 {
		t.Fatal("expired approval was blindly retried or discarded original unknown", err)
	}
	if _, err = w3.serviceToolInventory(ctx, true); err != nil {
		t.Fatal(err)
	}
	if proofs != 2 || acks != 2 {
		t.Fatal("expired approval produced a new effect")
	}
	current = recoveryFixture195(*p3)
	current.ID = "recovery195-new-approval"
	expireProof = false
	if _, err = w3.serviceToolInventory(ctx, true); err != nil {
		t.Fatal(err)
	}
	if proofs != 3 || acks != 3 || reports != 1 {
		t.Fatal("new explicit approval did not recover original stop receipt")
	}
}

func TestMaintenanceCapabilityOnlyContinuousInventoryOwner(t *testing.T) {
	var capabilities []*ti.ToolMaintenanceCapability
	var cancel context.CancelFunc
	var omitEcho bool
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/tool-inspections/claim":
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case "/api/v1/runner/tool-inventory":
			var report InventoryReport
			_ = json.NewDecoder(r.Body).Decode(&report)
			capabilities = append(capabilities, report.Maintenance)
			echo := report.Maintenance
			if omitEcho {
				echo = nil
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano), "maintenance": echo})
		case "/api/v1/runner/claim":
			cancel()
			_, _ = w.Write([]byte(`{"assignment":null}`))
		default:
			t.Errorf("unexpected owner request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer s.Close()
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{BaseURL: s.URL, ProjectID: "project195", Token: "worker195"}
	w := &Worker{Client: c, Journal: j, ID: "worker195"}
	if _, _, err = w.ReportTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	cancel = stop
	_ = w.Run(ctx)
	stop()
	if _, _, err = w.ReportTools(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 3 || capabilities[0] != nil || capabilities[1] == nil || capabilities[2] != nil || capabilities[1].Protocol != ti.MaintenanceProtocol {
		t.Fatal("one-off report advertised a continuous consumer", capabilities)
	}
	owner := *capabilities[1]
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	w2 := &Worker{Client: c, Journal: j, ID: w.ID}
	if err = w2.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	if *w2.maintenance != owner {
		t.Fatal("logical Journal maintenance owner changed across restart")
	}
	c2 := *c
	c2.ProjectID = "different-project"
	w2.Client = &c2
	if err = w2.enableToolMaintenance(); err == nil {
		t.Fatal("maintenance owner rebound to another project")
	}
	omitEcho = true
	pending, err := j.newToolInventory(c.BaseURL, c.ProjectID, w.ID, nil, nil, &owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ToolInventory(context.Background(), pending); err == nil {
		t.Fatal("missing maintenance ACK was treated as supported")
	}
	if original := j.PendingInventory(); original == nil || original.OperationID != pending.OperationID {
		t.Fatal("unacknowledged maintenance operation was replaced")
	}
}
