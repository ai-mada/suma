package main

import "testing"

func TestRequirementDesignContractSeparatesDraftPreviewAndApproval(t *testing.T) {
	api, _ := generate()
	for _, path := range []string{"/requirements/{id}/design/commands", "/requirements/{id}/design/adopt"} {
		op := mapAt(api, "paths", path, "post")
		security := op["security"].([]any)
		if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil || op["x-browser-original-view"] == nil {
			t.Fatal("design approval lost original browser boundary", path)
		}
	}
	if len(mapAt(api, "paths", "/requirements/{id}/design", "put")["security"].([]any)) != 2 {
		t.Fatal("scoped project API draft editing must remain legal")
	}
	if mapAt(api, "paths", "/requirements/{id}/design/preview", "post")["x-read-only"] != true {
		t.Fatal("preview must not be represented as adoption or reservation")
	}
	for _, name := range []string{"RequirementDesignSaveRequest", "RequirementDesignCommandRequest", "RequirementDesignAdoptRequest", "RequirementDesignPreviewRequest", "RequirementDesignTask", "RequirementDesignContent"} {
		if mapAt(api, "components", "schemas", name)["additionalProperties"] != false {
			t.Fatal("trusted input is not closed", name)
		}
	}
	context := mapAt(api, "components", "schemas", "RequirementDesignContext", "properties")
	if _, exists := context["tasks"]; exists {
		t.Fatal("execution must not repeat the whole task proposal")
	}
	if mapAt(api, "components", "schemas", "TaskContext", "properties", "design")["readOnly"] != true {
		t.Fatal("design context must be server-resolved")
	}
}
