package runner

import (
	"context"
	"encoding/json"
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstalledDiscoveryCapabilityClonePreservesOriginal(t *testing.T) {
	source := &ti.ToolMaintenanceCapability{Protocol: ti.MaintenanceProtocol, InstanceID: "owner205", InstalledDiscovery: &ii.Capability{Protocol: ii.Protocol, AdapterIDs: ii.AdapterIDs()}}
	cloned := cloneToolMaintenance(source)
	cloned.InstalledDiscovery.AdapterIDs[0] = "qoder-acp"
	if source.InstalledDiscovery.AdapterIDs[0] != "codex-jsonl" {
		t.Fatal("cloned inventory mutated original continuous discovery capability")
	}
	if !sameToolMaintenance(source, cloneToolMaintenance(source)) || sameToolMaintenance(source, cloned) {
		t.Fatal("nested capability ACK comparison is not exact by value")
	}
}

func installationFixture205(project, worker, claim, owner string) ii.Scan {
	now := time.Now().UTC()
	scan := ii.Scan{ID: "scan205", ProjectID: project, WorkerID: worker, AdapterIDs: []string{"codebuddy-cli"}, InstanceID: owner, Version: 2, State: "claimed", RequestedBy: "actor205", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(300 * time.Second).Format(time.RFC3339Nano), Fence: 1, ClaimID: claim, ClaimedAt: now.Format(time.RFC3339Nano), LeaseUntil: now.Add(60 * time.Second).Format(time.RFC3339Nano)}
	scan.SourceSHA256 = ii.ScanSourceSHA256(scan)
	return scan
}

func installationHead205(scan ii.Scan, request ii.ResultRequest) ii.Scan {
	scan.Version++
	scan.State = request.Outcome
	scan.ResultCode = request.FailureCode
	if request.Outcome != "unknown" {
		scan.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		scan.Result = &ii.Result{Items: request.Items, StopProof: request.StopProof}
	}
	if request.Outcome == "completed" {
		scan.ResultCode = "scan_completed"
	}
	return scan
}

// The complete HTTP/SQLite/native/browser journey belongs to Root's one actual
// check. These protocol fixtures exercise the two durable lost-ACK boundaries.
func TestInstalledDiscoveryRecoversOriginalClaimAndResult(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // Nothing installed: no external process or model.
	projected, e := installationItems(InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{{AdapterID: "codebuddy-cli", Installed: true, LaunchSupported: true, Version: "fixture205", ReasonCode: "workspace_required", Executable: "private-fixture-path205", Command: &CommandSpec{Env: map[string]string{"TEST_KEY": "private-fixture-value205"}}}}}, []string{"codebuddy-cli"})
	public, _ := json.Marshal(projected)
	if e != nil || strings.Contains(string(public), "private-fixture-") || strings.Contains(string(public), `"command"`) {
		t.Fatal("remote result included private paths or command configuration", e)
	}
	ctx := context.Background()
	var claims []string
	var results [][]byte
	var original ii.Scan
	var owner string
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture205" {
			t.Error("wrong machine credential")
		}
		switch r.URL.Path {
		case "/api/v1/runner/tool-installation-scans/claim":
			var req ii.ClaimRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			claims = append(claims, req.ClaimID)
			if len(claims) > 2 {
				_ = json.NewEncoder(out).Encode(ii.ClaimReply{})
				return
			}
			if len(claims) == 1 {
				original = installationFixture205(req.ProjectID, req.WorkerID, req.ClaimID, owner)
				out.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(out).Encode(ii.ClaimReply{Scan: &original})
		case "/api/v1/runner/tool-installation-scans/results":
			raw, _ := io.ReadAll(r.Body)
			results = append(results, raw)
			var req ii.ResultRequest
			_ = json.Unmarshal(raw, &req)
			if len(results) == 1 {
				out.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(out).Encode(installationHead205(original, req))
		default:
			t.Error("unexpected path", r.URL.Path)
			out.WriteHeader(404)
		}
	}))
	defer server.Close()
	directory := t.TempDir()
	j, err := OpenMaintenanceJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	worker := &Worker{Client: &Client{BaseURL: server.URL, ProjectID: "project205", Token: "fixture205"}, ID: "worker205", Journal: j, AllowInstalledToolDiscovery: true}
	if err = worker.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	owner = worker.maintenance.InstanceID
	worker.acceptInventory(InventoryReport{})
	if handled, e := worker.serviceInstallationScan(ctx); !handled || e == nil {
		t.Fatal("lost claim did not remain pending", handled, e)
	}
	if handled, e := worker.serviceInstallationScan(ctx); !handled || e == nil {
		t.Fatal("lost result did not remain pending", handled, e)
	}
	checkpoint, err := j.installationCheckpoint()
	if err != nil || checkpoint.Result == nil || checkpoint.Result.Outcome != "completed" {
		t.Fatal("exact result not durable", err)
	}
	if len(checkpoint.Result.Items) != 1 || checkpoint.Result.Items[0].ReasonCode != "not_installed" || checkpoint.Result.StopProof.NativeProcessCount != 0 {
		t.Fatal("scan facts were fabricated")
	}
	dir, _, err := j.installationProbeDirectory(*checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	source, _ := os.ReadFile(filepath.Join(dir, "source.json"))
	if strings.Contains(string(source), `"inspection"`) || !strings.Contains(string(source), `"installation_scan"`) {
		t.Fatal("scan invented an inspection source")
	}
	_ = j.Close()
	j, err = OpenMaintenanceJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	worker.Journal = j
	t.Setenv("PATH", strings.Repeat("x", 32769)) // Re-running discovery would now fail.
	if handled, e := worker.serviceInstallationScan(ctx); !handled || e != nil {
		t.Fatal("original result ACK was not restored", e)
	}
	if len(claims) != 2 || claims[0] != claims[1] || len(results) != 2 || string(results[0]) != string(results[1]) {
		t.Fatal("lost ACK changed original nonce/body or repeated the probe")
	}
	checkpoint, err = j.installationCheckpoint()
	if err != nil || !checkpoint.Complete {
		t.Fatal("completion not durable", err)
	}
	if _, e := worker.serviceInstallationScan(ctx); e != nil {
		t.Fatal(e)
	}
	archive, _ := os.ReadFile(filepath.Join(directory, "tool-installation-history", checkpoint.ClaimID+".json"))
	var saved installationCheckpoint
	if json.Unmarshal(archive, &saved) != nil || saved.Result == nil || saved.Result.IdempotencyKey != checkpoint.Result.IdempotencyKey {
		t.Fatal("rotating completion lost original command evidence")
	}
}

