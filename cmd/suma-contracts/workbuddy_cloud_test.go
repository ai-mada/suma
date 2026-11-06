package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
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

func TestWorkBuddyCloudExecutionWireUsesActualClaimAndWaitModels(t *testing.T) {
	api, _ := generate()
	if models["Assignment"] != reflect.TypeOf(dag.Assignment{}) || models["Result"] != reflect.TypeOf(dag.Result{}) {
		t.Fatal("cloud execution contract is detached from actual DAG models")
	}
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"CloudAssignmentBinding", "CloudWaitEvidence"} {
		if schemas[name] == nil || mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("execution evidence is missing or open", name)
		}
		fields := mapAt(schemas, name, "properties")
		for _, secret := range []string{"prompt", "token", "url", "endpoint", "task_id", "access_token"} {
			if fields[secret] != nil {
				t.Fatal("execution evidence gained supplier authority", name, secret)
			}
		}
	}
	for _, item := range []struct{ model, field, target string }{{"Assignment", "workbuddy_cloud", "CloudAssignmentBinding"}, {"Result", "workbuddy_cloud_wait", "CloudWaitEvidence"}} {
		field := mapAt(schemas, item.model, "properties", item.field)
		variants, ok := field["anyOf"].([]any)
		if !ok || len(variants) != 2 || variants[0].(map[string]any)["$ref"] != "#/components/schemas/"+item.target || field["description"] == nil {
			t.Fatal("cloud field no longer refers to the actual optional Go type", item)
		}
	}
	if mapAt(schemas, "Assignment", "properties", "workbuddy_cloud")["readOnly"] != true {
		t.Fatal("Runner can select its own grant binding")
	}
	binding := mapAt(schemas, "CloudAssignmentBinding", "properties")
	if mapAt(binding, "delegation_version")["maximum"] != 2147483647 || mapAt(binding, "delegation_version")["minimum"] != 1 || mapAt(binding, "delegation_sha256")["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatal("immutable binding lost exact persisted version/hash")
	}
	wait := mapAt(schemas, "CloudWaitEvidence", "properties")
	if mapAt(wait, "protocol")["const"] != "workbuddy-cloud/v1" || mapAt(wait, "claim_id")["maxLength"] != 100 || mapAt(wait, "create_operation_id")["minLength"] != 8 || mapAt(wait, "create_operation_id")["maxLength"] != 128 {
		t.Fatal("wait evidence lost exact claim/operation bounds")
	}
	if !reflect.DeepEqual(mapAt(wait, "cause")["enum"], []string{"creation_unconfirmed", "remote_unknown", "waiting_input", "delivery_required", "interrupted", "authorization_changed", "observation_unconfirmed"}) {
		t.Fatal("wait evidence acquired fabricated completion or stop state")
	}
	conditions, ok := mapAt(schemas, "CloudWaitEvidence")["allOf"].([]any)
	if !ok || len(conditions) != 1 {
		t.Fatal("wait submission reference/version pair is unconstrained")
	}
	condition := conditions[0].(map[string]any)
	if mapAt(condition, "else", "properties", "submission_version")["const"] != 0 || mapAt(condition, "then", "properties", "submission_version")["minimum"] != 1 {
		t.Fatal("wait allows unknown task identity with known submission version")
	}
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

func TestWorkBuddyCloudNodeDeliveryLinkIsOptionalClosedProjectMetadata(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	node := mapAt(schemas, "CloudNodeView")
	field := mapAt(node, "properties", "task_delivery")
	if field["$ref"] != "#/components/schemas/CloudTaskDeliveryLink" || field["anyOf"] != nil || field["readOnly"] != true {
		t.Fatal("node locator is not a nonnull read-only reference")
	}
	for _, name := range node["required"].([]string) {
		if name == "task_delivery" {
			t.Fatal("manual and unlinked cloud nodes must omit the locator")
		}
	}
	link := mapAt(schemas, "CloudTaskDeliveryLink")
	fields := mapAt(link, "properties")
	if link["additionalProperties"] != false || len(fields) != 3 ||
		!reflect.DeepEqual(link["required"], []string{"task_id", "task_version", "run_relation"}) {
		t.Fatal("locator projects fields beyond the safe task identity, version and relation")
	}
	version := mapAt(fields, "task_version")
	if version["minimum"] != 1 || version["maximum"] != 2147483647 ||
		!reflect.DeepEqual(mapAt(fields, "run_relation")["enum"], []string{"current", "changed", "unbound"}) ||
		mapAt(fields, "task_id")["pattern"] != "^[A-Za-z0-9_.-]{1,64}$" ||
		!reflect.DeepEqual(mapAt(fields, "task_id", "not")["enum"], []string{".", ".."}) {
		t.Fatal("locator's version, relation or task path constraints drifted")
	}
	if mapAt(schemas, "CloudNodeProgress", "properties")["task_delivery"] != nil {
		t.Fatal("browser-only locator appeared in CLI progress")
	}
}

