package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func preparationFixture224(t *testing.T, w *Worker) tp.Operation {
	t.Helper()
	now := time.Now().UTC()
	scanRef := ii.Reference{ScanID: "scan224", ClaimID: operationID(), Fence: 1}
	scanSHA := digest([]byte("owned scan fixture224"))
	stop := &ii.StopProof{Protocol: ii.StopProtocol, Scan: scanRef, SourceSHA256: scanSHA, ReceiptSHA256: digest([]byte("owned stop fixture224")), ProbeReturned: true, AllNativeTreesStopped: true, ObservedAt: now.Format(time.RFC3339Nano)}
	op := tp.Operation{ID: "preparation224", TenantID: "tenant224", ProjectID: w.Client.ProjectID, WorkerID: w.ID, Version: 2, State: "claimed", AdapterID: "qoder-acp", Configuration: tp.Configuration{Model: "fixed-model224"}, InstanceID: w.maintenance.InstanceID, InstallationScanID: scanRef.ScanID, InstallationScanVersion: 3, InstallationReference: scanRef, InstallationSourceSHA256: scanSHA, InstallationItem: ii.Item{AdapterID: "qoder-acp", Protocol: ac.Protocol, Installed: true, ReasonCode: "acp_configuration_required", Version: "reviewed-fixture224", ExecutableSHA256: digest([]byte("native fixture224"))}, InstallationStopProof: stop, ExpectedGenerationSHA256: w.currentGenerationSHA(), RequestedBy: "actor224", CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(300 * time.Second).Format(time.RFC3339Nano), Fence: 1, ClaimID: operationID(), ClaimedAt: now.Format(time.RFC3339Nano), LeaseUntil: now.Add(60 * time.Second).Format(time.RFC3339Nano)}
	op.SourceSHA256 = tp.SourceSHA256(op)
	if err := tp.ValidateOperation(op); err != nil {
		t.Fatal(err)
	}
	return op
}
func preparationWorker224(t *testing.T, base string, configs ...ExecConfig) *Worker {
	t.Helper()
	j, err := OpenMaintenanceJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	w := &Worker{Client: &Client{BaseURL: base, ProjectID: "project224", Token: "owned224"}, Journal: j, ID: "worker224", AllowInstalledToolDiscovery: true, AllowToolPreparation: true, MaintenanceOnly: true}
	if len(configs) > 0 {
		w.Exec = configs[0]
	}
	if err = w.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestManagedPreparationCapabilityAndImmutableSnapshots(t *testing.T) {
	w := preparationWorker224(t, "http://127.0.0.1:4310")
	c := w.currentToolMaintenance()
	copy := cloneToolMaintenance(c)
	copy.ManagedPreparation.AdapterIDs[0] = "cursor-cli"
	if sameToolMaintenance(c, copy) || !sameToolMaintenance(c, cloneToolMaintenance(c)) {
		t.Fatal("managed capability aliases or ignores its nested values")
	}
	before := w.currentExecSnapshot()
	base := w.currentGenerationSHA()
	w.execMu.Lock()
	w.managedCommands = map[string]CommandSpec{"new224": {Executable: "owned-not-run", ReadOnly: true}}
	w.managedGenerationSHA = digest([]byte("next224"))
	w.execMu.Unlock()
	if len(before.Commands) != 0 || len(w.currentExecSnapshot().Commands) != 1 || w.Exec.AllowExec || w.Exec.Commands != nil {
		t.Fatal("new generation mutated an inflight/base snapshot or widened execution")
	}
	a, err := baseExecutionSHA(ExecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := baseExecutionSHA(ExecConfig{AllowExec: true})
	other, _ := baseExecutionSHA(ExecConfig{Workspace: "another-resource"})
	if a != b || a == other || base == w.currentGenerationSHA() {
		t.Fatal("base/generation/resource identities were conflated")
	}
	if c.ManagedPreparation.GenerationSHA256 != base || w.currentToolMaintenance().ManagedPreparation.GenerationSHA256 != w.currentGenerationSHA() {
		t.Fatal("old report capability mutated or new report used stale generation")
	}
}

func TestManagedPreparationPrivateScanRequiresExactReceiptAndPins(t *testing.T) {
	w := preparationWorker224(t, "http://127.0.0.1:4310")
	op := preparationFixture224(t, w)
	if _, err := w.Journal.preparationNative(context.Background(), digest([]byte("scope")), op); err == nil || err.Error() != "rescan_required" {
		t.Fatal("old public scan was treated as private installation authority")
	}
	input := qoderPreparationInput221(t)
	_, sha, err := qoderNativeTarget(context.Background(), input.Executable)
	if err != nil {
		t.Fatal(err)
	}
	op.InstallationItem.ExecutableSHA256 = sha
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	receipt := installationPrivateReceipt{Protocol: "suma-installed-tool-private-v1", ScopeSHA256: scope, Reference: op.InstallationReference, SourceSHA256: op.InstallationSourceSHA256, Items: []installationNativeObservation{{Item: op.InstallationItem, Executable: input.Executable}}}
	dir, err := w.Journal.maintenanceDirectory("tool-installation-observations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writeToolProbeImmutable(filepath.Join(dir, op.InstallationReference.ClaimID+".json"), receipt); err != nil {
		t.Fatal(err)
	}
	if native, e := w.Journal.preparationNative(context.Background(), scope, op); e != nil || native.Executable != input.Executable {
		t.Fatal("exact immutable private scan rejected", e)
	}
	for _, edit := range []func(*tp.Operation){func(v *tp.Operation) { v.InstallationItem.Version = "other" }, func(v *tp.Operation) { v.InstallationSourceSHA256 = digest([]byte("other")) }, func(v *tp.Operation) { v.InstallationReference.Fence++ }} {
		changed := op
		edit(&changed)
		if _, e := w.Journal.preparationNative(context.Background(), scope, changed); e == nil {
			t.Fatal("a different native observation/source was adopted")
		}
	}
	public, _ := json.Marshal(op)
	if strings.Contains(string(public), input.Executable) {
		t.Fatal("private native path entered public operation")
	}
}

func TestManagedPreparationEmptyClaimAcceptsOnlyExactNullableReceipt(t *testing.T) {
	replies := []struct {
		body string
		ok   bool
	}{
		{`{"tenant_id":"tenant224","project_id":"project224","worker_id":"worker224","preparation":null}`, true},
		{`{"tenant_id":"tenant224","project_id":"project224","worker_id":"worker224"}`, false},
		{`{"tenant_id":"tenant224","project_id":"project224","worker_id":"worker224","preparation":{}}`, false},
		{`{"tenant_id":"tenant224","project_id":"project224","worker_id":"worker224","other":null}`, false},
		{`{"tenant_id":"tenant224","project_id":"other","worker_id":"worker224","preparation":null}`, false},
	}
	for _, tt := range replies {
		t.Run(tt.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(out, tt.body)
			}))
			defer server.Close()
			w := preparationWorker224(t, server.URL)
			got, err := w.Client.claimToolPreparation(context.Background(), w.ID, operationID())
			if (err == nil) != tt.ok || got.Preparation != nil {
				t.Fatalf("unexpected result from canonical claim: %+v, %v", got, err)
			}
		})
	}
}

