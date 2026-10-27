package main

import "github.com/ai-mada/suma/internal/platform"

func usageAccountingModels() []any {
	return []any{platform.UsagePolicy{}, platform.UsagePolicyWriteRequest{}, platform.UsageRateRevision{}, platform.UsageRateWriteRequest{}, platform.UsageMoneyBalance{}, platform.UsageBalance{}, platform.UsageDashboard{}, platform.UsageAttempt{}, platform.UsageAttemptPage{}, platform.UsageConfirmRequest{}}
}

func usageAccountingContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/usage", "get", "", "UsageDashboard", "200"},
		{"/usage/policies", "post", "UsagePolicyWriteRequest", "UsagePolicy", "200"},
		{"/usage/rates", "post", "UsageRateWriteRequest", "UsageRateRevision", "201"},
		{"/usage/attempts", "get", "", "UsageAttemptPage", "200"},
		{"/usage/attempts/{id}/confirm", "post", "UsageConfirmRequest", "UsageAttempt", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current-authorized execution usage and operator-priced estimates. An attempt freezes its profile/model/rate and monthly organization/project allowance in the same transaction as dispatch. Final evidence is counted once; missing values are unknown, cache/reasoning counters are not additive, unknown outcomes retain holds. Currency balances remain separate. Estimates are not vendor invoices or a hard provider spend cap. Manual confirmation preserves the original evidence and cannot lower known usage."
		if route.input != "" {
			request(op, route.input)
		} else {
			appendQuery(op, queryParameter("month", "UTC natural month; omitted means the current database month.", map[string]any{"type": "string", "pattern": `^[0-9]{4}-(0[1-9]|1[0-2])$`}))
			if route.output == "UsageAttemptPage" {
				appendQuery(op, queryParameter("state", "Optional accounting state.", map[string]any{"type": "string", "enum": []string{"reserved", "settled", "partial", "unreported", "reconciling", "confirmed", "unpriced"}}))
				appendQuery(op, queryParameter("limit", "Exact decimal integer page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
				appendQuery(op, queryParameter("before", "Existing ID in this tenant/project/month and state; created_at/id descending keyset.", map[string]any{"type": "string", "maxLength": 64}))
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		for key, value := range attrs {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, model := range []string{"UsagePolicyWriteRequest", "UsageRateWriteRequest", "UsageConfirmRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "expected_version", map[string]any{"minimum": 0, "maximum": 2147483646})
		field(model, "reason", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, model := range []string{"UsagePolicy", "UsagePolicyWriteRequest"} {
		field(model, "scope", map[string]any{"enum": []string{"organization", "project"}})
		field(model, "mode", map[string]any{"enum": []string{"observe", "enforce"}})
	}
	for _, model := range []string{"UsagePolicy", "UsagePolicyWriteRequest", "UsageRateRevision", "UsageRateWriteRequest", "UsageMoneyBalance"} {
		field(model, "currency", map[string]any{"enum": []string{"CNY", "USD"}})
	}
	for _, name := range []string{"input_rate_micros_per_million", "output_rate_micros_per_million", "input_token_reservation", "output_token_reservation"} {
		field("UsageRateWriteRequest", name, map[string]any{"minimum": 0, "maximum": 9007199254740991})
	}
	field("UsageRateWriteRequest", "expected_tool_version", map[string]any{"minimum": 1})
	field("UsageRateWriteRequest", "tool_profile_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("UsageConfirmRequest", "expected_version", map[string]any{"minimum": 1})
	field("UsageConfirmRequest", "source_ref", map[string]any{"minLength": 1, "maxLength": 2000, "description": "Human-reviewed evidence reference. Recorded as manual confirmation, not vendor-measured usage."})
	field("UsageConfirmRequest", "acknowledge_usage", map[string]any{"const": true})
	field("UsageAttempt", "state", map[string]any{"enum": []string{"reserved", "settled", "partial", "unreported", "reconciling", "confirmed", "unpriced"}})
	field("UsageAttempt", "usage_source", map[string]any{"enum": []string{"none", "tool_evidence", "manual_confirmation", "host_startup_proof"}})
	field("UsageAttempt", "usage_state", map[string]any{"enum": []string{"complete", "partial", "unreported"}})
	for _, model := range []string{"UsagePolicy", "UsageRateRevision", "UsageMoneyBalance", "UsageBalance", "UsageDashboard", "UsageAttempt", "UsageAttemptPage"} {
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
}
