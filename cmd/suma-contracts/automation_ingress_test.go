package main

import "testing"

func TestIngressContractsKeepSignatureAndSessionAuthoritySeparate(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	if paths["/hooks/{id}"] == nil {
		t.Fatal("missing signed receiver contract")
	}
	op := mapAt(paths, "/hooks/{id}", "post")
	for _, value := range op["parameters"].([]any) {
		if value.(map[string]any)["name"] == "project_id" {
			t.Fatal("receiver accepts caller-controlled project")
		}
	}
	security := op["security"].([]any)
	if len(security) != 2 || security[0].(map[string]any)["githubSignature"] == nil || security[1].(map[string]any)["sumaSignature"] == nil {
		t.Fatal("receiver signature methods missing or session credentials advertised")
	}
	if mapAt(op, "responses", "202", "content", "application/json", "schema")["$ref"] != "#/components/schemas/AutomationIngressReceipt" {
		t.Fatal("receiver should expose only minimal persisted receipt")
	}
	for _, path := range []string{"/automations/{id}/ingresses", "/automation-ingresses/{id}/commands", "/automation-ingresses/{id}/deliveries", "/automation-ingresses/{id}/revisions"} {
		method := "get"
		if path == "/automation-ingresses/{id}/commands" {
			method = "post"
		}
		if mapAt(paths, path, method)["x-disallowed-token-scopes"] == nil {
			t.Fatalf("management lacks human session gate %s", path)
		}
	}
	schemas := mapAt(api, "components", "schemas")
	if mapAt(schemas, "AutomationIngressCommandRequest", "properties", "secret")["writeOnly"] != true {
		t.Fatal("rotation secret is not write-only")
	}
	if mapAt(schemas, "AutomationIngressEvent")["additionalProperties"] != false {
		t.Fatal("generic event input is unconstrained")
	}
	for _, property := range []string{"secret", "key_ciphertext", "raw_body"} {
		if mapAt(schemas, "AutomationIngress", "properties")[property] != nil || mapAt(schemas, "AutomationIngressDelivery", "properties")[property] != nil {
			t.Fatal("secret or raw payload leaked into public schema")
		}
	}
}
