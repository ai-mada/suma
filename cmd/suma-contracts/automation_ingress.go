package main

import (
	"sort"

	"github.com/ai-mada/suma/internal/platform"
)

func automationIngressModels() []any {
	return []any{platform.AutomationIngressSpec{}, platform.AutomationIngress{}, platform.AutomationIngressCreateRequest{}, platform.AutomationIngressCommandRequest{}, platform.AutomationIngressRevision{}, platform.AutomationIngressList{}, platform.AutomationIngressRevisions{}, platform.AutomationIngressEvent{}, platform.AutomationIngressDelivery{}, platform.AutomationIngressDeliveries{}, platform.AutomationIngressReceipt{}}
}

func automationIngressContracts(add operationAdder) {
	get := func(path, model string, paged bool) {
		op := add(path, "get", model, false, false)
		successStatus(op, "200")
		browserOnly(op)
		if paged {
			appendQuery(op, queryParameter("limit", "Default 50, maximum 200.", map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}))
			appendQuery(op, queryParameter("cursor", "Opaque cursor scoped to organization, project, rule/ingress and collection.", map[string]any{"type": "string"}))
		}
	}
	get("/automations/{id}/ingresses", "AutomationIngressList", true)
	get("/automation-ingresses/{id}", "AutomationIngress", false)
	get("/automation-ingresses/{id}/revisions", "AutomationIngressRevisions", true)
	get("/automation-ingresses/{id}/deliveries", "AutomationIngressDeliveries", true)
	get("/automation-ingresses/{id}/deliveries/{delivery}", "AutomationIngressDelivery", false)
	op := add("/automations/{id}/ingresses", "post", "AutomationIngress", false, true)
	browserOnly(op)
	successStatus(op, "201")
	request(op, "AutomationIngressCreateRequest")
	op["description"] = "Creates paused metadata bound to the current immutable automation revision. Requires a real organization session with current write and approval authority. No signing secret is created or exposed."
	op = add("/automation-ingresses/{id}/commands", "post", "AutomationIngress", false, true)
	browserOnly(op)
	successStatus(op, "200")
	request(op, "AutomationIngressCommandRequest")
	op["description"] = "Versioned revise/enable/pause/revoke/rotate_key. Changes and key rotation pause the ingress. Enable/revise require current rule version and revision. Rotation accepts a caller-provided 64-character secret, stores only AES-GCM ciphertext and a global non-reuse fingerprint tombstone, and never echoes the secret in a receipt."
	op = add("/hooks/{id}", "post", "AutomationIngressReceipt", false, true)
	op["security"] = []any{map[string]any{"githubSignature": []string{}}, map[string]any{"sumaSignature": []string{}}}
	op["responses"].(map[string]any)["202"] = op["responses"].(map[string]any)["201"]
	delete(op["responses"].(map[string]any), "201")
	op["responses"].(map[string]any)["202"].(map[string]any)["description"] = "Signed delivery and any occurrence durably committed; not run success or test acceptance."
	op["responses"].(map[string]any)["200"].(map[string]any)["description"] = "Original persisted receipt recovered without another occurrence."
	for code, description := range map[string]string{"413": "Raw body exceeds 2 MiB; never silently truncated.", "429": "Process concurrency or durable per-ingress admission limit reached."} {
		op["responses"].(map[string]any)[code] = map[string]any{"description": description}
	}
	op["requestBody"] = map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"anyOf": []any{map[string]any{"$ref": "#/components/schemas/AutomationIngressEvent"}, map[string]any{"type": "object", "description": "GitHub native JSON. Raw bytes are verified before bounded parsing; repository numeric identity and the selected event structure are required. Unknown native fields are discarded."}}}}}}
	headers := map[string]string{"X-Hub-Signature-256": "GitHub sha256=<64 hex> HMAC of exact raw body.", "X-GitHub-Delivery": "Native GitHub delivery identity; not protected by the body signature.", "X-GitHub-Event": "Native event header; verified against the corresponding body structure.", "X-Suma-Delivery": "Stable safe-ASCII delivery identity signed with body, ingress and timestamp.", "X-Suma-Timestamp": "Canonical Unix seconds within database time +/-300 seconds.", "X-Suma-Signature-256": "sha256=<64 hex>; suma-v1 LF ingress_id LF timestamp LF delivery_id LF raw_body."}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		description := headers[name]
		op["parameters"] = append(op["parameters"].([]any), map[string]any{"in": "header", "name": name, "description": description, "required": false, "schema": map[string]any{"type": "string"}})
	}
	op["description"] = "Signature-only receiver: rejects browser cookies, Authorization, Origin, query strings, encodings and duplicate protocol headers. GitHub stores body and delivery-ID tombstones across revisions; it cannot prove unseen old payload freshness. Suma binds ingress, timestamp, delivery ID and exact body. Only normalized source facts are retained; no payload-to-command mapping. Receipt and occurrence are committed atomically; dispatch rechecks current ingress/rule authority and fixed workflow."
	op["x-request-schema-coverage"] = "Exact raw-body signature, provider-specific parser, bounded normalized event, current authority and transactional deduplication are handler-enforced. Header requirements depend on the stored provider."
}

