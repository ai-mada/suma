package main

import (
	"strings"
	"testing"
)

func TestToolProfileReplacementContractKeepsExplicitIntentAndImmutableHistory(t *testing.T) {
	api, types := generate()
	paths := mapAt(api, "paths")
	post := mapAt(paths, "/tool-profiles/{id}/replacements", "post")
	if post["requestBody"] == nil || post["x-read-only"] == true || mapAt(post, "responses")["201"] == nil {
		t.Fatal("handoff lost its explicit write request or relationship receipt")
	}
	for _, path := range []string{"/tool-profiles/{id}/replacements", "/tool-profile-replacements/{id}"} {
		op := mapAt(paths, path, "get")
		if op["requestBody"] != nil || op["x-read-only"] != true || op["x-disallowed-token-scopes"] == nil {
			t.Fatal("history read gained effects or lost original browser authority")
		}
	}
	schemas := mapAt(api, "components", "schemas")
	req := mapAt(schemas, "ToolProfileReplacementRequest")
	for _, model := range []string{"ToolProfileReplacementRequest", "ToolProfileReplacementReuse"} {
		if mapAt(schemas, model, "properties", "expected_version")["maximum"] != 2147483647 {
			t.Fatal("published version range differs from the original ingressVersion gate")
		}
	}
	if req["additionalProperties"] != false || len(req["oneOf"].([]any)) != 2 {
		t.Fatal("create and explicit reuse must remain closed alternate shapes")
	}
	for _, property := range []string{"project_id", "expected_version", "worker_id", "profile", "pause_original", "idempotency_key"} {
		found := false
		for _, field := range req["required"].([]string) {
			found = found || property == field
		}
		if !found {
			t.Fatalf("handoff lost an explicit basis or false-valued choice: %s", property)
		}
	}
	list := mapAt(schemas, "ToolProfileReplacementList", "properties")
	if list["total"] != nil || list["items"] == nil || list["next_cursor"] == nil {
		t.Fatal("bounded history cannot claim a full total")
	}
	for _, field := range []string{"original", "replacement", "created_by", "created_at"} {
		if mapAt(schemas, "ToolProfileReplacement", "properties")[field] == nil {
			t.Fatalf("relationship lost its immutable source or committing actor: %s", field)
		}
	}
	if !strings.Contains(types, `disposition: "created" | "reused"`) || !strings.Contains(types, `direction: "outgoing" | "incoming"`) {
		t.Fatal("typed consumers lost the closed relationship vocabulary")
	}
}