func TestManagedPreparationClaimAndUnknownResultRestoreOriginalIntent(t *testing.T) {
	var op tp.Operation
	var rawResults [][]byte
	var claims []string
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer owned224" {
			t.Error("wrong identity")
		}
		switch r.URL.Path {
		case "/api/v1/runner/tool-preparations/claim":
			var q tp.ClaimRequest
			_ = json.NewDecoder(r.Body).Decode(&q)
			claims = append(claims, q.ClaimID)
			if len(claims) == 1 {
				out.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(out).Encode(tp.Claim{TenantID: op.TenantID, ProjectID: op.ProjectID, WorkerID: op.WorkerID, Preparation: &op})
		case "/api/v1/runner/tool-preparations/results":
			raw, _ := io.ReadAll(r.Body)
			rawResults = append(rawResults, raw)
			if len(rawResults) == 1 {
				out.WriteHeader(503)
				return
			}
			head := op
			head.State = "unknown"
			head.Version++
			head.ResultCode = "worker_restarted"
			_ = json.NewEncoder(out).Encode(head)
		default:
			t.Error("unexpected request", r.URL.Path)
			out.WriteHeader(404)
		}
	}))
	defer server.Close()
	w := preparationWorker224(t, server.URL)
	op = preparationFixture224(t, w)
	if _, err := w.Client.claimToolPreparation(context.Background(), w.ID, op.ClaimID); err == nil {
		t.Fatal("lost claim was treated as confirmed")
	}
	got, err := w.Client.claimToolPreparation(context.Background(), w.ID, op.ClaimID)
	if err != nil || got.Preparation.ID != op.ID || claims[0] != claims[1] {
		t.Fatal("original claim nonce did not restore", err)
	}
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	p := toolPreparationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: op.ClaimID, Responded: true, TenantID: op.TenantID, Operation: &op, Started: true, Intent: &ACPToolPreparationInput{AdapterID: op.AdapterID}}
	if err = w.Journal.savePreparation(p); err != nil {
		t.Fatal(err)
	}
	if _, err = w.serviceToolPreparation(context.Background()); err == nil {
		t.Fatal("lost result acknowledgement was accepted")
	}
	durable, err := w.Journal.preparationCheckpoint()
	if err != nil || durable.Result.Outcome != "unknown" {
		t.Fatal("started-no-receipt was replayed instead of preserved", err)
	}
	key := durable.Result.IdempotencyKey
	if _, err = w.serviceToolPreparation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(rawResults) != 2 || string(rawResults[0]) != string(rawResults[1]) {
		t.Fatal("result retry changed original body/key")
	}
	if _, err = w.serviceToolPreparation(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, _ := w.Journal.preparationCheckpoint()
	if current.Complete || current.Closure != nil || current.Result.IdempotencyKey != key || w.canClaimInventory() {
		t.Fatal("missing genuine stop receipt freed unknown operation")
	}
	if _, err = os.Stat(filepath.Join(filepath.Dir(w.Journal.path), "tool-command-generations")); !os.IsNotExist(err) {
		t.Fatal("unknown operation ran its factory again")
	}
}