func TestInstalledDiscoveryUnknownUsesOnlyOriginalStopReceipt(t *testing.T) {
	for _, hasReceipt := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_receipt_blocked", true: "original_receipt_closure"}[hasReceipt], func(t *testing.T) {
			var requests []ii.ResultRequest
			var bodies [][]byte
			var scan ii.Scan
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/runner/tool-installation-scans/results" {
					t.Error("unknown attempted a new claim or probe")
					out.WriteHeader(404)
					return
				}
				raw, _ := io.ReadAll(r.Body)
				var req ii.ResultRequest
				_ = json.Unmarshal(raw, &req)
				requests = append(requests, req)
				bodies = append(bodies, raw)
				if len(requests) == 2 {
					out.WriteHeader(503)
					return
				}
				_ = json.NewEncoder(out).Encode(installationHead205(scan, req))
			}))
			defer server.Close()
			directory := t.TempDir()
			j, err := OpenMaintenanceJournal(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			worker := &Worker{Client: &Client{BaseURL: server.URL, ProjectID: "project205", Token: "fixture205"}, ID: "worker205", Journal: j, AllowInstalledToolDiscovery: true}
			if err = worker.enableToolMaintenance(); err != nil {
				t.Fatal(err)
			}
			scan = installationFixture205("project205", worker.ID, "claim205", worker.maintenance.InstanceID)
			scope, _ := InventoryScopeSHA256(server.URL, scan.ProjectID, scan.WorkerID)
			p := installationCheckpoint{ScopeSHA256: scope, WorkerID: worker.ID, ClaimID: scan.ClaimID, Responded: true, Scan: &scan, Started: true}
			if err = j.saveInstallation(p); err != nil {
				t.Fatal(err)
			}
			if hasReceipt {
				dir, source, e := j.installationProbeDirectory(p)
				if e != nil {
					t.Fatal(e)
				}
				collector, e := newToolProbeCollectorAt(dir, source)
				if e != nil || collector.closeReturned() != nil {
					t.Fatal("cannot save original no-process receipt", e)
				}
			}
			if _, err = worker.serviceInstallationScan(context.Background()); err != nil {
				t.Fatal(err)
			} // Restart observes unknown; never scans again.
			if len(requests) != 1 || requests[0].Outcome != "unknown" || requests[0].StopProof != nil {
				t.Fatal("unknown result invented process facts")
			}
			if _, _, e := worker.ReportTools(context.Background()); e == nil || len(requests) != 1 {
				t.Fatal("single-report mode bypassed original unknown scan")
			}
			_, err = worker.serviceInstallationScan(context.Background())
			if !hasReceipt {
				if err != nil || len(requests) != 1 || worker.canClaimInventory() {
					t.Fatal("missing durable stop proof did not keep maintenance blocked", err)
				}
				return
			}
			if err == nil || len(requests) != 2 {
				t.Fatal("lost closure ACK was not preserved")
			}
			_ = j.Close()
			j, err = OpenMaintenanceJournal(directory)
			if err != nil {
				t.Fatal(err)
			}
			worker.Journal = j
			if _, err = worker.serviceInstallationScan(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(requests) != 3 || requests[0].IdempotencyKey == requests[1].IdempotencyKey || string(bodies[1]) != string(bodies[2]) || requests[1].Outcome != "closed_unknown" || requests[1].StopProof.NativeProcessCount != 0 {
				t.Fatal("closure rewrote unknown or replayed its probe")
			}
			p2, _ := j.installationCheckpoint()
			if !p2.Complete || p2.Result.Outcome != "unknown" {
				t.Fatal("closure lost original unknown fact")
			}
		})
	}
}

