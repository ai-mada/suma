package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ti "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// Protocol fixtures exercise current HTTP/SQLite authority and atomic writes;
// their profiles are controlled data, not proof of an installed vendor/model.
type preparationFixture224 struct {
	f                     *fixture
	token, worker, runner string
	pool                  bool
	actor                 Actor
	profile               ToolProfile
	cap                   *tic.ToolMaintenanceCapability
	scan                  ti.Scan
}

func preparationFixtureFor224(t *testing.T, pooled bool, reason string) preparationFixture224 {
	t.Helper()
	h := preparationFixture224{pool: pooled}
	if pooled {
		p := newPoolMachineFixture(t)
		h.f, h.token, h.worker, h.runner = p.f, p.credential.Token, p.credential.Runner.WorkerID, p.credential.Runner.ID
	} else {
		h.f, h.token, h.profile = toolFixture(t)
		h.worker = h.profile.WorkerID
		h.actor = toolActorForTest(t, h.f, h.token)
	}
	h.cap = &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "controlled-instance224", InstalledDiscovery: &ti.Capability{Protocol: ti.Protocol, AdapterIDs: ti.AdapterIDs()}, ManagedPreparation: &tp.Capability{Protocol: tp.Protocol, AdapterIDs: tp.AdapterIDs(), GenerationSHA256: strings.Repeat("1", 64)}}
	if _, err := h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: h.cap}); err != nil {
		t.Fatal(err)
	}
	f, ctx := h.f, context.Background()
	scan, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, ti.CreateRequest{ProjectID: f.project, WorkerID: h.worker, AdapterIDs: []string{"cursor-cli"}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var claim ti.ClaimReply
	if pooled {
		claim, err = f.s.ClaimPoolToolInstallationScan(ctx, h.token, ti.PoolClaimRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: h.runner, ClaimID: newID()})
	} else {
		claim, err = f.s.ClaimToolInstallationScan(ctx, h.actor, f.project, ti.ClaimRequest{ProjectID: f.project, WorkerID: h.worker, ClaimID: newID()})
	}
	if err != nil || claim.Scan == nil || claim.Scan.ID != scan.ID {
		t.Fatal(claim, err)
	}
	result := installationResult205(*claim.Scan)
	profile := cursorACPProfile189()
	result.Items = []ti.Item{{AdapterID: "cursor-cli", Protocol: ti.AdapterProtocol("cursor-cli"), Installed: true, ReasonCode: reason, Version: profile.ToolVersion, ExecutableSHA256: profile.ExecutableSHA256}}
	if pooled {
		h.scan, err = f.s.CompletePoolToolInstallationScan(ctx, h.token, ti.PoolResultRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: h.runner, Scan: result.Scan, IdempotencyKey: result.IdempotencyKey, Outcome: result.Outcome, Items: result.Items, StopProof: result.StopProof})
	} else {
		h.scan, err = f.s.CompleteToolInstallationScan(ctx, h.actor, f.project, result)
	}
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h preparationFixture224) createRequest() tp.CreateRequest {
	return tp.CreateRequest{ProjectID: h.f.project, WorkerID: h.worker, InstallationScanID: h.scan.ID, ExpectedScanVersion: h.scan.Version, AdapterID: "cursor-cli", Configuration: tp.Configuration{Model: cursorACPProfile189().ACPExecution.Model, CredentialEnvRef: "CURSOR_API_KEY"}, ExpectedGenerationSHA256: h.cap.ManagedPreparation.GenerationSHA256, AcknowledgeConfigurationWrite: true, IdempotencyKey: newID()}
}

func (h preparationFixture224) claim(t *testing.T, key string) (tp.Claim, error) {
	t.Helper()
	ctx := context.Background()
	if h.pool {
		return h.f.s.ClaimPoolToolPreparation(ctx, h.token, tp.PoolClaimRequest{TenantID: h.f.user.TenantID, ProjectID: h.f.project, RunnerID: h.runner, ClaimID: key})
	}
	return h.f.s.ClaimToolPreparation(ctx, h.actor, h.f.project, tp.ClaimRequest{ProjectID: h.f.project, WorkerID: h.worker, ClaimID: key})
}