func TestManagedPreparationStopReceiptUsesOwnReferenceAndNoProbeReplay(t *testing.T) {
	w := preparationWorker224(t, "http://127.0.0.1:4310")
	op := preparationFixture224(t, w)
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	p := toolPreparationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: op.ClaimID, Responded: true, TenantID: op.TenantID, Operation: &op, Started: true, Intent: &ACPToolPreparationInput{AdapterID: op.AdapterID}}
	directory, source, err := w.Journal.preparationProbeDirectory(p)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollectorAt(directory, source)
	if err != nil {
		t.Fatal(err)
	}
	if err = collector.closeReturned(); err != nil {
		t.Fatal(err)
	}
	proof, err := w.Journal.preparationStopProof(p)
	if err != nil || proof.NativeProcessCount != 0 || proof.Preparation != preparationReference(op) {
		t.Fatal("own zero-native receipt not recoverable", err)
	}
	raw, _ := json.Marshal(source)
	if strings.Contains(string(raw), "installation_scan") || strings.Contains(string(raw), `"inspection"`) {
		t.Fatal("preparation invented another maintenance source")
	}
	var decoded toolProbeSource
	if err = json.Unmarshal(raw, &decoded); err != nil || decoded != source {
		t.Fatal("source roundtrip differs", err)
	}
	p.Operation = &tp.Operation{}
	if _, err = w.Journal.preparationStopProof(p); err == nil {
		t.Fatal("stop receipt accepted a different reference")
	}
}