func TestInstalledDiscoveryMaintenanceOnlyDoesNotClaimBusiness(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "worker", true: "pool"}[pool], func(t *testing.T) {
			var reports atomic.Int32
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if pool && r.URL.Path == "/api/v1/pool-runner/session" {
					_ = json.NewEncoder(out).Encode(map[string]any{"runner": map[string]any{"id": "runner205", "pool_id": "pool205", "worker_id": "pool-runner205", "max_slots": 1, "credential_version": 1}, "pool_active": true, "scopes": []any{map[string]string{"tenant_id": "tenant205", "project_id": "project205"}}})
					return
				}
				var body map[string]json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&body)
				if pool && (string(body["runner_id"]) != `"runner205"` || string(body["tenant_id"]) != `"tenant205"` || body["worker_id"] != nil || body["assignment"] != nil) {
					t.Error("maintenance crossed pool machine/project grant")
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/tool-inspections/claim"):
					_, _ = out.Write([]byte(`{"inspection":null}`))
				case strings.HasSuffix(r.URL.Path, "/tool-installation-scans/claim"):
					_, _ = out.Write([]byte(`{"scan":null}`))
					cancel()
				case strings.HasSuffix(r.URL.Path, "/tool-inventory"):
					var cap ti.ToolMaintenanceCapability
					_ = json.Unmarshal(body["maintenance"], &cap)
					if cap.InstalledDiscovery == nil || !reflect.DeepEqual(cap.InstalledDiscovery.AdapterIDs, ii.AdapterIDs()) {
						t.Error("continuous scan consumer did not advertise exact fixed directory")
					}
					reports.Add(1)
					_ = json.NewEncoder(out).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano), "maintenance": cap})
				default:
					t.Error("maintenance-only entered business endpoint", r.URL.Path)
					out.WriteHeader(500)
					cancel()
				}
			}))
			defer server.Close()
			directory := t.TempDir()
			done := make(chan error, 1)
			if pool {
				worker := &PoolWorker{Client: &Client{BaseURL: server.URL, Token: "fixture205", Pool: &PoolContext{PoolID: "pool205", RunnerID: "runner205"}}, Config: PoolLocalConfig{SchemaVersion: 1, PoolID: "pool205", RunnerID: "runner205", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant205", ProjectID: "project205", Workspace: directory}}}, StateDir: filepath.Join(directory, "state"), MaintenanceOnly: true, AllowInstalledToolDiscovery: true, PollInterval: time.Millisecond}
				go func() { done <- worker.Run(ctx) }()
			} else {
				j, err := OpenMaintenanceJournal(directory)
				if err != nil {
					t.Fatal(err)
				}
				defer j.Close()
				worker := &Worker{Client: &Client{BaseURL: server.URL, Token: "fixture205", ProjectID: "project205"}, Journal: j, ID: "worker205", MaintenanceOnly: true, AllowInstalledToolDiscovery: true, PollInterval: time.Millisecond}
				go func() { done <- worker.Run(ctx) }()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal("idle maintenance cancellation did not confirm shutdown", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("maintenance owner did not stop")
			}
			if reports.Load() != 1 {
				t.Fatal("continuous owner did not publish one current inventory before scanning", reports.Load())
			}
		})
	}
	directory := t.TempDir()
	j, err := OpenJournal(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Accept(Assignment{RunID: "run205", NodeID: "node205", Attempt: 1, Fence: 1, WorkerID: "worker205"}); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	original, _ := os.ReadFile(filepath.Join(directory, "journal.json"))
	if opened, e := OpenMaintenanceJournal(directory); e == nil {
		_ = opened.Close()
		t.Fatal("unfinished business journal accepted maintenance-only")
	}
	after, _ := os.ReadFile(filepath.Join(directory, "journal.json"))
	if string(original) != string(after) {
		t.Fatal("rejected mode changed original business execution evidence")
	}
	if _, e := discoverInstalledToolSubset(context.Background(), []string{"arbitrary-shell"}); e == nil {
		t.Fatal("arbitrary discovery executable accepted")
	}

	// Inspect the actual common launch boundary without launching a vendor. A
	// failed start cannot certify zero launched trees merely because no PID was
	// returned, while every attempted remote probe still receives a private home.
	j, err = OpenMaintenanceJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	scan := installationFixture205("project205", "worker205", "claim205", "owner205")
	p := installationCheckpoint{ScopeSHA256: strings.Repeat("a", 64), WorkerID: scan.WorkerID, ClaimID: scan.ClaimID, Responded: true, Scan: &scan, Started: true}
	dir, source, err := j.installationProbeDirectory(p)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollectorAt(dir, source)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(t.TempDir(), "absent-fixture205.exe"))
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH=retained-path", "HOME=original-private-home", "APPDATA=original-private-settings", "SYSTEMROOT=retained-system"}
	if _, e := startProbeProcess(collector.context(context.Background()), cmd); e == nil {
		t.Fatal("missing fixture unexpectedly launched")
	}
	values := map[string]string{}
	for _, value := range cmd.Env {
		name, v, _ := strings.Cut(value, "=")
		values[name] = v
	}
	probeRoot := filepath.Dir(cmd.Dir)
	home := filepath.Join(probeRoot, "home")
	if filepath.Dir(probeRoot) != dir || filepath.Base(cmd.Dir) != "working" || values["HOME"] != home || values["APPDATA"] != home || values["LOCALAPPDATA"] != home || values["PATH"] != "retained-path" || values["SYSTEMROOT"] != "retained-system" {
		t.Fatal("remote probe inherited original home/configuration or lost fixed OS/PATH")
	}
	if collector.closeReturned() != nil {
		t.Fatal("failed-start return receipt was not durable")
	}
	if _, e := j.installationStopProof(p); e == nil {
		t.Fatal("unverified native start became fabricated zero-tree proof")
	}
}
