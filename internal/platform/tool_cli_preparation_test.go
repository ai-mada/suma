package platform

import (
	"context"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ti "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// These are controlled protocol data, not an installation, login or model
// execution. Real native-file and dependency checks remain Runner-owned.
func managedCLIFixture225(t *testing.T, adapter string, pooled bool) (preparationFixture224, tp.CreateRequest) {
	t.Helper()
	h := preparationFixture224{pool: pooled}
	if pooled {
		pool := newPoolMachineFixture(t)
		h.f, h.token, h.worker, h.runner = pool.f, pool.credential.Token, pool.credential.Runner.WorkerID, pool.credential.Runner.ID
	} else {
		h.f, h.token, h.profile = toolFixture(t)
		h.worker, h.actor = h.profile.WorkerID, toolActorForTest(t, h.f, h.token)
	}
	h.cap = &tic.ToolMaintenanceCapability{
		Protocol: tic.MaintenanceProtocol, InstanceID: "controlled-cli-instance225",
		InstalledDiscovery: &ti.Capability{Protocol: ti.Protocol, AdapterIDs: ti.AdapterIDs()},
		ManagedPreparation: &tp.Capability{Protocol: tp.Protocol, AdapterIDs: tp.AdapterIDs(), GenerationSHA256: strings.Repeat("1", 64)},
	}
	if _, err := h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: h.cap}); err != nil {
		t.Fatal(err)
	}
	f, ctx := h.f, context.Background()
	scan, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, ti.CreateRequest{ProjectID: f.project, WorkerID: h.worker, AdapterIDs: []string{adapter}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var claimed ti.ClaimReply
	if pooled {
		claimed, err = f.s.ClaimPoolToolInstallationScan(ctx, h.token, ti.PoolClaimRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: h.runner, ClaimID: newID()})
	} else {
		claimed, err = f.s.ClaimToolInstallationScan(ctx, h.actor, f.project, ti.ClaimRequest{ProjectID: f.project, WorkerID: h.worker, ClaimID: newID()})
	}
	if err != nil || claimed.Scan == nil || claimed.Scan.ID != scan.ID {
		t.Fatal(claimed, err)
	}
	result := installationResult205(*claimed.Scan)
	version := "controlled-cli-version225"
	configuration := tp.Configuration{}
	if adapter == "claude-code-cli" {
		version = tc.ClaudeCLIVersion
		configuration.Model = "claude-sonnet-4-6"
	}
	item := ti.Item{AdapterID: adapter, Protocol: ti.AdapterProtocol(adapter), Installed: true, LaunchSupported: true, ReasonCode: "workspace_required", Version: version, ExecutableSHA256: strings.Repeat("b", 64)}
	if adapter == "codex-app-server" {
		item.LaunchSupported, item.ReasonCode = false, "configuration_required"
	}
	result.Items = []ti.Item{item}
	if pooled {
		h.scan, err = f.s.CompletePoolToolInstallationScan(ctx, h.token, ti.PoolResultRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: h.runner, Scan: result.Scan, IdempotencyKey: result.IdempotencyKey, Outcome: result.Outcome, Items: result.Items, StopProof: result.StopProof})
	} else {
		h.scan, err = f.s.CompleteToolInstallationScan(ctx, h.actor, f.project, result)
	}
	if err != nil {
		t.Fatal(err)
	}
	return h, tp.CreateRequest{ProjectID: f.project, WorkerID: h.worker, InstallationScanID: h.scan.ID, ExpectedScanVersion: h.scan.Version, AdapterID: adapter, Configuration: configuration, ExpectedGenerationSHA256: h.cap.ManagedPreparation.GenerationSHA256, AcknowledgeConfigurationWrite: true, IdempotencyKey: newID()}
}

