package platform

// Inline JSON Schema is self-contained for model providers; no component $ref
// leaves the control plane. The IDs/digest select data and never grant access.
func knowledgeReferenceParameters() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"references"}, "properties": map[string]any{
		"references": map[string]any{"type": "array", "minItems": 1, "maxItems": 16, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "revision", "sha256"}, "properties": map[string]any{
			"id":       map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"},
			"revision": map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483647},
			"sha256":   map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"},
		}}},
	}}
}
