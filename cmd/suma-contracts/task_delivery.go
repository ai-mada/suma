package main

import (
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/platform"
)

const taskDeliveryRepositoryIntakePath = "/tasks/{id}/delivery/repository-source/intakes"
const taskDeliveryRepositoryDispatchPath = taskDeliveryRepositoryIntakePath + "/{intake}/dispatch-plans"

func taskDeliveryModels() []any {
	return []any{platform.TaskDelivery{}, platform.TaskDeliveryRun{}, platform.TaskDeliveryRunTarget{}, platform.TaskDeliveryWorkBuddyOrigin{}, platform.TaskDeliveryNodeSummary{}, platform.TaskDeliveryNode{}, platform.TaskDeliveryExpertPlans{}, platform.TaskDeliveryExpertPlan{}, platform.TaskDeliveryIssues{}, platform.TaskDeliveryIssue{}, platform.TaskDeliveryRepositorySource{}, platform.TaskDeliveryRepositoryIntakeRequest{}, platform.TaskDeliveryRepositoryIntake{}, platform.TaskDeliveryRepositoryDispatchPreviewRequest{}, platform.TaskDeliveryRepositorySourceOptions{}, platform.TaskDeliveryRepositorySourceProfile{}}
}

// The selected-task view is separate from the complete editable dependency
// graph. Bounded projections never ship run output or imply review permission.
func taskDeliveryContracts(add operationAdder, api map[string]any) {
	read := add("/tasks/{id}/delivery", "get", "TaskDelivery", false, false)
	successStatus(read, "200")
	read["description"] = "Read one currently authorized task and its exact bound run in one snapshot. Counts are execution facts, not independent test or human acceptance eligibility. Team plans and trusted issues cover only current_run, with at most 20 references per collection. No logs, output, commands, credentials, implicit dispatch, review or issue creation."
	read["x-request-schema-coverage"] = "exact id and required project_id; current authorization, complete snapshot and trusted operation provenance enforced by server"
	source := add("/tasks/{id}/delivery/repository-source", "get", "TaskDeliveryRepositorySource", false, false)
	successStatus(source, "200")
	browserOnly(source)
	source["description"] = "Read the selected registered Git branch head for the current waiting WorkBuddy task attempt. Verifies current browser write authority and checks task, execution and repository configuration on both sides of the authenticated provider read. No repository mutation, code import or acceptance."
	source["x-request-schema-coverage"] = "exact id and five required single-value query parameters; route, versions, provider and waiting attempt verified by the server"
	for _, name := range []string{"task_version", "run_id", "target_id", "target_version"} {
		value := map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483647}
		if name == "run_id" || name == "target_id" {
			value = map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
		}
		parameter := queryParameter(name, "Exact current source selection.", value)
		parameter["required"] = true
		appendQuery(source, parameter)
	}
	responses := toolContractMap(source, "responses")
	responses["404"] = map[string]any{"description": "Task or registered repository source unavailable in this project"}
	responses["502"] = map[string]any{"description": "Registered repository provider could not be read"}
	responses["503"] = map[string]any{"description": "Registered repository provider is unavailable"}
	schemas := toolContractMap(api, "components", "schemas")
	taskDeliveryRepositoryIntakeContract(add, schemas)
	taskDeliveryRepositoryDispatchContracts(add, api, schemas)
	taskDeliveryRepositoryOptionsContract(add, schemas, source)
	for _, name := range []string{"TaskDelivery", "TaskDeliveryRun", "TaskDeliveryRunTarget", "TaskDeliveryWorkBuddyOrigin", "TaskDeliveryNodeSummary", "TaskDeliveryNode", "TaskDeliveryExpertPlans", "TaskDeliveryExpertPlan", "TaskDeliveryIssues", "TaskDeliveryIssue", "TaskDeliveryRepositorySource"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, value := range toolContractMap(model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	observed := toolContractMap(schemas, "TaskDeliveryRepositorySource", "properties")
	toolContractMap(observed, "object_format")["enum"] = []string{"sha1", "sha256"}
	toolContractMap(observed, "head_commit")["pattern"] = `^(?:[a-f0-9]{40}|[a-f0-9]{64})$`
	toolContractMap(observed, "observed_at")["format"] = "date-time"
	toolContractMap(schemas, "TaskDelivery", "properties", "run_basis_state")["enum"] = []string{"not_dispatched", "current", "changed", "unknown"}
	toolContractMap(schemas, "TaskDelivery", "properties", "formal_acceptance")["description"] = "True only for current formal acceptance checked against the exact task, dependency and run evidence. Not a can-review grant."
	toolContractMap(schemas, "TaskDeliveryRun", "properties", "workbuddy_delivery")["description"] = "First exact waiting WorkBuddy assignment in this bound run that requires code delivery verification. Omitted when unavailable. A read-only navigation target, independent of the bounded node list."
	target := toolContractMap(schemas, "TaskDeliveryRunTarget", "properties")
	toolContractMap(target, "run_id")["minLength"], toolContractMap(target, "run_id")["maxLength"] = 1, 100
	toolContractMap(target, "node_id")["minLength"], toolContractMap(target, "node_id")["maxLength"] = 1, 150
	toolContractMap(target, "attempt")["minimum"] = 1
	toolContractMap(schemas, "TaskDeliveryRun", "properties", "workbuddy_origin")["description"] = "Verified historical WorkBuddy attempt linked to this started continuation. Its repository or local candidate handoff is separate from test and acceptance facts."
	origin := toolContractMap(schemas, "TaskDeliveryWorkBuddyOrigin", "properties")
	toolContractMap(origin, "run_id")["pattern"] = `^[A-Za-z0-9_-]{1,100}$`
	toolContractMap(origin, "node_id")["const"] = "workbuddy-cloud"
	toolContractMap(origin, "attempt")["minimum"] = 1
	toolContractMap(origin, "kind")["enum"] = []string{"repository_source", "local_candidate"}
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
	toolContractMap(schemas, "TaskDeliveryIssue", "properties", "source")["enum"] = []string{"execution_failure", "review_rejected", "test_case", "package_test"}
	for _, name := range []string{"waiting", "running", "succeeded", "failed", "skipped", "reconciling", "other"} {
		toolContractMap(schemas, "TaskDeliveryNodeSummary", "properties", name)["minimum"] = 0
	}
}

func taskDeliveryRepositoryOptionsContract(add operationAdder, schemas, source map[string]any) {
	const model = "TaskDeliveryRepositorySourceOptions"
	op := add("/tasks/{id}/delivery/repository-source/options", "get", model, false, false)
	successStatus(op, "200")
	browserOnly(op)
	op["parameters"] = append([]any{}, source["parameters"].([]any)...)
	op["description"] = "Read exact repository handoff choices for a current browser project approver. The live registered branch pair is optional when unavailable. Retained approvals and eligible distinct-Worker profile choices are scoped to the current task, attempt, target version and execution route. This read creates no approval, plan or run."
	op["x-request-schema-coverage"] = "exact task ID and five required single-value query parameters; current project approval and original task/attempt basis verified server-side"
	op["x-response-cache-control"] = "no-store"
	toolContractMap(op, "responses")["404"] = map[string]any{"description": "Task or registered repository source unavailable in this project"}
	for _, name := range []string{model, "TaskDeliveryRepositorySourceProfile"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
		for _, field := range toolContractMap(schemas, name, "properties") {
			field.(map[string]any)["readOnly"] = true
		}
	}
	fields := toolContractMap(schemas, model, "properties")
	toolContractMap(fields, "live_state")["enum"] = []string{"ready", "unchanged", "unavailable"}
	toolContractMap(fields, "published_case_count")["minimum"] = 0
	toolContractMap(fields, "attempt")["minimum"] = 1
	toolContractMap(fields, "retained_intakes")["maxItems"] = 8
	for _, name := range []string{"producer_profiles", "test_profiles"} {
		toolContractMap(fields, name)["maxItems"] = 200
	}
}

func taskDeliveryRepositoryIntakeContract(add operationAdder, schemas map[string]any) {
	const requestModel, responseModel = "TaskDeliveryRepositoryIntakeRequest", "TaskDeliveryRepositoryIntake"
	op := add(taskDeliveryRepositoryIntakePath, "post", responseModel, false, true)
	successStatus(op, "201")
	browserOnly(op)
	request(op, requestModel)
	op["description"] = "Approve and capture a verified bundle for the exact registered repository and original waiting WorkBuddy task attempt. Commits and versions must match current observations. Replays recover the same intake receipt. Intake does not dispatch a Runner, record test evidence, or accept the delivery."
	op["x-request-schema-coverage"] = "exact path ID and closed body; no query; current project approval, task/target versions, source provenance and idempotency are enforced by the server"
	parameters := op["parameters"].([]any)
	kept := parameters[:0]
	for _, raw := range parameters {
		if raw.(map[string]any)["in"] != "query" {
			kept = append(kept, raw)
		}
	}
	op["parameters"] = kept
	responses := toolContractMap(op, "responses")
	responses["404"] = map[string]any{"description": "Task or registered repository source unavailable in this project"}
	responses["429"] = map[string]any{"description": "Registered repository source export is busy"}
	responses["502"] = map[string]any{"description": "Registered repository provider response is unavailable or invalid"}
	responses["503"] = map[string]any{"description": "Registered repository source export or bundle storage is unavailable"}

	for _, model := range []string{requestModel, responseModel} {
		toolContractMap(schemas, model)["additionalProperties"] = false
		for _, key := range []string{"project_id", "run_id", "target_id"} {
			field := toolContractMap(schemas, model, "properties", key)
			field["minLength"], field["maxLength"] = 1, 64
		}
		for _, key := range []string{"task_version", "target_version"} {
			field := toolContractMap(schemas, model, "properties", key)
			field["minimum"], field["maximum"] = 1, 2147483647
		}
	}
	const oid = `^(?:[a-f0-9]{40}|[a-f0-9]{64})$`
	for _, key := range []string{"expected_base", "expected_head"} {
		toolContractMap(schemas, requestModel, "properties", key)["pattern"] = oid
	}
	key := toolContractMap(schemas, requestModel, "properties", "idempotency_key")
	key["minLength"], key["maxLength"], key["x-max-utf8-bytes"] = 8, 128, 128
	properties := toolContractMap(schemas, responseModel, "properties")
	for _, field := range properties {
		field.(map[string]any)["readOnly"] = true
	}
	for _, key := range []string{"base_commit", "head_commit"} {
		toolContractMap(properties, key)["pattern"] = oid
	}
	toolContractMap(properties, "attempt")["minimum"] = 1
	toolContractMap(properties, "bundle_sha256")["pattern"] = `^[a-f0-9]{64}$`
	size := toolContractMap(properties, "size_bytes")
	size["minimum"], size["maximum"] = 1, cc.MaxBundleBytes
	toolContractMap(properties, "approved_at")["format"] = "date-time"
}

func taskDeliveryRepositoryDispatchContracts(add operationAdder, api, schemas map[string]any) {
	const model = "TaskDeliveryRepositoryDispatchPreviewRequest"
	op := add(taskDeliveryRepositoryDispatchPath, "post", "TaskDispatchPlan", false, true)
	successStatus(op, "201")
	browserOnly(op)
	request(op, model)
	op["description"] = "A current browser project approver previews a managed plan for one exact retained repository intake. The plan binds a fixed import profile and a different Worker's independent JUnit profile to the published task cases. Preview reserves a plan only."
	op["x-request-schema-coverage"] = "exact task and intake path IDs and closed body; no query; current browser approval, original task basis, intake and independent test authority are server-enforced"
	op["x-query-parameters-forbidden"] = true
	op["x-response-cache-control"] = "no-store"
	identifier := func() map[string]any { return map[string]any{"type": "string", "minLength": 1, "maxLength": 64} }
	op["parameters"] = []any{
		map[string]any{"in": "path", "name": "id", "required": true, "schema": identifier()},
		map[string]any{"in": "path", "name": "intake", "required": true, "schema": identifier()},
	}
	toolContractMap(op, "responses")["404"] = map[string]any{"description": "Task, retained intake or selected profile is unavailable in this project"}
	toolContractMap(schemas, model)["additionalProperties"] = false
	fields := toolContractMap(schemas, model, "properties")
	for _, name := range []string{"project_id", "producer_profile_id", "test_profile_id"} {
		field := toolContractMap(fields, name)
		field["minLength"], field["maxLength"] = 1, 64
	}
	for _, name := range []string{"expected_task_version", "expected_producer_profile_version", "expected_test_profile_version"} {
		field := toolContractMap(fields, name)
		field["minimum"], field["maximum"] = 1, 2147483647
	}
	key := toolContractMap(fields, "idempotency_key")
	key["minLength"], key["maxLength"], key["x-max-utf8-bytes"] = 8, 128, 128

	start := toolContractMap(api, "paths", "/task-dispatch-plans/{id}/start", "post")
	start["description"] = start["description"].(string) + " A plan for an approved repository intake requires current browser project approval on initial start and receipt recovery. The original WorkBuddy attempt and retained intake are rechecked in the transaction before the task changes runs."
	for _, route := range []struct{ namespace, security, scope string }{{"runner", "runnerAuth", "runner"}, {"pool-runner", "poolRunnerAuth", "pool"}} {
		bundle := add("/"+route.namespace+"/repository-source-intakes/{id}/bundle", "get", "", false, false)
		successStatus(bundle, "200")
		bundle["description"] = "Download verified retained Git bundle bytes for the exact currently leased repository import producer. Project, worker, dispatch, attempt, fence, lease and task binding are checked before and after the storage read."
		bundle["x-request-schema-coverage"] = "exact intake ID, required single project_id and X-Suma-Assignment; current producer lease and scope are server-enforced"
		bundle["x-machine-only"], bundle["x-read-only"], bundle["x-response-cache-control"] = true, true, "no-store"
		bundle["x-required-token-scope"] = route.scope
		bundle["security"] = []any{map[string]any{route.security: []string{}}}
		bundle["parameters"] = []any{
			map[string]any{"in": "path", "name": "id", "required": true, "schema": identifier()},
			map[string]any{"in": "query", "name": "project_id", "required": true, "schema": identifier()},
			map[string]any{"in": "header", "name": "X-Suma-Assignment", "required": true, "description": "Unpadded base64url JSON CandidateAssignmentRef for the exact producer lease, with complete Pool identity when applicable.", "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 16384}},
		}
		responses := toolContractMap(bundle, "responses")
		responses["404"] = map[string]any{"description": "Retained intake is unavailable to the current producer"}
		responses["200"] = map[string]any{"description": "Verified retained Git bundle, at most 64 MiB", "content": map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary", "maxLength": cc.MaxBundleBytes}}}}
	}
}
