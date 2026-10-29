package main

import "github.com/ai-mada/suma/internal/platform"

func myWorkModels() []any { return []any{platform.MyWorkPage{}, platform.MyWorkStatusCount{}} }

func myWorkContracts(add operationAdder, api map[string]any) {
	op := add("/my-work", "get", "MyWorkPage", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	op["description"] = "Current human's assigned tasks resolved from the canonical project resource binding. Exact filtered count and status totals, bounded live keyset pages, current authorization on every request. Does not infer personal review assignments, handoff receipt, execution success or acceptance. No caller-supplied user/owner identity. Database-native case semantics apply to literal title/description search."
	appendQuery(op, queryParameter("q", "Literal title/description text, at most200 Unicode codepoints; not a wildcard expression.", map[string]any{"type": "string", "maxLength": 200}))
	appendQuery(op, queryParameter("status", "Selected business state. Status counts cover the same person/text over all nonarchived states, including additional current business statuses.", map[string]any{"type": "string", "enum": []string{"all", "draft", "todo", "in_progress", "in_review", "accepted", "blocked", "cancelled"}, "default": "all"}))
	appendQuery(op, queryParameter("limit", "Bounded page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
	appendQuery(op, queryParameter("cursor", "Opaque canonical cursor bound to current user/project/resource/version and all filters; pages are live, not a historical snapshot.", map[string]any{"type": "string", "maxLength": 4096}))
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"MyWorkPage", "MyWorkStatusCount"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, value := range toolContractMap(model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	toolContractMap(schemas, "MyWorkPage", "properties", "resource_status")["enum"] = []string{"active", "unbound", "inactive"}
	toolContractMap(schemas, "MyWorkPage", "properties", "items")["maxItems"] = 50
	toolContractMap(schemas, "MyWorkPage", "properties", "limit")["maximum"] = 50
	toolContractMap(schemas, "MyWorkStatusCount", "properties", "total")["minimum"] = 0
}
