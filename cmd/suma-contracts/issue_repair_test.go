package main

import (
	"strings"
	"testing"
)

func TestIssueRepairContractKeepsClosureAsCurrentHumanCommand(t *testing.T) {
	api, types := generate()
	paths := mapAt(api, "paths")
	for _, action := range []string{"prepare", "regression", "close", "reopen"} {
		op := mapAt(paths, "/issues/{id}/repair/"+action, "post")
		request := mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"].(string)
		if !strings.Contains(request, "IssueRepair") || op["x-disallowed-token-scopes"] == nil {
			t.Fatal("repair command lost its exact closed request/authority", action)
		}
		if action == "close" || action == "reopen" {
			if op["x-required-project-authority"] != "current browser project approval" {
				t.Fatal("machine writes acquired closure authority", action)
			}
		}
	}
	for _, name := range []string{"IssueRepairView", "IssueRepairPrepareRequest", "IssueRepairBindRequest", "IssueRepairCloseRequest", "IssueRepairReopenRequest"} {
		if mapAt(api, "components", "schemas", name)["additionalProperties"] != false || !strings.Contains(types, "export interface "+name+" ") {
			t.Fatal("repair schema/type is missing or open", name)
		}
	}
	props := mapAt(api, "components", "schemas", "IssueRepairCloseRequest", "properties")
	if props["status"] != nil || props["closure"] != nil || props["passed"] != nil {
		t.Fatal("caller can supply their own closure facts")
	}
	source := mapAt(api, "components", "schemas", "IssueRepair", "properties", "automation_source")
	prepare := mapAt(api, "components", "schemas", "IssueRepairPrepareRequest", "properties")
	if source["$ref"] != "#/components/schemas/IssueRepairAutomationSource" || source["readOnly"] != true || prepare["automation_source"] != nil {
		t.Fatal("automation provenance must be a server-owned repair snapshot")
	}
}