func (h preparationFixture224) result(t *testing.T, request tp.ResultRequest) (tp.Operation, error) {
	t.Helper()
	request.ProjectID, request.WorkerID = h.f.project, h.worker
	if h.pool {
		return h.f.s.CompletePoolToolPreparation(context.Background(), h.token, tp.PoolResultRequest{TenantID: h.f.user.TenantID, ProjectID: h.f.project, RunnerID: h.runner, Preparation: request.Preparation, SourceSHA256: request.SourceSHA256, IdempotencyKey: request.IdempotencyKey, Outcome: request.Outcome, FailureCode: request.FailureCode, Receipt: request.Receipt, StopProof: request.StopProof})
	}
	return h.f.s.CompleteToolPreparation(context.Background(), h.actor, h.f.project, request)
}

func (h preparationFixture224) report(t *testing.T, request ToolInventoryRequest) (ToolInventoryReceipt, error) {
	t.Helper()
	request.ProjectID, request.WorkerID = h.f.project, h.worker
	if h.pool {
		return h.f.s.ReportPoolToolInventory(context.Background(), h.token, PoolToolInventoryRequest{TenantID: h.f.user.TenantID, ProjectID: h.f.project, RunnerID: h.runner, OperationID: request.OperationID, Items: request.Items, Maintenance: request.Maintenance, Inspection: request.Inspection, Preparation: request.Preparation})
	}
	return h.f.s.ReportToolInventory(context.Background(), h.actor, h.f.project, request)
}

