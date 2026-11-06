package main

import (
	"reflect"
	"testing"
)

func TestExpertTeamPlanListContractScopesOptionalTeamFilter(t *testing.T) {
	api, _ := generate()
	for _, path := range []string{"/expert-team-plans", "/expert-teams", "/expert-team-plans/{id}/revisions"} {
		found := false
		for _, param := range mapAt(api, "paths", path, "get")["parameters"].([]any) {
			p := param.(map[string]any)
			if p["in"] != "query" || p["name"] != "team_id" {
				continue
			}
			found = true
			if p["required"] == true || mapAt(p, "schema")["pattern"] != `^[A-Za-z0-9_-]{1,64}$` {
				t.Fatal("team filter must be optional and bounded", path)
			}
		}
		if found != (path == "/expert-team-plans") {
			t.Fatal("team filter is missing or accepted outside the plan collection", path)
		}
	}
}

func TestExpertTeamExecutionContractPreservesBoundedReadOnlyFacts(t *testing.T) {
	api, _ := generate()
	field := func(model, name string) map[string]any {
		return mapAt(api, "components", "schemas", model, "properties", name)
	}
	if field("ExpertTeamExecutionFact", "nodes")["maxItems"] != 24 || field("ExpertTeamExecutionFact", "nodes_total")["minimum"] != 0 {
		t.Fatal("bounded detail and whole-run count must remain distinct")
	}
	for _, model := range []string{"ExpertTeamExecutionFact", "ExpertTeamExecutionNodeFact"} {
		if mapAt(api, "components", "schemas", model)["additionalProperties"] != false {
			t.Fatal("execution projection shape must be explicit", model)
		}
		for name, property := range mapAt(api, "components", "schemas", model, "properties") {
			if property.(map[string]any)["readOnly"] != true {
				t.Fatal("execution fact must not imply writable authority", model, name)
			}
		}
	}
	if field("ExpertTeamExecutionNodeFact", "reason")["enum"] != nil || field("ExpertTeamExecutionNodeFact", "ready_at")["format"] != nil {
		t.Fatal("source reason is open text and readiness may be absent as an empty string")
	}
	want := []string{"waiting_dependencies", "ready", "queued", "waiting_execution", "executing", "execution_unknown", "awaiting_review", "accepted", "blocked", "stopped"}
	if !reflect.DeepEqual(field("ExpertTeamPlanItem", "state")["enum"], want) {
		t.Fatal("waiting and unknown must not collapse into running or success")
	}
	if field("ExpertTeamPlanItem", "execution")["readOnly"] != true {
		t.Fatal("plan execution projection is read only")
	}
}