func managedCLIResult225(value tp.Operation) tp.ResultRequest {
	result := preparationResultFor224(value)
	adapter := tp.AdapterRuntime(value.AdapterID)
	result.Receipt.Profile = tc.Profile{CommandID: tp.CommandID(value.ID), Adapter: adapter, Protocol: ti.AdapterProtocol(value.AdapterID), ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: value.InstallationItem.ExecutableSHA256, ToolVersion: value.InstallationItem.Version}
	if value.AdapterID == "claude-code-cli" {
		result.Receipt.Profile.ClaudeExecution = &tc.ClaudeExecutionCapability{Model: value.Configuration.Model, ModelSource: "administrator_cli_argument", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only", AuthStrategy: "existing_cli_login"}
	}
	return result
}

func managedCLIInventory225(h preparationFixture224, result tp.ResultRequest) ToolInventoryRequest {
	capability, managed := *h.cap, *h.cap.ManagedPreparation
	managed.GenerationSHA256 = result.Receipt.GenerationSHA256
	capability.ManagedPreparation = &managed
	return ToolInventoryRequest{ProjectID: h.f.project, WorkerID: h.worker, OperationID: newID(), Items: []tc.Observation{{Profile: result.Receipt.Profile, Available: false, Reason: "Controlled maintenance-only CLI fixture"}}, Maintenance: &capability, Preparation: &tp.InventoryBinding{Reference: result.Preparation, SourceSHA256: result.SourceSHA256, GenerationSHA256: result.Receipt.GenerationSHA256, LocalReceiptSHA256: result.Receipt.LocalReceiptSHA256}}
}

func TestManagedCLIProfilesKeepNativeIdentityAndFullInventory(t *testing.T) {
	for _, adapter := range []string{"codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli"} {
		t.Run(adapter, func(t *testing.T) {
			h, request := managedCLIFixture225(t, adapter, false)
			f, ctx := h.f, context.Background()
			badConfig := request
			badConfig.Configuration.CredentialEnvRef = "UNEXPECTED_CLI_KEY"
			if _, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, badConfig); err == nil {
				t.Fatal("CLI preparation silently ignored a credential override")
			}
			if adapter != "claude-code-cli" {
				badConfig = request
				badConfig.Configuration.Model = "not-an-explicit-generic-option"
				if _, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, badConfig); err == nil {
					t.Fatal("generic CLI preparation silently ignored a model override")
				}
			}
			created, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := h.claim(t, newID())
			if err != nil || claimed.Preparation == nil || claimed.Preparation.ID != created.ID {
				t.Fatal(claimed, err)
			}
			result := managedCLIResult225(*claimed.Preparation)
			for _, variant := range []string{"different-provider", "native-bytes", "native-version", "extra-capability"} {
				wrong := result
				receipt := *result.Receipt
				wrong.Receipt = &receipt
				switch variant {
				case "different-provider":
					if adapter == "claude-code-cli" {
						capability := *receipt.Profile.ClaudeExecution
						capability.Model = "claude-opus-4-6"
						receipt.Profile.ClaudeExecution = &capability
					} else if adapter == "codex-jsonl" {
						receipt.Profile.Adapter, receipt.Profile.Protocol = "codebuddy", "codebuddy-jsonl-v1"
					} else {
						receipt.Profile.Adapter, receipt.Profile.Protocol = "codex", "codex-jsonl-v1"
					}
				case "native-bytes":
					receipt.Profile.ExecutableSHA256 = strings.Repeat("e", 64)
				case "native-version":
					receipt.Profile.ToolVersion = "different-controlled-version225"
				case "extra-capability":
					receipt.Profile.WorkspaceToolsVersion = "attempt-workspace-v1"
				}
				if _, err = h.result(t, wrong); err == nil {
					t.Fatalf("%s receipt replaced the selected CLI facts", variant)
				}
			}
			head, err := h.result(t, result)
			if err != nil || head.State != "awaiting_inventory" || head.InventoryObservedAt != "" {
				t.Fatal(head, err)
			}
			inventory := managedCLIInventory225(h, result)
			wrong := inventory
			wrong.Items = append([]tc.Observation{}, inventory.Items...)
			wrong.Items[0].Profile.ProfileSHA256 = strings.Repeat("e", 64)
			if _, err = h.report(t, wrong); err == nil {
				t.Fatal("partial profile match completed the original preparation")
			}
			// Exercise the existing strict HTTP inventory envelope. The node is
			// not permitted to execute merely because its full profile is known.
			f.bearer("POST", "/runner/tool-inventory", h.token, inventory, 200)
			head, err = f.s.GetToolPreparation(ctx, f.actor, f.project, head.ID)
			if err != nil || head.State != "completed" || head.Receipt.Profile.Adapter != result.Receipt.Profile.Adapter || head.InventoryOperationID != inventory.OperationID {
				t.Fatal(head, err)
			}
		})
	}
}

