package main

import "testing"

func TestAutomationAndStreamContractsMatchDistinctFactAndAuthority(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, pair := range []struct{ path, method, model string }{
		{"/automations", "post", "AutomationRule"},
		{"/automations/{id}/commands", "post", "AutomationCommandResult"},
		{"/automations/preview", "post", "AutomationPreview"},
		{"/automations/{id}/occurrences", "get", "AutomationOccurrences"},
		{"/runs/{id}/automation-origin", "get", "AutomationRunOriginResult"},
	} {
		op := mapAt(paths, pair.path, pair.method)
		status := "200"
		if pair.path == "/automations" {
			status = "201"
		}
		if mapAt(op, "responses", status, "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+pair.model {
			t.Fatalf("wrong response %s", pair.path)
		}
		if op["x-disallowed-token-scopes"] == nil {
			t.Fatalf("missing human session gate %s", pair.path)
		}
	}
	stream := mapAt(paths, "/runs/{id}/events/stream", "get")
	if mapAt(stream, "responses", "200", "content", "text/event-stream", "schema")["type"] != "string" {
		t.Fatal("stream declared as JSON response")
	}
	if mapAt(stream, "x-event-schema")["$ref"] != "#/components/schemas/Event" {
		t.Fatal("stream loses shared durable Event contract")
	}
	if mapAt(stream, "responses", "429")["description"] == nil {
		t.Fatal("stream connection limit undocumented")
	}
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"AutomationSpec", "AutomationCreateRequest", "AutomationCommandRequest", "AutomationPreviewRequest"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatalf("unknown field rejection missing %s", name)
		}
	}
	if mapAt(schemas, "AutomationRule", "properties", "revision")["description"] == nil || mapAt(schemas, "AutomationCommandRequest", "properties", "action")["enum"] == nil {
		t.Fatal("revision and command state contract missing")
	}
}
