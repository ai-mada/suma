package main

import "github.com/ai-mada/suma/internal/platform"

func notificationInboxModels() []any {
	return []any{platform.NotificationInboxCounts{}, platform.NotificationInboxTask{}, platform.NotificationInboxToolInputTarget{}, platform.NotificationInboxItem{}, platform.NotificationInboxPage{}, platform.NotificationOrganizationInboxItem{}, platform.NotificationOrganizationInboxPage{}, platform.NotificationOrganizationUnreadSummary{}, platform.NotificationInboxCommand{}, platform.NotificationInboxCommandReceipt{}, platform.NotificationInboxBatchTarget{}, platform.NotificationInboxBatchCommand{}, platform.NotificationInboxBatchResult{}, platform.NotificationInboxBatchReceipt{}}
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
	organization := add("/notification-inbox/organization", "get", "NotificationOrganizationInboxPage", false, false)
	browserOnly(organization)
	successStatus(organization, "200")
	organization["x-read-only"] = true
	organization["description"] = "Current browser recipient across readable projects of the active organization. Counts and hydrated sources are restricted to current authority. A change to the visible project set invalidates earlier continuation cursors."
	appendQuery(organization, queryParameter("filter", "Inbox, unread or archived in the current organization.", map[string]any{"type": "string", "enum": []string{"inbox", "unread", "archived"}, "default": "inbox"}))
	appendQuery(organization, queryParameter("limit", "Bounded page size; counts cover all currently readable projects.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
	appendQuery(organization, queryParameter("cursor", "Opaque keyset cursor bound to current identity, organization, visible projects, filter and limit.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
	summary := add("/notification-inbox/organization/unread-summary", "get", "NotificationOrganizationUnreadSummary", false, false)
	browserOnly(summary)
	successStatus(summary, "200")
	summary["x-read-only"] = true
	summary["description"] = "Count the current browser recipient's unread and unarchived notifications in currently readable projects of the active organization. No query parameters or notification contents."
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
	batch := add("/notification-inbox/batch-commands", "post", "NotificationInboxBatchReceipt", false, true)
	browserOnly(batch)
	successStatus(batch, "200")
	request(batch, "NotificationInboxBatchCommand")
	batch["description"] = "Apply a single private inbox action atomically to explicitly selected notifications across current readable projects. Each target's owner and project access are rechecked before writing or replaying an original receipt."
	for _, op := range []map[string]any{read, write} {
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current human/user-API project authority; recipient is derived from current identity. Notifications come from original task assignment, mention, review, handoff, followed discussion, execution failure or a durable tool input request. Sources are hydrated from current tasks; unavailable sources have no cached title. Private read/archive commands require only project read authority and an owned notification, do not change tasks or approvals, and recover the original operation receipt after reauthorization."
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"NotificationInboxCounts", "NotificationInboxTask", "NotificationInboxToolInputTarget", "NotificationInboxItem", "NotificationInboxPage", "NotificationOrganizationInboxItem", "NotificationOrganizationInboxPage", "NotificationOrganizationUnreadSummary", "NotificationInboxCommand", "NotificationInboxCommandReceipt", "NotificationInboxBatchTarget", "NotificationInboxBatchCommand", "NotificationInboxBatchResult", "NotificationInboxBatchReceipt"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		if name != "NotificationInboxCommand" && name != "NotificationInboxBatchCommand" && name != "NotificationInboxBatchTarget" {
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
	field("NotificationInboxBatchTarget", "project_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("NotificationInboxBatchTarget", "notification_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("NotificationInboxBatchTarget", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("NotificationInboxBatchCommand", "items", map[string]any{"minItems": 1, "maxItems": 20, "description": "Explicit unique notification IDs and their original versions, in requested order."})
	field("NotificationInboxBatchCommand", "action", map[string]any{"enum": []string{"read", "unread", "archive", "restore"}})
	field("NotificationInboxBatchCommand", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128})
	field("NotificationInboxBatchReceipt", "results", map[string]any{"minItems": 1, "maxItems": 20, "description": "One result for each requested target in original order. The receipt contains private notification state only."})
	// Source domains contribute fields below; the inbox owns the complete kind
	// enumeration so a later domain cannot silently remove another source.
	field("NotificationInboxItem", "kind", map[string]any{"enum": []string{"task_assigned", "task_mentioned", "task_review_requested", "task_handoff_requested", "task_followed", "task_run_failed", "task_tool_input_requested"}})
	field("NotificationInboxItem", "handoff_id", map[string]any{"minLength": 1, "maxLength": 64, "description": "Exact handoff ID only when the current task, immutable recipient/basis and reverse private notification binding all match. Opening is a read, never acceptance or marking the notification read."})
	field("NotificationInboxItem", "run_id", map[string]any{"minLength": 1, "maxLength": 64, "description": "Exact original run ID only for an intact task_run_failed source with a persisted failure event and matching task binding. Opening does not change run, task, or notification state."})
	field("NotificationInboxItem", "tool_input_target", map[string]any{"description": "Present only for an intact task_tool_input_requested source. This locates the original question in its historical attempt. The workbench rechecks current access and answer eligibility."})
	field("NotificationInboxToolInputTarget", "input_id", map[string]any{"pattern": "^[0-9a-f]{32}$", "description": "Opaque ID of the original tool input within this attempt."})
	field("NotificationInboxToolInputTarget", "run_id", map[string]any{"minLength": 1, "maxLength": 100})
	field("NotificationInboxToolInputTarget", "node_id", map[string]any{"minLength": 1, "maxLength": 150})
	field("NotificationInboxToolInputTarget", "attempt", map[string]any{"minimum": 1, "maximum": int64(9007199254740991)})
	field("NotificationInboxItem", "sequence", map[string]any{"type": "string", "pattern": "^[1-9][0-9]{0,18}$", "description": "Positive signed BIGINT decimal string; never coerce to an imprecise JSON number."})
	field("NotificationInboxPage", "items", map[string]any{"maxItems": 50})
	field("NotificationInboxPage", "limit", map[string]any{"minimum": 1, "maximum": 50})
	field("NotificationOrganizationInboxPage", "items", map[string]any{"maxItems": 50})
	field("NotificationOrganizationInboxPage", "limit", map[string]any{"minimum": 1, "maximum": 50})
	field("NotificationOrganizationInboxPage", "scope_revision", map[string]any{"pattern": "^[0-9a-f]{64}$", "description": "Opaque equality marker for the current readable project set. Do not append pages from different markers."})
	field("NotificationOrganizationUnreadSummary", "unread", map[string]any{"minimum": 0})
	for _, key := range []string{"inbox", "unread", "archived"} {
		field("NotificationInboxCounts", key, map[string]any{"minimum": 0})
	}
}
