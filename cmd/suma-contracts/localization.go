package main

import "github.com/ai-mada/suma/internal/platform"

func localizationModels() []any { return []any{platform.LocalizationCatalog{}} }

// Localized prose is additive metadata. Stable protocol codes, record data and
// execution receipts do not vary with the selected display language.
func localizationContracts(api map[string]any) {
	toolContractMap(api, "components", "schemas", "LocalizationParameter", "properties", "kind")["enum"] = []string{"text", "identifier", "integer"}
	toolContractMap(api, "components", "schemas", "LocalizationCatalog", "properties", "template_format")["const"] = "named-dot-v1"
	errorModel := toolContractMap(api, "components", "schemas", "Error", "properties", "error")
	properties := errorModel["properties"].(map[string]any)
	properties["message_key"] = map[string]any{"type": "string", "description": "Stable catalog key; legacy.<code> with localized=false explicitly denotes an unmigrated exact diagnostic."}
	properties["locale"] = map[string]any{"type": "string", "enum": []string{"zh-CN", "en-US"}}
	properties["localized"] = map[string]any{"type": "boolean", "description": "False means the original exact source is preserved rather than hidden by a generic translated validation message."}
	properties["fallback"] = map[string]any{"type": "string"}
	properties["message_params"] = map[string]any{"type": "object", "additionalProperties": true}
	errorModel["required"] = []string{"code", "message", "message_key", "locale", "localized"}
	for _, path := range api["paths"].(map[string]any) {
		for _, raw := range path.(map[string]any) {
			op := raw.(map[string]any)
			appendQuery(op, map[string]any{"in": "header", "name": "Accept-Language", "description": "BCP47 preference list. Supported locales zh-CN and en-US; default zh-CN. Presentation only, never authorization or operation identity.", "schema": map[string]any{"type": "string", "maxLength": 8192}})
			appendQuery(op, map[string]any{"in": "header", "name": "X-Suma-Language", "description": "Optional explicit display choice overriding Accept-Language. Unknown choices fall through negotiation; account/organization preferences are not yet persisted.", "schema": map[string]any{"type": "string", "maxLength": 128}})
			for code, response := range op["responses"].(map[string]any) {
				r := response.(map[string]any)
				r["headers"] = map[string]any{"Content-Language": map[string]any{"description": "Negotiated request presentation language; does not imply every legacy diagnostic or raw log is translated.", "schema": map[string]any{"type": "string", "enum": []string{"zh-CN", "en-US"}}}}
				if code == "400" || code == "401" || code == "403" || code == "409" {
					r["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}
				}
			}
		}
	}
}
