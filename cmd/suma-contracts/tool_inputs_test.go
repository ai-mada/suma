package main

import "testing"

func TestToolInputsContractSeparatesHumanAnswersFromMachineFacts(t *testing.T) {
	api, typescript := generate()
	schemas := mapAt(api, "components", "schemas")
	if mapAt(schemas, "ToolInputList", "properties")["protocol"] == nil || mapAt(schemas, "ToolExecutionProfile", "properties")["user_input_protocol"] == nil {
		t.Fatal("configured capability is not projected")
	}
	for _, route := range []string{"/runner/tool-inputs", "/pool-runner/tool-inputs"} {
		op := mapAt(api, "paths", route, "post")
		if op["x-machine-only"] != true || responseSchema(op, "200")["$ref"] != "#/components/schemas/ToolInput" {
			t.Fatal("machine source lost its exact scope", route)
		}
	}
	op := mapAt(api, "paths", "/tool-inputs/{id}/answer", "post")
	if op["x-machine-only"] == true || responseSchema(op, "200")["$ref"] != "#/components/schemas/ToolInput" {
		t.Fatal("human answer shares machine source operation")
	}
	if mapAt(schemas, "ToolInputAnswerRequest", "properties")["decision"] != nil || mapAt(schemas, "ToolInputRequest", "properties")["acp"] != nil {
		t.Fatal("question answers mixed with permission branches")
	}
	if typescript == "" || mapAt(schemas, "CodexUserInputQuestion", "properties", "is_secret")["const"] != false {
		t.Fatal("unsupported secret transport advertised")
	}
}
