package main

import (
	"reflect"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudCollaborationContractsBindOriginalLoadAndPrivateArchive(t *testing.T) {
	api, _ := generate()
	schemas, paths := mapAt(api, "components", "schemas"), mapAt(api, "paths")
	if models["CloudHistoryLoad"] != reflect.TypeOf(wc.CloudHistoryLoad{}) {
		t.Fatal("load contract detached from the actual persisted DTO")
	}
	for _, name := range []string{"CloudCollaborationSource", "CloudHistoryLoadRequest", "CloudHistoryLoad", "CloudHistoryEntry", "CloudCollaborationView", "CloudHistoryPage"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("collaboration DTO is open", name)
		}
		for _, secret := range []string{"ticket", "acp_url", "rpc_id", "raw", "prompt", "uri", "token"} {
			if mapAt(schemas, name, "properties")[secret] != nil {
				t.Fatal("public collaboration leaked protocol authority", name, secret)
			}
		}
	}
	request := mapAt(schemas, "CloudHistoryLoadRequest")
	required := request["required"].([]string)
	contains := func(values []string, wanted string) bool {
		for _, value := range values {
			if value == wanted {
				return true
			}
		}
		return false
	}
	if !contains(required, "expected_version") || mapAt(request, "properties", "expected_version")["minimum"] != 0 {
		t.Fatal("first head version must be explicit zero, not an omitted default")
	}
	load := mapAt(schemas, "CloudHistoryLoad")
	for _, field := range []string{"operation_id", "head_version_at_start", "source", "deadline_at", "overdue"} {
		if !contains(load["required"].([]string), field) {
			t.Fatal("load receipt cannot bind original intent", field)
		}
	}
	if mapAt(load, "properties", "coverage")["const"] != "unverified" {
		t.Fatal("load acknowledgement gained an unsupported coverage guarantee")
	}
	for _, field := range []string{"latest_load", "last_loaded"} {
		p := mapAt(schemas, "CloudCollaborationView", "properties", field)
		if p["anyOf"] != nil || p["$ref"] != "#/components/schemas/CloudHistoryLoad" {
			t.Fatal("optional stored load became nullable", field)
		}
	}
	for _, route := range []struct {
		path, method string
		external     bool
	}{
		{"/workbuddy-cloud-submissions/{id}/collaboration", "get", false},
		{"/workbuddy-cloud-submissions/{id}/history-loads", "post", true},
		{"/workbuddy-cloud-history-loads/{id}", "get", false},
	} {
		op := mapAt(paths, route.path, route.method)
		if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) || !reflect.DeepEqual(op["x-disallowed-token-scopes"], []string{"api", "agent", "runner"}) {
			t.Fatal("route lost explicit human-session boundary", route.path)
		}
		if route.external {
			if op["x-no-external-effects"] == true || op["x-idempotency"] == nil {
				t.Fatal("session load was advertised as a passive archive GET")
			}
		} else if op["x-no-external-effects"] != true {
			t.Fatal("archive GET can accidentally reconnect", route.path)
		}
	}
}
