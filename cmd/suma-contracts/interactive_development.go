package main

import "github.com/ai-mada/suma/internal/platform"

func interactiveDevelopmentModels() []any {
	return []any{platform.TaskDiscussionDevelopmentCreateRequest{}, platform.TaskDiscussionDevelopmentPlan{}, platform.WorkspaceTerminalSession{}, platform.WorkspaceTerminalCreateRequest{}, platform.WorkspaceTerminalCommandRequest{}, platform.WorkspaceTerminalCommandReceipt{}, platform.WorkspaceTerminalOutputPage{}, platform.WorkspaceTerminalPage{}, platform.WorkspaceTerminalRecoveryRequest{}, platform.WorkspaceTerminalRecoveryView{}, platform.WorkspaceTerminalRecoveryReceipt{}, platform.WorkspaceTerminalCandidatePreviewRequest{}, platform.WorkspaceTerminalCandidatePlan{}, platform.WorkspaceTerminalCandidatePlanPage{}}
}

func interactiveDevelopmentContracts(add operationAdder, api map[string]any) {
	for _, r := range []struct{ path, method, input, output, status string }{
		{"/task-discussion-development-plans", "post", "TaskDiscussionDevelopmentCreateRequest", "TaskDiscussionDevelopmentPlan", "201"},
		{"/task-discussion-development-plans/{id}", "get", "", "TaskDiscussionDevelopmentPlan", "200"},
		{"/task-discussion-development-plans/{id}/start", "post", "TaskDispatchStartRequest", "Run", "201"},
		{"/workspace/terminal-sessions", "get", "", "WorkspaceTerminalPage", "200"},
		{"/workspace/terminal-sessions", "post", "WorkspaceTerminalCreateRequest", "WorkspaceTerminalSession", "201"},
		{"/workspace/terminal-sessions/{id}", "get", "", "WorkspaceTerminalSession", "200"},
		{"/workspace/terminal-sessions/{id}/output", "get", "", "WorkspaceTerminalOutputPage", "200"},
		{"/workspace/terminal-sessions/{id}/commands", "post", "WorkspaceTerminalCommandRequest", "WorkspaceTerminalCommandReceipt", "200"},
		{"/workspace/terminal-sessions/{id}/recovery", "get", "", "WorkspaceTerminalRecoveryView", "200"},
		{"/workspace/terminal-sessions/{id}/recover", "post", "WorkspaceTerminalRecoveryRequest", "WorkspaceTerminalRecoveryReceipt", "200"},
		{"/workspace/terminal-sessions/{id}/candidate-plans", "post", "WorkspaceTerminalCandidatePreviewRequest", "WorkspaceTerminalCandidatePlan", "201"},
		{"/workspace/terminal-sessions/{id}/candidate-plans", "get", "", "WorkspaceTerminalCandidatePlanPage", "200"},
		{"/workspace/terminal-candidate-plans/{id}", "get", "", "WorkspaceTerminalCandidatePlan", "200"},
		{"/workspace/terminal-candidate-plans/{id}/start", "post", "TaskDispatchStartRequest", "Run", "201"},
	} {
		op := add(r.path, r.method, r.output, false, r.input != "")
		successStatus(op, r.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		if r.input != "" {
			request(op, r.input)
		} else {
			op["x-read-only"] = true
		}
		if r.path == "/workspace/terminal-sessions" && r.method == "get" {
			p := queryParameter("task_id", "Exact selected task; terminal creation never accepts an arbitrary local directory.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
			p["required"] = true
			appendQuery(op, p)
		}
		if r.path == "/workspace/terminal-sessions/{id}/output" {
			appendQuery(op, queryParameter("after", "Last original output chunk parsed by xterm; missing retained history is an explicit gap.", map[string]any{"type": "integer", "minimum": 0}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskDiscussionDevelopmentCreateRequest", "TaskDiscussionDevelopmentPlan", "WorkspaceTerminalSession", "WorkspaceTerminalCreateRequest", "WorkspaceTerminalCommandRequest", "WorkspaceTerminalCommandReceipt", "WorkspaceTerminalOutputPage", "WorkspaceTerminalPage", "WorkspaceTerminalInputReceipt", "WorkspaceTerminalOutputChunk", "WorkspaceTerminalRecoveryRequest", "WorkspaceTerminalRecoveryView", "WorkspaceTerminalRecoveryReceipt", "WorkspaceTerminalRecoveryInput", "WorkspaceTerminalCandidatePreviewRequest", "WorkspaceTerminalCandidatePlan", "TerminalCandidateSource"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	field := func(model, name string, values map[string]any) {
		for k, v := range values {
			toolContractMap(schemas, model, "properties", name)[k] = v
		}
	}
	for _, model := range []string{"WorkspaceTerminalCreateRequest", "WorkspaceTerminalCommandRequest", "TaskDiscussionDevelopmentCreateRequest", "WorkspaceTerminalRecoveryRequest", "WorkspaceTerminalCandidatePreviewRequest"} {
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "idempotency_key", map[string]any{"minLength": 1, "maxLength": 128})
	}
	for _, p := range []struct {
		name     string
		min, max int
	}{{"columns", 20, 500}, {"rows", 5, 200}, {"timeout_seconds", 60, 3600}, {"expected_task_version", 1, 2147483646}, {"expected_tool_version", 1, 2147483646}} {
		field("WorkspaceTerminalCreateRequest", p.name, map[string]any{"minimum": p.min, "maximum": p.max})
	}
	field("WorkspaceTerminalCommandRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("WorkspaceTerminalCommandRequest", "control_epoch", map[string]any{"const": 1})
	field("WorkspaceTerminalCommandRequest", "action", map[string]any{"enum": []string{"input", "resize", "stop", "ack"}})
	field("WorkspaceTerminalSession", "state", map[string]any{"enum": []string{"queued", "running", "stop_requested", "exited", "stopped", "reconciling"}})
	field("WorkspaceTerminalInputReceipt", "state", map[string]any{"enum": []string{"accepted", "delivered", "delivery_unknown"}})
	field("WorkspaceTerminalRecoveryRequest", "reason", map[string]any{"minLength": 1, "x-max-utf8-bytes": 2000})
	field("WorkspaceTerminalRecoveryRequest", "unknown_inputs", map[string]any{"maxItems": 65536})
	toolContractMap(schemas, "WorkspaceTerminalCandidatePlanPage")["additionalProperties"] = false
	for _, model := range []string{"WorkspaceTerminalSession", "WorkspaceTerminalInputReceipt", "WorkspaceTerminalOutputChunk", "WorkspaceTerminalCommandReceipt", "WorkspaceTerminalOutputPage", "WorkspaceTerminalPage", "TaskDiscussionDevelopmentPlan", "WorkspaceTerminalRecoveryView", "WorkspaceTerminalRecoveryReceipt", "WorkspaceTerminalCandidatePlan"} {
		for _, property := range toolContractMap(schemas, model, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
}
