package main

import "testing"

func TestAttemptToolHistoryContractKeepsOpaqueWindowAndRawEvents(t *testing.T) {
	api, wire := generate()
	const path = "/runs/{id}/nodes/{node}/attempts/{attempt}/tool-events"
	paths := mapAt(api, "paths")
	if paths[path] == nil {
		t.Fatal("missing exact-attempt durable tool event route")
	}
	op := mapAt(paths, path, "get")
	if op["x-disallowed-token-scopes"] == nil || op["x-strict-query"] == nil {
		t.Fatal("current authorization and strict read query are not described")
	}
	response := mapAt(op, "responses", "200", "content", "application/json", "schema")
	if response["$ref"] != "#/components/schemas/AttemptToolEventPage" {
		t.Fatal("attempt history must not use run-wide numeric cursor envelope", response)
	}
	schemas := mapAt(api, "components", "schemas")
	props := mapAt(schemas, "AttemptToolEventPage", "properties")
	for _, name := range []string{"schema_version", "project_id", "run_id", "node_id", "attempt", "fence", "worker_id", "items", "watermark", "scanned_through", "has_more", "page_cursor", "next_cursor"} {
		if props[name] == nil {
			t.Fatal("missing page identity/window field", name)
		}
	}
	if props["total"] != nil || mapAt(props, "next_cursor")["type"] != "string" || mapAt(props, "page_cursor")["type"] != "string" {
		t.Fatal("opaque positions are not totals or numeric run cursors")
	}
	if mapAt(props, "items", "items")["$ref"] != "#/components/schemas/Event" || mapAt(props, "items")["maxItems"] != 100 {
		t.Fatal("history must retain bounded original parent Event envelopes")
	}
	if mapAt(schemas, "Event", "properties")["raw_data"] == nil || mapAt(props, "items", "items")["required"] == nil {
		t.Fatal("history must include original stored data separately from the compatible parsed projection")
	}
	if mapAt(props, "schema_version")["const"] != 1 || mapAt(props, "watermark")["maximum"] != int64(9007199254740991) {
		t.Fatal("version and safe numeric window are not constrained")
	}
	if mapAt(schemas, "AttemptToolEventPage")["allOf"] == nil || wire == "" {
		t.Fatal("continuation relationship or generated types missing")
	}
}
