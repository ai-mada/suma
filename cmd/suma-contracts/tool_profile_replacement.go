package main

import "github.com/ai-mada/suma/internal/platform"

func toolProfileReplacementModels() []any {
	return []any{platform.ToolProfileReplacementReuse{}, platform.ToolProfileReplacementRequest{}, platform.ToolProfileReplacement{}, platform.ToolProfileReplacementList{}, platform.ToolProfileReplacementListQuery{}}
}

func toolProfileReplacementRoutes(add operationAdder) {
	create := add("/tool-profiles/{id}/replacements", "post", "ToolProfileReplacement", false, true)
	browserOnly(create)
	request(create, "ToolProfileReplacementRequest")
	successStatus(create, "201")
	create["description"] = "Explicit immutable configuration preparation in one current-approval transaction: exact old version, fresh complete target inventory, explicit exact reuse or new paused profile, optional original pause, relation and original actor/body-bound receipt. Never activates a target, rewrites existing plans, stops processes or silently chooses a match. Same-key replay precedes new freshness/CAS checks after current authorization."
	for _, path := range []string{"/tool-profiles/{id}/replacements", "/tool-profile-replacements/{id}"} {
		model := "ToolProfileReplacement"
		if path == "/tool-profiles/{id}/replacements" {
			model = "ToolProfileReplacementList"
		}
		op := add(path, "get", model, false, false)
		browserOnly(op)
		successStatus(op, "200")
		op["x-read-only"], op["x-response-cache-control"] = true, "no-store"
		op["description"] = "Current authorized read of immutable handoff snapshots. History does not re-observe profiles, assert current availability, restore a write or grant activation authority."
		if model == "ToolProfileReplacementList" {
			appendQuery(op, queryParameter("direction", "Relation direction for this exact profile.", map[string]any{"type": "string", "enum": []string{"outgoing", "incoming"}, "default": "outgoing"}))
			appendQuery(op, queryParameter("limit", "Bounded page size; default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Opaque keyset cursor bound to tenant/project/profile/direction; not a cross-request snapshot.", map[string]any{"type": "string", "maxLength": 2048}))
		}
	}
}

func toolProfileReplacementSchemas(schemas map[string]any) {
	for _, name := range []string{"ToolProfileReplacementReuse", "ToolProfileReplacementRequest", "ToolProfileReplacement", "ToolProfileReplacementList", "ToolProfileReplacementListQuery"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, property string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", property)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for model, properties := range map[string][]string{"ToolProfileReplacementReuse": {"profile_id"}, "ToolProfileReplacementRequest": {"project_id", "worker_id"}, "ToolProfileReplacement": {"id", "project_id", "created_by"}, "ToolProfileReplacementList": {"project_id", "profile_id"}} {
		for _, property := range properties {
			field(model, property, map[string]any{"minLength": 1, "maxLength": 100, "pattern": `^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`})
		}
	}
	for _, model := range []string{"ToolProfileReplacementReuse", "ToolProfileReplacementRequest"} {
		field(model, "expected_version", map[string]any{"minimum": 1, "maximum": 2147483647})
	}
	field("ToolProfileReplacementRequest", "name", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200, "description": "Required for creation, omitted for reuse. Nonblank and already trimmed; never silently renamed."})
	field("ToolProfileReplacementRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "description": "Original same-actor operation key; preserve the exact submitted intent for uncertain-result recovery."})
	toolContractMap(schemas, "ToolProfileReplacementRequest")["oneOf"] = []any{
		map[string]any{"required": []string{"name"}, "properties": map[string]any{"reuse": false}},
		map[string]any{"required": []string{"reuse"}, "properties": map[string]any{"name": false}},
	}
	field("ToolProfileReplacement", "disposition", map[string]any{"enum": []string{"created", "reused"}})
	field("ToolProfileReplacement", "created_at", map[string]any{"format": "date-time"})
	field("ToolProfileReplacement", "created_by", map[string]any{"description": "Original committing actor. Immutable across receipt replay; a current history reader may be another authorized actor."})
	for _, model := range []string{"ToolProfileReplacementList", "ToolProfileReplacementListQuery"} {
		field(model, "direction", map[string]any{"enum": []string{"outgoing", "incoming"}})
	}
	field("ToolProfileReplacementList", "items", map[string]any{"maxItems": 100})
	field("ToolProfileReplacementList", "next_cursor", map[string]any{"minLength": 1, "maxLength": 2048})
	field("ToolProfileReplacementListQuery", "limit", map[string]any{"minimum": 1, "maximum": 100, "default": 20})
	field("ToolProfileReplacementListQuery", "cursor", map[string]any{"maxLength": 2048})
}
