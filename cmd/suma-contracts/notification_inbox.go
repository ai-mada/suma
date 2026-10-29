package main

import "github.com/ai-mada/suma/internal/platform"

func notificationInboxModels() []any {
	return []any{platform.NotificationInboxCounts{}, platform.NotificationInboxTask{}, platform.NotificationInboxItem{}, platform.NotificationInboxPage{}, platform.NotificationInboxCommand{}, platform.NotificationInboxCommandReceipt{}}
}

// The inbox projects real task assignments. Its private lifecycle requires
// current read authority, never project business-write or acceptance rights.
func notificationInboxContracts(add operationAdder, api map[string]any) {
	read := add("/notification-inbox", "get", "NotificationInboxPage", false, false)
	successStatus(read, "200")
	read["x-read-only"] = true
	appendQuery(read, queryParameter("filter", "inbox: all unarchived; unread: unread and unarchived; archived: archived regardless of read state.", map[string]any{"type": "string", "enum": []string{"inbox", "unread", "archived"}, "default": "inbox"}))
	appendQuery(read, queryParameter("limit", "Bounded page size; current totals cover the full recipient scope.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
	appendQuery(read, queryParameter("cursor", "Opaque immutable-sequence continuation bound to current identity, tenant, project, filter and limit. Not a frozen cross-page snapshot.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
	write := add("/notification-inbox/{id}/commands", "post", "NotificationInboxCommandReceipt", false, true)
	successStatus(write, "200")
	request(write, "NotificationInboxCommand")
	// The closed body supplies project scope; query parameters are rejected.
	parameters := []any{}
	for _, value := range write["parameters"].([]any) {
		p := value.(map[string]any)
		if p["in"] != "query" {
			parameters = append(parameters, p)
		}
	}
	write["parameters"] = parameters
	for _, op := range []map[string]any{read, write} {
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current human/user-API project authority; recipient is derived from current identity. Only task assignments from original business transactions create notifications. Sources are hydrated from current tasks; unavailable sources have no cached title. Private read/archive commands require only project read authority and an owned notification, do not change tasks or approvals, and recover the original operation receipt after reauthorization."
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"NotificationInboxCounts", "NotificationInboxTask", "NotificationInboxItem", "NotificationInboxPage", "NotificationInboxCommand", "NotificationInboxCommandReceipt"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		if name != "NotificationInboxCommand" {
			for _, value := range toolContractMap(model, "properties") {
				value.(map[string]any)["readOnly"] = true
			}
		}
	}
	field := func(model, key string, attrs map[string]any) {
		for k, v := range attrs {
			toolContractMap(schemas, model, "properties", key)[k] = v
		}
	}
	field("NotificationInboxCommand", "project_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("NotificationInboxCommand", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("NotificationInboxCommand", "action", map[string]any{"enum": []string{"read", "unread", "archive", "restore"}})
	field("NotificationInboxCommand", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128})
	field("NotificationInboxItem", "kind", map[string]any{"enum": []string{"task_assigned"}})
	field("NotificationInboxItem", "sequence", map[string]any{"type": "string", "pattern": "^[1-9][0-9]{0,18}$", "description": "Positive signed BIGINT decimal string; never coerce to an imprecise JSON number."})
	field("NotificationInboxPage", "items", map[string]any{"maxItems": 50})
	field("NotificationInboxPage", "limit", map[string]any{"minimum": 1, "maximum": 50})
	for _, key := range []string{"inbox", "unread", "archived"} {
		field("NotificationInboxCounts", key, map[string]any{"minimum": 0})
	}
}
