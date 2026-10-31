package platform

import (
	"strings"
	"testing"
)

func TestDesktopRunnerOpenCodeRegistrationPreservesProjectAuthorityAndReceipt(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	body := desktopRunnerRegistration(f, local, strings.Repeat("e7", 32))
	body["adapter_id"] = "opencode"
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 400)
	body["adapter_id"] = "opencode-sdk"
	original := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	worker, ok := original["worker"].(map[string]any)
	if !ok || original["adapter_id"] != "opencode-sdk" || original["operation_id"] != body["idempotency_key"] || worker["worker_id"] != body["worker_id"] || worker["status"] != "active" || len(worker["profiles"].([]any)) != 0 || domainACount(t, f.s, "suma_tool_profiles") != 0 {
		t.Fatal("SDK registration changed identity or enabled a tool configuration")
	}
	replayed := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	if stringify(original) != stringify(replayed) || domainACount(t, f.s, "suma_release_workers") != 1 || domainACount(t, f.s, "suma_credential_grants") != 1 {
		t.Fatal("SDK registration did not recover its original issuance body/key")
	}
	body["adapter_id"] = "codex-app-server"
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 409)
	body["adapter_id"] = "opencode-sdk"
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 403)
}
