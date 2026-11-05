package main

import "github.com/ai-mada/suma/internal/platform"

func taskDeliveryModels() []any {
	return []any{platform.TaskDelivery{}, platform.TaskDeliveryRun{}, platform.TaskDeliveryRunTarget{}, platform.TaskDeliveryNodeSummary{}, platform.TaskDeliveryNode{}, platform.TaskDeliveryExpertPlans{}, platform.TaskDeliveryExpertPlan{}, platform.TaskDeliveryIssues{}, platform.TaskDeliveryIssue{}}
}

// The selected-task view is separate from the complete editable dependency
// graph. Bounded projections never ship run output or imply review permission.
func taskDeliveryContracts(add operationAdder, api map[string]any) {
	read := add("/tasks/{id}/delivery", "get", "TaskDelivery", false, false)
	successStatus(read, "200")
	read["description"] = "Read one currently authorized task and its exact bound run in one snapshot. Counts are execution facts, not independent test or human acceptance eligibility. Team plans and trusted issues cover only current_run, with at most 20 references per collection. No logs, output, commands, credentials, implicit dispatch, review or issue creation."
	read["x-request-schema-coverage"] = "exact id and required project_id; current authorization, complete snapshot and trusted operation provenance enforced by server"
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"TaskDelivery", "TaskDeliveryRun", "TaskDeliveryRunTarget", "TaskDeliveryNodeSummary", "TaskDeliveryNode", "TaskDeliveryExpertPlans", "TaskDeliveryExpertPlan", "TaskDeliveryIssues", "TaskDeliveryIssue"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, value := range toolContractMap(model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	toolContractMap(schemas, "TaskDelivery", "properties", "run_basis_state")["enum"] = []string{"not_dispatched", "current", "changed", "unknown"}
	toolContractMap(schemas, "TaskDelivery", "properties", "formal_acceptance")["description"] = "True only for current formal acceptance checked against the exact task, dependency and run evidence. Not a can-review grant."
	toolContractMap(schemas, "TaskDeliveryRun", "properties", "workbuddy_delivery")["description"] = "First exact waiting WorkBuddy assignment in this bound run that requires code delivery verification. Omitted when unavailable. A read-only navigation target, independent of the bounded node list."
	target := toolContractMap(schemas, "TaskDeliveryRunTarget", "properties")
	toolContractMap(target, "run_id")["minLength"], toolContractMap(target, "run_id")["maxLength"] = 1, 100
	toolContractMap(target, "node_id")["minLength"], toolContractMap(target, "node_id")["maxLength"] = 1, 150
	toolContractMap(target, "attempt")["minimum"] = 1
	for _, name := range []string{"TaskDeliveryNodeSummary", "TaskDeliveryExpertPlans", "TaskDeliveryIssues"} {
		properties := toolContractMap(schemas, name, "properties")
		toolContractMap(properties, "total")["minimum"] = 0
		toolContractMap(properties, "limit")["const"] = 20
		toolContractMap(properties, "items")["maxItems"] = 20
		toolContractMap(properties, "complete")["description"] = "Whether the bounded item list covers the total; counts always cover this declared collection scope."
	}
	for _, name := range []string{"TaskDeliveryExpertPlans", "TaskDeliveryIssues"} {
		toolContractMap(schemas, name, "properties", "scope")["const"] = "current_run"
	}
	for _, name := range []string{"waiting", "running", "succeeded", "failed", "skipped", "reconciling", "other"} {
		toolContractMap(schemas, "TaskDeliveryNodeSummary", "properties", name)["minimum"] = 0
	}
}
