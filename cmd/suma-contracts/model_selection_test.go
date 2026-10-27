package main

import (
	"strings"
	"testing"
)

// HTTP rejects explicit null before pointer decoding. Generated client types
// must not advertise that value or a simultaneous native/Codex selection.
func TestModelSelectionRequestsMatchNonNullHTTPContract(t *testing.T) {
	api, types := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"TaskDispatchPreviewRequest", "ExpertTeamPlanItemRequest", "CandidateCompositionRepairCreateRequest", "CandidateCompositionRepairRevisionRequest"} {
		properties := mapAt(schemas, name, "properties")
		for _, key := range []string{"model_execution_selection", "codex_execution_selection"} {
			value, exists := properties[key]
			if !exists {
				continue
			}
			property := value.(map[string]any)
			if property["$ref"] == nil || property["anyOf"] != nil {
				t.Fatalf("%s.%s still allows null: %#v", name, key, property)
			}
		}
		start := strings.Index(types, "export interface "+name+" {")
		if start < 0 {
			t.Fatal("request client type absent: " + name)
		}
		block := strings.SplitN(types[start:], "}\n", 2)[0]
		for _, line := range strings.Split(block, "\n") {
			if strings.Contains(line, "_execution_selection?") && strings.Contains(line, "| null") {
				t.Fatalf("client advertises unsupported null: %s", line)
			}
		}
	}
	for _, name := range []string{"TaskDispatchPreviewRequest", "ExpertTeamPlanItemRequest"} {
		if mapAt(schemas, name)["not"] == nil {
			t.Fatal("dual selections advertised: " + name)
		}
	}
}