func TestWorkBuddyCloudProgressKeepsExactReadScopeAndClosedProjection(t *testing.T) {
	api, _ := generate()
	path := mapAt(api, "paths", "/runs/{id}/nodes/{node}/workbuddy-status")
	op := mapAt(path, "get")
	if len(path) != 1 || op["x-read-only"] != true || op["x-no-external-effects"] != true ||
		!reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}, map[string]any{"bearerAuth": []string{}}}) ||
		!reflect.DeepEqual(op["x-disallowed-token-scopes"], []string{"agent", "runner"}) {
		t.Fatal("cloud progress does not preserve exact read authority")
	}
	params := map[string]string{}
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["in"] == "header" {
			continue
		}
		if p["required"] != true {
			t.Fatal("scope parameter is optional", p["name"])
		}
		params[p["name"].(string)] = p["in"].(string)
	}
	if !reflect.DeepEqual(params, map[string]string{"id": "path", "node": "path", "project_id": "query"}) {
		t.Fatal("cloud progress scope changed", params)
	}
	schema := mapAt(api, "components", "schemas", "CloudNodeProgress")
	fields := mapAt(schema, "properties")
	if schema["additionalProperties"] != false || len(fields) != 12 {
		t.Fatal("cloud progress projection is open")
	}
	for _, name := range []string{"project_id", "run_id", "node_id", "run_version", "run_state", "node_state", "node_reason", "attempt", "submission_state", "provider_status", "observed_at", "observation_failed"} {
		if mapAt(fields, name)["readOnly"] != true {
			t.Fatal("cloud progress field is not read only", name)
		}
	}
	if !reflect.DeepEqual(mapAt(schema, "then")["required"], []string{"provider_status", "observed_at"}) {
		t.Fatal("known cloud observations can omit their saved evidence")
	}
}

func TestWorkBuddyCloudLookupGeneratedRoutesAreMachineOnlyLocalReads(t *testing.T) {
	api, _ := generate()
	for _, prefix := range []string{"/runner", "/pool-runner"} {
		path := prefix + "/workbuddy-cloud-submissions/lookup"
		if mapAt(api, "paths")[path] == nil {
			t.Fatal("missing exact cloud lookup route", path)
		}
		op := mapAt(api, "paths", path, "post")
		scheme := "runnerAuth"
		if prefix == "/pool-runner" {
			scheme = "bearerAuth"
		}
		if !reflect.DeepEqual(op["security"], []any{map[string]any{scheme: []string{}}}) || op["x-browser-original-view"] != nil {
			t.Fatal("lookup accepts non-machine credential", path)
		}
		if op["x-read-only"] != true || op["x-no-external-effects"] != true || op["x-query-parameters-forbidden"] != true {
			t.Fatal("lookup acquired an external or query command surface", path)
		}
		for _, raw := range op["parameters"].([]any) {
			if raw.(map[string]any)["in"] == "query" {
				t.Fatal("lookup accepts query identity")
			}
		}
		if mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/CloudLookupRequest" {
			t.Fatal("lookup request is not typed")
		}
		responses := mapAt(op, "responses")
		if responses["201"] != nil || mapAt(responses, "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/CloudLookupResult" {
			t.Fatal("lookup creates a resource or has untyped result")
		}
		headers := mapAt(responses, "200", "headers")
		if headers["Cache-Control"] == nil || mapAt(headers, "Cache-Control", "schema")["const"] != "no-store" {
			t.Fatal("final generated lookup lost Cache-Control: no-store", path)
		}
		if headers["Referrer-Policy"] == nil || mapAt(headers, "Referrer-Policy", "schema")["const"] != "no-referrer" {
			t.Fatal("final generated lookup lost Referrer-Policy: no-referrer", path)
		}
		if !strings.Contains(op["description"].(string), "not_found") || !strings.Contains(op["description"].(string), "not permission to Create") {
			t.Fatal("absent record is described as execution permission")
		}
	}
}

func TestWorkBuddyCloudLookupGeneratedSchemasPreserveAbsenceAndExactState(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"CloudLookupRequest", "CloudLookupResult"} {
		if schemas[name] == nil || mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("lookup schema missing or open", name)
		}
		fields := mapAt(schemas, name, "properties")
		for _, key := range []string{"idempotency_key", "expected_version", "prompt", "task_id", "url", "token", "action"} {
			if fields[key] != nil {
				t.Fatal("lookup accepts command/provider authority", name, key)
			}
		}
		if fields["assignment"] == nil || fields["project_id"] == nil || fields["delegation_id"] == nil {
			t.Fatal("lookup lost original scope", name)
		}
		assignmentDescription := mapAt(fields, "assignment")["description"].(string)
		if !strings.Contains(assignmentDescription, "does not require an active lease") || !strings.Contains(assignmentDescription, "does not authorize execution") || !strings.Contains(assignmentDescription, "same current attempt/fence/worker epoch") || !strings.Contains(assignmentDescription, "LeaseUntil, DispatchState") {
			t.Fatal("lookup confuses original assignment recovery with live execution permission", name)
		}
	}
	result := mapAt(schemas, "CloudLookupResult")
	if !reflect.DeepEqual(mapAt(result, "properties", "state")["enum"], []string{"not_found", "prepared", "unknown", "task_known"}) {
		t.Fatal("lookup fabricated business completion")
	}
	for _, raw := range mapAt(result, "properties") {
		if raw.(map[string]any)["readOnly"] != true {
			t.Fatal("lookup result became writable")
		}
	}
	constraints, ok := result["allOf"].([]any)
	if !ok || len(constraints) != 4 {
		t.Fatal("lookup state/submission relationship is unspecified")
	}
	for _, raw := range constraints {
		branch := raw.(map[string]any)
		state := mapAt(branch, "if", "properties", "state")["const"].(string)
		then := mapAt(branch, "then")
		if state == "not_found" {
			if !reflect.DeepEqual(mapAt(then, "not")["required"], []string{"submission"}) {
				t.Fatal("not_found permits a submission")
			}
		} else {
			if !reflect.DeepEqual(then["required"], []string{"submission"}) || mapAt(then, "properties", "submission", "properties", "state")["const"] != state {
				t.Fatal("known lookup state can disagree with original submission", state)
			}
		}
	}
}
