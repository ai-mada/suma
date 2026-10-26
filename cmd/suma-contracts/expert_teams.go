package main

import "github.com/ai-mada/suma/internal/platform"

func expertTeamModels() []any {
	return []any{platform.ExpertTeamRoleRequest{}, platform.ExpertTeamRole{}, platform.ExpertTeamConfiguration{}, platform.ExpertTeamSaveRequest{}, platform.ExpertTeamCommandRequest{}, platform.ExpertTeamConfigurationPage{}, platform.ExpertTeamPlanItemRequest{}, platform.ExpertTeamPlanCreateRequest{}, platform.ExpertTeamPlanItem{}, platform.ExpertTeamPlan{}, platform.ExpertTeamPlanStartRequest{}, platform.ExpertTeamPlanProgress{}, platform.ExpertTeamDependencyFact{}, platform.ExpertTeamPlanCommandRequest{}, platform.ExpertTeamPlanPage{}}
}

func expertTeamContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/expert-teams", "get", "", "ExpertTeamConfigurationPage", "200"},
		{"/expert-teams/{team_id}", "get", "", "ExpertTeamConfiguration", "200"},
		{"/expert-teams/{team_id}", "post", "ExpertTeamSaveRequest", "ExpertTeamConfiguration", "200"},
		{"/expert-teams/{team_id}/commands", "post", "ExpertTeamCommandRequest", "ExpertTeamConfiguration", "200"},
		{"/expert-teams/{team_id}/revisions/{revision}", "get", "", "ExpertTeamConfiguration", "200"},
		{"/expert-team-plans", "get", "", "ExpertTeamPlanPage", "200"},
		{"/expert-team-plans/{id}/commands", "post", "ExpertTeamPlanCommandRequest", "ExpertTeamPlan", "200"},
		{"/expert-team-plans/{id}/revisions", "get", "", "ExpertTeamPlanPage", "200"},
		{"/expert-team-plans/{id}/revisions/{revision}", "get", "", "ExpertTeamPlan", "200"},
		{"/expert-team-plans", "post", "ExpertTeamPlanCreateRequest", "ExpertTeamPlan", "201"},
		{"/expert-team-plans/{id}", "get", "", "ExpertTeamPlan", "200"},
		{"/expert-team-plans/{id}/start", "post", "ExpertTeamPlanStartRequest", "ExpertTeamPlan", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
		}
		if route.input == "ExpertTeamCommandRequest" {
			browserOnly(op)
		}
		op["description"] = "Current project expert execution templates and versioned dependency delivery plans. Independent tasks are the all-root case. Explicit activation advances successors only after current formal acceptance of this plan predecessor run; success alone is insufficient. Pause/stop halt new work; active runs retain original controls. Revise affects only unstarted sources. No silent code inheritance. Team labels never grant authority. Fixed role/resource/membership/workflow/tool/knowledge versions are rechecked before start. Each task retains a separate run, predecessor acceptance, independent tests and explicit human review. Draft/dispatch writes accept current scoped API writers; publication/retirement require browser approval. Stable operation keys recover original plans/runs without repeating execution."
		if route.method == "get" && (route.path == "/expert-teams" || route.path == "/expert-team-plans" || route.path == "/expert-team-plans/{id}/revisions") {
			appendQuery(op, queryParameter("limit", "Exact integer page size, 1..100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Opaque scope-bound page boundary from next_cursor.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(api, "components", "schemas", model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	requests := []string{"ExpertTeamSaveRequest", "ExpertTeamCommandRequest", "ExpertTeamPlanCreateRequest", "ExpertTeamPlanStartRequest", "ExpertTeamPlanCommandRequest"}
	for _, model := range append(append([]string{}, requests...), "ExpertTeamRoleRequest", "ExpertTeamPlanItemRequest") {
		toolContractMap(api, "components", "schemas", model)["additionalProperties"] = false
	}
	for _, model := range requests {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, model := range []string{"ExpertTeamSaveRequest", "ExpertTeamCommandRequest", "ExpertTeamPlanStartRequest"} {
		minimum := 1
		if model == "ExpertTeamSaveRequest" {
			minimum = 0
		}
		field(model, "expected_version", map[string]any{"minimum": minimum, "maximum": 2147483646})
	}
	for _, pair := range [][2]string{{"ExpertTeamRoleRequest", "expected_resource_version"}, {"ExpertTeamRoleRequest", "workflow_version"}, {"ExpertTeamRoleRequest", "expected_tool_version"}, {"ExpertTeamPlanItemRequest", "expected_task_version"}, {"ExpertTeamPlanCreateRequest", "expected_configuration_version"}} {
		field(pair[0], pair[1], map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("ExpertTeamSaveRequest", "roles", map[string]any{"minItems": 1, "maxItems": 8})
	field("ExpertTeamPlanCreateRequest", "items", map[string]any{"minItems": 1, "maxItems": 16})
	field("ExpertTeamRoleRequest", "knowledge", map[string]any{"maxItems": 8})
	field("ExpertTeamRoleRequest", "key", map[string]any{"pattern": "^[a-z][a-z0-9_-]{0,39}$", "maxLength": 40})
	field("ExpertTeamPlanItemRequest", "role_key", map[string]any{"pattern": "^[a-z][a-z0-9_-]{0,39}$", "maxLength": 40})
	field("ExpertTeamRoleRequest", "title", map[string]any{"minLength": 1, "maxLength": 120, "x-max-utf8-bytes": 120})
	field("ExpertTeamCommandRequest", "action", map[string]any{"enum": []string{"publish", "retire"}})
	field("ExpertTeamCommandRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("ExpertTeamPlanStartRequest", "acknowledge_dispatch", map[string]any{"const": true})
	field("ExpertTeamConfiguration", "state", map[string]any{"enum": []string{"unconfigured", "draft", "published", "retired"}})
	field("ExpertTeamPlanCommandRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("ExpertTeamPlanCommandRequest", "expected_configuration_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("ExpertTeamPlanCommandRequest", "action", map[string]any{"enum": []string{"pause", "resume", "stop", "revise"}})
	field("ExpertTeamPlanCommandRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("ExpertTeamPlanCommandRequest", "acknowledge", map[string]any{"const": true, "description": "Explicit activation/control decision; stop prevents new tasks and does not report active processes stopped."})
	field("ExpertTeamPlanCommandRequest", "items", map[string]any{"minItems": 1, "maxItems": 16, "description": "Explicit unstarted changes only; started task/run/source tuples remain immutable."})
	field("ExpertTeamPlanItem", "state", map[string]any{"enum": []string{"waiting_dependencies", "ready", "queued", "running", "awaiting_review", "accepted", "blocked", "stopped"}})
	field("ExpertTeamPlan", "revision", map[string]any{"minimum": 1})
	field("ExpertTeamPlan", "graph_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	field("ExpertTeamPlan", "state", map[string]any{"enum": []string{"ready", "active", "paused", "completed", "stopped"}})
}
