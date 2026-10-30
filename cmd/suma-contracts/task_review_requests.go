package main

import "github.com/ai-mada/suma/internal/platform"

func taskReviewRequestModels() []any {
	return []any{platform.TaskReviewTarget{}, platform.TaskReviewTargetPage{}, platform.TaskReviewDecision{}, platform.TaskReviewRequest{}, platform.TaskReviewRequestPage{}, platform.TaskReviewRequestDetail{}, platform.TaskReviewRequestCreate{}, platform.TaskReviewRequestCommand{}}
}

// Responsibility requests locate a delivery and its intended reviewer. Only
// the existing formal Review command can produce an acceptance decision.
func taskReviewRequestContracts(add operationAdder, api map[string]any) {
	targets := add("/task-review-targets", "get", "TaskReviewTargetPage", false, false)
	list := add("/task-review-requests", "get", "TaskReviewRequestPage", false, false)
	detail := add("/task-review-requests/{id}", "get", "TaskReviewRequestDetail", false, false)
	create := add("/task-review-requests", "post", "TaskReviewRequest", false, true)
	command := add("/task-review-requests/{id}/commands", "post", "TaskReviewRequest", false, true)
	for _, op := range []map[string]any{targets, list, detail, create, command} {
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current human/user-API authority; canonical current approver qualification is independent of responsibility. Requests and notifications commit atomically, never grant Review permission or imply test eligibility. Only an exact formal Review can close a matching request. Original operations recover after current authorization."
	}
	for _, op := range []map[string]any{targets, list, detail} {
		successStatus(op, "200")
		op["x-read-only"] = true
	}
	for _, op := range []map[string]any{targets, list} {
		task := queryParameter("task_id", "Exact current authorized task.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
		task["required"] = true
		appendQuery(op, task)
		appendQuery(op, queryParameter("limit", "Bounded page size; total covers the current complete scope.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
		appendQuery(op, queryParameter("cursor", "Canonical current-identity/project/task/page-size continuation. History order uses fixed9-fraction UTC creation time and ID, both descending.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
	}
	appendQuery(targets, queryParameter("q", "Literal current-approver search, exclusive with resource_ids.", map[string]any{"type": "string", "minLength": 1, "maxLength": 200}))
	appendQuery(targets, queryParameter("resource_ids", "At most20 unique comma-separated resources; current-qualified selected subset in original order, exclusive with q/cursor.", map[string]any{"type": "string", "minLength": 1, "maxLength": 1299}))
	successStatus(create, "201")
	successStatus(command, "200")
	request(create, "TaskReviewRequestCreate")
	request(command, "TaskReviewRequestCommand")
	for _, op := range []map[string]any{create, command} {
		var parameters []any
		for _, v := range op["parameters"].([]any) {
			if v.(map[string]any)["in"] != "query" {
				parameters = append(parameters, v)
			}
		}
		op["parameters"] = parameters
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"TaskReviewTarget", "TaskReviewTargetPage", "TaskReviewDecision", "TaskReviewRequest", "TaskReviewRequestPage", "TaskReviewRequestDetail", "TaskReviewRequestCreate", "TaskReviewRequestCommand"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		if name != "TaskReviewRequestCreate" && name != "TaskReviewRequestCommand" {
			for _, v := range toolContractMap(model, "properties") {
				v.(map[string]any)["readOnly"] = true
			}
		}
	}
	field := func(model, key string, attrs map[string]any) {
		for k, v := range attrs {
			toolContractMap(schemas, model, "properties", key)[k] = v
		}
	}
	for _, name := range []string{"TaskReviewRequestCreate", "TaskReviewRequestCommand"} {
		field(name, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128})
	}
	for _, key := range []string{"task_id", "reviewer_resource_id"} {
		field("TaskReviewRequestCreate", key, map[string]any{"minLength": 1, "maxLength": 64})
	}
	field("TaskReviewRequestCreate", "run_id", map[string]any{"minLength": 1, "maxLength": 100})
	for _, key := range []string{"expected_task_version", "expected_run_version"} {
		field("TaskReviewRequestCreate", key, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("TaskReviewRequestCommand", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("TaskReviewRequestCommand", "action", map[string]any{"enum": []string{"cancel"}})
	for _, pair := range [][2]string{{"TaskReviewRequestCreate", "note"}, {"TaskReviewRequestCommand", "reason"}} {
		field(pair[0], pair[1], map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 8000, "description": "Nonblank valid Unicode without NUL; preserve original text."})
	}
	for _, name := range []string{"TaskReviewTargetPage", "TaskReviewRequestPage"} {
		field(name, "items", map[string]any{"maxItems": 50})
		field(name, "limit", map[string]any{"minimum": 1, "maximum": 50})
	}
	field("TaskReviewRequest", "state", map[string]any{"enum": []string{"pending", "cancelled", "closed"}})
	field("TaskReviewDecision", "decision", map[string]any{"enum": []string{"accepted", "rejected"}})
	field("TaskReviewRequest", "binding_current", map[string]any{"description": "Only direct task version/basis and run identity/version/state consistency. Never acceptance eligibility or proof of current full quality gates."})
	field("NotificationInboxItem", "kind", map[string]any{"enum": []string{"task_assigned", "task_mentioned", "task_review_requested"}})
	field("NotificationInboxItem", "review_request_id", map[string]any{"minLength": 1, "maxLength": 64, "description": "Exact intact responsibility request with matching recipient and original notification relationship; no authority is granted."})
}
