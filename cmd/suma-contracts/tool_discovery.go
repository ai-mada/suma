package main

import "github.com/ai-mada/suma/internal/platform"

func toolDiscoveryModels() []any { return []any{platform.ToolDiscovery{}} }
func toolDiscoveryContracts(add operationAdder, api map[string]any) {
	op := add("/tool-discovery", "get", "ToolDiscovery", false, false)
	browserOnly(op)
	successStatus(op, "200")
	op["x-disallowed-token-scopes"] = []string{"api", "runner", "agent", "pool_runner"}
	op["description"] = "Browser-only current project/worker-authorized read of an already-reported nonsecret tool inventory. Private credential lineage or exact current Pool route/grant/runner credential is rechecked transactionally. State fresh contains validated observations only while database receive time is within60 seconds; stale and unreported contain empty items. This read never probes, installs, enables, dispatches or changes report timestamps."
	parameter := queryParameter("worker_id", "Exact currently authorized private or shared worker identity.", map[string]any{"type": "string", "minLength": 1, "maxLength": 100})
	parameter["required"] = true
	appendQuery(op, parameter)
}
