package main

import (
	ec "github.com/ai-mada/suma/internal/executioncontract"
	"github.com/ai-mada/suma/internal/platform"
)

func candidateRepairRevisionModels() []any {
	return []any{platform.CandidateCompositionRepairRevisionRequest{}, platform.CandidateCompositionRepairRevisionContext{}, platform.CandidateCompositionRepairRecoveryBasis{}, platform.CandidateCompositionRepairRecoveryAttempt{}, platform.CandidateCompositionRepairRevisionLineage{}, platform.TaskRunProvenance{}, platform.TaskRunProvenancePage{}, ec.StartupFailureProof{}}
}

func candidateRepairRevisionContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/candidate-compositions/{id}/repair-revision-context", "get", "", "CandidateCompositionRepairRevisionContext", "200"},
		{"/candidate-compositions/{id}/repair-revisions", "post", "CandidateCompositionRepairRevisionRequest", "CandidateCompositionPlan", "201"},
		{"/tasks/{id}/run-history", "get", "", "TaskRunProvenancePage", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
		}
		op["description"] = "Current-scoped explicit repair revision creates a new plan for the same reserved task; it never starts work. Immutable original plans/runs/facts remain readable. Failed replacement requires exact certified host startup proof; history separates actual binding from observed current references."
		if route.output == "TaskRunProvenancePage" {
			appendQuery(op, queryParameter("limit", "Canonical bounded decimal page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Original actor/project/task-bound next_cursor.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	field := func(model, name string, extra map[string]any) {
		property := toolContractMap(api, "components", "schemas", model, "properties", name)
		for key, value := range extra {
			property[key] = value
		}
	}
	for _, model := range []string{"CandidateCompositionRepairRevisionRequest", "StartupFailureProof"} {
		toolContractMap(api, "components", "schemas", model)["additionalProperties"] = false
	}
	for _, name := range []string{"expected_version", "expected_task_version", "workflow_version", "expected_tool_version"} {
		field("CandidateCompositionRepairRevisionRequest", name, map[string]any{"minimum": 1, "maximum": 2147483646, "x-integer-lexeme": true})
	}
	for _, name := range []string{"expected_conflict_sha256", "expected_recovery_sha256"} {
		field("CandidateCompositionRepairRevisionRequest", name, map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"})
	}
	field("CandidateCompositionRepairRevisionRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000})
	field("CandidateCompositionRepairRevisionRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("CandidateCompositionRepairRevisionRequest", "acknowledge", map[string]any{"const": true})
	field("CandidateCompositionRepairRevisionContext", "eligibility_mode", map[string]any{"enum": []string{"", "ready", "certified_startup_failure"}})
	field("CandidateCompositionRepairRevisionLineage", "ordinal", map[string]any{"minimum": 1, "maximum": 32})
	field("StartupFailureProof", "protocol", map[string]any{"const": ec.StartupFailureProtocol})
	field("StartupFailureProof", "stage", map[string]any{"const": "before_workspace"})
	field("StartupFailureProof", "purpose", map[string]any{"const": "candidate_conflict_repair"})
	field("StartupFailureProof", "workspace_started", map[string]any{"const": false})
	field("StartupFailureProof", "process_started", map[string]any{"const": false})
	field("TaskRunProvenance", "binding_kind", map[string]any{"enum": []string{"bound", "observed"}})
	field("CandidateCompositionPlan", "repair_revision", map[string]any{"readOnly": true, "description": "Protected immutable repair lineage; generic task/data writes cannot manufacture it."})
}
