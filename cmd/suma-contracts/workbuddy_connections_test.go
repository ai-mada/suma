package main

import "testing"

func TestWorkBuddyConnectionsContractKeepsGrantAndDispatchAuthoritySeparate(t *testing.T) {
	api, _ := generate()
	for _, route := range [][2]string{{"/workbuddy-connections", "get"}, {"/workbuddy-connections/{id}", "get"}, {"/workbuddy-authorizations", "post"}, {"/workbuddy-authorizations/{id}", "get"}, {"/workbuddy-connections/{id}/refresh", "post"}, {"/workbuddy-connections/{id}/disconnect", "post"}} {
		op := mapAt(api, "paths", route[0], route[1])
		security := op["security"].([]any)
		if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
			t.Fatal("provider grant management must retain browser authority", route)
		}
		if route[1] == "get" && op["x-read-only"] != true {
			t.Fatal("reading connection state must not exchange or refresh tokens", route)
		}
	}
	for _, name := range []string{"WorkBuddyConnection", "WorkBuddyConnectionPage", "WorkBuddyAuthorization", "WorkBuddyAuthorizationRequest", "WorkBuddyRefreshRequest", "WorkBuddyDisconnectRequest"} {
		model := mapAt(api, "components", "schemas", name)
		if model["additionalProperties"] != false {
			t.Fatal("connection contracts must reject invented account or credential fields", name)
		}
		for _, key := range []string{"access_token", "refresh_token", "client_secret", "ciphertext", "credential_digest"} {
			if mapAt(model, "properties")[key] != nil {
				t.Fatal("a public connection contract exposed private credentials", name, key)
			}
		}
	}
	if mapAt(api, "components", "schemas", "WorkBuddyAuthorizationRequest", "properties", "scopes")["maxItems"] != 2 {
		t.Fatal("application scope requests must stay bounded to reviewed task permissions")
	}
}

func TestWorkBuddyCallbackContractKeepsPublicLandingAndAuthenticatedConsumptionDistinct(t *testing.T) {
	api, _ := generate()
	landing := mapAt(api, "paths", "/workbuddy-oauth/callback", "get")
	if len(landing["security"].([]any)) != 0 || landing["x-no-external-effects"] != true {
		t.Fatal("cross-site GET must be an isolated static landing, not a grant exchange")
	}
	for _, raw := range landing["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] != "header" || parameter["required"] == true {
			t.Fatal("static landing may negotiate language but cannot require query or identity authority")
		}
	}
	if mapAt(landing, "responses", "200", "content")["text/html"] == nil {
		t.Fatal("landing must not return codes or grant state as public JSON")
	}
	post := mapAt(api, "paths", "/workbuddy-oauth/callback", "post")
	if len(post["security"].([]any)) != 1 || mapAt(post, "x-browser-original-view")["required"] != true {
		t.Fatal("consumption must use the browser session and no query authority")
	}
	for _, raw := range post["parameters"].([]any) {
		if raw.(map[string]any)["in"] != "header" {
			t.Fatal("callback scope must come from the protected state, not a query or path")
		}
	}
	request := mapAt(api, "components", "schemas", "WorkBuddyCallbackRequest")
	if request["additionalProperties"] != false || len(request["oneOf"].([]any)) != 2 {
		t.Fatal("callback must distinguish an authorization code from an explicit denial")
	}
	for _, key := range []string{"state", "code", "error"} {
		if mapAt(request, "properties", key)["writeOnly"] != true {
			t.Fatal("sensitive callback fields must not become returned SDK state", key)
		}
	}
	result := mapAt(api, "components", "schemas", "WorkBuddyCallbackResult", "properties")
	for _, key := range []string{"code", "state_token", "access_token", "refresh_token", "authorization_url"} {
		if result[key] != nil {
			t.Fatal("callback result must only provide a safe route and current connection facts", key)
		}
	}
}
