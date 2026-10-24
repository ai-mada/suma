package main

import (
	"strings"
	"testing"
)

// Personal preferences must never become implicitly project-scoped when the
// selected workbench project changes; generated clients use these parameters.
func TestLocalePreferencesContractsKeepBrowserIdentityAndExplicitVersion(t *testing.T) {
	api, _ := generate()
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		for _, method := range []string{"get", "patch"} {
			op := mapAt(api, "paths", path, method)
			security := op["security"].([]any)
			if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
				t.Fatal("preferences require a browser identity", path, method)
			}
			for _, raw := range op["parameters"].([]any) {
				if raw.(map[string]any)["name"] == "project_id" {
					t.Fatal("preferences accidentally depend on selected project", path)
				}
			}
			if _, ok := mapAt(op, "responses")["201"]; ok {
				t.Fatal("preference update returns200", path)
			}
		}
	}
	schema := mapAt(api, "components", "schemas", "LocalePreferenceUpdateRequest")
	if schema["additionalProperties"] != false {
		t.Fatal("unbounded preference mutation")
	}
	version := mapAt(schema, "properties", "expected_version")
	if version["minimum"] != 0 || version["maximum"] != 2147483646 {
		t.Fatal("preference CAS range lost")
	}
	if mapAt(api, "paths", "/organization/preferences", "patch")["x-required-organization-roles"] == nil {
		t.Fatal("organization authority missing")
	}
	for _, path := range []string{"/account/preferences", "/organization/preferences"} {
		headers := map[string]bool{}
		for _, raw := range mapAt(api, "paths", path, "patch")["parameters"].([]any) {
			parameter := raw.(map[string]any)
			if parameter["in"] == "header" && parameter["required"] == true {
				headers[parameter["name"].(string)] = true
			}
		}
		if !headers["X-Suma-Expected-User"] || (path == "/organization/preferences" && !headers["X-Suma-Expected-Organization"]) {
			t.Fatal("preference write lacks original-view identity binding", path)
		}
		if path == "/account/preferences" && headers["X-Suma-Expected-Organization"] {
			t.Fatal("global account recovery must work after the same user switches organization")
		}
	}
}

func TestWorkflowValidationContractSeparatesRawEvidenceAndPresentation(t *testing.T) {
	api, wire := generate()
	op := mapAt(api, "paths", "/workflows/validate", "post")
	if _, ok := mapAt(op, "responses")["200"]; !ok {
		t.Fatal("validation must return200")
	}
	if _, ok := mapAt(op, "responses")["201"]; ok {
		t.Fatal("validation does not create a workflow")
	}
	props := mapAt(api, "components", "schemas", "WorkflowValidation", "properties")
	if mapAt(props, "errors", "items")["type"] != "string" {
		t.Fatal("raw compiler evidence lost")
	}
	if mapAt(props, "diagnostics", "items")["$ref"] != "#/components/schemas/Rendered" {
		t.Fatal("typed localized diagnostic missing")
	}
	if !strings.Contains(wire, "export interface WorkflowValidation") {
		t.Fatal("wire DTO absent")
	}
}
