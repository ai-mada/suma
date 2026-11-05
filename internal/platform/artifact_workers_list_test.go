package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReleaseWorkerListCurrentCredentialManagement(t *testing.T) {
	f := newFixture(t)
	path := "/release-workers?project_id=" + f.project
	ownerCookie := f.cookie

	emptyRequest := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
	emptyRequest.AddCookie(ownerCookie)
	emptyReply := httptest.NewRecorder()
	f.handler.ServeHTTP(emptyReply, emptyRequest)
	var empty ReleaseWorkerList
	if err := json.Unmarshal(emptyReply.Body.Bytes(), &empty); err != nil || emptyReply.Code != http.StatusOK ||
		emptyReply.Header().Get("Cache-Control") != "no-store" || empty.Items == nil ||
		len(empty.Items) != 0 || !empty.CredentialManagementAllowed {
		t.Fatal("owner of empty project did not receive a current, non-cacheable management view", err)
	}
	otherProject := f.call("POST", "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, 201)
	otherPath := "/release-workers?project_id=" + otherProject["id"].(string)
	adminInvite := governanceInvite(f, "node-admin@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "admin"}})
	admin := governanceAccept(f, adminInvite, "node-admin@example.test")
	adminCookie := f.cookie
	adminID := admin["user"].(map[string]any)["id"]
	f.call(http.MethodGet, otherPath, nil, http.StatusForbidden)

	registration := f.call(http.MethodPost, "/release-workers", f.body(map[string]any{
		"worker_id": "managed-project-node", "profiles": []any{},
	}), http.StatusCreated)
	check := func(out map[string]any, canManage, canRotate bool) {
		t.Helper()
		items, ok := out["items"].([]any)
		if !ok || len(items) != 1 || out["credential_management_allowed"] != canManage {
			t.Fatal("incorrect list scope or management projection")
		}
		worker := items[0].(map[string]any)
		if worker["id"] != registration["id"] || worker["version"] != float64(1) || worker["status"] != "active" ||
			(worker["manual_rotation_allowed"] == true) != canRotate {
			t.Fatal("incorrect worker facts or row-level rotation eligibility")
		}
		raw := stringify(out)
		if strings.Contains(raw, registration["token"].(string)) || strings.Contains(raw, "token_hash") {
			t.Fatal("credential list leaked secret material")
		}
	}
	check(f.call(http.MethodGet, path, nil, http.StatusOK), true, true)

	// The organization owner can revoke this worker, but only its original
	// project administrator can rotate it while the administrator grant remains.
	f.cookie = ownerCookie
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
	check(f.call(http.MethodGet, path, nil, http.StatusOK), true, false)
	api := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api", "label": "browser"}), http.StatusCreated)["token"].(string)
	check(f.bearer(http.MethodGet, path, api, nil, http.StatusOK), false, false)
	f.bearer(http.MethodGet, otherPath, api, nil, http.StatusForbidden)
	f.call(http.MethodPost, "/project-access", f.body(map[string]any{
		"user_id": adminID, "role": "viewer", "expected_version": 1,
	}), http.StatusOK)
	f.cookie = adminCookie
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
	check(f.call(http.MethodGet, path, nil, http.StatusOK), false, false)
	f.call(http.MethodPost, "/release-workers", f.body(map[string]any{
		"worker_id": "refused-project-node", "profiles": []any{},
	}), http.StatusForbidden)
	f.call(http.MethodPatch, "/release-workers/"+registration["id"].(string), f.body(map[string]any{
		"expected_version": 1, "status": "revoked",
	}), http.StatusForbidden)
	check(f.call(http.MethodGet, path, nil, http.StatusOK), false, false)

	f.cookie = ownerCookie
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)
	f.call(http.MethodPost, "/project-access", f.body(map[string]any{
		"user_id": adminID, "action": "remove", "expected_version": 2,
	}), http.StatusOK)
	f.cookie = adminCookie
	f.call(http.MethodGet, path, nil, http.StatusForbidden)
}

func TestReleaseWorkerListRejectsMissingAndForeignProjectScope(t *testing.T) {
	f := newFixture(t)
	f.call(http.MethodGet, "/release-workers", nil, http.StatusBadRequest)
	f.call(http.MethodGet, "/release-workers?project_id=%40organization", nil, http.StatusBadRequest)
	otherOrganization := f.call(http.MethodPost, "/organizations", map[string]any{
		"name": "Other organization", "idempotency_key": newID(),
	}, http.StatusCreated)
	f.call(http.MethodPost, "/auth/switch", map[string]any{
		"organization_id": otherOrganization["id"], "idempotency_key": newID(),
	}, http.StatusOK)
	f.call(http.MethodGet, "/release-workers?project_id="+f.project, nil, http.StatusNotFound)
}
