package main

import "testing"

func TestToolExecutionContractsKeepProfilesAndRunFactsSeparate(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, path := range []string{"/tool-profiles", "/tool-profiles/{id}", "/tool-profiles/{id}/commands", "/runner/tool-inventory", "/task-dispatch-plans", "/task-dispatch-plans/{id}", "/task-dispatch-plans/{id}/start"} {
		if paths[path] == nil {
			t.Fatalf("missing concrete execution route %s", path)
		}
	}
	if mapAt(paths, "/runner/tool-inventory", "post")["x-required-token-scope"] != "runner" {
		t.Fatal("inventory must use independent Runner identity")
	}
	if mapAt(paths, "/tool-profiles", "post")["x-disallowed-token-scopes"] == nil {
		t.Fatal("profile management missing human gate")
	}
	schemas := mapAt(api, "components", "schemas")
	if mapAt(schemas, "Event", "properties")["sequence"] == nil || mapAt(schemas, "ToolEvent", "properties")["command_id"] == nil {
		t.Fatal("tool event shadowed persistent DAG event")
	}
	if mapAt(schemas, "NodeRun", "properties")["tool_evidence"] == nil {
		t.Fatal("current attempt evidence missing")
	}
	if mapAt(schemas, "TaskDispatchPlan", "properties")["context"] == nil {
		t.Fatal("task plan does not freeze its input")
	}
	if mapAt(schemas, "RunnerProgressRequest", "properties")["tool_event"] == nil || mapAt(schemas, "RunnerProgressRequest")["anyOf"] == nil {
		t.Fatal("typed-only progress cannot be represented")
	}
	for _, secret := range []string{"executable", "args", "env", "token", "api_key"} {
		if mapAt(schemas, "ToolExecutionProfile", "properties")[secret] != nil {
			t.Fatalf("public profile exposes %s", secret)
		}
	}
}
