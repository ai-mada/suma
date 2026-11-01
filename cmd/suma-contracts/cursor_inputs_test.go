package main

import (
	"testing"

	cu "github.com/ai-mada/suma/internal/cursorcontract"
)

func TestCursorInputsContractKeepsNativeUnionAndPlanAuthority(t *testing.T) {
	api, _ := generate()
	s := mapAt(api, "components", "schemas")
	found := false
	for _, protocol := range mapAt(s, "ToolInputList", "properties", "protocol")["enum"].([]string) {
		found = found || protocol == cu.InputProtocol
	}
	if !found {
		t.Fatal("actual Cursor input list violates its generated capability schema")
	}
	for _, name := range []string{"ToolInputRequest", "CursorInputRequest", "ToolInputAnswerRequest"} {
		if len(mapAt(s, name)["oneOf"].([]any)) != 2 || mapAt(s, name)["additionalProperties"] != false {
			t.Fatal("native union is not closed", name)
		}
	}
	for _, name := range []string{"CursorInputDecision", "CursorInputEvidence", "ToolInputDeliveryObservation"} {
		if mapAt(s, name)["additionalProperties"] != false {
			t.Fatal("open source or decision", name)
		}
	}
	for _, field := range []string{"plan_uri", "planUri", "permission", "rpc", "command", "configuration_sha256"} {
		if mapAt(s, "CursorInputDecision", "properties")[field] != nil {
			t.Fatal("browser decision acquired native authority", field)
		}
	}
	if mapAt(s, "ToolInputDeliveryObservation", "properties")["execution_closed"] == nil || mapAt(s, "ToolInputDeliveryObservation", "properties")["turn_closed"] != nil {
		t.Fatal("generic closure impersonated a Codex turn")
	}
	if mapAt(s, "CodexUserInputObservation", "properties")["turn_closed"] == nil {
		t.Fatal("actual Codex protocol fact was erased")
	}
	if mapAt(s, "CursorAskQuestion", "properties", "questions")["maxItems"] != cu.MaxQuestions {
		t.Fatal("source limits drifted")
	}
	paths := mapAt(api, "paths")
	for _, path := range []string{"/runner/tool-inputs", "/pool-runner/tool-inputs", "/tool-inputs/{id}/answer"} {
		if mapAt(paths, path, "post", "responses")["200"] == nil {
			t.Fatal("original receipt route changed", path)
		}
	}
}
