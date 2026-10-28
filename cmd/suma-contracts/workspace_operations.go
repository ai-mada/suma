package main

import wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"

// The parent-only envelope is distinct from SDK activity. Keeping both concrete
// requests in a oneOf prevents generated clients from mixing their provenance.
func workspaceOperationContracts(api map[string]any) {
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, attr := range attrs {
			value[key] = attr
		}
	}
	for _, model := range []string{"WorkspaceOperationEvent", "WorkspaceOperationCoverage", "HostOperationProgressRequest", "PoolHostOperationProgressRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"RunnerProgressRequest", "PoolRunnerProgressRequest"} {
		toolContractMap(schemas, model)["not"] = map[string]any{"required": []string{"host_operation"}}
	}
	for _, route := range []struct{ path, sdk, host string }{
		{"/runner/progress", "RunnerProgressRequest", "HostOperationProgressRequest"},
		{"/pool-runner/progress", "PoolRunnerProgressRequest", "PoolHostOperationProgressRequest"},
	} {
		op := toolContractMap(api, "paths", route.path, "post")
		toolContractMap(op, "requestBody", "content", "application/json")["schema"] = map[string]any{"oneOf": []any{
			map[string]any{"$ref": "#/components/schemas/" + route.sdk},
			map[string]any{"$ref": "#/components/schemas/" + route.host},
		}}
		op["description"] = "Authenticated original-assignment progress: SDK output/activity or a distinct, closed Go workspace-host transition. Host operation_id equals host_operation.event_id. Current credentials, profile and original attempt scope are checked before receipt recovery. Duplicate original facts recover the same receipt; changed bodies conflict. Host tracing never completes a node, grants a retry, or changes test/approval authority."
	}
	const event = "WorkspaceOperationEvent"
	field(event, "protocol", map[string]any{"const": wo.Protocol})
	field(event, "source", map[string]any{"const": wo.Source})
	field(event, "schema_version", map[string]any{"const": 1})
	field(event, "sequence", map[string]any{"minimum": 1, "maximum": wo.MaxRecords})
	field(event, "revision", map[string]any{"enum": []int{1, 2}})
	field(event, "action", map[string]any{"enum": []string{"list", "read", "write", "edit", "remove", "check", "merge_status", "merge_continue", "seal"}})
	field(event, "phase", map[string]any{"enum": []string{"started", "completed", "failed", "unknown"}})
	for _, name := range []string{"event_id", "request_sha256", "policy_sha256", "profile_sha256", "result_sha256"} {
		field(event, name, map[string]any{"pattern": "^[0-9a-f]{64}$"})
	}
	for _, name := range []string{"operation_id", "effect_operation_id", "check_id"} {
		field(event, name, map[string]any{"pattern": "^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$"})
	}
	for _, name := range []string{"generation_before", "generation_after"} {
		field(event, name, map[string]any{"minimum": 0, "maximum": wo.MaxGeneration})
	}
	for _, name := range []string{"started_at", "recorded_at", "completed_at"} {
		field(event, name, map[string]any{"format": "date-time", "maxLength": 35, "pattern": "Z$"})
	}
	field(event, "path", map[string]any{"minLength": 1, "maxLength": 1024, "description": "Optional validated normalized relative locator; absent for root or unsafe arguments. Never a file-open capability."})
	field(event, "error_code", map[string]any{"pattern": "^[a-z][a-z0-9_]{0,79}$"})
	field(event, "exit_code", map[string]any{"minimum": -2147483648, "maximum": 2147483647})
	toolContractMap(schemas, event)["x-max-utf8-bytes"] = wo.MaxEventBytes
	toolContractMap(schemas, event)["description"] = "Durable safe metadata from the authenticated Go workspace host, separate from SDK claims. revision1 is started, revision2 is terminal. IDs bind the original assignment; source text, command args, output, environment, endpoint and credentials are forbidden. completed check is an observed completion, whose exit_code/stopped/unknown status must be considered separately."
	toolContractMap(schemas, event)["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"phase": map[string]any{"const": "started"}}}, "then": map[string]any{"properties": map[string]any{"revision": map[string]any{"const": 1}}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"completed_at"}}, map[string]any{"required": []string{"exit_code"}}, map[string]any{"required": []string{"stopped"}}, map[string]any{"required": []string{"error_code"}}, map[string]any{"required": []string{"result_sha256"}}}}}, "else": map[string]any{"properties": map[string]any{"revision": map[string]any{"const": 2}}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"phase": map[string]any{"enum": []string{"completed", "failed"}}}}, "then": map[string]any{"required": []string{"completed_at"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"phase": map[string]any{"enum": []string{"failed", "unknown"}}}}, "then": map[string]any{"required": []string{"error_code"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"phase": map[string]any{"const": "unknown"}}}, "then": map[string]any{"not": map[string]any{"required": []string{"completed_at"}}}},
	}
	const coverage = "WorkspaceOperationCoverage"
	for _, name := range []string{"invocation_count", "pending_count"} {
		field(coverage, name, map[string]any{"minimum": 0, "maximum": wo.MaxCalls})
	}
	field(coverage, "record_count", map[string]any{"minimum": 0, "maximum": wo.MaxRecords})
	field(coverage, "max_invocations", map[string]any{"const": wo.MaxCalls})
	field(coverage, "max_records", map[string]any{"const": wo.MaxRecords})
	field(coverage, "state", map[string]any{"enum": []string{"complete", "partial", "unknown"}})
	toolContractMap(schemas, coverage)["description"] = "Local durable capture only: record_count = 2*invocation_count - pending_count. Does not prove HTTP acknowledgement, complete server history or loaded page coverage. Missing coverage means not provided, not zero."
}
