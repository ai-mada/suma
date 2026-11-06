package main

import "github.com/ai-mada/suma/internal/platform"

func automationModels() []any {
	return []any{platform.AutomationSpec{}, platform.AutomationRevision{}, platform.AutomationRule{}, platform.AutomationOccurrence{}, platform.AutomationCreateRequest{}, platform.AutomationPreviewRequest{}, platform.AutomationCommandRequest{}, platform.AutomationCommandResult{}, platform.AutomationPreview{}, platform.AutomationList{}, platform.AutomationOccurrences{}, platform.AutomationRevisions{}, platform.AutomationRunOriginResult{}, platform.AutomationRunOrigin{}}
}

func automationContracts(add operationAdder, paths map[string]any) {
	get := func(path, model string, paged bool) map[string]any {
		op := add(path, "get", model, false, false)
		successStatus(op, "200")
		browserOnly(op)
		if paged {
			appendQuery(op, queryParameter("limit", "Default 50; maximum 200.", map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}))
			appendQuery(op, queryParameter("cursor", "Opaque cursor scoped to this organization, project, rule and collection.", map[string]any{"type": "string"}))
		}
		return op
	}
	get("/automations", "AutomationList", true)
	get("/automations/{id}", "AutomationRule", false)
	occurrences := get("/automations/{id}/occurrences", "AutomationOccurrences", true)
	appendQuery(occurrences, queryParameter("filter", "Returns all occurrences or only blocked triggers, failed linked runs, and linked runs with a node requiring reconciliation. The cursor is scoped to the selected filter.", map[string]any{"type": "string", "enum": []string{"all", "attention"}, "default": "all"}))
	get("/automations/{id}/revisions", "AutomationRevisions", true)
	origin := get("/runs/{id}/automation-origin", "AutomationRunOriginResult", false)
	origin["description"] = "Reads the original automation trigger and immutable rule revision for this exact current project run. A readable run without automation provenance returns an explicit null origin. Missing or foreign runs return 404."
	op := add("/automations", "post", "AutomationRule", false, true)
	browserOnly(op)
	successStatus(op, "201")
	request(op, "AutomationCreateRequest")
	op["description"] = "Creates a draft with an immutable initial revision. Requires a real fixed public workflow version in the same project; cannot automate protected release workflows."
	op = add("/automations/preview", "post", "AutomationPreview", false, true)
	browserOnly(op)
	successStatus(op, "200")
	request(op, "AutomationPreviewRequest")
	op["description"] = "Read-only calendar preview using database time and bounded IANA timezone evaluation. Manual rules return an empty items list; cron returns five UTC/local pairs."
	op = add("/automations/{id}/commands", "post", "AutomationCommandResult", false, true)
	browserOnly(op)
	successStatus(op, "200")
	request(op, "AutomationCommandRequest")
	op["description"] = "Versioned revise/enable/pause/archive/trigger. Enable requires current approval authority; edits pause the rule. Trigger stores one deduplicated occurrence, not a successful run. Dispatch rechecks current activator authority and atomically binds the fixed workflow run."
	op = add("/runs/{id}/events/stream", "get", "", false, false)
	successStatus(op, "200")
	op["responses"].(map[string]any)["200"].(map[string]any)["content"] = map[string]any{"text/event-stream": map[string]any{"schema": map[string]any{"type": "string", "description": "UTF-8 SSE. id=durable Event.sequence, event=run_event, data=Event JSON. Comments are heartbeats; stream_error reports revocation or availability. Partial frames never advance the cursor."}}}
	appendQuery(op, queryParameter("after", "Zero or an existing durable event sequence from this exact run; must match Last-Event-ID when both supplied.", map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991), "default": 0}))
	op["parameters"] = append(op["parameters"].([]any), map[string]any{"in": "header", "name": "Last-Event-ID", "required": false, "schema": map[string]any{"type": "string", "pattern": "^[0-9]+$"}})
	op["responses"].(map[string]any)["429"] = map[string]any{"description": "Process or actor connection budget exhausted; Retry-After provided."}
	op["description"] = "Bounded 20-second connection with 5-second heartbeat, 2-second write deadlines, 128-event pages and repeated current session/project authorization. Browser cookie or bound API credential; no query-string secrets. Reconnect with last fully parsed sequence, preserving run identity."
	op["x-request-schema-coverage"] = "strict project/after query and Last-Event-ID validation; run-bound cursor and live authorization"
	op["x-event-schema"] = map[string]any{"$ref": "#/components/schemas/Event"}
	op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	history := paths["/runs/{id}/events"].(map[string]any)["get"].(map[string]any)
	history["description"] = "Authorized durable history window, at most 128 events. Resume using next_cursor; zero or an existing sequence from this exact run. Same current authority checks as streaming."
	for _, v := range history["parameters"].([]any) {
		parameter := v.(map[string]any)
		if parameter["name"] == "after" {
			parameter["schema"] = map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}
		}
	}
}

func enhanceAutomationSchemas(schemas map[string]any) {
	field := func(model, name string, values map[string]any) {
		m, ok := schemas[model].(map[string]any)
		if !ok {
			return
		}
		p, ok := m["properties"].(map[string]any)[name].(map[string]any)
		if !ok {
			return
		}
		for key, v := range values {
			p[key] = v
		}
	}
	for _, model := range []string{"AutomationSpec", "AutomationRule"} {
		field(model, "name", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
		field(model, "workflow_version", map[string]any{"minimum": 1, "maximum": 2147483647})
		field(model, "trigger", map[string]any{"enum": []string{"cron", "manual"}})
		field(model, "misfire", map[string]any{"enum": []string{"skip", "latest", "catch_up"}})
		field(model, "overlap", map[string]any{"enum": []string{"allow", "forbid", "queue"}})
		field(model, "catch_up_limit", map[string]any{"minimum": 1, "maximum": 100})
	}
	for _, model := range []string{"AutomationSpec", "AutomationCreateRequest", "AutomationPreviewRequest", "AutomationCommandRequest"} {
		schemas[model].(map[string]any)["additionalProperties"] = false
	}
	field("AutomationCommandRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("AutomationCommandRequest", "action", map[string]any{"enum": []string{"revise", "enable", "pause", "archive", "trigger"}, "description": "Only revise accepts/requires a non-null spec. Enable requires current project approval permission."})
	field("AutomationRule", "state", map[string]any{"enum": []string{"draft", "enabled", "paused", "archived"}})
	field("AutomationRule", "version", map[string]any{"minimum": 1, "description": "User-command CAS version; scheduler cursor movement does not consume this version."})
	field("AutomationRule", "revision", map[string]any{"minimum": 1, "description": "Immutable specification revision, incremented only by revise."})
	field("AutomationRunOrigin", "source", map[string]any{"enum": []string{"cron", "manual", "webhook"}})
	field("AutomationRunOrigin", "revision", map[string]any{"minimum": 1})
	field("AutomationRunOrigin", "scheduled_at", map[string]any{"format": "date-time"})
	field("AutomationOccurrence", "state", map[string]any{"enum": []string{"queued", "started", "skipped", "blocked"}})
	field("AutomationOccurrence", "source", map[string]any{"enum": []string{"cron", "manual"}})
	field("AutomationOccurrence", "run_id", map[string]any{"description": "The linked run in the current organization and project, or an empty string when no such run is visible."})
	field("AutomationOccurrence", "run_needs_reconciliation", map[string]any{"description": "True when a node of the linked run is reconciling within this organization and project. Omitted when no such node is recorded."})
}