func TestManagedCLIPoolInventoryReceiptRollbackAndPausedRegistration(t *testing.T) {
	h, request := managedCLIFixture225(t, "codebuddy-cli", true)
	f, ctx := h.f, context.Background()
	created, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := h.claim(t, newID())
	if err != nil || claim.Preparation == nil || claim.Preparation.ID != created.ID {
		t.Fatal(claim, err)
	}
	result := managedCLIResult225(*claim.Preparation)
	if _, err = h.result(t, result); err != nil {
		t.Fatal(err)
	}
	inventory := managedCLIInventory225(h, result)
	var before string
	if err = f.s.DB.QueryRow(`SELECT observed_at FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, h.worker).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`CREATE TRIGGER owned_cli_ack225 BEFORE INSERT ON suma_operations WHEN NEW.response LIKE '%"preparation_state":"completed"%' BEGIN SELECT RAISE(ABORT,'owned CLI ACK failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = h.report(t, inventory); err == nil {
		t.Fatal("failed inventory ACK committed CLI completion")
	}
	head, err := f.s.GetToolPreparation(ctx, f.actor, f.project, created.ID)
	if err != nil || head.State != "awaiting_inventory" {
		t.Fatal("partial CLI completion survived rollback", head, err)
	}
	var after string
	if err = f.s.DB.QueryRow(`SELECT observed_at FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, h.worker).Scan(&after); err != nil || before != after {
		t.Fatal("failed ACK changed CLI inventory", before, after, err)
	}
	if _, err = f.s.DB.Exec(`DROP TRIGGER owned_cli_ack225`); err != nil {
		t.Fatal(err)
	}
	ack, err := h.report(t, inventory)
	if err != nil || ack.PreparationState != "completed" || ack.Preparation == nil || *ack.Preparation != *inventory.Preparation {
		t.Fatal(ack, err)
	}
	replayed, err := h.report(t, inventory)
	if err != nil || stringify(replayed) != stringify(ack) {
		t.Fatal("original CLI ACK changed on replay", replayed, err)
	}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: h.worker, Name: "Controlled prepared CLI225", Profile: result.Receipt.Profile, IdempotencyKey: newID()})
	if err != nil || profile.State != "paused" || profile.Available {
		t.Fatal("preparation implicitly enabled or executed CLI", profile, err)
	}
}

func TestManagedCodexAppServerPreparationRequiresScopeOptInAndCatalogue(t *testing.T) {
	for _, scope := range []string{"ordinary", "pool"} {
		t.Run(scope, func(t *testing.T) {
			h, request := managedCLIFixture225(t, "codex-app-server", scope == "pool")
			f, ctx := h.f, context.Background()

			withoutCodex, cap := *h.cap, *h.cap.ManagedPreparation
			cap.AdapterIDs = []string{"codex-jsonl"}
			withoutCodex.ManagedPreparation = &cap
			if _, err := h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: &withoutCodex}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request); err == nil {
				t.Fatal("Codex JSONL opt-in granted a Codex app-server preparation")
			}
			if _, err := h.report(t, ToolInventoryRequest{OperationID: newID(), Items: []tc.Observation{}, Maintenance: h.cap}); err != nil {
				t.Fatal(err)
			}
			created, err := f.s.CreateToolPreparation(ctx, f.actor, f.project, request)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := h.claim(t, newID())
			if err != nil || claim.Preparation == nil || claim.Preparation.ID != created.ID {
				t.Fatal(claim, err)
			}
			result := managedCLIResult225(*claim.Preparation)
			result.Receipt.Profile.UserInputProtocol = cc.UserInputProtocol
			head, err := h.result(t, result)
			if err != nil || head.State != "awaiting_inventory" {
				t.Fatal(head, err)
			}
			inventory := managedCLIInventory225(h, result)
			if _, err = h.report(t, inventory); err == nil {
				t.Fatal("an unavailable Codex profile without a model catalogue completed preparation")
			}
			head, err = f.s.GetToolPreparation(ctx, f.actor, f.project, created.ID)
			if err != nil || head.State != "awaiting_inventory" {
				t.Fatal("missing catalogue changed accepted-generation state", head, err)
			}
			catalogue, err := cc.NewCatalog(strings.Repeat("c", 64), []cc.ModelOption{{Model: "controlled-model", ReasoningEfforts: []string{"medium"}, DefaultReasoningEffort: "medium"}})
			if err != nil {
				t.Fatal(err)
			}
			inventory.Items[0] = tc.Observation{Profile: result.Receipt.Profile, Available: true, CodexCatalog: &catalogue}
			ack, err := h.report(t, inventory)
			if err != nil || ack.PreparationState != "completed" {
				t.Fatal(ack, err)
			}
			replay, err := h.report(t, inventory)
			if err != nil || stringify(replay) != stringify(ack) {
				t.Fatal("Codex preparation inventory did not replay the original receipt", replay, err)
			}
			profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: h.worker, Name: "Controlled prepared Codex App Server", Profile: result.Receipt.Profile, IdempotencyKey: newID()})
			if err != nil || profile.State != "paused" || profile.Available {
				t.Fatal("preparation implicitly enabled Codex app-server", profile, err)
			}
		})
	}
}
