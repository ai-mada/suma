package main

import "github.com/ai-mada/suma/internal/platform"

func workBuddyConnectionModels() []any {
	return []any{platform.WorkBuddyConnection{}, platform.WorkBuddyConnectionPage{}, platform.WorkBuddyAuthorizationRequest{}, platform.WorkBuddyAuthorization{}, platform.WorkBuddyRefreshRequest{}, platform.WorkBuddyDisconnectRequest{}, platform.WorkBuddyCallbackRequest{}, platform.WorkBuddyCallbackResult{}}
}

// These commands own browser-scoped provider grants. They do not add an
// executor, transfer tokens to clients, or grant access to another account.
func workBuddyConnectionContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/workbuddy-connections", "get", "", "WorkBuddyConnectionPage", "200"},
		{"/workbuddy-connections/{id}", "get", "", "WorkBuddyConnection", "200"},
		{"/workbuddy-authorizations", "post", "WorkBuddyAuthorizationRequest", "WorkBuddyAuthorization", "201"},
		{"/workbuddy-authorizations/{id}", "get", "", "WorkBuddyAuthorization", "200"},
		{"/workbuddy-connections/{id}/refresh", "post", "WorkBuddyRefreshRequest", "WorkBuddyConnection", "200"},
		{"/workbuddy-connections/{id}/disconnect", "post", "WorkBuddyDisconnectRequest", "WorkBuddyConnection", "200"},
		{"/workbuddy-oauth/callback", "post", "WorkBuddyCallbackRequest", "WorkBuddyCallbackResult", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		browserOnly(op)
		successStatus(op, route.status)
		op["description"] = "Current browser session and project authority, private to the original human owner. Authorization is distinct from execution readiness. Codes and tokens never enter response projections or operation receipts. Unknown external effects are not automatically repeated."
		if route.method == "get" {
			op["x-read-only"] = true
		} else {
			request(op, route.input)
			var parameters []any
			for _, raw := range op["parameters"].([]any) {
				if raw.(map[string]any)["in"] != "query" {
					parameters = append(parameters, raw)
				}
			}
			op["parameters"] = parameters
		}
		if route.path == "/workbuddy-connections" {
			appendQuery(op, queryParameter("limit", "Bounded owner-scoped connection page.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Opaque continuation bound to the current owner and project. Every page is a fresh current-authorized read.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
		}
	}

	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"WorkBuddyConnection", "WorkBuddyConnectionPage", "WorkBuddyAuthorization", "WorkBuddyAuthorizationRequest", "WorkBuddyRefreshRequest", "WorkBuddyDisconnectRequest", "WorkBuddyCallbackRequest", "WorkBuddyCallbackResult"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		if name == "WorkBuddyConnection" || name == "WorkBuddyConnectionPage" || name == "WorkBuddyAuthorization" || name == "WorkBuddyCallbackResult" {
			for _, raw := range toolContractMap(model, "properties") {
				raw.(map[string]any)["readOnly"] = true
			}
		}
	}
	field := func(model, key string, attrs map[string]any) {
		for name, value := range attrs {
			toolContractMap(schemas, model, "properties", key)[name] = value
		}
	}
	scopes := map[string]any{"type": "string", "enum": []string{"user.task.readable", "user.task.invokable"}}
	for _, pair := range [][2]string{{"WorkBuddyConnection", "scopes"}, {"WorkBuddyConnectionPage", "available_scopes"}, {"WorkBuddyAuthorizationRequest", "scopes"}} {
		field(pair[0], pair[1], map[string]any{"maxItems": 2, "uniqueItems": true, "items": scopes})
	}
	field("WorkBuddyAuthorizationRequest", "scopes", map[string]any{"minItems": 1})
	for _, name := range []string{"WorkBuddyAuthorizationRequest", "WorkBuddyRefreshRequest", "WorkBuddyDisconnectRequest"} {
		field(name, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128})
		field(name, "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("WorkBuddyAuthorizationRequest", "connection_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("WorkBuddyAuthorizationRequest", "expected_version", map[string]any{"description": "Must be supplied with connection_id for explicit reauthorization; omit both when creating a new connection."})
	field("WorkBuddyDisconnectRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 8000, "description": "Local disablement only; does not revoke the provider grant or stop remote tasks."})
	field("WorkBuddyConnectionPage", "items", map[string]any{"maxItems": 50})
	field("WorkBuddyConnectionPage", "limit", map[string]any{"minimum": 1, "maximum": 50})
	field("WorkBuddyConnection", "state", map[string]any{"enum": []string{"connected", "authorizing", "refreshing", "authorization_unknown", "refresh_unknown", "reauthorization_required", "disconnected"}})
	field("WorkBuddyConnection", "allowed_actions", map[string]any{"maxItems": 3, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": []string{"authorize", "refresh", "disconnect"}}})
	field("WorkBuddyAuthorization", "authorization_url", map[string]any{"format": "uri", "maxLength": 8192, "description": "Original browser session only while pending. Keep in memory; the user explicitly follows the operator-controlled official authorization URL."})
	for _, key := range []string{"state", "code", "error"} {
		field("WorkBuddyCallbackRequest", key, map[string]any{"writeOnly": true})
	}
	field("WorkBuddyCallbackRequest", "state", map[string]any{"minLength": 1, "maxLength": 256})
	field("WorkBuddyCallbackRequest", "code", map[string]any{"maxLength": 8192})
	field("WorkBuddyCallbackRequest", "error", map[string]any{"enum": []string{"", "access_denied"}})
	toolContractMap(schemas, "WorkBuddyCallbackRequest")["oneOf"] = []any{
		map[string]any{"required": []string{"code"}, "properties": map[string]any{"code": map[string]any{"minLength": 1}, "error": map[string]any{"const": ""}}},
		map[string]any{"required": []string{"error"}, "properties": map[string]any{"error": map[string]any{"const": "access_denied"}, "code": map[string]any{"const": ""}}},
	}
	field("WorkBuddyCallbackResult", "redirect_url", map[string]any{"description": "Server-built same-origin relative tools route containing only safe project and connection IDs. Never a caller-supplied redirect or OAuth response."})
	landing := add("/workbuddy-oauth/callback", "get", "", false, false)
	landing["parameters"] = []any{}
	landing["security"] = []any{}
	landing["description"] = "Isolated no-store/no-referrer nonce-CSP callback bridge. GET neither consumes a grant nor calls the provider. It immediately removes response query data before an authenticated same-origin POST with the original browser-view headers. No OAuth code or state enters the SPA, storage or a response DTO."
	landing["x-no-external-effects"] = true
	landing["responses"] = map[string]any{"200": map[string]any{"description": "Static isolated callback bridge", "content": map[string]any{"text/html": map[string]any{"schema": map[string]any{"type": "string"}}}}}
}