func preparationResultFor224(value tp.Operation) tp.ResultRequest {
	profile := cursorACPProfile189()
	profile.CommandID = tp.CommandID(value.ID)
	proof := &tp.StopProof{Protocol: tp.StopProtocol, Preparation: preparationReference(value), SourceSHA256: value.SourceSHA256, ReceiptSHA256: strings.Repeat("2", 64), ProbeReturned: true, AllNativeTreesStopped: true, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	return tp.ResultRequest{ProjectID: value.ProjectID, WorkerID: value.WorkerID, Preparation: preparationReference(value), SourceSHA256: value.SourceSHA256, IdempotencyKey: newID(), Outcome: "prepared", Receipt: &tp.GenerationReceipt{ParentGenerationSHA256: value.ExpectedGenerationSHA256, GenerationSHA256: strings.Repeat("3", 64), CommandID: profile.CommandID, ArtifactSHA256: strings.Repeat("4", 64), LocalReceiptSHA256: strings.Repeat("5", 64), Profile: profile, StopProof: proof}}
}

func assertPreparationInventoryRaw224(t *testing.T, h preparationFixture224, result tp.ResultRequest) {
	t.Helper()
	capability := *h.cap
	managed := *h.cap.ManagedPreparation
	managed.GenerationSHA256 = result.Receipt.GenerationSHA256
	capability.ManagedPreparation = &managed
	binding := &tp.InventoryBinding{Reference: result.Preparation, SourceSHA256: result.SourceSHA256, GenerationSHA256: result.Receipt.GenerationSHA256, LocalReceiptSHA256: result.Receipt.LocalReceiptSHA256}
	items := []tc.Observation{{Profile: result.Receipt.Profile, Available: false, Reason: "Controlled maintenance-only fixture"}}
	var request any = ToolInventoryRequest{ProjectID: h.f.project, WorkerID: h.worker, OperationID: newID(), Items: items, Maintenance: &capability, Preparation: binding}
	route := "/runner/tool-inventory"
	if h.pool {
		request = PoolToolInventoryRequest{TenantID: h.f.user.TenantID, ProjectID: h.f.project, RunnerID: h.runner, OperationID: newID(), Items: items, Maintenance: &capability, Preparation: binding}
		route = "/pool-runner/tool-inventory"
	}
	for _, variant := range []string{"top-alias", "missing-read-only", "profile-alias"} {
		var body map[string]any
		if json.Unmarshal([]byte(stringify(request)), &body) != nil {
			t.Fatal("inventory fixture encoding")
		}
		profile := body["items"].([]any)[0].(map[string]any)["profile"].(map[string]any)
		switch variant {
		case "top-alias":
			body["Preparation"] = body["preparation"]
			delete(body, "preparation")
		case "missing-read-only":
			delete(profile, "read_only")
		case "profile-alias":
			profile["Tool_Version"] = profile["tool_version"]
			delete(profile, "tool_version")
		}
		h.f.bearer("POST", route, h.token, body, 400)
	}
	// Failed decoding must not consume the original operation or partially
	// complete the preparation. The same canonical request remains usable.
	h.f.bearer("POST", route, h.token, request, 200)
}

func TestManagedPreparationInventoryAtomicAcceptanceAndReplay(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint(pooled), func(t *testing.T) {
			h := preparationFixtureFor224(t, pooled, "acp_configuration_required")
			f, ctx := h.f, context.Background()
			request := h.createRequest()
			created, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			again, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
			if err != nil || stringify(created) != stringify(again) {
				t.Fatal("original create changed", again, err)
			}
			if _, err = f.s.CreateToolInstallationScan(ctx, f.actor, f.project, ti.CreateRequest{ProjectID: f.project, WorkerID: h.worker, AdapterIDs: []string{"cursor-cli"}, IdempotencyKey: newID()}); err == nil {
				t.Fatal("scan bypassed preparation occupancy")
			}
			if !pooled {
				if _, err = f.s.CreateToolInspection(ctx, f.actor, f.project, h.profile.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: h.profile.Version, IdempotencyKey: newID()}); err == nil {
					t.Fatal("inspection bypassed preparation")
				}
			}
			key := newID()
			claim, err := h.claim(t, key)
			if err != nil || claim.Preparation == nil || claim.TenantID != f.user.TenantID || claim.Preparation.SourceSHA256 != created.SourceSHA256 {
				t.Fatal(claim, err)
			}
			claimAgain, err := h.claim(t, key)
			if err != nil || stringify(claimAgain) != stringify(claim) {
				t.Fatal("claim ACK recovery renewed facts", claimAgain, err)
			}
			other, err := h.claim(t, newID())
			if err != nil || other.Preparation != nil {
				t.Fatal("second nonce reacquired active operation", other, err)
			}
			result := preparationResultFor224(*claim.Preparation)
			wrong := result
			wrong.Receipt = &tp.GenerationReceipt{}
			*wrong.Receipt = *result.Receipt
			wrong.Receipt.Profile.ToolVersion = "another version"
			if _, err = h.result(t, wrong); err == nil {
				t.Fatal("wrong native pin accepted")
			}
			if pooled {
				capOff := *h.cap
				capOff.ManagedPreparation = nil
				if _, err = h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: &capOff}); err != nil {
					t.Fatal(err)
				}
			}
			head, err := h.result(t, result)
			if err != nil || head.State != "awaiting_inventory" || head.ResultCode != "prepared" || head.InventoryObservedAt != "" {
				t.Fatal(head, err)
			}
			capCopy := *h.cap
			managed := *h.cap.ManagedPreparation
			managed.GenerationSHA256 = result.Receipt.GenerationSHA256
			capCopy.ManagedPreparation = &managed
			if pooled {
				capCopy.ManagedPreparation = nil
			}
			binding := &tp.InventoryBinding{Reference: result.Preparation, SourceSHA256: result.SourceSHA256, GenerationSHA256: result.Receipt.GenerationSHA256, LocalReceiptSHA256: result.Receipt.LocalReceiptSHA256}
			inventory := ToolInventoryRequest{OperationID: newID(), Preparation: binding, Maintenance: &capCopy, Items: []tc.Observation{{Profile: result.Receipt.Profile, Available: false, Reason: "Controlled maintenance-only fixture"}}}
			bare := inventory
			bare.Preparation = nil
			if _, err = h.report(t, bare); err == nil {
				t.Fatal("bare periodic report bypassed atomic acceptance")
			}
			wrongInventory := inventory
			wrongInventory.Items = []tc.Observation{{Profile: result.Receipt.Profile, Available: false, Reason: "Controlled fixture"}}
			wrongInventory.Items[0].Profile.ProfileSHA256 = strings.Repeat("6", 64)
			if _, err = h.report(t, wrongInventory); err == nil {
				t.Fatal("partial profile pin accepted")
			}
			var before string
			if err = f.s.DB.QueryRow(`SELECT observed_at FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, h.worker).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.DB.Exec(`CREATE TRIGGER owned_preparation_ack_failure BEFORE INSERT ON suma_operations WHEN NEW.response LIKE '%"preparation_state":"completed"%' BEGIN SELECT RAISE(ABORT,'owned ACK failure'); END`); err != nil {
				t.Fatal(err)
			}
			if _, err = h.report(t, inventory); err == nil {
				t.Fatal("inventory committed without atomic receipt")
			}
			persisted, err := f.s.GetToolPreparation(ctx, f.actor, f.project, head.ID)
			if err != nil || persisted.State != "awaiting_inventory" {
				t.Fatal("partial preparation completion", persisted, err)
			}
			var after string
			if err = f.s.DB.QueryRow(`SELECT observed_at FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, h.worker).Scan(&after); err != nil || before != after {
				t.Fatal("failed ACK changed current inventory", before, after, err)
			}
			if _, err = f.s.DB.Exec(`DROP TRIGGER owned_preparation_ack_failure`); err != nil {
				t.Fatal(err)
			}
			ack, err := h.report(t, inventory)
			if err != nil || ack.PreparationState != "completed" || ack.Preparation == nil || *ack.Preparation != *binding {
				t.Fatal(ack, err)
			}
			done, err := f.s.GetToolPreparation(ctx, f.actor, f.project, head.ID)
			if err != nil || done.State != "completed" || done.ResultCode != "completed" || done.InventoryObservedAt != ack.ObservedAt {
				t.Fatal(done, err)
			}
			if _, err = f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at='2000-01-01T00:00:00Z' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, h.worker); err != nil {
				t.Fatal(err)
			}
			replayed, err := h.report(t, inventory)
			if err != nil || stringify(replayed) != stringify(ack) {
				t.Fatal("inventory replay refreshed TTL", replayed, err)
			}
			capCopy.ManagedPreparation = nil
			if _, err = h.report(t, ToolInventoryRequest{OperationID: newID(), Items: inventory.Items, Maintenance: &capCopy}); err != nil {
				t.Fatal(err)
			}
			current, err := h.claim(t, key)
			if err != nil || current.Preparation == nil || current.Preparation.State != "completed" || current.Preparation.InventoryObservedAt != ack.ObservedAt {
				t.Fatal("old nonce history required new opt-in", current, err)
			}
			if _, err = f.s.DB.Exec(`DELETE FROM suma_sessions WHERE token_hash=?`, f.actor.credential.digest); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.GetToolPreparation(ctx, f.actor, f.project, head.ID); err == nil {
				t.Fatal("revoked browser read original receipt")
			}
		})
	}
}

