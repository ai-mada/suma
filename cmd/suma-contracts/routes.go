package main

import "strings"

type operationAdder func(path, method, model string, list, body bool) map[string]any

func projectScoped(path string) bool {
	return !strings.HasPrefix(path, "/hooks/") && !strings.HasPrefix(path, "/provider/") && !strings.HasPrefix(path, "/auth/") && path != "/projects" && path != "/organizations" && !strings.HasPrefix(path, "/organization/") && path != "/teams" && !strings.HasPrefix(path, "/teams/")
}
func queryParameter(name, description string, value map[string]any) map[string]any {
	return map[string]any{"in": "query", "name": name, "description": description, "schema": value}
}
func appendQuery(op map[string]any, parameter map[string]any) {
	op["parameters"] = append(op["parameters"].([]any), parameter)
}
func request(op map[string]any, model string, extraRequired ...string) {
	value := map[string]any{"$ref": "#/components/schemas/" + model}
	if len(extraRequired) > 0 {
		value = map[string]any{"allOf": []any{value, map[string]any{"type": "object", "required": extraRequired}}}
	}
	op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": value}}}
	op["x-request-schema-coverage"] = "audited wire fields and basic constraints; cross-field, authorization, receipt and state gates remain server-enforced"
}
func browserOnly(op map[string]any) {
	op["security"] = []any{map[string]any{"cookieAuth": []string{}}}
	op["x-disallowed-token-scopes"] = []string{"api", "agent", "runner"}
}
func successStatus(op map[string]any, status string) {
	responses := op["responses"].(map[string]any)
	other := "201"
	if status == "201" {
		other = "200"
	}
	delete(responses, other)
}
func pageParameters(op map[string]any) {
	appendQuery(op, queryParameter("limit", "Task window size, at most 200; default 200.", map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 200}))
	appendQuery(op, queryParameter("cursor", "Opaque cursor bound to the current organization, project and (for snapshots) baseline.", map[string]any{"type": "string"}))
}
func weekParameter(op map[string]any) {
	appendQuery(op, queryParameter("week_start", "YYYY-MM-DD Monday; omitted reads use the current UTC week.", map[string]any{"type": "string", "format": "date"}))
}
func extendContracts(add operationAdder, paths map[string]any) {
	get := func(path string) map[string]any { return paths[path].(map[string]any)["get"].(map[string]any) }
	post := func(path string) map[string]any { return paths[path].(map[string]any)["post"].(map[string]any) }
	// Existing session endpoints share the exact same response envelope as switch
	// and public invitation acceptance, and reject scoped project credentials.
	for _, path := range []string{"/auth/setup", "/auth/login", "/auth/session"} {
		method := "post"
		if path == "/auth/session" {
			method = "get"
		}
		op := paths[path].(map[string]any)[method].(map[string]any)
		for _, status := range []string{"200", "201"} {
			response := op["responses"].(map[string]any)[status].(map[string]any)
			response["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/SessionResponse"}}}
		}
		if path == "/auth/session" {
			browserOnly(op)
		}
	}
	browserOnly(post("/auth/tokens"))
	op := add("/auth/invitations/accept", "post", "SessionResponse", false, true)
	op["security"] = []any{}
	op["x-disallowed-token-scopes"] = []string{"api", "agent", "runner"}
	op["description"] = "Public one-time, email-bound invitation acceptance. Token and password are JSON body fields, never query parameters. Existing accounts must prove their current password; they are not overwritten. Consumed, expired or revoked tokens fail. Lost acceptance receipts recover through ordinary login, not automatic token replay. Sets the browser session cookie. No email is sent."
	request(op, "InvitationAcceptRequest")
	successStatus(op, "200")
	op = add("/auth/switch", "post", "SessionResponse", false, true)
	browserOnly(op)
	request(op, "SwitchOrganizationRequest")
	successStatus(op, "200")
	op["description"] = "Re-checks live organization membership and issues a browser session in the selected organization. Does not infer project access from team membership."
	for _, entry := range []struct{ path, model string }{{"/organizations", "OrganizationSummary"}, {"/organization/members", "OrganizationMember"}, {"/organization/invitations", "InvitationMetadata"}, {"/teams", "Team"}, {"/teams/{id}/members", "TeamMember"}, {"/project-access", "ProjectGrant"}} {
		op = add(entry.path, "get", entry.model, true, false)
		browserOnly(op)
		successStatus(op, "200")
		// Governance lists are complete {items,total}, without project cursor fields.
		for _, status := range []string{"200"} {
			response := op["responses"].(map[string]any)[status].(map[string]any)
			envelope := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
			delete(envelope["properties"].(map[string]any), "next_cursor")
		}
	}
	get("/organization/invitations")["description"] = "Invitation metadata only. Never returns plaintext tokens or token hashes. Expiry is evaluated when read."
	get("/project-access")["description"] = "Explicit project grants and removed tombstones with stable versions. Organization owners/admins retain inherited project authority; removing an explicit grant does not remove that inherited authority."
	get("/teams/{id}/members")["description"] = "Live human organization identities and project-scoped Agent/tool resources; includes removed tombstones. Team membership grants no project access."
	for _, entry := range []struct{ path, method, model, input, status string }{
		{"/organizations", "post", "OrganizationSummary", "OrganizationCreateRequest", "201"},
		{"/organization/members/{user_id}", "patch", "OrganizationMemberUpdate", "OrganizationMemberWriteRequest", "200"},
		{"/organization/invitations", "post", "InvitationCreation", "InvitationCreateRequest", "201"},
		{"/organization/invitations/{id}/revoke", "post", "InvitationMetadata", "VersionedOperation", "200"},
		{"/teams", "post", "Team", "TeamWriteRequest", "201"},
		{"/teams/{id}", "patch", "Team", "TeamWriteRequest", "200"},
		{"/teams/{id}/members", "post", "TeamMember", "TeamMemberWriteRequest", "200"},
		{"/project-access", "post", "ProjectGrant", "ProjectAccessWriteRequest", "200"},
	} {
		op = add(entry.path, entry.method, entry.model, false, true)
		browserOnly(op)
		request(op, entry.input)
		successStatus(op, entry.status)
		if entry.path == "/teams/{id}" {
			request(op, entry.input, "expected_version")
		}
		if entry.path == "/teams" {
			request(op, entry.input, "name")
		}
	}
	post("/organizations")["description"] = "Creates another organization owned by the current account; does not switch the active organization."
	post("/organization/invitations")["description"] = "Owner/admin creates a 24-hour email-bound invitation, with optional same-organization project grants. Secret is shown only on first creation; idempotent replay returns metadata, token_revealed=false and recovery guidance without the secret. No email is sent."
	paths["/organization/members/{user_id}"].(map[string]any)["patch"].(map[string]any)["description"] = "Versioned role change, explicit owner-only transfer_owner, or action=remove. Preserves the last owner; removal revokes organization sessions, project grants and human team membership."
	post("/teams/{id}/members")["description"] = "Human resource_id is a live organization user_id with empty project_id; Agent/tool resource_id is a real matching-kind project resource. Expected version is 0 for a new identity or the current tombstone version for re-addition. action=remove requires the current version."
	post("/project-access")["description"] = "Versioned explicit grant or action=remove. Use current tombstone version when re-granting; a new grant uses 0. Authority and role ceilings are checked server-side."
	op = add("/planning", "get", "PlanningResponse", false, false)
	pageParameters(op)
	weekParameter(op)
	successStatus(op, "200")
	appendQuery(op, queryParameter("task_id", "Optional exact task lookup for conflict recovery. total and metrics remain whole-project; the task window contains only this task and next_cursor is empty.", map[string]any{"type": "string"}))
	op["description"] = "Tasks contain canonical Record fields and an independent plan version. coverage distinguishes the loaded task window from whole-project metrics. Actual delivery progress is accepted task state; execution progress is separate. Unknown estimates and capacity are null, not zero."
	for _, path := range []string{"/iterations", "/milestones"} {
		op = add(path, "get", "PlanningPeriod", true, false)
		successStatus(op, "200")
		for _, status := range []string{"200"} {
			response := op["responses"].(map[string]any)[status].(map[string]any)
			envelope := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
			delete(envelope["properties"].(map[string]any), "next_cursor")
		}
		op = add(path, "post", "PlanningPeriod", false, true)
		fields := []string{"title"}
		if path == "/iterations" {
			fields = append(fields, "start_date", "end_date")
		} else {
			fields = append(fields, "target_date")
		}
		request(op, "PlanningPeriodWriteRequest", fields...)
		successStatus(op, "201")
		op = add(path+"/{id}", "patch", "PlanningPeriod", false, true)
		request(op, "PlanningPeriodWriteRequest", "expected_version")
		successStatus(op, "200")
		op["description"] = "Completing a plan requires every linked task to be accepted. A completed plan must be reopened before changing its linked task scope or schedule. Dates and task acceptance are independent facts."
	}
	op = add("/planning/schedule", "post", "PlannedTask", true, true)
	request(op, "ScheduleRequest")
	successStatus(op, "200")
	op["description"] = "Atomic batch of 1–200 complete task plans. Each item requires both expected_task_version and independent plan expected_version (0 when absent). Preserves actual task acceptance and delivery scope."
	// respondList returns the updated items and count, not a cursor.
	response := op["responses"].(map[string]any)["200"].(map[string]any)
	envelope := response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	delete(envelope["properties"].(map[string]any), "next_cursor")
	op = add("/planning/capacity", "get", "CapacityEntry", true, false)
	weekParameter(op)
	successStatus(op, "200")
	response = op["responses"].(map[string]any)["200"].(map[string]any)
	envelope = response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	delete(envelope["properties"].(map[string]any), "next_cursor")
	op = add("/planning/capacity", "post", "CapacityEntry", false, true)
	request(op, "CapacityWriteRequest")
	successStatus(op, "200")
	op["description"] = "member_id is a project /members resource ID, not an organization user ID. Monday-based available_hours is nullable (unknown), with 0 explicitly meaning no available time. Planning hours do not represent machine execution quotas."
	op = add("/planning/baselines", "get", "BaselineSummary", true, false)
	successStatus(op, "200")
	response = op["responses"].(map[string]any)["200"].(map[string]any)
	envelope = response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	delete(envelope["properties"].(map[string]any), "next_cursor")
	op["description"] = "Immutable baseline metadata only; request the selected baseline detail to read a paginated snapshot and whole-project comparison."
	op = add("/planning/baselines", "post", "BaselineSummary", false, true)
	request(op, "BaselineCaptureRequest")
	successStatus(op, "201")
	op["description"] = "Explicitly captures the complete current task/plan scope atomically, including estimates and actual statuses. Does not truncate to the currently loaded UI window."
	op = add("/planning/baselines/{id}", "get", "BaselineDetail", false, false)
	pageParameters(op)
	successStatus(op, "200")
	op["description"] = "Immutable snapshot.tasks is paginated; snapshot.total is the full captured task count. comparison is evaluated across the full current project scope as_of the response, not only the loaded snapshot page."
	op = add("/runner/progress", "post", "Acknowledgement", false, true)
	request(op, "RunnerProgressRequest")
	successStatus(op, "200")
	op["description"] = "Durably ACKs bounded actual output for the current assignment attempt, worker and fence. Repeating the same operation_id and content replays the receipt. Emits node_progress with sanitized data.message; cumulative output snapshots do not imply completion percentage or task acceptance."
	for _, path := range []string{"/runner/claim", "/runner/heartbeat", "/runner/results", "/runner/progress"} {
		op = post(path)
		op["security"] = []any{map[string]any{"runnerAuth": []string{}}}
		op["x-required-token-scope"] = "runner"
	}
}

