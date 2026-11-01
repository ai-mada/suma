package main

import (
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/platform"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	"strings"
)

func turnSteeringModels() []any {
	return []any{sc.SteeringContext{}, sc.SteeringCreateRequest{}, sc.SteeringPoll{}, sc.SteeringOpenResult{}, cc.CodexSteeringEvidence{}, platform.SteeringSessionOpenRequest{}, platform.SteeringPollRequest{}, platform.SteeringObservationRequest{}, platform.SteeringCloseRequest{}, platform.PoolSteeringSessionOpenRequest{}, platform.PoolSteeringPollRequest{}, platform.PoolSteeringObservationRequest{}, platform.PoolSteeringCloseRequest{}}
}

func turnSteeringContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/tasks/{id}/execution-steering", "get", "", "SteeringContext"},
		{"/task-execution-steering", "post", "SteeringCreateRequest", "SteeringInstruction"},
		{"/runner/steering/sessions", "post", "SteeringSessionOpenRequest", "SteeringOpenResult"},
		{"/runner/steering/poll", "post", "SteeringPollRequest", "SteeringPoll"},
		{"/runner/steering/observations", "post", "SteeringObservationRequest", "SteeringInstruction"},
		{"/runner/steering/close", "post", "SteeringCloseRequest", "SteeringSession"},
		{"/pool-runner/steering/sessions", "post", "PoolSteeringSessionOpenRequest", "SteeringOpenResult"},
		{"/pool-runner/steering/poll", "post", "PoolSteeringPollRequest", "SteeringPoll"},
		{"/pool-runner/steering/observations", "post", "PoolSteeringObservationRequest", "SteeringInstruction"},
		{"/pool-runner/steering/close", "post", "PoolSteeringCloseRequest", "SteeringSession"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, "200")
		op["description"] = "Explicit original human discussion message to one attested live Codex turn. Current identity, original assignment/profile, exact operation receipt and native write/ACK are independent. Never starts/resumes a turn, overrides models or grants permissions. Unknown native delivery is not retried."
		op["x-response-cache-control"] = "no-store"
		if route.input != "" {
			request(op, route.input)
			op["x-query-parameters-forbidden"], op["x-max-request-body-bytes"] = true, 1<<20
		}
		if strings.HasPrefix(route.path, "/runner/") || strings.HasPrefix(route.path, "/pool-runner/") {
			security := "runnerAuth"
			if strings.HasPrefix(route.path, "/pool-runner/") {
				security = "poolRunnerAuth"
			}
			op["security"], op["parameters"], op["x-machine-only"] = []any{map[string]any{security: []any{}}}, []any{}, true
		} else {
			browserOnly(op)
		}
		if route.method == "get" {
			op["x-read-only"], op["x-no-agent-effects"] = true, true
			p := queryParameter("message_id", "Exact immutable discussion source. Read permission is separate from permission to send one's own human message.", map[string]any{"type": "string", "minLength": 1, "maxLength": 100})
			p["required"] = true
			appendQuery(op, p)
		}
		if strings.HasSuffix(route.path, "/poll") {
			op["x-read-only"], op["x-no-agent-effects"] = true, true
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"CodexSteeringSource", "CodexSteeringObservation", "CodexSteeringInstructionEvidence", "CodexSteeringEvidence", "SteeringSession", "SteeringMessage", "SteeringInstruction", "SteeringContext", "SteeringCreateRequest", "SteeringPoll", "SteeringOpenResult", "SteeringSessionOpenRequest", "SteeringPollRequest", "SteeringObservationRequest", "SteeringCloseRequest", "PoolSteeringSessionOpenRequest", "PoolSteeringPollRequest", "PoolSteeringObservationRequest", "PoolSteeringCloseRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	toolContractMap(schemas, "CodexSteeringSource", "properties", "protocol")["const"] = cc.SteeringProtocol
	toolContractMap(schemas, "SteeringOpenResult")["oneOf"] = []any{map[string]any{"required": []string{"session"}, "not": map[string]any{"required": []string{"reason"}}}, map[string]any{"required": []string{"reason"}, "properties": map[string]any{"reason": map[string]any{"const": "not_task_execution"}}, "not": map[string]any{"required": []string{"session"}}}}
	toolContractMap(schemas, "SteeringOpenResult")["description"] = "A non-task workflow returns only the exact normal unavailability reason after machine/scope verification. Authorization, storage or unknown HTTP failures are not this outcome."
	toolContractMap(schemas, "SteeringContext", "properties", "reason")["enum"] = []string{"not_own_message", "task_accepted", "no_live_turn", "current_execution_unavailable"}
	toolContractMap(schemas, "SteeringContext", "properties", "targets")["maxItems"] = 10000
	toolContractMap(schemas, "SteeringContext", "properties", "instructions")["maxItems"] = 10000
	toolContractMap(schemas, "SteeringContext")["description"] = "One selected message's current authorized targets and history. No silent truncation; sender eligibility does not restrict authorized historical reading or original receipt recovery."
	toolContractMap(schemas, "SteeringSession", "properties", "state")["enum"] = []string{"active", "closed"}
	toolContractMap(schemas, "SteeringSession", "properties", "allowed_actions")["items"] = map[string]any{"type": "string", "const": "send"}
	toolContractMap(schemas, "SteeringSession", "properties", "allowed_actions")["maxItems"] = 1
	toolContractMap(schemas, "SteeringMessage", "properties", "body")["maxLength"] = 8000
	toolContractMap(schemas, "SteeringMessage", "properties", "body")["x-max-utf8-bytes"] = cc.MaxSteeringTextBytes
	toolContractMap(schemas, "SteeringMessage", "properties", "body")["description"] = "Exact saved human message, not trimmed or translated. Never accepted as a new body in the send request."
	toolContractMap(schemas, "SteeringPoll", "properties", "instructions")["maxItems"] = 32
	toolContractMap(schemas, "CodexSteeringEvidence", "properties", "instructions")["maxItems"] = 32
	for _, name := range []string{"SteeringMessage", "CodexSteeringInstructionEvidence"} {
		field := "body_sha256"
		if name == "CodexSteeringInstructionEvidence" {
			field = "message_sha256"
		}
		toolContractMap(schemas, name, "properties", field)["pattern"] = "^[a-f0-9]{64}$"
	}
	toolContractMap(schemas, "CodexSteeringObservation", "properties", "failure_code")["pattern"] = "^[a-z][a-z0-9_]{0,79}$"
	toolContractMap(schemas, "CodexSteeringObservation", "properties", "sequence")["minimum"] = 1
	toolContractMap(schemas, "CodexSteeringObservation", "properties", "sequence")["maximum"] = cc.MaxSafeInteger
	toolContractMap(schemas, "CodexEvidence", "properties", "steering")["description"] = "Original live session and safe delivery facts only. Acceptance is not model compliance or quality; current storage must independently match all instructions."
}
