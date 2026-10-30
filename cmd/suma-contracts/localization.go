package main

import "github.com/ai-mada/suma/internal/platform"

func localizationModels() []any {
	return []any{platform.LocalizationCatalog{}, platform.LocalePreferenceUpdateRequest{}, platform.AccountPreferences{}, platform.OrganizationPreferences{}, platform.WorkflowValidation{}}
}

func localePreferenceRoutes(add operationAdder) {
	for path, model := range map[string]string{"/account/preferences": "AccountPreferences", "/organization/preferences": "OrganizationPreferences"} {
		get := add(path, "get", model, false, false)
		browserOnly(get)
		successStatus(get, "200")
		patch := add(path, "patch", model, false, true)
		browserOnly(patch)
		successStatus(patch, "200")
		request(patch, "LocalePreferenceUpdateRequest", "locale", "expected_version", "idempotency_key")
		// Bind the displayed identity independently of ambient cookies, which
		// another browser tab can replace between rendering and submitting.
		appendQuery(patch, map[string]any{"in": "header", "name": "X-Suma-Expected-User", "required": true, "schema": map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$", "minLength": 32, "maxLength": 32}, "description": "Exactly one original-view user ID header. Required on new writes and exact replays; checked against current browser identity inside the write transaction. Not authorization. Account writes forbid an expected-organization header."})
		patch["x-concurrency"] = "Explicit version CAS and original-view identity binding; zero means no stored preference. Replays recheck current browser identity and organization authority. A historical receipt confirms that operation, not the current preference."
		if path == "/organization/preferences" {
			patch["x-required-organization-roles"] = []string{"owner", "admin"}
			appendQuery(patch, map[string]any{"in": "header", "name": "X-Suma-Expected-Organization", "required": true, "schema": map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$", "minLength": 32, "maxLength": 32}, "description": "Exactly one original-view organization ID header. Shared-cookie organization drift must fail before reading a receipt or modifying any preference."})
		}
	}
}

// Localized prose is additive metadata. Stable protocol codes, record data and
// execution receipts do not vary with the selected display language.
func localizationContracts(api map[string]any) {
	preference := toolContractMap(api, "components", "schemas", "LocalePreferenceUpdateRequest")
	preference["additionalProperties"] = false
	fields := preference["properties"].(map[string]any)
	fields["locale"] = map[string]any{"type": "string", "maxLength": 128, "description": "Empty string explicitly inherits defaults. Supported BCP47 aliases are canonicalized to zh-CN/en-US; the original submitted lexeme remains part of operation identity."}
	fields["expected_version"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 2147483646, "description": "Required JSON integer lexeme; missing/null/fractional/exponent forms are rejected."}
	fields["idempotency_key"] = map[string]any{"type": "string", "minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128, "description": "Stable operation identity; no control characters. Do not regenerate after an unconfirmed response."}
	for _, model := range []string{"AccountPreferences", "OrganizationPreferences"} {
		props := toolContractMap(api, "components", "schemas", model, "properties")
		props["locale"].(map[string]any)["enum"] = []string{"", "zh-CN", "en-US"}
		props["version"].(map[string]any)["minimum"] = 0
	}
	toolContractMap(api, "components", "schemas", "LocalizationParameter", "properties", "kind")["enum"] = []string{"text", "identifier", "integer"}
	toolContractMap(api, "components", "schemas", "LocalizationCatalog", "properties", "template_format")["const"] = "named-dot-v1"
	errorModel := toolContractMap(api, "components", "schemas", "Error", "properties", "error")
	properties := errorModel["properties"].(map[string]any)
	properties["message_key"] = map[string]any{"type": "string", "description": "Stable catalog key; untranslated.<code> with localized=false explicitly denotes an unmigrated exact diagnostic."}
	properties["locale"] = map[string]any{"type": "string", "enum": []string{"zh-CN", "en-US"}}
	properties["localized"] = map[string]any{"type": "boolean", "description": "False means the original exact source is preserved rather than hidden by a generic translated validation message."}
	properties["fallback"] = map[string]any{"type": "string"}
	properties["message_params"] = map[string]any{"type": "object", "additionalProperties": true}
	errorModel["required"] = []string{"code", "message", "message_key", "locale", "localized"}
	for _, path := range api["paths"].(map[string]any) {
		for _, raw := range path.(map[string]any) {
			op := raw.(map[string]any)
			appendQuery(op, map[string]any{"in": "header", "name": "Accept-Language", "description": "BCP47 preference list. Supported locales zh-CN and en-US; default zh-CN. Presentation only, never authorization or operation identity.", "schema": map[string]any{"type": "string", "maxLength": 8192}})
			appendQuery(op, map[string]any{"in": "header", "name": "X-Suma-Language", "description": "Explicit display choice. For verified browser identities: explicit header, account preference, current organization default, Accept-Language, then zh-CN. Other credentials keep request negotiation; display choice never grants authority.", "schema": map[string]any{"type": "string", "maxLength": 128}})
			for code, response := range op["responses"].(map[string]any) {
				r := response.(map[string]any)
				// Presentation metadata augments protocol headers. Replacing this map
				// would silently discard security and cache controls declared by a domain.
				headers, ok := r["headers"].(map[string]any)
				if !ok {
					headers = make(map[string]any)
					r["headers"] = headers
				}
				headers["Content-Language"] = map[string]any{"description": "Negotiated request presentation language; does not imply every raw diagnostic or raw log is translated.", "schema": map[string]any{"type": "string", "enum": []string{"zh-CN", "en-US"}}}
				if code == "400" || code == "401" || code == "403" || code == "409" {
					r["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}
				}
			}
		}
	}
}
