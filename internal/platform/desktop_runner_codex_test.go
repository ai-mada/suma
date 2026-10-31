package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDesktopRunnerCodexRegistrationRecoversOriginalAndRechecksAuthority(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	token := strings.Repeat("c5", 32)
	body := desktopRunnerRegistration(f, local, token)
	body["adapter_id"] = "codex-app-server"
	original := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	worker, ok := original["worker"].(map[string]any)
	if !ok || original["adapter_id"] != "codex-app-server" || original["operation_id"] != body["idempotency_key"] || worker["worker_id"] != body["worker_id"] || worker["status"] != "active" || len(worker["profiles"].([]any)) != 0 || domainACount(t, f.s, "suma_tool_profiles") != 0 || strings.Contains(stringify(original), token) {
		t.Fatal("native registration changed its identity, exposed the token or enabled a profile")
	}
	actor, err := f.s.AuthenticateActor(context.Background(), token)
	if err != nil || actor.kind != actorProjectRunner || actor.credential.projectID != f.project {
		t.Fatalf("native registration did not issue the original project runner credential: %v", err)
	}
	replayed := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	if stringify(original) != stringify(replayed) || domainACount(t, f.s, "suma_release_workers") != 1 || domainACount(t, f.s, "suma_credential_grants") != 1 {
		t.Fatal("same native registration body/key did not recover the exact original receipt")
	}
	body["adapter_id"] = "codex-jsonl"
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 409)
	body["adapter_id"] = "codex-app-server"
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 403)
}

func TestDesktopRunnerCodexRegistrationKeepsSignedScopeAndPrivateBoundary(t *testing.T) {
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	for _, check := range []struct {
		name   string
		mutate func(map[string]any)
		status int
	}{
		{"current_principal", func(body map[string]any) { body["user_id"] = newID() }, 403},
		{"original_workspace", func(body map[string]any) { body["expected_binding_sha256"] = strings.Repeat("0", 64) }, 409},
		{"original_instance", func(body map[string]any) { body["instance_id"] = strings.Repeat("0", 64) }, 409},
		{"private_configuration", func(body map[string]any) {
			body["codex_configuration"] = map[string]any{"provider_id": "private175", "base_url": "https://provider.invalid/v1", "api_key_env_ref": "PRIVATE175_KEY", "executable": "C:/private175/codex.exe"}
		}, 400},
	} {
		t.Run(check.name, func(t *testing.T) {
			body := desktopRunnerRegistration(f, local, strings.Repeat("d5", 32))
			body["adapter_id"] = "codex-app-server"
			check.mutate(body)
			desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), check.status)
		})
	}
	t.Run("adapter_is_signed", func(t *testing.T) {
		body := desktopRunnerRegistration(f, local, strings.Repeat("e5", 32))
		request := desktopRunnerTestRequest(f, "register", body)
		body["adapter_id"] = "codex-app-server"
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		request.Body = io.NopCloser(bytes.NewReader(raw))
		request.ContentLength = int64(len(raw))
		desktopRunnerTestCall(t, f, request, http.StatusForbidden)
	})
	for _, table := range []string{"suma_release_workers", "suma_credential_grants", "suma_tool_profiles"} {
		if domainACount(t, f.s, table) != 0 {
			t.Fatalf("rejected native registration persisted %s", table)
		}
	}
}
