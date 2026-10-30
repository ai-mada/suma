package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSharedTaskViewCLIColumnsPreservesOriginalBodyAndIteration(t *testing.T) {
	for _, tc := range []struct{ name, preferences string }{
		{"ordered", `{"columns":["title","updated","owner"],"iteration_id":"exact-iteration","group_by":"iteration","layout":"board"}`},
		{"title_only", `{"columns":["title"],"iteration_unset":true,"group_by":"iteration"}`},
		{"omitted", `{"iteration_id":"exact-iteration"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			var want map[string]any
			if err := json.Unmarshal([]byte(tc.preferences), &want); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var got map[string]any
				if json.NewDecoder(r.Body).Decode(&got) != nil || !reflect.DeepEqual(got["preferences"], want) || got["idempotency_key"] != "original-operation" || got["expected_version"] != float64(7) || got["project_id"] != "project-one" {
					t.Error("original preference payload or operation changed", got)
				}
				if r.Method != "PATCH" || r.URL.Path != "/api/v1/task-views/view-one" || r.URL.RawQuery != "" {
					t.Error("wrong typed mutation route", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":"view-one","version":8}`))
			}))
			defer server.Close()
			c, _ := newClient(server.URL, "project-one", "", time.Second)
			body := `{"name":"Columns","visibility":"private","expected_version":7,"preferences":` + tc.preferences + `,"idempotency_key":"original-operation"}`
			if err := sharedTaskViewsCommand(context.Background(), c, []string{"update", "view-one", "--file", "-"}, strings.NewReader(body), &strings.Builder{}); err != nil || calls != 1 {
				t.Fatal("typed CLI failed to preserve columns and iteration", err, calls)
			}
		})
	}
}

func TestSharedTaskViewCLIColumnsRejectsAmbiguousShapesBeforeNetwork(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer server.Close()
	c, _ := newClient(server.URL, "project-one", "", time.Second)
	for _, preferences := range []string{
		`{"columns":null}`, `{"columns":[]}`, `{"columns":["title","owner","owner"]}`,
		`{"columns":["title","unknown"]}`, `{"columns":["owner","title"]}`,
		`{"columns":["title","status","priority","owner","updated","status"]}`,
		`{"columns":["title",1]}`, `{"columns":["title",null]}`,
		`{"Columns":["title"]}`, `{"columns":["title"],"columns":["title","owner"]}`,
	} {
		body := `{"name":"Columns","visibility":"private","preferences":` + preferences + `,"idempotency_key":"original-operation"}`
		if err := sharedTaskViewsCommand(context.Background(), c, []string{"create", "--file", "-"}, strings.NewReader(body), &strings.Builder{}); err == nil {
			t.Fatal("accepted invalid columns", preferences)
		}
	}
	if calls != 0 {
		t.Fatal("invalid columns reached the network", calls)
	}
}
