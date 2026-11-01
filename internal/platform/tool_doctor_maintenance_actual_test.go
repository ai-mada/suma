package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// This journey uses the real HTTP handler and SQLite. Its machine is an owned
// HTTP contract fixture: it launches no probe/vendor/model and its zero-process
// proof is not native containment evidence. T195 covers native containment.
func TestToolDoctorMaintenanceActualHTTPOriginalOperationJourney(t *testing.T) {
	if os.Getenv("SUMA_TOOL_DOCTOR_199_ACTUAL") != "1" {
		t.Skip("explicit owned HTTP/SQLite journey only")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	evidenceName := "actual"
	if suffix := os.Getenv("SUMA_TOOL_DOCTOR_199_EVIDENCE_SUFFIX"); suffix != "" {
		if suffix != "final" {
			t.Fatal("only a separately preserved final evidence directory is allowed")
		}
		evidenceName += "-" + suffix
	}
	dir := filepath.Join(root, ".cache", "tool-doctor-199", evidenceName)
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("retain the original actual directory; never reseed evidence")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	t.Cleanup(func() {
		file := filepath.Join(dir, "private.sqlite")
		if _, e := os.Stat(file); os.IsNotExist(e) {
			if _, e = f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", file); e != nil {
				t.Errorf("preserve the owned SQLite evidence: %v", e)
			}
		}
	})
	server := httptest.NewServer(f.handler)
	defer server.Close()
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	type observation struct {
		Method, Path string
		Status       int
	}
	var requests []observation
	call := func(method, path string, body any, token string, want int, out any) []byte {
		t.Helper()
		var raw []byte
		if body != nil {
			raw, err = json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
		}
		req, e := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		if token == "" {
			req.AddCookie(f.cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if e != nil {
			t.Fatal(e)
		}
		requests = append(requests, observation{method, path, response.StatusCode})
		partial, _ := json.MarshalIndent(map[string]any{"status": "in_progress_or_interrupted", "requests": requests, "native_processes": 0, "model_calls": 0}, "", "  ")
		if e = os.WriteFile(filepath.Join(dir, "partial.json"), partial, 0600); e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, response.StatusCode, want, data)
		}
		if out != nil && json.Unmarshal(data, out) != nil {
			t.Fatal("invalid real HTTP response")
		}
		return data
	}
	var issued struct {
		Token string `json:"token"`
	}
	call("POST", "/release-workers", f.body(map[string]any{"worker_id": "doctor-worker-199", "profiles": []any{}, "deployment": false}), "", 201, &issued)
	worker := "doctor-worker-199"
	profile := toolTestProfile()
	inventory := ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}, Maintenance: &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-doctor-199"}}
	call("POST", "/runner/tool-inventory", inventory, issued.Token, 200, nil)
	var current, other ToolProfile
	for index, target := range []*ToolProfile{&current, &other} {
		p := profile
		if index == 1 {
			p.CommandID = "other-command-199"
		}
		call("POST", "/tool-profiles", ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Owned Doctor configuration", Profile: p, IdempotencyKey: newID()}, "", 201, target)
	}
	var reports []ToolProfileDiagnostics
	doctor := func(target ToolProfile, action, inspection, recovery string) ToolProfileDiagnostics {
		t.Helper()
		before := []int{expertCount(t, f, "suma_operations"), expertCount(t, f, "suma_tool_inspections"), expertCount(t, f, "suma_tool_inspection_recoveries"), expertCount(t, f, "dag_runs")}
		var report ToolProfileDiagnostics
		call("GET", "/tool-profiles/"+url.PathEscape(target.ID)+"/diagnostics?project_id="+url.QueryEscape(f.project), nil, "", 200, &report)
		r := report.MaintenanceResolution
		if !ValidToolProfileMaintenanceResolution(r, report.Tool) || r.Action != action || r.InspectionID != inspection || r.RecoveryID != recovery || report.NextAction != "review_profile" || report.Tool.Available || report.Tool.State != "paused" {
			t.Fatalf("maintenance recommendation altered dispatch facts or target: %+v", report)
		}
		after := []int{expertCount(t, f, "suma_operations"), expertCount(t, f, "suma_tool_inspections"), expertCount(t, f, "suma_tool_inspection_recoveries"), expertCount(t, f, "dag_runs")}
		if stringify(before) != stringify(after) {
			t.Fatal("Doctor GET created a domain operation")
		}
		reports = append(reports, report)
		return report
	}
	doctor(current, "inspect", "", "")
	intent := tic.CreateRequest{ProjectID: f.project, ExpectedVersion: current.Version, IdempotencyKey: newID()}
	var pending tic.Inspection
	call("POST", "/tool-profiles/"+current.ID+"/inspections", intent, "", 201, &pending)
	doctor(current, "view_inspection", pending.ID, "")
	doctor(other, "view_inspection", pending.ID, "")
	var original tic.Inspection
	call("GET", "/tool-inspections/"+pending.ID+"?project_id="+f.project, nil, "", 200, &original)
	if original.ToolProfileID != current.ID || original.ToolProfileID == other.ID {
		t.Fatal("cross-profile occupancy was relabeled as the selected profile")
	}
	var claim tic.ClaimReply
	call("POST", "/runner/tool-inspections/claim", tic.ClaimRequest{ProjectID: f.project, WorkerID: worker, ClaimID: newID()}, issued.Token, 200, &claim)
	if claim.Inspection == nil || claim.Inspection.ID != pending.ID {
		t.Fatal("lost exact original claim")
	}
	doctor(current, "view_inspection", pending.ID, "")
	ref := tic.Reference{InspectionID: pending.ID, ClaimID: claim.Inspection.ClaimID, Fence: claim.Inspection.Fence}
	var unknown tic.Inspection
	call("POST", "/runner/tool-inspections/results", tic.FailureRequest{ProjectID: f.project, WorkerID: worker, Inspection: ref, IdempotencyKey: newID(), Outcome: "unknown", FailureCode: "probe_timeout"}, issued.Token, 200, &unknown)
	doctor(current, "recover_inspection", pending.ID, "")
	var recovery tic.ToolInspectionRecovery
	createRecovery := tic.ToolInspectionRecoveryCreateRequest{ProjectID: f.project, ExpectedVersion: unknown.Version, Inspection: ref, IdempotencyKey: newID()}
	first := call("POST", "/tool-inspections/"+pending.ID+"/recoveries", createRecovery, "", 201, &recovery)
	replay := call("POST", "/tool-inspections/"+pending.ID+"/recoveries", createRecovery, "", 201, nil)
	if !bytes.Equal(first, replay) {
		t.Fatal("original recovery receipt replay changed")
	}
	doctor(current, "view_recovery", pending.ID, recovery.ID)
	call("GET", "/tool-inspections/"+pending.ID+"?project_id="+f.project, nil, "", 200, &original)
	proof := tic.ToolInspectionStopProof{Protocol: tic.ProbeStopProtocol, Inspection: ref, SourceSHA256: tic.InspectionSourceSHA256(original), ReceiptSHA256: digest("owned-zero-process-http-proof199"), ProbeReturned: true, AllNativeTreesStopped: true, NoPendingInventory: true, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	var stored tic.ToolInspectionRecovery
	call("POST", "/runner/tool-inspections/recovery/proofs", tic.ToolInspectionRecoveryProofRequest{ProjectID: f.project, WorkerID: worker, RecoveryID: recovery.ID, ExpectedVersion: recovery.Version, IdempotencyKey: newID(), Proof: proof}, issued.Token, 200, &stored)
	doctor(current, "view_recovery", pending.ID, recovery.ID)
	if reports[len(reports)-1].MaintenanceResolution.Reason != "recovery_proof_stored" {
		t.Fatal("stored proof was mistaken for a closed operation")
	}
	call("POST", "/runner/tool-inspections/recovery/acknowledgements", tic.ToolInspectionRecoveryAcknowledgementRequest{ProjectID: f.project, WorkerID: worker, RecoveryID: recovery.ID, ExpectedVersion: stored.Version, IdempotencyKey: newID(), ProofSHA256: stored.ProofSHA256}, issued.Token, 200, nil)
	doctor(current, "inspect", "", "")
	var next tic.Inspection
	intent.IdempotencyKey = newID()
	call("POST", "/tool-profiles/"+current.ID+"/inspections", intent, "", 201, &next)
	if next.ID == pending.ID {
		t.Fatal("new explicitly requested check reused old unknown operation")
	}
	if expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("maintenance started a development run")
	}
	if _, err = f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite")); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{"status": "actual_http_sqlite_original_maintenance_and_cross_profile_navigation_confirmed", "requests": requests, "reports": reports, "original_inspection": pending.ID, "recovery": recovery.ID, "next_inspection": next.ID, "business_runs": 0, "native_processes": 0, "model_calls": 0, "vendor_calls": 0, "machine_scope": "owned HTTP contract fixture with zero-process proof; no native probe or containment repeated", "source": "current scope and original immutable operation; 4 dispatch checks remain separate", "created_at": time.Now().UTC().Format(time.RFC3339Nano)}
	raw, e := json.MarshalIndent(receipt, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), issued.Token) {
		t.Fatal("private credential reached evidence")
	}
	if err = os.WriteFile(filepath.Join(dir, "receipt.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