func automationIngressSecurity(api map[string]any) {
	schemes := api["components"].(map[string]any)["securitySchemes"].(map[string]any)
	schemes["githubSignature"] = map[string]any{"type": "apiKey", "in": "header", "name": "X-Hub-Signature-256", "description": "Computed HMAC-SHA256, not the signing secret. Requires native delivery/event headers; raw body is verified with the encrypted ingress key."}
	schemes["sumaSignature"] = map[string]any{"type": "apiKey", "in": "header", "name": "X-Suma-Signature-256", "description": "Computed HMAC-SHA256, not a session token. Requires X-Suma-Delivery and X-Suma-Timestamp signed with the ingress and raw body."}
}

func enhanceAutomationIngressSchemas(schemas map[string]any) {
	field := func(model, name string, values map[string]any) {
		p := schemas[model].(map[string]any)["properties"].(map[string]any)[name].(map[string]any)
		for k, v := range values {
			p[k] = v
		}
	}
	for _, model := range []string{"AutomationIngressSpec", "AutomationIngressCreateRequest", "AutomationIngressCommandRequest", "AutomationIngressEvent"} {
		schemas[model].(map[string]any)["additionalProperties"] = false
	}
	for _, model := range []string{"AutomationIngressSpec", "AutomationIngress"} {
		field(model, "name", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
		field(model, "provider", map[string]any{"enum": []string{"github", "suma"}})
		field(model, "events", map[string]any{"minItems": 1, "maxItems": 8, "uniqueItems": true, "description": "Provider-specific allowlist: github push/workflow_run.completed/ping; suma git.push/ci.completed/monitor.firing/monitor.resolved."})
		field(model, "conclusions", map[string]any{"maxItems": 16, "uniqueItems": true})
		field(model, "repository", map[string]any{"maxLength": 201})
		field(model, "repository_id", map[string]any{"pattern": "^([1-9][0-9]{0,29})?$"})
		field(model, "ref", map[string]any{"maxLength": 250, "description": "Empty allows all branches and tags; otherwise exact refs/heads/... or refs/tags/...; configured filters require a present matching event field."})
	}
	field("AutomationIngress", "state", map[string]any{"enum": []string{"paused", "enabled", "revoked"}})
	field("AutomationIngress", "key_fingerprint", map[string]any{"description": "SHA256 of signing secret bytes for configuration recovery; never the secret or encryption key."})
	field("AutomationIngressCommandRequest", "action", map[string]any{"enum": []string{"revise", "enable", "pause", "revoke", "rotate_key"}})
	field("AutomationIngressCommandRequest", "secret", map[string]any{"pattern": "^[a-f0-9]{64}$", "writeOnly": true, "description": "Only rotate_key. Literal 64-character UTF-8 key, not hex-decoded; encrypted at rest, never repeated in public receipts."})
	for _, pair := range [][2]string{{"AutomationIngressCreateRequest", "expected_rule_version"}, {"AutomationIngressCreateRequest", "expected_rule_revision"}, {"AutomationIngressCommandRequest", "expected_version"}, {"AutomationIngressCommandRequest", "expected_rule_version"}, {"AutomationIngressCommandRequest", "expected_rule_revision"}} {
		field(pair[0], pair[1], map[string]any{"minimum": 1, "maximum": 2147483647})
	}
	schemas["AutomationIngressEvent"].(map[string]any)["required"] = []string{"schema_version", "event", "external_id"}
	field("AutomationIngressEvent", "schema_version", map[string]any{"const": 1})
	field("AutomationIngressEvent", "event", map[string]any{"minLength": 1, "maxLength": 80})
	field("AutomationIngressEvent", "external_id", map[string]any{"maxLength": 128})
	field("AutomationIngressEvent", "commit", map[string]any{"pattern": "^([a-f0-9]{40}|[a-f0-9]{64})?$"})
	field("AutomationIngressDelivery", "state", map[string]any{"enum": []string{"accepted", "ignored", "blocked"}})
	field("AutomationIngressReceipt", "state", map[string]any{"enum": []string{"accepted", "ignored", "blocked"}})
	field("AutomationOccurrence", "source", map[string]any{"enum": []string{"cron", "manual", "webhook"}})
}