func TestManagedPreparationUnknownSourceAndExactScope(t *testing.T) {
	h := preparationFixtureFor224(t, false, "version_unsupported")
	f, ctx := h.f, context.Background()
	if _, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, h.createRequest()); err == nil {
		t.Fatal("unsupported version admitted by installed=true")
	}
	// A separately completed, exact supported scan is required; no rescan is
	// invented by the preparation endpoint.
	h = preparationFixtureFor224(t, false, "acp_configuration_required")
	f = h.f
	request := h.createRequest()
	head, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := h.claim(t, newID())
	if err != nil || claim.Preparation == nil {
		t.Fatal(claim, err)
	}
	unknown := tp.ResultRequest{Preparation: preparationReference(*claim.Preparation), SourceSHA256: head.SourceSHA256, IdempotencyKey: newID(), Outcome: "unknown", FailureCode: "worker_restarted"}
	head, err = h.result(t, unknown)
	if err != nil || head.State != "unknown" {
		t.Fatal(head, err)
	}
	request.IdempotencyKey = newID()
	if _, err = f.s.CreateToolPreparation(ctx, f.actor, f.project, request); err == nil {
		t.Fatal("unknown silently freed maintenance")
	}
	capOff := *h.cap
	capOff.ManagedPreparation = nil
	if _, err = h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: &capOff}); err != nil {
		t.Fatal(err)
	}
	closed := unknown
	closed.Outcome = "closed_unknown"
	closed.IdempotencyKey = newID()
	closed.StopProof = preparationResultFor224(*claim.Preparation).Receipt.StopProof
	head, err = h.result(t, closed)
	if err != nil || head.State != "closed_unknown" || head.ResultCode != "worker_restarted" {
		t.Fatal(head, err)
	}
	request.IdempotencyKey = newID()
	if _, err = f.s.CreateToolPreparation(ctx, f.actor, f.project, request); err == nil {
		t.Fatal("historical closure reconstructed opt-in")
	}
	if _, err = h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: h.cap}); err != nil {
		t.Fatal(err)
	}
	request.IdempotencyKey = newID()
	next, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListToolPreparations(ctx, f.actor, f.project, ToolPreparationListQuery{WorkerID: h.worker, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	if _, err = f.s.ListToolPreparations(ctx, f.actor, f.project, ToolPreparationListQuery{WorkerID: "other-worker", Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("history cursor moved to different worker")
	}
	capCopy := *h.cap
	managed := *h.cap.ManagedPreparation
	managed.GenerationSHA256 = strings.Repeat("7", 64)
	capCopy.ManagedPreparation = &managed
	if _, err = h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: &capCopy}); err != nil {
		t.Fatal(err)
	}
	claim, err = h.claim(t, newID())
	if err != nil || claim.Preparation != nil {
		t.Fatal("new generation drift acquired old source", claim, err)
	}
	head, err = f.s.GetToolPreparation(ctx, f.actor, f.project, next.ID)
	if err != nil || head.State != "superseded" || !head.SourceChanged {
		t.Fatal(head, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE worker_id=?`, h.worker); err != nil {
		t.Fatal(err)
	}
	if _, err = h.claim(t, newID()); err == nil {
		t.Fatal("revoked machine acquired canonical scope")
	}
}

func TestManagedPreparationHTTPPresenceAndInitialSchema(t *testing.T) {
	h := preparationFixtureFor224(t, false, "acp_configuration_required")
	f, ctx := h.f, context.Background()
	request := h.createRequest()
	var raw map[string]any
	if json.Unmarshal([]byte(stringify(request)), &raw) != nil {
		t.Fatal("fixture encoding")
	}
	delete(raw, "acknowledge_configuration_write")
	f.call("POST", "/tool-preparations", raw, 400)
	raw["acknowledge_configuration_write"] = true
	raw["configuration"].(map[string]any)["executable"] = "C:/arbitrary.exe"
	f.call("POST", "/tool-preparations", raw, 400)
	delete(raw["configuration"].(map[string]any), "executable")
	raw["expected_scan_version"] = json.Number("1e0")
	f.call("POST", "/tool-preparations", raw, 400)
	raw["expected_scan_version"] = h.scan.Version
	created := f.call("POST", "/tool-preparations", raw, 201)
	id := created["id"].(string)
	f.call("GET", "/tool-preparations/"+id+"?project_id="+f.project+"&unknown=1", nil, 400)
	claim, err := h.claim(t, newID())
	if err != nil || claim.Preparation == nil {
		t.Fatal(claim, err)
	}
	result := preparationResultFor224(*claim.Preparation)
	raw = nil
	if json.Unmarshal([]byte(stringify(result)), &raw) != nil {
		t.Fatal("fixture encoding")
	}
	proof := raw["receipt"].(map[string]any)["stop_proof"].(map[string]any)
	delete(proof, "native_process_count")
	f.bearer("POST", "/runner/tool-preparations/results", h.token, raw, 400)
	proof["native_process_count"] = 0
	raw["receipt"].(map[string]any)["profile"].(map[string]any)["terminal_execution"] = nil
	f.bearer("POST", "/runner/tool-preparations/results", h.token, raw, 400)
	delete(raw["receipt"].(map[string]any)["profile"].(map[string]any), "terminal_execution")
	profileRaw := raw["receipt"].(map[string]any)["profile"].(map[string]any)
	profileRaw["Tool_Version"] = profileRaw["tool_version"]
	delete(profileRaw, "tool_version")
	f.bearer("POST", "/runner/tool-preparations/results", h.token, raw, 400)
	profileRaw["tool_version"] = profileRaw["Tool_Version"]
	delete(profileRaw, "Tool_Version")
	delete(profileRaw, "read_only")
	f.bearer("POST", "/runner/tool-preparations/results", h.token, raw, 400)
	profileRaw["read_only"] = false
	raw["preparation"].(map[string]any)["fence"] = json.Number("1e0")
	f.bearer("POST", "/runner/tool-preparations/results", h.token, raw, 400)
	raw["preparation"].(map[string]any)["fence"] = claim.Preparation.Fence
	f.bearer("POST", "/runner/tool-preparations/results", h.token, raw, 200)
	f.bearer("POST", "/runner/tool-inventory", h.token, map[string]any{"project_id": f.project, "worker_id": h.worker, "operation_id": newID(), "items": []any{}, "preparation": nil}, 400)
	assertPreparationInventoryRaw224(t, h, result)
	if _, err = f.s.DB.Exec(`ALTER TABLE suma_tool_preparations ADD COLUMN unsupported TEXT`); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Initialize(ctx); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("unknown preparation layout was accepted/reset", err)
	}
	var saved string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_preparations WHERE id=?`, id).Scan(&saved); err != nil || !strings.Contains(saved, id) {
		t.Fatal("schema refusal changed unique evidence", err)
	}
	t.Run("pool-raw-lexemes-and-profile-presence", func(t *testing.T) {
		p := preparationFixtureFor224(t, true, "acp_configuration_required")
		if _, err := p.f.s.CreateToolPreparation(ctx, p.f.actor, p.f.project, p.createRequest()); err != nil {
			t.Fatal(err)
		}
		claimed, err := p.claim(t, newID())
		if err != nil || claimed.Preparation == nil {
			t.Fatal(claimed, err)
		}
		ordinary := preparationResultFor224(*claimed.Preparation)
		request := tp.PoolResultRequest{TenantID: p.f.user.TenantID, ProjectID: p.f.project, RunnerID: p.runner, Preparation: ordinary.Preparation, SourceSHA256: ordinary.SourceSHA256, IdempotencyKey: ordinary.IdempotencyKey, Outcome: ordinary.Outcome, FailureCode: ordinary.FailureCode, Receipt: ordinary.Receipt}
		var fields map[string]any
		if json.Unmarshal([]byte(stringify(request)), &fields) != nil {
			t.Fatal("fixture encoding")
		}
		fields["preparation"].(map[string]any)["fence"] = json.Number("1e0")
		p.f.bearer("POST", "/pool-runner/tool-preparations/results", p.token, fields, 400)
		fields["preparation"].(map[string]any)["fence"] = ordinary.Preparation.Fence
		profile := fields["receipt"].(map[string]any)["profile"].(map[string]any)
		profile["Tool_Version"] = profile["tool_version"]
		delete(profile, "tool_version")
		p.f.bearer("POST", "/pool-runner/tool-preparations/results", p.token, fields, 400)
		profile["tool_version"] = profile["Tool_Version"]
		delete(profile, "Tool_Version")
		delete(profile, "read_only")
		p.f.bearer("POST", "/pool-runner/tool-preparations/results", p.token, fields, 400)
		profile["read_only"] = false
		p.f.bearer("POST", "/pool-runner/tool-preparations/results", p.token, fields, 200)
		assertPreparationInventoryRaw224(t, p, ordinary)
	})
}
