package main

import "testing"

func TestTurnSteeringContractSeparatesHumanSourceFromNativeDelivery(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	create := mapAt(schemas, "SteeringCreateRequest", "properties")
	if create["body"] != nil || create["thread_id"] != nil || create["model"] != nil || create["message_id"] == nil || create["expected_session_version"] == nil {
		t.Fatal("browser input acquired native target or override authority")
	}
	for _, route := range []string{"/runner/steering/sessions", "/pool-runner/steering/sessions"} {
		op := mapAt(api, "paths", route, "post")
		if op["x-machine-only"] != true || responseSchema(op, "200")["$ref"] != "#/components/schemas/SteeringOpenResult" {
			t.Fatal("live turn is not explicitly advertised by the original machine")
		}
	}
	if mapAt(api, "paths", "/tasks/{id}/execution-steering", "get")["x-no-agent-effects"] != true {
		t.Fatal("target reading must not execute")
	}
	observation := mapAt(schemas, "CodexSteeringObservation", "properties")
	if observation["write_written"] == nil || observation["native_accepted"] == nil || observation["decision"] != nil {
		t.Fatal("write and acceptance became a permission choice")
	}
}
