package main

import "github.com/ai-mada/suma/internal/platform"

func taskDesignViewModels() []any { return []any{platform.TaskDesignContextView{}} }

func taskDesignViewContracts(add operationAdder, api map[string]any) {
	op := add("/tasks/{id}/design-context", "get", "TaskDesignContextView", false, false)
	op["description"] = "Current-authorized task provenance read. Protected bindings and indexed actual create/update adoptions locate immutable approved text. Current eligibility never replaces the historical snapshot. No tool/workflow configuration or editable Record.Data establishes adoption."
	schemas := toolContractMap(api, "components", "schemas")
	fields := toolContractMap(schemas, "TaskDesignContextView", "properties")
	for _, name := range []string{"context", "requirement", "adopted_task", "adopted_by", "adopted_at", "bound", "eligibility"} {
		toolContractMap(fields, name)["readOnly"] = true
	}
	toolContractMap(fields, "eligibility")["enum"] = []string{"none", "current", "task_changed", "requirement_changed", "publication_changed", "retired", "excluded", "source_unavailable"}
}
