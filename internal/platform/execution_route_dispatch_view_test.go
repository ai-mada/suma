package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func requireTaskDispatchRouteView(t *testing.T, plan map[string]any) map[string]any {
	t.Helper()
	route, ok := plan["execution_route"].(map[string]any)
	if !ok {
		t.Fatal("dispatch preview does not expose its reviewed execution route")
	}
	return route
}

func TestTaskDispatchExecutionRouteViewFreezesExactPoolBasis(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	pool, grant := dispatchAuthorizedPool(t, f)
	profile := dispatchPoolTool(t, f, pool)
	dispatchSetRoute(t, f, 0, &grant)
	request["tool_profile_id"], request["expected_tool_version"] = profile.ID, profile.Version
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	route := requireTaskDispatchRouteView(t, plan)
	if route["tenant_id"] != f.user.TenantID || route["project_id"] != f.project || route["version"] != float64(1) || route["pool_id"] != pool.ID || route["grant_id"] != grant.ID || route["grant_version"] != float64(grant.Version) {
		t.Fatal("preview does not identify the exact reviewed route/grant", route)
	}
	var privateVersion, privateGrantVersion int
	var privatePool, privateGrant string
	if err := f.s.DB.QueryRow(`SELECT route_version,pool_id,grant_id,grant_version FROM suma_task_dispatch_route_bases WHERE plan_id=? AND tenant_id=? AND project_id=?`, plan["id"], f.user.TenantID, f.project).Scan(&privateVersion, &privatePool, &privateGrant, &privateGrantVersion); err != nil {
		t.Fatal(err)
	}
	if privateVersion != 1 || privatePool != pool.ID || privateGrant != grant.ID || privateGrantVersion != grant.Version {
		t.Fatal("public view and authority basis were not fixed together")
	}
	dispatchSetRoute(t, f, 1, nil)
	view := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if view["valid"] != false || stringify(requireTaskDispatchRouteView(t, view)) != stringify(route) {
		t.Fatal("GET replaced the historical route with the new default", view["valid"], view["execution_route"])
	}
	replay := f.call("POST", "/task-dispatch-plans", request, 201)
	if replay["id"] != plan["id"] || replay["valid"] != false || stringify(requireTaskDispatchRouteView(t, replay)) != stringify(route) {
		t.Fatal("preview operation replay changed the reviewed route")
	}
}

func TestTaskDispatchExecutionRouteViewRecordsExplicitLocalDefault(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	route := requireTaskDispatchRouteView(t, plan)
	if route["tenant_id"] != f.user.TenantID || route["project_id"] != f.project || route["version"] != float64(0) || route["pool_id"] != "" || route["grant_id"] != "" || route["grant_version"] != float64(0) {
		t.Fatal("explicitly reviewed local default was confused with unrecorded history", route)
	}
}

func TestTaskDispatchExecutionRouteViewLeavesOldUnrecordedNil(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	updateTaskDispatchPublicRoute(t, f, plan["id"].(string), nil)
	view := f.call("GET", "/task-dispatch-plans/"+plan["id"].(string)+"?project_id="+f.project, nil, 200)
	if _, present := view["execution_route"]; present {
		t.Fatal("GET invented a route for an old unrecorded payload", view["execution_route"])
	}
}

func TestTaskDispatchExecutionRouteCannotBeSubmittedByClient(t *testing.T) {
	f, _, request, _ := taskDispatchFixture(t)
	request["execution_route"] = map[string]any{"pool_id": newID(), "version": 1}
	f.call("POST", "/task-dispatch-plans", request, 400)
	delete(request, "execution_route")
	plan := f.call("POST", "/task-dispatch-plans", request, 201)
	f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"], "execution_route": map[string]any{"pool_id": newID(), "version": 1}}), 400)
	var runs int
	f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs)
	if runs != 0 {
		t.Fatal("client-supplied route created work", runs)
	}
}

// Deliberately alter the display-only payload in the test database. Neither a
// foreign displayed pool nor a forged current version may become authority.
func TestTaskDispatchExecutionRoutePubviewCannotAuthorizeStart(t *testing.T) {
	for _, mode := range []string{"foreign_pool_does_not_redirect", "current_view_does_not_refresh_private_basis"} {
		t.Run(mode, func(t *testing.T) {
			f, _, request, _ := taskDispatchFixture(t)
			plan := f.call("POST", "/task-dispatch-plans", request, 201)
			fake := map[string]any{"tenant_id": f.user.TenantID, "project_id": f.project, "version": float64(99), "pool_id": newID(), "grant_id": newID(), "grant_version": float64(99)}
			want := 201
			if mode == "current_view_does_not_refresh_private_basis" {
				dispatchSetRoute(t, f, 0, nil)
				fake = map[string]any{"tenant_id": f.user.TenantID, "project_id": f.project, "version": float64(1), "pool_id": "", "grant_id": "", "grant_version": float64(0)}
				want = 409
			}
			updateTaskDispatchPublicRoute(t, f, plan["id"].(string), fake)
			out := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), want)
			if want == 201 {
				tx, err := f.s.DB.BeginTx(context.Background(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				actual, err := dag.ReadRunExecutionRouteTx(context.Background(), tx, f.s.Dialect, f.user.TenantID, f.project, out["id"].(string))
				if err != nil || actual.PoolID != "" || actual.Version != 0 {
					t.Fatal("display payload changed the actual execution route", actual, err)
				}
			} else {
				var count int
				f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count)
				if count != 0 {
					t.Fatal("forged view bypassed the private basis")
				}
			}
		})
	}
}

func updateTaskDispatchPublicRoute(t *testing.T, f *fixture, id string, route map[string]any) {
	t.Helper()
	var raw string
	if err := f.s.DB.QueryRow(`SELECT payload FROM suma_task_dispatch_plans WHERE id=?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if route == nil {
		delete(payload, "execution_route")
	} else {
		payload["execution_route"] = route
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_task_dispatch_plans SET payload=? WHERE id=?`, stringify(payload), id); err != nil {
		t.Fatal(err)
	}
}
