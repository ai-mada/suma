package main

import ii "github.com/ai-mada/suma/internal/toolinstallationcontract"

func toolInstallationProbeDiagnosticSchemas(schemas, item map[string]any) {
	diagnostic := toolContractMap(schemas, "ToolInstallationProbeDiagnostic")
	diagnostic["additionalProperties"] = false
	diagnostic["description"] = "Classified failed metadata observation only. No raw output, local path, credentials, repair command or execution authority."
	toolContractMap(diagnostic, "properties", "stage")["enum"] = ii.ProbeDiagnosticStages()
	toolContractMap(diagnostic, "properties", "code")["enum"] = ii.ProbeDiagnosticCodes()
	toolContractMap(diagnostic, "properties", "next_action")["enum"] = ii.ProbeDiagnosticActions()
	var choices []any
	for _, code := range ii.ProbeDiagnosticCodes() {
		choices = append(choices, map[string]any{"properties": map[string]any{
			"code":        map[string]any{"const": code},
			"stage":       map[string]any{"enum": ii.ProbeDiagnosticStagesForCode(code)},
			"next_action": map[string]any{"const": ii.ProbeDiagnosticNextAction(code)},
		}})
	}
	diagnostic["oneOf"] = choices
	toolContractMap(item, "properties", "diagnostic")["description"] = "Optional classified failure; absent does not prove success. A present value is non-null and closed. Global unknown/stop reconciliation still takes precedence over the suggestion."
	constraints := item["allOf"].([]any)
	constraints = append(constraints, map[string]any{
		"if": map[string]any{"required": []string{"diagnostic"}},
		"then": map[string]any{"properties": map[string]any{
			"launch_supported": map[string]any{"const": false},
			"reason_code":      map[string]any{"not": map[string]any{"enum": []string{"not_installed", "workspace_required", "configuration_required", "acp_configuration_required", "cursor_editor_only"}}},
		}},
	})
	constraints = append(constraints, map[string]any{
		"if":   map[string]any{"required": []string{"diagnostic"}, "properties": map[string]any{"reason_code": map[string]any{"const": "scan_incomplete"}}},
		"then": map[string]any{"properties": map[string]any{"diagnostic": map[string]any{"properties": map[string]any{"stage": map[string]any{"const": "resolve"}}}}},
	})
	item["allOf"] = constraints
}
