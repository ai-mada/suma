package main

import "github.com/ai-mada/suma/internal/platform"

func requirementTraceabilityModels() []any {
	return []any{platform.RequirementTraceability{}, platform.RequirementTraceabilityCriterion{}, platform.RequirementTraceabilityTask{}}
}

// Criterion positions have meaning only within the returned requirement version.
// This contract exposes protected task lineage, never inferred test-case coverage.
func requirementTraceabilityContracts(add operationAdder, api map[string]any) {
	read := add("/requirements/{id}/traceability", "get", "RequirementTraceability", false, false)
	successStatus(read, "200")
	read["description"] = "Read current requirement criteria through the exact published design and protected adopted tasks. A criterion index is scoped to requirement ID and version. Changed, retired or unavailable sources do not remap old indices. Current-run test-node summaries are execution facts, not per-criterion test-case coverage or permission to accept. Reading never dispatches, reviews or creates issues."
	read["x-request-schema-coverage"] = "exact id and required single project_id; one current project-read authorization transaction; no arbitrary projection filters"
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"RequirementTraceability", "RequirementTraceabilityCriterion", "RequirementTraceabilityTask"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, value := range toolContractMap(model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	properties := toolContractMap(schemas, "RequirementTraceability", "properties")
	toolContractMap(properties, "eligibility")["enum"] = []string{"none", "current", "requirement_changed", "retired", "source_unavailable"}
	toolContractMap(properties, "criteria")["maxItems"] = 100
	toolContractMap(properties, "tasks")["maxItems"] = 100
	toolContractMap(properties, "requirement_version")["minimum"] = 1
	toolContractMap(properties, "design_version")["minimum"] = 0
	toolContractMap(properties, "published_revision")["minimum"] = 0
	toolContractMap(schemas, "RequirementTraceabilityCriterion", "properties", "index")["minimum"] = 1
	toolContractMap(schemas, "RequirementTraceabilityCriterion", "properties", "task_keys")["description"] = "Complete current published task keys covering this exact requirement version and index. Empty for an unmapped or noncurrent source."
	task := toolContractMap(schemas, "RequirementTraceabilityTask", "properties")
	toolContractMap(task, "binding_state")["enum"] = []string{"not_adopted", "current", "excluded", "retired", "publication_changed", "requirement_changed", "task_changed", "source_unavailable"}
	toolContractMap(task, "run_basis_state")["enum"] = []string{"not_dispatched", "current", "changed", "unknown"}
	toolContractMap(task, "formal_acceptance")["description"] = "Current published binding, exact design/run evidence and formal task acceptance all hold. This is not a test-case coverage claim or can-review grant."
}
