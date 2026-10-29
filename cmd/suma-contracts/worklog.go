package main

import "github.com/ai-mada/suma/internal/platform"

func worklogModels() []any {
	return []any{platform.Worklog{}, platform.WorklogCreateRequest{}, platform.WorklogCorrectionRequest{}, platform.WorklogVoidRequest{}, platform.WorklogListRequest{}, platform.WorklogPage{}, platform.WorklogRevisionPage{}}
}

func worklogContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/worklogs", "get", "", "WorklogPage", "200"},
		{"/worklogs", "post", "WorklogCreateRequest", "Worklog", "201"},
		{"/worklogs/{id}", "get", "", "Worklog", "200"},
		{"/worklogs/{id}/corrections", "post", "WorklogCorrectionRequest", "Worklog", "200"},
		{"/worklogs/{id}/void", "post", "WorklogVoidRequest", "Worklog", "200"},
		{"/worklogs/{id}/revisions", "get", "", "WorklogRevisionPage", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current-authorized manual human work register. Self entries require current project write authority and a bound human resource; acting for another person requires a browser manager and explicit acknowledgement. Corrections/void preserve immutable revisions. Integer minutes are transactionally limited to1440 per organization/account/date across projects, separately from estimates, machine usage, billing or approval. No automatic human time inference."
		if route.input != "" {
			request(op, route.input)
			// Typed writes select their project only from the closed body. Do
			// not advertise the generic optional query that this handler rejects.
			parameters := []any{}
			for _, item := range op["parameters"].([]any) {
				p := item.(map[string]any)
				if p["in"] == "query" && p["name"] == "project_id" {
					continue
				}
				parameters = append(parameters, item)
			}
			op["parameters"] = parameters
		} else {
			op["x-read-only"] = true
		}
		if route.path == "/worklogs" && route.method == "get" {
			for _, name := range []string{"from_date", "to_date"} {
				p := queryParameter(name, "Required inclusive business date, YYYY-MM-DD. Complete range may span at most31 days.", map[string]any{"type": "string", "format": "date"})
				p["required"] = true
				appendQuery(op, p)
			}
			for _, name := range []string{"member_id", "task_id"} {
				appendQuery(op, queryParameter(name, "Optional current-head filter; no inference from task assignee.", map[string]any{"type": "string"}))
			}
			appendQuery(op, queryParameter("status", "Current head state; summaries cover exactly these filters, not revision history.", map[string]any{"type": "string", "enum": []string{"all", "active", "voided"}, "default": "all"}))
		}
		if route.method == "get" && route.output != "Worklog" {
			appendQuery(op, queryParameter("limit", "Bounded page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Opaque scope/filter-bound keyset cursor. Scope changes require a fresh first page.", map[string]any{"type": "string"}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"Worklog", "WorklogCreateRequest", "WorklogCorrectionRequest", "WorklogVoidRequest", "WorklogListRequest", "WorklogSummary", "WorklogPage", "WorklogRevisionPage"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	for _, name := range []string{"Worklog", "WorklogCreateRequest", "WorklogCorrectionRequest"} {
		minutes := toolContractMap(schemas, name, "properties", "minutes")
		minutes["minimum"], minutes["maximum"] = 1, 1440
		toolContractMap(schemas, name, "properties", "work_date")["format"] = "date"
		toolContractMap(schemas, name, "properties", "note")["maxLength"] = 200
	}
	for _, name := range []string{"WorklogCreateRequest", "WorklogCorrectionRequest", "WorklogVoidRequest"} {
		key := toolContractMap(schemas, name, "properties", "idempotency_key")
		// JSON Schema length counts Unicode codepoints, while operation keys
		// use the shared8–128 UTF-8 byte bound. Two four-byte codepoints can
		// already satisfy the lower bound; express byte checks explicitly.
		key["minLength"], key["maxLength"] = 2, 128
		key["x-min-utf8-bytes"], key["x-max-utf8-bytes"] = 8, 128
		key["description"] = "Stable original-operation key,8–128 UTF-8 bytes. Byte limits are server-enforced; JSON Schema string length counts codepoints."
		toolContractMap(schemas, name, "properties", "on_behalf_ack")["description"] = "Explicitly acknowledge acting for another person; never grants authority. Required complete boolean, false for self is valid."
	}
	for _, name := range []string{"WorklogCorrectionRequest", "WorklogVoidRequest"} {
		toolContractMap(schemas, name, "properties", "expected_version")["minimum"] = 1
		reason := toolContractMap(schemas, name, "properties", "reason")
		reason["minLength"], reason["maxLength"] = 1, 200
	}
	for _, name := range []string{"WorklogCreateRequest", "WorklogCorrectionRequest"} {
		toolContractMap(schemas, name, "properties", "expected_task_version")["minimum"] = 1
	}
	toolContractMap(schemas, "Worklog", "properties", "source")["const"] = "human_manual"
	toolContractMap(schemas, "Worklog", "properties", "status")["enum"] = []string{"active", "voided"}
	for _, name := range []string{"Worklog", "WorklogPage", "WorklogSummary", "WorklogRevisionPage"} {
		for _, field := range toolContractMap(schemas, name, "properties") {
			field.(map[string]any)["readOnly"] = true
		}
	}
}
