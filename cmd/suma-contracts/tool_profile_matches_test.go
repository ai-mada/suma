package main

import "testing"

func TestToolProfileMatchContractIsCompleteReadWithoutWriteAuthority(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/tool-profiles/matches", "post")
	if op["x-read-only"] != true || op["x-response-cache-control"] != "no-store" || op["x-disallowed-token-scopes"] == nil {
		t.Fatal("exact matching lost its current human read boundary")
	}
	if mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/ToolProfileMatchRequest" || mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/ToolProfileMatchList" {
		t.Fatal("query route does not use the shared closed request/complete result")
	}
	schemas := mapAt(api, "components", "schemas")
	request := mapAt(schemas, "ToolProfileMatchRequest")
	if request["additionalProperties"] != false || mapAt(request, "properties")["idempotency_key"] != nil || mapAt(request, "properties")["profile"] == nil {
		t.Fatal("read query grants an operation or accepts partial identity")
	}
	if mapAt(schemas, "ToolProfileMatchList", "properties")["next_cursor"] != nil || mapAt(schemas, "ToolProfileMatchList", "properties", "items")["maxItems"] != 1000 {
		t.Fatal("complete match set became paginated or unbounded")
	}
}
