package main

import (
	"reflect"
	"strings"
	"testing"
)

// Use the real existing schemas, shared browser-view decorator and reflected
// model registry; only operation registration is captured before main is wired.
func cloudContractFixture(t *testing.T) map[string]any {
	t.Helper()
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, value := range workBuddyCloudModels() {
		typ := reflect.TypeOf(value)
		if prior, ok := models[typ.Name()]; ok && prior != typ {
			t.Fatalf("cloud model name collides: %s", typ.Name())
		}
		schema(typ)
	}
	for {
		added := false
		for _, name := range modelNames() {
			if schemas[name] == nil {
				schemas[name] = structSchema(models[name])
				added = true
			}
		}
		if !added {
			break
		}
	}
	paths := mapAt(api, "paths")
	add := func(path, method, output string, list, body bool) map[string]any {
		parameters := []any{map[string]any{"in": "query", "name": "project_id", "required": !body, "schema": map[string]any{"type": "string"}}}
		if strings.Contains(path, "{id}") {
			parameters = append(parameters, map[string]any{"in": "path", "name": "id", "required": true, "schema": map[string]any{"type": "string"}})
		}
		op := map[string]any{"parameters": parameters, "security": []any{}, "responses": map[string]any{"200": map[string]any{"description": "Success"}, "201": map[string]any{"description": "Created"}}}
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		mapAt(paths, path)[method] = op
		return op
	}
	workBuddyCloudContracts(add, api)
	currentAuthorityContracts(api)
	return api
}

func TestWorkBuddyCloudRoutesKeepBrowserAndMachineAuthoritySeparate(t *testing.T) {
	api := cloudContractFixture(t)
	browser := [][2]string{{"/workbuddy-cloud-nodes", "get"}, {"/workbuddy-cloud-delegations", "post"}, {"/workbuddy-cloud-delegations/{id}", "get"}, {"/workbuddy-cloud-delegations/{id}/revoke", "post"}, {"/workbuddy-cloud-submissions/{id}", "get"}, {"/workbuddy-cloud-submissions/{id}/observe", "post"}}
	for _, route := range browser {
		op := mapAt(api, "paths", route[0], route[1])
		if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) {
			t.Fatal("human view acquired machine authority", route)
		}
		if route[1] == "get" && op["x-read-only"] != true {
			t.Fatal("local read unexpectedly has external effects", route)
		}
		if route[1] == "post" && op["x-browser-original-view"] == nil {
			t.Fatal("human command lost original browser view", route)
		}
	}
	for _, prefix := range []string{"/runner", "/pool-runner"} {
		for _, suffix := range []string{"/workbuddy-cloud-submissions", "/workbuddy-cloud-submissions/{id}/refresh"} {
			op := mapAt(api, "paths", prefix+suffix, "post")
			security := op["security"].([]any)
			if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] != nil || op["x-browser-original-view"] != nil {
				t.Fatal("machine route borrowed browser authority")
			}
			for _, raw := range op["parameters"].([]any) {
				if raw.(map[string]any)["in"] == "query" {
					t.Fatal("strict machine write acquired a duplicate query scope")
				}
			}
		}
	}
	node := mapAt(api, "paths", "/workbuddy-cloud-nodes", "get")
	required := map[string]bool{}
	for _, raw := range node["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["in"] == "query" && p["required"] == true {
			required[p["name"].(string)] = true
		}
	}
	if !required["project_id"] || !required["run_id"] || !required["node_id"] {
		t.Fatal("node reopening lost exact source")
	}
}

func TestWorkBuddyCloudSchemasKeepOnceOnlyAndUnknownFacts(t *testing.T) {
	api := cloudContractFixture(t)
	for _, name := range []string{"CloudCreateRequest", "CloudRefreshRequest", "CloudObserveRequest", "CloudDelegateRequest", "CloudRevokeRequest"} {
		model := mapAt(api, "components", "schemas", name)
		if model["additionalProperties"] != false {
			t.Fatal("request contract is open", name)
		}
		fields := mapAt(model, "properties")
		for _, forbidden := range []string{"prompt", "task_id", "url", "token", "action"} {
			if fields[forbidden] != nil {
				t.Fatal("machine or delegation accepts provider input", name, forbidden)
			}
		}
		for key, raw := range fields {
			if strings.HasPrefix(key, "expected_") && strings.HasSuffix(key, "version") && raw.(map[string]any)["maximum"] != 2147483646 {
				t.Fatal("write version has no CAS room", name, key)
			}
		}
	}
	if mapAt(api, "components", "schemas", "CloudObserveRequest", "properties")["assignment"] != nil {
		t.Fatal("human observation became machine command")
	}
	if mapAt(api, "components", "schemas", "CloudDelegation", "properties", "max_creates")["const"] != 1 {
		t.Fatal("delegation silently gained more creates")
	}
	if !reflect.DeepEqual(mapAt(api, "components", "schemas", "CloudSubmission", "properties", "state")["enum"], []string{"prepared", "unknown", "task_known"}) {
		t.Fatal("submission is presented as acceptance")
	}
	if mapAt(api, "components", "schemas", "CloudSubmission", "properties", "usage_state")["const"] != "unreported" {
		t.Fatal("unknown usage became zero/complete")
	}
	if mapAt(api, "components", "schemas", "CloudSubmission", "properties", "version")["maximum"] != 2147483647 {
		t.Fatal("final persisted version cannot be read")
	}
	if mapAt(api, "components", "schemas", "CreateRequest", "properties", "prompt")["x-max-utf8-bytes"] != 256<<10 {
		t.Fatal("fixed provider request bound drifted")
	}
	for _, secret := range []string{"acp_token", "token", "link", "sandboxLink"} {
		if mapAt(api, "components", "schemas", "Task", "properties")[secret] != nil {
			t.Fatal("public task exposes provider access")
		}
	}
}
