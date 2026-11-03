package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

// Exercise the public normal/Pool result envelopes against real scoped rows;
// controlled diagnostics are observations, never vendor or execution proof.
func installationDiagnosticHTTP226(t *testing.T, pooled bool) {
	t.Helper()
	ctx := context.Background()
	var f *fixture
	var token, worker, runner string
	var machine Actor
	if pooled {
		p := newPoolMachineFixture(t)
		f, token, worker, runner = p.f, p.credential.Token, p.credential.Runner.WorkerID, p.credential.Runner.ID
	} else {
		var profile ToolProfile
		f, token, profile = toolFixture(t)
		worker, machine = profile.WorkerID, toolActorForTest(t, f, token)
	}
	installationMaintenance205(t, f, token, worker, pooled, runner)
	scan, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, tii.CreateRequest{ProjectID: f.project, WorkerID: worker, AdapterIDs: []string{"codebuddy-cli"}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var claim tii.ClaimReply
	if pooled {
		claim, err = f.s.ClaimPoolToolInstallationScan(ctx, token, tii.PoolClaimRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: runner, ClaimID: newID()})
	} else {
		claim, err = f.s.ClaimToolInstallationScan(ctx, machine, f.project, tii.ClaimRequest{ProjectID: f.project, WorkerID: worker, ClaimID: newID()})
	}
	if err != nil || claim.Scan == nil || claim.Scan.ID != scan.ID {
		t.Fatal(claim, err)
	}
	result := installationResult205(*claim.Scan)
	result.Items = []tii.Item{{AdapterID: "codebuddy-cli", Protocol: tii.AdapterProtocol("codebuddy-cli"), Installed: true, ReasonCode: "probe_failed"}}
	var body any = result
	endpoint := "/runner/tool-installation-scans/results"
	if pooled {
		body = tii.PoolResultRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: runner, Scan: result.Scan, IdempotencyKey: result.IdempotencyKey, Outcome: result.Outcome, Items: result.Items, StopProof: result.StopProof}
		endpoint = "/pool-runner/tool-installation-scans/results"
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	makeBody := func() map[string]any {
		var value map[string]any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatal(err)
		}
		value["items"].([]any)[0].(map[string]any)["diagnostic"] = map[string]any{"stage": "help", "code": "help_contract_mismatch", "next_action": "guide"}
		return value
	}
	const private = "PRIVATE_NATIVE_ERROR /private/account/auth.json"
	variants := []struct {
		name string
		edit func(map[string]any, map[string]any)
	}{
		{"null", func(item, diagnostic map[string]any) { item["diagnostic"] = nil }},
		{"alias", func(item, diagnostic map[string]any) { delete(item, "diagnostic"); item["Diagnostic"] = diagnostic }},
		{"missing-stage", func(item, diagnostic map[string]any) { delete(diagnostic, "stage") }},
		{"missing-code", func(item, diagnostic map[string]any) { delete(diagnostic, "code") }},
		{"missing-action", func(item, diagnostic map[string]any) { delete(diagnostic, "next_action") }},
		{"nested-alias", func(item, diagnostic map[string]any) {
			delete(diagnostic, "code")
			diagnostic["Code"] = "help_contract_mismatch"
		}},
		{"extra-raw-error", func(item, diagnostic map[string]any) { diagnostic["raw_error"] = private }},
		{"unknown-stage", func(item, diagnostic map[string]any) { diagnostic["stage"] = private }},
		{"unknown-code", func(item, diagnostic map[string]any) { diagnostic["code"] = private }},
		{"wrong-action", func(item, diagnostic map[string]any) { diagnostic["next_action"] = "scan" }},
		{"null-code", func(item, diagnostic map[string]any) { diagnostic["code"] = nil }},
		{"non-string", func(item, diagnostic map[string]any) { diagnostic["stage"] = false }},
		{"healthy-conflict", func(item, diagnostic map[string]any) {
			item["launch_supported"] = true
			item["reason_code"] = "workspace_required"
			item["version"] = "controlled"
		}},
		{"not-installed-conflict", func(item, diagnostic map[string]any) {
			item["installed"] = false
			item["reason_code"] = "not_installed"
		}},
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			value := makeBody()
			item := value["items"].([]any)[0].(map[string]any)
			variant.edit(item, item["diagnostic"].(map[string]any))
			failure := f.bearer("POST", endpoint, token, value, 400)
			if strings.Contains(stringify(failure), private) {
				t.Fatal("private native details escaped the fixed error envelope")
			}
		})
	}
	current, err := f.s.GetToolInstallationScan(ctx, f.actor, f.project, scan.ID)
	if err != nil || current.State != "claimed" || current.Result != nil {
		t.Fatal("rejected diagnostics changed original scan", current, err)
	}
	valid := makeBody()
	completed := f.bearer("POST", endpoint, token, valid, 200)
	replayed := f.bearer("POST", endpoint, token, valid, 200)
	if stringify(completed) != stringify(replayed) {
		t.Fatal("diagnostic receipt replay changed the accepted observation")
	}
	history := f.call("GET", "/tool-installation-scans/"+scan.ID+"?project_id="+f.project, nil, 200)
	var stored tii.Scan
	if json.Unmarshal([]byte(stringify(history)), &stored) != nil || stored.Result == nil || len(stored.Result.Items) != 1 {
		t.Fatal("diagnostic history unavailable")
	}
	row := stored.Result.Items[0]
	if row.Diagnostic == nil || row.Diagnostic.Stage != "help" || row.Diagnostic.Code != "help_contract_mismatch" || row.Diagnostic.NextAction != "guide" || strings.Contains(stringify(history), private) {
		t.Fatal("history lost the fixed diagnostic or exposed raw details")
	}
	// A valid old result never bypasses current machine authentication.
	if pooled {
		_, err = f.s.DB.Exec(`UPDATE suma_execution_pool_runners SET status='revoked' WHERE id=?`, runner)
	} else {
		_, err = f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, worker)
	}
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", endpoint, token, valid, 403)
	if _, err = f.s.DB.Exec(`DELETE FROM suma_sessions WHERE token_hash=?`, f.actor.credential.digest); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetToolInstallationScan(ctx, f.actor, f.project, scan.ID); err == nil {
		t.Fatal("revoked browser retained diagnostic history authority")
	}
}

func TestInstallationProbeDiagnosticNormalHTTPBoundariesAndReplay(t *testing.T) {
	installationDiagnosticHTTP226(t, false)
}

func TestInstallationProbeDiagnosticPoolHTTPBoundariesAndReplay(t *testing.T) {
	installationDiagnosticHTTP226(t, true)
}
