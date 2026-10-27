package main

import (
	"github.com/ai-mada/suma/internal/platform"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

type WorkspaceWriteClaimResponse struct {
	Lease *wwc.Lease `json:"lease"`
}

func workspaceWriteModels() []any {
	return []any{wwc.CreateRequest{}, wwc.Command{}, wwc.Page{}, wwc.InspectionRequest{}, wwc.ClaimRequest{}, wwc.Lease{}, wwc.ResultRequest{}, wwc.Receipt{}, WorkspaceWriteClaimResponse{}, platform.PoolWorkspaceWriteClaimRequest{}, platform.PoolWorkspaceWriteResultRequest{}}
}

func workspaceWriteContracts(add operationAdder, api map[string]any) {
	list := add("/workspace/writes", "get", "WorkspaceWritePage", false, false)
	list["description"] = "Recent50 metadata receipts belonging to the original current caller in this authorized project; exact total is caller-scoped. Never returns source content or grants filesystem access."
	create := add("/workspace/writes", "post", "WorkspaceWriteCommand", true, false)
	workspaceObservationBody(create, wwc.CreateRequest{})
	create["description"] = "Explicit existing-file save: current project write authority, exact live attempt/policy and optional current task basis; original file SHA and complete UTF-8 source up to48KiB. A single short execute permit is issued; timeout never requeues the write."
	get := add("/workspace/writes/{id}", "get", "WorkspaceWriteCommand", false, true)
	get["description"] = "Original current caller with project read authority may inspect immutable effects even after the native host closes. This is metadata, not a file read or permission to retry an uncertain write."
	inspect := add("/workspace/writes/{id}/inspect", "post", "WorkspaceWriteCommand", true, true)
	workspaceObservationBody(inspect, wwc.InspectionRequest{})
	inspect["description"] = "Explicit versioned request to examine the original native operation. Does not issue a second execute permit; missing proof remains reconciling. Stable retries preserve the original inspection key and version."
	for _, pool := range []bool{false, true} {
		prefix := "/runner/"
		var claim, result any = wwc.ClaimRequest{}, wwc.ResultRequest{}
		if pool {
			prefix = "/pool-runner/"
			claim = platform.PoolWorkspaceWriteClaimRequest{}
			result = platform.PoolWorkspaceWriteResultRequest{}
		}
		c := add(prefix+"workspace-writes/claim", "post", "WorkspaceWriteClaimResponse", true, false)
		workspaceObservationBody(c, claim)
		workspaceObservationMachine(c, pool)
		c["description"] = "Current exact worker scope claims one execute permit or an explicitly requested read-only inspection. Empty polls have no durable effect; inspection never carries source content."
		r := add(prefix+"workspace-writes/result", "post", "WorkspaceWriteReceipt", true, false)
		workspaceObservationBody(r, result)
		workspaceObservationMachine(r, pool)
		r["description"] = "Persist exact original command/request/attempt/policy/execution-fence effect metadata before ACK. Already-admitted facts can drain after scope ends; this cannot authorize another file effect."
	}
	schemas := api["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{"WorkspaceWriteCreateRequest", "WorkspaceWriteCommand", "WorkspaceWritePage", "WorkspaceWriteEffect", "WorkspaceWriteInspectionRequest", "WorkspaceWriteClaimRequest", "WorkspaceWriteLease", "WorkspaceWriteResultRequest", "WorkspaceWriteReceipt", "WorkspaceWriteClaimResponse", "PoolWorkspaceWriteClaimRequest", "PoolWorkspaceWriteResultRequest"} {
		if value, ok := schemas[name].(map[string]any); ok {
			value["additionalProperties"] = false
		}
	}
}
