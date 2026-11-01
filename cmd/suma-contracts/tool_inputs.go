package main

import (
	"strings"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/platform"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func toolInputModels() []any {
	return []any{cc.CodexUserInputRequest{}, cc.CodexUserInputResponse{}, cc.CodexUserInputEvidence{}, ic.ToolInput{}, ic.ToolInputList{}, ic.ToolInputAnswerRequest{}, platform.ToolInputOpenRequest{}, platform.ToolInputPollRequest{}, platform.ToolInputObservationRequest{}, platform.PoolToolInputOpenRequest{}, platform.PoolToolInputPollRequest{}, platform.PoolToolInputObservationRequest{}}
}

// Question answers supplement the original turn; they never grant command,
// filesystem, thread-resume or task-dispatch authority.
func toolInputContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/runner/tool-inputs", "post", "ToolInputOpenRequest", "ToolInput"},
		{"/runner/tool-inputs/poll", "post", "ToolInputPollRequest", "ToolInput"},
		{"/runner/tool-inputs/observations", "post", "ToolInputObservationRequest", "ToolInput"},
		{"/pool-runner/tool-inputs", "post", "PoolToolInputOpenRequest", "ToolInput"},
		{"/pool-runner/tool-inputs/poll", "post", "PoolToolInputPollRequest", "ToolInput"},
		{"/pool-runner/tool-inputs/observations", "post", "PoolToolInputObservationRequest", "ToolInput"},
		{"/runs/{run}/nodes/{node}/tool-inputs", "get", "", "ToolInputList"},
		{"/tool-inputs/{id}/answer", "post", "ToolInputAnswerRequest", "ToolInput"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, "200")
		op["description"] = "Exact original Codex input, attempt/fence, current identity and protected profile capability. Saved human answer, Runner observation, response write and native closure are distinct. No automatic answer, permission grant or process restart. Original answer operation replay recovers its receipt before fresh execution checks."
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
			for _, field := range []string{"attempt", "fence"} {
				p := queryParameter(field, "Original execution scope, including historical read-only questions.", map[string]any{"type": "integer", "minimum": 1, "maximum": cc.MaxSafeInteger})
				p["required"] = true
				appendQuery(op, p)
			}
		}
		if strings.HasSuffix(route.path, "/poll") {
			op["x-read-only"], op["x-no-agent-effects"] = true, true
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"CodexUserInputOption", "CodexUserInputQuestion", "CodexUserInputRequest", "CodexUserInputSelection", "CodexUserInputNativeAnswer", "CodexUserInputResponse", "CodexUserInputObservation", "CodexUserInputEvidence", "ToolInputRequest", "ToolInputAnswer", "ToolInput", "ToolInputList", "ToolInputAnswerRequest", "ToolInputOpenRequest", "ToolInputPollRequest", "ToolInputObservationRequest", "PoolToolInputOpenRequest", "PoolToolInputPollRequest", "PoolToolInputObservationRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	for _, name := range []string{"CodexUserInputRequest", "CodexUserInputEvidence"} {
		toolContractMap(schemas, name, "properties", "method")["const"] = cc.UserInputMethod
	}
	toolContractMap(schemas, "ToolInputRequest", "properties", "protocol")["const"] = cc.Protocol
	toolContractMap(schemas, "ToolInputList", "properties", "protocol")["enum"] = []string{"", cc.UserInputProtocol}
	toolContractMap(schemas, "ToolInputList", "properties", "protocol")["description"] = "Server-projected current protected profile capability; empty means not configured. Authorized history can remain readable. This is not liveness."
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "user_input_protocol")["const"] = cc.UserInputProtocol
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "user_input_protocol")["description"] = "Explicit local opt-in, supported only by writable Codex app-server profiles. Experimental native feature, default absent; not a permission grant."
	toolContractMap(schemas, "CodexUserInputQuestion", "properties", "is_secret")["const"] = false
	toolContractMap(schemas, "CodexUserInputQuestion", "properties", "options")["maxItems"] = 12
	for _, spec := range []struct {
		field string
		max   int
	}{{"id", 100}, {"header", 80}, {"question", 4000}} {
		p := toolContractMap(schemas, "CodexUserInputQuestion", "properties", spec.field)
		p["minLength"], p["maxLength"] = 1, spec.max
	}
	for _, spec := range []struct {
		field string
		max   int
	}{{"label", 200}, {"description", 1000}} {
		toolContractMap(schemas, "CodexUserInputOption", "properties", spec.field)["maxLength"] = spec.max
	}
	toolContractMap(schemas, "CodexUserInputRequest", "properties", "questions")["minItems"] = 1
	toolContractMap(schemas, "CodexUserInputRequest", "properties", "questions")["maxItems"] = 3
	toolContractMap(schemas, "CodexUserInputSelection", "properties", "kind")["enum"] = []string{"option", "text", "other", "skip"}
	toolContractMap(schemas, "CodexUserInputSelection", "properties", "text")["maxLength"] = 4000
	toolContractMap(schemas, "CodexUserInputObservation", "properties", "failure_code")["pattern"] = "^[a-z][a-z0-9_]{0,79}$"
	toolContractMap(schemas, "CodexUserInputObservation", "properties", "sequence")["minimum"] = 1
	toolContractMap(schemas, "CodexUserInputObservation", "properties", "sequence")["maximum"] = cc.MaxSafeInteger
	toolContractMap(schemas, "ToolInput", "properties", "state")["enum"] = []string{"pending", "answered", "closed", "expired"}
	toolContractMap(schemas, "ToolInput", "properties", "allowed_actions")["items"] = map[string]any{"type": "string", "const": "answer"}
	toolContractMap(schemas, "ToolInputList", "properties", "items")["maxItems"] = 32
	toolContractMap(schemas, "CodexEvidence", "properties", "user_inputs")["maxItems"] = 32
	toolContractMap(schemas, "CodexEvidence", "properties", "user_inputs")["description"] = "Safe identities and delivery facts only; combined with approvals at most32 original requests. Question and answer bodies stay in the separately authorized domain."
}
