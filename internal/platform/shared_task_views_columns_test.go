package platform

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func assertSharedTaskViewColumns(t *testing.T, row map[string]any, want []string) {
	t.Helper()
	prefs := row["preferences"].(map[string]any)
	raw, exists := prefs["columns"]
	if want == nil {
		if exists {
			t.Fatalf("omitted columns were materialized: %#v", raw)
		}
		return
	}
	wire, err := json.Marshal(raw)
	var got []string
	if !exists || err != nil || json.Unmarshal(wire, &got) != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("columns order changed: got %#v, want %#v", raw, want)
	}
}

func TestSharedTaskViewColumnsHTTPPreservesOrderAndOriginalOperation(t *testing.T) {
	for _, visibility := range []string{"private", "project"} {
		t.Run(visibility, func(t *testing.T) {
			f := newFixture(t)
			columns := []string{"title", "updated", "owner", "status", "priority"}
			req := f.body(map[string]any{"name": "Ordered columns", "visibility": visibility, "preferences": map[string]any{"columns": columns, "layout": "board", "iteration_id": "exact-iteration", "group_by": "iteration"}})
			saved := f.call("POST", "/task-views", req, 201)
			id := saved["id"].(string)
			assertSharedTaskViewColumns(t, saved, columns)
			assertSharedTaskViewColumns(t, f.call("GET", "/task-views/"+id+"?project_id="+f.project, nil, 200), columns)
			page := f.call("GET", "/task-views?project_id="+f.project, nil, 200)
			assertSharedTaskViewColumns(t, page["items"].([]any)[0].(map[string]any), columns)
			// A lost response recovers the same object, order and original version.
			replayed := f.call("POST", "/task-views", req, 201)
			if replayed["id"] != id || replayed["version"] != float64(1) {
				t.Fatal("original creation was not recovered", replayed)
			}
			assertSharedTaskViewColumns(t, replayed, columns)
			update := f.body(map[string]any{"name": "Only title", "visibility": visibility, "expected_version": 1, "preferences": map[string]any{"columns": []string{"title"}, "iteration_unset": true, "group_by": "iteration"}})
			assertSharedTaskViewColumns(t, f.call("PATCH", "/task-views/"+id, update, 200), []string{"title"})
			assertSharedTaskViewColumns(t, f.call("PATCH", "/task-views/"+id, update, 200), []string{"title"})
			// Recovering the old create must not replace the more recent title-only view.
			assertSharedTaskViewColumns(t, f.call("POST", "/task-views", req, 201), columns)
			current := f.call("GET", "/task-views/"+id+"?project_id="+f.project, nil, 200)
			assertSharedTaskViewColumns(t, current, []string{"title"})
			if current["version"] != float64(2) || current["preferences"].(map[string]any)["iteration_unset"] != true {
				t.Fatal("column save lost current version or iteration", current)
			}
			// Equal members in another order are a different original command.
			req["preferences"].(map[string]any)["columns"] = []string{"title", "owner", "updated", "status", "priority"}
			f.call("POST", "/task-views", req, 409)
			update["preferences"].(map[string]any)["columns"] = []string{"title", "status"}
			f.call("PATCH", "/task-views/"+id, update, 409)
		})
	}
}

func TestSharedTaskViewColumnsRejectsInvalidHTTPShapes(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct{ name, preferences string }{
		{"null", `{"columns":null}`},
		{"empty", `{"columns":[]}`},
		{"duplicate", `{"columns":["title","status","status"]}`},
		{"unknown", `{"columns":["title","iteration"]}`},
		{"missing_title", `{"columns":["status"]}`},
		{"title_not_first", `{"columns":["status","title"]}`},
		{"oversized", `{"columns":["title","status","priority","owner","updated","status"]}`},
		{"number", `{"columns":["title",1]}`},
		{"null_item", `{"columns":["title",null]}`},
		{"string", `{"columns":"title"}`},
		{"case_alias", `{"Columns":["title"]}`},
		{"duplicate_key", `{"columns":["title"],"columns":["title","status"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(`{"project_id":%q,"name":"Invalid columns","visibility":"private","preferences":%s,"idempotency_key":%q}`, f.project, tc.preferences, newID())
			f.call("POST", "/task-views", json.RawMessage(raw), 400)
		})
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_shared_task_views`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid column preferences were persisted", count, err)
	}
}

func TestSharedTaskViewColumnsOmissionKeepsOriginalSerializedPayload(t *testing.T) {
	// This is the existing normalized request bytes before columns was added.
	const original = `{"project_id":"project-one","name":"My view","visibility":"private","preferences":{"q":"","status":"","owner_id":"","priority":"","sort":"updated_desc","layout":"list","group_by":"status"},"idempotency_key":"original-operation"}`
	var req SharedTaskViewCreateRequest
	if err := json.Unmarshal([]byte(original), &req); err != nil {
		t.Fatal(err)
	}
	var err error
	req.Preferences, err = normalizeSharedTaskViewPreferences(req.Preferences)
	if err != nil || stringify(req) != original {
		t.Fatal("omission changed the immutable command payload", stringify(req), err)
	}
	f := newFixture(t)
	body := f.body(map[string]any{"name": "Default columns", "visibility": "private"})
	saved := f.call("POST", "/task-views", body, 201)
	assertSharedTaskViewColumns(t, saved, nil)
	assertSharedTaskViewColumns(t, f.call("POST", "/task-views", body, 201), nil)
	assertSharedTaskViewColumns(t, f.call("GET", "/task-views/"+saved["id"].(string)+"?project_id="+f.project, nil, 200), nil)
}
