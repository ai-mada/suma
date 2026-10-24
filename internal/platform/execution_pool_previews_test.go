package platform

import (
	"encoding/json"
	"testing"
	"time"
)

func poolHTTPApprovedGrant(t *testing.T, f *fixture, pool string) map[string]any {
	t.Helper()
	grant := f.call("POST", "/project-execution/grants", f.body(map[string]any{"pool_id": pool, "project_weight": 1}), 201)
	return f.call("POST", "/execution-pools/"+pool+"/grants/"+grant["id"].(string)+"/commands", map[string]any{"action": "approve", "expected_version": 1, "idempotency_key": newID()}, 200)
}

func TestPoolHTTPPolicyPreviewImmutableCommitAndCurrentAuthority(t *testing.T) {
	f := newFixture(t)
	pool := f.call("POST", "/execution-pools", poolCreateBody(), 201)
	path := "/execution-pools/" + pool["id"].(string) + "/policy-previews"
	policy := map[string]any{"concurrency_limit": 4, "resource_limits": map[string]any{}, "budget_limit": 10, "tenant_weights": map[string]any{f.user.TenantID: 3}}
	request := map[string]any{"expected_version": 1, "policy": policy, "idempotency_key": newID()}
	preview := f.call("POST", path, request, 201)
	if preview["valid"] != true || preview["payload_sha256"] == "" {
		t.Fatal("review preview missing fixed inputs")
	}
	if f.call("POST", path, request, 201)["id"] != preview["id"] {
		t.Fatal("preview retry created a different review")
	}
	before := f.call("GET", "/execution-pools/"+pool["id"].(string), nil, 200)["policy"].(map[string]any)
	if before["version"] != float64(1) || before["budget_spent"] != float64(0) {
		t.Fatal("preview changed admission facts")
	}
	path += "/" + preview["id"].(string)
	commit := map[string]any{"expected_version": 1, "idempotency_key": newID()}
	applied := f.call("POST", path+"/commit", commit, 200)
	if applied["policy"].(map[string]any)["version"] != float64(2) {
		t.Fatal("policy not adopted exactly once")
	}
	f.call("POST", path+"/commit", commit, 200)
	view := f.call("GET", path, nil, 200)
	if view["valid"] != false || view["state"] != "committed" || view["version"] != float64(2) {
		t.Fatal("committed review remains executable")
	}
	f.call("POST", path+"/commit", map[string]any{"expected_version": 1, "idempotency_key": newID()}, 409)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", path+"/commit", commit, 401)
}

func TestPoolHTTPRoutePreviewFixedGrantAndOldRun(t *testing.T) {
	f := newFixture(t)
	pool := f.call("POST", "/execution-pools", poolCreateBody(), 201)
	poolID := pool["id"].(string)
	grant := poolHTTPApprovedGrant(t, f, poolID)
	// A real API-created run predating the default switch must stay local.
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "旧本地流程", "nodes": []any{map[string]any{"id": "wait", "type": "approval"}}, "edges": []any{}}), 201)
	old := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"], "workflow_version": 1}), 201)
	request := f.body(map[string]any{"expected_version": 0, "route": map[string]any{"mode": "pool", "pool_id": poolID, "grant_id": grant["id"], "grant_version": 2}})
	preview := f.call("POST", "/project-execution/route-previews", request, 201)
	path := "/project-execution/route-previews/" + preview["id"].(string)
	commit := f.body(map[string]any{"expected_version": 1})
	adopted := f.call("POST", path+"/commit", commit, 200)
	if adopted["pool_id"] != poolID || adopted["version"] != float64(1) {
		t.Fatal("default execution route not changed")
	}
	f.call("POST", path+"/commit", commit, 200)
	current := f.call("GET", "/project-execution/route?project_id="+f.project, nil, 200)
	if current["pool_id"] != poolID {
		t.Fatal("refresh lost adopted default")
	}
	old = f.call("GET", "/runs/"+old["id"].(string)+"?project_id="+f.project, nil, 200)
	if old["execution_pool_id"] != nil && old["execution_pool_id"] != "" {
		t.Fatal("default change rerouted old run")
	}
	// Revocation invalidates a still-open review as a review fact (HTTP 200),
	// while a write must reject it without advancing the route.
	request["expected_version"], request["idempotency_key"] = 1, newID()
	stale := f.call("POST", "/project-execution/route-previews", request, 201)
	stalePath := "/project-execution/route-previews/" + stale["id"].(string)
	f.call("POST", "/project-execution/grants/"+grant["id"].(string)+"/commands", f.body(map[string]any{"action": "withdraw", "expected_version": 2}), 200)
	view := f.call("GET", stalePath+"?project_id="+f.project, nil, 200)
	if view["valid"] != false || view["reason"] == "" {
		t.Fatal("withdrawn grant still confirms old route preview")
	}
	f.call("POST", stalePath+"/commit", f.body(map[string]any{"expected_version": 1}), 409)
}

func TestPoolHTTPPreviewExpiryMetadataAndExplicitZeroVersion(t *testing.T) {
	f := newFixture(t)
	pool := f.call("POST", "/execution-pools", poolCreateBody(), 201)
	poolID := pool["id"].(string)
	base := "/execution-pools/" + poolID + "/policy-previews"
	request := map[string]any{"expected_version": 1, "policy": poolCreateBody()["policy"], "idempotency_key": newID()}
	preview := f.call("POST", base, request, 201)
	f.call("POST", "/execution-pools/"+poolID+"/commands", map[string]any{"action": "pause", "expected_version": 1, "idempotency_key": newID()}, 200)
	path := base + "/" + preview["id"].(string)
	if f.call("GET", path, nil, 200)["valid"] != false {
		t.Fatal("metadata change did not invalidate review")
	}
	f.call("POST", path+"/commit", map[string]any{"expected_version": 1, "idempotency_key": newID()}, 409)
	// Mutate an explicitly test-owned persisted deadline plus its bound hash;
	// expiry is checked from database time, not a browser-supplied clock.
	request["idempotency_key"] = newID()
	preview = f.call("POST", base, request, 201)
	var typed ExecutionPoolPolicyPreview
	raw, _ := json.Marshal(preview)
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatal(err)
	}
	typed.ExpiresAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	typed.PayloadSHA256 = poolPolicyPreviewHash(typed)
	if _, err := f.s.DB.Exec(`UPDATE suma_execution_pool_previews SET expires_at=?,payload=? WHERE id=?`, typed.ExpiresAt, stringify(typed), typed.ID); err != nil {
		t.Fatal(err)
	}
	if f.call("GET", base+"/"+typed.ID, nil, 200)["valid"] != false {
		t.Fatal("expired preview remained valid")
	}
	f.call("POST", base+"/"+typed.ID+"/commit", map[string]any{"expected_version": 1, "idempotency_key": newID()}, 409)
	for _, version := range []any{nil, "omitted", 0.5} {
		body := f.body(map[string]any{"route": map[string]any{"mode": "local"}})
		if version != "omitted" {
			body["expected_version"] = version
		}
		f.call("POST", "/project-execution/route-previews", body, 400)
	}
}
