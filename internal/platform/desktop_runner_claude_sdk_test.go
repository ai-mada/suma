package platform

import (
	"strings"
	"testing"
)

func TestDesktopRunnerClaudeSDKRegistrationKeepsOriginalAuthorityAndReceipt(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	body := desktopRunnerRegistration(f, local, strings.Repeat("e8", 32))
	body["adapter_id"] = "claude-code-sdk"
	original := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	replayed := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	if stringify(original) != stringify(replayed) || original["adapter_id"] != "claude-code-sdk" || domainACount(t, f.s, "suma_release_workers") != 1 || domainACount(t, f.s, "suma_tool_profiles") != 0 {
		t.Fatal("Claude SDK registration changed original issuance or created a tool profile")
	}
	body["adapter_id"] = "claude-code-cli"
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 409)
	body["adapter_id"] = "claude-code-sdk"
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 403)
}