func TestManagedPreparationInventoryRequiresExactCompletionEcho(t *testing.T) {
	binding := tp.InventoryBinding{Reference: tp.Reference{PreparationID: "preparation224", ClaimID: operationID(), Fence: 1}, SourceSHA256: digest([]byte("source")), GenerationSHA256: digest([]byte("generation")), LocalReceiptSHA256: digest([]byte("acceptance"))}
	mode := "missing"
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		var q map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&q)
		if _, ok := q["preparation"]; !ok {
			t.Error("report lost causal preparation")
		}
		ack := map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339Nano)}
		if mode != "missing" {
			copy := binding
			if mode == "wrong" {
				copy.Reference.Fence++
			}
			ack["preparation"] = copy
			ack["preparation_state"] = "completed"
		}
		_ = json.NewEncoder(out).Encode(ack)
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, ProjectID: "project224", Token: "owned224"}
	scope, _ := InventoryScopeSHA256(c.BaseURL, c.ProjectID, "worker224")
	report := InventoryReport{WorkerID: "worker224", OperationID: operationID(), Items: []tc.Observation{}, ScopeSHA256: scope, Preparation: &binding}
	for _, m := range []string{"missing", "wrong", "exact"} {
		mode = m
		_, err := c.ToolInventory(context.Background(), report)
		if (err == nil) != (m == "exact") {
			t.Fatal("inexact preparation inventory acknowledgement", m, err)
		}
	}
}

