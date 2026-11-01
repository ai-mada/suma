package main

import "testing"

func TestDevelopmentContinuationContractSeparatesSelectionFromProtectedCodeInput(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	source := mapAt(schemas, "DevelopmentContinuationSource", "properties")
	if len(source) != 5 || source["token"] != nil || source["candidate"] != nil {
		t.Fatal("source selection cannot submit candidate, secret or code authority")
	}
	create := mapAt(schemas, "TaskDiscussionDevelopmentCreateRequest", "properties")
	if create["continuation_source"] == nil || create["development_continuation"] != nil {
		t.Fatal("only the protected service resolves the continuation binding")
	}
	for _, model := range []string{"TaskDispatchPlan", "TaskDiscussionDevelopmentPlan", "Assignment"} {
		if mapAt(schemas, model, "properties", "development_continuation")["readOnly"] != true {
			t.Fatalf("protected binding must be read-only in %s", model)
		}
	}
	op := mapAt(api, "paths", "/tasks/{id}/development-continuation-options", "get")
	if op["x-read-only"] != true || responseSchema(op, "200")["$ref"] != "#/components/schemas/TaskDevelopmentContinuationOptions" {
		t.Fatal("source listing cannot itself start or accept code")
	}
	if mapAt(schemas, "DevelopmentContinuationBinding", "properties")["accepted_basis_sha256"] != nil {
		t.Fatal("unaccepted same-task input must not impersonate accepted code")
	}
}
