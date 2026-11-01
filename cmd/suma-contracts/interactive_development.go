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
	// A discussion handoff uses the ordinary dispatcher's model policy. Fixed
	// tool profiles may require neither selection; supplying both is forbidden
	// by the shared schema generator and the current dispatch validation.
	toolContractMap(schemas, "TaskDiscussionDevelopmentCreateRequest")["description"] = "Reviewed discussion handoff to a currently enabled writable development profile. Model selection is adapter-specific; fixed tool profiles may omit both selections. The server rechecks the original reply, knowledge pins and current dispatch authority before freezing the plan."
	field("TaskDiscussionDevelopmentCreateRequest", "model_execution_selection", map[string]any{"description": "Platform model selection when required by the selected profile. Mutually exclusive with codex_execution_selection; absence is valid for a supported fixed-configuration tool."})
	field("TaskDiscussionDevelopmentCreateRequest", "codex_execution_selection", map[string]any{"description": "Original Codex catalog selection when required by the selected profile. Mutually exclusive with model_execution_selection; Codex usage remains observe-only and does not imply a provider-enforced spending cap."})
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
	// The selected development base and the original bundle prerequisite are
	// different source facts. Consumers must not replace an adopted baseline
	// with the administrator base simply because it exists in a local cache.
	for _, model := range []string{"WorkspaceTerminalSession", "TerminalCandidateSource"} {
		field(model, "base_commit", map[string]any{"description": "Exact selected development base M; equals the administrator base when no code baseline was adopted."})
		field(model, "admin_base_commit", map[string]any{"description": "Original administrator Git prerequisite B retained for authenticated candidate bundle lineage."})
		field(model, "code_baseline", map[string]any{"description": "Exact adopted, accepted input frozen for this session or candidate. Its head is M and bundle prerequisite is B; current authorization is checked separately."})
	}
	field("WorkspaceTerminalPage", "code_baseline", map[string]any{"description": "Task's adopted code baseline read in the same authority snapshot as baseline_current and can_create."})
	field("WorkspaceTerminalPage", "baseline_current", map[string]any{"description": "Current source and task-binding qualification; an absent binding is valid only after this authorized read succeeds."})
	field("WorkspaceTerminalPage", "baseline_reason", map[string]any{"description": "Qualification failure reason from the same task read. It never changes historical frozen source facts."})
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