func enhanceSchemas(schemas map[string]any) {
	field := func(model, name string, attrs map[string]any) {
		properties := schemas[model].(map[string]any)["properties"].(map[string]any)
		value := properties[name].(map[string]any)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for _, model := range []string{"InvitationMetadata", "InvitationCreation"} {
		schemas[model].(map[string]any)["additionalProperties"] = false
		field(model, "email", map[string]any{"format": "email"})
		field(model, "role", map[string]any{"enum": []string{"admin", "member"}})
		field(model, "status", map[string]any{"enum": []string{"pending", "accepted", "revoked", "expired"}})
	}
	field("InvitationCreation", "token", map[string]any{"readOnly": true, "description": "First creation only. Never included in listing or replay; retain only in component memory until dismissed."})
	field("InvitationAcceptRequest", "token", map[string]any{"writeOnly": true, "minLength": 64, "maxLength": 64, "description": "One-time secret in JSON body only; UI accepts manual input or URL fragment, never query/log/localStorage."})
	field("InvitationAcceptRequest", "password", map[string]any{"writeOnly": true, "minLength": 1, "maxLength": 1024, "x-min-utf8-bytes": 12, "x-max-utf8-bytes": 1024, "description": "Existing account password proof; new account password. Server requires 12–1024 UTF-8 bytes, which is not a JSON Schema character count."})
	for _, model := range []string{"InvitationAcceptRequest", "InvitationCreateRequest"} {
		field(model, "email", map[string]any{"format": "email", "maxLength": 254})
	}
	field("InvitationCreateRequest", "role", map[string]any{"enum": []string{"member", "admin"}})
	field("InvitationCreateRequest", "grants", map[string]any{"maxItems": 100, "description": "Unique same-organization project grants; server validates project ownership."})
	field("InvitationGrant", "role", map[string]any{"enum": []string{"admin", "reviewer", "developer", "viewer"}})
	field("Team", "status", map[string]any{"enum": []string{"active", "archived"}})
	field("TeamWriteRequest", "status", map[string]any{"enum": []string{"active", "archived"}})
	for _, model := range []string{"TeamMember", "TeamMemberWriteRequest"} {
		field(model, "resource_kind", map[string]any{"enum": []string{"human", "agent", "tool"}})
	}
	field("TeamMemberWriteRequest", "project_id", map[string]any{"description": "Empty/omitted for humans; required real project ID for Agent and tool resources."})
	field("OrganizationMemberWriteRequest", "action", map[string]any{"enum": []string{"", "remove", "transfer_owner"}})
	field("OrganizationMemberWriteRequest", "role", map[string]any{"enum": []string{"owner", "admin", "member"}, "description": "Required except remove. owner requires owner-only transfer_owner action."})
	for _, model := range []string{"TeamMemberWriteRequest", "ProjectAccessWriteRequest"} {
		field(model, "action", map[string]any{"enum": []string{"", "remove"}})
	}
	for _, model := range []string{"TeamMember", "TeamMemberWriteRequest"} {
		field(model, "role", map[string]any{"enum": []string{"lead", "member", "observer"}})
	}
	field("ProjectAccessWriteRequest", "role", map[string]any{"enum": []string{"admin", "reviewer", "developer", "viewer"}, "description": "Required except remove; cannot exceed caller authority."})
	field("ProjectGrant", "role", map[string]any{"enum": []string{"", "admin", "reviewer", "developer", "viewer"}, "description": "Empty for removed tombstones."})
	for _, model := range []string{"PlanningPeriod", "PlanningPeriodWriteRequest"} {
		field(model, "status", map[string]any{"enum": []string{"planned", "active", "completed"}})
	}
	field("TaskPlan", "version", map[string]any{"minimum": 0, "description": "Independent plan version; 0 means no stored plan."})
	field("ScheduleChange", "expected_task_version", map[string]any{"minimum": 1})
	field("ScheduleChange", "expected_version", map[string]any{"minimum": 0})
	field("ScheduleRequest", "changes", map[string]any{"minItems": 1, "maxItems": 200})
	field("ScheduleChange", "estimated_hours", map[string]any{"description": "Required complete field. null is unknown; explicit 0 is known zero. Server accepts 0–100000 hours."})
	for _, model := range []string{"CapacityEntry", "CapacityWriteRequest"} {
		field(model, "week_start", map[string]any{"format": "date", "description": "Valid Monday in YYYY-MM-DD format."})
	}
	field("CapacityWriteRequest", "available_hours", map[string]any{"description": "null is unknown; numeric capacity must be 0–168 hours."})
	field("PlanningCoverage", "task_metrics", map[string]any{"enum": []string{"whole_project"}})
	field("PlanningCoverage", "timeline", map[string]any{"enum": []string{"loaded_tasks"}})
	field("PlanningMetrics", "acceptance_percent", map[string]any{"description": "100 * accepted_tasks / total_tasks; null when total_tasks is zero."})
	field("RunnerProgressRequest", "message", map[string]any{"minLength": 1, "maxLength": 8192, "x-max-utf8-bytes": 8192, "description": "Actual cumulative output tail. Server strips ANSI/control characters and redacts credential-shaped text; 1–8192 UTF-8 bytes."})
	mapAtField := func(model, name string) map[string]any {
		return schemas[model].(map[string]any)["properties"].(map[string]any)[name].(map[string]any)
	}
	for _, pair := range []struct {
		model, name string
		maximum     int
	}{{"ScheduleChange", "estimated_hours", 100000}, {"CapacityWriteRequest", "available_hours", 168}} {
		branch := mapAtField(pair.model, pair.name)["anyOf"].([]any)[0].(map[string]any)
		branch["minimum"], branch["maximum"] = 0, pair.maximum
	}
	mapAtField("Event", "data")["x-known-event-data"] = map[string]any{"node_progress": map[string]any{"$ref": "#/components/schemas/NodeProgressData"}}
}
