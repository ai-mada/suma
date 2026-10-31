package platform

import (
	"strings"
	"testing"
)

// Native discovery grants no execution authority. Qoder uses the same signed,
// current-project registration and original receipt as other managed tools.
func TestDesktopRunnerACPRegistrationPreservesOriginalAuthority(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	body := desktopRunnerRegistration(f, local, strings.Repeat("ac", 32))
	body["adapter_id"] = "qoder-acp"
	original := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	worker := original["worker"].(map[string]any)
	if worker["status"] != "active" || len(worker["profiles"].([]any)) != 0 || original["adapter_id"] != "qoder-acp" {
		t.Fatal("registration enabled a tool profile or changed the adapter")
	}
	replayed := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	if stringify(original) != stringify(replayed) || domainACount(t, f.s, "suma_release_workers") != 1 {
		t.Fatal("original registration was replaced or duplicated")
	}
	body["adapter_id"] = "codex-jsonl"
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 409)
	body["adapter_id"] = "qoder-acp"
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 403)
}
