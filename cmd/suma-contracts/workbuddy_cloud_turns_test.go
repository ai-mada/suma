package main

import (
	"reflect"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestCloudTurnContractKeepsOwnerOnceOnlyAndTypedControls(t *testing.T) {
	api, _ := generate()
	schemas, paths := mapAt(api, "components", "schemas"), mapAt(api, "paths")
	if models["CloudTurnPromptRequest"] != reflect.TypeOf(wc.CloudTurnPromptRequest{}) {
		t.Fatal("prompt DTO detached from admission")
	}
	for _, name := range []string{"CloudTurnSource", "CloudTurnPromptRequest", "CloudTurn", "CloudTurnView", "CloudTurnEntry", "CloudTurnPermissionOption", "CloudTurnPermission", "CloudTurnCommand", "CloudTurnPermissionReplyRequest", "CloudTurnControlRequest", "CloudTurnPage"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("open turn DTO", name)
		}
	}
	for _, name := range []string{"CloudTurnPromptRequest", "CloudTurnPermissionReplyRequest", "CloudTurnControlRequest"} {
		for _, forbidden := range []string{"url", "ticket", "token", "model", "assignment", "run_id", "rpc", "owner_id", "runtime_owner"} {
			if mapAt(schemas, name, "properties")[forbidden] != nil {
				t.Fatal("browser can inject authority", name, forbidden)
			}
		}
	}
	if mapAt(schemas, "CloudTurnPromptRequest", "properties", "expected_version")["minimum"] != 0 || mapAt(schemas, "CloudTurnControlRequest", "properties", "expected_version")["minimum"] != 1 {
		t.Fatal("head zero became implicit or control lost CAS")
	}
	for _, route := range []struct{ path, method, status string }{
		{"/workbuddy-cloud-submissions/{id}/turns", "get", "200"},
		{"/workbuddy-cloud-submissions/{id}/turns", "post", "202"},
		{"/workbuddy-cloud-turns/{id}", "get", "200"},
		{"/workbuddy-cloud-turns/{id}/permissions/{permission_id}/reply", "post", "202"},
		{"/workbuddy-cloud-turns/{id}/cancel", "post", "202"},
		{"/workbuddy-cloud-turns/{id}/reconcile", "post", "200"},
	} {
		op := mapAt(paths, route.path, route.method)
		if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) || mapAt(op, "responses")[route.status] == nil {
			t.Fatal("turn route authority/status drift", route.path)
		}
		if route.method == "get" && (op["x-read-only"] != true || op["x-no-external-effects"] != true) {
			t.Fatal("reading can attach or prompt", route.path)
		}
		if route.method == "post" && op["x-query-parameters-forbidden"] != true {
			t.Fatal("ambiguous command query", route.path)
		}
	}
	if mapAt(paths, "/workbuddy-cloud-turns/{id}/reconcile", "post")["x-no-external-effects"] != true {
		t.Fatal("reconciliation became remote replay")
	}
}
