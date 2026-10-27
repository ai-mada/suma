package main

import (
	"reflect"

	"github.com/ai-mada/suma/internal/platform"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type WorkspaceObservationClaimResponse struct {
	Lease *woc.Lease `json:"lease"`
}

func workspaceObservationModels() []any {
	return []any{woc.TargetPage{}, woc.Observation{}, woc.CreateRequest{}, woc.ClaimRequest{}, woc.ResultRequest{}, woc.Receipt{}, WorkspaceObservationClaimResponse{}, platform.PoolWorkspaceObservationClaimRequest{}, platform.PoolWorkspaceObservationResultRequest{}}
}
func workspaceObservationContracts(add operationAdder, api map[string]any) {
	targets := add("/workspace/targets", "get", "WorkspaceObservationTargetPage", false, false)
	targets["description"] = "Current project-authorized eligible native running attempts. This is not a worker connection or model health report. At most50 returned and201 inspected; truncation is explicit."
	create := add("/workspace/observations", "post", "WorkspaceObservation", true, false)
	create["description"] = "Request a bounded readonly list/read of an exact live native attempt. Original caller and current attempt/Pool grants are rechecked at lease, result and disclosure. Does not dispatch code, write a file or create test evidence."
	workspaceObservationBody(create, woc.CreateRequest{})
	get := add("/workspace/observations/{id}", "get", "WorkspaceObservation", false, true)
	get["description"] = "Read an observation only under its original current credential and exact live scope. Expiry or scope loss stops disclosure. metadata=true retains all authorization checks but omits source result."
	appendQuery(get, queryParameter("metadata", "Use true to recheck validity without retransmitting source content.", map[string]any{"type": "string", "enum": []string{"true"}}))
	for _, pool := range []bool{false, true} {
		prefix := "/runner/"
		var claim, result any = woc.ClaimRequest{}, woc.ResultRequest{}
		if pool {
			prefix = "/pool-runner/"
			claim = platform.PoolWorkspaceObservationClaimRequest{}
			result = platform.PoolWorkspaceObservationResultRequest{}
		}
		c := add(prefix+"workspace-observations/claim", "post", "WorkspaceObservationClaimResponse", true, false)
		workspaceObservationBody(c, claim)
		workspaceObservationMachine(c, pool)
		c["description"] = "Worker-authenticated lease for its exact active workspace. Monotonic observation lease fence; no source text is returned to the worker."
		r := add(prefix+"workspace-observations/result", "post", "WorkspaceObservationReceipt", true, false)
		workspaceObservationBody(r, result)
		workspaceObservationMachine(r, pool)
		r["description"] = "Persist exact scope/policy/lease-fence observation result before acknowledgment. Duplicate ACK recovery preserves the original result; stale receipts cannot disclose code."
	}
}
func workspaceObservationBody(op map[string]any, value any) {
	op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": schema(reflect.TypeOf(value))}}}
}

// Machine routes accept scope in the strict JSON envelope only. Advertising
// browser identity headers or a project query would mislead generated clients.
func workspaceObservationMachine(op map[string]any, pool bool) {
	scheme := "runnerAuth"
	if pool {
		scheme = "poolRunnerAuth"
	}
	op["security"] = []any{map[string]any{scheme: []string{}}}
	if parameters, ok := op["parameters"].([]any); ok {
		kept := []any{}
		for _, parameter := range parameters {
			value, ok := parameter.(map[string]any)
			if !ok {
				continue
			}
			name, _ := value["name"].(string)
			if name == "Accept-Language" || name == "X-Suma-Language" {
				kept = append(kept, parameter)
			}
		}
		op["parameters"] = kept
	}
}
