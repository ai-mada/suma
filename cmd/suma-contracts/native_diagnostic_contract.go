package main

import "github.com/ai-mada/suma/internal/localization"

// Keep generated clients precise without treating schema validation as source,
// permission or assignment attestation. The server also proves the complete raw
// message and enforces integer lexemes and UTF-8 byte bounds.
func nativeDiagnosticContracts(schemas map[string]any) {
	d := toolContractMap(schemas, "ToolDiagnostic")
	d["additionalProperties"] = false
	toolContractMap(d, "properties", "schema_version")["const"] = 1
	keys := []string{}
	cases := []any{}
	for _, entry := range localization.NativeCatalog() {
		keys = append(keys, entry.Key)
		properties := map[string]any{}
		required := []string{}
		for _, p := range entry.Parameters {
			rule := map[string]any{"type": "string", "minLength": 1, "maxLength": 512, "x-max-utf8-bytes": 512, "description": "Valid UTF-8 without control characters."}
			if p.Kind == localization.Integer {
				rule = map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991, "x-integer-lexeme": true, "description": "Exact nonnegative safe integer lexeme; fractional/exponent encodings rejected server-side."}
			}
			properties[p.Name] = rule
			required = append(required, p.Name)
		}
		params := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
		then := map[string]any{"properties": map[string]any{"message_params": params}}
		if len(required) > 0 {
			params["required"] = required
			then["required"] = []string{"message_params"}
		}
		cases = append(cases, map[string]any{"if": map[string]any{"required": []string{"message_key"}, "properties": map[string]any{"message_key": map[string]any{"const": entry.Key}}}, "then": then})
	}
	toolContractMap(d, "properties", "message_key")["enum"] = keys
	d["allOf"] = cases
	d["description"] = "Versioned native diagnostic identity only. No locale or rendered prose. Requires full canonical-source equality with ToolEvent.message; never grants authority or changes terminal/usage/test evidence."
	e := toolContractMap(schemas, "ToolEvent")
	e["allOf"] = []any{map[string]any{"if": map[string]any{"required": []string{"diagnostic"}, "properties": map[string]any{"diagnostic": map[string]any{"type": "object"}}}, "then": map[string]any{"properties": map[string]any{"protocol": map[string]any{"const": "suma-jsonl-v1"}, "kind": map[string]any{"const": "error"}}}}}
}
