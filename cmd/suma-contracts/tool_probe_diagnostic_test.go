package main

import (
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	"testing"
)

func TestProbeDiagnosticSchemaHasOnlyClassifiedFactsAndExactSuggestions(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	diagnostic := toolContractMap(schemas, "ToolInstallationProbeDiagnostic")
	if diagnostic["additionalProperties"] != false || len(toolContractMap(diagnostic, "properties")) != 3 {
		t.Fatal("diagnostic exposed arbitrary output or commands")
	}
	choices := diagnostic["oneOf"].([]any)
	if len(choices) != len(ii.ProbeDiagnosticCodes()) {
		t.Fatal("schema lost a classified failure")
	}
	for _, choice := range choices {
		fields := toolContractMap(choice.(map[string]any), "properties")
		code := toolContractMap(fields, "code")["const"].(string)
		if toolContractMap(fields, "next_action")["const"] != ii.ProbeDiagnosticNextAction(code) {
			t.Fatal("published suggestion drifted from runtime policy")
		}
		for _, stage := range toolContractMap(fields, "stage")["enum"].([]string) {
			if ii.ValidateProbeDiagnostic(ii.ProbeDiagnostic{Stage: stage, Code: code, NextAction: ii.ProbeDiagnosticNextAction(code)}) != nil {
				t.Fatal("published phase is not an observed failure contract")
			}
		}
	}
}
