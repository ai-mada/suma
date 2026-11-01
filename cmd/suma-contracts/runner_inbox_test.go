package main

import "testing"

func TestRunnerInboxContractIsCurrentBrowserScopedAndBounded(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/runner-pairings", "get")
	security := op["security"].([]any)
	if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil || op["x-read-only"] != true {
		t.Fatal("pairing inbox must require current browser administration")
	}
	parameters := map[string]map[string]any{}
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		parameters[p["name"].(string)] = p
	}
	for _, name := range []string{"project_id", "status", "q", "limit", "cursor"} {
		if parameters[name] == nil {
			t.Fatalf("missing inbox query %s", name)
		}
	}
	if parameters["project_id"]["required"] != true || mapAt(parameters["limit"], "schema")["maximum"] != 100 {
		t.Fatal("project scope and bounded collection size are required")
	}
	page := mapAt(api, "components", "schemas", "RunnerPairingPage")
	if page["additionalProperties"] != false || responseSchema(op, "200")["$ref"] != "#/components/schemas/RunnerPairingPage" {
		t.Fatal("inbox must use the closed shared page, not a fabricated list total")
	}
	for _, name := range []string{"items", "total", "as_of", "next_cursor"} {
		if mapAt(page, "properties", name)["readOnly"] != true {
			t.Fatalf("page fact must be read-only: %s", name)
		}
	}
	if mapAt(api, "components", "schemas", "RunnerPairing", "properties", "reason_code")["description"] == "" {
		t.Fatal("browser diagnostics must explain their limited authority")
	}
}
