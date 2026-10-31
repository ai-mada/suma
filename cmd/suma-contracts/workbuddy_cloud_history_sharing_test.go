package main

import (
	"reflect"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudHistorySharingContractsKeepExplicitSourceAndTarget(t *testing.T) {
	api, _ := generate()
	schemas, paths := mapAt(api, "components", "schemas"), mapAt(api, "paths")
	if models["CloudHistorySharedObservation"] != reflect.TypeOf(wc.CloudHistorySharedObservation{}) {
		t.Fatal("published provenance detached from durable DTO")
	}
	for _, name := range []string{"CloudHistorySharePreviewRequest", "CloudHistoryShareRequest", "CloudHistorySharePreview", "CloudHistorySharedEntry", "CloudHistorySharedObservation"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("open sharing contract", name)
		}
	}
	for _, name := range []string{"CloudHistorySharePreviewRequest", "CloudHistoryShareRequest"} {
		properties := mapAt(schemas, name, "properties")
		for _, forbidden := range []string{"body", "text", "author_id", "source", "token", "ticket", "url"} {
			if properties[forbidden] != nil {
				t.Fatal("caller can inject message or authority", name, forbidden)
			}
		}
		selection := mapAt(properties, "sequences")
		if selection["minItems"] != 1 || selection["maxItems"] != 8 || selection["uniqueItems"] != true {
			t.Fatal("exact bounded selection lost", name)
		}
	}
	for _, secret := range []string{"source", "request_sha256", "task_id", "operation_id", "owner_id", "ticket"} {
		if mapAt(schemas, "CloudHistorySharedObservation", "properties")[secret] != nil {
			t.Fatal("private source exposed in shared metadata", secret)
		}
	}
	p := mapAt(schemas, "TaskConversationMessage", "properties", "cloud_observation")
	if p["$ref"] != "#/components/schemas/CloudHistorySharedObservation" || p["anyOf"] != nil {
		t.Fatal("published source must be optional nonnull metadata")
	}
	for _, route := range []struct{ suffix, status, response string }{{"share-preview", "200", "CloudHistorySharePreview"}, {"shares", "201", "TaskConversationMessage"}} {
		op := mapAt(paths, "/workbuddy-cloud-history-loads/{id}/"+route.suffix, "post")
		if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) || op["x-no-external-effects"] != true {
			t.Fatal("sharing gained machine authority or provider I/O", route.suffix)
		}
		for _, raw := range op["parameters"].([]any) {
			if raw.(map[string]any)["in"] == "query" {
				t.Fatal("undeclared query input")
			}
		}
		if mapAt(op, "responses", route.status, "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.response {
			t.Fatal("incorrect message/preview success response")
		}
	}
}
