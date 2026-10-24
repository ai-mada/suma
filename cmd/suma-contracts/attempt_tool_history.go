package main

// A dedicated route and envelope keep attempt-scoped opaque positions separate
// from the legacy run-wide numeric event cursor. Both expose original Events.
func attemptToolHistoryContracts(add operationAdder) {
	op := add("/runs/{id}/nodes/{node}/attempts/{attempt}/tool-events", "get", "AttemptToolEventPage", false, false)
	successStatus(op, "200")
	op["x-disallowed-token-scopes"] = []string{"agent", "runner"}
	op["x-strict-query"] = []string{"project_id", "limit", "cursor"}
	op["x-candidate-scan-budget"] = 1000
	op["x-serialized-event-page-byte-budget"] = 4 * 1024 * 1024
	op["description"] = "Reads original persisted tool_progress envelopes attributable to the exact durable run/node/attempt/fence/worker. Current project, membership, session and API-token scope are checked in the same transaction as selection. Governance coordination may advance; execution, evidence, operation receipts, resources and budgets do not. Each bounded seek scans at most1000 candidates. Empty intermediate pages can have continuation; no total is invented. The first request fixes a node-tool-stream watermark; page_cursor re-reads that exact position/window, including the initial or empty page, and next_cursor advances within it. Only an explicit no-cursor read starts a new snapshot. Cursors confer no permission or cryptographic integrity. Unsupported inner tool metadata remains raw evidence."
	appendQuery(op, queryParameter("limit", "Default50;1..100. Each serialized event is at most2MiB and returned events together at most4MiB; byte limits can produce a short page with continuation.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}))
	appendQuery(op, queryParameter("cursor", "Nonempty unpadded-base64url position bound to tenant/project/run/node/attempt/fence/worker and fixed watermark. Use returned page_cursor for the same page or next_cursor to advance. No cursor creates a new window.", attemptToolCursorSchema()))
	for _, raw := range op["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["name"] == "attempt" {
			parameter["schema"] = map[string]any{"type": "string", "pattern": "^[1-9][0-9]{0,15}$", "maxLength": 16}
			parameter["description"] = "Canonical positive decimal, no leading zero/sign/fraction/exponent; value at most9007199254740991."
		}
	}
	op["responses"].(map[string]any)["404"] = map[string]any{"description": "The authorized run, node or execution attempt does not exist."}
}

func attemptToolCursorSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 4096, "pattern": "^[A-Za-z0-9_-]+$"}
}

func enhanceAttemptToolHistorySchemas(schemas map[string]any) {
	model := toolContractMap(schemas, "AttemptToolEventPage")
	model["additionalProperties"] = false
	props := toolContractMap(model, "properties")
	toolContractMap(props, "schema_version")["const"] = 1
	for _, name := range []string{"attempt", "fence", "watermark", "scanned_through"} {
		field := toolContractMap(props, name)
		field["minimum"] = 0
		field["maximum"] = int64(9007199254740991)
	}
	toolContractMap(props, "attempt")["minimum"] = 1
	toolContractMap(props, "items")["maxItems"] = 100
	toolContractMap(props, "items", "items")["required"] = []string{"raw_data"}
	toolContractMap(props, "items")["description"] = "Original parent Event envelopes in increasing unique sequence order, within the fixed watermark and exact durable attempt identity. Unknown inner payloads are raw evidence, not silently discarded."
	props["page_cursor"] = attemptToolCursorSchema()
	props["next_cursor"] = attemptToolCursorSchema()
	toolContractMap(props, "page_cursor")["description"] = "Opaque position that reproduces this page in the same saved window, including position0."
	toolContractMap(props, "watermark")["description"] = "Fixed highest candidate sequence when this window began,0 if originally empty; not a count or an ETA."
	toolContractMap(props, "scanned_through")["description"] = "Last candidate examined,0 at an empty start; at most watermark. It may belong to another attempt, so an empty page may still have continuation."
	model["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"has_more": map[string]any{"const": true}}},
		"then": map[string]any{"required": []string{"next_cursor"}},
		"else": map[string]any{"not": map[string]any{"required": []string{"next_cursor"}}},
	}}
	// The raw body is optional for legacy run/SSE events; this page alone
	// requires it, so a compatible map projection never claims byte fidelity.
	raw := toolContractMap(schemas, "Event", "properties", "raw_data")
	raw["maxLength"] = 2 * 1024 * 1024
	raw["description"] = "Original UTF-8 data text, populated only on exact-attempt history pages; never translated or normalized. data remains a compatible parsed projection. The full serialized Event, including both representations, is bounded to2MiB by the reader. Other event reads omit this field."
}
