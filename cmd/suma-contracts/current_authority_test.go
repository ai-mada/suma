package main

import "testing"

func TestCurrentAuthorityContractsDistinguishBrowserViewAndScopedAPI(t *testing.T) {
	api, _ := generate()
	for _, tc := range []struct {
		path, method string
		required     bool
	}{{"/tasks", "post", false}, {"/teams", "post", true}, {"/auth/tokens", "post", true}} {
		op := mapAt(api, "paths", tc.path, tc.method)
		if mapAt(op, "x-browser-original-view")["missing_status"] != 428 {
			t.Fatal("missing view contract", tc.path)
		}
		count := 0
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "X-Suma-Expected-User" || p["name"] == "X-Suma-Expected-Organization" {
				count++
				if p["required"] != tc.required {
					t.Fatal("scope API wrongly requires browser view", tc.path, p)
				}
			}
		}
		if count != 2 {
			t.Fatal("missing or duplicate view headers", tc.path, count)
		}
	}
	for _, path := range []string{"/workflows/validate", "/runner/claim", "/auth/logout", "/agent/tools/invoke"} {
		if mapAt(api, "paths", path, "post")["x-browser-original-view"] != nil {
			t.Fatal("read/machine/withdraw route gained browser view", path)
		}
	}
	input := mapAt(api, "components", "schemas", "ProjectCredentialIssueRequest")
	if input["additionalProperties"] != false {
		t.Fatal("unbounded issuance source override")
	}
	found := false
	for _, key := range input["required"].([]string) {
		if key == "idempotency_key" {
			found = true
		}
	}
	if !found {
		t.Fatal("credential issuance requires stable operation key")
	}
}