func TestManagedPreparationUnknownClosureAndCapabilityOptInRemainIndependent(t *testing.T) {
	w := preparationWorker224(t, "http://127.0.0.1:4310")
	op := preparationFixture224(t, w)
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	p := toolPreparationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: op.ClaimID, Responded: true, TenantID: op.TenantID, Operation: &op, Started: true, Intent: &ACPToolPreparationInput{AdapterID: op.AdapterID}}
	p.Result = w.preparationResult(p, "unknown", "worker_restarted", nil, nil)
	p.ResultAcked = true
	original := *p.Result
	p.Closure = w.preparationResult(p, "closed_unknown", "worker_restarted", nil, &tp.StopProof{Protocol: tp.StopProtocol, Preparation: preparationReference(op), SourceSHA256: op.SourceSHA256, ReceiptSHA256: digest([]byte("fixture receipt")), ProbeReturned: true, AllNativeTreesStopped: true, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err := validateToolPreparationCheckpoint(p); err != nil || !reflect.DeepEqual(original, *p.Result) || p.Result.IdempotencyKey == p.Closure.IdempotencyKey {
		t.Fatal("closure rewrote unknown intent", err)
	}
	p.Closure.IdempotencyKey = p.Result.IdempotencyKey
	if validateToolPreparationCheckpoint(p) == nil {
		t.Fatal("closure reused unknown key")
	}
	p.Closure = nil
	if err := w.Journal.savePreparation(p); err != nil {
		t.Fatal(err)
	}
	w.AllowToolPreparation = false
	if _, err := w.serviceToolPreparation(context.Background()); err == nil {
		t.Fatal("old pending operation ignored revoked local opt-in")
	}
	ordinary := &Worker{Client: w.Client, Journal: w.Journal, ID: w.ID}
	if err := ordinary.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	if ordinary.maintenance.ManagedPreparation != nil || ordinary.maintenance.InstalledDiscovery != nil || w.Exec.AllowExec {
		t.Fatal("ordinary or one-off capability silently widened")
	}
	// Deep cloning must also preserve the original inspection/scan declaration.
	cap := &ti.ToolMaintenanceCapability{Protocol: ti.MaintenanceProtocol, InstanceID: operationID(), InstalledDiscovery: &ii.Capability{Protocol: ii.Protocol, AdapterIDs: ii.AdapterIDs()}, ManagedPreparation: &tp.Capability{Protocol: tp.Protocol, AdapterIDs: tp.AdapterIDs(), GenerationSHA256: op.ExpectedGenerationSHA256}}
	clone := cloneToolMaintenance(cap)
	clone.InstalledDiscovery.AdapterIDs[0] = "qoder-acp"
	if sameToolMaintenance(cap, clone) {
		t.Fatal("scan mismatch hidden by equal preparation capability")
	}
}

func TestManagedGenerationRecoversHeadBeforeMirrorWithoutRewritingOrProbing(t *testing.T) {
	// This is a protocol/durability fixture. The original static factory writes
	// real commands and pins an owned inert native-format file, never launched.
	var operation tp.Operation
	var claims []string
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tool-preparations/claim" {
			t.Error("restore performed an external effect", r.URL.Path)
			out.WriteHeader(404)
			return
		}
		var req tp.ClaimRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		claims = append(claims, req.ClaimID)
		_ = json.NewEncoder(out).Encode(tp.Claim{TenantID: operation.TenantID, ProjectID: operation.ProjectID, WorkerID: operation.WorkerID, Preparation: &operation})
	}))
	defer server.Close()
	originalConfig := ExecConfig{Commands: map[string]CommandSpec{"existing-base224": {Executable: "original-command-not-started224", ReadOnly: true, Env: map[string]string{"PRIVATE_BASE_VALUE": "must-not-spread-to-maintenance224"}}}}
	w := preparationWorker224(t, server.URL, originalConfig)
	operation = preparationFixture224(t, w)
	input := qoderPreparationInput221(t)
	// A small inert, task-owned native-format file is sufficient for static
	// preparation. It is never launched and permits a real byte-drift boundary
	// without touching os.Executable or copying a large installed binary.
	input.Executable = filepath.Join(t.TempDir(), "owned-static-qoder224.exe")
	if err := os.WriteFile(input.Executable, []byte("MZ\x00\x00owned-inert-static-fixture224"), 0700); err != nil {
		t.Fatal(err)
	}
	operation.Configuration.Model = input.Model
	operation.InstallationItem.Version = input.ReviewedVersion
	_, sha, err := qoderNativeTarget(context.Background(), input.Executable)
	if err != nil {
		t.Fatal(err)
	}
	operation.InstallationItem.ExecutableSHA256 = sha
	operation.SourceSHA256 = tp.SourceSHA256(operation)
	scope, _ := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	p := toolPreparationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: operation.ClaimID, Responded: true, TenantID: operation.TenantID, Operation: &operation, Started: true, Intent: &input}
	dir, err := w.Journal.maintenanceDirectory("tool-command-generations")
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(dir, operation.ID+"-commands.json")
	raw, _ := json.Marshal(input)
	if _, err = PrepareACPToolCommands(context.Background(), artifact, strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	artifactRaw, _ := os.ReadFile(artifact)
	prepared, err := DecodeCommands(artifactRaw)
	if err != nil {
		t.Fatal(err)
	}
	spec := prepared[QoderToolPreparationCommandID]
	id := tp.CommandID(operation.ID)
	profileSHA, err := effectiveCommandProfileDigest(w.Exec, spec, sha)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := localACPCapability(spec)
	if err != nil {
		t.Fatal(err)
	}
	profile := tc.Profile{CommandID: id, Adapter: spec.Adapter, Protocol: spec.Protocol, ProfileSHA256: profileSHA, ExecutableSHA256: sha, ToolVersion: input.ReviewedVersion, ACPExecution: capability}
	probeDir, source, err := w.Journal.preparationProbeDirectory(p)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollectorAt(probeDir, source)
	if err != nil {
		t.Fatal(err)
	}
	if err = collector.closeReturned(); err != nil {
		t.Fatal(err)
	}
	proof, err := w.Journal.preparationStopProof(p)
	if err != nil {
		t.Fatal(err)
	}
	generation := toolCommandGeneration{Protocol: tp.Protocol, ScopeSHA256: scope, TenantID: operation.TenantID, ProjectID: w.Client.ProjectID, WorkerID: w.ID, BaseSHA256: w.managedBaseSHA, ParentSHA256: w.currentGenerationSHA(), Preparation: preparationReference(operation), SourceSHA256: operation.SourceSHA256, Commands: map[string]CommandSpec{id: spec}, Profiles: map[string]tc.Profile{id: profile}}
	generationRaw, _ := json.Marshal(generation)
	generationSHA := digest(generationRaw)
	head := &toolGenerationHead{GenerationSHA256: generationSHA, Generation: generation, Receipt: tp.GenerationReceipt{ParentGenerationSHA256: generation.ParentSHA256, GenerationSHA256: generationSHA, CommandID: id, ArtifactSHA256: digest(artifactRaw), Profile: profile, StopProof: &proof}, Inventory: InventoryReport{WorkerID: w.ID, OperationID: operationID(), ScopeSHA256: scope, Items: []tc.Observation{{Profile: profile, Available: false, Reason: "execution is disabled by the local administrator"}}}}
	// Force the exact failure AFTER durable head, BEFORE inventory mirror.
	if err = os.Mkdir(w.Journal.inventoryPath(), 0700); err != nil {
		t.Fatal(err)
	}
	if err = w.persistManagedGeneration(p, head); err == nil {
		t.Fatal("fixture failed to interrupt mirror persistence")
	}
	if len(w.currentExecSnapshot().Commands) != 1 {
		t.Fatal("failed mirror published early")
	}
	if err = os.Remove(w.Journal.inventoryPath()); err != nil {
		t.Fatal(err)
	}
	accepted, err := w.Journal.readToolGenerationHead()
	if err != nil || accepted == nil {
		t.Fatal("durable head lost at mirror failure", err)
	}
	w.managedRestored = true // The SAME running process completed its startup earlier.
	if err = w.ensureAcceptedGeneration(context.Background(), accepted); err != nil {
		t.Fatal(err)
	}
	pending := w.Journal.PendingInventory()
	if w.currentGenerationSHA() != generationSHA || len(w.currentExecSnapshot().Commands) != 2 || pending == nil || pending.OperationID != head.Inventory.OperationID || pending.Preparation == nil || w.canClaimInventory() || w.Exec.AllowExec {
		t.Fatal("recovery did not publish exact accepted snapshot and original pending report")
	}
	for _, file := range []string{w.Journal.toolGenerationHeadPath(), filepath.Join(dir, generationSHA+".json"), filepath.Join(dir, operation.ID+"-accepted.json")} {
		bytes, e := os.ReadFile(file)
		if e != nil || strings.Contains(string(bytes), "must-not-spread-to-maintenance224") {
			t.Fatal("base credential value was duplicated into maintenance state", e)
		}
	}
	before, _ := os.ReadFile(artifact)
	// Fresh in-memory Worker, same durable Journal and original canonical nonce.
	restartConfig := originalConfig
	restartConfig.AllowExec = true
	restarted := &Worker{Client: w.Client, Journal: w.Journal, ID: w.ID, AllowInstalledToolDiscovery: true, AllowToolPreparation: true, Exec: restartConfig}
	if err = restarted.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	operation.State = "awaiting_inventory"
	operation.ResultCode = "prepared"
	operation.Receipt = &accepted.Receipt
	if err = restarted.restoreToolGeneration(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(artifact)
	if len(claims) != 1 || claims[0] != operation.ClaimID || !reflect.DeepEqual(before, after) || restarted.currentGenerationSHA() != generationSHA || len(w.Exec.Commands) != 1 {
		t.Fatal("restart used new claim, reran factory or mutated base commands")
	}
	// Keep the head, manifest, profile and acceptance bytes EXACTLY unchanged.
	// This reaches the native file SHA check, rather than failing an earlier
	// in-memory manifest equality guard.
	if err = os.WriteFile(input.Executable, []byte("MZ\x00\x00different-owned-inert-bytes224"), 0700); err != nil {
		t.Fatal(err)
	}
	drifted := &Worker{Client: w.Client, Journal: w.Journal, ID: w.ID, AllowInstalledToolDiscovery: true, AllowToolPreparation: true, Exec: restartConfig}
	if err = drifted.enableToolMaintenance(); err != nil {
		t.Fatal(err)
	}
	if err = drifted.restoreToolGeneration(context.Background()); err == nil || err.Error() != "managed native generation changed" {
		t.Fatal("native byte drift did not reach the static pin guard", err)
	}
	if drifted.currentGenerationSHA() == generationSHA || len(drifted.currentExecSnapshot().Commands) != 1 || len(claims) != 2 || claims[1] != operation.ClaimID {
		t.Fatal("drifted restore published a generation or changed its original canonical claim")
	}
}
