package main

import (
	"strings"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/platform"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func toolInputModels() []any {
	return []any{cc.CodexUserInputRequest{}, cc.CodexUserInputResponse{}, cc.CodexUserInputEvidence{}, cu.CursorInputRequest{}, cu.CursorInputNativeResult{}, cu.CursorInputEvidence{}, ic.ToolInput{}, ic.ToolInputList{}, ic.ToolInputAnswerRequest{}, platform.ToolInputOpenRequest{}, platform.ToolInputPollRequest{}, platform.ToolInputObservationRequest{}, platform.PoolToolInputOpenRequest{}, platform.PoolToolInputPollRequest{}, platform.PoolToolInputObservationRequest{}}
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
		op["description"] = "Exact original Codex question or Cursor ACP question/plan, attempt/fence, current identity and protected runtime capability. Saved human decision, Runner observation, response write and execution closure remain distinct. Plan confirmation grants no command permission or formal baseline. Original answer operation replay recovers its receipt before fresh execution checks."
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
	cursorInputSchemas(schemas)
	for _, name := range []string{"CodexUserInputRequest", "CodexUserInputEvidence"} {
		toolContractMap(schemas, name, "properties", "method")["const"] = cc.UserInputMethod
	}
	toolContractMap(schemas, "ToolInputRequest", "properties", "protocol")["enum"] = []string{cc.Protocol, "acp-stdio-v1"}
	toolContractMap(schemas, "ToolInputList", "properties", "protocol")["enum"] = []string{"", cc.UserInputProtocol, cu.InputProtocol}
	toolContractMap(schemas, "ToolInputList", "properties", "protocol")["description"] = "Server-projected current protected profile capability; empty means not configured. Authorized history can remain readable. This is not liveness."
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "user_input_protocol")["enum"] = []string{cc.UserInputProtocol, cu.InputProtocol}
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "user_input_protocol")["description"] = "Exact writable runtime capability: explicit experimental Codex opt-in, or fixed Cursor ACP question/plan support. Never grants command permissions, task dispatch or formal baseline approval."
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

func cursorInputSchemas(schemas map[string]any) {
	for _, name := range []string{"CursorRPCRequestID", "CursorQuestionOption", "CursorQuestion", "CursorAskQuestion", "CursorTodo", "CursorPlanPhase", "CursorCreatePlan", "CursorInputRequest", "CursorInputSelection", "CursorInputDecision", "CursorInputNativeAnswer", "CursorInputNativeOutcome", "CursorInputNativeResult", "CursorInputEvidence", "ToolInputDeliveryObservation"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	ref := func(name string) map[string]any { return map[string]any{"$ref": "#/components/schemas/" + name} }
	toolContractMap(schemas, "ToolInputRequest")["oneOf"] = []any{
		map[string]any{"required": []string{"codex"}, "properties": map[string]any{"protocol": map[string]any{"const": cc.Protocol}, "codex": ref("CodexUserInputRequest")}, "not": map[string]any{"required": []string{"cursor"}}},
		map[string]any{"required": []string{"cursor"}, "properties": map[string]any{"protocol": map[string]any{"const": "acp-stdio-v1"}, "cursor": ref("CursorInputRequest")}, "not": map[string]any{"required": []string{"codex"}}},
	}
	toolContractMap(schemas, "CursorInputRequest")["oneOf"] = []any{
		map[string]any{"required": []string{"questions"}, "properties": map[string]any{"method": map[string]any{"const": cu.AskQuestionMethod}, "questions": ref("CursorAskQuestion")}, "not": map[string]any{"required": []string{"plan"}}},
		map[string]any{"required": []string{"plan"}, "properties": map[string]any{"method": map[string]any{"const": cu.CreatePlanMethod}, "plan": ref("CursorCreatePlan")}, "not": map[string]any{"required": []string{"questions"}}},
	}
	for _, name := range []string{"ToolInputAnswer", "ToolInputAnswerRequest"} {
		toolContractMap(schemas, name)["oneOf"] = []any{
			map[string]any{"required": []string{"answers"}, "properties": map[string]any{"answers": map[string]any{"type": "array", "items": ref("CodexUserInputSelection"), "minItems": 1, "maxItems": 3}}, "not": map[string]any{"required": []string{"cursor"}}},
			map[string]any{"required": []string{"cursor"}, "properties": map[string]any{"cursor": ref("CursorInputDecision"), "answers": map[string]any{"type": "array", "maxItems": 0}}},
		}
	}
	toolContractMap(schemas, "CursorRPCRequestID", "properties", "kind")["enum"] = []string{"string", "number"}
	toolContractMap(schemas, "CursorInputDecision", "properties", "outcome")["enum"] = []string{"answered", "skipped", "cancelled", "accepted", "rejected"}
	toolContractMap(schemas, "CursorTodo", "properties", "status")["enum"] = []string{"pending", "in_progress", "completed", "cancelled"}
	toolContractMap(schemas, "CursorInputEvidence", "properties", "kind")["enum"] = []string{"questions", "plan"}
	toolContractMap(schemas, "CursorInputEvidence", "properties", "question_count")["minimum"] = 0
	toolContractMap(schemas, "CursorInputEvidence", "properties", "question_count")["maximum"] = cu.MaxQuestions
	toolContractMap(schemas, "CursorAskQuestion", "properties", "questions")["minItems"] = 1
	toolContractMap(schemas, "CursorAskQuestion", "properties", "questions")["maxItems"] = cu.MaxQuestions
	toolContractMap(schemas, "CursorQuestion", "properties", "options")["minItems"] = 1
	toolContractMap(schemas, "CursorQuestion", "properties", "options")["maxItems"] = cu.MaxOptions
	toolContractMap(schemas, "CursorInputSelection", "properties", "option_ids")["uniqueItems"] = true
	toolContractMap(schemas, "CursorInputSelection", "properties", "option_ids")["minItems"] = 1
	toolContractMap(schemas, "CursorInputSelection", "properties", "option_ids")["maxItems"] = cu.MaxOptions
	toolContractMap(schemas, "CursorCreatePlan", "properties", "plan")["maxLength"] = cu.MaxPlanBytes
	toolContractMap(schemas, "CursorCreatePlan", "properties", "todos")["maxItems"] = cu.MaxTodos
	toolContractMap(schemas, "CursorCreatePlan", "properties", "phases")["maxItems"] = cu.MaxPhases
	toolContractMap(schemas, "CursorCreatePlan")["description"] = "Complete original read-only plan. Accepted may write a private native HOME plan file; it is not a formal project baseline. Source canonical payload is limited to256KiB and SHA256-bound; no client-supplied URI."
	toolContractMap(schemas, "ToolInputDeliveryObservation", "properties", "sequence")["minimum"] = 1
	toolContractMap(schemas, "ToolInputDeliveryObservation", "properties", "sequence")["maximum"] = cc.MaxSafeInteger
	toolContractMap(schemas, "ToolInputDeliveryObservation")["description"] = "Durable delivery facts. Execution closure is projected from each real runtime; a complete response write is not a native acceptance ACK. Cursor has no request-resolved acknowledgement."
	toolContractMap(schemas, "ACPExecutionEvidence", "properties", "user_inputs")["maxItems"] = 32
}
