package platform

import (
	"testing"
)

func poolCreateBody() map[string]any {
	return map[string]any{"name": "团队共享开发池", "description": "仅授权范围执行", "policy": map[string]any{"concurrency_limit": 2, "resource_limits": map[string]any{}, "budget_limit": nil, "tenant_weights": map[string]any{}}, "idempotency_key": newID()}
}

func TestPoolHTTPBilateralGrantAndExplicitCurrentScope(t *testing.T) {
	f := newFixture(t)
	pool := f.call("POST", "/execution-pools", poolCreateBody(), 201)
	id := pool["id"].(string)
	request := f.body(map[string]any{"pool_id": id, "project_weight": 2})
	grant := f.call("POST", "/project-execution/grants", request, 201)
	if grant["status"] != "pending" || grant["shared_project_name"] != "验证项目" {
		t.Fatal("project request did not freeze shared label")
	}
	again := f.call("POST", "/project-execution/grants", request, 201)
	if again["id"] != grant["id"] {
		t.Fatal("request duplicated the grant")
	}
	inbox := f.call("GET", "/execution-pools/"+id+"/grants", nil, 200)
	if inbox["total"] != float64(1) {
		t.Fatal("owner inbox omitted pending scope")
	}
	command := map[string]any{"action": "approve", "expected_version": 1, "idempotency_key": newID()}
	approved := f.call("POST", "/execution-pools/"+id+"/grants/"+grant["id"].(string)+"/commands", command, 200)
	if approved["status"] != "approved" || approved["version"] != float64(2) {
		t.Fatal("approval failed to advance exact delegation")
	}
	f.call("POST", "/execution-pools/"+id+"/grants/"+grant["id"].(string)+"/commands", command, 200)
	f.call("POST", "/execution-pools/"+id+"/grants/"+grant["id"].(string)+"/commands", map[string]any{"action": "approve", "expected_version": 1, "idempotency_key": newID()}, 409)
	var mirrored int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_scopes WHERE pool_id=? AND tenant_id=? AND project_id=? AND grant_id=? AND grant_version=2 AND status='active'`, id, f.user.TenantID, f.project, grant["id"]).Scan(&mirrored); err != nil || mirrored != 1 {
		t.Fatal("bilateral status and scheduler mirror diverged", err)
	}
	f.call("POST", "/project-execution/grants/"+grant["id"].(string)+"/commands", f.body(map[string]any{"action": "withdraw", "expected_version": 2}), 200)
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_scopes WHERE pool_id=? AND status='active'`, id).Scan(&mirrored); err != nil || mirrored != 0 {
		t.Fatal("withdraw left admission authority active", err)
	}
	// Project scope tokens cannot turn this new organization API into a second
	// way to issue machine credentials or change another project's delegation.
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	f.bearer("GET", "/execution-pools", token, nil, 403)
	f.bearer("POST", "/execution-pools/"+id+"/runners", token, map[string]any{"name": "forged", "max_slots": 1, "ttl_seconds": 3600, "idempotency_key": newID()}, 403)
}

func TestPoolHTTPRequiresCompleteTypedPolicy(t *testing.T) {
	f := newFixture(t)
	for _, change := range []func(map[string]any){
		func(p map[string]any) { delete(p, "budget_limit") },
		func(p map[string]any) { p["concurrency_limit"] = 1.25 },
		func(p map[string]any) { p["resource_limits"] = nil },
		func(p map[string]any) { p["tenant_weights"] = nil },
		func(p map[string]any) { p["unexpected"] = true },
	} {
		body := poolCreateBody()
		change(body["policy"].(map[string]any))
		f.call("POST", "/execution-pools", body, 400)
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_execution_pools`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid policy created partial pool", err)
	}
}
