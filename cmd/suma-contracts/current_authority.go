package main

import (
	"github.com/ai-mada/suma/internal/platform"
	"strings"
)

// Browser commands share the server's pure route classifier. Scope-bound API
// and machine credentials keep their own contracts and do not borrow a view.
func currentAuthorityContracts(api map[string]any) {
	paths := api["paths"].(map[string]any)
	for path, entry := range paths {
		for method, raw := range entry.(map[string]any) {
			op := raw.(map[string]any)
			if !platform.BrowserCommandRequiresView(strings.ToUpper(method), strings.TrimPrefix(path, "/")) {
				continue
			}
			hasBrowser, onlyBrowser := false, true
			for _, scheme := range op["security"].([]any) {
				for name := range scheme.(map[string]any) {
					if name == "cookieAuth" {
						hasBrowser = true
					} else {
						onlyBrowser = false
					}
				}
			}
			if !hasBrowser {
				continue
			}
			op["x-browser-original-view"] = map[string]any{"required": true, "account_only": path == "/account/preferences", "missing_status": 428, "mismatch_status": 403, "is_authorization": false}
			for _, name := range []string{"X-Suma-Expected-User", "X-Suma-Expected-Organization"} {
				if path == "/account/preferences" && name == "X-Suma-Expected-Organization" {
					continue
				}
				exists := false
				for _, rawParameter := range op["parameters"].([]any) {
					p := rawParameter.(map[string]any)
					if p["in"] == "header" && p["name"] == name {
						exists = true
					}
				}
				if !exists {
					appendQuery(op, map[string]any{"in": "header", "name": name, "required": onlyBrowser, "x-required-for-credential-kind": "browser-session", "schema": map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$", "minLength": 32, "maxLength": 32}, "description": "Original rendered account/organization identity. Required for browser commands and exact retries; scoped API credentials do not need it. Compared with current identity, never used to select or authorize an account or organization."})
				}
			}
			op["responses"].(map[string]any)["428"] = map[string]any{"description": "Missing, duplicate or invalid original-view context for a browser command. Refresh the view; do not silently select another identity.", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/Error"}}}}
		}
	}
	tokens := paths["/auth/tokens"].(map[string]any)["post"].(map[string]any)
	request(tokens, "ProjectCredentialIssueRequest", "project_id", "idempotency_key")
	successStatus(tokens, "201")
	tokens["responses"].(map[string]any)["201"].(map[string]any)["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ProjectCredentialIssueResult"}}}
	tokens["description"] = "Current browser organization owner/admin explicitly issues an independent project-scoped API, Runner or Agent Grant for 24 hours. Browser logout does not revoke the independent Grant. Exact same-key retries rotate the secret in the same fixed-expiry slot and revoke the prior secret; plaintext secrets are never stored in receipts. Unclassified legacy sessions require login and explicit reissuance."
	input := toolContractMap(api, "components", "schemas", "ProjectCredentialIssueRequest")
	input["additionalProperties"] = false
	fields := input["properties"].(map[string]any)
	fields["scope"].(map[string]any)["enum"] = []string{"api", "runner", "agent"}
	fields["scope"].(map[string]any)["default"] = "runner"
	fields["label"].(map[string]any)["maxLength"] = 200
	fields["idempotency_key"].(map[string]any)["minLength"] = 8
	fields["idempotency_key"].(map[string]any)["maxLength"] = 128
	logout := paths["/auth/logout"].(map[string]any)["post"].(map[string]any)
	logout["description"] = "Withdraw-only, idempotent logout. A browser credential revokes its complete fixed-expiry session family; a scoped credential revokes its exact credential slot. Unknown, already revoked and legacy secrets return an empty successful logout response and grant no other authority."
}
